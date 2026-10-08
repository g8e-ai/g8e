---
doc_id: tests
title: Testing Guide
audience: maintainers and coding agents
status: current
last_updated: 2026-10-06
version: v2.3.2
owners:
  - internal/cli/cmd/test/
  - internal/testutil/
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

Defines the four-tier test model, execution entry points, fixture lifecycle, specialized test harnesses, and testing invariants for the g8e Go platform, Ensemble, Console, and protocol packages.

The test suite is structured into four distinct tiers:
- **Tier 1 (Unit)**: Fast, in-memory, hermetic unit tests that execute without a running platform, external databases, or network listeners.
- **Tier 2 (In-Process Integration)**: In-process integration tests that exercise real local SQLite databases, local PKI generation, pub/sub, and the in-process Gateway with race detection on non-Windows hosts.
- **Tier 3 (Live Platform E2E)**: End-to-end tests exercising deployed or containerized service boundaries, including steady-state platform verification and stateful enrollment topologies.
- **Tier 4 (External-Service Integration)**: Tests calling live external LLM providers or network services, gated by credentials and pytest markers.

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
| INV-TEST-RUN-05 | Tier 4 tests requiring external network access, live LLM endpoints, or cloud credentials MUST be isolated behind dedicated markers and executed through explicit targets (`make ensemble-test-external`, `make test-external`). Tiers 1 through 3 MUST NOT depend on external APIs or credentials. |

### Test structure and isolation (`INV-TEST-ISO`)

| ID | Rule |
| --- | --- |
| INV-TEST-ISO-01 | Tests MUST be table-driven where applicable and MUST use descriptive names that state the exact behavior verified. MUST NOT use generic test names like `TestCoverage`, `TestMisc`, or `TestEdgeCases`. |
| INV-TEST-ISO-02 | Tests MUST use `testutil.TempDir(t)` for isolated runtime roots and pass the resulting base directory to `fs.NewRuntimeFileService` and `paths.InitWithBase`. MUST NOT append `.g8e` manually. |
| INV-TEST-ISO-03 | Tests MUST NOT use `os.Chdir` to align runtime state. `os.Chdir` is permitted only for behavior requiring directory discovery (e.g. Swagger source discovery), and MUST restore the original working directory with `t.Cleanup`. |
| INV-TEST-ISO-04 | Tests MUST use typed constants from `internal/constants/` for statuses, paths, reason strings, and permissions instead of ad hoc string literals. |
| INV-TEST-ISO-05 | Tests that discover user configuration MUST isolate both `HOME` and `USERPROFILE` with `t.Setenv`; Go's `os.UserHomeDir` reads `USERPROFILE` on Windows. Browser approval tests MUST inject a browser opener so automated suites do not launch a real browser. |

### Fixture lifecycle (`INV-TEST-FIX`)

| ID | Rule |
| --- | --- |
| INV-TEST-FIX-01 | `NewGatewayFixture` registers its own teardown via `t.Cleanup`. Callers MUST NOT close databases, stop the Gateway, or close downstream servers a second time. |
| INV-TEST-FIX-02 | Temporary credentials and fixture resources MUST register cleanup with `t.Cleanup`. Setup helpers MUST NOT use helper-local `defer` statements that run before the test body. |
| INV-TEST-FIX-03 | Tests MUST provide explicit cancellation contexts and join background goroutines before test completion. |
| INV-TEST-FIX-04 | `./g8e test e2e-docker` manages the Docker Compose stack lifecycle (`docker compose up -d` / `docker compose down -v`), waits up to 60 seconds for Gateway and Ensemble health, and executes Tier 3 tests against real local containers. Passing `--cross-enrollment` additionally activates the secondary gateway container for cross-enrollment test scenarios. |

## Owned surfaces

