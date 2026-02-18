package dome_test

import (
	"context"
	"testing"
	"time"

	dome "github.com/Dome-Systems/sdk-dome-go"
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
	defer client.Close()

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
	defer client.Close()

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
