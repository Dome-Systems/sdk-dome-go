---
description: Locked terminology and positioning phrases for all Dome Platform content
---

# Terminology (Locked)

**Product names:**
- **Dome Platform** - The complete platform (Dome + Moot)
- **Dome** - Control plane component (agent registry, Vault, gateway)
- **Moot** - Authorization service component (intent, evidence, judges)

**Key terms (use consistently):**
| Term | Use | Don't Use |
|------|-----|-----------|
| Dome Platform | ✅ | ❌ IBAC Adjudicator, Adjudicator Platform |
| Intent extraction | ✅ | ❌ Intent parsing, intent detection |
| The Court | ✅ | ❌ Judge panel, multi-model system |
| ext_authz | ✅ | ❌ external authorization, ext-authz |
| Evidence verification | ✅ | ❌ Claim validation |
| Precedent graduation | ✅ | ❌ Rule promotion, policy learning |

**See:** [docs/internal/reference/REF-glossary.md](docs/internal/reference/REF-glossary.md) for complete terminology

# Positioning (Locked)

**Use these phrases verbatim across all documents:**

**Platform-level:**
- "Dome Platform provides comprehensive agent management and authorization"
- "Control plane with intent-based authorization"
- "System of Control for Agentic Infrastructure" - governance layer for autonomous AI agents

**Technical patterns:**
- "ext_authz side-call pattern" (not "external authorization" or "auth proxy")
- "Authorization layer, not gateway" (Moot positioning)
- "The Court" (capitalized when referring to multi-judge system)

**Moot-specific:**
- "Thief to catch a thief" - intentional, probabilistic systems require intentional, probabilistic governance
- "You can't govern reasoning with rules alone"
- "Intent-based authorization for intent-driven systems"

# Key Differentiators (Moot)

Always list in this order:
1. **Intent Understanding** - Understands what agent is trying to accomplish
2. **Evidence Verification** - Corroborates claims via external systems (MCP)
3. **Deliberative Reasoning** - Multi-model consensus for complex cases
4. **Institutional Memory** - Learns from past decisions, graduates to policies
5. **Response Validation** - Catches PII/inference leakage before responses reach agents
