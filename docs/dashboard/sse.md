# Server-Sent Events

## Current Status

The dashboard contains a browser SSE client, but the current static-host deployment does not provide functional event delivery. The browser client requests the wrong Gateway endpoint from the wrong origin and expects a different event envelope from the one emitted by the Gateway stream. Authentication can restore an existing browser session, but that session does not make the current event connection operational.

The Node.js dashboard process serves static files only. It does not proxy Gateway requests or mount the retained dashboard-side SSE routes and services. See [Gateway Integration](gateway.md) for the complete runtime boundary.

## Intended Browser Architecture

The browser uses SSE for session-targeted platform activity because the Gateway WebSocket surface requires mTLS and is not available to browser clients. The dashboard creates one event connection after authentication restores a public web-session ID. Its event manager owns the browser `EventSource`, while the application event bus distributes decoded payloads to chat, operator, approval, status, and authentication components.

The public web-session ID identifies the connection in local browser state. It is not sent in the event URL and does not authenticate the request. The Gateway derives the user and web session from its HttpOnly session cookie.

## Connection Lifecycle

After startup authentication succeeds, the dashboard starts its event manager with the public web-session ID. The manager maintains one active source:

1. A request without a web-session ID stops without opening a connection.
2. An open connection for the requested session is reused.
3. A connection for another session is closed before the replacement is created.
4. The new `EventSource` requests credentials so the browser can include cookies for its target origin.
5. Opening the source marks it connected, resets failure counters, starts the inactivity timer, and publishes a local connection-opened event.
6. Each message resets the inactivity timer, is parsed as JSON, and is dispatched by its application event type.
7. A source error marks the connection inactive, publishes a local connection-error event, and schedules another attempt.

Logout closes the source, clears inactivity and reconnect timers, removes the active-session association, and resets reconnect counters. Returning to a visible browser tab starts a connection attempt only when the current source is inactive and an active session ID remains available.

## Expected Event Envelope

The dashboard event manager expects each SSE message to contain a top-level string `type`. Application events also contain a top-level `data` field, whose value is emitted through the application event bus. A `null` payload is accepted, but an absent `data` field is not.

Connection-establishment and keepalive event types are consumed as transport events and are not forwarded to feature components. Invalid JSON, a missing or non-string type, and application events without a `data` field are not dispatched. Chat handlers interpret the forwarded chat event payloads separately.

## Keepalive and Reconnection

The manager uses a 120-second inactivity timeout, reset when a connection opens or any message arrives. Expiry closes the source and schedules reconnection.

Reconnect attempts use exponential backoff from one second to 30 seconds with up to 25 percent jitter and a one-second minimum. After ten scheduled attempts, the manager publishes a terminal connection-failed event. Authentication treats that event as session expiry and clears local authenticated state, although a network, endpoint, or wire-contract failure does not prove that the Gateway session expired.

The manager also records rapid failures within five seconds for diagnostics. Its current rapid-failure counter does not change the calculated reconnect delay beyond the normal exponential backoff.

## Current URL Resolution Constraint

The current client creates a credentialed `EventSource` with the relative URL `/api/v1/sse/events`. Three independent incompatibilities prevent this from implementing the Gateway browser stream:

1. **Wrong origin:** A relative URL resolves against the dashboard origin. The static dashboard host does not proxy this path, and runtime configuration for `G8E_GATEWAY_URL` is not applied to `EventSource`.
2. **Wrong endpoint:** `/api/v1/sse/events` is the Gateway's finite JSON polling endpoint, not an SSE response. The live browser endpoint is `/api/v1/sse/stream`.
3. **Wrong envelope:** The dashboard parser expects `{ type, data }`. Gateway stream frames contain the complete persisted push envelope, with the application event under its `event` field. The application event may itself be an object or a JSON string.

Exposing `/api/v1/sse/events` at the dashboard origin fixes only origin routing and still does not produce an SSE stream. A functional direct integration constructs an absolute URL from `G8E_GATEWAY_URL`, uses `/api/v1/sse/stream`, and unwraps the Gateway push envelope before dispatch. The Gateway's replay cursor, heartbeat framing, polling fallback, and payload contract are documented in [Gateway SSE Streaming](../architecture/sse.md).

## Authentication and Deployment

A direct Gateway `EventSource` uses `{ withCredentials: true }`. The Gateway must allow the exact dashboard origin through CORS, and the browser must accept the Gateway's Secure, HttpOnly session cookie. JavaScript does not read the cookie or place a raw session ID in the URL.

The dashboard Content Security Policy permits connections to the configured Gateway origin. It does not permit browser WebSocket connections. Browser trust for the Gateway certificate and cross-site cookie policy still apply; see [Gateway Integration](gateway.md#deployment-requirements).

## Retained Dashboard SSE Code

The source tree retains a dashboard-local SSE route, internal push route, and in-memory delivery service. That design accepts internal events, keeps one local response per web session, sends keepalives every 20 seconds, and does not persist or replay missed events. The current static host does not instantiate these services or mount these routes, so they are not runtime event surfaces and must not be used as deployment endpoints.

## Tests

The browser SSE manager unit suite verifies event envelope dispatch, infrastructure-event suppression, credentialed `EventSource` construction, open and basic error state, session replacement, active-state reporting, disconnect counter cleanup, and inactivity timeout closure. It does not currently verify reconnect timing, jitter, attempt exhaustion, visibility handling, invalid JSON in the message callback, or end-to-end Gateway compatibility. Chat SSE handler tests cover feature-specific payload interpretation separately.

Dashboard unit tests do not mount the static host with the Gateway or prove browser event delivery. Deployment verification must exercise the configured Gateway origin, CORS credentials, session cookie, live stream path, and Gateway envelope parsing together.

## Related

- [Gateway SSE Streaming](../architecture/sse.md) - Gateway push ingestion, persistence, replay, framing, and consumer authentication
- [Network Architecture](../architecture/network.md) - Gateway protocol surfaces, ports, and network topology
- [Ensemble SSE](../ensemble/sse.md) - First-party event production pipeline
- [Authentication](auth.md)
- [Gateway Integration](gateway.md)
- [Testing](tests.md)
