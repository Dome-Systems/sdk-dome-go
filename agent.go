package dome

import (
	"context"
	"time"

	"connectrpc.com/connect"

	apiv1 "github.com/Dome-Systems/sdk-dome-go/internal/api"
)

const (
	registrationRetryBase = 5 * time.Second
	registrationRetryMax  = 2 * time.Minute
)

// AgentInfo holds the result of a successful registration.
type AgentInfo struct {
	ID           string
	Name         string
	Status       string
	Capabilities []string
	Metadata     map[string]string
	Token        string
}

// RegisterOptions configures agent registration.
type RegisterOptions struct {
	Name         string
	Description  string
	ParentID     string
	Capabilities []string
	Metadata     map[string]string
}

// Register registers an agent with the Dome control plane.
//
// If an agent with the same name already exists (CodeAlreadyExists), Register
// finds the existing agent and returns its info. This makes Register idempotent
// and safe to call on every startup.
//
// On success, Register starts a background heartbeat goroutine (unless
// WithoutHeartbeat was used).
//
// If WithGracefulDegradation was set and registration fails (e.g. API
// unreachable), Register logs a warning and retries in the background instead
// of returning an error. AgentID returns empty until background registration
// succeeds.
func (c *Client) Register(ctx context.Context, opts RegisterOptions) (*AgentInfo, error) {
	if opts.Name == "" {
		return nil, errorf("agent name is required")
	}

	info, err := c.doRegister(ctx, opts)
	if err != nil {
		if c.config.gracefulDegradation {
			c.logger.Warn("registration failed, retrying in background",
				"agent_name", opts.Name,
				"error", err,
			)
			c.startBackgroundRegistration(opts)
			return &AgentInfo{Name: opts.Name}, nil
		}
		return nil, err
	}

	c.setAgentID(info.ID)

	if !c.config.disableHeartbeat {
		c.startHeartbeat(info.ID)
	}

	return info, nil
}

// doRegister performs the actual registration RPC call with idempotency handling.
func (c *Client) doRegister(ctx context.Context, opts RegisterOptions) (*AgentInfo, error) {
	req := &apiv1.RegisterAgentRequest{
		Name:         opts.Name,
		Capabilities: opts.Capabilities,
		Metadata:     opts.Metadata,
	}

	if opts.ParentID != "" {
		req.ParentId = &opts.ParentID
	}

	resp, err := c.rpc.RegisterAgent(ctx, connect.NewRequest(req))
	if err != nil {
		// Idempotent: if the agent already exists, find it by name.
		if connect.CodeOf(err) == connect.CodeAlreadyExists {
			return c.findExistingAgent(ctx, opts.Name)
		}
		return nil, errorf("register agent: %w", err)
	}

	return agentFromProto(resp.Msg.GetAgent(), resp.Msg.GetToken()), nil
}

// startBackgroundRegistration spawns a goroutine that retries registration
// with exponential backoff. On success, it sets the agent ID and transitions
// to the heartbeat loop within the same goroutine (avoiding a deadlock with
// startHeartbeat which would try to cancel/wait on itself).
func (c *Client) startBackgroundRegistration(opts RegisterOptions) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.cancel != nil {
		c.cancel()
		<-c.stopped
	}

	ctx, cancel := context.WithCancel(context.Background())
	c.cancel = cancel
	c.stopped = make(chan struct{})

	go func() {
		defer close(c.stopped)

		err := retryWithBackoff(ctx, func(retryCtx context.Context) error {
			info, regErr := c.doRegister(retryCtx, opts)
			if regErr != nil {
				c.logger.Debug("background registration retry failed",
					"agent_name", opts.Name,
					"error", regErr,
				)
				return regErr
			}

			c.logger.Info("background registration succeeded",
				"agent_id", info.ID,
				"agent_name", opts.Name,
			)
			c.setAgentID(info.ID)
			return nil
		}, registrationRetryBase, registrationRetryMax)

		if err != nil {
			c.logger.Debug("background registration canceled", "error", err)
			return
		}

		// Registration succeeded — run heartbeat in this goroutine.
		if !c.config.disableHeartbeat {
			c.runHeartbeat(ctx, c.AgentID())
		}
	}()
}

// findExistingAgent looks up an agent by name via ListAgents.
func (c *Client) findExistingAgent(ctx context.Context, name string) (*AgentInfo, error) {
	resp, err := c.rpc.ListAgents(ctx, connect.NewRequest(&apiv1.ListAgentsRequest{
		Limit: 100,
	}))
	if err != nil {
		return nil, errorf("list agents for idempotent registration: %w", err)
	}

	for _, a := range resp.Msg.GetAgents() {
		if a.GetName() == name {
			return agentFromProto(a, ""), nil
		}
	}

	return nil, errorf("agent %q already exists but could not be found", name)
}

// agentFromProto converts a protobuf Agent to an AgentInfo.
func agentFromProto(a *apiv1.Agent, token string) *AgentInfo {
	if a == nil {
		return &AgentInfo{}
	}
	return &AgentInfo{
		ID:           a.GetId(),
		Name:         a.GetName(),
		Status:       a.GetStatus().String(),
		Capabilities: a.GetCapabilities(),
		Metadata:     a.GetMetadata(),
		Token:        token,
	}
}
