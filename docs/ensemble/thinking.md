# Thinking

## Overview

g8ee leverages LLM provider reasoning capabilities (thinking/reasoning tokens) as part of the L2 consensus process. Provider thinking is captured, signed, and incorporated into GovernanceEnvelope transactions.

## Provider Reasoning

Different LLM providers expose reasoning differently:

- **Gemini** — Native thinking tokens with thought signatures
- **Other providers** — Reasoning captured via structured output parsing

## Thought Signatures

Some providers (e.g., Gemini) cryptographically sign their thinking output. These signatures are:

- Verified before incorporation into consensus
- Included in GovernanceEnvelope metadata
- Used to establish provenance and non-repudiation of agent reasoning

## L2 Consensus Integration

Thinking output feeds into the Tribunal consensus process:

1. Agents produce reasoning via provider thinking
2. Reasoning is verified (signatures checked where available)
3. Verified reasoning is shared with other Tribunal agents
4. Consensus round aggregates agent reasoning into a decision
5. Decision is signed and packaged into a GovernanceEnvelope

## Related

- [Governance](governance.md) — L2 consensus process
- [LLM Providers](llm-providers.md) — Provider-specific reasoning implementations
- [Agents](agents.md) — Tribunal agent composition
