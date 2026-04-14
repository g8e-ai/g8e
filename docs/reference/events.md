# g8e Event Type Specification

Official wire-protocol event type reference for the g8e platform. All inter-component communication uses these event types.

---

## Protocol Format

```
g8e.v<version>.<domain>.<resource>[.<sub-resource>...].<action>
```

- **Protocol prefix** -- `g8e.v1` (current version)
- **Domain** -- top-level namespace: `app`, `operator`, `ai`, `platform`, `source`
- **Resource path** -- dot-separated hierarchy identifying the subject
- **Action** -- past-tense verb or state at the leaf position

### Source of Truth

`shared/constants/events.json` is the single canonical definition. Component-level bindings consume it:

| Component | File | Mechanism |
|-----------|------|-----------|
| g8ee (Python) | `components/g8ee/app/constants/events.py` | `EventType(str, Enum)` with hardcoded wire values |
| g8ed (Node.js server) | `components/g8ed/constants/events.js` | `EventType` frozen object, reads from `events.json` via `shared.js` |
| g8ed (browser client) | `components/g8ed/public/js/constants/events.js` | `EventType` frozen object with hardcoded wire values (subset) |
| g8eo (Go) | `components/g8eo/constants/events.go` | `Event` struct tree with hardcoded wire values (operator subset) |

### Naming Rules

1. Wire values use lowercase dot-delimited segments: `g8e.v1.operator.command.started`
2. Constant names use `UPPER_SNAKE_CASE`: `OPERATOR_COMMAND_STARTED`
3. Every leaf is a **past-tense action** (`created`, `failed`, `received`) or a **state** (`active`, `open`)
4. New events must be added to `events.json` first, then propagated to component bindings

---

## Domains

The protocol defines five top-level domains. Total event count: **238**.

| Domain | Count | Description |
|--------|-------|-------------|
| `app` | 35 | Application-layer entities: cases, tasks, investigations |
| `operator` | 114 | Operator (g8eo) lifecycle, commands, file ops, network, audit, bootstrap |
| `ai` | 48 | LLM chat, streaming, lifecycle, tool calls, tribunal |
| `platform` | 36 | Auth, SSE transport, terminal UI, telemetry, sentinel |
| `source` | 5 | Event origin tags for message attribution |

---

## `app` -- Application Events (35)

### `app.case` -- Case Management

| Wire Value | Description |
|------------|-------------|
| `g8e.v1.app.case.created` | New case created |
| `g8e.v1.app.case.updated` | Case metadata updated |
| `g8e.v1.app.case.assigned` | Case assigned to a user |
| `g8e.v1.app.case.escalated` | Case escalated |
| `g8e.v1.app.case.resolved` | Case marked resolved |
| `g8e.v1.app.case.closed` | Case closed |
| `g8e.v1.app.case.selected` | User selected a case in the UI |
| `g8e.v1.app.case.cleared` | Active case selection cleared |
| `g8e.v1.app.case.switched` | User switched between cases |
| `g8e.v1.app.case.creation.requested` | Case creation requested (pre-persistence) |
| `g8e.v1.app.case.update.requested` | Case update requested (pre-persistence) |

### `app.task` -- Task Lifecycle

| Wire Value | Description |
|------------|-------------|
| `g8e.v1.app.task.created` | Task created |
| `g8e.v1.app.task.updated` | Task metadata updated |
| `g8e.v1.app.task.assigned` | Task assigned |
| `g8e.v1.app.task.started` | Task execution started |
| `g8e.v1.app.task.completed` | Task completed successfully |
| `g8e.v1.app.task.failed` | Task failed |

### `app.investigation` -- Investigation Lifecycle

| Wire Value | Description |
|------------|-------------|
| `g8e.v1.app.investigation.created` | Investigation created |
| `g8e.v1.app.investigation.updated` | Investigation metadata updated |
| `g8e.v1.app.investigation.loaded` | Investigation data loaded from storage |
| `g8e.v1.app.investigation.requested` | Investigation data requested |
| `g8e.v1.app.investigation.started` | Investigation started |
| `g8e.v1.app.investigation.closed` | Investigation closed |
| `g8e.v1.app.investigation.escalated` | Investigation escalated |

