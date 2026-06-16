---
title: Tests
---

# Testing g8e

Last Updated: 2026-06-15

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

## Test Architecture (3-Tier Model)

g8e tests are organized into three clearly defined tiers using Go build tags:

| Tier | Name | Target Directory | Build Tag | External Deps | Execution Time |
| --- | --- | --- | --- | --- | --- |
| **Tier 1** | **Unit Tests** | `internal/...` & `pkg/...` | *No tags* (Runs by default) | None (mock/stub-only, no files/network/DB) | < 10ms per test |
| **Tier 2** | **In-Memory Integration** | `internal/...` & `test/` | `//go:build integration` | SQLite in-memory, local PKI generation, local pubsub | < 2s per suite |
| **Tier 3** | **Docker E2E** | `test/e2e/` | `//go:build e2e` | Docker containers (gateway + operator) | < 30s per suite |

---

## Test Harness

### CLI Test Commands

```bash
./g8e test unit        # Run Tier 1 (Unit) tests - no external dependencies
./g8e test integration # Run Tier 2 (In-Memory Integration) tests - SQLite in-memory, local PKI
./g8e test e2e         # Run Tier 3 (Docker E2E) tests - requires Docker
./g8e test scenario    # Run Tier 2 (Scenario) tests - requires running gateway
./g8e test coverage    # Run tests with coverage report
./g8e test lint        # Run linting and quality checks
./g8e emulator list    # List emulator scenarios
./g8e emulator run     # Run emulator scenarios against real Gateway/Operator
./g8e test chaos       # Generate realistic governance events for testing
./g8e test summary     # View chaos test summary from test vault
```

The CLI test commands map directly to the 3-tier test architecture:

- **`./g8e test unit`** - Runs unit tests without build tags. These tests use mocks/stubs and have no external dependencies (no files, network, or DB). Fast feedback loop for local development.

- **`./g8e test integration`** - Runs in-memory integration tests with the `integration` build tag. These tests use SQLite in-memory databases, local PKI generation, and local pubsub. No running gateway required.

- **`./g8e test e2e`** - Runs Docker-based E2E tests with the `e2e` build tag. These tests require Docker and use `docker-compose.yml` to spin up gateway and operator containers.

- **`./g8e test scenario`** - Runs scenario-specific integration tests with the `integration` build tag. These tests exercise end-to-end governance workflows across doctrine, consensus, and notary modes. Requires running gateway and authenticated CLI session.

- **`./g8e test coverage`** - Runs tests with coverage profiling and enforces a minimum coverage threshold (60%). Use PKG flag to test a specific package, VERBOSE for detailed output.

- **`./g8e test lint`** - Runs golangci-lint with modern Go best practices. This includes staticcheck, govet, and additional linters for bug prevention, security, and code quality.

- **`./g8e emulator list`** - Lists available emulator scenarios with their posture requirements and personas.

- **`./g8e emulator run`** - Runs emulator scenarios against a real Gateway/Operator. Impersonates arbitrary AI tools and agents, exercising the full protocol surface (MCP, A2A, A2A protobuf, and official governance envelopes with mock consensus and principal signing), then audits every result against the Operator's signed receipts.

- **`./g8e test chaos`** - Generates realistic governance events for testing. Creates a test vault with distributed event categories (70% Good Actor, 20% Prompt Injection, 10% MitM) to test governance pipeline behavior under various conditions.

- **`./g8e test summary`** - Views aggregated chaos test results from the test vault database. Queries the chaos_events table across all test runs in the test vault directory.

Validates the g8e Node and protocol enforcement (`GovernanceEnvelope`, 5-layer governance, Audit Vault). Tests cover pub/sub command dispatch, L1/L2/L3/L4/L5 verification, transaction replay protection, state root validation, and audit vault integrity.

### Scenario Tests

```bash
./g8e test scenario
./g8e test scenario --run forge_signature
```

