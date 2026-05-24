# g8eo (Operator) Internal Structure Codemap

## Overview

g8eo is the Go-based Governed Operator - the sovereign execution boundary and protocol substrate. It operates in multiple modes: outbound operator mode (executes mutations on local host), gateway mode (platform persistence and messaging backbone), MCP serve mode (protocol translation gateway), and OpenClaw node host mode. All modes enforce the L1/L2/L3 governance gauntlet and maintain local-first audit architecture.

```text
services/g8eo/
├── cmd/                         # Binary entry points
│   ├── g8eo/                    # Main Operator binary (multi-mode)
│   ├── chaos_tester/            # Chaos testing tool
│   ├── exporter/                # Audit export tool
│   └── uap-ping/                # UAP protocol ping utility
│
├── internal/                    # Private implementation (not exported)
│   ├── certs/                   # Certificate management and trust bundle loading
│   ├── cli/                     # Platform CLI subcommands
│   │   ├── api/                 # API client for Operator communication
│   │   ├── auth/                # Authentication client
│   │   ├── cmd/                 # CLI command handlers (platform, apps, auth, data, test, evals, security, setup, vars)
│   │   ├── config/              # CLI configuration
│   │   └── platform/            # Platform process management
│   ├── cmd/                     # Stream command handling (subprocess, SSH)
│   ├── config/                  # Configuration loading and validation
│   ├── constants/               # Operator-specific constants (agents, API paths, auth, events)
│   ├── contracts/               # Protocol contract tests
│   ├── httpclient/              # HTTP client for outbound connections
│   ├── interfaces/              # Interface definitions
│   ├── marshaler/               # Envelope marshaling/unmarshaling
│   ├── models/                  # Operator-specific data models
│   ├── protocol/                # Protocol integration layer
│   │   └── proto/               # Generated protobuf code (from protocol/proto/)
│   ├── responder/               # Response handling
│   ├── security/                # Cryptographic operations (Ed25519)
│   ├── services/                # Core service layer
│   │   ├── auth/                # Bootstrap service for device-link enrollment
│   │   ├── execution/           # Command execution, file edit, fs operations
│   │   ├── gateway/             # Gateway mode: platform persistence, PKI, auth, pub/sub broker
│   │   ├── governance/          # L1/L2/L3 verification (TransactionVerifier, Tribunal, Actuator)
│   │   ├── keystore/            # Platform-specific key storage (Darwin Keychain, Linux file backend)
│   │   ├── mcp/                 # MCP gateway for protocol translation (MCP/A2A)
│   │   ├── openclaw/            # OpenClaw node host service
│   │   ├── pubsub/              # Pub/sub command channel, results streaming, loopback
│   │   ├── sentinel/            # PII/secret scrubbing and output projection
│   │   ├── sqliteutil/          # SQLite utilities and migrations
│   │   ├── storage/             # Audit vault (SQLite+Git), ledger, local store, replay store
│   │   ├── system/              # System operations (git resolution, port checking)
│   │   └── vault/               # Vault operations (encryption, DEK management)
│   └── testutil/                # Test utilities and fixtures
│
├── pkg/                         # Public packages
│   └── uap/                     # Universal Access Protocol utilities
│
├── tests/                       # Integration and end-to-end tests
│   ├── byo_client_test.go       # BYO client integration tests
│   ├── mcp_gateway_test.go      # MCP gateway tests
│   └── mcp_real_operator_test.go # Real Operator MCP tests
│
├── tools/                       # Vendored build tools and dependencies
├── Makefile                     # Build targets
└── go.mod                       # Go module definition
```

## Core Service Layer Breakdown

### `services/governance/`
- **Purpose**: Implement the L1/L2/L3 verification gauntlet
- **Key Components**:
  - `transaction_verifier.go` - TransactionVerifier: envelope integrity, hash binding, expiry, nonce/replay, state root, L1/L2/L3 verification
  - `tribunal.go` - Tribunal: L2 consensus signature generation and verification
  - `actuator.go` - Actuator: L3 authorization and signed receipt generation
  - `processor.go` - EnvelopeProcessor interface for transaction gate
- **Critical Path**: Every mutation MUST pass through TransactionVerifier before execution
- **Verification Layers**:
  - L1 (Doctrine): forbidden patterns, whitelist, blacklist via protobuf field options
  - L2 (Consensus): Ed25519 tribunal signature verification
  - L3 (Notary): WebAuthn/FIDO2 human approval (posture-dependent)

### `services/execution/`
- **Purpose**: Command execution and file operations
- **Key Components**:
  - `execution.go` - ExecutionService: shell command execution with concurrency control
  - `file_edit.go` - FileEditService: file write, delete, create operations
  - `fs_grep.go` - FsGrepService: filesystem search
  - `fs_list.go` - FsListService: filesystem listing
- **Invariant**: Only verified transactions reach execution layer

