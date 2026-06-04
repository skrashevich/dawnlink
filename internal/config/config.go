package config

import (
	"fmt"
	"net"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
)

type Config struct {
	GitHubAppName          string
	GitHubAppID            int64
	GitHubClientID         string
	GitHubClientSecret     string
	GitHubPEMPath          string
	AppSecret              string
	FallbackInstallationID int64
	Port                   string
	BaseURL                string
	PublicURLs             []string
	DatabaseFile           string
	DefaultLocale          string

	DownloadAnalyticsCollect   bool
	DownloadAnalyticsView      bool
	DownloadAnalyticsSecret    string
	DownloadAnalyticsRetention int
}

func Load() Config {
	appID, _ := strconv.ParseInt(os.Getenv("GITHUB_APP_ID"), 10, 64)
	fallbackID, _ := strconv.ParseInt(os.Getenv("FALLBACK_INSTALLATION_ID"), 10, 64)
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}
	publicURLs := parsePublicURLs(os.Getenv("URL"), port)
	baseURL := publicURLs[0]
	db := os.Getenv("DATABASE_FILE")
	if db == "" {
		db = "./db.sqlite"
	}
	locale := os.Getenv("DEFAULT_LOCALE")
	if locale == "" {
		locale = "en"
	}
	retention, _ := strconv.Atoi(os.Getenv("DOWNLOAD_ANALYTICS_RETENTION_DAYS"))
	if retention == 0 && envBool("DOWNLOAD_ANALYTICS_COLLECT") {
		retention = 180
	}
	return Config{
		GitHubAppName:          envOr("GITHUB_APP_NAME", "dawnl-ink"),
		GitHubAppID:            appID,
		GitHubClientID:         os.Getenv("GITHUB_CLIENT_ID"),
		GitHubClientSecret:     os.Getenv("GITHUB_CLIENT_SECRET"),
		GitHubPEMPath:          os.Getenv("GITHUB_PEM_FILENAME"),
		AppSecret:              os.Getenv("APP_SECRET"),
		FallbackInstallationID: fallbackID,
		Port:                   port,
		BaseURL:                baseURL,
		PublicURLs:             publicURLs,
		DatabaseFile:           db,
		DefaultLocale:          locale,

		DownloadAnalyticsCollect:   envBool("DOWNLOAD_ANALYTICS_COLLECT"),
		DownloadAnalyticsView:      envBool("DOWNLOAD_ANALYTICS_VIEW"),
		DownloadAnalyticsSecret:    os.Getenv("DOWNLOAD_ANALYTICS_SECRET"),
		DownloadAnalyticsRetention: retention,
	}
}

func (c Config) Validate() error {
	var problems []string
	if c.GitHubAppID <= 0 {
		problems = append(problems, "GITHUB_APP_ID must be a positive integer")
	}
	if c.GitHubPEMPath == "" {
		problems = append(problems, "GITHUB_PEM_FILENAME is required")
	}
	if len(c.AppSecret) < 32 || c.AppSecret == "change-me" {
		problems = append(problems, "APP_SECRET must be a random value of at least 32 characters")
	}
	if (c.GitHubClientID == "") != (c.GitHubClientSecret == "") {
		problems = append(problems, "GITHUB_CLIENT_ID and GITHUB_CLIENT_SECRET must be configured together")
	}
	if c.DownloadAnalyticsView {
		if c.DownloadAnalyticsSecret == "" {
			problems = append(problems, "DOWNLOAD_ANALYTICS_SECRET is required when DOWNLOAD_ANALYTICS_VIEW is enabled")
		} else if len(c.DownloadAnalyticsSecret) < 16 {
			problems = append(problems, "DOWNLOAD_ANALYTICS_SECRET must be at least 16 characters when analytics UI is enabled")
		}
	}
	if c.DownloadAnalyticsView && !c.DownloadAnalyticsCollect {
		problems = append(problems, "DOWNLOAD_ANALYTICS_COLLECT must be enabled when DOWNLOAD_ANALYTICS_VIEW is enabled")
	}
	for i, raw := range c.PublicURLs {
		base, err := url.Parse(raw)
		if err != nil || base.Scheme == "" || base.Host == "" {
			problems = append(problems, fmt.Sprintf("URL[%d] must be an absolute URL", i))
		}
	}
	if len(problems) > 0 {
		return fmt.Errorf("invalid configuration: %s", strings.Join(problems, "; "))
	}
	return nil
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func envBool(key string) bool {
	v := strings.TrimSpace(strings.ToLower(os.Getenv(key)))
	return v == "1" || v == "true" || v == "yes" || v == "on"
}

func parsePublicURLs(raw, port string) []string {
	if raw == "" {
		return []string{normalizeBaseURL("http://localhost:" + port + "/")}
	}
	parts := strings.Split(raw, ",")
	urls := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		urls = append(urls, normalizeBaseURL(part))
	}
	if len(urls) == 0 {
		return []string{normalizeBaseURL("http://localhost:" + port + "/")}
	}
	return urls
}

func normalizeBaseURL(raw string) string {
	if !strings.HasSuffix(raw, "/") {
		return raw + "/"
	}
	return raw
}

// BaseURLForRequest returns the configured public URL that matches the request
// host, or the primary URL when no match is found.
func (c Config) BaseURLForRequest(r *http.Request) string {
	if r == nil {
		return c.BaseURL
	}
	host := requestHost(r)
	for _, u := range c.PublicURLs {
		if publicURLHost(u) == host {
			return u
		}
	}
	return c.BaseURL
}

func requestHost(r *http.Request) string {
	host := r.Host
	if fwd := r.Header.Get("X-Forwarded-Host"); fwd != "" {
		host = strings.TrimSpace(strings.Split(fwd, ",")[0])
	}
	return normalizeHost(host)
}

func publicURLHost(raw string) string {
	u, err := url.Parse(raw)
	if err != nil {
		return ""
	}
	return normalizeHost(u.Host)
}

func normalizeHost(host string) string {
	host = strings.ToLower(strings.TrimSpace(host))
	if h, _, err := net.SplitHostPort(host); err == nil {
		return h
	}
	return host
}