### `app.investigation.list` -- Investigation List Operations

| Wire Value | Description |
|------------|-------------|
| `g8e.v1.app.investigation.list.requested` | Investigation list fetch requested |
| `g8e.v1.app.investigation.list.received` | Investigation list data received |
| `g8e.v1.app.investigation.list.completed` | Investigation list operation completed |
| `g8e.v1.app.investigation.list.failed` | Investigation list operation failed |

### `app.investigation.status` -- Investigation Status Transitions

| Wire Value | Description |
|------------|-------------|
| `g8e.v1.app.investigation.status.updated.open` | Investigation status set to open |
| `g8e.v1.app.investigation.status.updated.closed` | Investigation status set to closed |
| `g8e.v1.app.investigation.status.updated.escalated` | Investigation status set to escalated |
| `g8e.v1.app.investigation.status.updated.resolved` | Investigation status set to resolved |

### `app.investigation.chat` -- Investigation Chat Messages

| Wire Value | Description |
|------------|-------------|
| `g8e.v1.app.investigation.chat.message.user` | User-originated chat message |
| `g8e.v1.app.investigation.chat.message.ai` | AI-originated chat message |
| `g8e.v1.app.investigation.chat.message.system` | System-originated chat message |

---

## `operator` -- Operator Events (114)

### `operator.heartbeat` -- Heartbeat

| Wire Value | Description |
|------------|-------------|
| `g8e.v1.operator.heartbeat.sent` | Operator heartbeat sent |
| `g8e.v1.operator.heartbeat.requested` | Heartbeat requested from operator |
| `g8e.v1.operator.heartbeat.received` | Heartbeat received by platform |
| `g8e.v1.operator.heartbeat.missed` | Expected heartbeat not received (stale detection) |

### `operator.shutdown` -- Shutdown

| Wire Value | Description |
|------------|-------------|
| `g8e.v1.operator.shutdown.requested` | Graceful shutdown requested |
| `g8e.v1.operator.shutdown.acknowledged` | Operator acknowledged shutdown |

### `operator.panel` -- Panel

| Wire Value | Description |
|------------|-------------|
| `g8e.v1.operator.panel.list.updated` | Operator panel list refreshed |

### `operator.status` -- Status Transitions

| Wire Value | Description |
|------------|-------------|
| `g8e.v1.operator.status.updated.active` | Operator transitioned to active |
| `g8e.v1.operator.status.updated.available` | Operator transitioned to available |
| `g8e.v1.operator.status.updated.unavailable` | Operator transitioned to unavailable |
| `g8e.v1.operator.status.updated.bound` | Operator bound to a session |
| `g8e.v1.operator.status.updated.offline` | Operator transitioned to offline |
| `g8e.v1.operator.status.updated.stale` | Operator marked stale (heartbeat timeout) |
| `g8e.v1.operator.status.updated.stopped` | Operator stopped |
| `g8e.v1.operator.status.updated.terminated` | Operator terminated |

### `operator.api` -- API Key

| Wire Value | Description |
|------------|-------------|
| `g8e.v1.operator.api.key.refreshed` | Operator API key rotated |

### `operator.device` -- Device

| Wire Value | Description |
|------------|-------------|
| `g8e.v1.operator.device.registered` | Operator device registered |

### `operator.command` -- Command Execution

| Wire Value | Description |
|------------|-------------|
| `g8e.v1.operator.command.requested` | Command execution requested |
| `g8e.v1.operator.command.started` | Command execution started |
| `g8e.v1.operator.command.completed` | Command completed successfully |
| `g8e.v1.operator.command.failed` | Command failed |
| `g8e.v1.operator.command.cancelled` | Command cancelled |
| `g8e.v1.operator.command.execution` | Command execution event (generic) |
| `g8e.v1.operator.command.result` | Command result received |
| `g8e.v1.operator.command.output.received` | Incremental command output received |

### `operator.command.status` -- Command Status Updates