### `services/storage/`
- **Purpose**: Local-first audit architecture (LFAA)
- **Key Components**:
  - `audit_vault.go` - AuditVaultService: SQLite-based audit event storage with session validation (fail-closed)
  - `ledger.go` - LedgerService: Git-backed commit history and diff tracking (go-git native)
  - `local_store.go` - LocalStoreService: consolidated execution vault with encryption
  - `replay_store.go` - ReplayStore: nonce replay protection interface
  - `history_handler.go` - HistoryHandler: file history/diff/restore operations
  - `commitment_ledger.go` - CommitmentLedger: reputation Merkle commitments
- **Storage**: `.g8e/data/` (SQLite) + `.g8e/ledger/.git` (Git)
- **Fail-closed**: Audit vault rejects events without valid operator_session_id or unknown sessions

### `services/auth/`
- **Purpose**: Bootstrap and device-link enrollment
- **Key Components**:
  - `bootstrap.go` - BootstrapService: device-link token authentication and bootstrap config application
- **Note**: Full auth lifecycle (users, sessions, passkeys, PKI) lives in gateway mode

### `services/gateway/`
- **Purpose**: Gateway mode - platform persistence, PKI authority, and messaging backbone
- **Key Components**:
  - `gateway_service.go` - GatewayService: top-level orchestrator for gateway mode
  - `gateway_db.go` - GatewayDBService: SQLite database for canonical state
  - `gateway_auth.go` - AuthService: authentication and authorization
  - `gateway_certs.go` - PKIAuthority: certificate issuance and revocation
  - `registration_service.go` - RegistrationService: device-link enrollment
  - `passkey_service.go` - PasskeyService: WebAuthn/FIDO2 passkey operations
  - `gateway_http.go` - HTTPHandler: HTTP routing and middleware
  - `gateway_pubsub.go` - PubSubBroker: in-memory pub/sub messaging
  - `secret_manager.go` - SecretManager: signing key storage (Actuator, Tribunal)
  - `user_service.go` - UserService: user management
  - `session_service.go` - SessionService: session management
  - `api_key_service.go` - APIKeyService: API key management
  - `app_enrollment_service.go` - AppEnrollmentService: application enrollment
- **Mode**: Gateway mode serves inbound requests; does NOT execute commands or initiate outbound connections

### `services/mcp/`
- **Purpose**: MCP/A2A protocol translation gateway
- **Key Components**:
  - `gateway.go` - GatewayService: MCP JSON-RPC to GovernanceEnvelope translation
  - `field_parser.go` - FieldPathRegistry: field path parsing for suspended transactions
  - `models.go` - SuspendedTransaction model
- **Flow**: MCP tool calls → GovernanceEnvelope → governance verification → Actuator execution → downstream MCP/A2A dispatch
- **Wire Format**: Canonical JSON (protojson) for UniversalEnvelope on all client-facing surfaces

### `services/pubsub/`
- **Purpose**: Pub/sub command channel and results streaming
- **Key Components**:
  - `pubsub_commands.go` - PubSubCommandService: command envelope dispatch with governance integration
  - `pubsub_results.go` - PubSubResultsService: result streaming to clients
  - `command_service.go` - CommandService: typed command handlers
  - `file_ops_service.go` - FileOpsService: file operation handlers
  - `history_service.go` - HistoryService: history/diff/restore handlers
  - `heartbeat_service.go` - HeartbeatService: operator heartbeat
  - `port_service.go` - PortService: port availability checks
  - `audit_service.go` - AuditService: audit event publishing
- **Loopback**: InProcessPubSubClient for in-process command dispatch in gateway mode

### `services/sentinel/`
- **Purpose**: PII and secret scrubbing before output exposure
- **Key Components**:
  - `sentinel.go` - Sentinel: pattern-based secret detection, PII redaction, output projection
  - `sentinel_input.go` - Input validation and sanitization
- **Invariant**: Raw data never crosses the trust boundary without scrubbing

### `services/system/`
- **Purpose**: System-level operations
- **Key Components**:
  - Git operations via go-git (native Go implementation, not shell exec)
  - Port availability checking
  - Filesystem operations
- **Note**: Git binary resolution returns "embedded" stub; all git operations use go-git library

## Protocol Integration Layer

### `internal/protocol/`
- **Purpose**: Bridge between g8eo and protocol definitions
- **Components**:
  - `proto/` - Generated protobuf code from `protocol/proto/` (commonv1, operatorv1, pubsubv1)
- **Source**: All protobuf schemas come from `protocol/proto/` (canonical protocol substrate)
- **Wire Format**: Canonical JSON (protojson) for UniversalEnvelope on all client-facing surfaces (HTTP APIs, pub/sub, receipts, audit exports)
- **Signing Basis**: Deterministic transaction_hash computed from normalized envelope fields; wire encoding is irrelevant since verifier enforces id == computed hash

## Configuration

### `internal/config/`
- **Purpose**: Configuration loading and validation
- **Sources**:
  - Command-line flags (`--pki-dir`, `--data-dir`, `--secrets-dir`, `--doctrine`/`--consensus`/`--notary`, `--mcp-serve`, `--openclaw`)
  - Environment variables (`G8E_OPERATOR_API_KEY`, `G8E_PKI_DIR`, `G8E_RUNTIME_DIR`, `OPENCLAW_GATEWAY_TOKEN`)
  - Settings file via `config.LoadSettings()`
