# Dome Go SDK

Go client SDK for the Dome Platform — agent registration, authentication,
and governance.

## This is a public SDK

Every decision must prioritize the developer experience. This is the first
thing a customer touches. It must be:
- **Minimal** — small API surface, few dependencies, zero config for common cases
- **Idiomatic** — feels like stdlib Go, not a framework
- **Documented** — every public symbol has godoc, every pattern has an example
- **Stable** — no breaking changes without a major version bump

## Package Design

Module: `github.com/Dome-Systems/sdk-dome-go`, package: `dome`

```go
import "github.com/Dome-Systems/sdk-dome-go"

dome.Init(dome.WithCredentials(os.Getenv("DOME_AGENT_TOKEN")))
defer dome.Shutdown(context.Background())

agent, err := dome.Start(ctx, dome.StartOptions{Name: "my-agent"})
```

## Rules

- `.claude/rules/terminology.md` — Locked product terms
- `.claude/rules/sdk-design.md` — SDK design rules (public API, dependencies, errors)

## Skills

- `.claude/skills/go-sdk/skill.md` — Go SDK development patterns

## Build

```bash
make test       # Run all tests
make lint       # Run golangci-lint
make generate   # Regenerate protobuf (requires buf)
```

## Dependencies (locked — additions require justification)

- `connectrpc.com/connect` — Connect RPC client
- `google.golang.org/protobuf` — Protobuf runtime

No other dependencies. No zerolog, no zap, no logrus. Logging uses `log/slog` (stdlib).

## Design Spec

The full SDK design specification lives in **prod-platform**:
[`docs/specs/SPEC-sdk-design.md`](https://github.com/Dome-Systems/prod-platform/blob/main/docs/specs/SPEC-sdk-design.md)

Covers: public API surface, enforcement progression (Phase 1-4), credential flow, Vault OIDC auth, multi-language strategy, dependency policy, release plan.

Related: [ADR-009 — SDK-First Enforcement](https://github.com/Dome-Systems/prod-platform/blob/main/docs/implementation/adrs/ADR-009-sdk-first-enforcement.md), [UJ-2 — Register My Agent](https://github.com/Dome-Systems/prod-platform/blob/main/docs/use-cases/UJ-2-register-agent-sdk.md)

## Architecture

- Public API: `dome.go`, `client.go`, `options.go`, `agent.go`, `check.go`, `middleware.go`
- Internal: `internal/api/` (generated protobuf), `internal/vault/` (Vault auth transport)
- Vault OIDC auth is an implementation detail — developers never see it
- The credential blob from `dome agents register` is opaque to consumers

## Test Requirements

- Table-driven tests for all public functions
- httptest-based mock server for RPC tests
- Mock Vault endpoint for auth tests
- >90% coverage target
- `go test -race ./...` must pass
