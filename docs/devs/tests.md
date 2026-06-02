---
title: Tests
---

# Testing g8e

Last Updated: 2026-06-01

g8e tests run directly on the host using real infrastructure. The test environment is the production environment. If it does not work in tests, it will not work in production.

---

## Test Philosophy

- **Hermetic execution** - Tests run on the host via `./g8e test` or in Docker for end-to-end testing. The g8e Node is a unified g8e Node that operates as g8e Gateway (Policy Decision Point) in gateway mode or as g8e Operator (Policy Execution Point) in Operator mode.
- **Real infrastructure** - Tests use the actual SQLite database, PKI certificates, and pub/sub channels. Platform starts via `./g8e gw start`.
- **No mocks** - Mocking internal services, database clients, or cross-component communication is prohibited. Integration tests use real wire paths.
- **mTLS required** - Operator communication requires mTLS. Authentication via `./g8e auth login` issues certificates from `.g8e/pki`.
- **Reproduce first** - Reproduce bugs with failing tests before fixes.
- **Contract tests** - Enforce alignment between the Operator and `protocol/` constants/models with typed protobuf assertions.
- **Docker for E2E** - Docker is encouraged for end-to-end testing of the g8e Node in Operator mode against the local g8e Gateway to validate real deployment scenarios.

---

## Test Harness

### CLI Test Commands

```bash
./g8e test            # runs all Go tests (unit + integration)
./g8e test ci         # CI suite: proto, lint, vulncheck, platform tests with coverage
./g8e test unit       # unit tests only
./g8e test integration # integration tests only
./g8e test chaos      # Chaos engineering tests
./g8e test scenario   # Scenario integration tests
./g8e test review     # Review integration test vault results
./g8e test summary    # Show summary of all integration test results
```

The `./g8e test` command runs the full test suite including unit tests across `cmd/`, `internal/`, `pkg/`, and `test/` packages, followed by integration tests from `test/scenario/`. The `ci` subcommand mirrors the GitHub Actions CI pipeline exactly, running proto generation, linting, vulncheck, and platform tests with coverage enforcement.

Validates the g8e Node and protocol enforcement (`GovernanceEnvelope`, 5-layer governance, Audit Vault). Tests cover pub/sub command dispatch, L1/L2/L3/L4/L5 verification, transaction replay protection, state root validation, and audit vault integrity.

### Scenario Tests

```bash
./g8e test scenario
./g8e test scenario --run forge_signature
```

Integration tests exercising end-to-end governance workflows across doctrine, consensus, and notary modes. Tests cover the 5-layer verification sequence (L1-L5), transaction replay protection, state root validation, and receipt verification. Requires the g8e Gateway to be running.

**Test Types**:
- **Table-driven scenarios** - JSON fixtures in `test/scenario/fixtures/` covering security gates (bad integrity, hash mismatch, replay, stale state root, L2/L3 validation) and finance workflows
- **Golden snapshots** - Deterministic receipt comparison excluding volatile fields (signature, timestamp, signer key). Golden files auto-create on missing and auto-update on mismatch
- **Property-based invariants** - Fuzz-style tests verifying core governance invariants (integrity + freshness + state + required-gates must all pass in order)
- **Concurrency tests** - Double-submit replay detection using goroutines to verify TOCTOU resistance
- **Negative controls** - Tests that intentionally flip expectations to prove the suite can detect failures
- **Receipt verification** - Separate axis testing cryptographic receipt validation (signature verification, field tampering detection)
- **Receipt persistence** - Database persistence verification for accepted transactions (receipts stored in `console_audit` collection), rejected transactions verify no persistence

### Chaos Tests

```bash
./g8e test chaos --count 100
```

Chaos engineering tests firing random payloads at the Operator to verify fail-closed behavior and invariant enforcement.

---

## Workflow

```bash
# 1. Start the Gateway
./g8e gw start

# 2. Authenticate (required for mTLS tests)
./g8e auth login

# 3. Run tests
./g8e test
./g8e test unit
./g8e test scenario
```

### First-time Setup

If no users exist, the first login automatically bootstraps the platform:

```bash
./g8e gw start
./g8e auth login
```

This creates the first user and issues mTLS certificates for the g8e Gateway and CLI.

### Trust Bundle Requirements

Live Operator integration tests (MCP, A2A, Native) require the canonical trust bundle at `.g8e/pki/trust/g8eg-ca-bundle.pem`. This bundle contains the root CA, hub intermediate CA, Operator intermediate CA, and gateway peer CA certificates.

**Canonical trust bundle path**: `.g8e/pki/trust/g8eg-ca-bundle.pem`

**No legacy bundle paths are accepted**. Tests will fail closed if the canonical bundle is missing or malformed. Do not attempt to use legacy paths such as `.g8e/g8e-gw-ca-bundle.pem` or `.g8e/pki/ca-bundle.pem`.

