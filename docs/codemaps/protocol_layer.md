# Protocol Layer Codemap

## Overview

The protocol layer is the single source of truth for all g8e protocol definitions. It contains protobuf schemas, governance constants, and shared data models. All services MUST consume protocol definitions from this layer; no local schema duplication is permitted.

```text
protocol/
├── proto/                       # Protobuf schemas (wire format source of truth)
│   ├── buf.yaml                 # Buf module configuration
│   ├── common.proto             # Common message definitions
│   ├── operator.proto           # Operator-specific messages
│   └── pubsub.proto             # Pub/sub message definitions
│
├── constants/                   # Governance constants and rules
│   ├── doctrine/                # L1 Doctrine definitions
│   │   ├── doctrine_registry.json       # Doctrine registry
│   │   ├── gitleaks_doctrine.json       # Secret detection patterns
│   │   ├── mcp_vectors_doctrine.json    # MCP threat patterns
│   │   └── owasp_crs_doctrine.json     # OWASP security patterns
│   │
│   ├── agents.json              # Agent type definitions
│   ├── api_paths.json           # API path mappings
│   ├── channels.json            # Pub/sub channel definitions
│   ├── collections.json        # Canonical collection schemas
│   ├── events.json              # Event type definitions
│   ├── headers.json             # HTTP header constants
│   ├── intents.json             # Intent definitions
│   ├── paths.json               # Path mappings
│   ├── platform.json            # Platform configuration
│   ├── ports.json               # Port assignments
│   ├── prompts.json             # Prompt templates
│   ├── pubsub.json              # Pub/sub configuration
│   ├── senders.json             # Sender identifiers
│   ├── status.json              # Status codes
│   └── ...                      # Other constant files
│
├── models/                      # Shared data models (JSON schemas)
│   ├── agents/                  # Agent-related models
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
protocol/proto/  # Generated Go protobuf code
protocol/python/g8e_protocol/  # Generated Python constants
docs/reference/api/                 # Generated API documentation
```

## Protobuf Schemas (`proto/`)

### `common.proto`
- **Purpose**: Common message definitions used across all services
- **Key Messages**:
  - `GovernanceEnvelope` - The canonical mutation envelope
  - `GovernanceMetadata` - Unified L1/L2/L3 governance proofs
  - `L1Metadata` - Doctrine (L1) validation status
  - `L2Metadata` - Quorum (L2) consensus signature
  - `L3Proof` - Notary (L3) authorization proof (WebAuthn or mTLS)
  - `L3Metadata` - Notary authorization metadata
  - `Component` - Source component identifier enum
- **Wire Format**: protojson (canonical JSON)
- **Signing Basis**: Deterministic transaction_hash from normalized fields
- **Custom Options**: `forbidden_patterns` field option for L1 reflection

