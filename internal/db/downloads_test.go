package db

import (
	"path/filepath"
	"testing"
)

func TestRecordDownloadAndSummary(t *testing.T) {
	dir := t.TempDir()
	store, err := Open(filepath.Join(dir, "test.db"), "01234567890123456789012345678901", nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	if err := store.RecordDownload(DownloadRecord{
		RouteKind:    RouteWorkflow,
		Owner:        "acme",
		Repo:         "app",
		ArtifactName: "build-linux",
		Workflow:     "ci.yml",
		Branch:       "main",
		RunID:        42,
		StatusFilter: "success",
		PrivateLink:  true,
		ClientIPHash: HashClientIP("01234567890123456789012345678901", "203.0.113.1"),
	}); err != nil {
		t.Fatal(err)
	}
	if err := store.RecordDownload(DownloadRecord{
		RouteKind:    RouteRun,
		Owner:        "acme",
		Repo:         "app",
		ArtifactName: "build-linux",
		RunID:        42,
	}); err != nil {
		t.Fatal(err)
	}

	sum, err := store.DownloadAnalyticsSummary([]RepoRef{{Owner: "acme", Repo: "app"}}, 10)
	if err != nil {
		t.Fatal(err)
	}
	if sum.TotalAll != 2 {
		t.Fatalf("TotalAll = %d, want 2", sum.TotalAll)
	}
	if sum.PrivateCount != 1 {
		t.Fatalf("PrivateCount = %d, want 1", sum.PrivateCount)
	}
	if len(sum.TopRepos) == 0 || sum.TopRepos[0].Label != "acme/app" {
		t.Fatalf("TopRepos = %#v", sum.TopRepos)
	}
	if len(sum.Recent) != 2 {
		t.Fatalf("Recent len = %d, want 2", len(sum.Recent))
	}
}

func TestPurgeDownloadEventsOlderThan(t *testing.T) {
	dir := t.TempDir()
	store, err := Open(filepath.Join(dir, "test.db"), "01234567890123456789012345678901", nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	_, err = store.db.Exec(`INSERT INTO download_events (created_at, route_kind, owner, repo, artifact_name)
VALUES ('2000-01-01T00:00:00Z', 'artifact', 'o', 'r', 'old')`)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.RecordDownload(DownloadRecord{
		RouteKind: RouteArtifact, Owner: "o", Repo: "r", ArtifactName: "new",
	}); err != nil {
		t.Fatal(err)
	}
	n, err := store.PurgeDownloadEventsOlderThan(30)
	if err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Fatalf("purged %d rows, want 1", n)
	}
	sum, err := store.DownloadAnalyticsSummary([]RepoRef{{Owner: "o", Repo: "r"}}, 5)
	if err != nil {
		t.Fatal(err)
	}
	if sum.TotalAll != 1 {
		t.Fatalf("TotalAll after purge = %d, want 1", sum.TotalAll)
	}
}

func TestDownloadAnalyticsSummaryRepoScope(t *testing.T) {
	dir := t.TempDir()
	store, err := Open(filepath.Join(dir, "test.db"), "01234567890123456789012345678901", nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	if err := store.RecordDownload(DownloadRecord{RouteKind: RouteArtifact, Owner: "mine", Repo: "app", ArtifactName: "a"}); err != nil {
		t.Fatal(err)
	}
	if err := store.RecordDownload(DownloadRecord{RouteKind: RouteArtifact, Owner: "other", Repo: "app", ArtifactName: "b"}); err != nil {
		t.Fatal(err)
	}

	mine, err := store.DownloadAnalyticsSummary([]RepoRef{{Owner: "mine", Repo: "app"}}, 10)
	if err != nil {
		t.Fatal(err)
	}
	if mine.TotalAll != 1 {
		t.Fatalf("scoped TotalAll = %d, want 1", mine.TotalAll)
	}
}

func TestHashClientIPStable(t *testing.T) {
	secret := "01234567890123456789012345678901"
	a := HashClientIP(secret, "198.51.100.2")
	b := HashClientIP(secret, "198.51.100.2")
	if a == "" || a != b {
		t.Fatalf("hash = %q %q", a, b)
	}
	if HashClientIP(secret, "198.51.100.3") == a {
		t.Fatal("different IPs produced same hash")
	}
}
