package policy

import (
	"testing"
)

const baseCedar = `
@id("capability-based-access")
permit(
    principal is Dome::Agent,
    action,
    resource
) when {
    principal.capabilities.contains(context.required_capability)
};

@id("denied-tools-block")
forbid(
    principal is Dome::Agent,
    action == Dome::Action::"mcp:call",
    resource is Dome::MCPTool
) when {
    principal.denied_tools.contains(resource.path)
};
`

const salaryPolicy = `
@id("hr-salary-access")
permit(
    principal is Dome::Agent,
    action == Dome::Action::"mcp:call",
    resource == Dome::MCPTool::"hr-mcp/get_salary"
) when {
    principal.capabilities.contains("hr:salary:read")
};

@id("hr-salary-restriction")
forbid(
    principal is Dome::Agent,
    action == Dome::Action::"mcp:call",
    resource == Dome::MCPTool::"hr-mcp/get_salary"
) when {
    principal.capabilities.contains("hr:salary:read") == false
};
`

func TestEngine_LoadBundle(t *testing.T) {
	e := NewEngine()
	err := e.LoadBundle(map[string]string{
		"base.cedar": baseCedar,
	}, "v1")
	if err != nil {
		t.Fatalf("LoadBundle error: %v", err)
	}
	if e.PolicyCount() != 2 {
		t.Errorf("PolicyCount = %d, want 2", e.PolicyCount())
	}
}

func TestEngine_LoadBundle_InvalidCedar(t *testing.T) {
	e := NewEngine()
	err := e.LoadBundle(map[string]string{
		"bad.cedar": "this is not valid cedar",
	}, "v1")
	if err == nil {
		t.Fatal("expected error for invalid Cedar")
	}
}

func TestEngine_Evaluate_CapabilityGate(t *testing.T) {
	e := NewEngine()
	if err := e.LoadBundle(map[string]string{"base.cedar": baseCedar}, "v1"); err != nil {
		t.Fatal(err)
	}

	agent := AgentContext{
		ID:           "agent-1",
		Capabilities: []string{"mcp:call"},
	}

	// Agent has mcp:call capability → allowed.
	d := e.Evaluate(agent, CheckInput{
		Action:             ActionMCPCall,
		Resource:           "hr-mcp/search_employees",
		ResourceType:       "mcp",
		RequiredCapability: ActionMCPCall,
	})
	if !d.Allow {
		t.Errorf("expected allow, got deny: %s", d.Reason)
	}
}

func TestEngine_Evaluate_MissingCapability(t *testing.T) {
	e := NewEngine()
	if err := e.LoadBundle(map[string]string{"base.cedar": baseCedar}, "v1"); err != nil {
		t.Fatal(err)
	}

	agent := AgentContext{
		ID:           "agent-1",
		Capabilities: []string{}, // no capabilities
	}

	d := e.Evaluate(agent, CheckInput{
		Action:             ActionMCPCall,
		Resource:           "hr-mcp/search_employees",
		ResourceType:       "mcp",
		RequiredCapability: ActionMCPCall,
	})
	if d.Allow {
		t.Error("expected deny for agent without capability")
	}
}

func TestEngine_Evaluate_DeniedToolsForbid(t *testing.T) {
	e := NewEngine()
	if err := e.LoadBundle(map[string]string{"base.cedar": baseCedar}, "v1"); err != nil {
		t.Fatal(err)
	}

	agent := AgentContext{
		ID:           "agent-1",
		Capabilities: []string{"mcp:call"},
		DeniedTools:  []string{"hr-mcp/get_salary"},
	}

	// Has capability but tool is denied → forbid wins.
	d := e.Evaluate(agent, CheckInput{
		Action:             ActionMCPCall,
		Resource:           "hr-mcp/get_salary",
		ResourceType:       "mcp",
		RequiredCapability: ActionMCPCall,
	})
	if d.Allow {
		t.Error("expected deny for denied tool, forbid should win over permit")
	}
}

func TestEngine_Evaluate_SalaryPolicy(t *testing.T) {
	e := NewEngine()
	if err := e.LoadBundle(map[string]string{
		"base.cedar":       baseCedar,
		"ceo-salary.cedar": salaryPolicy,
	}, "v1"); err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name         string
		capabilities []string
		resource     string
		wantAllow    bool
	}{
		{
			name:         "with hr:salary:read → allowed",
			capabilities: []string{"mcp:call", "hr:salary:read"},
			resource:     "hr-mcp/get_salary",
			wantAllow:    true,
		},
		{
			name:         "without hr:salary:read → denied by forbid",
			capabilities: []string{"mcp:call"},
			resource:     "hr-mcp/get_salary",
			wantAllow:    false,
		},
		{
			name:         "non-salary tool with mcp:call → allowed",
			capabilities: []string{"mcp:call"},
			resource:     "hr-mcp/search_employees",
			wantAllow:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			agent := AgentContext{
				ID:           "agent-1",
				Capabilities: tt.capabilities,
			}
			d := e.Evaluate(agent, CheckInput{
				Action:             ActionMCPCall,
				Resource:           tt.resource,
				ResourceType:       "mcp",
				RequiredCapability: ActionMCPCall,
			})
			if d.Allow != tt.wantAllow {
				t.Errorf("Allow = %v, want %v (reason: %s)", d.Allow, tt.wantAllow, d.Reason)
			}
		})
	}
}

func TestEngine_Evaluate_ResourceTypeInference(t *testing.T) {
	e := NewEngine()
	if err := e.LoadBundle(map[string]string{"base.cedar": baseCedar}, "v1"); err != nil {
		t.Fatal(err)
	}

	agent := AgentContext{
		ID:           "agent-1",
		Capabilities: []string{"mcp:call", "llm:chat"},
	}

	// ResourceType omitted — should be inferred from Action.
	d := e.Evaluate(agent, CheckInput{
		Action:             ActionLLMChat,
		Resource:           "openai/gpt-4",
		RequiredCapability: ActionLLMChat,
	})
	if !d.Allow {
		t.Errorf("expected allow for llm:chat with capability, got deny: %s", d.Reason)
	}
}

func TestEngine_Evaluate_NoPolicies(t *testing.T) {
	e := NewEngine()

	agent := AgentContext{ID: "agent-1", Capabilities: []string{"mcp:call"}}
	d := e.Evaluate(agent, CheckInput{
		Action:             ActionMCPCall,
		Resource:           "anything",
		RequiredCapability: ActionMCPCall,
	})
	if d.Allow {
		t.Error("expected deny with no policies loaded (default deny)")
	}
}
