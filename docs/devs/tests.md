---
title: Tests
---

# Testing g8e

Last Updated: 2026-07-24

g8e tests run directly on the host using real infrastructure. If it does not work in tests, it will not work in production.

---

## Always

- **Use real infrastructure** — Tests use actual SQLite, PKI certificates, and pub/sub channels. Platform starts via `./g8e gw start`.
- **Reproduce bugs first** — Write a failing test that reproduces the bug before fixing it.
- **Use contract tests** — Enforce alignment between the Operator and `protocol/` constants/models with typed protobuf assertions.
- **Let `NewGatewayFixture` handle cleanup** — `NewGatewayFixture` registers teardown internally via `t.Cleanup`. Callers do not need to defer or register any cleanup.
- **Hold temp credential files for the whole test** — Register their removal with `t.Cleanup`, not `defer`, in any setup helper.
- **Use path constants** — ALL filepath strings in test code must be defined as constants in `internal/constants/paths.go`. Use `constants.Paths.Infra.*` for runtime state paths. The only exception is `TestPaths` for isolated test environments, where the base directory must still come from a constant. Use `constants.PermFileReadOnly` and other `constants.Perm*` constants for file permission assertions in tests.
- **Use `fileSvc.FileExists` instead of `os.IsNotExist`** — Check file existence via `fileSvc.FileExists(ctx, relPath)` or `errors.Is(err, constants.ErrNotFound)` instead of `os.IsNotExist`. The `RuntimeFileService` wraps `os.*` calls and returns typed `constants.Err*` errors.
- **`testutil.TempDir` returns absolute paths** — `testutil.TempDir(t)` returns an absolute path to a temporary directory. Use it as the `baseDir` for `fs.NewRuntimeFileService` and `paths.InitWithBase`. Do not join it with `constants.RuntimeDirname` manually; the file service handles that.
- **Do not set `DataDir`/`CredentialsDir` in test configs** — Test configs must use `RuntimeDir` and `fileSvc.Resolve(constants.*)` instead of `DataDir`/`CredentialsDir` fields. These fields have been removed from config structs. Pass `fileSvc` to services that need file I/O.
- **Use typed error constants** — Check for typed constants from `internal/constants/` instead of hardcoded strings in assertions and error message checks.
- **Use regression test markers** — Use standardized marker constants (`RegressionMarkerAfterFix`, `RegressionMarkerBeforeFix`, `RegressionMarkerIssue`) instead of hardcoded strings. See `internal/services/gateway/pki_authority_test.go` for examples.
- **Enable race detection** — `-race` on all non-Windows platforms across all test targets.
- **Use explicit cancellation contexts** — Goroutines require explicit cancellation contexts and clear channel ownership.
- **Use the canonical trust bundle path** — `.g8e/pki/trust/g8eg-ca-bundle.pem`. Contains root CA, hub intermediate, Operator intermediate, and gateway peer CA.
- **Use the shared tribunal factory** — `tribunal.NewTribunalFromPolicy` is used by both production `BootstrapTribunal` and test `SetupTribunal` to avoid duplication.
- **Keep `storagetest.TestSQLAuditStore` in test code only** — Production code uses `storage.SQLAuditStore` from `audit_store.go`.
- **Use descriptive test names** — Test function names must describe the specific behavior being verified, not generic categories. Good: `TestHandleFsReadRequest_ScrubbingRedactsSecrets`, `TestHandleEvalAnswerRequestSync_TruncatesAnswerExceedingMaxBytes`. Bad: `TestCoverage`, `TestEdgeCases`, `TestGap`, `TestMisc`. Subtest names (`t.Run`) must describe the specific scenario, not just "success" or "error".
- **Use descriptive test filenames** — Test filenames must describe their scope, not generic categories. Good: `file_ops_scrubbing_test.go`, `vault_writer_error_paths_test.go`. Bad: `edge_test.go`, `misc_test.go`, `coverage_test.go`. Do not use "coverage", "gap", or "edge" in test filenames — name the file after the behavior or component it tests.

---

## Never

