---
title: Thinking & Reasoning
parent: Architecture
---

# Thinking & Reasoning

"Thinking" in g8e is a dual-layered architecture designed to guarantee that agentic actions are the result of deliberate, cross-validated reasoning rather than reflexive generation.

1.  **Structural Reasoning (L2 Consensus)**: A verifiable proof that an action was generated through deliberate consensus (the **Tribunal**).
2.  **Provider Reasoning (LLM Thinking)**: The native reasoning capabilities of LLM providers (e.g., Gemini's `thinking_config`, Anthropic's `thinking_budget`, OpenAI's `reasoning_effort`) abstracted into a canonical internal vocabulary.

## 1. Structural Reasoning: The Tribunal

Structural reasoning is the process of generating an intended action through consensus. In the reference application, this is implemented by the **Tribunal** within the **Engine** (`g8ee`).

The outcome of this process is a valid **L2 Signature** bound to a `GovernanceEnvelope` (UAP JSON). The **Substrate** (`g8eo`) enforces this signature as a hard gate before any state-changing operation reaches the host.

### Mechanics of Consensus
- **Information Isolation**: Five heterogeneous agents (Axiom, Concord, Variance, Pragma, Nemesis) generate candidate commands in parallel, sealed environments.
- **Specialized Lenses**: Each member views the intent through a specific lens (e.g., **Axiom** for composition, **Concord** for safety).
- **The Auditor**: An independent **Auditor** stage (primary model tier) judges the candidates against the original intent, with the authority to revise or swap the winner if technical flaws (e.g., quoting errors, flag misuse) are found.
- **Warden Defensive Analysis**: The **Warden** stage performs pre-execution risk classification (LOW, MEDIUM, HIGH) to calibrate the human approval experience.

For a deep dive into the Tribunal's mechanics, see `@/home/bob/g8e/docs/architecture/governance.md`.

## 2. Provider Reasoning: Thinking Levels

Provider reasoning refers to the internal "chains of thought" produced by modern LLMs. g8e unifies these provider-specific features behind a canonical abstraction, allowing agents to request "High Reasoning" without knowing the underlying wire format.

### Canonical Vocabulary
The `ThinkingLevel` enum (defined in `@/home/bob/g8e/components/g8ee/app/constants/settings.py`) defines intensity:

| Value     | Semantics                                                        |
|-----------|------------------------------------------------------------------|
| `OFF`     | Thinking disabled. Providers omit all thinking-related fields.  |
| `MINIMAL` | Least-expensive non-zero reasoning. Small token budget.          |
| `LOW`     | Light reasoning.                                                 |
| `MEDIUM`  | Default reasoning for most primary calls.                        |
| `HIGH`    | Maximum reasoning the model exposes.                            |

### The Lifecycle: From Intent to Wire Format

1.  **Agent Declaration**: Primary reasoning agents (like **Sage**) default to `HIGH` reasoning via the `AIGenerationConfigBuilder`.
2.  **Clamping**: `AIGenerationConfigBuilder` looks up the `LLMModelConfig` and calls `clamp_thinking_level`. If a model has no reasoning support (e.g., legacy or "lite" models), it returns `OFF`.
3.  **Translation**: Pure functions in `@/home/bob/g8e/components/g8ee/app/llm/thinking.py` translate the level for the provider:
    - **Gemini**: `thinking_config.thinking_level`.
    - **Anthropic**: `thinking.budget_tokens` (mapped via model-specific budgets).
    - **OpenAI**: `reasoning.effort`.
    - **Ollama**: Dispatched via `ThinkingDialect` (e.g., `native_toggle` for models supporting the `think` kwarg).
4.  **Turn Processing**: The Engine consumes the stream through a state machine in `@/home/bob/g8e/components/g8ee/app/services/ai/agent_turn.py`:
    - `THINKING`: Accumulates thought chunks.
    - `THINKING_END`: Signals the transition to visible output (text or tool calls).
    - **Preservation**: Thought tokens are stored in `Part` objects (`thought=True`) and preserved across turns to maintain tool-calling context.

## 3. Sovereignty & Safety

Reasoning never bypasses the core security invariants of the g8e Protocol.

1.  **Scrubbing Protocol**: Sensitive data is redacted by the **Operator** (LFAA scrubbing) *before* context reaches the Engine. Models only "think" over scrubbed data.
2.  **Audit Vault**: Internal "thought processes" are returned to the host and stored in the local **Audit Vault**. They are treated as first-class metadata of the transaction.
3.  **Governance Binding**: Reasoning results (votes, rationales, auditor verdicts) are committed to the `governance` block of the UAP JSON envelope, providing a tamper-evident chain of reasoning for every action.

## Implementation Details

- **Model Capabilities**: `LLMModelConfig` in `@/home/bob/g8e/components/g8ee/app/models/model_configs.py` is the single source of truth for which models support thinking.
- **Enforcement**: Structural consensus is enforced in the `g8eo` dispatch path; provider thinking is orchestrated in the `g8ee` ReAct loop.
