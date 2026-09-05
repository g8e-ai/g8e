# Docker Gateway Guide

Last Updated: 2026-09-05
Version: v2.1.4

This document describes the procedures for building and deploying the g8e Gateway using Docker and Docker Compose.

## Quick Start

### Build Image

Build the g8e container image from the repository root:

```bash
docker build -t g8e-gateway:latest .
```

### Run Standalone Container

Start the gateway in doctrine mode with default port mappings and volume persistence:

```bash
docker run -d \
  --name g8e-gateway \
  -p 8080:8080 \
  -p 8443:8443 \
  -v g8e-data:/root/.g8e \
  g8e-gateway:latest \
  gw start -f --posture doctrine
```

The `-f` flag runs the gateway in the foreground instead of spawning a background subprocess. This is required for container usage so the process does not exit immediately. Standalone execution runs only the Gateway Policy Decision Point without the operator, dashboard, or ensemble services.

## Docker Compose Deployment

The repository includes a root `docker-compose.yml` that deploys the full platform stack: the Gateway, the Operator, the Ensemble, and the Dashboard on a shared `g8e-net` bridge network.

The compose stack uses a two-phase startup model. Only the gateway starts by default (unprofiled). The platform workloads (operator, ensemble, and dashboard) belong to the `bootstrapped` Compose profile (`profiles: [bootstrapped]`) and require an enrolled owner before they can start. The gateway starts with zero users and issues no platform certificates until the first owner enrolls and approves pending platform enrollment requests.

### Core Services

- **`g8e-gateway`**: Policy Decision Point. Admits transactions, manages PKI, and enforces L1-L3 governance. Starts via `gw start -f` with CORS and passkey RP origins configured for the dashboard (`http://${G8E_HOSTNAME:-localhost}:${G8E_DASHBOARD_PORT:-3000}`). Starts with zero users and issues no platform certificates until the first owner enrolls.
- **`g8e-operator`**: Policy Execution Point. Connects to the gateway via outbound mTLS to pull work from its pub/sub channel, re-verify proofs, and execute L4-L5 actions. Uses `operator start -e g8e.local`. The gateway registers `g8e.local` and `g8eg` as network aliases on `g8e-net`, so the operator resolves the gateway by the hostname matching its certificate SANs. Gated behind the `bootstrapped` profile. On first start with no credentials, the operator submits a platform enrollment request and waits for owner approval before becoming ready.
- **`ensemble`**: First-party agentic ensemble (g8ee) Python FastAPI service built from `ensemble/Dockerfile`. Submits governed intents through MCP and publishes SSE events. Gated behind the `bootstrapped` profile. Submits a platform enrollment request at startup via the owner-approved platform enrollment protocol and waits for owner approval before its lifespan completes. Connects to the gateway document store, pub/sub, and blob APIs over mTLS using its enrolled credentials (`pki/issued/apps/g8ee.crt`). Mounts `g8e-operator-data:/operator-state:ro` read-only to access the operator's enrolled mTLS certificate for privileged governance envelope signing. See the [g8ee documentation](../ensemble/index.md) for the ensemble component architecture, agents, governance, and SSE pipeline.
- **`dashboard`**: Operator dashboard (g8ed) Node.js Express service built from `dashboard/Dockerfile`. Provides the browser UI for chat, operator management, audit, and settings. Gated behind the `bootstrapped` profile. Runs as the non-root `g8e` user (UID 1001) with volume mounted at `g8e-dashboard-data:/data` and `G8E_RUNTIME_DIR=/data`. Submits a platform enrollment request at startup to obtain an mTLS app identity (`spiffe://g8e.local/app/g8ed`) for prepared server-to-server gateway clients; the current static host does not construct those clients. The browser SPA authenticates independently to the gateway via WebAuthn passkeys. See the [g8ed documentation](../dashboard/index.md) for the dashboard component architecture, auth, gateway integration, and operator surfaces.

### Service Dependencies and Health Checks

The operator, ensemble, and dashboard services depend on the gateway health check passing (`depends_on` with `condition: service_healthy` for `g8e-gateway`). The ensemble service also depends on `g8e-operator: condition: service_started`.

Health checks defined in `docker-compose.yml` reflect truthful service readiness:

- `g8e-gateway`: `wget --no-verbose --tries=1 --spider http://localhost:8080/api/v1/health`
- `g8e-operator`: `test -f /root/.g8e/pki/operator.crt`
- `ensemble`: `python3 -c "import urllib.request; urllib.request.urlopen('http://localhost:8000/health')"`
- `dashboard`: `wget --no-verbose --tries=1 --spider http://localhost:3000/`

Because platform enrollment requires owner approval, the operator, ensemble, and dashboard health checks remain not-ready while approval is pending. Once the owner approves their requests, each component completes enrollment and transitions to healthy.

