# g8e Codemap

## Overview

This codemap provides a comprehensive technical overview of g8e, covering the protocol substrate, internal structure, operational modes, and complete request lifecycle. g8e is a Go-based Governed Operator that serves as the sovereign execution boundary and protocol substrate.

The Operator runs in multiple modes:
- **Outbound mode**: Traditional operator with pub/sub subscription to cloud platform, executes commands locally
- **Gateway mode**: Platform persistence and messaging backbone, serves inbound requests via HTTP/WebSocket, delegates execution to downstream MCP/A2A servers
- **MCP serve mode**: Protocol translation gateway over stdio (JSON-RPC)
- **OpenClaw mode**: Connects to OpenClaw Gateway as node host

All modes share the same fail-closed verification gauntlet and Actuator execution boundary.

## High-Level Architecture

### Component Structure

```text
g8e/
├── protocol/                    # MANDATORY - Protocol definitions (shared truth)
│   ├── proto/                   # Protobuf schemas (wire format source of truth)
│   ├── constants/               # Governance constants, doctrine, collections
│   ├── models/                  # Shared data models (agents, audit, etc.)
│   └── workload_identity.go     # Go workload identity implementation
│
├── cmd/                         # Binary entry points
│   ├── g8eo/                    # Main Operator binary (multi-mode)
│   ├── exporter/                # Audit export tool
│   ├── chaos_tester/            # Chaos testing tool
│   └── uap-ping/                # UAP protocol ping utility
│
├── internal/                    # Private implementation (not exported)
│   ├── services/                # Core service layer
│   │   ├── gateway/             # Gateway mode: platform persistence, PKI, auth, pub/sub
│   │   ├── governance/          # L1-L5 verification (L1Doctrine, L2Consensus, L3Notary, L4Warden, L5Actuator)
│   │   ├── execution/           # Command execution, file edit, fs operations
│   │   ├── storage/             # Audit vault (SQLite+Git), ledger, local store, replay store
│   │   ├── pubsub/              # Pub/sub command channel, results streaming, loopback
│   │   ├── mcp/                 # MCP/A2A protocol translation gateway
│   │   ├── sovereignty/         # Sovereignty Boundary Plane: data scrubbing, rehydration, token persistence
│   │   ├── auth/                # Bootstrap and device-link enrollment
│   │   ├── openclaw/            # OpenClaw node host service
│   │   └── vault/               # Vault operations (encryption, DEK management)
│   ├── config/                  # Configuration loading and validation
│   ├── constants/               # Operator-specific constants (agents, API paths, events)
│   ├── cli/                     # Platform CLI subcommands
│   └── models/                  # Operator-specific data models
│
├── pkg/                         # Public packages
│   └── uap/                     # Universal Access Protocol utilities
│
├── test/                        # Integration and end-to-end tests
└── docs/                        # Architecture and user documentation
```

### Component Responsibilities

#### Protocol Layer (`protocol/`)
- **Purpose**: Single source of truth for all protocol definitions
- **Contents**:
  - `proto/` - Protobuf schemas for GovernanceEnvelope, receipts, audit events
  - `constants/` - Doctrine patterns, L1/L2/L3 validation rules, collection schemas, ports, API paths
  - `models/` - Shared data models (agents, operator metadata, audit structures)
  - `workload_identity.go` - Go workload identity implementation
- **Invariant**: All services MUST consume protocol definitions from this layer. No local schema duplication.

#### Governed Operator (g8eo)
- **Purpose**: Single binary that operates in multiple modes based on command-line flags
- **Language**: Go
- **Entry Point**: `cmd/g8eo/main.go` → `g8e` binary
- **Output**: `bin/g8e` (binary name is `g8e`, not `g8e.operator`)

##### Operational Modes

**Gateway Mode** (platform persistence + pub/sub broker):
- Flags: `--doctrine`, `--consensus`, or `--notary`
- Responsibilities:
  - Admission APIs for envelope submission (POST /api/governance/envelope)
  - mTLS/PKI management and device-link lifecycle
  - Replay protection and session scoping
  - State-root distribution
  - SQLite persistence for platform state (GatewayDBService)
  - In-process command service as sovereign execution gateway
  - In-process PubSubBroker for messaging
  - PKI authority for certificate issuance and revocation

**MCP Serve Mode** (BYO client proxy):
- Flag: `--mcp-serve`
- Responsibilities:
  - MCP stdio JSON-RPC proxy to Operator's mTLS HTTP API
  - Enables standard MCP clients to interact with g8e
  - Protocol translation: MCP tool calls → GovernanceEnvelope

**OpenClaw Node Host Mode**:
- Flag: `--openclaw`
- Responsibilities:
  - Connects to OpenClaw Gateway via WebSocket
  - Advertises system.run and system.which capabilities
  - Executes shell commands on demand
  - No g8e client infrastructure required

