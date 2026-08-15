---
title: Tests
---

# Testing g8e

Last Updated: 2026-08-15

g8e tests run directly on the host using real infrastructure. If it does not work in tests, it will not work in production.

---

## Always

- **Use real infrastructure**: Tests use actual SQLite, PKI certificates, and pub/sub channels. Platform starts via `./g8e gw start`.
- **Reproduce bugs first**: Write a failing test that reproduces the bug before fixing it.
- **Use contract tests**: Enforce alignment between the Operator and `protocol/` constants/models with typed protobuf assertions.
- **Let `NewGatewayFixture` handle cleanup**: `NewGatewayFixture` registers teardown internally via `t.Cleanup`. Callers do not need to defer or register any cleanup.
- **Hold temp credential files for the whole test**: Register their removal with `t.Cleanup`, not `defer`, in any setup helper.
- **Use path constants**: ALL filepath strings in test code must be defined as constants in `internal/constants/paths.go`. Use `paths.Infra.*` (from `internal/paths`) for runtime state paths. The only exception is `TestPaths` for isolated test environments, where the base directory must still come from a constant. Use `constants.PermFileReadOnly` and other `constants.Perm*` constants for file permission assertions in tests.
- **Use `fileSvc.FileExists` instead of `os.IsNotExist`**: `RuntimeFileService` (`internal/services/fs`) is the canonical `.g8e/` file I/O abstraction. Check file existence via `fileSvc.FileExists(ctx, relPath)`, read content via `fileSvc.ReadFile`, and inspect metadata via `fileSvc.Stat`. Assert missing files with `errors.Is(err, constants.ErrNotFound)` instead of `os.IsNotExist`. The `RuntimeFileService` wraps `os.*` calls and returns typed `constants.Err*` errors.
- **`testutil.TempDir` returns absolute paths**: `testutil.TempDir(t)` returns an absolute path to a temporary directory. Use it as the `baseDir` for `fs.NewRuntimeFileService` and `paths.InitWithBase`. Do not join it with `constants.RuntimeDirname` manually; the file service handles that.
- **Do not set `DataDir`/`CredentialsDir` in test configs**: Test configs must use `RuntimeDir` and `fileSvc.Resolve(constants.*)` instead of `DataDir`/`CredentialsDir` fields. These fields have been removed from config structs. Pass `fileSvc` to services that need file I/O.
- **Use typed error constants**: Check for typed constants from `internal/constants/` instead of hardcoded strings in assertions and error message checks.
- **Use regression test markers**: Use standardized marker constants (`RegressionMarkerAfterFix`, `RegressionMarkerBeforeFix`, `RegressionMarkerIssue`) instead of hardcoded strings. See `internal/services/gateway/pki_authority_test.go` for examples.
- **Enable race detection**: `-race` on all non-Windows platforms across all test targets.
- **Use explicit cancellation contexts**: Goroutines require explicit cancellation contexts and clear channel ownership.
- **Use the canonical trust bundle path**: `.g8e/pki/trust/g8eg-ca-bundle.pem`. Contains root CA, hub intermediate, Operator intermediate, and gateway peer CA.
- **Use the shared consensus factory**: `consensus.NewConsensusFromPolicy` is used by both production `BootstrapConsensus` and test `SetupConsensus` to avoid duplication.
- **Keep `storagetest.TestSQLAuditStore` in test code only**: Production code uses `storage.SQLAuditStore` from `audit_store.go`.
- **Use descriptive test names**: Test function names must describe the specific behavior being verified, not generic categories. Good: `TestHandleFsReadRequest_ScrubbingRedactsSecrets`, `TestHandleEvalAnswerRequestSync_TruncatesAnswerExceedingMaxBytes`. Bad: `TestCoverage`, `TestEdgeCases`, `TestGap`, `TestMisc`. Subtest names (`t.Run`) must describe the specific scenario, not just "success" or "error".
- **Use descriptive test filenames**: Test filenames must describe their scope, not generic categories. Good: `file_ops_scrubbing_test.go`, `vault_writer_error_paths_test.go`. Bad: `edge_test.go`, `misc_test.go`, `coverage_test.go`. Do not use "coverage", "gap", or "edge" in test filenames; name the file after the behavior or component it tests.

