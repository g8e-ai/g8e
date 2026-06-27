---
title: SSE Streaming
---

# SSE Streaming

Last Updated: 2026-06-27
Version: v1.2.4

The g8e Gateway includes a built-in Server-Sent Events (SSE) streaming infrastructure that enables real-time event delivery from app workloads to browser and CLI clients. This system allows g8e-compatible agentic ensembles to publish typed events (including audit events) for downstream consumption.

---

## Overview

The SSE system provides three endpoints:

- **`POST /api/v1/sse/push`**: App workloads push events (authenticated via mTLS)
- **`GET /api/v1/sse/events`**: Poll for historical events (with `since_id` and `limit` params)
- **`GET /api/v1/sse/stream`**: Real-time SSE stream with live event delivery (mTLS authenticated). All clients — CLI, browser, dashboard — use this single endpoint.

Events are stored in the `sse_events` table and routed by one of three identifiers:
- `web_session_id`: Web UI session events
- `cli_session_id`: CLI / BYO session events
- `user_id`: Background fan-out across every session a user owns

---

## Architecture

```mermaid
flowchart TD
    subgraph App ["App Workload"]
        producer["Event Producer"]
    end

    subgraph Gateway ["g8e Gateway"]
        push["POST /api/v1/sse/push"]
        db[("sse_events table")]
        events["GET /api/v1/sse/events"]
        stream["GET /api/v1/sse/stream"]
        pubsub[["Pub/Sub Broker"]]
    end

    subgraph Client ["Client"]
        browser["Browser / CLI"]
    end

    producer -- "mTLS POST" --> push
    push --> db
    push --> pubsub
    db --> events
    db --> stream
    pubsub --> stream
    browser -- "mTLS GET /api/v1/sse/stream" --> stream
    browser -- "mTLS GET /api/v1/sse/events" --> events
```

---

## Endpoints

### POST /api/v1/sse/push

**Authentication**: mTLS with app workload identity (SPIFFE ID with `/app/` prefix, excluding `g8eo` and `g8eg`)

**Request Body**:
```json
{
  "web_session_id": "string | null",
  "cli_session_id": "string | null",
  "user_id": "string | null",
  "event": {
    "type": "string",
    "data": "any"
  }
}
```

Exactly one of `web_session_id`, `cli_session_id`, or `user_id` must be set.

**Response**:
```json
{
  "success": true,
  "delivered": 1
}
```

**Behavior**:
1. Validates mTLS peer certificate for app workload identity (SPIFFE ID with `/app/` prefix, excluding `g8eo` and `g8eg`).
2. Reads and parses the JSON body; validates that the `event` field is present.
3. Extracts `event.type` for indexing; defaults to `unknown` if absent.
4. Appends the full request body as `payload` to the `sse_events` table with auto-increment ID, `event_type`, and `producer_id` (the caller SPIFFE ID).
5. Enforces producer-to-target ownership. The app identity must be associated with the target session or user. Ownership is verified via `protocol.WorkloadIdentity.MatchesApp` against bound Operator sessions.
6. Publishes the full request body to the Pub/Sub channel for real-time fan-out (channel format: `sse:cli:<id>`, `sse:web:<id>`, or `sse:user:<id>`).
7. Returns success confirmation with delivered count.

---

### GET /api/v1/sse/events

**Authentication**: mTLS with Operator session

**Query Parameters**:
- `web_session_id`: Filter by web session
- `cli_session_id`: Filter by CLI session
- `user_id`: Filter by user
- `since_id`: Return events with ID > since_id (default: 0)
- `limit`: Maximum events to return (default: 200, max: 1000)

**Response**:
```json
{
  "events": [
    {
      "id": 1,
      "web_session_id": "session-123",
      "event_type": "g8e.v1.operator.audit.command.recorded",
      "payload": "{\"type\":\"...\",\"data\":{...}}",
      "created_at": "2026-01-01T00:00:00Z"
    }
  ],
  "count": 1
}
```

Unset routing fields are omitted from the JSON response (the `SSEEventRow` model uses `omitempty` tags on `web_session_id`, `cli_session_id`, and `user_id`).

**Behavior**:
1. Validates Operator session authorization for requested route.
2. Queries `sse_events` table for events matching route and `since_id`.
3. Returns ordered list (ascending by ID).

---

### GET /api/v1/sse/stream

**Authentication**: mTLS with Operator session. All clients (CLI, browser, dashboard) authenticate via mTLS client certificate. There is no cookie-based auth path — the transport determines identity, not the browser session.

