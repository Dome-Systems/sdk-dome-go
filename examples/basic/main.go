// Command basic demonstrates the simplest way to use the Dome Go SDK.
//
// Usage:
//
//	DOME_AGENT_TOKEN=$(dome agents register my-agent --json | jq -r .token) \
//	DOME_API_URL=http://localhost:8080 \
//	  go run .
package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"

	dome "github.com/Dome-Systems/sdk-dome-go"
)

func main() {
	// Initialize the global Dome client.
	opts := []dome.Option{
		dome.WithCredentials(os.Getenv("DOME_AGENT_TOKEN")),
	}
	if apiURL := os.Getenv("DOME_API_URL"); apiURL != "" {
		opts = append(opts, dome.WithAPIURL(apiURL))
	}

	if err := dome.Init(opts...); err != nil {
		log.Fatalf("dome.Init: %v", err)
	}
	defer func() { _ = dome.Shutdown(context.Background()) }()

	// Register the agent. Safe to call on every startup (idempotent).
	agent, err := dome.Register(context.Background(), dome.RegisterOptions{
		Name:         "basic-example",
		Description:  "Dome SDK basic example agent",
		Capabilities: []string{"example"},
		Metadata: map[string]string{
			"sdk": "go",
		},
	})
	if err != nil {
		log.Fatalf("dome.Register: %v", err)
	}

	log.Printf("Agent registered: %s (ID: %s, Status: %s)", agent.Name, agent.ID, agent.Status)
	log.Println("Heartbeat is running in the background. Press Ctrl+C to exit.")

	// Wait for interrupt signal.
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)
	<-sig

	log.Println("Shutting down...")
}
