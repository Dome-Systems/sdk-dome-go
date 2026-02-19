// Package policy provides Cedar-based policy evaluation for the Dome SDK.
// The entity model matches the sidecar's implementation in prod-platform
// (dome/internal/policy/entities.go) to ensure consistent policy evaluation.
package policy

import "github.com/cedar-policy/cedar-go"

// Entity type constants — must match prod-platform exactly.
const (
	EntityTypeAgent      = cedar.EntityType("Dome::Agent")
	EntityTypeAction     = cedar.EntityType("Dome::Action")
	EntityTypeMCPTool    = cedar.EntityType("Dome::MCPTool")
	EntityTypeLLMModel   = cedar.EntityType("Dome::LLMModel")
	EntityTypeCredential = cedar.EntityType("Dome::Credential")
	EntityTypeResource   = cedar.EntityType("Dome::Resource")
)

// Action constants — must match prod-platform exactly.
const (
	ActionMCPCall          = "mcp:call"
	ActionLLMChat          = "llm:chat"
	ActionCredentialFetch  = "credential:fetch"
	ActionCredentialRevoke = "credential:revoke"
)
