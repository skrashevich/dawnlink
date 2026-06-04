package handlers

import (
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/skrashevich/dawnlink/internal/db"
	"github.com/skrashevich/dawnlink/internal/github"
	"github.com/skrashevich/dawnlink/internal/render"
)

var routePatterns = struct {
	repoRoot         *regexp.Regexp
	workflowBadge    *regexp.Regexp
	workflowBranch   *regexp.Regexp
	workflowArtifact *regexp.Regexp
	runDash          *regexp.Regexp
	runArtifact      *regexp.Regexp
	artifact         *regexp.Regexp
	job              *regexp.Regexp
}{
	repoRoot:         regexp.MustCompile(`^/([^/]+)/([^/]+)$`),
	workflowBadge:    regexp.MustCompile(`^/([^/]+)/([^/]+)/workflows/([^/]+)/([^/]+)/badge\.svg$`),
	workflowBranch:   regexp.MustCompile(`^/([^/]+)/([^/]+)/workflows/([^/]+)/([^/]+)$`),
	workflowArtifact: regexp.MustCompile(`^/([^/]+)/([^/]+)/workflows/([^/]+)/([^/]+)/([^/]+?)(\.zip)?$`),
	runDash:          regexp.MustCompile(`^/([^/]+)/([^/]+)/actions/runs/([0-9]+)$`),
	runArtifact:      regexp.MustCompile(`^/([^/]+)/([^/]+)/actions/runs/([0-9]+)/([^/]+?)(\.zip)?$`),
	artifact:         regexp.MustCompile(`^/([^/]+)/([^/]+)/actions/artifacts/([0-9]+)(\.zip)?$`),
	job:              regexp.MustCompile(`^/([^/]+)/([^/]+)/runs/([0-9]+)(\.txt)?$`),
}

func (s *Server) serveArtifactRoutes(w http.ResponseWriter, r *http.Request) error {
	path := r.URL.EscapedPath()
	if m := routeMatch(routePatterns.repoRoot, path); m != nil {
		return s.repoWorkflows(w, r, m[1], m[2])
	}
	if m := routeMatch(routePatterns.workflowBadge, path); m != nil {
		return s.badgeByBranch(w, r, m[1], m[2], m[3], m[4])
	}
	if m := routeMatch(routePatterns.workflowBranch, path); m != nil {
		return s.dashByBranch(w, r, m[1], m[2], m[3], m[4])
	}
	if m := routeMatch(routePatterns.workflowArtifact, path); m != nil {
		return s.byBranch(w, r, m[1], m[2], m[3], m[4], m[5], m[6] == ".zip")
	}
	if m := routeMatch(routePatterns.runDash, path); m != nil {
		return s.dashByRun(w, r, m[1], m[2], m[3])
	}
	if m := routeMatch(routePatterns.runArtifact, path); m != nil {
		return s.byRun(w, r, m[1], m[2], m[3], m[4], m[5] == ".zip")
	}
	if m := routeMatch(routePatterns.artifact, path); m != nil {
		return s.byArtifact(w, r, m[1], m[2], m[3], m[4] == ".zip")
	}
	if m := routeMatch(routePatterns.job, path); m != nil {
		return s.byJob(w, r, m[1], m[2], m[3], m[4] == ".txt")
	}
	return &httpError{status: 404, message: r.URL.Path}
}

func routeMatch(re *regexp.Regexp, escapedPath string) []string {
	match := re.FindStringSubmatch(escapedPath)
	for i := 1; i < len(match); i++ {
		value, err := url.PathUnescape(match[i])
		if err != nil {
			return nil
		}
		match[i] = value
	}
	return match
}

