# Thinking

## Overview

The g8e Agentic Ensemble (`g8ee`) integrates LLM provider reasoning capabilities (chain-of-thought, extended thinking tokens, and reasoning effort levels) into agent turn lifecycles. Provider thinking enables deep investigative planning and analysis by reasoning agents such as Sage while isolating internal scratchpad deliberation from durable memory and user-facing summaries. Provider-native thinking outputs, token budgets, and vendor context tokens (such as Gemini thought signatures) are normalized through a canonical vocabulary and pure translation layer before execution.

## Canonical Thinking Abstraction

The ensemble defines a provider-agnostic abstraction for thinking configuration across models and capacity tiers:

- **`ThinkingLevel` (`app.constants.ThinkingLevel`)** — Canonical internal vocabulary representing reasoning intensity: `OFF`, `MINIMAL`, `LOW`, `MEDIUM`, and `HIGH`. `OFF` explicitly disables thinking on models where thinking is opt-in.
- **`ThinkingDialect` (`app.constants.ThinkingDialect`)** — On-the-wire dialect selector for self-hosted models (such as Ollama), supporting `NONE` (omits reasoning parameters) and `NATIVE_TOGGLE` (passes boolean `think` flags).
- **`LLModelConfig` (`app.models.model_configs.LLModelConfig`)** — Immutable model configuration registry defining supported thinking capabilities:
  - `supported_thinking_levels` — List of valid `ThinkingLevel` values. An empty list indicates no thinking support; a list containing `OFF` indicates opt-in thinking; a list omitting `OFF` indicates an always-on reasoning model.
  - `thinking_budgets` — Optional per-model mapping from `ThinkingLevel` to integer token counts for providers requiring explicit token budgets.
  - `thinking_dialect` — Declared wire dialect for self-hosted models.
  - `thinking_output_reserve` — Minimum visible-output headroom (default: 4,096 tokens) reserved above token budgets for providers enforcing total output constraints (such as Anthropic).
- **`clamp_thinking_level` (`app.models.model_configs.clamp_thinking_level`)** — Pure function that resolves a requested `ThinkingLevel` against a target model's `LLModelConfig`. Clamps unsupported levels down to the highest supported intensity, falls back to `OFF` when thinking is unsupported, and selects the lowest supported intensity when `OFF` is requested on always-on reasoning models.

## Provider-Specific Thinking Translators

Provider adapters convert canonical `ThinkingLevel` settings into vendor-specific wire formats using pure translator functions defined in `app.llm.thinking`:

### Gemini 3+ (`translate_for_gemini`)

Returns `GeminiThinkingTranslation(enabled, thinking_level, include_thoughts)`:

- Maps `ThinkingLevel` directly to Gemini SDK level strings (`minimal`, `low`, `medium`, `high`).
- Couples thinking level with the `include_thoughts` boolean toggle.
- When `ThinkingLevel.OFF` is selected, sets `enabled=False`, omitting the `thinking_config` payload from outbound requests.

### Anthropic (`translate_for_anthropic`)

Returns `AnthropicThinkingTranslation(enabled, budget_tokens, level)`:

- Maps `ThinkingLevel` to token counts via model-specific `thinking_budgets` or fallback defaults in `ANTHROPIC_DEFAULT_THINKING_BUDGETS` (`MINIMAL`: 1,024; `LOW`: 2,048; `MEDIUM`: 8,192; `HIGH`: 16,384 tokens).
- Emits `thinking={"type": "enabled", "budget_tokens": N}` and automatically strips `top_k` and `top_p` sampling parameters as required by the Anthropic Messages API.
- Automatically increases `max_tokens` to `budget_tokens + thinking_output_reserve` to ensure adequate capacity for visible response generation.
- When `ThinkingLevel.OFF` is selected, sets `enabled=False`, sets `budget_tokens=0`, omits the `thinking` key, and leaves sampling parameters intact.

### OpenAI (`translate_for_openai`)

Returns `OpenAIThinkingTranslation(enabled, reasoning_effort)`:

