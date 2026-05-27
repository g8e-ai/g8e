# g8e Codemap

Structural reference. For protocol semantics see [protocol.md](protocol.md), for gateway architecture see [gateway.md](gateway.md), for operator details see [operator.md](operator.md).

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
├── cmd/                            # Binary entry points
├── internal/                       # Private implementation
├── pkg/                            # Public packages
├── protocol/                       # Protocol definitions (shared truth)
├── test/                           # Integration / E2E tests
├── docs/                           # Architecture and user docs
├── Makefile                        # Build orchestration
├── go.mod / go.sum
├── buf.gen.yaml                    # Buf protobuf generation config
└── VERSION
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
│   ├── doctrine/                   # L1 doctrine registries (gitleaks, OWASP CRS, MCP vectors)
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
│   ├── paths.json
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
├── test-fixtures/
├── workload_identity.go
├── Makefile
└── go.mod
```

## cmd/

Each subdirectory produces one binary. All import `internal/` and `protocol/`.

```text
cmd/
├── g8e/main.go                     # Platform CLI (delegates to internal/cli/cmd)
├── g8eo/main.go                    # Operator binary (multi-mode: gateway, mcp-serve, insecure, outbound)
├── chaos_tester/main.go            # Chaos / fuzz testing harness
└── uap-ping/main.go                # UAP protocol ping utility
```

## internal/

All private. Nothing here is importable by external modules.

```text
internal/
├── certs/                          # Embedded trust bundles, cert loading
├── cli/                            # Platform CLI (cmd/g8e delegates here)
│   ├── api/                        #   HTTP client for operator communication
│   ├── auth/                       #   Authentication client
│   ├── cmd/                        #   Subcommands: platform, auth, data, security, setup, test, vars
│   ├── config/                     #   CLI configuration
│   └── platform/                   #   Platform process management
├── cmd/                            # Stream command handling (subprocess, SSH)
├── config/                         # Config loading and validation
├── constants/                      # Generated Go constants from protocol/constants/*.json
├── contracts/                      # Protocol contract tests (constants enforcement, docs drift)
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
├── governance/                     # L1-L5 verification gauntlet
│   ├── processor.go                #   EnvelopeProcessor interface
│   ├── l1_doctrine.go              #   L1: forbidden patterns, threat analysis
│   ├── l2_consensus.go             #   L2: Ed25519 consensus signature verification
│   ├── l3_notary.go                #   L3: L3Notary interface + outboundL3Notary (CLI approval)
│   ├── l4_warden.go                #   L4: fail-closed verification gate
│   ├── l5_actuator.go              #   L5: execution boundary, receipt signer
│   └── mocks/                      #   Test mocks
│
├── gateway/                        # Gateway mode orchestrator
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
│   ├── registration_service.go     #   Device-link enrollment
│   ├── secret_manager.go           #   Signing key storage
│   ├── trust_scripts.go            #   PKI trust bootstrap
│   ├── user_service.go
│   ├── session_service.go
│   ├── app_enrollment_service.go
│   └── pki/                        #   PKI dir structure (authorities, issued, revocation, root, trust)
│
├── execution/                      # Command execution
│   ├── execution.go                #   Shell execution with concurrency control
│   ├── file_edit.go                #   File write/delete/create
│   ├── file_edit_unix.go
│   ├── fs_grep.go                  #   Filesystem search
│   ├── fs_list.go                  #   Directory listing
│   └── fs_list_unix.go
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
│   ├── g8es_pubsub_client.go       #   OperatorPubSubClient (outbound mode)
│   └── inprocess_client.go         #   InProcessPubSubClient (gateway loopback)
│
├── mcp/                            # MCP/A2A protocol translation
│   ├── gateway.go                  #   JSON-RPC to GovernanceEnvelope
│   ├── field_parser.go             #   Field path parsing for suspended txns
│   └── models.go                   #   SuspendedTransaction model
│
├── sovereignty/
│   └── boundary.go                 #   Secret detection, PII redaction, rehydration
│
├── auth/
│   ├── bootstrap.go                #   Device-link token auth + bootstrap config
│   ├── device_auth.go
│   └── fingerprint.go
│
├── keystore/
│   ├── keystore.go                 #   Keystore interface
│   ├── backend_darwin.go           #   macOS Keychain
│   ├── backend_linux.go            #   Linux keyring
│   ├── backend_file_linux.go       #   Linux file fallback
│   └── backend_inmemory.go         #   In-memory (tests)
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
└── uap/
    ├── types.go                    # Universal Access Protocol types
    └── types_test.go
```

## test/

Integration and scenario-based tests.

```text
test/
├── a2a_gateway_test.go             # A2A gateway integration
├── a2a_real_operator_test.go       # Real operator A2A tests
├── byo_client_test.go              # BYO client integration
├── mcp_gateway_test.go             # MCP gateway tests
├── mcp_real_operator_test.go       # Real operator MCP tests
└── scenario/
    ├── scenario.go                 # Scenario runner framework
    ├── runner.go
    ├── db_setup.go
    ├── generate_fixtures.go
    ├── generate_gate_fixtures.go
    ├── generate_l1_pattern.go
    ├── report.go
    ├── scenario_test.go
    ├── concurrency_test.go
    ├── fuzz_test.go
    ├── receipt_verification_test.go
    ├── fixtures/                   # Test fixture data
    └── golden/                     # Golden file outputs
```

## Runtime Storage Layout

```text
.g8e/
├── pki/                            # Certificates, keys, hub-bundle.pem
├── data/                           # SQLite databases (audit vault, gateway DB, local store)
├── ledger/                         # Git-backed ledger (go-git)
│   └── sessions/{id}/.git          # Session-scoped repos
├── secrets/                        # Signing keys, encrypted vault
└── logs/
```

## Build Targets

```makefile
make build              # bin/g8e operator binary
make generate           # proto + constants generation
make proto              # Buf protobuf codegen
make constants          # Go constants from JSON
make test-g8eo          # All tests (race detection)
make lint-g8eo          # golangci-lint
make vulncheck-g8eo     # govulncheck
make ci                 # Full CI pipeline
make clean              # Remove artifacts + runtime state
make docs-build         # MkDocs site
make docs-cli           # CLI reference generation
make validate-doctrines # Doctrine JSON schema check
```
