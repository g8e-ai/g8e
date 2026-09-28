---
doc_id: dashboard
title: Dashboard Architecture
audience: maintainers and coding agents
status: current
last_updated: 2026-09-28
version: v2.2.3
owners:
  - dashboard/
  - dashboard/g8e-adapter/
related:
  - gateway.md
  - auth.md
  - sse.md
  - network.md
  - operator.md
when_to_read: Understanding the first-party browser interface (g8ed), static host architecture, WebAuthn browser authentication, server-sent events delivery, workload enrollment, and security boundaries.
do_not_use_for:
  - Gateway protocol and routing (see gateway.md)
  - WebAuthn ceremonies and session management (see auth.md)
  - Event publication and stream architecture (see sse.md)
  - Network topology and PKI (see network.md)
---

# Dashboard Architecture

## Purpose

g8ed is the first-party browser interface for g8e. A Node.js 22 and Express 5 process serves a framework-free JavaScript single-page application. The browser and the dashboard container have separate identities and communicate with different Gateway surfaces.

The current g8ed runtime is a static host plus Gateway-direct browser client. Express serves application assets and injects the browser-facing Gateway origin. The browser authenticates to the Gateway with WebAuthn, sends credentialed API requests to Gateway browser routes and the Gateway→g8ee ensemble proxy, and connects SSE to `${G8E_GATEWAY_URL}/api/v1/sse/stream` with nested envelope normalization. The dashboard container enrolls its own `g8ed` workload identity at startup; the running static host does not use that credential for outbound requests.

The audited `g8e-adapter` package at [dashboard/g8e-adapter/](dashboard/g8e-adapter/) is a separate, generator-neutral integration core for observe frontends. It implements the Gateway browser contract (absolute configured origin, `/api/v1/sse/stream`, nested envelope parsing, `withCredentials: true`, endpoint allowlist) and its Vitest suite passes 445 tests, including the contract-pack drift check. The adapter is not the transport used by the first-party g8ed SPA. It supplies the minimal host, reference frontend, and generated observe-frontends described in the [Generator-Neutral Builder Guide](../guides/build_observe_frontend.md). The evaluation explorer is another adapter consumer and is served by the Gateway's evaluation-explorer listener, not by the dashboard container.

See [Dashboard documentation](../dashboard/index.md) for component-level details and development guidance.

## Quick index

