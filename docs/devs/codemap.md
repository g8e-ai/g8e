# g8e Codemap

Structural reference. For protocol semantics see [docs/architecture/g8e.md](../architecture/g8e.md), for gateway architecture see [docs/architecture/gateway.md](../architecture/gateway.md), for Operator details see [docs/architecture/operator.md](../architecture/operator.md).

## Dependency Graph

```text
protocol/                          <-- canonical source of truth, no internal imports
    ^
    |
internal/                          <-- private implementation; imports protocol/
    ^       ^
    |       |
  cmd/    pkg/                     <-- entry points + public API; import internal/ + protocol/
    ^
    |
  test/                            <-- integration tests; import cmd/ + internal/ + protocol/
```

## Repository Root

```text
g8e/
├── cmd/                            # g8e Node entry points
├── internal/                       # Private implementation
├── pkg/                            # Public packages
├── protocol/                       # Protocol definitions (shared truth)
├── test/                           # Integration / E2E tests
├── docs/                           # Architecture and user docs
├── .github/                        # GitHub workflows and templates
├── Makefile                        # Build orchestration
├── go.mod / go.sum
├── buf.gen.yaml                    # Buf protobuf generation config
├── .golangci.yml                   # Linting configuration
├── .gitignore
├── VERSION
├── CHANGELOG.md
├── CODE_OF_CONDUCT.md
├── CONTRIBUTING.md
├── LICENSE
├── README.md
└── SECURITY.md
```

## protocol/

No imports from `internal/` or `cmd/`. Consumed downstream by all other packages.

```text
protocol/
├── proto/g8e/
│   ├── common/v1/common.proto      # GovernanceEnvelope, L1/L2/L3 metadata, Component enum, forbidden_patterns option
│   ├── operator/v1/operator.proto  # Command/result payloads, ActionReceipt, CommitmentAttestation, PKI, passkey, MCP/A2A
│   └── pubsub/v1/pubsub.proto     # PubSubMessage, PubSubEvent
├── constants/
│   ├── doctrine/                   # L1 doctrine registries
│   │   ├── doctrine_registry.json
│   │   ├── gitleaks_doctrine.json
│   │   ├── mcp_vectors_doctrine.json
│   │   └── owasp_crs_doctrine.json
│   ├── agents.json
│   ├── api_paths.json
│   ├── channels.json
│   ├── collections.json
│   ├── document_ids.json
│   ├── env_vars.json
│   ├── events.json
│   ├── field_paths.json
│   ├── headers.json
│   ├── intents.json
│   ├── kv_keys.json
│   ├── platform.json
│   ├── ports.json
│   ├── prompts.json
│   ├── pubsub.json
│   ├── senders.json
│   ├── status.json
│   └── timestamp.json
├── models/                         # Shared JSON schema models
│   ├── agents/
│   ├── agent_activity_metadata.json
│   ├── case.json
│   ├── conversation.json
│   ├── conversation_message.json
│   ├── investigation.json
│   ├── operator_document.json
│   ├── platform_settings.json
│   ├── reputation_commitment.json
│   ├── reputation_state.json
│   ├── security_constraints.json
│   ├── stake_resolution.json
│   ├── tool_results.json
│   ├── user.json
│   ├── user_settings.json
│   └── errors.py
├── docs/                           # Protocol documentation
│   └── reference/
│       └── api/
├── examples/                       # Protocol usage examples
│   ├── governance_envelope/
│   ├── mcp_server/
│   └── workload_identity/
├── python/                         # Python protocol bindings
│   ├── examples/
│   └── g8e/
│       └── models/
├── test-fixtures/
├── workload_identity.go
├── Makefile
└── go.mod
```

## cmd/

Each subdirectory produces one binary. All import `internal/` and `protocol/`.

```text
cmd/
└── operator/
    └── main.go                     # g8e Operator g8e Node (multi-mode: gateway, insecure, outbound)
```

## internal/

All private. Nothing here is importable by external modules.

```text
internal/
├── auditor/                        # Governance auditor
│   ├── client/
│   ├── config/
│   ├── report/
│   └── scenarios/
├── certs/                          # Embedded trust bundles, cert loading
├── chaos/                          # Chaos testing utilities
├── cli/                            # Platform CLI
│   ├── api/                        #   HTTP client for Operator communication
│   ├── auth/                       #   Authentication client
│   ├── cmd/                        #   Subcommands
│   ├── config/                     #   CLI configuration
│   ├── errors/                     #   CLI error handling
│   ├── jsonrpc/                    #   JSON-RPC client
│   ├── platform/                   #   Platform process management
│   └── stdio/                      #   Stdio handling
├── cmd/                            # Stream command handling (subprocess, SSH)
├── config/                         # Config loading and validation
├── constants/                      # Generated Go constants from protocol/constants/*.json
├── contracts/                      # Protocol contract tests (constants enforcement, docs drift)
├── docs/                           # Internal documentation
├── httpclient/                     # Outbound HTTP client
├── interfaces/                     # Shared interface definitions
├── marshaler/                      # GovernanceEnvelope marshal/unmarshal
├── models/                         # Internal data models (auth, commands, file_edit, fs, gateway, heartbeat, wire)
├── responder/                      # Response handling
├── security/                       # Ed25519 cryptographic operations
├── services/                       # Core service layer (see next section)
└── testutil/                       # Test utilities, governance mocks, proto helpers
```

