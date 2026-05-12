---
title: Pub/Sub Channels
parent: Architecture
---

# g8e Pub/Sub Architecture

Last Updated: 2026-05-12
Version: v0.2.4

The g8e platform utilizes a high-performance, WebSocket-based Pub/Sub system for all real-time inter-component communication. This decoupled architecture allows a central engine (bundled `g8ee` or BYO agent) to orchestrate distributed agents (`g8eo` Operators) across heterogeneous environments without direct network visibility.

As of v0.2.0, the "g8es" component abstraction has been removed. The host listener is now the **Operator binary running in `--listen` mode**, which acts as the WebSocket broker for the platform.

---

## The Lifecycle of a Session

The lifecycle of an Operator session is entirely governed by its pub/sub interactions.

### 1. Bootstrap & Authentication
When an Operator starts, it identifies itself via a **Bootstrap Handshake**:
- **Request**: Operator publishes a `GovernanceEnvelope` to an ephemeral `auth.publish:session:{hash}` channel.
- **Validation**: The authentication authority (bundled `g8ee` or a substrate-native service) validates the request and responds with a bootstrap configuration (API keys, resource limits, certs) on `auth.response:session:{hash}`.
- **Finalization**: Once authenticated, the Operator transitions to its dedicated per-session channels.

### 2. Activity Monitoring (Heartbeats)
Operators maintain their `AVAILABLE` status by publishing periodic signals to their `heartbeat` channel. 
- **Efficiency**: Clients use a single **Pattern Subscription** (`heartbeat:*`) to observe all active Operators simultaneously.
- **Resource Tracking**: Heartbeats contain real-time CPU, Memory, and Disk metrics used for task scheduling and fleet monitoring.

### 3. Command Orchestration
The primary function of the platform is the delivery and execution of commands:
- **Dispatch**: A client publishes serialized `GovernanceEnvelope` bytes to the Operator's `cmd` channel.
- **Execution**: The Operator executes the typed payload and publishes serialized `GovernanceEnvelope` result bytes back to the `results` channel.
- **Completion**: The client matches the `execution_id` and processes the result.

---

## Channel Taxonomy

### Per-Operator-Session Channels
Canonical format: `{prefix}:{operator_id}:{operator_session_id}`

| Prefix | Source | Destination | Purpose |
| :--- | :--- | :--- | :--- |
| `cmd` | Client | `g8eo` | Command delivery and system control requests. |
| `results` | `g8eo` | Client | Return of stdout, stderr, exit codes, and file artifacts. |
| `heartbeat` | `g8eo` | Client | Signal of life and resource utilization. |

### Platform Broadcast Channels
Broadcasting system-wide state changes to all listeners (primarily for the UI).

| Channel | Purpose |
| :--- | :--- |
| `operator_heartbeats` | Aggregated stream of all heartbeats for dashboard monitoring. |
| `sse_events` | Real-time event stream for browser-based UI updates. |
| `system_events` | High-priority system notifications (e.g., config changes, outages). |
| `g8eo_results` | **Deprecated**. Use per-session `results` channels. |

---

## Universal Envelope & Protocol

All inter-component communication is governed by the **Universal Envelope** protocol defined in `@/shared/proto/common.proto`.

### Why the Envelope?
The envelope provides a cryptographic and technical contract that separates routing from logic:
- **Identity Binding**: Every message is bound to an `operator_id`, `operator_session_id`, and `investigation_id`.
- **State Integrity**: Includes a `state_merkle_root` for BFT verification, ensuring commands are based on the latest fleet state.
- **Governance Metadata**: Carries L1/L2/L3 metadata required for compliance and safety.
- **Type Safety**: Inner payloads are strictly typed `operator.proto` messages, eliminating parsing ambiguity.

---

## Governance & Safety

Pub/Sub is the primary channel for governed client-to-operator communication. This provides a critical security boundary:

### 1. No Inbound TCP
Operators only require outbound WSS connectivity to the platform. No ports are opened on the target host, significantly reducing the attack surface.

### 2. 3-Layer Validation Hierarchy
- **L1 Technical Bedrock**: Hard gates (e.g., `sudo`, `su`) enforced by code validators via Protobuf reflection.
- **L2 Consensus (Tribunal)**: 5-agent plurality verification. Signatures are verified using a shared `auditor_hmac_key`.
- **L3 Authorization (Approval)**: Human-in-the-loop by default. Auto-approval is authorization metadata and never bypasses L1 or L2.

### 3. Audit & Transparency
Every command envelope carries correlation fields and governance evidence. This ensures that every action taken on an endpoint is attributable to a specific human intent, AI consensus, and technical gate.

---

## Technical Invariants

### 1. Single Source of Truth
All channel prefixes and segment counts are defined in `@/shared/constants/channels.json`. 
- **Never hand-roll**: Do not use string interpolation.
- **Use typed wrappers**: Python uses `OperatorChannel` in `@/components/g8ee/app/constants/channels.py`. Go uses constructors in `components/g8eo/constants/channels.go`.

### 2. Bounded Parsing
The `operator_session_id` may contain the separator character (`:`). To prevent data loss, always use a **bounded split** with a maximum of 2 splits when parsing a 3-segment channel.
```python
# Canonical parsing logic in g8ee
parts = channel.split(":", 2) # Ensures the session ID remains intact
```

### 3. Resource Management
- **Refcounted Subscriptions**: The `PubSubClient` tracks how many services are interested in a channel. A channel is only physically `UNSUBSCRIBE`d from the broker when its local refcount reaches zero.
- **Auto-Cleanup**: `SessionAuthListener` enforces a 300s TTL on ephemeral auth listeners to prevent memory leaks.
- **Fail-Closed Behavior**: Envelopes missing an ID or carrying malformed payloads are rejected at the dispatcher level before reaching any service handlers.
