---
doc_id: tests
title: Testing Guide
audience: maintainers and coding agents
status: current
last_updated: 2026-10-05
version: v2.2.6
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

Defines the four-tier test model, execution entry points, fixture lifecycle, and testing invariants for the g8e Go platform, Ensemble, Console, and protocol packages.

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
| INV-TEST-ISO-05 | Tests that discover user configuration MUST isolate both `HOME` and `USERPROFILE` with `t.Setenv`; Go's `os.UserHomeDir` reads `USERPROFILE` on Windows. Browser approval tests MUST inject a browser opener so automated suites do not launch a real browser. |

### Fixture lifecycle (`INV-TEST-FIX`)

| ID | Rule |
| --- | --- |
| INV-TEST-FIX-01 | `NewGatewayFixture` registers its own teardown via `t.Cleanup`. Callers MUST NOT close databases, stop the Gateway, or close downstream servers a second time. |
| INV-TEST-FIX-02 | Temporary credentials and fixture resources MUST register cleanup with `t.Cleanup`. Setup helpers MUST NOT use helper-local `defer` statements that run before the test body. |
| INV-TEST-FIX-03 | Tests MUST provide explicit cancellation contexts and join background goroutines before test completion. |
| INV-TEST-FIX-04 | `./g8e test e2e-docker` manages the Docker Compose stack lifecycle (`docker compose up -d` / `docker compose down -v`), waits up to 60 seconds for Gateway and Ensemble health, and executes Tier 3 tests against real local containers. |

## Owned surfaces

| Claim | Path | Verify |
| --- | --- | --- |
| Test tiers and CLI commands | `internal/cli/cmd/test/test.go` | `./g8e test --help` |
| Makefile test targets | `Makefile` | `make test`, `make test-unit`, `make test-integration`, `make test-coverage` |
| Integration Gateway fixture | `test/fixtures/gateway_fixture.go` | `NewGatewayFixture` |
| File service test isolation | `internal/testutil/paths.go`, `internal/services/fs/file_service.go` | `testutil.TempDir` |
| Socket test tags and serial boundary tests | `internal/testutil/test_tiers_test.go` | `TestTestTiers_ListenersRequireIntegrationAndBoundaryTestsStaySerial` |
| Windows port probes avoid listening and detect address conflicts | `internal/netutil/port_probe_windows.go` | `TestTCPPortProbe_BindsWithoutAcceptingConnectionsAndReleasesPort`, `TestTCPPortProbe_RejectsIPv4AndIPv6LoopbackConflicts` (Windows Tier 2) |
| Live E2E suite | `test/e2e/` | `./g8e test e2e` |
| Completion transcript wire contract (Go side) | `internal/services/gateway/platform_enrollment_validation_test.go` | `TestPlatformEnrollmentCompletionTranscriptGoldenVector` |

## Procedures

### Run Test Tiers

```bash
# Tier 1: Fast unit tests (no network or databases, parallel across packages)
./g8e test unit

# Narrow Tier 1 by package pattern or test name
./g8e test unit --pkg ./internal/cli/cmd/test --run TestTestUnitCmd

# Tier 2: In-process integration tests (local SQLite, PKI, pub/sub, race detector)
./g8e test integration

# Narrow Tier 2 by package pattern or test name
./g8e test integration --pkg ./internal/cli/cmd/gw --run TestGatewayConnect

# Tier 3: Live platform E2E (against a running stack)
./g8e test e2e

# Tier 3: Docker-backed E2E (requires Docker; Compose up, wait for health, test, compose down -v)
./g8e test e2e-docker

# Static analysis and linting
./g8e test lint
```

### Add a Hermetic Command Test

1. Use `cmdtest.NewCmdTestEnv(t)` from `internal/cli/cmd/cmdtest/` to obtain an isolated `RuntimeFileService` and matching configuration.
2. Inject `cmdtest.ConfigLoaderFor(cfg)`, client factories, and file-service factories into the command constructor under test.
3. For file-service error paths, add a test case in `internal/cli/cmd/<group>/factory_error_<group>_test.go` using `cmdtest.FailingFileSvcFactory(errFactory)` to verify that `constants.ErrFileServiceInit` is wrapped and downstream calls are skipped.

Hermetic command tests may use small, isolated file fixtures. `testutil.TempDir`
returns an absolute base directory backed by `testing.T.TempDir`, outside the
source tree by default; the owning test removes it regardless of later working
directory changes. Pass the base directory directly to the runtime file service.

### Native Windows Tests

Run the root Makefile targets from PowerShell with Git for Windows Bash and GNU
Make available. `make dev-setup` installs the pinned contributor tools, and
`make dev-check` verifies prerequisites before `make ci`. Full local CI covers
the Console and adapter, Go platform, protocol, Ensemble, website, and scripts.
`make _ci-test` runs uncached unit tests across all Go packages followed by the
integration suite with coverage; the coverage threshold is 75%.

