package config

import (
	"fmt"
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
	DatabaseFile           string
	DefaultLocale          string
}

func Load() Config {
	appID, _ := strconv.ParseInt(os.Getenv("GITHUB_APP_ID"), 10, 64)
	fallbackID, _ := strconv.ParseInt(os.Getenv("FALLBACK_INSTALLATION_ID"), 10, 64)
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}
	baseURL := os.Getenv("URL")
	if baseURL == "" {
		baseURL = "http://localhost:" + port + "/"
	}
	if baseURL[len(baseURL)-1] != '/' {
		baseURL += "/"
	}
	db := os.Getenv("DATABASE_FILE")
	if db == "" {
		db = "./db.sqlite"
	}
	locale := os.Getenv("DEFAULT_LOCALE")
	if locale == "" {
		locale = "en"
	}
	return Config{
		GitHubAppName:          envOr("GITHUB_APP_NAME", "dawnlink"),
		GitHubAppID:            appID,
		GitHubClientID:         os.Getenv("GITHUB_CLIENT_ID"),
		GitHubClientSecret:     os.Getenv("GITHUB_CLIENT_SECRET"),
		GitHubPEMPath:          os.Getenv("GITHUB_PEM_FILENAME"),
		AppSecret:              os.Getenv("APP_SECRET"),
		FallbackInstallationID: fallbackID,
		Port:                   port,
		BaseURL:                baseURL,
		DatabaseFile:           db,
		DefaultLocale:          locale,
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
	base, err := url.Parse(c.BaseURL)
	if err != nil || base.Scheme == "" || base.Host == "" {
		problems = append(problems, "URL must be an absolute URL")
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
