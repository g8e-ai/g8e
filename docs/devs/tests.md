---
title: Tests
---

# Testing g8e

Last Updated: 2026-06-07

g8e tests run directly on the host using real infrastructure. The test environment is the production environment. If it does not work in tests, it will not work in production.

---

## Test Philosophy

- **Hermetic execution** - Tests run on the host via `./g8e test`. The g8e Node is a unified g8e Node that operates as g8e Gateway (Policy Decision Point) in gateway mode or as g8e Operator (Policy Execution Point) in Operator mode.
- **Real infrastructure** - Tests use the actual SQLite database, PKI certificates, and pub/sub channels. Platform starts via `./g8e gw start`.
- **No mocks** - Mocking internal services, database clients, or cross-component communication is prohibited. Integration tests use real wire paths.
- **mTLS required** - Operator communication requires mTLS. Authentication via `./g8e auth login` issues certificates from `.g8e/pki`.
- **Reproduce first** - Reproduce bugs with failing tests before fixes.
- **Contract tests** - Enforce alignment between the Operator and `protocol/` constants/models with typed protobuf assertions.

---

## Test Architecture (4-Tier Model)

g8e tests are organized into four clearly defined tiers using Go build tags:

| Tier | Name | Target Directory | Build Tag | External Deps | Execution Time |
| --- | --- | --- | --- | --- | --- |
| **Tier 1** | **Unit Tests** | `internal/...` & `pkg/...` | *No tags* (Runs by default) | None (mock/stub-only, no files/network/DB) | < 10ms per test |
| **Tier 2** | **In-Memory Integration** | `internal/...` & `test/` | `//go:build integration` | SQLite in-memory, local PKI generation, local pubsub | < 2s per suite |
| **Tier 3** | **Live-Platform E2E** | `test/` & `test/scenario/` | `//go:build e2e` | Running g8e gateway & operator processes | < 30s per suite |
| **Tier 4** | **Chaos & Stress** | `internal/test/chaos/` | `//go:build chaos` | Fuzz/load driver (can run in-process) | Custom |

---

## Test Harness

### CLI Test Commands

```bash
./g8e test unit        # Run Tier 1 (Unit) tests - no external dependencies
./g8e test integration # Run Tier 2 (In-Memory Integration) tests - SQLite in-memory, local PKI
./g8e test e2e         # Run Tier 3 (Live Platform E2E) tests - requires running gateway
./g8e test scenario    # Run Tier 3 (Scenario) tests - requires running gateway
./g8e test chaos       # Run Tier 4 (Chaos & Stress) tests
```

The CLI test commands map directly to the 4-tier test architecture:

- **`./g8e test unit`** - Runs unit tests without build tags. These tests use mocks/stubs and have no external dependencies (no files, network, or DB). Fast feedback loop for local development.

- **`./g8e test integration`** - Runs in-memory integration tests with the `integration` build tag. These tests use SQLite in-memory databases, local PKI generation, and local pubsub. No running gateway required.

- **`./g8e test e2e`** - Runs live-platform E2E tests with the `e2e` build tag. These tests require a running g8e gateway and authenticated CLI session (`./g8e gw start` and `./g8e auth login`).

- **`./g8e test scenario`** - Runs scenario-specific E2E tests with the `e2e` build tag. These tests exercise end-to-end governance workflows across doctrine, consensus, and notary modes. Requires running gateway.

- **`./g8e test chaos`** - Runs chaos engineering tests with the `chaos` build tag. These tests fire random payloads at the Operator to verify fail-closed behavior and invariant enforcement.

Validates the g8e Node and protocol enforcement (`GovernanceEnvelope`, 5-layer governance, Audit Vault). Tests cover pub/sub command dispatch, L1/L2/L3/L4/L5 verification, transaction replay protection, state root validation, and audit vault integrity.

### Scenario Tests

```bash
./g8e test scenario
./g8e test scenario --run forge_signature
```

Integration tests exercising end-to-end governance workflows across doctrine, consensus, and notary modes. Tests cover the 5-layer verification sequence (L1-L5), transaction replay protection, state root validation, and receipt verification. Requires the g8e Gateway to be running and authenticated CLI session.

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

**Test Categories**:
- **Good Actor (60%)** - Valid read operations (FS_LIST) that should execute successfully
- **Prompt Injection (20%)** - Forbidden bash commands (EXECUTE_BASH) that should be blocked at L1
- **Man-in-the-Middle (10%)** - Envelopes with corrupted transaction hashes that should fail hash validation
- **File Mutation (10%)** - Valid FILE_EDIT mutations that should execute with L3 proof

