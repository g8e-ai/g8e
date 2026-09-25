# Testing

Last Updated: 2026-09-25
Version: v2.2.x

## Overview

g8ee uses pytest with automatic asyncio support, strict marker and configuration checking, a 60-second test timeout, and warning-as-error behavior with narrowly scoped SDK exceptions. The pytest configuration lives in `ensemble/pyproject.toml`, and the shared harness lives in `ensemble/tests/conftest.py`.

The ensemble test suite contains unit tests, integration tests, external-provider tests, shared fake conformance checks, and a top-level protocol-constant parity check. `ensemble/tests/e2e/` exists as a support location, and `e2e` is a registered marker, but there are currently no ensemble end-to-end test cases. The Go-native evaluator is a platform subsystem with tests under `internal/services/evaluation/` and `internal/cli/cmd/`; it is not part of the Python ensemble suite.

Every pytest session runs the shared configuration hook, which probes the locally configured Gateway/Operator services for platform settings. A successful probe supplies platform settings; a timeout or connection failure falls back to local bootstrap settings. This means even a unit-only pytest invocation can attempt local service connections during startup, although unit test bodies use fakes and do not require those services.

## Test Tiers

| Tier | Scope | Location | Selection | Dependencies |
| --- | --- | --- | --- | --- |
| Tier 1 | Unit tests | `ensemble/tests/unit/` | Directory selection and the `unit` marker | Test doubles and in-memory state in the test body |
| Tier 2 | Integration tests | `ensemble/tests/integration/` | Directory selection and the `integration` marker | In-process fakes, the HTTPS/WebSocket mock Gateway, or locally configured Gateway/Operator services, depending on the fixture |
| Tier 3 | End-to-end tests | `ensemble/tests/e2e/` | `e2e` marker | Reserved for full-stack tests; no ensemble Tier 3 test cases are currently implemented |
| Tier 4 | External-provider tests | `ensemble/tests/integration/` | `ai_integration`, `requires_web_search`, `requires_api`, or `requires_typesafe` | Configured LLM, TypeSafe/Jev, Vertex AI Search, or other enabled external API configuration |

The root `make ensemble-test` target and the ensemble CI job collect only `tests/unit/` and `tests/integration/`. They do not collect tests located directly under `tests/` or under `tests/fakes/`. Running pytest against `tests/` also collects `tests/test_constants_parity.py` and `tests/fakes/test_fakes_protocol_conformance.py`, as well as any other matching top-level checks.

The `tests/e2e/` directory currently contains only package support and does not add executable end-to-end cases. Do not treat the presence of the directory or the registered marker as evidence that a full-stack E2E suite exists.

## Set Up the Test Environment

The suite requires Python 3.12 or later, the in-tree Python protocol package, and the ensemble test dependencies. From the repository root, install the editable protocol package and the ensemble test extras:

```bash
pip install -e protocol/python
pip install -e 'ensemble[dev,test]'
```

Alternatively, from `ensemble/`, `make setup` installs the editable protocol package and the ensemble `dev` and `test` extras. The root Makefile prefers a repository-root `.venv`; the component Makefile prefers `ensemble/.venv` and otherwise uses tools on `PATH`. See [Development](devs.md) for the complete environment setup and the distinction between those environments.

Run commands from the repository root unless a command explicitly says otherwise. Refresh the editable installs after changing `protocol/python/`.

## Run the Main Ensemble Suite

From the repository root:

- `make ensemble-test` runs `ensemble/tests/unit/` and `ensemble/tests/integration/` with `-m "not ai_integration and not requires_web_search and not requires_api"`.
- `make test-external` runs only integration tests selected by `ai_integration`, `requires_web_search`, `requires_api`, or `requires_typesafe`.
- `make ensemble-lint` runs Ruff and Pyright against `ensemble/app`.
- `make ci-ensemble` runs `ensemble-lint` followed by `ensemble-test`.
- `make build-ensemble` builds the `g8e-ensemble:<VERSION>` Docker image; it does not run tests.

The external target does not make provider failures into skips. Tests skip when the collection-time configuration gate can detect that required settings are absent. Invalid credentials, unavailable providers, quota errors, and other failures from configured services fail the tests that execute.

From `ensemble/`:

- `make test` runs pytest against all of `tests/` without an external-marker exclusion and can make live provider calls when the loaded settings enable those tests.
- `make lint` runs Ruff and Pyright against `app/`.
- `make format` formats `app/` and `tests/` with Ruff and modifies files.
- `make check` runs `format`, `lint`, and the unfiltered test target in that order.
- `make proto` checks that the canonical Python protobuf stubs are current; it does not regenerate them.

For focused pytest runs from `ensemble/`:

```bash
python -m pytest tests/unit/
python -m pytest tests/integration/ -m "not ai_integration and not requires_web_search and not requires_api and not requires_typesafe"
python -m pytest tests/ -m "not ai_integration and not requires_web_search and not requires_api and not requires_typesafe and not e2e"
python -m pytest tests/integration/ -m ai_integration
python -m pytest tests/integration/ -m "requires_web_search or requires_api or requires_typesafe"
```

The third command includes top-level parity and fake conformance checks while excluding external-provider and E2E-marked tests. The repository `./g8e test` subcommands run the Go platform test suites; they do not run the Python ensemble suite.