---

## Never

- **Never mock internal services, database clients, or cross-component communication**: Integration tests use real wire paths. Tier 1 unit tests may use stubs/mocks for external dependencies only.
- **Never use `t.Parallel()` in integration tests**: Each test gets its own isolated data directory and random port. Sequential execution avoids resource contention.
- **Never use `defer` for fixture teardown in a setup helper**: A deferred cleanup fires when the helper returns, tearing the gateway down before the test body runs. `NewGatewayFixture` handles cleanup internally via `t.Cleanup`. Symptom of incorrect cleanup: `sql: database is closed` on the first gateway call.
- **Never clean up twice**: `NewGatewayFixture` registers cleanup exactly once via `t.Cleanup`. Do not add additional cleanup calls that stop the gateway or close databases.
- **Never hardcode filepath strings**: No dynamic path construction with `filepath.Join()` and string literals. No relative paths like `"../../"`, `"./"`, `".g8e/"`, `"/pki/"` inline.
- **Never use legacy trust bundle paths**: Tests fail closed if the canonical bundle is missing or malformed. Do not use `.g8e/g8e-gw-ca-bundle.pem` or `.g8e/pki/ca-bundle.pem`.
- **Never mutate local PKI state in tests**: If trust bundle issues persist, restart the gateway and re-authenticate manually.
- **Never use hand-trolled error strings**: Use typed constants instead of hardcoded strings for error reason strings, status codes, and rejection reasons.
- **Never use generic test names**: No `TestCoverage`, `TestEdgeCases`, `TestGap`, `TestMisc`, or any name that does not describe the specific behavior under test. The name must tell the reader what is being verified.
- **Never use generic test filenames**: No `edge_test.go`, `misc_test.go`, `coverage_test.go`, or any filename that does not describe its scope. Do not use "coverage", "gap", or "edge" in test filenames.

---

## cwd-Based `os.Chdir` Classification

Tests must not use `os.Chdir` to align `.g8e/` runtime state. Instead, inject `fileSvcFactoryFor(fileSvc)` into `*WithConfig` functions using a temp-rooted `fileSvc` from `newCmdTestEnv(t)`.

### Legitimate `os.Chdir` (Retain with Justification)

- **Source-tree discovery**: Demo commands resolve `./demos/`, `compose.yml`, `doctrine/`, `target-data/` relative to cwd. These are not `.g8e/` runtime paths. Files: `demos_*test.go`.
- **Config-layer tests**: `config.Load("")` reads from cwd by design. The config package is the layer that translates cwd into `fileSvc` baseDir. Files: `config/config_test.go`.
- **Chaos tests**: `runChaos` calls `configLoad` which reads from cwd. Injecting `fileSvcFactory` would require config-layer injection. Files: `chaos_integration_test.go`.

Each retained `os.Chdir` file must have a file-level comment explaining why cwd usage is legitimate.

### Illegitimate `os.Chdir` (Eliminate)

Any test that aligns `.g8e/` runtime state for command tests must use `newCmdTestEnv(t)` + `fileSvcFactoryFor(fileSvc)` injection. This pattern returns a pre-aligned `(fileSvc, cfg)` pair where `cfg.RuntimeDir == fileSvc.Resolve("")`.

---

## Factory-Error Test Requirement

Every `fileSvcFactory` injection point in `internal/cli/cmd/` must have a corresponding `TestXxxCmdWithConfig_FileSvcFactoryError` test in `internal/cli/cmd/factory_error_test.go`. The test asserts that:

1. The error is wrapped with `constants.ErrFileServiceInit`
2. The underlying factory error is preserved via `errors.Is`
3. Downstream dependencies are not called

Each test obtains a temp-rooted config from `newCmdTestEnv(t)`, constructs the command under test via its `*WithConfig` constructor wired with `configLoaderFor(cfg)` and `failingFileSvcFactory(errFactory)`, then calls `cmd.RunE` and asserts `errors.Is` against both `constants.ErrFileServiceInit` and the original factory error. See `internal/cli/cmd/factory_error_test.go` for the canonical pattern.

