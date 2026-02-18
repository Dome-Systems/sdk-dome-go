package dome

import (
	"log/slog"
	"os"
	"time"
)

const (
	// DefaultAPIURL is the default Dome API server address for local development.
	DefaultAPIURL = "http://localhost:8080"

	// DefaultHeartbeatInterval is the SDK-side heartbeat interval.
	// This is half the server deadline (60s) to give one retry before
	// the agent appears stale.
	DefaultHeartbeatInterval = 30 * time.Second
)

// clientConfig holds resolved configuration for the SDK client.
type clientConfig struct {
	apiURL            string
	apiKey            string
	credentials       string
	heartbeatInterval time.Duration
	disableHeartbeat  bool
	logger            *slog.Logger
}

// Option configures the SDK client.
type Option func(*clientConfig)

// WithCredentials sets the opaque credential token (base64-encoded blob from
// `dome agents register`). The SDK decodes this to extract Vault auth config
// automatically.
func WithCredentials(token string) Option {
	return func(c *clientConfig) {
		c.credentials = token
	}
}

// WithCredentialsFile reads the credential token from a file.
func WithCredentialsFile(path string) Option {
	return func(c *clientConfig) {
		data, err := os.ReadFile(path)
		if err == nil {
			c.credentials = string(data)
		}
	}
}

// WithAPIKey sets a static API key for authentication. This is a simpler
// alternative to WithCredentials for development or testing.
func WithAPIKey(key string) Option {
	return func(c *clientConfig) {
		c.apiKey = key
	}
}

// WithAPIURL sets the Dome API server URL.
func WithAPIURL(url string) Option {
	return func(c *clientConfig) {
		c.apiURL = url
	}
}

// WithHeartbeatInterval sets the heartbeat interval. Must be positive.
func WithHeartbeatInterval(d time.Duration) Option {
	return func(c *clientConfig) {
		if d > 0 {
			c.heartbeatInterval = d
		}
	}
}

// WithoutHeartbeat disables the background heartbeat goroutine.
func WithoutHeartbeat() Option {
	return func(c *clientConfig) {
		c.disableHeartbeat = true
	}
}

// WithLogger sets a custom slog logger. By default, the SDK uses slog.Default().
func WithLogger(l *slog.Logger) Option {
	return func(c *clientConfig) {
		c.logger = l
	}
}

func defaultConfig() clientConfig {
	return clientConfig{
		apiURL:            DefaultAPIURL,
		heartbeatInterval: DefaultHeartbeatInterval,
		logger:            slog.Default(),
	}
}
