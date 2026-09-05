# LLM Providers

## Overview

The g8e Agentic Ensemble (`g8ee`) interacts with Large Language Models through a provider-agnostic abstraction layer and factory pattern. The LLM subsystem decouples agent reasoning, consensus evaluation, and tool execution from vendor-specific SDKs and APIs. All provider adapters implement a unified interface organized around three functional capacity tiers (`primary`, `assistant`, `lite`), normalize inputs and outputs through canonical data models (`app.llm.llm_types`), translate reasoning intensity through standardized thinking translators (`app.llm.thinking`), and map vendor exceptions to typed capability errors (`app.llm.providers._capability`).

## Supported Providers

g8ee registers six concrete LLM providers in the central `LLMProvider` enum (`app.constants.config.LLMProvider`):

- **Gemini (`gemini`)** — Powered by `GeminiProvider` (`app.llm.providers.gemini`) wrapping the `google-genai` SDK. Supports native thinking levels (`minimal`, `low`, `medium`, `high`), cryptographic `ThoughtSignature` verification and retention across streaming parts and tool calls, structured JSON schema outputs via `ResponseFormat`, parallel tool use with `ToolGroup`, and multi-candidate generation responses.
- **Anthropic (`anthropic`)** — Powered by `AnthropicProvider` (`app.llm.providers.anthropic`) wrapping the official `anthropic` SDK for Claude models (`claude-opus-4-6`, `claude-sonnet-4-6`, `claude-haiku-4-5`). Supports extended thinking token budgets (`thinking.budget_tokens`), automatic token headroom reservation (`thinking_output_reserve`), strict user/assistant message role alternation, tool use (`tool_use` and `tool_result` content blocks), and streaming chunks.
- **OpenAI (`openai`)** — Powered by `OpenAIProvider` (`app.llm.providers.open_ai`) wrapping the `AsyncOpenAI` client for GPT models (`gpt-5.4-mini`, `gpt-4o`). Supports reasoning effort configuration (`reasoning.effort`), function calling (`tool_calls`), JSON schema structured outputs (`response_format`), and streaming completions.
- **Ollama (`ollama`)** — Powered by `OllamaProvider` (`app.llm.providers.ollama`) wrapping the `ollama.AsyncClient` for self-hosted open-weight models (Qwen 3.5, GLM 5.1, Gemma 4, Nemotron 3, Llama 3.2). Supports per-model reasoning dialects (`ThinkingDialect.NATIVE_TOGGLE` via `think=True/False` vs `ThinkingDialect.NONE`), internal network mTLS / TLS verification using the platform trust bundle, function calling, and structured JSON outputs.
- **llama.cpp (`llamacpp`)** — Powered by `LlamaCppProvider` (`app.llm.providers.llama_cpp`), which inherits directly from `OpenAIProvider` to communicate with local OpenAI-compatible HTTP servers without requiring API keys.
- **Fake Provider (`fake`)** — Powered by `FakeProvider` (`app.llm.providers.fake`), a zero-dependency, in-process deterministic provider for CI, airgapped builds, and automated scenario evaluations (`LLMProvider.FAKE`). Pattern-matches user message instructions to emit deterministic tool calls (`file_create_on_operator`, `file_write_on_operator`) or low-risk structured outputs without making network calls.

## Capacity Tiers and Provider Interface

All LLM provider implementations inherit from the abstract base class `LLMProvider` (`app.llm.provider.LLMProvider`). The interface defines streaming and non-streaming generation methods corresponding to the three operational tiers:

- **Primary Tier (`PrimaryLLMSettings`)** — `generate_content_stream_primary` and `generate_content_primary`. Used by primary reasoning agents (Sage, Auditor) for multi-step investigation planning, evidence synthesis, tool execution, and thinking. Settings configure `system_instructions`, `tools`, `thinking_config`, `tool_config`, sampling controls (`top_p_nucleus_sampling`, `top_k_filtering`), and `max_output_tokens`.
- **Assistant Tier (`AssistantLLMSettings`)** — `generate_content_stream_assistant` and `generate_content_assistant`. Used by fast-path responders and analytical support agents (Dash, Codex) for direct answers, memory building, and title generation. Settings support `system_instructions`, `response_format` for structured JSON schemas, and sampling controls without tool definitions.
- **Lite Tier (`LiteLLMSettings`)** — `generate_content_stream_lite` and `generate_content_lite`. Used by high-throughput, low-latency agents (Triage, Warden risk analyzers, Scribe, Tribunal members) for fast classification, command generation, and structured risk analysis. Settings support `system_instructions`, strict `response_format` JSON schema enforcement, and sampling controls.
- **Resource Lifecycle Management** — Providers implement `validate_config(api_key, endpoint)` for static configuration validation, `close()` and `force_close()` for HTTP client teardown, and async context management (`__aenter__` and `__aexit__`).

