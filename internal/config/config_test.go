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

func TestParsePublicURLs(t *testing.T) {
	urls := parsePublicURLs("https://dawnl.ink/, https://dawnl.ink/", "8080")
	if len(urls) != 2 {
		t.Fatalf("len(urls) = %d, want 2", len(urls))
	}
	if urls[0] != "https://dawnl.ink/" {
		t.Fatalf("primary URL = %q", urls[0])
	}
	if urls[1] != "https://dawnl.ink/" {
		t.Fatalf("secondary URL = %q", urls[1])
	}
}

func TestBaseURLForRequest(t *testing.T) {
	cfg := Config{
		BaseURL:    "https://dawnl.ink/",
		PublicURLs: []string{"https://dawnl.ink/", "https://dawnl.ink/"},
	}

	r := httptest.NewRequest("GET", "https://dawnl.ink/owner/repo", nil)
	r.Host = "dawnl.ink"
	if got := cfg.BaseURLForRequest(r); got != "https://dawnl.ink/" {
		t.Fatalf("BaseURLForRequest = %q, want https://dawnl.ink/", got)
	}

	r = httptest.NewRequest("GET", "https://unknown.example/", nil)
	r.Host = "unknown.example"
	if got := cfg.BaseURLForRequest(r); got != "https://dawnl.ink/" {
		t.Fatalf("unknown host fallback = %q, want primary URL", got)
	}

	r = httptest.NewRequest("GET", "https://dawnl.ink/", nil)
	r.Host = "dawnl.ink"
	r.Header.Set("X-Forwarded-Host", "dawnl.ink")
	if got := cfg.BaseURLForRequest(r); got != "https://dawnl.ink/" {
		t.Fatalf("forwarded host = %q, want https://dawnl.ink/", got)
	}
}