## internal/services/

The core of the operator. `g8eo.go` orchestrates Outbound mode; `gateway/` orchestrates Gateway mode.

```text
services/
├── g8eo.go                         # Outbound mode orchestrator
│
├── governance/                     # L1-L5 verification sequence
│   ├── processor.go                #   EnvelopeProcessor interface
│   ├── l1_doctrine.go              #   L1: Technical Bedrock (Hard Gates)
│   ├── l2_consensus.go             #   L2: Consensus (Distributed Agreement)
│   ├── l3_notary.go                #   L3: Notary (Authorization)
│   ├── l4_warden.go                #   L4: Warden (Pre-dispatch Gate)
│   ├── l5_actuator.go              #   L5: Actuator (Execution Boundary)
│   └── mocks/                      #   Test mocks
│
├── gateway/                        # Gateway mode orchestrator (PDP)
│   ├── db/                         #   Database schema
│   │   └── schema.sql
│   ├── gateway_service.go          #   Top-level init, GovernanceDeps assembly
│   ├── gateway_db.go               #   SQLite canonical state
│   ├── gateway_auth.go             #   Authentication / authorization
│   ├── gateway_certs.go            #   PKIAuthority: cert issuance + revocation
│   ├── gateway_http.go             #   HTTP routing (mTLS, bootstrap, public)
│   ├── gateway_pubsub.go           #   In-memory PubSubBroker
│   ├── governance_envelope.go      #   HTTP envelope submission handler
│   ├── cli_l3_notary.go            #   CLIL3Notary: L3 via mTLS certs
│   ├── composite_l3_verifier.go    #   Delegates L3 to PasskeyService or CLIL3Notary
│   ├── passkey_service.go          #   WebAuthn/FIDO2 passkey ops
│   ├── registration_service.go     #   CSR-based enrollment
│   ├── secret_manager.go           #   Signing key storage
│   ├── peer_connection.go          #   Gateway peer connection handling
│   ├── user_service.go
│   ├── session_service.go
│   ├── app_enrollment_service.go
│   ├── admin_controller.go
│   ├── auth_controller.go
│   ├── db_controller.go
│   ├── invitation_service.go
│   ├── jwks.go
│   ├── jwt_native.go
│   ├── operator_controller.go
│   └── pki_controller.go
│
├── execution/                      # Command execution
│   ├── execution.go                #   Shell execution with concurrency control
│   ├── file_edit.go                #   File write/delete/create
│   ├── file_edit_unix.go
│   ├── fs_grep.go                  #   Filesystem search
│   ├── fs_list.go                  #   Directory listing
│   ├── fs_list_386.go
│   ├── fs_list_amd64.go
│   └── fs_list_arm64.go
│
├── storage/                        # Local-first audit architecture
│   ├── audit_vault.go              #   SQLite audit vault (receipts, events, sessions)
│   ├── ledger.go                   #   Git-backed file mutation ledger (go-git)
│   ├── local_store.go              #   Consolidated execution vault (encrypted)
│   ├── replay_store.go             #   Nonce replay protection
│   ├── history_handler.go          #   File history/diff/restore
│   └── commitment_ledger.go        #   Reputation Merkle commitments
│
├── pubsub/                         # Command dispatch + results streaming
│   ├── pubsub_commands.go          #   PubSubCommandService (implements EnvelopeProcessor)
│   ├── pubsub_results.go           #   Result streaming
│   ├── command_service.go          #   Typed command handlers
│   ├── file_ops_service.go         #   File operation handlers
│   ├── history_service.go          #   History/diff/restore handlers
│   ├── heartbeat_service.go
│   ├── port_service.go
│   ├── audit_service.go
│   ├── vault_writer.go             #   Vault persistence for executions + diffs
│   ├── publish_helpers.go          #   Execution ID extraction, result publishing
│   ├── results_publisher.go        #   ResultsPublisher interface
│   ├── tls_errors.go               #   TLS error classification
│   ├── l2_verifier.go              #   L2 signature verification
│   ├── protocol_helpers.go         #   Envelope helpers
│   ├── g8es_pubsub_client.go       #   Operator pub/sub client (outbound mode)
│   └── inprocess_client.go         #   In-process pub/sub client (gateway loopback)
│
├── mcp/                            # MCP/A2A protocol translation
│   ├── gateway.go                  #   JSON-RPC to GovernanceEnvelope
│   ├── field_parser.go             #   Field path parsing for suspended txns
│   ├── models.go                   #   SuspendedTransaction model
│   ├── native_handlers.go
│   └── native_tools.go
│
├── sovereignty/
│   └── boundary.go                 #   Sovereignty Boundary Plane: data scrubbing/rehydration
│
├── auth/
│   ├── bootstrap.go                #   Device-link token auth + bootstrap config
│   └── fingerprint.go
│
├── keystore/
│   ├── keystore.go                 #   Keystore interface
│   ├── backend_darwin.go           #   macOS Keychain
│   ├── backend_linux.go            #   Linux keyring
│   ├── backend_file_linux.go       #   Linux file fallback
│   └── backend_inmemory.go         #   In-memory (tests)
│
├── network/
│   ├── identity.go                 #   Network identity resolution
│   └── identity_test.go
│
├── sqliteutil/
│   ├── db.go                       #   Connection management
│   ├── migration.go                #   Schema migrations
│   ├── compress.go
│   ├── pruner.go
│   ├── validate.go
│   └── timestamp.go
│
├── vault/
│   ├── vault.go                    #   Vault operations
│   ├── vault_crypto.go             #   DEK management
│   └── vault_header.go             #   Vault header format
│
├── system/
│   ├── git.go                      #   Returns "embedded" (all git via go-git)
│   ├── path.go
│   ├── system_utils.go
│   └── utils.go
│
└── insecure_mcp/
    └── insecure_mcp_node_service.go    #   WebSocket node host
```

