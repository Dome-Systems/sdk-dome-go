package tokenexchange

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

func TestTransport_ExchangeAndInjectBearer(t *testing.T) {
	var exchangeCalls int32

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/auth/token":
			atomic.AddInt32(&exchangeCalls, 1)

			var req map[string]string
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				t.Errorf("decode request: %v", err)
				w.WriteHeader(http.StatusBadRequest)
				return
			}

			if req["role_id"] != "test-role" || req["secret_id"] != "test-secret" {
				t.Errorf("unexpected credentials: %v", req)
				w.WriteHeader(http.StatusUnauthorized)
				return
			}

			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"access_token": "test-jwt-token",
				"token_type":   "Bearer",
				"expires_in":   3600,
			})

		case "/test-api":
			auth := r.Header.Get("Authorization")
			if auth != "Bearer test-jwt-token" {
				t.Errorf("Authorization = %q, want %q", auth, "Bearer test-jwt-token")
				w.WriteHeader(http.StatusUnauthorized)
				return
			}
			w.WriteHeader(http.StatusOK)

		default:
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	transport := NewTransport(http.DefaultTransport, Config{
		APIURL:   server.URL,
		RoleID:   "test-role",
		SecretID: "test-secret",
	})

	client := &http.Client{Transport: transport}
	resp, err := client.Get(server.URL + "/test-api")
	if err != nil {
		t.Fatalf("request error: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusOK)
	}

	if got := atomic.LoadInt32(&exchangeCalls); got != 1 {
		t.Errorf("exchange calls = %d, want 1", got)
	}
}

func TestTransport_TokenCaching(t *testing.T) {
	var exchangeCalls int32

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/auth/token":
			atomic.AddInt32(&exchangeCalls, 1)
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"access_token": "cached-jwt",
				"token_type":   "Bearer",
				"expires_in":   3600,
			})

		case "/test-api":
			w.WriteHeader(http.StatusOK)

		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	transport := NewTransport(http.DefaultTransport, Config{
		APIURL:   server.URL,
		RoleID:   "test-role",
		SecretID: "test-secret",
	})

	client := &http.Client{Transport: transport}

	// Make 5 requests — should only exchange once.
	for i := 0; i < 5; i++ {
		resp, err := client.Get(server.URL + "/test-api")
		if err != nil {
			t.Fatalf("request %d error: %v", i+1, err)
		}
		_ = resp.Body.Close()
	}

	if got := atomic.LoadInt32(&exchangeCalls); got != 1 {
		t.Errorf("exchange calls = %d, want 1 (should be cached)", got)
	}
}

func TestTransport_TokenRefreshOnExpiry(t *testing.T) {
	var exchangeCalls int32
	now := time.Now()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/auth/token":
			n := atomic.AddInt32(&exchangeCalls, 1)
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"access_token": "jwt-v" + string(rune('0'+n)),
				"token_type":   "Bearer",
				"expires_in":   60, // 60 seconds
			})

		case "/test-api":
			w.WriteHeader(http.StatusOK)

		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	transport := NewTransport(http.DefaultTransport, Config{
		APIURL:   server.URL,
		RoleID:   "test-role",
		SecretID: "test-secret",
	})
	// Override nowFunc to control time.
	transport.nowFunc = func() time.Time { return now }

	client := &http.Client{Transport: transport}

	// First request — triggers exchange.
	resp, err := client.Get(server.URL + "/test-api")
	if err != nil {
		t.Fatalf("request 1 error: %v", err)
	}
	_ = resp.Body.Close()

	if got := atomic.LoadInt32(&exchangeCalls); got != 1 {
		t.Fatalf("exchange calls after request 1 = %d, want 1", got)
	}

	// Advance time past the 30s buffer (token expires at +60s, buffer is 30s,
	// so at +31s the token should still be valid).
	now = now.Add(31 * time.Second)
	transport.nowFunc = func() time.Time { return now }

	resp, err = client.Get(server.URL + "/test-api")
	if err != nil {
		t.Fatalf("request 2 error: %v", err)
	}
	_ = resp.Body.Close()

	// At +31s with 60s expiry and 30s buffer, token should be refreshed.
	if got := atomic.LoadInt32(&exchangeCalls); got != 2 {
		t.Errorf("exchange calls after expiry = %d, want 2", got)
	}
}

func TestTransport_SkipsExistingAuth(t *testing.T) {
	var exchangeCalled bool

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/auth/token":
			exchangeCalled = true
			w.WriteHeader(http.StatusOK)

		case "/test-api":
			auth := r.Header.Get("Authorization")
			if auth != "Bearer existing-token" {
				t.Errorf("Authorization = %q, want %q", auth, "Bearer existing-token")
			}
			w.WriteHeader(http.StatusOK)

		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	transport := NewTransport(http.DefaultTransport, Config{
		APIURL:   server.URL,
		RoleID:   "test-role",
		SecretID: "test-secret",
	})

	req, _ := http.NewRequest(http.MethodGet, server.URL+"/test-api", nil)
	req.Header.Set("Authorization", "Bearer existing-token")

	resp, err := transport.RoundTrip(req)
	if err != nil {
		t.Fatalf("request error: %v", err)
	}
	_ = resp.Body.Close()

	if exchangeCalled {
		t.Error("exchange should not be called when Authorization header is already set")
	}
}

func TestTransport_ExchangeError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_ = json.NewEncoder(w).Encode(map[string]string{
			"error": "invalid_client",
		})
	}))
	defer server.Close()

	transport := NewTransport(http.DefaultTransport, Config{
		APIURL:   server.URL,
		RoleID:   "bad-role",
		SecretID: "bad-secret",
	})

	req, _ := http.NewRequest(http.MethodGet, server.URL+"/test-api", nil)
	_, err := transport.RoundTrip(req)
	if err == nil {
		t.Fatal("expected error on exchange failure")
	}
}
