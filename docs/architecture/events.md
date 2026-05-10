---
title: Events
parent: Architecture
---

# g8e Event Naming Specification

Last Updated: 2026-05-07
Version: v0.2.0

The g8e platform uses unified, hierarchical event names to identify state transitions and lifecycle signals. Operator command/result traffic is governed by the g8e protocol: serialized Protobuf `UniversalEnvelope` bytes carry `event_type`, typed `operator.proto` payload bytes, operator/session context, state roots, and L1/L2/L3 governance metadata.

---

## Architecture & Transport

Events in g8e are state transitions and lifecycle signals that drive the system's reactivity. The architecture follows a hub-and-spoke model with `g8ed` at the center.

### 1. The Central Hub: `g8ed`
`g8ed` (Node.js) serves as the central event router and composition root. It manages two primary transport layers:
- **Internal HTTP Push**: Receives events from `g8ee` (Python) via standard POST requests.
- **OPERATOR Pub/Sub**: A WebSocket-based backbone for communication with `g8eo` (Go) operators. `g8ed` acts as a proxy, translating Pub/Sub messages into SSE for the Terminal.
- **Server-Sent Events (SSE)**: Pushes real-time updates to human-interactive clients.

### 2. Event Producers
- **`g8ee` (Python)**: The AI reasoning engine. Emits chat streams, tool requests, and Tribunal consensus results.
- **`g8eo` (Go)**: The operator agent. Emits command execution results, heartbeats, and status updates via Pub/Sub.
- **`g8ed` (Node.js)**: The platform service. Emits lifecycle events (auth, session management) and proxies events from other components.

### 3. Delivery Lifecycle
1. **Emission**: A producer constructs a typed event using the **Routing Tuple**.
2. **Ingestion**: `g8ed` receives the event via HTTP or Pub/Sub.
3. **Routing**: `SSEService` determines the destination based on `web_session_id` or `user_id`.
4. **Delivery**: The event is serialized to the SSE wire format and pushed to the client.

---

## The Routing Tuple

To ensure events reach the correct context, every event carries a routing tuple that governs its delivery and Terminal placement.

| Field | Description | Required For |
|-------|-------------|--------------|
| `web_session_id` | Unique ID of the browser connection. | Point-to-point delivery (`SessionEvent`). |
| `user_id` | Unique ID of the user. | User-wide fan-out (`BackgroundEvent`). |
| `case_id` | Correlation ID for the active case. | Contextual grouping and audit log association. |
| `investigation_id` | Correlation ID for the active investigation. | Chat history and state recovery. |

---

## Event Classification

### Session Events
Intended for a specific active session (e.g., a specific tab). These are used when the triggering action originated from a known `web_session_id`.
- **Examples**: AI streaming text, tool results, and Proof of Human Presence (PHP) requests.
- **Model**: `SessionEvent` in `g8ee`.

### Background Events
System-initiated events with no specific active session. `g8ed` fans these out to **every active SSE session** owned by the `user_id`.
- **Examples**: Global notifications, operator status changes (e.g., `OPERATOR_STATUS_UPDATED_ACTIVE`).
- **Model**: `BackgroundEvent` in `g8ee`.

---

## Core Pipelines

### 1. LLM Chat & Iterations
The chat pipeline uses a granular event sequence to expose the AI's "thought process":
- `LLM_CHAT_ITERATION_STARTED`: Start of an AI reasoning turn.
- `LLM_CHAT_ITERATION_TEXT_CHUNK_RECEIVED`: Individual tokens streamed to the UI.
- `LLM_CHAT_ITERATION_THINKING_STARTED`: Signals the AI has entered an internal chain-of-thought phase.
- `LLM_CHAT_ITERATION_COMPLETED`: End of a reasoning turn.

### 2. Universal Tool Lifecycle
All tools follow a standardized request/response pattern for auditability:
1. `LLM_TOOL_*_REQUESTED`: The AI requests tool execution.
2. `LLM_TOOL_*_RECEIVED`: The execution environment acknowledges the request.
3. `LLM_TOOL_*_COMPLETED` / `FAILED`: The final result or error.

### 3. The Tribunal
Consensus-based command generation emits discrete events for each internal stage:
- `TRIBUNAL_SESSION_STARTED`: Initialization of the consensus group.
- `TRIBUNAL_VOTING_STARTED`: Collection of command candidates from members.
- `TRIBUNAL_VOTING_CONSENSUS_REACHED`: Successful agreement on a command.
- `TRIBUNAL_VOTING_DISSENT_RECORDED`: Capture of minority member disagreements.
- `TRIBUNAL_SESSION_COMPLETED`: Final validated command is ready for dispatch.

---

## Governance & Safety

### Log-First, Act-After (LFAA)
Critical lifecycle events (commands, file edits, AI decisions) are recorded by the `AuditService` **before** the action is executed. This ensures a permanent, tamper-resistant record even if the operation fails or the component crashes.

### The Sentinel
Before `g8eo` executes a command, it passes through **The Sentinel** (pre-execution analysis).
1. If a threat is detected, `g8eo` emits an `operator.command.failed` event.
2. The `error_type` is set to `sentinel_blocked`.
3. The violation is recorded in the audit vault before the command is discarded.

---

## Event Name Specification

### Name Format
Events follow a hierarchical, dot-separated naming convention:
```
g8e.v<version>.<domain>.<resource>[.<sub-resource>...].<action>
```
- **Protocol prefix**: `g8e.v1`
- **Domain**: Namespace (`app`, `operator`, `ai`, `platform`, `source`)
- **Action**: Always a **past-tense** verb (`created`, `failed`) or state (`active`).

### Canonical Truth
`shared/constants/events.json` is the single source of truth for event names. `shared/proto/` is the canonical schema source for g8e protocol envelopes and typed operator payloads.
- **`g8ee` (Python)**: `EventType` Enum in `app/constants/events.py`.
- **`g8ed` (Node.js)**: `EventType` object in `public/js/constants/events.js`.
- **`g8eo` (Go)**: Event constants in `constants/events.go`.

---

## Adding New Events

1. **Update Canonical Truth**: Add the new event string to `shared/constants/events.json`.
2. **Propagate to Components**: Update the respective constants in `g8ee`, `g8ed`, and `g8eo`.
3. **Verify Wire Value**: Ensure the string matches the dot-joined hierarchy and follows the past-tense rule.

