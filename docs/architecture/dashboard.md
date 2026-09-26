---
title: Dashboard (g8ed)
parent: Architecture
---

# Dashboard (g8ed)

Last Updated: 2026-09-23
Version: v2.1.12

## Purpose

g8ed is the first-party browser interface for g8e. A Node.js 22 and Express 5 process serves a framework-free JavaScript single-page application. The browser and the dashboard container have separate identities and communicate with different Gateway surfaces.

The current g8ed runtime provides static application delivery, Gateway-direct session restoration, and logout. Its browser interface also contains passkey registration and sign-in controls plus modules for ensemble chat, Operator management, approvals, audit inspection, settings, and terminal activity, but the current sign-in flow does not provide the user identifier required by the Gateway, and the retained feature modules do not have an active API backend in the running dashboard server. Interactive registration and sign-in are therefore not operational in this interface.

The audited `g8e-adapter` package at `dashboard/g8e-adapter/` is a separate, generator-neutral integration core for observe frontends. It implements the Gateway browser contract (absolute configured origin, `/api/v1/sse/stream`, nested envelope parsing, `withCredentials: true`, endpoint allowlist) and its current Vitest suite passes 445 tests, including the contract-pack drift check. The adapter is not the transport used by the legacy g8ed SPA. It supplies the minimal host, reference frontend, and generated observe-frontends described in the [Generator-Neutral Builder Guide](../guides/build_observe_frontend.md). The evaluation explorer is another adapter consumer and is served by the Gateway's evaluation-explorer listener, not by the dashboard container. The legacy dashboard transport (`dashboard/server.js`, `dashboard/public/js/`, `dashboard/routes/`) remains separate because its authentication and SSE behavior do not match the Gateway browser contract.

See the [Dashboard documentation](../dashboard/index.md) for component-level details and development guidance.

## Runtime Boundaries

The dashboard consists of three security boundaries:

- **Static host:** Express serves the application, publishes the browser-reachable Gateway origin, applies browser security headers, and returns the single-page application for unknown HTML routes. It does not terminate TLS, authenticate users, store browser sessions, proxy WebSockets, or provide dashboard API handlers.
- **Browser application:** The browser authenticates directly to the Gateway over HTTPS with WebAuthn and sends credentialed requests using the Gateway-issued HttpOnly session cookie.
- **Container workload:** Before serving the application, the dashboard container obtains or validates an owner-approved `g8ed` workload certificate through the Gateway's plain-HTTP enrollment surface. This identity is independent of the browser session.

The running static host does not use the enrolled workload certificate for outbound Gateway requests. Enrollment is still a startup requirement: the dashboard does not begin listening until a usable identity exists, and enrollment failure stops startup.

## Startup and Deployment Flow

In the unified Docker deployment, the dashboard container starts after the Gateway health check succeeds. The dashboard still cannot listen until it has a usable workload identity; if the Gateway has not yet been bootstrapped, enrollment submission retries while owner bootstrap completes. Startup then proceeds as follows:

1. The entrypoint waits for the Gateway's plain-HTTP health surface, polling up to 30 times at two-second intervals.
2. The dashboard loads its installed workload identity or starts owner-approved enrollment.
3. The Gateway console presents a new enrollment request to an owner. The dashboard remains unavailable while approval is pending.
4. The dashboard stores the approved certificate, private key, trust bundle, and any resumable pending state in its persistent runtime volume.
5. Express starts on plain HTTP and publishes the configured browser-facing Gateway origin to the single-page application.

`G8E_GATEWAY_URL` identifies the HTTPS Gateway origin reachable from the user's browser. `G8E_GATEWAY_HTTP_URL` identifies the plain-HTTP enrollment origin reachable from the container. `GATEWAY_HEALTH_URL` and `GATEWAY_HEALTH_PATH` identify the container health probe, while `G8E_RUNTIME_DIR` identifies persistent dashboard state. `PORT` controls the static host port and defaults to `3000`.

The dashboard process does not provide HTTPS. Deployments that require HTTPS on the dashboard origin terminate it in an external proxy or load balancer. The browser must also trust the Gateway certificate, and the Gateway must permit the exact dashboard origin through credentialed CORS and matching WebAuthn relying-party configuration.

See [Unified Docker Stack](../guides/unified_stack.md) for the deployment procedure and [Connect Apps to Gateway](../guides/connect_apps_to_gateway.md) for workload enrollment.

## Browser Authentication

The browser's passkey registration and authentication code calls the Gateway directly and translates Gateway challenge data for the WebAuthn browser API. In the current g8ed interface, however, the sign-in request omits the user identifier required by the Gateway, so a new registration or sign-in does not complete. Existing valid sessions can still be restored, and logout remains Gateway-authoritative.

A successful ceremony would create an HttpOnly `g8e_web_session_cookie` at the Gateway origin. Dashboard JavaScript cannot read this cookie and does not synthesize bearer, session, cookie, or API-key headers for Gateway requests. The browser attaches the cookie to credentialed requests, and the Gateway remains the authority for session validity and user identity.

