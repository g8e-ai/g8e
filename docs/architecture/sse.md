---
title: SSE Streaming
---

# SSE Streaming

Last Updated: 2026-08-07
Version: v1.7.0

The Governance Gateway provides a Server-Sent Events (SSE) streaming infrastructure that enables real-time event delivery from app workloads to browser and CLI clients. g8e-compatible agentic ensembles publish typed events, including audit events, for downstream consumption. The gateway also produces SSE events internally for platform workflows such as passkey registration and L3 transaction approval.

---

## Overview

The SSE system provides three endpoints:

- **`POST /api/v1/sse/push`**: App workloads push events. Requires mTLS with app workload identity.
- **`GET /api/v1/sse/events`**: Poll for historical events. Supports dual auth: mTLS for CLI or operator, web session cookie for browser.
- **`GET /api/v1/sse/stream`**: Real-time SSE stream with live event delivery. Supports dual auth: mTLS for CLI or operator, web session cookie for browser. All clients use this single endpoint.

Every event carries two dimensions: an ownership dimension (`user_id`, always required) and a delivery dimension (exactly one of `web_session_id` or `cli_session_id`). The gateway enforces this at the type level so a `web_session_id` can never be mis-delivered as a `cli_session_id` or vice versa. `user_id` alone is not a valid route; it must always be paired with a session identifier.

---

## Architecture

```mermaid
flowchart TD
    subgraph App ["App Workload"]
        producer["Event Producer"]
    end

    subgraph Gateway ["Governance Gateway"]
        push["POST /api/v1/sse/push"]
        internal["Internal Producers\n(approval, passkey)"]
        db[("Event Store")]
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
    internal --> db
    internal --> pubsub
    db --> events
    db --> stream
    pubsub --> stream
    browser -- "mTLS or cookie" --> stream
    browser -- "mTLS or cookie" --> events
```

---

## Endpoints

### POST /api/v1/sse/push

**Authentication**: mTLS with app workload identity. The caller certificate must have a SPIFFE URI SAN with an `/app/` prefix. Gateway and Operator identities are blocked from pushing.

**Request**: The body must include `user_id` (required), exactly one of `web_session_id` or `cli_session_id` (required delivery target), and an `event` object containing a `type` string and payload data.

**Response**: Returns a success status and delivered count.

**Authorization**: The app identity must be associated with the target session. Ownership is verified against bound Operator sessions before the event is persisted. If ownership verification fails, the handler returns a 403 Forbidden status and no event is stored.

**Pub/Sub**: On success, the event payload is published to the target channel for real-time fan-out.

### Internal SSE Producers

The gateway produces SSE events directly, bypassing the push endpoint. These events use `g8eg` as the producer identifier for attribution. Two internal event types exist:

- **`approval.completed`**: Emitted when a user completes the WebAuthn approval ceremony for an L3 transaction. Scoped to the `cli_session_id` that submitted the transaction, so the waiting CLI client receives real-time notification without polling.
- **`passkey.registered`**: Emitted when a new passkey is enrolled. Scoped to `cli_session_id` so the waiting CLI client receives real-time notification.

### GET /api/v1/sse/events

**Authentication**: Dual auth. If a client certificate is present, mTLS authentication is used. Otherwise, the web session cookie is validated.

**Routing**: The route is built entirely from auth context. For mTLS, `user_id` is derived from the certificate and `cli_session_id` is sent via the `X-G8E-CLI-Session-ID` header. For cookie auth, `web_session_id` is derived from the session cookie. Routing identifiers must not be passed in the URL.

**Query Parameters**:
- `since_id`: Return events with ID greater than this value (default: 0)
- `limit`: Maximum events to return (default: 200, max: 1000)

**Response**: Returns an ordered list of events ascending by ID with count. Unset routing fields are omitted from each event in the response.

**Authorization**: The authenticated identity must own the requested routing target. For mTLS auth, Operator session ownership or CLI user ownership is verified. For cookie auth, the web session ID or CLI session user must match the authenticated identity.

### GET /api/v1/sse/stream

**Authentication**: Dual auth, same as the events endpoint. When both a client certificate and cookie are present, mTLS takes precedence.

**Routing**: Same as the events endpoint. The route is built entirely from auth context. For mTLS, `user_id` is derived from the certificate and `cli_session_id` is sent via the `X-G8E-CLI-Session-ID` header. For cookie auth, `web_session_id` is derived from the session cookie. Routing identifiers must not be passed in the URL.

**Query Parameters**:
- `since_id`: Start from event ID (also supports the `Last-Event-ID` header for reconnection)

**Response**: A standard SSE stream (`text/event-stream`). The stream sets `Cache-Control: no-cache`, `Connection: keep-alive`, and `X-Accel-Buffering: no` headers.

**Replay**: Replay behavior depends on the `since_id` parameter and `Last-Event-ID` header:
- If `Last-Event-ID` is present, replay starts from that cursor (0 means from the beginning).
- If `since_id` is present and greater than 0, replay starts from that ID.
- If `since_id` is absent (fresh connection), the full backlog is replayed from ID 0.
- If `since_id` is explicitly 0 and no `Last-Event-ID` header is present, replay is skipped and the stream starts with only real-time events.

Replay returns up to 1000 rows from the event store. Each replayed event includes an `id:` field. If the replay query fails, an error sentinel event is emitted on the stream so the client can detect the gap. If the replay hits the 1000-row limit, a truncation sentinel event is emitted signaling that more backlog may exist.

**Live events**: Real-time events from pub/sub are emitted with both `id:` and `data:` fields. The `id:` field carries the database row ID, enabling deduplication against replayed events. The `data:` field carries the full push payload. No `event:` field is emitted; the event type is embedded in the data payload.

