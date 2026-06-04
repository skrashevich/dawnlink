package db

import "testing"

func TestDelimitedSetStringIsStable(t *testing.T) {
	set := NewDelimitedSet("Zulu,alpha,Bravo")
	if got, want := set.String(), "alpha,bravo,zulu"; got != want {
		t.Fatalf("String() = %q, want %q", got, want)
	}
}

func TestPrivateRepoPasswordVerification(t *testing.T) {
	store := &Store{appSecret: "a-long-random-secret"}
	inst := &RepoInstallation{
		RepoOwner:      "owner",
		InstallationID: 42,
		PublicRepos:    NewDelimitedSet("public"),
		PrivateRepos:   NewDelimitedSet("private"),
	}
	password := store.Password(inst, "private")
	if _, err := store.Verify(inst, "private", password); err != nil {
		t.Fatalf("valid password rejected: %v", err)
	}
	if _, err := store.Verify(inst, "private", "wrong"); err == nil {
		t.Fatal("invalid password accepted")
	}
}
