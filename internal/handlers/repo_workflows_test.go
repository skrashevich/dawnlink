package handlers

import (
	"fmt"
	"net/http"
	"strings"
	"testing"
)

func TestWorkflowShortName(t *testing.T) {
	tests := map[string]string{
		".github/workflows/binaries.yml": "binaries",
		".github/workflows/ci.yaml":    "ci",
		"build.yml":                      "build",
	}
	for path, want := range tests {
		if got := workflowShortName(path); got != want {
			t.Fatalf("workflowShortName(%q) = %q, want %q", path, got, want)
		}
	}
}

func TestWorkflowBadgeLinks(t *testing.T) {
	s := &Server{}
	r, _ := http.NewRequest(http.MethodGet, "https://dawnl.ink/skrashevich/enkapp", nil)
	badge, target, md := workflowBadgeLinks(s, r, "skrashevich", "enkapp", "ci", "main", "")
	if !strings.Contains(badge, "/skrashevich/enkapp/workflows/ci/main/badge.svg") {
		t.Fatalf("badge URL = %q", badge)
	}
	if !strings.Contains(target, "/skrashevich/enkapp/workflows/ci/main") {
		t.Fatalf("target URL = %q", target)
	}
	if md != fmt.Sprintf("[![build](%s)](%s)", badge, target) {
		t.Fatalf("markdown = %q", md)
	}
}

func TestRepoRootRoute(t *testing.T) {
	m := routeMatch(routePatterns.repoRoot, "/owner/my-repo")
	if m == nil {
		t.Fatal("repo root route did not match")
	}
	if m[1] != "owner" || m[2] != "my-repo" {
		t.Fatalf("match = %#v", m)
	}
	if routeMatch(routePatterns.repoRoot, "/owner/repo/workflows/ci/main") != nil {
		t.Fatal("workflow path should not match repo root")
	}
}
