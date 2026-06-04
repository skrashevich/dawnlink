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
		GitHubAppName: "dawnlink",
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

func TestParsePublicURLs(t *testing.T) {
	urls := parsePublicURLs("https://dawnlink.svk.su/, https://dawnlink.svk.app/", "8080")
	if len(urls) != 2 {
		t.Fatalf("len(urls) = %d, want 2", len(urls))
	}
	if urls[0] != "https://dawnlink.svk.su/" {
		t.Fatalf("primary URL = %q", urls[0])
	}
	if urls[1] != "https://dawnlink.svk.app/" {
		t.Fatalf("secondary URL = %q", urls[1])
	}
}

func TestBaseURLForRequest(t *testing.T) {
	cfg := Config{
		BaseURL:    "https://dawnlink.svk.su/",
		PublicURLs: []string{"https://dawnlink.svk.su/", "https://dawnlink.svk.app/"},
	}

	r := httptest.NewRequest("GET", "https://dawnlink.svk.app/owner/repo", nil)
	r.Host = "dawnlink.svk.app"
	if got := cfg.BaseURLForRequest(r); got != "https://dawnlink.svk.app/" {
		t.Fatalf("BaseURLForRequest = %q, want https://dawnlink.svk.app/", got)
	}

	r = httptest.NewRequest("GET", "https://unknown.example/", nil)
	r.Host = "unknown.example"
	if got := cfg.BaseURLForRequest(r); got != "https://dawnlink.svk.su/" {
		t.Fatalf("unknown host fallback = %q, want primary URL", got)
	}

	r = httptest.NewRequest("GET", "https://dawnlink.svk.app/", nil)
	r.Host = "dawnlink.svk.app"
	r.Header.Set("X-Forwarded-Host", "dawnlink.svk.su")
	if got := cfg.BaseURLForRequest(r); got != "https://dawnlink.svk.su/" {
		t.Fatalf("forwarded host = %q, want https://dawnlink.svk.su/", got)
	}
}
