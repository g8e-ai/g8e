# Unified Docker Stack Guide

Last Updated: 2026-08-31
Version: v2.1.2

This guide describes how to bring up the complete g8e platform — gateway, operator, ensemble (g8ee), and dashboard (g8ed) — as a single Docker Compose stack from the repository root, using either Docker Compose directly or the first-class `./g8e docker` CLI management commands.

## What Ships in the Unified Stack

The repository root `docker-compose.yml` defines four services on a single `g8e-net` bridge network:

| Service | Image | Port | Role |
| --- | --- | --- | --- |
| `g8e-gateway` | repo-root `Dockerfile` (Go, FIPS) | 8080 (HTTP), 8443 (HTTPS) | Policy Decision Point: admits transactions, manages PKI, enforces L1-L3 governance, brokers pub/sub, serves the console SPA and MCP/A2A endpoints. |
| `g8e-operator` | repo-root `Dockerfile` (Go, FIPS) | — | Policy Execution Point: outbound-only mTLS to the gateway, pulls work from its pub/sub channel, re-verifies proofs, and executes L4-L5 actions. |
| `ensemble` | `ensemble/Dockerfile` (Python/FastAPI) | 8000 | First-party agentic ensemble (g8ee): submits `GovernanceEnvelope` transactions directly to the privileged governance endpoint with the operator transport identity, publishes SSE events, and drives the AI reasoning loop. See the [g8ee documentation](../ensemble/index.md). |
| `dashboard` | `dashboard/Dockerfile` (Node.js/Express) | 3000 | Operator dashboard (g8ed): browser UI for chat, operator management, audit, and settings. See the [g8ed documentation](../dashboard/index.md). |

The gateway and operator share the same Go binary (the repo-root `Dockerfile`), built with FIPS 140-3 approved mode enabled. The ensemble and dashboard each have their own Dockerfile rooted at the repository root so they can copy their source and the in-tree `protocol/python/` package.

## Quick Start

The unified stack uses a two-phase startup model. Only the gateway starts by default (unprofiled); the operator, dashboard, and ensemble belong to the `bootstrapped` Compose profile (`profiles: [bootstrapped]`) and require a human owner to enroll before they can start. The gateway starts with zero users and issues no platform certificates until the first owner enrolls and approves pending platform enrollment requests.

### Automated Workflow (`./g8e docker start --full`)

The easiest way to start the complete stack is through the interactive CLI command from the repository root:

```bash
./g8e docker start --full
```

This single command starts the stack containers, waits for the gateway health check to pass, enrolls the CLI user (creating the first owner and CLI mTLS identity), and interactively prompts to approve each platform workload enrollment request in sequence (ensemble, dashboard, and operator).

### Manual Workflow (Docker Compose)

You can also orchestrate the lifecycle manually in three distinct phases:

#### Phase 1: Start the gateway

From the repository root:

```bash
docker compose up -d --build
```

This builds the images and starts only the gateway service. The gateway becomes healthy immediately. No other containers start because the operator, dashboard, and ensemble are gated behind the `bootstrapped` profile.

#### Phase 2: Enroll the first owner

After the gateway is healthy, enroll the first owner to establish the root of trust and create the CLI mTLS identity:

```bash
# 1. Wait for the gateway to be healthy.
until curl -fsS http://localhost:8080/api/v1/health >/dev/null 2>&1; do sleep 2; done

# 2. Enroll the first owner.
#    -e specifies the gateway HTTP discovery endpoint (host or host:port).
#    mTLS port defaults to 8443 and does not need to be set explicitly.
./g8e auth enroll user -e localhost
```

If the local CLI has a stale trust bundle from a previous gateway instance (for example, after `docker compose down -v`), the coordinator detects that the gateway is not yet bootstrapped and initializes cleanly instead of failing on stale trust anchors.

#### Phase 3: Start and approve platform workloads

Once the first owner is enrolled, start the operator, dashboard, and ensemble:

