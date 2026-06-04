package github

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
)

func TestRepositoryUnmarshalOwner(t *testing.T) {
	for _, tc := range []struct {
		name string
		data string
		want string
	}{
		{name: "object", data: `{"name":"repo","full_name":"owner/repo","owner":{"login":"owner"}}`, want: "owner"},
		{name: "string", data: `{"name":"repo","full_name":"owner/repo","owner":"owner"}`, want: "owner"},
		{name: "full name fallback", data: `{"name":"repo","full_name":"owner/repo"}`, want: "owner"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var repo Repository
			if err := json.Unmarshal([]byte(tc.data), &repo); err != nil {
				t.Fatal(err)
			}
			if repo.Owner != tc.want {
				t.Fatalf("Owner = %q, want %q", repo.Owner, tc.want)
			}
		})
	}
}

func TestExchangeOAuthCodeJSON(t *testing.T) {
	withHTTPTransport(t, func(r *http.Request) *http.Response {
		if got := r.Header.Get("Accept"); got != "application/json" {
			t.Errorf("Accept = %q, want application/json", got)
		}
		return jsonResponse(`{"access_token":"token-value","token_type":"bearer"}`)
	})

	token, err := ExchangeOAuthCode("client", "secret", "code")
	if err != nil {
		t.Fatal(err)
	}
	if token != "token-value" {
		t.Fatalf("token = %q, want token-value", token)
	}
}

func TestExchangeOAuthCodeRejectsMissingToken(t *testing.T) {
	withHTTPTransport(t, func(r *http.Request) *http.Response {
		return jsonResponse(`{}`)
	})

	if _, err := ExchangeOAuthCode("client", "secret", "code"); err == nil {
		t.Fatal("expected missing-token error")
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) {
	return f(r)
}

func withHTTPTransport(t *testing.T, fn func(*http.Request) *http.Response) {
	t.Helper()
	oldClient := httpClient
	httpClient = &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		return fn(r), nil
	})}
	t.Cleanup(func() { httpClient = oldClient })
}

func jsonResponse(body string) *http.Response {
	return &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": {"application/json"}},
		Body:       io.NopCloser(strings.NewReader(body)),
	}
}

func TestIsNotFoundUsesAPIStatus(t *testing.T) {
	if !IsNotFound(&APIError{Status: http.StatusNotFound}) {
		t.Fatal("404 API error was not recognized")
	}
	if IsNotFound(errors.New("unrelated error containing 404")) {
		t.Fatal("plain error was incorrectly recognized as not found")
	}
}

func TestListWorkflowRunsOmitsEmptyEvent(t *testing.T) {
	withHTTPTransport(t, func(r *http.Request) *http.Response {
		q := r.URL.Query()
		if q.Has("event") {
			t.Fatalf("event query param should be omitted, got %q", r.URL.RawQuery)
		}
		if got := q.Get("branch"); got != "main" {
			t.Fatalf("branch = %q, want main", got)
		}
		if got := q.Get("status"); got != "success" {
			t.Fatalf("status = %q, want success", got)
		}
		return jsonResponse(`{"workflow_runs":[{"id":1,"status":"completed","conclusion":"success"}]}`)
	})

	runs, err := ListWorkflowRuns("Owner", "Repo", "ci.yml", "main", "", "success", AppToken{JWT: "jwt"}, 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(runs) != 1 || runs[0].ID != 1 {
		t.Fatalf("runs = %+v", runs)
	}
}

func TestListWorkflowRunsIncludesEventWhenSet(t *testing.T) {
	withHTTPTransport(t, func(r *http.Request) *http.Response {
		if got := r.URL.Query().Get("event"); got != "push" {
			t.Fatalf("event = %q, want push", got)
		}
		return jsonResponse(`{"workflow_runs":[]}`)
	})

	if _, err := ListWorkflowRuns("owner", "repo", "ci.yml", "main", "push", "", AppToken{JWT: "jwt"}, 1); err != nil {
		t.Fatal(err)
	}
}

func TestListWorkflowRunJobs(t *testing.T) {
	withHTTPTransport(t, func(r *http.Request) *http.Response {
		if !strings.Contains(r.URL.Path, "/actions/runs/42/jobs") {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		return jsonResponse(`{"jobs":[{"id":99,"name":"build"}]}`)
	})

	jobs, err := ListWorkflowRunJobs("owner", "repo", 42, AppToken{JWT: "jwt"})
	if err != nil {
		t.Fatal(err)
	}
	if len(jobs) != 1 || jobs[0].ID != 99 {
		t.Fatalf("jobs = %+v", jobs)
	}
}

func TestNextLinkFindsNonFirstRelation(t *testing.T) {
	resp := &http.Response{Header: http.Header{
		"Link": {`<https://api.github.com/items?page=1>; rel="prev", <https://api.github.com/items?page=3>; rel="next"`},
	}}
	if got := nextLink(resp); got != "https://api.github.com/items?page=3" {
		t.Fatalf("nextLink() = %q", got)
	}
}