| Wire Value | Description |
|------------|-------------|
| `g8e.v1.operator.command.status.updated.queued` | Command queued for execution |
| `g8e.v1.operator.command.status.updated.running` | Command currently running |
| `g8e.v1.operator.command.status.updated.completed` | Command status set to completed |
| `g8e.v1.operator.command.status.updated.failed` | Command status set to failed |
| `g8e.v1.operator.command.status.updated.cancelled` | Command status set to cancelled |

### `operator.command.cancel` -- Command Cancellation

| Wire Value | Description |
|------------|-------------|
| `g8e.v1.operator.command.cancel.requested` | Command cancellation requested |
| `g8e.v1.operator.command.cancel.acknowledged` | Operator acknowledged cancel |
| `g8e.v1.operator.command.cancel.failed` | Cancellation failed |

### `operator.command.approval` -- Command Approval

| Wire Value | Description |
|------------|-------------|
| `g8e.v1.operator.command.approval.preparing` | Preparing approval request |
| `g8e.v1.operator.command.approval.requested` | User approval requested for command |
| `g8e.v1.operator.command.approval.granted` | User approved command execution |
| `g8e.v1.operator.command.approval.rejected` | User rejected command execution |

### `operator.file.edit` -- File Edit Operations

| Wire Value | Description |
|------------|-------------|
| `g8e.v1.operator.file.edit.requested` | File edit requested |
| `g8e.v1.operator.file.edit.started` | File edit started |
| `g8e.v1.operator.file.edit.completed` | File edit completed |
| `g8e.v1.operator.file.edit.failed` | File edit failed |
| `g8e.v1.operator.file.edit.timeout` | File edit timed out |

### `operator.file.edit.approval` -- File Edit Approval

| Wire Value | Description |
|------------|-------------|
| `g8e.v1.operator.file.edit.approval.requested` | File edit approval requested |
| `g8e.v1.operator.file.edit.approval.granted` | File edit approved |
| `g8e.v1.operator.file.edit.approval.rejected` | File edit rejected |
| `g8e.v1.operator.file.edit.approval.feedback` | File edit approval feedback received |

### `operator.file.history` -- File History

| Wire Value | Description |
|------------|-------------|
| `g8e.v1.operator.file.history.fetch.started` | File history fetch started |
| `g8e.v1.operator.file.history.fetch.requested` | File history fetch requested |
| `g8e.v1.operator.file.history.fetch.received` | File history data received |
| `g8e.v1.operator.file.history.fetch.completed` | File history fetch completed |
| `g8e.v1.operator.file.history.fetch.failed` | File history fetch failed |

### `operator.file.diff` -- File Diff

| Wire Value | Description |
|------------|-------------|
| `g8e.v1.operator.file.diff.fetch.started` | File diff fetch started |
| `g8e.v1.operator.file.diff.fetch.requested` | File diff fetch requested |
| `g8e.v1.operator.file.diff.fetch.received` | File diff data received |
| `g8e.v1.operator.file.diff.fetch.completed` | File diff fetch completed |
| `g8e.v1.operator.file.diff.fetch.failed` | File diff fetch failed |

### `operator.file.restore` -- File Restore

| Wire Value | Description |
|------------|-------------|
| `g8e.v1.operator.file.restore.requested` | File restore requested |
| `g8e.v1.operator.file.restore.received` | File restore data received |
| `g8e.v1.operator.file.restore.completed` | File restore completed |
| `g8e.v1.operator.file.restore.failed` | File restore failed |

### `operator.filesystem.list` -- Filesystem Listing

| Wire Value | Description |
|------------|-------------|
| `g8e.v1.operator.filesystem.list.started` | Filesystem list started |
| `g8e.v1.operator.filesystem.list.requested` | Filesystem list requested |
| `g8e.v1.operator.filesystem.list.received` | Filesystem list data received |
| `g8e.v1.operator.filesystem.list.completed` | Filesystem list completed |
| `g8e.v1.operator.filesystem.list.failed` | Filesystem list failed |

### `operator.filesystem.read` -- Filesystem Read

| Wire Value | Description |
|------------|-------------|
| `g8e.v1.operator.filesystem.read.started` | Filesystem read started |
| `g8e.v1.operator.filesystem.read.requested` | Filesystem read requested |
| `g8e.v1.operator.filesystem.read.received` | Filesystem read data received |
| `g8e.v1.operator.filesystem.read.completed` | Filesystem read completed |
| `g8e.v1.operator.filesystem.read.failed` | Filesystem read failed |