- **Validation**: Strict validation on startup; fail fast on misconfiguration
- **Postures**: Gateway mode supports three postures (doctrine, consensus, notary) with different L1/L2/L3 enforcement levels

## Entry Points

### Main Operator Binary
- **Path**: `cmd/g8eo/main.go`
- **Modes**:
  - **Outbound Operator mode** (default): Executes mutations on local host, initiates mTLS connections to platform
  - **Gateway mode** (`--doctrine`/`--consensus`/`--notary`): Platform persistence and messaging backbone, serves inbound requests
  - **MCP serve mode** (`--mcp-serve`): Protocol translation gateway over stdio (JSON-RPC)
  - **OpenClaw mode** (`--openclaw`): Connect to OpenClaw Gateway as node host
  - **CLI subcommands**: `platform`, `apps`, `auth`, `data`, `test`, `evals`, `security`, `setup`, `vars`
  - **Vault management**: `--rekey-vault`, `--verify-vault`, `--reset-vault`
  - **Stream mode**: `stream` subprocess for approval UI
- **Output**: `build/linux-amd64/g8e` (binary name is `g8e`, not `g8e.operator`)

### Supporting Tools
- **chaos_tester**: Chaos and fault injection testing
- **exporter**: Audit vault export and reporting
- **uap-ping**: UAP protocol connectivity testing

## Testing Structure

### Unit Tests
- Located alongside source files (`*_test.go`)
- Focus on individual service logic
- Use `internal/testutil/` for fixtures
- Key test suites: governance (transaction_verifier, tribunal, actuator), execution, storage (audit_vault, ledger), gateway, pubsub, sentinel

### Integration Tests
- **Path**: `tests/`
- **BYO client tests**: Verify external client integration
- **MCP gateway tests**: Verify MCP server behavior
- **Real Operator tests**: End-to-end with real binary

## Critical Data Paths

### Outbound Operator Mode Mutation Flow
```text
Platform (mTLS) → PubSubCommandService → TransactionVerifier (L1/L2/L3)
→ ExecutionService (Actuator) → AuditVault (SQLite) → Ledger (Git commit)
→ PubSubResultsService (result streaming)
```

### Gateway Mode Mutation Flow
```text
HTTP POST /api/governance/envelope → Gateway HTTP handler → EnvelopeProcessor
→ TransactionVerifier (L1/L2/L3) → ExecutionService (Actuator)
→ AuditVault → Ledger → Signed ActionReceipt response
```

### MCP Tool Call Flow
```text
MCP Client (JSON-RPC) → MCP Gateway → GovernanceEnvelope formation
→ TransactionVerifier → ExecutionService → MCP Gateway (egress dispatch)
→ Downstream MCP/A2A server → Result streaming → MCP Client
```

### Audit Write Flow
```text
Any service → AuditVaultService → SQLite (session validation required)
→ LedgerService → Git commit (go-git native) → Tamper-evident history
```

## Storage Layout

```text
.g8e/
├── pki/                         # PKI material (certificates, keys, hub-bundle.pem)
├── data/                        # SQLite databases (audit vault, gateway DB, local store)
├── ledger/                      # Git-backed ledger
│   └── .git/                    # Git repository (go-git native)
├── secrets/                     # Platform secrets (signing keys, encrypted vault)
└── logs/                        # Operator logs
    └── operator-listen.log
```

## Build Targets

```makefile
make build              # Build linux/amd64 binary
make build-all          # Build all architectures (amd64 arm64 386)
make test               # Run tests
make test-sudo          # Run tests with sudo (for privileged operations)
make fmt                # Format code
make vet                # Run go vet
make lint               # Run golangci-lint
make vulncheck          # Run govulncheck
make check              # Run fmt, vet, lint, vulncheck, test
make clean              # Clean build artifacts
make clean-go-cache     # Clean Go build cache
make clean-deep         # Deep clean including all caches
make deps               # Update dependencies
make version            # Show version info
```

## Key Invariants

1. **Fail-closed execution**: Only verified transactions reach execution layer; TransactionVerifier rejects invalid envelopes before execution
2. **Protocol substrate**: All protobuf schemas from `protocol/proto/` (canonical protocol source of truth)
3. **Local-first audit**: Audit vault and ledger are host-local only; session validation required for audit writes
4. **Wire format**: Canonical JSON (protojson) for UniversalEnvelope on all client-facing surfaces; binary protobuf only for internal storage
5. **Git-native**: Ledger uses go-git library (native Go implementation), not shell git commands
6. **Sentinel scrubbing**: All output passes through Sentinel before crossing trust boundary
7. **Replay protection**: Nonce validation via ReplayStore prevents transaction replay
8. **State binding**: State root verification ensures transactions bind to current system state
9. **Multi-mode architecture**: Single binary supports outbound operator, gateway, MCP serve, and OpenClaw modes
