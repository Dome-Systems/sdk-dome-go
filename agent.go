package dome

import (
	"context"

	"connectrpc.com/connect"

	apiv1 "github.com/Dome-Systems/sdk-dome-go/internal/api"
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
func (c *Client) Register(ctx context.Context, opts RegisterOptions) (*AgentInfo, error) {
	if opts.Name == "" {
		return nil, errorf("agent name is required")
	}

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

	info := agentFromProto(resp.Msg.GetAgent(), resp.Msg.GetToken())

	c.setAgentID(info.ID)

	if !c.config.disableHeartbeat {
		c.startHeartbeat(info.ID)
	}

	return info, nil
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
			info := agentFromProto(a, "")
			c.setAgentID(info.ID)

			if !c.config.disableHeartbeat {
				c.startHeartbeat(info.ID)
			}

			return info, nil
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
