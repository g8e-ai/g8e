---
doc_id: ensemble_tests
title: Ensemble Testing Guide
audience: developers and coding agents
status: current
last_updated: 2026-09-30
version: v2.2.5
owners:
  - ensemble/tests/
  - ensemble/pyproject.toml
  - ensemble/Makefile
related:
  - docs/devs/tests.md
  - docs/ensemble/devs.md
  - docs/ensemble/architecture.md
  - docs/ensemble/evals.md
when_to_read: Setting up the ensemble test environment, running test suites, understanding test structure and markers, writing integration tests, auditing test configuration.
do_not_use_for:
  - Platform (Go) test model — see docs/devs/tests.md
  - Ensemble development environment setup — see docs/ensemble/devs.md
  - Ensemble architecture — see docs/ensemble/architecture.md
  - Evaluation and benchmarking — see docs/ensemble/evals.md
---

# Ensemble Testing Guide

## Purpose

Defines how to set up, run, and audit the g8ee ensemble test suite. The ensemble uses pytest with automatic asyncio support, strict marker and configuration checking, a 60-second test timeout, and warning-as-error behavior for narrowly scoped SDK exceptions. The test suite is organized by tier (unit, integration, end-to-end, external-provider) with clear boundaries between fast isolated tests and tests requiring external services or credentials. Configuration lives in [ensemble/pyproject.toml](ensemble/pyproject.toml) and shared pytest infrastructure in [ensemble/tests/conftest.py](ensemble/tests/conftest.py).

## Quick index

