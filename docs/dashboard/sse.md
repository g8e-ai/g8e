---
doc_id: dashboard-sse
title: Server-Sent Events
audience: developers and platform operators
status: current
last_updated: 2026-09-28
version: v2.2.3
owners:
  - dashboard/public/js/utils/sse-connection-manager.js
  - dashboard/public/js/utils/gateway-sse-normalizer.js
  - dashboard/public/js/utils/gateway-sse-polling-fallback.js
  - dashboard/public/js/constants/sse-constants.js
  - docs/dashboard/sse.md
related:
  - docs/architecture/sse.md
  - docs/dashboard/gateway.md
  - docs/dashboard/auth.md
  - docs/dashboard/architecture.md
  - docs/dashboard/devs.md
  - docs/dashboard/tests.md
  - docs/architecture/network.md
  - docs/architecture/auth.md
  - docs/ensemble/sse.md
  - docs/guides/build_observe_frontend.md
when_to_read: Building or debugging browser SSE integration, understanding reconnection behavior, testing telemetry delivery, or troubleshooting connection failures and polling fallback.
do_not_use_for:
  - Gateway SSE architecture and event model (docs/architecture/sse.md)
  - Gateway integration and CORS configuration (docs/dashboard/gateway.md)
  - Browser authentication and session identity (docs/dashboard/auth.md)
  - Dashboard container architecture (docs/dashboard/architecture.md)
  - Browser JavaScript coding patterns (docs/dashboard/devs.md)
---

# Server-Sent Events

## Purpose

The dashboard browser uses Server-Sent Events (SSE) to receive session-targeted application telemetry from the Gateway. Connections are credentialed through a Secure, HttpOnly web-session cookie and scoped to the active user session. SSE delivery is telemetry only—it does not authorize mutations, does not create a GovernanceEnvelope, and does not replace Gateway state, signed receipts, or durable application records. Gateway contract details and event registration are documented in [SSE Streaming](../architecture/sse.md).

## Quick index

