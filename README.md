# Dome Go SDK

Go client SDK for the [Dome Platform](https://domesystems.ai) — agent registration, authentication, and governance.

## Install

```bash
go get github.com/Dome-Systems/sdk-dome-go
```

## Quick Start

```go
package main

import (
    "context"
    "log"
    "os"

    dome "github.com/Dome-Systems/sdk-dome-go"
)

func main() {
    // Initialize with credentials from `dome agents register`.
    if err := dome.Init(dome.WithCredentials(os.Getenv("DOME_AGENT_TOKEN"))); err != nil {
        log.Fatal(err)
    }
    defer dome.Shutdown(context.Background())

    // Register your agent.
    agent, err := dome.Register(context.Background(), dome.RegisterOptions{
        Name: "my-agent",
    })
    if err != nil {
        log.Fatal(err)
    }

    log.Printf("Registered agent %s (ID: %s)", agent.Name, agent.ID)
    // Your agent runs here. Heartbeat is automatic.
}
```

## Authentication

The SDK accepts credentials in several ways (checked in order):

1. `dome.WithCredentials(token)` — opaque credential blob from `dome agents register`
2. `dome.WithAPIKey(key)` — static API key
3. `DOME_AGENT_TOKEN` environment variable
4. `DOME_API_KEY` environment variable
5. `DOME_TOKEN` environment variable

The recommended approach is `WithCredentials` with the token from `dome agents register`, which includes Vault OIDC authentication automatically.

## Documentation

Full documentation is available at [docs.domesystems.ai](https://docs.domesystems.ai).

## License

MIT License. See [LICENSE](LICENSE) for details.