**Chaos Test Infrastructure** (`internal/test/chaos/chaos.go`):
- **Envelope Construction** - Tests for signed envelope creation, timestamp handling, session ID binding, operator ID, state root, L2 signature, and nonce format
- **Replay Protection** - In-memory replay store with ReserveNonce, FinalizeNonce, and ReleaseNonce operations
- **L3 Notary Mock** - Chaos-specific L3 notary that always returns true for testing
- **State Root Provider** - Dynamic state root with GetCurrentStateRoot and UpdateRoot operations
- **Rejection Classification** - Classifies rejection reasons (L1_BLOCKED, HASH_FAIL, L2_REJECTED, EXPIRED, REPLAY, REJECTED)
- **Batch Event Writer** - Queues and flushes chaos events to audit vault with auto-flush on batch size
- **Counters** - Atomic counters for executed, l1Blocked, hashFail, other, executedGoodActor, executedFileMut
- **Category Distribution** - Verifies chaos category distribution matches expected percentages
- **Envelope Variants** - Tests for good actor, prompt injection, file mutation, and MitM envelope construction
- **Test-Only Audit Store** - Uses `storagetest.TestSQLAuditStore` (test infrastructure, not production code) which implements the `TransactionAuditStore` interface via a no-op `DocSet` method

---

## Test Components

### Integration Tests (`test/`)

Integration tests exercise end-to-end workflows with real infrastructure (no mocks). These tests require the Gateway to be running and authentication completed.

#### Gateway Protocol Tests

**A2A Gateway Tests** (`test/a2a_gateway_test.go`):
- `TestA2AGateway_SkillCallEndToEnd` - Validates A2A protocol translation to GovernanceEnvelope, 3-layer verification (L1/L2/L3), suspension & OOB approval, and downstream dispatch
- `TestA2AGateway_PayloadVariations` - Tests different payload structures and edge cases
- `TestA2AGateway_ErrorCases` - Validates error handling and fail-closed behavior

**MCP Gateway Tests** (`test/mcp_gateway_test.go`):
- `TestMCPGateway_EndToEnd` - Validates MCP protocol translation (JSON-RPC tools/list, tools/call) to GovernanceEnvelope, 3-layer verification, suspension & OOB approval, and downstream dispatch
- `TestMCPGateway_PayloadVariations` - Tests different JSON-RPC payload structures
- `TestMCPGateway_ErrorCases` - Validates error handling for malformed JSON-RPC

**MCP Stdio Tests** (`test/mcp_stdio_test.go`):
- `TestMCPGateway_ConfigOutput` - Validates MCP stdio config generation
- `TestMCPGateway_CommandExists` - Verifies MCP stdio command availability
- `TestMCPGateway_JSONRPCParsing` - Tests JSON-RPC message parsing
- `TestMCPGateway_ConfigTemplate` - Validates config template rendering

**Universal Gateway Tests** (`test/universal_gateway_integration_test.go`):
- `TestUniversalGateway_RealMCPFlow` - Real MCP protocol translation with live platform
- `TestUniversalGateway_RealA2AFlow` - Real A2A protocol translation with live platform
- `TestUniversalGateway_MultiProtocolAutoDetection` - Auto-detection of MCP vs A2A payloads
- `TestUniversalGateway_GovernanceEnvelopeVerification` - Full L1/L2/L3 verification with real infrastructure
- `TestUniversalGateway_OOBSuspensionAndApproval` - OOB suspension and WebAuthn approval flow
- `TestUniversalGateway_RealDownstreamIntegration` - Real downstream server integration
- `TestUniversalGateway_CanonicalJSONWireFormat` - Canonical JSON wire format validation

#### Real Operator Tests

**A2A Real Operator** (`test/a2a_real_operator_test.go`):
- `TestA2ARealOperator_Smoke` - Smoke test for A2A real operator integration

**MCP Real Operator** (`test/mcp_real_operator_test.go`):
- `TestMCPRealOperator_Smoke` - Smoke test for MCP real operator integration

**Native Real Operator** (`test/native_real_operator_test.go`):
- `TestNativeRealOperator_Smoke` - Smoke test for native real operator integration

#### BYO Client Tests

**BYO Client** (`test/byo_client_test.go`):
- `TestBYOClientParity_EndToEnd` - Protocol-aware BYO client testing canonical JSON wire format, mTLS enrollment, state binding, fail-closed L3, and real execution

#### Integration Helpers

**Integration Helper** (`test/integration_helper.go`):
- `NewLiveOperatorHTTPClient` - Creates mTLS HTTP client configured for live platform testing
- `ResolveRepoRootFromTestDir` - Resolves repository root using go list