### Troubleshooting Trust Bundle Issues

If integration tests fail with `x509: certificate signed by unknown authority` or `AppendCertsFromPEM` errors:

1. Verify the canonical trust bundle exists:
   ```bash
   ls -la .g8e/pki/trust/g8eg-ca-bundle.pem
   ```

2. If the bundle is missing or corrupted, regenerate local PKI by explicit developer action:
   ```bash
   # Stop the gateway
   ./g8e gw stop

   # Remove the existing PKI directory
   rm -rf .g8e/pki

   # Restart the gateway (this regenerates PKI)
   ./g8e gw start

   # Re-authenticate to obtain new certificates
   ./g8e auth login
   ```

3. Verify the bundle parses correctly:
   ```bash
   openssl crl2pkcs7 -nocrl -certfile .g8e/pki/trust/g8eg-ca-bundle.pem
   ```

Tests do not mutate local PKI state. If trust bundle issues persist, the gateway must be restarted and authentication re-performed.

---

## Test Implementation

### Go Tests

- **Tooling** - Standard `go test` with optional `gotestsum` for dots-style output.
- **Race detection** - Enabled via `-race` in CI and by default in `./g8e test unit` and `./g8e test ci`.
- **Parallelism** - `-parallel 4` with `180s` timeout.
- **Coverage** - `--coverage` flag generates reports. CI enforces 60% coverage threshold.
- **Concurrency** - Goroutines require explicit cancellation contexts and clear channel ownership.
- **Integration tags** - Scenario tests require `-tags=integration` to access test fixtures and Gateway gate infrastructure.
- **Path constants** - Tests must use `constants.Paths.Infra.*` constants for runtime state paths (e.g., `constants.Paths.Infra.PkiDir` for `.g8e/pki`). Hardcoded path strings like `.g8e/pki` are prohibited in test code.

### Makefile Test Targets

- **`make test`** - Runs all tests with race detection and 180s timeout.
- **`make test-short`** - Runs short tests with race detection and 60s timeout.
- **`make test-coverage`** - Runs tests with coverage and enforces 60% threshold. Use `PKG=./path/to/pkg` for specific packages, `VERBOSE=true` for verbose output.
- **`make test-shuffle`** - Runs all tests with randomized order.
- **`make test-integration`** - Runs integration tests (requires platform running and auth login).
- **`make test-scenario`** - Runs scenario integration tests (requires platform running).
- **`make test-gateway`** - Runs gateway tests (A2A gateway, MCP gateway, MCP stdio).
- **`make test-mcp`** - Runs MCP tests (MCP gateway, MCP real operator, MCP stdio).
- **`make test-a2a`** - Runs A2A tests (A2A gateway, A2A real operator).
- **`make test-universal-gateway`** - Runs universal gateway integration tests (requires platform running and auth login).
- **`make test-byo`** - Runs BYO client tests (requires platform running and auth login).
- **`make test-native`** - Runs native real Operator tests (requires platform running and auth login).

### Lints

- **`make lint`** - Runs all linting and quality checks including golangci-lint, vulncheck, and doctrine validation.
- **`make lint-no-embedded-newlines`** - Checks for compilation errors including embedded newlines.
- **`make vulncheck`** - Runs `govulncheck` on Go dependencies.
- **`make validate-doctrines`** - Validates doctrine JSON schema against the governance policy model.

---

## Infrastructure Ports

Defaults from `protocol/constants/ports.json` (canonical source of truth):

- `8440` - Gateway mTLS API and Pub/Sub
- `8441` - Gateway Bootstrap (plain HTTP; CSR signing)
- `8443` - Gateway Public TLS (browser/BYO bootstrap)
- `18789` - Insecure MCP Gateway

All defaults are unprivileged ports (>1024). To run on `443`/`80`, grant `CAP_NET_BIND_SERVICE` to the g8e Node or front with an external port redirect.

---

## Continuous Integration

GitHub Actions (`.github/workflows/build-and-test.yml`) enforces:

- **`verify-proto`** - Generated Go and Python code sync with `.proto` definitions.
- **`lint-g8eo`** - Runs `golangci-lint` on the g8e Node and protocol code.
- **`vulncheck-g8eo`** - Scans Go dependencies for known vulnerabilities.
- **`test-g8eo`** (blocking) - Installs Go, starts the gateway, runs `./g8e test ci` with 60% coverage threshold.
- **`test-scenarios`** - Runs scenario integration tests with `-tags=integration`.
- **`constants-lint`** - Enforces use of constants instead of raw string literals.
- **`docs-lint`** - Validates Markdown formatting with markdownlint.
- **`docs-build`** - Builds CLI reference and documentation site.