Integration tests exercising end-to-end governance workflows across doctrine, consensus, and notary modes. Tests cover the 5-layer verification sequence (L1-L5), transaction replay protection, state root validation, and receipt verification. Requires the g8e Gateway to be running and authenticated CLI session. These tests use the `integration` build tag.

**Test Types**:
- **Table-driven scenarios** - JSON fixtures in `test/scenario/fixtures/` covering security gates (bad integrity, hash mismatch, replay, stale state root, L2/L3 validation) and finance workflows
- **Golden snapshots** - Deterministic receipt comparison excluding volatile fields (signature, timestamp, signer key). Golden files auto-create on missing and auto-update on mismatch
- **Property-based invariants** - Fuzz-style tests verifying core governance invariants (integrity + freshness + state + required-gates must all pass in order)
- **Concurrency tests** - Double-submit replay detection using goroutines to verify TOCTOU resistance
- **Negative controls** - Tests that intentionally flip expectations to prove the suite can detect failures
- **Receipt verification** - Separate axis testing cryptographic receipt validation (signature verification, field tampering detection)
- **Receipt persistence** - Database persistence verification for accepted transactions (receipts stored in `console_audit` collection), rejected transactions verify no persistence

### Docker E2E Tests

```bash
make test-docker
make test-gov
```

Docker-based E2E tests that spin up gateway and operator containers using docker-compose. These tests use the `e2e` build tag and require Docker to be installed and running.

**TestDockerGateway_Health** (`test/e2e/gateway_e2e_test.go`):
- Tests gateway HTTP health endpoint
- Verifies CA bundle discoverable over HTTP
- Checks HTTPS port reachability (no mTLS)
- Validates operator container connection

**TestDockerGateway_GovDemo** (`test/e2e/gateway_e2e_test.go`):
- Tests the gov demo compose configuration
- Same health checks as above but using gov demo compose file

### Emulator

```bash
./g8e emulator list
./g8e emulator run [scenario ...]
./g8e emulator audit
```

The emulator is a universal agent testing and auditing tool that impersonates arbitrary AI tools and agents against a **REAL** g8e Gateway and Operator. It serves as a protocol compliance verifier by exercising the full g8e surface while recording every exchange for detailed audit.

**Key Design Principle**: The ONLY fiction is the client identity. The Gateway and Operator are real infrastructure components. The emulator merely wears different "personas" to test how the system behaves when various AI tools interact with it.

