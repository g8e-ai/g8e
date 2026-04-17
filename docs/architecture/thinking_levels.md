# Thinking Levels Abstraction

g8ee addresses four LLM providers (Anthropic, Gemini, OpenAI, Ollama), each of which exposes a different mechanism for enabling extended reasoning ("thinking", "reasoning effort", "extended thinking"). Rather than let per-provider vocabulary leak into the agent layer, g8ee defines a single canonical vocabulary and a per-provider translation layer that maps intent to wire format.

## Canonical Vocabulary

The `ThinkingLevel` enum in `components/g8ee/app/constants/settings.py` defines five values:

| Value   | Semantics                                                        |
|---------|------------------------------------------------------------------|
| `OFF`     | Thinking is disabled for this call. Providers omit all thinking-related request fields. |
| `MINIMAL` | Least-expensive non-zero reasoning. Bounded to a very small token budget. |
| `LOW`     | Light reasoning.                                                |
| `MEDIUM`  | Default reasoning for most primary/tool calls.                   |
| `HIGH`    | Maximum reasoning the model exposes.                             |

`OFF` is a first-class value — never `None`. Callers that want "no thinking" pass `ThinkingLevel.OFF` so intent and schema always agree. Using `None` would force every translator to branch on `is None` and every model config to encode "no-thinking" twice (once as a missing entry, once as a falsy capability flag).

`THINKING_LEVEL_PRIORITY_ASC = (MINIMAL, LOW, MEDIUM, HIGH)` is the only ordering used by clamping logic. `OFF` is excluded — it is not a priority, it is the absence of priority.

## Source of Truth: `LLMModelConfig.supported_thinking_levels`

Each `LLMModelConfig` entry in `components/g8ee/app/models/model_configs.py` declares the exact set of levels a model accepts. The list is authoritative — no boolean flag overrides it.

Two shapes matter:

* **`[]` — no thinking capability.**  
  The model cannot reason, so the translator returns a disabled result for every desired level, and the provider omits every thinking-related field. Ollama Gemma, Ollama Llama, and the generic OpenAI default fall in this category.
* **`[OFF, ...]` — opt-in thinking.**  
  `OFF` is the caller's way to say "skip thinking for this call". Most cloud models (Anthropic, Gemini) use this shape.
* **`[...]` (no `OFF`) — always-on reasoning.**  
  The model has no off switch at the API layer. Callers that request `OFF` are clamped to the model's lowest supported level. This shape is reserved for future always-on reasoning models.

A derived `supports_thinking` read-only `@property` on `LLMModelConfig` is provided for ergonomics (`any non-empty list` == `True`). Do not set it directly — tests and application code must mutate the levels list.

## Clamping

`clamp_thinking_level(desired, config)` in `model_configs.py` resolves the agent's desired level to a level the model actually accepts:

1. Empty support list → `OFF`.
2. `OFF` desired + `OFF` supported → `OFF`.
3. `OFF` desired + always-on model → lowest supported intensity.
4. Non-`OFF` desired → highest supported level `<=` desired.
5. Desired below everything supported → lowest supported level (e.g. `MINIMAL` requested on a `[LOW, MEDIUM, HIGH]` model returns `LOW`).

Every translator delegates to `clamp_thinking_level` first, so agent intent is always clamped to model reality before any wire-format synthesis.

## Per-Provider Translators

`components/g8ee/app/llm/thinking.py` defines one pure translator function per provider. Translators are deterministic, take `(ThinkingLevel, LLMModelConfig, *optional)`, and return a small typed dataclass the provider adapter applies to its outbound request. They must not mutate either input.

### Gemini (`translate_for_gemini`)

Emits `config.thinking_config.thinking_level` as a lowercase string (`"minimal"`, `"low"`, `"medium"`, `"high"`) plus `include_thoughts`. When the level clamps to `OFF` and no thoughts are requested, the provider omits the `thinking_config` key entirely.

### Anthropic (`translate_for_anthropic`)

Emits `thinking = {"type": "enabled", "budget_tokens": N}`. The budget is resolved by:

1. Looking up `config.thinking_budgets[clamped_level]` first (per-model override).
2. Falling back to `ANTHROPIC_DEFAULT_THINKING_BUDGETS[clamped_level]`.

When thinking is enabled the provider also forces `temperature=1.0` and drops `top_k`/`top_p`, matching Anthropic's API contract.

#### max_tokens uplift

Anthropic requires `max_tokens > thinking.budget_tokens`. The provider uplifts a caller-supplied `max_tokens` to `budget + _ANTHROPIC_THINKING_OUTPUT_RESERVE` (currently `4_096`) when the caller's value would leave too little headroom for a visible reply. This prevents the "1-token output" regression that an earlier `min(budget, max_tokens-1)` guard could produce — most visibly for Opus HIGH, whose per-model override (`32_000`) exceeds Opus's default `max_output_tokens` (`8_192`).

### OpenAI (`translate_for_openai`)

Emits `reasoning.effort` as one of `"minimal" | "low" | "medium" | "high"`. When the level clamps to `OFF`, the provider omits the `reasoning` key entirely.

### Ollama (`translate_for_ollama`)

Ollama hosts a heterogeneous zoo of model families. A single `think=True` kwarg does not work uniformly across Qwen, Llama, Gemma, and Nemotron, so the model config declares a `thinking_dialect`:

| Dialect         | Wire Behaviour                                                 |
|-----------------|----------------------------------------------------------------|
| `NONE`          | `think` kwarg is omitted entirely regardless of level.         |
| `NATIVE_TOGGLE` | `think=True` for any non-`OFF` level; `think=False` for `OFF`. |

Primary, assistant, and lite code paths all route through `_apply_think_kwarg` so the dialect dispatch is uniform — no hardcoded `think=False` on any path.

## Request-Builder Contract

`AIGenerationConfigBuilder._build_thinking_config(model_name, desired_level=HIGH, include_thoughts=True)` is the single entry point for constructing a `ThinkingConfig` for a `PrimaryLLMSettings`. It:

1. Looks up the `LLMModelConfig`.
2. Clamps `desired_level` to the model's supported set.
3. Returns `ThinkingConfig(thinking_level=clamped, include_thoughts=include_thoughts_if_supported)`.

`HIGH` is the default because the Proposer and other primary agents want the most capable reasoning the model can offer. Lower-stakes agents (triage, memory updates, title generation) use `get_lite_generation_config` which forces `ThinkingLevel.OFF`.

## Adding a New Model

1. Declare the `LLMModelConfig` in `app/models/model_configs.py`.
2. Set `supported_thinking_levels` to exactly what the model accepts. Leave it `[]` for non-reasoning models.
3. If the provider is Anthropic and the model benefits from non-default budgets, set `thinking_budgets`.
4. If the provider is Ollama, set `thinking_dialect` (`NONE` for non-reasoning families; `NATIVE_TOGGLE` for Qwen/GLM/Nemotron).
5. Register the config in `MODEL_REGISTRY.configs`.

Translators and the request builder pick up the new entry automatically — no provider-adapter code should need changes for standard additions.

## Testing Discipline

* `tests/unit/llm/test_thinking_translators.py` exercises the translator matrix (clamping, per-model budget overrides, dialect dispatch).
* Each provider's unit tests exercise `_build_kwargs` / `_build_genai_config` to confirm that clamped levels reach the SDK boundary correctly.
* Integration probing in `tests/conftest.py` dynamically confirms real-world thinking support against the bound primary model and applies a scoped override to `MODEL_REGISTRY` for the session.

Never assert against `supports_thinking is True/False` directly — assert against the contents of `supported_thinking_levels`. The boolean is a derived view.