### Environment Variables

The compose file supports the following environment variable overrides. A `.env.example` file at the repository root documents all six with defaults and descriptions:

- **`G8E_PREFIX`**: Prefix for container names (default: `g8e`). Containers are named `${G8E_PREFIX}-gateway`, `${G8E_PREFIX}-operator`, `${G8E_PREFIX}-ensemble`, and `${G8E_PREFIX}-dashboard`.
- **`G8E_HTTP_PORT`**: Host port mapped to the gateway HTTP discovery surface, container port 8080 (default: `8080`). Used for health checks, CA bundle fetch, and platform enrollment submission.
- **`G8E_HTTPS_PORT`**: Host port mapped to the gateway HTTPS/mTLS surface, container port 8443 (default: `8443`). Used for MCP, A2A, governance envelopes, document store, WebSocket pub/sub, and the console SPA.
- **`G8E_ENSEMBLE_PORT`**: Host port mapped to the ensemble FastAPI API, container port 8000 (default: `8000`).
- **`G8E_DASHBOARD_PORT`**: Host port mapped to the dashboard Express server, container port 3000 (default: `3000`).
- **`G8E_HOSTNAME`**: Public hostname the gateway advertises in approval links, host-header validation, CORS, and passkey RP origins (default: `localhost`). Set to a real hostname (e.g. `dev.g8e.local`) when accessing the gateway from outside localhost.

### Resource Limits

All four services define CPU and memory constraints in `docker-compose.yml`:

| Service | CPU limit | Memory limit | CPU reservation | Memory reservation |
| --- | --- | --- | --- | --- |
| `g8e-gateway` | 2 | 1G | 0.5 | 256M |
| `g8e-operator` | 2 | 1G | 0.5 | 256M |
| `ensemble` | 2 | 2G | 0.5 | 512M |
| `dashboard` | 1 | 512M | 0.25 | 128M |

These limits document expected resource allocations for the development stack. Adjust them for production workloads or create a compose override file (`docker-compose.override.yml`).

## CLI Management (`./g8e docker`)

The `g8e` CLI provides dedicated management subcommands for the root Docker Compose stack:

| Command | Description |
| --- | --- |
| `./g8e docker start` | Start the gateway container (default profile). |
| `./g8e docker start --full` | Start the full stack and run the interactive enrollment walkthrough. |
| `./g8e docker start --full --skip-enroll` | Start the full stack without the interactive walkthrough. |
| `./g8e docker stop` | Stop and remove stack containers while preserving volumes and networks. |
| `./g8e docker status` | Display container and service status (`docker compose ps`). |
| `./g8e docker build` | Build all Docker images defined in `docker-compose.yml` (`--no-cache` supported). |
| `./g8e docker logs [service] [-f]` | Show and stream logs for the stack or a specific service. |
| `./g8e docker reset [--full]` | Clean containers, volumes, and networks, then restart the stack. |
| `./g8e docker rebuild [--full]` | Rebuild images and restart the stack (`--no-cache` supported). |
| `./g8e docker clean` | Remove all containers, volumes, and networks across all profiles. |

## Owner-Approved Platform Bootstrap

Deploying the full stack requires completing the owner-approved platform enrollment sequence:

```bash
# 1. Start the gateway service.
docker compose up -d --build
# Or use the CLI wrapper:
# ./g8e docker start

# 2. Wait for the gateway to become healthy.
until curl -fsS http://localhost:8080/api/v1/health >/dev/null 2>&1; do sleep 2; done

# 3. Enroll the first owner to establish the root of trust and create the CLI mTLS identity.
./g8e auth enroll user -e localhost

# 4. Start the platform workloads in the bootstrapped profile.
docker compose --profile bootstrapped up -d

# 5. List pending platform enrollment requests submitted by the workloads.
./g8e auth pending-platform-enrollments

# 6. Approve each request by exact request ID (recommended order: operator, dashboard, ensemble).
./g8e auth approve-platform-enrollment <operator-request-id> --yes
./g8e auth approve-platform-enrollment <dashboard-request-id> --yes
./g8e auth approve-platform-enrollment <ensemble-request-id> --yes

# 7. Verify that all workloads have completed enrollment and are healthy.
docker compose ps
# Or:
# ./g8e docker status
```

Alternatively, running `./g8e docker start --full` automates steps 1 through 6 via an interactive walkthrough that enrolls the owner and prompts for approval of each pending component. The gateway console at `https://localhost:8443/console/` also supports listing and approving pending platform enrollment requests in a browser.

The approval commands operate on request IDs only. They never print or accept requester tokens, token hashes, CSR PEM, or certificates. Pending discovery and approval both go through authenticated HTTPS using the CLI mTLS identity created during owner enrollment.

