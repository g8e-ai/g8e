---
title: Thinking Levels
parent: Architecture
---

# Thinking Levels

Last Updated: 5-6-2026
Version: v.0.2.0

g8ee unifies extended reasoning across four LLM providers (Anthropic, Gemini, OpenAI, Ollama) behind a single abstraction. Each provider exposes "thinking" differently—Gemini uses `thinking_config`, OpenAI uses `reasoning_effort`, Anthropic uses token budgets, and Ollama uses a boolean flag. Rather than letting this provider-specific vocabulary leak into the agent layer, g8e defines a canonical vocabulary and a per-provider translation layer.

## Why This Abstraction Exists

**Provider heterogeneity is a leaky abstraction problem.** Without a canonical layer, every agent would need to know that "high reasoning" on Anthropic means a 16K token budget, on OpenAI it means `reasoning_effort="high"`, and on Ollama it means `think=True`. This makes agents brittle to provider changes and impossible to reason about.

**Deep Grounding over Local Data.** g8e doesn't just send a prompt; it injects a high-fidelity "world view" (state, history, user preferences, fleet context) that remains local to the host. The thinking levels are the volume knobs for how intensely a model should reason over this deep context. Higher thinking levels are assigned to the **ReAct loop**'s primary reasoner (Sage), ensuring that when a complex investigation is underway, the model is given the computational headroom to synthesize thousands of lines of local context without losing the "machine-domain" thread.

**The abstraction boundary is intentional.** The agent layer speaks in terms of reasoning intensity (OFF through HIGH). The provider layer translates that intensity into whatever wire format the provider expects. This separation allows:
- **Provider Agnosticism**: Adding new providers without touching agent code.
- **Model Flexibility**: Swapping models within a provider without changing agent intent.
- **Testability**: Testing agent behavior independently of provider mechanics.

## Canonical Vocabulary

The `ThinkingLevel` enum (defined in `app/constants/settings.py`) defines five values representing reasoning intensity:

| Value     | Semantics                                                        |
|-----------|------------------------------------------------------------------|
| `OFF`     | Thinking disabled. Providers omit all thinking-related fields.  |
| `MINIMAL` | Least-expensive non-zero reasoning. Small token budget.          |
| `LOW`     | Light reasoning.                                                 |
| `MEDIUM`  | Default reasoning for most primary/tool calls.                  |
| `HIGH`    | Maximum reasoning the model exposes.                            |

**`OFF` is a first-class value, never `None`.** Using `OFF` explicitly makes intent and schema agree. The `ThinkingConfig` object defaults to `OFF`, ensuring that thinking is an opt-in feature.

**Priority ordering is explicit.** `THINKING_LEVEL_PRIORITY_ASC = (MINIMAL, LOW, MEDIUM, HIGH)` defines the only ordering used by clamping logic. `OFF` is excluded—it is not a priority, it is the absence of priority.

## The Lifecycle: From Agent Intent to Wire Format

### 1. Agent Declares Desired Level

Agents specify a `ThinkingLevel` when requesting generation. Primary agents (Proposer, Tribunal) default to `HIGH` via `AIGenerationConfigBuilder._build_thinking_config()`. Lightweight agents (triage, title generation) explicitly use `OFF` via `AIGenerationConfigBuilder.get_lite_generation_config()` to minimize latency and cost.

### 2. Request Builder Clamps to Model Reality

`AIGenerationConfigBuilder` looks up the `LLMModelConfig` for the bound model and calls `clamp_thinking_level(desired, config)`. This function (in `app/models/model_configs.py`) resolves the agent's desired level based on the model's `supported_thinking_levels`:

- **No Support**: If `supported_thinking_levels` is empty, returns `OFF`.
- **Opt-in (`OFF` in list)**: Returns `OFF` if desired; otherwise returns highest supported level ≤ desired.
- **Always-on (no `OFF` in list)**: If `OFF` is desired, returns the lowest supported level (e.g., `MINIMAL`).
- **Fallback**: If the desired level is below everything supported, returns the lowest supported level.

### 3. Provider Translator Converts to Wire Format

Each provider has a pure translator function in `app/llm/thinking.py` that takes the clamped level and model config, returning a typed result the provider adapter applies to its outbound request:

- **Gemini**: `translate_for_gemini` returns a `GeminiThinkingTranslation` with `thinking_level` string (e.g., `"high"`).
- **Anthropic**: `translate_for_anthropic` returns an `AnthropicThinkingTranslation` with `budget_tokens` derived from `thinking_budgets` or defaults.
- **OpenAI**: `translate_for_openai` returns an `OpenAIThinkingTranslation` with `reasoning_effort` string (e.g., `"medium"`).
- **Ollama**: `translate_for_ollama` returns an `OllamaThinkingTranslation` with a `think` boolean, dispatched by the model's `thinking_dialect`.