- [Purpose](#purpose)
- [Quick index](#quick-index)
- [Invariants](#invariants)
- [Owned surfaces](#owned-surfaces)
- [Procedures](#procedures)
- [Anti-patterns](#anti-patterns)
- [Links out](#links-out)

## Invariants

| ID | Rule |
| --- | --- |
| INV-DASHBOARD-SSE-01 | The browser creates at most one SSEConnectionManager per browser context. Session changes replace the active connection before creating a new one. |
| INV-DASHBOARD-SSE-02 | The web-session identifier associates SSE connections with local authenticated state; it is not placed in the SSE URL and does not authenticate requests. Gateway authentication comes from the Secure, HttpOnly web-session cookie sent via `credentials: 'include'`. |
| INV-DASHBOARD-SSE-03 | Connections without a web-session identifier abort without creating an EventSource. A valid session identifier is required at connection time. |
| INV-DASHBOARD-SSE-04 | The inactivity timeout is 120 seconds; any message resets the timer. When the timer expires, the connection closes and a reconnect is scheduled. |
| INV-DASHBOARD-SSE-05 | Reconnect attempts use exponential backoff starting at one second, capped at 30 seconds, with up to 25 percent jitter and a one-second minimum. The manager allows up to ten reconnect attempts before polling fallback. |
| INV-DASHBOARD-SSE-06 | When EventSource reconnect attempts exhaust, polling fallback queries persisted push envelopes using credentialed GET requests to the Gateway. A 401 response stops polling and emits a connection-failed event with reason `polling_unauthenticated`. |
| INV-DASHBOARD-SSE-07 | All SSE messages are normalized through the same nested-envelope parser whether delivered by live stream or polling fallback. Infrastructure events (connection established, keepalive) are consumed as transport events and not forwarded to feature components. |
| INV-DASHBOARD-SSE-08 | Tab visibility changes do not close the SSE connection. When the tab becomes visible after being hidden, the manager attempts to reconnect if the connection is inactive and an active session remains. |

## Owned surfaces

| Surface | Path | Responsibility |
| --- | --- | --- |
| Connection manager | [dashboard/public/js/utils/sse-connection-manager.js](dashboard/public/js/utils/sse-connection-manager.js) | Manages credentialed EventSource, handles open/error states, tracks reconnect attempts, emits connection events |
| Envelope normalizer | [dashboard/public/js/utils/gateway-sse-normalizer.js](dashboard/public/js/utils/gateway-sse-normalizer.js) | Unwraps nested Gateway push envelopes and extracts event type, timestamp, payload, and ID |
| Polling fallback | [dashboard/public/js/utils/gateway-sse-polling-fallback.js](dashboard/public/js/utils/gateway-sse-polling-fallback.js) | Polls persisted events when live stream is unavailable, deduplicates by ID, respects 401 unauthenticated responses |
| Configuration constants | [dashboard/public/js/constants/sse-constants.js](dashboard/public/js/constants/sse-constants.js) | Defines keepalive timeout, reconnect delays, backoff behavior, and polling defaults |
| Browser event bus | [dashboard/public/js/utils/eventbus.js](dashboard/public/js/utils/eventbus.js) | Routes normalized application events to feature components (chat, Operator, approvals, status, authentication) |
| Unit tests | [dashboard/test/unit/frontend/sse/sse-connection-manager.unit.test.js](dashboard/test/unit/frontend/sse/sse-connection-manager.unit.test.js) | Tests EventSource construction, state transitions, inactivity timeout, session replacement, and polling fallback |

## Procedures

### Connection Lifecycle

1. A connection request without a web-session identifier stops without creating an EventSource.
2. An active connection for the requested session is reused.
3. A connection for another session is closed and cleaned up before the replacement is created.
4. The manager creates a credentialed EventSource against `gatewayUrl(ApiPaths.sse.stream())` with `{ withCredentials: true }`.
5. The `onopen` callback marks the connection active, resets reconnect and failure counters, stops any active polling fallback, emits a local connection-opened event, and starts the inactivity timer.
6. Each `onmessage` event resets the inactivity timer, passes the frame through `normalizeGatewayEvent`, updates the last event ID, and dispatches the normalized result through the event bus.
7. The `onerror` callback marks the connection inactive, emits a connection-error event, records the failure count, and schedules a reconnect.

### Session Replacement

When a user authenticates or changes session, the active web-session identifier is updated and a new connection is established. If an existing connection is active, it is closed and removed before the replacement is created.

`disconnect()` closes the EventSource, stops polling fallback, clears all timers (inactivity and reconnect), clears the active session association, and resets reconnect counters.

### Tab Visibility Handling

When a tab becomes visible again after being hidden, the manager reconnects if the EventSource is inactive and an active session association remains. The connection is maintained while the tab is hidden.

### Keepalive and Inactivity

The manager uses a 120-second inactivity timer (`SSEClientConfig.KEEPALIVE_TIMEOUT_MS`). The timer resets when the connection opens or any message arrives. When it expires, the manager closes the EventSource, marks it inactive, and schedules a reconnect.

### Reconnection Strategy

Scheduled reconnects use exponential backoff: the delay is `baseDelay * 2^attempt`, capped at 30 seconds, with jitter of ±25 percent of the exponential delay (minimum one second). The manager allows up to ten reconnect attempts (`SSEClientConfig.MAX_RECONNECT_ATTEMPTS`).

After ten failed reconnect attempts, the connection manager emits a connection-failed event with reason `max_attempts_exceeded` and starts the polling fallback.

### Polling Fallback

When the EventSource stream exhausts reconnect attempts, the polling fallback queries stored push envelopes from `GET /api/v1/sse/events?since_id=<id>&limit=<n>` with `credentials: 'include'`. Polling uses the same nested-envelope normalization as the live stream and dispatches decoded payloads through the existing event bus path.

Default polling interval is five seconds with a batch limit of 100 events. A 401 response stops polling immediately and emits a connection-failed event with reason `polling_unauthenticated`.

A successful EventSource reconnect stops polling and resumes live streaming.

### Event Envelope and Normalization

Gateway stream frames carry an SSE `id` field and a `data` field whose JSON value is the complete persisted push envelope. The application event is nested under that envelope's `event` field. `normalizeGatewayEvent` unwraps this structure and yields `{ id, type, timestamp, payload }` objects for the event bus.

Infrastructure events (PLATFORM_SSE_CONNECTION_ESTABLISHED, PLATFORM_SSE_KEEPALIVE_SENT) are consumed as transport metadata and are not forwarded to feature components.

### Deployment Requirements

A direct Gateway EventSource uses `{ withCredentials: true }` and requires all of the following conditions:

- `G8E_GATEWAY_URL` is set to the HTTPS Gateway origin reachable from the user's browser. The dashboard fails closed at startup if it is missing.
- The exact dashboard origin is allowed by Gateway credentialed CORS configuration, including scheme, hostname, and port.
- The browser trusts the Gateway certificate and runs the dashboard in a WebAuthn secure context (HTTPS or the browser's localhost exception).
- The Gateway issues a browser session cookie accepted for cross-origin credentialed requests. JavaScript does not read the cookie or put a raw session identifier in the URL.
- The dashboard Content Security Policy permits connections to the configured Gateway origin. It does not permit browser WebSocket connections.
- The deployed SSE endpoint `/api/v1/sse/stream` is reachable from the browser with the same origin as the one configured in `G8E_GATEWAY_URL`.

The dashboard container's enrolled g8ed workload identity is separate from the browser session and is not used for browser SSE requests.

### Testing

The Vitest suite in [dashboard/test/unit/frontend/sse/sse-connection-manager.unit.test.js](dashboard/test/unit/frontend/sse/sse-connection-manager.unit.test.js) covers:
- EventSource and credentialed URL construction
- Envelope normalization integration
- Connection lifecycle (open, error, close states)
- Session replacement and isolation
- Active-state reporting and disconnect cleanup
- Inactivity timeout and reconnect scheduling
- Polling fallback activation and 401 response handling
- Message event resets to inactivity timer

Deployment verification must exercise the complete configured path: a browser-trusted Gateway certificate, exact CORS origin configuration, valid Gateway web-session cookie, `G8E_GATEWAY_URL`, the live `/api/v1/sse/stream` route, Gateway envelope parsing, reconnect behavior under connection loss, and polling fallback when the stream is unavailable.

## Anti-patterns

- Placing the web-session identifier in the SSE URL instead of relying on the Secure, HttpOnly cookie for authentication. Authentication context must come from the cookie, not the URL.
- Hard-coding the Gateway origin in browser code. The origin must be loaded from `/g8e-config.js` so that browser and deployment configuration remain synchronized.
- Treating the 120-second inactivity timer as a heartbeat interval; the timer is reset on any message arrival, not a fixed interval.
- Updating `last_updated` or `version` metadata without auditing the document end-to-end against current code (INV-DOC-FMT-02 from docs/devs/docs.md).
- Duplicating SSE behavior documentation in multiple places. The canonical Gateway contract belongs in [docs/architecture/sse.md](../architecture/sse.md); the browser integration details belong here.
- Assuming persisted events are durable long-term records. The retention window exists only for reconnect support; use the governance record store for authoritative state.
- Emitting application events without registering them in `protocol/constants/events.json` with `"transport": ["sse"]`. The Gateway rejects unregistered events at 422.

## Links out

- [SSE Streaming](../architecture/sse.md): Gateway SSE event model, contract details, and event registration.
- [Gateway Integration](gateway.md): Browser CORS configuration and G8E_GATEWAY_URL setup.
- [Dashboard Architecture](architecture.md): Container lifecycle, workload enrollment, and security boundaries.
- [Authentication and Authorization](auth.md): Web-session identity and credential handling.
- [Developer Guide](devs.md): Browser module patterns and JavaScript standards.
- [Testing Guide](tests.md): Browser test setup and fixture management.
- [Network Architecture](../architecture/network.md): TLS, certificate trust, and cross-component transport.
- [Authentication and Authorization](../architecture/auth.md): mTLS identities and session model.
- [Ensemble SSE](../ensemble/sse.md): First-party event production and application event types.
- [Generator-Neutral Builder Guide](../guides/build_observe_frontend.md): The audited g8e-adapter and contract pack for generated observe frontends.
