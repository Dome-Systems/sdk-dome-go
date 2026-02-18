// Package dome provides a Go client SDK for the Dome Platform.
//
// The SDK handles agent registration, heartbeat, policy checks, and lifecycle
// management. Agents integrate with a few lines of code and heartbeat
// automatically.
//
// Quick start using the global client:
//
//	dome.Init(dome.WithCredentials(os.Getenv("DOME_AGENT_TOKEN")))
//	defer dome.Shutdown(context.Background())
//
//	agent, err := dome.Register(ctx, dome.RegisterOptions{
//	    Name: "my-agent",
//	})
//
// For explicit client management:
//
//	client, err := dome.NewClient(
//	    dome.WithCredentials(os.Getenv("DOME_AGENT_TOKEN")),
//	    dome.WithAPIURL("https://api.dome.example.com"),
//	)
//	defer client.Close()
//
//	agent, err := client.Register(ctx, dome.RegisterOptions{
//	    Name: "my-agent",
//	})
package dome

import (
	"context"
	"net/http"
	"sync"
)

var (
	globalMu     sync.Mutex
	globalClient *Client
)

// Init initializes the global Dome client. Call this once at startup.
// Options configure authentication, API URL, and logging.
//
// If no credentials are provided, Init attempts to read from DOME_AGENT_TOKEN,
// DOME_API_KEY, or DOME_TOKEN environment variables (in that order).
func Init(opts ...Option) error {
	globalMu.Lock()
	defer globalMu.Unlock()

	if globalClient != nil {
		_ = globalClient.Close()
		globalClient = nil
	}

	c, err := NewClient(opts...)
	if err != nil {
		return err
	}
	globalClient = c
	return nil
}

// Register registers an agent using the global client.
// Init must be called before Register.
func Register(ctx context.Context, opts RegisterOptions) (*AgentInfo, error) {
	c, err := getGlobalClient()
	if err != nil {
		return nil, err
	}
	return c.Register(ctx, opts)
}

// Check evaluates a policy decision using the global client.
// Currently always returns allowed. Policy enforcement will be added
// in a future version.
func Check(ctx context.Context, req CheckRequest) (*Decision, error) {
	c, err := getGlobalClient()
	if err != nil {
		return nil, err
	}
	return c.Check(ctx, req)
}

// Middleware wraps an http.Handler with Dome governance using the global client.
// Currently logs requests. Policy enforcement will be added in a future version.
func Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c, err := getGlobalClient()
		if err != nil {
			// If no client is initialized, pass through.
			next.ServeHTTP(w, r)
			return
		}
		c.Middleware(next).ServeHTTP(w, r)
	})
}

// Shutdown gracefully stops the global client, stopping the heartbeat goroutine
// and releasing resources. It is safe to call Shutdown multiple times.
func Shutdown(_ context.Context) error {
	globalMu.Lock()
	defer globalMu.Unlock()

	if globalClient != nil {
		err := globalClient.Close()
		globalClient = nil
		return err
	}
	return nil
}

func getGlobalClient() (*Client, error) {
	globalMu.Lock()
	defer globalMu.Unlock()
	if globalClient == nil {
		return nil, errorf("client not initialized (call dome.Init first)")
	}
	return globalClient, nil
}
