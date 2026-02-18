---
description: SDK design rules for the Dome Go SDK public API
---

# SDK Design Rules

## Public API Surface

- **Minimal surface** — every new exported symbol needs justification
- **Functional options** for all configuration — no config structs in public API
- **Two entry points**: `dome.Init()` (global) and `dome.NewClient()` (explicit)
- **Stubs are OK** — `Check()` and `Middleware()` exist as stubs to reserve the API shape

## Error Handling

- All errors prefixed `"dome: "` — consistent, greppable
- Wrap underlying errors with `%w` for unwrapping
- Never panic — return errors from all public functions

## Context

- Context flows through all blocking operations
- Background goroutines (heartbeat) are cancellable via `Close()`/`Shutdown()`
- Init/Shutdown use `context.Background()` internally when no context is provided

## Logging

- **No forced logging framework** — use `log/slog` with `WithLogger()` option
- Default: `slog.Default()` (writes to stderr)
- SDK should be quiet by default — only log warnings and errors

## Dependencies

- Dependencies must be justified — prefer stdlib
- `connectrpc.com/connect` and `google.golang.org/protobuf` are the only allowed third-party deps
- No HTTP framework, no logging framework, no config framework

## Versioning

- Breaking changes require major version bump — additive changes only in minor/patch
- Follow Go module versioning: v0.x for pre-1.0, semver after

## Examples

- Examples compile — every example in `examples/` and godoc must build
- Examples use `dome.Init()` pattern (global client) for simplicity

## Credentials

- Credential internals are hidden — consumers pass opaque token string, SDK handles Vault
- `WithCredentials(token)` decodes the base64 blob and auto-configures Vault auth
- `WithCredentialsFile(path)` reads token from a file
- Fallback: `DOME_AGENT_TOKEN` env var, then `DOME_API_KEY`, then `DOME_TOKEN`

## Resilience

- **Graceful degradation** — agent starts even if Dome API is temporarily unreachable
- Heartbeat failures are logged, not fatal
- Registration retries are the caller's responsibility (SDK provides building blocks)