**Architecture**:
- **client/** - Thin, faithful HTTP client with mTLS support and exchange recording
- **config/** - Runtime configuration (auth, URLs, ensemble size, L3 mode)
- **scenarios/** - Ordered registry of impersonation scripts (MCP, A2A, governance)
- **report/** - Generates machine-readable JSON and human-readable Markdown reports

**Protocol Coverage**:
- **MCP (Model Context Protocol)** - Tools, resources, and prompts
- **A2A (Agent-to-Agent)** - Skill invocations with JSON and protobuf payloads
- **Governance envelopes** - L2 consensus and L3 notary flows

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
For consensus/notary scenarios, the emulator uses mock cryptographic actors:
- **Ensemble**: Mock consensus agents that co-sign L2 envelopes
- **Principal**: Mock human notary for L3 signing (or drives real OOB approve flow)

This allows testing maximal governance envelopes without requiring actual distributed consensus infrastructure.

**Emulator Commands**:
- **`./g8e emulator list`** - Lists available scenarios with their posture requirements and personas
- **`./g8e emulator run`** - Runs scenarios against a real Gateway/Operator with configurable mTLS, public surface, L3 mode (mock|suspend), ensemble size, and phase filtering (doctrine|notary|all)
- **`./g8e emulator audit`** - Audits signed receipts from the Operator for a specific session

**Emulator Configuration**:
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

**Native Tool Registry Tests** (`test/native_tool_registry_integration_test.go`):
- `TestRegisterNativeTools` - Validates registration of all 27 native tools (db_discover_topology, db_query_validate, db_isolated_read, db_index_triage, log_stream_filter, sys_oom_detect, config_diff_mask, proc_metric_top, fs_disk_profile, proc_signal_safe, net_socket_audit, net_endpoint_ping, net_http_probe, sys_info, net_dns_resolve, tls_cert_inspect, sys_env_vars, fs_file_checksum, sys_service_status, sys_container_status, fs_disk_usage, sys_time_clock, proc_tree, git_ops, cloud_metadata, k8s_inspect, shell_execute, net_ssh_known_hosts, operator_deploy)
- `TestRegisterNativeTools_DuplicateRegistration` - Verifies duplicate registration fails
- `TestRegisterNativeTools_NilRegistry` - Tests panic on nil registry
- `TestRegisterNativeTools_ToolNameConsistency` - Validates naming convention (lowercase with underscores)
- `TestRegisterNativeTools_SchemaValidity` - Verifies all tool schemas are valid
- `TestRegisterNativeTools_PartialRegistration` - Tests partial registration failure

#### Docker E2E Tests (`test/e2e/`)

**Gateway E2E** (`test/e2e/gateway_e2e_test.go`):
- `TestDockerGateway_Health` - Tests Docker-based gateway health endpoints
- `TestDockerGateway_GovDemo` - Tests Docker-based gateway using gov demo compose

**E2E Harness** (`test/e2e/harness.go`):
- `DockerE2EFixture` - Manages Docker-based E2E test infrastructure
- `NewDockerE2EFixture` - Creates and starts Docker containers for testing
- `GetHealth` - Retrieves gateway health status
- `GetCABundle` - Retrieves CA bundle from gateway
- `CheckOperatorContainer` - Verifies operator container connection

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

#### Test Fixtures (`test/fixtures/`)

Reusable test infrastructure for integration and E2E tests:

**Gateway Fixture** (`test/fixtures/gateway_fixture.go`):
- `GatewayFixture` - Reusable gateway setup for integration tests with full lifecycle management
- `NewGatewayFixture` - Creates a fully configured gateway with downstream server, execution services, governance dependencies, and MCP gateway wiring
- `EnrollClientIdentity` - Performs CSR enrollment for test clients with certificate generation and operator session creation
- `CreateMTLSClient` - Creates HTTP client configured for mTLS using enrolled identity
- `WaitForReady` - Polls HTTP health endpoint until server accepts connections
- `SetPublicBaseURL` - Sets public base URL for MCP gateway (used for approval links)
- Supports configurable posture (notary, consensus, doctrine), custom downstream URLs, and test port zero allowance
- Handles path initialization, mock downstream MCP server, actuator key setup, and automatic cleanup

**Docker Operator Fixture** (`test/fixtures/docker_operator_fixture.go`):
- `DockerOperatorFixture` - Manages Docker-based operator containers for true multi-operator testing scenarios
- `NewDockerOperatorFixture` - Creates and starts a Docker operator with configurable image, hostname, gateway URL, network, and environment variables
- `Stop` - Stops the operator container
- `GetLogs` - Retrieves container logs for debugging
- `ExecCommand` - Executes commands inside the container
- `WaitForReady` - Waits for operator to be ready by checking logs for a readiness marker
- Supports auto-remove containers, custom Docker networks, and automatic image building from `Dockerfile.operator`
- Includes cleanup function for graceful container shutdown and removal

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
- `gateway_auth_bench_test.go` - Gateway authentication benchmark tests
- `gateway_auth_test.go` - Gateway authentication tests
- `gateway_db_test.go` - Gateway database tests
- `gateway_http_test.go` - Gateway HTTP handler tests
- `gateway_jwt_integration_test.go` - JWT integration tests
- `gateway_pubsub_test.go` - Gateway pub/sub tests
- `gateway_service_test.go` - Gateway service tests
- `governance_envelope_fuzz_test.go` - Fuzz testing for governance envelopes
- `governance_envelope_test.go` - Governance envelope validation tests
- `jwks_test.go` - JWKS endpoint tests
- `network_identity_test.go` - Network identity tests
- `operator_controller_test.go` - Operator controller tests
- `passkey_service_test.go` - WebAuthn passkey tests
- `pki_authority_test.go` - PKI authority tests
- `pki_controller_test.go` - PKI controller tests
- `registration_service_test.go` - Registration service tests
- `replay_store_service_test.go` - Replay store service tests
- `secret_manager_test.go` - Secret manager tests
- `state_root_service_test.go` - State root service tests
- `state_root_test.go` - State root management tests
- `user_service_test.go` - User service tests

#### Governance Service Tests (`internal/services/governance/`)

Five-layer governance pipeline tests:
- `actuator_pub_export_test.go` - L5 Actuator public export tests
- `eval_answer_test.go` - L1 Doctrine evaluation answer tests
- `eval_answer_integration_test.go` - L1 Doctrine evaluation answer integration tests
- `governance_test.go` - General governance tests
- `governance_integration_test.go` - Governance integration tests with `//go:build integration` tag
- `l1_doctrine_payload_test.go` - L1 Doctrine payload validation tests
- `l1_doctrine_test.go` - L1 Doctrine pattern matching tests
- `l2_consensus_test.go` - L2 Consensus signature verification tests
- `l3_notary_test.go` - L3 Notary human presence proof tests
- `l3_notary_integration_test.go` - L3 Notary integration tests with `//go:build integration` tag
- `l4_warden_test.go` - L4 Warden integrity tests
- `l5_actuator_test.go` - L5 Actuator execution tests
- `l5_actuator_integration_test.go` - L5 Actuator integration tests with `//go:build integration` tag
- `processor_test.go` - Governance processor tests
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
- `internal/services/g8eo_integration_test.go` - g8e Operator integration tests with `//go:build integration` tag
- `internal/services/g8eo_lifecycle_test.go` - g8e Operator lifecycle management tests
- `internal/services/pubsub/audit_service_test.go` - Pub/sub audit service tests
- `internal/services/pubsub/command_service_test.go` - Pub/sub command service tests
- `internal/services/pubsub/file_ops_service_test.go` - Pub/sub file operations service tests
- `internal/services/pubsub/g8eg_pubsub_client_test.go` - g8e Gateway pub/sub client tests
- `internal/services/pubsub/g8eg_pubsub_mock_test.go` - g8e Gateway pub/sub mock tests
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
- `internal/services/system/system_utils_test.go` - System utility tests with `//go:build !windows` tag

#### CLI Tests (`internal/cli/`)

CLI command and configuration tests:
- `api/client_test.go` - API client tests
- `auth/agent_enroll_test.go` - Agent enrollment tests with SPIFFE URI SAN generation
- `auth/client_test.go` - Auth client tests
- `auth/windows_crypto_test.go` - Windows crypto tests
- `cmd/approve_test.go` - Approve command tests
- `cmd/auth_test.go` - Auth command tests
- `cmd/chaos_test.go` - Chaos command tests
- `cmd/cmd_test.go` - General command tests
- `cmd/data_test.go` - Data command tests
- `cmd/emulator_test.go` - Emulator command tests
- `cmd/goose_test.go` - Goose command tests
- `cmd/main_test.go` - Main command tests
- `cmd/mcp_backup_test.go` - MCP backup command tests
- `cmd/mcp_test.go` - MCP command tests
- `cmd/platform_test.go` - Platform command tests
- `cmd/security_test.go` - Security command tests
- `cmd/test_paths_test.go` - Test path validation tests
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
- `internal/constants/document_ids_test.go` - Document ID constants tests
- `internal/constants/env_vars_test.go` - Environment variable constants tests
- `internal/constants/exit_codes_test.go` - Exit code constants tests
- `internal/constants/field_paths_test.go` - Field path constants tests
- `internal/constants/output_test.go` - Output constants tests
- `internal/constants/paths_test.go` - Path constants tests
- `internal/constants/pubsub_test.go` - Pub/sub constants tests

#### Contract Tests

- `internal/contracts/constants_enforcement_test.go` - Constants enforcement tests
- `internal/contracts/protocol_constants_test.go` - Protocol constants tests

#### Infrastructure Tests

- `internal/httpclient/errors_test.go` - HTTP client error tests
- `internal/httpclient/httpclient_test.go` - HTTP client tests
- `internal/marshaler/marshaler_test.go` - Marshaler tests
- `internal/response/writer_test.go` - Response writer tests
- `internal/response/writer_fuzz_test.go` - Response writer fuzz tests
- `internal/security/path_test.go` - Security path validation tests
- `internal/certs/embed_test.go` - Embedded certificates tests
- `internal/certs/fetch_test.go` - Certificate fetch tests
- `internal/testutil/pubsub_integration_test.go` - Pub/sub integration tests with `//go:build integration` tag
- `internal/testutil/pubsub_unit_test.go` - Pub/sub unit tests

#### Storage Test Infrastructure (`internal/services/storage/` and `internal/services/storage/storagetest/`)

Storage service tests and test-only audit storage implementations separated from production code to avoid import cycles:

- `execution_vault_test.go` - Execution vault tests
- `history_handler_test.go` - History handler tests
- `ledger_diffcontent_test.go` - Ledger diff content tests
- `ledger_diffstat_test.go` - Ledger diff stat tests
- `ledger_git_test.go` - Ledger Git integration tests
- `ledger_test.go` - Ledger tests
- `replay_store_test.go` - Replay store tests
- `storage_test_helpers_test.go` - Storage test helper tests
- `token_store_test.go` - Token store tests
- `vault_requirement_test.go` - Vault requirement tests

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

- `cmd/operator/actuator_pub_export_test.go` - Operator actuator export tests
- `cmd/operator/main_subprocess_test.go` - Operator subprocess tests
- `cmd/operator/main_test.go` - Operator main tests
- `cmd/operator/terminal_linux_test.go` - Linux terminal tests
- `internal/cmd/stream_ssh_test.go` - SSH stream tests
- `internal/cmd/stream_ssh_utils_test.go` - SSH stream utility tests
- `internal/cmd/stream_subprocess_test.go` - Subprocess stream tests
- `internal/cmd/stream_test.go` - General stream tests

#### MCP Service Tests (`internal/services/mcp/`)

MCP gateway and native tool integration tests:
- `audit_attribution_test.go` - Audit attribution tests
- `byo_client_e2e_test.go` - BYO client E2E tests
- `config_test.go` - MCP configuration tests
- `field_parser_test.go` - Field parser tests
- `gateway_integration_test.go` - Gateway integration tests with real envelope processing, GatewaySigned propagation, SSE streaming, circuit breaker, error code mapping, native tool execution, read_field tool with L3 validation, and real L3 verification
- `gateway_test.go` - Gateway service tests
- `mcp_endpoint_test.go` - MCP endpoint tests
- `native_handlers_test.go` - Native handler tests
- `native_tools_integration_test.go` - Native tools integration tests with real SQLite databases, audit vault persistence, log filtering with scrubbing, process metrics and signal safety, network auditing and probing, concurrency tests for TOCTOU resistance, property-based tests for safety invariants, and negative controls for intentional failures
- `registry_test.go` - Tool registry tests
- `run_shell_command_test.go` - Shell command execution tests
- `suspended_transaction_test.go` - Suspended transaction tests
- `sys_tools_test.go` - System tools tests
- `validation_test.go` - Validation tests

#### Package Tests

- `pkg/governance/types_test.go` - Governance types tests

---

## Workflow

```bash
# 1. Run unit tests (no gateway required)
./g8e test unit

# 2. Run in-memory integration tests (no gateway required)
./g8e test integration

# 3. For Docker E2E tests, ensure Docker is running
make test-docker
make test-gov

# 4. For scenario tests, start the Gateway
./g8e gw start

# 5. Authenticate (required for mTLS tests)
./g8e auth login

# 6. Run scenario tests
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
- **Path constants** - ALL filepath strings in test code MUST be defined as constants in `internal/constants/paths.go`. No filepath strings may be constructed dynamically or hardcoded inline, including relative paths like `"../../"`, `"./"`, `".g8e/"`, `"/pki/"`, etc. Dynamic path construction using `filepath.Join()` with string literals is prohibited. Tests must use `constants.Paths.Infra.*` constants for runtime state paths (e.g., `constants.Paths.Infra.PkiDir` for `.g8e/pki`). The only exception is when using `TestPaths` for isolated test environments - the base directory for TestPaths must come from a constant, and all path construction within TestPaths must use constants. This eliminates magic strings and improves maintainability and system robustness.
- **Typed error constants** - When testing error handling, check for any hand-trolled strings that should be properly typed errors (e.g., error reason strings, status codes, rejection reasons). Use typed constants from `internal/constants/` instead of hardcoded strings in assertions and error message checks.
- **Regression test markers** - When documenting known issues in regression tests (e.g., Phase0Regression tests), use standardized marker constants instead of hardcoded strings. See `internal/services/gateway/pki_authority_test.go` for examples:
  - `RegressionMarkerAfterFix` - indicates expected behavior after a fix is implemented
  - `RegressionMarkerBeforeFix` - indicates current (broken) behavior before a fix
  - `RegressionMarkerIssue` - identifies a specific issue being tracked (e.g., C1, C2, H2)

### Makefile Test Targets

- **`make test`** - Runs Tier 1 (Unit) and Tier 2 (In-Memory Integration) tests.
- **`make test-unit`** - Runs Tier 1 (Unit) tests without build tags. No external dependencies.
- **`make test-integration`** - Runs Tier 2 (In-Memory Integration) tests with `integration` build tag. Uses SQLite in-memory, local PKI, local pubsub.
- **`make test-docker`** - Runs Tier 3 (Docker E2E) tests with `e2e` build tag. Requires Docker.
- **`make test-gov`** - Runs Tier 3 (Gov Demo E2E) tests with `e2e` build tag. Requires Docker.
- **`make test-short`** - Runs short unit tests with race detection and 60s timeout.
- **`make test-coverage`** - Runs tests with coverage (enforces 65% threshold). Use PKG=./path/to/pkg for specific package, VERBOSE=true for verbose output.
- **`make test-shuffle`** - Runs all tests with randomized order.
- **`make test-gateway`** - Runs gateway-specific integration tests (A2A gateway, MCP gateway, MCP stdio).
- **`make test-mcp`** - Legacy target. Redirects to `make test-integration`.
- **`make test-a2a`** - Legacy target. Redirects to `make test-integration`.
- **`make test-universal-gateway`** - Legacy target. Redirects to `make test-integration`.
- **`make test-byo`** - Legacy target. Redirects to `make test-integration`.
- **`make test-native`** - Legacy target. Redirects to `make test-integration`.
- **`make test-scenario`** - Legacy target. Redirects to `make test-integration`.

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

- **`verify-proto`** - Runs proto generation and validates that generated files are in sync with protocol definitions.
- **`validate-doctrines`** - Validates doctrine JSON schema against the governance policy model.
- **`swagger-generate`** - Generates and validates Swagger/OpenAPI documentation from code annotations.
- **`lint`** - Runs golangci-lint with build tags for integration tests.
- **`vulncheck`** - Scans Go dependencies for known vulnerabilities using govulncheck.
- **`test-unit`** - Runs Tier 1 (Unit) tests without build tags.
- **`test-integration`** - Runs Tier 2 (In-Memory Integration) tests with the `integration` build tag.

The CI workflow does not run Tier 3 (Docker E2E) tests. Docker E2E tests require manual execution with Docker installed.