| Claim | Path | Verify |
| --- | --- | --- |
| Test tiers and CLI commands | `internal/cli/cmd/test/test.go` | `./g8e test --help` |
| Chaos testing harness and summary | `internal/tools/chaos/`, `internal/cli/cmd/test/chaos.go` | `./g8e test chaos --help`, `./g8e test summary` |
| Public loop qualification harness | `internal/cli/cmd/test/public_loop.go` | `./g8e test public-loop --help` |
| Makefile platform test targets | `Makefile` | `make test`, `make test-unit`, `make test-integration`, `make test-coverage`, `make test-docker`, `make test-cross-enrollment`, `make test-airgap` |
| Cross-component test targets | `Makefile`, `ensemble/`, `console/`, `protocol/` | `make ensemble-test`, `make ensemble-test-external`, `make console-test`, `make ci-protocol` |
| Integration Gateway fixture | `test/fixtures/gateway_fixture.go` | `NewGatewayFixture` |
| File service test isolation | `internal/testutil/paths.go`, `internal/services/fs/file_service.go` | `testutil.TempDir` |
| Socket test tags and serial boundary tests | `internal/testutil/test_tiers_test.go` | `TestTestTiers_ListenersRequireIntegrationAndBoundaryTestsStaySerial` |
| Windows port probes avoid listening and detect address conflicts | `internal/netutil/port_probe_windows.go`, `internal/netutil/port_probe_windows_integration_test.go` | `TestTCPPortProbe_BindsWithoutAcceptingConnectionsAndReleasesPort`, `TestTCPPortProbe_RejectsIPv4AndIPv6LoopbackConflicts` (Windows Tier 2) |
| Live E2E suite | `test/e2e/` | `./g8e test e2e` |
| Completion transcript wire contract (Go side) | `internal/services/gateway/platform_enrollment_validation_test.go` | `TestPlatformEnrollmentCompletionTranscriptGoldenVector` |

## Procedures

### Run Test Tiers

```bash
# Tier 1: Fast unit tests (no network or databases, parallel across packages)
./g8e test unit
make test-unit

# Narrow Tier 1 by package pattern or test name
./g8e test unit --pkg ./internal/cli/cmd/test --run TestTestUnitCmd

# Tier 2: In-process integration tests (local SQLite, PKI, pub/sub, race detector)
./g8e test integration
make test-integration

# Narrow Tier 2 by package pattern or test name
./g8e test integration --pkg ./internal/cli/cmd/gw --run TestGatewayConnect

# Tier 3: Live platform E2E (against an already-running stack)
./g8e test e2e
make test-docker

# Tier 3: Specific scenario tests against a running stack
./g8e test e2e --run TestPlatformEnrollment_PendingDiscovery
./g8e test e2e --run TestCrossEnrollment

# Tier 3: Docker-backed E2E (starts compose stack, waits for health, tests, tears down)
./g8e test e2e-docker

# Tier 3: Docker-backed E2E with cross-enrollment profile
./g8e test e2e-docker --cross-enrollment
make test-cross-enrollment

# Tier 4: External-service integration tests (real LLM/API calls, gated by credentials)
make ensemble-test-external

# Enforce coverage threshold (75% minimum, atomic mode across integration suite)
./g8e test coverage
make test-coverage

# Narrow coverage run by package or verbosity
./g8e test coverage --pkg ./internal/services/auth --verbose

# Static analysis and linting
./g8e test lint
make lint
```

### Run Specialized Test Harnesses

```bash
# Run chaos testing: fire synthetic governance events directly in-process
./g8e test chaos --count 100

# View aggregated chaos test results from the test vault database
./g8e test summary

# Run provider-free public feed qualification loop
./g8e test public-loop --candidate candidate.json --output evidence.json

# Verify air-gapped vendored compilation without network access
make test-airgap
```

`./g8e test chaos` generates a realistic distribution of governance events (70% valid good actor intent, 20% L1 forbidden command prompt injection, 10% corrupted transaction hash MitM) directly in-process through `TransactionVerifier` and `Actuator`, bypassing network and TLS layers. Results persist to SQLite databases under `.g8e/test-vault/<timestamp>-chaos-test/`. `./g8e test summary` aggregates outcomes across all runs in the test vault directory.

`./g8e test public-loop` verifies candidate identity records against public feeds, executes an in-process qualification run in an isolated temporary runtime, and writes typed public-loop evidence JSON. Both `--candidate` and `--output` are required flags.

### Cross-Component Test Execution

