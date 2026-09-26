# Server-Sent Events

## Purpose and Current Status

The dashboard source contains two different SSE implementations:

- The active first-party browser application uses `SSEConnectionManager` in `dashboard/public/js/utils/sse-connection-manager.js`. It is intended to receive session-scoped activity through the Gateway.
- The separately packaged `dashboard/g8e-adapter/` contains the audited browser integration for generated observe frontends. Its `SseStream` uses the Gateway stream contract and its `SsePollingFallback` uses the stored-events contract. It is not the connection manager used by the first-party dashboard application.

The standard dashboard deployment is a static SPA host. The browser authenticates directly to the Gateway, while `dashboard/server.js` serves assets and `/g8e-config.js`; it does not proxy Gateway requests. The first-party `SSEConnectionManager` connects to `${window.G8E_GATEWAY_URL}/api/v1/sse/stream` with `withCredentials: true` and normalizes the nested Gateway push envelope before dispatching to the in-browser event bus.

SSE delivery is telemetry. It does not authorize mutations, create a `GovernanceEnvelope`, or replace Gateway state, signed receipts, or durable application records. The Gateway contract and its authentication, replay, retention, and backpressure behavior are defined in [Gateway SSE Streaming](../architecture/sse.md).

## Intended Browser Architecture

The browser uses credentialed SSE for session-targeted activity because the Gateway WebSocket surface requires mTLS and is not available to browser clients. After authentication restores a valid Gateway browser session and obtains its web-session identifier, the application starts one `SSEConnectionManager`. The manager owns the browser `EventSource`; the in-browser event bus distributes decoded application payloads to chat, Operator, approval, status, and authentication components.

The web-session identifier is used by the manager to associate the connection with the local authenticated state. It is not placed in the SSE URL and does not authenticate the request. Gateway authentication comes from the Secure, HttpOnly web-session cookie, which the browser sends when the request uses credentials.

## First-Party Connection Lifecycle

The current first-party manager behaves as follows:

1. A connection request without a web-session identifier stops without creating an `EventSource`.
2. An active connection for the requested session is reused.
3. A connection for another session is closed before the replacement is created.
4. The manager creates a credentialed `EventSource` with `{ withCredentials: true }`.
5. The `open` callback marks the manager connected, resets reconnect and failure counters, emits a local connection-opened event, and starts the inactivity timer.
6. Each message resets the inactivity timer, parses the message as JSON, and dispatches it through `handleSSEEvent`.
7. The `error` callback marks the manager inactive, emits a local connection-error event, records the failure, and schedules another attempt.

`disconnect()` closes the source, clears the inactivity and reconnect timers, clears the active session association, and resets reconnect counters. A visible browser tab starts another attempt only when the current source is inactive and an active session association remains. The manager also performs an open-state check before a scheduled reconnect; the browser's native `EventSource` implementation may independently retry while it remains in its connecting state.

## Browser Event Envelope

The first-party manager expects the legacy dashboard-local envelope:

```json
{
  "type": "llm.chat.iteration.text.chunk.received",
  "data": {
    "content": "partial response"
  }
}
```

The manager requires a top-level string `type`. For non-infrastructure events it also requires a `data` property; that property may be `null` and is emitted unchanged through the event bus. Connection-establishment and keepalive event types are consumed as transport events and are not forwarded to feature components. Invalid JSON, a missing or non-string `type`, and non-infrastructure events without `data` are dropped.

This is not the Gateway stream envelope. A Gateway normal event frame has an SSE `id` and a `data` field whose JSON value is the complete persisted push envelope. The application event is nested under that envelope's `event` field, which may contain a JSON string or an object, and the application `type` and payload are inside the nested value. The Gateway does not emit a normal SSE `event` field. A client that connects directly to the Gateway must unwrap and validate this nested structure; the first-party manager does not do so.

## Reconnection and Inactivity Behavior

The first-party manager uses a 120-second inactivity timer. The timer is reset when the connection opens or any message arrives. When it expires, the manager closes the source, marks it inactive, and schedules a reconnect.

Scheduled reconnects use exponential backoff starting at one second and capped at 30 seconds, with up to 25 percent positive or negative jitter and a one-second minimum. The manager schedules at most ten attempts before emitting a terminal connection-failed event with reason `max_attempts_exceeded`. Authentication currently treats that terminal event as session expiry and clears local authenticated state, although a connection failure does not by itself prove that the Gateway session expired.

The manager records rapid failures occurring within five seconds for diagnostics. The rapid-failure count does not currently alter the computed delay. The unit suite does not prove the exact reconnect timing, jitter, exhaustion, visibility, invalid-JSON callback, or end-to-end Gateway behavior.

## Current URL and Contract Constraint

The first-party manager currently constructs `EventSource(ApiPaths.sse.events(), { withCredentials: true })`, where `ApiPaths.sse.events()` is the relative path `/api/v1/sse/events`.

This creates three independent incompatibilities with the direct Gateway browser contract:

