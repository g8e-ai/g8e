# g8e High-Level Architecture Codemap

## Overview

g8e is organized as a protocol-first substrate with a single binary that operates in multiple modes. The protocol layer is the single source of truth for all wire formats, governance contracts, and shared constants. The `g8e` binary implements the protocol at different layers of the governance stack based on operational mode.

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
│   │   ├── sentinel/            # PII/secret scrubbing and output projection
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
├── evals/                       # Evaluation framework and gold sets
└── docs/                        # Architecture and user documentation
```

## Component Responsibilities

### Protocol Layer (`protocol/`)
- **Purpose**: Single source of truth for all protocol definitions
- **Contents**:
  - `proto/` - Protobuf schemas for GovernanceEnvelope, receipts, audit events
  - `constants/` - Doctrine patterns, Doctrine (L1Doctrine)/Consensus (L2Consensus)/Notary (L3Notary) validation rules, collection schemas, ports, API paths
  - `models/` - Shared data models (agents, operator metadata, audit structures)
  - `workload_identity.go` - Go workload identity implementation
- **Invariant**: All services MUST consume protocol definitions from this layer. No local schema duplication.

### Governed Operator (g8eo)
- **Purpose**: Single binary that operates in multiple modes based on command-line flags
- **Language**: Go
- **Entry Point**: `cmd/g8eo/main.go` → `g8e` binary
- **Output**: `bin/g8e` (binary name is `g8e`, not `g8e.operator`)

#### Operational Modes

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
  - No g8e infrastructure (g8ee, client) required

**Standard Operator Mode** (default):
- No special flags required
- Responsibilities:
  - Enforce Doctrine (L1Doctrine), Consensus (L2Consensus), and Notary (L3Notary) governance gauntlet
  - Execute mutations through fail-closed L5Actuator
  - Maintain local audit vault (SQLite + Git-backed ledger)
  - Outbound mTLS connection to Gateway
  - Device-link enrollment via bootstrap service

**CLI Subcommands**:
- Commands: `platform`, `apps`, `auth`, `data`, `test`, `evals`, `security`, `setup`, `vars`
- Vault management: `--rekey-vault`, `--verify-vault`, `--reset-vault`
- Stream mode: `stream` subprocess for approval UI


## Dependency Flow

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

## Critical Invariants

1. **Protocol-first**: All wire formats and governance rules defined in `protocol/` only
2. **Single binary**: One `g8e` binary operates in multiple modes based on flags
3. **Mode-specific behavior**: Gateway mode includes PDP+execution; MCP serve enables BYO clients; OpenClaw enables external orchestration
4. **Fail-closed**: All mutations must pass L1Doctrine/L2Consensus/L3Notary before execution via L4Warden
5. **Local audit**: Operator maintains tamper-evident audit vault on host (SQLite + Git-backed ledger via go-git)
6. **BYO-capable**: Protocol supports any conforming producer via MCP/A2A/tool calls
7. **Wire format**: Canonical JSON (protojson) for UniversalEnvelope on all client-facing surfaces; binary protobuf only for internal storage
8. **Session validation**: Audit vault rejects events without valid operator_session_id or unknown sessions (fail-closed)

## Build Artifacts

- `g8e` - Static Go binary (single binary for all modes, output to `bin/g8e`)
- Protocol constants exported to JSON and Python via `cmd/exporter`
- Supporting tools: `chaos_tester`, `exporter`, `uap-ping`

## Entry Points

- CLI: `./g8e` (shell script in repo root, delegates to `bin/g8e`)
- Binary: `bin/g8e` (compiled from `cmd/g8eo/main.go`)
- Main source: `cmd/g8eo/main.go`
- Protocol constants: `cmd/exporter/main.go`
