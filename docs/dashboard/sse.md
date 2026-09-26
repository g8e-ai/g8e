# Server-Sent Events

## Purpose and Current Status

The dashboard source contains two different SSE implementations:

- The active first-party browser application uses `SSEConnectionManager` in `dashboard/public/js/utils/sse-connection-manager.js`. It connects to `${window.G8E_GATEWAY_URL}/api/v1/sse/stream` with `withCredentials: true`, normalizes the nested Gateway push envelope via `gateway-sse-normalizer.js`, and dispatches application payloads to the in-browser event bus.
- The separately packaged `dashboard/g8e-adapter/` contains the audited browser integration for generated observe frontends. Its `SseStream` uses the Gateway stream contract and its `SsePollingFallback` uses the stored-events contract. It is not the connection manager used by the first-party dashboard application.

The standard dashboard deployment is a static SPA host. The browser authenticates directly to the Gateway, while `dashboard/server.js` serves assets and `/g8e-config.js`; it does not proxy Gateway requests.

SSE delivery is telemetry. It does not authorize mutations, create a `GovernanceEnvelope`, or replace Gateway state, signed receipts, or durable application records. The Gateway contract and its authentication, replay, retention, and backpressure behavior are defined in [Gateway SSE Streaming](../architecture/sse.md).

## Intended Browser Architecture

The browser uses credentialed SSE for session-targeted activity because the Gateway WebSocket surface requires mTLS and is not available to browser clients. After authentication restores a valid Gateway browser session and obtains its web-session identifier, the application starts one `SSEConnectionManager`. The manager owns the browser `EventSource`; the in-browser event bus distributes decoded application payloads to chat, Operator, approval, status, and authentication components.

The web-session identifier is used by the manager to associate the connection with the local authenticated state. It is not placed in the SSE URL and does not authenticate the request. Gateway authentication comes from the Secure, HttpOnly web-session cookie, which the browser sends when the request uses credentials.

## First-Party Connection Lifecycle

The current first-party manager behaves as follows:

1. A connection request without a web-session identifier stops without creating an `EventSource`.
2. An active connection for the requested session is reused.
3. A connection for another session is closed before the replacement is created.
4. The manager creates a credentialed `EventSource` against `gatewayUrl(ApiPaths.sse.stream())` with `{ withCredentials: true }`.
5. The `open` callback marks the manager connected, resets reconnect and failure counters, emits a local connection-opened event, and starts the inactivity timer.
6. Each message resets the inactivity timer, passes the frame through `normalizeGatewayEvent`, and dispatches the result through `handleSSEEvent`.
7. The `error` callback marks the manager inactive, emits a local connection-error event, records the failure, and schedules another attempt.

`disconnect()` closes the source, clears the inactivity and reconnect timers, clears the active session association, and resets reconnect counters. A visible browser tab starts another attempt only when the current source is inactive and an active session association remains.

## Browser Event Envelope

The first-party manager normalizes Gateway stream frames before dispatch. A Gateway normal event frame has an SSE `id` and a `data` field whose JSON value is the complete persisted push envelope. The application event is nested under that envelope's `event` field. `gateway-sse-normalizer.js` unwraps this structure and yields `{ type, data }` objects for the event bus.

Infrastructure events (connection established, keepalive) are consumed as transport events and are not forwarded to feature components.

## Reconnection and Inactivity Behavior

The first-party manager uses a 120-second inactivity timer. The timer is reset when the connection opens or any message arrives. When it expires, the manager closes the source, marks it inactive, and schedules a reconnect.

Scheduled reconnects use exponential backoff starting at one second and capped at 30 seconds, with up to 25 percent positive or negative jitter and a one-second minimum. The manager schedules at most ten attempts before emitting a terminal connection-failed event with reason `max_attempts_exceeded`. Authentication currently treats that terminal event as session expiry and clears local authenticated state, although a connection failure does not by itself prove that the Gateway session expired.

## Polling Fallback (not yet implemented)

The plan-specified fallback to `GET /api/v1/sse/events?since_id=&limit=` when the stream drops is not yet implemented in the first-party SPA. The audited `dashboard/g8e-adapter/` provides `SsePollingFallback` for observe frontends. See [Generator-Neutral Builder Guide](../guides/build_observe_frontend.md).

## Authentication and Deployment Requirements

A direct Gateway `EventSource` uses `{ withCredentials: true }`. The deployment must satisfy all of these conditions:

- `G8E_GATEWAY_URL` is set to the HTTPS Gateway origin reachable from the user's browser. The dashboard host fails closed at startup if it is missing.
- The exact dashboard origin is allowed by Gateway credentialed CORS configuration, including scheme, hostname, and port.
- The browser trusts the Gateway certificate and runs the dashboard in a WebAuthn secure context, either HTTPS or the browser's localhost exception.
- The Gateway issues a browser session cookie accepted for the cross-origin request. JavaScript does not read the cookie or put a raw session identifier in the URL.
- The dashboard Content Security Policy permits connections to the configured Gateway origin. It does not permit browser WebSocket connections.

The dashboard container's enrolled `g8ed` workload identity is separate from the browser session and is not used by the static host for browser SSE requests.

## Tests and Verification Scope

The first-party Vitest suite tests the manager's Gateway stream URL construction, envelope normalization integration, credentialed `EventSource` construction, open and error state, session replacement, active-state reporting, disconnect cleanup, and inactivity timeout closure. Chat handler tests separately verify feature-specific interpretation of event payloads.

The adapter suite tests Gateway envelope normalization, durable event IDs, deduplication, stream sentinels, reconnect and visibility reconciliation, polling fallback, and public spectator SSE separately.

Deployment verification must exercise the complete configured path: a browser-trusted Gateway certificate, exact CORS origin, valid Gateway web-session cookie, `G8E_GATEWAY_URL`, the live `/api/v1/sse/stream` route, Gateway envelope parsing, and reconnect behavior.

## Related

- [Gateway SSE Streaming](../architecture/sse.md) - Gateway push ingestion, persistence, replay, framing, retention, and consumer authentication
- [Gateway Integration](gateway.md) - Browser-direct connectivity, runtime configuration, and request ownership
- [Dashboard Architecture](architecture.md) - Dashboard boundaries, deployment, and feature status
- [Network Architecture](../architecture/network.md) - Gateway TLS surfaces, ports, and network topology
- [Ensemble SSE](../ensemble/sse.md) - First-party event production pipeline
- [Generator-Neutral Builder Guide](../guides/build_observe_frontend.md) - Audited adapter and observe frontend contract
- [Authentication](auth.md) - Browser session behavior and dashboard workload enrollment
- [Testing](tests.md) - Dashboard test commands and scope
