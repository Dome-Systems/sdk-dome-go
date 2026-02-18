---
description: Go SDK development patterns for the Dome Go SDK
---

# Go SDK Skill

## Public API Design

The SDK exposes two usage patterns:

### Global Client (recommended for most users)
```go
dome.Init(dome.WithCredentials(token))
defer dome.Shutdown(ctx)
agent, err := dome.Register(ctx, dome.RegisterOptions{Name: "my-agent"})
decision, err := dome.Check(ctx, dome.CheckRequest{Action: "read", Resource: "users"})
```

### Explicit Client (for multi-client or testing)
```go
client, err := dome.NewClient(dome.WithCredentials(token))
defer client.Close()
agent, err := client.Register(ctx, dome.RegisterOptions{Name: "my-agent"})
```

## Functional Options Pattern

All configuration uses functional options:
```go
type Option func(*clientConfig)

func WithCredentials(token string) Option     // Opaque base64 credential blob
func WithCredentialsFile(path string) Option  // Read credentials from file
func WithAPIURL(url string) Option            // Dome API server URL
func WithMode(mode Mode) Option               // Operating mode
func WithLogger(l *slog.Logger) Option        // Custom logger
func WithHeartbeatInterval(d time.Duration) Option
func WithoutHeartbeat() Option
```

## Credential Handling

The credential flow is:
1. User passes opaque base64 token from `dome agents register`
2. `WithCredentials()` stores raw token string
3. `NewClient()` calls `decodeToken()` to parse → `AgentCredentials`
4. `AgentCredentials.AuthMethod` determines transport:
   - `"approle"` → VaultAuthConfig with AppRole creds
   - `"kubernetes"` → VaultAuthConfig with K8s SA auth
   - Empty/missing → fall back to raw token as Bearer

Internal types (not exported):
- `agentCredentials` — parsed JSON from base64 blob
- `vaultAuthConfig` — Vault connection params
- `vaultTransport` — http.RoundTripper that handles Vault OIDC

## Transport Layer

`internal/vault/transport.go` implements `http.RoundTripper`:
1. Authenticates to Vault (AppRole or K8s SA)
2. Requests OIDC identity token from `identity/oidc/token/<role>`
3. Caches tokens with TTL buffer (30s for OIDC, 60s for Vault native)
4. Injects `Authorization: Bearer <oidc-jwt>` on outgoing requests

## Mode System

```go
type Mode int
const (
    Observe  Mode = iota  // Telemetry only, no registration required
    Monitor               // Register + heartbeat (default)
    Enforce               // Monitor + Cedar policy checks (future)
    Govern                // Enforce + Moot intent authorization (future)
)
```

Mode affects behavior:
- **Observe**: Init succeeds even without valid credentials
- **Monitor**: Registration required, heartbeat runs
- **Enforce/Govern**: Not yet implemented, reserved for future

## Testing Patterns

### Mock Connect Server
```go
func testServer(t *testing.T) string {
    mux := http.NewServeMux()
    mux.Handle(agentv1connect.NewAgentRegistryHandler(&mockHandler{}))
    server := httptest.NewServer(mux)
    t.Cleanup(server.Close)
    return server.URL
}
```

### Mock Vault
```go
vault := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
    switch {
    case r.URL.Path == "/v1/auth/approle/login":
        // Return mock vault token
    case strings.HasPrefix(r.URL.Path, "/v1/identity/oidc/token/"):
        // Return mock OIDC JWT
    }
}))
```

### Test Organization
- `client_test.go` — NewClient, Register, Close (uses mock Connect server)
- `credentials_test.go` — DecodeToken with real/mock credential blobs
- `heartbeat_test.go` — Heartbeat interval, failure resilience, disable option
- `internal/vault/transport_test.go` — Vault auth flows, token caching, refresh

## Release Process

1. Tag with semver: `git tag v0.1.0`
2. Push tag: `git push origin v0.1.0`
3. Go proxy picks it up automatically
4. Update CHANGELOG.md with release notes