### `operator.logs` -- Log Fetching

| Wire Value | Description |
|------------|-------------|
| `g8e.v1.operator.logs.fetch.requested` | Log fetch requested |
| `g8e.v1.operator.logs.fetch.received` | Log data received |
| `g8e.v1.operator.logs.fetch.completed` | Log fetch completed |
| `g8e.v1.operator.logs.fetch.failed` | Log fetch failed |

### `operator.history` -- History Fetching

| Wire Value | Description |
|------------|-------------|
| `g8e.v1.operator.history.fetch.requested` | History fetch requested |
| `g8e.v1.operator.history.fetch.received` | History data received |
| `g8e.v1.operator.history.fetch.completed` | History fetch completed |
| `g8e.v1.operator.history.fetch.failed` | History fetch failed |

### `operator.intent` -- Intent Approval

| Wire Value | Description |
|------------|-------------|
| `g8e.v1.operator.intent.granted` | Intent granted |
| `g8e.v1.operator.intent.denied` | Intent denied |
| `g8e.v1.operator.intent.revoked` | Intent revoked |
| `g8e.v1.operator.intent.approval.requested` | Intent approval requested |
| `g8e.v1.operator.intent.approval.granted` | Intent approval granted |
| `g8e.v1.operator.intent.approval.rejected` | Intent approval rejected |

### `operator.network.ping` -- Network Ping

| Wire Value | Description |
|------------|-------------|
| `g8e.v1.operator.network.ping.requested` | Ping requested |
| `g8e.v1.operator.network.ping.received` | Ping response received |
| `g8e.v1.operator.network.ping.completed` | Ping completed |
| `g8e.v1.operator.network.ping.failed` | Ping failed |

### `operator.network.port.check` -- Port Check

| Wire Value | Description |
|------------|-------------|
| `g8e.v1.operator.network.port.check.requested` | Port check requested |
| `g8e.v1.operator.network.port.check.started` | Port check started |
| `g8e.v1.operator.network.port.check.received` | Port check result received |
| `g8e.v1.operator.network.port.check.completed` | Port check completed |
| `g8e.v1.operator.network.port.check.failed` | Port check failed |

### `operator.audit` -- Audit Trail

| Wire Value | Description |
|------------|-------------|
| `g8e.v1.operator.audit.user.recorded` | User action audit entry recorded |
| `g8e.v1.operator.audit.ai.recorded` | AI action audit entry recorded |
| `g8e.v1.operator.audit.command.recorded` | Command audit entry recorded |
| `g8e.v1.operator.audit.direct.command.recorded` | Direct command audit entry recorded |
| `g8e.v1.operator.audit.direct.command.result.recorded` | Direct command result audit entry recorded |

### `operator.bootstrap` -- Bootstrap

| Wire Value | Description |
|------------|-------------|
| `g8e.v1.operator.bootstrap.requested` | Operator bootstrap requested |
| `g8e.v1.operator.bootstrap.received` | Bootstrap data received |
| `g8e.v1.operator.bootstrap.completed` | Bootstrap completed |
| `g8e.v1.operator.bootstrap.failed` | Bootstrap failed |
| `g8e.v1.operator.bootstrap.config.received` | Bootstrap config received |

### `operator` -- Binding

| Wire Value | Description |
|------------|-------------|
| `g8e.v1.operator.bound` | Operator bound to session |
| `g8e.v1.operator.unbound` | Operator unbound from session |

### `operator.terminal` -- Terminal

| Wire Value | Description |
|------------|-------------|
| `g8e.v1.operator.terminal.thinking.append` | Append to thinking indicator |
| `g8e.v1.operator.terminal.thinking.complete` | Thinking indicator completed |
| `g8e.v1.operator.terminal.approval.denied` | Terminal approval denied |
| `g8e.v1.operator.terminal.auth.state.changed` | Terminal auth state changed |

### `operator.mcp` -- MCP (Model Context Protocol)

