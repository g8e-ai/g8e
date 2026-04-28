---
title: Events
parent: Architecture
---

# g8e Event Protocol Specification

The g8e platform uses a unified, hierarchical event protocol for all inter-component communication. This system ensures that AI agents, operators, and user interfaces remain synchronized in real-time across a distributed environment.

---

## Architecture & Lifecycle

Events in g8e are not just "messages"—they are state transitions and lifecycle signals that drive the system's reactivity.

### 1. The Central Hub: `g8ed`
`g8ed` (Node.js) serves as the central event router. It maintains two primary transport layers:
- **Server-Sent Events (SSE)**: Pushes real-time updates to browser clients.
- **G8ES Pub/Sub**: A WebSocket-based backbone for bidirectional communication with `g8eo` (Go) operators.

### 2. Event Producers
- **`g8ee` (Python)**: The primary AI logic engine. It emits events (like chat chunks, tool calls, and tribunal results) to `g8ed` via internal HTTP push.
- **`g8eo` (Go)**: The operator agent. It emits command results, heartbeats, and status updates to `g8ed` via the results channel of the G8ES Pub/Sub.

### 3. The Lifecycle of an Event
1. **Emission**: A producer (e.g., `g8ee`) constructs a typed event (e.g., `SessionEvent`).
2. **Routing**: `g8ed` receives the event and uses the **Routing Tuple** (see below) to determine the destination.
3. **Delivery**: 
   - If the target is a browser, `SSEService` pushes it via an open SSE connection.
   - If the target is an operator, `PubSubBroker` routes it to the specific WebSocket channel.

---

## The Routing Tuple

To ensure events reach the correct context, every event carries a routing tuple:

| Field | Description | Required For |
|-------|-------------|--------------|
| `web_session_id` | Unique ID of the browser connection. | Point-to-point delivery (SessionEvent). |
| `user_id` | Unique ID of the user. | User-wide fan-out (BackgroundEvent). |
| `case_id` | Correlation ID for the active case. | UI grouping and audit log. |
| `investigation_id` | Correlation ID for the active investigation. | UI grouping and state recovery. |

---

## Event Classification

### Session Events
Events intended for a specific browser session. Use these when a triggering request arrived on a known `web_session_id`.
- **Examples**: AI chat chunks, command execution results, approval requests.

### Background Events
System-initiated events with no specific browser session. `g8ed` fans these out to **every active SSE session** owned by the `user_id`.
- **Examples**: Global notifications, operator availability changes, system-wide alerts.

---

## Governance & Safety

The event system is integrated with g8e's safety layers:

### The Sentinel
Before an operator (`g8eo`) executes a command, the request is passed through **The Sentinel**. If a threat is detected, a `g8e.v1.operator.command.failed` event is emitted with a `sentinel_blocked` error type, and the action is recorded in the audit vault.

### Immutable Audit Trail (LFAA)
Critical lifecycle events (commands, file edits, AI decisions) are automatically recorded by the `AuditService`. These events are "Log-First, Act-After" (LFAA), ensuring a permanent, tamper-resistant record of all system activity.

---

## Protocol Format

```
g8e.v<version>.<domain>.<resource>[.<sub-resource>...].<action>
```

- **Protocol prefix**: `g8e.v1` (current version)
- **Domain**: Top-level namespace (`app`, `operator`, `ai`, `platform`, `source`)
- **Resource path**: dot-separated hierarchy identifying the subject
- **Action**: Past-tense verb (`created`, `failed`) or state (`active`, `open`)

### Canonical Truths
`shared/constants/events.json` is the single source of truth. All components must bind to these values:
- **`g8ee`**: `EventType(str, Enum)` in `app/constants/events.py`
- **`g8ed`**: Frozen `EventType` object in `constants/events.js` (reads JSON)
- **`g8eo`**: Event struct tree in `constants/events.go`

---

## Domain Overview

Total event types defined: **270**.

### 1. `app` -- Application Layer
Manages the high-level entities users interact with.
- **`app.case`**: Creation, escalation, and resolution of cases.
- **`app.task`**: Lifecycle of discrete units of work.
- **`app.investigation`**: State and chat history of active investigations.

### 2. `operator` -- Agent Execution
Handles the lifecycle and actions of the `g8eo` agents.
- **`operator.command`**: Execution request, streaming output, and final results.
- **`operator.file`**: Edits, diffs, and history fetching.
- **`operator.status`**: Availability, binding, and heartbeat detection.

### 3. `ai` -- AI Intelligence
Drives the LLM pipeline and verification systems.
- **`ai.llm.chat`**: Streaming chunks, token usage, and iteration management.
- **`ai.tribunal`**: Multi-model consensus and verification sessions.
- **`ai.reputation`**: Agent commitment and slashing (commitment to truth).

### 4. `platform` -- System Infrastructure
Infrastructure-level signals for the platform itself.
- **`platform.auth`**: Login, session validation, and auth state changes.
- **`platform.sse`**: Connection health, keepalives, and errors.
- **`platform.sentinel`**: Real-time mode changes and threat detection alerts.

### 5. `source` -- Message Attribution
Carry-along tags for message payloads to identify origin (User, AI, System).

---

## Adding New Events

1. **Define in JSON**: Add to `shared/constants/events.json`.
2. **Propagate to Python**: Add to `components/g8ee/app/constants/events.py`.
3. **Propagate to JS**: Add to `components/g8ed/constants/events.js`.
4. **Propagate to Go**: Add to `components/g8eo/constants/events.go` (if produced/consumed by operator).
5. **Verify**: Ensure the wire value matches the hierarchy.
