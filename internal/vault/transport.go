// Package vault implements Vault-based authentication for the Dome SDK.
// This is an internal package — SDK consumers never interact with it directly.
package vault

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"
)

// AuthConfig configures Vault-based authentication for the SDK.
type AuthConfig struct {
	// VaultAddr is the Vault server address.
	VaultAddr string

	// Role is the Vault auth role name (K8s auth or AppRole).
	Role string

	// OIDCRole is the identity OIDC role name (e.g., "dome-agent").
	// Tokens are read from identity/oidc/token/<OIDCRole>.
	OIDCRole string

	// AuthMethod selects the Vault auth backend: "kubernetes" (default) or "approle".
	AuthMethod string

	// ServiceAccountTokenPath is the path to the K8s SA JWT token.
	// Defaults to /var/run/secrets/kubernetes.io/serviceaccount/token.
	ServiceAccountTokenPath string

	// AppRoleID and AppSecretID are used when AuthMethod is "approle".
	AppRoleID   string
	AppSecretID string
}

// Transport implements http.RoundTripper. It authenticates to Vault,
// requests an OIDC identity token, and injects it as a Bearer token into
// every outgoing request. Tokens are cached and refreshed before expiry.
type Transport struct {
	base     http.RoundTripper
	config   AuthConfig
	ReadFile func(string) ([]byte, error) // for testability

	Mu         sync.Mutex
	OidcToken  string
	OidcExpiry time.Time
	vaultToken string
	vaultExp   time.Time
}

// NewTransport creates a new Vault-authenticated HTTP transport.
func NewTransport(base http.RoundTripper, config AuthConfig) *Transport {
	if base == nil {
		base = http.DefaultTransport
	}
	if config.AuthMethod == "" {
		config.AuthMethod = "kubernetes"
	}
	if config.ServiceAccountTokenPath == "" {
		config.ServiceAccountTokenPath = "/var/run/secrets/kubernetes.io/serviceaccount/token"
	}
	return &Transport{
		base:     base,
		config:   config,
		ReadFile: os.ReadFile,
	}
}

// RoundTrip implements http.RoundTripper.
func (t *Transport) RoundTrip(req *http.Request) (*http.Response, error) {
	if req.Header.Get("Authorization") != "" {
		return t.base.RoundTrip(req)
	}

	token, err := t.getOIDCToken()
	if err != nil {
		return nil, fmt.Errorf("dome: vault auth: %w", err)
	}

	clone := req.Clone(req.Context())
	clone.Header.Set("Authorization", "Bearer "+token)
	return t.base.RoundTrip(clone)
}

// getOIDCToken returns a cached OIDC token or fetches a new one.
func (t *Transport) getOIDCToken() (string, error) {
	t.Mu.Lock()
	defer t.Mu.Unlock()

	// Return cached token if still valid (with 30s buffer).
	if t.OidcToken != "" && time.Now().Add(30*time.Second).Before(t.OidcExpiry) {
		return t.OidcToken, nil
	}

	// Ensure we have a valid Vault native token.
	if err := t.ensureVaultToken(); err != nil {
		return "", err
	}

	// Request OIDC identity token.
	token, expiry, err := t.requestOIDCToken()
	if err != nil {
		return "", err
	}

	t.OidcToken = token
	t.OidcExpiry = expiry
	return token, nil
}

// ensureVaultToken authenticates to Vault if the native token is expired or absent.
func (t *Transport) ensureVaultToken() error {
	if t.vaultToken != "" && time.Now().Add(60*time.Second).Before(t.vaultExp) {
		return nil
	}

	switch t.config.AuthMethod {
	case "kubernetes":
		return t.loginKubernetes()
	case "approle":
		return t.loginAppRole()
	default:
		return fmt.Errorf("unsupported vault auth method: %q", t.config.AuthMethod)
	}
}

// loginKubernetes authenticates to Vault using a K8s ServiceAccount JWT.
func (t *Transport) loginKubernetes() error {
	saTokenBytes, err := t.ReadFile(t.config.ServiceAccountTokenPath)
	if err != nil {
		return fmt.Errorf("read SA token: %w", err)
	}

	body := fmt.Sprintf(`{"role":%q,"jwt":%q}`, t.config.Role, strings.TrimSpace(string(saTokenBytes)))
	return t.vaultLogin("auth/kubernetes/login", body)
}

// loginAppRole authenticates to Vault using AppRole credentials.
func (t *Transport) loginAppRole() error {
	body := fmt.Sprintf(`{"role_id":%q,"secret_id":%q}`, t.config.AppRoleID, t.config.AppSecretID)
	return t.vaultLogin("auth/approle/login", body)
}

// vaultLogin performs a Vault login and stores the resulting token.
func (t *Transport) vaultLogin(path, body string) error {
	url := t.config.VaultAddr + "/v1/" + path

	req, err := http.NewRequest(http.MethodPost, url, strings.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := t.base.RoundTrip(req)
	if err != nil {
		return fmt.Errorf("vault login request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("vault login failed (HTTP %d): %s", resp.StatusCode, string(respBody))
	}

	var result authResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return fmt.Errorf("decode vault login response: %w", err)
	}

	if result.Auth.ClientToken == "" {
		return fmt.Errorf("vault login response missing client_token")
	}

	t.vaultToken = result.Auth.ClientToken
	t.vaultExp = time.Now().Add(time.Duration(result.Auth.LeaseDuration) * time.Second)
	return nil
}

// requestOIDCToken fetches a signed JWT from Vault's identity OIDC engine.
func (t *Transport) requestOIDCToken() (string, time.Time, error) {
	url := t.config.VaultAddr + "/v1/identity/oidc/token/" + t.config.OIDCRole

	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return "", time.Time{}, err
	}
	req.Header.Set("X-Vault-Token", t.vaultToken)

	resp, err := t.base.RoundTrip(req)
	if err != nil {
		return "", time.Time{}, fmt.Errorf("vault OIDC token request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return "", time.Time{}, fmt.Errorf("vault OIDC token failed (HTTP %d): %s", resp.StatusCode, string(respBody))
	}

	var result oidcResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", time.Time{}, fmt.Errorf("decode vault OIDC response: %w", err)
	}

	if result.Data.Token == "" {
		return "", time.Time{}, fmt.Errorf("vault OIDC response missing token")
	}

	// TTL is in seconds from now.
	expiry := time.Now().Add(time.Duration(result.Data.TTL) * time.Second)
	return result.Data.Token, expiry, nil
}

// Vault API response types (minimal subset needed for auth).
type authResponse struct {
	Auth struct {
		ClientToken   string `json:"client_token"`
		LeaseDuration int    `json:"lease_duration"`
	} `json:"auth"`
}

type oidcResponse struct {
	Data struct {
		Token string `json:"token"`
		TTL   int    `json:"ttl"`
	} `json:"data"`
}