### `operator.proto`
- **Purpose**: Operator-specific message definitions
- **Key Message Categories**:
  - **Command Payloads**: `CommandRequested`, `CommandCancelRequested`, `FileEditRequested`, `FsListRequested`, `FsReadRequested`, `FsGrepRequested`
  - **MCP/A2A Protocol**: `McpCallRequested`, `A2aCallRequested`, `McpResourceListRequested`, `McpResourceReadRequested`, `McpPromptListRequested`, `McpPromptGetRequested`
  - **Result Payloads**: `CommandResult`, `FsListResult`, `FsReadResult`, `FsGrepResult`, `FileEditResult`, `PortCheckResult`, `FetchLogsResult`
  - **PKI/Certificate**: `SignCertificateRequested`, `SignCertificateResult`, `RevokeCertificateRequested`, `RevokeCertificateResult`, `GetRevocationBundleRequested`, `GetRevocationBundleResult`
  - **Device Link**: `CreateDeviceLinkRequested`, `DeviceLink`, `DeviceLinkResult`, `ListDeviceLinksRequested`, `ListDeviceLinksResult`, `DeleteDeviceLinkRequested`
  - **Operator Lifecycle**: `TerminateOperatorRequested`, `TerminateOperatorResult`, `RotateAPIKeyRequested`, `RotateAPIKeyResult`, `ListOperatorSlotsRequested`, `ListOperatorSlotsResult`, `BindOperatorsRequested`, `BindOperatorsResult`, `UnbindOperatorsRequested`, `UnbindOperatorsResult`, `SetTargetContextRequested`, `SetTargetContextResult`, `OperatorDocument`
  - **Audit/History**: `FetchHistoryRequested`, `FetchHistoryResult`, `FetchFileHistoryRequested`, `FetchFileHistoryResult`, `FetchFileDiffRequested`, `FetchFileDiffResult`, `RestoreFileRequested`, `RestoreFileResult`, `DirectCommandAuditRequested`, `DirectCommandResultAuditRequested`, `AuditMsgRequested`
  - **Heartbeat**: `HeartbeatRequested`, `HeartbeatResult` (with system identity, network info, performance metrics, OS details, etc.)
  - **Governance Receipts**: `ActionReceipt` (signed execution proof), `CommitmentAttestation` (Auditor's commitment chain)
  - **Passkey/WebAuthn**: `PasskeyRegisterChallengeRequested`, `PasskeyRegisterChallengeResult`, `PasskeyRegisterVerifyRequested`, `PasskeyRegisterVerifyResult`, `PasskeyAuthChallengeRequested`, `PasskeyAuthChallengeResult`, `PasskeyAuthVerifyRequested`, `PasskeyAuthVerifyResult`, `ListPasskeyCredentialsRequested`, `ListPasskeyCredentialsResult`, `RevokePasskeyCredentialRequested`, `RevokePasskeyCredentialResult`, `PasskeyCredential`
  - **Intent Grants**: `GrantIntentRequested`, `GrantIntentResult`, `RevokeIntentRequested`, `RevokeIntentResult`
  - **Shutdown**: `ShutdownRequested`
  - **Evals**: `EvalAnswerRequested`
- **Enums**: `ExecutionStatus`, `HeartbeatType`
- **Usage**: Operator command execution, MCP/A2A protocol translation, PKI management, device-link flows, audit trails, WebAuthn brokerage

### `pubsub.proto`
- **Purpose**: Pub/sub message definitions for command/result channels
- **Key Messages**:
  - `PubSubMessage` - Command pub/sub message (action, channel, data)
  - `PubSubEvent` - Event pub/sub message (type, channel, pattern, data)
- **Usage**: Redis pub/sub command dispatch and event streaming

### `buf.yaml` (protocol/proto/)
- **Purpose**: Buf module configuration
- **Configuration**:
  - Module name: `buf.build/g8e/platform`
  - Lint rules: DEFAULT
  - Breaking change detection: FILE

### `buf.gen.yaml` (root)
- **Purpose**: Buf generation configuration
- **Plugins**:
  - `protoc-gen-go` (local): Generates Go code to `protocol/proto/`
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
  - Agent name constants (Dash, Sage)
  - Triage classification constants (Complexity, Confidence, Intent, Posture)
  - Tribunal member constants (Axiom, Concord, Nemesis, Pragma, Variance)
  - Tribunal voting constants (Auditor reasons, tie-break reasons)
- **Note**: Full agent definitions are in `models/agents/`

### API Paths (`constants/api_paths.json`)
- **Purpose**: API path mappings for all services
- **Contents**:
  - Gateway API paths
  - Operator API paths
  - g8e-compatible agentic ensemble API paths
  - Internal admin paths
  - Path constants with Go and Python enum generation

### Channels (`constants/channels.json`)
- **Purpose**: Pub/sub channel definitions
- **Contents**:
  - Command channels (cmd::)
  - Result channels (result::)
  - Event channels (event::)
  - Channel patterns and access rules
  - Go and Python constant generation

### Collections (`constants/collections.json`)
- **Purpose**: Canonical collection names for storage
- **Contents**:
  - User collections (users, organizations, api_keys, web_sessions, cli_sessions)
  - Governance collections (trusted_signers, revoked_certificates, passkey_challenges)
  - Operator collections (operators, operator_sessions, bound_sessions, operator_usage)
  - App collections (cases, investigations, memories, tasks)
  - Reputation collections (reputation_state, reputation_commitments, stake_resolutions)
  - Audit collections (console_audit, login_audit, auth_admin_audit, agent_activity_metadata)
  - System collections (account_locks, settings, app_policies, tribunal_commands, chaos_events)
  - Go and Python constant generation

### Events (`constants/events.json`)
- **Purpose**: Event type definitions for audit and pub/sub
- **Contents**:
  - AI agent events (conflict, continue approval, tribunal voting, triage)
  - LLM chat events (iterations, streaming, lifecycle, tool calls)
  - App events (case, investigation, task lifecycle)
  - Operator events (command execution, bootstrap, PKI, device-link, audit)
  - Go and Python constant generation

## Shared Data Models (`models/`)

### Agent Models (`models/agents/`)
- **Purpose**: Agent-related data models
- **Contents**:
  - Agent configuration models
  - Agent state models
  - Agent reputation models

### Domain Models
- `agent_activity_metadata.json` - Agent activity tracking schema
- `auditor_commands.json` - Auditor command definitions
- `case.json` - Case/investigation schema
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

### Root Makefile (not protocol/Makefile)
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
   - Generate Go code to `protocol/proto/`
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
- **Location**: `protocol/proto/`
- **Usage**: Envelope verification, receipt generation, audit events, MCP/A2A dispatch

### g8eg (Gateway)
- **Consumes**: `proto/` (Go generated code)
- **Location**: `protocol/proto/` (shared with g8eo)
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

1. **Single source of truth**: All schemas in `protocol/proto/`
2. **No local duplication**: Services MUST NOT define local schemas
3. **Protojson wire format**: JSON on all client-facing surfaces (HTTP APIs, pub/sub, receipts)
4. **Hash-based signing**: Transaction hash independent of wire encoding
5. **Generated code**: Go code generated from protobuf; Python constants exported from Go
6. **No backward compatibility**: Breaking changes require migration (per user directive)
7. **Local generation**: No BSR remote plugins; local-only generation
8. **Python constants path**: Generated to `protocol/python/g8e_protocol/`
9. **Build orchestration**: Proto generation via root Makefile, not protocol/Makefile
