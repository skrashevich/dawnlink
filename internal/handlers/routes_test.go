package handlers

import (
	"regexp"
	"testing"
)

func TestArtifactURL(t *testing.T) {
	got := artifactURL("https://example.com/artifact?preview=", "private-token", "completed")
	want := "https://example.com/artifact?h=private-token&preview=&status=completed"
	if got != want {
		t.Fatalf("artifactURL() = %q, want %q", got, want)
	}
}

func TestArtifactURLDoesNotDuplicateToken(t *testing.T) {
	got := artifactURL("https://example.com/artifact?h=old", "new", "success")
	want := "https://example.com/artifact?h=new"
	if got != want {
		t.Fatalf("artifactURL() = %q, want %q", got, want)
	}
}

func TestRouteMatchSupportsSlashInBranch(t *testing.T) {
	re := regexp.MustCompile(`^/([^/]+)/([^/]+)/workflows/([^/]+)/([^/]+)$`)
	match := routeMatch(re, "/owner/repo/workflows/build/feature%2Fbranch")
	if match == nil || match[4] != "feature/branch" {
		t.Fatalf("routeMatch() = %#v", match)
	}
}