**Standard Operator Mode** (default):
- No special flags required
- Responsibilities:
  - Enforce L1/L2/L3 governance gauntlet
  - Execute mutations through fail-closed L5Actuator
  - Maintain local audit vault (SQLite + Git-backed ledger)
  - Outbound mTLS connection to Gateway
  - Device-link enrollment via bootstrap service

**CLI Subcommands**:
- Commands: `platform`, `apps`, `auth`, `data`, `test`, `evals`, `security`, `setup`, `vars`
- Vault management: `--rekey-vault`, `--verify-vault`, `--reset-vault`
- Stream mode: `stream` subprocess for approval UI

### Dependency Flow

```text
┌─────────────────────────────────────────────────────────────┐
│                        PROTOCOL LAYER                        │
│  (proto, constants, models - consumed by g8e binary)       │
└─────────────────────────────────────────────────────────────┘
                              │
                              ▼
                    ┌─────────────────┐
                    │   g8e binary     │
                    │  (multi-mode)    │
                    │  cmd/g8eo/main.go│
                    └─────────────────┘
                              │
              ┌───────────────┼───────────────┐
              ▼               ▼               ▼
        ┌──────────┐   ┌──────────┐   ┌──────────┐
        │Gateway   │   │   MCP    │   │OpenClaw  │
        │Mode      │   │  Serve   │   │  Node    │
        │(PDP+Exec)│   │  Proxy   │   │  Host    │
        └──────────┘   └──────────┘   └──────────┘
              │               │               │
              └───────────────┴───────────────┘
                              │
                    BYO clients interact via
                    selected operational mode
```

## Protocol Layer

### Protocol Structure

```text
protocol/
├── proto/                       # Protobuf schemas (wire format source of truth)
│   ├── buf.yaml                 # Buf module configuration
│   └── g8e/                     # Protobuf package hierarchy
│       ├── common/v1/
│       │   └── common.proto     # Common message definitions
│       ├── operator/v1/
│       │   └── operator.proto   # Operator-specific messages
│       └── pubsub/v1/
│           └── pubsub.proto     # Pub/sub message definitions
│
├── constants/                   # Governance constants and rules
│   ├── doctrine/                # L1 Doctrine definitions
│   │   ├── doctrine_registry.json       # Doctrine registry
│   │   ├── gitleaks_doctrine.json       # Secret detection patterns
│   │   ├── mcp_vectors_doctrine.json    # MCP threat patterns
│   │   └── owasp_crs_doctrine.json     # OWASP security patterns
│   ├── agents.json              # Agent constant mappings
│   ├── api_paths.json           # API path mappings
│   ├── channels.json            # Pub/sub channel definitions
│   ├── collections.json        # Canonical collection schemas
│   ├── document_ids.json        # Document ID constants
│   ├── env_vars.json            # Environment variable names
│   ├── events.json              # Event type definitions
│   ├── field_paths.json         # JSON field path constants
│   ├── headers.json             # HTTP header constants
│   ├── intents.json             # Intent definitions
│   ├── kv_keys.json             # Key-value store key constants
│   ├── paths.json               # Path mappings
│   ├── platform.json            # Platform configuration
│   ├── ports.json               # Port assignments
│   ├── prompts.json             # Prompt templates
│   ├── pubsub.json              # Pub/sub configuration
│   ├── senders.json             # Sender identifiers
│   ├── status.json              # Status codes
│   └── timestamp.json          # Timestamp format constants
│
├── models/                      # Shared data models (JSON schemas)
│   ├── agents/                  # Agent configuration models
│   ├── agent_activity_metadata.json
│   ├── auditor_commands.json
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
│   ├── consensus.json
│   ├── user.json
│   ├── user_settings.json
│   └── errors.py                # Python error definitions
│
├── test-fixtures/               # Protocol test fixtures
│   ├── gold-set-schema.json
│   ├── ledger-hash-fixtures.json
│   ├── sse-events-schema.json
│   └── sse-events.json
│
├── Makefile                     # Protocol Go lint/test targets
├── go.mod                       # Go module for protocol tools
└── workload_identity.go         # Workload identity utilities
```

### Protobuf Schemas

#### `proto/g8e/common/v1/common.proto`
- **Purpose**: Common message definitions used across all services
- **Key Messages**:
  - `GovernanceEnvelope` - The canonical mutation envelope binding identity, intent, state, and governance proofs
  - `GovernanceMetadata` - Unified L1/L2/L3 governance proofs
  - `L1Metadata` - Doctrine (L1) validation status with violations list
  - `L2Metadata` - Consensus (L2) signature with agent IDs and key ID
  - `L3Proof` - Notary (L3) authorization proof supporting WebAuthn or mTLS
  - `L3Metadata` - Notary (L3) authorization metadata with proof and auto_approved flag
  - `Component` - Source component identifier enum (G8EO, CLIENT)
