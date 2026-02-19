package dome

import (
	"context"
	"time"

	"connectrpc.com/connect"
	"google.golang.org/protobuf/types/known/timestamppb"

	apiv1 "github.com/Dome-Systems/sdk-dome-go/internal/api"
)

// reportEvent sends an SDK lifecycle event to the control plane.
// Errors are logged but never returned — event emission is fire-and-forget.
func (c *Client) reportEvent(ctx context.Context, eventType string) {
	agentID := c.AgentID()
	c.reportEventForAgent(ctx, agentID, eventType)
}

// reportEventForAgent sends an event for a specific agent ID.
// Use this when the caller already holds c.mu (e.g., from Close).
func (c *Client) reportEventForAgent(ctx context.Context, agentID, eventType string) {
	if agentID == "" {
		return
	}

	req := &apiv1.ReportEventRequest{
		AgentId:   agentID,
		EventType: eventType,
		Timestamp: timestamppb.New(time.Now()),
	}

	_, err := c.rpc.ReportEvent(ctx, connect.NewRequest(req))
	if err != nil {
		c.logger.Debug("failed to report event", "event_type", eventType, "error", err)
	}
}
