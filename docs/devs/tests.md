---
title: Tests
---

# Testing g8e

g8e uses four test tiers across the Go platform, Ensemble, evals, Dashboard, and protocol packages. Unit tests isolate code paths, integration tests use local platform infrastructure, live end-to-end tests exercise deployed services over their public interfaces, and external tests call third-party providers.

## Test tiers

| Tier | Scope | Selection | Dependencies |
| --- | --- | --- | --- |
| **Tier 1: Unit** | Go packages under `internal/` and `protocol/`, Ensemble unit tests, eval unit tests, and Dashboard unit tests | Untagged Go tests, pytest unit paths or markers, and Vitest | No live platform or third-party service |
| **Tier 2: In-process integration** | Go integration tests under `internal/` and `test/`, Ensemble integration tests, and eval integration tests | Go `integration` build tag and pytest integration paths or markers | Local files, processes, SQLite, PKI, pub/sub, and in-process services |
| **Tier 3: Live platform E2E** | Network assertions under `test/e2e/` | Go `e2e` build tag | Running Gateway, Operator, Ensemble, and Dashboard with local owner credentials |
| **Tier 4: External** | Ensemble tests that call live LLM, search, or other provider APIs | `ai_integration`, `requires_web_search`, or `requires_api` pytest markers | Configured third-party credentials and endpoints |

The Go integration command enables the `integration` build tag across `./...`, so it runs untagged tests in addition to integration-tagged tests. The E2E command runs `test/e2e/` with the `e2e` tag and also executes untagged helper tests in that package.

## Core rules

- Reproduce a bug with a failing regression test before changing production code.
- Use table-driven tests with `testify/assert` and `testify/require` where applicable.
- Give test functions, subtests, and files names that state the behavior under test. Do not use generic names such as `TestCoverage`, `TestEdgeCases`, `TestGap`, `TestMisc`, `edge_test.go`, `coverage_test.go`, or `misc_test.go`.
- Keep Tier 1 tests independent of a running platform and third-party services. Tier 2 and Tier 3 tests use real local platform boundaries instead of mocking internal services, database clients, or cross-component communication.
- Do not use `t.Parallel()` in integration or E2E tests. The Go E2E runner also passes `-parallel=1` as a backstop.
- Run Go platform suites through `./g8e test` or the repository Makefile targets. Do not invoke `go test` directly for platform tests.
- Go test targets enable the race detector on non-Windows platforms and disable test caching with `-count=1` where the runner controls the invocation.
- Give goroutines explicit cancellation contexts and clear ownership. Join long-running goroutines before the test completes.
- Register resource and temporary credential cleanup with `t.Cleanup`. A setup helper must not defer cleanup that needs to remain active for the test body.
- Use typed constants from `internal/constants/` in assertions instead of duplicating status values, reason strings, paths, or permissions.
- Use contract tests to keep Go, Python, generated protobufs, JSON schemas, and `protocol/constants/` aligned.

## Running the Go platform suites

The `g8e test` command is the primary entry point:

- `./g8e test unit` runs Tier 1 tests under `internal/` and `protocol/`.
- `./g8e test integration` runs the repository with the `integration` build tag and local integration infrastructure.
- `./g8e test e2e` runs Tier 3 tests against an already running platform.
- `./g8e test e2e --run <regexp>` selects E2E tests compatible with the platform state prepared by the user.
- `./g8e test e2e-full` starts the root Compose stack with the `bootstrapped` profile, waits for Gateway and Ensemble health, runs E2E tests, and tears the stack down with `docker compose down -v` when the command exits.
- `./g8e test e2e-full --run <regexp>` limits the full-lifecycle command to a compatible scenario.
- `./g8e test coverage` delegates to `make test-coverage`; `--pkg` narrows the package and `--verbose` enables verbose test output.
- `./g8e test lint` runs `golangci-lint`. It installs the repository-pinned linter version if the binary is unavailable.
- `./g8e test chaos` generates governance test events, and `./g8e test summary` reads the latest chaos run from the test vault.