Helpers: `failingFileSvcFactory(err)` returns a factory that always errors. `panickingClientFactory()` returns a client factory that panics if called (proves downstream is not reached).

---

## CLI Command Test Helpers

- **`newCmdTestEnv(t)`**: Returns `(fs.RuntimeFileService, *config.Config)` with a temp-rooted `fileSvc` and aligned `cfg`. Uses `setupTestConfig` internally, which calls `fileSvc.CreateRuntimeTree`, `config.Load`, and writes a dummy trust bundle via `fileSvc.WriteFile`.
- **`fileSvcFactoryFor(fileSvc)`**: Returns a `fileSvcFactory` closure that always returns the given `fileSvc`. Used to inject a hermetic `fileSvc` into `*WithConfig` functions.
- **`configLoaderFor(cfg)`**: Returns a config loader closure that always returns the given `cfg`.
- **`mustRel(t, fileSvc, absPath)`**: Converts an absolute `.g8e/` path to a relative path, failing the test on error.
- **`newAuthTestEnv(t)`**: Returns `(fileSvc, cfg)` with auth-specific fixture setup (temp-rooted `fileSvc` with runtime tree created, minimal config with `ProjectRoot`/`RuntimeDir`/`Paths.Host` set).

---

## CLI Enrollment Coordinator Tests

The `EnrollmentCoordinator` (`internal/cli/auth/enrollment.go`) owns the CLI enrollment state machine. It is unit-tested in `internal/cli/auth/enrollment_coordinator_test.go` with injected mocks for `EnrollmentGateway`, `SystemTrustInstaller`, `BrowserOpener`, and `PasskeyRegistrar`. The command adapter layer (`internal/cli/cmd`) is tested in `auth_enroll_test.go` via the `enrollerFactory` parameter injected through the `*WithConfig` command constructors; tests pass `mockEnrollerFactory(mock)` to inject a `mockEnroller` that returns canned `EnrollmentResult`s without network I/O.

### Coordinator-Level Tests (`internal/cli/auth/enrollment_coordinator_test.go`)

Full state machine coverage:
- Healthy reuse (no rotation) — `LocalStateComplete` + `RotateCLI=false` → no gateway calls
- Partial → recovery — `LocalStatePartial` → `CreateRecoveryRequest` called, `Bootstrap` not called
- Expired → rotation — expiring CLI cert → `Rotate` called
- Absent → bootstrap — `LocalStateAbsent` → `Bootstrap` called
- `--no-system-trust` skip — `SystemTrustInstaller.Install` not called, `PasskeyRegistrar.Register` still called
- System trust failure stops before browser — `SystemTrustInstaller` returns error → `BrowserOpener`/`PasskeyRegistrar` not called
- `--rotate-cli` forces rotation — healthy identity + `RotateCLI=true` → `Rotate` called

### CredentialStore Tests (`internal/cli/auth/credential_store_test.go`)

5 tests exercising `CredentialStore` directly:
- `TestCredentialStore_InterruptedCommitRetry` — cancelled-context Commit fails, second Stage+Commit succeeds, no orphaned tmp files
- `TestCredentialStore_RollbackWritesNoCanonicalFiles` — Rollback writes no canonical files; `Rollback(nil)` is a safe no-op
- `TestCredentialStore_CommittedFilePermissions` — CLI cert, CLI key, credentials JSON all have 0600 mode after Commit
- `TestCredentialStore_ConcurrentStageCommitNoTornState` — two concurrent Stage+Commit sequences leave a complete, consistent identity (race-clean under `-race`)
- `TestCredentialStore_ClearRetainsTrustBundle` — Clear removes local credentials but retains the runtime trust bundle (§4.3 ownership)

### Command-Layer Tests (`internal/cli/cmd/auth_enroll_test.go`)