## pkg/

Public API. Importable by external consumers.

```text
pkg/
└── governance/
    ├── types.go                    # GovernanceEnvelope types and hash generation
    └── types_test.go
```

## test/

Integration and scenario-based tests.

```text
test/
├── a2a_gateway_test.go             # A2A gateway integration
├── a2a_real_operator_test.go       # Real Operator A2A tests
├── byo_client_test.go              # BYO client integration
├── integration_helper.go           # Integration test helpers
├── mcp_gateway_test.go             # MCP gateway tests
├── mcp_real_operator_test.go       # Real Operator MCP tests
├── mcp_stdio_test.go               # MCP stdio tests
├── native_real_operator_test.go    # Native real Operator tests
├── universal_gateway_integration_test.go # Universal gateway integration tests
└── scenario/
    ├── scenario.go                 # Scenario runner framework
    ├── scenario_test.go            # Scenario tests
    ├── concurrency_test.go         # Concurrency tests
    ├── envelope_builder.go         # Envelope construction helpers
    └── README.md                   # Scenario documentation
```

## Runtime Storage Layout

```text
.g8e/
├── pki/                            # Certificates, keys, g8eg-ca-bundle.pem
├── data/                           # SQLite databases (audit vault, gateway DB, local store)
├── ledger/                         # Git-backed ledger (go-git)
│   └── sessions/{id}/.git          # Session-scoped repos
├── secrets/                        # Signing keys, encrypted vault
└── logs/
```

## Build Targets

```makefile
make build                        # Build g8e for all platforms (linux, windows, darwin)
make build-linux                  # Build g8e for Linux (amd64, arm64, 386)
make build-windows                # Build g8e for Windows (amd64, arm64)
make build-darwin                 # Build g8e for Darwin (amd64, arm64)
make build-compressed             # Build g8e for all platforms with UPX compression
make build-linux-compressed       # Build g8e for Linux with UPX compression
make build-windows-compressed     # Build g8e for Windows with UPX compression
make build-darwin-compressed      # Build g8e for Darwin with UPX compression
make generate                     # Generate all protocol artifacts (proto)
make proto                        # Generate all Protobuf code (Go)
make proto-python                 # Generate Python Protobuf code
make proto-force                   # Force generate Protobuf code
make buf-install                  # Install Buf CLI locally
make protoc-install               # Install protoc compiler
make upx-install                  # Install UPX compressor
make test                         # Run all tests with race detection
make test-short                   # Run short tests with race detection
make test-coverage                # Run tests with coverage (enforces 60% threshold)
make test-shuffle                 # Run all tests with randomized order
make test-integration             # Run integration tests (requires platform running and auth login)
make test-scenario                # Run scenario integration tests (requires platform running)
make test-gateway                 # Run gateway tests
make test-mcp                     # Run MCP tests
make test-a2a                     # Run A2A tests
make test-universal-gateway       # Run universal gateway integration tests
make test-byo                     # Run BYO client tests (requires platform running and auth login)
make test-native                  # Run native real Operator tests
make lint                         # Run all linting and quality checks
make lint-no-embedded-newlines    # Check for compilation errors
make vulncheck                    # Run vulnerability check
make validate-doctrines           # Validate doctrine JSON schema
make ingest-doctrines             # Doctrine ingestion (deprecated)
make update-doctrines             # Update doctrine sources
make clean                        # Remove all build artifacts and runtime state
make clean-harness                # Clean up stale harness directories
make ci                           # Run full CI pipeline locally
make ci-platform                  # Run platform-only CI (operator, protocol, proto, docs)
```
