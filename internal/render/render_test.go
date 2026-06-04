package render

import (
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/skrashevich/dawnlink/internal/i18n"
)

func TestIndexPage(t *testing.T) {
	b, err := i18n.Load("ru")
	if err != nil {
		t.Fatal(err)
	}
	eng, err := New(b, "http://localhost:8080/")
	if err != nil {
		t.Fatal(err)
	}
	w := httptest.NewRecorder()
	r := httptest.NewRequest("GET", "/", nil)
	err = eng.Page(w, r, "index.html", PageData{
		PageBlock: "index_body",
		Extra: map[string]any{
			"ReconfigureURL": "https://example.com/install",
			"AuthURL":        "https://example.com/auth",
			"HasAuth":        true,
			"HasMessages":    false,
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if w.Code != 200 {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	if w.Body.Len() < 100 {
		t.Fatalf("short body: %s", w.Body.String())
	}
}

func TestLanguageLinksPreserveQuery(t *testing.T) {
	b, err := i18n.Load("ru")
	if err != nil {
		t.Fatal(err)
	}
	eng, err := New(b, "http://localhost:8080/")
	if err != nil {
		t.Fatal(err)
	}
	w := httptest.NewRecorder()
	r := httptest.NewRequest("GET", "/owner/repo?h=secret&status=completed", nil)
	err = eng.Page(w, r, "error_page.html", PageData{PageBlock: "error_body"})
	if err != nil {
		t.Fatal(err)
	}
	body := w.Body.String()
	if !strings.Contains(body, `href="/owner/repo?h=secret&amp;lang=en&amp;status=completed"`) {
		t.Fatalf("language link did not preserve query: %s", body)
	}
}
