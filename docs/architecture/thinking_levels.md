---
title: Thinking Levels
parent: Architecture
---

# Thinking Levels

g8ee unifies extended reasoning across four LLM providers (Anthropic, Gemini, OpenAI, Ollama) behind a single abstraction. Each provider exposes "thinking" differently—Gemini uses `thinking_config`, OpenAI uses `reasoning.effort`, Anthropic uses token budgets, Ollama uses a boolean flag. Rather than let this provider-specific vocabulary leak into the agent layer, g8e defines a canonical vocabulary and a per-provider translation layer.

## Why This Abstraction Exists

**Provider heterogeneity is a leaky abstraction problem.** Without a canonical layer, every agent would need to know that "high reasoning" on Anthropic means a 16K token budget, on OpenAI it means `reasoning.effort="high"`, and on Ollama it means `think=True` (for some models) or nothing at all (for others). This makes agents brittle to provider changes and impossible to reason about.

**The abstraction boundary is intentional.** The agent layer speaks in terms of reasoning intensity (OFF through HIGH). The provider layer translates that intensity into whatever wire format the provider expects. This separation allows:
- Adding new providers without touching agent code
- Swapping models within a provider without changing agent intent
- Testing agent behavior independently of provider mechanics

## Canonical Vocabulary

The `ThinkingLevel` enum defines five values representing reasoning intensity:

| Value   | Semantics                                                        |
|---------|------------------------------------------------------------------|
| `OFF`     | Thinking disabled. Providers omit all thinking-related fields.  |
| `MINIMAL` | Least-expensive non-zero reasoning. Small token budget.          |
| `LOW`     | Light reasoning.                                                 |
| `MEDIUM`  | Default reasoning for most primary/tool calls.                  |
| `HIGH`    | Maximum reasoning the model exposes.                            |

**`OFF` is a first-class value, never `None`.** Using `OFF` explicitly makes intent and schema agree. Using `None` would force every translator to branch on `is None` and every model config to encode "no-thinking" twice (once as a missing entry, once as a falsy capability flag).

**Priority ordering is explicit.** `THINKING_LEVEL_PRIORITY_ASC = (MINIMAL, LOW, MEDIUM, HIGH)` defines the only ordering used by clamping logic. `OFF` is excluded—it is not a priority, it is the absence of priority.

## The Lifecycle: From Agent Intent to Wire Format

### 1. Agent Declares Desired Level

Agents specify a `ThinkingLevel` when requesting generation. Primary agents (Proposer, Tribunal) default to `HIGH` because they want maximum reasoning. Lightweight agents (triage, title generation) explicitly use `OFF` to avoid latency and cost.

### 2. Request Builder Clamps to Model Reality

`AIGenerationConfigBuilder._build_thinking_config()` looks up the `LLMModelConfig` for the bound model and calls `clamp_thinking_level(desired, config)`. This function resolves the agent's desired level to what the model actually accepts:

- Empty support list → `OFF` (model cannot reason)
- `OFF` desired + `OFF` supported → `OFF`
- `OFF` desired + always-on model → lowest supported intensity
- Non-`OFF` desired → highest supported level ≤ desired
- Desired below everything supported → lowest supported level

**Clamping happens before any wire-format synthesis.** This ensures agent intent is always adjusted to model reality before translation.

### 3. Provider Translator Converts to Wire Format

Each provider has a pure translator function in `app/llm/thinking.py` that takes the clamped level and model config, then returns a typed result the provider adapter applies to its outbound request:

- **Gemini**: Emits `thinking_config.thinking_level` (string) + `include_thoughts`. Omits the key entirely when `OFF`.
- **Anthropic**: Emits `thinking={"type": "enabled", "budget_tokens": N}`. Budget comes from per-model override or default table. Drops `top_k`/`top_p` when enabled (API contract).
- **OpenAI**: Emits `reasoning.effort` (string). Omits the key when `OFF`.
- **Ollama**: Emits `think=True/False` based on `thinking_dialect`. `NONE` dialect omits the kwarg entirely; `NATIVE_TOGGLE` uses the boolean.

### 4. Provider Adapter Applies Translation

The provider adapter (e.g., `anthropic.py`, `gemini.py`) takes the translator result and applies it to the SDK request. For Anthropic, this includes a `max_tokens` uplift to ensure `max_tokens > budget_tokens`, preventing "1-token output" regressions when the thinking budget exceeds the default output limit.

## Source of Truth: Model Configuration

`LLMModelConfig.supported_thinking_levels` is the single authoritative source for what a model accepts. Three shapes are possible:

- **`[]`** — No thinking capability. The translator returns disabled for every level. Examples: Ollama Llama, generic OpenAI default.
- **`[OFF, ...]`** — Opt-in thinking. `OFF` disables it. Most cloud models and reasoning-capable self-hosted models use this.
- **`[...]` (no `OFF`)** — Always-on reasoning. Reserved for future models that cannot disable thinking at the API layer.

**No boolean flag overrides this list.** The derived `supports_thinking` property is a convenience view, not the source of truth.

## Adding a New Model

1. Declare the `LLMModelConfig` in `app/models/model_configs.py`
2. Set `supported_thinking_levels` to exactly what the model accepts (empty list for non-reasoning models)
3. For Anthropic models with non-default budget needs, set `thinking_budgets`
4. For Ollama models, set `thinking_dialect` (`NONE` for non-reasoning families, `NATIVE_TOGGLE` for reasoning-capable families)
5. Register the config in `MODEL_REGISTRY.configs`

Translators and the request builder pick up the new entry automatically—no provider-adapter code changes are needed for standard additions.

## Testing Discipline

- `tests/unit/llm/test_thinking_translators.py` exercises the translator matrix (clamping, per-model budget overrides, dialect dispatch)
- Each provider's unit tests confirm clamped levels reach the SDK boundary correctly
- Integration probing in `tests/conftest.py` dynamically confirms real-world thinking support against the bound primary model

**Never assert against `supports_thinking` directly.** Assert against the contents of `supported_thinking_levels`—the boolean is a derived view.
