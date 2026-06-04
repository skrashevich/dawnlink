package config

import "testing"

func TestValidate(t *testing.T) {
	valid := Config{
		GitHubAppID:    1,
		GitHubPEMPath:  "key.pem",
		AppSecret:      "01234567890123456789012345678901",
		BaseURL:        "https://dawn.example/",
		DefaultLocale:  "ru",
		DatabaseFile:   "db.sqlite",
		Port:           "8080",
		GitHubAppName:  "dawnlink",
		GitHubClientID: "",
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
