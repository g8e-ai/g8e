# g8e Dashboard (g8ed)

Browser-based single-page application and static SPA host for the g8e platform. Vanilla JavaScript, no framework, no build step. The server is a minimal Express static host; all authentication and API calls go directly from the browser to the g8e Gateway over HTTPS.

g8ed is a first-party component of the g8e platform, shipped in-tree under `dashboard/`. It is the operator web UI for the gateway/operator binary — the same UI that was originally split out of the g8e repo for isolated hardening and is now reunited as part of the v2.0.0 monorepo.

## Quick Start

```bash
npm ci
G8E_GATEWAY_URL=https://localhost:8443 \
G8E_GATEWAY_HTTP_URL=http://localhost:8080 \
G8E_RUNTIME_DIR=/tmp/g8ed-runtime \
npm run dev
```

The server listens on `http://localhost:3000` and serves assets from `public/`. The browser makes active auth/API calls directly to the g8e Gateway configured by `G8E_GATEWAY_URL`. The server uses `G8E_GATEWAY_HTTP_URL` and `G8E_RUNTIME_DIR` to load or enroll its independent `g8ed` app workload identity before listening.

## Prerequisites

- Node.js 22+
- npm 11+
- A running g8e Gateway reachable from the browser

## Docker

The dashboard ships with a Dockerfile rooted at the g8e repo root (build context `.`), so it can be built as part of the unified platform stack:

```bash
docker build -f dashboard/Dockerfile -t g8e-dashboard .
```

See the repo-root `docker-compose.yml` for the unified stack wiring (gateway + operator + ensemble + dashboard).

## Documentation

- [Dashboard documentation](../docs/dashboard/index.md): Detailed component architecture, authentication, gateway integration, SSE, operator surfaces, development, and tests.
- [Platform-level architecture](../docs/architecture/dashboard.md): g8ed's role and boundaries in the complete g8e platform.

## Scripts

| Script | Description |
| --- | --- |
| `npm start` | Run the static SPA host (`node server.js`) |
| `npm run dev` | Run with nodemon file-watching |
| `npm test` | Run Vitest once |
| `npm run test:watch` | Run Vitest in watch mode |
| `npm run test:coverage` | Run Vitest with V8 coverage |

## Environment Variables

| Variable | Default | Description |
| --- | --- | --- |
| `PORT` | `3000` | HTTP port |
| `G8E_GATEWAY_URL` | none | Required browser-facing HTTPS gateway origin |
| `G8E_GATEWAY_HTTP_URL` | none | Required container-facing plain-HTTP app enrollment origin |
| `G8E_RUNTIME_DIR` | none | Required writable root for the dashboard app identity |
| `GATEWAY_HEALTH_URL` | `http://g8eg:8080` | Gateway health base URL used by the Docker entrypoint |
| `GATEWAY_HEALTH_PATH` | `/api/v1/health` | Gateway health path used by the Docker entrypoint |

## License

Copyright (c) 2026 Lateralus Labs, LLC. Licensed under the Business Source License 1.1 — see [LICENSE](./LICENSE) for details. As of the Change Date (2030-08-18), this software converts to Apache License 2.0.
