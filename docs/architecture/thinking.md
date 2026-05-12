---
title: Thinking Levels
parent: Architecture
---

# Thinking & Reasoning

Last Updated: 2026-05-12
Version: v0.2.4

"Thinking" in g8e is a dual-layered architecture designed to guarantee that agentic actions are the result of deliberate, cross-validated reasoning rather than reflexive generation. 

1.  **Structural Reasoning (L2 Consensus)**: A verifiable proof that an action was generated through deliberate consensus (e.g., via the bundled **Tribunal** in the application layer).
2.  **Provider Reasoning (LLM Thinking)**: The native reasoning capabilities of LLM providers (e.g., Gemini's `thinking_config`, Anthropic's `thinking_budget`) abstracted into a canonical vocabulary.

## 1. Structural Reasoning: Consensus

g8e enforces structural reasoning at the **Substrate** level by requiring an L2 Consensus proof in the UAP JSON envelope for state-changing operations. 

The bundled **Engine** (`g8ee`) adapter implements this via a **Tribunal** of five heterogeneous agents. This ensures that every shell command is debated before execution.

- **Information Isolation**: Members generate candidates in parallel, sealed environments.
- **Heterogeneous Lenses**: Personas like **Axiom** (composition), **Variance** (edge cases), and **Nemesis** (adversary) ensure the problem is viewed from multiple angles.
- **Weighted Voting**: A deterministic voting ladder (shortest wins, non-Nemesis wins) resolves candidate sets into a single winner.
- **Auditing & Revision**: An independent **Auditor** verifies the winner against the original intent, potentially revising or swapping it if technical flaws are found.

For a deep dive into the Tribunal's mechanics and the L1/L2/L3 hierarchy, see `@/home/bob/g8e/docs/architecture/governance.md`.

## 2. Provider Reasoning: The Thinking Level Abstraction

g8e unifies provider-specific reasoning features (Anthropic, Gemini, OpenAI, Ollama) behind a canonical abstraction. This allows agents to request "High Reasoning" without needing to know the underlying provider's wire format.

### Why This Abstraction Exists

- **Provider Agnosticism**: Adding a new provider requires only a new translator, not changes to agent logic.
- **Deep Grounding over Local Data**: Thinking levels serve as "volume knobs" for how intensely a model should reason over the deep context (state, history, user preferences) injected by the Engine.
- **Machine-Domain Coherence**: Higher thinking levels provide the computational headroom necessary for Sage and the Tribunal to maintain the thread in complex, multi-turn tool loops.

### Canonical Vocabulary

The `ThinkingLevel` enum (defined in `@/home/bob/g8e/components/g8ee/app/constants/settings.py`) defines intensity:

| Value     | Semantics                                                        |
|-----------|------------------------------------------------------------------|
| `OFF`     | Thinking disabled. Providers omit all thinking-related fields.  |
| `MINIMAL` | Least-expensive non-zero reasoning. Small token budget.          |
| `LOW`     | Light reasoning.                                                 |
| `MEDIUM`  | Default reasoning for most primary/tool calls.                  |
| `HIGH`    | Maximum reasoning the model exposes.                            |

### The Lifecycle: From Intent to Wire Format

1.  **Agent Declares Desired Level**: Primary reasoning agents (Sage, Tribunal) default to `HIGH` via `@/home/bob/g8e/components/g8ee/app/services/ai/generation_config_builder.py`.
2.  **Request Builder Clamps to Model Reality**: `AIGenerationConfigBuilder` looks up the `LLMModelConfig` and calls `clamp_thinking_level`. If a model has no reasoning support, it returns `OFF`. If it's always-on, it returns the lowest supported level.
3.  **Provider Translator Converts to Wire Format**: Pure functions in `@/home/bob/g8e/components/g8ee/app/llm/thinking.py` translate the clamped level:
    - **Gemini**: `thinking_config` level.
    - **Anthropic**: `budget_tokens` from model-specific budgets.
    - **OpenAI**: `reasoning_effort` intensity.
    - **Ollama**: `think` boolean dispatched by the model's `thinking_dialect`.
4.  **Adapter Applies Translation**: The adapter (e.g., `@/home/bob/g8e/components/g8ee/app/llm/providers/anthropic.py`) applies the final parameters to the SDK request, including **Output Reserve Uplift** to ensure response headroom.

## Thinking in the Data Stream

The model's internal thinking process is treated as a first-class component of the UAP JSON envelope.

### The Thinking State Machine
`@/home/bob/g8e/components/g8ee/app/services/ai/agent_turn.py` implements a state machine to handle interleaved thought and output tokens. When a `thought` chunk is received, the Engine accumulates it and emits a `THINKING_START` signal. When non-thought tokens arrive, it flushes the thoughts and signals `THINKING_END`.

### Thought Signatures
Provider-emitted signatures (e.g., Gemini) are preserved across turns to maintain tool-calling context and are stored in the local audit vault.

## Sovereignty & Safety (LFAA)

Reasoning intensity **never** compromises the **Local-First Audit Architecture (LFAA)**.

1.  **Scrubbing Protocol**: Sensitive PII and secrets are redacted by the Operator *before* the context reaches the Engine. Models only "think" over scrubbed data.
2.  **Audit Vault**: The model's "thought process" is returned to the host, encrypted, and stored in the local Audit Vault.
3.  **Governance Persistence**: Reasoning results (votes, rationales) are bound to the UAP JSON envelope in the `governance` block, providing a tamper-evident chain of reasoning for every action.

## Testing Discipline

- **Translators**: `@/home/bob/g8e/components/g8ee/tests/unit/llm/test_thinking_translators.py` verifies the mapping and budget logic.
- **Clamping**: Verified through model config unit tests ensuring adherence to priority rules.
- **Integration**: `tests/conftest.py` probes thinking support against live models during test runs.

**Source of Truth**: `LLMModelConfig` in `@/home/bob/g8e/components/g8ee/app/models/model_configs.py` is the single authoritative source for model capabilities.
