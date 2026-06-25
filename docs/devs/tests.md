---
title: Tests
---

# Testing g8e

Last Updated: 2026-06-25

g8e tests run directly on the host using real infrastructure. The test environment is the production environment. If it does not work in tests, it will not work in production.

---

## Test Philosophy

- **Hermetic execution** - Tests run on the host via `./g8e test`. The g8e Node is a unified g8e Node that operates as g8e Gateway (Policy Decision Point) in gateway mode or as g8e Operator (Policy Execution Point) in Operator mode.
- **Real infrastructure** - Tests use the actual SQLite database, PKI certificates, and pub/sub channels. Platform starts via `./g8e gw start`.
- **No mocks** - Mocking internal services, database clients, or cross-component communication is prohibited. Integration tests use real wire paths.
- **mTLS required** - Operator communication requires mTLS. Authentication via `./g8e auth enroll` issues certificates from `.g8e/pki`.
- **Reproduce first** - Reproduce bugs with failing tests before fixes.
- **Contract tests** - Enforce alignment between the Operator and `protocol/` constants/models with typed protobuf assertions.

---

## Test Architecture (3-Tier Model)

g8e tests are organized into three clearly defined tiers using Go build tags:

| Tier | Name | Target Directory | Build Tag | External Deps | Execution Time |
| --- | --- | --- | --- | --- | --- |
| **Tier 1** | **Unit Tests** | `internal/...` & `pkg/...` | *No tags* (Runs by default) | None (mock/stub-only, no files/network/DB) | < 10ms per test |
| **Tier 2** | **In-Process Integration** | `internal/...` & `test/` | `//go:build integration` | On-disk SQLite, local PKI generation, local pubsub (gateway runs in-process) | < 2s per suite |
| **Tier 3** | **Docker E2E** | `test/e2e/` | `//go:build e2e` | Docker containers (gateway + operator via docker-compose) | < 30s per suite |

---

## Test Harness

### CLI Test Commands

```bash
./g8e test unit        # Run Tier 1 (Unit) tests - no external dependencies
./g8e test integration # Run Tier 2 (In-Process Integration) tests - on-disk SQLite, local PKI
./g8e test e2e         # Run Tier 3 (E2E) tests - requires running gateway
./g8e test coverage    # Run tests with coverage report
./g8e test lint        # Run linting and quality checks
./g8e agent-harness list    # List agent harness scenarios
./g8e agent-harness run     # Run agent harness scenarios against real Gateway/Operator
./g8e test chaos       # Generate realistic governance events for testing
./g8e test summary     # View chaos test summary from test vault
```

The CLI test commands map directly to the 3-tier test architecture:

- **`./g8e test unit`** - Runs unit tests without build tags. These tests use mocks/stubs and have no external dependencies (no files, network, or DB). Fast feedback loop for local development.

- **`./g8e test integration`** - Runs in-process integration tests with the `integration` build tag. These tests run the gateway in-process against real on-disk SQLite databases, local PKI generation, and local pubsub. No separately running gateway required.

- **`./g8e test e2e`** - Runs live-platform E2E tests with the `e2e` build tag. These tests require a running g8e gateway and authenticated CLI session. The Docker E2E tests in `test/e2e/` spin up their own docker-compose infrastructure.

- **`./g8e test coverage`** - Runs tests with coverage profiling and enforces a minimum coverage threshold (70%). Use PKG flag to test a specific package, VERBOSE for detailed output.

- **`./g8e test lint`** - Runs golangci-lint with modern Go best practices. This includes staticcheck, govet, and additional linters for bug prevention, security, and code quality.

- **`./g8e agent-harness list`** - Lists available agent harness scenarios with their posture requirements and personas.

- **`./g8e agent-harness run`** - Runs agent harness scenarios against a real Gateway/Operator. Impersonates arbitrary AI tools and agents, exercising the full protocol surface (MCP, A2A, A2A protobuf, and official governance envelopes with mock consensus and principal signing), then audits every result against the Operator's signed receipts.

- **`./g8e test chaos`** - Generates realistic governance events for testing. Creates a test vault with distributed event categories (70% Good Actor, 20% Prompt Injection, 10% MitM) to test governance pipeline behavior under various conditions.

- **`./g8e test summary`** - Views aggregated chaos test results from the test vault database. Queries the chaos_events table across all test runs in the test vault directory.

Validates the g8e Node and protocol enforcement (`GovernanceEnvelope`, 5-layer governance, Audit Vault). Tests cover pub/sub command dispatch, L1/L2/L3/L4/L5 verification, transaction replay protection, state root validation, and audit vault integrity.

### Docker E2E Tests

```bash
make test-docker
```

Docker-based E2E tests that spin up gateway and operator containers using docker-compose. These tests use the `e2e` build tag and require Docker to be installed and running.

**TestDockerGateway_Health** (`test/e2e/gateway_e2e_test.go`):
- Tests gateway HTTP health endpoint
- Verifies CA bundle discoverable over HTTP
- Checks HTTPS port reachability (no mTLS)
- Validates operator container connection

### Agent Harness

```bash
./g8e agent-harness list
./g8e agent-harness run [scenario ...]
./g8e agent-harness audit
```

The agent harness is a universal agent testing and auditing tool that impersonates arbitrary AI tools and agents against a **REAL** g8e Gateway and Operator. It serves as a protocol compliance verifier by exercising the full g8e surface while recording every exchange for detailed audit.

**Key Design Principle**: The ONLY fiction is the client identity. The Gateway and Operator are real infrastructure components. The agent harness merely wears different "personas" to test how the system behaves when various AI tools interact with it.

