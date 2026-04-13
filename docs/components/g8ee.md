# g8ee — g8e Engine

g8ee is the AI engine for g8e. It provides an agentic, LLM-powered interface for infrastructure operations and troubleshooting, with full human-in-the-loop safety controls, data sovereignty, and multi-provider LLM abstraction.

> For cross-component AI architecture — transport map, conversation data models, and command execution pipeline — see [architecture/ai_agents.md](../architecture/ai_agents.md).
>
> For deep-reference security documentation — internal auth token, Sentinel scrubbing patterns, LFAA vault encryption, operator binding, web/operator session security, and the full threat model — see [architecture/security.md](../architecture/security.md).

---

## Architecture

g8ee is a Python/FastAPI service. The `ChatPipelineService` is the central coordinator — it assembles context, drives the AI agent, and handles post-response persistence.

```
ChatPipelineService
  ├── g8e agent            — Streaming agent with tool calling loop
  │     ├── AIToolService    — Tool registration and execution
  │     │     └── WebSearchProvider — Vertex AI Search (Discovery Engine) executor
  │     └── GroundingService — Provider-native grounding extraction, inline citation insertion
  ├── AIResponseAnalyzer      — Risk analysis (command, file op, error)
  ├── AIRequestBuilder        — Generation config, thinking config, attachments
  ├── InvestigationService    — (Domain Layer) Workflow determination, context enrichment, history management
  │     ├── InvestigationDataService — (Data Layer) Pure CRUD for investigations
  │     ├── OperatorDataService      — (Data Layer) Pure CRUD for operators
  │     └── MemoryDataService        — (Data Layer) Pure CRUD for memories
  ├── ChatTaskManager         — Task lifecycle and cancellation
  ├── CaseDataService         — Case management and SSE updates
  ├── MemoryGenerationService — Background memory updates from conversation
  ├── AttachmentService       — Attachment storage and retrieval
  └── EventService           — Internal SSE event delivery to g8ed
```

### Component Relationships

```mermaid
graph LR
    Browser -- "HTTPS / SSE" --> g8ed
    g8ed -- "HTTP (Internal)" --> g8ee
    g8ee -- "HTTP / WebSocket" --> g8es
    g8ed -- "WebSocket (mTLS)" --> g8eo
    g8eo -- "Pub/Sub" --> g8ee
```

- **g8ed** -- Web gateway; relays browser requests to g8ee and SSE events back to the browser.
- **g8es** -- Multi-purpose persistence layer:
    - **Document Store** (SQLite `documents`) -- Investigation state, operator documents, settings.
    - **KV Store** (SQLite `kv_store`) -- High-frequency state, session data, query cache.
    - **Pub/Sub Broker** (WebSocket/WSS) -- Command dispatch and event bus.
    - **Blob Store** (SQLite `blobs`) -- Binary attachments and large payloads.
- **g8eo** -- Operator daemon running on target systems; executes commands and manages local audit storage.

---

## Client Architecture

g8ee enforces a strict hierarchy for data access: **Clients** handle raw transport/protocol, **Handler Services** wrap clients with domain logic, and **Orchestrators** (like `ChatPipelineService`) consume the services.

g8ee maintains 5 core data clients, each with exactly one handler service:

| Client | Handler Service | Responsibility |
|--------|-----------------|----------------|
| `DBClient` | `DBService` | Authoritative document persistence (SQLite `documents` via g8es) |
| `KVCacheClient` | `KVService` | High-frequency state and session data (SQLite `kv_store` via g8es) |
| `PubSubClient` | `PubSubService` | Event-driven messaging and operator command dispatch |
| `BlobClient` | `BlobService` | Binary data storage and retrieval (SQLite `blobs` via g8es) |
| `InternalHttpClient` | `HTTPService` | External API communication (via `ServiceFactory`) |

### Initialization & Lifespan

Client and service lifecycle is managed in `app/main.py` via a 6-phase bootstrap process:
1. **Bootstrap Settings** -- Load minimal config for connectivity.
2. **Core Clients** -- Instantiate all 4 g8es clients (DB, KV, PubSub, Blob).
3. **Handler Services** -- Wrap clients in their respective services.
4. **CacheAsideService** -- Initialize the query caching layer.
5. **Platform Settings** -- Load full platform configuration.
6. **Service Factory** -- Construct all remaining domain services (Investigation, Operator, AI).

All clients share a common TLS configuration using the platform CA certificate.

---

## Streaming Architecture

### LLM Provider Abstraction

All LLM communication passes through the `LLMProvider` abstract base class (`app/llm/provider.py`). Six role-specific methods must be implemented:

| Method | Returns | Used For |
|--------|---------|----------|
| `generate_content_stream_primary` | `AsyncGenerator[StreamChunkFromModel]` | Agent main loop — yields chunks as they arrive |
| `generate_content_primary` | `GenerateContentResponse` | Non-streaming primary model calls |
| `generate_content_stream_assistant` | `AsyncGenerator[StreamChunkFromModel]` | Streaming assistant model calls |
| `generate_content_assistant` | `GenerateContentResponse` | Risk analysis, memory, title generation |
| `generate_content_stream_lite` | `AsyncGenerator[StreamChunkFromModel]` | Streaming lite model calls |
| `generate_content_lite` | `GenerateContentResponse` | Triage, eval |

Each method accepts a role-specific settings dataclass (`PrimaryLLMSettings`, `AssistantLLMSettings`, `LiteLLMSettings`) that carries the generation parameters appropriate for that role. LLM configuration is sourced from `G8eeUserSettings.llm` (`LLMSettings`) — there is no platform-level LLM default. The `get_llm_provider(settings.llm)` factory constructs a provider from user settings on each request.

`StreamChunkFromModel` is the canonical inter-layer type (`app/models/agent.py`). Its `type` field is a `StreamChunkFromModelType` enum (`app/constants/__init__.py`) with values: `text`, `thinking`, `thinking.update`, `thinking.end`, `tool.call`, `tool.result`, `citations`, `complete`, `error`, `retry`. All provider-specific types are translated to `StreamChunkFromModel` at the provider boundary — nothing above the provider layer touches SDK types.

### Provider Implementations

| Provider | Module | Streaming Model |
|----------|--------|-----------------|
| `GeminiProvider` | `app/llm/providers/gemini.py` | Opens the SDK stream with tenacity retry on the connection step only, then yields `StreamChunkFromModel` objects immediately as each SDK chunk arrives — no buffering |
| `AnthropicProvider` | `app/llm/providers/anthropic.py` | Streams via `client.messages.stream`, accumulating tool input JSON across deltas; yields text and thinking chunks immediately, emits tool call chunks on `content_block_stop` |
| `OllamaProvider` | `app/llm/providers/ollama.py` | Streams via the `ollama` Python SDK's AsyncClient; enables `think=true` for primary model calls to support Ollama's thinking feature; when tools are present falls back to a non-streaming call and yields the response as a single chunk |
| `OpenAICompatibleProvider` | `app/llm/providers/openai_compatible.py` | Streams via `AsyncOpenAI` for OpenAI-compatible endpoints; when tools are present falls back to a non-streaming call and yields the response as a single chunk |

**Gemini retry contract:** `_open_stream_attempt` wraps only the `generate_content_stream` API call in a tenacity retry (up to 4 attempts, exponential backoff, retryable on 429/503). Once the stream is open, chunks flow directly — no retry is possible mid-stream. If the stream breaks after yielding has started, the error propagates to the agent's retry guard, which prevents re-attempting a partially-delivered response.

**Thought signatures (Gemini 3):** Every tool call Part requires a thought signature or the API returns 400. `GeminiProvider` normalises inbound SDK `thought_signature` bytes to a base64 string (`ThoughtSignature.from_sdk`) and passes it through as-is on outbound requests. Thought and text parts carry signatures when available; signature-only parts are emitted as empty-text parts per the Gemini 3 streaming spec.

