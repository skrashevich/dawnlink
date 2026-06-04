package handlers

import (
	"fmt"
	"net/http"
	"sort"
	"strings"

	"github.com/skrashevich/dawnlink/internal/github"
	"github.com/skrashevich/dawnlink/internal/render"
)

type repoWorkflowLink struct {
	Title          string
	URL            string
	Path           string
	BadgeURL       string
	BadgeTargetURL string
	BadgeMarkdown  string
}

func (s *Server) repoWorkflows(w http.ResponseWriter, r *http.Request, owner, repo string) error {
	h := r.URL.Query().Get("h")
	token, pw, err := s.store.VerifiedToken(owner, repo, h)
	if err != nil {
		return &httpError{status: 404, message: s.t(r, "repo_not_found_private")}
	}
	if pw != "" {
		h = pw
		w.Header().Set("X-Robots-Tag", "noindex")
	}

	repository, err := github.GetRepository(owner, repo, token)
	if err != nil {
		if github.IsNotFound(err) {
			return &httpError{status: 404, message: s.t(r, "repo_not_found_private")}
		}
		return err
	}
	branch := repository.DefaultBranch
	if branch == "" {
		branch = "main"
	}

	workflows, err := github.ListWorkflows(owner, repo, token)
	if err != nil {
		if github.IsNotFound(err) {
			return &httpError{status: 404, message: s.t(r, "repo_not_found_private")}
		}
		return err
	}

	var links []repoWorkflowLink
	for _, wf := range workflows {
		if wf.State != "" && wf.State != "active" {
			continue
		}
		short := workflowShortName(wf.Path)
		if short == "" {
			continue
		}
		u := s.abs(r, fmt.Sprintf("/%s/%s/workflows/%s/%s", owner, repo, pathComponent(short), pathComponent(branch)))
		u = artifactURL(u+"?preview", h, "")
		badgeURL, badgeTarget, badgeMD := workflowBadgeLinks(s, r, owner, repo, short, branch, h)
		title := short
		if wf.Name != "" && !strings.EqualFold(wf.Name, short) {
			title = fmt.Sprintf("%s (%s)", wf.Name, short)
		}
		links = append(links, repoWorkflowLink{
			Title:          title,
			URL:            u,
			Path:           wf.Path,
			BadgeURL:       badgeURL,
			BadgeTargetURL: badgeTarget,
			BadgeMarkdown:  badgeMD,
		})
	}
	sort.Slice(links, func(i, j int) bool {
		return strings.ToLower(links[i].Title) < strings.ToLower(links[j].Title)
	})

	canonical := artifactURL(s.abs(r, fmt.Sprintf("/%s/%s", owner, repo)), h, "")
	return s.renderPage(w, r, "repo_workflows.html", render.PageData{
		Title:     fmt.Sprintf("%s/%s", owner, repo),
		Canonical: canonical,
		PageBlock: "repo_workflows_body",
		Extra: map[string]any{
			"Owner":        owner,
			"Repo":         repo,
			"Branch":       branch,
			"Links":        links,
			"DashboardURL": s.abs(r, "/dashboard"),
			"GitHubURL":    fmt.Sprintf("https://github.com/%s/%s", owner, repo),
		},
	})
}

func workflowBadgeLinks(s *Server, r *http.Request, owner, repo, workflow, branch, h string) (badgeURL, targetURL, markdown string) {
	targetURL = s.abs(r, fmt.Sprintf("/%s/%s/workflows/%s/%s", owner, repo, pathComponent(workflow), pathComponent(branch)))
	badgeURL = s.abs(r, fmt.Sprintf("/%s/%s/workflows/%s/%s/badge.svg", owner, repo, pathComponent(workflow), pathComponent(branch)))
	if h != "" {
		targetURL = artifactURL(targetURL, h, "")
		badgeURL = artifactURL(badgeURL, h, "")
	}
	markdown = fmt.Sprintf("[![%s](%s)](%s)", s.cfg.PrimaryDomain(), badgeURL, targetURL)
	return badgeURL, targetURL, markdown
}

func workflowShortName(path string) string {
	base := path
	if i := strings.LastIndex(path, "/"); i >= 0 {
		base = path[i+1:]
	}
	base = strings.TrimSuffix(base, ".yml")
	return strings.TrimSuffix(base, ".yaml")
}

func (s *Server) dashboardRepoURL(r *http.Request, owner, repo string, private bool, password string) string {
	u := s.abs(r, fmt.Sprintf("/%s/%s", owner, repo))
	if private && password != "" {
		u = artifactURL(u, password, "")
	}
	return u
}