func (s *Server) index(w http.ResponseWriter, r *http.Request) error {
	if u := r.URL.Query().Get("url"); u != "" {
		parsed, err := url.Parse(u)
		if err == nil && strings.EqualFold(parsed.Scheme, "https") && strings.EqualFold(parsed.Hostname(), "github.com") {
			if newPath, ok := matchGitHubImport(parsed.Path); ok {
				if h := r.URL.Query().Get("h"); h != "" {
					sep := "?"
					if strings.Contains(newPath, "?") {
						sep = "&"
					}
					newPath += sep + "h=" + url.QueryEscape(h)
				}
				return redirect(newPath, http.StatusFound)
			}
		}
		msgs := []string{s.t(r, "link_not_recognized"), u}
		return s.renderIndex(w, r, msgs)
	}
	return s.renderIndex(w, r, nil)
}

func (s *Server) renderIndex(w http.ResponseWriter, r *http.Request, messages []string) error {
	example := workflowExample{
		workflowURL: "https://github.com/skrashevich/dawnlink/blob/main/.github/workflows/binaries.yml",
		owner:       "skrashevich", repo: "dawnlink", workflow: "binaries", branch: "main", artifact: "dawnlink-linux-amd64",
	}
	extra := map[string]any{
		"Messages":           messages,
		"ExampleWorkflow":    example.workflowURL,
		"ExampleDest":        s.abs(r, fmt.Sprintf("/%s/%s/workflows/%s/%s/%s", example.owner, example.repo, example.workflow, example.branch, example.artifact)),
		"ExampleArtifact":    example.artifact,
		"ExampleBotWorkflow": fmt.Sprintf("https://github.com/%s/%s/blob/%s/.github/workflows/pr-comment.yml", example.owner, example.repo, example.branch),
		"ReconfigureURL":     fmt.Sprintf("https://github.com/apps/%s/installations/new", s.cfg.GitHubAppName),
		"AuthURL":            s.oauthURL(r, "/dashboard"),
		"HasAuth":            s.cfg.GitHubClientID != "" && s.cfg.GitHubClientSecret != "",
		"HasMessages":        len(messages) > 0,
	}
	if ids, err := s.cachedIndexExampleIDs(example); err == nil {
		applyIndexExampleExtras(example, ids, func(path string) string { return s.abs(r, path) }, extra)
	}
	return s.renderPage(w, r, "index.html", render.PageData{
		Canonical: s.abs(r, "/"),
		Messages:  messages,
		PageBlock: "index_body",
		Extra:     extra,
	})
}

type workflowExample struct {
	workflowURL, owner, repo, workflow, branch, artifact string
}

func (s *Server) dashboard(w http.ResponseWriter, r *http.Request) error {
	userTok, err := s.resolveUserSession(w, r, "/dashboard")
	if err != nil {
		return err
	}
	accounts, totalRepos, err := s.loadDashboardAccounts(r, userTok)
	if err != nil {
		return err
	}
	w.Header().Set("X-Robots-Tag", "noindex")
	return s.renderPage(w, r, "dashboard.html", render.PageData{
		Title:     s.t(r, "dashboard_title"),
		Canonical: s.abs(r, "/dashboard"),
		PageBlock: "dashboard_body",
		Extra: map[string]any{
			"Accounts":       accounts,
			"TotalRepos":     totalRepos,
			"ReconfigureURL": fmt.Sprintf("https://github.com/apps/%s/installations/new", s.cfg.GitHubAppName),
			"AnalyticsURL":   s.downloadAnalyticsURL(r),
		},
	})
}

type dashboardAccount struct {
	Owner        string
	PublicRepos  []dashboardRepo
	PrivateRepos []dashboardRepo
}

type dashboardRepo struct {
	Name string
	URL  string
}

func (s *Server) setup(w http.ResponseWriter, r *http.Request) error {
	idStr := r.URL.Query().Get("installation_id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		return &httpError{status: 400, message: "installation_id required"}
	}
	jwt, err := s.ghApp.JWT()
	if err != nil {
		return err
	}
	inst, err := github.GetInstallation(id, jwt)
	if err != nil {
		return err
	}
	if _, err := s.store.RefreshFromInstallation(*inst, nil); err != nil {
		return err
	}
	return redirect("/", http.StatusFound)
}