**Query Parameters**:
- `web_session_id`: Filter by web session
- `cli_session_id`: Filter by CLI session
- `user_id`: Filter by user
- `since_id`: Start from event ID (supports `Last-Event-ID` header)

**Response**: SSE stream. Replayed historical events include the `id:` field:
```
id: 1
event: g8e.v1.operator.audit.command.recorded
data: {"type":"...","data":{...}}

id: 2
event: g8e.v1.operator.audit.ai.recorded
data: {"type":"...","data":{...}}
```

Real-time events from Pub/Sub omit the `id:` field:
```
event: g8e.v1.operator.audit.command.recorded
data: {"type":"...","data":{...}}
```

**Behavior**:
1. Validates Operator session authorization for requested route.
2. Sets SSE response headers: `Content-Type: text/event-stream`, `Cache-Control: no-cache`, `Connection: keep-alive`, `X-Accel-Buffering: no` (for Nginx), and permissive CORS headers based on the request `Origin`.
3. Subscribes to the Pub/Sub channel for real-time events (channel format: `sse:cli:<id>`, `sse:web:<id>`, or `sse:user:<id>`) using a buffered channel of 100 entries. If the buffer is full, incoming events are dropped with a back-pressure warning log.
4. If `since_id` is greater than 0, replays historical events from the `sse_events` table since `since_id` (up to 1000 rows) in SSE format, including the `id:` field for each row. If `since_id` is 0 or absent, no replay occurs and the stream begins with only real-time events.
5. Streams new events as they arrive from Pub/Sub. Real-time events are emitted without an `id:` field; the `event:` field carries the extracted type and the `data:` field carries the full push payload.
6. Sends heartbeat comments every 30 seconds (SSE comment format `: heartbeat\n\n`).

---

## Event Types

The SSE system is generic and supports any event type. Defined audit event types include:

- `g8e.v1.operator.audit.ai.recorded`: AI action audit log.
- `g8e.v1.operator.audit.command.recorded`: Command execution audit log.
- `g8e.v1.operator.audit.direct.command.recorded`: Direct command audit log.
- `g8e.v1.operator.audit.direct.command.result.recorded`: Direct command result audit log.
- `g8e.v1.operator.audit.user.recorded`: User action audit log.
- `g8e.v1.platform.sse.connection.established`: SSE connection established.
- `g8e.v1.platform.sse.connection.opened`: SSE connection opened.
- `g8e.v1.platform.sse.connection.closed`: SSE connection closed.
- `g8e.v1.platform.sse.connection.failed`: SSE connection failed.
- `g8e.v1.platform.sse.connection.error`: SSE connection error.
- `g8e.v1.platform.sse.keepalive.sent`: SSE heartbeat sent.

See `protocol/constants/events.json` for the complete event type catalog.

---

## Database Schema

The `sse_events` table stores all events:

```sql
CREATE TABLE IF NOT EXISTS sse_events (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    web_session_id TEXT,
    cli_session_id TEXT,
    user_id TEXT,
    event_type TEXT NOT NULL,
    payload TEXT NOT NULL,
    producer_id TEXT,
    created_at TEXT NOT NULL,
    CHECK (
        (CASE WHEN web_session_id IS NULL THEN 0 ELSE 1 END)
      + (CASE WHEN cli_session_id IS NULL THEN 0 ELSE 1 END)
      + (CASE WHEN user_id        IS NULL THEN 0 ELSE 1 END)
      = 1
    )
);
CREATE INDEX IF NOT EXISTS idx_sse_web ON sse_events(web_session_id, id) WHERE web_session_id IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_sse_cli ON sse_events(cli_session_id, id) WHERE cli_session_id IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_sse_user ON sse_events(user_id, id) WHERE user_id IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_sse_created ON sse_events(created_at);
```

**Constraints**:
- Exactly one of `web_session_id`, `cli_session_id`, or `user_id` must be non-null (enforced by CHECK constraint).
- `producer_id` is the SPIFFE ID of the app workload that produced the event.
- Events are immutable once written (append-only).

**Important**: SSE event inserts do NOT alter the state root. This is intentional to allow high-frequency event streaming without governance overhead.

---

## Security Model

### Producer Authorization
- Only app workloads with valid mTLS certificates can push events.
- Certificate must have SPIFFE URI SAN with `/app/` prefix.
- Gateway identities (`g8eo`, `g8eg`) are explicitly blocked from pushing.
- Producer identity is recorded in `producer_id` for attribution.
- The app identity must be associated with the target session or user. Ownership is verified via `protocol.WorkloadIdentity.MatchesApp` against bound Operator sessions. The event is appended to the database before the ownership check; if ownership verification fails, the handler returns 403 but the row remains persisted.

