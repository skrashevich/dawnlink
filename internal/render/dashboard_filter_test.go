package render

import (
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/skrashevich/dawnlink/internal/i18n"
)

type dashRepo struct {
	Name string
	URL  string
}

type dashAccount struct {
	Owner        string
	PublicRepos  []dashRepo
	PrivateRepos []dashRepo
}

func TestDashboardRendersRepoFilterAttribute(t *testing.T) {
	b, err := i18n.Load("en")
	if err != nil {
		t.Fatal(err)
	}
	eng, err := New(b, "http://localhost:8080/")
	if err != nil {
		t.Fatal(err)
	}
	w := httptest.NewRecorder()
	r := httptest.NewRequest("GET", "/dashboard", nil)
	err = eng.Page(w, r, "dashboard.html", PageData{
		PageBlock: "dashboard_body",
		Extra: map[string]any{
			"Accounts": []dashAccount{{
				Owner:       "acme",
				PublicRepos: []dashRepo{{Name: "Hello-World", URL: "/acme/Hello-World"}},
			}},
			"TotalRepos":     1,
			"ReconfigureURL": "https://example.com/install",
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	body := w.Body.String()
	if !strings.Contains(body, `data-repo="hello-world"`) {
		t.Fatalf("expected lowercased data-repo attribute, got fragment: %s", body[strings.Index(body, "dashboard-repo"):strings.Index(body, "dashboard-repo")+120])
	}
}