**Ollama Provider:** The `OllamaProvider` is a dedicated provider for Ollama endpoints, using the official `ollama` Python SDK's AsyncClient:
- **Endpoint Handling:** Strips `/v1` suffix if present to match Ollama's native API format
- **Thinking Support:** Enables `think=true` parameter for primary model calls to support Ollama's thinking feature; extracts `thinking` field from responses and streams it as `thought=True` chunks
- **SSL Verification:** Follows the standard SSL strategy — uses platform CA cert for internal endpoints, certifi bundle for external endpoints, disables verification for HTTP
- **Tool Calling:** Converts tool declarations to Ollama's function calling format; falls back to non-streaming when tools are present to avoid hanging

### Agent Streaming Loop

`g8e agent.stream_response` (`app/services/ai/agent.py`) is the single streaming implementation used by all chat paths. It runs a ReAct function-calling loop and yields `StreamChunkFromModel` objects:

```
stream_response
  └── _stream_with_tool_loop       (ReAct loop — runs until no pending tool calls)
        ├── _process_provider_turn      (consumes one provider stream, owns thinking state)
        │     yields: TEXT, THINKING, THINKING_UPDATE, THINKING_END, TOOL_CALL chunks
        └── _execute_turn_tool_calls (sequential function execution)
              yields: TOOL_CALL, TOOL_RESULT chunks
        final yields: CITATIONS (if grounding used), COMPLETE
```

Retry behaviour in `stream_response`: if the provider raises a retryable error and streaming has not yet started (`streaming_started=False`), the entire attempt is retried up to `AGENT_MAX_RETRIES` times with exponential backoff. Once any `TEXT` chunk has been yielded (`streaming_started=True`), errors are surfaced immediately — a partial response is never replayed.

**Invocation context lifecycle:** `stream_response` is an `async generator`. Python dispatches async-generator cleanup (`async_generator_athrow`) in a new `asyncio` task that runs in a different `Context` than the request that created the generator. Because `ContextVar.reset(token)` requires the token to be reset in the exact same `Context` it was set in, the invocation context lifecycle **must not** be owned by `stream_response`. It is owned by `run_with_sse` (a normal `async def` coroutine with a stable `Context`) via `start_invocation_context` before iteration and `reset_invocation_context` in `finally`. `stream_response` reads the already-set context value; it never holds a token.

`_process_provider_turn` owns all thinking state transitions for one LLM call. Thinking chunks are emitted as `StreamChunkFromModelType.THINKING` (or `THINKING_UPDATE`/`THINKING_END`) and delivered to g8ed for UI rendering if the model supports it. Text chunks are yielded as `StreamChunkFromModelType.TEXT` immediately.

### SSE Delivery Pipeline

`g8e agent.run_with_sse` consumes `stream_response` and delivers each `TEXT` chunk to the browser via `EventService` without batching or delay:

```
LLM SDK  →  GeminiProvider  →  stream_response  →  deliver_via_sse
                                                       │  TEXT chunk arrives
                                                       ▼
                                             EventService.publish (SessionEvent)
                                                       │  HTTP POST
                                                       ▼
                                                    g8ed /api/internal/sse/push
                                                       │  SSE fan-out
                                                       ▼
                                                    Browser
```

Each `TEXT` chunk produces exactly one HTTP POST to g8ed (`LLM_CHAT_ITERATION_TEXT_CHUNK_RECEIVED` event). g8ed relays it to the browser immediately via its SSE connection. `LLM_CHAT_ITERATION_TEXT_COMPLETED` is published once after the loop exits, carrying finish reason, citation metadata, and token usage.

`deliver_via_sse` chunk dispatch:

| `StreamChunkFromModelType` | SSE event published | Side effect |
|-------------------|--------------------|--------------|
| `TEXT` | `LLM_CHAT_ITERATION_TEXT_CHUNK_RECEIVED` | Appends to `AgentStreamingContext.response_text` |
| `THINKING` | none | `AgentStreamingContext.set_thinking_started()` |
| `THINKING_UPDATE` | none | — |
| `THINKING_END` | none | `AgentStreamingContext.set_thinking_ended()` |
| `TOOL_CALL` | `LLM_TOOL_SEARCH_WEB_REQUESTED` (search_web only); `OPERATOR_NETWORK_PORT_CHECK_REQUESTED` (check_port only); none for all other tools | — |
| `TOOL_RESULT` | `LLM_TOOL_SEARCH_WEB_COMPLETED` or `LLM_TOOL_SEARCH_WEB_FAILED` (search_web only); always `OPERATOR_COMMAND_COMPLETED` (command result) or `LLM_CHAT_ITERATION_COMPLETED` (turn tick) | — |
| `CITATIONS` | `LLM_CHAT_ITERATION_CITATIONS_RECEIVED` (only when `grounding_used=True`) | Stores `grounding_metadata` on `AgentStreamingContext` |
| `COMPLETE` | none (triggers `LLM_CHAT_ITERATION_TEXT_COMPLETED` after loop) | Stores `token_usage` and `finish_reason` on `AgentStreamingContext` |
| `ERROR` | none | Raises appropriate G8eError subclass (e.g., BusinessLogicError, ExternalServiceError) |
| `RETRY` | none | — |

`deliver_via_sse` initializes `grounding_metadata` and `token_usage` to `None` before the loop to prevent `UnboundLocalError` if the stream is empty or ends before those chunks arrive.

`THINKING`, `THINKING_UPDATE`, and `THINKING_END` chunks produce an SSE push (`LLM_CHAT_ITERATION_THINKING_RECEIVED`, etc.) when supported. They also update `AgentStreamingContext` state.

`AgentStreamingContext` accumulates `response_text` across all `TEXT` chunks for DB persistence after the stream completes. It is not involved in delivery — it is write-only during streaming.

### Error Handling

g8ee uses a unified error model derived from `G8eError`. Custom error signatures have been updated to support component attribution and detailed context.

| Error Class | Purpose | Key Parameters |
|-------------|---------|----------------|
| `ResourceNotFoundError` | Resource not found in DB | `message`, `resource_type`, `resource_id` |
| `AuthorizationError` | Permission denied | `message="Insufficient permissions"` |
| `ValidationError` | Request/model validation failure | `message`, `field`, `constraint` |
| `BusinessLogicError` | Invariant violation | `message`, `code` |
| `ExternalServiceError` | External service failure | `message`, `service_name`, `cause` |
| `NetworkError` | Network connectivity failure | `message`, `retry_suggested`, `cause` |
| `ConfigurationError` | Configuration missing or invalid | `message`, `key` |

---

## Data Models

Core data models for cache operations and markers are located in `app/models/cache.py`.

- **ArrayUnion** — Marker for appending to array fields: `ArrayUnion(values=[...], max_length=N)`
- **ArrayRemove** — Marker for removing from array fields: `ArrayRemove(values=[...])`
- **BatchWriteOperation** — Container for atomic multi-document writes.

## Cache-Aside Service

g8ee uses `CacheAsideService` to manage synchronization between the authoritative `DBService` and the `KVService` (g8es).

- **Invariants**: All write operations (`create`, `update`, `delete`, `batch`) **invalidate** the cache. Population only occurs during a `get_document` MISS.
- **`create_document`**: Checks for document existence in the DB first. If it exists, the call fails with a `DatabaseError`. If not, it writes to the DB and invalidates the cache key.
- **`get_document`**: Implements lazy-loading. It checks the KV cache first; if missing, it fetches from the DB and "warms" the cache.
- **`query_documents`**: Uses MD5 hashing of query parameters to cache result sets.
- **`append_to_array`**: Atomic `arrayUnion` on DB followed by cache invalidation.