- **Never mock internal services, database clients, or cross-component communication** — Integration tests use real wire paths. Tier 1 unit tests may use stubs/mocks for external dependencies only.
- **Never use `t.Parallel()` in integration tests** — Each test gets its own isolated data directory and random port. Sequential execution avoids resource contention.
- **Never use `defer` for fixture teardown in a setup helper** — A deferred cleanup fires when the helper returns, tearing the gateway down before the test body runs. `NewGatewayFixture` handles cleanup internally via `t.Cleanup`. Symptom of incorrect cleanup: `sql: database is closed` on the first gateway call.
- **Never clean up twice** — `NewGatewayFixture` registers cleanup exactly once via `t.Cleanup`. Do not add additional cleanup calls that stop the gateway or close databases.
- **Never hardcode filepath strings** — No dynamic path construction with `filepath.Join()` and string literals. No relative paths like `"../../"`, `"./"`, `".g8e/"`, `"/pki/"` inline.
- **Never use legacy trust bundle paths** — Tests fail closed if the canonical bundle is missing or malformed. Do not use `.g8e/g8e-gw-ca-bundle.pem` or `.g8e/pki/ca-bundle.pem`.
- **Never mutate local PKI state in tests** — If trust bundle issues persist, restart the gateway and re-authenticate manually.
- **Never use hand-trolled error strings** — Use typed constants instead of hardcoded strings for error reason strings, status codes, and rejection reasons.
- **Never use generic test names** — No `TestCoverage`, `TestEdgeCases`, `TestGap`, `TestMisc`, or any name that does not describe the specific behavior under test. The name must tell the reader what is being verified.
- **Never use generic test filenames** — No `edge_test.go`, `misc_test.go`, `coverage_test.go`, or any filename that does not describe its scope. Do not use "coverage", "gap", or "edge" in test filenames.

---

## cwd-Based `os.Chdir` Classification

Tests must not use `os.Chdir` to align `.g8e/` runtime state. Instead, inject `fileSvcFactoryFor(fileSvc)` into `*WithConfig` functions using a temp-rooted `fileSvc` from `newCmdTestEnv(t)`.

### Legitimate `os.Chdir` (Retain with Justification)

- **Source-tree discovery** — Demo commands resolve `./demos/`, `compose.yml`, `doctrine/`, `target-data/` relative to cwd. These are not `.g8e/` runtime paths. Files: `demos_*test.go`.
- **Config-layer tests** — `config.Load("")` reads from cwd by design. The config package is the layer that translates cwd into `fileSvc` baseDir. Files: `config/config_test.go`.
- **Chaos tests** — `runChaos` calls `configLoad` which reads from cwd. Injecting `fileSvcFactory` would require config-layer injection. Files: `chaos_integration_test.go`.

Each retained `os.Chdir` file must have a file-level comment explaining why cwd usage is legitimate.

### Illegitimate `os.Chdir` (Eliminate)

Any test that aligns `.g8e/` runtime state for command tests must use `newCmdTestEnv(t)` + `fileSvcFactoryFor(fileSvc)` injection. This pattern returns a pre-aligned `(fileSvc, cfg)` pair where `cfg.RuntimeDir == fileSvc.Resolve("")`.

---

## Factory-Error Test Requirement

Every `fileSvcFactory` injection point in `internal/cli/cmd/` must have a corresponding `TestXxxCmdWithConfig_FileSvcFactoryError` test in `factory_error_test.go`. The test asserts that:

1. The error is wrapped with `constants.ErrFileServiceInit`
2. The underlying factory error is preserved via `errors.Is`
3. Downstream dependencies are not called

Pattern:

```go
func TestXxxCmdWithConfig_FileSvcFactoryError(t *testing.T) {
    _, cfg := newCmdTestEnv(t)
    cmd := xxxCmdWithConfig(configLoaderFor(cfg), /* other deps */, failingFileSvcFactory(factoryErr))
    err := cmd.RunE(cmd, []string{})
    require.Error(t, err)
    assert.ErrorIs(t, err, constants.ErrFileServiceInit)
    assert.ErrorIs(t, err, factoryErr)
}
```

Helpers: `failingFileSvcFactory(err)` returns a factory that always errors. `panickingClientFactory()` returns a client factory that panics if called (proves downstream is not reached).

---

## CLI Command Test Helpers