- Maps `ThinkingLevel` to the `reasoning.effort` parameter string (`low`, `medium`, `high`).
- When `ThinkingLevel.OFF` is selected, sets `enabled=False` and omits the `reasoning` object from outbound requests.

### Ollama (`translate_for_ollama`)

Returns `OllamaThinkingTranslation(enabled, think)`:

- Evaluates `model_config.thinking_dialect` declared during model registration.
- Under `ThinkingDialect.NATIVE_TOGGLE`, passes `think=True` or `think=False` to `AsyncClient.chat()`.
- Under `ThinkingDialect.NONE`, sets `think=None`, omitting thinking parameters from the API call.

## Thought Signatures

Gemini 3 and 2.5 models emit opaque, encrypted context tokens (`thought_signature`) on thinking-enabled responses, particularly when generating tool calls:

- **Canonical Representation** — `ThoughtSignature` (`app.llm.llm_dataclasses.ThoughtSignature`) encapsulates the provider signature as an immutable dataclass, normalizing raw SDK byte buffers and strings into base64-encoded strings via `ThoughtSignature.from_sdk()`.
- **Multi-Turn Tool Calling Contract** — The Gemini API requires that when a model response contains a tool call accompanied by a thought signature, subsequent requests containing tool execution results must echo that exact signature on the corresponding tool call `Part` in conversation history. Omitting the signature produces an HTTP 400 Bad Request error.
- **Structural Invariants** — Signed parts cannot be merged with unsigned parts or other signed parts. Outbound adapters (`app.llm.providers.gemini._content_to_genai`) convert signature-only parts to empty-text parts to preserve context without injecting spurious text payloads.

## Stream Processing and Turn Lifecycle

Thinking tokens and thought signatures are processed during streaming execution in `app.services.ai.agent_turn`:

- **Turn State Machine** — `TurnState` tracks streaming state transitions across three phases:
  ```
  INACTIVE → (thought chunk) → ACTIVE → (text or tool call chunk) → INACTIVE
  ```
- **Chunk Handling** — `handle_thought_chunk` captures incoming thinking text into `thinking_text_parts` and records any `thought_signature`. When the stream transitions to plain text or tool invocations, `flush_thinking_block` packages accumulated text into a `Part(text=..., thought=True, thought_signature=...)`.
- **SSE Event Streaming** — `agent_sse.py` translates stream chunk transitions into Server-Sent Events for real-time frontend rendering:
  - `g8e.v1.ai.llm.chat.iteration.thinking.started` (`ThinkingActionType.START`) — Initial thinking chunk received.
  - `g8e.v1.ai.llm.chat.iteration.thinking.update` (`ThinkingActionType.UPDATE`) — Incremental thinking text received.
  - `g8e.v1.ai.llm.chat.iteration.thinking.end` (`ThinkingActionType.END`) — Thinking phase completed prior to text or tool execution.

## Scratchpad Isolation and Memory Sanitization

Thinking content represents intermediate model scratchpad deliberation and is strictly isolated from durable system state:

- **Memory Extraction Filtering** — `MemoryGenerationService` filters out messages containing thinking blocks (`is_thinking=True` or `thought=True`) when analyzing conversations for long-term user preferences and investigation insights.
- **Context Hygiene** — Thinking tokens are separated from user-facing answer generation, preventing unverified chain-of-thought assumptions or raw internal syntax from polluting case histories or executive summaries.

## Related

- [Architecture](architecture.md) — System architecture, protocol surfaces, and model hierarchy
- [LLM Providers](llm-providers.md) — Provider implementations, capacity tiers, and error translation
- [Agents](agents.md) — Agent persona definitions, capacity tiers, and Tribunal consensus
- [Prompts](prompts.md) — System prompt assembly and persona templating
- [Governance](governance.md) — Five-layer verification pipeline and envelope validation
- [Constants](constants.md) — Sourced protocol constants and application definitions
- [Evals](evals.md) — Benchmark evaluation suite and Judge scoring rubrics