func (s *Server) normalizeWorkflow(workflow string) string {
	if _, err := strconv.ParseInt(workflow, 10, 64); err == nil {
		return workflow
	}
	if !strings.HasSuffix(workflow, ".yml") && !strings.HasSuffix(workflow, ".yaml") {
		return workflow + ".yml"
	}
	return workflow
}

func (s *Server) statusParam(r *http.Request) (string, error) {
	status := r.URL.Query().Get("status")
	if status == "" {
		status = "success"
	}
	if status != "success" && status != "completed" {
		return "", &httpError{status: 400, message: s.t(r, "status_bad")}
	}
	return status, nil
}

func (s *Server) getLatestRun(owner, repo, workflow, branch, status string, token github.Token) (*github.WorkflowRun, error) {
	runs, err := github.ListWorkflowRuns(owner, repo, workflow, branch, "", status, token, 1)
	if err != nil {
		if github.IsNotFound(err) {
			return nil, &httpError{status: 404, message: fmt.Sprintf("Repository '%s/%s' or workflow '%s' not found.", owner, repo, workflow)}
		}
		return nil, err
	}
	if len(runs) == 0 {
		return nil, &httpError{status: 404, message: fmt.Sprintf("No successful runs for workflow '%s' branch '%s'.", workflow, branch)}
	}
	return &runs[0], nil
}

func (s *Server) dashByBranch(w http.ResponseWriter, r *http.Request, owner, repo, workflow, branch string) error {
	h := r.URL.Query().Get("h")
	token, pw, err := s.store.VerifiedToken(owner, repo, h)
	if err != nil {
		return &httpError{status: 404, message: s.t(r, "repo_not_found_private")}
	}
	if pw != "" {
		h = pw
	}
	workflow = s.normalizeWorkflow(workflow)
	status, err := s.statusParam(r)
	if err != nil {
		return err
	}
	run, err := s.getLatestRun(owner, repo, workflow, branch, status, token)
	if err != nil {
		return err
	}
	owner, repo = run.Repository.Owner, run.Repository.Name
	if run.Repository.FullName != "" {
		parts := strings.SplitN(run.Repository.FullName, "/", 2)
		if len(parts) == 2 {
			owner, repo = parts[0], parts[1]
		}
	}
	var messages []string
	if run.UpdatedAt.Before(time.Now().Add(-90 * 24 * time.Hour)) {
		messages = append(messages, s.t(r, "old_run_warning"))
	}
	arts, err := github.ListRunArtifacts(owner, repo, run.ID, token)
	if err != nil {
		if github.IsNotFound(err) {
			return &httpError{status: 404, message: fmt.Sprintf("No artifacts for workflow '%s' branch '%s'.", workflow, branch)}
		}
		return err
	}
	if len(arts) == 0 {
		return &httpError{status: 404, message: fmt.Sprintf("No artifacts for workflow '%s' branch '%s'.", workflow, branch)}
	}
	wfShort := strings.TrimSuffix(strings.TrimSuffix(workflow, ".yml"), ".yaml")
	var links []ArtifactLink
	for _, a := range arts {
		o, n := a.RepoOwnerName()
		if o == "" {
			o, n = owner, repo
		}
		u := s.abs(r, fmt.Sprintf("/%s/%s/workflows/%s/%s/%s", o, n, pathComponent(wfShort), pathComponent(branch), pathComponent(a.Name)))
		u = artifactURL(u, h, status)
		links = append(links, ArtifactLink{URL: u, Title: a.Name})
	}
	sort.Slice(links, func(i, j int) bool { return links[i].Title < links[j].Title })
	if len(links) == 1 && r.URL.Query().Get("preview") == "" {
		return redirect(links[0].URL, http.StatusFound)
	}
	if len(links) == 1 && r.URL.Query().Get("preview") != "" {
		messages = append(messages, s.t(r, "preview_hint"))
	}
	canonical := s.abs(r, fmt.Sprintf("/%s/%s/workflows/%s/%s", owner, repo, pathComponent(wfShort), pathComponent(branch)))
	canonical = artifactURL(canonical, "", status)
	return s.renderArtifactList(w, r, []string{fmt.Sprintf("%s/%s", owner, repo), fmt.Sprintf("Workflow %s | Branch %s", workflow, branch)}, canonical, links, messages)
}

