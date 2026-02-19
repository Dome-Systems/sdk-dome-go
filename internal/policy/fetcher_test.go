package policy

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestFetcher_Fetch_Success(t *testing.T) {
	bundle := BundleResponse{
		Version: "2026-02-19T00:00:00Z",
		Hash:    "abc123",
		Policies: []PolicyFile{
			{Filename: "base.cedar", Content: baseCedar},
		},
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/policies/bundle" {
			http.NotFound(w, r)
			return
		}
		if r.Header.Get("X-Tenant-ID") != "tenant-1" {
			t.Errorf("X-Tenant-ID = %q, want %q", r.Header.Get("X-Tenant-ID"), "tenant-1")
		}
		w.Header().Set("ETag", `"abc123"`)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(bundle)
	}))
	defer server.Close()

	f := NewFetcher(server.Client(), server.URL, "tenant-1")
	result, err := f.Fetch()
	if err != nil {
		t.Fatalf("Fetch error: %v", err)
	}
	if !result.Changed {
		t.Error("expected Changed=true on first fetch")
	}
	if result.Bundle.Version != "2026-02-19T00:00:00Z" {
		t.Errorf("Version = %q, want %q", result.Bundle.Version, "2026-02-19T00:00:00Z")
	}
	if len(result.Bundle.Policies) != 1 {
		t.Errorf("Policies count = %d, want 1", len(result.Bundle.Policies))
	}
}

func TestFetcher_Fetch_ETagCaching(t *testing.T) {
	callCount := 0

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		if etag := r.Header.Get("If-None-Match"); etag == `"abc123"` {
			w.WriteHeader(http.StatusNotModified)
			return
		}
		bundle := BundleResponse{Version: "v1", Hash: "abc123"}
		w.Header().Set("ETag", `"abc123"`)
		json.NewEncoder(w).Encode(bundle)
	}))
	defer server.Close()

	f := NewFetcher(server.Client(), server.URL, "tenant-1")

	// First fetch — should get the bundle.
	r1, err := f.Fetch()
	if err != nil {
		t.Fatalf("first Fetch error: %v", err)
	}
	if !r1.Changed {
		t.Error("first fetch should return Changed=true")
	}

	// Second fetch — should get 304.
	r2, err := f.Fetch()
	if err != nil {
		t.Fatalf("second Fetch error: %v", err)
	}
	if r2.Changed {
		t.Error("second fetch should return Changed=false (304)")
	}

	if callCount != 2 {
		t.Errorf("server called %d times, want 2", callCount)
	}
}

func TestFetcher_Fetch_NotFound(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	f := NewFetcher(server.Client(), server.URL, "tenant-1")
	result, err := f.Fetch()
	if err != nil {
		t.Fatalf("Fetch error: %v", err)
	}
	if result.Changed {
		t.Error("expected Changed=false for 404")
	}
}

func TestSyncer_LoadsBundle(t *testing.T) {
	bundle := BundleResponse{
		Version: "v1",
		Hash:    "abc",
		Policies: []PolicyFile{
			{Filename: "base.cedar", Content: baseCedar},
		},
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(bundle)
	}))
	defer server.Close()

	engine := NewEngine()
	fetcher := NewFetcher(server.Client(), server.URL, "tenant-1")
	syncer := NewSyncer(fetcher, engine, 0, func(string, ...any) {})
	defer syncer.Stop()

	// syncOnce directly.
	if err := syncer.syncOnce(); err != nil {
		t.Fatalf("syncOnce error: %v", err)
	}

	if engine.PolicyCount() != 2 { // base.cedar has 2 policies
		t.Errorf("PolicyCount = %d, want 2", engine.PolicyCount())
	}
}