**Architecture**:
- **client/** - Thin, faithful HTTP client with mTLS support and exchange recording
- **config/** - Runtime configuration (auth, URLs, ensemble size, L3 mode)
- **scenarios/** - Ordered registry of impersonation scripts (MCP, A2A, governance)
- **report/** - Generates machine-readable JSON and human-readable Markdown reports

**Protocol Coverage**:
- **MCP (Model Context Protocol)** - Tools, resources, and prompts
- **A2A (Agent-to-Agent)** - Skill invocations with JSON and protobuf payloads
- **Governance envelopes** - L2 consensus and L3 notary flows

**Governance Scenarios** (4 scenarios added 2026-06-24):
- `agent-delegation` (Doctrine) - CLI delegates to agent persona, verifies distinct SPIFFE identity
- `tribunal-quorum` (Consensus) - 2-of-3 co-sign, asserts admission success
- `tribunal-veto` (Consensus) - False vote, asserts rejection with status ≥ 400
- `notary-oob` (Notary) - Suspend mode, OOB principal approval flow

The registry now has 14 scenarios total (5 MCP + 3 A2A + 6 governance).

**Testing Postures**:
Scenarios run under different enforcement modes:
- **Doctrine** - L1 enforced, L2/L3 audited
- **Consensus** - L1/L2 enforced, L3 audited
- **Notary** - L1/L2/L3 strictly enforced

**Personas Impersonated**:
- Claude Desktop, Cursor, enterprise agents (MCP clients)
- A2A peers with JSON and protobuf transports
- Mock consensus ensemble (L2 co-signers)
- Mock principal (L3 human notary)

**Governance Testing**:
For consensus/notary scenarios, the agent harness uses mock cryptographic actors:
- **Ensemble**: Mock consensus agents that co-sign L2 envelopes
- **Principal**: Mock human notary for L3 signing (or drives real OOB approve flow)

This allows testing maximal governance envelopes without requiring actual distributed consensus infrastructure.

**Agent Harness Commands**:
- **`./g8e agent-harness list`** - Lists available scenarios with their posture requirements and personas
- **`./g8e agent-harness run`** - Runs scenarios against a real Gateway/Operator with configurable mTLS, public surface, L3 mode (mock|suspend), ensemble size, and phase filtering (doctrine|notary|all)
- **`./g8e agent-harness audit`** - Audits signed receipts from the Operator for a specific session

**Agent Harness Configuration**:
- Supports JSON config overlay for complex scenarios
- Configurable mTLS surface, public surface, client certificates, and CA bundle
- Operator API key authentication for MCP/A2A surface
- Session-scoped audit for specific operator sessions
- Verbose mode for request/response echo
- Report output directory for receipts and summaries

---

## Test Components

### Integration Tests (`test/`)

Integration tests exercise end-to-end workflows with real infrastructure (no mocks). These tests require the Gateway to be running and authentication completed.

#### Gateway Protocol Tests

**A2A Gateway Tests** (`test/a2a_gateway_test.go`):
- `TestA2AGateway_SkillCallEndToEnd` - Validates A2A protocol translation to GovernanceEnvelope, 3-layer verification (L1/L2/L3), suspension and OOB approval, and downstream dispatch
- `TestA2AGateway_PayloadVariations` - Tests different payload structures and edge cases
- `TestA2AGateway_ErrorCases` - Validates error handling and fail-closed behavior

**MCP Gateway Tests** (`test/mcp_gateway_test.go`):
- `TestMCPGateway_EndToEnd` - Validates MCP protocol translation (JSON-RPC tools/list, tools/call) to GovernanceEnvelope, 3-layer verification, suspension and OOB approval, and downstream dispatch
- `TestMCPGateway_PayloadVariations` - Tests different JSON-RPC payload structures
- `TestMCPGateway_ErrorCases` - Validates error handling for malformed JSON-RPC

**Goose Integration Tests** (`test/goose_integration_test.go`):
- `TestGooseGovernanceConfig` - Validates Goose governance configuration generation

**MCP Backup Integration Tests** (`test/mcp_backup_integration_test.go`):
- `TestBackupConfigFile` - Validates MCP backup config file handling

**Tribunal Consensus Tests** (`test/tribunal_consensus_integration_test.go`):
- `TestTribunalConsensus_IdempotentEnrollment` - Verifies repeated CLI enrollment returns the same identity without error
- `TestTribunalConsensus_MalformedCSR` - Validates that malformed CSR submissions are rejected with typed error
- `TestTribunalConsensus_DelegatedAppEnrollment` - CLI delegates to an agent app via `CreateCLIMTLSClient`, verifies distinct SPIFFE identity and successful enrollment
- `TestTribunalConsensus_QuorumReached` - 2-of-3 tribunal co-sign produces admission success
- `TestTribunalConsensus_QuorumNotReached` - Insufficient votes (1-of-3) produces rejection with quorum-not-met error
- `TestTribunalConsensus_VetoByMITRE` - A false vote from MITRE member produces veto rejection
- `TestTribunalConsensus_L1ToL5Walkthrough` - Full L1–L5 identity-bound action walkthrough with receipt verification

All 7 tests use `GatewayFixture` with real PKI, SQLite, `TribunalService`, and `LocalDeliberator` — no mocks, no `t.Parallel()`.

**Universal Gateway Tests** (`test/universal_gateway_integration_test.go`):
- `TestUniversalGateway_MCPFlow` - Real MCP protocol translation with live platform
- `TestUniversalGateway_A2AFlow` - Real A2A protocol translation with live platform
- `TestUniversalGateway_MultiProtocolAutoDetection` - Auto-detection of MCP vs A2A payloads
- `TestUniversalGateway_GovernanceEnvelopeVerification` - Full L1/L2/L3 verification with real infrastructure
- `TestUniversalGateway_OOBSuspensionAndApproval` - OOB suspension and WebAuthn approval flow
- `TestUniversalGateway_DownstreamIntegration` - Real downstream server integration
- `TestUniversalGateway_CanonicalJSONWireFormat` - Canonical JSON wire format validation

**Native Tool Registry Tests** (`test/native_tool_registry_integration_test.go`):
- `TestRegisterNativeTools` - Validates registration of all 30 native tools (db_discover_topology, db_query_validate, db_isolated_read, db_index_triage, log_stream_filter, sys_oom_detect, config_diff_mask, proc_metric_top, fs_disk_profile, proc_signal_safe, net_socket_audit, net_endpoint_ping, net_http_probe, sys_info, net_dns_resolve, tls_cert_inspect, sys_env_vars, fs_file_checksum, sys_service_status, sys_container_status, fs_disk_usage, sys_time_clock, proc_tree, git_ops, cloud_metadata, k8s_inspect, shell_execute, net_ssh_known_hosts, operator_deploy, file_read)
- `TestRegisterNativeTools_DuplicateRegistration` - Verifies duplicate registration fails
- `TestRegisterNativeTools_NilRegistry` - Tests panic on nil registry
- `TestRegisterNativeTools_ToolNameConsistency` - Validates naming convention (lowercase with underscores)
- `TestRegisterNativeTools_SchemaValidity` - Verifies all tool schemas are valid
- `TestRegisterNativeTools_PartialRegistration` - Tests partial registration failure

#### Docker E2E Tests (`test/e2e/`)

**Gateway E2E** (`test/e2e/gateway_e2e_test.go`):
- `TestDockerGateway_Health` - Tests Docker-based gateway health endpoints

**Auth E2E** (`test/e2e/auth_e2e_test.go`):
- `TestDockerGateway_Auth` - Black-box cross-container auth validation with 3 subtests:
  - `mTLS handshake over network` - Verifies operator establishes mTLS with gateway over Docker bridge (log-based assertion)
  - `CA bundle consistency` - Compares CA bundle from `/.well-known` with operator's trusted chain (log-based)
  - `restart persistence` - Restarts operator container, verifies re-authentication using persisted enrolled identity (no fresh bootstrap)

**MCP Stdio E2E** (`test/mcp_stdio_test.go`):
- `TestMCPGateway_ConfigOutput` - Validates MCP stdio config generation
- `TestMCPGateway_CommandExists` - Verifies MCP stdio command availability
- `TestMCPGateway_JSONRPCParsing` - Tests JSON-RPC message parsing
- `TestMCPGateway_ConfigTemplate` - Validates config template rendering

**E2E Harness** (`test/e2e/harness.go`):
- `DockerE2EFixture` - Manages Docker-based E2E test infrastructure
- `NewDockerE2EFixture` - Creates and starts Docker containers for testing
- `GetHealth` - Retrieves gateway health status
- `GetCABundle` - Retrieves CA bundle from gateway
- `CheckOperatorContainer` - Verifies operator container connection
- `OperatorLogs` - Returns combined stdout/stderr logs of the operator container (black-box observation)
- `RestartOperator` - Restarts operator container and waits for re-authentication via health polling

#### Integration Helpers

**Integration Helper** (`test/integration_helper.go`):
- `NewLiveOperatorHTTPClient` - Creates mTLS HTTP client configured for live platform testing
- `ResolveRepoRootFromTestDir` - Resolves repository root using `go list -m`
- `RunCLICommand` - Executes `./g8e` commands with error handling and output capture, building the binary if needed
- `RunCLICommandRequire` - Convenience wrapper around `RunCLICommand` that calls `t.Fatalf` on failure

#### Test Fixtures (`test/fixtures/`)

Reusable test infrastructure for integration and E2E tests:

**Gateway Fixture** (`test/fixtures/gateway_fixture.go`):
- `GatewayFixture` - Reusable gateway setup for integration tests with full lifecycle management
- `NewGatewayFixture` - Creates a fully configured gateway with downstream server, execution services, governance dependencies, and MCP gateway wiring
- `EnrollClientIdentity` - Performs CSR enrollment for test clients with certificate generation and operator session creation. Correctly sets `CLICertificate`, `CLIPrivateKey`, and `CLISessionID` (CS-13 fix)
- `CreateMTLSClient` - Creates HTTP client configured for mTLS using enrolled operator identity
- `CreateCLIMTLSClient` - Creates mTLS HTTP client using the CLI certificate (distinct from operator cert) with `X-G8E-CLI-Session-ID` header injection via `cliSessionRoundTripper`. Used for delegated app enrollment and CLI-identity-bound flows
- `SetupTribunal` - Wires a real `TribunalService` via the shared `tribunal.NewTribunalFromPolicy` factory with Ed25519 member keys, `TribunalPolicy`, and `LocalDeliberator`. Generates `nMembers` key pairs, registers each as a `TrustedSigner`, and supports `nServiceMembers < nMembers` split for quorum-not-reached simulation
- `WaitForReady` - Polls HTTP health endpoint until server accepts connections
- `SetPublicBaseURL` - Sets public base URL for MCP gateway (used for approval links)
- Supports configurable posture (notary, consensus, doctrine), custom downstream URLs, and test port zero allowance
- Handles path initialization, mock downstream MCP server, and actuator key setup
- `Cleanup` stops the gateway to release database locks but **does not delete the data directory**; integration runs leave their artifacts behind (see [Fixture Lifecycle and Results](#fixture-lifecycle-and-results))

#### Fixture Lifecycle and Results

Integration fixtures (`GatewayFixture`) follow a deliberate lifecycle so that runs stay isolated and reproducible while leaving inspectable artifacts behind.

**Results are persisted, never cleaned up between or within runs.** `NewGatewayFixture` writes each run's data/vault/PKI to a fresh, uniquely-named directory under `<repo>/test-results/` (created via `os.MkdirTemp`, so concurrent fixtures in the same test and second never collide). This directory is **not** placed under `t.TempDir()` and is **not** deleted, so results accumulate across runs for later inspection. `test-results/` is gitignored. `Cleanup` stops the gateway, cancelling its context, joining the `Start` goroutine, and calling `Stop` to release database locks, but it intentionally leaves the data directory on disk.

**Own the teardown at the test scope, exactly once.** Register cleanup with `t.Cleanup(f.Cleanup)` (preferred) or `defer f.Cleanup()` in the test body. Follow these rules:

- **Never `defer f.Cleanup()` inside a setup helper.** A deferred cleanup fires when the *helper returns*, which is before the test body runs, tearing the gateway down and closing every database out from under the test. The symptom is `sql: database is closed` on the first gateway call. If a helper creates the fixture, it must register teardown with `t.Cleanup` (which runs at the end of the test), not `defer`.
- **Never clean up twice.** If a setup helper already registered `t.Cleanup(f.Cleanup)`, the caller must not also `defer f.Cleanup()`. `Cleanup` joins the gateway's `Start` goroutine over a buffered (size 1) error channel; a second call blocks forever on that already-drained channel and the test hangs until the timeout panic.
- **Hold temp credential files for the whole test.** Cert/key temp files handed to the mTLS client must outlive the test body. Register their removal with `t.Cleanup`, not `defer`, in any setup helper for the same reason as above.

**Tests run sequentially.** Integration tests do not call `t.Parallel()`. Each test gets its own isolated data directory and a random port (`AllowTestPortZero: true`), so they neither share state nor contend for ports. Do not add `t.Parallel()` to these suites.

### Unit Tests

#### Models Tests (`internal/models/`)

Tests for data model serialization and validation:
- `heartbeat_test.go` - Heartbeat model tests
- `timestamp_test.go` - Timestamp model tests

#### Gateway Service Tests (`internal/services/gateway/`)

Comprehensive gateway service testing:
- `admin_controller_test.go` - Admin API controller tests including authz-precedence subtest (CS-5: no-auth + no-tribunal-ID → 401, not 400)
- `app_enrollment_service_test.go` - App enrollment flow tests
- `app_policy_store_service_test.go` - App policy store tests
- `auth_controller_approvals_test.go` - Auth controller approvals tests
- `auth_controller_bootstrap_test.go` - Auth controller bootstrap tests
- `auth_controller_passkey_test.go` - Auth controller passkey tests
- `auth_controller_session_test.go` - Auth controller session tests
- `auth_controller_test.go` - Authentication controller tests
- `auth_integrity_test.go` - Authentication integrity tests
- `blob_store_service_test.go` - Blob store service tests
- `bootstrap_test.go` - Gateway bootstrap tests
- `cli_session_verifier_test.go` - CLI session verifier and unified L3 notary tests
- `cli_session_service_test.go` - CLI session service tests
- `db_controller_test.go` - Database controller tests
- `document_store_service_test.go` - Document store service tests
- `gateway_auth_bench_test.go` - Gateway authentication benchmark tests
- `gateway_auth_test.go` - Gateway authentication tests
- `gateway_certs_test.go` - Gateway certificate tests
- `gateway_db_test.go` - Gateway database tests
- `gateway_http_test.go` - Gateway HTTP handler tests
- `gateway_jwt_integration_test.go` - JWT integration tests
- `gateway_pubsub_test.go` - Gateway pub/sub tests
- `gateway_service_test.go` - Gateway service tests
- `governance_envelope_fuzz_test.go` - Fuzz testing for governance envelopes
- `governance_envelope_quality_test.go` - Governance envelope quality tests
- `governance_envelope_test.go` - Governance envelope validation tests
- `jwks_test.go` - JWKS endpoint tests
- `jwt_native_test.go` - Native JWT tests
- `jwt_native_unit_test.go` - Native JWT unit tests
- `kv_store_service_test.go` - KV store service tests
- `network_identity_test.go` - Network identity tests
- `operator_controller_test.go` - Operator controller tests
- `passkey_service_test.go` - WebAuthn passkey tests
- `pki_authority_test.go` - PKI authority tests
- `pki_controller_test.go` - PKI controller tests
- `registration_service_test.go` - Registration service tests
- `replay_store_service_native_unit_test.go` - Replay store native unit tests
- `replay_store_service_test.go` - Replay store service tests
- `secret_manager_test.go` - Secret manager tests
- `signer_store_service_test.go` - Signer store service tests
- `sse_event_service_test.go` - SSE event service tests
- `state_root_service_test.go` - State root service tests
- `state_root_test.go` - State root management tests
- `test_setup_test.go` - Test setup helpers tests
- `tribunal_store_service_test.go` - Tribunal store service tests
- `user_service_integration_test.go` - User service integration tests
- `user_service_test.go` - User service tests
- `web_session_service_test.go` - Web session service tests

**Console SPA Tests** (`internal/services/gateway/console/`):
- `console_test.go` - Console SPA handler and static asset tests

#### Tribunal Service Tests (`internal/services/tribunal/`)

Tribunal consensus and key provisioning tests:
- `factory_test.go` - `NewTribunalFromPolicy` factory tests including `FileKeyProvider` key loading (success, not-found, invalid seed length, invalid hex), `SaveMemberKey` provisioning (creates directory and file), and multi-member key resolution
- `local_deliberator_test.go` - `LocalDeliberator` tests including quorum counting, veto detection, and nil-member handling
- `service_test.go` - `TribunalService` tests including deliberation, nil-key member skipping (aligns with factory contract), and quorum behavior

**Shared Factory** (`internal/services/tribunal/factory.go`):
- `NewTribunalFromPolicy` - Shared factory used by both production `BootstrapTribunal` and test `SetupTribunal` to construct `TribunalService` from a `TribunalPolicy`, `KeyProvider`, and `L1Doctrine`. Eliminates duplication (CS-12)
- `KeyProvider` interface + `KeyProviderFunc` adapter - Allows each caller to supply keys from its own source
- `FileKeyProvider` - Loads Ed25519 seeds from `{secretsDir}/{prefix}{tribunalID}_{appID}.key` for multi-member co-signing (CS-9)
- `SaveMemberKey` - Provisioning helper that writes hex-encoded Ed25519 seeds to the secrets directory

#### Governance Service Tests (`internal/services/governance/`)

Five-layer governance pipeline tests:
- `actuator_pub_export_test.go` - L5 Actuator public export tests
- `capability_test.go` - Governance capability tests
- `eval_answer_test.go` - L1 Doctrine evaluation answer tests
- `eval_answer_integration_test.go` - L1 Doctrine evaluation answer integration tests
- `governance_test.go` - General governance tests
- `governance_integration_test.go` - Governance integration tests with `//go:build integration` tag. Includes `TestGovernanceFailClosed` with 5 fail-closed subtests: `NilReplayStore_FailClosed`, `NilStateRootProvider_FailClosed`, `EmptyStateRoot_FailClosed`, `NilDoctrine_DefaultsToValid`, `NilTribunalStore_ConsensusFailClosed`. Uses real implementations (`StatefulMockReplayStore`, `SimpleStateRootProvider`), `testutil.NewTestLogger()`, and typed error assertions via `errors.Is` (CS-7)
- `l1_doctrine_payload_test.go` - L1 Doctrine payload validation tests
- `l1_doctrine_test.go` - L1 Doctrine pattern matching tests
- `l3_notary_test.go` - L3 Notary human presence proof tests
- `l3_notary_integration_test.go` - L3 Notary integration tests with `//go:build integration` tag
- `l4_warden_consensus_test.go` - L4 Warden consensus verification tests
- `l4_warden_doctrine_test.go` - L4 Warden doctrine verification tests
- `l4_warden_notary_test.go` - L4 Warden notary verification tests
- `l4_warden_test.go` - L4 Warden integrity tests
- `l5_actuator_test.go` - L5 Actuator execution tests
- `l5_actuator_integration_test.go` - L5 Actuator integration tests with `//go:build integration` tag
- `processor_test.go` - Governance processor tests

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
- `internal/services/g8eo_integration_test.go` - g8e Operator integration tests with `//go:build integration` tag
- `internal/services/g8eo_lifecycle_test.go` - g8e Operator lifecycle management tests
- `internal/services/pubsub/audit_service_test.go` - Pub/sub audit service tests
- `internal/services/pubsub/command_service_test.go` - Pub/sub command service tests
- `internal/services/pubsub/file_ops_service_test.go` - Pub/sub file operations service tests
- `internal/services/pubsub/g8eg_pubsub_client_test.go` - g8e Gateway pub/sub client tests
- `internal/services/pubsub/pubsubtest/mock_client_test.go` - PubSub mock client tests
- `internal/services/pubsub/heartbeat_service_test.go` - Pub/sub heartbeat tests
- `internal/services/pubsub/history_service_test.go` - Pub/sub history service tests
- `internal/services/pubsub/inprocess_client_test.go` - In-process pub/sub client tests
- `internal/services/pubsub/port_service_test.go` - Pub/sub port service tests
- `internal/services/pubsub/protocol_helpers_test.go` - Pub/sub protocol helper tests
- `internal/services/pubsub/publish_helpers_test.go` - Pub/sub publish helper tests
- `internal/services/pubsub/pubsub_commands_test.go` - Pub/sub command tests
- `internal/services/pubsub/pubsub_fixtures_test.go` - Pub/sub fixture tests
- `internal/services/pubsub/pubsub_integration_helpers_test.go` - Pub/sub integration helper tests
- `internal/services/pubsub/pubsub_l3_integration_test.go` - Pub/sub L3 integration tests
- `internal/services/pubsub/pubsub_results_test.go` - Pub/sub results tests
- `internal/services/pubsub/pubsub_test_helpers_test.go` - Pub/sub test helper tests
- `internal/services/pubsub/tls_errors_test.go` - Pub/sub TLS error tests
- `internal/services/pubsub/vault_writer_test.go` - Pub/sub vault writer tests
- `internal/services/system/git_test.go` - Git utility tests
- `internal/services/system/system_utils_test.go` - System utility tests with `//go:build !windows` tag
- `internal/services/system/utils_test.go` - General system utility tests

#### Keystore Tests (`internal/services/keystore/`)

Keyring backend and keystore abstraction tests:
- `keystore_test.go` - Keystore abstraction tests
- `keyring_file_test.go` - File-based keyring backend tests
- `keyring_keychain_test.go` - macOS Keychain keyring backend tests
- `keyring_libsecret_test.go` - Linux libsecret keyring backend tests
- `keyring_memory_test.go` - In-memory keyring backend tests

#### SQLite Utility Tests (`internal/services/sqliteutil/`)

SQLite helper and utility tests:
- `compress_test.go` - SQLite compression utility tests
- `db_test.go` - SQLite database helper tests
- `pruner_test.go` - SQLite pruner tests
- `timestamp_test.go` - SQLite timestamp utility tests
- `validate_test.go` - SQLite validation utility tests

#### Scrubbing Tests (`internal/services/scrubbing/`)

- `boundary_test.go` - Log scrubbing boundary tests

#### Network Service Tests (`internal/services/network/`)

- `identity_test.go` - Network identity tests

#### Vault Service Tests (`internal/services/vault/`)

- `vault_test.go` - Vault service tests

#### CLI Tests (`internal/cli/`)

CLI command and configuration tests:
- `api/client_test.go` - API client tests
- `auth/agent_enroll_test.go` - Agent enrollment tests with SPIFFE URI SAN generation
- `auth/bootstrap_test.go` - Auth bootstrap tests
- `auth/certificate_test.go` - Certificate handling tests
- `auth/client_test.go` - Auth client tests
- `auth/credentials_test.go` - Credentials tests
- `auth/csr_test.go` - CSR tests
- `auth/fingerprint_test.go` - Fingerprint tests
- `auth/http_client_test.go` - HTTP client tests
- `auth/operator_test.go` - Operator auth tests
- `auth/passkey_bootstrap_test.go` - Passkey bootstrap tests
- `auth/windows_crypto_test.go` - Windows crypto tests
- `cmd/agent_harness_test.go` - Agent harness command tests
- `cmd/approve_integration_test.go` - Approve command integration tests
- `cmd/audit_integration_test.go` - Audit command integration tests
- `cmd/audit_test.go` - Audit command tests
- `cmd/auth_integration_test.go` - Auth command integration tests
- `cmd/auth_test.go` - Auth command tests
- `cmd/chaos_integration_test.go` - Chaos command integration tests
- `cmd/chaos_test.go` - Chaos command tests
- `cmd/cmd_integration_test.go` - General command integration tests
- `cmd/cmd_test.go` - General command tests
- `cmd/data_test.go` - Data command tests
- `cmd/demos_integration_test.go` - Demos command integration tests
- `cmd/demos_test.go` - Demos command tests
- `cmd/gateway_test.go` - Gateway command tests
- `cmd/main_test.go` - Main command tests
- `cmd/mcp_integration_test.go` - MCP command integration tests
- `cmd/mcp_test.go` - MCP command tests
- `cmd/mcp_unix_test.go` - MCP command Unix-specific tests
- `cmd/mcp_windows_test.go` - MCP command Windows-specific tests
- `cmd/operator_test.go` - Operator command tests
- `cmd/platform_test.go` - Platform command tests
- `cmd/security_test.go` - Security command tests
- `cmd/swagger_test.go` - Swagger command tests
- `cmd/test_paths_test.go` - Test path validation tests
- `cmd/vault_test.go` - Vault command tests
- `config/config_test.go` - Configuration loader tests
- `platform/browser_test.go` - Browser platform tests
- `platform/process_identity_test.go` - Process identity tests
- `platform/process_test.go` - Process tests
- `platform/process_unix_test.go` - Unix process tests
- `platform/process_windows_test.go` - Windows process tests

#### Configuration and Constants Tests

- `internal/config/config_test.go` - Configuration tests
- `internal/constants/mappings_test.go` - Constants mappings tests

#### Infrastructure Tests

- `internal/httpclient/httpclient_test.go` - HTTP client tests
- `internal/marshaler/marshaler_test.go` - Marshaler tests
- `internal/response/writer_test.go` - Response writer tests
- `internal/response/writer_fuzz_test.go` - Response writer fuzz tests
- `internal/security/path_test.go` - Security path validation tests
- `internal/certs/embed_test.go` - Embedded certificates tests
- `internal/certs/fetch_test.go` - Certificate fetch tests
- `internal/testutil/crypto_test.go` - Crypto utility tests
- `internal/testutil/governance_mocks_test.go` - Governance mock tests
- `internal/testutil/helpers_test.go` - Test helper tests
- `internal/testutil/proto_helpers_test.go` - Proto helper tests
- `internal/testutil/pubsub_integration_test.go` - Pub/sub integration tests with `//go:build integration` tag
- `internal/testutil/pubsub_test.go` - Pub/sub tests
- `internal/testutil/pubsub_unit_test.go` - Pub/sub unit tests

#### Utility Package Tests

- `internal/exitcode/exitcode_test.go` - Exit code constants tests
- `internal/netutil/netutil_test.go` - Network utility tests
- `internal/paths/paths_test.go` - Path constants tests
- `internal/pathutil/pathutil_test.go` - Path utility tests
- `internal/pkg/ssh/config_test.go` - SSH config tests
- `internal/uuid/uuid_test.go` - UUID generation tests

#### Storage Test Infrastructure (`internal/services/storage/` and `internal/services/storage/storagetest/`)

Storage service tests and test-only audit storage implementations separated from production code to avoid import cycles:

- `audit_store_unit_test.go` - Audit store unit tests
- `commitment_ledger_test.go` - Commitment ledger tests
- `execution_vault_test.go` - Execution vault tests
- `history_handler_test.go` - History handler tests
- `history_handler_unit_test.go` - History handler unit tests
- `ledger_diffcontent_test.go` - Ledger diff content tests
- `ledger_diffstat_test.go` - Ledger diff stat tests
- `ledger_git_test.go` - Ledger Git integration tests
- `ledger_test.go` - Ledger tests
- `replay_store_test.go` - Replay store tests
- `storage_test_helpers_test.go` - Storage test helper tests
- `suspended_transaction_store_test.go` - Suspended transaction store tests

**Test-only audit storage infrastructure** (`internal/services/storage/storagetest/`):

- `audit_vault.go` - `TestSQLAuditStore` (test-only monolithic audit service with Git ledger integration)
- `audit_store_config_test.go` - Configuration tests for the test audit store
- `audit_store_e2e_test.go` - End-to-end audit trail tests including `TestSQLAuditStore_EndToEnd_AuditTrail`
- `audit_store_encryption_test.go` - Encryption tests for sensitive content fields
- `audit_store_event_test.go` - Event storage and retrieval tests
- `audit_store_mutation_test.go` - Mutation operation tests
- `audit_store_receipt_test.go` - Receipt storage and verification tests
- `audit_store_session_test.go` - Session management tests
- `audit_store_test.go` - General audit store tests
- `helpers.go` - Test helper functions (`testGitPath`, `createTestVault`)

**Key distinction**: `storagetest.TestSQLAuditStore` is test infrastructure and should only be used in test code. Production code uses `storage.SQLAuditStore` from `audit_store.go`.

#### Command Tests

- `cmd/operator/main_test.go` - Operator main tests
- `internal/cli/serve/cert_test.go` - Serve certificate tests
- `internal/cli/serve/gateway_test.go` - Serve gateway command tests
- `internal/cli/serve/logger_test.go` - Serve logger tests
- `internal/cli/serve/operator_test.go` - Serve operator command tests
- `internal/cli/serve/terminal_linux_test.go` - Linux terminal tests
- `internal/cli/serve/version_test.go` - Serve version tests
- `internal/cli/stream/stream_ssh_test.go` - SSH stream tests
- `internal/cli/stream/stream_ssh_utils_test.go` - SSH stream utility tests
- `internal/cli/stream/stream_subprocess_test.go` - Subprocess stream tests
- `internal/cli/stream/stream_test.go` - General stream tests

#### MCP Service Tests (`internal/services/mcp/`)

MCP gateway and native tool integration tests:
- `audit_attribution_test.go` - Audit attribution tests
- `config_test.go` - MCP configuration tests
- `db_discover_topology_test.go` - DB discover topology tool tests
- `db_index_triage_test.go` - DB index triage tool tests
- `db_query_validate_test.go` - DB query validate tool tests
- `field_parser_test.go` - Field parser tests
- `file_read_test.go` - File read tool tests
- `fs_disk_profile_test.go` - FS disk profile tool tests
- `fs_disk_usage_test.go` - FS disk usage tool tests
- `fs_disk_usage_windows_test.go` - FS disk usage Windows-specific tests
- `fs_file_checksum_test.go` - FS file checksum tool tests
- `gateway_integration_test.go` - Gateway integration tests with real envelope processing, SSE streaming, circuit breaker, error code mapping, native tool execution, read_field tool with L3 validation, and real L3 verification
- `gateway_test.go` - Gateway service tests
- `git_ops_test.go` - Git ops tool tests
- `k8s_inspect_test.go` - Kubernetes inspect tool tests
- `log_stream_filter_test.go` - Log stream filter tool tests
- `mcp_endpoint_test.go` - MCP endpoint tests
- `models_test.go` - MCP model tests
- `native_handlers_test.go` - Native handler tests
- `native_tool_registry_test.go` - Native tool registry tests
- `native_tools_integration_test.go` - Native tools integration tests with real SQLite databases, audit vault persistence, log filtering with scrubbing, process metrics and signal safety, network auditing and probing, concurrency tests for TOCTOU resistance, property-based tests for safety invariants, and negative controls for intentional failures
- `net_dns_resolve_test.go` - DNS resolve tool tests with mock resolver
- `net_http_probe_test.go` - HTTP probe tool tests
- `net_socket_audit_test.go` - Socket audit tool tests
- `net_ssh_known_hosts_test.go` - SSH known hosts tool tests
- `operator_deploy_test.go` - Operator deploy tool tests
- `proc_signal_safe_integration_test.go` - Process signal safe integration tests
- `proc_signal_safe_test.go` - Process signal safe tool tests
- `proc_tree_test.go` - Process tree tool tests
- `registry_test.go` - Tool registry tests
- `run_shell_command_test.go` - Shell command execution tests
- `suspended_transaction_test.go` - Suspended transaction tests
- `sys_container_status_test.go` - Container status tool tests
- `sys_env_vars_test.go` - Environment variables tool tests
- `sys_info_test.go` - System info tool tests
- `sys_oom_detect_test.go` - OOM detection tool tests
- `sys_time_clock_test.go` - Time clock tool tests
- `sys_tools_test.go` - System tools tests
- `tls_cert_inspect_test.go` - TLS certificate inspect tool tests
- `validation_test.go` - Validation tests

#### Package Tests

- `pkg/governance/types_test.go` - Governance types tests

#### Protocol Tests (`protocol/`)

- `workload_identity_test.go` - Workload identity SPIFFE URI parsing tests

#### Agent Harness Tests (`internal/tools/agent_harness/`)

Agent harness client, config, and scenario tests:
- `client/audit_test.go` - Audit exchange recording tests
- `client/client_test.go` - HTTP client tests
- `client/envelope_test.go` - Governance envelope construction tests
- `client/mtls_test.go` - mTLS client configuration tests
- `client/protocols_test.go` - Protocol coverage tests (MCP, A2A, protobuf)
- `config/config_test.go` - Runtime configuration tests
- `scenarios/governance_test.go` - Governance scenario tests
- `scenarios/mcp_a2a_test.go` - MCP and A2A scenario tests
- `scenarios/scenario_test.go` - Scenario registry and execution tests

#### Chaos Tool Tests (`internal/tools/chaos/`)

- `chaos_test.go` - Chaos event generation tests

---

## Workflow

```bash
# 1. Run unit tests (no gateway required)
./g8e test unit

# 2. Run in-process integration tests (no separately running gateway required)
./g8e test integration

# 3. For Docker E2E tests, ensure Docker is running
make test-docker

# 4. Authenticate (required for mTLS tests)
./g8e auth enroll
```

### First-time Setup

If no users exist, the first login automatically bootstraps the platform:

```bash
./g8e gw start
./g8e auth enroll
```

This creates the first user and issues mTLS certificates for the g8e Gateway and CLI.

### MCP mTLS Authentication Flow

MCP gateway integration tests (`test/mcp_gateway_test.go`) use mTLS authentication to communicate with the Gateway. The test harness follows this flow:

1. **In-Process Gateway**: Tests use `GatewayFixture` from `test/fixtures/gateway_fixture.go` to start a fully configured gateway in-process with downstream server, execution services, and governance dependencies
2. **Client Enrollment**: Tests call `EnrollClientIdentity` from `test/fixtures/gateway_fixture.go` to perform CSR enrollment, generating certificates and creating an operator session
3. **mTLS Client Creation**: Tests use `CreateMTLSClient` from the fixture to create an HTTP client configured with the enrolled identity certificates
4. **HTTPS Port Targeting**: All post-enrollment MCP calls target the HTTPS port (8443) with mTLS enforced
5. **SPIFFE Identity Extraction**: The Gateway's `auth.Middleware` extracts the `OperatorSessionID` from the mTLS certificate's SPIFFE URI SAN (format: `spiffe://g8e.local/operator/<org_id>/<operator_id>/<session_id>`)
6. **Session Validation**: The extracted session ID is validated against the database to ensure the operator session is active

**Key Implementation Details**:
- MCP routes are exclusively available on the HTTPS port (8443); they are NOT available on the HTTP bootstrap port (8080)
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
   ./g8e auth enroll
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
- **Race detection** - Enabled via `-race` in CI and by default in `./g8e test unit`.
- **Parallelism** - `-parallel 4` with `180s` timeout.
- **Coverage** - `--coverage` flag generates reports. CI enforces 70% coverage threshold.
- **Concurrency** - Goroutines require explicit cancellation contexts and clear channel ownership.
- **Integration tags** - Scenario tests require `-tags=integration` to access test fixtures and Gateway gate infrastructure.
- **Path constants** - ALL filepath strings in test code MUST be defined as constants in `internal/constants/paths.go`. No filepath strings may be constructed dynamically or hardcoded inline, including relative paths like `"../../"`, `"./"`, `".g8e/"`, `"/pki/"`, etc. Dynamic path construction using `filepath.Join()` with string literals is prohibited. Tests must use `constants.Paths.Infra.*` constants for runtime state paths (e.g., `constants.Paths.Infra.PkiDir` for `.g8e/pki`). The only exception is when using `TestPaths` for isolated test environments - the base directory for TestPaths must come from a constant, and all path construction within TestPaths must use constants. This eliminates magic strings and improves maintainability and system robustness.
- **Typed error constants** - When testing error handling, check for any hand-trolled strings that should be properly typed errors (e.g., error reason strings, status codes, rejection reasons). Use typed constants from `internal/constants/` instead of hardcoded strings in assertions and error message checks.
- **Regression test markers** - When documenting known issues in regression tests (e.g., Phase0Regression tests), use standardized marker constants instead of hardcoded strings. See `internal/services/gateway/pki_authority_test.go` for examples:
  - `RegressionMarkerAfterFix` - indicates expected behavior after a fix is implemented
  - `RegressionMarkerBeforeFix` - indicates current (broken) behavior before a fix
  - `RegressionMarkerIssue` - identifies a specific issue being tracked (e.g., C1, C2, H2)

### Makefile Test Targets

- **`make test`** - Runs Tier 1 (Unit) and Tier 2 (In-Process Integration) tests.
- **`make test-unit`** - Runs Tier 1 (Unit) tests without build tags. No external dependencies.
- **`make test-integration`** - Runs Tier 2 (In-Process Integration) tests with `integration` build tag. Uses on-disk SQLite, local PKI, local pubsub.
- **`make test-docker`** - Runs Tier 3 (Docker E2E) tests with `e2e` build tag. Requires Docker.
- **`make test-coverage`** - Runs tests with coverage (enforces 70% threshold). Use PKG=./path/to/pkg for specific package, VERBOSE=true for verbose output.
- **`make test-gateway`** - Runs gateway-specific integration tests (A2A gateway, MCP gateway). Targets `test/a2a_gateway_test.go`, `test/mcp_gateway_test.go`, and `test/mcp_stdio_test.go` with the `integration` build tag.
- **`make ci`** - Runs the full CI pipeline locally (proto verification, swagger generation, lint, vulncheck, tests). Equivalent to the GitHub Actions workflow.

### Lints

- **`make lint`** - Runs all linting and quality checks including golangci-lint, lint-no-embedded-newlines, vulncheck, validate-doctrines, and swagger-generate.
- **`make lint-no-embedded-newlines`** - Checks for compilation errors including embedded newlines.
- **`make vulncheck`** - Runs `govulncheck` on Go dependencies.
- **`make validate-doctrines`** - Validates doctrine JSON schema against the governance policy model.
- **`make swagger-generate`** - Generates Swagger/OpenAPI documentation from code annotations.

---

## Infrastructure Ports

Defaults from `protocol/constants/ports.json` (canonical source of truth):

- `8080` - Gateway HTTP (bootstrap, CA bundle, health)
- `8443` - Gateway HTTPS (mTLS API, MCP, public)

All defaults are unprivileged ports (>1024). To run on `443`/`80`, grant `CAP_NET_BIND_SERVICE` to the g8e Node or front with an external port redirect.

---

## Continuous Integration

GitHub Actions (`.github/workflows/build-and-test.yml`) enforces:

- **Verify Proto & Doctrines** - Runs `make proto`, validates generated files are in sync, and runs `make validate-doctrines`.
- **Generate & Validate Swagger Docs** - Runs `make swagger-generate` and validates generated swagger files are in sync.
- **Lint** - Runs golangci-lint via `golangci-lint-action`.
- **Security Scan** - Runs `make vulncheck` (govulncheck).
- **Unit Tests** - Runs `make test-unit`.
- **Integration Tests** - Runs `make test-integration`.
- **Cross-Compile (Windows)** - Runs `make build-windows`.

The CI workflow does not run Tier 3 (Docker E2E) tests. Docker E2E tests require manual execution with Docker installed.