- [Purpose](#purpose)
- [Invariants](#invariants)
- [Owned surfaces](#owned-surfaces)
- [Startup and Deployment Flow](#startup-and-deployment-flow)
- [Browser Authentication](#browser-authentication)
- [Capability Status](#capability-status)
- [Event Delivery](#event-delivery)
- [Security Properties](#security-properties)
- [Procedures](#procedures)
- [Build and Verification](#build-and-verification)
- [Anti-patterns](#anti-patterns)
- [Links out](#links-out)

## Invariants

| ID | Rule |
| --- | --- |
| INV-DASH-01 | The dashboard consists of three distinct security boundaries: static host (Express application server, no authentication), browser application (direct WebAuthn to Gateway, HttpOnly session cookie), and container workload (mTLS enrollment certificate). The running host does not use the workload certificate for outbound requests. |
| INV-DASH-02 | The browser authenticates directly to the Gateway via WebAuthn; the dashboard static host neither reads nor validates the resulting HttpOnly session cookie. Session authentication is terminated at the Gateway. |
| INV-DASH-03 | All browser requests use `credentials: 'include'` and flow to Gateway browser routes or the Gateway→g8ee ensemble proxy. The browser does not hold Operator private keys and cannot access workload-authenticated transports (mTLS WebSocket). |
| INV-DASH-04 | The dashboard static host serves plain HTTP on a configurable port (default 3000) and does not terminate TLS, proxy WebSockets, store browser sessions, or provide API handlers. Deployments requiring HTTPS terminate it in an external proxy or load balancer. |
| INV-DASH-05 | Content Security Policy restricts browser connections to the dashboard origin and configured Gateway origin, prevents framing (`frame-ancestors 'none'`), and blocks plugin content. |

## Runtime Boundaries

The dashboard consists of three security boundaries:

- **Static host:** Express serves the application, publishes the browser-reachable Gateway origin, applies browser security headers, and returns the single-page application for unknown HTML routes. It does not terminate TLS, authenticate users, store browser sessions, proxy WebSockets, or provide dashboard API handlers.
- **Browser application:** The browser authenticates directly to the Gateway over HTTPS with WebAuthn and sends credentialed requests using the Gateway-issued HttpOnly session cookie.
- **Container workload:** Before serving the application, the dashboard container obtains or validates an owner-approved `g8ed` workload certificate through the Gateway's plain-HTTP enrollment surface. This identity is independent of the browser session.

The running static host does not use the enrolled workload certificate for outbound Gateway requests. Enrollment is still a startup requirement: the dashboard does not begin listening until a usable identity exists, and enrollment failure stops startup.

## Owned surfaces

| Claim | Path | Verify |
| --- | --- |
| Static SPA host | [dashboard/server.js](dashboard/server.js) | `npm run lint`, `npm test`, `make dashboard-test` |
| Browser application entry point | [dashboard/public/index.html](dashboard/public/index.html) | `npm test` |
| Gateway browser contract | [dashboard/g8e-adapter/](dashboard/g8e-adapter/) | `npm test` in adapter; `make dashboard-boundary-check` |
| Application build | [dashboard/package.json](dashboard/package.json) | `npm ci`, `npm start`, `npm run lint` |
| Workload enrollment service | [dashboard/services/infra/app-enrollment-service.js](dashboard/services/infra/app-enrollment-service.js) | Tests in [dashboard/test/](dashboard/test/) |

## Startup and Deployment Flow

In the unified Docker deployment, the dashboard container starts after the Gateway health check succeeds. The dashboard still cannot listen until it has a usable workload identity; if the Gateway has not yet been bootstrapped, enrollment submission retries while owner bootstrap completes. Startup then proceeds as follows:

1. The entrypoint waits for the Gateway's plain-HTTP health surface, polling up to 30 times at two-second intervals.
2. The dashboard loads its installed workload identity or starts owner-approved enrollment.
3. The Gateway console presents a new enrollment request to an owner. The dashboard remains unavailable while approval is pending.
4. The dashboard stores the approved certificate, private key, trust bundle, and any resumable pending state in its persistent runtime volume.
5. Express starts on plain HTTP and publishes the configured browser-facing Gateway origin to the single-page application.

Environment variables control the dashboard startup:

- `G8E_GATEWAY_URL` (required): HTTPS origin of the Gateway reachable from the browser (e.g., `https://host:8443`). No fallback; startup fails if unset.
- `PORT` (optional): Express listen port for the static host. Defaults to `3000`.
- `G8E_RUNTIME_DIR` (via AppEnrollmentService): Directory for persisting workload certificate, private key, and trust bundle. Defaults to `.g8e/runtime/` relative to current working directory.

The dashboard process does not provide HTTPS. Deployments that require HTTPS on the dashboard origin terminate it in an external proxy or load balancer. The browser must also trust the Gateway certificate, and the Gateway must permit the exact dashboard origin through credentialed CORS and matching WebAuthn relying-party configuration.

See [Unified Docker Stack](../guides/unified_stack.md) for the deployment procedure and [Connect Apps to Gateway](../guides/connect_apps_to_gateway.md) for workload enrollment.

## Browser Authentication

The browser's passkey registration and authentication ceremonies call the Gateway directly, translate Gateway challenge data for the WebAuthn browser API using `options.publicKey`, and supply an explicit `user_id` on authenticate challenge. Returning users may persist `user_id` in `localStorage` under `g8e_user_id`. A successful ceremony creates an HttpOnly `g8e_web_session_cookie` at the Gateway origin. Dashboard JavaScript cannot read this cookie and does not synthesize bearer, session, cookie, or API-key headers for Gateway requests.

At startup, the browser requests the current user and public web-session identifier. Logout asks the Gateway to invalidate the session, disconnects event delivery, clears local state, and returns to the home route. See [Dashboard Authentication](../dashboard/auth.md) and [Authentication and Authorization](./auth.md).

## Capability Status

| Capability | Current runtime status |
| --- | --- |
| Static application and runtime Gateway configuration | Active. Express serves checked-in assets and the browser-facing Gateway origin. |
| Passkey session restoration and logout | Active. The browser calls the Gateway directly over HTTPS when an existing valid session cookie is present. |
| Passkey registration and sign-in | Active via Gateway-direct WebAuthn (`options.publicKey`, explicit `user_id` on authenticate challenge). |
| Container workload enrollment | Active and required before the static host listens. The resulting mTLS identity is not consumed by the running host after startup. |
| Server-Sent Events | Active via absolute `${G8E_GATEWAY_URL}/api/v1/sse/stream` with Gateway nested push envelope normalization. Polling fallback is not yet implemented in the first-party SPA. |
| Chat, cases, Operator management, approvals, settings, and terminal actions | Active via Gateway browser routes and the Gateway→g8ee ensemble proxy (`/api/v1/chat`, `/api/v1/settings`, `/api/v1/operator/*`, etc.). |
| Audit log UI | Browser calls Gateway `/api/v1/audit/*` paths, but those routes currently default to mTLS-only in the Gateway auth registry. Audit REST from the browser may fail until Gateway route reclassification or a UI downgrade. |
| Device links and Operator API keys from browser | Stub-rejected in `operator-panel-service.js`; UI controls may still be visible. |
| Gateway mTLS WebSocket access | Not available to the browser. The static host does not proxy the Gateway's workload-only WebSocket surface. |

This status distinction prevents browser components present in the source tree from being mistaken for deployed platform capabilities without checking Gateway route ownership.

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

## Procedures

### Run the Dashboard Server

```bash
cd dashboard
npm ci           # Install dependencies
npm start        # Start on http://localhost:3000
# In another terminal:
export G8E_GATEWAY_URL=https://host:8443
npm run dev      # Run with nodemon for development
```

### Verify and Test

```bash
# From dashboard/ directory:
npm run lint                # Run ESLint
npm test                    # Run Vitest suite (2,107 tests)
npm run test:watch         # Watch mode
npm run test:coverage      # Coverage report

# From repository root:
make dashboard-lint        # ESLint verification
make dashboard-test        # Full Vitest suite
make dashboard-boundary-check  # Gateway boundary invariants
make build-dashboard       # Build container image
```

### Verify the Adapter Contract

```bash
cd dashboard/g8e-adapter
npm ci
npm test           # Contract pack tests (445 tests)
npm run lint
npm run build      # TypeScript build
npm run gen:contract-pack:check  # Contract pack drift detection
```

## Build and Verification

The dashboard browser assets have no compilation step; Node serves the checked-in files directly. From `dashboard/`, `npm start` runs the server, `npm run lint` runs ESLint, and `npm test` runs the Vitest suite. Repository-level verification uses `make dashboard-lint`, `make dashboard-boundary-check`, and `make dashboard-test`, while `make build-dashboard` builds the dashboard container image. The separate adapter is verified from `dashboard/g8e-adapter/` with `npm test`, `npm run lint`, `npm run build`, and `npm run gen:contract-pack:check`.

The current test suite (v2.2.3) passes 2,107 dashboard tests (60 files) and 445 adapter tests (20 files). The dashboard tests cover browser authentication, event handling and reconnection, static-host behavior, startup enrollment, UI components, models, and architecture boundary guards. See [Dashboard Development](../dashboard/devs.md), [Dashboard Tests](../dashboard/tests.md), and the [Generator-Neutral Builder Guide](../guides/build_observe_frontend.md).

## Anti-patterns

- Storing session or authentication state in the dashboard host. Sessions terminate at the Gateway; the host neither reads nor validates the HttpOnly cookie.
- Adding HTTPS termination to the dashboard itself. Use an external proxy or load balancer; the host serves plain HTTP only.
- Bypassing the Gateway to access Operator mTLS WebSocket or core APIs. The browser client cannot hold workload credentials and cannot use workload-authenticated transports.
- Hard-coding the Gateway origin in the browser SPA. Always inject it at server startup via the `/g8e-config.js` endpoint and `window.G8E_GATEWAY_URL`.
- Duplicating Gateway contract logic. Use the audited `g8e-adapter` package and contract pack for generated observe frontends rather than re-implementing SSE, envelope parsing, or request signing in the browser.
- Modifying CSP headers to allow external script sources, WebSocket, or non-gateway origins. CSP whitelist is strict by design and enforced via [server.js](dashboard/server.js).
- Assuming the workload enrollment certificate is used for outbound requests from the running host. The certificate is required for startup but is not consumed after the server listens.

## Links out

- [Dashboard documentation](../dashboard/index.md): Component documentation and development guidance.
- [Authentication and Authorization](./auth.md): Gateway WebAuthn, browser sessions, and workload enrollment.
- [SSE Streaming](./sse.md): Gateway event publication and browser delivery surfaces.
- [Gateway Architecture](./gateway.md): Gateway protocol and trust boundaries.
- [Operator Architecture](./operator.md): Governed host execution and local verification.
- [Ensemble](./ensemble.md): The first-party ensemble that publishes browser-visible events.
- [Generator-Neutral Builder Guide](../guides/build_observe_frontend.md): The audited g8e-adapter and contract pack for generated observe frontends.
- [Public Spectator Architecture and Threat Model](./public_spectator.md): The separate anonymous public-mirror observation mode and outbound-only export architecture.
- [Unified Docker Stack](../guides/unified_stack.md): Full-stack deployment including g8ed.
- [Connect Apps to Gateway](../guides/connect_apps_to_gateway.md): Workload enrollment and authentication integration.