| Wire Value | Description |
|------------|-------------|
| `g8e.v1.operator.mcp.tools.call` | MCP tool call dispatched |
| `g8e.v1.operator.mcp.tools.result` | MCP tool call result received |
| `g8e.v1.operator.mcp.resources.list` | MCP resource list requested |
| `g8e.v1.operator.mcp.resources.read` | MCP resource read requested |
| `g8e.v1.operator.mcp.resources.result` | MCP resource result received |

---

## `ai` -- AI Events (48)

### `ai.llm.config` -- LLM Configuration

| Wire Value | Description |
|------------|-------------|
| `g8e.v1.ai.llm.config.requested` | LLM configuration requested |
| `g8e.v1.ai.llm.config.received` | LLM configuration received |
| `g8e.v1.ai.llm.config.failed` | LLM configuration failed |

### `ai.llm.lifecycle` -- LLM Lifecycle

| Wire Value | Description |
|------------|-------------|
| `g8e.v1.ai.llm.lifecycle.requested` | LLM lifecycle start requested |
| `g8e.v1.ai.llm.lifecycle.started` | LLM lifecycle started |
| `g8e.v1.ai.llm.lifecycle.completed` | LLM lifecycle completed |
| `g8e.v1.ai.llm.lifecycle.failed` | LLM lifecycle failed |
| `g8e.v1.ai.llm.lifecycle.stopped` | LLM lifecycle stopped (user-initiated) |
| `g8e.v1.ai.llm.lifecycle.error.occurred` | LLM lifecycle error occurred |

### `ai.llm.chat` -- Chat Submission

| Wire Value | Description |
|------------|-------------|
| `g8e.v1.ai.llm.chat.submitted` | Chat message submitted by user |
| `g8e.v1.ai.llm.chat.stop.show` | Show stop button in UI |
| `g8e.v1.ai.llm.chat.stop.hide` | Hide stop button in UI |
| `g8e.v1.ai.llm.chat.filter.event` | Chat filter event triggered |

### `ai.llm.chat.message` -- Chat Message Handling

| Wire Value | Description |
|------------|-------------|
| `g8e.v1.ai.llm.chat.message.sent` | Chat message sent to LLM pipeline |
| `g8e.v1.ai.llm.chat.message.replayed` | Chat message replayed from history |
| `g8e.v1.ai.llm.chat.message.processing.failed` | Chat message processing failed |
| `g8e.v1.ai.llm.chat.message.dead.lettered` | Chat message moved to dead letter queue |

### `ai.llm.chat.iteration` -- Chat Iteration Lifecycle

| Wire Value | Description |
|------------|-------------|
| `g8e.v1.ai.llm.chat.iteration.started` | LLM chat iteration started |
| `g8e.v1.ai.llm.chat.iteration.completed` | LLM chat iteration completed |
| `g8e.v1.ai.llm.chat.iteration.failed` | LLM chat iteration failed |
| `g8e.v1.ai.llm.chat.iteration.stopped` | LLM chat iteration stopped (user-initiated) |
| `g8e.v1.ai.llm.chat.iteration.thinking.started` | LLM thinking phase started |
| `g8e.v1.ai.llm.chat.iteration.citations.received` | Grounding citations received |

### `ai.llm.chat.iteration.text` -- Text Response

| Wire Value | Description |
|------------|-------------|
| `g8e.v1.ai.llm.chat.iteration.text.received` | Full text response received |
| `g8e.v1.ai.llm.chat.iteration.text.chunk.received` | Incremental text chunk received |
| `g8e.v1.ai.llm.chat.iteration.text.completed` | Text response completed |
| `g8e.v1.ai.llm.chat.iteration.text.truncated` | Text response truncated (token limit) |

### `ai.llm.chat.iteration.stream` -- Streaming

| Wire Value | Description |
|------------|-------------|
| `g8e.v1.ai.llm.chat.iteration.stream.started` | LLM stream started |
| `g8e.v1.ai.llm.chat.iteration.stream.delta.received` | Stream delta chunk received |
| `g8e.v1.ai.llm.chat.iteration.stream.completed` | Stream completed |
| `g8e.v1.ai.llm.chat.iteration.stream.failed` | Stream failed |

### `ai.llm.tool` -- LLM Tool Calls