func (s *Server) dashByRun(w http.ResponseWriter, r *http.Request, owner, repo, runIDStr string) error {
	runID, err := strconv.ParseInt(runIDStr, 10, 64)
	if err != nil {
		return &httpError{status: 404, message: "run not found"}
	}
	h := r.URL.Query().Get("h")
	token, pw, err := s.store.VerifiedToken(owner, repo, h)
	if err != nil {
		return &httpError{status: 404, message: s.t(r, "repo_not_found_private")}
	}
	if pw != "" {
		h = pw
	}
	arts, err := github.ListRunArtifacts(owner, repo, runID, token)
	if err != nil {
		if github.IsNotFound(err) {
			return &httpError{status: 404, message: fmt.Sprintf("Run #%d not found.", runID)}
		}
		return err
	}
	if len(arts) == 0 {
		return &httpError{status: 404, message: fmt.Sprintf("No artifacts for run #%d.", runID)}
	}
	var links []ArtifactLink
	for _, a := range arts {
		o, n := a.RepoOwnerName()
		if o == "" {
			o, n = owner, repo
		}
		u := s.abs(r, fmt.Sprintf("/%s/%s/actions/runs/%d/%s", o, n, runID, pathComponent(a.Name)))
		if h != "" {
			u += "?h=" + url.QueryEscape(h)
		}
		links = append(links, ArtifactLink{URL: u, Title: a.Name})
	}
	sort.Slice(links, func(i, j int) bool { return links[i].Title < links[j].Title })
	canonical := s.abs(r, fmt.Sprintf("/%s/%s/actions/runs/%d", owner, repo, runID))
	return s.renderArtifactList(w, r, []string{fmt.Sprintf("%s/%s", owner, repo), fmt.Sprintf("Run #%d", runID)}, canonical, links, nil)
}

func (s *Server) byBranch(w http.ResponseWriter, r *http.Request, owner, repo, workflow, branch, artifact string, zip bool) error {
	h := r.URL.Query().Get("h")
	token, pw, err := s.store.VerifiedToken(owner, repo, h)
	if err != nil {
		return &httpError{status: 404, message: s.t(r, "repo_not_found_private")}
	}
	if pw != "" {
		h = pw
	}
	workflow = s.normalizeWorkflow(workflow)
	status, err := s.statusParam(r)
	if err != nil {
		return err
	}
	run, err := s.getLatestRun(owner, repo, workflow, branch, status, token)
	if err != nil {
		return err
	}
	owner, repo = run.Repository.Owner, run.Repository.Name
	if run.Repository.FullName != "" {
		parts := strings.SplitN(run.Repository.FullName, "/", 2)
		if len(parts) == 2 {
			owner, repo = parts[0], parts[1]
		}
	}
	return s.byRunInternal(w, r, owner, repo, run.ID, artifact, run.CheckSuiteID(), h, zip, workflow, branch, run.Event, status)
}

func (s *Server) byRun(w http.ResponseWriter, r *http.Request, owner, repo, runIDStr, artifact string, zip bool) error {
	runID, err := strconv.ParseInt(runIDStr, 10, 64)
	if err != nil {
		return &httpError{status: 404, message: "run not found"}
	}
	h := r.URL.Query().Get("h")
	_, pw, err := s.store.VerifiedToken(owner, repo, h)
	if err != nil {
		return &httpError{status: 404, message: s.t(r, "repo_not_found_private")}
	}
	if pw != "" {
		h = pw
	}
	return s.byRunInternal(w, r, owner, repo, runID, artifact, 0, h, zip, "", "", "", "")
}