Tests inject a `mockEnroller` via `mockEnrollerFactory(mock)` + `noopCheckOperatorRunning` stub:
- `TestEnrollCmd_OptionPropagation` — defaults, `--no-system-trust`, `--rotate-cli`, both flags
- `TestEnrollCmd_CoordinatorErrorPropagates` — command surfaces `ErrSystemTrustInstallFailed`
- `TestEnrollCmd_HealthyReusedIdentityNoRotate` — Reused=true, RotateCLI=false
- `TestEnrollCmd_RotateCLIFlagForcesRotation` — `--rotate-cli` wiring
- `TestEnrollCmd_NoSystemTrustFlagWired` — `--no-system-trust` wiring
- `TestEnrollCmd_SystemTrustInstalledOutput` — browser-close guidance printed
- `TestLogoutCmd_OSRootCARetained` — OS root CA retained on logout
- `TestMCPStdio_DoesNotInvokeEnrollment` — stdio never calls the coordinator factory

### Gateway-Side Recovery/Rotation Tests (`internal/services/gateway/`)

- `cli_recovery_controller_test.go` — recovery request, status, approve, complete (proof-of-possession, token expiry, replay)
- `cli_recovery_service_test.go` — token hashing, atomic state transitions, cleanup
- `cli_rotation_controller_test.go` — mTLS rotation, session replacement, cert revocation
- `cli_session_service_test.go` — CLI session creation, replacement, deactivation, lookup by mTLS certificate
- `dispatch_service_test.go` / `dispatch_service_integration_test.go` — `DispatchController` request validation, governance pipeline routing, dispatch response shape; integration variant uses a real gateway fixture
- `operator_controller_test.go` — operator list, bind/unbind, target context, reauth, session lookup (`GET /api/v1/operators/session/{id}`)
- `gateway_http_test.go` / `gateway_auth_test.go` — route removal assertions (`TestRemovedCLIEnrollRoute`, `TestRouteAuthRegistry_RotationAndRemovedEnroll`)

### Governance Tests (`internal/services/governance/`)

- `remote_state_root_provider_test.go` — `RemoteStateRootProvider` fetches the gateway state Merkle root from `/api/v1/state` over mTLS; covers success, HTTP error, malformed response, and network failure paths

### E2E Tests (`test/e2e/`)

- `command_roundtrip_e2e_test.go` — end-to-end operator command dispatch via `POST /api/v1/operators/commands`: CLI enrolls, submits a command, operator receives and executes it, result is recorded
- `operator_registry_e2e_test.go` — operator registration, listing, and session lookup over the Docker Compose stack
- `pubsub_heartbeat_e2e_test.go` — operator heartbeat liveness via the pub/sub channel; verifies the gateway detects operator presence and absence

---

## Cross-Protocol Gateway Tests

The MCP and A2A gateway protocols share the same suspension, approval, and error-handling mechanics but differ in wire shape (endpoint path, JSON-RPC method, params field names, response structure). Cross-protocol tests use a shared adapter pattern to avoid duplicating test logic.

### `protocolAdapter` Interface

Defined in `test/protocol_test_helpers_test.go` (build tag: `integration`). The interface abstracts the wire-level differences between protocols:

- **`name()`**: Short identifier for subtest names (`"mcp"`, `"a2a"`)
- **`endpoint()`**: Canonical API path from `constants.APIPaths` (no path literals)
- **`callMethod()`**: JSON-RPC method (`"tools/call"` for MCP, `"a2a/call"` for A2A)
- **`nameParamKey()`** / **`payloadParamKey()`**: JSON params field names (`"name"`/`"arguments"` for MCP, `"skill_name"`/`"payload"` for A2A)
- **`makeCallBody(name, payload)`**: Builds a JSON-RPC request body. `payload` is `any` because payload-variation tests deliberately exercise arbitrary shapes (nested, unicode, large, empty, null) — this is the documented exception to the "no `Any` types for known shapes" rule in devs.md
- **`parseSuspendedStatus(t, body)`**: Extracts the suspended-status signal from the response body for assertion against typed constants

Implementations: `mcpAdapter` and `a2aAdapter`. The `bothAdapters()` helper returns `[]protocolAdapter{mcpAdapter{}, a2aAdapter{}}`.

