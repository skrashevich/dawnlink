package db

import (
	"path/filepath"
	"testing"
)

func TestAllInstanceRepoRefs(t *testing.T) {
	dir := t.TempDir()
	store, err := Open(filepath.Join(dir, "test.db"), "01234567890123456789012345678901", nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	if err := store.Write(&RepoInstallation{
		RepoOwner:      "acme",
		InstallationID: 1,
		PublicRepos:    NewDelimitedSet("app,web"),
		PrivateRepos:   NewDelimitedSet("secrets"),
	}); err != nil {
		t.Fatal(err)
	}

	refs, err := store.AllInstanceRepoRefs()
	if err != nil {
		t.Fatal(err)
	}
	if len(refs) != 3 {
		t.Fatalf("len(refs) = %d, want 3", len(refs))
	}
}