### Consumer Authorization
- All SSE endpoints (`/api/v1/sse/events`, `/api/v1/sse/stream`) require mTLS with an authenticated Operator session. There is no cookie-based auth path.
- The Operator must be authorized for the requested route:
  - For `web_session_id`: Must own the web session.
  - For `cli_session_id`: Must own the CLI session.
  - For `user_id`: Must be the user.
- Multi-tenant isolation is enforced at the database query level.

### Transport Security
- All SSE endpoints (`/api/v1/sse/push`, `/api/v1/sse/events`, `/api/v1/sse/stream`) require mTLS (HTTPS port 8443).
- Not available on HTTP bootstrap port (8080).
- Pub/Sub channels are scoped to routing targets (format: `sse:cli:<id>`, `sse:web:<id>`, `sse:user:<id>`)

---

## Use Cases

### Real-time Audit Streaming
App workloads can push audit events as they occur, enabling real-time audit log viewers in the web UI or CLI.

```json
POST /api/v1/sse/push
{
  "user_id": "user-123",
  "event": {
    "type": "g8e.v1.operator.audit.command.recorded",
    "data": {
      "command": "ls -la",
      "exit_code": 0,
      "timestamp": "2026-01-01T00:00:00Z"
    }
  }
}
```

### LLM Streaming
g8e-compatible agentic ensembles stream LLM generation chunks to the browser:

```json
POST /api/v1/sse/push
{
  "web_session_id": "session-abc",
  "event": {
    "type": "g8e.v1.ai.llm.chat.iteration.text.chunk.received",
    "data": {
      "chunk": "Hello, "
    }
  }
}
```

### Background Notifications
Fan-out notifications across all user sessions:

```json
POST /api/v1/sse/push
{
  "user_id": "user-123",
  "event": {
    "type": "g8e.v1.platform.notification",
    "data": {
      "message": "Task completed"
    }
  }
}
```

---

## Implementation Details

### Core Files
- `internal/services/gateway/gateway_http_sse.go`: HTTP handlers for SSE endpoints (`handleInternalSSEPush`, `handleInternalSSEEvents`, `handleInternalSSEStream`).
- `internal/services/gateway/sse_event_service.go`: SSE event storage and retrieval service (`SSEEventsAppend`, `SSEEventsListSince`, `SSEEventsListAllSince`, `SSEEventsCleanup`, `SSEEventsWipe`, `SSEEventsCount`).
- `internal/services/gateway/gateway_pubsub.go`: Pub/Sub integration for real-time fan-out (`RegisterHandler`, `Publish`).
- `internal/services/gateway/db_controller.go`: Admin endpoints for SSE event management (`handleSSEEvents`).
- `internal/services/gateway/db/schema.sql`: Database schema for `sse_events` table.
- `internal/constants/api_paths.go`: API path constants.
- `protocol/constants/events.json`: Event type catalog.
- `internal/models/gateway.go`: SSE event row models (`SSEEventRow`, `SSEPushResponse`, `SSEEventsResponse`, `SSEEventsCountResponse`, `SSEEventsWipeResponse`).

### State Root Impact
SSE event inserts are deliberately excluded from state root calculation. The `sse_events` table has no triggers to increment `state_version`, allowing high-frequency event streaming without triggering governance consensus rounds. Events are considered ephemeral telemetry, not governance state.

### Pruning
The `sse_events` table is pruned through two mechanisms:

**Automatic cleanup**: The gateway maintenance loop (`RunMaintenance` in `internal/services/gateway/gateway_db.go`) calls `SSEEventsCleanup` every 30 seconds with a 1-hour retention window. Events older than 1 hour are automatically deleted.

**Manual admin endpoints**:
- `DELETE /api/v1/data/_sse_events`: Admin wipe endpoint (requires mTLS). Deletes all rows and returns the count.
- `GET /api/v1/data/_sse_events/count`: Admin count endpoint (requires mTLS). Returns the total row count.

---

## Best Practices

1. **Event Batching**: For high-frequency events, consider batching before pushing to reduce database load.
2. **Error Handling**: Implement retry logic for failed push operations.
3. **Reconnection**: Clients should support automatic reconnection with `Last-Event-ID` header.
4. **Event Size**: Keep payloads under 1MB to avoid performance issues.
5. **Monitoring**: Monitor `sse_events` table size and growth rate.

---

## See Also
- [Gateway Architecture](./gateway.md): Overall gateway design.
- [Network Architecture](./network.md): mTLS and PKI details.
- [Protocol Constants](../../protocol/constants/events.json): Complete event type catalog.