### Unit Tests

#### Models Tests (`internal/models/`)

Tests for data model serialization and validation:
- `auth_test.go` - Authentication model tests
- `base_test.go` - Base model tests
- `commands_test.go` - Command model tests
- `file_edit_test.go` - File edit model tests
- `fs_grep_test.go` - File system grep model tests
- `fs_list_test.go` - File system list model tests
- `gateway_test.go` - Gateway model tests
- `heartbeat_test.go` - Heartbeat model tests
- `suspended_test.go` - Suspended state model tests
- `timestamp_test.go` - Timestamp model tests
- `wire_test.go` - Wire format model tests

#### Gateway Service Tests (`internal/services/gateway/`)

Comprehensive gateway service testing:
- `admin_controller_test.go` - Admin API controller tests
- `app_enrollment_service_test.go` - App enrollment flow tests
- `auth_controller_test.go` - Authentication controller tests
- `auth_integrity_test.go` - Authentication integrity tests
- `bootstrap_test.go` - Gateway bootstrap tests
- `cli_l3_notary_test.go` - CLI-based L3 notary tests
- `composite_l3_verifier_test.go` - Composite L3 verification tests
- `db_controller_test.go` - Database controller tests
- `gateway_auth_test.go` - Gateway authentication tests
- `gateway_db_test.go` - Gateway database tests
- `gateway_http_test.go` - Gateway HTTP handler tests
- `gateway_jwt_integration_test.go` - JWT integration tests
- `gateway_pubsub_test.go` - Gateway pub/sub tests
- `governance_envelope_fuzz_test.go` - Fuzz testing for governance envelopes
- `governance_envelope_test.go` - Governance envelope validation tests
- `jwks_test.go` - JWKS endpoint tests
- `listen_service_test.go` - Listen service tests
- `network_identity_test.go` - Network identity tests
- `operator_controller_test.go` - Operator controller tests
- `passkey_service_test.go` - WebAuthn passkey tests
- `pki_authority_test.go` - PKI authority tests
- `pki_controller_test.go` - PKI controller tests
- `registration_service_test.go` - Registration service tests
- `secret_manager_test.go` - Secret manager tests
- `session_service_test.go` - Session service tests
- `state_root_test.go` - State root management tests
- `user_service_test.go` - User service tests
- `test_setup.go` - Shared test infrastructure setup

#### Governance Service Tests (`internal/services/governance/`)

Five-layer governance pipeline tests:
- `actuator_pub_export_test.go` - L5 Actuator public export tests
- `eval_answer_test.go` - L1 Doctrine evaluation answer tests
- `governance_test.go` - General governance tests
- `l1_doctrine_payload_test.go` - L1 Doctrine payload validation tests
- `l1_doctrine_test.go` - L1 Doctrine pattern matching tests
- `l3_notary_test.go` - L3 Notary human presence proof tests
- `l4_warden_test.go` - L4 Warden integrity tests
- `l5_actuator_test.go` - L5 Actuator execution tests
- `transaction_verifier_test.go` - Transaction verifier integration tests

#### Execution Service Tests (`internal/services/execution/`)

Command execution and file operation tests:
- `execution_activity_test.go` - Execution activity tracking tests
- `execution_integration_test.go` - Execution integration tests
- `execution_shell_operators_test.go` - Shell operator execution tests
- `execution_test.go` - General execution service tests
- `file_edit_integration_test.go` - File edit integration tests
- `file_edit_operations_test.go` - File edit operation tests
- `file_edit_test.go` - File edit service tests
- `file_edit_validation_test.go` - File edit validation tests
- `fs_grep_test.go` - File system grep tests
- `fs_list_test.go` - File system list tests

#### Auth Service Tests (`internal/services/auth/`)

Authentication and PKI tests:
- `bootstrap_test.go` - Auth bootstrap tests
- `fingerprint_test.go` - Certificate fingerprint tests
- `fingerprint_windows_test.go` - Windows-specific fingerprint tests

#### Other Service Tests

- `internal/services/g8eo_test.go` - g8e Operator lifecycle tests
- `internal/services/g8eo_integration_test.go` - g8e Operator integration tests
- `internal/services/g8eo_lifecycle_test.go` - g8e Operator lifecycle management tests
- `internal/services/pubsub/heartbeat_service_test.go` - Pub/sub heartbeat tests

#### CLI Tests (`internal/cli/`)