- **`newCmdTestEnv(t)`** — Returns `(fs.RuntimeFileService, *config.Config)` with a temp-rooted `fileSvc` and aligned `cfg`. Uses `setupTestConfig` internally, which calls `fileSvc.CreateRuntimeTree`, `config.LoadWithPaths`, and writes a dummy trust bundle via `fileSvc.WriteFile`.
- **`fileSvcFactoryFor(fileSvc)`** — Returns a `fileSvcFactory` closure that always returns the given `fileSvc`. Used to inject a hermetic `fileSvc` into `*WithConfig` functions.
- **`configLoaderFor(cfg)`** — Returns a config loader closure that always returns the given `cfg`.
- **`mustRel(t, fileSvc, absPath)`** — Converts an absolute `.g8e/` path to a relative path, failing the test on error.
- **`newAuthTestEnv(t)`** — Returns `(fileSvc, cfg)` with auth-specific fixture setup (temp-rooted `fileSvc` with runtime tree created, minimal config with `ProjectRoot`/`RuntimeDir`/`Paths.Host` set).

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
./g8e test unit        # Tier 1 — no external dependencies
./g8e test integration # Tier 2 — on-disk SQLite, local PKI
./g8e test e2e         # Tier 3 — requires running gateway
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

# 4. Authenticate (required for mTLS tests)
./g8e auth enroll
```

**First-time setup** — if no users exist, the first login bootstraps the platform:

```bash
./g8e gw start
./g8e auth enroll
```

### Demo Scenarios

The demo scenarios tool (`g8e demos scenarios run`) impersonates arbitrary AI tools against a **REAL** g8e Gateway and Operator. The only fiction is the client identity — the Gateway and Operator are real infrastructure.

**37 scenarios total**: 7 MCP + 3 A2A + 6 governance + 2 DoW + 6 DHS + 2 gov/finance + 3 secure-data + 3 swarm + 5 FedRAMP.

The interactive demo runner (`g8e demos run <org> [scenario]`) provides 26 platform demos across 9 environments: healthcare (4), gov (1), finance (1), secure-data (3), dow (3), dhs (5), swarm (3), fedramp (5), frontend (1). These drive the real Gateway and Operator with interactive passkey authentication and posture switching.

**Testing postures**:
- **Doctrine** — L1 enforced, L2/L3 audited
- **Consensus** — L1/L2 enforced, L3 audited
- **Notary** — L1/L2/L3 strictly enforced

**Governance testing** uses mock cryptographic actors (ensemble co-signers, principal notary) to test maximal governance envelopes without distributed consensus infrastructure.

### MCP mTLS Authentication Flow

1. `GatewayFixture` starts a fully configured gateway in-process
2. `EnrollClientIdentity` performs CSR enrollment, generating certificates and operator session
3. `CreateMTLSClient` creates an HTTP client with enrolled identity certificates
4. All post-enrollment MCP calls target HTTPS port (8443) with mTLS enforced
5. Gateway's `auth.Middleware` extracts `OperatorSessionID` from SPIFFE URI SAN (`spiffe://g8e.local/operator/<org_id>/<operator_id>/<session_id>`)
6. Session ID validated against database

**Key details**:
- MCP routes are on HTTPS port (8443) only; HTTP port (8080) serves bootstrap endpoints (`/bootstrap`, `/enroll`, `/.well-known/g8e/pki/*`), the console SPA, browser-facing passkey endpoints, and health checks
- `ExtractOperatorSessionID` in `protocol/workload_identity.go` parses the SPIFFE URI (path segment 6)
- Tests include wait logic for operator session persistence before authenticated calls

### Fixture Lifecycle

`GatewayFixture` writes each run's data/vault/PKI to a fresh, uniquely-named directory under `<repo>/test-results/` (via `os.MkdirTemp`). This directory is **not** deleted — results accumulate for inspection. `test-results/` is gitignored. `NewGatewayFixture` registers cleanup internally via `t.Cleanup`, which stops the gateway and releases database locks but leaves data on disk.

Key fixture methods: `NewGatewayFixture`, `EnrollClientIdentity`, `CreateMTLSClient`, `CreateCLIMTLSClient`, `CreateNoCertClient`, `SetupTribunal`, `WaitForReady`. `PublicBaseURL` is set via `GatewayFixtureOptions` at construction time.

