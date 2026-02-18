package vault

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestTransport_AppRoleLogin_OIDCToken(t *testing.T) {
	var (
		loginReceived bool
		oidcRequested bool
		bearerSeen    string
	)

	// Mock Vault server.
	vaultSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/v1/auth/approle/login":
			loginReceived = true
			_ = json.NewEncoder(w).Encode(authResponse{
				Auth: struct {
					ClientToken   string `json:"client_token"`
					LeaseDuration int    `json:"lease_duration"`
				}{
					ClientToken:   "vault-token-abc",
					LeaseDuration: 3600,
				},
			})
		case r.Method == http.MethodGet && r.URL.Path == "/v1/identity/oidc/token/dome-agent":
			oidcRequested = true
			if r.Header.Get("X-Vault-Token") != "vault-token-abc" {
				t.Errorf("OIDC request missing vault token, got %q", r.Header.Get("X-Vault-Token"))
			}
			_ = json.NewEncoder(w).Encode(oidcResponse{
				Data: struct {
					Token string `json:"token"`
					TTL   int    `json:"ttl"`
				}{
					Token: "oidc-jwt-xyz",
					TTL:   3600,
				},
			})
		default:
			t.Errorf("unexpected vault request: %s %s", r.Method, r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer vaultSrv.Close()

	// Mock downstream API.
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		bearerSeen = r.Header.Get("Authorization")
		w.WriteHeader(http.StatusOK)
	}))
	defer api.Close()

	transport := NewTransport(http.DefaultTransport, AuthConfig{
		VaultAddr:   vaultSrv.URL,
		Role:        "test-role",
		OIDCRole:    "dome-agent",
		AuthMethod:  "approle",
		AppRoleID:   "role-123",
		AppSecretID: "secret-456",
	})

	client := &http.Client{Transport: transport}
	resp, err := client.Get(api.URL + "/api/v1/agents")
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if !loginReceived {
		t.Error("Vault AppRole login was not called")
	}
	if !oidcRequested {
		t.Error("Vault OIDC token was not requested")
	}
	if bearerSeen != "Bearer oidc-jwt-xyz" {
		t.Errorf("bearer token = %q, want %q", bearerSeen, "Bearer oidc-jwt-xyz")
	}
}

func TestTransport_KubernetesLogin(t *testing.T) {
	var loginBody string

	vaultSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/v1/auth/kubernetes/login":
			body := make([]byte, 1024)
			n, _ := r.Body.Read(body)
			loginBody = string(body[:n])
			_ = json.NewEncoder(w).Encode(authResponse{
				Auth: struct {
					ClientToken   string `json:"client_token"`
					LeaseDuration int    `json:"lease_duration"`
				}{
					ClientToken:   "vault-k8s-token",
					LeaseDuration: 3600,
				},
			})
		case r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, "/v1/identity/oidc/token/"):
			_ = json.NewEncoder(w).Encode(oidcResponse{
				Data: struct {
					Token string `json:"token"`
					TTL   int    `json:"ttl"`
				}{
					Token: "k8s-oidc-jwt",
					TTL:   3600,
				},
			})
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer vaultSrv.Close()

	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer api.Close()

	transport := NewTransport(http.DefaultTransport, AuthConfig{
		VaultAddr:  vaultSrv.URL,
		Role:       "dome-k8s-role",
		OIDCRole:   "dome-agent",
		AuthMethod: "kubernetes",
	})
	// Mock the SA token file read.
	transport.ReadFile = func(path string) ([]byte, error) {
		return []byte("fake-sa-jwt-token"), nil
	}

	client := &http.Client{Transport: transport}
	resp, err := client.Get(api.URL + "/test")
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if !strings.Contains(loginBody, `"jwt":"fake-sa-jwt-token"`) {
		t.Errorf("K8s login body should contain SA JWT, got: %s", loginBody)
	}
	if !strings.Contains(loginBody, `"role":"dome-k8s-role"`) {
		t.Errorf("K8s login body should contain role, got: %s", loginBody)
	}
}