func (s *Server) byRunInternal(w http.ResponseWriter, r *http.Request, owner, repo string, runID int64, artifact string, checkSuiteID int64, h string, zip bool, workflow, branch, event, status string) error {
	tok, _, err := s.store.VerifiedToken(owner, repo, h)
	if err != nil {
		return &httpError{status: 404, message: s.t(r, "repo_not_found_private")}
	}
	arts, err := github.ListRunArtifacts(owner, repo, runID, tok)
	if err != nil {
		if github.IsNotFound(err) {
			return &httpError{status: 404, message: fmt.Sprintf("No artifacts for run #%d.", runID)}
		}
		return err
	}
	var art *github.Artifact
	for i := range arts {
		if arts[i].Name == artifact || arts[i].Name == artifact+".zip" {
			art = &arts[i]
			break
		}
	}
	if art == nil {
		return &httpError{status: 404, message: fmt.Sprintf("Artifact '%s' not found.", artifact)}
	}
	o, n := art.RepoOwnerName()
	if o != "" {
		owner, repo = o, n
	}
	meta := downloadAnalyticsMeta{
		RouteKind:    db.RouteRun,
		Owner:        owner,
		Repo:         repo,
		ArtifactName: artifact,
		RunID:        runID,
		StatusFilter: status,
		RunEvent:     event,
	}
	if workflow != "" {
		meta.RouteKind = db.RouteWorkflow
		meta.Workflow = workflow
		meta.Branch = branch
	}
	links, canonical, title, err := s.buildArtifactLinks(r, owner, repo, art.ID, checkSuiteID, h, zip, meta)
	if err != nil {
		return err
	}
	if workflow != "" {
		wfShort := strings.TrimSuffix(strings.TrimSuffix(workflow, ".yml"), ".yaml")
		links = append(links, ArtifactLink{
			URL:   githubActionsURL(owner, repo, event, branch, status),
			Title: fmt.Sprintf(s.t(r, "browse_runs"), branch),
			Ext:   true,
		})
		links = append(links, ArtifactLink{
			URL:   artifactURL(s.abs(r, fmt.Sprintf("/%s/%s/workflows/%s/%s/%s.zip", owner, repo, pathComponent(wfShort), pathComponent(branch), pathComponent(artifact))), "", status),
			Title: "stable_workflow",
			H:     h,
		})
		canonical = s.abs(r, fmt.Sprintf("/%s/%s/workflows/%s/%s/%s", owner, repo, pathComponent(wfShort), pathComponent(branch), pathComponent(artifact)))
		canonical = artifactURL(canonical, "", status)
		title = []string{fmt.Sprintf("%s/%s", owner, repo), fmt.Sprintf("Workflow %s | Branch %s | Artifact %s", workflow, branch, artifact)}
	} else {
		links = append(links, ArtifactLink{
			URL:   fmt.Sprintf("https://github.com/%s/%s/actions/runs/%d#artifacts", owner, repo, runID),
			Title: fmt.Sprintf(s.t(r, "view_run"), runID),
			Ext:   true,
		})
		links = append(links, ArtifactLink{URL: s.abs(r, fmt.Sprintf("/%s/%s/actions/runs/%d/%s.zip", owner, repo, runID, pathComponent(artifact))), Title: "stable_run", H: h})
		canonical = s.abs(r, fmt.Sprintf("/%s/%s/actions/runs/%d/%s", owner, repo, runID, pathComponent(artifact)))
		title = []string{fmt.Sprintf("%s/%s", owner, repo), fmt.Sprintf("Run #%d | Artifact %s", runID, artifact)}
	}
	return s.renderArtifactPage(w, r, title, canonical, links)
}

