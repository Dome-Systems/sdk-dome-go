package dome

import (
	"context"

	"github.com/Dome-Systems/sdk-dome-go/internal/policy"
)

// CheckRequest describes a policy evaluation request.
type CheckRequest struct {
	// Action is the operation being performed (e.g., "mcp:call", "llm:chat").
	Action string
	// Resource is the target of the action (e.g., "hr-mcp/get_salary", "openai/gpt-4").
	Resource string
	// ResourceType optionally specifies the resource category: "mcp", "llm",
	// "credential". If empty, it is inferred from Action.
	ResourceType string
	// Context provides additional key-value pairs for policy evaluation.
	Context map[string]string
}

// Decision is the result of a policy evaluation.
type Decision struct {
	// Allowed indicates whether the action is permitted.
	Allowed bool
	// Reason provides a human-readable explanation of the decision.
	Reason string
	// PolicyVersion is the version of the policy bundle used for evaluation,
	// or empty if no policies are loaded.
	PolicyVersion string
}

// Check evaluates a policy decision against the locally cached Cedar policy
// bundle. If no policies are loaded (bundle not yet fetched, or policy
// disabled), Check returns allowed (fail-open for v0.4.0; fail-closed in v1.0).
func (c *Client) Check(_ context.Context, req CheckRequest) (*Decision, error) {
	if c.config.disablePolicy || !c.policyEngine.HasPolicies() {
		return &Decision{
			Allowed: true,
			Reason:  "no policy bundle loaded",
		}, nil
	}

	c.mu.Lock()
	agentCtx := c.agentCtx
	c.mu.Unlock()

	// Determine required capability — defaults to the action itself.
	requiredCap := req.Action
	if req.Context != nil {
		if cap, ok := req.Context["required_capability"]; ok {
			requiredCap = cap
		}
	}

	input := policy.CheckInput{
		Action:             req.Action,
		Resource:           req.Resource,
		ResourceType:       req.ResourceType,
		RequiredCapability: requiredCap,
		Context:            req.Context,
	}

	d := c.policyEngine.Evaluate(agentCtx, input)
	return &Decision{
		Allowed:       d.Allow,
		Reason:        d.Reason,
		PolicyVersion: d.PolicyVersion,
	}, nil
}
