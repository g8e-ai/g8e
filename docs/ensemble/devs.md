# Development

## Overview

The g8e Agentic Ensemble (`g8ee`) is implemented in Python as a FastAPI service situated in-tree under `ensemble/`. It serves as the primary reasoning and decision-making engine for the g8e platform, communicating with the Governance Gateway (`g8eg`) and Governed Operator (`g8eo`) via mTLS, signed `GovernanceEnvelope` transactions, SSE streaming, and pub/sub messaging.

The ensemble relies on the in-tree `g8e` Python protocol package (`protocol/python/`) as the single source of truth for protocol constants, wire models, enums, and protobuf schemas. See [Protocol Reference](../architecture/protocol.md) for the platform-level protocol specification.

## Setup

```bash
# Navigate to the ensemble directory
cd ensemble

# Create and activate a Python 3.12+ virtual environment
python3 -m venv .venv
source .venv/bin/activate

# Install the in-tree g8e protocol package in editable mode, followed by ensemble dev and test dependencies
pip install -e ../protocol/python
pip install -e ".[dev,test,docs]"

# Install pre-commit hooks
pre-commit install

# Generate Python protobuf stubs from protocol definitions
make proto
```

Alternatively, running `make setup` from the `ensemble/` directory installs the editable protocol package and all dev/test dependencies automatically.

## g8e Package Dependency

g8ee depends on the `g8e` Python package (`g8e>=1.7.8`) as the single source of truth for protocol constants, enums, and models. In the repository monorepo structure, `pyproject.toml` configures `[tool.uv.sources]` to resolve `g8e` directly to `../protocol/python`. In container builds, the Dockerfile installs `protocol/python/` before `ensemble/` so dependencies resolve to the local in-tree package without requiring external PyPI distribution.

## Model Hierarchy

Base models are sourced from `g8e.models.base` and re-exported through `app.models.base`, which acts as the central model foundation and Pydantic import hub:

- **`G8eBaseModel`** — Base model from `g8e.models.base`, re-exported via `app.models.base`. Extends Pydantic's `BaseModel` with protojson-compatible serialization (`exclude_none=True` by default) and UTC normalization. All ensemble models inherit from this.
- **`UTCDatetime`** — Type alias from `g8e.models.base`, re-exported via `app.models.base`. Serializes datetimes to ISO 8601 with a `Z` suffix in UTC canonical form.
- **`G8eTimestampedModel`** — Base lifecycle model in `app.models.base` adding UTC timestamp fields (`created_at`, `updated_at`) and helper method `update_timestamp()`.
- **`G8eIdentifiableModel`** — Persisted entity base in `app.models.base` extending `G8eTimestampedModel` with a stable UUID4 document identifier (`id`) and `generate_id(prefix)` helper.
- **`G8eAuditableModel`** — Actor-tracking base in `app.models.base` extending `G8eIdentifiableModel` with `created_by` and `updated_by` fields and `update_audit_info()` helper.
- **`recursive_serialize`** — Utility in `app.models.base` for boundary crossing and flattening nested structures with datetime serialization.
- **Pydantic primitives** (`ConfigDict`, `Field`, `ValidationError`, `field_validator`, `model_validator`, `BaseModel`, `PrivateAttr`, `TypeAdapter`, `ValidationInfo`, `computed_field`) — Re-exported through `app.models.base`. All `from pydantic import` statements across `app/` route through `app.models.base` (except `app/models/base.py` itself).

Ensemble-specific models extend the protocol base models:

- **`RequestContext`** — Subclasses `g8e.models.context.RequestContext` in `app.models.http_context`, adding `operator_id` and `operator_session_id` for governance envelope routing while defaulting `source_component` to `g8ee`.
- **`G8eHttpContext`** — Standard request context model in `app.models.http_context` handling session mutual exclusivity, identity validation against authenticated callers, and conversion to `RequestContext`.
- **`BoundOperator`** — Re-exported directly from `g8e.models.context`.
- **`ChatMessageRequest`** — Defined in `app.models.internal_api` via multiple inheritance: subclasses `g8e.models.internal_api.ChatMessageRequest` and `RequestOverrides` mixin, overriding attachments with typed `list[AttachmentMetadata]`.
- **`ResourceCreationRequest` and `ChatStartedResponse`** — Directly re-exported from `g8e.models.internal_api`.
- **Settings models** — Subclasses of protocol definitions from `g8e.models.settings` in `app.models.settings` (`CommandValidationSettings`, `SearchSettings`, `EvalJudgeSettings`, `LLMSettings`, `BatchExecutionSettings`, `G8eeUserSettings`).
- **SSE wire models** — `SessionEventWire` and `BackgroundEventWire` in `app.models.events` subclass `g8e.models.events` to wrap internal `SessionEvent` and `BackgroundEvent` routing envelopes; all 11 SSE payload classes (`AiProcessingStoppedPayload`, `AIToolLifecyclePayload`, `ChatCitationsReadyPayload`, `ChatErrorPayload`, `ChatProcessingStartedPayload`, `ChatResponseChunkPayload`, `ChatResponseCompletePayload`, `ChatRetryPayload`, `ChatThinkingPayload`, `ChatTurnCompletePayload`, `TriageClarificationQuestionsPayload`) are re-exported from `g8e.models.events`.

