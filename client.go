package dome

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"sync"

	"github.com/Dome-Systems/sdk-dome-go/internal/api/agentv1connect"
	"github.com/Dome-Systems/sdk-dome-go/internal/policy"
	"github.com/Dome-Systems/sdk-dome-go/internal/tokenexchange"
	"github.com/Dome-Systems/sdk-dome-go/internal/vault"
)

// Client is the Dome SDK client. It handles agent registration, heartbeat,
// and Cedar policy evaluation. Use NewClient to create one, or use the
// global Init/Start/Shutdown functions.
type Client struct {
	rpc        agentv1connect.AgentRegistryClient
	httpClient *http.Client // used for policy bundle fetch
	config     clientConfig
	logger     *slog.Logger

	mu       sync.Mutex
	agentID  string
	tenantID string
	cancel   func()
	stopped  chan struct{}

	// Policy evaluation.
	policyEngine *policy.Engine
	policySyncer *policy.Syncer
	agentCtx     policy.AgentContext // cached agent context for Cedar evaluation

	// Auth events queued before Start() sets the agent ID.
	pendingAuthEvents []string
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

	// Create the client early so the auth callback can queue events.
	c := &Client{
		config:       cfg,
		logger:       cfg.logger,
		policyEngine: policy.NewEngine(),
	}

	// Auth event callback — queues events until Start() sets the agent ID.
	authCallback := func(eventType string) {
		c.mu.Lock()
		defer c.mu.Unlock()
		if c.agentID != "" {
			// Agent ID is set — emit directly.
			go c.reportEventForAgent(context.Background(), c.agentID, eventType)
		} else {
			// Queue for emission after Start().
			c.pendingAuthEvents = append(c.pendingAuthEvents, eventType)
		}
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
		if creds != nil && creds.APIURL != "" && creds.AuthMethod == "approle" {
			// Token exchange via Dome API: agent never talks to Vault.
			transport = tokenexchange.NewTransport(http.DefaultTransport, tokenexchange.Config{
				APIURL:   creds.APIURL,
				RoleID:   creds.RoleID,
				SecretID: creds.SecretID,
			}, authCallback)
		} else if creds != nil && creds.VaultAddr != "" && creds.OIDCRoleName != "" {
			// Legacy Vault-based auth: direct Vault connectivity required.
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
			transport = vault.NewTransport(http.DefaultTransport, vaultCfg, authCallback)
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
	c.rpc = agentv1connect.NewAgentRegistryClient(httpClient, cfg.apiURL)
	c.httpClient = httpClient

	return c, nil
}

// Close stops the background heartbeat goroutine, policy syncer, and releases
// resources. It is safe to call Close multiple times.
func (c *Client) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	// Stop policy syncer.
	if c.policySyncer != nil {
		c.policySyncer.Stop()
		c.policySyncer = nil
	}

	if c.cancel != nil {
		// Emit agent.stopped before canceling the heartbeat context.
		// Use c.agentID directly since we already hold c.mu.
		if c.agentID != "" {
			c.reportEventForAgent(context.Background(), c.agentID, "agent.stopped")
		}
		c.cancel()
		<-c.stopped
		c.cancel = nil
	}
	return nil
}

// startPolicySyncer begins periodic policy bundle sync from the control plane.
func (c *Client) startPolicySyncer() {
	c.mu.Lock()
	defer c.mu.Unlock()

	// Use the same tenant ID from the agent's context (extracted from auth).
	// The fetcher sends X-Tenant-ID header — the server extracts it from the
	// auth token, so we can use a placeholder. The HTTP client already carries
	// the auth transport.
	fetcher := policy.NewFetcher(c.httpClient, c.config.apiURL, c.tenantID)
	c.policySyncer = policy.NewSyncer(fetcher, c.policyEngine, c.config.policyRefresh, func(msg string, args ...any) {
		c.logger.Debug(msg, args...)
	})
	c.policySyncer.Start()
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
