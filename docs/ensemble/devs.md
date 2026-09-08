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

## Constants

The in-tree `g8e` Python package is the source of truth for constants and enums shared with the Gateway and Operator. Ensemble modules import collection names, document identifiers, key-value key patterns, channels, intents, prompt identifiers, Gateway API paths, HTTP headers, component names, and protocol enums from `g8e.constants` or `g8e.enums`. The dependency resolves to `protocol/python/` through `ensemble/pyproject.toml` during local development.

`ensemble/app/constants/` also contains values that belong only to the ensemble, including internal API paths, provider configuration, runtime path resolution, environment variable names, conversation sender identifiers, and mappings between protocol events and action types. A value remains local only when the shared protocol has no equivalent. Shared protocol strings are not duplicated locally.

Tests in `ensemble/tests/unit/constants/` verify the ensemble accessors and protocol alignment. `ensemble/tests/test_constants_parity.py` validates the JSON registries in `protocol/constants/` against the ensemble's typed registry models.

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
- [Prompts](prompts.md) — System prompt assembly and persona templating
- [Thinking](thinking.md) — L2 consensus, provider reasoning, and thought signatures
- [PKI & Trust](pki.md) — Public Key Infrastructure, trust bundles, and workload enrollment
- [Storage](storage.md) — Storage tiers and data sovereignty principles
- [LLM Providers](llm-providers.md) — Provider implementations and capacity tiers
- [Server-Sent Events](sse.md) — Real-time event streaming pipeline and Gateway push delivery
- [Testing](tests.md) — Testing framework, test tiers, and practices
- [Evals](evals.md) — Benchmark evaluation suite and Judge scoring rubrics
- [Getting Started](getting-started.md) — Initial setup guide