At startup, the browser requests the current user and public web-session identifier. Logout asks the Gateway to invalidate the session, disconnects event delivery, clears local state, and returns to the home route. See [Dashboard Authentication](../dashboard/auth.md) and [Authentication and Authorization](./auth.md).

## Capability Status

| Capability | Current runtime status |
| --- | --- |
| Static application and runtime Gateway configuration | Active. Express serves checked-in assets and the browser-facing Gateway origin. |
| Passkey session restoration and logout | Active. The browser calls the Gateway directly over HTTPS when an existing valid session cookie is present. |
| Passkey registration and sign-in | Active via Gateway-direct WebAuthn (`options.publicKey`, explicit `user_id` on authenticate challenge). |
| Container workload enrollment | Active and required before the static host listens. The resulting mTLS identity is not consumed by the running host after startup. |
| Server-Sent Events | Active via absolute `${G8E_GATEWAY_URL}/api/v1/sse/stream` with Gateway nested push envelope normalization. |
| Chat, cases, Operator management, approvals, settings, and terminal actions | Active via Gateway browser routes and the Gateway→g8ee ensemble proxy (`/api/v1/chat`, `/api/v1/settings`, `/api/v1/operator/*`). |
| Audit log UI | Browser calls Gateway audit paths; full audit aggregation may remain limited to mTLS operator surfaces. |
| Gateway mTLS WebSocket access | Not available to the browser. The static host does not proxy the Gateway's workload-only WebSocket surface. |

This status distinction prevents browser components present in the source tree from being mistaken for deployed platform capabilities. New browser integration uses the Gateway origin explicitly; the dashboard origin serves application assets and configuration only.

## Event Delivery

The g8ed browser client creates one credentialed `EventSource` against the absolute Gateway stream URL, normalizes the nested Gateway push envelope (shared logic with `g8e-adapter`), and distributes application payloads through an in-browser event bus. It monitors activity and retries failures with bounded backoff and jitter.

The separate `g8e-adapter` remains the audited reference for observe frontends (replay cursors, reconciliation, polling fallback). See [Dashboard Server-Sent Events](../dashboard/sse.md), [SSE Streaming](./sse.md), and the [Generator-Neutral Builder Guide](../guides/build_observe_frontend.md).

## Security Properties

- Browser sessions terminate at the Gateway. The dashboard host neither reads nor validates the HttpOnly session cookie.
- The container workload private key remains in the dashboard runtime volume and is never included in browser configuration or static assets.
- Content Security Policy restricts browser connections to the dashboard and configured Gateway origins and prevents framing and plugin content.
- The browser does not hold Operator private keys and cannot use workload-authenticated Gateway transports.
- The dashboard is a control surface, not a governance or execution authority. It does not directly mutate a managed host.

When a supported Gateway request produces a governed operation, authorization remains in the platform's five-layer interlock: L1 Doctrine validates intent and detects forbidden patterns, L2 Consensus verifies required Ed25519 votes, L3 Notary verifies required human authorization, L4 Warden checks integrity and replay controls before dispatch, and L5 Actuator executes through an isolated capability and produces a signed receipt. The active posture determines whether L2 and L3 are required. See [Governance](./governance.md).

## Build and Verification

The legacy dashboard browser assets have no compilation step; Node serves the checked-in files directly. From `dashboard/`, `npm start` runs the server, `npm run lint` runs ESLint, and `npm test` runs the Vitest suite. Repository-level verification uses `make dashboard-lint` and `make dashboard-test`, while `make build-dashboard` builds the dashboard container image. The separate adapter is verified from `dashboard/g8e-adapter/` with `npm test`, `npm run lint`, `npm run build`, `npm run build:host`, and `npm run gen:contract-pack:check`.

The current local runs for this repository version pass 2,145 legacy-dashboard tests and 445 adapter tests. The dashboard tests cover browser authentication, event handling and reconnection, static-host behavior, startup enrollment, UI components, models, and inactive server-side modules; they do not make inactive routes operational. See [Dashboard Development](../dashboard/devs.md), [Dashboard Tests](../dashboard/tests.md), and the [Generator-Neutral Builder Guide](../guides/build_observe_frontend.md).

## Related Documentation

- [Dashboard documentation](../dashboard/index.md): Component documentation and development guidance.
- [Authentication and Authorization](./auth.md): Gateway WebAuthn, browser sessions, and workload enrollment.
- [SSE Streaming](./sse.md): Gateway event publication and browser delivery surfaces.
- [Gateway Architecture](./gateway.md): Gateway protocol and trust boundaries.
- [Operator Architecture](./operator.md): Governed host execution and local verification.
- [Ensemble](./ensemble.md): The first-party ensemble that publishes browser-visible events.
- [Build a g8e-Compatible Frontend](../guides/build_frontend.md): Browser integration requirements.
- [Generator-Neutral Builder Guide](../guides/build_observe_frontend.md): The audited g8e-adapter and contract pack for generated observe frontends.
- [Public Spectator Architecture and Threat Model](./public_spectator.md): The separate anonymous public-mirror observation mode and outbound-only export architecture.
- [Unified Docker Stack](../guides/unified_stack.md): Full-stack deployment including g8ed.
