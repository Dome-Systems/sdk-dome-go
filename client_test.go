package dome_test

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"connectrpc.com/connect"

	dome "github.com/Dome-Systems/sdk-dome-go"
	apiv1 "github.com/Dome-Systems/sdk-dome-go/internal/api"
	"github.com/Dome-Systems/sdk-dome-go/internal/api/agentv1connect"
)

// mockHandler implements the AgentRegistryHandler for testing.
type mockHandler struct {
	agentv1connect.UnimplementedAgentRegistryHandler
	agents map[string]*apiv1.Agent
	nextID int
}

func newMockHandler() *mockHandler {
	return &mockHandler{agents: make(map[string]*apiv1.Agent)}
}

func (h *mockHandler) RegisterAgent(_ context.Context, req *connect.Request[apiv1.RegisterAgentRequest]) (*connect.Response[apiv1.RegisterAgentResponse], error) {
	msg := req.Msg

	// Check for duplicate name.
	for _, a := range h.agents {
		if a.GetName() == msg.GetName() {
			return nil, connect.NewError(connect.CodeAlreadyExists, nil)
		}
	}

	h.nextID++
	agent := &apiv1.Agent{
		Id:           fmt.Sprintf("agent-%d", h.nextID),
		Name:         msg.GetName(),
		Status:       apiv1.AgentStatus_AGENT_STATUS_ACTIVE,
		Capabilities: msg.GetCapabilities(),
		Metadata:     msg.GetMetadata(),
	}
	h.agents[agent.Id] = agent

	return connect.NewResponse(&apiv1.RegisterAgentResponse{
		Agent: agent,
		Token: "test-token",
	}), nil
}

func (h *mockHandler) ListAgents(_ context.Context, _ *connect.Request[apiv1.ListAgentsRequest]) (*connect.Response[apiv1.ListAgentsResponse], error) {
	var agents []*apiv1.Agent
	for _, a := range h.agents {
		agents = append(agents, a)
	}
	return connect.NewResponse(&apiv1.ListAgentsResponse{
		Agents: agents,
		Total:  int32(len(agents)),
	}), nil
}

func (h *mockHandler) Heartbeat(_ context.Context, _ *connect.Request[apiv1.HeartbeatRequest]) (*connect.Response[apiv1.HeartbeatResponse], error) {
	return connect.NewResponse(&apiv1.HeartbeatResponse{}), nil
}

// testServer creates a test HTTP server backed by a mock handler.
func testServer(t *testing.T) string {
	t.Helper()

	handler := newMockHandler()
	mux := http.NewServeMux()
	path, h := agentv1connect.NewAgentRegistryHandler(handler)
	mux.Handle(path, h)

	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)

	return server.URL
}

func TestNewClient_RequiresAuth(t *testing.T) {
	t.Setenv("DOME_AGENT_TOKEN", "")
	t.Setenv("DOME_API_KEY", "")
	t.Setenv("DOME_TOKEN", "")

	_, err := dome.NewClient()
	if err == nil {
		t.Fatal("expected error when no authentication is provided")
	}
}

