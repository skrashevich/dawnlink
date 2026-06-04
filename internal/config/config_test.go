package config

import (
	"net/http/httptest"
	"testing"
)

func TestValidate(t *testing.T) {
	valid := Config{
		GitHubAppID:   1,
		GitHubPEMPath: "key.pem",
		AppSecret:     "01234567890123456789012345678901",
		BaseURL:       "https://dawn.example/",
		PublicURLs:    []string{"https://dawn.example/"},
		DefaultLocale: "ru",
		DatabaseFile:  "db.sqlite",
		Port:          "8080",
		GitHubAppName: "dawnl-ink",
	}
	if err := valid.Validate(); err != nil {
		t.Fatalf("valid config rejected: %v", err)
	}

	invalid := valid
	invalid.AppSecret = "change-me"
	if err := invalid.Validate(); err == nil {
		t.Fatal("insecure APP_SECRET accepted")
	}
}

func TestValidateDownloadAnalytics(t *testing.T) {
	base := Config{
		GitHubAppID:   1,
		GitHubPEMPath: "key.pem",
		AppSecret:     "01234567890123456789012345678901",
		BaseURL:       "https://dawn.example/",
		PublicURLs:    []string{"https://dawn.example/"},
	}
	if err := base.Validate(); err != nil {
		t.Fatalf("default analytics off: %v", err)
	}

	viewNoAuth := base
	viewNoAuth.DownloadAnalyticsCollect = true
	viewNoAuth.DownloadAnalyticsView = true
	if err := viewNoAuth.Validate(); err == nil {
		t.Fatal("view without OAuth or admin secret accepted")
	}

	adminOnly := base
	adminOnly.DownloadAnalyticsCollect = true
	adminOnly.DownloadAnalyticsView = true
	adminOnly.DownloadAnalyticsAdminSecret = "01234567890123456789012345678901"
	if err := adminOnly.Validate(); err != nil {
		t.Fatalf("admin-only analytics config: %v", err)
	}

	view := base
	view.GitHubClientID = "client"
	view.GitHubClientSecret = "secret"
	view.DownloadAnalyticsCollect = true
	view.DownloadAnalyticsView = true
	if err := view.Validate(); err != nil {
		t.Fatalf("valid analytics config: %v", err)
	}

	viewNoCollect := view
	viewNoCollect.DownloadAnalyticsCollect = false
	if err := viewNoCollect.Validate(); err == nil {
		t.Fatal("view without collect accepted")
	}
}

func TestEnvBool(t *testing.T) {
	t.Setenv("TEST_BOOL_FLAG", "true")
	if !envBool("TEST_BOOL_FLAG") {
		t.Fatal("expected true")
	}
	t.Setenv("TEST_BOOL_FLAG", "off")
	if envBool("TEST_BOOL_FLAG") {
		t.Fatal("expected false")
	}
}

func TestParsePublicURLs(t *testing.T) {
	urls := parsePublicURLs("https://primary.example/, https://secondary.example/", "8080")
	if len(urls) != 2 {
		t.Fatalf("len(urls) = %d, want 2", len(urls))
	}
	if urls[0] != "https://primary.example/" {
		t.Fatalf("primary URL = %q", urls[0])
	}
	if urls[1] != "https://secondary.example/" {
		t.Fatalf("secondary URL = %q", urls[1])
	}
}

func TestPrimaryDomainUsesFirstPublicURL(t *testing.T) {
	cfg := Config{
		BaseURL:    "https://fallback.example/",
		PublicURLs: []string{"https://primary.example:8443/", "https://secondary.example/"},
	}
	if got := cfg.PrimaryDomain(); got != "primary.example" {
		t.Fatalf("PrimaryDomain() = %q, want primary.example", got)
	}
}

func TestBaseURLForRequest(t *testing.T) {
	cfg := Config{
		BaseURL:    "https://primary.example/",
		PublicURLs: []string{"https://primary.example/", "https://secondary.example/"},
	}

	r := httptest.NewRequest("GET", "https://secondary.example/owner/repo", nil)
	r.Host = "secondary.example"
	if got := cfg.BaseURLForRequest(r); got != "https://secondary.example/" {
		t.Fatalf("BaseURLForRequest = %q, want https://secondary.example/", got)
	}

	r = httptest.NewRequest("GET", "https://unknown.example/", nil)
	r.Host = "unknown.example"
	if got := cfg.BaseURLForRequest(r); got != "https://primary.example/" {
		t.Fatalf("unknown host fallback = %q, want primary URL", got)
	}

	r = httptest.NewRequest("GET", "https://primary.example/", nil)
	r.Host = "primary.example"
	r.Header.Set("X-Forwarded-Host", "secondary.example")
	if got := cfg.BaseURLForRequest(r); got != "https://secondary.example/" {
		t.Fatalf("forwarded host = %q, want https://secondary.example/", got)
	}
}
