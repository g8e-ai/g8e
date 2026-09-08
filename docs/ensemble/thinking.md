# Thinking

## Overview

The g8e Agentic Ensemble (`g8ee`) normalizes provider reasoning controls and reasoning output across Gemini, Anthropic, OpenAI-compatible services, and Ollama. A canonical thinking level selects the closest reasoning mode supported by the configured model, while each provider adapter translates that selection into its native request format. During a streamed agent turn, g8ee separates thought content from visible answer text, preserves provider context required by tool-call protocols, and reports the reasoning lifecycle to clients.

The main implementation entry points are `ensemble/app/llm/thinking.py`, `ensemble/app/models/model_configs.py`, and `ensemble/app/services/ai/agent_turn.py`.

## When Thinking Runs

Thinking configuration is part of the primary generation call shape. The main chat agent uses this call shape for both simple and complex turns because either path can enter the tool loop. Primary settings request `high` reasoning and provider-returned thought content by default, then reduce that request to the selected model's supported level.

Assistant and lite generation call shapes do not accept thinking configuration. Triage, memory extraction, title generation, risk analysis, and other concise or structured helper calls therefore run without requested reasoning output. A provider can still behave outside its declared contract, but g8ee does not ask these call shapes to return thoughts.

## Canonical Levels and Model Profiles

`ThinkingLevel` defines five values: `off`, `minimal`, `low`, `medium`, and `high`. Each registered model declares its accepted values through `supported_thinking_levels`:

- An empty list means that g8ee does not request thinking from the model.
- A list containing `off` describes an opt-in model whose thinking can be disabled.
- A list without `off` describes an always-on reasoning model. No built-in model profile currently uses this form.

The level resolver returns the highest supported intensity at or below the requested intensity. If the request is lower than every supported intensity, it returns the model's lowest supported intensity. It resolves models without declared thinking support to `off`; for an always-on profile, an `off` request resolves to the lowest supported intensity.

Unknown model names use a shared conservative profile with thinking disabled. Custom or newly released models require a registered profile before g8ee requests reasoning from them. See [LLM Providers](llm-providers.md#model-capability-registry) for the current model list and supported levels.

## Provider Translation

Provider adapters translate only the resolved level. They omit unsupported reasoning fields rather than sending a level that the model profile does not declare.

### Gemini

Gemini receives `thinking_level` with `include_thoughts`. The level is one of `minimal`, `low`, `medium`, or `high`; the current Gemini Flash Lite profile is the only registered Gemini profile that accepts `minimal`. When the resolved level is `off`, g8ee omits the thinking configuration and does not request thought content.

### Anthropic

Anthropic receives extended thinking with an integer token budget. The default budgets are 1,024 tokens for `minimal`, 2,048 for `low`, 8,192 for `medium`, and 16,384 for `high`. The Claude Opus profile overrides the supported budgets to 4,096 for `low`, 16,384 for `medium`, and 32,000 for `high`.

While extended thinking is active, the adapter omits `top_k`. It raises `max_tokens` when necessary so the total output limit is at least the thinking budget plus the model's visible-output reserve. The default reserve is 4,096 tokens, while the Claude Opus profile uses 8,192. When the resolved level is `off`, the adapter omits the thinking object, preserves `top_k`, and does not raise `max_tokens` for reasoning.

### OpenAI

OpenAI receives `reasoning.effort` with the resolved `minimal`, `low`, `medium`, or `high` value. The current registered reasoning profile, `gpt-5.4-mini`, accepts only `minimal` and `low` in addition to `off`. When the resolved level is `off`, g8ee omits the reasoning object.

### Ollama

Every registered Ollama profile declares a thinking dialect. Native-toggle models receive `think=true` when thinking is enabled and `think=false` when it is disabled. Profiles with the `none` dialect receive no `think` parameter. Current native-toggle profiles expose a binary `off` or `high` capability, so lower nonzero requests resolve to `high`.

llama.cpp inherits the OpenAI-compatible translation and can send `reasoning.effort` when the selected model has a registered thinking profile. No llama.cpp-specific model profiles are registered, so unknown llama.cpp model names resolve thinking to `off`. The fake provider accepts primary settings for tests but does not emit thought content.

## Reasoning Output and Tool Context

Gemini, Anthropic, OpenAI, and Ollama adapters normalize provider reasoning output into thought parts. Gemini reads parts marked as thoughts, Anthropic reads thinking blocks, OpenAI reads `reasoning_content`, and Ollama reads the message's `thinking` field. The stream processor emits thought text separately from visible text and accumulates each contiguous thought block for the current model turn.

The turn lifecycle follows these transitions:

- The first thought chunk enters the active thinking phase.
- Additional thought chunks extend the current block.
- Visible text or a tool call ends the active phase before that output is processed.
- End of stream also closes an active thinking phase.

The completed thought block remains in the model response parts used by the in-memory ReAct tool loop. This allows the next provider request to retain reasoning context and any required signatures. Visible response assembly and interrogation detection read only non-thought text.

## Thought Signatures

Gemini and Anthropic can return opaque signatures with thinking output. These values are provider context tokens, not signatures that g8ee verifies cryptographically. g8ee preserves them so later tool-result turns can satisfy the originating provider's conversation protocol.

Gemini byte signatures are normalized to base64 strings at the provider boundary. A signed part remains a separate part during consolidation because merging it with unsigned content or another signed part would change the provider's required history structure. Signature-only Gemini parts become empty-text parts on the outbound request so the signature remains in context without adding visible text.

Anthropic thinking-block signatures remain attached to the corresponding thinking blocks when conversation history is converted back to the Messages API format.

## Streaming Events

The stream processor emits internal `THINKING` chunks for thought text and one `THINKING_END` chunk whenever it leaves an active thinking phase. The event layer publishes all three lifecycle actions under `g8e.v1.ai.llm.chat.iteration.thinking.started` using `ChatThinkingPayload.action_type`:

- `start` carries the first thought chunk in a phase.
- `update` carries each later thought chunk in that phase.
- `end` carries no thought text and marks the transition to visible text, tool execution, or stream completion.

The protocol registry also defines `thinking.update` and `thinking.end` event identifiers, but the current ensemble runtime does not publish them.

## Memory and Telemetry

Durable memory generation skips conversation messages marked `is_thinking`. This filtering prevents a thinking-only message from becoming a user preference or investigation summary. The filter does not remove thought parts from the turn-local provider history, where they can be required for multi-turn tool calling.

The agent records provider-reported input, output, cache, total, and thinking token counts in model-call telemetry and aggregates them into the completed response metadata. Gemini supplies a separate thought-token count. The current Anthropic, OpenAI, and Ollama adapters do not populate a separate thinking-token field, so `thinking_tokens` remains zero for those calls even when their output includes reasoning.

## Related

- [Ensemble Architecture](../architecture/ensemble.md): Platform-level summary of g8ee's role
- [Architecture](architecture.md): System architecture, protocol surfaces, and model hierarchy
- [LLM Providers](llm-providers.md): Provider implementations, model roles, and capability profiles
- [Agents](agents.md): Agent persona definitions, model tiers, and Tribunal consensus
- [Prompts](prompts.md): System prompt assembly and persona templating
- [Governance](governance.md): Five-layer verification pipeline and envelope validation
- [Evals](evals.md): Benchmark evaluation suite and Judge scoring rubrics