The Makefile omits `-race` on Windows. Run integration tests on Linux or macOS
for race-detector verification. Windows file mode assertions use
`testutil.FileMode`: Go exposes the read-only attribute, not Unix ownership or
Windows ACL protection. `testutil.Symlink` skips a fixture only when Windows
denies symlink privilege; enable Developer Mode or the symlink privilege to
exercise those cases.

For tests that discover user configuration, set both home variables to the same
test-owned directory before constructing the command or service. Isolate
`APPDATA` and `LOCALAPPDATA` too when exercising Windows application-config
discovery. Setting only `HOME` can write into the developer's real profile.
Fixtures passed to a runtime file service must share its drive when the test
uses `filepath.Rel`; a temporary directory on another drive has no relative path.

Browser approval tests inject their browser opener. Windows background Operator
and helper processes set `HideWindow` so they do not open console windows.
Assertions on process attributes verify that setup; automated suite success
alone does not prove that every desktop interaction is invisible.

Port-availability checks share `netutil.CheckTCPPortAvailable`. On Windows,
the probe binds an exclusive wildcard TCP socket and closes it without calling
`listen`, avoiding Windows Defender Firewall prompts for temporary test
binaries. A dual-stack probe checks both IPv4 and IPv6; IPv4-only hosts use an
IPv4 probe. This does not reserve the port for a later Gateway start. A real
Gateway listening on network interfaces can still require firewall approval.

### Diagnose Slow Tests

Measure uncached execution through the canonical entry point. Integration runs
already use `-count=1` and the race detector on non-Windows. The package duration
in an `ok` line includes test setup and cleanup, but excludes compilation.

```bash
# Preserve individual test durations while keeping the normal integration settings.
GOFLAGS=-json ./g8e test integration --pkg ./internal/services/gateway > /tmp/g8e-tests.jsonl

# Profile a representative selection in one package.
GOFLAGS=-cpuprofile=/tmp/g8e-gateway.cpu ./g8e test integration --pkg ./internal/services/gateway --run '^TestHandleBootstrap'
go tool pprof -top -cum /tmp/g8e-gateway.cpu
```

The CLI prints progress lines alongside Go's JSON events; ignore non-JSON lines
when parsing the capture. Compare top-level `pass` events for package totals and
individual tests. Do not add parent and subtest durations together.

Use `testing/synctest` for self-contained timer, timeout, and cancellation tests
whose goroutines communicate through in-process channels. It advances virtual
time while preserving production deadlines and waits for the bubble's goroutines
to exit. Do not use it around real network listeners or subprocesses. For HTTP or
process tests, synchronize on explicit events, cancel contexts, and join owned
goroutines. An ordering test should finish after proving the ordering; it should
not wait for an unrelated enrollment timeout to release a stub TUI.

Construct only the dependencies the behavior exercises. Use `httptest.NewRecorder`
and a minimal handler for middleware unit tests. Reserve SQLite/PKI/Gateway
fixtures for real integration boundaries, and keep one authoritative test for
each behavior instead of repeating a unit assertion inside a full fixture.
Keep race detection and per-test isolation when optimizing fixture cost.

Tests that open localhost listeners, including `httptest.NewServer`, `httptest.NewTLSServer`, and `httptest.NewUnstartedServer`, belong behind the `integration` build tag. Split mixed files so pure parsing and validation tests remain in Tier 1; move callers of listener-opening helpers with those helpers. Remove `t.Parallel()` from the moved tests. The Tier 1 source check in `internal/testutil/test_tiers_test.go` catches direct listener/dial calls without tier tags and parallel calls in files restricted to Integration/E2E, including files for other operating systems. It does not trace production calls; review those dependencies when choosing a tier.

## Anti-patterns

- Calling `t.Parallel()` in Tier 2 or Tier 3 tests (INV-TEST-RUN-04).
- Invoking `go test` directly for platform suites instead of `./g8e test` or `make` (INV-TEST-RUN-01).
- Mocking databases or internal services in integration suites (INV-TEST-RUN-03).
- Calling `os.Chdir` without restoring the original directory in `t.Cleanup` (INV-TEST-ISO-03).
- Double-closing resources managed by `NewGatewayFixture` (INV-TEST-FIX-01).
- Asserting a panic on a production code path (e.g. `assert.Panics`) instead of asserting the returned typed error with `assert.ErrorIs` against the sentinel in `internal/constants/errors.go` (INV-CODE-06, INV-ERR-01).

## Links out

- [Developer Guidelines](devs.md): coding invariants and repository standards.
- [Code Map](codemap.md): package and runtime ownership maps.
- [Documentation Guide](docs.md): documentation audit, catalog, and formatting standards.
- [Release Process](release_process.md): native evaluation acceptance and release verification.
- [Ensemble Testing](../ensemble/tests.md): pytest fixtures, fakes, and external credential gates.
- [Console Architecture & Development](../architecture/console.md#test): Vitest, typecheck, ESLint, and embed checks.
