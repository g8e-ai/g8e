# Agents

## Overview

g8ee uses a multi-agent ensemble architecture where agents collaborate through structured roles. Each agent has a defined persona, scope of authority, and set of capabilities.

## Agent Hierarchy

Agents are organized in a tiered hierarchy with clear separation of responsibilities:

- **Tribunal Agents** — Participate in L2 consensus, evaluate transactions, and sign GovernanceEnvelopes
- **Specialist Agents** — Domain-specific agents for tasks like code analysis, security review, and infrastructure planning
- **Coordinator Agents** — Manage ensemble coordination, task delegation, and result aggregation

## Personas

Each agent persona defines:

- **Role** — The agent's function within the ensemble
- **Scope** — What the agent is authorized to do
- **Capabilities** — Tools and APIs available to the agent
- **Constraints** — L1/L2/L3 governance rules that bound the agent's actions

## Ensemble Coordination

Agents communicate through structured protocols:

- Task delegation via typed messages
- Consensus rounds for L2 governance
- Result aggregation and conflict resolution
- Reputation tracking across consensus rounds

## Related

- [Governance](governance.md) — How agents participate in the 3-layer model
- [Thinking](thinking.md) — Provider reasoning and thought signatures
- [Prompts](prompts.md) — Prompt architecture that drives agent behavior
