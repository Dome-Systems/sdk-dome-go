package dome

import (
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"sync"

	"github.com/Dome-Systems/sdk-dome-go/internal/api/agentv1connect"
	"github.com/Dome-Systems/sdk-dome-go/internal/vault"
)

// Client is the Dome SDK client. It handles agent registration and
// background heartbeat. Use NewClient to create one, or use the global
// Init/Register/Shutdown functions.
type Client struct {
	rpc    agentv1connect.AgentRegistryClient
	config clientConfig
	logger *slog.Logger

	mu      sync.Mutex
	agentID string
	cancel  func()
	stopped chan struct{}
}

// NewClient creates a new Dome SDK client.
//
// Authentication is resolved in priority order:
//  1. Explicit WithCredentials option (base64 credential blob)
//  2. Explicit WithAPIKey option
//  3. DOME_AGENT_TOKEN environment variable (credential blob)
//  4. DOME_API_KEY environment variable
//  5. DOME_TOKEN environment variable (backwards compatibility)
func NewClient(opts ...Option) (*Client, error) {
	cfg := defaultConfig()
	for _, o := range opts {
		o(&cfg)
	}

	var transport http.RoundTripper

	// Try to decode a credential blob (from WithCredentials or env).
	credToken := cfg.credentials
	if credToken == "" {
		credToken = os.Getenv("DOME_AGENT_TOKEN")
	}

	if credToken != "" {
		creds, err := decodeToken(credToken)
		if err != nil {
			return nil, errorf("decode credentials: %w", err)
		}
		if creds != nil && creds.VaultAddr != "" && creds.OIDCRoleName != "" {
			// Vault-based auth: decode credential blob into vault transport config.
			vaultCfg := vault.AuthConfig{
				VaultAddr: creds.VaultAddr,
				OIDCRole:  creds.OIDCRoleName,
			}
			switch creds.AuthMethod {
			case "approle":
				vaultCfg.AuthMethod = "approle"
				vaultCfg.AppRoleID = creds.RoleID
				vaultCfg.AppSecretID = creds.SecretID
			case "kubernetes":
				vaultCfg.AuthMethod = "kubernetes"
				vaultCfg.Role = creds.KubeAuthRole
			default:
				vaultCfg.AuthMethod = "approle"
				vaultCfg.AppRoleID = creds.RoleID
				vaultCfg.AppSecretID = creds.SecretID
			}
			transport = vault.NewTransport(http.DefaultTransport, vaultCfg)
		} else if creds != nil {
			// Credential blob present but no Vault OIDC — use raw token as bearer.
			transport = &bearerTransport{base: http.DefaultTransport, token: credToken}
		}
	}

	// Fall back to static API key if no credential blob resolved a transport.
	if transport == nil {
		apiKey := cfg.apiKey
		if apiKey == "" {
			apiKey = os.Getenv("DOME_API_KEY")
		}
		if apiKey == "" {
			apiKey = os.Getenv("DOME_TOKEN")
		}
		if apiKey == "" {
			return nil, errors.New("dome: authentication required (use WithCredentials, WithAPIKey, DOME_AGENT_TOKEN, DOME_API_KEY, or DOME_TOKEN)")
		}
		transport = &bearerTransport{base: http.DefaultTransport, token: apiKey}
	}

	httpClient := &http.Client{Transport: transport}
	rpc := agentv1connect.NewAgentRegistryClient(httpClient, cfg.apiURL)

	return &Client{
		rpc:    rpc,
		config: cfg,
		logger: cfg.logger,
	}, nil
}

// Close stops the background heartbeat goroutine and releases resources.
// It is safe to call Close multiple times.
func (c *Client) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.cancel != nil {
		c.cancel()
		<-c.stopped
		c.cancel = nil
	}
	return nil
}

// AgentID returns the registered agent's ID, or empty if not yet registered.
func (c *Client) AgentID() string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.agentID
}

// setAgentID stores the agent ID and is called after successful registration.
func (c *Client) setAgentID(id string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.agentID = id
}

// bearerTransport injects a static Bearer token into every outgoing request.
type bearerTransport struct {
	base  http.RoundTripper
	token string
}

func (t *bearerTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	clone := req.Clone(req.Context())
	clone.Header.Set("Authorization", "Bearer "+t.token)
	return t.base.RoundTrip(clone)
}

// errorf returns a formatted error prefixed with "dome:".
func errorf(format string, args ...any) error {
	return fmt.Errorf("dome: "+format, args...)
}