## Headless Gateway-Only Deployment

To run only the gateway without starting the platform workloads, start the default compose profile and enroll the owner using the `--headless` flag:

```bash
docker compose up -d --build
./g8e auth enroll user --headless -e localhost
```

Because `g8e-operator`, `ensemble`, and `dashboard` belong to the `bootstrapped` profile, running `docker compose up -d` without `--profile bootstrapped` starts only `g8e-gateway`. The `--headless` flag on `auth enroll user` completes user bootstrap while skipping the interactive browser-based passkey ceremony, making it suitable for CI, headless servers, and automation environments.

## Demo Environments

Functional demo environments are located in the `demos/` directory. These configurations demonstrate multi-network isolation and specialized doctrine enforcement:

- **Healthcare**: FHIR R4 compliance and PHI protection (`demos/healthcare/`).
- **Finance**: High-integrity ledger and audit requirements (`demos/finance/`).
- **DHS**: Persistent sovereign capability for coalition data handling with USPER PII minimization, cross-domain release control, and consensus posture (`demos/dhs/`).
- **FedRAMP**: Sovereign cloud governance with audit integrity, access control, cross-domain protection, and human-in-the-loop resource destruction approval (`demos/fedramp/`).

To run a demo:

```bash
cd demos/healthcare
docker compose up -d --build
```

Each demo uses its own `compose.yml` file with isolated networks (`net_untrusted`, `net_perimeter`, `net_internal`, `net_secure`, `net_mgmt`), volumes, and doctrine bind mounts. Demos build from the repo-root `Dockerfile` via `context: ../..` in each compose `build` stanza, compiling from source in-container without requiring a pre-built binary. See [demos/README.md](../../demos/README.md) for the full demo environment guide.

Demo stacks follow the same owner-approved platform enrollment bootstrap flow as the root compose file. After starting a demo stack, enroll the first owner against the demo gateway and approve the operator's pending request before dependent services become ready. The `g8e demos start` and `g8e demos run` commands print exact bootstrap instructions, including the demo gateway port and approval commands.

### Consensus Bootstrap Files

Demos that use consensus or notary posture (DHS, FedRAMP) require a bootstrap configuration file:

- **`consensus-bootstrap.json`**: Defines the consensus policy, member app IDs, quorum threshold, and cryptographic seeds for member signing keys. The file supports two key modes:
  - **`member_seeds`** (preferred): A map of member app IDs to individual hex-encoded Ed25519 seeds. Each member receives its own key pair, ensuring a single compromised key cannot satisfy a multi-member quorum.
  - **`seed_hex`** (legacy fallback): A single hex-encoded Ed25519 seed shared across all members. When `member_seeds` is omitted but `seed_hex` is provided, the same key is registered for every member.

When `member_seeds` is present, it takes precedence over `seed_hex`. If both are omitted, a fresh key pair is generated and shared across members.

The gateway container mounts `consensus-bootstrap.json` to seed the ConsensusPolicy and trusted signer registry at startup:

```yaml
# gateway
volumes:
  - ./config/consensus-bootstrap.json:/etc/g8e/consensus-bootstrap.json:ro
```

```yaml
# agent
volumes:
  - ./config/consensus-bootstrap.json:/etc/g8e/consensus-bootstrap.json:ro
```

When `member_seeds` is used, each member signs votes with its own distinct key, and the gateway verifies each signature against the registered public key.

### Gateway Posture in Demos

The DHS and FedRAMP demos boot the gateway in `consensus` posture via the `G8E_GATEWAY_POSTURE` environment variable (default: `consensus`) and maintain it for the entire run. Scenarios execute under this posture without restarting or recreating the gateway mid-demo.

## Configuration

### Port Mapping

The gateway exposes two primary ports:
- **8080**: HTTP for bootstrap discovery, health checks, CA bundle download, and catch-all redirect to HTTPS.
- **8443**: HTTPS for MCP, A2A, governance envelopes, document store, WebSocket pub/sub, and the console SPA.

Custom ports are configured via CLI flags or environment variables:

**CLI Example:**
```bash
docker run -d \
  g8e-gateway:latest \
  gw start -f --posture doctrine --http-port 3000 --https-port 3443
```

**Compose Example:**
```yaml
services:
  g8e-gateway:
    ports:
      - "3000:8080"
      - "3443:8443"
    command: ["gw", "start", "-f", "--posture", "consensus"]
```

Container ports remain 8080 and 8443; only the host-side mapping changes.

### Data Persistence

The gateway maintains state in `/root/.g8e` within the container. This directory contains:
- **`data/`**: SQLite database.
- **`pki/`**: TLS certificates and trust bundles.
- **`secrets/`**: Platform secret keys and vault key.
- **`vault/`**: Encrypted storage for sensitive materials.

