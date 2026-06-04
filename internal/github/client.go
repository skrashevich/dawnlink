package github

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

const apiBase = "https://api.github.com/"

var oauthAccessTokenURL = "https://github.com/login/oauth/access_token"

var httpClient = &http.Client{
	Timeout: 15 * time.Second,
	CheckRedirect: func(req *http.Request, via []*http.Request) error {
		if len(via) >= 10 {
			return fmt.Errorf("too many redirects")
		}
		return nil
	},
}

type Token interface {
	Header() string
}

type AppToken struct{ JWT string }

func (t AppToken) Header() string { return "Bearer " + t.JWT }

type UserToken struct{ Value string }

func (t UserToken) Header() string { return "token " + t.Value }

type InstallationToken struct {
	Value          string
	InstallationID int64
}

func (t InstallationToken) Header() string { return "token " + t.Value }

type OAuthToken struct{ Value string }

func (t OAuthToken) Header() string { return "token " + t.Value }

func Get(path string, token Token, params url.Values) (*http.Response, error) {
	u, _ := url.Parse(apiBase + strings.TrimPrefix(path, "/"))
	if len(params) > 0 {
		u.RawQuery = params.Encode()
	}
	req, err := http.NewRequest(http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	if token != nil {
		req.Header.Set("Authorization", token.Header())
	}
	return httpClient.Do(req)
}

func Post(path string, token Token, body any) (*http.Response, error) {
	var r io.Reader
	if body != nil {
		switch b := body.(type) {
		case url.Values:
			r = strings.NewReader(b.Encode())
		default:
			data, err := json.Marshal(b)
			if err != nil {
				return nil, err
			}
			r = bytes.NewReader(data)
		}
	}
	u := apiBase + strings.TrimPrefix(path, "/")
	req, err := http.NewRequest(http.MethodPost, u, r)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	if token != nil {
		req.Header.Set("Authorization", token.Header())
	}
	if body != nil {
		if _, ok := body.(url.Values); ok {
			req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		} else {
			req.Header.Set("Content-Type", "application/json")
		}
	}
	return httpClient.Do(req)
}

func PostForm(externalURL string, form url.Values) (*http.Response, error) {
	req, err := http.NewRequest(http.MethodPost, externalURL, strings.NewReader(form.Encode()))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	return httpClient.Do(req)
}

type APIError struct {
	Status int
	Body   string
}

func (e *APIError) Error() string {
	return fmt.Sprintf("github api %d: %s", e.Status, e.Body)
}

func decodeJSON(resp *http.Response, v any) error {
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		body, _ := io.ReadAll(resp.Body)
		return &APIError{Status: resp.StatusCode, Body: string(body)}
	}
	return json.NewDecoder(resp.Body).Decode(v)
}

func nextLink(resp *http.Response) string {
	for _, l := range resp.Header["Link"] {
		for part := range strings.SplitSeq(l, ",") {
			if strings.Contains(part, `rel="next"`) {
				target, _, _ := strings.Cut(part, ";")
				if target != "" {
					return strings.Trim(strings.TrimSpace(target), "<>")
				}
			}
		}
	}
	return ""
}

type Installation struct {
	ID        int64   `json:"id"`
	Account   Account `json:"account"`
	UpdatedAt string  `json:"updated_at"`
}

type Account struct {
	Login string `json:"login"`
}

type Repository struct {
	FullName      string `json:"full_name"`
	Name          string `json:"name"`
	Owner         string
	DefaultBranch string `json:"default_branch"`
	Private       bool   `json:"private"`
	Fork          bool   `json:"fork"`
}

