# Unified Docker Stack Guide

Last Updated: 2026-08-20
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
docker compose up -d --build
```

This builds all four images and starts the stack. The gateway boots first; the operator, ensemble, and dashboard start once the gateway's health check passes (`/api/v1/health`). All four containers start, but the operator, ensemble, and dashboard remain not-ready until their owner-approved platform enrollment requests are approved. Do not use `docker compose up --wait` before approval; it is expected to time out while enrollment is pending.

### Owner-approved platform activation

After `docker compose up -d --build`, the gateway is healthy but the platform workloads are not. Activate them by enrolling the first owner and approving each pending enrollment request:

```bash
# 1. Wait for the gateway to be healthy.
until curl -fsS http://localhost:8080/api/v1/health >/dev/null 2>&1; do sleep 2; done

# 2. Enroll the first owner. This creates the first user and a usable CLI mTLS identity.
#    -e is the gateway HTTP discovery endpoint (host or host:port); the coordinator
#    derives the HTTP port. --port defaults to 8443 for mTLS and does not need to be set.
./g8e auth enroll user -e localhost

# 3. List pending platform enrollment requests (operator, dashboard, ensemble).
./g8e auth pending-platform-enrollments

# 4. Approve each request by exact request ID. The recommended order is operator, dashboard, ensemble.
./g8e auth approve-platform-enrollment <operator-request-id> --yes
./g8e auth approve-platform-enrollment <dashboard-request-id> --yes
./g8e auth approve-platform-enrollment <ensemble-request-id> --yes

# 5. Wait for the workloads to become healthy.
docker compose ps
```

Alternatively, use the gateway console at `https://localhost:8443/console/` to list pending requests and approve them in a browser. The console signs in via the same first-owner WebAuthn passkey created in step 2.

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
| `G8E_HTTP_PORT` | `8080` | Host port for the gateway HTTP discovery surface. |
| `G8E_HTTPS_PORT` | `8443` | Host port for the gateway HTTPS/mTLS surface. |
| `G8E_ENSEMBLE_PORT` | `8000` | Host port for the ensemble API. |
| `G8E_DASHBOARD_PORT` | `3000` | Host port for the dashboard. |
| `G8E_HOSTNAME` | `localhost` | Public hostname the gateway advertises in approval links, CORS, and passkey RP origins. Set to a real hostname when the browser reaches the gateway via that hostname. |

A `.env.example` file at the repository root documents all six variables with defaults and one-line descriptions; copy it to `.env` and edit, or set the variables inline.

Example: run the stack on alternate ports:

```bash
G8E_HTTP_PORT=18080 G8E_HTTPS_PORT=18443 G8E_DASHBOARD_PORT=13000 docker compose up
```

## mTLS and PKI

The gateway generates its own CA on startup and initializes its PKI under the `g8e-gateway-data` named volume at `/root/.g8e`. The gateway starts with zero users and issues no platform certificates until the first owner enrolls and approves pending enrollment requests. The operator, ensemble, and dashboard each enroll via the owner-approved platform enrollment protocol: they submit a platform enrollment request (with a CSR and system fingerprint) to the gateway's plain-HTTP discovery surface, the owner approves the request by exact request ID via authenticated mTLS, and the component signs a canonical completion transcript and receives its enrolled credentials. See [auth.md](../architecture/auth.md) §1.5 for the full protocol.

The operator stores its enrolled identity certificate in the `g8e-operator-data` named volume. The ensemble has its own `g8e-ensemble-data` named volume mounted at `/root/.g8e` (read-write) and writes its enrolled cert, key, and trust bundle to its own runtime tree (`pki/issued/apps/g8ee.crt`, `g8ee.key`, `pki/trust/hub-bundle.pem`) after the owner approves its platform enrollment request. The `AppEnrollmentService` (`ensemble/app/services/infra/app_enrollment_service.py`) implements the nine-step resumable enrollment sequence: load-or-validate installed identity, load persisted pending attempt, generate a P-256 key and CSR, submit the platform enrollment request, poll status with bounded backoff, sign the canonical completion transcript, validate the response, write credentials atomically, and return the app identity. Pending state (private key, requester token, request ID, CSR fingerprint, expiry) is persisted to `pki/pending-enrollment/g8ee.json` with 0600 permissions so the ensemble resumes the same request and key material on restart. The dashboard follows the same model: it has its own `g8e-dashboard-data` named volume mounted at `/data` (the dashboard container runs as the non-root `g8e` user, so the volume is mounted at `/data`, which is created and chowned to `g8e:g8e` in the Dockerfile, rather than `/root/.g8e`) and enrolls via `dashboard/services/infra/app-enrollment-service.js`, obtaining `spiffe://g8e.local/app/g8ed` and writing credentials to `pki/issued/apps/g8ed.crt`, `g8ed.key`, `pki/trust/hub-bundle.pem`. The dashboard's **container** holds an mTLS app identity for server-to-server gateway calls, while the dashboard's **browser SPA** still authenticates via WebAuthn passkeys (the two identity surfaces are independent). See [Dashboard (g8ed)](../architecture/dashboard.md) for the dashboard architecture.

The ensemble's `G8E_GATEWAY_HTTP_URL` points at the gateway's plain-HTTP discovery surface (`http://g8e.local:8080`) for platform enrollment request submission and CA bundle fetch. `G8E_OPERATOR_URL` and `G8E_OPERATOR_PUBSUB_URL` point at the gateway's HTTPS mTLS surface (`https://g8e.local:8443` and `wss://g8e.local:8443`) for the operator clients that connect after enrollment. `G8E_RUNTIME_DIR` points at `/root/.g8e` inside the ensemble's own volume. The dashboard's `G8E_GATEWAY_HTTP_URL` points at the gateway's plain-HTTP discovery surface (`http://g8eg:8080`, using the gateway's docker network alias) and `G8E_RUNTIME_DIR` points at `/data` inside the dashboard's own volume (the dashboard runs as the non-root `g8e` user, so `/data` is used instead of `/root/.g8e`). See [Network Architecture](../architecture/network.md) for the full PKI hierarchy and [Build Apps](./build_apps.md) § Identity and Authentication for the public app enrollment contract.

## Relationship to Per-Demo Composes

The unified platform compose is distinct from the per-demo composes in `demos/<org>/compose.yml`. The per-demo composes are org-specific, hermetically sealed deployments on five isolated networks that exercise particular compliance scenarios; they build the gateway/operator image from the repo-root `Dockerfile` via `context: ../..` and are driven by the `g8e demos` CLI. The per-demo composes do not include the ensemble or dashboard. See the [Demos README](../../demos/README.md) for the full distinction.

## Stopping and Cleaning Up

```bash
# Stop the stack (keep volumes):
docker compose down

# Stop the stack and remove volumes (PKI state, audit ledger):
docker compose down -v
```

Removing volumes destroys the gateway's PKI and audit state. The next `docker compose up` re-bootstraps the CA and requires the owner to re-enroll and re-approve the platform workloads.

## Related Documentation

- [Platform Architecture Overview](../architecture/overview.md): The five-layer governance pipeline and component roles.
- [Ensemble (g8ee)](../architecture/ensemble.md): The agentic ensemble that runs as the `ensemble` service.
- [Dashboard (g8ed)](../architecture/dashboard.md): The operator dashboard that runs as the `dashboard` service.
- [Gateway Architecture](../architecture/gateway.md): The gateway service stack.
- [Operator Architecture](../architecture/operator.md): The operator execution boundary.
- [Docker Gateway Guide](./docker_gateway.md): Building and running the gateway image standalone.