### Shared Test Tables

Two table-driven tests iterate over `bothAdapters()` × a shared case table:

- **`test/protocol_payload_test.go`** → `TestGatewayProtocols_PayloadVariationsSuspendExecution` + `payloadCases` (5 cases: nested, unicode, large 100KB, empty, null). Asserts `constants.MCPApprovalPausedPrefix` for MCP, `constants.GatewayResponseStatusSuspended` for A2A.
- **`test/protocol_errors_test.go`** → `TestGatewayProtocols_MalformedRequestsReturnJSONRPCErrors` + `errorCases` (7 cases: invalid version, missing method, unknown method, malformed JSON, missing name, invalid payload, missing params). Malformed-JSON case asserts `constants.JSONRPCErrorCodeParseError` + `constants.JSONRPCErrorMessageParseError`.

Subtest names follow `protocol/scenario` format (e.g. `mcp/unicode_and_special_characters`).

### Rules

- **Extend the shared tables, not per-protocol test functions**: New payload shapes or error scenarios are added to `payloadCases` or `errorCases` and automatically covered for both protocols. Do not add new per-protocol payload or error test functions to `mcp_gateway_test.go` or `a2a_gateway_test.go`.
- **Use typed constants for assertions**: Assert against `constants.GatewayResponseStatusSuspended`, `constants.MCPApprovalPausedPrefix`, `constants.JSONRPCErrorCodeParseError`, and `constants.JSONRPCErrorMessageParseError` — not hardcoded strings.
- **New protocols**: Add a new `protocolAdapter` implementation and register it in `bothAdapters()`. The shared tables cover it automatically.

### Fixture Helpers

The `adapterFixture` struct in `test/protocol_test_helpers_test.go` bundles a `GatewayFixture` with an enrolled identity and mTLS client. Helpers:

- **`newAdapterFixture(t, testName, downstreamURL)`**: Creates a gateway fixture configured for both MCP and A2A downstream servers, enrolls a client identity, and returns an `adapterFixture`.
- **`postAdapter(t, adapter, body)`**: Sends a raw request body to the adapter's endpoint. Used by error-case tests that send malformed bodies.
- **`postAdapterWithStatus(t, adapter, body)`**: Same as `postAdapter` but also returns the HTTP status code.

---

## Reference

### Test Architecture (3-Tier Model)

| Tier | Name | Target Directory | Build Tag | External Deps | Execution Time |
| --- | --- | --- | --- | --- | --- |
| **Tier 1** | **Unit Tests** | `internal/...` & `pkg/...` | *No tags* | None (stub-only, no files/network/DB) | < 10ms per test |
| **Tier 2** | **In-Process Integration** | `internal/...` & `test/` | `//go:build integration` | On-disk SQLite, local PKI, local pubsub (gateway in-process) | < 2s per suite |
| **Tier 3** | **Docker E2E** | `test/e2e/` | `//go:build e2e` | Docker containers (gateway + operator via docker-compose) | < 30s per suite |

### CLI Test Commands

```bash
./g8e test unit        # Tier 1 - no external dependencies
./g8e test integration # Tier 2 - on-disk SQLite, local PKI
./g8e test e2e         # Tier 3 - requires running gateway
./g8e test coverage    # Coverage report (75% threshold enforced)
./g8e test lint        # golangci-lint + quality checks
./g8e demos scenarios list    # List demo scenarios
./g8e demos scenarios run     # Run scenarios against real Gateway/Operator
./g8e test chaos       # Generate governance events (70% Good, 20% Injection, 10% MitM)
./g8e test summary     # View chaos test summary from test vault
```

### Makefile Targets

```bash
make test              # Tier 1 + Tier 2
make test-unit         # Tier 1 only
make test-integration  # Tier 2 only (integration build tag)
make test-docker       # Tier 3 (e2e build tag, requires Docker)
make test-coverage     # Coverage with -coverprofile and -covermode=atomic
make test-airgap       # Verify vendored build works without network access
make ci                # Full CI pipeline (proto, swagger, lint, vulncheck, tests)
make lint              # golangci-lint + lint-no-embedded-newlines + vulncheck + validate-doctrines + swagger-generate
```

