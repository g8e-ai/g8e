# g8e Dashboard (g8ed)

g8ed is the first-party browser interface for g8e. A Node.js 22 and Express 5 process serves a framework-free JavaScript single-page application from `public/`; the browser assets require no compilation or bundling step.

The browser performs passkey authentication and session requests directly against the g8e Gateway over HTTPS. Before Express starts listening, the dashboard also loads or enrolls an independent owner-approved `g8ed` workload identity through the Gateway's plain-HTTP enrollment surface. The current static host does not use that workload identity after startup.

## Prerequisites

- Node.js 22 or newer
- npm 11 or newer
- A running, bootstrapped g8e Gateway reachable from both the dashboard host and the browser
- An enrolled owner who can approve the dashboard workload identity
- A browser with WebAuthn support

The Gateway must allow the dashboard's exact browser origin through credentialed CORS and use matching WebAuthn relying-party settings. The browser must trust the Gateway certificate.

## Local development

From `dashboard/`:

```bash
npm ci
G8E_GATEWAY_URL=https://localhost:8443 \
G8E_GATEWAY_HTTP_URL=http://localhost:8080 \
G8E_RUNTIME_DIR=/tmp/g8ed-runtime \
npm run dev
```

A fresh runtime directory causes the dashboard to submit a platform enrollment request and wait before opening port `3000`. Approve the exact request through the Gateway console or with the repository-root `./g8e auth pending-platform-enrollments` and `./g8e auth approve-platform-enrollment <request-id> --yes` commands. After approval, the dashboard stores its identity under `G8E_RUNTIME_DIR` and serves the application at `http://localhost:3000`.

`G8E_GATEWAY_URL` is the HTTPS Gateway origin used by the browser. `G8E_GATEWAY_HTTP_URL` is the plain-HTTP Gateway origin used by the Node.js startup enrollment flow. See [Dashboard development](../docs/dashboard/development.md) for source organization and detailed setup guidance.

## Current runtime scope

The active Express application publishes `G8E_GATEWAY_URL` through `/g8e-config.js`, applies browser security headers, serves static assets, and returns `index.html` for unknown HTML routes. It does not terminate TLS, manage browser sessions, proxy WebSockets, proxy Server-Sent Events, or mount the server-side routes retained under `routes/`.

Passkey registration, sign-in, session validation, and logout use the Gateway directly. The source tree also contains browser modules for chat, cases, Operator management, approvals, audit, settings, terminal activity, and Server-Sent Events. In the standard separate-origin deployment, these modules are not operational because their dashboard-origin API and relative event paths have no handlers in the running static host. See [Dashboard architecture](../docs/architecture/dashboard.md) for the current capability status and security boundaries.

## Scripts

| Script | Description |
| --- | --- |
| `npm start` | Run the static host with `node server.js` |
| `npm run dev` | Run the static host with nodemon file watching |
| `npm run lint` | Run ESLint across the dashboard source |
| `npm test` | Run Vitest once |
| `npm run test:watch` | Run Vitest in watch mode |
| `npm run test:ui` | Open the Vitest UI |
| `npm run test:coverage` | Run Vitest with V8 coverage |

From the repository root, `make dashboard-lint` runs ESLint, `make dashboard-test` runs the Vitest suite, and `make build-dashboard` builds the container image.

## Environment variables

| Variable | Default | Purpose |
| --- | --- | --- |
| `PORT` | `3000` | Express listen port |
| `G8E_GATEWAY_URL` | none | Required HTTPS Gateway origin reachable from the browser |
| `G8E_GATEWAY_HTTP_URL` | none | Required plain-HTTP Gateway origin used for workload enrollment |
| `G8E_RUNTIME_DIR` | none | Required writable root for dashboard identity and pending enrollment state |
| `GATEWAY_HEALTH_URL` | `http://g8eg:8080` | Gateway readiness origin used by the container entrypoint |
| `GATEWAY_HEALTH_PATH` | `/api/v1/health` | Gateway readiness path used by the container entrypoint |

Missing required `G8E_*` configuration, an unexpected identity-read failure, or an enrollment failure prevents the server from listening.

## Docker

The Dockerfile uses the repository root as its build context:

```bash
docker build -f dashboard/Dockerfile -t g8e-dashboard .
```

The image runs as a non-root user, waits for Gateway health, and starts the static host on port `3000`. The repository-root `docker-compose.yml` supplies the required origins, mounts a persistent volume at `/data`, and configures that path as the runtime directory. For the complete Gateway, Operator, ensemble, and dashboard startup and approval flow, see the [Unified Docker Stack guide](../docs/guides/unified_stack.md).

## Documentation

- [Dashboard documentation](../docs/dashboard/index.md): Component architecture, authentication, Gateway integration, events, development, and testing.
- [Platform dashboard architecture](../docs/architecture/dashboard.md): g8ed's role, boundaries, deployment flow, and current capability status.
- [Dashboard testing](../docs/dashboard/tests.md): Vitest configuration, test scope, and verification commands.
- [Documentation guide](../docs/devs/docs.md): Repository-wide audit, ownership, generation, cross-linking, and versioning rules.

## License

Copyright (c) 2026 Lateralus Labs, LLC. Licensed under the Business Source License 1.1; see [LICENSE](./LICENSE). The Change License is Apache License 2.0; the license defines the applicable effective-date terms.