func (s *Server) byArtifact(w http.ResponseWriter, r *http.Request, owner, repo, artifactIDStr string, zip bool) error {
	artifactID, err := strconv.ParseInt(artifactIDStr, 10, 64)
	if err != nil {
		return &httpError{status: 404, message: "artifact not found"}
	}
	h := r.URL.Query().Get("h")
	_, pw, err := s.store.VerifiedToken(owner, repo, h)
	if err != nil {
		return &httpError{status: 404, message: s.t(r, "repo_not_found_private")}
	}
	if pw != "" {
		h = pw
	}
	checkSuite := int64(0)
	meta := downloadAnalyticsMeta{RouteKind: db.RouteArtifact}
	links, canonical, title, err := s.buildArtifactLinks(r, owner, repo, artifactID, checkSuite, h, zip, meta)
	if err != nil {
		return err
	}
	return s.renderArtifactPage(w, r, title, canonical, links)
}

func (s *Server) buildArtifactLinks(r *http.Request, owner, repo string, artifactID, checkSuiteID int64, h string, zip bool, meta downloadAnalyticsMeta) ([]ArtifactLink, string, []string, error) {
	token, _, err := s.store.VerifiedToken(owner, repo, h)
	if err != nil {
		return nil, "", nil, &httpError{status: 404, message: err.Error()}
	}
	cacheKey := fmt.Sprintf("%s/%s/%d", strings.ToLower(owner), strings.ToLower(repo), artifactID)
	tmp, err := s.zipCache.Get(cacheKey, func() (string, error) {
		return github.ArtifactZipURL(owner, repo, artifactID, token)
	})
	if err != nil {
		if _, ok := err.(*github.ArtifactDownloadError); ok {
			return nil, "", nil, &httpError{status: 404, message: "artifact expired"}
		}
		if github.IsNotFound(err) {
			return nil, "", nil, &httpError{status: 404, message: fmt.Sprintf("Artifact #%d not found.", artifactID)}
		}
		return nil, "", nil, err
	}
	base, _, _ := strings.Cut(tmp, "?")
	if !strings.HasSuffix(base, ".zip") {
		return nil, "", nil, &httpError{status: 404, message: "zip only"}
	}
	if zip {
		meta.Owner = owner
		meta.Repo = repo
		meta.ArtifactID = artifactID
		if meta.RouteKind == "" {
			meta.RouteKind = db.RouteArtifact
		}
		if meta.ArtifactName == "" {
			meta.ArtifactName = fmt.Sprintf("#%d", artifactID)
		}
		meta.PrivateLink = h != ""
		s.maybeRecordDownload(r, meta)
		return nil, "", nil, redirect(tmp, http.StatusFound)
	}
	var links []ArtifactLink
	links = append(links, ArtifactLink{URL: tmp, Title: "ephemeral"})
	if checkSuiteID != 0 {
		links = append(links, ArtifactLink{
			URL:   fmt.Sprintf("https://github.com/%s/%s/suites/%d/artifacts/%d", owner, repo, checkSuiteID, artifactID),
			Title: "github",
			Ext:   true,
		})
	}
	canonical := s.abs(r, fmt.Sprintf("/%s/%s/actions/artifacts/%d", owner, repo, artifactID))
	links = append(links, ArtifactLink{URL: canonical + ".zip", Title: "stable_artifact", H: h})
	title := []string{fmt.Sprintf("%s/%s", owner, repo), fmt.Sprintf("Artifact #%d", artifactID)}
	return links, canonical, title, nil
}

