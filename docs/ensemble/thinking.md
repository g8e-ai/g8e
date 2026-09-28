---
doc_id: ensemble_thinking
title: Agentic Ensemble Thinking
audience: platform and feature developers, coding agents
status: current
last_updated: 2026-09-28
version: v2.2.3
owners:
  - ensemble/app/llm/thinking.py
  - ensemble/app/models/model_configs.py
  - ensemble/app/services/ai/generation_config_builder.py
  - ensemble/app/services/ai/agent_turn.py
  - ensemble/app/llm/providers/
related:
  - docs/ensemble/llm-providers.md
  - docs/ensemble/agents.md
  - docs/ensemble/governance.md
  - docs/ensemble/architecture.md
  - docs/ensemble/sse.md
  - docs/ensemble/protocol.md
when_to_read: Understanding how the ensemble handles model reasoning, implementing provider-specific thinking behavior, configuring thinking levels, or debugging streaming thought events.
do_not_use_for:
  - LLM provider capability registry (docs/ensemble/llm-providers.md)
  - Agent personas and routing (docs/ensemble/agents.md)
  - Governance enforcement and authorization (docs/ensemble/governance.md)
  - Protocol surfaces and event structures (docs/ensemble/protocol.md)
---

# Agentic Ensemble Thinking

## Purpose

The g8e Agentic Ensemble (`g8ee`) represents provider-specific reasoning using a canonical `ThinkingLevel` and translates it at the provider boundary during generation request assembly. The supported levels are `off`, `minimal`, `low`, `medium`, and `high`. During a streamed agent turn, g8ee normalizes provider reasoning into thought parts, emits thought lifecycle events separately from visible text, and preserves provider context required by multi-turn tool execution.

Thinking is an application-level model capability. It does not authorize a governed operation, substitute for protocol L2 consensus or L3 notary authorization, or attest to provider behavior outside the g8ee path. See [Governance](governance.md) and [Architecture: AI Agents and the g8e Governance Boundary](../architecture/agents.md).

## Quick index

