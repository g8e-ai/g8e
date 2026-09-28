---
doc_id: dashboard-index
title: Dashboard Overview
audience: operators, developers, and security reviewers
status: current
last_updated: 2026-09-28
version: v2.2.3
owners:
  - dashboard/
  - dashboard/server.js
  - dashboard/g8e-adapter/
related:
  - docs/dashboard/architecture.md
  - docs/dashboard/auth.md
  - docs/dashboard/gateway.md
  - docs/dashboard/sse.md
  - docs/dashboard/operators.md
  - docs/dashboard/devs.md
  - docs/dashboard/tests.md
  - docs/architecture/dashboard.md
  - docs/architecture/gateway.md
when_to_read: Understanding g8ed roles and boundaries, planning deployments, auditing browser capabilities, or navigating dashboard documentation.
do_not_use_for:
  - Runtime and container architecture (docs/dashboard/architecture.md)
  - Security properties and enrollment detail (docs/dashboard/auth.md)
  - Gateway CORS and WebAuthn configuration (docs/dashboard/gateway.md)
  - Server-Sent Events lifecycle and reconnection (docs/dashboard/sse.md)
  - Browser application coding patterns (docs/dashboard/devs.md)
---

# Dashboard Overview

## Purpose

g8ed is the first-party browser interface for g8e. The dashboard serves a single-page application over plain HTTP without authenticating users or proxying Gateway traffic. The browser authenticates directly to the g8e Gateway over HTTPS. This document maps dashboard roles, browser-accessible surfaces, deployment configuration, and related documentation.

## Quick index

