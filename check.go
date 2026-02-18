package dome

import (
	"context"
)

// CheckRequest describes a policy evaluation request.
type CheckRequest struct {
	// Action is the operation being performed (e.g., "read", "write", "execute").
	Action string
	// Resource is the target of the action (e.g., "users", "orders/123").
	Resource string
	// Context provides additional key-value pairs for policy evaluation.
	Context map[string]string
}

// Decision is the result of a policy evaluation.
type Decision struct {
	// Allowed indicates whether the action is permitted.
	Allowed bool
	// Reason provides a human-readable explanation of the decision.
	Reason string
}

// Check evaluates a policy decision. Currently always returns allowed.
// Policy enforcement will be added in a future version.
func (c *Client) Check(_ context.Context, _ CheckRequest) (*Decision, error) {
	return &Decision{Allowed: true, Reason: "policy enforcement not yet implemented"}, nil
}
