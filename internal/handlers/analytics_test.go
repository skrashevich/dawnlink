package handlers

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/skrashevich/dawnlink/internal/config"
	"github.com/skrashevich/dawnlink/internal/db"
	"github.com/skrashevich/dawnlink/internal/i18n"
	"github.com/skrashevich/dawnlink/internal/render"
)

func TestDownloadAnalyticsDisabledByDefault(t *testing.T) {
	srv := analyticsTestServer(t, config.Config{
		GitHubAppID:   1,
		GitHubPEMPath: "x",
		AppSecret:     "01234567890123456789012345678901",
		BaseURL:       "http://localhost:8080/",
		PublicURLs:    []string{"http://localhost:8080/"},
	})
	req := httptest.NewRequest(http.MethodGet, "/analytics/downloads", nil)
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", rec.Code)
	}
}

func TestDownloadAnalyticsURL(t *testing.T) {
	cfg := config.Config{
		BaseURL:                  "http://localhost:8080/",
		PublicURLs:               []string{"http://localhost:8080/"},
		GitHubClientID:           "client",
		GitHubClientSecret:       "secret",
		DownloadAnalyticsCollect: true,
		DownloadAnalyticsView:    true,
	}
	srv := analyticsTestServer(t, cfg)
	req := httptest.NewRequest(http.MethodGet, "/dashboard", nil)
	got := srv.downloadAnalyticsURL(req)
	want := "http://localhost:8080/analytics/downloads"
	if got != want {
		t.Fatalf("downloadAnalyticsURL() = %q, want %q", got, want)
	}
}

func TestDownloadAnalyticsAdminInstanceScope(t *testing.T) {
	cfg := config.Config{
		GitHubAppID:                  1,
		GitHubPEMPath:                "x",
		AppSecret:                    "01234567890123456789012345678901",
		BaseURL:                      "http://localhost:8080/",
		PublicURLs:                   []string{"http://localhost:8080/"},
		DownloadAnalyticsCollect:     true,
		DownloadAnalyticsView:        true,
		DownloadAnalyticsAdminSecret: "owner-admin-secret-32chars-min",
	}
	srv := analyticsTestServer(t, cfg)

	if err := srv.store.Write(&db.RepoInstallation{
		RepoOwner:      "acme",
		InstallationID: 1,
		PublicRepos:    db.NewDelimitedSet("app"),
	}); err != nil {
		t.Fatal(err)
	}
	if err := srv.store.RecordDownload(db.DownloadRecord{
		RouteKind: db.RouteArtifact, Owner: "acme", Repo: "app", ArtifactName: "mine",
	}); err != nil {
		t.Fatal(err)
	}
	if err := srv.store.RecordDownload(db.DownloadRecord{
		RouteKind: db.RouteArtifact, Owner: "other", Repo: "lib", ArtifactName: "theirs",
	}); err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodGet, "/analytics/downloads?token=owner-admin-secret-32chars-min", nil)
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body: %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "Instance owner") && !strings.Contains(rec.Body.String(), "Владелец инстанса") {
		t.Fatal("expected instance owner scope label")
	}
	if strings.Contains(rec.Body.String(), "theirs") {
		t.Fatal("instance view must not include downloads outside connected repos")
	}
}

func TestDownloadAnalyticsWrongAdminToken(t *testing.T) {
	cfg := config.Config{
		GitHubAppID:                  1,
		GitHubPEMPath:                "x",
		AppSecret:                    "01234567890123456789012345678901",
		BaseURL:                      "http://localhost:8080/",
		PublicURLs:                   []string{"http://localhost:8080/"},
		DownloadAnalyticsCollect:     true,
		DownloadAnalyticsView:        true,
		DownloadAnalyticsAdminSecret: "owner-admin-secret-32chars-min",
	}
	srv := analyticsTestServer(t, cfg)
	req := httptest.NewRequest(http.MethodGet, "/analytics/downloads?token=wrong", nil)
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", rec.Code)
	}
}

func TestDownloadAnalyticsRequiresOAuth(t *testing.T) {
	cfg := config.Config{
		GitHubAppID:              1,
		GitHubPEMPath:            "x",
		AppSecret:                "01234567890123456789012345678901",
		BaseURL:                  "http://localhost:8080/",
		PublicURLs:               []string{"http://localhost:8080/"},
		GitHubClientID:           "client",
		GitHubClientSecret:       "secret",
		DownloadAnalyticsCollect: true,
		DownloadAnalyticsView:    true,
	}
	srv := analyticsTestServer(t, cfg)

	req := httptest.NewRequest(http.MethodGet, "/analytics/downloads", nil)
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)
	if rec.Code != http.StatusFound {
		t.Fatalf("status = %d, want 302 to GitHub OAuth", rec.Code)
	}
	loc := rec.Header().Get("Location")
	if !strings.HasPrefix(loc, "https://github.com/login/oauth/authorize") {
		t.Fatalf("Location = %q", loc)
	}
	if !strings.Contains(loc, "state=%2Fanalytics%2Fdownloads") && !strings.Contains(loc, "state=/analytics/downloads") {
		t.Fatalf("OAuth state for analytics missing in %q", loc)
	}
}

func analyticsTestServer(t *testing.T, cfg config.Config) *Server {
	t.Helper()
	dir := t.TempDir()
	store, err := db.Open(filepath.Join(dir, "test.db"), cfg.AppSecret, nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	bundle, err := i18n.Load("en")
	if err != nil {
		t.Fatal(err)
	}
	eng, err := render.New(bundle, cfg.BaseURL)
	if err != nil {
		t.Fatal(err)
	}
	return New(cfg, store, nil, eng, bundle)
}
