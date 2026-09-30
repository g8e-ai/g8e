---
doc_id: ensemble_llm_providers
title: LLM Providers
audience: platform and feature developers, coding agents
status: current
last_updated: 2026-09-28
version: v2.2.3
owners:
  - ensemble/app/llm/
  - ensemble/app/models/model_configs.py
  - ensemble/app/constants/config.py
related:
  - decision-providers.md
  - architecture.md
  - agents.md
  - thinking.md
  - prompts.md
  - tests.md
  - evals.md
when_to_read: Configuring LLM providers, adding new models, understanding generation call shapes and capabilities, tracing provider-specific behavior
do_not_use_for:
  - Platform coding invariants (../devs/devs.md)
  - Prompt engineering (prompts.md)
  - Agent routing logic (agents.md)
---

# LLM Providers

## Purpose

The g8e Agentic Ensemble (`g8ee`) uses a provider-neutral interface for model requests. The interface normalizes messages, streamed chunks, tool calls, structured responses, token usage, finish reasons, and provider reasoning into common application types. The provider factory selects a configured adapter for each model role and reuses its client across calls.

## Quick index

- [Purpose](#purpose)
- [Quick index](#quick-index)
- [Invariants](#invariants)
- [Owned surfaces](#owned-surfaces)
- [Procedures](#procedures)
- [Anti-patterns](#anti-patterns)
- [Links out](#links-out)

Invariant groups: [Configuration bootstrap](#bootstrap-configuration-inv-llm-bootstrap), [Provider registry](#provider-registry-inv-llm-providers), [Model capability registry](#model-capability-inv-llm-models), [Generation call shapes](#generation-shapes-inv-llm-calls).

## Invariants

### Bootstrap configuration (`INV-LLM-BOOTSTRAP`)

| ID | Rule |
| --- | --- |
| INV-LLM-BOOTSTRAP-01 | Environment variables provide the lowest-priority bootstrap values for API keys and endpoints only (INV-ENV-04). Platform settings replace them when explicitly set, and request-specific role overrides take precedence over platform settings. |
| INV-LLM-BOOTSTRAP-02 | Each role (`primary`, `assistant`, `lite`) accepts `ENDPOINT` and `API_KEY` environment variables under prefixes `G8E_LLM_PRIMARY_*`, `G8E_LLM_ASSISTANT_*`, `G8E_LLM_LITE_*`. Provider and model selection come only from Gateway-backed platform settings and request overrides, never from the environment. Role-specific credentials and endpoints take precedence over provider-level values. |
| INV-LLM-BOOTSTRAP-03 | A model name remains required in settings; the system does not automatically select a provider's default model. |

### Provider registry (`INV-LLM-PROVIDERS`)

| ID | Rule |
| --- | --- |
| INV-LLM-PROVIDERS-01 | Supported providers are: Gemini, Anthropic, OpenAI, Ollama, llama.cpp, g8e, Fake, and Jev. Each provider has distinct configuration requirements and adapter behavior. |
| INV-LLM-PROVIDERS-02 | Gemini retries transient errors (timeouts, HTTP 429, HTTP 503) for up to 4 attempts with exponential backoff (min 2s, max 30s). OpenAI and Anthropic disable SDK retries. Ollama and llama.cpp do not add provider-level retries. |
| INV-LLM-PROVIDERS-03 | Ollama rejects endpoints containing `/v1`; Ollama uses the native `/api/chat` surface. OpenAI-compatible providers (OpenAI, llama.cpp) append `/v1` to endpoints when absent. |
| INV-LLM-PROVIDERS-04 | The g8e provider routes inference through the Gateway's `/api/v1/inference/dispatch` endpoint over mTLS. Inline-data content parts are rejected with a `ModelCapabilityError`. |
| INV-LLM-PROVIDERS-05 | Anthropic ignores `response_format` for assistant and lite calls and relies on prompt instructions for structured output. Gemini, OpenAI-compatible providers, and Ollama pass JSON Schemas to the backend. |

### Model capability registry (`INV-LLM-MODELS`)

| ID | Rule |
| --- | --- |
| INV-LLM-MODELS-01 | The model registry in [model_configs.py](../../ensemble/app/models/model_configs.py) declares thinking levels, thinking budgets, output reserves, tool support, structured-output support, context limits, output limits, stop sequences, and sampling defaults for all known model names. |
| INV-LLM-MODELS-02 | Unknown model names use the shared `UNKNOWN_MODEL_CONFIG` which disables thinking, tools, and provider-enforced structured-output decisions. Register a model profile before relying on reasoning, tools, or structured output for a custom model. |
| INV-LLM-MODELS-03 | Ollama-registered models MUST declare `thinking_dialect` explicitly (`NONE` for no reasoning, `NATIVE_TOGGLE` for native `think=true/false`). Missing dialect at import time raises `ValueError`. |

### Generation call shapes (`INV-LLM-CALLS`)

| ID | Rule |
| --- | --- |
| INV-LLM-CALLS-01 | All adapters implement streaming and non-streaming methods for three call shapes: Primary (system instructions, tools, tool-calling policy, thinking, sampling, stops, response modalities, output limits), Assistant (system instructions, optional structured response, sampling, stops, output limits), Lite (same as Assistant, no tools or thinking). |
| INV-LLM-CALLS-02 | The main chat agent always uses the primary generation call shape. Complex turns select the primary model and provider. Simple turns select the assistant model; provider lookup for simple turns follows the lite role. |
| INV-LLM-CALLS-03 | OpenAI-compatible primary calls with tools use a non-streaming provider request and emit the completed response through the streaming interface, avoiding endpoints that stall when tools and streaming combine. |

## Owned surfaces

| Claim | Path | Verify |
| --- | --- | --- |
| Provider implementations | `ensemble/app/llm/providers/` | Each adapter file (gemini.py, openai.py, anthropic.py, ollama.py, g8e.py, fake.py, llamacpp.py) implements `LLMProvider` interface |
| Provider factory | [factory.py](../../ensemble/app/llm/factory.py) | Role resolution, cache key generation, provider instantiation |
| Model registry | [model_configs.py](../../ensemble/app/models/model_configs.py) | MODEL_REGISTRY and model profile definitions |
| LLM settings | [settings.py](../../ensemble/app/models/settings.py) | LLMSettings.resolve() method, role fallback chains, environment variable support |
| Thinking translation | [thinking.py](../../ensemble/app/llm/thinking.py) | Provider-specific thinking level translation and wire format mapping |

## Procedures

### Configure Model Roles

g8ee has three independently configurable model roles:

| Wire value | Display label | Use |
| --- | --- | --- |
| `primary` | Primary | Complex chat turns, tool-capable agent loops, and primary reasoning work |
| `assistant` | Assistant | The model selected for simple chat turns |
| `lite` | Lite | Triage, Tribunal generation, risk analysis, title generation, memory extraction, semantic eval judge, and other concise or structured tasks |

Wire values are canonical in APIs, settings, telemetry, and persisted records. User-facing documentation and UI copy use the display labels (`Primary`, `Assistant`, `Lite`). Do not introduce alternate display names for `lite`.

Configure a provider and model for every role that uses a distinct backend. If the assistant role has no provider, provider resolution falls back to primary. If the lite role has no provider, resolution falls back to assistant and then primary. Model resolution follows the same direction, with assistant falling back to primary and lite falling back to assistant and then primary.

The main chat agent always uses the primary generation call shape because both simple and complex turns can enter the tool loop. Complex turns select the primary model and provider. Simple turns select the assistant model, while provider lookup follows the lite role, so the configured lite provider must accept the assistant model when those roles use different backends.

### Environment bootstrap

Environment variables provide the lowest-priority bootstrap values. Platform settings replace them when explicitly set, and request-specific role overrides take precedence over platform settings.

Each role accepts `PROVIDER`, `MODEL`, `ENDPOINT`, and `API_KEY` variables under these prefixes:

- `G8E_LLM_PRIMARY_*`
- `G8E_LLM_ASSISTANT_*`
- `G8E_LLM_LITE_*`

Provider-level credentials and endpoints are also available through:

- OpenAI: `G8E_LLM_OPENAI_API_KEY`, `G8E_LLM_OPENAI_ENDPOINT`
- Anthropic: `G8E_LLM_ANTHROPIC_API_KEY`, `G8E_LLM_ANTHROPIC_ENDPOINT`
- Gemini: `G8E_LLM_GEMINI_API_KEY`
- Ollama: `G8E_LLM_OLLAMA_API_KEY`, `G8E_LLM_OLLAMA_ENDPOINT`
- llama.cpp: `G8E_LLM_LLAMACPP_API_KEY`, `G8E_LLM_LLAMACPP_ENDPOINT`
- System One (via Ollama): `G8E_LLM_JEV_MODEL` (uses existing Ollama endpoint settings)

Role-specific credentials and endpoints take precedence over provider-level values. A model name remains required; the settings layer does not automatically select a provider's default model.

### Generation and execution controls

The LLM settings model carries these cross-provider controls:

| Setting | Default | Effect |
| --- | --- | --- |
| `llm_max_tokens` | Unset | Overrides the registry output limit or the 20,000-token system fallback when set |
| `llm_command_gen_enabled` | `true` | Enables Tribunal command generation; disabling it makes command requests fail closed |
| `llm_command_gen_auditor` | `true` | Enables the Auditor stage after Tribunal candidate generation |
| `llm_command_gen_passes` | `5` | Sets the number of Tribunal generation passes; runtime resolution enforces at least one pass |
| `llm_parallel_tool_calls` | `true` | Executes multiple tool calls from one model turn concurrently; this controls g8ee execution rather than a provider request parameter |

### Supported Providers

| Provider | Configuration requirements | Adapter behavior |
| --- | --- | --- |
| Gemini (`gemini`) | API key; the Google SDK manages the endpoint | Uses `google-genai`; supports primary tools, Google Search grounding, structured assistant and lite output, streamed and non-streamed calls, thinking levels, usage metadata, and opaque thought-signature retention. Retries initial timeouts, HTTP 429, and HTTP 503 for up to 4 attempts with exponential backoff. |
| Anthropic (`anthropic`) | API key and endpoint; default endpoint is `https://api.anthropic.com` | Uses the Anthropic Messages API; supports primary tools, extended thinking, streamed and non-streamed calls, role alternation, and usage metadata. Disables SDK retries. Does not apply `response_format` for assistant or lite calls. |
| OpenAI (`openai`) | API key and endpoint; default endpoint is `https://api.openai.com/v1` | Uses Chat Completions; supports primary function calling, registered-model reasoning effort (off, minimal, low for gpt-5.4-mini only), JSON Schema response formats for assistant and lite calls, streaming, and usage metadata. Disables SDK retries. Appends `/v1` to endpoint if absent. |
| Ollama (`ollama`) | Endpoint; API key is optional; default endpoint is `http://localhost:11434` | Uses Ollama's native chat API; supports primary tools, per-model `think` toggles (off/high binary for native-toggle models), JSON Schema formats for assistant and lite calls, streaming, and usage metadata. Rejects endpoints containing `/v1`. No adapter-level retries. |
| llama.cpp (`llamacpp`) | Endpoint; API key is optional; default endpoint is `http://localhost:11444` | Uses the OpenAI-compatible adapter; inherits OpenAI behavior and appends `/v1` when absent. Actual tool, schema, and streaming support depend on the server and loaded model. |
| g8e (`g8e`) | No provider endpoint or API key; requires the startup-injected `InternalHttpClient` | Routes inference through the Gateway's `/api/v1/inference/dispatch` endpoint over mTLS. The Gateway and Inference Operator apply the governed L1-L5 path and return a signed receipt with the typed result. Ordered messages, tools, structured output, thinking controls, usage, and evaluation bindings cross the governed request; inline-data parts fail closed with a `ModelCapabilityError`. |
| Fake (`fake`) | No credentials or endpoint | Runs in process without network access; emits deterministic text, structured lite responses, and selected tool calls for CI, air-gapped tests, and scenarios. |
| System One (`jev`) | Uses existing Ollama endpoint settings; no separate API key (optional reverse-proxy auth via `G8E_LLM_OLLAMA_API_KEY`) | **Lite role only.** Ollama System One API for triage and semantic eval judge — not a generative LLM. Models: `nimble` (9B), `tev1` (4B/0.8B). See [Decision Providers](decision-providers.md). |

Provider validation runs for every configured role before chat starts. A configured model without a provider fails validation. OpenAI and Anthropic require both credentials and endpoints, Gemini requires credentials, Ollama and llama.cpp require endpoints, and the fake and g8e providers have no provider-level credential or endpoint requirements. The g8e provider still fails if the startup-injected `InternalHttpClient` is unavailable.

### Generation Call Shapes

All adapters implement streaming and non-streaming methods for the three call shapes:

- **Primary** supports system instructions, tools, tool-calling policy, thinking configuration, sampling controls, stop sequences, response modalities, and output limits.
- **Assistant** supports system instructions, optional structured response format, sampling controls, stop sequences, and output limits. It does not accept tools or thinking configuration.
- **Lite** has the same provider-facing fields as assistant and serves short, high-throughput, or structured tasks. It does not accept tools or thinking configuration.

OpenAI-compatible primary calls with tools use a non-streaming provider request and emit the completed response through the streaming interface. This avoids endpoints that stall when tools and streaming are combined.

### Model Capability Registry

The model registry supplies generation defaults and capability decisions for known model names. It records thinking levels, thinking budgets, output reserve, tool and structured-output support, context limits, output limits, stop sequences, and sampling defaults. Services use these profiles to decide whether to expose tools or request provider-enforced structured output.

The registry contains these unique model names:

| Provider family | Registered models | Thinking profile | Structured-output profile |
| --- | --- | --- | --- |
| Gemini | `gemini-3.1-pro-preview`, `gemini-3.1-pro-preview-customtools`, `gemini-3.1-flash-lite`, `gemini-3-flash-preview` | Off, low, medium, and high; flash-lite also supports minimal | Enabled |
| Anthropic | `claude-opus-4-6`, `claude-sonnet-4-6`, `claude-haiku-4-5` | Opus and Sonnet: off, low, medium, high; Haiku: off, minimal, low | Not declared |
| OpenAI | `gpt-5.4-mini` | Off, minimal, and low | Enabled |
| Ollama | `gemma4:e4b`, `gemma4:e2b`, `gemma4:e2b-g8ea`, `gemma4:12b`, `granite4.2:8b`, `granite4.2:3b`, `llama3.2:3b`, `qwen3.5:2b` | Gemma4, Granite, Qwen: off/high native toggle; Llama: none | Disabled; adapter serializes caller-supplied schemas |

Adapters can send other model names to a backend, but unknown names use the shared unknown profile. That profile disables thinking, tools, and provider-enforced structured-output decisions. Register a model profile before relying on reasoning, tools, or provider-enforced structured output for a custom model. Unregistered llama.cpp model names use the unknown profile.

### Thinking translation

Primary services request one of `off`, `minimal`, `low`, `medium`, or `high`. The registry clamps the request to the highest supported level at or below the requested intensity. A model with no thinking profile resolves to off; a model that cannot turn thinking off resolves an off request to its lowest supported intensity.

Providers translate the resolved level as follows:

- Gemini sends `thinking_level` and `include_thoughts`, or omits thinking configuration when off.
- Anthropic sends an extended-thinking token budget. It removes sampling parameters while thinking is active and increases `max_tokens` when necessary to preserve visible-output headroom.
- OpenAI sends `reasoning.effort`, or omits reasoning when off.
- Ollama sends `think=true` or `think=false` for native-toggle models and omits the parameter for models without a thinking dialect.

Gemini and Anthropic may return opaque provider signatures with thinking content. g8ee preserves these tokens across message history and tool-result turns when the provider protocol requires them. g8ee does not cryptographically verify provider thought signatures. See [Thinking](thinking.md) for the reasoning lifecycle and signature handling rules.

### Structured Output and Tools

Canonical tool declarations and JSON Schemas are converted at the provider boundary. Primary calls expose tools when the model profile permits them. Assistant and lite calls may carry a response format, but enforcement depends on the adapter: Gemini, OpenAI-compatible providers (OpenAI, llama.cpp), and Ollama pass a schema to the backend; the Anthropic adapter ignores `response_format` and relies on prompt instructions for structured output.

Provider capability failures are translated only at call sites that requested thinking or tools. Recognized rejection messages become typed `ThinkingCapabilityError` or `ToolCapabilityError`; unrelated provider failures retain the original exception. This translation concentrates on primary tool-capable calls rather than every assistant and lite request.

### Retries, Caching, and Shutdown

Gemini uses tenacity for exponential backoff retries on transient errors (timeouts, HTTP 429, HTTP 503) for up to four attempts. OpenAI and Anthropic disable SDK retries (max_retries=0). Ollama and llama.cpp do not add provider-level retries. Agent and evaluation services may apply their own retries around an adapter call.

The factory caches provider clients by connection configuration, not by model. Gemini keys include provider and API key; OpenAI and Anthropic keys include provider, endpoint, and API key; Ollama and llama.cpp keys include provider, normalized endpoint, and API key; the fake provider uses one cache entry. Shutdown calls `clear_provider_cache()` to force-close cached clients.

### Model Call Evidence

Network adapters record a SHA-256 hash and a sensitive-data scan attestation for the exact outbound provider request. Service call sites can attach this input evidence to model-call telemetry with the agent role, provider class, model, monotonic timestamps, available token counts, finish reason, retry count, success state, error type, and an output hash. The g8e provider additionally records governed-dispatch evidence, including the signed receipt and identity-bound inference result. The fake provider does not record provider-boundary evidence because it does not cross a network boundary.

Usage fields remain zero with `usage_reported=false` when a provider omits usage. Telemetry stores hashes and sensitivity findings rather than raw prompts and outputs. Evidence and retry telemetry are assembled by participating services, so direct adapter calls do not independently create complete model-call records.

## Anti-patterns

- Configuring a model without its provider role (fails validation)
- Hard-wrapping environment variable names or documenting them with line numbers instead of stable symbol names (env vars are defined in constants/env_vars.py)
- Configuring Jev for generative text generation (it only supports decision evaluation; use a generative provider for text output)
- Creating a new Ollama model entry without declaring `thinking_dialect` (explicit declaration required; missing dialect raises ValueError at import time)
- Passing inline-data parts to the g8e provider (fails closed with `ModelCapabilityError`)
- Assuming unknown model names have tool or thinking support (they use the unknown profile which disables both)

## Links out

- [Decision Providers](decision-providers.md): Jev (System One) for lite-role triage and eval judge
- [Architecture](architecture.md): Ensemble components and request flow
- [Agents](agents.md): Agent roles and model-tier assignments
- [Thinking](thinking.md): Reasoning translation and provider signatures
- [Prompts](prompts.md): Prompt assembly and persona templates
- [Testing](tests.md): Ensemble test tiers and commands
- [Evals](evals.md): Evaluation evidence and Judge scoring
