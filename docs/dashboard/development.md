# Development

## Prerequisites

- Node.js 22 or newer
- npm 11 or newer
- A reachable g8e Gateway
- A browser that supports WebAuthn for authentication testing

## Install and Run

From `dashboard/`:

```bash
npm ci
G8E_GATEWAY_URL=https://localhost:8443 \
G8E_GATEWAY_HTTP_URL=http://localhost:8080 \
G8E_RUNTIME_DIR=/tmp/g8ed-runtime \
npm run dev
```

The server defaults to port `3000`. All three `G8E_*` variables are required by the current startup path: `G8E_GATEWAY_URL` configures browser requests, while `G8E_GATEWAY_HTTP_URL` and `G8E_RUNTIME_DIR` configure app identity loading or enrollment. A fresh runtime directory causes startup enrollment to wait for owner approval.

Available scripts are:

| Command | Behavior |
| --- | --- |
| `npm start` | Run `node server.js` |
| `npm run dev` | Run with nodemon file watching |
| `npm test` | Run Vitest once |
| `npm run test:watch` | Run Vitest in watch mode |
| `npm run test:ui` | Open the Vitest UI |
| `npm run test:coverage` | Produce text, HTML, and LCOV V8 coverage |

From the repository root, `make dashboard-test` runs the dashboard suite and `make build-dashboard` builds the image.

## No Frontend Build Step

The browser application uses native ES modules, checked-in CSS, and checked-in vendor assets. There is no bundler, transpiler, generated asset manifest, or framework compiler. Editing a file under `public/` changes what Express serves on the next request; JavaScript and CSS receive no-cache headers despite the static middleware's long default asset lifetime.

## Browser Composition Model

`public/js/app.js` creates shared application services and feature components. Cross-component state changes use `EventBus` and constants from `public/js/constants/events.js`. HTTP paths use builders from `public/js/constants/api-paths.js`; feature files do not define endpoint strings inline. Browser data structures are represented by model classes under `public/js/models/`.

Feature HTML fragments live under `public/js/components/templates/` and are loaded as static assets. The live runtime does not render the EJS files under `views/`.

## Server Composition Model

`server.js` exports `createApp()` for static-host tests and `runStartupEnrollment()` for enrollment-path tests. The executable entrypoint reads environment configuration, performs enrollment, then listens. Keeping app construction separate from process startup lets tests instantiate Express without binding a port or terminating the test process.

The directories `routes/`, most of `services/`, `middleware/`, server-side `models/`, and `views/` contain retained code from the previous backend architecture. Do not infer runtime activation from file presence; only modules imported by `server.js` participate in the live application.

## Docker

The repository-root build context is required:

```bash
docker build -f dashboard/Dockerfile -t g8e-dashboard .
```

The image installs production dependencies, runs as the non-root `g8e` user with UID 1001, and uses `/data` for the writable runtime volume. `entrypoint.sh` waits for gateway health before starting Node. In `docker-compose.yml`, the browser gateway URL uses the externally reachable hostname and HTTPS port, while enrollment and health use the internal `g8eg` alias over plain HTTP.

## Environment Variables

| Variable | Default | Required behavior |
| --- | --- | --- |
| `PORT` | `3000` | Express listen port |
| `G8E_GATEWAY_URL` | none | Required browser-facing HTTPS gateway origin |
| `G8E_GATEWAY_HTTP_URL` | none | Required plain-HTTP app enrollment origin |
| `G8E_RUNTIME_DIR` | none | Required writable app identity root |
| `GATEWAY_HEALTH_URL` | `http://g8eg:8080` in entrypoint | Gateway readiness origin |
| `GATEWAY_HEALTH_PATH` | `/api/v1/health` in entrypoint | Gateway readiness path |

## Dependency Model

Runtime dependencies are declared in `package.json` and locked in `package-lock.json`. Express and `express-rate-limit` serve the active host. `@peculiar/x509` supports CSR generation, Node's `X509Certificate` parses installed certificates, and `undici` supports the prepared mTLS HTTP client. Browser vendor libraries are checked into `public/js/vendor/`.

## Related

- [Architecture](architecture.md)
- [Authentication](auth.md)
- [Testing](tests.md)
- [Unified Docker Stack](../guides/unified_stack.md) — Docker Compose deployment for Gateway, Operator, Ensemble, and Dashboard
- [Docker Gateway Guide](../guides/docker_gateway.md) — Gateway container deployment and configuration
- [Platform Getting Started](../guides/getting_started.md) — Platform installation and quick start guide