Mount a persistent volume to preserve state across container lifecycles:

```bash
docker run -v g8e-data:/root/.g8e g8e-gateway:latest gw start -f --posture doctrine
```

### Host Identity Bind Mounts

So the gateway serving certificate covers the host's real IPs and hostname when the gateway runs in a container, the root `docker-compose.yml` and each demo's `compose.yml` bind-mount the host's `/etc/hosts` and `/etc/hostname` read-only into the container at `/etc/hosts.host` and `/etc/hostname.host`:

```yaml
volumes:
  - /etc/hosts:/etc/hosts.host:ro
  - /etc/hostname:/etc/hostname.host:ro
```

The network identity detector reads these host-mounted files before the container's own `/etc/hosts` and `/etc/hostname`, de-duplicating IPs and aliases across both sources. The serving certificate is regenerated on startup when SAN drift is detected (expected IPs or DNS names missing from the existing certificate). See [Network Architecture](../architecture/network.md#8-network-identity-detection) for the detection pipeline.

### Governance Posture

Specify the security posture at startup using the `--posture` flag:

- **`--posture doctrine`**: L1 enforced; L2/L3 audited.
- **`--posture consensus`**: L1/L2 enforced; L3 audited.
- **`--posture ratify`**: L1/L3 enforced; L2 audited.
- **`--posture notary`**: L1/L2/L3 strictly enforced.

## Architecture

### Multi-Stage Build

The `Dockerfile` employs a multi-stage build process:

1. **Build Stage**: Uses `golang:1.26.6` to compile platform binaries via `make build-all`.
2. **Runtime Stage**: Uses pinned Debian Bookworm (`debian@sha256:30482e873082e906a4908c10529180aefb6f77620aea7404b909829fadc5d168`) for the execution environment.

The image includes:
- The `g8e` binary at `/g8e`.
- All platform binaries at `/opt/g8e/bin/` for node deployment via `/.well-known/g8e/bin/`.
- Protocol constants at `/protocol/constants`.
- Reference data at `/docs/reference` (KSI catalog and COSAiS overlays for the compliance CLI).
- Required utilities including `curl`, `wget`, and `ca-certificates`.

The build links the Go Cryptographic Module v1.0.0 (CMVP Cert #5247) and enters FIPS 140-3 approved mode on startup for linux/amd64 targets. Runtime strict enforcement is configured via `GODEBUG=fips140=only`.

Protocol constants are bundled from the `protocol/` directory. The Python protocol package (`g8e`) can be installed separately:

```bash
pip install g8e==2.1.4
```

Or configure `G8E_PROTOCOL_DIR=/protocol` to direct the Python package to bundled constants. See [Protocol Library](../architecture/protocol.md) for details.

### Health Monitoring

The `Dockerfile` does not define an image-level health check because the same image serves as both the gateway (listening on HTTP/HTTPS) and the operator (an outbound-only client). Health checks are declared per-service in `docker-compose.yml`, where each service expresses its own readiness signal.

Inspect container health status with:

```bash
docker inspect --format='{{.State.Health.Status}}' g8e-gateway
docker inspect --format='{{.State.Health.Status}}' g8e-operator
docker inspect --format='{{.State.Health.Status}}' g8e-ensemble
docker inspect --format='{{.State.Health.Status}}' g8e-dashboard
```

## Troubleshooting

### Log Inspection

```bash
# Gateway logs
docker logs g8e-gateway

# Full compose logs
docker compose logs -f

# Follow logs via CLI
./g8e docker logs -f
```

### Service Status

Verify the internal state of the gateway:

```bash
docker exec g8e-gateway /g8e gw status
```

Or check container status with the CLI:

```bash
./g8e docker status
```

## Production Considerations

- **Base Image**: The runtime image uses Debian Bookworm slim. Maintain regular updates to the base image for security patches.
- **Resource Constraints**: The root `docker-compose.yml` defines per-service CPU and memory limits (see Resource Limits above). Adjust these for production workloads.
- **Vault Management**: The vault auto-initializes on first run with a generated key. Use `G8E_VAULT_KEY` to override the default key file path, or `--vault-key` to specify it via CLI. The vault must be unlocked at startup; if the key cannot be read, the gateway fails to start.
- **Certificates**: By default, the gateway generates self-signed certificates. For production, provide valid certificates via the `--pki-dir` volume or use `--cert-mode full` with appropriate hostnames.
- **Operator Connectivity**: The operator connects to the gateway via `g8e.local`, resolved through the Docker network alias on the shared bridge network. Ensure this hostname matches the gateway certificate SANs in production deployments. On first start with no installed credentials, the operator performs owner-approved platform enrollment against this endpoint and waits for approval before it becomes ready. See Owner-Approved Platform Bootstrap above.