For the full list of call behaviors and TTL strategies, see [architecture/storage.md](../architecture/storage.md#cache-aside-service).

---

## Workflow Modes

### Operator Bound

Activated when at least one g8eo Operator has `status=bound`.

- **Full tool suite** — command execution, file operations, directory listing, port checks, web search (if configured).
- **Human-in-the-loop** — all state-changing operations require explicit user approval before execution.
- **Thinking** — enabled for models that declare `supports_thinking=True`; uses the highest supported thinking level from the model's config.
- **Cloud Operators** — AWS-type operators use the intent system for Just-in-Time permission escalation (see [Cloud Operator & AWS Intents](#cloud-operator--aws-intents)). g8ep operators (`cloud_subtype=g8ep`) are a special type of cloud operator that provide direct system access and bypass the intent system.
- **Multi-operator** — multiple operators may be bound simultaneously; the AI selects the target per command using `target_operator` (hostname, operator ID, or index). Batch operations use `target_operators` for unified single-approval execution across N systems.

### Operator Not Bound

Advisory mode — no operator connected.

- **Limited tools** — `search_web` only (when `vertex_search_enabled=true` in `platform_settings` and credentials are configured); no execution capability.
- **No tools** — when `vertex_search_enabled` is not set or credentials are missing, zero tools are registered. The system prompt automatically uses the no-search variant (`capabilities_no_search.txt`, `execution_no_search.txt`) and suppresses the tools section entirely, preventing `MALFORMED_TOOL_CALL` from the model attempting an undeclared function.
- **Thinking** — same model-capability rules apply.
- **Behavior** — AI provides guidance, suggested commands, and analysis without executing anything.

#### Tool Availability Model

`AIToolService.search_web_available` (a `@property`) reflects whether `search_web` is registered in `_tool_declarations`. `ChatPipelineService` passes this flag to `build_modular_system_prompt`, which delegates it to `load_mode_prompts`. When `operator_not_bound` and `search_web_available=False`, `load_mode_prompts` swaps the capabilities and execution prompt files for their no-search variants — no string manipulation occurs at runtime.

```
vertex_search_enabled=true + credentials  →  search_web registered  →  search_web_available=True
                                             standard prompt files
                                             tools section included

vertex_search_enabled not set / missing  →  search_web not registered  →  search_web_available=False
                                             *_no_search.txt variant files
                                             tools section suppressed
```

---
## Investigation Context

### Pull and Enrichment

`InvestigationService` (`components/g8ee/app/services/investigation/investigation_context.py`) is the single entry point for building the context object the agent receives on every turn. It orchestrates `InvestigationDataService`, `OperatorDataService`, and `MemoryDataService` to assemble a complete picture of the current state.

**Step 1 — fetch:** `get_investigation_context` resolves the `InvestigationModel` via `InvestigationDataService` by `investigation_id` (preferred) or by `case_id` (falls back to the most-recently-created investigation). Lookup retries up to `INVESTIGATION_LOOKUP_MAX_RETRIES` times with configurable per-attempt delays to handle propagation lag.

**Step 2 — memory attach:** `_attach_memory_context` fetches the `InvestigationMemory` document for the investigation via `MemoryDataService` and attaches it to the `EnrichedInvestigationContext`. No memory is a valid state; the agent proceeds without it.

**Step 3 — operator enrichment:** `get_enriched_investigation_context` iterates `g8e_context.bound_operators`, loads each `OperatorDocument` via `OperatorDataService` (only `BOUND` status operators), and populates `operator_documents`. 

**Step 4 — operator context extraction:** `_extract_single_operator_context` maps an `OperatorDocument` (system info + latest heartbeat snapshot) to a typed `OperatorContext` — OS, hostname, architecture, CPU, memory, disk, username, shell, working directory, timezone, container environment, init system, and cloud-specific fields.

The resulting `EnrichedInvestigationContext` carries:
- `operator_documents` — list of live `OperatorDocument` records
- `memory` — the attached `InvestigationMemory` (or `None`)

For more details on how these documents are persisted, see [architecture/storage.md](../architecture/storage.md).

### Security

`get_investigation_context` logs a security warning if `user_id` is not provided — all user-facing queries must be scoped by `user_id` for tenant isolation.


## LLM Configuration

> **Recommended LLM: Google Gemini 3.1.** The platform was designed around Gemini best practices. The Gemini provider is the most robust and extensively tested integration. Other providers are supported but are not part of the standard test pipeline.

### Model Roles

| Role | Env Variable | Used For |
|------|-------------|----------|
| **Primary** | `LLM_MODEL` | All chat, reasoning, and operator-bound workflows |
| **Assistant** | `LLM_ASSISTANT_MODEL` | Triage, risk analysis, memory updates, Tribunal, title generation |

The assistant model always has thinking disabled regardless of capability.

### Per-Message Model Override

The UI exposes two separate model dropdowns: **Primary** (complex tasks) and **Assistant** (simple tasks). Each dropdown is populated with provider-specific model options. Selecting a model overrides the corresponding server default for that request. An empty selection (`""`) defers to the configured server default.

### LLM Config Discovery

On SSE connect, g8ed pushes a `llm.config` event containing provider-specific `primary_models` and `assistant_models` arrays plus the current defaults. The browser populates both model dropdowns from this event — no separate HTTP call is required.

### Triage & Routing

Before invoking the primary model, g8ee classifies each incoming message as `simple` or `complex` using the `triage_message` utility. This avoids the full model for messages that can be handled cheaply by the `assistant` model. 

**Classification Rules:**
- **Short-circuit:** Messages with attachments always escalate to the primary model (multimodal analysis).
- **Empty messages:** Escalated to primary model with a default follow-up question.
- **Complexity signals:** Assistant model looks for technical depth, reasoning chains, or explicit requests for action.
- **Short-circuit Follow-up:** If triage returns a `follow_up_question` with low confidence, `ChatPipelineService` delivers it immediately and stops further processing.

---

## Function Tools

### Active Tools

| Tool | Approval Required | Purpose |
|------|-------------------|---------|
| `run_commands_with_operator` | Yes | Execute shell commands on target systems |
| `file_create_on_operator` | Yes | Create new files with content |
| `file_write_on_operator` | Yes | Replace entire file contents |
| `file_update_on_operator` | Yes | Surgical find-and-replace within files |
| `file_read_on_operator` | No | Read file content (with optional line ranges) |
| `list_files_and_directories_with_detailed_metadata` | No | Directory listing with metadata |
| `fetch_file_history` | No | Retrieve file edit history and commit information |
| `fetch_file_diff` | No | Retrieve specific file diffs and change details |
| `check_port_status` | No | Check TCP/UDP port reachability |
| `grant_intent_permission` | Yes (via intent flow) | Request AWS intent permissions for cloud operators |
| `revoke_intent_permission` | Yes (via intent flow) | Revoke AWS intent permissions |
| `search_web` | No | Web search via Vertex AI Search — requires `vertex_search_enabled=true` in `platform_settings` |

Automatic Function Calling (AFC) is always disabled. g8ee uses a custom sequential function-calling loop to preserve thought signatures across multi-step operations.

---

## Operator Execution

### Operator Service Layer

`OperatorCommandService` (`app/services/operator/command_service.py`) is the entry point for operator tool execution. It is a pure injection target — business logic is owned by focused sub-services stored on `app.state` at startup.

`OperatorApprovalService` (`app/services/operator/approval_service.py`) is a first-class service on `app.state.approval_service`, independently constructed in `main.py` and injected into `OperatorCommandService.build()`. The g8ee router for `/api/internal/operator/approval/respond` depends on `OperatorApprovalService` directly — approval responses do not pass through `OperatorCommandService`.

The approval service exposes three typed request methods, each accepting a Pydantic model:
- `request_command_approval(CommandApprovalRequest)` — command execution approval
- `request_file_edit_approval(FileEditApprovalRequest)` — file operation approval
- `request_intent_approval(IntentApprovalRequest)` — IAM intent permission approval

All three models extend `ApprovalRequestBase` (`app/models/operators.py`) which carries common fields: `g8e_context`, `timeout_seconds`, `justification`, `execution_id`, `operator_session_id`, `operator_id`.

Approval responses use `handle_approval_response(OperatorApprovalResponse)` — a single typed model (`app/models/internal_api.py`) with `approval_id`, `approved`, `reason`, `operator_session_id`, and `operator_id`. The router enriches `operator_session_id`/`operator_id` from `G8eHttpContext.bound_operators[0]` before calling the service.

| Sub-service | Responsibility |
|-------------|----------------|
| `OperatorPubSubService` | Pub/sub lifecycle, channel subscription, command dispatch, result waiting |
| `OperatorApprovalService` | Human-in-the-loop approval request, poll, and response flow (first-class on `app.state`) |
| `OperatorExecutionService` | Command validation, risk analysis, batch execution, pub/sub command dispatch |
| `OperatorResultHandlerService` | Inbound result parsing from g8eo pub/sub messages |
| `OperatorFileService` | File create/write/update/read operations on the operator |
| `OperatorFilesystemService` | Directory listing (`fs_list`) and file read (`fs_read`) |
| `OperatorIntentService` | AWS intent permission grant and revocation |
| `OperatorLFAAService` | Local-First Audit Architecture event dispatch |
| `OperatorPortService` | TCP/UDP port reachability checks |

All service contracts are defined as `Protocol` types in `app/services/protocols.py`. The circular dependency between `OperatorPubSubService` and `OperatorResultHandlerService` is resolved by the factory via a single post-construction assignment — no `None` injection.


### Heartbeat Flow

g8ee is the persistence authority for heartbeats. It subscribes to `heartbeat:{operator_id}:{session}` channels, validates and persists each heartbeat (rolling buffer of last 10, latest snapshot, system info), then notifies g8ed for SSE fan-out to the browser. See [components/g8ed.md — Heartbeat Architecture](g8ed.md#heartbeat-architecture) for the full end-to-end flow including g8ed's role.

### Defensive Safety

Before dispatching any state-changing operation, g8ee runs AI-powered safety analysis: command risk classification (LOW / MEDIUM / HIGH, fails closed to HIGH), file operation safety (blocks writes to system paths and destructive ops on dirty git repos), and error analysis with auto-fix (maximum 2 retries before escalating to the user). See [architecture/security.md — Operator Commands via Sentinel](../architecture/security.md#operator-commands-via-sentinel-g8eo) for full details.

### MCP Adapter

> For comprehensive MCP architecture, provider-agnostic design, and translation layer patterns, see [architecture/mcp.md](../architecture/mcp.md).

g8ee implements an **MCP Client Adapter** that translates outbound tool calls into the Model Context Protocol (MCP) JSON-RPC 2.0 format. This is part of g8e' provider-agnostic event system design—MCP is one protocol translator among potentially many, all mapping to internal event types.

- **Tool Call Wrapping**: Outbound execution requests are wrapped in MCP `tools/call` JSON-RPC payloads.
- **Result Unwrapping**: Inbound `g8e.v1.operator.mcp.tools.result` events are unwrapped from JSON-RPC responses.
- **Structured Reconstruction**: g8ee uses a two-tier reconstruction logic for MCP results:
    1. **Structured Metadata (Preferred)**: Uses a g8e-specific `_metadata` field in the MCP result containing the `original_payload` and `event_type` for 100% reliable reconstruction.
    2. **Content Heuristics (Fallback)**: Parses serialized JSON from the `text` content using field-name heuristics (e.g., `host`, `entries`, `history`) to map back to internal typed payloads.
- **Integration**: The adapter is integrated into `OperatorCommandService` and `OperatorPubSubService`, making the transition to MCP transparent to the AI engine.
- **Governance**: Standards-based wire format while preserving g8e' secure pub/sub transport and human-in-the-loop governance layers.

### MCP Gateway Service

> For comprehensive MCP architecture, provider-agnostic design, and translation layer patterns, see [architecture/mcp.md](../architecture/mcp.md).

`MCPGatewayService` (`app/services/mcp/gateway_service.py`) enables external MCP clients (e.g. Claude Code) to execute g8e tools through the standard governance pipeline. It sits behind two internal endpoints on `internal_router.py`:

| Endpoint | Purpose |
|----------|---------|
| `POST /api/internal/mcp/tools/list` | Returns tool declarations formatted as MCP `tools/list` response items |
| `POST /api/internal/mcp/tools/call` | Executes a tool call through `AIToolService.execute_tool()` |

**Tool listing** reads `AIToolService.get_tools()` with the resolved `AgentMode` and converts each `ToolDeclarations` into MCP format (`{ name, description, inputSchema }`).

**Tool calling** builds a synthetic `EnrichedInvestigationContext` from `G8eHttpContext.bound_operators` (resolving operator documents from cache via `OperatorDataService`), then delegates to `AIToolService.execute_tool()`. The full governance pipeline runs: security validation, operator binding check, risk analysis, human approval gate, and audit logging. The result is converted to MCP `CallToolResult` format (`{ content: [{ type: "text", text }], isError }`).

**Initialization:** `MCPGatewayService` is created on `app.state.mcp_gateway_service` during startup, after `AIToolService` and `OperatorDataService`.

### Multi-Operator Binding

When multiple operators are bound, the AI must specify a `target_operator`. Resolution tries hostname match first, then exact operator ID, then list index. For batch operations, `target_operators` accepts a list (or `["all"]`) and g8ee requests a single unified approval covering all N target systems, then executes sequentially.

### Cloud Operator Self-Discovery

A small set of read-only AWS IAM introspection commands are auto-approved without user interaction (e.g., `aws sts get-caller-identity`, role/policy listing). All other AWS operations require explicit approval or an intent grant.

### Operator Execution History & Activity

g8ee tracks all operator-related actions and results to maintain a continuous picture of the environment.

#### Activity Log
The `add_operator_activity` method in `OperatorDataService` records high-level events (command execution, file edits, approvals) to the operator's `activity_log` array in g8es. These entries use the `ConversationHistoryMessage` model and are primarily used for UI visibility into what the AI has done on a specific system.

#### Command History
All command results are appended to `command_results_history` on the `OperatorDocument` via `append_command_result`. g8eo results are processed by `OperatorResultHandlerService` and routed to this history.

#### Local Retention (LFAA)
While g8es stores a summary of recent activity, the **Operator remains the system of record** via LFAA. g8ee dispatches audit events to the operator's local vault for long-term retention and cryptographic verification.

---

## Cloud Operator & AWS Intents

The Operator has two `OperatorType` values: **System** (`system` — cloud CLI tools blocked) and **Cloud** (`cloud` — cloud CLI tools enabled). Cloud operators carry an additional `cloud_subtype` field (`aws`, `gcp`, `azure`) identifying the provider. The intent system described in this section applies only to Cloud Operators with `cloud_subtype=aws`. g8ep operators (`cloud_subtype=g8ep`) have direct system access and do not use the intent system.

Cloud Operators for AWS implement a **Zero Standing Privileges** model. The Operator is started with `--cloud --provider aws` — either automatically on the g8ep sidecar (local dev, credentials from `~/.aws`) or deployed to an EC2 instance in the customer's VPC (credentials from IAM instance profile). In both cases the AI launches with only bootstrap permissions (STS identity, IAM role introspection) and dynamically requests additional permissions through the intent system.

### Intent Workflow

```mermaid
sequenceDiagram
    participant User
    participant AI as g8ee / AI
    participant g8ed
    participant AWS as IAM / AWS

    User->>AI: Request AWS operation
    AI->>AI: Check granted_intents in context
    AI->>g8ed: grant_intent_permission (intent, justification)
    G8ed->>User: Approval prompt in terminal UI
    User->>g8ed: Approve / Deny
    G8ed->>AI: Approval response
    AI->>AWS: Auto-attach intent policy via Escalation Role
    AI->>User: Proceed with original operation
```

### Two-Role Security Architecture

| Role | Attached To | Can Do | Cannot Do |
|------|------------|--------|-----------|
| **Operator Role** | EC2 instance profile or `~/.aws` credentials | Execute AWS operations (EC2, S3, RDS, etc.) | Modify its own IAM policies |
| **Escalation Role** | Assumed temporarily | Attach `Intent-*` prefixed policies to the Operator Role | Access any AWS resources directly |

The Escalation Role requires an external ID (prevents confused deputy attacks) and is only assumed during permission escalation — credentials are cleared immediately after.

This architecture ensures that even a compromised AI cannot grant itself arbitrary permissions; it can only attach pre-defined intent policies.

### Available Intents

Intents are grouped into discovery (read-only) and management (read-write) tiers. Management and write intents require their corresponding discovery/read intent as a prerequisite. The AI automatically requests all required intents together in a single approval.

**Compute**
- `ec2_discovery` — View EC2 instances, VPCs, subnets, security groups
- `ec2_management` — Start, stop, reboot instances *(requires ec2_discovery)*
- `ec2_snapshot_management` — Create and manage EC2/EBS snapshots and AMIs *(requires ec2_discovery)*

**Storage**
- `s3_read` — Read objects from S3
- `s3_write` — Write files to S3 *(requires s3_read)*
- `s3_delete` — Delete objects from S3 *(requires s3_read)*
- `s3_bucket_discovery` — List and view S3 bucket configurations
- `terraform_state` — Manage Terraform state in S3/DynamoDB

**IaC**
- `cloudformation_deployment` — Create and update CloudFormation stacks

**Monitoring & Logging**
- `cloudwatch_logs` — View and write CloudWatch Logs
- `cloudwatch_metrics` — Read CloudWatch metrics

**Secrets & Security**
- `secrets_read` — Read from AWS Secrets Manager
- `kms_discovery` — View KMS keys and aliases
- `kms_crypto` — Encrypt/decrypt with KMS *(requires kms_discovery)*
- `iam_discovery` — Read-only IAM access for debugging

**Database**
- `rds_discovery` — View RDS databases and clusters
- `rds_management` — Start, stop, reboot RDS *(requires rds_discovery)*
- `rds_snapshot_management` — Create and manage RDS snapshots *(requires rds_discovery)*
- `dynamodb_discovery` — View DynamoDB tables
- `dynamodb_read` — Query and scan DynamoDB
- `dynamodb_write` — Write to DynamoDB *(requires dynamodb_read)*

**Aurora**
- `aurora_cluster_management` — Manage Aurora clusters: failover, modify, add/remove instances *(requires rds_discovery)*
- `aurora_scaling` — Manage Aurora Serverless v2 scaling and capacity *(requires rds_discovery)*
- `aurora_cloning` — Clone Aurora clusters for dev/test/analysis *(requires rds_discovery)*
- `aurora_global_database` — Cross-region Aurora Global Database operations *(requires rds_discovery)*

**Containers & Orchestration**
- `ecs_discovery` — View ECS clusters, services, tasks
- `ecs_management` — Update services, run tasks *(requires ecs_discovery)*
- `eks_discovery` — View EKS clusters and nodegroups
- `lambda_discovery` — View Lambda functions
- `lambda_invoke` — Execute Lambda functions *(requires lambda_discovery)*

**Networking**
- `vpc_discovery` — View VPCs, subnets, route tables, security groups
- `elb_discovery` — View load balancers and target groups
- `route53_discovery` — View DNS hosted zones and records
- `route53_management` — Create/modify DNS records *(requires route53_discovery)*
- `autoscaling_discovery` — View Auto Scaling groups
- `autoscaling_management` — Scale ASGs *(requires autoscaling_discovery)*

**Messaging & Events**
- `sns_discovery` — View SNS topics
- `sns_publish` — Publish to SNS *(requires sns_discovery)*
- `sqs_discovery` — View SQS queues
- `sqs_management` — Send and manage SQS messages *(requires sqs_discovery)*
- `eventbridge_discovery` — View EventBridge rules and event buses

**Serverless & Analytics**
- `apigateway_discovery` — View API Gateway APIs
- `stepfunctions_discovery` — View Step Functions state machines
- `stepfunctions_execution` — Start and stop Step Functions executions *(requires stepfunctions_discovery)*
- `athena_discovery` — View Athena workgroups and queries
- `athena_query_execution` — Execute Athena queries *(requires athena_discovery)*
- `glue_discovery` — View Glue databases and crawlers

**Other**
- `elasticache_discovery` — View ElastiCache clusters
- `cloudfront_discovery` — View CloudFront distributions
- `codedeploy_discovery` — View CodeDeploy applications and deployments
- `acm_discovery` — View SSL/TLS certificates
- `cost_explorer` — View AWS cost and usage data

---

## Data Sovereignty & LFAA

### Sovereign Mode (Local Storage)

When local storage is enabled on an Operator (the default), all tool call outputs — command stdout/stderr, file contents, directory listings — are stored in the Operator's working directory. g8ee receives only metadata (hashes, sizes) with a `stored_locally=true` flag, not the raw data itself.

This means the **g8e platform never stores sensitive command output or file content**.

### Local-First Audit Architecture (LFAA)

LFAA extends sovereign mode to make the **Operator the system of record** for all conversation context. The platform is a stateless relay; no chat history or execution logs persist in the platform database.

> *"The Platform handles routing. The Operator handles retention."*

Key LFAA components on the Operator:
- **Audit Vault** — Local SQLite database (`{workdir}/.g8e/data/g8e.db`) storing all events: user messages, command executions, file mutations, and AI responses. Sensitive fields encrypted at rest.
- **Ledger** — Local Git repository (`{workdir}/.g8e/data/ledger`) providing cryptographic version history for every file the AI has modified.

For complete schema DDL, exact table/column definitions, vault encryption details (AES-256-GCM envelope encryption, KEK derivation, DEK wrapping), and data flow specifics, see [architecture/storage.md — g8eo Operator Storage](../architecture/storage.md#g8eo--operator-storage).

---

## Sentinel: Data Protection & Threat Detection

Sentinel is a dual-purpose security system that performs data scrubbing and pre-execution threat detection in a single scan pass.

- **g8ee Python scrubber** — scrubs sensitive data from user messages before they reach the AI (27 patterns: service tokens, cloud credentials, PII, connection strings, private keys).
- **g8eo Go sentinel** — scrubs command output before it leaves the Operator, and performs pre-execution threat detection mapped to MITRE ATT&CK categories. Threat detection is Go-only.
- **`sentinel_mode`** on an investigation controls whether the AI reads from the scrubbed vault or the raw vault. The Python bool is converted to the wire string format at the pub/sub boundary — never pass the raw bool to g8eo payloads. See [architecture/storage.md — Sentinel Mode and Vault Mode](../architecture/storage.md#sentinel-mode-and-vault-mode) for the conversion mapping.

For the full pattern list, threat categories, and scrubbed-vs-preserved data breakdown, see [architecture/security.md — Sentinel Output Scrubbing](../architecture/security.md#sentinel-output-scrubbing) and [architecture/security.md — Operator Commands via Sentinel](../architecture/security.md#operator-commands-via-sentinel-g8eo).

---

## Attachment Processing

g8ee processes multi-modal file attachments for LLM consumption.

### Flow

1. **g8ed** stores full `AttachmentData` JSON (including base64 data, filename, content type) in g8es Blob Store and forwards a `store_key` reference to G8EE.
2. **g8ee** retrieves the full attachment from Blob Store on demand, classifies the attachment type (PDF, image, text), and formats it as a `Part` object for the LLM provider.
3. **Blob Store** stores the complete attachment payload — both binary data and metadata in a single JSON object.

### Supported Types

| Type | LLM Format | Max Size |
|------|-----------|----------|
| PDF | Inline binary part | 10 MB |
| Images (PNG, JPEG, WebP, HEIC, GIF, BMP) | Inline binary part | 10 MB |
| Text (TXT, JSON, YAML, Python, JS, etc.) | Decoded text part | 5 MB |

---

## Data Connectivity

g8ee uses three distinct clients for data operations.

### Client Summary

| Client | Transport | Purpose |
|--------|-----------|---------|
| `DBClient` | HTTP | Document store — cases, investigations, operators, memories. All requests authenticated via `X-Internal-Auth` header. |
| `KVClient` | HTTP + WebSocket | KV store operations and pub/sub (command dispatch, results, heartbeats). All requests authenticated via `X-Internal-Auth` header. |
| `InternalHttpClient ` | HTTP | g8ee → g8ed internal API — SSE push, operator queries, heartbeat forwarding, intent management |

### KV Key Structure

All KV keys are versioned with a `v1` prefix. Keys are constructed exclusively via the `KVKey` builder class in `app/constants/kv_keys.py` — never construct key strings manually.

For the full KV key namespace (all patterns, builders, owners, TTLs) and the complete document collection registry, see [architecture/storage.md — KV Store](../architecture/storage.md#kv-store) and [architecture/storage.md — Document Store](../architecture/storage.md#document-store).

### Pub/Sub Channels

g8ee publishes commands to `cmd:{operator_id}:{operator_session_id}` and subscribes to the corresponding `results:*` and `heartbeat:*` channels. The canonical channel listing and wire format are in [components/g8es.md — Channel Naming Convention](g8es.md#channel-naming-convention).

#### Subscribe-and-Wait Contract

All channel subscriptions in g8ee follow a **subscribe-and-wait** flow: establish the connection, send the subscribe action, wait for the broker's `{"type":"subscribed","channel":"..."}` ack, then return. No publish or command dispatch may occur on a channel until the subscription ack has been received.

This is enforced by `KVClient.subscribe()` (`app/clients/db_client.py`):

1. Sends `{"action":"subscribe","channel":"..."}` over the shared WebSocket
2. Registers an `asyncio.Event` in `_ack_events[channel]`
3. `_ws_reader` routes incoming `{"type":"subscribed"}` frames to set the matching event
4. `subscribe()` awaits the event with a 5-second timeout before returning
5. The `_ack_events` entry is removed under `finally` — whether ack received or timed out

`OperatorPubSubService.register_operator_session()` calls `subscribe()` before adding the session to `_active_operator_sessions_set`. This guarantees that `publish_command()` is never called on a channel whose subscription has not been confirmed by the broker.

**Rule:** Never publish to a channel before `subscribe()` has returned. `subscribe()` returning is the proof that the broker has registered the subscription. Any test that calls `publish` after `subscribe` is race-free by construction — no `asyncio.sleep` is needed or permitted.

### Internal HTTP Communication (g8ed → g8ee)

g8ee communicates with other components via direct HTTP using `X-Internal-Auth` for authentication and standard `G8eHttpContext` headers.

#### Key Internal Endpoints

| Endpoint | Method | Purpose |
|----------|--------|---------|
| `/api/internal/chat` | POST | Primary non-streaming chat entry point; handles case/investigation creation and background AI processing |
| `/api/internal/chat/stop` | POST | Stops active AI processing for an investigation |
| `/api/internal/operator/approval/respond` | POST | Processes command approval/denial from g8ed |
| `/api/internal/operator/direct-command` | POST | Executes commands from the terminal UI (g8e Direct) |
| `/api/internal/operators/register-operator-session` | POST | Subscribes g8ee to heartbeat/result channels for a new session |

#### Authentication Discovery
g8ee discovers the authoritative `internal_auth_token` by reading `/g8es/ssl/internal_auth_token` at startup (or via `INTERNAL_AUTH_TOKEN` env var). This is the absolute source of truth for service-to-service authentication.

#### Context Propagation
The canonical header list and ownership rules are in [components/g8ed.md — Internal HTTP Communication](g8ed.md#internal-http-communication-g8ed--g8ee).

Key fields consumed by G8EE:

| Header | Required | Description |
|--------|----------|-------------|
| `X-G8E-WebSession-ID` | Yes | Browser session identifier |
| `X-G8E-User-ID` | Yes | User identifier |
| `X-G8E-Case-ID` | Yes | Case identifier — `UNKNOWN_ID` sentinel when `X-G8E-New-Case: true` |
| `X-G8E-Investigation-ID` | Yes | Investigation identifier — `UNKNOWN_ID` sentinel when `X-G8E-New-Case: true` |
| `X-G8E-New-Case` | No | `"true"` when this is the first message of a new conversation; absent otherwise |
| `X-G8E-Bound-Operators` | No | JSON array of all bound operators (id, session, status, hostname, type) |
| `X-G8E-Source-Component` | Yes | Source component name (must be a valid `ComponentName` value) |

`operator_id` and `operator_session_id` are **not** passed as individual headers. The full bound-operator list travels in `X-G8E-Bound-Operators`; individual operator resolution happens at execution time via `OperatorExecutionService.resolve_target_operator()`.

#### New Case Protocol (`X-G8E-New-Case`)

When a user sends their first message in a new conversation, no `case_id` or `investigation_id` exists yet. g8ed signals this by setting `X-G8E-New-Case: true` and sending `UNKNOWN_ID` sentinels for both `X-G8E-Case-ID` and `X-G8E-Investigation-ID`. g8ee reads `g8e_context.new_case` and branches into inline case and investigation creation.

**g8ed side** (`services/clients/internal_http_client.js` → `buildG8eContextHeaders`):
- Detects a new case when `context.case_id` is an empty string (the value set by `chat_routes.js` when no `case_id` was present in the request body)
- Sets `X-G8E-New-Case: true`, `X-G8E-Case-ID: unknown`, `X-G8E-Investigation-ID: unknown`
- Existing-case path: sets `X-G8E-Case-ID` and `X-G8E-Investigation-ID` to the real IDs; `X-G8E-New-Case` is omitted

**g8ee side** (`app/dependencies.py` → `get_g8e_http_context`):
- Reads `X-G8E-New-Case` and sets `G8eHttpContext.new_case = True`
- When `new_case=True`, relaxes the normal non-empty validation for `case_id` and `investigation_id` — `UNKNOWN_ID` sentinels are accepted
- When `new_case=False` (default), both IDs must be non-empty real values

**g8ee router** (`app/routers/internal_router.py` → `internal_chat`):
- Branches on `g8e_context.new_case` — creates `CaseModel` + `InvestigationModel` inline, stamps the new IDs onto a `model_copy` of `g8e_context`, pushes `CASE_CREATED` SSE to g8ed, enqueues background AI title generation, then proceeds to `run_chat` with the updated context
- Returns `ChatStartedResponse` with the new `case_id` and `investigation_id`

**Security:** The frontend cannot forge this signal. `chat_routes.js` makes the new-case determination server-side based on whether an authenticated request body contained a valid `case_id` string — client-supplied non-string values (`0`, `false`, etc.) are explicitly rejected by the type guard and treated as new-case.

#### Bound Operator Resolution Contract

`X-G8E-Bound-Operators` is the **exclusive source of truth** for which operators are available to the AI on any given request. g8ee performs no independent lookup against the operator document store to determine binding state.

To prevent header bloat, this header carries only minimal identity and status (`operator_id`, `operator_session_id`, `status`). Full metadata such as `operator_type` and `system_info` is fetched by g8ee from the shared KV cache when needed.

**How g8ee consumes it:**

1. `G8eHttpContext.parse_bound_operators` (`models/http_context.py`) parses the JSON array from the header into a `list[BoundOperator]` on every request.
2. `InvestigationService.get_enriched_investigation_context` (`services/investigation/investigation_context.py`) iterates `g8e_context.bound_operators`, filters to `status == BOUND`, fetches each operator's document via `OperatorDataService` for system info, and attaches `operator_documents` + `OperatorAvailability` to the investigation context.
3. `InvestigationService.determine_workflow_type` reads `investigation.operator_availability.operators_bound` — `True` → `WorkflowType.OPERATOR_BOUND`, `False` → `WorkflowType.OPERATOR_NOT_BOUND`. This determines whether the AI has execution capability or operates in advisory mode.
4. At command execution time, `_resolve_target_operator()` selects the specific operator from `g8e_context.bound_operators` based on the command target.

**Rule:** Never add a fallback that queries the operator document store by `web_session_id` to resolve binding state in G8EE. If `g8e_context.bound_operators` is empty, the session has no bound operators — that is the correct answer.

#### G8eHttpContext Application Barrier Contract

`G8eHttpContext` is the **single authoritative context object** within the g8ee application boundary. It must be passed intact to all internal service methods — never dismantled into individual loose parameters (`web_session_id`, `case_id`, `investigation_id`, `user_id`) and then re-assembled downstream.

**Rules:**

- `G8eHttpContext` is extracted from HTTP headers exactly once, at the FastAPI router (dependency injection via `get_g8e_http_context` / `get_g8e_http_context_for_chat`).
- All internal service methods (`ChatPipelineService._prepare_chat_context`, `_persist_ai_response`, `run_chat`, `_run_chat_impl`, etc.) receive the full `g8e_context: G8eHttpContext` object and derive fields from it directly.
- When new identifiers are created inline (e.g., `case_id` and `investigation_id` for a new conversation), the router updates `g8e_context` via `model_copy(update={...})` and passes the updated object downstream. No loose variables are threaded through.
- Never use `or ""` or any coercion to produce a fallback for a field that may be `None`. If a required field is absent, that is a caller contract violation — fix the caller or the model, not the consumer.

**LFAA audit guard:** `web_session_id` is a required, non-empty string in all LFAA audit event payloads. The pipeline dispatches LFAA events only when `web_session_id` is present (`if op_id and op_session and web_session_id`). No coercion to `""` is permitted.

### g8ed Internal API Methods

`InternalHttpClient ` exposes typed methods for all g8ee → g8ed communication:

| Method | Purpose |
|--------|---------|
| `push_sse_event` | Push a typed event to a browser session via g8ed's SSE relay |
| `get_operator_by_user_id` | Fetch the active operator for a user |
| `get_operator_status` | Fetch an operator document by ID |
| `update_operator_heartbeat` | Forward heartbeat telemetry for SSE broadcast |
| `update_operator_context` | Notify g8ed of context changes (case, investigation, task) |
| `grant_intent` / `revoke_intent` | Manage AWS intent permissions on a Cloud Operator |
| `bind_operators` | Bind operator(s) to a web session |
| `fetch_documentation` | Fetch auto-generated markdown docs from g8ed |

For services that push many event types (operator status, heartbeat, AI progress), use `EventService` (`services/infra/g8ed_event_service.py`), which wraps `push_sse_event` with typed construction.

---

## Prompt System

### Mode-Aware Prompts

System prompts are assembled from discrete files in `app/prompts_data/`, organized by mode and concern:

```
prompts_data/
  core/                     — Shared identity and safety constraints
  modes/
    operator_bound/         — Full execution capability prompts
    operator_not_bound/     — Advisory mode prompts
    cloud_operator_bound/   — AWS Cloud Operator prompts
  tools/                — Per-tool descriptions
  system/                   — Response constraints, sentinel mode
```

Each mode directory contains `capabilities.txt`, `execution.txt`, and `tools.txt`. The `operator_not_bound/` directory additionally contains `capabilities_no_search.txt` and `execution_no_search.txt` — loaded automatically by `load_mode_prompts` when `search_web_available=False`.

### Prompt Assembly

`app/llm/prompts.py` owns all runtime prompt construction. `build_modular_system_prompt` assembles the full system prompt by combining loaded prompt files with injected runtime context (operator system info, organization context, investigation state, learned memories). It is the only entry point for building system prompts — callers must not assemble prompt strings directly.

### Prompt Constants

All prompt file paths, agent modes, and section labels are defined in `shared/constants/prompts.json` — the authoritative source. g8ee loads this file and populates three enums in `app/constants/prompts.py`: `AgentMode`, `PromptSection`, and `PromptFile`.

**Rule:** Never hardcode prompt file paths or section label strings in application code. Always use the enums from `app/constants/prompts.py`. All new prompt files must have a corresponding `PromptFile` entry added to `shared/constants/prompts.json` before use.

---

## Web Search & Grounding

g8ee supports two distinct search modes with different data flows:

### Mode 1: `search_web` Function Tool (Vertex AI Search)

The AI explicitly calls `search_web` as a function tool. g8ee executes a Vertex AI Search (Discovery Engine) query via the `searchLite` SDK method against a pre-configured website data store and returns structured results to the LLM as a `ToolResponse` part. No grounding metadata is produced — results are plain text the model reasons over.

**Authentication:** Auth uses an explicit API key (`VERTEX_SEARCH_API_KEY`) passed as `google.auth.api_key.Credentials`. Application Default Credentials (ADC) are never used. The `searchLite` method is required for API key authentication — the `search` method requires OAuth 2.0 and will return `UNAUTHENTICATED` with an API key. See [Google's migration guide](https://cloud.google.com/generative-ai-app-builder/docs/migrate-from-cse) for background.

The recommended setup uses a single GCP API key restricted to both the Discovery Engine API and the Gemini for Google Cloud API — the same key serves as both `VERTEX_SEARCH_API_KEY` and `GEMINI_API_KEY`. Run `./g8e search setup` for a guided walkthrough.

#### Service Wiring

`WebSearchProvider` is constructed once at application startup in `lifespan()` (`app/main.py`) and stored on `app.state.web_search_provider`. It is never instantiated inside another service. `get_g8eeweb_search_provider()` in `app/dependencies.py` exposes it as a FastAPI dependency.

`AIToolService` accepts `web_search_provider: WebSearchProvider | None` as a constructor parameter. When `None`, `search_web` is not registered and the no-search prompt variants are used automatically. Construction is conditional on `VertexSearchSettings.is_configured` (requires `enabled=True`, non-empty `project_id`, `engine_id`, and `api_key`).

```
app startup (lifespan)
  └── VertexSearchSettings.is_configured?
        ├── yes → WebSearchProvider(project_id, engine_id, api_key, location)
        │          stored on app.state.web_search_provider
        │          injected into AIToolService via web_search_provider=
        └── no  → app.state.web_search_provider
                   AIToolService(web_search_provider=None)
                   search_web not registered
```

#### Resilience

`WebSearchProvider.search()` wraps the SDK call in a retry loop with exponential backoff:

- **Timeout:** each attempt is bounded by `WEB_SEARCH_CLIENT_TIMEOUT` via `asyncio.wait_for`
- **Retryable errors:** `asyncio.TimeoutError`, `google.api_core.exceptions.ServiceUnavailable`, `google.api_core.exceptions.ResourceExhausted` — retried up to `WEB_SEARCH_CLIENT_MAX_RETRIES` times with `WEB_SEARCH_CLIENT_RETRY_BACKOFF` base delay
- **Non-retryable errors:** any other `GoogleAPICallError` (e.g. `InvalidArgument`, `Unauthenticated`, `PermissionDenied`) — raises `NetworkError` immediately; callers receive structured error context
- **Exhausted retries:** returns `SearchWebResult(success=False, error=...)` after all attempts fail

#### Result Contract

`SearchWebResult.results` is always `list[WebSearchResultItem]` — never `None`. On failure, `results` is an empty list. Callers must not test for `None`; test `result.success` to distinguish success from failure.

- **Registered by:** `AIToolService._build_search_web_tool()`
- **Executor:** `WebSearchProvider` (`app/services/ai/grounding/web_search_provider.py`)
- **SDK method:** `SearchServiceClient.search_lite()` (API key auth)
- **Result type:** `SearchWebResult` (`success`, `query`, `results: list[WebSearchResultItem]`, `total_results`)
- **Config required:** `vertex_search_enabled=true`, `vertex_search_project_id`, `vertex_search_engine_id`, `vertex_search_api_key` (all in `platform_settings`)
- **Available in:** both operator-bound and operator-not-bound modes

### Mode 2: Native Gemini Grounding (Google Search Tool)

When using a Gemini provider with the `google_search` SDK tool enabled in `GenerateContentConfig`, the model retrieves live search results internally and attaches `SdkGroundingRawData` to the response at the provider boundary. This produces rich grounding metadata with source attribution.

- **Extraction:** `GeminiProvider` populates `GenerateContentResponse.grounding_raw` with `SdkGroundingRawData`
- **Processing:** `GroundingService.extract_provider_grounding()` converts `SdkGroundingRawData` → canonical `GroundingMetadata`
- **Citations:** `WebSearchProvider.add_inline_citations()` strips LLM auto-citations, assigns sequential `[N]` markers at segment boundaries, and populates `GroundingMetadata.sources` with resolved `GroundingSourceInfo` (domain, favicon, citation number)
- **Wire delivery:** `StreamChunkData.grounding_metadata` carries the result; `citations` SSE chunk delivers it to the browser

### Model Boundaries

| Model | Lives In | Consumes | Produces |
|-------|----------|----------|----------|
| `SdkGroundingRawData` | `app/llm/llm_types.py` | `GeminiProvider` only | Attached to `GenerateContentResponse.grounding_raw` |
| `GroundingMetadata` | `app/models/grounding.py` | `WebSearchProvider`, `GroundingService`, `g8e agent`, wire | Canonical platform model |
| `GroundingSourceInfo` | `app/models/grounding.py` | Browser renderer | Resolved source with citation number and favicon |
| `SearchWebResult` | `app/models/tool_results.py` | `AIToolService`, LLM boundary | Typed result from `search_web` tool call |

**`AIResponseAnalyzer` does not handle grounding.** All grounding and citation logic is owned exclusively by `WebSearchProvider` and `GroundingService` (`app/services/ai/grounding/`).

---

## Configuration Reference

### Config Source

g8ee loads its runtime configuration from the `platform_settings` document in g8es (`components` collection, document ID `platform_settings`), under a `settings` key. This is read asynchronously at startup via `Settings.from_db(cache_aside_service)` with up to 20 retries (3s interval) before falling back to hardcoded defaults.

SSL/TLS cert paths (`SSLSettings`) are not loaded from g8es — they are resolved from the filesystem at property access time and configured at the container/deployment level.

**Boolean coercion:** all boolean fields in `platform_settings` are stored as strings. The values `"false"`, `"False"`, `"FALSE"`, and `"0"` map to `False`; all other non-empty strings map to `True`. This matches g8ed's `USER_SETTINGS` `select` options exactly.

### Service Connections

These are deployment-level configuration (environment variables / Docker Compose — not stored in g8es):

| Variable | Default | Purpose |
|----------|---------|---------|
| `G8E_INTERNAL_HTTP_URL` | `https://g8es` | g8ed HTTP URL |
| `G8E_INTERNAL_PUBSUB_URL` | `wss://g8es` | g8es WebSocket pub/sub URL |

### platform_settings Keys

The following keys are read from the `settings` map inside the `platform_settings` g8es document. g8ed owns writing this document; g8ee reads it. All values are strings; booleans use `"true"`/`"false"`, numbers use their string representation.

#### LLM Provider (`LLMSettings`)

| Key | Default | Description |
|-----|---------|-------------|
| `llm_provider` | `ollama` | The active LLM provider (`ollama`, `openai`, `anthropic`, `gemini`) |
| `ollama_model` | `gemma4:e4b` | The model name for Ollama |
| `ollama_assistant_model` | `gemma4:e4b` | The assistant model name for Ollama |
| `ollama_endpoint` | `http://host.docker.internal:11434` | The Ollama API endpoint |
| `openai_endpoint` | `https://api.openai.com/v1` | The OpenAI API endpoint |
| `openai_api_key` | - | OpenAI API key |
| `anthropic_endpoint` | `https://api.anthropic.com/v1` | The Anthropic API endpoint |
| `anthropic_api_key` | - | Anthropic API key |
| `gemini_api_key` | - | Google Gemini API key |
| `llm_temperature` | `1.0` | Sampling temperature |
| `llm_max_tokens` | `1000000` | Maximum tokens per response |

#### Other Settings

| Key | Default | Description |
|-----|---------|-------------|
| `vertex_search_enabled` | `false` | Enable/disable Vertex AI Search |
| `vertex_search_project_id` | - | GCP Project ID for Vertex Search |
| `vertex_search_engine_id` | - | Discovery Engine ID |
| `vertex_search_api_key` | - | API Key for Vertex Search |

#### Tribunal Command Generator (`LLMSettings`)

| Key | Default | Purpose |
|-----|---------|---------|
| `command_gen_enabled` | `true` | Master switch for Tribunal command generation |
| `command_gen_passes` | `3` | Number of independent generation passes (1–10) |
| `command_gen_verifier` | `true` | Enable the SLM verifier pass |

Temperatures are fixed per Tribunal member and are not configurable. Values are sourced from shared/constants/agents.json (single source of truth across g8ee, g8ed, and g8eo): Axiom → 0.0, Concord → 0.4, Variance → 0.8.

**Model resolution:** The Tribunal uses the assistant model. If `assistant_model` is not configured, it falls back to `primary_model`, then to the provider's default model. A concrete model string is always resolved before the pipeline starts.

#### Security & Auth (`AuthSettings`, `CommandValidationSettings`)

| Key | Default | Purpose |
|-----|---------|---------|
| `internal_auth_token` | — | Component-to-component auth (`X-Internal-Auth` header); required before g8ee accepts requests |
| `g8e_api_key` | — | Optional API key for external client authentication |
| `enable_command_whitelisting` | `false` | Restrict commands to an allowlist |
| `enable_command_blacklisting` | `false` | Block commands matching a denylist |

#### Vertex AI Search (`VertexSearchSettings`)

| Key | Default | Purpose |
|-----|---------|---------|
| `vertex_search_enabled` | `false` | Enable `search_web` tool via Vertex AI Search (Discovery Engine) |
| `vertex_search_project_id` | — | GCP project ID hosting the Vertex AI Search data store |
| `vertex_search_engine_id` | — | Vertex AI Search app/engine ID |
| `vertex_search_location` | `global` | Data store location (`global` for global data stores) |
| `vertex_search_api_key` | — | GCP API key restricted to Discovery Engine API; can be shared with `gemini_api_key` |

`VertexSearchSettings.is_configured` requires `enabled=True` and all three of `project_id`, `engine_id`, `api_key` non-empty. Only when configured is `WebSearchProvider` constructed and `search_web` registered.

#### Google Programmable Search Engine (`GoogleSearchSettings`)

| Key | Default | Purpose |
|-----|---------|---------|
| `google_search_enabled` | `false` | Enable `search_web` tool via Google Programmable Search Engine |
| `google_search_api_key` | — | Google Cloud API key with Custom Search JSON API enabled |
| `google_search_engine_id` | — | Programmable Search Engine `cx` value |

`GoogleSearchSettings.is_configured` requires `enabled=True` and both `api_key` and `engine_id` non-empty.

---

## Testing

See [testing.md — g8ee](../testing.md#g8ee--python) for test infrastructure, pytest fixtures, mock locations, markers, AI integration test details, and how to run tests.

### Test Coverage

**Current test count:** 2,461 passing tests (unit + integration)

**Well-covered areas:**
- AI tool registration and execution (including `fetch_file_history` and `fetch_file_diff`)
- Chat pipeline and streaming architecture
- Operator command execution and approval flows
- Investigation context assembly and memory management
- LLM provider abstraction and grounding
- SSE event delivery and lifecycle management
- Web search integration and citation processing
- Pub/sub communication patterns
- Security validation and threat detection

**Test types:**
- **Unit tests** - Business logic isolation with mocked external boundaries
- **Integration tests** - Real g8es and service wiring
- **AI integration tests** - Real LLM provider calls (credentials required)
- **Contract tests** - Wire protocol and constants enforcement
