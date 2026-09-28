---
doc_id: ensemble-devs
title: Ensemble Development Guide
audience: maintainers and coding agents
status: current
last_updated: 2026-09-28
version: v2.2.3
owners:
  - ensemble/
  - ensemble/app/
  - ensemble/tests/
  - ensemble/Makefile
related:
  - docs/ensemble/index.md
  - docs/ensemble/architecture.md
  - docs/ensemble/agents.md
  - docs/ensemble/governance.md
  - docs/ensemble/protocol.md
  - docs/ensemble/sse.md
  - docs/ensemble/tests.md
  - docs/ensemble/getting-started.md
  - docs/devs/devs.md
  - docs/devs/docs.md
when_to_read: Understanding ensemble architecture, developing ensemble features, debugging service wiring, authoring tests, or reviewing code changes to ensemble application code.
do_not_use_for:
  - Platform-wide coding invariants (docs/devs/devs.md)
  - Component architecture boundaries and integration (docs/ensemble/architecture.md)
  - LLM provider implementation details (docs/ensemble/llm-providers.md)
  - Agent personas and governance enforcement (docs/ensemble/agents.md)
  - Test organization and fixture setup (docs/ensemble/tests.md)
---

# Development

## Scope and architecture

The g8e Agentic Ensemble (`g8ee`) is an optional Python 3.12+ FastAPI application in [ensemble/](../../ensemble/) that owns conversational orchestration, LLM provider integration, typed tool execution, investigation and case workflows, application settings, Operator workflow coordination, and application event publication. It is not the Gateway or an Operator and it does not create an execution authority outside the g8e governance paths.

For Operator commands, g8ee calls `GatewayOperatorClient.dispatch()` with a registered `event_type` and typed protobuf payload bytes against the Gateway's command API. The Gateway derives `action_type` from the event registry, constructs the governed envelope, and the target Operator independently performs required verification and execution. For application-record writes, g8ee uses `GovernanceClient` to submit canonical protojson envelopes over the enrolled app mTLS identity. Audit records use `GatewayOperatorClient.ingest_audit_record()`. g8ee does not own a Gateway pub/sub subscription. Application approvals, model output, memories, reputation, and SSE events do not replace protocol L2 or L3 evidence. See [Ensemble Architecture](architecture.md) and [AI Agents and the g8e Governance Boundary](../architecture/agents.md) for the complete boundary model.

The FastAPI application is assembled in [ensemble/app/main.py](../../ensemble/app/main.py). Its lifespan phases (in order) load bootstrap settings, enroll or load the g8ee application identity with the Gateway, resolve mTLS configuration, connect the DB/KV/blob transport clients, construct handler services (DBService, KVService, BlobService), create CacheAsideService, load Gateway-backed platform settings, construct GovernanceClient, call `ServiceFactory.create_all_services()` to wire all domain services, start lifecycle services, then yield readiness. Shutdown clears the LLM provider state, stops services, closes transport clients, and closes the DB service.

## Repository and dependency ownership

The in-tree `g8e` Python package under [protocol/python/](../../protocol/python/) is the source of truth for shared protocol constants, enums, models, and protobuf-generated Python code. [ensemble/pyproject.toml](../../ensemble/pyproject.toml) resolves the `g8e` dependency to `../protocol/python` through `[tool.uv.sources]`; container builds install the local protocol package before installing `ensemble`. Do not duplicate protocol identifiers in [ensemble/app/](../../ensemble/app/) when a shared protocol value exists.

Ensemble-only values live under [ensemble/app/constants/](../../ensemble/app/constants/). These include internal API paths, environment variable names, runtime paths, provider configuration, message-sender identifiers, and mappings that have no protocol equivalent. [app/constants/generated_paths.py](../../ensemble/app/constants/generated_paths.py) and related generated modules expose values copied from the protocol package; update their source rather than hand-editing generated output.

Generated Python protobuf modules are placed in [protocol/python/g8e/proto/](../../protocol/python/g8e/proto/) by the canonical protocol generator. The ensemble Make target does not generate them: `cd ensemble && make proto` runs the generator in check mode and fails when the canonical stubs are stale. To regenerate all language outputs, run `make proto` from the repository root; the Python portion invokes [protocol/python/scripts/generate_protos.py](../../protocol/python/scripts/generate_protos.py).

The application model hub is `app.models.base`. It re-exports the protocol `G8eBaseModel`, `UTCDatetime`, Pydantic helpers, and the ensemble lifecycle bases:

- `G8eBaseModel` provides the shared Pydantic/protojson-compatible foundation.
- `G8eTimestampedModel` adds UTC `created_at` and optional `updated_at` fields plus `update_timestamp()`.
- `G8eIdentifiableModel` adds a stable UUID4 document identifier and `generate_id()`.
- `G8eAuditableModel` adds `created_by` and `updated_by` plus `update_audit_info()`.
- `recursive_serialize()` converts nested models, datetimes, lists, and dictionaries at cache/database boundaries.

