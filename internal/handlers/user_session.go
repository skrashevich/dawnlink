package handlers

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/skrashevich/dawnlink/internal/github"
)

const (
	sessionCookieName = "dl_oauth"
	sessionMaxAge     = 12 * 3600
)

var allowedOAuthReturnPaths = map[string]bool{
	"/dashboard":           true,
	"/analytics/downloads": true,
}

func oauthReturnPath(state string) string {
	state = strings.TrimSpace(state)
	if allowedOAuthReturnPaths[state] {
		return state
	}
	return "/dashboard"
}

func (s *Server) oauthURL(r *http.Request, returnPath string) string {
	v := url.Values{
		"client_id":    {s.cfg.GitHubClientID},
		"scope":        {""},
		"redirect_uri": {s.abs(r, "/dashboard")},
	}
	if returnPath != "" && returnPath != "/dashboard" {
		v.Set("state", returnPath)
	}
	return "https://github.com/login/oauth/authorize?" + v.Encode()
}

// resolveUserSession returns a GitHub user token from the OAuth callback code or a signed
// session cookie. On success after code exchange it redirects to the OAuth state path.
func (s *Server) resolveUserSession(w http.ResponseWriter, r *http.Request, defaultReturn string) (github.UserToken, error) {
	var none github.UserToken
	if s.cfg.GitHubClientID == "" || s.cfg.GitHubClientSecret == "" {
		return none, &httpError{status: 404, message: "not found"}
	}
	if code := r.URL.Query().Get("code"); code != "" {
		returnPath := oauthReturnPath(r.URL.Query().Get("state"))
		if returnPath == "" {
			returnPath = oauthReturnPath(defaultReturn)
		}
		tokenStr, err := github.ExchangeOAuthCode(s.cfg.GitHubClientID, s.cfg.GitHubClientSecret, code)
		if err != nil {
			if strings.HasPrefix(err.Error(), "bad_verification_code") {
				return none, redirect(defaultReturn, http.StatusFound)
			}
			return none, err
		}
		s.setUserSession(w, r, tokenStr)
		return none, redirect(returnPath, http.StatusFound)
	}
	if token, ok := s.readUserSession(r); ok {
		return github.UserToken{Value: token}, nil
	}
	w.Header().Set("X-Robots-Tag", "noindex")
	return none, redirect(s.oauthURL(r, defaultReturn), http.StatusFound)
}

func (s *Server) setUserSession(w http.ResponseWriter, r *http.Request, token string) {
	exp := time.Now().Add(sessionMaxAge * time.Second).Unix()
	payload := fmt.Sprintf("%d|%s", exp, token)
	mac := signSessionPayload(s.cfg.AppSecret, payload)
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookieName,
		Value:    payload + "." + mac,
		Path:     "/",
		MaxAge:   sessionMaxAge,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Secure:   r.TLS != nil,
	})
}

func (s *Server) readUserSession(r *http.Request) (string, bool) {
	c, err := r.Cookie(sessionCookieName)
	if err != nil || c.Value == "" {
		return "", false
	}
	payload, mac, ok := strings.Cut(c.Value, ".")
	if !ok || !hmac.Equal([]byte(mac), []byte(signSessionPayload(s.cfg.AppSecret, payload))) {
		return "", false
	}
	expStr, token, ok := strings.Cut(payload, "|")
	if !ok || token == "" {
		return "", false
	}
	exp, err := strconv.ParseInt(expStr, 10, 64)
	if err != nil || time.Now().Unix() > exp {
		return "", false
	}
	return token, true
}

func signSessionPayload(secret, payload string) string {
	h := hmac.New(sha256.New, []byte(secret))
	h.Write([]byte("dl_oauth_session\x00"))
	h.Write([]byte(payload))
	return hex.EncodeToString(h.Sum(nil))
}
