# Protocol

## Overview

The g8e protocol defines how g8ee communicates with the Governance Gateway (g8eg) and Governed Operator (g8eo). All communication uses typed, signed GovernanceEnvelope transactions.

## GovernanceEnvelope

The core protocol unit. A typed transaction container carrying:

- **Payload** — The action or decision being proposed
- **Signatures** — Ed25519 signatures from participating agents
- **Metadata** — Consensus round info, reputation stakes, timestamps
- **Action type** — Categorized action being requested (see [Constants](constants.md))

## Gateway Communication

### HTTP Surface (Port 8080)

- Trust bundle download
- Device-link enrollment
- CSR signing

### HTTPS Surface (Port 8443)

- Governance envelope submission
- MCP/A2A APIs
- Document store operations
- WebSocket pub/sub
- WebAuthn challenges
- OOB approval UI

## Transaction Lifecycle

```
Agent proposes action → L1 validation → L2 consensus (Tribunal) → L3 authorization (if required) → Gateway approval → Operator execution
```

## Related

- [Governance](governance.md) — 3-layer governance model
- [Constants](constants.md) — Action type mappings and protocol constants
- [Architecture](architecture.md) — Component overview