```bash
# Ensemble (Python): unit and in-process integration tests (Tier 1 + Tier 2)
make ensemble-test

# Ensemble: external LLM and live provider tests (Tier 4)
make ensemble-test-external

# Ensemble: linting and typechecking
make ensemble-lint

# Console (TypeScript): Vitest unit and component tests
make console-test

# Console: linting, typechecking, and embedded asset verification
make console-lint
make console-embed-check

# Protocol: Python pytest, conformance tests, and Node TypeScript checks
make ci-protocol

# Full local CI pipeline across all components. Stages run in sequence even under make -j:
# console and adapter, platform, protocol, Ensemble, website, scripts.
# It refreshes generated artifacts locally; GitHub Actions separately enforces committed freshness.
make ci
```

### Add a Hermetic Command Test

1. Use `cmdtest.NewCmdTestEnv(t)` from `internal/cli/cmd/cmdtest/` to obtain an isolated `RuntimeFileService` and matching configuration.
2. Inject `cmdtest.ConfigLoaderFor(cfg)`, client factories, and file-service factories into the command constructor under test.
3. For file-service error paths, add a test case in `internal/cli/cmd/<group>/factory_error_<group>_test.go` using `cmdtest.FailingFileSvcFactory(errFactory)` to verify that `constants.ErrFileServiceInit` is wrapped and downstream calls are skipped.

Hermetic command tests may use small, isolated file fixtures. `testutil.TempDir` returns an absolute base directory backed by `testing.T.TempDir`, outside the source tree by default; the owning test removes it regardless of later working directory changes. Pass the base directory directly to the runtime file service.

### Native Windows Tests

Run the root Makefile targets from PowerShell with Git for Windows Bash and GNU Make available. `make dev-setup` installs the pinned contributor tools, and `make dev-check` verifies prerequisites before `make ci`. Full local CI covers the Console and adapter, Go platform, protocol, Ensemble, website, and scripts. `make _ci-test` runs uncached unit tests across all Go packages followed by the integration suite with coverage; the coverage threshold is 75%.

The Makefile omits `-race` on Windows (`TEST_RACE`). Run integration tests on Linux or macOS for race-detector verification. Windows file mode assertions use `testutil.FileMode`: Go exposes the read-only attribute, not Unix ownership or Windows ACL protection. `testutil.Symlink` skips a fixture only when Windows denies symlink privilege; enable Developer Mode or grant the symlink privilege to exercise those cases.

For tests that discover user configuration, set both home variables (`HOME` and `USERPROFILE`) to the same test-owned directory before constructing the command or service. Isolate `APPDATA` and `LOCALAPPDATA` too when exercising Windows application-config discovery. Setting only `HOME` can write into the developer's real profile. Fixtures passed to a runtime file service must share its drive when the test uses `filepath.Rel`; a temporary directory on another drive has no relative path.

Browser approval tests inject their browser opener. Windows background Operator and helper processes set `HideWindow` so they do not open console windows. Assertions on process attributes verify that setup; automated suite success alone does not prove that every desktop interaction is invisible.

Port-availability checks share `netutil.CheckTCPPortAvailable`. On Windows, the probe binds an exclusive wildcard TCP socket and closes it without calling `listen`, avoiding Windows Defender Firewall prompts for temporary test binaries. A dual-stack probe checks both IPv4 and IPv6; IPv4-only hosts use an IPv4 probe. This does not reserve the port for a later Gateway start. A real Gateway listening on network interfaces can still require firewall approval.

### Diagnose Slow Tests

The enrollment burst deadline contract (`TestPlatformEnrollmentBurst`) runs only on a normal build; use `GOFLAGS='-run=^TestPlatformEnrollmentBurst$ -v' make test-integration TEST_RACE= TEST_PKGS=./internal/services/gateway` to measure it.

Measure uncached execution through the canonical entry point. Integration runs already use `-count=1` and the race detector on non-Windows. The package duration in an `ok` line includes test setup and cleanup, but excludes compilation.