func TestNewClient_AcceptsExplicitKey(t *testing.T) {
	t.Setenv("DOME_AGENT_TOKEN", "")
	t.Setenv("DOME_API_KEY", "")
	t.Setenv("DOME_TOKEN", "")

	client, err := dome.NewClient(dome.WithAPIKey("test-key"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer func() { _ = client.Close() }()
}

func TestNewClient_AcceptsEnvKey(t *testing.T) {
	t.Setenv("DOME_AGENT_TOKEN", "")
	t.Setenv("DOME_API_KEY", "env-key")
	t.Setenv("DOME_TOKEN", "")

	client, err := dome.NewClient()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer func() { _ = client.Close() }()
}

func TestNewClient_FallsBackToDomeToken(t *testing.T) {
	t.Setenv("DOME_AGENT_TOKEN", "")
	t.Setenv("DOME_API_KEY", "")
	t.Setenv("DOME_TOKEN", "fallback-token")

	client, err := dome.NewClient()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer func() { _ = client.Close() }()
}

func TestRegister_Success(t *testing.T) {
	serverURL := testServer(t)

	client, err := dome.NewClient(
		dome.WithAPIKey("test-key"),
		dome.WithAPIURL(serverURL),
		dome.WithoutHeartbeat(),
	)
	if err != nil {
		t.Fatalf("NewClient error: %v", err)
	}
	defer func() { _ = client.Close() }()

	info, err := client.Register(context.Background(), dome.RegisterOptions{
		Name: "test-agent",
	})
	if err != nil {
		t.Fatalf("Register error: %v", err)
	}

	if info.ID == "" {
		t.Error("expected agent ID to be set")
	}
	if info.Name != "test-agent" {
		t.Errorf("Name = %q, want %q", info.Name, "test-agent")
	}
	if client.AgentID() == "" {
		t.Error("expected client.AgentID() to be set after registration")
	}
}

func TestRegister_Idempotent(t *testing.T) {
	serverURL := testServer(t)

	client, err := dome.NewClient(
		dome.WithAPIKey("test-key"),
		dome.WithAPIURL(serverURL),
		dome.WithoutHeartbeat(),
	)
	if err != nil {
		t.Fatalf("NewClient error: %v", err)
	}
	defer func() { _ = client.Close() }()

	info1, err := client.Register(context.Background(), dome.RegisterOptions{
		Name: "idempotent-agent",
	})
	if err != nil {
		t.Fatalf("first Register error: %v", err)
	}

	info2, err := client.Register(context.Background(), dome.RegisterOptions{
		Name: "idempotent-agent",
	})
	if err != nil {
		t.Fatalf("second Register error: %v", err)
	}

	if info1.ID != info2.ID {
		t.Errorf("idempotent registration returned different IDs: %q vs %q", info1.ID, info2.ID)
	}
}

func TestRegister_MissingName(t *testing.T) {
	serverURL := testServer(t)

	client, err := dome.NewClient(
		dome.WithAPIKey("test-key"),
		dome.WithAPIURL(serverURL),
		dome.WithoutHeartbeat(),
	)
	if err != nil {
		t.Fatalf("NewClient error: %v", err)
	}
	defer func() { _ = client.Close() }()

	_, err = client.Register(context.Background(), dome.RegisterOptions{})
	if err == nil {
		t.Fatal("expected error for missing name")
	}
}

func TestClose_StopsHeartbeat(t *testing.T) {
	serverURL := testServer(t)

	client, err := dome.NewClient(
		dome.WithAPIKey("test-key"),
		dome.WithAPIURL(serverURL),
	)
	if err != nil {
		t.Fatalf("NewClient error: %v", err)
	}

	_, err = client.Register(context.Background(), dome.RegisterOptions{
		Name: "heartbeat-agent",
	})
	if err != nil {
		t.Fatalf("Register error: %v", err)
	}

	if err := client.Close(); err != nil {
		t.Fatalf("Close error: %v", err)
	}

	// Calling Close again should be safe.
	if err := client.Close(); err != nil {
		t.Fatalf("second Close error: %v", err)
	}
}

func TestCheck_ReturnsAllowed(t *testing.T) {
	client, err := dome.NewClient(
		dome.WithAPIKey("test-key"),
		dome.WithoutHeartbeat(),
	)
	if err != nil {
		t.Fatalf("NewClient error: %v", err)
	}
	defer func() { _ = client.Close() }()

	decision, err := client.Check(context.Background(), dome.CheckRequest{
		Action:   "read",
		Resource: "users",
	})
	if err != nil {
		t.Fatalf("Check error: %v", err)
	}
	if !decision.Allowed {
		t.Error("expected Check to return allowed")
	}
}

func TestRegister_GracefulDegradation_UnreachableAPI(t *testing.T) {
	// Point at a server that will refuse connections.
	client, err := dome.NewClient(
		dome.WithAPIKey("test-key"),
		dome.WithAPIURL("http://127.0.0.1:1"), // nothing listens here
		dome.WithGracefulDegradation(),
		dome.WithoutHeartbeat(),
	)
	if err != nil {
		t.Fatalf("NewClient error: %v", err)
	}
	defer func() { _ = client.Close() }()

	// Register should NOT return an error — it degrades gracefully.
	info, err := client.Register(context.Background(), dome.RegisterOptions{
		Name: "graceful-agent",
	})
	if err != nil {
		t.Fatalf("Register should not error with graceful degradation, got: %v", err)
	}

	// Agent ID should be empty (registration hasn't succeeded yet).
	if client.AgentID() != "" {
		t.Errorf("AgentID() = %q, want empty before background registration succeeds", client.AgentID())
	}

	// Info should have the name but no ID.
	if info.Name != "graceful-agent" {
		t.Errorf("info.Name = %q, want %q", info.Name, "graceful-agent")
	}
	if info.ID != "" {
		t.Errorf("info.ID = %q, want empty", info.ID)
	}
}

func TestRegister_GracefulDegradation_EventualSuccess(t *testing.T) {
	// Use an HTTP handler that initially rejects registration, then allows it.
	handler := newMockHandler()
	var failRegistration atomic.Bool
	failRegistration.Store(true)

	mux := http.NewServeMux()
	path, h := agentv1connect.NewAgentRegistryHandler(handler)
	mux.Handle(path, h)

	// Wrap to intercept RegisterAgent and fail initially.
	wrapper := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/dome.agent.v1.AgentRegistry/RegisterAgent" && failRegistration.Load() {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		mux.ServeHTTP(w, r)
	})

	server := httptest.NewServer(wrapper)
	t.Cleanup(server.Close)

	client, err := dome.NewClient(
		dome.WithAPIKey("test-key"),
		dome.WithAPIURL(server.URL),
		dome.WithGracefulDegradation(),
	)
	if err != nil {
		t.Fatalf("NewClient error: %v", err)
	}
	defer func() { _ = client.Close() }()

	info, err := client.Register(context.Background(), dome.RegisterOptions{
		Name: "eventual-agent",
	})
	if err != nil {
		t.Fatalf("Register should not error with graceful degradation, got: %v", err)
	}
	if info.ID != "" {
		t.Errorf("info.ID = %q, want empty initially", info.ID)
	}

	// Allow registration to succeed now.
	failRegistration.Store(false)

	// Wait for background registration to succeed (up to 30s — retry base is 5s).
	deadline := time.Now().Add(30 * time.Second)
	for time.Now().Before(deadline) {
		if client.AgentID() != "" {
			return // success
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Fatal("background registration did not succeed within timeout")
}

func TestRegister_WithoutGracefulDegradation_ReturnsError(t *testing.T) {
	// Without graceful degradation, unreachable API should return error.
	client, err := dome.NewClient(
		dome.WithAPIKey("test-key"),
		dome.WithAPIURL("http://127.0.0.1:1"),
		dome.WithoutHeartbeat(),
	)
	if err != nil {
		t.Fatalf("NewClient error: %v", err)
	}
	defer func() { _ = client.Close() }()

	_, err = client.Register(context.Background(), dome.RegisterOptions{
		Name: "strict-agent",
	})
	if err == nil {
		t.Fatal("expected error without graceful degradation")
	}
}

func TestInit_Shutdown(t *testing.T) {
	t.Setenv("DOME_AGENT_TOKEN", "")
	t.Setenv("DOME_API_KEY", "")
	t.Setenv("DOME_TOKEN", "")

	err := dome.Init(dome.WithAPIKey("test-key"), dome.WithoutHeartbeat())
	if err != nil {
		t.Fatalf("Init error: %v", err)
	}

	err = dome.Shutdown(context.Background())
	if err != nil {
		t.Fatalf("Shutdown error: %v", err)
	}

	// Shutdown again should be safe.
	err = dome.Shutdown(context.Background())
	if err != nil {
		t.Fatalf("second Shutdown error: %v", err)
	}
}
