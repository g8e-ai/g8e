---
title: Thinking Levels
parent: Architecture
---

# Thinking Levels

g8ee unifies extended reasoning across four LLM providers (Anthropic, Gemini, OpenAI, Ollama) behind a single abstraction. Each provider exposes "thinking" differently—Gemini uses `thinking_config`, OpenAI uses `reasoning.effort`, Anthropic uses token budgets, and Ollama uses a boolean flag. Rather than letting this provider-specific vocabulary leak into the agent layer, g8e defines a canonical vocabulary and a per-provider translation layer.

## Why This Abstraction Exists

**Provider heterogeneity is a leaky abstraction problem.** Without a canonical layer, every agent would need to know that "high reasoning" on Anthropic means a 16K token budget, on OpenAI it means `reasoning.effort="high"`, and on Ollama it means `think=True`. This makes agents brittle to provider changes and impossible to reason about.

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

Agents specify a `ThinkingLevel` when requesting generation. Primary agents (Proposer, Tribunal) default to `HIGH` via `AIGenerationConfigBuilder._build_thinking_config()`. Lightweight agents (triage, title generation) explicitly use `OFF` to minimize latency and cost.

### 2. Request Builder Clamps to Model Reality

`AIGenerationConfigBuilder` looks up the `LLMModelConfig` for the bound model and calls `clamp_thinking_level(desired, config)`. This function resolves the agent's desired level based on the model's `supported_thinking_levels`:

- **No Support**: If `supported_thinking_levels` is empty, returns `OFF`.
- **Opt-in (`OFF` in list)**: Returns `OFF` if desired; otherwise returns highest supported level ≤ desired.
- **Always-on (no `OFF` in list)**: If `OFF` is desired, returns the lowest supported level (e.g., `MINIMAL`).
- **Fallback**: If the desired level is below everything supported, returns the lowest supported level.

**Clamping happens before any wire-format synthesis.** This ensures agent intent is always adjusted to model reality before translation.

### 3. Provider Translator Converts to Wire Format

Each provider has a pure translator function in `app/llm/thinking.py` that takes the clamped level and model config, then returns a typed result the provider adapter applies to its outbound request:

- **Gemini**: Returns a `GeminiThinkingTranslation` with `thinking_level` string (e.g., `"high"`).
- **Anthropic**: Returns an `AnthropicThinkingTranslation` with a `budget_tokens` integer derived from `thinking_budgets` or defaults.
- **OpenAI**: Returns an `OpenAIThinkingTranslation` with `reasoning_effort` string (e.g., `"medium"`).
- **Ollama**: Returns an `OllamaThinkingTranslation` with a `think` boolean, dispatched by the model's `thinking_dialect`.

### 4. Provider Adapter Applies Translation

The adapter (e.g., `app/llm/providers/anthropic.py`) applies the translation to the SDK request. 

**The Output Reserve Uplift**: For Anthropic, which requires `max_tokens > budget_tokens`, the adapter uplifts `max_tokens` to `budget_tokens + thinking_output_reserve`. This ensures the model has enough headroom to provide a complete response after its internal reasoning phase.

## Source of Truth: Model Configuration

`LLMModelConfig` in `app/models/model_configs.py` is the single authoritative source for model capabilities.

- **`supported_thinking_levels`**: The explicit list of supported `ThinkingLevel` values.
- **`thinking_budgets`**: Model-specific token budgets for Anthropic (overrides `ANTHROPIC_DEFAULT_THINKING_BUDGETS`).
- **`thinking_dialect`**: For Ollama, defines how to toggle thinking (`NATIVE_TOGGLE` or `NONE`).
- **`thinking_output_reserve`**: Headroom reserved for visible output when thinking is enabled (defaults to 4,096).

**Unknown Models**: If a model name is not found in `MODEL_REGISTRY`, `UNKNOWN_MODEL_CONFIG` is used. It has an empty `supported_thinking_levels` list, effectively disabling thinking for any unregistered model.

## Adding a New Model

1. **Define Config**: Add a `LLMModelConfig` entry in `app/models/model_configs.py`.
2. **Set Levels**: Populate `supported_thinking_levels` with exactly what the model supports. Include `OFF` if the API allows disabling it.
3. **Set Budgets/Dialects**: Define `thinking_budgets` for Anthropic or `thinking_dialect` for Ollama.
4. **Register**: Add the config to `MODEL_REGISTRY.configs` and (for Ollama) `_OLLAMA_CONFIGS`.

Translators and the request builder pick up the new entry automatically—no provider-adapter code changes are needed for standard additions.

## Testing Discipline

- **Translators**: `tests/unit/llm/test_thinking_translators.py` verifies the provider-specific mapping and budget logic.
- **Clamping**: Verified through model config unit tests ensuring `clamp_thinking_level` adheres to the priority rules.
- **Integration**: `tests/conftest.py` contains logic to probe thinking support against live models during integration test runs.

**Never assert against `supports_thinking` directly.** This boolean is a derived convenience property. Always assert against the contents of `supported_thinking_levels`.
