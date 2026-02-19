// Package tokenexchange implements token exchange authentication for the Dome SDK.
// Instead of talking to Vault directly, the SDK exchanges AppRole credentials
// for a JWT via the Dome API server's POST /api/v1/auth/token endpoint.
package tokenexchange

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"
)

// Config configures the token exchange transport.
type Config struct {
	// APIURL is the Dome API server URL (e.g., "https://api.dome.example.com").
	APIURL string
	// RoleID is the AppRole role_id from the credential blob.
	RoleID string
	// SecretID is the AppRole secret_id from the credential blob.
	SecretID string
}

// Transport implements http.RoundTripper. It exchanges AppRole credentials
// for a JWT via the Dome API server, caches the JWT, and injects it as a
// Bearer token into every outgoing request.
type Transport struct {
	base   http.RoundTripper
	config Config

	mu      sync.Mutex
	token   string
	expiry  time.Time
	nowFunc func() time.Time // for testing
}

// NewTransport creates a new token-exchange HTTP transport.
func NewTransport(base http.RoundTripper, config Config) *Transport {
	if base == nil {
		base = http.DefaultTransport
	}
	return &Transport{
		base:    base,
		config:  config,
		nowFunc: time.Now,
	}
}

// tokenEndpoint returns the full URL for the token exchange endpoint.
func (t *Transport) tokenEndpoint() string {
	base := strings.TrimRight(t.config.APIURL, "/")
	return base + "/api/v1/auth/token"
}

// RoundTrip implements http.RoundTripper.
func (t *Transport) RoundTrip(req *http.Request) (*http.Response, error) {
	// Skip injection for requests that already have auth or are hitting
	// the token endpoint itself (prevents infinite recursion).
	if req.Header.Get("Authorization") != "" {
		return t.base.RoundTrip(req)
	}
	if req.URL.String() == t.tokenEndpoint() {
		return t.base.RoundTrip(req)
	}

	token, err := t.getToken()
	if err != nil {
		return nil, fmt.Errorf("dome: token exchange: %w", err)
	}

	clone := req.Clone(req.Context())
	clone.Header.Set("Authorization", "Bearer "+token)
	return t.base.RoundTrip(clone)
}

// getToken returns a cached JWT or exchanges credentials for a new one.
func (t *Transport) getToken() (string, error) {
	t.mu.Lock()
	defer t.mu.Unlock()

	now := t.nowFunc()

	// Return cached token if still valid (with 30s buffer before expiry).
	if t.token != "" && now.Add(30*time.Second).Before(t.expiry) {
		return t.token, nil
	}

	// Exchange credentials for a new JWT.
	token, expiresIn, err := t.exchange()
	if err != nil {
		return "", err
	}

	t.token = token
	t.expiry = now.Add(time.Duration(expiresIn) * time.Second)
	return token, nil
}

// exchange calls the Dome API token exchange endpoint.
func (t *Transport) exchange() (token string, expiresIn int, err error) {
	body, _ := json.Marshal(map[string]string{
		"grant_type": "approle",
		"role_id":    t.config.RoleID,
		"secret_id":  t.config.SecretID,
	})

	req, err := http.NewRequest(http.MethodPost, t.tokenEndpoint(), bytes.NewReader(body))
	if err != nil {
		return "", 0, err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := t.base.RoundTrip(req)
	if err != nil {
		return "", 0, fmt.Errorf("token exchange request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return "", 0, fmt.Errorf("token exchange failed (HTTP %d): %s", resp.StatusCode, string(respBody))
	}

	var result struct {
		AccessToken string `json:"access_token"`
		TokenType   string `json:"token_type"`
		ExpiresIn   int    `json:"expires_in"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", 0, fmt.Errorf("decode token exchange response: %w", err)
	}

	if result.AccessToken == "" {
		return "", 0, fmt.Errorf("token exchange response missing access_token")
	}

	return result.AccessToken, result.ExpiresIn, nil
}
