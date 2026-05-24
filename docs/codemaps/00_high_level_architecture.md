# g8e High-Level Architecture Codemap

## Overview

g8e is organized as a protocol-first substrate with three service components. The protocol layer is the single source of truth for all wire formats, governance contracts, and shared constants. Services implement the protocol at different layers of the governance stack.

```text
g8e/
├── protocol/                    # MANDATORY - Protocol definitions (shared truth)
│   ├── proto/                   # Protobuf schemas (wire format source of truth)
│   ├── constants/               # Governance constants, doctrine, collections
│   ├── models/                  # Shared data models (agents, audit, etc.)
│   └── python/                  # Python protocol bindings
│
├── services/                    # Service implementations
│   ├── g8eo/                    # MANDATORY - Governed Operator (enforcement point)
│   ├── g8ee/                    # OPTIONAL - g8e-Compliant Agentic Ensemble
│   └── g8eg/                    # OPTIONAL - Governance Gateway (PDP)
│
├── evals/                       # Evaluation framework and gold sets
└── docs/                        # Architecture and user documentation
```

## Component Responsibilities

### Protocol Layer (`protocol/`)
- **Purpose**: Single source of truth for all protocol definitions
- **Contents**:
  - `proto/` - Protobuf schemas for GovernanceEnvelope, receipts, audit events
  - `constants/` - Doctrine patterns, L1/L2/L3 validation rules, collection schemas
  - `models/` - Shared data models (agents, operator metadata, audit structures)
  - `python/` - Python protocol package for g8ee integration
- **Invariant**: All services MUST consume protocol definitions from this layer. No local schema duplication.

### Governed Operator (`services/g8eo/`)
- **Purpose**: Sovereign execution boundary and enforcement point
- **Language**: Go
- **Key Responsibilities**:
  - Enforce L1/L2/L3 governance gauntlet
  - Execute mutations through fail-closed Actuator
  - Maintain local audit vault (Git-backed)
  - Serve as MCP server for BYO clients
  - Outbound-only mTLS tunnel to Gateway
- **Entry Point**: `cmd/g8eo/main.go` → `g8e.operator` binary

### g8e-Compliant Agentic Ensemble (`services/g8ee/`)
- **Purpose**: Reference implementation of a g8e-compliant producer
- **Language**: Python
- **Key Responsibilities**:
  - ReAct loop over agent hierarchy (Triage, Sage, Tribunal, Warden, Auditor, Nemesis)
  - Reach L2 consensus via heterogeneous model ensemble
  - Form and sign GovernanceEnvelope
  - Submit envelopes to Gateway for admission
- **Entry Point**: `app/main.py` → FastAPI application

### Governance Gateway (`services/g8eg/`)
- **Purpose**: Policy Decision Point (PDP) and admission broker
- **Language**: Go
- **Key Responsibilities**:
  - Admission APIs for envelope submission
  - mTLS/PKI management and device-link lifecycle
  - Replay protection and session scoping
  - State-root distribution to Operators
  - Fan-out to registered Operators
- **Entry Point**: Built from `services/g8eo/` Makefile as `g8e.gateway` binary target
- **Note**: g8eg has no separate cmd/ directory; binary is compiled alongside g8eo

## Dependency Flow

```text
┌─────────────────────────────────────────────────────────────┐
│                        PROTOCOL LAYER                        │
│  (proto, constants, models - consumed by all services)      │
└─────────────────────────────────────────────────────────────┘
                              │
              ┌───────────────┼───────────────┐
              ▼               ▼               ▼
        ┌──────────┐   ┌──────────┐   ┌──────────┐
        │   g8ee   │──▶│   g8eg   │◀──│   g8eo   │
        │(Producer)│   │  (PDP)   │   │(PEP/Exec)│
        └──────────┘   └──────────┘   └──────────┘
              │               │               │
              └───────────────┴───────────────┘
                              │
                    g8e submits signed
                  GovernanceEnvelope to g8eg
                    g8eo pulls via mTLS
```

## Critical Invariants

1. **Protocol-first**: All wire formats and governance rules defined in `protocol/` only
2. **Operator sovereignty**: g8eo is the only component with execution authority
3. **Outbound-only**: g8eo opens mTLS tunnel to g8eg; no inbound listeners
4. **Fail-closed**: All mutations must pass L1/L2/L3 before execution
5. **Local audit**: g8eo maintains tamper-evident audit vault on host
6. **BYO-capable**: Protocol supports any conforming producer, not just g8ee

## Build Artifacts

- `g8e.operator` - Static Go binary (~4MB) from `services/g8eo/`
- `g8e.gateway` - Static Go binary from `services/g8eo/` Makefile (build-g8eg target)
- `g8e-protocol` - Python package from `protocol/python/`
- `g8ee` - Python application (requires virtual environment)

## Entry Points

- CLI: `./g8e` (shell script in repo root)
- Operator binary: `services/g8eo/build/linux-amd64/g8e.operator`
- Gateway binary: `services/g8eo/build/linux-amd64/g8e.gateway`
- Ensemble app: `services/g8ee/app/main.py`
