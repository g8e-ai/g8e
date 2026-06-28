---
title: Tests
---

# Testing g8e

Last Updated: 2026-06-28

g8e tests run directly on the host using real infrastructure. If it does not work in tests, it will not work in production.

---

## Always

- **Use real infrastructure** — Tests use actual SQLite, PKI certificates, and pub/sub channels. Platform starts via `./g8e gw start`.
- **Reproduce bugs first** — Write a failing test that reproduces the bug before fixing it.
- **Use contract tests** — Enforce alignment between the Operator and `protocol/` constants/models with typed protobuf assertions.
- **Use `t.Cleanup(f.Cleanup)` for fixture teardown** — Register cleanup at the test scope, exactly once. If a setup helper creates the fixture, it must register teardown with `t.Cleanup`, not `defer`.
- **Hold temp credential files for the whole test** — Register their removal with `t.Cleanup`, not `defer`, in any setup helper.
- **Use path constants** — ALL filepath strings in test code must be defined as constants in `internal/constants/paths.go`. Use `constants.Paths.Infra.*` for runtime state paths. The only exception is `TestPaths` for isolated test environments, where the base directory must still come from a constant.
- **Use typed error constants** — Check for typed constants from `internal/constants/` instead of hardcoded strings in assertions and error message checks.
- **Use regression test markers** — Use standardized marker constants (`RegressionMarkerAfterFix`, `RegressionMarkerBeforeFix`, `RegressionMarkerIssue`) instead of hardcoded strings. See `internal/services/gateway/pki_authority_test.go` for examples.
- **Enable race detection** — `-race` on all non-Windows platforms across all test targets.
- **Use explicit cancellation contexts** — Goroutines require explicit cancellation contexts and clear channel ownership.
- **Use the canonical trust bundle path** — `.g8e/pki/trust/g8eg-ca-bundle.pem`. Contains root CA, hub intermediate, Operator intermediate, and gateway peer CA.
- **Use the shared tribunal factory** — `tribunal.NewTribunalFromPolicy` is used by both production `BootstrapTribunal` and test `SetupTribunal` to avoid duplication.
- **Keep `storagetest.TestSQLAuditStore` in test code only** — Production code uses `storage.SQLAuditStore` from `audit_store.go`.

---

## Never

- **Never mock internal services, database clients, or cross-component communication** — Integration tests use real wire paths. Tier 1 unit tests may use stubs/mocks for external dependencies only.
- **Never use `t.Parallel()` in integration tests** — Each test gets its own isolated data directory and random port. Sequential execution avoids resource contention.
- **Never `defer f.Cleanup()` inside a setup helper** — A deferred cleanup fires when the helper returns, tearing the gateway down before the test body runs. Symptom: `sql: database is closed` on the first gateway call.
- **Never clean up twice** — If a setup helper already registered `t.Cleanup(f.Cleanup)`, the caller must not also `defer f.Cleanup()`. A second call blocks forever on the already-drained error channel and the test hangs until timeout.
- **Never hardcode filepath strings** — No dynamic path construction with `filepath.Join()` and string literals. No relative paths like `"../../"`, `"./"`, `".g8e/"`, `"/pki/"` inline.
- **Never use legacy trust bundle paths** — Tests fail closed if the canonical bundle is missing or malformed. Do not use `.g8e/g8e-gw-ca-bundle.pem` or `.g8e/pki/ca-bundle.pem`.
- **Never mutate local PKI state in tests** — If trust bundle issues persist, restart the gateway and re-authenticate manually.
- **Never use hand-trolled error strings** — Use typed constants instead of hardcoded strings for error reason strings, status codes, and rejection reasons.

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
./g8e test coverage    # Coverage report (70% threshold enforced)
./g8e test lint        # golangci-lint + quality checks
./g8e agent-harness list    # List agent harness scenarios
./g8e agent-harness run     # Run scenarios against real Gateway/Operator
./g8e agent-harness audit   # Audit signed receipts from Operator
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

### Agent Harness

The agent harness impersonates arbitrary AI tools against a **REAL** g8e Gateway and Operator. The only fiction is the client identity — the Gateway and Operator are real infrastructure.

**16 scenarios total**: 5 MCP + 3 A2A + 6 governance + 2 DoW.

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
- MCP routes are on HTTPS port (8443) only; HTTP port (8080) is bootstrap-only (`/bootstrap`, `/enroll`, `/.well-known/g8e/pki/*`)
- `ExtractOperatorSessionID` in `protocol/workload_identity.go` parses the SPIFFE URI (path segment 6)
- Tests include wait logic for operator session persistence before authenticated calls

### Fixture Lifecycle

`GatewayFixture` writes each run's data/vault/PKI to a fresh, uniquely-named directory under `<repo>/test-results/` (via `os.MkdirTemp`). This directory is **not** deleted — results accumulate for inspection. `test-results/` is gitignored. `Cleanup` stops the gateway and releases database locks but leaves data on disk.

Key fixture methods: `NewGatewayFixture`, `EnrollClientIdentity`, `CreateMTLSClient`, `CreateCLIMTLSClient`, `SetupTribunal`, `WaitForReady`, `SetPublicBaseURL`, `Cleanup`.

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

- `8080` — Gateway HTTP (bootstrap, CA bundle, health)
- `8443` — Gateway HTTPS (mTLS API, MCP, public)

### Continuous Integration

GitHub Actions (`.github/workflows/build-and-test.yml`) enforces: proto verification, swagger generation/validation, golangci-lint, govulncheck, unit tests, integration tests, and Windows cross-compile. CI does **not** run Tier 3 Docker E2E tests.