CLI command and configuration tests:
- `api/client_test.go` - API client tests
- `auth/client_test.go` - Auth client tests
- `auth/windows_crypto_test.go` - Windows crypto tests
- `cmd/agent_test.go` - Agent command tests
- `cmd/approve_test.go` - Approve command tests
- `cmd/auditor_test.go` - Emulator command tests
- `cmd/auth_test.go` - Auth command tests
- `cmd/chaos_test.go` - Chaos command tests
- `cmd/cmd_test.go` - General command tests
- `cmd/data_test.go` - Data command tests
- `cmd/main_test.go` - Main command tests
- `cmd/mcp_test.go` - MCP command tests
- `cmd/platform_test.go` - Platform command tests
- `cmd/security_test.go` - Security command tests
- `cmd/setup_test.go` - Setup command tests
- `config/config_test.go` - Configuration loader tests
- `errors/errors_test.go` - Error handling tests
- `jsonrpc/types_test.go` - JSON-RPC type tests
- `platform/browser_test.go` - Browser platform tests
- `platform/process_identity_test.go` - Process identity tests
- `platform/process_test.go` - Process tests

#### Configuration and Constants Tests

- `internal/config/config_test.go` - Configuration tests
- `internal/config/settings_test.go` - Settings tests
- `internal/constants/channels_test.go` - Channel constants tests
- `internal/constants/env_vars_test.go` - Environment variable constants tests
- `internal/constants/exit_codes_test.go` - Exit code constants tests
- `internal/constants/paths_test.go` - Path constants tests

#### Contract Tests

- `internal/contracts/constants_enforcement_test.go` - Constants enforcement tests
- `internal/contracts/protocol_constants_test.go` - Protocol constants tests

#### Infrastructure Tests

- `internal/httpclient/errors_test.go` - HTTP client error tests
- `internal/httpclient/httpclient_test.go` - HTTP client tests
- `internal/marshaler/marshaler_test.go` - Marshaler tests
- `internal/auditor/client/mtls_test.go` - Auditor mTLS client tests
- `internal/certs/embed_test.go` - Embedded certificates tests
- `internal/certs/fetch_test.go` - Certificate fetch tests
- `internal/test/chaos/chaos_test.go` - Chaos engineering tests (detailed above)

#### Storage Test Infrastructure (`internal/services/storage/storagetest/`)

Test-only audit storage implementations separated from production code to avoid import cycles:

- `audit_vault.go` - `TestSQLAuditStore` (test-only monolithic audit service with Git ledger integration)
- `audit_vault_test.go` - Comprehensive tests for the test audit store
- `helpers.go` - Test helper functions (`testGitPath`, `createTestVault`)

**Key distinction**: `storagetest.TestSQLAuditStore` is test infrastructure and should only be used in test code (e.g., chaos tester). Production code uses `storage.SQLAuditStore` from `audit_store.go`.

#### Command Tests

- `cmd/operator/actuator_pub_export_test.go` - Operator actuator export tests
- `cmd/operator/main_subprocess_test.go` - Operator subprocess tests
- `cmd/operator/main_test.go` - Operator main tests
- `cmd/operator/terminal_linux_test.go` - Linux terminal tests
- `internal/cmd/stream_ssh_test.go` - SSH stream tests
- `internal/cmd/stream_ssh_utils_test.go` - SSH stream utility tests
- `internal/cmd/stream_subprocess_test.go` - Subprocess stream tests
- `internal/cmd/stream_test.go` - General stream tests

#### Protocol Tests

- `protocol/workload_identity_test.go` - Workload identity SPIFFE URI tests

#### Package Tests

- `pkg/governance/types_test.go` - Governance types tests

---

## Workflow

```bash
# 1. Run unit tests (no gateway required)
./g8e test unit

# 2. Run in-memory integration tests (no gateway required)
./g8e test integration

# 3. For E2E tests, start the Gateway
./g8e gw start

# 4. Authenticate (required for mTLS tests)
./g8e auth login

# 5. Run E2E tests
./g8e test e2e
./g8e test scenario
```

### First-time Setup

If no users exist, the first login automatically bootstraps the platform:

```bash
./g8e gw start
./g8e auth login
```

This creates the first user and issues mTLS certificates for the g8e Gateway and CLI.

### MCP mTLS Authentication Flow

MCP gateway integration tests (`test/mcp_gateway_test.go`) use mTLS authentication to communicate with the Gateway. The test harness follows this flow:

1. **Bootstrap Enrollment**: Tests first bootstrap the platform via the HTTP port (8080) using `/bootstrap` and `/enroll` endpoints
2. **mTLS Client Creation**: Tests use `NewLiveOperatorHTTPClient` from `test/integration_helper.go` to create an HTTP client configured with:
   - The enrolled operator certificate (`.g8e/pki/client/operator-cert.pem`)
   - The operator private key (`.g8e/pki/client/operator-key.pem`)
   - The canonical trust bundle (`.g8e/pki/trust/g8eg-ca-bundle.pem`)