## Constants Sourcing

Protocol constants and enums are sourced directly from `g8e.constants` accessors and `g8e.enums` dynamic loaders:

- **DB collections** (`app/constants/collections.py`) — Sourced via `g8e.constants.collection()` for 17 shared collections (`settings`, `users`, `web_sessions`, `operator_sessions`, `cli_sessions`, `organizations`, `operators`, `operator_usage`, `cases`, `investigations`, `tasks`, `memories`, `revoked_certificates`, `agent_activity_metadata`, `reputation_state`, `reputation_commitments`, `stake_resolutions`); 2 g8ee-specific collections (`api_keys`, `tribunal_commands`); document IDs via `g8e.constants.document_id()` (`platform_settings`, `user_settings_prefix`).
- **KV keys** (`app/constants/kv_keys.py`) — Sourced via `g8e.constants.kv_key()` for 24 accessor methods wrapping protocol keys and `CachePrefix`; 2 g8ee-specific keys (`cli_session`, `operator_slot_counter`); static prefixes derived via `KVKeyPrefix`. Key patterns use `sessions` (plural) per protocol convention.
- **Channels** (`app/constants/channels.py`) — Sourced via `g8e.constants.channel()` for 9 shared channels (`Governance`, `OperatorIntent`, `OperatorDevice`, `SseEvent`, `StorageDocument`, `StorageKv`, `StorageBlob`, `Error`, `Message`); 7 g8ee-specific channels (`cmd`, `results`, `heartbeat`, `g8eo_results`, `operator_heartbeats`, `sse_events`, `system_events`); pubsub enums (`PubSubChannel`, `PubSubAuthPrefix`, `PubSubAction`, `PubSubWireEventType`, `PubSubField`, `PubSubMessageType`).
- **Intents** (`app/constants/intents.py`) — Sourced via `g8e.constants.intent()` for all 52 `CloudIntent` values, alongside dependency mappings (`CLOUD_INTENT_DEPENDENCIES`), confirmation prompts (`CLOUD_INTENT_QUESTIONS`), and IAM verification actions (`CLOUD_INTENT_VERIFICATION_ACTIONS`).
- **Prompts** (`app/constants/prompts.py`) — Sourced via `g8e.constants.prompt()` for 14 `PromptSection` values and 3 `AgentMode` values; 1 g8ee-specific section (`SENTINEL_MODE`); prompt file path mappings (`PromptFile`, `AGENT_MODE_PROMPT_FILES`); UI context labels (`InvestigationContextLabel`).
- **API paths** (`app/constants/api_paths.py`) — `GatewayAPIPaths` class wraps `g8e.constants.API_PATHS` for Gateway route lookups; `InternalAPIPaths` provides typed access for g8ee-internal and client routing defined in `api_paths.json`.
- **Protocol enums** (`app/constants/generated_status.py`, `app/constants/config.py`, `app/constants/platform.py`, `app/constants/errors.py`) — Sourced from `g8e.enums`, including `EventType` (297 members), `SessionType`, `ErrorCode`, `ErrorCategory`, `ErrorSeverity`, `AuthMethod`, `CloudSubtype`, `ConversationStatus`, `EscalationRisk`, `ExecutionStatus`, `FileOperation`, `HealthStatus`, `InfrastructureStatus`, `NetworkProtocol`, `AttachmentType`, `ToolDisplayCategory`, `ToolCallStatus`, `ThinkingActionType`, `ApprovalErrorType`, and `ApprovalType`.
- **Operator tools** (`app/constants/generated_status.py`) — `OperatorToolName` re-exports 19 protocol tool names from `g8e.enums.OperatorToolName` extended with 2 ensemble-specific tool identifiers (`GRANT_INTENT`, `REVOKE_INTENT`).
- **HTTP headers** (`app/constants/__init__.py`) — 32 canonical headers sourced from `g8e.constants` (`AUTHORIZATION`, `CASE_ID`, `INVESTIGATION_ID`, `SOURCE_COMPONENT`, `SYSTEM_FINGERPRINT`, `WEB_SESSION_ID`, `CLI_SESSION_ID`, `OPERATOR_ID`, etc.); 3 ensemble-local reverse-proxy headers (`X_PROXY_USER_EMAIL`, `X_PROXY_CLI_SESSION_ID`, `X_PROXY_WEB_SESSION_ID`).
- **Component attribution** (`app/constants/generated_status.py`, `app/constants/config.py`) — `ComponentName` imported from `g8e.constants` (`CLIENT`, `G8EO`, `G8EO_GATEWAY`); `G8EE_COMPONENT = "g8ee"` defined locally for outbound requests.

