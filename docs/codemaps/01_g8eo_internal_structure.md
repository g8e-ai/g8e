# g8eo (Operator) Internal Structure Codemap

## Overview

g8eo is a Go-based service implementing the Governed Operator - the sovereign execution boundary. It enforces the L1/L2/L3 governance gauntlet, executes mutations through a fail-closed Actuator, and maintains a local Git-backed audit vault.

```text
services/g8eo/
├── cmd/                         # CLI entry points
│   ├── g8eo/                    # Main Operator binary
│   ├── chaos_tester/            # Chaos testing tool
│   ├── exporter/                # Audit export tool
│   └── uap-ping/                # UAP protocol ping utility
│
├── internal/                    # Private implementation (not exported)
│   ├── cmd/                     # Command-line handling
│   ├── config/                  # Configuration loading and validation
│   ├── constants/               # Operator-specific constants
│   ├── contracts/               # Protocol contract tests
│   ├── httpclient/              # HTTP client for Gateway communication
│   ├── marshaler/               # Envelope marshaling/unmarshaling
│   ├── models/                  # Operator-specific data models
│   ├── protocol/                # Protocol integration layer
│   │   ├── proto/               # Generated protobuf code
│   │   └── types/               # Protocol type adapters
│   ├── security/                # Cryptographic operations (Ed25519, mTLS)
│   ├── services/                # Core service layer
│   │   ├── auth/                # Authentication and device-link management
│   │   ├── execution/           # Actuator and command execution
│   │   ├── gateway/             # Gateway communication (mTLS tunnel)
│   │   ├── governance/          # L1/L2/L3 verification logic
│   │   ├── keystore/            # Key storage and management
│   │   ├── mcp/                 # MCP server implementation
│   │   ├── openclaw/            # OpenClaw protocol integration
│   │   ├── pubsub/              # Pub/sub command channel
│   │   ├── sentinel/            # PII/secret scrubbing
│   │   ├── sqliteutil/          # SQLite utilities
│   │   ├── storage/             # Audit vault and ledger (Git-backed)
│   │   └── system/              # System operations (git, ports, etc.)
│   └── testutil/                # Test utilities and fixtures
│
├── pkg/                         # Public packages (if any)
│   └── uap/                     # Universal Access Protocol utilities
│
├── tests/                       # Integration and end-to-end tests
│   ├── byo_client_test.go       # BYO client integration tests
│   ├── mcp_gateway_test.go      # MCP gateway tests
│   └── mcp_real_operator_test.go # Real Operator MCP tests
│
├── evals/                       # Evaluation framework
│   ├── g8e_evals/               # Eval runner and benchmarks
│   ├── gold_sets/               # Gold standard test sets
│   └── reports/                 # Eval execution reports
│
├── tools/                       # Build tools and vendored dependencies
├── Makefile                     # Build targets
└── go.mod                       # Go module definition
```

## Core Service Layer Breakdown

### `services/governance/`
- **Purpose**: Implement the L1/L2/L3 verification gauntlet
- **Key Components**:
  - Doctrine validation (L1 - forbidden patterns, whitelist, blacklist)
  - Quorum verification (L2 - Ed25519 signature verification)
  - Notary authorization (L3 - WebAuthn/FIDO2 human approval)
  - Envelope integrity and hash binding checks
  - State root freshness validation
- **Critical Path**: Every mutation MUST pass through this layer before execution

### `services/execution/`
- **Purpose**: Fail-closed Actuator for command execution
- **Key Components**:
  - Command dispatch and execution
  - Timeout and resource limits
  - Output capture and streaming
  - Signed receipt generation
- **Invariant**: Only the Actuator can execute host mutations

### `services/storage/`
- **Purpose**: Local audit vault and Git-backed ledger
- **Key Components**:
  - `audit_vault.go` - SQLite-based audit event storage
  - `ledger.go` - Git-backed commit history and diff tracking
  - Session management and validation
  - Tamper-evident commit chain
- **Storage**: `.g8e/audit/` (SQLite) + `.g8e/ledger/.git` (Git)

### `services/auth/`
- **Purpose**: Authentication and device-link lifecycle
- **Key Components**:
  - Device-link token generation and validation
  - mTLS certificate management
  - Session scoping and replay protection
  - Operator identity management

### `services/gateway/`
- **Purpose**: Outbound-only mTLS tunnel to Governance Gateway
- **Key Components**:
  - mTLS client configuration
  - Envelope fetching from Gateway
  - Receipt pushing to Gateway
  - Connection health monitoring