```bash
# 3. Start the bootstrapped-profile workloads.
docker compose --profile bootstrapped up -d

# 4. List pending platform enrollment requests submitted by the workloads.
./g8e auth pending-platform-enrollments

# 5. Approve each request by exact request ID (recommended order: operator, dashboard, ensemble).
./g8e auth approve-platform-enrollment <operator-request-id> --yes
./g8e auth approve-platform-enrollment <dashboard-request-id> --yes
./g8e auth approve-platform-enrollment <ensemble-request-id> --yes

# 6. Verify that all workloads have completed enrollment and are healthy.
docker compose ps
```

Alternatively, use the gateway console at `https://localhost:8443/console/` to list pending requests and approve them in a browser. The console signs in via the same first-owner WebAuthn passkey created during owner enrollment.

Once the stack is healthy, the following endpoints are available:

- Gateway HTTP Discovery & CA Bundle: `http://localhost:8080`
- Gateway HTTPS & Console SPA: `https://localhost:8443` (Console: `https://localhost:8443/console/`)
- Ensemble API: `http://localhost:8000`
- Dashboard UI: `http://localhost:3000`

The gateway runs in `doctrine` posture by default with CORS and passkey RP origins pre-configured for the dashboard origin (`http://localhost:3000`). This allows the dashboard to register WebAuthn passkeys and make authenticated cross-origin requests to the gateway from the browser.

## CLI Management (`./g8e docker`)

The `g8e` binary provides dedicated CLI subcommands for managing the root Docker Compose stack:

| Command | Description |
| --- | --- |
| `./g8e docker start` | Start the gateway container (default profile). |
| `./g8e docker start --full` | Start the full stack and run the interactive owner enrollment and workload approval walkthrough. |
| `./g8e docker start --full --skip-enroll` | Start the full stack in the background without the interactive walkthrough (workloads remain pending approval). |
| `./g8e docker stop` | Stop and remove stack containers while preserving volumes and networks. |
| `./g8e docker status` | Display container and service status (`docker compose ps`). |
| `./g8e docker build` | Build all Docker images defined in `docker-compose.yml` (`--no-cache` supported). |
| `./g8e docker logs [service] [-f]` | Show and stream logs for the stack or a specific service container. |
| `./g8e docker reset [--full]` | Clean containers, volumes, and networks, then restart the stack. |
| `./g8e docker rebuild [--full]` | Rebuild images from scratch and restart the stack (`--no-cache` supported). |
| `./g8e docker clean` | Remove all containers, volumes, and networks across all profiles (`--yes` supported). |

## Service Dependencies, Health Checks, and Resource Limits

The operator, ensemble, and dashboard services declare dependencies on the gateway being healthy (`depends_on` with `condition: service_healthy` for `g8e-gateway`). The ensemble service also depends on `g8e-operator: condition: service_started`.

Health checks defined in `docker-compose.yml` reflect truthful service readiness:

- `g8e-gateway`: `wget --no-verbose --tries=1 --spider http://localhost:8080/api/v1/health`
- `g8e-operator`: `test -f /root/.g8e/pki/operator.crt`
- `ensemble`: `python3 -c "import urllib.request; urllib.request.urlopen('http://localhost:8000/health')"`
- `dashboard`: `wget --no-verbose --tries=1 --spider http://localhost:3000/`

Because platform enrollment requires owner approval, the operator, ensemble, and dashboard health checks remain not-ready while approval is pending. Once the owner approves their requests, each component completes enrollment and transitions to healthy.

All four services define CPU and memory resource constraints in `docker-compose.yml`:

| Service | CPU limit | Memory limit | CPU reservation | Memory reservation |
| --- | --- | --- | --- | --- |
| `g8e-gateway` | 2 | 1G | 0.5 | 256M |
| `g8e-operator` | 2 | 1G | 0.5 | 256M |
| `ensemble` | 2 | 2G | 0.5 | 512M |
| `dashboard` | 1 | 512M | 0.25 | 128M |

## Environment Variables

The compose file supports the following environment variable overrides. A `.env.example` file at the repository root documents all six variables with defaults and descriptions:

