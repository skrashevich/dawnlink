package handlers

import (
	"net/http/httptest"
	"testing"

	"github.com/skrashevich/dawnlink/internal/config"
)

func TestUserSessionRoundTrip(t *testing.T) {
	cfg := config.Config{AppSecret: "01234567890123456789012345678901"}
	srv := &Server{cfg: cfg}
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "https://example.com/dashboard", nil)
	srv.setUserSession(rec, req, "github-user-token")

	req2 := httptest.NewRequest("GET", "https://example.com/dashboard", nil)
	for _, c := range rec.Result().Cookies() {
		req2.AddCookie(c)
	}
	got, ok := srv.readUserSession(req2)
	if !ok || got != "github-user-token" {
		t.Fatalf("readUserSession() = %q, %v", got, ok)
	}
}

func TestOAuthReturnPath(t *testing.T) {
	if oauthReturnPath("/analytics/downloads") != "/analytics/downloads" {
		t.Fatal("expected analytics path")
	}
	if oauthReturnPath("https://evil.example/steal") != "/dashboard" {
		t.Fatal("unexpected open redirect allowed")
	}
}
