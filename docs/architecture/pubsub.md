---
title: Pub/Sub Channels
parent: Architecture
---

# g8e Pub/Sub Architecture

Last Updated: 2026-05-10
Version: v0.2.2

The g8e platform utilizes a high-performance, WebSocket-based Pub/Sub system for all real-time inter-component communication. This decoupled architecture allows the central engine (`g8ee`) to orchestrate distributed agents (`g8eo` Operators) across heterogeneous environments without direct network visibility.

---

## The Lifecycle of a Session

The lifecycle of an Operator session is entirely governed by its pub/sub interactions.

### 1. Bootstrap & Authentication
When an Operator starts, it may not yet have a persistent session. It identifies itself via a **Bootstrap Handshake**:
- **Request**: Operator publishes to an ephemeral `auth.publish:session:{hash}` channel.
- **Validation**: `g8ee` (via `SessionAuthListener`) validates the request and responds with a bootstrap configuration (API keys, resource limits, certs) on `auth.response:session:{hash}`.
- **Finalization**: Once authenticated, the Operator transitions to its dedicated per-session channels.

### 2. Activity Monitoring (Heartbeats)
Operators maintain their `AVAILABLE` status by publishing periodic signals to their `heartbeat` channel. 
- **Efficiency**: `g8ee` uses a single **Pattern Subscription** (`heartbeat:*`) to observe all active Operators simultaneously, avoiding the race conditions and overhead of per-session registration.
- **Resource Tracking**: Heartbeats contain real-time CPU, Memory, and Disk metrics used for task scheduling.

### 3. Command Orchestration
The primary function of the platform is the delivery and execution of commands:
- **Dispatch**: `g8ee` publishes serialized `UniversalEnvelope` bytes to the Operator's `cmd` channel.
- **Tracking**: `g8ee` registers an `asyncio.Future` correlated by `execution_id`.
- **Completion**: The Operator executes the typed payload and publishes serialized `UniversalEnvelope` result bytes back to the `results` channel. The platform matches the `execution_id`, completes the future, and returns the result to the caller.

---

## Channel Taxonomy

### Per-Operator-Session Channels
Canonical format: `{prefix}:{operator_id}:{operator_session_id}`

| Prefix | Source | Destination | Purpose |
| :--- | :--- | :--- | :--- |
| `cmd` | `g8ee` | `g8eo` | Command delivery and system control requests. |
| `results` | `g8eo` | `g8ee` | Return of stdout, stderr, exit codes, and file artifacts. |
| `heartbeat` | `g8eo` | `g8ee` | Signal of life and resource utilization. |

### Platform Broadcast Channels
Broadcasting system-wide state changes to all listeners (primarily for the UI).

| Channel | Purpose |
| :--- | :--- |
| `operator_heartbeats` | Aggregated stream of all heartbeats for dashboard monitoring. |
| `sse_events` | Real-time event stream for browser-based UI updates. |
| `system_events` | High-priority system notifications (e.g., config changes, outages). |
| `g8eo_results` | **Deprecated**. Use per-session `results` channels. |

---

## Technical Invariants

### 1. Single Source of Truth
All channel prefixes and segment counts are defined in `@/shared/constants/channels.json`. 
- **Never hand-roll**: Do not use string interpolation (e.g., `f"{prefix}:{id}"`).
- **Use typed wrappers**: Python uses `OperatorChannel` in `@/components/g8ee/app/constants/channels.py`. Go uses constructors in `components/g8eo/constants/channels.go`.

### 2. Bounded Parsing
The `operator_session_id` may contain the separator character (`:`). To prevent data loss, always use a **bounded split** with a maximum of 2 splits when parsing a 3-segment channel.
```python
# Canonical parsing logic

Last Updated: 2026-05-10
Version: v0.2.2
parts = channel.split(":", 2) # Ensures the session ID remains intact
```

### 3. Resource Management
- **Refcounted Subscriptions**: The `PubSubClient` (Python) tracks how many services are interested in a channel. A channel is only physically `UNSUBSCRIBE`d from the broker when its local refcount reaches zero.
- **Auto-Cleanup**: `SessionAuthListener` enforces a 300s TTL on ephemeral auth listeners to prevent memory leaks from abandoned bootstrap attempts.

### 4. Protocol Payloads
Operator command and result channel payloads carry serialized g8e protocol `UniversalEnvelope` bytes whose inner payloads are typed `operator.proto` messages. Pub/sub channels provide routing only; `UniversalEnvelope` provides the protocol contract by binding `event_type`, payload bytes, operator/session context, state root, and L1/L2/L3 governance metadata. The envelope and governance contract is documented in [Protocol](protocol.md).

---

## Safety & Governance

Pub/Sub is the only way `g8ee` communicates with `g8eo`. This provides a critical security boundary:
- **No Inbound TCP to Operators**: Operators only need outbound WSS to the platform.
- **Audit Logging**: Every command envelope carries correlation fields and governance evidence for compliance and local LFAA retention.
- **Type Safety**: Operator command and result payloads are validated against generated Protobuf models before processing.
- **Governance Binding**: Auto-approval is L3 authorization state only; it never bypasses L1 Technical Bedrock or L2 Consensus.