`e2e-full` removes Compose volumes during teardown. Use it only when discarding the stack state is intentional. It does not perform the interactive owner and component enrollment walkthrough, so the selected tests must match the state available to the started Compose stack.

## Makefile targets

The root Makefile provides these test and quality entry points:

- `make test` runs `test-unit` followed by `test-integration`.
- `make test-unit` runs the configured Tier 1 package set serially and excludes packages listed in `TEST_EXCLUDE_PKGS`.
- `make test-integration` runs all Go packages serially with the `integration` build tag.
- `make test-docker` runs the approved steady-state E2E subset through `./g8e test e2e`.
- `make test-coverage` runs the configured Go package set with the `integration` tag, atomic coverage, repository exclusions, and a 75 percent minimum. `PKG` and `VERBOSE` customize the invocation.
- `make test-airgap` validates the vendored Go build and pinned demo assets without downloading dependencies.
- `make ensemble-test` runs Ensemble Tier 1 and non-external Tier 2 tests.
- `make evals-test`, `make evals-test-unit`, and `make evals-test-integration` run the standalone eval package in its locked `uv` environment.
- `make test-external` runs Tier 4 Ensemble tests selected by external-service markers.
- `make dashboard-test` runs the Dashboard Vitest suite.
- `make lint` runs the Go build check, vulnerability scan, doctrine and COSAiS validation, Swagger generation, and `golangci-lint`.
- `make ci` aggregates the local platform, Ensemble, and Dashboard checks. GitHub Actions also runs protocol Python, conformance, eval, website, smoke, dependency, secret, and license jobs.

The package and file exclusions in the Makefile are the source of truth for Go test discovery and coverage filtering. The CLI unit runner and `make test-unit` do not use identical package selection or timeout flags, so use the same entry point locally that the relevant CI job uses when reproducing a failure.

## Runtime files and test paths

`RuntimeFileService` in `internal/services/fs` is the canonical abstraction for `.g8e/` test I/O. Construct relative runtime paths from constants in `internal/constants/paths.go`, resolve absolute paths with `fileSvc.Resolve`, and convert absolute runtime paths back with the file service relative-path method.

Use `fileSvc.ReadFile`, `fileSvc.WriteFile`, `fileSvc.Stat`, `fileSvc.Remove`, and `fileSvc.FileExists` instead of direct `os` file operations for `.g8e/` state. Missing runtime files return typed errors; assert `errors.Is(err, constants.ErrNotFound)` instead of using `os.IsNotExist`. Use `constants.Perm*` values for permission assertions.

`testutil.TempDir(t)` returns an absolute temporary base directory. Pass it directly to `fs.NewRuntimeFileService` and `paths.InitWithBase`; do not append the runtime directory name yourself. Use `TestPaths` for isolated environments and pass `fileSvc` explicitly to services that perform runtime I/O. CLI test configs use `RuntimeDir` and resolved paths; do not add `DataDir`, `CredentialsDir`, or `PKIDir` fields to those config structs.

The canonical trust bundle is `.g8e/pki/trust/g8eg-ca-bundle.pem`. Tests do not use legacy bundle paths or mutate the developer's local PKI state to repair a failure.

## CLI command tests

Hermetic command tests use the shared environment and dependency-injection helpers in `internal/cli/cmd/testenv_test.go`. The environment pairs a temporary `RuntimeFileService` with a config whose `RuntimeDir` matches the service root, while injected config loaders, file-service factories, and client factories keep command tests independent of the developer's runtime tree and network.

Every command constructor that accepts a file-service factory has a corresponding initialization-error test in `internal/cli/cmd/factory_error_test.go`. The test asserts that the command wraps `constants.ErrFileServiceInit`, preserves the original error for `errors.Is`, and does not call downstream dependencies.

### Working-directory changes

Tests do not use `os.Chdir` to align `.g8e/` runtime state. Command tests inject a temporary file service and config instead.

