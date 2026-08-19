# Development

## Setup

```bash
# Create virtual environment
python3 -m venv venv
source venv/bin/activate

# Install with dev dependencies
pip install -e ".[dev,test]"

# Install pre-commit hooks
pre-commit install

# Generate protobuf stubs from g8e protocol definitions
make proto
```

## g8e Package Dependency

g8ee depends on the `g8e` Python package (`g8e>=1.5.6`) as the single source of truth for protocol constants, enums, and models. The package is installed from PyPI or as an editable install from the protocol repository.

## Model Hierarchy

Base models are sourced from `g8e.models.base` and re-exported through `app.models.base`:

- **`G8eBaseModel`** — from `g8e.models.base`, re-exported via `app.models.base`. All g8ee models inherit from this.
- **`UTCDatetime`** — from `g8e.models.base`, re-exported via `app.models.base`.
- **Pydantic primitives** (`ConfigDict`, `Field`, `ValidationError`, `field_validator`, `model_validator`, `BaseModel`, `PrivateAttr`, `TypeAdapter`, `ValidationInfo`, `computed_field`) — all re-exported via `app.models.base`. All `from pydantic import` in `app/` routes through `app.models.base` (except `app/models/base.py` itself, which is the re-export hub).

g8ee-specific subclasses extend the g8e bases:

- **`RequestContext`** — subclasses `g8e.models.context.RequestContext`, adds `operator_id` and `operator_session_id`
- **`BoundOperator`** — directly re-exported from `g8e.models.context`
- **`ChatMessageRequest`** — multiple inheritance: `g8e.models.internal_api.ChatMessageRequest` + `RequestOverrides` mixin
- **`ResourceCreationRequest`**, **`ChatStartedResponse`** — directly re-exported from `g8e.models.internal_api`
- **Settings models** — subclassed from `g8e.models.settings`
- **SSE wire models** — `SessionEventWire` and `BackgroundEventWire` subclass `g8e.models.events`; all 11 payload classes re-exported from `g8e.models.events`

## Constants Sourcing

Protocol constants are sourced from `g8e.constants` accessors:

- **DB collections** — `g8e.constants.collection()` for 17 shared collections; 2 g8ee-specific (`api_keys`, `tribunal_commands`)
- **KV keys** — `g8e.constants.kv_key()` for 23 shared key patterns; 2 g8ee-specific (`cli_session`, `operator_slot_counter`)
- **Channels** — `g8e.constants.channel()` for 9 shared channels; 7 g8ee-specific
- **Intents** — `g8e.constants.intent()` for all 52 `CloudIntent` values
- **Prompts** — `g8e.constants.prompt()` for 14 `PromptSection` values and 3 `AgentMode` values; `SENTINEL_MODE` is g8ee-specific
- **API paths** — `GatewayAPIPaths` class wraps `g8e.constants.API_PATHS`; `InternalAPIPaths` for g8ee-internal routing
- **Enums** — 12 enums re-exported from `g8e.enums` (`CloudSubtype`, `ConversationStatus`, `EscalationRisk`, `ExecutionStatus`, `FileOperation`, `HealthStatus`, `NetworkProtocol`, `AttachmentType`, `ToolDisplayCategory`, `ToolCallStatus`, `ThinkingActionType`, `ApprovalErrorType`); 3 left as g8ee-specific (`ApprovalType`, `InfrastructureStatus`, `AuthMethod`)

## Protobuf Stubs

Generated Python protobuf stubs from the g8e protocol `.proto` files are placed in `app/proto/`. This directory is gitignored (generated files, not checked in).

- Regenerate with: `make proto`
- Stubs: `common_pb2.py`, `operator_pb2.py`, `pubsub_pb2.py`
- Re-exported via `app/proto/__init__.py`

## Coding Standards

- **Linter:** Ruff (`ruff check .`)
- **Formatter:** Ruff (`ruff format .`)
- **Type checker:** Pyright (`pyright`)
- **Pre-commit:** Runs ruff + pyright on staged files
- **Pydantic imports:** All `from pydantic import` in `app/` must route through `app.models.base` re-exports (except `app/models/base.py` itself)

## Dependency Groups

- `dev` — Development tools (ruff, pyright, pre-commit)
- `test` — Testing framework (pytest, pytest-cov, etc.)
- `docs` — Documentation tools (mkdocs, mkdocs-material)

```bash
pip install -e ".[dev,test,docs]"
```

## Project Conventions

- Follow existing code style — let Ruff and Pyright guide you
- Keep changes minimal and focused
- Add tests for new functionality
- Update documentation when interfaces change
- Source protocol constants from `g8e.constants` — do not hardcode protocol strings
- All business-critical DB writes go through `GovernanceClient` governance envelopes

## Related

- [Testing](tests.md) — Testing framework and practices
- [Evals](evals.md) — Evaluation suite
- [Getting Started](getting-started.md) — Initial setup guide