### Workflow

```bash
# 1. Unit tests (no gateway required)
./g8e test unit

# 2. In-process integration tests (no separately running gateway required)
./g8e test integration

# 3. Docker E2E tests (requires Docker)
make test-docker

# 4. Authenticate (required for non-demo mTLS tests; demo runs enroll inline)
./g8e auth enroll
```

**First-time setup**: if no users exist, the first login bootstraps the platform:

```bash
./g8e gw start
./g8e auth enroll
```

### Demo Scenarios

The demo scenarios tool (`g8e demos scenarios run`) impersonates arbitrary AI tools against a **REAL** g8e Gateway and Operator. The only fiction is the client identity; the Gateway and Operator are real infrastructure.

**28 scenarios total**: 7 MCP + 3 A2A + 6 governance + 6 DHS + 1 finance + 5 FedRAMP.

The interactive demo runner (`g8e demos run <org> [scenario]`) provides 16 platform demos across 5 environments: healthcare (4), finance (1), dhs (5), fedramp (5), frontend (1). These drive the real Gateway and Operator with posture switching. For notary demos (dhs, fedramp), `demos run` enrolls a host CLI session and registers a WebAuthn passkey inline before running scenarios. A browser window opens automatically for the passkey ceremony, with no separate terminal or manual `auth enroll` step. The enrolled `user_id` and `cli_session_id` are threaded into the harness so the suspended transaction and the browser approver share the same user identity.

**Testing postures**:
- **Doctrine**: L1 enforced, L2/L3 audited
- **Consensus**: L1/L2 enforced, L3 audited
- **Notary**: L1/L2/L3 strictly enforced

**Governance testing** uses cryptographic actors (per-member Ed25519 signers for L2 consensus, human browser approval for L3 notary) to exercise governance flows via `MCPToolsCall` (Path A). The gateway runs `L2ConsensusDeliberator` internally and suspends transactions requiring L3 notary approval. The harness drives the real out-of-band L3 flow via `WaitForHumanApproval` (`client/client.go`), which subscribes to the gateway's SSE stream for `approval.completed` events matching the transaction hash, blocks until a human completes the WebAuthn passkey ceremony in their browser, and verifies the approval status via the mTLS status endpoint.

Each consensus member signs with its own distinct key derived from `member_seeds` in the bootstrap config, making `RequireDistinct` and quorum cryptographically meaningful. `SubmitMaximal`/`Ensemble` remain as conformance testing infrastructure only. Unit tests for `WaitForHumanApproval` (3 tests in `client_test.go`: success, SSE timeout, status endpoint error), `shellCommandMap` (`shell_command_test.go`), and `WaitForApprovalSSE` (`approval_sse_test.go`) provide coverage. A fail-closed regression test (`TestGatewayModeService_GetGovernanceDeps_AlwaysUsesRealNotary`) verifies the gateway always requires real WebAuthn proof.

### MCP mTLS Authentication Flow

1. `GatewayFixture` starts a fully configured gateway in-process
2. `EnrollClientIdentity` performs CSR enrollment, generating certificates and operator session
3. `CreateMTLSClient` creates an HTTP client with enrolled identity certificates
4. All post-enrollment MCP calls target HTTPS port (8443) with mTLS enforced
5. Gateway's `auth.Middleware` extracts `OperatorSessionID` from SPIFFE URI SAN (`spiffe://g8e.local/operator/<org_id>/<operator_id>/<session_id>`)
6. Session ID validated against database

**Key details**:
- MCP routes are on HTTPS port (8443) only; HTTP port (8080) serves bootstrap endpoints (`/bootstrap`, `/.well-known/g8e/pki/*`), CLI recovery discovery (`/api/v1/auth/cli/recovery/{request,status,complete}`), deploy scripts, node binary download, and health checks. The old `handleCLIEnrollment` route (`/api/v1/auth/cli/enroll`) and per-platform trust-install script routes (`/web-cert.sh`, `/web-cert.ps1`, `/.well-known/g8e/pki/trust-windows`) were removed in v1.7.2; CLI enrollment is now driven client-side by the `EnrollmentCoordinator`.
- `ExtractOperatorSessionID` in `protocol/workload_identity.go` parses the SPIFFE URI (path segment 6)
- Tests include wait logic for operator session persistence before authenticated calls