- **Direction**: Operator initiates tunnel; no inbound listeners

### `services/mcp/`
- **Purpose**: MCP server for BYO client integration
- **Key Components**:
  - MCP protocol implementation
  - Tool registration and dispatch
  - Envelope formation from MCP tool calls
  - Result streaming back to clients

### `services/pubsub/`
- **Purpose**: Pub/sub command channel for internal communication
- **Key Components**:
  - Command envelope publishing
  - Result subscription and streaming
  - Loopback testing infrastructure
  - UniversalEnvelope handling

### `services/sentinel/`
- **Purpose**: PII and secret scrubbing before output exposure
- **Key Components**:
  - Pattern-based secret detection
  - PII redaction
  - Safe output projection
- **Invariant**: Raw data never crosses the trust boundary

### `services/system/`
- **Purpose**: System-level operations
- **Key Components**:
  - Git operations (via go-git, not exec)
  - Port availability checking
  - Filesystem operations
  - Process management

## Protocol Integration Layer

### `internal/protocol/`
- **Purpose**: Bridge between g8eo and protocol definitions
- **Components**:
  - `proto/` - Generated protobuf code from `protocol/proto/`
  - `types/` - Type adapters and conversions
- **Source**: All protobuf schemas come from `protocol/proto/`

## Configuration

### `internal/config/`
- **Purpose**: Configuration loading and validation
- **Sources**:
  - Command-line flags (`--listen`, `--pki-dir`, etc.)
  - Environment variables (`G8E_PKI_DIR`, `G8E_RUNTIME_DIR`)
  - Configuration files (if any)
- **Validation**: Strict validation on startup; fail fast on misconfiguration

## Entry Points

### Main Operator Binary
- **Path**: `cmd/g8eo/main.go`
- **Modes**:
  - `--listen` - Run in listen mode (accept envelopes via mTLS tunnel)
  - `--once` - Execute a single command and exit
  - `--chaos` - Run chaos testing
- **Output**: `build/linux-amd64/g8e.operator`

### Supporting Tools
- **chaos_tester**: Chaos and fault injection testing
- **exporter**: Audit vault export and reporting
- **uap-ping**: UAP protocol connectivity testing

## Testing Structure

### Unit Tests
- Located alongside source files (`*_test.go`)
- Focus on individual service logic
- Use `internal/testutil/` for fixtures

### Integration Tests
- **Path**: `tests/`
- **BYO client tests**: Verify external client integration
- **MCP gateway tests**: Verify MCP server behavior
- **Real Operator tests**: End-to-end with real binary

### Eval Framework
- **Path**: `evals/`
- **Purpose**: Golden master testing for protocol compliance
- **Components**:
  - `g8e_evals/` - Eval runner and benchmarks
  - `gold_sets/` - Canonical test inputs and expected outputs
  - `reports/` - Execution reports and coverage

## Critical Data Paths

### Mutation Request Flow
```text
Gateway (mTLS) → services/gateway/ → services/governance/ (L1/L2/L3)
→ services/execution/ (Actuator) → services/storage/ (Audit Vault)
→ services/gateway/ (Receipt push)
```

### MCP Tool Call Flow
```text
MCP Client → services/mcp/ → Envelope formation → services/governance/
→ services/execution/ → Result streaming → MCP Client
```

### Audit Write Flow
```text
Any service → services/storage/audit_vault.go → SQLite
→ services/storage/ledger.go → Git commit → Tamper-evident history
```

## Storage Layout

```text
.g8e/
├── pki/                         # PKI material (certificates, keys)
├── audit/                       # SQLite audit vault
├── ledger/                      # Git-backed ledger
│   └── .git/                    # Git repository
└── logs/                        # Operator logs
    └── operator-listen.log
```

## Build Targets

```makefile
make build-g8eo          # Build g8e.operator binary
make test-g8eo           # Run g8eo unit tests
make lint-g8eo           # Run linters
make clean-g8eo          # Clean build artifacts
```

## Key Invariants

1. **Fail-closed execution**: Only `services/execution/` can execute mutations
2. **Protocol consumption**: All protobuf schemas from `protocol/proto/`
3. **Local audit**: Audit vault and ledger are host-local only
4. **Outbound-only**: No inbound listeners; mTLS tunnel initiated by Operator
5. **Git-native**: Ledger uses go-git, not shell git commands
6. **Sentinel scrubbing**: All output passes through `services/sentinel/`