```bash
# Preserve individual test durations while keeping the normal integration settings.
GOFLAGS=-json ./g8e test integration --pkg ./internal/services/gateway > /tmp/g8e-tests.jsonl

# Profile a representative selection in one package.
GOFLAGS=-cpuprofile=/tmp/g8e-gateway.cpu ./g8e test integration --pkg ./internal/services/gateway --run '^TestHandleBootstrap'
go tool pprof -top -cum /tmp/g8e-gateway.cpu
```

The CLI prints progress lines alongside Go's JSON events; ignore non-JSON lines when parsing the capture. Compare top-level `pass` events for package totals and individual tests. Do not add parent and subtest durations together.

Use `testing/synctest` for self-contained timer, timeout, and cancellation tests whose goroutines communicate through in-process channels. It advances virtual time while preserving production deadlines and waits for the bubble's goroutines to exit. Do not use it around real network listeners or subprocesses. For HTTP or process tests, synchronize on explicit events, cancel contexts, and join owned goroutines. An ordering test should finish after proving the ordering; it should not wait for an unrelated enrollment timeout to release a stub TUI.

Construct only the dependencies the behavior exercises. Use `httptest.NewRecorder` and a minimal handler for middleware unit tests. Reserve SQLite/PKI/Gateway fixtures for real integration boundaries, and keep one authoritative test for each behavior instead of repeating a unit assertion inside a full fixture. Keep race detection and per-test isolation when optimizing fixture cost.

Tests that open localhost listeners, including `httptest.NewServer`, `httptest.NewTLSServer`, and `httptest.NewUnstartedServer`, belong behind the `integration` build tag. Split mixed files so pure parsing and validation tests remain in Tier 1; move callers of listener-opening helpers with those helpers. Remove `t.Parallel()` from the moved tests. The Tier 1 source check in `internal/testutil/test_tiers_test.go` catches direct listener/dial calls without tier tags and parallel calls in files restricted to Integration/E2E, including files for other operating systems. It does not trace production calls; review those dependencies when choosing a tier.

## Anti-patterns

- Calling `t.Parallel()` in Tier 2 or Tier 3 tests (INV-TEST-RUN-04).
- Invoking `go test` directly for platform suites instead of `./g8e test` or `make` (INV-TEST-RUN-01).
- Mocking databases or internal services in integration suites (INV-TEST-RUN-03).
- Calling external network APIs or requiring live credentials during Tier 1, Tier 2, or Tier 3 test execution (INV-TEST-RUN-02, INV-TEST-RUN-05).
- Calling `os.Chdir` without restoring the original directory in `t.Cleanup` (INV-TEST-ISO-03).
- Double-closing resources managed by `NewGatewayFixture` (INV-TEST-FIX-01).
- Leaving temporary directories or background goroutines unjoined when writing integration tests (INV-TEST-FIX-02, INV-TEST-FIX-03).
- Describing `bootstrapped` as an active Compose profile in `docker-compose.yml` (INV-TEST-13).
- Asserting a panic on a production code path (e.g. `assert.Panics`) instead of asserting the returned typed error with `assert.ErrorIs` against the sentinel in `internal/constants/errors.go` (INV-CODE-06, INV-ERR-01).

## Links out

- [Developer Guidelines](devs.md): coding invariants and repository standards.
- [Code Map](codemap.md): package and runtime ownership maps.
- [Documentation Guide](docs.md): documentation audit, catalog, and formatting standards.
- [Release Process](release_process.md): native evaluation acceptance and release verification.
- [Ensemble Testing](../ensemble/tests.md): pytest fixtures, fakes, and external credential gates.
- [Console Architecture & Development](../architecture/console.md#test): Vitest, typecheck, ESLint, and embed checks.

### Scale-test cohorts

`TestOperatorFleet_HoldsUnderFanOut` accepts `G8E_E2E_FLEET_SESSIONS`, a
comma-separated list of owner Operator session UUIDs. Its length must match
`G8E_E2E_FLEET_SIZE`. With a cohort, registry health checks and governed CLI
fan-out target exactly those remote sessions; the embedded Operator is excluded.
Missing or duplicate sessions fail closed, and selected stopped/stale sessions
remain subject to health assertions. Without a cohort the existing whole-registry
scenario and `--all-active` dispatch remain in effect. A failed enrollment, soak,
or fan-out stops subsequent stages. Reports label the target scope.
