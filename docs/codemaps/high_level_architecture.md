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
├── services/
│   └── g8eo/                    # MANDATORY - Single binary with multiple operational modes
│       ├── cmd/g8eo/            # Main entry point (g8e binary)
│       ├── internal/            # All service implementations
│       │   ├── services/        # Gateway, governance, execution, MCP, pubsub, etc.
│       │   ├── protocol/        # Internal protocol bindings
│       │   └── constants/       # Generated constants from protocol/
│       └── protocol/            # Protocol constants export (JSON/Python)
│
├── evals/                       # Evaluation framework and gold sets
└── docs/                        # Architecture and user documentation
```

## Component Responsibilities

### Protocol Layer (`protocol/`)
- **Purpose**: Single source of truth for all protocol definitions
- **Contents**:
  - `proto/` - Protobuf schemas for GovernanceEnvelope, receipts, audit events
  - `constants/` - Doctrine patterns, L1/L2/L3 validation rules, collection schemas, ports, API paths
  - `models/` - Shared data models (agents, operator metadata, audit structures)
  - `workload_identity.go` - Go workload identity implementation
- **Invariant**: All services MUST consume protocol definitions from this layer. No local schema duplication.

### Governed Operator (`services/g8eo/`)
- **Purpose**: Single binary that operates in multiple modes based on command-line flags
- **Language**: Go
- **Entry Point**: `cmd/g8eo/main.go` → `g8e` binary

#### Operational Modes

**Gateway Mode** (platform persistence + pub/sub broker):
- Flags: `--doctrine`, `--consensus`, or `--notary`
- Responsibilities:
  - Admission APIs for envelope submission
  - mTLS/PKI management and device-link lifecycle
  - Replay protection and session scoping
  - State-root distribution
  - SQLite persistence for platform state
  - In-process command service as sovereign execution gateway

**MCP Serve Mode** (BYO client proxy):
- Flag: `--mcp-serve`
- Responsibilities:
  - MCP stdio JSON-RPC proxy to Operator's mTLS HTTP API
  - Enables standard MCP clients to interact with g8e

**OpenClaw Node Host Mode**:
- Flag: `--openclaw`
- Responsibilities:
  - Connects to OpenClaw Gateway via WebSocket
  - Advertises system.run and system.which capabilities
  - Executes shell commands on demand

**Standard Operator Mode** (default):
- No special flags required
- Responsibilities:
  - Enforce L1/L2/L3 governance gauntlet
  - Execute mutations through fail-closed Actuator
  - Maintain local audit vault (Git-backed)
  - Outbound mTLS connection to Gateway


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
4. **Fail-closed**: All mutations must pass L1/L2/L3 before execution
5. **Local audit**: Operator maintains tamper-evident audit vault on host (Git-backed)
6. **BYO-capable**: Protocol supports any conforming producer via MCP/A2A/tool calls

## Build Artifacts

- `g8e` - Static Go binary from `services/g8eo/` (single binary for all modes)
- Protocol constants exported to JSON and Python via `services/g8eo/cmd/exporter`

## Entry Points

- CLI: `./g8e` (shell script in repo root, symlink to `services/g8eo/build/linux-amd64/g8e`)
- Binary: `services/g8eo/build/linux-amd64/g8e`
- Main source: `services/g8eo/cmd/g8eo/main.go`
