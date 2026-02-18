package dome

import (
	"context"
	"time"

	"connectrpc.com/connect"

	apiv1 "github.com/Dome-Systems/sdk-dome-go/internal/api"
)

const maxHeartbeatInterval = 5 * time.Minute

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

	go func() {
		defer close(c.stopped)
		c.runHeartbeat(ctx, agentID)
	}()
}

// runHeartbeat sends heartbeats at the configured interval until the context is
// canceled. On consecutive failures, the interval backs off exponentially up to
// maxHeartbeatInterval. On success, the interval resets to the configured value.
//
// This is the pure logic — it does not manage c.cancel/c.stopped. Callers are
// responsible for goroutine lifecycle.
func (c *Client) runHeartbeat(ctx context.Context, agentID string) {
	// Send initial heartbeat immediately.
	c.sendHeartbeat(ctx, agentID)

	baseInterval := c.config.heartbeatInterval
	currentInterval := baseInterval
	consecutiveFailures := 0

	timer := time.NewTimer(currentInterval)
	defer timer.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-timer.C:
			if c.sendHeartbeat(ctx, agentID) {
				consecutiveFailures = 0
				currentInterval = baseInterval
			} else {
				consecutiveFailures++
				currentInterval = backoff(baseInterval, maxHeartbeatInterval, consecutiveFailures)
			}
			timer.Reset(currentInterval)
		}
	}
}

// sendHeartbeat sends a single heartbeat RPC. Returns true on success.
func (c *Client) sendHeartbeat(ctx context.Context, agentID string) bool {
	_, err := c.rpc.Heartbeat(ctx, connect.NewRequest(&apiv1.HeartbeatRequest{
		AgentId: agentID,
	}))
	if err != nil {
		c.logger.Warn("heartbeat failed", "agent_id", agentID, "error", err)
		return false
	}
	return true
}