- [Purpose](#purpose)
- [Quick index](#quick-index)
- [Invariants](#invariants)
- [Owned surfaces](#owned-surfaces)
- [Procedures](#procedures)
- [Anti-patterns](#anti-patterns)
- [Links out](#links-out)

## Invariants

### Deployment and architecture (`INV-DASHBOARD-DEPLOY`)

| ID | Rule |
| --- | --- |
| INV-DASHBOARD-DEPLOY-01 | The dashboard is a static host plus Gateway-direct browser client, not a platform API proxy. The Express server serves browser assets and runtime configuration; all platform requests originate from browser code to the Gateway. |
| INV-DASHBOARD-DEPLOY-02 | The dashboard host does not terminate TLS, validate session cookies, or mount Gateway API routes. External reverse proxies or load balancers terminate TLS if the dashboard origin requires HTTPS. |
| INV-DASHBOARD-DEPLOY-03 | Container workload enrollment must complete before the Express server listens. Enrollment failure exits the process with status 1. The enrolled workload identity is not used for outbound Gateway requests after startup. |

### Browser capabilities (`INV-DASHBOARD-BROWSER`)

| ID | Rule |
| --- | --- |
| INV-DASHBOARD-BROWSER-01 | The browser application is restricted to a defined allowlist of Gateway endpoints (documented in [dashboard/g8e-adapter/src/api/allowlist.ts](dashboard/g8e-adapter/src/api/allowlist.ts)). The allowlist enforces a read-only surface: bootstrap status, passkey ceremonies, session restoration, observe data, and Server-Sent Events. |
| INV-DASHBOARD-BROWSER-02 | Browser capabilities are asymmetric: the browser CAN fetch observe data, passkeys, and SSE events; the browser CANNOT list/bind operators, grant approvals, execute terminal commands, or read audit logs directly. These require user action through chat/ensemble or CLI access. |
| INV-DASHBOARD-BROWSER-03 | All browser HTTP calls go through `window.serviceClient` with enforcement of the allowlist. Generic request helpers that bypass the allowlist are not exposed to application code. |

## Owned surfaces

| Surface | Path | Purpose |
| --- | --- | --- |
| Dashboard host | [dashboard/server.js](dashboard/server.js) | Serves browser SPA and injects Gateway origin via `/g8e-config.js` |
| Browser API allowlist | [dashboard/g8e-adapter/src/api/allowlist.ts](dashboard/g8e-adapter/src/api/allowlist.ts) | Enforces browser endpoint restrictions and naming |
| Browser SPA | [dashboard/public/](dashboard/public/) | Static HTML, CSS, and ES modules; no build step required |
| Workload enrollment | [dashboard/services/infra/app-enrollment-service.js](dashboard/services/infra/app-enrollment-service.js) | Manages container identity before server startup |
| Browser tests | [dashboard/test/](dashboard/test/) | Vitest configuration and boundary verification |

## Procedures

### Application Delivery

Express serves checked-in assets from [dashboard/public/](dashboard/public/) and injects the Gateway origin through `/g8e-config.js` (implemented in [dashboard/server.js](dashboard/server.js)). The browser loads this configuration before any other script, so the Gateway origin is available immediately for platform requests.

The dashboard host applies security headers:
- Content-Security-Policy restricts connections to the dashboard and configured Gateway origins
- X-Frame-Options and Referrer-Policy limit framing and referrer leakage
- Permissions-Policy disables sensitive browser capabilities (camera, geolocation, payment, etc.)

See [server.js](dashboard/server.js) and [architecture.md](architecture.md#dashboard-host).

### Browser Session and Authentication

Gateway validates the HttpOnly `g8e_web_session_cookie` issued during passkey sign-in or registration. The dashboard host never reads or validates the session cookie. Browser code uses `credentials: 'include'` so the cookie is sent automatically for all Gateway requests.

Passkey ceremonies (registration and authentication) accept `options.publicKey` and supply an explicit `user_id` on authenticate challenge, enabling account linking without prior enrollment.

See [auth.md](auth.md) for ceremony flows and session lifecycle.

### Gateway Origin Configuration

| Endpoint | Purpose | Example |
| --- | --- | --- |
| Browser-facing origin | `G8E_GATEWAY_URL` | `https://g8e.example.com:8443` |
| Container enrollment origin | `G8E_GATEWAY_HTTP_URL` | `http://gateway:8080` |
| Gateway health check | `GATEWAY_HEALTH_URL` | `http://gateway:8080/health` |

The container entrypoint waits for Gateway health before starting the dashboard. Both Gateway origins must be correct and reachable for full functionality. The browser must trust the Gateway's TLS certificate.

See [architecture.md#configuration](architecture.md#configuration).

### Browser Accessible Endpoints

The browser can access only endpoints listed in the allowlist ([dashboard/g8e-adapter/src/api/allowlist.ts](dashboard/g8e-adapter/src/api/allowlist.ts)). These include:

| Capability | Method | Path | Purpose |
| --- | --- | --- | --- |
| Bootstrap status | GET | `/api/v1/auth/bootstrap/status` | Check owner bootstrap completion |
| Passkey registration (challenge) | POST | `/api/v1/auth/passkeys/console/register/challenge` | Start passkey registration |
| Passkey registration (verify) | POST | `/api/v1/auth/passkeys/console/register/verify` | Complete passkey registration |
| Passkey authentication (challenge) | POST | `/api/v1/auth/passkeys/console/authenticate/challenge` | Start passkey sign-in |
| Passkey authentication (verify) | POST | `/api/v1/auth/passkeys/console/authenticate/verify` | Complete passkey sign-in |
| Passkey enrollment (challenge) | POST | `/api/v1/auth/passkeys/enrollment/register/challenge` | Start workload/device passkey |
| Passkey enrollment (verify) | POST | `/api/v1/auth/passkeys/enrollment/register/verify` | Complete workload/device passkey |
| Current user | GET | `/api/v1/users/me` | Fetch logged-in user profile |
| Session status | GET | `/api/v1/auth/sessions/me` | Check session validity and metadata |
| Logout | POST | `/api/v1/auth/logout` | Terminate session |
| Observe bootstrap | GET | `/api/v1/observe/bootstrap` | Fetch observable status |
| Observe runs | GET | `/api/v1/observe/runs` | List evaluation runs |
| Observe run detail | GET | `/api/v1/observe/runs/{id}` | Fetch run details |
| Observe evals | GET | `/api/v1/observe/evals` | List evaluations |
| Observe eval detail | GET | `/api/v1/observe/evals/{id}` | Fetch eval details |
| Observe downloads | GET | `/api/v1/observe/downloads` | List downloadable artifacts |
| Observe download detail | GET | `/api/v1/observe/downloads/{id}` | Fetch download metadata |
| Server-Sent Events | GET | `/api/v1/sse/stream` | Receive platform events (streaming) |
| Event polling fallback | GET | `/api/v1/sse/events` | Poll stored events after SSE reconnect exhaustion |
| Health check | GET | `/api/v1/health` | Verify Gateway connectivity |

### Container Workload Enrollment

Container workload enrollment occurs before the Express server listens. The dashboard loads an existing valid identity or starts a new enrollment request with the Gateway's plain-HTTP bootstrap surface.

Enrollment workflow:
1. Generate P-256 private key and CSR
2. Submit enrollment request to `G8E_GATEWAY_HTTP_URL` bootstrap surface
3. Wait for owner approval in Gateway console
4. Sign completion transcript with private key
5. Validate Gateway response and store certificate and trust bundle

The enrolled `g8ed` certificate is stored in `G8E_RUNTIME_DIR` (default: `/data`) and reused if valid (more than seven days remaining). Enrollment state is persistent; the dashboard remains unavailable while approval is pending.

See [architecture.md#startup-and-deployment](architecture.md#startup-and-deployment).

### Same-Machine Verification

Verify Gateway health, certificate trust, and CORS configuration:

```bash
./g8e gw connect http://localhost:3000
```

This command checks that the running dashboard is reachable, the Gateway certificate is trusted, and CORS headers permit the dashboard origin.

## Anti-patterns

- Claiming the dashboard proxies platform APIs. The dashboard serves only browser assets and configuration; all platform requests originate from browser code.
- Serving the dashboard over HTTPS directly from the container. Use an external reverse proxy or load balancer to terminate TLS so workload credential rotation remains independent of browser deployment.
- Bypassing the browser API allowlist to access additional Gateway endpoints. The allowlist enforces a minimal, auditable surface; changes require code review and documentation updates.
- Extending browser capabilities beyond the allowlist without corresponding documentation and boundary verification. See [dashboard/test/unit/architecture/test_g8ed_gateway_boundary.test.js](dashboard/test/unit/architecture/test_g8ed_gateway_boundary.test.js).
- Hard-editing workload enrollment state in `G8E_RUNTIME_DIR`. Use the standard `G8E_GATEWAY_HTTP_URL` and `G8E_GATEWAY_URL` configuration and let [AppEnrollmentService](dashboard/services/infra/app-enrollment-service.js) manage state.

## Links out

- [Architecture](architecture.md) — Runtime boundaries, browser composition, and request ownership
- [Authentication](auth.md) — Browser sessions, passkey ceremonies, and workload enrollment detail
- [Gateway Integration](gateway.md) — CORS configuration, WebAuthn relying-party setup, and troubleshooting
- [Server-Sent Events](sse.md) — Event stream lifecycle, envelope normalization, and polling fallback
- [Operator Surfaces](operators.md) — Operator deployment and terminal access
- [Development](devs.md) — Local setup, npm scripts, and JavaScript patterns
- [Testing](tests.md) — Test scope and verification commands
- [Platform Dashboard Architecture](../architecture/dashboard.md) — Platform-level design and trust boundaries
- [Gateway Architecture](../architecture/gateway.md) — Gateway request routing and authentication modes
- [Network Architecture](../architecture/network.md) — Container deployment and security boundaries
- [Unified Docker Stack](../guides/unified_stack.md) — Production deployment walkthrough