## Provider Factory and Caching

The factory entry point `get_llm_provider(settings, is_assistant=False, is_lite=False)` (`app.llm.factory`) instantiates and caches provider singletons based on the provided `LLMSettings` (`app.models.settings.LLMSettings`):

- **Role Resolution** — Evaluates the requested role (`primary`, `assistant`, or `lite`) via `settings.resolve(role)`, returning the configured provider type, API key, endpoint URL, and model name. Each tier can use a different provider and model.
- **Singleton Caching** — Instantiated providers are cached in `_provider_cache` using a compound key derived from provider type, endpoint, and API key (`provider|endpoint|api_key`). Singletons are reused across agent turns to avoid repeated client initialization and TLS connection overhead.
- **Cache Teardown** — Calling `clear_provider_cache()` during application shutdown or test cleanup forces the closure of all underlying HTTP sessions and resets the cache.
- **Network and TLS Verification Strategy** — Gemini uses public Google APIs with standard trust bundles. Anthropic and OpenAI public cloud endpoints use default trust bundles, while custom proxy endpoints can use platform CA certificates. Ollama and llama.cpp endpoints on internal networks or Docker bridges utilize the platform trust bundle (`g8eg-ca-bundle.pem`) for mTLS and internal TLS verification.

## Model Configuration Registry

Model capabilities and operational limits are centralized in `app.models.model_configs` via frozen `LLModelConfig` instances registered in `MODEL_REGISTRY`:

- **Model Capabilities** — Each `LLModelConfig` defines context window bounds (`context_window_input`, `context_window_output`), output token limits (`max_output_tokens`), tool support (`supports_tools`), structured output support (`supports_structured_output`), stop sequences (`stop_sequences`), and default sampling parameters (`top_k`, `top_p`).
- **Thinking Capabilities** — Encoded via `supported_thinking_levels` (`list[ThinkingLevel]`). An empty list indicates a non-reasoning model where thinking parameters are omitted from outbound requests. Opt-in reasoning models include `ThinkingLevel.OFF` alongside intensity levels (`LOW`, `MEDIUM`, `HIGH`). Always-on reasoning models omit `OFF`, requiring an explicit intensity level.
- **Thinking Budgets and Reserves** — `thinking_budgets` specifies per-level integer token budgets for providers requiring exact token allocations (Anthropic). `thinking_output_reserve` specifies minimum visible response headroom (e.g., 4,096 tokens for Sonnet/Haiku, 8,192 tokens for Opus) added to `max_tokens` when extended thinking is active to prevent truncation of final answers.
- **Scoped Test Overrides** — `MODEL_REGISTRY.override(name, **updates)` provides a context manager to install temporary capability overrides during unit tests and capability probes without mutating singleton state.

## Thinking and Reasoning Translation

Application services request reasoning effort using the canonical `ThinkingLevel` enum (`app.constants.config.ThinkingLevel`): `OFF`, `MINIMAL`, `LOW`, `MEDIUM`, `HIGH`. Pure translator functions in `app.llm.thinking` map the requested level and `LLModelConfig` to provider-native representations after applying `clamp_thinking_level`:

- **Gemini (`translate_for_gemini`)** — Emits `GeminiThinkingTranslation` with `thinking_level` string enum (`minimal`, `low`, `medium`, `high`) and `include_thoughts` boolean. Inbound cryptographic thought signatures (`ThoughtSignature`) are preserved in base64 format and re-attached to outbound tool calls to satisfy Gemini 3+ protocol requirements.
- **Anthropic (`translate_for_anthropic`)** — Emits `AnthropicThinkingTranslation` with `thinking={"type": "enabled", "budget_tokens": N}` and strips `top_k` and `top_p` sampling parameters when enabled. When disabled (`ThinkingLevel.OFF`), thinking parameters are omitted.
- **OpenAI (`translate_for_openai`)** — Emits `OpenAIThinkingTranslation` with `reasoning.effort` set to the level string (`minimal`, `low`, `medium`, `high`). Omits the `reasoning` parameter when thinking is disabled.
- **Ollama (`translate_for_ollama`)** — Evaluates `model_config.thinking_dialect`. Models declaring `ThinkingDialect.NATIVE_TOGGLE` pass `think=True` or `think=False` to `AsyncClient.chat()`. Models declaring `ThinkingDialect.NONE` omit thinking arguments entirely.

## Model Capability Error Translation

Provider adapters intercept vendor-specific SDK runtime errors at catch sites and pass them to `translate_capability_error` (`app.llm.providers._capability`). When an error message matches known capability-rejection fingerprints:

- **Thinking Incompatibility** — Rejections mentioning thinking configuration or unsupported reasoning parameters raise a typed `ThinkingNotSupportedError` (`app.errors.ThinkingNotSupportedError`).
- **Tool Incompatibility** — Rejections mentioning unsupported function calling or tool use raise a typed `ToolsNotSupportedError` (`app.errors.ToolsNotSupportedError`).
- **Typed Error Handling** — Downstream services catch typed capability exceptions with `isinstance` rather than parsing unstructured error strings, allowing automated fallback and escalation.

## Structured Output and Function Calling

The LLM subsystem provides end-to-end typing for structured extraction and operator tool calling:

- **Tool Group and Declarations** — Canonical `ToolGroup` and `ToolDeclaration` models define available tools and parameter schemas. Provider adapters convert these declarations to vendor formats (`_tools_to_openai`, `_tools_to_ollama`, Anthropic `tools` dictionaries, and Gemini function declarations).
- **JSON Schema Generation** — `schema_from_model` (`app.llm.llm_schema`) converts Pydantic `G8eBaseModel` classes into canonical JSON schemas for structured extraction.
- **Response Format Enforcement** — `ResponseFormat` with `ResponseJsonSchema` enforces structured output formatting for Warden risk assessments (`FileOperationRiskAnalysis`, `CommandRiskAnalysis`, `ErrorAnalysisResult`), Triage classifications (`TriageResult`), and evaluation scoring (`JudgeEvaluation`).

## Provider Evidence Contract

Every Anthropic, Gemini, Ollama, OpenAI-compatible, and fake-provider model call exposes normalized evidence at the provider boundary. A record identifies the configured provider and exact model, captures monotonic start and end times in one clock domain, records retry count and terminal finish state, and hashes the canonical model-boundary input and output. Token usage is split into the provider values available for that call, including input, output, reasoning, cached, and total counts where supported.

Adapters do not infer or fabricate unavailable usage. A provider that omits a count leaves it unavailable, and downstream reconciliation reports the missing field separately from measured usage. Retries preserve their attempt metadata rather than collapsing failed calls into a synthetic successful duration. Raw prompts and outputs are restricted evidence; analytical telemetry carries hashes and references instead of secret-bearing content.

## Related

- [Ensemble Architecture](../architecture/ensemble.md) — Platform-level summary of g8ee's role in the g8e platform
- [Architecture](architecture.md) — System architecture, protocol surfaces, and model hierarchy
- [Governance](governance.md) — Five-layer verification pipeline and envelope validation
- [Agents](agents.md) — Agent persona definitions, capacity tiers, and Tribunal consensus
- [Thinking](thinking.md) — L2 consensus, provider reasoning, and thought signatures
- [Prompts](prompts.md) — System prompt assembly and persona templating
- [Constants](constants.md) — Sourced protocol constants and application definitions
- [PKI & Trust](pki.md) — Public Key Infrastructure, trust bundles, and workload enrollment
- [Storage](storage.md) — Storage tiers and data sovereignty principles
- [Development](devs.md) — Developer setup, guidelines, and coding standards
- [Testing](tests.md) — Testing framework, test tiers, and practices
- [Evals](evals.md) — Benchmark evaluation suite and Judge scoring rubrics
