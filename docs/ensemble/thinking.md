# Thinking

## Overview

The g8e Agentic Ensemble (`g8ee`) represents provider reasoning with a canonical `ThinkingLevel` and translates it at the provider boundary. The supported levels are `off`, `minimal`, `low`, `medium`, and `high`. During a streamed primary agent turn, g8ee normalizes provider reasoning into thought parts, emits thought lifecycle events separately from visible text, and preserves provider context required by later tool-result requests.

The main implementation entry points are `ensemble/app/constants/config.py`, `ensemble/app/models/model_configs.py`, `ensemble/app/llm/thinking.py`, `ensemble/app/llm/providers/`, `ensemble/app/services/ai/generation_config_builder.py`, and `ensemble/app/services/ai/agent_turn.py`.

Thinking is an application-level model capability. It does not authorize a governed operation, substitute for protocol L2 consensus or L3 notary authorization, or attest to provider behavior outside the g8e path. See [Governance](governance.md) and [AI Agents and the g8e Governance Boundary](../architecture/agents.md).

## When Thinking Runs

The primary generation call shape carries `ThinkingConfig`, tools, and system instructions. The production generation builder requests `high` and `include_thoughts=true` for primary calls, then clamps that request to the selected model's registered capability. This call shape is used for both complex and simple chat turns because either route can enter the tool loop; a simple turn selects the assistant model while still using the primary call shape.

Assistant and lite call shapes do not carry thinking configuration. Triage, Tribunal generation, risk analysis, title generation, memory extraction, response analysis, and other assistant or lite helper calls therefore do not request reasoning output. The provider may return unexpected fields, but g8ee does not enable thinking for these call shapes. Ollama assistant and lite requests explicitly send `think=false` for native-toggle models and omit the parameter for models whose dialect is `none`.

## Canonical Levels and Model Profiles

`LLModelConfig.supported_thinking_levels` is the capability source of truth:

- An empty list means the model has no registered thinking capability and resolves every request to `off`.
- A list containing `off` describes a model whose reasoning can be disabled.
- A list without `off` describes an always-on reasoning model. No built-in model profile currently uses this form.

`clamp_thinking_level` resolves a requested level to the highest supported intensity at or below it. If the request is lower than every supported intensity, it returns the model's lowest supported intensity. An `off` request returns `off` when the model supports it; for an always-on profile it returns the lowest supported intensity.

