package policy

import (
	"fmt"
	"strings"
	"sync"

	"github.com/cedar-policy/cedar-go"
)

// Decision represents the result of a policy evaluation.
type Decision struct {
	Allow         bool
	Reason        string
	PolicyVersion string
}

// AgentContext holds agent attributes for policy evaluation.
type AgentContext struct {
	ID           string
	TenantID     string
	Namespace    string
	Capabilities []string
	AllowedTools []string
	DeniedTools  []string
}

// CheckInput holds the request parameters for a policy check.
type CheckInput struct {
	Action             string
	Resource           string
	ResourceType       string // "mcp", "llm", "credential", or empty
	RequiredCapability string
	Context            map[string]string
}

// Engine evaluates Cedar policies locally.
type Engine struct {
	mu            sync.RWMutex
	policySet     *cedar.PolicySet
	policyVersion string
}

// NewEngine creates a new Cedar policy engine with no policies loaded.
func NewEngine() *Engine {
	return &Engine{
		policySet: cedar.NewPolicySet(),
	}
}

// LoadBundle replaces the current policy set with policies parsed from raw
// Cedar source files. Each entry maps filename to content.
func (e *Engine) LoadBundle(policies map[string]string, version string) error {
	newPolicySet := cedar.NewPolicySet()

	for filename, content := range policies {
		parsed, err := cedar.NewPolicySetFromBytes(filename, []byte(content))
		if err != nil {
			return fmt.Errorf("parse %s: %w", filename, err)
		}
		for name, p := range parsed.All() {
			uniqueName := cedar.PolicyID(fmt.Sprintf("%s:%s", filename, name))
			newPolicySet.Add(uniqueName, p)
		}
	}

	e.mu.Lock()
	e.policySet = newPolicySet
	e.policyVersion = version
	e.mu.Unlock()

	return nil
}

// Evaluate runs Cedar policy evaluation for the given agent and request.
func (e *Engine) Evaluate(agent AgentContext, input CheckInput) *Decision {
	e.mu.RLock()
	defer e.mu.RUnlock()

	// Build Cedar request.
	principal := cedar.NewEntityUID(EntityTypeAgent, cedar.String(agent.ID))
	action := cedar.NewEntityUID(EntityTypeAction, cedar.String(input.Action))
	resource := mapResource(input)

	contextMap := cedar.RecordMap{}
	if input.RequiredCapability != "" {
		contextMap[cedar.String("required_capability")] = cedar.String(input.RequiredCapability)
	}
	for k, v := range input.Context {
		contextMap[cedar.String(k)] = cedar.String(v)
	}

	req := cedar.Request{
		Principal: principal,
		Action:    action,
		Resource:  resource,
		Context:   cedar.NewRecord(contextMap),
	}

	// Build entities.
	entities := cedar.EntityMap{}

	agentEntity := cedar.Entity{
		UID:        principal,
		Attributes: buildAgentAttributes(agent),
	}
	entities[principal] = agentEntity

	resourceEntity := cedar.Entity{
		UID:        resource,
		Attributes: buildResourceAttributes(input),
	}
	entities[resource] = resourceEntity

	// Evaluate.
	decision, diagnostic := cedar.Authorize(e.policySet, entities, req)

	return &Decision{
		Allow:         decision == cedar.Allow,
		Reason:        extractReason(decision, diagnostic),
		PolicyVersion: e.policyVersion,
	}
}

// PolicyCount returns the number of loaded policies.
func (e *Engine) PolicyCount() int {
	e.mu.RLock()
	defer e.mu.RUnlock()
	count := 0
	for range e.policySet.All() {
		count++
	}
	return count
}

// HasPolicies returns true if any policies are loaded.
func (e *Engine) HasPolicies() bool {
	return e.PolicyCount() > 0
}

func mapResource(input CheckInput) cedar.EntityUID {
	resType := input.ResourceType
	if resType == "" {
		// Infer from action.
		switch {
		case strings.HasPrefix(input.Action, "mcp:"):
			resType = "mcp"
		case strings.HasPrefix(input.Action, "llm:"):
			resType = "llm"
		case strings.HasPrefix(input.Action, "credential:"):
			resType = "credential"
		}
	}

	entityType := EntityTypeResource
	switch resType {
	case "mcp":
		entityType = EntityTypeMCPTool
	case "llm":
		entityType = EntityTypeLLMModel
	case "credential":
		entityType = EntityTypeCredential
	}
	return cedar.NewEntityUID(entityType, cedar.String(input.Resource))
}

func buildAgentAttributes(agent AgentContext) cedar.Record {
	attrs := cedar.RecordMap{
		cedar.String("id"): cedar.String(agent.ID),
	}

	if agent.TenantID != "" {
		attrs[cedar.String("tenant_id")] = cedar.String(agent.TenantID)
	}
	if agent.Namespace != "" {
		attrs[cedar.String("namespace")] = cedar.String(agent.Namespace)
	}

	attrs[cedar.String("capabilities")] = toStringSet(agent.Capabilities)
	attrs[cedar.String("allowed_tools")] = toStringSet(agent.AllowedTools)
	attrs[cedar.String("denied_tools")] = toStringSet(agent.DeniedTools)

	return cedar.NewRecord(attrs)
}

func buildResourceAttributes(input CheckInput) cedar.Record {
	attrs := cedar.RecordMap{
		cedar.String("path"): cedar.String(input.Resource),
	}
	if input.ResourceType != "" {
		attrs[cedar.String("type")] = cedar.String(input.ResourceType)
	}
	return cedar.NewRecord(attrs)
}

func toStringSet(values []string) cedar.Value {
	if len(values) == 0 {
		return cedar.NewSet()
	}
	items := make([]cedar.Value, len(values))
	for i, v := range values {
		items[i] = cedar.String(v)
	}
	return cedar.NewSet(items...)
}

func extractReason(decision cedar.Decision, diagnostic cedar.Diagnostic) string {
	if decision == cedar.Allow {
		if len(diagnostic.Reasons) > 0 {
			return fmt.Sprintf("allowed by policy: %s", diagnostic.Reasons[0].PolicyID)
		}
		return "allowed"
	}
	if len(diagnostic.Reasons) > 0 {
		return fmt.Sprintf("denied by policy: %s", diagnostic.Reasons[0].PolicyID)
	}
	if len(diagnostic.Errors) > 0 {
		return fmt.Sprintf("policy error: %s", diagnostic.Errors[0].String())
	}
	return "denied: no matching permit policy"
}