[app.models.http_context.RequestContext](../../ensemble/app/models/http_context.py) extends the protocol request context with `operator_id`, `operator_session_id`, and evaluation context. `G8eHttpContext` validates session exclusivity, requires identity for non-exempt requests, binds context identifiers to the authenticated caller, and validates bound Operator sessions. The middleware and dependency functions in [app/middleware/http_context.py](../../ensemble/app/middleware/http_context.py) and [app/dependencies.py](../../ensemble/app/dependencies.py) own request-context extraction and authentication dependencies.

Most application modules import Pydantic types through `app.models.base`; [app/llm/model_evidence.py](../../ensemble/app/llm/model_evidence.py) retains a direct `BaseModel` import. New application models should use the hub and inherit the appropriate typed protocol or ensemble base rather than introducing parallel serialization behavior.

## Local setup

Run these commands from `ensemble/` unless stated otherwise. Python 3.12 or newer is required.

```bash
cd ensemble
python3 -m venv .venv
source .venv/bin/activate
pip install -e ../protocol/python
pip install -e ".[dev,test,docs]"
pre-commit install
```

`make setup` from `ensemble/` installs the editable protocol package and the ensemble `dev` and `test` extras. The `docs` extra is not included by that target; install `.[dev,test,docs]` when working on MkDocs documentation. The root Makefile also uses a repository-root `.venv` for its ensemble targets, so install the editable packages into that environment when using `make ensemble-test` or `make ensemble-lint` from the repository root.

After changing [protocol/python/](../../protocol/python/), refresh the environment used by the command you are running:

```bash
pip install -e protocol/python
pip install -e 'ensemble[dev,test]'
```

The service reads `.env` with `python-dotenv` at import time without overriding existing environment variables. Local bootstrap settings are assembled by `SettingsService`; verified bootstrap secrets can come from the configured bootstrap material, LLM provider defaults can come from environment variables, and platform settings are loaded through the Gateway-backed cache-aside service. Platform settings take precedence over environment defaults, and request overrides are applied at the user-settings boundary. Do not treat the `/operator-state` mount in the unified Compose deployment as a general host filesystem or execution channel.

## Development commands

The [ensemble/Makefile](../../ensemble/Makefile) selects `ensemble/.venv/bin/python`, `ruff`, and `pyright` when those files exist and otherwise falls back to tools on `PATH`.

```bash
# From ensemble/
make setup       # Install editable protocol and ensemble dev/test packages
make test        # Run all tests under tests/
make lint        # Run Ruff on app/ and Pyright on app/
make format      # Format app/ and tests/ with Ruff
make check       # Format, lint, then test
make proto       # Check canonical Python protobuf stubs
make clean       # Remove Python caches and egg-info artifacts
```

Use the repository-root targets when you need the supported split between local unit/in-process integration tests and external tests:

```bash
make ensemble-test    # tests/unit and tests/integration, excluding external-service markers
make test-external    # integration tests marked ai_integration, requires_web_search, requires_api, or requires_typesafe
make ensemble-lint    # Ruff and Pyright for ensemble/app
make build-ensemble   # Build g8e-ensemble:<VERSION> from ensemble/Dockerfile
```

The root `make ensemble-test` target runs [tests/unit/](../../ensemble/tests/unit/) and [tests/integration/](../../ensemble/tests/integration/) with `-m "not ai_integration and not requires_web_search and not requires_api and not requires_typesafe"`. The `make test-external` target runs marked integration tests with `-m "ai_integration or requires_web_search or requires_api or requires_typesafe"` and requires relevant credentials or external services. The test suite defines markers for `unit`, `integration`, `ai_integration`, `ai`, `e2e`, `smoke`, `thinking`, `tools`, `operator_wire`, `requires_operator`, `requires_api`, `requires_web_search`, `requires_typesafe`, `slow`, `aws`, and `intent_workflow`; inspect the test and fixture before selecting a marker because some require a live Gateway, Operator, or external provider.

For a focused test, use the environment selected by the target or invoke the ensemble interpreter explicitly:

```bash
ensemble/.venv/bin/python -m pytest tests/unit/services/evaluation/test_trace_service.py -v
```

Tests use the pytest configuration in [ensemble/pyproject.toml](../../ensemble/pyproject.toml): strict markers and configuration, automatic asyncio mode, a 60-second timeout, warning-as-error behavior with narrowly scoped SDK exceptions, and coverage configured for `app/`. The repository integration fixtures and the external-service markers are the authority for infrastructure requirements; do not infer test tier solely from a directory name.

## Coding standards

