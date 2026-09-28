---
doc_id: dashboard-devs
title: Dashboard Development Guide
audience: developers working on the g8ed dashboard
status: current
last_updated: 2026-09-28
version: v2.2.3
owners:
  - dashboard/
  - dashboard/server.js
  - dashboard/services/infra/app-enrollment-service.js
related:
  - docs/dashboard/architecture.md
  - docs/dashboard/auth.md
  - docs/dashboard/gateway.md
  - docs/dashboard/sse.md
  - docs/dashboard/tests.md
  - docs/guides/unified_stack.md
  - docs/architecture/network.md
  - docs/devs/tests.md
when_to_read: Setting up the dashboard development environment, understanding the static host architecture, running local tests, building containers, and working with workload enrollment.
do_not_use_for:
  - Browser application architecture (docs/dashboard/architecture.md)
  - WebAuthn and identity flows (docs/dashboard/auth.md)
  - Gateway credentials and CORS requirements (docs/dashboard/gateway.md)
---

# Dashboard Development Guide

## Purpose

Defines how to set up the local development environment, run the g8ed dashboard static host, execute tests, and build containers. g8ed is a Node.js static host for a Gateway-direct browser SPA; it is not the Gateway API backend. The dashboard serves plain HTTP without terminating TLS, authenticating browser users, or proxying Gateway requests.

## Quick index