1. **Wrong origin:** A relative URL resolves against the static dashboard host. Although `G8E_GATEWAY_URL` is injected through `/g8e-config.js` and is used by the dashboard's general Gateway service client, the SSE manager does not use it. The static host has no proxy or handler for this request.
2. **Wrong endpoint:** `/api/v1/sse/events` is the Gateway's finite JSON polling endpoint. A live SSE connection must use `/api/v1/sse/stream`.
3. **Wrong envelope:** The manager expects a flat `{ type, data }` object, while Gateway stream frames carry the complete push envelope and nest the application event under `event`.

Adding a same-origin proxy for `/api/v1/sse/events` would fix only the origin and would still request a finite JSON response with the wrong wire contract. A functional direct integration must construct an absolute URL from the configured Gateway origin, use `/api/v1/sse/stream`, send credentials, and unwrap the Gateway push envelope. It must also handle Gateway replay IDs, heartbeats, `truncated` and `replay_failed` sentinels, retention limits, and live-buffer drops. The audited `dashboard/g8e-adapter/` implementation provides this integration core and a polling fallback; see [Generator-Neutral Builder Guide](../guides/build_observe_frontend.md).

## Authentication and Deployment Requirements

A direct Gateway `EventSource` uses `{ withCredentials: true }`. The deployment must satisfy all of these conditions:

- `G8E_GATEWAY_URL` is set to the HTTPS Gateway origin reachable from the user's browser. The dashboard host fails closed at startup if it is missing.
- The exact dashboard origin is allowed by Gateway credentialed CORS configuration, including scheme, hostname, and port.
- The browser trusts the Gateway certificate and runs the dashboard in a WebAuthn secure context, either HTTPS or the browser's localhost exception.
- The Gateway issues a browser session cookie accepted for the cross-origin request. JavaScript does not read the cookie or put a raw session identifier in the URL.
- The dashboard Content Security Policy permits connections to the configured Gateway origin. It does not permit browser WebSocket connections.

The dashboard container's enrolled `g8ed` workload identity is separate from the browser session and is not used by the static host for browser SSE requests. The browser authenticates directly to the Gateway; the dashboard host is not an mTLS browser proxy.

## Retained Dashboard SSE Routes and Service

The source tree retains an older dashboard-local SSE design that is separate from the Gateway SSE bridge:

- `routes/platform/sse_routes.js` defines a browser-authenticated `GET /events` route relative to the dashboard's platform router. It registers one in-memory response per web session, emits a connection-established event, pushes initial state, and sends keepalives every 20 seconds.
- `routes/internal/internal_sse_routes.js` defines an internal-origin `POST /push` route for application events. It can normalize citation numbers and replace the Operator-list payload before forwarding the event to the local service.
- `services/platform/sse_service.js` stores connections only in process memory, writes `data: <json>` frames directly to the response, replaces an existing connection for a session, and returns successfully even when no connection is active. It does not persist events, replay missed events, or provide durable cursors.
- The platform router also defines an internal-only `/health` route, and the internal router mounts the internal `/sse/push` route in that retained application.

`dashboard/server.js` does not import or instantiate these routers or service. The static host serves files only, so these retained paths are not deployed dashboard endpoints and must not be used as the Gateway integration contract.

## Tests and Verification Scope

The first-party Vitest suite tests the manager's direct event-dispatch behavior, infrastructure-event suppression, credentialed `EventSource` construction, open and basic error state, session replacement, active-state reporting, disconnect cleanup, and inactivity timeout closure. Chat handler tests separately verify feature-specific interpretation of event payloads.

The adapter suite tests Gateway envelope normalization, durable event IDs, deduplication, stream sentinels, reconnect and visibility reconciliation, polling fallback, and public spectator SSE separately. Those tests do not activate the first-party dashboard manager or prove that the static host is connected to a live Gateway.

Deployment verification must exercise the complete configured path: a browser-trusted Gateway certificate, exact CORS origin, valid Gateway web-session cookie, `G8E_GATEWAY_URL`, the live `/api/v1/sse/stream` route, Gateway envelope parsing, replay and reconnect behavior, and the actual dashboard build or adapter selected for deployment.

## Related

- [Gateway SSE Streaming](../architecture/sse.md) - Gateway push ingestion, persistence, replay, framing, retention, and consumer authentication
- [Gateway Integration](gateway.md) - Browser-direct connectivity, runtime configuration, and current request ownership
- [Dashboard Architecture](architecture.md) - Dashboard boundaries, deployment, and active feature status
- [Network Architecture](../architecture/network.md) - Gateway TLS surfaces, ports, and network topology
- [Ensemble SSE](../ensemble/sse.md) - First-party event production pipeline
- [Generator-Neutral Builder Guide](../guides/build_observe_frontend.md) - Audited adapter and observe frontend contract
- [Authentication](auth.md) - Browser session behavior and dashboard workload enrollment
- [Testing](tests.md) - Dashboard test commands and scope
