package handlers

import (
	"bytes"
	"fmt"
	"html"
	"net/http"
	"strings"
	"sync"

	"github.com/skrashevich/dawnlink/internal/github"
)

type badgeInfo struct {
	leftLabel  string
	rightLabel string
	rightColor string
	title      string
}

func badgeFromRun(run *github.WorkflowRun, leftLabel string) badgeInfo {
	if leftLabel == "" {
		leftLabel = "build"
	}
	if run == nil {
		return badgeInfo{
			leftLabel:  leftLabel,
			rightLabel: "no builds",
			rightColor: "#7a8a9e",
			title:      leftLabel + ": no builds",
		}
	}
	rightLabel, rightColor := badgeStatus(run.Status, run.Conclusion)
	return badgeInfo{
		leftLabel:  leftLabel,
		rightLabel: rightLabel,
		rightColor: rightColor,
		title:      fmt.Sprintf("%s: %s", leftLabel, rightLabel),
	}
}

func badgeStatus(status, conclusion string) (label, color string) {
	switch status {
	case "queued", "in_progress", "waiting", "requested", "pending":
		return "running", "#e8a54b"
	case "completed":
		switch conclusion {
		case "success":
			return "passing", "#3d9b6a"
		case "failure":
			return "failing", "#f07178"
		case "cancelled":
			return "cancelled", "#7a8a9e"
		case "skipped":
			return "skipped", "#7a8a9e"
		case "timed_out":
			return "timed out", "#f07178"
		case "action_required":
			return "action required", "#e8a54b"
		case "neutral":
			return "neutral", "#7a8a9e"
		case "stale":
			return "stale", "#7a8a9e"
		case "":
			return "completed", "#7a8a9e"
		default:
			return conclusion, "#7a8a9e"
		}
	default:
		if status == "" {
			return "unknown", "#7a8a9e"
		}
		return status, "#7a8a9e"
	}
}

func renderBadgeSVG(info badgeInfo) []byte {
	left := html.EscapeString(info.leftLabel)
	right := html.EscapeString(info.rightLabel)
	title := html.EscapeString(info.title)

	leftWidth := badgeTextWidth(info.leftLabel) + 10
	rightWidth := badgeTextWidth(info.rightLabel) + 10
	totalWidth := leftWidth + rightWidth

	var buf bytes.Buffer
	fmt.Fprintf(&buf, `<svg xmlns="http://www.w3.org/2000/svg" width="%d" height="20" role="img" aria-label="%s">`, totalWidth, title)
	fmt.Fprintf(&buf, `<title>%s</title>`, title)
	fmt.Fprintf(&buf, `<g shape-rendering="crispEdges">`)
	fmt.Fprintf(&buf, `<rect width="%d" height="20" fill="#555"/>`, leftWidth)
	fmt.Fprintf(&buf, `<rect x="%d" width="%d" height="20" fill="%s"/>`, leftWidth, rightWidth, info.rightColor)
	fmt.Fprintf(&buf, `</g>`)
	fmt.Fprintf(&buf, `<g fill="#fff" text-anchor="middle" font-family="Verdana,Geneva,DejaVu Sans,sans-serif" text-rendering="geometricPrecision" font-size="11">`)
	fmt.Fprintf(&buf, `<text x="%.1f" y="14" fill="#010101" fill-opacity=".3">%s</text>`, float64(leftWidth)/2, left)
	fmt.Fprintf(&buf, `<text x="%.1f" y="13">%s</text>`, float64(leftWidth)/2, left)
	fmt.Fprintf(&buf, `<text x="%.1f" y="14" fill="#010101" fill-opacity=".3">%s</text>`, float64(leftWidth)+float64(rightWidth)/2, right)
	fmt.Fprintf(&buf, `<text x="%.1f" y="13">%s</text>`, float64(leftWidth)+float64(rightWidth)/2, right)
	fmt.Fprintf(&buf, `</g></svg>`)
	return buf.Bytes()
}

func badgeTextWidth(text string) int {
	width := 0
	for _, r := range text {
		switch {
		case r == ' ':
			width += 4
		case r >= 'A' && r <= 'Z':
			width += 7
		default:
			width += 6
		}
	}
	return width
}

func (s *Server) badgeByBranch(w http.ResponseWriter, r *http.Request, owner, repo, workflow, branch string) error {
	h := r.URL.Query().Get("h")
	token, pw, err := s.store.VerifiedToken(owner, repo, h)
	if err != nil {
		return s.serveBadge(w, badgeFromRun(nil, badgeLabelParam(r)))
	}
	if pw != "" {
		h = pw
	}
	workflow = s.normalizeWorkflow(workflow)
	label := badgeLabelParam(r)
	cacheKey := fmt.Sprintf("badge:%s/%s/%s/%s/%s", strings.ToLower(owner), strings.ToLower(repo), workflow, branch, h)

	info, err := s.badgeCache.Get(cacheKey, func() (badgeInfo, error) {
		run, runErr := s.getLatestRunAny(owner, repo, workflow, branch, token)
		if runErr != nil {
			if he, ok := runErr.(*httpError); ok && he.status == 404 {
				return badgeFromRun(nil, label), nil
			}
			return badgeInfo{}, runErr
		}
		return badgeFromRun(run, label), nil
	})
	if err != nil {
		return s.serveBadge(w, badgeFromRun(nil, label))
	}

	w.Header().Set("Content-Type", "image/svg+xml; charset=utf-8")
	w.Header().Set("Cache-Control", "public, max-age=300")
	if h != "" {
		w.Header().Set("X-Robots-Tag", "noindex, nofollow")
	}
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(renderBadgeSVG(info))
	return nil
}

func badgeLabelParam(r *http.Request) string {
	return strings.TrimSpace(r.URL.Query().Get("label"))
}

func (s *Server) serveBadge(w http.ResponseWriter, info badgeInfo) error {
	w.Header().Set("Content-Type", "image/svg+xml; charset=utf-8")
	w.Header().Set("Cache-Control", "public, max-age=60")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(renderBadgeSVG(info))
	return nil
}

func (s *Server) getLatestRunAny(owner, repo, workflow, branch string, token github.Token) (*github.WorkflowRun, error) {
	type result struct {
		run *github.WorkflowRun
		err error
	}
	ch := make(chan result, 2)
	events := []string{"push", "schedule"}
	var wg sync.WaitGroup
	for _, event := range events {
		wg.Go(func() {
			runs, err := github.ListWorkflowRuns(owner, repo, workflow, branch, event, "", token, 1)
			if err != nil {
				if github.IsNotFound(err) {
					ch <- result{err: &httpError{status: 404, message: fmt.Sprintf("Repository '%s/%s' or workflow '%s' not found.", owner, repo, workflow)}}
					return
				}
				ch <- result{err: err}
				return
			}
			if len(runs) > 0 {
				ch <- result{run: &runs[0]}
			}
		})
	}
	wg.Wait()
	close(ch)
	var runs []*github.WorkflowRun
	for res := range ch {
		if res.err != nil {
			if he, ok := res.err.(*httpError); ok {
				return nil, he
			}
			return nil, res.err
		}
		if res.run != nil {
			runs = append(runs, res.run)
		}
	}
	if len(runs) == 0 {
		return nil, nil
	}
	var best *github.WorkflowRun
	for _, run := range runs {
		if best == nil || run.UpdatedAt.After(best.UpdatedAt) {
			best = run
		}
	}
	return best, nil
}
