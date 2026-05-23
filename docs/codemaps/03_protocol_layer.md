# Protocol Layer Codemap

## Overview

The protocol layer is the single source of truth for all g8e protocol definitions. It contains protobuf schemas, governance constants, shared data models, and language-specific bindings. All services MUST consume protocol definitions from this layer; no local schema duplication is permitted.

```text
protocol/
├── proto/                       # Protobuf schemas (wire format source of truth)
│   ├── buf.yaml                 # Buf configuration
│   ├── common.proto             # Common message definitions
│   └── operator.proto           # Operator-specific messages
│
├── constants/                   # Governance constants and rules
│   ├── doctrine/                # L1 Doctrine definitions
│   │   ├── forbidden_patterns.json    # Forbidden command patterns
│   │   ├── whitelist.json             # Allowed command patterns
│   │   └── blacklist.json             # Denied command patterns
│   │
│   ├── agents.json              # Agent type definitions
│   ├── api_paths.json           # API path mappings
│   ├── channels.json            # Pub/sub channel definitions
│   ├── collections.json        # Canonical collection schemas
│   ├── events.json              # Event type definitions
│   └── ...                      # Other constant files
│
├── models/                      # Shared data models
│   ├── agents/                  # Agent-related models
│   ├── agent_activity_metadata.json
│   ├── auditor_commands.json
│   ├── case.json
│   └── ...                      # Other shared models
│
├── python/                      # Python protocol bindings
│   ├── g8e_protocol/            # Python package
│   │   ├── __init__.py
│   │   ├── models/              # Generated Python models
│   │   └── ...                  # Other Python modules
│   ├── pyproject.toml           # Python project configuration
│   └── ...
│
├── test-fixtures/               # Protocol test fixtures
│
├── Makefile                     # Protocol build targets
├── go.mod                       # Go module for protocol tools
└── workload_identity.go         # Workload identity utilities
```

## Protobuf Schemas (`proto/`)

### `common.proto`
- **Purpose**: Common message definitions used across all services
- **Key Messages**:
  - `GovernanceEnvelope` - The canonical mutation envelope
  - `UniversalEnvelope` - Universal payload wrapper
  - `L1Signature` - L1 Doctrine signature
  - `L2Signature` - L2 Quorum signature
  - `L3Signature` - L3 Notary signature
  - `ActionReceipt` - Signed execution receipt
  - `AuditEvent` - Audit event structure
- **Wire Format**: protojson (canonical JSON)
- **Signing Basis**: Deterministic transaction_hash from normalized fields

### `operator.proto`
- **Purpose**: Operator-specific message definitions
- **Key Messages**:
  - `DeviceLinkRequest` - Device-link enrollment
  - `DeviceLinkResponse` - Device-link response
  - `OperatorStatus` - Operator health/status
  - `RegistrationRequest` - Operator registration
  - Registration and lifecycle messages
- **Usage**: Operator-Gateway communication, device-link flows

### `buf.yaml`
- **Purpose**: Buf configuration for protobuf generation
- **Configuration**:
  - Local-only generation (no BSR remote plugins)
  - Go and Python output targets
  - Import path configuration

## Governance Constants (`constants/`)

### L1 Doctrine (`constants/doctrine/`)

#### `forbidden_patterns.json`
- **Purpose**: Forbidden command patterns (hard gates)
- **Examples**: `sudo`, `su`, `rm -rf /`, etc.
- **Enforcement**: Rejected immediately at L1; no override

#### `whitelist.json`
- **Purpose**: Explicitly allowed command patterns
- **Examples**: Diagnostic commands (`uptime`, `df`, `ps`)
- **Enforcement**: Auto-approval path for benign commands

#### `blacklist.json`
- **Purpose**: Explicitly denied command patterns
- **Examples**: Dangerous commands with context
- **Enforcement**: Rejected at L1 with specific reason

### Agent Definitions (`constants/agents.json`)
- **Purpose**: Agent type and capability definitions
- **Contents**:
  - Agent types (Triage, Sage, Tribunal, Warden, Auditor, Nemesis)
  - Agent capabilities and permissions
  - Agent reputation parameters

### API Paths (`constants/api_paths.json`)
- **Purpose**: API path mappings for all services
- **Contents**:
  - Gateway API paths
  - Operator API paths
  - Ensemble API paths
  - Internal admin paths

### Channels (`constants/channels.json`)
- **Purpose**: Pub/sub channel definitions
- **Contents**:
  - Command channels
  - Result channels
  - Event channels
  - Channel permissions and access rules

### Collections (`constants/collections.json`)
- **Purpose**: Canonical collection schemas for storage
- **Contents**:
  - Investigation collections
  - Memory collections
  - Audit collections
  - Governance collections
  - Collection validation rules

### Events (`constants/events.json`)
- **Purpose**: Event type definitions for audit and pub/sub
- **Contents**:
  - Request event types
  - Response event types
  - Audit event types
  - Event schemas and validation

## Shared Data Models (`models/`)

### Agent Models (`models/agents/`)
- **Purpose**: Agent-related data models
- **Contents**:
  - Agent configuration models
  - Agent state models
  - Agent reputation models

### Other Models
- **agent_activity_metadata.json** - Agent activity tracking
- **auditor_commands.json** - Auditor command definitions
- **case.json** - Case/investigation models
- **Other domain-specific models**

## Python Bindings (`python/`)

### `g8e_protocol/`
- **Purpose**: Python package for protocol consumption
- **Contents**:
  - Generated Python models from protobuf
  - Type definitions and validators
  - Utility functions for envelope handling
- **Usage**: Imported by g8ee and other Python services

### `pyproject.toml`
- **Purpose**: Python project configuration
- **Dependencies**: protobuf, pydantic, etc.
- **Build**: Generates Python code from protobuf schemas

## Test Fixtures (`test-fixtures/`)
- **Purpose**: Canonical test data for protocol compliance
- **Contents**:
  - Sample envelopes
  - Sample signatures
  - Sample audit events
- **Usage**: Contract tests, golden master tests

## Build Targets

### `Makefile`
```makefile
make proto              # Generate Go and Python code from protobuf
```text
make proto-go           # Generate Go code only
make proto-python       # Generate Python code only
make lint-proto         # Lint protobuf schemas
make clean-proto        # Clean generated code
```

### Generation Process
1. Run `buf generate` with `buf.gen.yaml`
2. Generate Go code to `services/g8eo/internal/protocol/proto/`
3. Generate Python code to `protocol/python/g8e_protocol/`
4. Validate generated code with contract tests

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
- **Location**: `services/g8eo/internal/protocol/proto/`
- **Usage**: Envelope verification, receipt generation, audit events

### g8ee (Ensemble)
- **Consumes**: `python/g8e_protocol/` (Python package)
- **Location**: Imported via `g8e_protocol` package
- **Usage**: Envelope formation, receipt parsing, type definitions

### g8eg (Gateway)
- **Consumes**: `proto/` (Go generated code)
- **Location**: Similar to g8eo
- **Usage**: Envelope admission, signature verification, state distribution

## Contract Tests

### Protocol Contract Tests
- **Purpose**: Verify protocol compliance across services
- **Location**: `services/g8eo/internal/contracts/`
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
3. **Protojson wire format**: JSON on all client-facing surfaces
4. **Hash-based signing**: Transaction hash independent of wire encoding
5. **Generated code**: All code generated from protobuf schemas
6. **No backward compatibility**: Breaking changes require migration
7. **Local generation**: No BSR remote plugins; local-only generation
