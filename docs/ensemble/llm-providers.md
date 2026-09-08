# LLM Providers

## Overview

The g8e Agentic Ensemble (`g8ee`) uses a provider-neutral interface for model requests. The interface normalizes messages, streamed chunks, tool calls, structured responses, token usage, finish reasons, and provider reasoning into common application types. The provider factory selects a configured adapter for each model role and reuses its client across calls.

The primary implementation entry points are `ensemble/app/llm/provider.py`, `ensemble/app/llm/factory.py`, and `ensemble/app/models/model_configs.py`.

## Configure Model Roles

g8ee has three independently configurable model roles:

| Role | Use |
| --- | --- |
| `primary` | Complex chat turns, tool-capable agent loops, and primary reasoning work |
| `assistant` | The model selected for simple chat turns |
| `lite` | Triage, Tribunal generation, risk analysis, title generation, memory extraction, and other concise or structured tasks |

Configure a provider and model for every role that uses a distinct backend. If the assistant role has no provider, provider resolution falls back to primary. If the lite role has no provider, resolution falls back to assistant and then primary. Model resolution follows the same direction, with assistant falling back to primary and lite falling back to assistant and then primary.

The main chat agent always uses the primary generation call shape because both simple and complex turns can enter the tool loop. Complex turns select the primary model and provider. Simple turns select the assistant model, while provider lookup follows the lite role, so the configured lite provider must accept the assistant model when those roles use different backends.

### Environment bootstrap

Environment variables provide the lowest-priority bootstrap values. Stored settings and request-specific role values replace them when present.

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

Role-specific credentials and endpoints take precedence over provider-level values. A model name remains required; the settings layer does not automatically select a provider's default model.

### Generation and execution controls

The LLM settings model also carries these cross-provider controls:

| Setting | Default | Effect |
| --- | --- | --- |
| `llm_max_tokens` | Unset | Overrides the registry output limit or the 20,000-token system fallback when set |
| `llm_command_gen_enabled` | `true` | Enables Tribunal command generation; disabling it makes command requests fail closed |
| `llm_command_gen_auditor` | `true` | Enables the Auditor stage after Tribunal candidate generation |
| `llm_command_gen_passes` | `5` | Sets the number of Tribunal generation passes; runtime resolution enforces at least one pass |
| `llm_parallel_tool_calls` | `true` | Executes multiple tool calls from one model turn concurrently; this controls g8ee execution rather than a provider request parameter |

## Supported Providers

| Provider | Configuration requirements | Adapter behavior |
| --- | --- | --- |
| Gemini (`gemini`) | API key; the Google SDK manages the endpoint | Uses `google-genai`; supports primary tools, Google Search grounding, structured assistant and lite output, streamed and non-streamed calls, thinking levels, usage metadata, and opaque thought-signature retention |
| Anthropic (`anthropic`) | API key and endpoint; default endpoint is `https://api.anthropic.com` | Uses the Anthropic Messages API; supports primary tools, extended thinking, streamed and non-streamed calls, role alternation, and usage metadata; the adapter does not apply `response_format` for assistant or lite calls |
| OpenAI (`openai`) | API key and endpoint; default endpoint is `https://api.openai.com/v1` | Uses Chat Completions; supports primary function calling, registered-model reasoning effort, JSON Schema response formats for assistant and lite calls, streaming, and usage metadata |
| Ollama (`ollama`) | Endpoint; API key is optional; default endpoint is `http://localhost:11434` | Uses Ollama's native chat API; supports primary tools, per-model `think` toggles, JSON Schema formats for assistant and lite calls, streaming, and usage metadata; endpoints containing `/v1` are rejected |
| llama.cpp (`llamacpp`) | Endpoint; API key is optional; default endpoint is `http://localhost:11444` | Uses the OpenAI-compatible adapter and appends `/v1` when absent; actual tool, schema, and streaming support depends on the server and loaded model |
| Fake (`fake`) | No credentials or endpoint | Runs in process without network access; emits deterministic text, structured lite responses, and selected tool calls for CI, air-gapped tests, and scenarios |

Provider validation runs for every configured role before chat starts. A configured model without a provider fails validation. OpenAI and Anthropic require both credentials and endpoints, Gemini requires credentials, Ollama and llama.cpp require endpoints, and the fake provider has no external requirements.

The LLM adapters rely on their SDK transports for TLS verification. They do not attach the g8ee workload mTLS certificate or explicitly pass the platform trust bundle to model-provider connections. Custom HTTPS endpoints therefore require trust configuration that the selected SDK and its process environment recognize.

## Generation Call Shapes

All adapters implement streaming and non-streaming methods for the three call shapes:

- **Primary** supports system instructions, tools, tool-calling policy, thinking configuration, sampling controls, stop sequences, response modalities, and output limits.
- **Assistant** supports system instructions, optional structured response format, sampling controls, stop sequences, and output limits. It does not accept tools or thinking configuration.
- **Lite** has the same provider-facing fields as assistant and serves short, high-throughput, or structured tasks. It does not accept tools or thinking configuration.