## Protobuf Stubs

Generated Python protobuf stubs from the g8e protocol `.proto` definitions are placed in `app/proto/`. This directory is gitignored (generated artifacts, not committed to source control).

- Regenerate with: `make proto` (from `ensemble/` or root)
- Input proto files: `protocol/proto/g8e/common/v1/common.proto`, `protocol/proto/g8e/operator/v1/operator.proto`, `protocol/proto/g8e/pubsub/v1/pubsub.proto`
- Output stubs: `common_pb2.py`, `operator_pb2.py`, `pubsub_pb2.py`
- Re-exported via: `app/proto/__init__.py`

## Development Commands

### From the Repository Root

```bash
# Run Tier 1 + Tier 2 unit and in-process integration tests
make ensemble-test

# Run Tier 4 external tests (real LLM/API calls, gated on credentials)
make test-external

# Run Ruff linter and Pyright type checker on the ensemble
make ensemble-lint

# Build the ensemble container image
make build-ensemble
```

### From the `ensemble/` Directory

```bash
# Run tests
make test

# Run linter and type checker
make lint

# Auto-format code with Ruff
make format

# Run format, lint, and test sequentially
make check

# Clean __pycache__, .pyc, and egg-info artifacts
make clean

# Generate protobuf stubs
make proto
```

## Coding Standards

- **Linter:** Ruff (`ruff check app tests evals`) configured in `pyproject.toml`
- **Formatter:** Ruff (`ruff format app tests evals`) with double quotes, 4-space indentation, and 100-character line length
- **Type checker:** Pyright (`pyright app`) with strict typing rules
- **Pre-commit:** Enforces Ruff linting, Ruff formatting, and Pyright validation on staged files
- **Pydantic imports:** All `from pydantic import` statements in `app/` must route through `app.models.base` re-exports (except `app/models/base.py` itself)
- **Error handling:** Return and check centralized error codes from `app.constants.errors` and raise typed exceptions from `app.errors`
- **Governance transactions:** All business-critical state mutations (case updates, operator commands, file edits, memories) route through `GovernanceClient` via signed `GovernanceEnvelope` structures

## Dependency Groups

The project specifies dependencies in `ensemble/pyproject.toml` organized into functional optional groups:

- `dev` — Development tools (`ruff`, `pyright`, `pre-commit`)
- `test` — Testing framework (`pytest`, `pytest-cov`, `pytest-asyncio`, `pytest-mock`, `pytest-timeout`, `pytest-xdist`)
- `docs` — Documentation tools (`mkdocs`, `mkdocs-material`, `mkdocstrings[python]`)
- `embeddings` — Optional embedding models (`sentence-transformers`)

```bash
pip install -e ".[dev,test,docs]"
```

## Project Conventions

- Follow existing code patterns — let Ruff and Pyright guide type safety and formatting
- Keep changes minimal, focused, and covered by tests
- Add unit tests for new functionality and regression tests for bug fixes
- Update documentation in `docs/ensemble/` when interfaces or models change
- Source protocol constants and enums from `g8e.constants` and `g8e.enums` — never hardcode protocol strings
- Never bypass the 5-layer verification pipeline or commit unstaged mutations directly

## Related

- [Platform Developer Guidelines](../devs/devs.md) — g8e platform-wide developer guidelines, coding standards, and conventions
- [Code Map](../devs/codemap.md) — Platform-wide codebase map and component directory structure
- [Protocol Reference](../architecture/protocol.md) — Platform-level canonical wire contracts and protobuf schema definitions
- [Architecture](architecture.md) — System architecture, protocol surfaces, and model hierarchy
- [Governance](governance.md) — Five-layer verification pipeline and envelope validation
- [Agents](agents.md) — Agent hierarchy, personas, and Tribunal consensus
- [Constants](constants.md) — Sourced protocol constants and application definitions
- [Prompts](prompts.md) — System prompt assembly and persona templating
- [Thinking](thinking.md) — L2 consensus, provider reasoning, and thought signatures
- [PKI & Trust](pki.md) — Public Key Infrastructure, trust bundles, and workload enrollment
- [Storage](storage.md) — Storage tiers and data sovereignty principles
- [LLM Providers](llm-providers.md) — Provider implementations and capacity tiers
- [Server-Sent Events](sse.md) — Real-time event streaming pipeline and Gateway push delivery
- [Testing](tests.md) — Testing framework, test tiers, and practices
- [Evals](evals.md) — Benchmark evaluation suite and Judge scoring rubrics
- [Getting Started](getting-started.md) — Initial setup guide