- [Purpose](#purpose)
- [Quick index](#quick-index)
- [Invariants](#invariants)
- [Owned surfaces](#owned-surfaces)
- [Procedures](#procedures)
- [Anti-patterns](#anti-patterns)
- [Links out](#links-out)

## Invariants

### Development environment (`INV-DASHBOARD-DEV`)

| ID | Rule |
| --- | --- |
| INV-DASHBOARD-DEV-01 | The dashboard requires Node.js 22 or newer as specified in `dashboard/package.json` and `dashboard/Dockerfile`. Older versions are not supported. |
| INV-DASHBOARD-DEV-02 | Local development uses `npm ci` to install dependencies; `npm install` is not run in production or CI environments. The `package-lock.json` is committed and canonical. |
| INV-DASHBOARD-DEV-03 | The Gateway must be bootstrapped and reachable at `G8E_GATEWAY_URL` (HTTPS) for browser requests and `G8E_GATEWAY_HTTP_URL` (plain HTTP) for enrollment bootstrap. Both are required for full functionality. |

### Server and static content (`INV-DASHBOARD-HOST`)

| ID | Rule |
| --- | --- |
| INV-DASHBOARD-HOST-01 | The Express server exports `createApp({ gatewayOrigin })` for test construction and `runStartupEnrollment({ enrollmentService, onFatalError })` for pre-startup enrollment. The executable entrypoint validates `G8E_GATEWAY_URL` and runs enrollment before listening. |
| INV-DASHBOARD-HOST-02 | Static files under `public/` default to one-year immutable caching; HTML, JavaScript, and CSS responses use `Cache-Control: no-cache`. The `/g8e-config.js` endpoint uses `no-cache, no-store, must-revalidate`. |
| INV-DASHBOARD-HOST-03 | Browser security headers include CSP `connect-src` restricted to the configured Gateway origin, `X-Frame-Options: DENY`, and `Permissions-Policy` disabling sensitive capabilities. |
| INV-DASHBOARD-HOST-04 | The SPA fallback serves `index.html` for unknown GET requests that accept HTML, rate-limited to 100 requests per minute to prevent DoS. |

### Browser application (`INV-DASHBOARD-SPA`)

| ID | Rule |
| --- | --- |
| INV-DASHBOARD-SPA-01 | The browser application uses native ES modules, checked-in CSS, static HTML templates, and checked-in vendor assets under `public/js/vendor/`. There is no bundler or transpiler for the first-party SPA. |
| INV-DASHBOARD-SPA-02 | Cross-component state uses `EventBus` initialized in `public/js/app.js`. Gateway HTTP paths are built from `public/js/constants/api-paths.js`. Browser data structures live under `public/js/models/`. |
| INV-DASHBOARD-SPA-03 | All browser API calls go through `window.serviceClient` to the Gateway at `window.G8E_GATEWAY_URL`. There is no client-side localhost fallback; the origin is injected from the server via `/g8e-config.js` and must be valid before the application bootstrap. |

### Workload identity and enrollment (`INV-DASHBOARD-ENROLL`)

| ID | Rule |
| --- | --- |
| INV-DASHBOARD-ENROLL-01 | Startup enrollment occurs before the Express server listens. On failure, the process exits with status 1. On success, the resolved `AppIdentity` is returned and the server begins listening. |
| INV-DASHBOARD-ENROLL-02 | The enrollment service reads and writes state under `G8E_RUNTIME_DIR`. Installed certificates are reused only when they parse, contain a URI subject alternative name, and have more than seven days of validity remaining. |
| INV-DASHBOARD-ENROLL-03 | Enrollment generates an ECDSA P-256 key and CSR, submits over the plain-HTTP bootstrap origin, waits for owner approval, proves possession of the private key, and installs returned material. The resolved workload identity is not used for outbound Gateway requests after startup. |

## Owned surfaces

| Claim | Path | Verify |
| --- | --- | --- |
| Dashboard static host | `dashboard/server.js` | Exports `createApp()` and `runStartupEnrollment()` |
| npm scripts and dependencies | `dashboard/package.json` | Node 22+, correct scripts, runtime deps |
| Workload enrollment service | `dashboard/services/infra/app-enrollment-service.js` | Implements load-then-enroll decision |
| Browser SPA source | `dashboard/public/js/` | Native ES modules, static assets |
| Test suite | `dashboard/test/` | Vitest configuration and fixtures |
| Docker image | `dashboard/Dockerfile` | Node 22 Alpine, non-root user, `/data` mount |
| Container entrypoint | `dashboard/entrypoint.sh` | Gateway health polling, Node startup |

## Procedures

### Local development setup

Install dependencies and start the development server from `dashboard/`:

```bash
npm ci
G8E_GATEWAY_URL=https://localhost:8443 \
G8E_GATEWAY_HTTP_URL=http://localhost:8080 \
G8E_RUNTIME_DIR=/tmp/g8ed-runtime \
npm run dev
```

The `npm run dev` command runs the server through nodemon for automatic restart on file changes. The environment variables inject the Gateway origin, HTTP bootstrap URL, and writable runtime directory. On first startup with an empty runtime directory, the process submits a workload enrollment request and exits if enrollment fails.

Approve enrollment through the Gateway console or with repository-root CLI commands:

```bash
./g8e auth enroll pending
./g8e auth enroll approve <request-id> --yes
```

After approval, restart the development server.

### Runtime environment variables

| Variable | Required | Default | Purpose |
| --- | --- | --- | --- |
| `G8E_GATEWAY_URL` | Yes | None | HTTPS Gateway origin injected into the browser through `/g8e-config.js`; used by CSP `connect-src` directive |
| `G8E_RUNTIME_DIR` | Yes | None | Writable root for installed workload identity and pending enrollment state |
| `G8E_GATEWAY_HTTP_URL` | For enrollment | None | Plain-HTTP Gateway bootstrap origin used when enrolling or renewing workload identity |
| `PORT` | No | `3000` | Express listen port |
| `GATEWAY_HEALTH_URL` | Container only | `http://g8eg:8080` in `entrypoint.sh` | Container entrypoint health check origin; Node process does not read this |
| `GATEWAY_HEALTH_PATH` | Container only | `/api/v1/health` in `entrypoint.sh` | Container entrypoint health check path; Node process does not read this |

### npm scripts and make targets

Available npm scripts in `dashboard/package.json`:

```bash
npm start                # Run node server.js
npm run dev              # Run node server.js through nodemon
npm run lint             # Run ESLint
npm test                 # Run Vitest once
npm run test:watch       # Run Vitest in watch mode
npm run test:ui          # Open Vitest UI
npm run test:coverage    # Produce V8 coverage under dashboard/coverage/
```

Repository-root make targets for dashboard:

```bash
make dashboard-lint              # ESLint in dashboard/
make dashboard-boundary-check    # Guard against dashboard-origin platform API patterns
make dashboard-test              # Full Vitest suite
make build-dashboard             # Build version-tagged Docker image
```

### Source layout

```text
dashboard/
├── server.js                          Static host entrypoint
├── entrypoint.sh                      Container Gateway health wait
├── services/infra/app-enrollment-service.js
│                                      Workload enrollment service
├── public/                            Browser SPA (HTML, CSS, JS, media)
│   └── js/
│       ├── app.js                     Application bootstrap
│       ├── components/                Feature components and templates
│       ├── constants/                 API paths, events, UI constants
│       ├── models/                    Browser data models
│       ├── utils/                     Service client, SSE, auth helpers
│       └── vendor/                    Checked-in browser vendor libraries
├── test/                              Vitest suite
├── eslint.config.js                   ESLint configuration
├── vitest.config.js                   Vitest configuration
└── g8e-adapter/                       Separate TypeScript adapter (own tooling)
```

The active server code is `server.js` plus `services/infra/`. Browser constants, models, and utilities live under `public/js/`, not at the dashboard root. The `g8e-adapter/` directory is a separate TypeScript frontend adapter with independent build and test tooling; the npm and make commands in this guide apply to the first-party dashboard host only.

### Container build and deployment

Build the Docker image from the repository root:

```bash
docker build -f dashboard/Dockerfile -t g8e-dashboard .
```

The image uses Node 22 Alpine 3.21, installs production dependencies via npm in the builder stage, prunes dev dependencies, runs as non-root user `g8e` (UID 1001), exposes port `3000`, and uses `/data` for the writable runtime volume.

The container entrypoint is `entrypoint.sh`, which polls the Gateway health endpoint up to 30 times at two-second intervals, then starts Node. A failed health check exits the container with status 1.

In the root `docker-compose.yml`, the dashboard service uses the `bootstrapped` profile, mounts `g8e-dashboard-data` at `/data`, sets `G8E_RUNTIME_DIR=/data`, and configures separate browser-facing and container-internal Gateway URLs. The browser accesses the HTTPS origin; the container uses the plain-HTTP origin for enrollment bootstrap.

### Static host architecture

The Express application created by `createApp({ gatewayOrigin })` performs these steps in order:

1. Sets browser security headers, including CSP `connect-src` restricted to the configured Gateway origin.
2. Logs requests with method, path, status code, and duration in milliseconds.
3. Serves `/g8e-config.js` with `no-cache, no-store, must-revalidate` headers; contains `window.G8E_GATEWAY_URL = <origin>`.
4. Serves static files from `public/` with one-year immutable caching for all asset types except HTML/JS/CSS.
5. Rate-limits HTML SPA fallback to 100 requests per minute.

The server does not mount API routes or proxy Gateway requests. The browser sends API, SSE, and passkey traffic directly to `G8E_GATEWAY_URL` with `credentials: 'include'`.

### Workload identity files

The enrollment service reads and writes state under `G8E_RUNTIME_DIR` with these paths:

| Relative path | Purpose | Mode | Read/Write |
| --- | --- | --- | --- |
| `pki/issued/apps/g8ed.crt` | Installed certificate chain | `0600` | Load at startup |
| `pki/issued/apps/g8ed.key` | Installed private key | `0600` | Load at startup |
| `pki/trust/hub-bundle.pem` | Gateway trust bundle | `0644` | Load at startup |
| `pki/pending-enrollment/dashboard.json` | Resumable enrollment state | `0600` | Read/write during enrollment |

On startup, the enrollment service attempts to load an installed identity. If none is available (missing, expired, or nearing expiry), it generates a P-256 key and CSR, submits an enrollment request over `G8E_GATEWAY_HTTP_URL`, and persists the private key with `0600` permissions. The service then polls status with bounded exponential backoff until approval or deadline. After approval, it proves possession of the private key, completes the enrollment, and installs the returned certificate and trust bundle.

### Browser application state and configuration

The browser application bootstrap in `public/js/app.js` creates shared instances of `EventBus`, `AuthManager`, `SSEConnectionManager`, and feature components. Cross-component state communication uses the `EventBus` and constants from `public/js/constants/events.js`.

Gateway HTTP paths are constructed from constants in `public/js/constants/api-paths.js`. Browser-side data models live under `public/js/models/`. Feature HTML fragments under `public/js/components/templates/` load as static assets via Express, not bundled or transpiled.

Gateway configuration is loaded before any application scripts from `/g8e-config.js`. The browser application checks `window.G8E_GATEWAY_URL` before initializing; there is no client-side localhost fallback or default value.

All browser API calls to the Gateway route through `window.serviceClient` to `ServiceName.GATEWAY` at `window.G8E_GATEWAY_URL`. The service client attaches mTLS credentials and the `credentials: 'include'` flag to all requests.

## Anti-patterns

- Attempting to use `npm install` in production or CI environments instead of `npm ci` with a committed `package-lock.json`.
- Starting the server without validating `G8E_GATEWAY_URL` or running the startup enrollment phase.
- Modifying CSP headers or removing the `connect-src` restriction to the configured Gateway origin.
- Hardcoding a localhost fallback in the browser application when `G8E_GATEWAY_URL` is not available.
- Hand-editing generated or bundled assets instead of editing source files and restarting the dev server.
- Disabling the SPA fallback rate limiter or increasing the limit beyond 100 requests per minute without threat modeling.
- Reusing an expired or near-expiry certificate without re-running enrollment.
- Deploying the container without mounting a persistent volume for `G8E_RUNTIME_DIR`.

## Links out

- [Dashboard Architecture](architecture.md): Runtime boundaries, browser-to-gateway communication, event flows.
- [Authentication](auth.md): WebAuthn, identity flows, enrollment callbacks.
- [Gateway Integration](gateway.md): CORS configuration, CSP requirements, browser certificate validation.
- [Server-Sent Events](sse.md): SSE stream lifecycle, browser connection handling.
- [Dashboard Testing](tests.md): Test fixtures, Vitest configuration, mocking strategy.
- [Unified Docker Stack](../guides/unified_stack.md): Multi-service compose configuration.
- [Network Architecture](../architecture/network.md): Threat model, communication patterns, trust boundaries.
- [Platform Testing](../devs/tests.md): Integration and end-to-end test coverage.