### Docker E2E Fixture (Tier 3)

`TestMain` in `test/e2e/main_test.go` spins up a single Docker Compose stack (gateway + operator) once for all E2E tests, then tears it down after `m.Run()`. The shared fixture is stored in the package-level `sharedFixture` variable. Tests that require Docker check for nil and skip if unavailable; tests that do not require Docker (e.g. MCP config output) run regardless. Set `G8E_E2E_SKIP_DOCKER=1` to skip Docker setup entirely while still running non-Docker E2E tests.

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

- `8080` — Gateway HTTP (bootstrap, CA bundle, console, health)
- `8443` — Gateway HTTPS (mTLS API, MCP, public)

### Python Test Suite

The Python protocol package (`protocol/python/`) includes a pytest suite in `protocol/python/tests/` with 151 tests across 4 files:

- `test_constants.py` — Constant dict loading, value integrity, and namespace conventions
- `test_enums.py` — Enum generation, name conversion helpers, and dynamic attribute access
- `test_models.py` — Model instantiation, validation rules, serialization round-trips, and `G8eBaseModel` behavior
- `test_version.py` — Version string consistency with `pyproject.toml` and semver format

Run locally:
```bash
cd protocol/python
pip install -e ".[dev]"
python -m pytest tests/ -v
```

CI runs pytest on a Python 3.10-3.14 matrix (`python-tests` job).

### Protocol Conformance Suite

The conformance suite in `protocol/conformance/` contains 420 tests across 3 files that enforce parity between Go constants, Python runtime values, and canonical JSON in `protocol/constants/`:

- `test_constants.py` — JSON file structure, `_go_const`/`_python_const` presence, value uniqueness, Go naming conventions, Python-JSON parity, event value namespace conventions
- `test_models.py` — Model schema integrity, field parity between Python Pydantic models and JSON schemas, serialization round-trips, validation rules
- `test_hash_parity.py` — Cross-language transaction hash parity using shared test vectors (`hash_vectors.json`), verifying Python `compute_transaction_hash` matches Go `GenerateMessageID` for standard, nested intent, unicode, empty payload, empty intent, optional omitted, and timestamp normalization cases

Run locally:
```bash
pip install -e protocol/python/".[dev]"
python -m pytest protocol/conformance/ -v
```

CI runs conformance tests on Python 3.14 (`conformance` job).

### Performance Benchmarks

Go benchmarks cover hot paths in 4 packages (19 benchmarks total):

- `internal/services/sqliteutil/` — gzip compress/decompress at various payload sizes, SHA-256 hashing, compress+decompress round-trip (7 benchmarks)
- `internal/constants/` — `ActionType.IsMutation` for mutation and non-mutation types (2 benchmarks)
- `internal/services/mcp/` — JSON-RPC request/response marshal/unmarshal, tool call params parsing, tool result serialization (4 benchmarks)
- `internal/services/gateway/` — auth cache get/set/invalidate/expiry, state root calculation with small and large datasets (6 benchmarks)

Run benchmarks:
```bash
go test -bench=. -benchmem ./internal/services/sqliteutil/ ./internal/constants/ ./internal/services/mcp/ ./internal/services/gateway/
```

### Smoke Tests

Two smoke test scripts verify that published packages work in clean environments:

- `scripts/smoke-test-python.sh` — Creates a clean venv, installs the Python package, verifies README imports, and runs example scripts
- `scripts/smoke-test-go.sh` — Creates a temp Go module, uses `go mod edit -replace` to point at the local repo, imports `protocol.NewWorkloadIdentity()`, and builds

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
- `python-tests` — Pytest on Python 3.10-3.14 matrix with version sync verification
- `python-audit` — pip-audit `--skip-editable` for Python dependency vulnerability scanning
- `conformance` — Protocol conformance suite (420 tests) on Python 3.14
- `smoke-test` — Clean-environment install verification for both Python and Go packages
- `secret-scan` — gitleaks full-history secret scanning
- `license-check` — go-licenses report with forbidden copyleft license detection (GPL, AGPL, LGPL, SSPL, BUSL)

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
- Post-publish `verify-install` job: fresh `pip install g8e==<version>` from PyPI on ubuntu/macos/windows with import verification
