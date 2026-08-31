---
title: Dashboard (g8ed)
parent: Architecture
---

# Dashboard (g8ed)

Last Updated: 2026-08-31
Version: v2.1.2

## What g8ed Is

g8ed is the first-party browser interface for the g8e platform. It is a vanilla JavaScript single-page application served by a minimal Node.js 22 / Express 5 host from `dashboard/`. The dashboard provides browser components for passkey authentication, ensemble chat, operator management, governance approvals, audit inspection, settings, and live platform events.

The current runtime has two distinct identity surfaces:

- The browser authenticates directly to the gateway over HTTPS with WebAuthn passkeys and an HttpOnly web-session cookie.
- The dashboard container enrolls as the `g8ed` app and stores an mTLS workload identity for server-to-server gateway clients. The static host does not currently construct those clients, so this identity is forward-compatible infrastructure rather than the browser's authentication mechanism.

See the [Dashboard documentation](../dashboard/index.md) for the detailed component architecture, authentication flows, gateway contract, SSE behavior, operator surfaces, development workflow, and tests.

## Role in the Platform

g8ed sits in the human-facing application tier. It is a control surface, not a governance or execution authority:

- It starts and verifies WebAuthn registration and authentication ceremonies against the gateway.
- It sends authenticated browser requests with `credentials: 'include'`, allowing the browser to attach the gateway-issued HttpOnly session cookie.
- It consumes gateway SSE events and dispatches typed event payloads to UI components through an in-browser event bus.
- It renders chat, operator, approval, audit, settings, and terminal experiences from framework-free JavaScript components and HTML templates.
- It never mutates a target host directly. Host actions remain subject to the gateway's governance pipeline and the bound operator's local verification.

The dashboard does not construct `GovernanceEnvelope` transactions, hold operator private keys in the browser, or proxy the gateway's mTLS WebSocket surface. Browser-accessible operations use the gateway's HTTPS and SSE surfaces.

## Runtime Boundary

`dashboard/server.js` is a static SPA host. At startup it requires the browser-facing `G8E_GATEWAY_URL`, resolves or enrolls the container's app identity, creates the Express application, and begins listening only after enrollment succeeds. The Express application:

- Publishes `G8E_GATEWAY_URL` as `window.G8E_GATEWAY_URL` from `/g8e-config.js` without a localhost fallback.
- Sets Content Security Policy and browser security headers, including a `connect-src` restricted to the configured gateway origin.
- Serves `dashboard/public/` and returns `index.html` for unknown HTML routes so client-side navigation can resolve.
- Rate-limits the SPA fallback.
- Does not mount the route modules under `dashboard/routes/`, create a gateway HTTP client, terminate TLS, authenticate browser sessions, or proxy WebSockets.

The frontend `ServiceClient` resolves `ServiceName.GATEWAY` to `window.G8E_GATEWAY_URL` and resolves `ServiceName.g8ed` to the dashboard origin. Gateway authentication, user, session, and passkey requests use the gateway origin. Some retained operator, chat, audit, settings, and approval modules still target the legacy g8ed origin; because the static host mounts no API routers, those modules are migration inventory rather than an active server API. Detailed documentation distinguishes the active browser-direct surface from this retained legacy surface.

## Browser Authentication

The browser requests registration or authentication challenges from the gateway, converts base64url WebAuthn fields to browser credential buffers, invokes `navigator.credentials.create()` or `navigator.credentials.get()`, serializes the credential response, and sends it to the corresponding gateway verification endpoint. A successful verification establishes an HttpOnly `g8e_web_session_cookie` at the gateway origin.

JavaScript cannot read the cookie. Every gateway request uses `credentials: 'include'`; the dashboard does not synthesize browser authentication headers. Session startup validates the current user and obtains the public web-session identifier from the gateway. Logout calls the gateway, disconnects SSE, clears local UI state, and navigates home.

See [Dashboard Authentication](../dashboard/auth.md) and [Authentication & Authorization](./auth.md).

## Container Workload Enrollment

Before Express listens, `AppEnrollmentService` tries to load an installed app identity. A missing, expired, near-expiry, or otherwise unusable identity enters the owner-approved platform enrollment protocol over the gateway's plain-HTTP bootstrap surface. g8ed generates an ECDSA P-256 key and CSR, persists resumable pending state, waits for approval, signs the completion transcript, validates the response, and writes the installed identity under the dashboard runtime directory.

In the unified Docker stack, `G8E_RUNTIME_DIR=/data` and the `g8e-dashboard-data` volume persists:

- `pki/issued/apps/g8ed.crt`
- `pki/issued/apps/g8ed.key`
- `pki/trust/hub-bundle.pem`
- `pki/pending-enrollment/g8ed.json` while enrollment is pending

Enrollment fails closed: an unexpected identity-load failure or failed enrollment prevents the server from listening and exits the container non-zero. The browser never receives or uses this private key. See [Dashboard Authentication](../dashboard/auth.md) and [Connect Apps to Gateway](../guides/connect_apps_to_gateway.md).

## Event Flow

After browser authentication, `G8eDashboardApp` initializes `SSEConnectionManager`. The manager owns one `EventSource`, reconnects with capped exponential backoff and jitter, resets its keepalive timeout on activity, and emits application events through `EventBus`. Infrastructure events are consumed by the manager; typed application events are forwarded to chat, approval, operator, and status components.

The current `EventSource` path is built from the relative gateway SSE path. The static host does not proxy that path, so a deployment must expose the SSE endpoint at the resolved browser origin or complete the pending gateway-origin migration for this client. See [Dashboard SSE](../dashboard/sse.md) and [SSE Streaming](./sse.md).

## Build and Test

The dashboard has no frontend compilation step. Node serves the checked-in assets in `dashboard/public/` directly. The component provides:

- `npm start` — runs `node server.js`.
- `npm run dev` — runs the server through nodemon.
- `npm test` — runs the Vitest suite once.
- `npm run test:coverage` — runs Vitest with V8 coverage over `public/js/**/*.js` and `services/**/*.js`.
- `make dashboard-test` — runs the repository-level dashboard test target.
- `make build-dashboard` — builds the dashboard image from the repository root.

Tests live under `dashboard/test/` and cover browser components, models, rendering, SSE reconnection, the static server, app enrollment, and prepared mTLS client wiring. See [Dashboard Development](../dashboard/development.md) and [Dashboard Tests](../dashboard/tests.md).

## Related Documentation

- [Dashboard documentation](../dashboard/index.md): Detailed g8ed component documentation.
- [Authentication & Authorization](./auth.md): Gateway WebAuthn, web sessions, and workload enrollment.
- [SSE Streaming](./sse.md): Gateway event publication, storage, polling, and streaming.
- [Ensemble (g8ee)](./ensemble.md): The first-party ensemble whose events the dashboard renders.
- [Build a g8e-Compatible Frontend](../guides/build_frontend.md): Public browser integration contract.
- [Unified Docker Stack](../guides/unified_stack.md): Full-stack deployment including g8ed.