| Wire Value | Description |
|------------|-------------|
| `g8e.v1.ai.llm.tool.g8e.web.search.requested` | Web search tool call requested |
| `g8e.v1.ai.llm.tool.g8e.web.search.received` | Web search results received |
| `g8e.v1.ai.llm.tool.g8e.web.search.completed` | Web search completed |
| `g8e.v1.ai.llm.tool.g8e.web.search.failed` | Web search failed |

### `ai.tribunal` -- Tribunal (Multi-Model Verification)

| Wire Value | Description |
|------------|-------------|
| `g8e.v1.ai.tribunal.session.started` | Tribunal session started |
| `g8e.v1.ai.tribunal.session.completed` | Tribunal session completed |
| `g8e.v1.ai.tribunal.session.failed` | Tribunal session failed |
| `g8e.v1.ai.tribunal.session.fallback.triggered` | Tribunal fallback to primary response triggered |

### `ai.tribunal.voting` -- Tribunal Voting

| Wire Value | Description |
|------------|-------------|
| `g8e.v1.ai.tribunal.voting.started` | Voting round started |
| `g8e.v1.ai.tribunal.voting.failed` | Voting round failed |
| `g8e.v1.ai.tribunal.voting.pass.completed` | Voting pass completed |
| `g8e.v1.ai.tribunal.voting.pass.failed` | Voting pass failed |
| `g8e.v1.ai.tribunal.voting.consensus.reached` | Consensus reached |
| `g8e.v1.ai.tribunal.voting.consensus.not_reached` | Consensus not reached |
| `g8e.v1.ai.tribunal.voting.review.started` | Review round started |
| `g8e.v1.ai.tribunal.voting.review.completed` | Review round completed |
| `g8e.v1.ai.tribunal.voting.review.failed` | Review round failed |

---

## `platform` -- Platform Events (36)

### `platform.auth.login` -- Login

| Wire Value | Description |
|------------|-------------|
| `g8e.v1.platform.auth.login.requested` | Login requested |
| `g8e.v1.platform.auth.login.succeeded` | Login succeeded |
| `g8e.v1.platform.auth.login.failed` | Login failed |

### `platform.auth.logout` -- Logout

| Wire Value | Description |
|------------|-------------|
| `g8e.v1.platform.auth.logout.requested` | Logout requested |
| `g8e.v1.platform.auth.logout.succeeded` | Logout succeeded |
| `g8e.v1.platform.auth.logout.failed` | Logout failed |

### `platform.auth.session` -- Session

| Wire Value | Description |
|------------|-------------|
| `g8e.v1.platform.auth.session.validation.requested` | Session validation requested |
| `g8e.v1.platform.auth.session.validation.succeeded` | Session validation succeeded |
| `g8e.v1.platform.auth.session.validation.failed` | Session validation failed |
| `g8e.v1.platform.auth.session.expired` | Session expired |

### `platform.auth.user` -- User Auth State

| Wire Value | Description |
|------------|-------------|
| `g8e.v1.platform.auth.user.authenticated` | User authenticated |
| `g8e.v1.platform.auth.user.unauthenticated` | User unauthenticated |

### `platform.auth.component` -- Component Initialization

| Wire Value | Description |
|------------|-------------|
| `g8e.v1.platform.auth.component.initialized.authstate` | Auth state component initialized |
| `g8e.v1.platform.auth.component.initialized.chat` | Chat component initialized |
| `g8e.v1.platform.auth.component.initialized.operator` | Operator component initialized |

### `platform.auth` -- Auth Misc

| Wire Value | Description |
|------------|-------------|
| `g8e.v1.platform.auth.info` | Auth info event |

### `platform.usage` -- Usage

| Wire Value | Description |
|------------|-------------|
| `g8e.v1.platform.usage.updated` | Platform usage metrics updated |

### `platform.notification` -- Notification

| Wire Value | Description |
|------------|-------------|
| `g8e.v1.platform.notification` | Platform notification |

### `platform.sse` -- SSE Transport

