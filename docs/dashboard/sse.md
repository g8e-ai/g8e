# Server-Sent Events

## Browser Event Architecture

g8ed uses Server-Sent Events for browser-visible platform activity. `SSEConnectionManager` owns the browser `EventSource`; `EventBus` distributes decoded application events to authentication, chat, operator, approval, and status components.

The browser does not connect to the gateway's mTLS WebSocket pub/sub endpoint (see [SSE Streaming](../architecture/sse.md) for the gateway-side event surfaces). Web browsers cannot present the dashboard container's app certificate, and the static host does not proxy WebSockets.

## Connection Lifecycle

`G8eDashboardApp` creates the SSE manager during application initialization. After `AuthManager` validates an authenticated session and exposes a public web-session ID, the application calls `initializeConnection(webSessionId)`.

The manager maintains one active connection:

1. Reject connection attempts without a web-session ID.
2. Reuse an active connection already associated with the requested session.
3. Close and detach handlers from any previous `EventSource`.
4. Create a credentialed `EventSource`.
5. Record connection timing and emit `PLATFORM_SSE_CONNECTION_OPENED` after `onopen`.
6. Parse each message as JSON and dispatch its typed payload.
7. On an error, mark the connection inactive, clear keepalive state, emit a connection error, and schedule reconnection.

`disconnect()` closes the source, clears keepalive and reconnect timers, removes active-session state, and resets counters. Logout calls it explicitly.

## Event Envelope Dispatch

Incoming messages are expected to contain a string `type` and, for application events, a `data` payload. The manager handles infrastructure events such as connection establishment and keepalive internally. It emits other event types through `EventBus` with the `data` object as the payload.

Messages with invalid JSON, a missing or non-string type, or a missing payload for a non-infrastructure event are not forwarded. Feature handlers subscribe using names from `public/js/constants/events.js`; chat-specific translation lives in `components/chat-sse-handlers.js`.

## Keepalive and Reconnection

The manager resets a keepalive timeout whenever it opens a connection or receives a message. A timeout closes the source and enters reconnect scheduling.

Reconnect delay uses capped exponential backoff with ±25 percent jitter and a configured minimum delay. The manager tracks total attempts and consecutive failures, detects rapid failures, avoids duplicate reconnect timers, and emits `PLATFORM_SSE_CONNECTION_FAILED` after the configured maximum. `AuthManager` subscribes to that terminal event and clears an authenticated UI session as expired.

Changing browser visibility does not close a healthy connection. Returning to a visible tab triggers a connection attempt only when the existing source is inactive and an active session ID remains available.

## Current URL Resolution Constraint

`SSEConnectionManager` currently constructs `EventSource` from `ApiPaths.sse.events()`, which returns the relative path `/api/v1/sse/events`. Unlike `ServiceClient`, `EventSource` does not receive `ServiceName.GATEWAY` and does not prepend `window.G8E_GATEWAY_URL`.

Because the current Express host does not proxy `/api/v1/sse/events`, the path resolves against the dashboard origin. A deployment must expose the gateway SSE path at that origin, or the client must complete gateway-origin URL construction, before browser-direct SSE works across separate dashboard and gateway origins. The broader gateway event contract remains documented in [SSE Streaming](../architecture/sse.md).

## Authentication

The `EventSource` is created with `{ withCredentials: true }`. When its URL resolves to the gateway origin and gateway CORS is configured for the dashboard origin, the browser includes the gateway's HttpOnly web-session cookie. The URL does not carry a raw session ID.

The public web-session ID retained by the manager scopes local connection state; it does not replace gateway cookie authentication.

## Tests

`test/unit/frontend/sse/sse-connection-manager.unit.test.js` covers connection creation, credential options, open and message handling, malformed events, keepalive, reconnection backoff, visibility behavior, session switching, failure limits, and disconnect cleanup. Chat SSE handler tests cover feature-specific event interpretation separately.

## Related

- [Gateway SSE Streaming](../architecture/sse.md) — Gateway-side SSE push ingestion, filtering, and consumer endpoints
- [Network Architecture](../architecture/network.md) — Gateway protocol surfaces, ports, and network topology
- [Ensemble SSE](../ensemble/sse.md) — Ensemble event production pipeline that feeds dashboard SSE consumers
- [Authentication](auth.md)
- [Gateway Integration](gateway.md)
- [Testing](tests.md)