func (s *Server) byJob(w http.ResponseWriter, r *http.Request, owner, repo, jobIDStr string, txt bool) error {
	jobID, err := strconv.ParseInt(jobIDStr, 10, 64)
	if err != nil {
		return &httpError{status: 404, message: "job not found"}
	}
	h := r.URL.Query().Get("h")
	token, pw, err := s.store.VerifiedToken(owner, repo, h)
	if err != nil {
		return &httpError{status: 404, message: s.t(r, "repo_not_found_private")}
	}
	if pw != "" {
		h = pw
	}
	cacheKey := fmt.Sprintf("logs:%s/%s/%d", strings.ToLower(owner), strings.ToLower(repo), jobID)
	tmp, err := s.logsCache.Get(cacheKey, func() (string, error) {
		return github.JobLogsURL(owner, repo, jobID, token)
	})
	if err != nil {
		if _, ok := err.(*github.ArtifactDownloadError); ok {
			return &httpError{status: 404, message: s.t(r, "logs_expired")}
		}
		if github.IsNotFound(err) {
			return &httpError{status: 404, message: fmt.Sprintf("Job #%d not found.", jobID)}
		}
		return err
	}
	if txt {
		return redirect(tmp, http.StatusFound)
	}
	canonical := s.abs(r, fmt.Sprintf("/%s/%s/runs/%d", owner, repo, jobID))
	links := []ArtifactLink{
		{URL: canonical + ".txt", Title: "stable_logs", H: h},
		{URL: tmp, Title: s.t(r, "ephemeral_logs")},
		{URL: fmt.Sprintf("https://github.com/%s/%s/runs/%d", owner, repo, jobID), Title: fmt.Sprintf(s.t(r, "view_job"), jobID), Ext: true},
	}
	title := []string{fmt.Sprintf("%s/%s", owner, repo), fmt.Sprintf("Job #%d", jobID)}
	return s.renderArtifactPage(w, r, title, canonical, links)
}

func githubActionsURL(owner, repo, event, branch, status string) string {
	q := url.Values{}
	q.Set("query", fmt.Sprintf("event:%s is:%s branch:%s", event, status, branch))
	return fmt.Sprintf("https://github.com/%s/%s/actions?%s", owner, repo, q.Encode())
}

func (s *Server) renderArtifactList(w http.ResponseWriter, r *http.Request, titleParts []string, canonical string, links []ArtifactLink, messages []string) error {
	return s.renderPage(w, r, "artifact_list.html", render.PageData{
		Title:     strings.Join(titleParts, " | "),
		Canonical: canonical,
		PageBlock: "artifact_list_body",
		Extra: map[string]any{
			"TitleParts": titleParts,
			"Links":      links,
			"Messages":   messages,
		},
	})
}

var artifactLinkTitles = map[string]string{
	"ephemeral":       "ephemeral_link",
	"github":          "github_login_required",
	"stable_artifact": "stable_artifact_link",
	"stable_workflow": "stable_workflow_link",
	"stable_run":      "stable_run_link",
	"stable_logs":     "stable_logs_link",
}

func (s *Server) renderArtifactPage(w http.ResponseWriter, r *http.Request, titleParts []string, canonical string, links []ArtifactLink) error {
	for i := range links {
		if key, ok := artifactLinkTitles[links[i].Title]; ok {
			links[i].Title = s.t(r, key)
		}
		if !strings.HasPrefix(links[i].URL, "http") {
			links[i].URL = s.abs(r, strings.TrimPrefix(links[i].URL, "/"))
		}
		if links[i].H != "" {
			links[i].URL = artifactURL(links[i].URL, links[i].H, "")
		}
	}
	sort.SliceStable(links, func(i, j int) bool { return links[i].Ext && !links[j].Ext })
	return s.renderPage(w, r, "artifact.html", render.PageData{
		Title:     strings.Join(titleParts, " | "),
		Canonical: canonical,
		PageBlock: "artifact_body",
		Extra: map[string]any{
			"TitleParts": titleParts,
			"Links":      links,
		},
	})
}

func artifactURL(rawURL, h, status string) string {
	u, err := url.Parse(rawURL)
	if err != nil {
		return rawURL
	}
	q := u.Query()
	if h != "" {
		q.Set("h", h)
	}
	if status != "" && status != "success" {
		q.Set("status", status)
	}
	u.RawQuery = q.Encode()
	return u.String()
}

func pathComponent(value string) string {
	return url.PathEscape(value)
}