### 4. Provider Adapter Applies Translation

The adapter (e.g., `app/llm/providers/anthropic.py`) applies the translation to the SDK request. 

**The Output Reserve Uplift**: For Anthropic, which requires `max_tokens > budget_tokens`, the adapter uplifts `max_tokens` to `budget_tokens + thinking_output_reserve`. This ensures the model has enough headroom for a complete response after its internal reasoning phase.

## Processing the "Thought" Stream

In the **ReAct loop** (orchestrated by `g8eEngine` in `app/services/ai/agent.py`), the model's thinking process is handled as a first-class stream component.

### The Thinking State Machine

`app/services/ai/agent_turn.py` implements a state machine to handle interleaved thinking and output tokens:

1. **INACTIVE**: Waiting for the first chunk.
2. **ACTIVE**: A `thought` chunk is received. All subsequent text chunks are accumulated as thoughts.
3. **INACTIVE**: A non-thought chunk (text or tool call) is received. Accumulated thoughts are flushed into a `thought` Part, and a `THINKING_END` signal is emitted.

This ensures that the "Thinking process" is clearly delimited for the UI and the local audit vault.

### Thought Signatures

Some providers (like Gemini) emit a `thought_signature`. This is preserved across the turn and included in consolidated response parts to maintain tool-calling context.

## Thinking in the ReAct Loop

The intensity of reasoning is coupled to the agent's role in the **ReAct (Reasoning + Acting)** loop:

- **Surface Triage (`OFF`)**: Triage agents operate on raw input with zero thinking to maximize speed. They don't need to "reason"; they need to "classify" against the incoming posture.
- **Deep Investigation (`HIGH`)**: **Sage** operates at `HIGH` intensity. This is necessary because Sage is processing the `OperatorContext`—a dense bundle of real-time system state, local audit history, and fleet-wide precedents. `HIGH` thinking ensures the model can maintain long-range coherence over this deep grounding while navigating multi-turn tool loops.
- **Consensus Validation (`MEDIUM/HIGH`)**: The **Tribunal** uses elevated thinking to translate intent into shell syntax, ensuring that edge cases (spaces, nulls, locales) are considered before a vote is cast.

## Source of Truth: Model Configuration

`LLMModelConfig` in `app/models/model_configs.py` is the single authoritative source for model capabilities.

- **`supported_thinking_levels`**: The explicit list of supported `ThinkingLevel` values.
- **`thinking_budgets`**: Model-specific token budgets for Anthropic (overrides `ANTHROPIC_DEFAULT_THINKING_BUDGETS`).
- **`thinking_dialect`**: For Ollama, defines how to toggle thinking (`NATIVE_TOGGLE` or `NONE`).
- **`thinking_output_reserve`**: Headroom reserved for visible output when thinking is enabled (defaults to 4,096).

**Unknown Models**: If a model name is not found in `MODEL_REGISTRY`, `UNKNOWN_MODEL_CONFIG` is used. It has an empty `supported_thinking_levels` list, effectively disabling thinking for any unregistered model.

## The Sovereignty Partition

Crucially, **Thinking Levels do not compromise Local-First Audit Architecture (LFAA).**

While we increase the "thinking" intensity, the data being thought about follows the **Scrubbing Protocol**:
1. **Local Context Enrichment**: The Operator assembles the deep context on-host.
2. **Sentinel Scrubbing**: Sensitive PII/secrets are redacted before the context reaches the Engine.
3. **Reasoning Engine**: The Engine applies the Thinking Level to the scrubbed context.
4. **Local Audit**: The model's "thinking process" (where exposed by the provider) is returned to the host and stored in the encrypted Audit Vault.

This ensures that the "Deep Context" is used to drive high-accuracy reasoning without ever exposing the raw, unredacted state to the LLM provider's infrastructure.

## Testing Discipline

- **Translators**: `tests/unit/llm/test_thinking_translators.py` verifies the provider-specific mapping and budget logic.
- **Clamping**: Verified through model config unit tests ensuring `clamp_thinking_level` adheres to the priority rules.
- **Integration**: `tests/conftest.py` contains logic to probe thinking support against live models during integration test runs.

**Never assert against `supports_thinking` directly.** This boolean is a derived convenience property. Always assert against the contents of `supported_thinking_levels`.