| Variable | Default | Description |
| --- | --- | --- |
| `G8E_PREFIX` | `g8e` | Prefix for container names (`${G8E_PREFIX}-gateway`, `${G8E_PREFIX}-operator`, etc.). |
| `G8E_HTTP_PORT` | `8080` | Host port mapped to the gateway HTTP discovery surface. |
| `G8E_HTTPS_PORT` | `8443` | Host port mapped to the gateway HTTPS/mTLS surface. |
| `G8E_ENSEMBLE_PORT` | `8000` | Host port mapped to the ensemble API. |
| `G8E_DASHBOARD_PORT` | `3000` | Host port mapped to the dashboard Express server. |
| `G8E_HOSTNAME` | `localhost` | Public hostname the gateway advertises in approval links, CORS, and passkey RP origins. Set to a real hostname when accessing the gateway from outside localhost. |

Example: run the stack on alternate ports:

```bash
G8E_HTTP_PORT=18080 G8E_HTTPS_PORT=18443 G8E_DASHBOARD_PORT=13000 docker compose up -d --build
```

## mTLS, PKI, and Workload Identity

The gateway generates its own root CA on startup and initializes its PKI under the `g8e-gateway-data` named volume at `/root/.g8e`. The canonical gateway trust bundle is maintained at `.g8e/pki/trust/g8eg-ca-bundle.pem` and served over plain HTTP at `http://<gateway>:8080/.well-known/g8e/pki/ca-bundle`. The compose file bind-mounts the host's `/etc/hosts` and `/etc/hostname` read-only into the gateway container at `/etc/hosts.host` and `/etc/hostname.host` so the network identity detector covers the host's real IPs and hostname in the serving certificate SANs; the certificate is regenerated on startup when SAN drift is detected. See [Docker Gateway Guide](./docker_gateway.md#host-identity-bind-mounts) for details.

The gateway starts with zero users and issues no platform certificates until the first owner enrolls and approves pending enrollment requests. The operator, ensemble, and dashboard each enroll via the owner-approved platform enrollment protocol: they submit a platform enrollment request (containing a CSR and system fingerprint) to the gateway's plain-HTTP discovery surface, the owner approves the request by exact request ID via authenticated mTLS, and the component signs a canonical completion transcript and receives its enrolled credentials. See [auth.md](../architecture/auth.md) §1.5 for the full protocol specification.

### Component Identity and Storage

- **`g8e-operator`**: Stores its enrolled identity certificate and private key in the `g8e-operator-data` named volume at `/root/.g8e/pki/operator.crt` and `/root/.g8e/pki/operator.key`. It connects outbound to the gateway over mTLS (`operator start -e g8e.local`) using `spiffe://g8e.local/operator/<org_id>/<operator_id>/<operator_session_id>`.
- **`ensemble`**: Has its own `g8e-ensemble-data` named volume mounted at `/root/.g8e` (read-write) and writes its enrolled cert, key, and trust bundle to its own runtime tree (`pki/issued/apps/g8ee.crt`, `g8ee.key`, `pki/trust/hub-bundle.pem`) after owner approval. The `AppEnrollmentService` (`ensemble/app/services/infra/app_enrollment_service.py`) manages the nine-step resumable enrollment sequence. Pending state (private key, requester token, request ID, CSR fingerprint, expiry) is persisted to `pki/pending-enrollment/g8ee.json` with 0600 permissions so the ensemble resumes the same request and key material across container restarts. Its app identity SPIFFE URI is `spiffe://g8e.local/app/g8ee`.
- **`dashboard`**: Has its own `g8e-dashboard-data` named volume mounted at `/data` (the dashboard container runs as the non-root `g8e` user with UID 1001, so `/data` is owned by `g8e:g8e` rather than `/root/.g8e`) with `G8E_RUNTIME_DIR=/data`. It enrolls via `dashboard/services/infra/app-enrollment-service.js`, obtaining `spiffe://g8e.local/app/g8ed` and writing credentials to `pki/issued/apps/g8ed.crt`, `g8ed.key`, `pki/trust/hub-bundle.pem`. The dashboard's **container** holds an mTLS app identity for prepared server-to-server gateway clients, which the current static host does not construct, while the dashboard's **browser SPA** authenticates independently via WebAuthn passkeys. See [Dashboard (g8ed)](../architecture/dashboard.md) for the dashboard architecture.

### Operator Cert Sharing for Governance Envelopes

The gateway Privileged Route Registry restricts `POST /api/v1/governance/envelopes` to authorized operator sessions and rejects submissions authenticated with app certificates with `ErrPrivilegedEndpointAccess`. To submit governed envelopes while preserving this security boundary, the compose file mounts the operator's data volume read-only into the ensemble container:

- Volume mount: `g8e-operator-data:/operator-state:ro`
- Environment variables: `G8E_GOVERNANCE_OPERATOR_CERT=/operator-state/pki/operator.crt` and `G8E_GOVERNANCE_OPERATOR_KEY=/operator-state/pki/operator.key`

The ensemble `GovernanceClient` (`ensemble/app/clients/governance_client.py`) builds a dedicated TLS configuration from the operator cert/key specifically for governance transport. It lazily parses the operator SPIFFE URI (`spiffe://g8e.local/operator/<org_id>/<operator_id>/<operator_session_id>`) from the certificate to bind the envelope's `operator_id` and `operator_session_id`, satisfying the gateway `verifyEnvelopeIdentityBinding` check. All other ensemble gateway traffic (settings, SSE push, health) continues to use the ensemble's own app certificate (`spiffe://g8e.local/app/g8ee`).

## Headless Gateway-Only Deployment

To run only the gateway without starting the platform workloads, start the default compose profile and enroll the owner using the `--headless` flag:

```bash
docker compose up -d --build
./g8e auth enroll user --headless -e localhost
```

Because `g8e-operator`, `ensemble`, and `dashboard` belong to the `bootstrapped` profile, running `docker compose up -d` without `--profile bootstrapped` starts only `g8e-gateway`. The `--headless` flag on `auth enroll user` completes user bootstrap while skipping the interactive browser-based passkey ceremony, making it suitable for CI, headless servers, and automation environments.

## Relationship to Per-Demo Composes

The unified platform compose is distinct from the per-demo composes in `demos/<org>/compose.yml` (Healthcare, Finance, DHS, FedRAMP, Frontend). The per-demo composes are org-specific, hermetically sealed deployments on five isolated networks (`net_untrusted`, `net_perimeter`, `net_internal`, `net_secure`, `net_mgmt`) that exercise specialized compliance and doctrine scenarios. They build the gateway and operator images from the repo-root `Dockerfile` via `context: ../..` and are driven by the `g8e demos` CLI. The per-demo composes do not include the ensemble or dashboard services. See the [Demos README](../../demos/README.md) for the full comparison.

## Stopping and Cleaning Up

```bash
# Stop the stack (keep volumes, preserving PKI state and audit ledger):
docker compose down
# Or:
# ./g8e docker stop

# Stop the stack and remove volumes (PKI state and audit ledger):
docker compose down -v
# Or:
# ./g8e docker clean
```

Removing volumes destroys the gateway's PKI and audit state. The next startup re-bootstraps the CA and requires the owner to re-enroll and re-approve the platform workloads.

## Related Documentation

- [Platform Architecture Overview](../architecture/overview.md): The five-layer governance pipeline and component roles.
- [Ensemble (g8ee)](../architecture/ensemble.md): The agentic ensemble architecture, agent models, and event streams.
- [Dashboard (g8ed)](../architecture/dashboard.md): The operator dashboard architecture and UI surfaces.
- [g8ee Documentation](../ensemble/index.md): Detailed g8ee component documentation — agents, governance, prompts, SSE, storage, and evals.
- [g8ed Documentation](../dashboard/index.md): Detailed g8ed component documentation — architecture, auth, gateway integration, SSE, and operator surfaces.
- [Gateway Architecture](../architecture/gateway.md): The gateway service stack, endpoints, and policy enforcement.
- [Operator Architecture](../architecture/operator.md): The operator execution boundary and L4-L5 lifecycle.
- [Docker Gateway Guide](./docker_gateway.md): Building and running the gateway image standalone and in demo environments.
- [Authentication and Identity](../architecture/auth.md): The full authentication, passkey ceremony, and platform enrollment protocol specifications.
