# Architecture

## Overview

g8ee is part of a 3-component g8e platform:

- **Governance Gateway (g8eg)** — Central Policy Decision Point (PDP). Owns platform-level PKI, coordination, Pub/Sub, and transaction validation/suspension. Runs in Gateway mode (`--doctrine`, `--consensus`, or `--notary`).
- **Governed Operator (g8eo)** — Host-side Policy Execution Point (PEP). Enforces protocol compliance, verifies L1/L2/L3 signatures, and executes transactions via the Actuator stage. Runs on target hosts.
- **g8e Agentic Ensemble (g8ee)** — Optional g8e-compliant agentic ensemble. Acts as an L2 producer, emitting typed, signed GovernanceEnvelope transactions to the Gateway for validation/approval.

## Protocol Surfaces

The Gateway exposes two ports:

- **HTTP (8080)** — Trust bundle download, device-link enrollment, CSR signing (plain HTTP, no TLS)
- **HTTPS (8443)** — Governance envelopes, MCP/A2A APIs, document store, WebSocket pub/sub, browser login, WebAuthn challenge, OOB approval UI (mTLS + public surface multiplexed)

## Code Structure

```
g8ee/
├── app/                 # Main application code
│   ├── clients/         # External service clients
│   ├── constants/       # Application constants (sourced from g8e.constants)
│   ├── db/              # Database models and operations
│   ├── llm/             # LLM provider implementations
│   ├── models/          # Pydantic models (subclass g8e.models bases)
│   ├── proto/           # Generated protobuf stubs (gitignored, make proto)
│   ├── routers/         # FastAPI route handlers
│   ├── services/        # Business logic services
│   └── utils/           # Utility functions
├── tests/               # Unit and integration tests
├── evals/               # Evaluation suite and benchmarks
├── config/              # Configuration files
├── scripts/             # Utility scripts
├── docs/                # Documentation
└── pyproject.toml       # Python project configuration
```

## Model Hierarchy

g8ee models inherit from the `g8e` protocol package (`g8e>=1.5.6`):

- **`G8eBaseModel`** → from `g8e.models.base` (re-exported via `app.models.base`)
- **`RequestContext`** → subclasses `g8e.models.context.RequestContext`, adds `operator_id` and `operator_session_id`
- **`BoundOperator`** → directly re-exported from `g8e.models.context`
- **`ChatMessageRequest`** → multiple inheritance: g8e base + `RequestOverrides`
- **`ResourceCreationRequest`**, **`ChatStartedResponse`** → directly re-exported from `g8e.models.internal_api`
- **Settings models** → subclassed from `g8e.models.settings`
- **SSE wire models** → `SessionEventWire` and `BackgroundEventWire` subclass `g8e.models.events`; all 11 payload classes re-exported from `g8e.models.events`
- **Constants** → sourced from `g8e.constants` accessors (`collection()`, `kv_key()`, `channel()`, `intent()`, `prompt()`)
- **Protobuf stubs** → generated from `protocol/proto/g8e/` via `make proto` into `app/proto/` (gitignored)

## Governance Architecture

All business-critical DB writes (cases, investigations, memories, reputation) go through `GovernanceClient` governance envelopes, which:

1. Fetch the current state Merkle root from the Gateway
2. Build a g8e-compliant UAP envelope with transaction hash (SHA256 of canonical JSON), nonce, and L3 proof (mTLS certificate fingerprint)
3. Submit to the Gateway's `POST /api/v1/governance/envelopes` endpoint for L1/L2/L3 verification
4. Return a signed `ActionReceipt` as proof of execution

The `GovernanceClient` uses mTLS for all Gateway communication, passing CA cert, client cert, and client key paths to the HTTP session.

## Related

- [Governance](governance.md) — 3-layer governance model
- [Agents](agents.md) — Agent hierarchy and personas
- [Protocol](protocol.md) — Protocol reference for Gateway integration
