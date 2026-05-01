---
title: Events
parent: Architecture
---

# g8e Event Protocol Specification

The g8e platform uses a unified, hierarchical event protocol for all inter-component communication. This system ensures that AI agents, operators, and user interfaces remain synchronized in real-time across a distributed environment.

---

## Architecture & Lifecycle

Events in g8e are state transitions and lifecycle signals that drive the system's reactivity.

### 1. The Central Hub: `g8ed`
`g8ed` (Node.js) serves as the central event router and composition root. It maintains two primary transport layers:
- **Server-Sent Events (SSE)**: Pushes real-time updates to browser clients.
- **G8ES Pub/Sub**: A WebSocket-based backbone for bidirectional communication with `g8eo` (Go) operators via the `g8es` message broker.

### 2. Event Producers
- **`g8ee` (Python)**: The AI logic engine. It emits events (chat chunks, tool calls, and tribunal results) to `g8ed` via internal HTTP push.
- **`g8eo` (Go)**: The operator agent. It emits command results, heartbeats, and status updates to `g8ed` via the results channel of the G8ES Pub/Sub.
- **`g8ed` (Node.js)**: The platform service. It emits lifecycle events (auth, session, case/investigation updates) and proxies messages between components.

### 3. The Lifecycle of an Event
1. **Emission**: A producer (e.g., `g8ee`) constructs a typed event (e.g., `SessionEvent`).
2. **Routing**: `g8ed` receives the event and uses the **Routing Tuple** (see below) to determine the destination.
3. **Delivery**: 
   - If the target is a browser, `SSEService` pushes it via an open SSE connection.
   - If the target is an operator, `PubSubBroker` routes it to the specific WebSocket channel assigned to that operator session.

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

## Core Pipelines & Tool Lifecycle

### 1. LLM Chat & Iterations
The chat pipeline uses a streaming architecture where the AI's "thought process" is exposed in real-time:
- `LLM_CHAT_ITERATION_STARTED`: Signifies the start of an AI reasoning turn.
- `LLM_CHAT_ITERATION_TEXT_CHUNK_RECEIVED`: Individual tokens streamed to the UI.
- `LLM_CHAT_ITERATION_THINKING_STARTED`: Signals the AI has entered a "thinking" phase (for models supporting it).
- `LLM_CHAT_ITERATION_COMPLETED`: End of a single reasoning/text generation turn.

### 2. Universal Tool Lifecycle
Tools follow a strict request/response pattern to ensure UI responsiveness and auditability:
1. `LLM_TOOL_*_REQUESTED`: The AI has decided to call a tool.
2. `LLM_TOOL_*_RECEIVED`: The tool execution environment has acknowledged the request.
3. `LLM_TOOL_*_COMPLETED` / `FAILED`: The final result or error of the tool execution.

### 3. The Tribunal
Multi-model consensus sessions emit discrete events for each stage:
- `TRIBUNAL_SESSION_STARTED`: Initialization of the consensus group.
- `TRIBUNAL_VOTING_STARTED`: Collection of command candidates.
- `TRIBUNAL_VOTING_WARDEN_STARTED`: The Warden begins defensive analysis.
- `TRIBUNAL_SESSION_WARDEN_BLOCKED`: The Warden has blocked the command (circuit breaker).
- `TRIBUNAL_VOTING_AUDIT_STARTED`: The dissent-aware Auditor begins evaluating the winner.
- `TRIBUNAL_SESSION_COMPLETED`: Final validated command is ready for dispatch.

---

## Governance & Safety

### Log-First, Act-After (LFAA)
Critical lifecycle events (commands, file edits, AI decisions) are automatically recorded by the `AuditService` before the action is executed. This ensures a permanent, tamper-resistant record of all system activity, even if the action itself fails or is interrupted.

### The Sentinel
Before an operator (`g8eo`) executes a command, the request is passed through **The Sentinel**. If a threat is detected:
1. A `g8e.v1.operator.command.failed` event is emitted.
2. The error type is set to `sentinel_blocked`.
3. The attempted violation is recorded in the audit vault.

---

## Protocol Format

```
g8e.v<version>.<domain>.<resource>[.<sub-resource>...].<action>
```

- **Protocol prefix**: `g8e.v1` (current version)
- **Domain**: Top-level namespace (`app`, `operator`, `ai`, `platform`, `source`)
- **Resource path**: dot-separated hierarchy identifying the subject.
- **Action**: Past-tense verb (`created`, `failed`) or state (`active`, `open`).

### Canonical Truths
`shared/constants/events.json` is the single source of truth. All components must bind to these values:
- **`g8ee`**: `EventType` Enum in `app/constants/events.py`.
- **`g8ed`**: `EventType` object in `public/js/constants/events.js`.
- **`g8eo`**: Event constants in `constants/events.go`.

---

## Domain Overview

Total event types defined: **274**.

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
- **`ai.reputation`**: Phase 2 agent commitment and truth-verification signals.

### 4. `platform` -- System Infrastructure
Infrastructure-level signals for the platform itself.
- **`platform.auth`**: Login, session validation, and auth state changes.
- **`platform.sse`**: Connection health, keepalives, and errors.
- **`platform.telemetry`**: Health reports, performance metrics, and audit logs.

### 5. `source` -- Message Attribution
Carry-along tags for message payloads to identify origin (`user.chat`, `ai.primary`, `system`).

---

## Adding New Events

1. **Define in JSON**: Add the new event string to `shared/constants/events.json`.
2. **Propagate to Python**: Add the corresponding member to `EventType` in `components/g8ee/app/constants/events.py`.
3. **Propagate to JS**: Add to `components/g8ed/public/js/constants/events.js`.
4. **Propagate to Go**: Add to `components/g8eo/constants/events.go` if used by the operator.
5. **Verify**: Ensure the wire value matches the hierarchy and follows the past-tense rule.
