package handlers

import (
	"testing"

	"github.com/skrashevich/dawnlink/internal/github"
)

func TestPickExampleArtifact(t *testing.T) {
	arts := []github.Artifact{
		{ID: 1, Name: "other"},
		{ID: 2, Name: "wanted"},
	}
	got := pickExampleArtifact(arts, "wanted")
	if got.ID != 2 {
		t.Fatalf("pickExampleArtifact() = %+v, want id 2", got)
	}
	got = pickExampleArtifact(arts, "missing")
	if got.ID != 1 {
		t.Fatalf("pickExampleArtifact() fallback = %+v, want id 1", got)
	}
}

func TestApplyIndexExampleExtras(t *testing.T) {
	ex := workflowExample{owner: "o", repo: "r"}
	ids := indexExampleIDs{CheckSuiteID: 10, ArtifactID: 20, RunID: 30, JobID: 40}
	extra := map[string]any{}
	applyIndexExampleExtras(ex, ids, func(path string) string { return "https://dawn.test" + path }, extra)
	if extra["ExampleArtifactDest"] != "https://dawn.test/o/r/actions/artifacts/20" {
		t.Fatalf("artifact dest = %v", extra["ExampleArtifactDest"])
	}
	if extra["ExampleJobDest"] != "https://dawn.test/o/r/runs/40" {
		t.Fatalf("job dest = %v", extra["ExampleJobDest"])
	}
}