- [Purpose](#purpose)
- [Quick index](#quick-index)
- [Invariants](#invariants)
- [Owned surfaces](#owned-surfaces)
- [When Thinking Runs](#when-thinking-runs)
- [Canonical Levels and Model Profiles](#canonical-levels-and-model-profiles)
- [Provider Translation](#provider-translation)
- [Reasoning Output and Tool Context](#reasoning-output-and-tool-context)
- [Thought Signatures](#thought-signatures)
- [Streaming Events](#streaming-events)
- [Memory and Telemetry](#memory-and-telemetry)
- [Procedures](#procedures)
- [Anti-patterns](#anti-patterns)
- [Links out](#links-out)

## Invariants

Ids are stable. Append the next free number in each group; do not renumber.

### Thinking Configuration and Behavior (`INV-ENS-THINK-CONFIG`)

| ID | Rule |
| --- | --- |
| INV-ENS-THINK-CONFIG-01 | `LLModelConfig.supported_thinking_levels` is the canonical source of truth for each model's reasoning capability. Providers MUST NOT accept thinking requests for levels not in this list. |
| INV-ENS-THINK-CONFIG-02 | `clamp_thinking_level()` MUST resolve any requested level to the highest supported intensity at or below it. If the request is lower than all supported levels, it returns the lowest supported level. An `off` request returns `off` when supported; for always-on models it returns the lowest supported intensity. |
| INV-ENS-THINK-CONFIG-03 | Primary generation calls MUST carry `ThinkingConfig` built by `_build_thinking_config()` with a default request of `high` and `include_thoughts=true`. Assistant and lite call shapes MUST NOT carry thinking configuration. |
| INV-ENS-THINK-CONFIG-04 | Provider adapters MUST omit all reasoning-related request fields (thinking_config, reasoning, think parameter, etc.) when the resolved level is `off`. Sending unsupported level names is prohibited. |

### Streaming and State Management (`INV-ENS-THINK-STREAM`)

| ID | Rule |
| --- | --- |
| INV-ENS-THINK-STREAM-01 | Thought chunks MUST transition the turn state from INACTIVE → ACTIVE on first chunk, emit `THINKING`, and emit `THINKING_END` on transition to visible text, tool execution, or stream end. |
| INV-ENS-THINK-STREAM-02 | Thought parts MUST be preserved in the in-memory ReAct loop history to satisfy provider multi-turn tool-call protocol requirements. Consolidation MUST NOT merge thought parts with non-thought content. |
| INV-ENS-THINK-STREAM-03 | Durable memory generation MUST skip conversation messages marked `is_thinking`. This filter does not apply to turn-local provider history. |

### Provider Translation (`INV-ENS-THINK-PROVIDER`)

| ID | Rule |
| --- | --- |
| INV-ENS-THINK-PROVIDER-01 | Provider translators in `ensemble/app/llm/thinking.py` are pure functions returning typed translation results. Each provider adapter interprets the result according to its own wire protocol. |
| INV-ENS-THINK-PROVIDER-02 | Gemini adapters MUST send `thinking_config.thinking_level` (string enum) and `include_thoughts` boolean. Thought byte signatures MUST be normalized to base64 strings. |
| INV-ENS-THINK-PROVIDER-03 | Anthropic adapters MUST send thinking as `{"type": "enabled", "budget_tokens": N}` when enabled. When thinking is active, adapters MUST omit `top_k` and `top_p`. Max tokens MUST be uplifted by `thinking_output_reserve` to preserve visible-output headroom. |
| INV-ENS-THINK-PROVIDER-04 | OpenAI-compatible adapters MUST send `reasoning.effort` with the resolved level string. When off, the `reasoning` object MUST be omitted. |
| INV-ENS-THINK-PROVIDER-05 | Ollama adapters MUST respect the model's `thinking_dialect`. Native-toggle models send `think=true` when enabled, `think=false` when off. Models with dialect `none` MUST NOT send a `think` parameter. |

## Owned surfaces

| Claim | Path | Verify |
| --- | --- | --- |
| Thinking translators | `ensemble/app/llm/thinking.py` | Provider-specific functions return typed translation results |
| Model capability registry | `ensemble/app/models/model_configs.py` | `LLModelConfig` defines `supported_thinking_levels` for each model |
| Clamping logic | `ensemble/app/models/model_configs.py` line 451 | `clamp_thinking_level()` resolves requests to supported levels |
| ThinkingConfig builder | `ensemble/app/services/ai/generation_config_builder.py` | `_build_thinking_config()` produces configuration for primary calls |
| Stream state machine | `ensemble/app/services/ai/agent_turn.py` | `TurnState` and `_handle_*_chunk()` functions maintain INACTIVE ↔ ACTIVE transitions |
| Provider adapters | `ensemble/app/llm/providers/*.py` | Gemini, Anthropic, OpenAI, Ollama, G8E, and Fake implementations apply translations |

## When Thinking Runs

The primary generation call shape carries `ThinkingConfig`, tools, and system instructions. The production generation builder in [ensemble/app/services/ai/generation_config_builder.py](../../ensemble/app/services/ai/generation_config_builder.py) requests `high` with `include_thoughts=true` for primary calls, then clamps that request to the selected model's registered capability via [clamp_thinking_level()](../../ensemble/app/models/model_configs.py#L451). This call shape is used for both complex and simple chat turns because either route can enter the tool loop; a simple turn selects the assistant model while still using the primary call shape.

Assistant and lite call shapes do not carry thinking configuration. Triage, Tribunal generation, risk analysis, title generation, memory extraction, response analysis, and other assistant or lite helper calls are built with [build_assistant_settings()](../../ensemble/app/services/ai/generation_config_builder.py#L169) or [build_lite_settings()](../../ensemble/app/services/ai/generation_config_builder.py#L195), which omit `thinking_config`. The provider may return unexpected thinking fields, but g8ee does not enable thinking for these call shapes. Ollama assistant and lite requests explicitly send `think=false` for native-toggle models and omit the parameter for models whose dialect is `none`.

## Canonical Levels and Model Profiles

[LLModelConfig.supported_thinking_levels](../../ensemble/app/models/model_configs.py#L79) is the capability source of truth:

- An empty list means the model has no registered thinking capability and resolves every request to `off`.
- A list containing `off` describes a model whose reasoning can be disabled.
- A list without `off` describes an always-on reasoning model. No built-in model profile uses this form.

[clamp_thinking_level()](../../ensemble/app/models/model_configs.py#L451) resolves a requested level to the highest supported intensity at or below it. If the request is lower than every supported intensity, it returns the model's lowest supported intensity. An `off` request returns `off` when the model supports it; for an always-on profile it returns the lowest supported intensity.

The production primary builder always requests `high`; the other levels are used by the canonical translation API and capability-specific callers. Unknown model names resolve to [UNKNOWN_MODEL_CONFIG](../../ensemble/app/models/model_configs.py#L496), which disables thinking, tools, and provider-enforced structured-output decisions. A custom model must have a registered profile before g8ee relies on its reasoning capability. See [LLM Providers: Model Capability Registry](llm-providers.md) for the current registry.

## Provider Translation

Provider translators in [ensemble/app/llm/thinking.py](../../ensemble/app/llm/thinking.py) are pure functions of a requested level and an immutable model configuration. They return typed translation results; the provider adapter applies the result to its outbound request. Providers omit reasoning fields when the resolved level is `off` rather than sending an unsupported level.

### Gemini

Gemini receives `thinking_config.thinking_level` and `include_thoughts` via [translate_for_gemini()](../../ensemble/app/llm/thinking.py#L56). The registered Gemini profiles support `off`, `low`, `medium`, and `high`; `gemini-3.1-flash-lite` also supports `minimal`. When the resolved level is `off`, the adapter omits `thinking_config` and disables thought output. Gemini thought parts may carry opaque thought signatures, which the Gemini adapter normalizes to base64 strings.

### Anthropic

Anthropic receives extended thinking with a token budget instead of a level name via [translate_for_anthropic()](../../ensemble/app/llm/thinking.py#L97). The default budgets in [ANTHROPIC_DEFAULT_THINKING_BUDGETS](../../ensemble/app/models/model_configs.py#L108) are 1,024 tokens for `minimal`, 2,048 for `low`, 8,192 for `medium`, and 16,384 for `high`. The Claude Opus profile overrides the supported budgets to 4,096 for `low`, 16,384 for `medium`, and 32,000 for `high`.

When extended thinking is active, the adapter omits `top_k` and `top_p`. It raises `max_tokens` when necessary so the total output limit includes the thinking budget plus visible-output headroom: 4,096 tokens by default or 8,192 for Claude Opus (via [thinking_output_reserve](../../ensemble/app/models/model_configs.py#L101)). When thinking is off, it omits the thinking object and preserves the sampling parameters. Anthropic thinking blocks can carry opaque signatures, which remain attached when history is converted back to the Messages API format.

### OpenAI-compatible providers

OpenAI receives `reasoning.effort` with the resolved `minimal`, `low`, `medium`, or `high` value via [translate_for_openai()](../../ensemble/app/llm/thinking.py#L139). The registered `gpt-5.4-mini` profile supports `off`, `minimal`, and `low`; the generic OpenAI profile has no declared thinking capability and therefore resolves to `off`. When thinking is off, the adapter omits the `reasoning` object.

llama.cpp inherits the OpenAI-compatible adapter, so it uses the same translation and can return `reasoning_content`. No llama.cpp-specific model profiles are registered; its model names therefore resolve to [UNKNOWN_MODEL_CONFIG](../../ensemble/app/models/model_configs.py#L496) unless a matching model is registered.

### Ollama

Each registered Ollama profile declares a thinking dialect via [thinking_dialect](../../ensemble/app/models/model_configs.py#L93). Native-toggle models receive `think=true` when the resolved level is enabled and `think=false` when it is off via [translate_for_ollama()](../../ensemble/app/llm/thinking.py#L171). Profiles with the `none` dialect receive no `think` parameter. Current native-toggle profiles expose binary `off` or `high` capability, so a lower nonzero request resolves to `high`. The adapter reads reasoning from the response message's `thinking` field.

### Governed `g8e` provider

The `g8e` provider sends inference through the Gateway's governed inference-dispatch endpoint. Its `InferenceThinkingControl` is derived from the selected model's registered Ollama thinking dialect and carries an enabled flag plus `include_thoughts`. The response adapter converts protocol thinking parts into canonical thought parts and maps the dispatch response's optional thinking-token count into `UsageMetadata`. The Gateway and Inference Operator govern the inference request separately from g8ee's model reasoning; see [LLM Providers: Supported Providers](llm-providers.md).

### Fake provider

The fake provider accepts primary settings for tests and scenarios, but its deterministic responses do not emit thought content. It does not cross a provider network boundary.

## Reasoning Output and Tool Context

[gemini.py](../../ensemble/app/llm/providers/gemini.py), [anthropic.py](../../ensemble/app/llm/providers/anthropic.py), [open_ai.py](../../ensemble/app/llm/providers/open_ai.py), [ollama.py](../../ensemble/app/llm/providers/ollama.py), and [g8e.py](../../ensemble/app/llm/providers/g8e.py) in [ensemble/app/llm/providers/](../../ensemble/app/llm/providers/) normalize parts marked as thoughts, thinking blocks, `reasoning_content`, message `thinking` fields, and protocol thinking parts. These adapters expose the result as canonical `Part(thought=True, ...)` values or streamed `StreamChunkFromModel` values.

For one provider stream, the turn processor in [ensemble/app/services/ai/agent_turn.py](../../ensemble/app/services/ai/agent_turn.py) maintains this state machine:

```
INACTIVE -> thought chunk -> ACTIVE -> visible text or tool call -> INACTIVE
```

The first thought chunk starts a phase and is emitted as a `THINKING` chunk. Additional thought chunks extend the phase. Visible text or a tool call closes the phase before that output is processed. End of stream also closes an active phase. `THINKING_END` is emitted once for each transition out of the active phase. Implementations of this state machine appear in [_handle_thought_chunk()](../../ensemble/app/services/ai/agent_turn.py#L128) and [_handle_text_chunk()](../../ensemble/app/services/ai/agent_turn.py#L146).

The completed thought block remains in the model response parts used by the in-memory ReAct loop. This preserves reasoning context and provider-required signatures for the next request in a tool turn. Consolidation merges only adjacent plain, unsigned, non-thought text parts; it preserves thought parts, tool calls, and signature-bearing parts in order. Visible response assembly and interrogation detection read only non-thought text.

## Thought Signatures

Thought signatures are opaque provider context values, not cryptographic signatures that g8ee verifies. g8ee preserves them only to satisfy the originating provider's conversation or tool-call protocol.

Gemini inbound byte signatures are normalized to base64 strings. A part with a signature is never merged with unsigned content or another signed part. Signature-only Gemini parts are sent back as empty-text parts so the signature remains in provider history without adding visible text. Gemini tool-call parts retain their required signature, and signatures on other returned parts remain attached.

Anthropic signatures remain attached to their corresponding thinking blocks when conversation history is converted back to the Messages API format. OpenAI-compatible, Ollama, fake, and governed `g8e` response adapters do not add a separate thought-signature field to their canonical response parts.

## Streaming Events

The stream processor emits internal `THINKING` chunks for thought text and a `THINKING_END` chunk when a phase ends. The SSE layer publishes all thinking lifecycle actions as `g8e.v1.ai.llm.chat.iteration.thinking.started` with `ChatThinkingPayload.phase`:

- `start` — first thought chunk in a phase.
- `update` — each later thought chunk in that phase.
- `end` — phase transition to visible text, tool execution, or stream completion.

These events are session-targeted delivery telemetry. They are not governance state, authorization, or durable execution evidence.

## Memory and Telemetry

Durable memory generation in [ensemble/app/services/ai/memory_service.py](../../ensemble/app/services/ai/) skips conversation messages marked `is_thinking`. This prevents a thinking-only message from becoming a user preference or investigation summary. The filter does not remove thought parts from the turn-local provider history, where they can be required for multi-turn tool calling.

The agent accumulates provider-reported input, output, cache, total, and thinking token counts in turn results and includes them in model-call telemetry and completed response metadata. Gemini supplies a separate thought-token count, and the governed `g8e` provider can propagate one from the inference response. The Anthropic, OpenAI-compatible, and Ollama adapters do not populate a separate thinking-token field, so `thinking_tokens` remains zero for those calls even when their responses contain reasoning. A provider that omits usage leaves the usage fields at zero with `usage_reported=false`.

## Procedures

### Adding a New Model with Thinking Support

1. Define the model's `supported_thinking_levels` in the appropriate model config section of [ensemble/app/models/model_configs.py](../../ensemble/app/models/model_configs.py).
2. For Anthropic models, optionally override `thinking_budgets` per level; otherwise the default table is used.
3. For Ollama models, set `thinking_dialect` to either `NATIVE_TOGGLE` or `NONE`.
4. For Anthropic Opus-class models, consider increasing `thinking_output_reserve` to 8,192.
5. Run `./g8e test lint` to validate the configuration.

### Debugging Streaming Thought Events

1. Check `TurnState.thinking_active` in [ensemble/app/services/ai/agent_turn.py](../../ensemble/app/services/ai/agent_turn.py) to confirm the state machine is transitioning correctly.
2. Verify that `THINKING` chunks are being emitted for all thought content before `THINKING_END`.
3. Confirm that thought parts are preserved in `response.parts` for the ReAct loop history.
4. For provider-specific issues, check the corresponding adapter's normalization logic in [ensemble/app/llm/providers/](../../ensemble/app/llm/providers/).

### Tracing a Thinking Request End-to-End

1. Verify the model has thinking support via `clamp_thinking_level()` returning non-`off`.
2. Check the call shape: primary calls carry `ThinkingConfig`; assistant/lite calls do not.
3. Trace the provider translator output (Gemini: thinking_level + include_thoughts; Anthropic: budget_tokens; OpenAI: reasoning.effort; Ollama: think boolean).
4. Inspect the provider response for normalized thought parts.
5. Verify thought parts are emitted as `THINKING` / `THINKING_END` chunks in the stream.

## Anti-patterns

- Hand-editing provider adapter translations instead of updating the canonical translator function in [ensemble/app/llm/thinking.py](../../ensemble/app/llm/thinking.py).
- Requesting thinking on assistant or lite call shapes, which do not carry `thinking_config`.
- Registering an Ollama model without declaring its `thinking_dialect`, which causes a runtime ValueError.
- Merging thought parts with non-thought content during consolidation, which breaks provider multi-turn protocol requirements.
- Including thinking-marked messages in durable memory generation, which pollutes the memory update context.
- Sending unsupported thinking levels directly to providers instead of clamping via `clamp_thinking_level()`.
- Hard-coding line numbers in cross-references to thinking-related code instead of using repository-relative paths.

## Links out

- [LLM Providers](llm-providers.md): Provider implementations, model roles, and capability profiles.
- [Agents](agents.md): Agent persona definitions, model tiers, and Tribunal consensus.
- [Governance](governance.md): Five-layer verification pipeline and envelope validation.
- [Architecture](architecture.md): System architecture, protocol surfaces, and model hierarchy.
- [SSE Events](sse.md): Server-sent event structures and lifecycle.
- [Protocol](protocol.md): Protocol buffers, messages, and contract definitions.
- [Ensemble Architecture](../architecture/ensemble.md): Platform-level summary of g8ee's role.
- [Documentation Guide](../devs/docs.md): Documentation audit and ownership rules.
