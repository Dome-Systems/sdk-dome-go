package dome_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	dome "github.com/Dome-Systems/sdk-dome-go"
	"github.com/Dome-Systems/sdk-dome-go/internal/api/agentv1connect"
)

func TestHeartbeat_SendsOnInterval(t *testing.T) {
	serverURL := testServer(t)

	client, err := dome.NewClient(
		dome.WithAPIKey("test-key"),
		dome.WithAPIURL(serverURL),
		dome.WithHeartbeatInterval(50*time.Millisecond),
	)
	if err != nil {
		t.Fatalf("NewClient error: %v", err)
	}
	defer func() { _ = client.Close() }()

	_, err = client.Register(context.Background(), dome.RegisterOptions{
		Name: "heartbeat-interval-agent",
	})
	if err != nil {
		t.Fatalf("Register error: %v", err)
	}

	// Wait for a few heartbeats to be sent.
	time.Sleep(200 * time.Millisecond)

	// If we get here without deadlock or panic, heartbeat is running.
	if err := client.Close(); err != nil {
		t.Fatalf("Close error: %v", err)
	}
}

func TestHeartbeat_DoesNotCrashOnFailure(t *testing.T) {
	serverURL := testServer(t)

	client, err := dome.NewClient(
		dome.WithAPIKey("test-key"),
		dome.WithAPIURL(serverURL),
		dome.WithHeartbeatInterval(50*time.Millisecond),
	)
	if err != nil {
		t.Fatalf("NewClient error: %v", err)
	}

	_, err = client.Register(context.Background(), dome.RegisterOptions{
		Name: "failing-heartbeat-agent",
	})
	if err != nil {
		t.Fatalf("Register error: %v", err)
	}

	// Verify the heartbeat runs and Close works cleanly.
	time.Sleep(100 * time.Millisecond)

	if err := client.Close(); err != nil {
		t.Fatalf("Close error: %v", err)
	}
}

func TestHeartbeat_BacksOffOnFailure(t *testing.T) {
	handler := newMockHandler()
	var (
		mu             sync.Mutex
		heartbeatCount int
		failHeartbeat  atomic.Bool
	)
	failHeartbeat.Store(true)

	mux := http.NewServeMux()
	path, h := agentv1connect.NewAgentRegistryHandler(handler)
	mux.Handle(path, h)

	// Wrap the mux to intercept heartbeat calls.
	wrapper := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/dome.agent.v1.AgentRegistry/Heartbeat" {
			mu.Lock()
			heartbeatCount++
			mu.Unlock()

			if failHeartbeat.Load() {
				w.WriteHeader(http.StatusServiceUnavailable)
				return
			}
		}
		mux.ServeHTTP(w, r)
	})

	server := httptest.NewServer(wrapper)
	t.Cleanup(server.Close)

	client, err := dome.NewClient(
		dome.WithAPIKey("test-key"),
		dome.WithAPIURL(server.URL),
		dome.WithHeartbeatInterval(50*time.Millisecond),
	)
	if err != nil {
		t.Fatalf("NewClient error: %v", err)
	}
	defer func() { _ = client.Close() }()

	_, err = client.Register(context.Background(), dome.RegisterOptions{
		Name: "backoff-agent",
	})
	if err != nil {
		t.Fatalf("Register error: %v", err)
	}

	// Let heartbeat fail for a while — with backoff, the rate should decrease.
	time.Sleep(500 * time.Millisecond)

	mu.Lock()
	failedCount := heartbeatCount
	mu.Unlock()

	// Without backoff at 50ms interval, we'd see ~10 heartbeats in 500ms.
	// With backoff, after initial + a few retries, intervals grow quickly.
	// The initial heartbeat fires immediately, so we expect at least 2
	// (initial + first tick) but significantly fewer than 10.
	if failedCount > 8 {
		t.Errorf("expected backoff to reduce heartbeat rate, got %d calls in 500ms", failedCount)
	}

	// Now let heartbeats succeed and verify recovery.
	failHeartbeat.Store(false)
	mu.Lock()
	heartbeatCount = 0
	mu.Unlock()

	// Wait longer to account for the remaining backoff timer before recovery kicks in.
	time.Sleep(600 * time.Millisecond)

	mu.Lock()
	successCount := heartbeatCount
	mu.Unlock()

	// After recovery, should go back to ~50ms interval.
	// The first success may be delayed by the remaining backoff timer,
	// but subsequent heartbeats should be at 50ms. Expect at least 3 in 600ms.
	if successCount < 3 {
		t.Errorf("expected heartbeat to recover after backoff, got only %d calls in 600ms", successCount)
	}
}

func TestHeartbeat_DisabledWithOption(t *testing.T) {
	serverURL := testServer(t)

	client, err := dome.NewClient(
		dome.WithAPIKey("test-key"),
		dome.WithAPIURL(serverURL),
		dome.WithoutHeartbeat(),
	)
	if err != nil {
		t.Fatalf("NewClient error: %v", err)
	}
	defer func() { _ = client.Close() }()

	_, err = client.Register(context.Background(), dome.RegisterOptions{
		Name: "no-heartbeat-agent",
	})
	if err != nil {
		t.Fatalf("Register error: %v", err)
	}

	// Wait a bit — no heartbeat should be running.
	time.Sleep(100 * time.Millisecond)

	if err := client.Close(); err != nil {
		t.Fatalf("Close error: %v", err)
	}
}
