---
doc_id: sse
title: SSE Streaming
audience: maintainers and coding agents
status: current
last_updated: 2026-09-28
version: v2.2.3
owners:
  - internal/services/gateway/sse_controller.go
  - internal/services/gateway/sse_event_service.go
  - internal/services/gateway/observe_producer_controller.go
  - internal/services/gateway/observe_producer.go
related:
  - docs/architecture/gateway.md
  - docs/architecture/auth.md
  - docs/architecture/network.md
  - docs/ensemble/sse.md
  - docs/dashboard/sse.md
  - docs/guides/build_observe_frontend.md
when_to_read: Understanding SSE event flow, integration patterns for telemetry producers, gateway stream architecture, or client reconnection behavior.
do_not_use_for:
  - Governance envelope model (see gateway.md)
  - Authentication mechanisms (see auth.md)
  - Enrollment and approval state (see protocol.md)
---

# SSE Streaming

## Purpose

Defines the Gateway's SSE event bridge for session-targeted application telemetry and internal workflow notifications. SSE is an append-only, session-scoped delivery channel for asynchronous events; publishing an event neither mutates governed state nor alters the five-layer governance interlock. Authoritative outcomes live in signed receipts and platform records.

## Quick index

- [Purpose](#purpose)
- [Quick index](#quick-index)
- [Invariants](#invariants)
- [Owned surfaces](#owned-surfaces)
- [Procedures](#procedures)
- [Anti-patterns](#anti-patterns)
- [Links out](#links-out)

Invariant groups: [Event model](#event-model-inv-sse-evt), [Producer authentication](#producer-authentication-inv-sse-prod), [Consumer authentication](#consumer-authentication-inv-sse-cons), [Delivery semantics](#delivery-semantics-inv-sse-del).

## Invariants

Ids are stable. Append the next free number in a topic. Do not renumber.

### Event model (`INV-SSE-EVT`)

| ID | Rule |
| --- | --- |
| INV-SSE-EVT-01 | Every SSE event is keyed by two routing dimensions: `user_id` (ownership) and exactly one of `web_session_id` or `cli_session_id` (delivery target). A bare `user_id` is not a valid route. The Gateway does not provide user-wide fan-out. |
| INV-SSE-EVT-02 | The event registration in `protocol/constants/events.json` is the authority for whether an event is published, persisted, or ephemeral. Only events with `transport: "sse"` and a listed producer are accepted; unregistered, non-SSE, or wrong-producer events are rejected at the Gateway boundary with 422. |
| INV-SSE-EVT-03 | SSE frames carry `id:` and `data:` fields; no `event:` field is emitted. Consumers read the application event type from the nested `event.type` value in the JSON data payload. Heartbeats are SSE comments and do not advance the event cursor. |
| INV-SSE-EVT-04 | Ephemeral events are delivered via live streams but not persisted to the SSE store; consumers cannot later recover them via replay. Persist-before-publish ordering ensures rejected events never reach consumers. |

### Producer authentication (`INV-SSE-PROD`)

| ID | Rule |
| --- | --- |
| INV-SSE-PROD-01 | `POST /api/v1/sse/push` requires an mTLS peer certificate with an app SPIFFE URI SAN under `/app/`. The handler accepts any app identity except the reserved Gateway and Operator identities (`/app/g8eo` and `/app/g8eg`). Gateway and Operator certificates are not accepted as external SSE producers. |
| INV-SSE-PROD-02 | A non-ensemble producer's target web session must have an Operator associated with the app identity; a CLI target's session must resolve to an Operator also associated with the app. The ensemble identity (`spiffe://g8e.local/app/g8ee`) bypasses per-Operator checks after normal app authentication. Authorization occurs before persistence, so rejected pushes never create durable SSE rows. |
| INV-SSE-PROD-03 | The observe producer endpoints (`POST /api/v1/observe/producer/agent-state`, `POST /api/v1/observe/producer/run-state`) accept only app-workload mTLS certificates. Strict JSON decoding rejects unknown fields. `user_id` is derived from the mTLS peer certificate; the request body carries no `user_id` field. |

### Consumer authentication (`INV-SSE-CONS`)

| ID | Rule |
| --- | --- |
| INV-SSE-CONS-01 | Consumers use dual authentication: either a web-session cookie (browser) or mTLS certificate + `X-G8E-CLI-Session-ID` header (CLI or Operator). When both are presented, mTLS takes precedence; an invalid certificate does not fall back to cookie auth. App certificates cannot consume SSE events. |
| INV-SSE-CONS-02 | `GET /api/v1/sse/events` (polling) and `GET /api/v1/sse/stream` (live) build the route entirely from authenticated context; routing query parameters are ignored. Queries match both the authenticated user and session before returning rows. |
| INV-SSE-CONS-03 | An Operator holding an mTLS certificate can select an owned CLI or web session target via the corresponding header. The Gateway verifies the target belongs to that Operator session before returning events. |

### Delivery semantics (`INV-SSE-DEL`)

| ID | Rule |
| --- | --- |
| INV-SSE-DEL-01 | `GET /api/v1/sse/events` returns rows with id > `since_id`, ordered ascending, with `limit` defaulting to 200 and clamped to [1, 1000]. Polling does not delete or acknowledge rows. |
| INV-SSE-DEL-02 | `GET /api/v1/sse/stream` replays at most 1,000 rows per connection. When replay hits the limit, an unnumbered `truncated` sentinel carries the last emitted id and limit; the client may reconnect with a higher cursor. A `Last-Event-ID` header or positive `since_id` query parameter requests replay; `since_id=0` without `Last-Event-ID` requests live-only (skips replay). |
| INV-SSE-DEL-03 | Each live stream maintains a 100-event in-memory queue. If a consumer falls behind, the Gateway drops the oldest queued event; the consumer can recover via cursor-based replay on reconnect. A dropped event is not automatically backfilled on the same connection. |
| INV-SSE-DEL-04 | The stream heartbeats every 30 seconds with an SSE comment; response headers are flushed immediately so clients may signal readiness before the first event. The maintenance loop (every 30 seconds) removes events older than one hour; this history window supports short reconnect windows and is not a durable audit record. |

## Owned surfaces

| Claim | Path | Verify |
| --- | --- |
| SSE push handler | `internal/services/gateway/sse_controller.go:handleInternalSSEPush` | Validates producer auth, event registry, target ownership, persist-before-publish |
| SSE poll handler | `internal/services/gateway/sse_controller.go:handleInternalSSEEvents` | Builds route from auth context, enforces user/session ownership, returns rows since ID |
| SSE stream handler | `internal/services/gateway/sse_controller.go:handleInternalSSEStream` | Replay + live delivery, dedup by row ID, 30s heartbeat, 100-event backpressure queue |
| SSE event storage | `internal/services/gateway/sse_event_service.go` | Append, list, cleanup (every 30s, remove >1h old), wipe, count |
| Observe producers | `internal/services/gateway/observe_producer_controller.go` | Agent-state and run-state endpoints, persist-before-publish, auth from mTLS cert |
| Event registry | `protocol/constants/events.json` | Authority for event registration, producer list, persistence mode |

## Procedures

### Producing an event

1. Producer obtains mTLS app certificate (SPIFFE ID under `/app/`, not `/app/g8eg` or `/app/g8eo`).
2. POST the event envelope to `/api/v1/sse/push` with `Content-Type: application/json`.
   - Body: `{ "user_id": "<uid>", "web_session_id": "<sid>" | "cli_session_id": "<sid>", "event": <JSON object with "type" field> }`
   - Exactly one of `web_session_id` or `cli_session_id` is required; both is an error.
3. Gateway validates event type against `protocol/constants/events.json`:
   - Event must be registered.
   - `transport` array must include `"sse"`.
   - Producer's SPIFFE ID must be in the `producers` list (or producer must be "ensemble").
4. Gateway verifies producer owns the target session:
   - For web target: an Operator bound to that web session must be associated with the producer's app ID.
   - For CLI target: the session's Operator must be associated with the producer's app ID.
   - Ensemble (`/app/g8ee`) skips this check.
5. Gateway persists the row if `persistence != "ephemeral"`, then publishes to the live channel.
6. Response: `{ "success": true, "delivered": 1 }` (200) or error status with reason.

### Consuming events via polling

1. Authenticate via cookie (browser) or mTLS + header (CLI/Operator).
2. GET `/api/v1/sse/events?since_id=<id>&limit=<n>` (optional query parameters).
3. Gateway builds route from auth context (ignores query IDs), verifies ownership, queries the DB.
4. Returns: `{ "events": [ { "id": 1, "user_id": "alice", "web_session_id": "s1", "event_type": "eval.started", "payload": "{...}", "created_at": "2026-09-28T..." }, ... ], "count": <n> }`
5. Repeat with updated `since_id` to poll for new events.

### Consuming events via live stream

1. Authenticate via cookie (browser) or mTLS + header (CLI/Operator).
2. GET `/api/v1/sse/stream` (with optional `Last-Event-ID` header or `since_id` query param).
3. Stream begins with response headers immediately flushed.
4. Replay phase: Gateway sends up to 1,000 stored events (one per frame) or a `truncated` sentinel if limit is hit.
5. Live phase: Gateway sends new events as they arrive, with 30s heartbeat comments.
6. Deduplication: Events emitted during replay are suppressed when they appear on the live channel.
7. Reconnection: Client sends `Last-Event-ID: <id>` header; Gateway resumes from that cursor.
8. On disconnect: Client may reconnect and recover missed events; dropped oldest events can be recovered via DB replay.

### Running maintenance

The Gateway runs maintenance every 30 seconds automatically:

```go
// internal/services/gateway/gateway_db.go
if _, err := s.stores.SSEStore.SSEEventsCleanup(time.Hour); err != nil {
    s.logger.Warn("SSE event cleanup error", "error", err)
}
```

- Deletes all rows where `created_at < now() - 1 hour`.
- Does not delete or acknowledge polled rows.
- Live replay continues to function during cleanup.

## Anti-patterns

- Publishing an event and immediately querying for it without waiting for stream delivery. Replay is not instantaneous; use a stream connection with live subscriptions.
- Sending `user_id` in the request body to SSE consumer endpoints. Route is built entirely from auth context; body IDs are ignored.
- Supplying both `web_session_id` and `cli_session_id` in a producer request, or neither. The Gateway rejects exactly-one violations at 400/422.
- Assuming persisted events are durable long-term records. The 1-hour retention window is for reconnect support, not audit logging. Use the governance record store for authoritative state.
- Registering a producer event without listing it in `protocol/constants/events.json` with `"transport": ["sse"]`. Unregistered events are rejected at 422.
- Emitting events with `Last-Event-ID` set by the client as the replay start, then forgetting that the client may hold stale IDs after network partition. The server replays from the highest available ID; if older rows have been cleaned up, the client may miss intermediate events.

## Links out

- [Gateway Architecture](./gateway.md): Gateway services and protocol surfaces.
- [Authentication and Authorization](./auth.md): mTLS identities, CLI sessions, web sessions, and app policies.
- [Network Architecture](./network.md): TLS listeners, PKI, and cross-component transport.
- [Ensemble SSE](../ensemble/sse.md): First-party event production and application event types.
- [Dashboard SSE](../dashboard/sse.md): Browser connection lifecycle and current integration constraints.
- [Generator-Neutral Builder Guide](../guides/build_observe_frontend.md): The audited g8e-adapter and contract pack for generated observe frontends.
- [Public Spectator Architecture and Threat Model](./public_spectator.md): The separate anonymous public-mirror SSE relay and outbound-only export architecture.
- [AI Agents and the Governance Boundary](./agents.md): Distinction between event telemetry and governed execution.
- [Evaluations](./evals.md): Model campaign evidence and observe publication.
- [Protocol Reference](./protocol.md): Event registration, status maps, and constant inventory.