## Markers and External Configuration

The suite registers markers in `ensemble/pyproject.toml` and enforces them with `--strict-markers`. Directory selection remains the primary unit/integration split; markers identify external dependencies and specialized behavior. The currently used markers are `unit`, `integration`, `ai_integration`, `requires_web_search`, `requires_typesafe`, `requires_operator`, and `slow`. The harness also applies `thinking` and `tools` dynamically to selected accuracy scenarios. `e2e`, `smoke`, `ai`, `aws`, `intent_workflow`, `requires_api`, and `operator_wire` are registered classifications with no general current test population, although `requires_api` remains part of the external selection and gating commands.

The external markers mean:

- `ai_integration` identifies integration tests that use a configured LLM provider. Without an explicit `G8E_TEST_LLM_PRIMARY_PROVIDER`, collection uses the loaded LLM settings to determine whether an LLM provider is configured. When that environment variable is set, the harness checks the provider-specific key or endpoint fields before allowing those tests to run.
- `requires_web_search` identifies tests that require enabled Vertex AI Search settings with a project ID, engine ID, and API key.
- `requires_api` identifies tests that require enabled external search/API settings. The marker is registered and included by the filters, but no current ensemble test is directly marked with it.
- `requires_typesafe` identifies integration tests that call the live TypeSafe System One (Jev) API. Collection skips these tests when neither `G8E_LLM_JEV_API_KEY` nor `TYPESAFE_API_KEY` is set. Examples: `test_jev_triage_integration.py`, `test_jev_eval_judge_integration.py`.
- `requires_operator` identifies integration tests that require a live Gateway/Operator path, such as mTLS inference. Integration fixtures can also skip when required CA material or Operator connectivity is unavailable, even when a test does not carry this marker.

The harness accepts environment overrides for external test settings. LLM settings use `G8E_TEST_LLM_PRIMARY_PROVIDER`, provider-appropriate primary credentials or endpoints, optional assistant and lite provider/model/credential/endpoint variables, and optional `G8E_TEST_LLM_MAX_TOKENS`. Web search settings use `G8E_TEST_WEB_SEARCH_PROJECT_ID`, `G8E_TEST_WEB_SEARCH_ENGINE_ID`, `G8E_TEST_WEB_SEARCH_API_KEY`, and optional `G8E_TEST_WEB_SEARCH_LOCATION`, which defaults to `global`.

The environment variables configure the test process; they do not bypass provider authentication or platform authorization. When no environment LLM override is supplied, the harness may use LLM settings loaded from the local platform configuration. When the required configuration is detectably absent, collection adds a skip marker. A configured but invalid or unavailable service remains a test failure.

## Fixtures and Isolation

The shared harness provides unique investigation, user, case, operator, and session identifiers. `TaskTracker` closes captured coroutines and cancels and awaits captured tasks after each test. The fake implementations under `ensemble/tests/fakes/` provide typed service-boundary doubles, and `tests/fakes/mock_gateway.py` provides an in-process HTTPS and WebSocket mock Gateway for tests that use it.

Integration fixtures are not uniformly hermetic. `all_services` first checks the local CA certificate and a live TLS connection to the configured Operator, then constructs most services with fakes and a governance client that writes through to the fake database. Other integration tests connect to local Gateway database, KV, pub/sub, or inference services. Inspect the fixtures used by a test before assuming that it runs without local platform state.

Integration approval helpers can resolve pending approvals after a code path returns or inline as approvals are registered. The inline callback is required for flows that block while waiting for an approval, including long-running benchmark scenarios. Integration cleanup tracks created documents and waits for background work before deleting them.

Protocol checks cover two separate concerns. Fake conformance tests verify that the doubles in `tests/fakes/` implement their declared Python protocols. The top-level constants parity test validates protocol JSON against the ensemble's typed constants models and supports `G8E_PROTOCOL_DIR` when the protocol directory is not at its repository-relative location. Run pytest against all of `tests/` when changing shared fakes or protocol constants.

## Coverage and Quality

Coverage tracks branch coverage for `app/` and omits tests, package entry points, conftest files, empty modules, pytest caches, and site packages. The ensemble does not configure a coverage failure threshold. From `ensemble/`, this command produces a terminal report without selecting configured external-provider tests:

```bash
python -m pytest tests/ -m "not ai_integration and not requires_web_search and not requires_api and not e2e" --cov=app --cov-report=term-missing
```

Add `--cov-report=html` or `--cov-report=json` to write reports to the configured locations under `coverage-reports/g8ee/`. Ruff and Pyright targets cover only `app/` in the ensemble Makefile and CI job. Go-native evaluation code is covered by the platform commands `./g8e test unit`, `./g8e test lint`, and the platform coverage workflow.

## Related

- [Platform Testing](../devs/tests.md) describes the Go platform test model and `./g8e test` commands.
- [Development](devs.md) covers ensemble environment setup, component commands, and quality checks.
- [Evals](evals.md) covers benchmark execution, evidence, and reports.
- [Architecture](architecture.md) describes ensemble components and protocol surfaces.
- [Documentation Guide](../devs/docs.md) defines the repository documentation audit and validation standard.