### Fixture Lifecycle

`GatewayFixture` writes each run's data/vault/PKI to a fresh, uniquely-named directory under `<repo>/test-results/` (via `os.MkdirTemp`). This directory is **not** deleted; results accumulate for inspection. `test-results/` is gitignored. `NewGatewayFixture` registers cleanup internally via `t.Cleanup`, which stops the gateway and releases database locks but leaves data on disk.

Key fixture methods: `NewGatewayFixture`, `EnrollClientIdentity`, `CreateMTLSClient`, `CreateCLIMTLSClient`, `CreateNoCertClient`, `SetupConsensus`, `WaitForReady`. `PublicBaseURL` is set via `GatewayFixtureOptions` at construction time.

### Docker E2E Fixture (Tier 3)

`TestMain` in `test/e2e/main_test.go` spins up a single Docker Compose stack (gateway + operator) once for all E2E tests, then tears it down after `m.Run()`. The shared fixture is stored in the package-level `sharedFixture` variable. Tests that require Docker check for nil and skip if unavailable; tests that do not require Docker (e.g. MCP config output) run regardless. E2E tests are Tier 3 and require Docker — there is no opt-out. A fixture-setup failure exits non-zero with a `FATAL: E2E fixture setup failed` message so a broken Docker environment can never produce a green build with zero tests run. On any non-zero exit, container logs and compose state are captured to a temp dir before teardown.

The Dockerfile uses a BuildKit cache mount (`--mount=type=cache,target=/root/.cache/go-build`) to preserve the Go build cache across Docker image rebuilds. The harness sets `DOCKER_BUILDKIT=1` to enable this. First run after code changes rebuilds from scratch (~100s); subsequent runs with warm cache complete in ~25s.

### Trust Bundle Troubleshooting

If integration tests fail with `x509: certificate signed by unknown authority`:

```bash
# 1. Verify bundle exists
ls -la .g8e/pki/trust/g8eg-ca-bundle.pem

# 2. Regenerate if missing/corrupted
./g8e gw stop
rm -rf .g8e/pki
./g8e gw start
./g8e auth enroll

# 3. Verify bundle parses
openssl crl2pkcs7 -nocrl -certfile .g8e/pki/trust/g8eg-ca-bundle.pem
```

### Infrastructure Ports

From `protocol/constants/ports.json`:

- `8080`: Gateway HTTP (bootstrap, CA bundle, console, health)
- `8443`: Gateway HTTPS (mTLS API, MCP, public)

### Python Test Suite

The Python protocol package (`protocol/python/`) includes a pytest suite in `protocol/python/tests/` with 151 tests across 4 files:

- `test_constants.py`: Constant dict loading, value integrity, and namespace conventions
- `test_enums.py`: Enum generation, name conversion helpers, and dynamic attribute access
- `test_models.py`: Model instantiation, validation rules, serialization round-trips, and `G8eBaseModel` behavior
- `test_version.py`: Version string consistency with `pyproject.toml` and semver format

Run locally:
```bash
cd protocol/python
pip install -e ".[dev]"
python -m pytest tests/ -v
```

CI runs pytest on a Python 3.10-3.14 matrix (`python-tests` job).

### Protocol Conformance Suite

The conformance suite in `protocol/conformance/` contains 420 tests across 3 files that enforce parity between Go constants, Python runtime values, and canonical JSON in `protocol/constants/`:

- `test_constants.py`: JSON file structure, `_go_const`/`_python_const` presence, value uniqueness, Go naming conventions, Python-JSON parity, event value namespace conventions
- `test_models.py`: Model schema integrity, field parity between Python Pydantic models and JSON schemas, serialization round-trips, validation rules
- `test_hash_parity.py`: Cross-language transaction hash parity using shared test vectors (`hash_vectors.json`), verifying Python `compute_transaction_hash` matches Go `GenerateMessageID` for standard, nested intent, unicode, empty payload, empty intent, optional omitted, and timestamp normalization cases