- **Wire Format**: protojson (canonical JSON)
- **Signing Basis**: Deterministic transaction_hash from normalized envelope fields
- **Custom Options**: `forbidden_patterns` field option (extension 50001) for L1 reflection on payload fields

#### `proto/g8e/operator/v1/operator.proto`
- **Purpose**: Operator-specific message definitions for command execution, MCP/A2A protocol translation, PKI management, device-link flows, audit trails, and WebAuthn brokerage
- **Key Message Categories**:
  - **Command Payloads**: `CommandRequested`, `CommandCancelRequested`, `FileEditRequested`, `FsListRequested`, `FsReadRequested`, `FsGrepRequested`
  - **MCP/A2A Protocol**: `McpCallRequested`, `A2aCallRequested`, `McpResourceListRequested`, `McpResourceReadRequested`, `McpPromptListRequested`, `McpPromptGetRequested`
  - **Result Payloads**: `CommandResult`, `FsListResult`, `FsReadResult`, `FsGrepResult`, `FileEditResult`, `PortCheckResult`, `FetchLogsResult`
  - **PKI/Certificate**: `SignCertificateRequested`, `SignCertificateResult`, `RevokeCertificateRequested`, `RevokeCertificateResult`, `GetRevocationBundleRequested`, `GetRevocationBundleResult`
  - **Device Link**: `CreateDeviceLinkRequested`, `DeviceLink`, `DeviceLinkResult`, `ListDeviceLinksRequested`, `ListDeviceLinksResult`, `DeleteDeviceLinkRequested`
  - **Operator Lifecycle**: `TerminateOperatorRequested`, `TerminateOperatorResult`, `RotateAPIKeyRequested`, `RotateAPIKeyResult`, `ListOperatorSlotsRequested`, `ListOperatorSlotsResult`, `BindOperatorsRequested`, `BindOperatorsResult`, `UnbindOperatorsRequested`, `UnbindOperatorsResult`, `SetTargetContextRequested`, `SetTargetContextResult`, `OperatorDocument`
  - **Audit/History**: `FetchHistoryRequested`, `FetchHistoryResult`, `FetchFileHistoryRequested`, `FetchFileHistoryResult`, `FetchFileDiffRequested`, `FetchFileDiffResult`, `RestoreFileRequested`, `RestoreFileResult`, `DirectCommandAuditRequested`, `DirectCommandResultAuditRequested`, `AuditMsgRequested`
  - **Heartbeat**: `HeartbeatRequested`, `HeartbeatResult` (with system identity, network info, performance metrics, OS details, etc.)
  - **Governance Receipts**: `ActionReceipt` (signed execution proof with transaction_id, transaction_hash, status, state roots, signature, gateway_signed flag, l2_valid flag, l3_valid flag), `CommitmentAttestation` (Auditor's commitment chain)
  - **Passkey/WebAuthn**: `PasskeyRegisterChallengeRequested`, `PasskeyRegisterChallengeResult`, `PasskeyRegisterVerifyRequested`, `PasskeyRegisterVerifyResult`, `PasskeyAuthChallengeRequested`, `PasskeyAuthChallengeResult`, `PasskeyAuthVerifyRequested`, `PasskeyAuthVerifyResult`, `ListPasskeyCredentialsRequested`, `ListPasskeyCredentialsResult`, `RevokePasskeyCredentialRequested`, `RevokePasskeyCredentialResult`, `PasskeyCredential`
  - **Intent Grants**: `GrantIntentRequested`, `GrantIntentResult`, `RevokeIntentRequested`, `RevokeIntentResult`
  - **Shutdown**: `ShutdownRequested`
  - **Evals**: `EvalAnswerRequested`
- **Enums**: `ExecutionStatus` (UNSPECIFIED, EXECUTING, COMPLETED, FAILED, CANCELLED, TIMEOUT), `HeartbeatType` (UNSPECIFIED, AUTOMATIC, MANUAL)
- **Service Definition**: `OperatorService` with RPC methods for command execution, cancellation, file operations, and filesystem access

#### `proto/g8e/pubsub/v1/pubsub.proto`
- **Purpose**: Pub/sub message definitions for command/result channels
- **Key Messages**:
  - `PubSubMessage` - Command pub/sub message (action, channel, data bytes)
  - `PubSubEvent` - Event pub/sub message (type, channel, pattern, data bytes)
- **Usage**: Redis pub/sub command dispatch and event streaming

### Governance Constants

#### L1 Doctrine (`constants/doctrine/`)

**doctrine_registry.json**
- **Purpose**: Central registry of all doctrine sources
- **Contents**: References to gitleaks, MCP vectors, and OWASP CRS doctrines
- **Enforcement**: Loaded at runtime for L1 validation

**gitleaks_doctrine.json**
- **Purpose**: Secret and credential detection patterns
- **Examples**: API keys, tokens, passwords, certificates
- **Enforcement**: Blocks potential secret leaks in commands/outputs

**mcp_vectors_doctrine.json**
- **Purpose**: MCP-specific threat patterns
- **Examples**: Dangerous tool names, malicious argument patterns
- **Enforcement**: Blocks MCP tool calls with forbidden patterns

**owasp_crs_doctrine.json**
- **Purpose**: OWASP Core Rule Set security patterns
- **Examples**: SQL injection, XSS, command injection patterns
- **Enforcement**: Blocks web-style attack vectors in commands

### Other Key Constants

**agents.json** - Agent constant mappings (dash, sage, consensus members)
**api_paths.json** - API path mappings
**channels.json** - Pub/sub channel definitions
**collections.json** - Canonical collection schemas
**events.json** - Event type definitions
**headers.json** - HTTP header constants
**intents.json** - Intent definitions
**paths.json** - Path mappings
**ports.json** - Port assignments
**status.json** - Status codes

### Wire Format and Signing

- **Format**: protojson (canonical JSON) on all client-facing surfaces
- **Rationale**: MCP is JSON-RPC, A2A is JSON/HTTP, LLM tool-calling is JSON
- **Binary protobuf**: Used only as internal storage implementation detail
- **Transaction Hash**: Deterministic SHA-256 of normalized envelope fields
- **Hash Binding**: `id == SHA-256(canonical_fields)` enforced by verifier
- **Field Independence**: Wire encoding irrelevant to signature verification

## Internal Structure

### Directory Layout

```text
cmd/                           # Binary entry points
├── g8eo/                        # Main Operator binary (multi-mode)
├── chaos_tester/                # Chaos testing tool
├── exporter/                    # Audit export tool
└── uap-ping/                    # UAP protocol ping utility

internal/                       # Private implementation (not exported)
├── certs/                      # Certificate management and trust bundle loading
├── cli/                        # Platform CLI subcommands
│   ├── api/                    # API client for Operator communication
│   ├── auth/                   # Authentication client
│   ├── cmd/                    # CLI command handlers (platform, apps, auth, data, test, evals, security, setup, vars)
│   ├── config/                 # CLI configuration
│   └── platform/               # Platform process management
├── cmd/                        # Stream command handling (subprocess, SSH)
├── config/                     # Configuration loading and validation
├── constants/                  # Operator-specific constants (agents, API paths, auth, events)
├── contracts/                  # Protocol contract tests
├── httpclient/                 # HTTP client for outbound connections
├── interfaces/                 # Interface definitions
├── marshaler/                  # Envelope marshaling/unmarshaling
├── models/                     # Operator-specific data models
├── responder/                  # Response handling
├── security/                   # Cryptographic operations (Ed25519)
├── services/                   # Core service layer
│   ├── auth/                   # Bootstrap service for device-link enrollment
│   ├── execution/              # Command execution, file edit, fs operations
│   ├── gateway/                # Gateway mode: platform persistence, PKI, auth, pub/sub broker
│   ├── governance/             # L1-L5 verification (L1Doctrine, L2Consensus, L3Notary, L4Warden, L5Actuator)
│   ├── keystore/               # Platform-specific key storage (Darwin Keychain, Linux file backend)
│   ├── mcp/                    # MCP gateway for protocol translation (MCP/A2A)
│   ├── openclaw/               # OpenClaw node host service
│   ├── pubsub/                 # Pub/sub command channel, results streaming, loopback
│   ├── sovereignty/            # Sovereignty Boundary Plane: data scrubbing, rehydration, token persistence
│   ├── sqliteutil/             # SQLite utilities and migrations
│   ├── storage/                # Audit vault (SQLite+Git), ledger, local store, replay store
│   ├── system/                 # System operations (git resolution, port checking)
│   └── vault/                  # Vault operations (encryption, DEK management)
└── testutil/                   # Test utilities and fixtures

pkg/                            # Public packages
└── uap/                        # Universal Access Protocol utilities

protocol/                       # Protocol substrate (canonical source of truth)
├── constants/                  # Protocol constants (agents, API paths, channels, events, collections)
├── models/                     # Protocol data models (agents, case, etc.)
├── proto/                      # Protobuf schema definitions (commonv1, operatorv1, pubsubv1)
└── test-fixtures/              # Protocol test fixtures

test/                           # Integration and end-to-end tests
├── byo_client_test.go          # BYO client integration tests
├── mcp_gateway_test.go         # MCP gateway tests
└── mcp_real_operator_test.go  # Real Operator MCP tests

Makefile                        # Build targets
go.mod                          # Go module definition
```

### Core Service Layer Breakdown

#### `services/governance/`
- **Purpose**: Implement the L1-L5 verification gauntlet
- **Key Components**:
  - `l1_doctrine.go` - L1Doctrine: Technical Bedrock (Hard Gates)
  - `l2_consensus.go` - L2Consensus: Consensus stage
  - `l3_notary.go` - L3Notary: Notary Authorization (Human)
  - `l4_warden.go` - L4Warden: Fail-closed verification gate
  - `l5_actuator.go` - L5Actuator: Mutation execution boundary and receipt issuer
- **Critical Path**: Every mutation MUST pass through L4Warden before execution
- **Verification Layers**:
  - L1 (Doctrine): forbidden patterns, whitelist, blacklist via protobuf field options
  - L2 (Consensus): Ed25519 signature verification
  - L3 (Notary): WebAuthn/FIDO2 human approval (posture-dependent)
  - L4 (Warden): Verification gate orchestrator
  - L5 (Actuator): Execution boundary and receipt signer

#### `services/execution/`
- **Purpose**: Command execution and file operations
- **Key Components**:
  - `execution.go` - ExecutionService: shell command execution with concurrency control
  - `file_edit.go` - FileEditService: file write, delete, create operations
  - `fs_grep.go` - FsGrepService: filesystem search
  - `fs_list.go` - FsListService: filesystem listing
  - `file_edit_unix.go` - Unix-specific file operations
  - `fs_list_unix.go` - Unix-specific filesystem operations
- **Invariant**: Only verified transactions reach execution layer
- **Testing**: Comprehensive integration and shell operator tests

#### `services/storage/`
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

#### `services/auth/`
- **Purpose**: Bootstrap and device-link enrollment
- **Key Components**:
  - `bootstrap.go` - BootstrapService: device-link token authentication and bootstrap config application
  - `device_auth.go` - Device authentication and enrollment
  - `fingerprint.go` - Device fingerprinting for identity verification
- **Note**: Full auth lifecycle (users, sessions, passkeys, PKI) lives in gateway mode

#### `services/gateway/`
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
  - `secret_manager.go` - SecretManager: signing key storage (L5Actuator, Consensus)
  - `user_service.go` - UserService: user management
  - `session_service.go` - SessionService: session management
  - `api_key_service.go` - APIKeyService: API key management
  - `app_enrollment_service.go` - AppEnrollmentService: application enrollment
- **Mode**: Gateway mode serves inbound requests; does NOT execute commands or initiate outbound connections

#### `services/mcp/`
- **Purpose**: MCP/A2A protocol translation gateway
- **Key Components**:
  - `gateway.go` - GatewayService: MCP JSON-RPC to GovernanceEnvelope translation
  - `field_parser.go` - FieldPathRegistry: field path parsing for suspended transactions
  - `models.go` - SuspendedTransaction model
- **Flow**: MCP tool calls → GovernanceEnvelope → governance verification → Actuator execution → downstream MCP/A2A dispatch
- **Wire Format**: Canonical JSON (protojson) for UniversalEnvelope on all client-facing surfaces
- **Testing**: BYO client E2E tests and gateway integration tests

#### `services/pubsub/`
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
  - `l2_verifier.go` - L2 signature verification for pub/sub commands
  - `protocol_helpers.go` - Protocol envelope helpers
  - `g8es_pubsub_client.go` - Pub/sub client for g8es communication
  - `inprocess_client.go` - InProcessPubSubClient for in-process command dispatch
- **Loopback**: InProcessPubSubClient for in-process command dispatch in gateway mode

#### `services/sovereignty/`
- **Purpose**: Sovereignty Boundary Plane - data scrubbing, rehydration, and token persistence
- **Key Components**:
  - `boundary.go` - SovereigntyService: pattern-based secret detection, PII redaction, output projection, token persistence, rehydration
- **Invariant**: Raw data never crosses the trust boundary without scrubbing; sensitive data is tokenized and rehydrated at execution boundary
- **Testing**: Comprehensive fuzz testing and LFAA integration tests

#### `services/system/`
- **Purpose**: System-level operations
- **Key Components**:
  - `git.go` - Git binary resolution (returns "embedded" stub for go-git)
  - `path.go` - Path resolution and validation
  - `system_utils.go` - System utilities (port checking, filesystem operations)
- **Note**: Git binary resolution returns "embedded" stub; all git operations use go-git library

### Storage Layout

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

### Build Targets

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

## Data Flow and Request Lifecycle

### High-Level Request Flow

```text
┌─────────────────────────────────────────────────────────────────────┐
│                         ENVELOPE SUBMISSION                          │
│  Gateway mode: POST /api/governance/envelope (HTTP/WebSocket)        │
│  Outbound mode: pub/sub message from cloud platform                  │
│  Wire format: Canonical protojson GovernanceEnvelope                 │
└─────────────────────────────────────────────────────────────────────┘
                                    │
                                    ▼
┌─────────────────────────────────────────────────────────────────────┐
│                  PUBSUBCOMMANDSERVICE.DISPATCH                        │
│  ProcessEnvelope() (Gateway mode, synchronous)                        │
│  handleCommandPayload() (Outbound mode, async pub/sub)               │
│  Payload size validation, protojson decode                           │
└─────────────────────────────────────────────────────────────────────┘
                                    │
                                    ▼
┌─────────────────────────────────────────────────────────────────────┐
│                  L4WARDEN.VERIFYENVELOPE                       │
│  In-flight nonce tracking → Nonce reservation (SQLite)               │
│  Stateless: hash, L1 doctrine, payload decode                        │
│  Stateful: state root freshness                                       │
│  Posture: L1/L2/L3 based on governance posture                        │
└─────────────────────────────────────────────────────────────────────┘
                                    │
                                    ▼
┌─────────────────────────────────────────────────────────────────────┐
│                    L5ACTUATOR.EXECUTE                                  │
│  Sign initial receipt → Log receipt (SQLite + console_audit)          │
│  Sovereignty payload rehydration (if available)                      │
│  Execute handler (local or MCP/A2A egress)                           │
│  Update receipt with final status → Return signed receipt             │
└─────────────────────────────────────────────────────────────────────┘
                                    │
                                    ▼
┌─────────────────────────────────────────────────────────────────────┐
│                    AUDIT VAULT ANCHORING                             │
│  SQLite receipts table (transaction-native audit)                    │
│  Session-scoped git ledger (go-git) for file mutations              │
└─────────────────────────────────────────────────────────────────────┘
                                    │
                                    ▼
┌─────────────────────────────────────────────────────────────────────┐
│                   RECEIPT RETURN                                      │
│  Signed ActionReceipt with execution status, L2/L3 validity flags    │
└─────────────────────────────────────────────────────────────────────┘
```

### Detailed Flow: Operator Processing

#### Phase 1: Envelope Dispatch

**Entry Points**:
- `PubSubCommandService.ProcessEnvelope()` - Synchronous HTTP API entry point for Gateway mode (POST /api/governance/envelope)
- `PubSubCommandService.handleCommandPayload()` - Asynchronous pub/sub message handler for Outbound mode
- `PubSubCommandService.HandleCommandData()` - Gateway mode internal dispatch from HTTP/WebSocket

**Process**:
1. Payload size validation (rejects oversized payloads)
2. Protojson decode to `GovernanceEnvelope` (rejects non-JSON formats)
3. Dispatch to verification gauntlet

**Key Components**:
- `internal/services/pubsub/pubsub_commands.go` - Dispatch logic
- Wire format: canonical protojson JSON (not binary protobuf)
- Gateway mode: No pub/sub subscription, commands arrive via HTTP/WebSocket
- Outbound mode: Pub/sub subscription to cloud platform command channel

#### Phase 2: Transaction Verification

**Entry Point**: `L4Warden.VerifyEnvelope()`

**Verification Stages**:

1. **In-Flight Tracking** (early race prevention):
   - Track nonce in concurrent-safe in-flight map (sync.Map)
   - Reject if same nonce already processing

2. **Nonce Reservation** (durable replay protection):
   - Reserve nonce in SQLite replay store (atomic CheckAndSetNonce)
   - Check expiry timestamp
   - Reject if nonce already used (replay attack)
   - Nonce remains reserved until execution completes or fails
   - Release nonce on verification failure (allows retry)

3. **Stateless Validation** (no external state required):
   - Validate required fields (id, transaction_hash, payload)
   - Decode typed protobuf payload based on action_type
   - Compute transaction hash from normalized envelope fields
   - Verify id == computed hash (hash binding invariant)
   - L1 Doctrine validation via protobuf field options (forbidden patterns)
   - Extended L1 validation for MCP/A2A argument payloads via L1Doctrine (recursive threat analysis)

4. **Stateful Validation** (requires external state):
   - Verify state_merkle_root matches current state root
   - Reject stale state roots (prevents replay against old state)
   - State root provided by StateRootProvider (GatewayDBService in both modes)

5. **Posture Validation** (governance posture-aware):
   - L2 signature verification (if required by posture)
   - L3 proof verification (if required by posture and action is mutation)
   - Support for external app policy L3 bypass (auto-approve intents)
   - Governance postures: doctrine (L1 enforced, L2/L3 audited), consensus (L1/L2 enforced, L3 audited), notary (L1/L2/L3 strictly enforced)

**Key Components**:
- `internal/services/governance/l4_warden.go` - Verification logic
- `internal/services/storage/replay_store.go` - Nonce replay protection (SQLite)
- `protocol/constants/status.json` - Action type definitions and mutation flags
- `protocol/proto/common.proto` - GovernanceEnvelope schema with L1 field options
- `internal/services/governance/l1_doctrine.go` - MCP/A2A argument threat analysis
- Governance postures: doctrine, consensus, notary (configurable via --doctrine, --consensus, --notary flags)

#### Phase 3: Actuator Execution

**Entry Point**: `L5Actuator.Execute()`

**Process**:
1. Prepare initial ActionReceipt with EXECUTING status
2. Sign receipt with Actuator's Ed25519 key (fail-closed if signing fails)
3. Log receipt to audit vault before execution (fail-closed if logging fails)
4. Sovereignty payload rehydration (if available, restores scrubbed content)
5. Dispatch to registered ExecutionHandler based on action_type
   - Local execution: CommandService, FileOpsService, PortService, HistoryHandler
   - MCP/A2A egress: MCPGateway.DispatchToDownstream / DispatchToA2ADownstream
6. Capture execution result (success/failure/timeout)
7. Update receipt with final status and state root after execution
8. Sign final receipt
9. Return signed ActionReceipt (even on execution failure)

**Key Components**:
- `internal/services/governance/l5_actuator.go` - Execution boundary
- `internal/services/execution/` - Local execution handlers (command, file edit, fs operations)
- `internal/services/pubsub/` - CommandService, FileOpsService, PortService, HistoryHandler
- `internal/services/mcp/gateway.go` - MCP/A2A protocol translation egress
- `protocol/proto/operator.proto` - ActionReceipt schema
- Fail-closed invariant: no execution without signed receipt
- Receipt includes L2Valid and L3Valid flags for governance posture tracking

#### Phase 4: Audit Anchoring

**Entry Points**:
- `AuditVaultService.RecordActionReceipt()` - SQLite receipts table (transaction-native audit)
- `AuditVaultService.RecordEvent()` - SQLite events table (legacy command-centric audit)
- `LedgerService` - Session-scoped git-backed file mutation ledger (go-git)

**Process**:
1. Validate operator_session_id (must reference pre-created session in sessions table)
2. Write ActionReceipt to SQLite receipts table with execution metadata
3. Write ActionReceipt document to console_audit collection (document store)
4. For file mutations: commit to session-scoped git ledger with diff
5. Store tamper-evident history with commit hashes per session

**Key Components**:
- `internal/services/storage/audit_vault.go` - SQLite vault (receipts, events, sessions tables)
- `internal/services/storage/ledger.go` - Git ledger (go-git, session-scoped)
- `.g8e/data/g8e.db` - SQLite database location
- `.g8e/data/ledger/.git` - Global git repository
- `.g8e/data/ledger/sessions/{session_id}/.git` - Session-scoped git repositories
- Fail-closed: reject events with unknown sessions, never auto-create sessions
- Receipts table provides transaction-native audit with L2/L3 validity flags

#### Phase 5: Receipt Return

**Process**:
1. Actuator returns signed ActionReceipt to caller
2. Receipt contains execution status, state roots, signature, and L2/L3 validity flags
3. Caller (HTTP API or pub/sub) returns receipt to client
4. Receipt serves as cryptographic proof of execution attempt
5. Receipt returned even on execution failure (status=FAILED)

**Key Components**:
- `operatorv1.ActionReceipt` - Protobuf receipt schema
- Ed25519 signature verification by clients
- CanonicalizeActionReceipt for deterministic signing/verification
- Receipt includes gateway_signed flag for Gateway mode tracking

### Critical Decision Points

#### Fail-Closed Points

1. **In-Flight Nonce**: Same nonce already processing → reject (early race prevention)
2. **Nonce Replay**: Nonce already used in replay store → reject (durable replay protection)
3. **Envelope Decode**: Non-JSON payload → reject (only protojson accepted)
4. **Transaction Hash**: id != computed hash → reject (hash binding invariant)
5. **Expiry**: Transaction expired → reject
6. **State Root**: state_merkle_root != current state root → reject
7. **L1 Doctrine**: Forbidden pattern in typed payload → reject
8. **L1 Doctrine**: MCP/A2A argument threat detected → reject (recursive analysis)
9. **L2 Signature**: Invalid or missing signature (if required by posture) → reject
10. **L3 Proof**: Invalid or missing proof (if required by posture and mutation) → reject
11. **Actuator Signing**: Cannot sign receipt → reject (no execution without signed receipt)
12. **Audit Logging**: Cannot log receipt → reject (no execution without audit)
13. **Session Validation**: Unknown operator_session_id → reject (audit vault fail-closed)

#### Audit Points

1. **Nonce Reservation**: Durable SQLite write before expensive verification
2. **Verification Decisions**: Log each gate decision with nonce and reason
3. **Blocked Transactions**: Log blocked transactions to receipts table with BLOCKED status
4. **Receipt Signing**: Log initial receipt (EXECUTING status) before execution
5. **Execution**: Log command execution with stdout/stderr (truncated)
6. **Receipt Finalization**: Log final receipt with execution status
7. **Error**: Log all failures with context and nonce

### Storage Flow

#### Replay Store (SQLite)
```
Nonce reservation → ReserveNonce() → SQLite INSERT
→ FinalizeNonce() on success → ReleaseNonce() on failure
→ Prevents replay attacks across Operator restarts
→ Atomic CheckAndSetNonce for early durable commitment
```

#### Audit Vault (SQLite)
```
ActionReceipt → RecordActionReceipt() → Session validation
→ SQLite INSERT to receipts table → Transaction-native audit
→ Event → RecordEvent() → Session validation
→ SQLite INSERT to events table → Legacy command-centric audit
→ Fail-closed: rejects unknown sessions, never auto-creates
```

#### Git Ledger (go-git, session-scoped)
```
File mutation → LedgerFileWrite() → Session-scoped git staging
→ Git commit (go-git) → Commit hash → Tamper-evident history
→ Diff computation → Rollback capability
→ Session-scoped repos: .g8e/data/ledger/sessions/{session_id}/.git
→ Global repo: .g8e/data/ledger/.git
```

### Error Handling

#### Verification Failure
```
L4Warden.VerifyEnvelope() fails
→ Release nonce reservation (allows retry)
→ Return governance.ErrXxx sentinel error
→ Log blocked transaction to receipts table (BLOCKED status)
→ No receipt generated (verification failed before execution)
```

#### Execution Failure
```
L5Actuator.Execute() handler fails
→ Update receipt with FAILED status
→ Sign final receipt
→ Log receipt to audit
→ Return signed receipt (status=FAILED)
→ Client receives cryptographic evidence of failure
```

#### System Failure
```
L5Actuator signing or audit logging fails
→ Fail-closed: do not execute handler
→ Return error without receipt
→ Client receives verification error
→ No mutation occurs without audit trail
```

### Performance Considerations

#### Concurrency
- In-flight nonce tracking: concurrent-safe sync.Map
- Replay store: SQLite with atomic ReserveNonce/FinalizeNonce
- Execution service: semaphore-based concurrency control
- L5Actuator: sync.WaitGroup for graceful shutdown
- Audit vault writes: sync.WaitGroup for concurrent write safety

#### Caching
- State root: Provided by StateRootProvider (GatewayDBService in both modes)
- Doctrine rules: Loaded from protobuf field options (no runtime cache)
- L2 signers: Loaded from filesystem at startup (FilesystemSignerStore)
- Governance posture: Configured at startup (doctrine/consensus/notary)
- MCP/A2A downstream URLs: Configured at startup

#### Streaming
- Command output: streamingWriter with line-by-line logging
- Output truncation: 10MB limit per stream to prevent OOM
- Pub/sub messages: async goroutine processing with WaitGroup
- File ledger copies: streaming for unencrypted files, in-memory for encrypted (100MB limit)

### Security Boundaries

#### Trust Boundaries
1. **Client → Operator**: mTLS authentication (Gateway mode HTTP/WebSocket, Outbound mode pub/sub)
2. **Envelope → Verification**: Fail-closed L4Warden with L1/L2/L3 gates
3. **Verification → L5Actuator**: VerifiedTransaction with L2/L3 validity flags
4. **L5Actuator → Execution**: Fail-closed receipt signing before execution
5. **Execution → Audit**: SQLite + session-scoped Git ledger (host-local only)
6. **Audit → Client**: Signed receipt as cryptographic proof
7. **MCP/A2A Egress**: Downstream server dispatch via MCPGateway (Gateway mode only)

#### Data Sovereignty
- Raw execution output: Stored in SQLite audit vault (host-local, encrypted if vault unlocked)
- File mutations: Stored in session-scoped Git ledger (host-local, encrypted if vault unlocked)
- Scrubbed output: Returned in receipt (crosses trust boundary)
- State root: Provided by StateRootProvider (GatewayDBService in both modes)
- Nonce replay protection: SQLite replay store (host-local)
- MCP/A2A downstream results: Bounded to 4 KiB in receipt summary

## Key Invariants

1. **Protocol-first**: All wire formats and governance rules defined in `protocol/` only
2. **Single binary**: One `g8e` binary operates in multiple modes based on flags
3. **Fail-closed execution**: All verification gates default to reject; L4Warden rejects invalid envelopes before execution
4. **Audit-first**: Receipt logged before execution begins
5. **Hash binding**: Transaction hash computed from normalized fields, id == hash
6. **State binding**: Envelope bound to current state root
7. **Local audit**: Audit vault and ledger are host-local only; session validation required for audit writes (fail-closed)
8. **Replay protection**: Durable nonce reservation in SQLite with in-flight tracking
9. **Wire format**: Canonical JSON (protojson) for UniversalEnvelope on all client-facing surfaces; binary protobuf only for internal storage
10. **Session validation**: Audit vault rejects events with unknown sessions, never auto-creates
11. **Governance posture**: L1/L2/L3 enforcement based on doctrine/consensus/notary mode
12. **Receipt on failure**: Signed receipt returned even on execution failure
13. **Git-native**: Ledger uses go-git library (native Go implementation), not shell git commands
14. **Sovereignty scrubbing**: All output passes through Sovereignty Boundary Plane before crossing trust boundary
15. **No backwards compatibility**: Rip and replace approach; no compatibility shims or deprecated paths
