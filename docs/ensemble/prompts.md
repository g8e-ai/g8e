# Prompts

## Overview

g8ee uses a structured prompt architecture to drive agent behavior. Prompts are templated, composable, and enforce governance constraints at the prompt level.

## Prompt Structure

Prompts are organized by:

- **Agent persona** — Each agent role has dedicated prompt templates
- **Task type** — Templates for different action categories
- **Governance context** — L1/L2/L3 rules injected into prompts to constrain agent behavior

## Key Design Principles

- **Separation of concerns** — System prompts, task prompts, and governance rules are maintained independently
- **Composability** — Prompts are assembled from reusable components
- **Determinism** — Prompt structure is designed to produce consistent, parseable outputs
- **Auditability** — All prompts are versioned and logged with transactions

## Related

- [Agents](agents.md) — Agent personas that consume prompts
- [Thinking](thinking.md) — How prompts interact with provider reasoning
- [LLM Providers](llm-providers.md) — Provider-specific prompt handling
