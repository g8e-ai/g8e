# Development

Guide for working on the g8ed dashboard source tree, local commands, runtime configuration, and container build.

g8ed is a Node.js static host for a Gateway-direct browser SPA. It is not the Gateway API backend. See [Dashboard Architecture](architecture.md) for runtime boundaries.

## Prerequisites

- Node.js 22 or newer (`dashboard/package.json`)
- npm compatible with `dashboard/package-lock.json`
- A bootstrapped g8e Gateway reachable from the dashboard container and the browser
- An owner who can approve dashboard workload enrollment when no valid identity is installed
- A browser that supports WebAuthn for authentication testing

The Gateway must allow the exact dashboard origin through credentialed CORS and configure the same origin for WebAuthn. The browser must trust the Gateway certificate. See [Gateway Integration](gateway.md#deployment-requirements).

## Install and Run

From `dashboard/`:

```bash
npm ci
G8E_GATEWAY_URL=https://localhost:8443 \
G8E_GATEWAY_HTTP_URL=http://localhost:8080 \
G8E_RUNTIME_DIR=/tmp/g8ed-runtime \
npm run dev
```

| Variable | Required | Purpose |
| --- | --- | --- |
| `G8E_GATEWAY_URL` | Yes | HTTPS Gateway origin injected into the browser through `/g8e-config.js` |
| `G8E_RUNTIME_DIR` | Yes | Writable root for installed workload identity and pending enrollment state |
| `G8E_GATEWAY_HTTP_URL` | For enrollment | Plain-HTTP Gateway bootstrap origin used when enrolling or renewing the workload identity |
| `PORT` | No | Listen port; defaults to `3000` |

The host serves plain HTTP. It does not terminate TLS, authenticate browser users, or proxy Gateway requests. The browser sends API, SSE, and passkey traffic directly to `G8E_GATEWAY_URL` with `credentials: 'include'`.

On first startup with an empty runtime directory, the process submits a workload enrollment request and exits if enrollment fails. It does not listen until a valid identity is installed. See [Authentication](auth.md#container-startup-enrollment).

Approve enrollment through the Gateway console or with repository-root CLI commands:

```bash
./g8e auth enroll pending
./g8e auth enroll approve <request-id> --yes
```

### npm Scripts

| Command | Behavior |
| --- | --- |
| `npm start` | Run `node server.js` |
| `npm run dev` | Run `node server.js` through nodemon |
| `npm run lint` | Run ESLint |
| `npm test` | Run Vitest once |
| `npm run test:watch` | Run Vitest in watch mode |
| `npm run test:ui` | Open the Vitest UI |
| `npm run test:coverage` | Produce V8 coverage under `dashboard/coverage/` |

### Repository Make Targets

| Command | Behavior |
| --- | --- |
| `make dashboard-lint` | ESLint in `dashboard/` |
| `make dashboard-boundary-check` | Grep guard against dashboard-origin platform API patterns in runtime paths |
| `make dashboard-test` | Full Vitest suite |
| `make build-dashboard` | Build the version-tagged dashboard Docker image |

## Source Layout

```text
dashboard/
├── server.js                           # Static host entrypoint
├── entrypoint.sh                       # Container Gateway health wait
├── services/infra/app-enrollment-service.js
├── public/                             # Browser SPA (HTML, CSS, JS, media)
│   └── js/
│       ├── app.js                      # Application bootstrap
│       ├── components/                 # Feature components and templates
│       ├── constants/                  # API paths, events, UI constants
│       ├── models/                     # Browser data models
│       └── utils/                      # Service client, SSE, auth helpers
├── test/                               # Vitest suite
└── g8e-adapter/                        # Separate TypeScript adapter (own tooling)
```

The active server code is `server.js` plus `services/infra/`. Browser constants, models, and utilities live under `public/js/`, not at the dashboard root.

## No Frontend Build Step

The browser application uses native ES modules, checked-in CSS, static HTML templates, and checked-in vendor assets. There is no bundler or transpiler for the first-party SPA. Editing a file under `public/` changes what Express serves on the next request.

Static middleware defaults to one-year immutable caching, but HTML, JavaScript, and CSS responses use `Cache-Control: no-cache`. `/g8e-config.js` uses `no-cache, no-store, must-revalidate`.

`dashboard/g8e-adapter/` is a separate TypeScript frontend adapter with its own build and test tooling. The commands in this guide apply to the first-party dashboard host, not the adapter.

## Browser Application

`public/js/app.js` creates the shared `EventBus`, `AuthManager`, `SSEConnectionManager`, and feature components.

Cross-component state uses `EventBus` and constants from `public/js/constants/events.js`. Gateway HTTP paths are built from `public/js/constants/api-paths.js`. Browser data structures live under `public/js/models/`.

Feature HTML fragments under `public/js/components/templates/` load as static assets. Gateway configuration is loaded before application scripts from `/g8e-config.js`. There is no client-side localhost fallback for the Gateway origin.

All browser API calls go through `window.serviceClient` to `ServiceName.GATEWAY` at `window.G8E_GATEWAY_URL`.

## Static Host

`server.js` exports:

| Export | Purpose |
| --- | --- |
| `createApp({ gatewayOrigin })` | Construct the Express app for tests |
| `runStartupEnrollment({ enrollmentService, onFatalError })` | Resolve workload identity before listen |

The executable entrypoint validates `G8E_GATEWAY_URL`, runs startup enrollment, creates the app, and listens.

The Express application:

1. Sets browser security headers, including CSP `connect-src` for the configured Gateway origin
2. Logs requests
3. Serves `/g8e-config.js`
4. Serves static files from `public/`
5. Rate-limits the HTML SPA fallback to 100 requests per minute

It does not mount API routes or proxy Gateway traffic.

## Workload Identity Files

The enrollment service reads and writes state under `G8E_RUNTIME_DIR`:

| Relative path | Purpose | Mode |
| --- | --- | --- |
| `pki/issued/apps/g8ed.crt` | Installed dashboard certificate and chain | `0600` |
| `pki/issued/apps/g8ed.key` | Installed dashboard private key | `0600` |
| `pki/trust/hub-bundle.pem` | Gateway trust bundle from enrollment | `0644` |
| `pki/pending-enrollment/dashboard.json` | Resumable enrollment request state | `0600` |

Enrollment generates an ECDSA P-256 key and CSR, submits it over the plain-HTTP bootstrap origin, waits for owner approval, proves possession of the private key, and installs the returned material. Existing certificates are reused only when they parse, contain a URI subject alternative name, and have more than seven days of validity remaining. See [Authentication](auth.md#container-startup-enrollment).

The resolved workload identity is not used for outbound Gateway requests after startup.

## Docker

Build from the repository root:

```bash
docker build -f dashboard/Dockerfile -t g8e-dashboard .
```

The image uses Node 22 Alpine, installs production dependencies in the final stage, runs as non-root user `g8e` (UID 1001), exposes port `3000`, and uses `/data` for the writable runtime volume.

`entrypoint.sh` polls the Gateway health surface up to 30 times at two-second intervals, then starts Node. A failed health check exits the container.

In the root `docker-compose.yml`, the dashboard uses the `bootstrapped` profile, mounts `g8e-dashboard-data` at `/data`, sets `G8E_RUNTIME_DIR=/data`, and configures separate browser-facing and container-internal Gateway URLs. See [Unified Docker Stack](../guides/unified_stack.md).

## Environment Variables

| Variable | Default | Purpose |
| --- | --- | --- |
| `PORT` | `3000` | Express listen port |
| `G8E_GATEWAY_URL` | None | Required HTTPS Gateway origin for browser requests and CSP |
| `G8E_GATEWAY_HTTP_URL` | None | Plain-HTTP Gateway bootstrap origin for workload enrollment |
| `G8E_RUNTIME_DIR` | None | Required writable root for identity and pending enrollment state |
| `GATEWAY_HEALTH_URL` | `http://g8eg:8080` in `entrypoint.sh` | Gateway readiness origin for the container entrypoint |
| `GATEWAY_HEALTH_PATH` | `/api/v1/health` in `entrypoint.sh` | Gateway readiness path for the container entrypoint |

`GATEWAY_HEALTH_URL` and `GATEWAY_HEALTH_PATH` affect only the container entrypoint. The Node process does not read them. Set `G8E_GATEWAY_HTTP_URL` explicitly when enrollment is needed; it is not derived from `G8E_GATEWAY_URL`.

## Dependencies

Runtime dependencies:

| Package | Use |
| --- | --- |
| `express` | Static host |
| `express-rate-limit` | SPA fallback rate limiting |
| `@peculiar/x509` | CSR generation during enrollment |

Node built-ins provide certificate parsing (`X509Certificate`) and enrollment HTTP (`fetch`). Browser vendor libraries are checked into `public/js/vendor/`.

## Related

- [Dashboard Architecture](architecture.md)
- [Authentication](auth.md)
- [Gateway Integration](gateway.md)
- [Server-Sent Events](sse.md)
- [Testing](tests.md)
- [Unified Docker Stack](../guides/unified_stack.md)
- [Docker Gateway Guide](../guides/docker_gateway.md)
- [Platform Getting Started](../guides/getting_started.md)
- [Network Architecture](../architecture/network.md)
- [Platform Testing](../devs/tests.md)
