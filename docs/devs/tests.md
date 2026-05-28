---
title: Tests
---

# Testing g8e

Last Updated: 2026-05-25

g8e tests run directly on the host using real infrastructure. The test environment is the production environment. If it does not work in tests, it will not work in production.

---

## Test Philosophy

- **Hermetic execution** - Tests run on the host via `./g8e test`. The g8e Operator is a unified binary that operates as Governance Gateway (Policy Decision Point) in gateway mode or as g8e Operator (Policy Execution Point) in cloud mode.
- **Real infrastructure** - Tests use the actual SQLite database, PKI certificates, and pub/sub channels. Platform starts via `./g8e platform start`.
- **No mocks** - Mocking internal services, database clients, or cross-component communication is prohibited. Integration tests use real wire paths.
- **mTLS required** - Operator communication requires mTLS. Authentication via `./g8e auth login` issues certificates from `.g8e/pki`.
- **Reproduce first** - Reproduce bugs with failing tests before fixes.
- **Contract tests** - Enforce alignment between the Operator and `protocol/` constants/models with typed protobuf assertions.

---

## Test Harness

### Operator Tests

```bash
./g8e test            # runs all Go tests
./g8e test g8eo       # explicit Operator test target
./g8e test ci         # CI suite: Operator + scenario integration
./g8e test chaos      # Chaos engineering tests
./g8e test scenario   # Scenario integration tests
```

Validates the Operator and protocol enforcement (`GovernanceEnvelope`, 5-layer governance, Audit Vault). Tests cover pub/sub command dispatch, L1/L2/L3/L4/L5 verification, transaction replay protection, state root validation, and audit vault integrity.

### Scenario Tests

```bash
./g8e test scenario
./g8e test scenario --run forge_signature
```

Integration tests exercising end-to-end governance workflows across doctrine, consensus, and notary modes. Tests cover the 5-layer verification sequence (L1-L5), transaction replay protection, state root validation, and receipt verification. Requires the Operator to be running.

**Test Types**:
- **Table-driven scenarios** - JSON fixtures in `test/scenario/fixtures/` covering security gates (bad integrity, hash mismatch, replay, stale state root, L2/L3 validation) and finance workflows
- **Golden snapshots** - Deterministic receipt comparison excluding volatile fields (signature, timestamp, signer key). Run with `G8E_UPDATE_GOLDEN=1` to regenerate
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
# 1. Start the Operator
./g8e platform start

# 2. Authenticate (required for mTLS tests)
./g8e auth login

# 3. Run Operator tests
./g8e test
./g8e test g8eo

# 4. Run scenario integration tests
./g8e test scenario
```

### First-time Setup

If no users exist, the first login automatically bootstraps the platform:

```bash
./g8e platform start
./g8e auth login
```

This creates the first user and issues mTLS certificates for the Operator and CLI.

---

## Test Implementation

### Go (Operator)

- **Tooling** - Standard `go test` with optional `gotestsum` for dots-style output.
- **Race detection** - Enabled via `-race` in CI and by default in `./g8e test`.
- **Parallelism** - `-parallel 4` with `180s` timeout.
- **Coverage** - `--coverage` generates reports.
- **Concurrency** - Goroutines require explicit cancellation contexts and clear channel ownership.
- **Integration tags** - Scenario tests require `-tags=integration` to access test fixtures and Operator gate infrastructure.

### Lints

- **`make lint-no-bare-session-id`** - CI-enforced lint preventing bare `session_id`. Excludes vendor, generated files, `.local.dev`, `.github`, `docs`, `site`, and the Makefile.
- **`make validate-doctrines`** - Validates doctrine JSON schema against the governance policy model.

---

## Infrastructure Ports

Defaults from `protocol/constants/ports.json` (canonical source of truth):

- `8440` - Operator mTLS API and Pub/Sub
- `8441` - Operator Bootstrap (plain HTTP; device-link enrollment)
- `8443` - Operator Public TLS (browser/BYO bootstrap)
- `18789` - Insecure MCP Gateway

All defaults are unprivileged ports (>1024). To run on `443`/`80`, grant `CAP_NET_BIND_SERVICE` to the Operator binary or front with an external port redirect.

---

## Continuous Integration

GitHub Actions (`.github/workflows/build-and-test.yml`) enforces:

- **`verify-proto`** - Generated Go and Python code sync with `.proto` definitions.
- **`lint-g8eo`** - Runs `golangci-lint` on the Operator and protocol code.
- **`vulncheck-g8eo`** - Scans Go dependencies for known vulnerabilities.
- **`test-g8eo`** (blocking) - Installs Go, starts the platform, runs `./g8e test` with 60% coverage threshold.
- **`test-scenarios`** - Runs scenario integration tests with `-tags=integration`.
- **`constants-lint`** - Enforces use of constants instead of raw string literals.
- **`docs-lint`** - Validates Markdown formatting with markdownlint.
- **`docs-build`** - Builds CLI reference and documentation site.