**Deduplication**: The stream maintains a cursor tracking the highest event ID emitted. Live events with an ID less than or equal to the cursor are suppressed, preventing duplicate delivery of events that were already sent during replay.

**Heartbeat**: The stream sends a heartbeat comment every 30 seconds to keep the connection alive.

**Back-pressure**: The stream uses a 100-element buffered event channel with a drop-oldest policy. If the buffer fills, the oldest queued event is evicted to make room for newer events. Evicted events can be recovered via database replay on reconnect.

---

## Event Types

The SSE system is generic and supports any event type. The protocol catalog defines audit event types (AI actions, command execution, MCP calls, user actions) and platform SSE lifecycle event types (connection established, opened, closed, failed, error, keepalive sent).

Two gateway-produced event types are managed internally:

- `approval.completed`: L3 transaction approval completed, scoped to `cli_session_id`
- `passkey.registered`: Passkey enrollment completed, scoped to `cli_session_id`

See the [Constants Reference](../../protocol/docs/constants.md) for the complete event type listing.

---

## Security Model

### Producer Authorization

Only app workloads with valid mTLS certificates can push events. The certificate must have a SPIFFE URI SAN with an `/app/` prefix. Gateway and Operator identities are blocked from pushing. Producer identity is recorded for attribution.

The app identity must be associated with the target session. Ownership is verified against bound Operator sessions before the event is persisted. If ownership verification fails, the handler returns 403 and no event is stored. The gateway also produces events internally (approval, passkey) by writing directly to the event store and pub/sub broker.

### Consumer Authorization

SSE consumer endpoints support dual auth: mTLS with an Operator session or CLI user, or web session cookie for browser access. When both are present, mTLS takes precedence. App workload certificates are rejected from consumer endpoints.

The route is built entirely from auth context, not from URL parameters. For mTLS, `user_id` is derived from the certificate and `cli_session_id` is sent via the `X-G8E-CLI-Session-ID` header. For cookie auth, `web_session_id` is derived from the session cookie.

Authorization is enforced per routing target as defense-in-depth:
- **mTLS path**: For `web_session_id`, the Operator session must own the web session. For `cli_session_id`, the Operator session must own the CLI session, or the CLI user must match. The route's `user_id` must always match the authenticated user.
- **Cookie path**: The `web_session_id` is always derived from the authenticated session cookie (never from the URL). The session must match. For `cli_session_id`, the CLI session user must match. The route's `user_id` must always match the authenticated user.

Multi-tenant isolation is enforced at the query level.

### Transport Security

- SSE push requires mTLS on HTTPS port 8443 with app workload identity
- SSE consumer endpoints support dual auth on HTTPS port 8443
- Not available on HTTP bootstrap port 8080
- Pub/Sub channels are scoped to routing targets

---

## Use Cases

### Real-time Audit Streaming

App workloads push audit events as they occur, enabling real-time audit log viewers in the web UI or CLI. Route to a specific `web_session_id` or `cli_session_id` for targeted delivery.

### LLM Streaming

g8e-compatible agentic ensembles stream LLM generation chunks to the browser by pushing events scoped to `web_session_id`. The browser SSE consumer renders chunks as they arrive.

### L3 Approval Workflow

When a transaction requires L3 notary approval, the gateway suspends it and emits an `approval.completed` event scoped to the `cli_session_id` that submitted the transaction, once the user completes the WebAuthn ceremony. CLI clients subscribe to the SSE stream and resume automatically without polling.

### Passkey Enrollment

During interactive passkey enrollment, the CLI opens the browser console for WebAuthn registration. When enrollment completes, the gateway emits a `passkey.registered` event scoped to `cli_session_id`. The CLI client receives the event in real-time and proceeds.

---

## Event Retention

Events are ephemeral telemetry, not governance state. SSE event inserts do not alter the state root, allowing high-frequency streaming without governance overhead.

**Automatic cleanup**: The gateway maintenance loop prunes events older than 1 hour, running every 30 seconds.

**Admin endpoints**: Administrators can wipe all events or query the total count via authenticated admin endpoints on the data API.

---

## CLI Consumers

The CLI includes a reusable SSE client that connects to the gateway stream, parses frames, and dispatches events to a handler. It supports reconnection with exponential backoff and jitter, capped at 30 seconds, and sends the `Last-Event-ID` header on reconnect for cursor-based replay. Custom headers (such as `X-G8E-CLI-Session-ID`) are set for mTLS session identification.

Three CLI consumers use this client:
- **Approval wait**: Blocks until an `approval.completed` event with a matching transaction hash arrives, with a 3-minute timeout. Used by the `g8e approve` command and the MCP L3 approval flow.
- **Passkey enrollment**: Waits for a `passkey.registered` event during interactive passkey enrollment, with a 5-minute timeout.
- **TUI adapter**: Subscribes to the SSE stream and translates events into terminal UI messages for the dashboard view. Uses a fixed 3-second reconnection interval.

---

## Best Practices

1. **Event batching**: For high-frequency events, consider batching before pushing to reduce database load.
2. **Error handling**: Implement retry logic for failed push operations.
3. **Reconnection**: Clients should support automatic reconnection with `Last-Event-ID` header.
4. **Event size**: Keep payloads under 1MB to avoid performance issues.
5. **Monitoring**: Monitor event store size and growth rate.

---

## See Also
- [Gateway Architecture](./gateway.md): Overall gateway design.
- [Network Architecture](./network.md): mTLS and PKI details.
- [Constants Reference](../../protocol/docs/constants.md): Platform constant system details.
