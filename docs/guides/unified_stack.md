# Unified Docker Stack Guide

Last Updated: 2026-08-19
Version: v2.0.0

This guide describes how to bring up the complete g8e platform — gateway, operator, ensemble (g8ee), and dashboard (g8ed) — as a single Docker Compose stack from the repository root.

## What Ships in the Unified Stack

The repo-root `docker-compose.yml` defines four services on a single `g8e-net` bridge network:

| Service | Image | Port | Role |
| --- | --- | --- | --- |
| `g8e-gateway` | repo-root `Dockerfile` (Go, FIPS) | 8080 (HTTP), 8443 (HTTPS) | Policy Decision Point: admits transactions, manages PKI, enforces L1-L3 governance, brokers pub/sub, serves the console SPA and MCP/A2A endpoints. |
| `g8e-operator` | repo-root `Dockerfile` (Go, FIPS) | — | Policy Execution Point: outbound-only mTLS to the gateway, pulls work from its pub/sub channel, re-verifies proofs, executes L4-L5 on the gateway host. |
| `ensemble` | `ensemble/Dockerfile` (Python/FastAPI) | 8000 | First-party agentic ensemble (g8ee): submits governed intents through MCP, publishes SSE events, drives the AI reasoning loop. |
| `dashboard` | `dashboard/Dockerfile` (Node.js/Express) | 3000 | Operator dashboard (g8ed): browser UI for chat, operator management, audit, and settings. |

The gateway and operator share the same Go binary (the repo-root `Dockerfile`), built with FIPS 140-3 approved mode enabled. The ensemble and dashboard each have their own Dockerfile rooted at the repo root so they can copy their source and the in-tree `protocol/python/` package.

## Quick Start

From the repository root:

```bash
docker compose up
```

This builds all four images and starts the stack. The gateway boots first; the operator, ensemble, and dashboard start once the gateway's health check passes (`/api/v1/health`).

Once the stack is healthy:

- Gateway HTTP: http://localhost:8080
- Gateway HTTPS: https://localhost:8443
- Ensemble API: http://localhost:8000
- Dashboard: http://localhost:3000

The gateway runs in `doctrine` posture by default with CORS and passkey RP origins pre-configured for the dashboard origin (`http://localhost:3000`). This lets the dashboard register WebAuthn passkeys and make authenticated cross-origin requests to the gateway from the browser.

## Environment Variables

The compose file supports the following environment variable overrides:

| Variable | Default | Description |
| --- | --- | --- |
| `G8E_PREFIX` | `g8e` | Prefix for container names. |
| `G8E_HTTP_PORT` | `8080` | Host port for the gateway HTTP port. |
| `G8E_HTTPS_PORT` | `8443` | Host port for the gateway HTTPS port. |
| `G8E_ENSEMBLE_PORT` | `8000` | Host port for the ensemble API. |
| `G8E_DASHBOARD_PORT` | `3000` | Host port for the dashboard. |

Example: run the stack on alternate ports:

```bash
G8E_HTTP_PORT=18080 G8E_HTTPS_PORT=18443 G8E_DASHBOARD_PORT=13000 docker compose up
```

## mTLS and PKI

The gateway generates its own CA on startup and initializes its PKI under the `g8e-gateway-data` named volume at `/root/.g8e`. The operator enrolls over mTLS and receives its identity certificate, stored in the `g8e-operator-data` named volume. The ensemble mounts the gateway's data volume read-only at `/root/.g8e` so it can read the gateway's issued app cert/key and the trust bundle for mTLS. The dashboard does not require direct PKI access; it authenticates through the browser via WebAuthn passkeys.

The ensemble's `G8E_RUNTIME_DIR`, `G8E_PKI_DIR`, and `G8E_CA_CERT_PATH` environment variables point at `/root/.g8e` and `/root/.g8e/pki/...`, matching the gateway's `working_dir: /root` and the volume mount. See [Network Architecture](../architecture/network.md) for the full PKI hierarchy and enrollment procedures.

## Relationship to Per-Demo Composes

The unified platform compose is distinct from the per-demo composes in `demos/<org>/compose.yml`. The per-demo composes are org-specific, hermetically sealed deployments on five isolated networks that exercise particular compliance scenarios; they build the gateway/operator image from the repo-root `Dockerfile` via `context: ../..` and are driven by the `g8e demos` CLI. The per-demo composes do not include the ensemble or dashboard. See the [Demos README](../../demos/README.md) for the full distinction.

## Stopping and Cleaning Up

```bash
# Stop the stack (keep volumes):
docker compose down

# Stop the stack and remove volumes (PKI state, audit ledger):
docker compose down -v
```

Removing volumes destroys the gateway's PKI and audit state. The next `docker compose up` re-bootstrap the CA and re-enroll the operator.

## Related Documentation

- [Platform Architecture Overview](../architecture/overview.md): The five-layer governance pipeline and component roles.
- [Ensemble (g8ee)](../architecture/ensemble.md): The agentic ensemble that runs as the `ensemble` service.
- [Dashboard (g8ed)](../architecture/dashboard.md): The operator dashboard that runs as the `dashboard` service.
- [Gateway Architecture](../architecture/gateway.md): The gateway service stack.
- [Operator Architecture](../architecture/operator.md): The operator execution boundary.
- [Docker Gateway Guide](./docker_gateway.md): Building and running the gateway image standalone.
