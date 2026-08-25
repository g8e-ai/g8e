# Testing

## Overview

g8ee uses pytest with marker-based test categorization. Tests are organized into suites aligned with the g8e 4-tier test model: Tier 1 (Unit), Tier 2 (In-Process Integration), Tier 3 (Docker E2E), and Tier 4 (External / Live LLM and APIs). Tier 1 and Tier 2 tests execute entirely offline using mocks, fakes, and in-process components. Tier 4 tests interact with live LLM providers and external search APIs, and are automatically skipped when required credentials or endpoints are absent.

## Test Architecture

| Tier | Name | Target Directory | Markers / Tags | External Dependencies | Execution Profile |
| --- | --- | --- | --- | --- | --- |
| **Tier 1** | **Unit Tests** | `ensemble/tests/unit/` | `unit` | None (pure stubs, fakes, and mocks) | Sub-second per file, isolated in-memory |
| **Tier 2** | **In-Process Integration** | `ensemble/tests/integration/` | `integration`, `intent_workflow`, `operator_wire` | In-process fake services and mock gateway | 1-5 seconds per suite, no live external APIs |
| **Tier 3** | **Docker E2E** | `ensemble/tests/e2e/` | `e2e` | Containerized platform (Gateway + Operator) | Full application lifecycle |
| **Tier 4** | **External Tests** | `ensemble/tests/integration/` | `ai_integration`, `requires_web_search`, `requires_api` | Live LLM providers, Google Vertex AI Search API | Seconds to minutes, gated on credentials |

## Test Structure

```
ensemble/
├── tests/
│   ├── unit/                    # Tier 1 unit tests (isolated, fast)
│   │   ├── clients/             # DBClient, KVCacheClient, PubSubClient, HttpClient, GovernanceClient
│   │   ├── config/              # Configuration loading, validation, and defaults
│   │   ├── constants/           # API paths, channels, collections, intents, KV keys, prompts parity
│   │   ├── db/                  # DB service, KV service, blob service, and model tests
│   │   ├── llm/                 # LLM providers, prompt loaders, structured output, thinking translators
│   │   ├── main/                # FastAPI lifespan, dependency injection, and startup routines
│   │   ├── models/              # Pydantic schemas, event payloads, SSE wire models, persona models
│   │   ├── routers/             # FastAPI HTTP routes, triage endpoints, and internal routers
│   │   ├── security/            # Output sanitization, timestamp validation, sentinel scrubber
│   │   └── services/            # Agent loops, tribunal consensus, auditor, judge, reputation, memory
│   ├── integration/             # Tier 2 in-process integration and Tier 4 external tests
│   │   ├── invariants/          # Structural invariants (such as Tribunal information isolation)
│   │   └── test_*.py            # Multi-service integration, pipeline integrity, event contracts
│   ├── e2e/                     # Tier 3 end-to-end tests
│   ├── fakes/                   # In-memory fake service clients, providers, and test fixtures
│   └── conftest.py              # Global pytest hooks, settings probes, and shared fixtures
└── evals/
    └── tests/                   # Eval-specific tests (SSE wire, CLI UX, IFEval smoke, auth parity)
```

## Running Tests

### Makefile Targets

From the repository root:

```bash
# Run Tier 1 + Tier 2 unit and in-process integration tests (no external dependencies)
make ensemble-test

# Run Tier 4 external tests (real LLM/API calls, gated on credentials)
make test-external

# Run linting and type checking (Ruff + Pyright) on the ensemble
make ensemble-lint

# Run ensemble linting and testing in the CI pipeline
make ci-ensemble
```

From the `ensemble/` directory:

```bash
# Run all non-external tests via the ensemble Makefile
make test

# Run Ruff linter and Pyright type checker
make lint

# Format code with Ruff
make format

# Run format, lint, and test sequentially
make check
```

### Pytest Commands

Direct pytest invocations from the `ensemble/` directory:

```bash
# Run Tier 1 and Tier 2 tests (skipping Tier 4 external and Tier 3 E2E tests)
pytest tests/ -v -m "not ai_integration and not requires_web_search and not requires_api and not e2e"

# Run Tier 1 unit tests only
pytest tests/unit/ -v

# Run Tier 2 integration tests only
pytest tests/integration/ -v -m "not ai_integration and not requires_web_search and not requires_api"

# Run Tier 4 external AI integration tests
pytest tests/integration/ -v -m "ai_integration"

# Run Tier 4 external web search tests
pytest tests/integration/ -v -m "requires_web_search or requires_api"

# Run eval suite tests
pytest evals/tests/ -v

# Run the complete test suite across tests/ and evals/tests/
pytest tests/ evals/tests/ -v

# Run with test coverage report
pytest --cov=app --cov-report=term-missing
```

