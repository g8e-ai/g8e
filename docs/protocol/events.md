---
title: Events
parent: Architecture
---

# g8e Event Specification

Last Updated: 2026-05-12
Version: v0.2.4

The g8e platform uses a unified, hierarchical event system to drive state transitions and lifecycle signals. All cross-component traffic is governed by the **Governance Envelope**, a UAP JSON transport wrapper that carries governance metadata, state roots, and typed payloads.

---

## Architecture & Transport

Events in g8e are the heartbeat of the system's reactivity. The architecture follows a decentralized model where a conforming Operator (reference: `g8eo`) provides the primary pub/sub transport and persistence hub for all clients.

### 1. The Reference Hub: `g8eo`
The reference Operator in **Listen Mode** serves as the central event broker and coordination point for the bundled deployment.
- **Operator Pub/Sub**: A WebSocket-based (WSS) backbone for real-time communication between all components.
- **Audit Logging**: `g8eo` captures and persists events into the host-authoritative audit vault.

### 2. Event Producers & Consumers
- **Clients (bundled `g8ee`, or BYO)**: Emit intent, chat streams, and governance proofs. Consume real-time updates and results.
- **Managed Operators (`g8eo`)**: Emit command results, heartbeats, and filesystem updates.
- **Dashboard (``)**: Provides a reference UI and relay for Server-Sent Events (SSE) to browser-based clients.

### 2. Event Producers
- **`g8ee` (Engine)**: The AI reasoning engine. Emits chat streams, tool requests, and Tribunal consensus results.
- **`g8eo` (Operator)**: The remote execution agent. Emits command results, heartbeats, and filesystem updates.
- **`` (Dashboard)**: The platform service. Emits lifecycle events (auth, session management) and proxies third-party events.

### 3. Delivery Lifecycle
1. **Emission**: A producer serializes a **Governance Envelope** (UAP JSON) containing a typed Protobuf payload.
2. **Ingestion**: The substrate (`g8eo`) or relay (``) receives the envelope via Pub/Sub or HTTP.
3. **Routing**: The event is distributed based on the **Routing Tuple** defined in the envelope.
4. **Delivery**: The event is pushed to the client via WSS/SSE or persisted to the audit log.

---

## The Governance Envelope

The `GovernanceEnvelope` (UAP JSON, defined in `protocol/proto/common.proto`) is the canonical wrapper for all platform transactions.

| Field | Description |
|-------|-------------|
| `id` | Unique UUID v4 for the message. |
| `event_type` | Canonical event string (e.g., `g8e.v1.operator.command.completed`). |
| `state_merkle_root` | The Merkle root of the fleet state at the time of generation (used for BFT verification). |
| `governance` | L1 (Technical), L2 (Tribunal), and L3 (Human) metadata/signatures. |
| `payload` | Serialized bytes of the typed Protobuf message (e.g., `CommandRequested`). |

---

## The Routing Tuple

To ensure events reach the correct context, every event carries a routing tuple that governs its delivery and Terminal placement.

| Field | Description | Required For |
|-------|-------------|--------------|
| `web_session_id` | Unique ID of the browser connection. | Point-to-point delivery to a specific tab. |
| `operator_id` | Unique ID of the target operator slot. | Routing to a specific remote host. |
| `operator_session_id`| Unique ID for the current operator process. | Correlation of long-running command streams. |
| `case_id` | Correlation ID for the active case. | Contextual grouping and audit log association. |
| `investigation_id` | Correlation ID for the active investigation. | Chat history and state recovery. |

---

## Governance & Safety

### 1. L1: Technical Bedrock (Hard Gates)
`g8eo` enforces L1 safety using Protobuf reflection. It inspects the `forbidden_patterns` option on incoming message fields (e.g., `CommandRequested.command`) and rejects any payload containing prohibited strings like `sudo` or `rm -rf /`.

### 2. L2: Consensus (The Tribunal)
The Tribunal attaches an ED25519 signature from a trusted signer over `transaction_hash|true`. `g8eo` verifies this signature before executing any command, ensuring the instruction originated from a valid consensus group.

### 3. L3: Authorization (Human Approval)
Human-in-the-loop signatures (captured via Passkeys) are carried in the `L3Metadata` field. For benign diagnostic commands, an `auto_approved` flag may be set, but it never bypasses L1 or L2 gates.

### 4. BFT State Verification
To prevent "stale state" attacks, `g8eo` compares the `state_merkle_root` in the incoming envelope with its local ledger root. If they mismatch, the command is rejected because the AI's reasoning was based on an outdated view of the system.

---

## Core Pipelines

### 1. LLM Chat & Iterations
The chat pipeline uses granular events to expose the AI's "thought process":
- `g8e.v1.ai.llm.chat.iteration.started`: Start of an AI reasoning turn.
- `g8e.v1.ai.llm.chat.iteration.thinking.started`: AI has entered an internal chain-of-thought phase.
- `g8e.v1.ai.llm.chat.iteration.text.chunk.received`: Tokens streamed to the UI.
- `g8e.v1.ai.llm.chat.iteration.completed`: End of a reasoning turn.

### 2. Operator Command Lifecycle
Standardized request/response pattern for auditability and reactivity:
1. `g8e.v1.operator.command.requested`: AI/User requests execution.
2. `g8e.v1.operator.command.started`: Operator acknowledges and forks the process.
3. `g8e.v1.operator.command.status.updated.running`: Real-time stdout/stderr increments.
4. `g8e.v1.operator.command.completed` / `failed`: Final execution result.

---

## Event Name Specification

### Name Format
Events follow a hierarchical, dot-separated naming convention:
```
g8e.v1.<domain>.<resource>[.<sub-resource>...].<action>
```
- **Domain**: Namespace (`app`, `operator`, `ai`, `platform`, `source`).
- **Action**: Always a **past-tense** verb (`created`, `failed`) or state (`active`).

### Canonical Truth
- `protocol/constants/events.json`: The single source of truth for event name strings.
- `protocol/proto/`: The canonical schema source for envelopes and typed payloads.
- **Python**: `EventType` StrEnum in `app/constants/events.py`.
- **Go**: Constants in `services/g8eo/constants/events.go`.
- **Node.js**: Constants in `protocol/constants/events.json` (shared via symlink or copy).

---

## Adding New Events

1. **Update Canonical Truth**: Add the new event string to `protocol/constants/events.json`.
2. **Define Schema**: If the event carries a payload, define a message in `protocol/proto/operator.proto`.
3. **Propagate**: Update the respective constant enums in `g8ee` and `g8eo`.
4. **Verify**: Ensure the name follows the hierarchical past-tense rule.


