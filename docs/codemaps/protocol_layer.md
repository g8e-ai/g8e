# Protocol Layer Codemap

## Overview

The protocol layer is the single source of truth for all g8e protocol definitions. It contains protobuf schemas, governance constants, and shared data models. All services MUST consume protocol definitions from this layer; no local schema duplication is permitted.

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
│   │
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
│   │   ├── assistant.json
│   │   ├── auditor.json
│   │   ├── lite.json
│   │   ├── primary.json
│   │   ├── title_generator.json
│   │   ├── triage.json
│   │   └── tribunal.json
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
│   ├── tribunal.json
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

# Generated outputs (not in protocol/)
protocol/proto/g8e/              # Generated Go protobuf code (source_relative)
protocol/python/g8e_protocol/    # Generated Python constants
docs/reference/api/              # Generated API documentation
```

## Protobuf Schemas (`proto/`)

### `proto/g8e/common/v1/common.proto`
- **Purpose**: Common message definitions used across all services
- **Key Messages**:
  - `GovernanceEnvelope` - The canonical mutation envelope binding identity, intent, state, and governance proofs
  - `GovernanceMetadata` - Unified L1/L2/L3 governance proofs
  - `L1Metadata` - Doctrine (L1) validation status with violations list
  - `L2Metadata` - Quorum (L2) consensus signature with tribunal signature, agent IDs, and key ID
  - `L3Proof` - Notary (L3) authorization proof supporting WebAuthn (client_data_json, authenticator_data, signature, credential_id) or mTLS (mtls_cert_fingerprint)
  - `L3Metadata` - Notary authorization metadata with proof and auto_approved flag
  - `Component` - Source component identifier enum (G8EE, G8EO, CLIENT)
- **Wire Format**: protojson (canonical JSON)
- **Signing Basis**: Deterministic transaction_hash from normalized envelope fields
- **Custom Options**: `forbidden_patterns` field option (extension 50001) for L1 reflection on payload fields

### `proto/g8e/operator/v1/operator.proto`
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

### `proto/g8e/pubsub/v1/pubsub.proto`
- **Purpose**: Pub/sub message definitions for command/result channels
- **Key Messages**:
  - `PubSubMessage` - Command pub/sub message (action, channel, data bytes)
  - `PubSubEvent` - Event pub/sub message (type, channel, pattern, data bytes)
- **Usage**: Redis pub/sub command dispatch and event streaming

### `proto/buf.yaml`
- **Purpose**: Buf module configuration
- **Configuration**:
  - Module name: `buf.build/g8e/platform`
  - Lint rules: DEFAULT
  - Breaking change detection: FILE

### `buf.gen.yaml` (root)
- **Purpose**: Buf generation configuration for local-only generation
- **Plugins**:
  - `protoc-gen-go` (local): Generates Go code to `protocol/proto/` with `paths=source_relative`
  - `protoc-gen-go-grpc` (local): Generates gRPC Go code to `protocol/proto/` with `paths=source_relative`
  - `protoc-gen-doc` (local): Generates Markdown documentation to `docs/reference/api/`
- **Note**: Python constants are generated separately via Go exporter in `cmd/exporter`

## Governance Constants (`constants/`)

### L1 Doctrine (`constants/doctrine/`)

#### `doctrine_registry.json`
- **Purpose**: Central registry of all doctrine sources
- **Contents**: References to gitleaks, MCP vectors, and OWASP CRS doctrines
- **Enforcement**: Loaded at runtime for L1 validation

#### `gitleaks_doctrine.json`
- **Purpose**: Secret and credential detection patterns
- **Examples**: API keys, tokens, passwords, certificates
- **Enforcement**: Blocks potential secret leaks in commands/outputs

#### `mcp_vectors_doctrine.json`
- **Purpose**: MCP-specific threat patterns
- **Examples**: Dangerous tool names, malicious argument patterns
- **Enforcement**: Blocks MCP tool calls with forbidden patterns

#### `owasp_crs_doctrine.json`
- **Purpose**: OWASP Core Rule Set security patterns
- **Examples**: SQL injection, XSS, command injection patterns
- **Enforcement**: Blocks web-style attack vectors in commands

### Agent Definitions (`constants/agents.json`)
- **Purpose**: Agent constant mappings (not full definitions)
- **Contents**:
  - Agent name constants (dash, sage)
  - Tribunal member constants (axiom, concord, nemesis, pragma, variance)
  - Tribunal auditor reason constants (auditor_error, empty_response, no_valid_revision, ok, revised_from_dissent, swapped_to_dissenter, whitelist_violation)
  - Tribunal tie-break reason constants (alphabetical, excluded_nemesis, shortest)
- **Note**: Full agent configuration definitions are in `models/agents/`

### API Paths (`constants/api_paths.json`)
- **Purpose**: API path mappings for all services
- **Contents**:
  - `client` - Client API paths (chat, health, SSE events/stream)
  - `client_full` - Full client API paths with /api prefix
  - `g8ee` - g8ee application paths (auth, case, chat, investigation, operators, settings)
  - `g8ee_full` - Full g8ee paths with /api/v1 prefix
  - `g8eo` - Operator API paths
  - `g8eo_full` - Full Operator paths with /api prefix
  - `internal` - Internal admin paths
- **Generation**: Go and Python enum generation from `_go_const` and `_python_const` fields

### Channels (`constants/channels.json`)
- **Purpose**: Pub/sub channel definitions
- **Contents**:
  - `Governance` - Governance channel
  - `Message` - Message channel
  - `OperatorDevice` - Operator device channel
  - `OperatorIntent` - Operator intent channel
  - `SseEvent` - SSE event channel
  - `StorageBlob` - Storage blob channel
  - `StorageDocument` - Storage document channel
  - `StorageKv` - Storage key-value channel
  - Pub/sub actions: `Publish`, `Subscribe`, `PSubscribe`, `Unsubscribe`
  - Pub/sub events: `Message`, `PMessage`, `Subscribed`
- **Generation**: Go and Python constant generation from `_go_const` and `_python_const` fields

### Collections (`constants/collections.json`)
- **Purpose**: Canonical collection names for storage
- **Contents**:
  - User collections: `users`, `organizations`, `api_keys`, `web_sessions`, `cli_sessions`
  - Governance collections: `trusted_signers`, `revoked_certificates`, `passkey_challenges`
  - Operator collections: `operators`, `operator_sessions`, `bound_sessions`, `operator_usage`
  - App collections: `cases`, `investigations`, `memories`, `tasks`
  - Reputation collections: `reputation_state`, `reputation_commitments`, `stake_resolutions`
  - Audit collections: `console_audit`, `login_audit`, `auth_admin_audit`, `agent_activity_metadata`
  - System collections: `account_locks`, `settings`, `app_policies`, `tribunal_commands`, `chaos_events`
- **Generation**: Go and Python constant generation from `_go_const` and `_python_const` fields

### Events (`constants/events.json`)
- **Purpose**: Event type definitions for audit and pub/sub
- **Contents**:
  - AI agent events: conflict detection/resolution, continue approval, tribunal voting/consensus, triage clarification
  - LLM chat events: iterations (started, completed, failed, retry), streaming (delta, text, thinking), lifecycle, tool calls, message lifecycle
  - App events: case lifecycle (created, assigned, escalated, resolved, closed), investigation lifecycle, task lifecycle
  - Operator events: command execution (requested, started, completed, failed, cancelled), bootstrap, PKI (API key refresh), audit recording, device-link, heartbeat
  - MCP/A2A events: call requested
- **Generation**: Go and Python constant generation from `_go_const` and `_python_const` fields

## Shared Data Models (`models/`)

### Agent Models (`models/agents/`)
- **Purpose**: Agent configuration and behavior models
- **Contents**:
  - `assistant.json` - Assistant agent configuration
  - `auditor.json` - Auditor agent configuration
  - `lite.json` - Lite agent configuration
  - `primary.json` - Primary agent configuration
  - `title_generator.json` - Title generator agent configuration
  - `triage.json` - Triage agent configuration
  - `tribunal.json` - Tribunal agent configuration

### Domain Models
- `agent_activity_metadata.json` - Agent activity tracking schema
- `auditor_commands.json` - Auditor command definitions
- `case.json` - Case schema
- `conversation.json` - Conversation schema
- `conversation_message.json` - Conversation message schema
- `investigation.json` - Investigation schema
- `operator_document.json` - Operator document schema
- `platform_settings.json` - Platform settings schema
- `reputation_commitment.json` - Reputation commitment schema
- `reputation_state.json` - Reputation state schema
- `security_constraints.json` - Security constraints schema
- `stake_resolution.json` - Stake resolution schema
- `tool_results.json` - Tool results schema
- `tribunal.json` - Tribunal schema
- `user.json` - User schema
- `user_settings.json` - User settings schema
- `errors.py` - Python error definitions

## Generated Python Bindings

### Location: `protocol/python/g8e_protocol/`
- **Purpose**: Python package for protocol constant consumption
- **Contents**:
  - `channels.py` - Pub/sub channel constants
  - `collections.py` - Collection name constants
  - `document_ids.py` - Document ID constants
  - `events.py` - Event type constants
  - `headers.py` - HTTP header constants
  - `intents.py` - Intent constants
  - `platform.py` - Platform configuration constants
  - `prompts.py` - Prompt template constants
  - `pubsub.py` - Pub/sub configuration constants
  - `senders.py` - Sender identifier constants
  - `status.py` - Status code constants
  - `generated_paths.py` - Generated path mappings
- **Generation**: Exported from Go constants via `cmd/exporter`
- **Usage**: Imported by g8e-compatible agentic ensembles and other Python services

### Note on Protobuf Python Generation
- Python protobuf code is NOT currently generated from .proto files
- Python services use JSON wire format (protojson) directly
- Only Go protobuf code is generated to `protocol/proto/`

## Test Fixtures (`test-fixtures/`)
- **Purpose**: Canonical test data for protocol compliance
- **Contents**:
  - `gold-set-schema.json` - Gold set schema definition
  - `ledger-hash-fixtures.json` - Ledger hash test fixtures
  - `sse-events-schema.json` - SSE event schema definition
  - `sse-events.json` - SSE event test data
- **Usage**: Contract tests, golden master tests, evals

## Build Targets

### Root Makefile
```makefile
make generate           # Generate all protocol artifacts (proto + constants)
make proto              # Generate Go Protobuf code only
make constants          # Generate Go constants and export to JSON/Python
make clean-constants    # Remove generated constants files
make buf-install        # Install Buf CLI locally
```

### Protocol Makefile (protocol/Makefile)
```makefile
make test               # Run Go tests for protocol tools
make fmt                # Format Go code
make vet                # Run go vet
make lint               # Run golangci-lint
```

### Generation Process
1. **Protobuf Generation** (`make proto`):
   - Run `buf generate protocol/proto` with root `buf.gen.yaml`
   - Generate Go code to `protocol/proto/g8e/` (source_relative)
   - Generate Markdown docs to `docs/reference/api/`

2. **Constants Generation** (`make constants`):
   - Run Go generator in `internal/constants/generate_registry.go`
   - Export constants to JSON and Python via `cmd/exporter`
   - Generate Python constants to `protocol/python/g8e_protocol/`

3. **Validation**:
   - Contract tests in `internal/contracts/`
   - Protocol tests via `make test` in protocol/ directory

## Protocol Invariants

### Wire Format
- **Format**: protojson (canonical JSON)
- **Rationale**: MCP is JSON-RPC, A2A is JSON/HTTP, LLM tool-calling is JSON
- **Binary protobuf**: Used only as internal storage implementation detail

### Signing Basis
- **Transaction Hash**: Deterministic SHA-256 of normalized envelope fields
- **Field Independence**: Wire encoding irrelevant to signature verification
- **Hash Binding**: `id == SHA-256(canonical_fields)` enforced by verifier

### Schema Source of Truth
- **Single Source**: `.proto` files in `protocol/proto/`
- **No Duplication**: Services MUST NOT define local schemas
- **Generation**: All code generated from protobuf schemas

### Versioning
- **Semantic Versioning**: Protocol versions follow semver
- **Backward Compatibility**: NO backward compatibility (per user directive)
- **Breaking Changes**: Require protocol version bump and migration

## Critical Protocol Flows

### Envelope Formation
```text
Intent → protojson GovernanceEnvelope → transaction_hash computation
→ L1/L2/L3 signatures → id field set to hash → ready for submission
```

### Envelope Verification
```text
Received envelope → id extraction → hash recomputation from fields
→ id == hash check → signature verification → L1/L2/L3 validation
→ state root freshness → typed payload decode → dispatch
```

### Receipt Generation
```text
Execution complete → ActionReceipt formation → execution metadata
→ output hash → signer signature → protojson encoding → return
```

## Integration Points

### g8eo (Operator)
- **Consumes**: `proto/` (Go generated code)
- **Location**: `protocol/proto/g8e/` (source_relative)
- **Usage**: Envelope verification, receipt generation, audit events, MCP/A2A dispatch

### g8eg (Gateway)
- **Consumes**: `proto/` (Go generated code)
- **Location**: `protocol/proto/g8e/` (shared with g8eo)
- **Usage**: Envelope admission, signature verification, state distribution, L2 consensus

### g8e-Compatible Agentic Ensembles
- **Consumes**: Python constants from `protocol/python/g8e_protocol/`
- **Location**: Imported via `g8e_protocol` package
- **Usage**: Event type constants, collection names, API paths, headers
- **Wire Format**: Uses protojson directly (no Python protobuf generation)

### BYO Clients
- **Consumes**: Direct protojson wire format or Python constants
- **Location**: Can import `g8e_protocol` package or use .proto files directly
- **Usage**: Envelope formation, receipt parsing, type definitions
- **Wire Format**: protojson (canonical JSON) - no binary protobuf on wire

## Contract Tests

### Protocol Contract Tests
- **Purpose**: Verify protocol compliance across services
- **Location**: `internal/contracts/`
- **Coverage**:
  - Envelope serialization/deserialization
  - Signature verification
  - Hash computation
  - Type validation

### Golden Master Tests
- **Purpose**: Verify protocol implementation against canonical examples
- **Location**: `evals/gold_sets/`
- **Usage**: Regression testing for protocol changes

## Key Invariants

1. **Single source of truth**: All schemas in `protocol/proto/g8e/` with package hierarchy (common/v1, operator/v1, pubsub/v1)
2. **No local duplication**: Services MUST NOT define local schemas
3. **Protojson wire format**: JSON on all client-facing surfaces (HTTP APIs, pub/sub, receipts)
4. **Hash-based signing**: Transaction hash independent of wire encoding
5. **Generated code**: Go code generated from protobuf with source_relative output; Python constants exported from Go
6. **No backward compatibility**: Breaking changes require migration (per user directive)
7. **Local generation**: No BSR remote plugins; local-only generation via buf.gen.yaml
8. **Python constants path**: Generated to `protocol/python/g8e_protocol/`
9. **Build orchestration**: Proto generation via root Makefile, not protocol/Makefile
10. **Package hierarchy**: Protobuf files follow g8e package structure (g8e.common.v1, g8e.operator.v1, g8e.pubsub.v1)
