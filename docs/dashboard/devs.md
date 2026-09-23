# Development

This guide describes the current g8ed source tree, local commands, runtime configuration, and container build. g8ed is a Node.js static host for a browser application; it is not the Gateway API backend. The current runtime and security boundaries are documented in [Dashboard Architecture](architecture.md).

## Prerequisites

- Node.js 22 or newer, as required by `dashboard/package.json`
- npm compatible with the repository's `package-lock.json` (the container installs npm 11.12.1)
- A bootstrapped g8e Gateway reachable from the dashboard process and the browser
- An owner who can approve a dashboard workload enrollment request when the runtime has no valid identity
- A browser that supports WebAuthn for browser-authentication testing

The Gateway must allow the exact dashboard origin through credentialed CORS and configure the same origin for WebAuthn. The browser must trust the Gateway certificate. See [Gateway Integration](gateway.md#deployment-requirements) for the complete deployment contract.

## Install and Run

From `dashboard/`:

```bash
npm ci
G8E_GATEWAY_URL=https://localhost:8443 \
G8E_GATEWAY_HTTP_URL=http://localhost:8080 \
G8E_RUNTIME_DIR=/tmp/g8ed-runtime \
npm run dev
```

`G8E_GATEWAY_URL` is required on every executable startup and is the HTTPS Gateway origin injected into the browser through `/g8e-config.js`. `G8E_RUNTIME_DIR` is required while the startup enrollment service loads or stores the dashboard identity. `G8E_GATEWAY_HTTP_URL` is required when startup must enroll or renew the identity; an already valid installed identity can be loaded without contacting the Gateway. A fresh runtime directory submits an enrollment request and keeps the dashboard from listening until an owner approves it through the Gateway console or the repository-root enrollment commands described in [Dashboard Authentication](auth.md#container-startup-enrollment).

The host listens on `PORT`, which defaults to `3000`, and serves the application over plain HTTP. It does not terminate TLS, authenticate browser users, proxy Gateway requests, or mount the retained server-side API and event routes. The browser sends supported Gateway requests directly to `G8E_GATEWAY_URL` with credentials included. Existing Gateway sessions can be restored and logged out; the current dashboard sign-in flow and feature API paths are not operational in the standard separate-origin deployment. Do not treat retained feature modules or their unit tests as proof that those server routes are live.

Available scripts are:

| Command | Behavior |
| --- | --- |
| `npm start` | Run `node server.js` |
| `npm run dev` | Run `node server.js` through nodemon file watching |
| `npm run lint` | Run ESLint across the dashboard source |
| `npm test` | Run Vitest once |
| `npm run test:watch` | Run Vitest in watch mode |
| `npm run test:ui` | Open the Vitest UI |
| `npm run test:coverage` | Produce text, HTML, and LCOV V8 coverage under `dashboard/coverage/` |

From the repository root, `make dashboard-lint` runs ESLint, `make dashboard-test` runs the dashboard suite, and `make build-dashboard` builds the version-tagged dashboard image.

## No Frontend Build Step

The browser application uses native ES modules, checked-in CSS, static HTML templates, and checked-in vendor assets. There is no bundler, transpiler, generated asset manifest, or framework compiler for the legacy dashboard application. Editing a file under `public/` changes what Express serves on the next request. The static middleware has a one-year immutable default cache lifetime, but HTML, JavaScript, and CSS responses override that default with `Cache-Control: no-cache`; `/g8e-config.js` is served with `no-cache, no-store, must-revalidate`.

The `dashboard/g8e-adapter/` directory is a separate TypeScript frontend adapter with its own build and test tooling. The commands in this guide apply to the legacy dashboard host, not to that adapter. Use the adapter's package metadata and documentation for adapter work.

## Browser Composition Model

`public/js/app.js` creates the shared `EventBus`, authentication manager, SSE connection manager, and active feature components. Cross-component state changes use `EventBus` and constants from `public/js/constants/events.js`. Browser HTTP paths use builders from `public/js/constants/api-paths.js`, and browser data structures are represented by model classes under `public/js/models/`.

Feature HTML fragments under `public/js/components/templates/` are loaded as static assets. The live runtime does not render the EJS files under `views/`. Browser-facing Gateway configuration is loaded before the application scripts from `/g8e-config.js`; there is no client-side localhost fallback.

## Server Composition Model

`server.js` exports `createApp()` for static-host tests and `runStartupEnrollment()` for enrollment-path tests. The executable entrypoint validates `G8E_GATEWAY_URL`, resolves the workload identity through `AppEnrollmentService`, creates the Express application, and then listens. Separating app construction from process startup lets tests instantiate Express without binding a port or running enrollment.

The active Express application applies browser security headers, logs requests, serves `/g8e-config.js` and static files, and rate-limits the HTML SPA fallback to 100 requests per minute per limiter key. It does not import or mount the retained modules under `routes/`, most of `services/`, `middleware/`, server-side `models/`, or `views/`. `services/infra/app-enrollment-service.js` is active during startup; the resolved workload identity is currently not used for outbound requests after startup.

## Runtime Identity and Files

The startup enrollment service reads and writes identity state below `G8E_RUNTIME_DIR`:

| Relative path | Purpose | Permission |
| --- | --- | --- |
| `pki/issued/apps/g8ed.crt` | Installed dashboard certificate and chain | `0600` |
| `pki/issued/apps/g8ed.key` | Installed dashboard private key | `0600` |
| `pki/trust/hub-bundle.pem` | Gateway trust bundle returned by enrollment | `0644` |
| `pki/pending-enrollment/dashboard.json` | Resumable enrollment request state | `0600` |

Enrollment generates an ECDSA P-256 key and CSR, submits it over the configured plain-HTTP bootstrap origin, waits for owner approval, proves possession of the private key, and installs the returned material. Existing certificates are reused only when they parse, contain a URI subject alternative name, and have more than seven days of validity remaining. See [Authentication](auth.md#container-startup-enrollment) for the validation limits and full lifecycle.

## Docker

The repository-root build context is required because the Dockerfile copies `dashboard/` from the monorepo root:

```bash
docker build -f dashboard/Dockerfile -t g8e-dashboard .
```

The image uses Node 22 Alpine, installs dependencies in a builder stage, prunes development dependencies for the final stage, runs as the non-root `g8e` user with UID 1001, exposes port `3000`, and uses `/data` for the writable runtime volume. `entrypoint.sh` makes up to 30 Gateway health-check attempts, waiting two seconds between failed attempts, before starting Node. A failed health check exits the container.

In the root `docker-compose.yml`, the dashboard uses the `bootstrapped` profile, mounts `g8e-dashboard-data` at `/data`, sets `G8E_RUNTIME_DIR=/data`, uses the external browser-facing hostname and HTTPS port for `G8E_GATEWAY_URL`, and uses the internal `g8eg:8080` alias for enrollment and health. The browser-facing dashboard origin must still match the Gateway's CORS and WebAuthn configuration. See [Unified Docker Stack](../guides/unified_stack.md) for the owner-bootstrap sequence.

## Environment Variables

| Variable | Default | Required behavior |
| --- | --- | --- |
| `PORT` | `3000` | Express listen port |
| `G8E_GATEWAY_URL` | None | Required HTTPS Gateway origin for browser requests and CSP `connect-src` |
| `G8E_GATEWAY_HTTP_URL` | None | Plain-HTTP Gateway bootstrap origin required for new or renewed workload enrollment |
| `G8E_RUNTIME_DIR` | None | Required writable root for installed identity and pending enrollment state |
| `GATEWAY_HEALTH_URL` | `http://g8eg:8080` in `entrypoint.sh` | Gateway readiness origin polled by the container entrypoint |
| `GATEWAY_HEALTH_PATH` | `/api/v1/health` in `entrypoint.sh` | Gateway readiness path polled by the container entrypoint |

`GATEWAY_HEALTH_URL` and `GATEWAY_HEALTH_PATH` affect only the container entrypoint. The Node process does not use them. The dashboard does not derive `G8E_GATEWAY_HTTP_URL` from `G8E_GATEWAY_URL`; set both explicitly when enrollment is needed.

## Dependency Model

Runtime dependencies are declared in `package.json` and locked in `package-lock.json`. Express and `express-rate-limit` serve the active host. `@peculiar/x509` supports CSR generation, Node's built-in `X509Certificate` parses installed certificates, and Node's built-in `fetch` backs the enrollment HTTP client. Browser vendor libraries are checked into `public/js/vendor/`. Several retained server-side modules use additional declared dependencies but are not activated by `server.js`.

## Related

- [Dashboard Architecture](architecture.md)
- [Authentication](auth.md)
- [Gateway Integration](gateway.md)
- [Server-Sent Events](sse.md)
- [Testing](tests.md)
- [Unified Docker Stack](../guides/unified_stack.md) — Docker Compose deployment for Gateway, Operator, Ensemble, and Dashboard
- [Docker Gateway Guide](../guides/docker_gateway.md) — Gateway container deployment and configuration
- [Platform Getting Started](../guides/getting_started.md) — Platform installation and quick start guide
- [Network Architecture](../architecture/network.md) — Gateway protocol surfaces, ports, and network topology
- [Platform Testing](../devs/tests.md) — g8e platform test model and verification commands
- [Documentation Guide](../devs/docs.md) — Documentation audit, ownership, and validation rules