The production primary builder always requests `high`; the other levels are used by the canonical translation API and capability-specific callers. Unknown model names resolve to the shared conservative profile, which disables thinking, tools, and provider-enforced structured-output decisions. A custom model must have a registered profile before g8ee relies on its reasoning capability. See [LLM Providers](llm-providers.md#model-capability-registry) for the current registry.

## Provider Translation

Provider translators are pure functions of a requested level and an immutable model configuration. They return typed translation results; the provider adapter applies the result to its outbound request. Providers omit reasoning fields when the resolved level is `off` rather than sending an unsupported level.

### Gemini

Gemini receives `thinking_config.thinking_level` and `include_thoughts`. The registered Gemini profiles support `off`, `low`, `medium`, and `high`; `gemini-3.1-flash-lite` also supports `minimal`. When the resolved level is `off`, the adapter omits `thinking_config` and disables thought output. Gemini thought parts may carry opaque thought signatures, which the adapter normalizes to base64 strings.

### Anthropic

Anthropic receives extended thinking with a token budget instead of a level name. The default budgets are 1,024 tokens for `minimal`, 2,048 for `low`, 8,192 for `medium`, and 16,384 for `high`. The Claude Opus profile overrides the supported budgets to 4,096 for `low`, 16,384 for `medium`, and 32,000 for `high`.

When extended thinking is active, the adapter omits `top_k` and `top_p`. It raises `max_tokens` when necessary so the total output limit includes the thinking budget plus visible-output headroom: 4,096 tokens by default or 8,192 for Claude Opus. When thinking is off, it omits the thinking object and preserves the sampling parameters. Anthropic thinking blocks can carry opaque signatures, which remain attached when history is converted back to the Messages API format.

### OpenAI-compatible providers

OpenAI receives `reasoning.effort` with the resolved `minimal`, `low`, `medium`, or `high` value. The registered `gpt-5.4-mini` profile supports `off`, `minimal`, and `low`; the generic OpenAI profile has no declared thinking capability and therefore resolves to `off`. When thinking is off, the adapter omits the `reasoning` object.

llama.cpp inherits the OpenAI-compatible adapter, so it uses the same translation and can return `reasoning_content`. No llama.cpp-specific model profiles are registered; its model names therefore resolve to the unknown profile unless a matching model is registered.

### Ollama

Each registered Ollama profile declares a thinking dialect. Native-toggle models receive `think=true` when the resolved level is enabled and `think=false` when it is off. Profiles with the `none` dialect receive no `think` parameter. Current native-toggle profiles expose binary `off` or `high` capability, so a lower nonzero request resolves to `high`. The adapter reads reasoning from the response message's `thinking` field.

### Governed `g8e` provider

The `g8e` provider sends inference through the Gateway's governed inference-dispatch endpoint. Its `InferenceThinkingControl` is derived from the selected model's registered Ollama thinking dialect and carries an enabled flag plus `include_thoughts`. The response adapter converts protocol thinking parts into canonical thought parts and maps the dispatch response's optional thinking-token count into `UsageMetadata`. The Gateway and Inference Operator govern the inference request separately from g8ee's model reasoning; see [LLM Providers](llm-providers.md#supported-providers).

### Fake provider

The fake provider accepts primary settings for tests and scenarios, but its deterministic responses do not emit thought content. It does not cross a provider network boundary.

## Reasoning Output and Tool Context

Gemini normalizes parts marked as thoughts, Anthropic normalizes thinking blocks, OpenAI-compatible adapters normalize `reasoning_content`, Ollama normalizes the message `thinking` field, and the governed `g8e` adapter normalizes protocol thinking parts. These adapters expose the result as canonical `Part(thought=True, ...)` values or streamed `StreamChunkFromModel` values.

For one provider stream, the turn processor maintains this state machine:

```text
INACTIVE -> thought chunk -> ACTIVE -> visible text or tool call -> INACTIVE
```

The first thought chunk starts a phase and is emitted as a `THINKING` chunk. Additional thought chunks extend the phase. Visible text or a tool call closes the phase before that output is processed. End of stream also closes an active phase. `THINKING_END` is emitted once for each transition out of the active phase.

The completed thought block remains in the model response parts used by the in-memory ReAct loop. This preserves reasoning context and provider-required signatures for the next request in a tool turn. Consolidation merges only adjacent plain, unsigned, non-thought text parts; it preserves thought parts, tool calls, and signature-bearing parts in order. Visible response assembly and interrogation detection read only non-thought text.

## Thought Signatures

Thought signatures are opaque provider context values, not cryptographic signatures that g8ee verifies. g8ee preserves them only to satisfy the originating provider's conversation or tool-call protocol.

Gemini inbound byte signatures are normalized to base64 strings. A part with a signature is never merged with unsigned content or another signed part. Signature-only Gemini parts are sent back as empty-text parts so the signature remains in provider history without adding visible text. Gemini tool-call parts retain their required signature, and signatures on other returned parts remain attached.

Anthropic signatures remain attached to their corresponding thinking blocks when conversation history is converted back to the Messages API format. OpenAI-compatible, Ollama, fake, and governed `g8e` response adapters do not add a separate thought-signature field to their canonical response parts.

## Streaming Events

The stream processor emits internal `THINKING` chunks for thought text and a `THINKING_END` chunk when a phase ends. The SSE layer publishes all three lifecycle actions under the single event identifier `g8e.v1.ai.llm.chat.iteration.thinking.started` using `ChatThinkingPayload.action_type`:

- `start` carries the first thought chunk in a phase.
- `update` carries each later thought chunk in that phase.
- `end` carries no thought text and marks the transition to visible text, tool execution, or stream completion.

The protocol registry also defines `thinking.update` and `thinking.end` identifiers, but the current ensemble runtime does not publish those identifiers. These events are session-targeted delivery telemetry; they are not governance state, authorization, or durable execution evidence.

## Memory and Telemetry

Durable memory generation skips conversation messages marked `is_thinking`. This prevents a thinking-only message from becoming a user preference or investigation summary. The filter does not remove thought parts from the turn-local provider history, where they can be required for multi-turn tool calling.

The agent accumulates provider-reported input, output, cache, total, and thinking token counts in turn results and includes them in model-call telemetry and completed response metadata. Gemini supplies a separate thought-token count, and the governed `g8e` provider can propagate one from the inference response. The Anthropic, OpenAI-compatible, and Ollama adapters currently do not populate a separate thinking-token field, so `thinking_tokens` remains zero for those calls even when their responses contain reasoning. A provider that omits usage leaves the usage fields at zero with `usage_reported=false`.

## Related

- [Ensemble Architecture](../architecture/ensemble.md): Platform-level summary of g8ee's role
- [Architecture](architecture.md): System architecture, protocol surfaces, and model hierarchy
- [LLM Providers](llm-providers.md): Provider implementations, model roles, and capability profiles
- [Agents](agents.md): Agent persona definitions, model tiers, and Tribunal consensus
- [Prompts](prompts.md): System prompt assembly and persona templating
- [Governance](governance.md): Five-layer verification pipeline and envelope validation
- [Evals](evals.md): Benchmark evaluation suite and Judge scoring rubrics
- [Documentation Guide](../devs/docs.md): Documentation audit and ownership rules