func (r *Repository) UnmarshalJSON(data []byte) error {
	var raw struct {
		FullName string          `json:"full_name"`
		Name     string          `json:"name"`
		Owner    json.RawMessage `json:"owner"`
		Private  bool            `json:"private"`
		Fork     bool            `json:"fork"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	r.FullName = raw.FullName
	r.Name = raw.Name
	r.Private = raw.Private
	r.Fork = raw.Fork

	if len(raw.Owner) > 0 && string(raw.Owner) != "null" {
		if raw.Owner[0] == '"' {
			if err := json.Unmarshal(raw.Owner, &r.Owner); err != nil {
				return err
			}
		} else {
			var owner Account
			if err := json.Unmarshal(raw.Owner, &owner); err != nil {
				return err
			}
			r.Owner = owner.Login
		}
	}
	r.parseOwner()
	return nil
}

func (r *Repository) parseOwner() {
	if r.Owner == "" && strings.Contains(r.FullName, "/") {
		r.Owner = strings.SplitN(r.FullName, "/", 2)[0]
	}
}

type WorkflowRun struct {
	ID            int64      `json:"id"`
	Status        string     `json:"status"`
	Conclusion    string     `json:"conclusion"`
	Event         string     `json:"event"`
	WorkflowID    int64      `json:"workflow_id"`
	CheckSuiteURL string     `json:"check_suite_url"`
	UpdatedAt     time.Time  `json:"-"`
	UpdatedAtRaw  string     `json:"updated_at"`
	Repository    Repository `json:"repository"`
}

func (w *WorkflowRun) parseTimes() {
	if w.UpdatedAt.IsZero() && w.UpdatedAtRaw != "" {
		if t, err := time.Parse(time.RFC3339, w.UpdatedAtRaw); err == nil {
			w.UpdatedAt = t
		}
	}
}

func (w WorkflowRun) CheckSuiteID() int64 {
	parts := strings.Split(strings.TrimRight(w.CheckSuiteURL, "/"), "/")
	if len(parts) == 0 {
		return 0
	}
	id, _ := strconv.ParseInt(parts[len(parts)-1], 10, 64)
	return id
}

type Artifact struct {
	ID   int64  `json:"id"`
	Name string `json:"name"`
	URL  string `json:"url"`
}

func (a *Artifact) RepoOwnerName() (owner, name string) {
	// url: https://api.github.com/repos/owner/name/actions/artifacts/id
	parts := strings.Split(a.URL, "/")
	for i, p := range parts {
		if p == "repos" && i+2 < len(parts) {
			return parts[i+1], parts[i+2]
		}
	}
	return "", ""
}

type installationsResponse struct {
	Installations []Installation `json:"installations"`
}

type repositoriesResponse struct {
	Repositories []Repository `json:"repositories"`
}

type Workflow struct {
	ID    int64  `json:"id"`
	Name  string `json:"name"`
	Path  string `json:"path"`
	State string `json:"state"`
}

type workflowListResponse struct {
	Workflows []Workflow `json:"workflows"`
}

type workflowRunsResponse struct {
	WorkflowRuns []WorkflowRun `json:"workflow_runs"`
}

type artifactsResponse struct {
	Artifacts []Artifact `json:"artifacts"`
}

type WorkflowJob struct {
	ID   int64  `json:"id"`
	Name string `json:"name"`
}

type workflowJobsResponse struct {
	Jobs []WorkflowJob `json:"jobs"`
}

func ListUserInstallations(token UserToken) ([]Installation, error) {
	path := "user/installations"
	params := url.Values{"per_page": {"100"}}
	var all []Installation
	for {
		resp, err := Get(path, token, params)
		if err != nil {
			return nil, err
		}
		var out installationsResponse
		if err := decodeJSON(resp, &out); err != nil {
			return nil, err
		}
		all = append(all, out.Installations...)
		path, params = nextPage(resp)
		if path == "" {
			return all, nil
		}
	}
}

func GetInstallation(id int64, appToken AppToken) (*Installation, error) {
	resp, err := Get(fmt.Sprintf("app/installations/%d", id), appToken, nil)
	if err != nil {
		return nil, err
	}
	var inst Installation
	if err := decodeJSON(resp, &inst); err != nil {
		return nil, err
	}
	return &inst, nil
}

func ListInstallationRepos(token Token, installationID int64, userScoped bool) ([]Repository, error) {
	path := "installation/repositories"
	if userScoped {
		path = fmt.Sprintf("user/installations/%d/repositories", installationID)
	}
	var all []Repository
	params := url.Values{"per_page": {"100"}}
	for {
		resp, err := Get(path, token, params)
		if err != nil {
			return nil, err
		}
		var out repositoriesResponse
		if err := decodeJSON(resp, &out); err != nil {
			return nil, err
		}
		for i := range out.Repositories {
			out.Repositories[i].parseOwner()
			all = append(all, out.Repositories[i])
		}
		path, params = nextPage(resp)
		if path == "" {
			break
		}
	}
	return all, nil
}

func GetRepository(owner, repo string, token Token) (*Repository, error) {
	path := fmt.Sprintf("repos/%s/%s", strings.ToLower(owner), strings.ToLower(repo))
	resp, err := Get(path, token, nil)
	if err != nil {
		return nil, err
	}
	var out Repository
	if err := decodeJSON(resp, &out); err != nil {
		return nil, err
	}
	out.parseOwner()
	return &out, nil
}

func ListWorkflows(owner, repo string, token Token) ([]Workflow, error) {
	path := fmt.Sprintf("repos/%s/%s/actions/workflows", strings.ToLower(owner), strings.ToLower(repo))
	var all []Workflow
	params := url.Values{"per_page": {"100"}}
	for {
		resp, err := Get(path, token, params)
		if err != nil {
			return nil, err
		}
		var out workflowListResponse
		if err := decodeJSON(resp, &out); err != nil {
			return nil, err
		}
		all = append(all, out.Workflows...)
		path, params = nextPage(resp)
		if path == "" {
			return all, nil
		}
	}
}

func ListWorkflowRuns(owner, repo, workflow, branch, event, status string, token Token, max int) ([]WorkflowRun, error) {
	path := fmt.Sprintf("repos/%s/%s/actions/workflows/%s/runs", strings.ToLower(owner), strings.ToLower(repo), workflow)
	params := url.Values{
		"branch":   {branch},
		"per_page": {strconv.Itoa(max)},
	}
	if event != "" {
		params.Set("event", event)
	}
	if status != "" {
		params.Set("status", status)
	}
	resp, err := Get(path, token, params)
	if err != nil {
		return nil, err
	}
	var out workflowRunsResponse
	if err := decodeJSON(resp, &out); err != nil {
		return nil, err
	}
	for i := range out.WorkflowRuns {
		out.WorkflowRuns[i].parseTimes()
		out.WorkflowRuns[i].Repository.parseOwner()
	}
	return out.WorkflowRuns, nil
}

func ListRunArtifacts(owner, repo string, runID int64, token Token) ([]Artifact, error) {
	path := fmt.Sprintf("repos/%s/%s/actions/runs/%d/artifacts", strings.ToLower(owner), strings.ToLower(repo), runID)
	params := url.Values{"per_page": {"100"}}
	var all []Artifact
	for {
		resp, err := Get(path, token, params)
		if err != nil {
			return nil, err
		}
		var out artifactsResponse
		if err := decodeJSON(resp, &out); err != nil {
			return nil, err
		}
		all = append(all, out.Artifacts...)
		path, params = nextPage(resp)
		if path == "" {
			return all, nil
		}
	}
}

func ListWorkflowRunJobs(owner, repo string, runID int64, token Token) ([]WorkflowJob, error) {
	path := fmt.Sprintf("repos/%s/%s/actions/runs/%d/jobs", strings.ToLower(owner), strings.ToLower(repo), runID)
	params := url.Values{"per_page": {"100"}}
	var all []WorkflowJob
	for {
		resp, err := Get(path, token, params)
		if err != nil {
			return nil, err
		}
		var out workflowJobsResponse
		if err := decodeJSON(resp, &out); err != nil {
			return nil, err
		}
		all = append(all, out.Jobs...)
		path, params = nextPage(resp)
		if path == "" {
			return all, nil
		}
	}
}

func nextPage(resp *http.Response) (string, url.Values) {
	next := nextLink(resp)
	if next == "" {
		return "", nil
	}
	u, err := url.Parse(next)
	if err != nil {
		return "", nil
	}
	return strings.TrimPrefix(u.Path, "/"), u.Query()
}

type ArtifactDownloadError struct {
	Status int
}

func (e *ArtifactDownloadError) Error() string {
	return fmt.Sprintf("artifact download error: %d", e.Status)
}

func ArtifactZipURL(owner, repo string, artifactID int64, token Token) (string, error) {
	path := fmt.Sprintf("repos/%s/%s/actions/artifacts/%d/zip", strings.ToLower(owner), strings.ToLower(repo), artifactID)
	req, err := http.NewRequest(http.MethodGet, apiBase+path, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("Authorization", token.Header())
	client := &http.Client{
		Timeout: 15 * time.Second,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode == 410 || (resp.StatusCode == 500) {
		return "", &ArtifactDownloadError{Status: resp.StatusCode}
	}
	if resp.StatusCode >= 400 {
		body, _ := io.ReadAll(resp.Body)
		return "", &APIError{Status: resp.StatusCode, Body: string(body)}
	}
	loc := resp.Header.Get("Location")
	if loc == "" {
		return "", fmt.Errorf("no redirect location")
	}
	return loc, nil
}

func JobLogsURL(owner, repo string, jobID int64, token Token) (string, error) {
	path := fmt.Sprintf("repos/%s/%s/actions/jobs/%d/logs", strings.ToLower(owner), strings.ToLower(repo), jobID)
	req, err := http.NewRequest(http.MethodGet, apiBase+path, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("Authorization", token.Header())
	client := &http.Client{
		Timeout: 15 * time.Second,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode == 410 {
		return "", &ArtifactDownloadError{Status: resp.StatusCode}
	}
	if resp.StatusCode >= 400 {
		body, _ := io.ReadAll(resp.Body)
		return "", &APIError{Status: resp.StatusCode, Body: string(body)}
	}
	loc := resp.Header.Get("Location")
	if loc == "" {
		return "", fmt.Errorf("no redirect location")
	}
	return loc, nil
}

func ExchangeOAuthCode(clientID, clientSecret, code string) (string, error) {
	resp, err := PostForm(oauthAccessTokenURL, url.Values{
		"client_id":     {clientID},
		"client_secret": {clientSecret},
		"code":          {code},
	})
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("github oauth %d: %s", resp.StatusCode, string(body))
	}
	var out struct {
		AccessToken      string `json:"access_token"`
		Error            string `json:"error"`
		ErrorDescription string `json:"error_description"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return "", fmt.Errorf("decode github oauth response: %w", err)
	}
	if out.Error != "" {
		if out.ErrorDescription != "" {
			return "", fmt.Errorf("%s: %s", out.Error, out.ErrorDescription)
		}
		return "", errors.New(out.Error)
	}
	if out.AccessToken == "" {
		return "", fmt.Errorf("github oauth response did not include an access token")
	}
	return out.AccessToken, nil
}

func IsNotFound(err error) bool {
	var apiErr *APIError
	return errors.As(err, &apiErr) &&
		(apiErr.Status == http.StatusNotFound ||
			apiErr.Status == http.StatusUnauthorized ||
			apiErr.Status == http.StatusUnavailableForLegalReasons)
}