func TestTransport_CachesToken(t *testing.T) {
	loginCount := 0
	oidcCount := 0

	vaultSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/v1/auth/approle/login":
			loginCount++
			_ = json.NewEncoder(w).Encode(authResponse{
				Auth: struct {
					ClientToken   string `json:"client_token"`
					LeaseDuration int    `json:"lease_duration"`
				}{
					ClientToken:   "vault-token",
					LeaseDuration: 3600,
				},
			})
		case r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, "/v1/identity/oidc/token/"):
			oidcCount++
			_ = json.NewEncoder(w).Encode(oidcResponse{
				Data: struct {
					Token string `json:"token"`
					TTL   int    `json:"ttl"`
				}{
					Token: "cached-oidc-jwt",
					TTL:   3600,
				},
			})
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer vaultSrv.Close()

	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer api.Close()

	transport := NewTransport(http.DefaultTransport, AuthConfig{
		VaultAddr:   vaultSrv.URL,
		Role:        "role",
		OIDCRole:    "dome-agent",
		AuthMethod:  "approle",
		AppRoleID:   "r",
		AppSecretID: "s",
	})

	client := &http.Client{Transport: transport}

	// Make 3 requests — should only login + get OIDC once.
	for i := 0; i < 3; i++ {
		resp, err := client.Get(api.URL + "/test")
		if err != nil {
			t.Fatalf("request %d failed: %v", i, err)
		}
		_ = resp.Body.Close()
	}

	if loginCount != 1 {
		t.Errorf("vault login count = %d, want 1 (should be cached)", loginCount)
	}
	if oidcCount != 1 {
		t.Errorf("OIDC token count = %d, want 1 (should be cached)", oidcCount)
	}
}

func TestTransport_RefreshesExpiredToken(t *testing.T) {
	oidcCount := 0

	vaultSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/v1/auth/approle/login":
			_ = json.NewEncoder(w).Encode(authResponse{
				Auth: struct {
					ClientToken   string `json:"client_token"`
					LeaseDuration int    `json:"lease_duration"`
				}{
					ClientToken:   "vault-token",
					LeaseDuration: 3600,
				},
			})
		case r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, "/v1/identity/oidc/token/"):
			oidcCount++
			_ = json.NewEncoder(w).Encode(oidcResponse{
				Data: struct {
					Token string `json:"token"`
					TTL   int    `json:"ttl"`
				}{
					Token: "refreshed-jwt",
					TTL:   3600,
				},
			})
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer vaultSrv.Close()

	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer api.Close()

	transport := NewTransport(http.DefaultTransport, AuthConfig{
		VaultAddr:   vaultSrv.URL,
		Role:        "role",
		OIDCRole:    "dome-agent",
		AuthMethod:  "approle",
		AppRoleID:   "r",
		AppSecretID: "s",
	})

	client := &http.Client{Transport: transport}

	// First request.
	resp, err := client.Get(api.URL + "/test")
	if err != nil {
		t.Fatalf("request 1 failed: %v", err)
	}
	_ = resp.Body.Close()

	// Force expiry.
	transport.Mu.Lock()
	transport.OidcExpiry = time.Now().Add(-1 * time.Minute)
	transport.Mu.Unlock()

	// Second request — should refresh.
	resp, err = client.Get(api.URL + "/test")
	if err != nil {
		t.Fatalf("request 2 failed: %v", err)
	}
	_ = resp.Body.Close()

	if oidcCount != 2 {
		t.Errorf("OIDC token count = %d, want 2 (should refresh after expiry)", oidcCount)
	}
}

func TestTransport_SkipsExistingAuthHeader(t *testing.T) {
	vaultSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("vault should not be called when auth header is already set")
		w.WriteHeader(http.StatusNotFound)
	}))
	defer vaultSrv.Close()

	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer existing-token" {
			t.Errorf("auth header = %q, want %q", r.Header.Get("Authorization"), "Bearer existing-token")
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer api.Close()

	transport := NewTransport(http.DefaultTransport, AuthConfig{
		VaultAddr:   vaultSrv.URL,
		Role:        "role",
		OIDCRole:    "dome-agent",
		AuthMethod:  "approle",
		AppRoleID:   "r",
		AppSecretID: "s",
	})

	req, _ := http.NewRequest(http.MethodGet, api.URL+"/test", nil)
	req.Header.Set("Authorization", "Bearer existing-token")

	resp, err := (&http.Client{Transport: transport}).Do(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	_ = resp.Body.Close()
}

func TestTransport_LoginFailure(t *testing.T) {
	vaultSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(`{"errors":["permission denied"]}`))
	}))
	defer vaultSrv.Close()

	transport := NewTransport(http.DefaultTransport, AuthConfig{
		VaultAddr:   vaultSrv.URL,
		Role:        "role",
		OIDCRole:    "dome-agent",
		AuthMethod:  "approle",
		AppRoleID:   "r",
		AppSecretID: "s",
	})

	req, _ := http.NewRequest(http.MethodGet, "http://localhost/test", nil)
	_, err := transport.RoundTrip(req)
	if err == nil {
		t.Fatal("expected error on login failure")
	}
	if !strings.Contains(err.Error(), "vault login failed") {
		t.Errorf("error = %q, want it to mention vault login failure", err)
	}
}