| Wire Value | Description |
|------------|-------------|
| `g8e.v1.platform.sse.keepalive.sent` | SSE keepalive sent |
| `g8e.v1.platform.sse.connection.established` | SSE connection established (server-side) |
| `g8e.v1.platform.sse.connection.opened` | SSE connection opened (client-side) |
| `g8e.v1.platform.sse.connection.closed` | SSE connection closed |
| `g8e.v1.platform.sse.connection.failed` | SSE connection failed |
| `g8e.v1.platform.sse.connection.error` | SSE connection error |

### `platform.terminal` -- Terminal UI

| Wire Value | Description |
|------------|-------------|
| `g8e.v1.platform.terminal.opened` | Terminal opened |
| `g8e.v1.platform.terminal.minimized` | Terminal minimized |
| `g8e.v1.platform.terminal.maximized` | Terminal maximized |
| `g8e.v1.platform.terminal.closed` | Terminal closed |

### `platform.sentinel` -- Sentinel

| Wire Value | Description |
|------------|-------------|
| `g8e.v1.platform.sentinel.mode.changed` | Sentinel mode changed |

### `platform.external` -- External Services

| Wire Value | Description |
|------------|-------------|
| `g8e.v1.platform.external.service.configured` | External service configured |

### `platform.telemetry` -- Telemetry

| Wire Value | Description |
|------------|-------------|
| `g8e.v1.platform.telemetry.health.reported` | Health telemetry reported |
| `g8e.v1.platform.telemetry.performance.recorded` | Performance telemetry recorded |
| `g8e.v1.platform.telemetry.error.logged` | Error telemetry logged |
| `g8e.v1.platform.telemetry.audit.logged` | Audit telemetry logged |

### `platform.console` -- Console Log

| Wire Value | Description |
|------------|-------------|
| `g8e.v1.platform.console.log.entry.received` | Console log entry received |
| `g8e.v1.platform.console.log.connected.confirmed` | Console log connection confirmed |

---

## `source` -- Event Source Tags (5)

Source tags identify the origin of a message in a conversation. These are not lifecycle events -- they are attribution labels carried on message payloads.

| Wire Value | Description |
|------------|-------------|
| `g8e.v1.source.user.chat` | Message originated from user via chat |
| `g8e.v1.source.user.terminal` | Message originated from user via terminal |
| `g8e.v1.source.ai.primary` | Message originated from primary AI |
| `g8e.v1.source.ai.assistant` | Message originated from assistant AI |
| `g8e.v1.source.system` | Message originated from the system |

---

## Component Coverage

Not every component needs every event. This table shows which domains each component binding covers.

| Domain | `events.json` | g8ee (Python) | g8ed (server JS) | g8ed (client JS) | g8eo (Go) |
|--------|:---:|:---:|:---:|:---:|:---:|
| `app` | 35 | 35 | 35 | 35 | -- |
| `operator` | 114 | 114 | 114 | 114 | 65 |
| `ai` | 48 | 48 | 48 | 48 | -- |
| `platform` | 36 | 36 | 36 | 36 | -- |
| `source` | 5 | 5 | 5 | 5 | -- |
| **Total** | **238** | **238** | **238** | **238** | **65** |

g8eo only binds the operator-domain events it produces or consumes. g8ed client JS mirrors all events with hardcoded values. g8ed server JS and g8ee mirror the full set.

---

## Wire Envelope

All events are transmitted as JSON objects with a `type` field carrying the wire value:

```json
{
  "type": "g8e.v1.operator.command.completed",
  "data": {
    "execution_id": "exec-abc123",
    "output": "command output",
    "exit_code": 0,
    "success": true,
    "investigation_id": "inv-xyz",
    "case_id": "case-456",
    "web_session_id": "ws-789"
  }
}
```

SSE events use the same structure, serialized via `G8eBaseModel.forWire()` at the g8ed SSE service boundary.

---

## Adding New Events

1. Add the wire value to `shared/constants/events.json` following the nested hierarchy
2. Add the `EventType` enum member to `components/g8ee/app/constants/events.py`
3. Add the `EventType` property to `components/g8ed/constants/events.js` (reads from JSON automatically)
4. If the event is needed in the browser, add it to `components/g8ed/public/js/constants/events.js`
5. If the event is consumed or produced by g8eo, add it to `components/g8eo/constants/events.go`
6. Update this spec with the new wire value and description
