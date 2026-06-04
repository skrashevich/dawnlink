package handlers

import "testing"

func TestWorkflowImportEscapesBranchWithSlash(t *testing.T) {
	got, ok := matchGitHubImport("/owner/repo/blob/feature/branch/.github/workflows/build.yml")
	if !ok {
		t.Fatal("workflow URL was not recognized")
	}
	want := "/owner/repo/workflows/build/feature%2Fbranch?preview"
	if got != want {
		t.Fatalf("matchGitHubImport() = %q, want %q", got, want)
	}
}
