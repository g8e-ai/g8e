---
doc_id: tests
title: Testing Guide
audience: maintainers and coding agents
status: current
last_updated: 2026-09-26
version: v2.2.0
owners:
  - internal/cli/cmd/test/test.go
  - Makefile
  - test/e2e/
  - test/fixtures/
related:
  - docs/devs/devs.md
  - docs/devs/codemap.md
  - docs/devs/docs.md
  - docs/devs/release_process.md
when_to_read: Running tests, writing regression tests, adding fixtures, or updating test workflows.
do_not_use_for:
  - Coding standards and general invariants (docs/devs/devs.md)
  - Documentation audit and generation workflow (docs/devs/docs.md)
  - Release procedure and compliance bundles (docs/devs/release_process.md)
  - Diagnostic triage and recovery (docs/devs/troubleshooting.md)
---

# Testing Guide

## Purpose

Defines the four-tier test model, execution entry points, fixture lifecycle, and testing invariants for the g8e Go platform, Ensemble, Dashboard, and protocol packages.

## Quick index

- [Purpose](#purpose)
- [Invariants](#invariants)
- [Owned surfaces](#owned-surfaces)
- [Procedures](#procedures)
- [Anti-patterns](#anti-patterns)
- [Links out](#links-out)

Invariant groups: [Test execution](#test-execution-inv-test-run), [Test structure and isolation](#test-structure-and-isolation-inv-test-iso), [Fixture lifecycle](#fixture-lifecycle-inv-test-fix).

## Invariants

Ids are stable. Append the next free number in a topic. Do not renumber.

### Test execution (`INV-TEST-RUN`)

| ID | Rule |
| --- | --- |
| INV-TEST-RUN-01 | Platform suites MUST run through `./g8e test ...` or the root Makefile targets. MUST NOT invoke `go test` directly for platform suites (protocol package targets are the exception). |
| INV-TEST-RUN-02 | Tier 1 tests MUST stay independent of a running platform, network, or external databases. |
| INV-TEST-RUN-03 | Tier 2 and Tier 3 tests MUST exercise real local or deployed boundaries. MUST NOT mock internal services, database clients, or cross-component communication in those tiers. |
| INV-TEST-RUN-04 | Integration (Tier 2) and E2E (Tier 3) tests MUST NOT call `t.Parallel()`. |

### Test structure and isolation (`INV-TEST-ISO`)

| ID | Rule |
| --- | --- |
| INV-TEST-ISO-01 | Tests MUST be table-driven where applicable and MUST use descriptive names that state the exact behavior verified. MUST NOT use generic test names like `TestCoverage`, `TestMisc`, or `TestEdgeCases`. |
| INV-TEST-ISO-02 | Tests MUST use `testutil.TempDir(t)` for isolated runtime roots and pass the resulting base directory to `fs.NewRuntimeFileService` and `paths.InitWithBase`. MUST NOT append `.g8e` manually. |
| INV-TEST-ISO-03 | Tests MUST NOT use `os.Chdir` to align runtime state. `os.Chdir` is permitted only for behavior requiring directory discovery (e.g. demos, Swagger source discovery), and MUST restore the original working directory with `t.Cleanup`. |
| INV-TEST-ISO-04 | Tests MUST use typed constants from `internal/constants/` for statuses, paths, reason strings, and permissions instead of ad hoc string literals. |

### Fixture lifecycle (`INV-TEST-FIX`)

| ID | Rule |
| --- | --- |
| INV-TEST-FIX-01 | `NewGatewayFixture` registers its own teardown via `t.Cleanup`. Callers MUST NOT close databases, stop the Gateway, or close downstream servers a second time. |
| INV-TEST-FIX-02 | Temporary credentials and fixture resources MUST register cleanup with `t.Cleanup`. Setup helpers MUST NOT use helper-local `defer` statements that run before the test body. |
| INV-TEST-FIX-03 | Tests MUST provide explicit cancellation contexts and join background goroutines before test completion. |
| INV-TEST-FIX-04 | `./g8e test e2e-full` manages the Docker Compose stack lifecycle (`docker compose up -d` / `docker compose down -v`), waits up to 60 seconds for Gateway and Ensemble health, and executes Tier 3 tests against real local containers. |

## Owned surfaces

| Claim | Path | Verify |
| --- | --- | --- |
| Test tiers and CLI commands | `internal/cli/cmd/test/test.go` | `./g8e test --help` |
| Makefile test targets | `Makefile` | `make test`, `make test-unit`, `make test-integration`, `make test-coverage` |
| Integration Gateway fixture | `test/fixtures/gateway_fixture.go` | `NewGatewayFixture` |
| File service test isolation | `internal/testutil/tempdir.go`, `internal/services/fs/file_service.go` | `testutil.TempDir` |
| Live E2E suite | `test/e2e/` | `./g8e test e2e` |

## Procedures

### Run Test Tiers

```bash
# Tier 1: Fast unit tests (no network, no disk I/O, parallel across packages)
./g8e test unit

# Tier 2: In-process integration tests (local SQLite, PKI, pub/sub, race detector)
./g8e test integration

# Narrow Tier 2 by package pattern or test name
./g8e test integration --pkg ./internal/cli/cmd/gw --run TestGatewayConnect

# Tier 3: Live platform E2E (against a running stack)
./g8e test e2e

# Tier 3: Full lifecycle E2E (Compose up, wait for health, test, compose down -v)
./g8e test e2e-full

# Static analysis and linting
./g8e test lint
```

### Add a Hermetic Command Test

1. Use `cmdtest.NewCmdTestEnv(t)` from `internal/cli/cmd/cmdtest/` to obtain an isolated `RuntimeFileService` and matching configuration.
2. Inject `cmdtest.ConfigLoaderFor(cfg)`, client factories, and file-service factories into the command constructor under test.
3. For file-service error paths, add a test case in `internal/cli/cmd/<group>/factory_error_<group>_test.go` using `cmdtest.FailingFileSvcFactory(errFactory)` to verify that `constants.ErrFileServiceInit` is wrapped and downstream calls are skipped.

## Anti-patterns

- Calling `t.Parallel()` in Tier 2 or Tier 3 tests (INV-TEST-RUN-04).
- Invoking `go test` directly for platform suites instead of `./g8e test` or `make` (INV-TEST-RUN-01).
- Mocking databases or internal services in integration suites (INV-TEST-RUN-03).
- Calling `os.Chdir` without restoring the original directory in `t.Cleanup` (INV-TEST-ISO-03).
- Double-closing resources managed by `NewGatewayFixture` (INV-TEST-FIX-01).

## Links out

- [Developer Guidelines](devs.md): coding invariants and repository standards.
- [Code Map](codemap.md): package and runtime ownership maps.
- [Documentation Guide](docs.md): documentation audit, catalog, and formatting standards.
- [Release Process](release_process.md): native evaluation acceptance and release verification.
- [Ensemble Testing](../ensemble/tests.md): pytest fixtures, fakes, and external credential gates.
- [Dashboard Testing](../dashboard/tests.md): Vitest and ESLint testing.
