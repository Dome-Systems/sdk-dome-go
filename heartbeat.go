package dome

import (
	"context"
	"time"

	"connectrpc.com/connect"

	apiv1 "github.com/Dome-Systems/sdk-dome-go/internal/api"
)

// startHeartbeat launches a background goroutine that sends periodic heartbeats.
func (c *Client) startHeartbeat(agentID string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	// If already running, stop the old one.
	if c.cancel != nil {
		c.cancel()
		<-c.stopped
	}

	ctx, cancel := context.WithCancel(context.Background())
	c.cancel = cancel
	c.stopped = make(chan struct{})

	go c.heartbeatLoop(ctx, agentID)
}

// heartbeatLoop sends heartbeats at the configured interval until the context is canceled.
func (c *Client) heartbeatLoop(ctx context.Context, agentID string) {
	defer close(c.stopped)

	// Send initial heartbeat immediately.
	c.sendHeartbeat(ctx, agentID)

	ticker := time.NewTicker(c.config.heartbeatInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			c.sendHeartbeat(ctx, agentID)
		}
	}
}

// sendHeartbeat sends a single heartbeat RPC. Failures are logged but do not crash.
func (c *Client) sendHeartbeat(ctx context.Context, agentID string) {
	_, err := c.rpc.Heartbeat(ctx, connect.NewRequest(&apiv1.HeartbeatRequest{
		AgentId: agentID,
	}))
	if err != nil {
		c.logger.Warn("heartbeat failed", "agent_id", agentID, "error", err)
	}
}