## Test Markers and Credential Gating

The pytest suite registers markers in `pyproject.toml` to classify tests and control execution.

### Marker Reference

- `unit` — Fast, isolated unit tests without external dependencies.
- `integration` — Integration tests verifying multi-component interaction.
- `ai_integration` — Tier 4 tests requiring live LLM API access.
- `requires_web_search` — Tier 4 tests requiring web search provider configuration.
- `requires_api` — Tier 4 tests requiring external live API endpoints (e.g. Vertex AI Search).
- `requires_operator` — Integration tests requiring a live g8e Gateway or Operator instance.
- `operator_wire` — Integration tests publishing tool-call events directly to Operator PubSub.
- `thinking` — Tests verifying provider thinking, reasoning tokens, and thought signatures.
- `tools` — Tests verifying LLM function calling and tool execution workflows.
- `intent_workflow` — Tests for intent-based permission workflows with the ensemble.
- `e2e` — End-to-end full application flows.
- `slow` — Tests taking longer than 1 second.
- `smoke` — Quick verification tests for smoke testing.

### Dynamic Credential Gating

The test harness in `tests/conftest.py` inspects the runtime environment and Gateway status during test collection:

1. **Operator Probe** — On startup, `pytest_configure` probes the local Operator to load platform settings. It prints `operator: ok` if connected or `operator: down` when falling back to local bootstrap settings.
2. **LLM Credential Detection** — Tests marked with `ai_integration` check for LLM provider credentials via `G8E_TEST_LLM_*` environment variables (`G8E_TEST_LLM_PRIMARY_PROVIDER`, `G8E_TEST_LLM_PRIMARY_API_KEY`, `G8E_TEST_LLM_PRIMARY_ENDPOINT`, `G8E_TEST_LLM_PRIMARY_MODEL`, etc.) or configured user settings. If credentials are missing, tests are skipped with `reason="no llm creds"`.
3. **Web Search Gating** — Tests marked with `requires_web_search` or `requires_api` check for Google Vertex AI Search settings (`G8E_TEST_WEB_SEARCH_PROJECT_ID`, `G8E_TEST_WEB_SEARCH_ENGINE_ID`, `G8E_TEST_WEB_SEARCH_API_KEY`). If unconfigured, tests are skipped with `reason="no web search"` or `reason="no vertex search"`.
4. **Fail-Safe CI Execution** — External tests never fail CI due to missing credentials. They run only when the requisite environment variables or active provider endpoints are supplied.

## Test Harness and Fixtures

The test infrastructure provides reusable fixtures and fakes in `tests/conftest.py` and `tests/fakes/`:

- **`TaskTracker` (`task_tracker`)** — Tracks coroutines and `asyncio.Task` instances created during test execution, guaranteeing proper cancellation and cleanup upon test completion to prevent resource leaks and `RuntimeWarning` errors.
- **Isolation IDs** — `unique_investigation_id`, `unique_user_id`, `unique_case_id`, `unique_operator_id`, `unique_session_id`, and `unique_web_session_id` supply unique UUIDs to maintain test isolation.
- **Service Mocks and Fakes** — `mock_governance_client`, `mock_cache_aside_service`, `fake_cache_aside_service`, `mock_blob_service`, `mock_event_service`, `mock_investigation_service`, `mock_client_http_client`, and `mock_operator_document` provide typed, spec-compliant mocks and in-memory test doubles.
- **Protocol Conformance** — `tests/fakes/test_fakes_protocol_conformance.py` and `tests/test_constants_parity.py` assert that all fake services and Python constants remain synchronized with protocol specifications and Go constants.

## Code Coverage and Quality Checks

- **Coverage Configuration** — Coverage settings in `pyproject.toml` track branch coverage across the `app/` package, omitting test suites, entry points, and generated protobuf stubs. Reports can be generated in terminal, HTML (`coverage-reports/g8ee/index.html`), and JSON formats.
- **Linting and Type Checking** — Code style and type constraints are validated with Ruff (`ruff check .` and `ruff format .`) and Pyright (`pyright`). Lint rules enforce strict error handling, naming standards, and clean import structures routing through `app.models.base`.

## Related

- [Development](devs.md) — Development environment setup, protobuf generation, and coding standards
- [Evals](evals.md) — Evaluation suite, benchmark datasets, and Judge scoring rubrics
- [Architecture](architecture.md) — System architecture, protocol surfaces, and model hierarchy
- [Governance](governance.md) — Five-layer verification pipeline and envelope validation
- [Constants](constants.md) — Application constants and protocol synchronization