- [Purpose](#purpose)
- [Invariants](#invariants)
- [Owned surfaces](#owned-surfaces)
- [Procedures](#procedures)
- [Anti-patterns](#anti-patterns)
- [Links out](#links-out)

## Invariants

### Test organization and markers

| ID | Rule |
| --- | --- |
| INV-TEST-ENUM-01 | The ensemble test suite contains unit tests (Tier 1), integration tests (Tier 2), external-provider tests (Tier 4), fake conformance checks, and protocol-constant parity checks. End-to-end tests (Tier 3) are reserved; the `e2e` directory and marker exist but contain no executable test cases. |
| INV-TEST-ENUM-02 | Unit tests live in `ensemble/tests/unit/` and integration tests in `ensemble/tests/integration/`. The `ensemble-test` and `ci-ensemble` targets collect only `tests/unit/` and `tests/integration/`, excluding external-provider markers. Top-level checks under `tests/test_constants_parity.py` and `tests/fakes/test_fakes_protocol_conformance.py` are collected when running pytest over all of `tests/`. |
| INV-TEST-ENUM-03 | Markers are registered in [ensemble/pyproject.toml](ensemble/pyproject.toml:99) and enforced via `--strict-markers`. Active markers are `unit`, `integration`, `ai_integration`, `requires_web_search`, `requires_system_one`, `requires_operator`, `slow`, `thinking`, and `tools`. Reserved markers with no current test population are `e2e`, `smoke`, `ai`, `aws`, `intent_workflow`, `operator_wire`, and others for future use. |
| INV-TEST-ENUM-04 | External markers (`ai_integration`, `requires_web_search`, `requires_system_one`, `requires_api`, `requires_operator`) are gated at pytest collection time in [ensemble/tests/conftest.py:377](ensemble/tests/conftest.py#L377). Tests skip when required services (LLM keys, web search config, Ollama with System One model, Operator connectivity) are detectably absent. Invalid or unavailable configured services remain test failures. |
| INV-TEST-ENUM-05 | The g8ee platform enrollment client is covered at two layers. Mocked client behavior is in `tests/unit/services/infra/app_enrollment_service_test.py` and startup fail-closed behavior in `tests/unit/main/test_main_lifespan.py`. The completion transcript wire contract is pinned in `tests/unit/services/infra/app_enrollment_transcript_contract_test.py` against the generated protobuf message and a golden vector shared with the Gateway's `TestPlatformEnrollmentCompletionTranscriptGoldenVector`. The cross-language live enrollment harness was removed; these checks do not cover the real client against a live Gateway. No test or fixture may substitute self-issued credentials for enrollment. |

### Configuration and startup

| ID | Rule |
| --- | --- |
| INV-TEST-CONF-01 | Every pytest session calls `pytest_configure` hook in [ensemble/tests/conftest.py:348](ensemble/tests/conftest.py#L348), which probes the locally configured Gateway/Operator for platform settings. A successful probe supplies settings; timeout or connection failure falls back to local bootstrap settings from `SettingsService().get_local_settings()`. This means unit-only invocations may attempt local service connections during startup, though unit test bodies use fakes and do not require those services. |
| INV-TEST-CONF-02 | LLM settings are loaded from `G8E_TEST_LLM_PRIMARY_PROVIDER` and related env vars set by `./g8e test --llm-provider` flags, built by `_llm_settings_from_env()` in [ensemble/tests/conftest.py:83](ensemble/tests/conftest.py#L83). When not supplied, the harness checks platform settings loaded from Operator or local bootstrap. Web search settings use `G8E_TEST_WEB_SEARCH_*` env vars built by `_web_search_settings_from_env()` in [ensemble/tests/conftest.py:251](ensemble/tests/conftest.py#L251). |
| INV-TEST-CONF-03 | The `pytest_collection_modifyitems` hook dynamically adds skip markers for tests whose required external credentials or Operator connectivity are absent at collection time. Collection-time configuration gates control marker-based skips; environment variable overrides are explicit; invalid configured services are runtime test failures. |

### Fixtures and lifecycle

| ID | Rule |
| --- | --- |
| INV-TEST-FIX-01 | The main harness [ensemble/tests/conftest.py](ensemble/tests/conftest.py) provides: `unique_investigation_id`, `unique_user_id`, `unique_case_id`, `unique_operator_id`, `unique_session_id`, `unique_web_session_id` (unique identifiers per test); `mock_governance_client`, `mock_operator_document` (mocks); `test_settings` (session-scoped platform settings); `mock_cache_aside_service`, `fake_cache_aside_service` (cache-aside service variants); `task_tracker` (for coroutine and task cleanup). |
| INV-TEST-FIX-02 | Integration-specific fixtures are defined in [ensemble/tests/integration/conftest.py](ensemble/tests/integration/conftest.py). The `all_services` fixture constructs application services; it first checks for a local CA certificate and live TLS connection to the configured Operator (skipping if unavailable), then uses real or mocked service boundaries depending on the fixture variant. |
| INV-TEST-FIX-03 | Fake implementations under [ensemble/tests/fakes/](ensemble/tests/fakes/) provide typed service-boundary doubles. Conformance checks in `tests/fakes/test_fakes_protocol_conformance.py` verify they implement their declared Python protocols. The in-process HTTPS and WebSocket mock Gateway in `tests/fakes/mock_gateway.py` supports tests that need a mock gateway without external services. |

### Test environment and dependencies

| ID | Rule |
| --- | --- |
| INV-TEST-ENV-01 | The suite requires Python 3.12+, the in-tree Python protocol package at [protocol/python/](protocol/python/), and ensemble test dependencies from [ensemble/pyproject.toml](ensemble/pyproject.toml:53). Install via `pip install -e protocol/python` and `pip install -e 'ensemble[dev,test]'` from the repository root, or `make setup` from `ensemble/`. |
| INV-TEST-ENV-02 | Coverage tracks branch coverage for `app/` and omits tests, `conftest.py`, entry points, empty modules, and site packages (configured in [ensemble/pyproject.toml:146](ensemble/pyproject.toml#L146)). The suite does not enforce a coverage failure threshold. Run `python -m pytest tests/ -m "not ai_integration and not requires_web_search and not requires_api and not requires_system_one and not e2e" --cov=app --cov-report=term-missing` from `ensemble/` for a terminal report. |

## Owned surfaces

| Claim | Path | Verify |
| --- | --- | --- |
| Pytest configuration | [ensemble/pyproject.toml](ensemble/pyproject.toml:86) | Async mode, markers, timeout, warning filters, coverage settings |
| Shared test harness | [ensemble/tests/conftest.py](ensemble/tests/conftest.py) | Fixtures, configuration hooks, collection-time marker gating |
| Integration fixtures | [ensemble/tests/integration/conftest.py](ensemble/tests/integration/conftest.py) | `all_services`, service construction, Operator probe and fallback |
| Unit test directory | [ensemble/tests/unit/](ensemble/tests/unit/) | Tier 1 test cases, fakes and mocks |
| Integration test directory | [ensemble/tests/integration/](ensemble/tests/integration/) | Tier 2 and Tier 4 test cases, external-provider markers, mTLS and gateway tests |
| End-to-end directory | [ensemble/tests/e2e/](ensemble/tests/e2e/) | Package support only; no executable test cases |
| Enrollment client tests | [ensemble/tests/unit/services/infra/](ensemble/tests/unit/services/infra/) | Mocked client behavior and completion transcript contract |
| Fake implementations | [ensemble/tests/fakes/](ensemble/tests/fakes/) | Service doubles and mock Gateway |
| Makefile test targets | [ensemble/Makefile](ensemble/Makefile:48), [Makefile](Makefile:690) | `make test`, `make setup`, root `make ensemble-test`, `make ensemble-test-external`, `make ensemble-lint`, `make ci-ensemble` |

## Procedures

### Set up the test environment

The suite requires Python 3.12 or later, the in-tree Python protocol package, and the ensemble test dependencies. From the repository root, install the editable protocol package and the ensemble test extras:

```bash
pip install -e protocol/python
pip install -e 'ensemble[dev,test]'
```

Alternatively, from `ensemble/`, run:

```bash
make setup
```

The root Makefile prefers a repository-root `.venv`; the ensemble Makefile prefers `ensemble/.venv` and falls back to tools on `PATH`. See [Ensemble Development Guide](devs.md) for the complete environment setup.

Run commands from the repository root unless a command explicitly says otherwise. Refresh the editable installs after changing [protocol/python/](protocol/python/).

### Run test suites from the repository root

- `make ensemble-test` runs `ensemble/tests/unit/` and `ensemble/tests/integration/` with `-m "not ai_integration and not requires_web_search and not requires_api"`. Tests marked `requires_system_one` are gated at collection time when Ollama with a System One model is unavailable.
- `make ensemble-test-external` runs only tests in `ensemble/tests/integration/` marked with `ai_integration`, `requires_web_search`, `requires_api`, or `requires_system_one`.
- `make ensemble-lint` runs Ruff and Pyright against [ensemble/app](ensemble/app).
- `make ci-ensemble` runs `ensemble-lint` followed by `ensemble-test`.
- `make ensemble-build` builds the `g8e-ensemble:<VERSION>` Docker image without running tests.

### Run test suites from `ensemble/`

- `make test` runs pytest against all of `tests/` without external-marker exclusion and can make live provider calls when loaded settings enable those tests.
- `make lint` runs Ruff and Pyright against [app/](app/).
- `make format` formats [app/](app/) and [tests/](tests/) with Ruff.
- `make check` runs `format`, `lint`, and `test` in order.
- `make proto` verifies that the canonical Python protobuf stubs are current; it does not regenerate them.

### Run focused test suites from `ensemble/`

For targeted pytest runs, use the markers and directories:

```bash
python -m pytest tests/unit/
python -m pytest tests/integration/ -m "not ai_integration and not requires_web_search and not requires_api and not requires_system_one"
python -m pytest tests/ -m "not ai_integration and not requires_web_search and not requires_api and not requires_system_one and not e2e"
python -m pytest tests/integration/ -m ai_integration
python -m pytest tests/integration/ -m "requires_web_search or requires_api or requires_system_one"
```

The third command includes top-level parity and fake conformance checks while excluding external-provider and E2E-marked tests. The repository `./g8e test` subcommands run the Go platform test suites; they do not run the Python ensemble suite.

### Generate and review test coverage

Coverage tracks branch coverage for [app/](app/) and omits tests, `conftest.py`, entry points, empty modules, and site packages. From `ensemble/`, generate a terminal report while excluding external-provider and e2e tests:

```bash
python -m pytest tests/ -m "not ai_integration and not requires_web_search and not requires_api and not requires_system_one and not e2e" --cov=app --cov-report=term-missing
```

Add `--cov-report=html` or `--cov-report=json` to write reports to `coverage-reports/g8ee/`. Ruff and Pyright targets cover only [app/](app/) in the ensemble Makefile and CI job. Go-native evaluation code is covered by `./g8e test unit`, `./g8e test lint`, and the platform coverage workflow.

## Anti-patterns

- Running Tier 4 (external-provider) tests without supplying required credentials or with invalid configuration, expecting them to skip. External markers gate at collection time only when credentials are detectably absent; configured but invalid services fail tests.
- Assuming a unit-only pytest invocation makes no network attempts. The `pytest_configure` hook probes Operator for platform settings; even unit tests may trigger startup-phase connections. Provide Operator on localhost or configure timeout expectations.
- Hand-editing fixture implementations or integration-specific service construction patterns without auditing their use across [ensemble/tests/integration/](ensemble/tests/integration/). Fixture variants support different isolation levels; changing one may affect marker gating, cleanup, or Operator probe behavior.
- Running `pytest` directly without setting markers, expecting external tests to be excluded. The `make ensemble-test` target applies explicit marker filters. Direct invocation of `pytest tests/` collects all test cases including external-provider tests.
- Mixing Tier 1 and Tier 2 isolation in a single test file. Keep unit tests in [ensemble/tests/unit/](ensemble/tests/unit/) and integration tests in [ensemble/tests/integration/](ensemble/tests/integration/) to preserve test-target selectivity.

## Links out

- [Platform Testing Guide](../devs/tests.md) — Go platform test model, `./g8e test` commands, and CI scope.
- [Ensemble Development Guide](devs.md) — Ensemble environment setup, Python version requirements, component Makefile, and development workflows.
- [Ensemble Evals](evals.md) — Benchmark execution, accuracy scenarios, evidence generation, and eval reports.
- [Ensemble Architecture](architecture.md) — Component overview, service boundaries, and protocol surfaces.
- [Documentation Guide](../devs/docs.md) — Repository documentation standards, audit workflow, and invariants.
