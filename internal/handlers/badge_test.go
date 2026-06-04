package handlers

import (
	"strings"
	"testing"

	"github.com/skrashevich/dawnlink/internal/github"
)

func TestBadgeStatus(t *testing.T) {
	tests := []struct {
		status, conclusion, wantLabel, wantColor string
	}{
		{"completed", "success", "passing", "#3d9b6a"},
		{"completed", "failure", "failing", "#f07178"},
		{"in_progress", "", "running", "#e8a54b"},
		{"completed", "cancelled", "cancelled", "#7a8a9e"},
		{"", "", "unknown", "#7a8a9e"},
	}
	for _, tc := range tests {
		label, color := badgeStatus(tc.status, tc.conclusion)
		if label != tc.wantLabel || color != tc.wantColor {
			t.Fatalf("badgeStatus(%q, %q) = (%q, %q), want (%q, %q)", tc.status, tc.conclusion, label, color, tc.wantLabel, tc.wantColor)
		}
	}
}

func TestBadgeFromRun(t *testing.T) {
	info := badgeFromRun(&github.WorkflowRun{Status: "completed", Conclusion: "success"}, "")
	if info.rightLabel != "passing" || info.leftLabel != "build" {
		t.Fatalf("badgeFromRun() = %+v", info)
	}
	info = badgeFromRun(nil, "ci")
	if info.rightLabel != "no builds" || info.leftLabel != "ci" {
		t.Fatalf("badgeFromRun(nil) = %+v", info)
	}
}

func TestRenderBadgeSVG(t *testing.T) {
	svg := string(renderBadgeSVG(badgeInfo{
		leftLabel:  "build",
		rightLabel: "passing",
		rightColor: "#3d9b6a",
		title:      "build: passing",
	}))
	if !strings.Contains(svg, `aria-label="build: passing"`) {
		t.Fatalf("missing aria-label: %s", svg)
	}
	if !strings.Contains(svg, ">passing<") {
		t.Fatalf("missing status text: %s", svg)
	}
}

func TestWorkflowBadgeRouteMatch(t *testing.T) {
	re := routePatterns.workflowBadge
	match := routeMatch(re, "/owner/repo/workflows/binaries/main/badge.svg")
	if match == nil || match[1] != "owner" || match[4] != "main" {
		t.Fatalf("routeMatch() = %#v", match)
	}
}
