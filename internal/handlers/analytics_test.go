package handlers

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/skrashevich/dawnlink/internal/config"
	"github.com/skrashevich/dawnlink/internal/db"
	"github.com/skrashevich/dawnlink/internal/i18n"
	"github.com/skrashevich/dawnlink/internal/render"
)

func TestDownloadAnalyticsDisabledByDefault(t *testing.T) {
	srv := analyticsTestServer(t, config.Config{
		GitHubAppID:   1,
		GitHubPEMPath: "x",
		AppSecret:     "01234567890123456789012345678901",
		BaseURL:       "http://localhost:8080/",
		PublicURLs:    []string{"http://localhost:8080/"},
	})
	req := httptest.NewRequest(http.MethodGet, "/analytics/downloads?token=secret", nil)
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", rec.Code)
	}
}

func TestDownloadAnalyticsURL(t *testing.T) {
	cfg := config.Config{
		BaseURL:                  "http://localhost:8080/",
		PublicURLs:               []string{"http://localhost:8080/"},
		DownloadAnalyticsCollect: true,
		DownloadAnalyticsView:    true,
		DownloadAnalyticsSecret:  "my-analytics-secret",
	}
	srv := analyticsTestServer(t, cfg)
	req := httptest.NewRequest(http.MethodGet, "/dashboard", nil)
	got := srv.downloadAnalyticsURL(req)
	want := "http://localhost:8080/analytics/downloads?token=my-analytics-secret"
	if got != want {
		t.Fatalf("downloadAnalyticsURL() = %q, want %q", got, want)
	}
	cfg.DownloadAnalyticsView = false
	srv2 := analyticsTestServer(t, cfg)
	if srv2.downloadAnalyticsURL(req) != "" {
		t.Fatal("expected empty URL when view disabled")
	}
}

func TestDownloadAnalyticsRequiresToken(t *testing.T) {
	cfg := config.Config{
		GitHubAppID:                1,
		GitHubPEMPath:              "x",
		AppSecret:                  "01234567890123456789012345678901",
		BaseURL:                    "http://localhost:8080/",
		PublicURLs:                 []string{"http://localhost:8080/"},
		DownloadAnalyticsCollect:   true,
		DownloadAnalyticsView:      true,
		DownloadAnalyticsSecret:    "analytics-secret-token",
	}
	srv := analyticsTestServer(t, cfg)

	req := httptest.NewRequest(http.MethodGet, "/analytics/downloads", nil)
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("missing token status = %d, want 404", rec.Code)
	}

	req = httptest.NewRequest(http.MethodGet, "/analytics/downloads?token=analytics-secret-token", nil)
	rec = httptest.NewRecorder()
	srv.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("valid token status = %d, want 200", rec.Code)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "Download analytics") && !strings.Contains(body, "Аналитика") {
		t.Fatal("expected analytics page body")
	}
}

func analyticsTestServer(t *testing.T, cfg config.Config) *Server {
	t.Helper()
	dir := t.TempDir()
	store, err := db.Open(filepath.Join(dir, "test.db"), cfg.AppSecret, nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	bundle, err := i18n.Load("en")
	if err != nil {
		t.Fatal(err)
	}
	eng, err := render.New(bundle, cfg.BaseURL)
	if err != nil {
		t.Fatal(err)
	}
	return New(cfg, store, nil, eng, bundle)
}
