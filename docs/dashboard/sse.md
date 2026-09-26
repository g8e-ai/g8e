# Server-Sent Events

## Overview

The first-party browser application uses `SSEConnectionManager` in `dashboard/public/js/utils/sse-connection-manager.js`. It connects to `${window.G8E_GATEWAY_URL}/api/v1/sse/stream` with `withCredentials: true`, normalizes the nested Gateway push envelope through `gateway-sse-normalizer.js`, and dispatches application payloads to the in-browser event bus.

The separately packaged `dashboard/g8e-adapter/` supplies the audited browser integration for generated observe frontends. Its `SseStream` and `SsePollingFallback` implement the same Gateway contracts for adapter consumers. The first-party dashboard does not import the adapter at runtime.

The dashboard host serves assets and `/g8e-config.js` only; it does not proxy Gateway requests.

SSE delivery is telemetry. It does not authorize mutations, create a `GovernanceEnvelope`, or replace Gateway state, signed receipts, or durable application records. Gateway contract details are in [Gateway SSE Streaming](../architecture/sse.md).

## Browser Architecture

The browser uses credentialed SSE for session-targeted activity because the Gateway WebSocket surface requires mTLS and is not available to browser clients. After authentication restores a valid Gateway browser session and obtains its web-session identifier, the application starts one `SSEConnectionManager`. The manager owns the browser `EventSource`; the in-browser event bus distributes decoded application payloads to chat, Operator, approval, status, and authentication components.

The web-session identifier associates the connection with local authenticated state. It is not placed in the SSE URL and does not authenticate the request. Gateway authentication comes from the Secure, HttpOnly web-session cookie, which the browser sends when the request uses credentials.

## Connection Lifecycle

1. A connection request without a web-session identifier stops without creating an `EventSource`.
2. An active connection for the requested session is reused.
3. A connection for another session is closed before the replacement is created.
4. The manager creates a credentialed `EventSource` against `gatewayUrl(ApiPaths.sse.stream())` with `{ withCredentials: true }`.
5. The `open` callback marks the manager connected, resets reconnect and failure counters, stops any active polling fallback, emits a local connection-opened event, and starts the inactivity timer.
6. Each message resets the inactivity timer, passes the frame through `normalizeGatewayEvent`, updates the last event ID, and dispatches the result through `handleSSEEvent`.
7. The `error` callback marks the manager inactive, emits a local connection-error event, records the failure, and schedules another attempt.

`disconnect()` closes the source, stops polling fallback, clears the inactivity and reconnect timers, clears the active session association, and resets reconnect counters. When a tab becomes visible again, the manager reconnects if the source is inactive and an active session association remains.

## Event Envelope

Gateway stream frames carry an SSE `id` and a `data` field whose JSON value is the complete persisted push envelope. The application event is nested under that envelope's `event` field. `gateway-sse-normalizer.js` unwraps this structure and yields `{ type, data }` objects for the event bus.

Infrastructure events (connection established, keepalive) are consumed as transport events and are not forwarded to feature components.

## Reconnection and Inactivity

The manager uses a 120-second inactivity timer (`SSEClientConfig.KEEPALIVE_TIMEOUT_MS`). The timer resets when the connection opens or any message arrives. When it expires, the manager closes the source, marks it inactive, and schedules a reconnect.

Scheduled reconnects use exponential backoff starting at one second and capped at 30 seconds, with up to 25 percent jitter and a one-second minimum. The manager allows up to ten reconnect attempts (`SSEClientConfig.MAX_RECONNECT_ATTEMPTS`).

## Polling Fallback

When the EventSource stream exhausts reconnect attempts, `gateway-sse-polling-fallback.js` polls stored push envelopes from `GET /api/v1/sse/events?since_id=<id>&limit=<n>` with credentials. Polling uses the same nested-envelope normalization as the live stream and dispatches decoded payloads through the existing `handleSSEEvent` path. A successful stream reconnect stops polling.

Default polling interval is five seconds with a batch limit of 100 events. A `401` response stops polling and emits a connection-failed event with reason `polling_unauthenticated`.

## Authentication and Deployment Requirements

A direct Gateway `EventSource` uses `{ withCredentials: true }`. The deployment must satisfy all of these conditions:

- `G8E_GATEWAY_URL` is set to the HTTPS Gateway origin reachable from the user's browser. The dashboard host fails closed at startup if it is missing.
- The exact dashboard origin is allowed by Gateway credentialed CORS configuration, including scheme, hostname, and port.
- The browser trusts the Gateway certificate and runs the dashboard in a WebAuthn secure context (HTTPS or the browser's localhost exception).
- The Gateway issues a browser session cookie accepted for the cross-origin request. JavaScript does not read the cookie or put a raw session identifier in the URL.
- The dashboard Content Security Policy permits connections to the configured Gateway origin. It does not permit browser WebSocket connections.

The dashboard container's enrolled `g8ed` workload identity is separate from the browser session and is not used for browser SSE requests.

## Tests

The Vitest suite in `test/unit/frontend/sse/sse-connection-manager.unit.test.js` covers Gateway stream URL construction, envelope normalization integration, credentialed `EventSource` construction, open and error state, session replacement, active-state reporting, disconnect cleanup, and inactivity timeout closure. Chat handler tests verify feature-specific interpretation of event payloads.

The adapter suite in `dashboard/g8e-adapter/` tests Gateway envelope normalization, durable event IDs, deduplication, stream sentinels, reconnect and visibility reconciliation, polling fallback, and public spectator SSE separately.

Deployment verification must exercise the complete configured path: a browser-trusted Gateway certificate, exact CORS origin, valid Gateway web-session cookie, `G8E_GATEWAY_URL`, the live `/api/v1/sse/stream` route, Gateway envelope parsing, reconnect behavior, and polling fallback when the stream is unavailable.

## Related

- [Gateway SSE Streaming](../architecture/sse.md)
- [Gateway Integration](gateway.md)
- [Dashboard Architecture](architecture.md)
- [Network Architecture](../architecture/network.md)
- [Ensemble SSE](../ensemble/sse.md)
- [Generator-Neutral Builder Guide](../guides/build_observe_frontend.md)
- [Authentication](auth.md)
- [Testing](tests.md)