3. **HTTPS Port Targeting**: All post-enrollment MCP calls target the HTTPS port (8443) with mTLS enforced
4. **SPIFFE Identity Extraction**: The Gateway's `auth.Middleware` extracts the `OperatorSessionID` from the mTLS certificate's SPIFFE URI SAN (format: `spiffe://g8e.local/operator/<org_id>/<operator_id>/<session_id>`)
5. **Session Validation**: The extracted session ID is validated against the database to ensure the operator session is active

**Key Implementation Details**:
- MCP routes are exclusively available on the HTTPS port (8443) - they are NOT available on the HTTP bootstrap port (8080)
- The HTTP port (8080) is limited to bootstrap endpoints only (`/bootstrap`, `/enroll`, `/.well-known/g8e/pki/*`)
- The `ExtractOperatorSessionID` function in `protocol/workload_identity.go` parses the SPIFFE URI to extract the session ID from path segment 6
- Tests include wait logic to ensure operator session persistence before making authenticated calls

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
- **Regression test markers** - When documenting known issues in regression tests (e.g., Phase0Regression tests), use standardized marker constants instead of hardcoded strings. See `internal/services/gateway/pki_authority_test.go` for examples:
  - `RegressionMarkerAfterFix` - indicates expected behavior after a fix is implemented
  - `RegressionMarkerBeforeFix` - indicates current (broken) behavior before a fix
  - `RegressionMarkerIssue` - identifies a specific issue being tracked (e.g., C1, C2, H2)

### Makefile Test Targets

- **`make test`** - Runs all tests (unit + integration + e2e).
- **`make test-unit`** - Runs Tier 1 (Unit) tests without build tags. No external dependencies.
- **`make test-integration`** - Runs Tier 2 (In-Memory Integration) tests with `integration` build tag. Uses SQLite in-memory, local PKI, local pubsub.
- **`make test-e2e`** - Runs Tier 3 (Live Platform E2E) tests with `e2e` build tag. Requires running gateway and auth login.
- **`make test-scenario`** - Runs Tier 3 (Scenario) tests with `e2e` build tag. Requires running gateway and auth login.
- **`make test-short`** - Runs short unit tests with race detection and 60s timeout.
- **`make test-coverage`** - Runs tests with coverage and enforces 60% threshold. Use `PKG=./path/to/pkg` for specific packages, `VERBOSE=true` for verbose output.
- **`make test-shuffle`** - Runs all tests with randomized order.
- **`make test-gateway`** - Runs gateway-specific tests (A2A gateway, MCP gateway, MCP stdio).
- **`make test-mcp`** - Runs MCP tests (MCP gateway, MCP real operator, MCP stdio). Requires running gateway and auth login.
- **`make test-a2a`** - Runs A2A tests (A2A gateway, A2A real operator). Requires running gateway and auth login.
- **`make test-universal-gateway`** - Runs universal gateway integration tests. Requires running gateway and auth login.
- **`make test-byo`** - Runs BYO client tests. Requires running gateway and auth login.
- **`make test-native`** - Runs native real Operator tests. Requires running gateway and auth login.

### Lints

- **`make lint`** - Runs all linting and quality checks including golangci-lint, vulncheck, and doctrine validation.
- **`make lint-no-embedded-newlines`** - Checks for compilation errors including embedded newlines.
- **`make vulncheck`** - Runs `govulncheck` on Go dependencies.
- **`make validate-doctrines`** - Validates doctrine JSON schema against the governance policy model.

---

## Infrastructure Ports

Defaults from `protocol/constants/ports.json` (canonical source of truth):

- `8080` - Gateway HTTP (bootstrap + MCP)
- `8443` - Gateway HTTPS (mTLS API + public)
- `18789` - Insecure MCP Gateway

All defaults are unprivileged ports (>1024). To run on `443`/`80`, grant `CAP_NET_BIND_SERVICE` to the g8e Node or front with an external port redirect.

---

## Continuous Integration

GitHub Actions (`.github/workflows/build-and-test.yml`) enforces:

- **`verify-lint`** - Runs proto generation, linting, and vulnerability scanning.
- **`security`** - Scans Go dependencies for known vulnerabilities.
- **`test-unit-integration`** - Runs Tier 1 (Unit) and Tier 2 (In-Memory Integration) tests. Does not start the gateway.
- **`test-e2e`** - Runs Tier 3 (Live Platform E2E) tests. Starts the gateway, authenticates, runs tests, and stops the gateway. Depends on `verify-lint` to save resources.