- Use Ruff for linting and formatting. The configured target is Python 3.12, with a 100-character line length and four-space indentation.
- Use Pyright against `app/`; the project configuration enables strict typing rules.
- Route new Pydantic imports through `app.models.base` and use typed protocol or application models rather than raw dictionaries for known shapes.
- Import shared constants, API paths, model types, and enums from the in-tree `g8e` package. Keep values in `app/constants/` only when they are ensemble-owned.
- Keep service construction and dependency wiring in `ServiceFactory` and its typed `CoreServices`, `DataServices`, `DomainServices`, `OperatorServices`, and `AllServices` containers. Do not create a second production wiring path in a router or provider.
- Preserve the startup dependency order: bootstrap settings and identity, transport clients, DB/KV/blob handlers, cache-aside service, platform settings, governance and domain services, then lifecycle start hooks.
- Return or raise the typed errors defined by `app.errors` and use the centralized error codes in `app.constants.errors` for machine-checked failures.
- Route business-critical application-record mutations through `GovernanceClient`. Do not bypass the Gateway with direct storage writes or treat application approval as protocol authorization.
- Preserve exact Operator/session binding when constructing command requests. Do not broadcast commands or trust caller-supplied identity headers without authenticated-context validation.
- Add unit tests for isolated behavior and integration tests for real Gateway, Operator, HTTP dispatch, or provider boundaries. Add a regression test before fixing a bug.
- Keep documentation under [docs/ensemble/](../) synchronized when interfaces, models, provider boundaries, lifecycle behavior, or test commands change.

Pre-commit runs the configured Ruff and Pyright checks on staged files. It is an additional local check, not a replacement for the Make targets or the relevant integration tests.

## LLM provider boundary

LLM provider adapters live under [ensemble/app/llm/providers/](../../ensemble/app/llm/providers/). Supported providers are OpenAI, Anthropic, Gemini, Ollama, llama.cpp, g8e (governed inference), and fake (testing only). Provider selection and role-specific model configuration are typed in `app.models.settings`; roles are primary, assistant, and lite. The lite role may be set to Jev (see [Decision Providers](decision-providers.md)) for triage and eval judge—not a generative LLM but a decision API. Optional Vertex AI Search grounding is represented by `GroundingService` and `WebSearchProvider` when search is enabled.

The `g8e` provider routes inference through the Gateway's governed `/api/v1/inference/dispatch` endpoint via the internal HTTP client. Other provider adapters remain provider integrations and inherit no automatic governance from originating in g8ee. Provider behavior, native network access, and side channels remain outside the g8e execution boundary.

## Service and test wiring

`ServiceFactory.create_all_services()` constructs the production graph in dependency order: core services (HTTP, events, settings), data services (CRUD for investigations, operators, memories, cases, reputation), domain services (investigation logic, memory generation, reputation scoring, SSH inventory), and operator-adjacent services (authentication, API keys, certificates). It then assembles attachment, approval, tool execution, the g8e ensemble agent, and the chat pipeline. The chat pipeline creates its trace service unless injected. Operator heartbeat persistence and evaluation are Gateway-owned; g8ee has no heartbeat subscription.

Tests can inject fakes via [ensemble/tests/fakes/](../../ensemble/tests/fakes/). Unit tests remain isolated from live infrastructure. Integration tests under [ensemble/tests/integration/](../../ensemble/tests/integration/) cover Gateway-backed cache and settings, Gateway HTTP dispatch, mTLS inference, SSE event contracts, Operator workflows, and cross-service paths. The [tests/e2e/](../../ensemble/tests/e2e/) package provides end-to-end test support; its markers and fixtures determine whether a live deployment is required.

## Protobuf and generated artifacts

Protocol definitions consumed by g8ee are maintained under [protocol/proto/g8e/](../../protocol/proto/g8e/). Generated Python protobuf modules live in [protocol/python/g8e/proto/](../../protocol/python/g8e/proto/) with protocol-package ownership. Do not hand-edit generated Python modules. The ensemble `make proto` target verifies that canonical stubs are current via [protocol/python/scripts/generate_protos.py](../../protocol/python/scripts/generate_protos.py) with `--check`. Run the protocol root's `make proto` to regenerate all language outputs, then run parity tests ([tests/test_constants_parity.py](../../ensemble/tests/test_constants_parity.py)) and relevant unit tests under [tests/unit/constants/](../../ensemble/tests/unit/constants/) and [tests/unit/clients/](../../ensemble/tests/unit/clients/).

## Related documentation

- [g8ee index](index.md) — Component documentation map and entry points.
- [Ensemble Architecture](architecture.md) — Component boundaries, runtime, storage, and request flow.
- [Governance](governance.md) — Five-layer verification and posture semantics.
- [Agents](agents.md) — Persona hierarchy, Tribunal, Auditor, and Marshal behavior.
- [Protocol](protocol.md) — Ensemble-facing protocol integration.
- [LLM Providers](llm-providers.md) — Provider implementations and configuration.
- [Storage](storage.md) — Gateway-backed storage and restart behavior.
- [Server-Sent Events](sse.md) — Event delivery and session targeting.
- [Testing](tests.md) — Component test organization and infrastructure.
- [Getting Started](getting-started.md) — Local and unified deployment setup.
- [Platform Developer Guidelines](../devs/devs.md) — Repository-wide coding and testing rules.
- [Documentation Guide](../devs/docs.md) — Documentation audit, ownership, generation, and validation rules.