OpenAI-compatible primary calls with tools use a non-streaming provider request and emit the completed response through the streaming interface. This avoids endpoints that stall when tools and streaming are combined.

## Model Capability Registry

The model registry supplies generation defaults and capability decisions for known model names. It records thinking levels, thinking budgets, output reserve, tool and structured-output support, context limits, output limits, stop sequences, and sampling defaults. Services use these profiles to decide whether to expose tools or request provider-enforced structured output.

The registry contains these unique model names:

| Provider family | Registered models | Thinking profile | Structured-output profile |
| --- | --- | --- | --- |
| Gemini | `gemini-3.1-pro-preview`, `gemini-3.1-pro-preview-customtools`, `gemini-3.1-flash-lite`, `gemini-3-flash-preview` | Off plus low, medium, and high; flash lite also supports minimal | Enabled |
| Anthropic | `claude-opus-4-6`, `claude-sonnet-4-6`, `claude-haiku-4-5` | Opus and Sonnet support off, low, medium, and high; Haiku supports off, minimal, and low | Not declared |
| OpenAI | `gpt-5.4`, `gpt-5.4-mini` | `gpt-5.4` has no declared thinking support; mini supports off, minimal, and low | Enabled |
| Ollama | `gemma4:e4b`, `gemma4:e2b`, `llama3.2:3b`, `qwen3.5:2b` | All except Llama use an off/high native toggle; Llama has no thinking mode | Enabled for Gemma4 E4B and E2B |

Adapters can send other model names to a backend, but unknown names use the shared unknown profile. That profile disables thinking and provider-enforced structured-output decisions while leaving tools enabled. Add a registered profile before relying on reasoning or structured output from a custom model.

### Thinking translation

Primary services request one of `off`, `minimal`, `low`, `medium`, or `high`. The registry clamps the request to the highest supported level at or below the requested intensity. A model with no thinking profile resolves to off; a model that cannot turn thinking off resolves an off request to its lowest supported intensity.

Providers translate the resolved level as follows:

- Gemini sends `thinking_level` and `include_thoughts`, or omits thinking configuration when off.
- Anthropic sends an extended-thinking token budget. It removes sampling parameters while thinking is active and increases `max_tokens` when necessary to preserve visible-output headroom.
- OpenAI sends `reasoning.effort`, or omits reasoning when off.
- Ollama sends `think=true` or `think=false` for native-toggle models and omits the parameter for models without a thinking dialect.

Gemini and Anthropic may return opaque provider signatures with thinking content. g8ee preserves these tokens across message history and tool-result turns when the provider protocol requires them. g8ee does not cryptographically verify provider thought signatures. See [Thinking](thinking.md) for the reasoning lifecycle and signature handling rules.

## Structured Output and Tools

Canonical tool declarations and JSON Schemas are converted at the provider boundary. Primary calls can expose tools when the model profile permits them. Assistant and lite calls can carry a response format, but enforcement depends on the adapter: Gemini, OpenAI-compatible providers, and Ollama pass a schema to the backend; Anthropic currently relies on prompt instructions because its adapter ignores the response format.

Provider capability failures are translated only at catch sites that know the request asked for thinking or tools. Recognized rejection messages become typed thinking or tool capability errors; unrelated provider failures retain the original exception. This translation is concentrated on primary tool-capable calls rather than every assistant and lite request.

## Retries, Caching, and Shutdown

Gemini retries initial timeouts, HTTP 429 responses, and HTTP 503 responses for up to four attempts with exponential backoff. The OpenAI and Anthropic clients disable SDK retries, and the Ollama adapter does not add provider-level retries. Agent and evaluation services may apply their own retries around an adapter call.

The factory caches provider clients by connection configuration, not by model. Gemini keys include provider and API key; OpenAI and Anthropic keys include provider, endpoint, and API key; Ollama and llama.cpp keys include provider, normalized endpoint, and API key; the fake provider uses one cache entry. Shutdown calls `clear_provider_cache()` to force-close cached clients.

## Model Call Evidence

Network adapters record a SHA-256 hash and a sensitive-data scan attestation for the exact outbound provider request. Service call sites can attach this input evidence to model-call telemetry with the agent role, provider class, model, monotonic timestamps, available token counts, finish reason, retry count, success state, error type, and an output hash. The fake provider does not record provider-boundary evidence because it does not cross a network boundary.

Usage fields remain zero with `usage_reported=false` when a provider omits usage. Telemetry stores hashes and sensitivity findings rather than raw prompts and outputs. Evidence and retry telemetry are assembled by participating services, so direct adapter calls do not independently create complete model-call records.

## Related

- [Ensemble Architecture](../architecture/ensemble.md): Platform-level summary of g8ee's role
- [Architecture](architecture.md): Ensemble components and request flow
- [Agents](agents.md): Agent roles and model-tier assignments
- [Thinking](thinking.md): Reasoning translation and provider signatures
- [Prompts](prompts.md): Prompt assembly and persona templates
- [PKI and Trust](pki.md): Workload identity and platform connections
- [Testing](tests.md): Ensemble test tiers and commands
- [Evals](evals.md): Evaluation evidence and Judge scoring