A working-directory change is limited to behavior that explicitly discovers source-tree or configuration files from the current directory. Current examples include demo and Swagger source discovery, `config.Load("")`, chaos configuration loading, and E2E repository-root discovery. Each test file that changes the process working directory includes a file-level explanation, restores the original directory with `t.Cleanup` or equivalent cleanup, and does not run those tests in parallel.

## Integration fixtures

`GatewayFixture` in `test/fixtures/gateway_fixture.go` starts a Gateway in-process with local SQLite, PKI, pub/sub, governance services, and an MCP or A2A downstream server. `NewGatewayFixture` registers shutdown through `t.Cleanup`; callers do not stop the Gateway, close its databases, or close its generated downstream server a second time.

Fixture scaffolding uses temporary paths, while each Gateway data and vault run is written to a unique directory under `test-results/`. Cleanup stops services and joins the Gateway goroutine but intentionally leaves those result directories for inspection.

`EnrollClientIdentity` creates an enrolled test identity, waits for the operator session to persist, and supports strict mTLS clients for authenticated requests. Integration tests use the generated PKI and session records instead of bypassing authentication. Consensus and notary postures use the shared consensus construction path so production and fixture wiring remain aligned.

## Live E2E tests

`./g8e test e2e` performs network assertions only. `TestMain` loads owner credentials and endpoints from the local `.g8e/` runtime tree, constructs bounded public and mTLS clients with strict certificate verification, and fails non-zero if Gateway, Ensemble, or Dashboard preflight checks fail. The test package does not start, stop, restart, or inspect containers.

The suite contains both approved steady-state tests and tests that require pending, denied, restarted, or gateway-only states. No single deployment state satisfies every scenario, so select stateful tests with `--run` after preparing the required state. `make test-docker` selects the approved steady-state subset defined in the Makefile.

The suite-level preflight requires Gateway, Ensemble, and Dashboard to be reachable. As a result, the current E2E entry point cannot successfully execute the gateway-only headless scenario in `test/e2e/platform_enrollment_headless_e2e_test.go`.

`./g8e test e2e-full` owns Compose startup and teardown around the same network-only test binary. Its lifecycle wrapper does not change test assertions or make mutually exclusive stateful scenarios compatible.

## Protocol and cross-language tests

Go protocol tests cover workload identities and canonicalization vectors. The Python package under `protocol/python/` tests constants, generated protobufs, typed models, receipt verification, compliance models, and package version consistency. The conformance suite under `protocol/conformance/` checks parity among Go definitions, Python runtime values, JSON schemas, constants registries, and shared hash or receipt vectors.

Integration tests for MCP and A2A use the shared adapter and case tables in `test/protocol_test_helpers_test.go`, `test/protocol_payload_test.go`, and `test/protocol_errors_test.go`. Add shared payload or malformed-request cases to those tables so both protocols receive the same assertions. Protocol-specific assertions use typed endpoint, status, and JSON-RPC constants.

## Other component suites

The root test model also applies to first-party non-Go components:

- [Ensemble tests](../ensemble/tests.md) documents pytest paths, markers, fakes, external credential gating, and the standalone eval package.
- [Dashboard tests](../dashboard/tests.md) documents Vitest configuration, browser helpers, server tests, and Dashboard coverage.
- The website uses `npm run check` and `npm run build`; GitHub Actions also validates the Cloudflare deployment bundle with a dry run.

## Continuous integration

The primary GitHub Actions workflow runs generated artifact checks, README drift checks, Go lint and vulnerability scanning, Tier 1 and Tier 2 Go tests, cross-compilation, static-link verification, protocol Python tests, protocol conformance, smoke tests, Ensemble tests, eval tests, Dashboard tests, website tests, secret scanning, dependency auditing, and license checks. Tier 3 live platform E2E and Tier 4 external-provider tests are not part of the primary CI workflow.

The FIPS workflow builds the Linux AMD64 FIPS variant, runs its self-check, runs Go Tier 1 and Tier 2 tests with `GOFIPS140=v1.0.0`, and verifies static linking. Release workflows separately verify binary checksums and signatures, clean Go installation, Python package metadata, and clean Python installation.

See [Documentation guidelines](./docs.md) for the standards used to maintain this document.
