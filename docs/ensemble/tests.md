# Testing

## Overview

g8ee uses pytest and organizes tests by dependency level. The main ensemble suite contains unit, integration, and external-provider tests. A Tier 3 directory and marker are reserved for end-to-end tests, but the current ensemble suite does not contain Tier 3 test cases. The standalone eval package has its own locked environment, pytest configuration, and Tier 1 and Tier 2 suites.

Pytest probes the local Operator when every main ensemble test session starts, including sessions that select only unit tests. The probe loads platform settings when the Operator is available and falls back to local bootstrap settings when it is not. Unit test bodies remain isolated from live services, but starting a unit test session is not strictly free of network attempts because of this probe.

## Test Tiers

| Tier | Scope | Location | Selection | Dependencies |
| --- | --- | --- | --- | --- |
| Tier 1 | Unit tests | `ensemble/tests/unit/` | Directory selection and the `unit` marker | Test doubles and in-memory state in the test body |
| Tier 2 | Integration tests | `ensemble/tests/integration/` | Directory selection and the `integration` marker | A mixture of in-process fakes, an HTTPS and WebSocket mock Gateway, and fixtures that connect to a locally configured Gateway or Operator |
| Tier 3 | End-to-end tests | `ensemble/tests/e2e/` | `e2e` marker | Reserved for full-stack tests; no ensemble Tier 3 tests are currently implemented |
| Tier 4 | External-provider tests | `ensemble/tests/integration/` | `ai_integration`, `requires_web_search`, or `requires_api` | Configured LLM or web search providers |

The main suite also contains shared fakes and top-level parity tests under `ensemble/tests/`. The root `make ensemble-test` target and the ensemble CI job select only `tests/unit/` and `tests/integration/`, so they do not collect tests located directly under `tests/` or `tests/fakes/`. Running pytest against `tests/` collects those additional checks.

## Set Up the Test Environment

The main ensemble requires Python 3.12 or later, the in-tree Python protocol package, and the ensemble test dependencies. Follow [Development](devs.md) for environment setup. Run ensemble commands from the repository root unless a command explicitly says otherwise.

The eval package uses `uv` and its own `uv.lock`. Its root Makefile targets invoke `uv run --locked --extra test`, so eval dependencies do not need to be installed into the main ensemble environment.

## Run the Main Ensemble Suite

From the repository root:

- `make ensemble-test` runs `ensemble/tests/unit/` and `ensemble/tests/integration/` while excluding `ai_integration`, `requires_web_search`, and `requires_api` tests.
- `make test-external` runs integration tests carrying at least one external marker. Missing configuration detected during collection skips the affected tests; invalid credentials, unavailable providers, and provider errors still fail tests that run.
- `make ensemble-lint` runs Ruff and Pyright against `ensemble/app`.
- `make ci-ensemble` runs `ensemble-lint` followed by `ensemble-test`. It does not run standalone eval checks.

From `ensemble/`:

- `make test` runs pytest against all of `tests/` without an external-marker exclusion. If live provider settings are available, this command can make external calls.
- `make lint` runs Ruff and Pyright against `app/`.
- `make format` formats `app/` and `tests/` with Ruff and modifies files.
- `make check` formats, lints, and then runs the unfiltered test target.

For focused pytest runs from `ensemble/`:

- `python -m pytest tests/unit/` runs the unit directory.
- `python -m pytest tests/integration/ -m "not ai_integration and not requires_web_search and not requires_api"` runs integration tests without external-provider tests.
- `python -m pytest tests/ -m "not ai_integration and not requires_web_search and not requires_api and not e2e"` runs all currently implemented non-external checks, including top-level parity and fake conformance tests.
- `python -m pytest tests/integration/ -m ai_integration` runs live LLM tests.
- `python -m pytest tests/integration/ -m "requires_web_search or requires_api"` runs live search tests.

The repository `./g8e test` subcommands run the Go platform test suites. They do not run the Python ensemble or eval suites.

## Run the Standalone Eval Tests

From the repository root:

- `make evals-test` runs eval Tier 1 and Tier 2 tests.
- `make evals-test-unit` runs tests marked `unit`.
- `make evals-test-integration` runs tests marked `integration`. Some tests use local filesystem or subprocess dependencies, and CI builds the `g8e` binary before this tier.
- `make evals-lint` runs Ruff over `g8e_evals` and its tests, then runs Pyright with the eval project configuration.

The eval package registers an `e2e` marker for tests that require a live stack or provider, but no current eval test uses that marker and the root eval targets do not select it.

## Markers and External Configuration

The main suite registers markers in `ensemble/pyproject.toml` and enforces them with pytest strict marker checking. Directory selection defines the primary unit and integration suites. Markers further identify external dependencies or specialized behavior.

The active external markers are:

- `ai_integration` identifies tests that call a configured LLM provider.
- `requires_web_search` identifies tests that require complete Vertex AI Search configuration.
- `requires_api` identifies tests that require an enabled external API configuration. The marker is registered and included by test filters, but no current ensemble test uses it.

The suite also registers `unit`, `integration`, `e2e`, `slow`, `smoke`, `ai`, `aws`, `intent_workflow`, `thinking`, `tools`, `operator_wire`, and `requires_operator`. Some are reserved classifications and have no current usages. Run `python -m pytest --markers` from `ensemble/` for the descriptions pytest uses.

At session startup, the harness prints `operator: ok` when the settings probe succeeds or `operator: down` when it uses local bootstrap settings. Environment-based LLM configuration uses `G8E_TEST_LLM_PRIMARY_PROVIDER` with provider-appropriate credentials or endpoints; optional primary, assistant, and lite model settings refine the configuration. Environment-based web search configuration requires `G8E_TEST_WEB_SEARCH_PROJECT_ID`, `G8E_TEST_WEB_SEARCH_ENGINE_ID`, and `G8E_TEST_WEB_SEARCH_API_KEY`, with optional `G8E_TEST_WEB_SEARCH_LOCATION`.

Collection-time gating behaves as follows:

1. When `G8E_TEST_LLM_PRIMARY_PROVIDER` is set, the harness validates the provider-specific key or endpoint before selecting `ai_integration` tests. Without that environment variable, the loaded LLM settings determine whether those tests run.
2. `requires_web_search` tests run only when search is enabled and project ID, engine ID, and API key are present.
3. `requires_api` tests run when external search is enabled.

Credential gating prevents calls when required configuration is detectably absent. It does not convert authentication failures, provider outages, quota errors, or invalid configured values into skips.

## Fixtures and Isolation

The shared harness provides unique investigation, user, case, operator, and session identifiers. It also provides task tracking that closes coroutines and cancels background tasks after each test, typed fakes for service boundaries, and an in-process mock Gateway that serves HTTPS and WebSocket protocol surfaces.

Integration fixtures are not uniformly hermetic. Some construct services with fakes or the mock Gateway, while others connect through configured TLS credentials to local Gateway database, KV, or PubSub services. Check the fixtures used by a test before assuming that it runs without local platform state.

Protocol checks cover two separate concerns. Fake conformance tests verify that test doubles implement their declared Python protocols. Constants parity tests validate protocol JSON against the ensemble's typed constants models. Because these checks live outside the unit and integration directories, run pytest against all of `tests/` when changing shared fakes or protocol constants.

## Coverage and Quality

Coverage configuration tracks branch coverage for `app/` and omits tests, package entry points, conftest files, and empty modules. The ensemble does not configure a coverage failure threshold. From `ensemble/`, `python -m pytest tests/ -m "not ai_integration and not requires_web_search and not requires_api and not e2e" --cov=app --cov-report=term-missing` produces a terminal report without making configured external-provider calls. Add `--cov-report=html` or `--cov-report=json` to write reports to the configured paths under `coverage-reports/g8ee/`.

Ruff checks only `app/` in the main ensemble Makefile and CI targets. Pyright also checks only `app/`. The standalone eval lint target checks both production and test code.

## Related

- [Platform Testing](../devs/tests.md) describes the Go platform test model and `./g8e test` commands.
- [Development](devs.md) covers ensemble environment setup and quality commands.
- [Evals](evals.md) covers benchmark execution, evidence, and reports.
- [Architecture](architecture.md) describes the ensemble's components and protocol surfaces.