Run locally:
```bash
pip install -e protocol/python/".[dev]"
python -m pytest protocol/conformance/ -v
```

CI runs conformance tests on Python 3.14 (`conformance` job).

### Performance Benchmarks

Go benchmarks cover hot paths in 4 packages (19 benchmarks total):

- `internal/services/sqliteutil/`: gzip compress/decompress at various payload sizes, SHA-256 hashing, compress+decompress round-trip (7 benchmarks)
- `internal/constants/`: `ActionType.IsMutation` for mutation and non-mutation types (2 benchmarks)
- `internal/services/mcp/`: JSON-RPC request/response marshal/unmarshal, tool call params parsing, tool result serialization (4 benchmarks)
- `internal/services/gateway/`: auth cache get/set/invalidate/expiry, state root calculation with small and large datasets (6 benchmarks)

Run benchmarks:
```bash
go test -bench=. -benchmem ./internal/services/sqliteutil/ ./internal/constants/ ./internal/services/mcp/ ./internal/services/gateway/
```

### Smoke Tests

Two smoke test scripts verify that published packages work in clean environments:

- `scripts/smoke-test-python.sh`: Creates a clean venv, installs the Python package, verifies README imports, and runs example scripts
- `scripts/smoke-test-go.sh`: Creates a temp Go module, uses `go mod edit -replace` to point at the local repo, imports `protocol.NewWorkloadIdentity()`, and builds

CI runs both scripts on every PR (`smoke-test` job).

### Continuous Integration

GitHub Actions (`.github/workflows/build-and-test.yml`) enforces:

**Core CI** (`ci` job, runs on `ubuntu-latest`):
- Version sync verification (`VERSION` file vs `protocol/python/pyproject.toml` vs `protocol/python/g8e/__init__.py`)
- Proto verification and doctrine validation
- Swagger generation and validation
- golangci-lint
- govulncheck
- Unit tests and integration tests
- Windows cross-compile (Linux runner only)

**Additional CI jobs**:
- `python-tests`: Pytest on Python 3.10-3.14 matrix with version sync verification
- `python-audit`: pip-audit `--skip-editable` for Python dependency vulnerability scanning
- `conformance`: Protocol conformance suite (420 tests) on Python 3.14
- `smoke-test`: Clean-environment install verification for both Python and Go packages
- `secret-scan`: gitleaks full-history secret scanning
- `license-check`: go-licenses report with forbidden copyleft license detection (GPL, AGPL, LGPL, SSPL, BUSL)

**Local-only targets** (not run in CI):
- `demo-verify`: Builds and runs all 5 demo environments via Docker Compose

CI does **not** run Tier 3 Docker E2E tests.

### Release Pipeline Verification

**Binary releases** (`.github/workflows/release-binary.yml`, triggered by `v*` tags):
- Cross-platform binary builds (linux/amd64/arm64/386, darwin/amd64/arm64, windows/amd64/arm64)
- SHA-256 checksums
- cosign/sigstore keyless artifact signing (`.sig` files uploaded with release)
- Post-publish `verify-install` job: fresh `go install` on ubuntu/macos/windows with `--version` and `--help` verification

**Python releases** (`.github/workflows/release-python-protocol.yml`, triggered by `protocol/v*` tags):
- Package metadata validation (required fields, name, URLs)
- Copy protocol constants and doctrine files into package (`protocol/constants/*.json` and `protocol/constants/doctrine/` into `g8e/_data/`)
- `twine check` on built dist
- Package includes `py.typed` PEP 561 marker for type checker support
- Post-publish `verify-install` job: polls the PyPI JSON API (`https://pypi.org/pypi/g8e/json`) until the version appears in `releases`, then fresh `pip install --no-cache-dir g8e==<version>` on ubuntu/macos/windows with import verification. Polling the API (rather than pip's CDN-cached index) avoids spurious "No matching distribution" failures when a CDN edge has not yet propagated the new version.
