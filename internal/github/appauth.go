package github

import (
	"crypto/rsa"
	"fmt"
	"os"
	"time"

	"github.com/golang-jwt/jwt/v5"

	"github.com/skrashevich/dawnlink/internal/cache"
)

type AppAuth struct {
	appID      int64
	pemPath    string
	privateKey *rsa.PrivateKey

	jwtCache   *cache.TTLCache[int64, AppToken]
	tokenCache *cache.TTLCache[int64, *InstallationToken]
}

func NewAppAuth(appID int64, pemPath string) (*AppAuth, error) {
	a := &AppAuth{
		appID:      appID,
		pemPath:    pemPath,
		jwtCache:   cache.NewTTLCache[int64, AppToken](10, 9*time.Minute),
		tokenCache: cache.NewTTLCache[int64, *InstallationToken](1000, 55*time.Minute),
	}
	if pemPath != "" {
		data, err := os.ReadFile(pemPath)
		if err != nil {
			return nil, err
		}
		key, err := jwt.ParseRSAPrivateKeyFromPEM(data)
		if err != nil {
			return nil, err
		}
		a.privateKey = key
	}
	return a, nil
}

func (a *AppAuth) JWT() (AppToken, error) {
	if a.privateKey == nil {
		return AppToken{}, fmt.Errorf("GITHUB_PEM_FILENAME not configured")
	}
	return a.jwtCache.Get(a.appID, func() (AppToken, error) {
		now := time.Now().UTC()
		claims := jwt.MapClaims{
			"iat": now.Add(-60 * time.Second).Unix(),
			"exp": now.Add(10 * time.Minute).Unix(),
			"iss": a.appID,
		}
		t := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
		signed, err := t.SignedString(a.privateKey)
		if err != nil {
			return AppToken{}, err
		}
		return AppToken{JWT: signed}, nil
	})
}

func (a *AppAuth) InstallationToken(installationID int64) (*InstallationToken, error) {
	if a.privateKey == nil {
		return nil, fmt.Errorf("GITHUB_PEM_FILENAME not configured")
	}
	return a.tokenCache.Get(installationID, func() (*InstallationToken, error) {
		jwtTok, err := a.JWT()
		if err != nil {
			return nil, err
		}
		path := fmt.Sprintf("app/installations/%d/access_tokens", installationID)
		body := map[string]any{
			"permissions": map[string]string{"actions": "read"},
		}
		resp, err := Post(path, jwtTok, body)
		if err != nil {
			return nil, err
		}
		if resp.StatusCode == 404 {
			return nil, fmt.Errorf("installation %d not found", installationID)
		}
		var out struct {
			Token string `json:"token"`
		}
		if err := decodeJSON(resp, &out); err != nil {
			return nil, err
		}
		if out.Token == "" {
			return nil, fmt.Errorf("github installation token response did not include a token")
		}
		return &InstallationToken{Value: out.Token, InstallationID: installationID}, nil
	})
}
