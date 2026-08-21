# Docker Gateway Guide

Last Updated: 2026-08-20
Version: v1.7.7

This document describes the procedures for building and deploying the g8e Gateway using Docker and Docker Compose.

## Quick Start

### Build Image

Build the g8e container image from the repository root:

```bash
docker build -t g8e-gateway:latest .
```

### Run Container

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

The `-f` flag runs the gateway in the foreground instead of spawning a background subprocess. This is required for container usage so the process does not exit immediately. The root `docker-compose.yml` also uses `gw start -f`.

## Docker Compose Deployment

The repository includes a root `docker-compose.yml` that deploys the full platform stack: the Gateway, the Dashboard, the Ensemble, and the Operator. The gateway and operator use the same `Dockerfile`; the dashboard and ensemble have their own images built from `dashboard/Dockerfile` and `ensemble/Dockerfile`. All four services are unprofiled and start together as the full-stack default. The platform workloads (operator, dashboard, ensemble) start in a not-ready state and remain not-ready until their owner-approved platform enrollment requests are approved. There is no `activated` Compose profile; full-stack startup is the default.

### Core Services

- **g8e-gateway**: Provides the persistence layer and governance enforcement. Starts via the `gw start -f` CLI subcommand, which defaults to `doctrine` posture. The gateway starts with zero users and issues no platform certificates until the first owner enrolls and approves pending enrollment requests.
- **g8e-operator**: Connects to the gateway to provide execution capabilities. Uses the `operator start -e g8e.local` command. The gateway registers `g8e.local` as a network alias on the shared `g8e-net` bridge network, so the operator resolves the gateway by the hostname matching its certificate SANs. On first start with no installed credentials, the operator submits a platform enrollment request and waits for owner approval before it becomes ready.
- **ensemble**: The Python FastAPI ensemble service. Submits a platform enrollment request at startup via the owner-approved platform enrollment protocol and waits for owner approval before its lifespan completes. Connects to the gateway document store, pub/sub, and blob APIs over mTLS using its enrolled credentials.
- **dashboard**: The Node.js Express dashboard service. Submits a platform enrollment request at startup via the owner-approved platform enrollment protocol and waits for owner approval before its Express server listens. The browser still authenticates to the gateway via WebAuthn passkeys; the dashboard's enrolled credential is for its server-to-server mTLS calls.

The operator, ensemble, and dashboard services each depend on the gateway health check passing before starting (`depends_on` with `condition: service_healthy` for the gateway). Their own healthchecks remain truthful: the operator checks `operator.crt` existence, the ensemble probes its FastAPI `/health` endpoint, and the dashboard probes its Express `/` endpoint. Because enrollment is not complete until the owner approves, these healthchecks stay not-ready while approval is pending. `docker compose up --wait` without prior owner approval is expected to wait or time out; this is correct behavior, not a bug. Do not weaken healthchecks to liveness checks to work around it.

### Environment Variables

The compose file supports the following environment variable overrides. A `.env.example` file at the repository root documents all six with defaults and one-line descriptions; copy it to `.env` and edit, or set the variables inline.

- **`G8E_PREFIX`**: Prefix for container names (default: `g8e`). Containers are named `${G8E_PREFIX}-gateway`, `${G8E_PREFIX}-operator`, `${G8E_PREFIX}-ensemble`, and `${G8E_PREFIX}-dashboard`.
- **`G8E_HTTP_PORT`**: Host port mapped to the gateway HTTP discovery surface, container port 8080 (default: `8080`). Used for health checks, CA bundle fetch, and platform enrollment submission.
- **`G8E_HTTPS_PORT`**: Host port mapped to the gateway HTTPS/mTLS surface, container port 8443 (default: `8443`). Used for MCP, A2A, governance envelopes, document store, WebSocket pub/sub, and the console SPA.
- **`G8E_ENSEMBLE_PORT`**: Host port mapped to the ensemble FastAPI API, container port 8000 (default: `8000`).
- **`G8E_DASHBOARD_PORT`**: Host port mapped to the dashboard Express server, container port 3000 (default: `3000`).
- **`G8E_HOSTNAME`**: Public hostname the gateway advertises in approval links, host-header validation, CORS, and passkey RP origins (default: `localhost`). Set to a real hostname (e.g. `dev.g8e.local`) when the browser reaches the gateway via that hostname instead of localhost.

### Resource Limits

All four services define CPU and memory constraints in the compose file:

| Service | CPU limit | Memory limit | CPU reservation | Memory reservation |
| --- | --- | --- | --- | --- |
| `g8e-gateway` | 2 | 1G | 0.5 | 256M |
| `g8e-operator` | 2 | 1G | 0.5 | 256M |
| `ensemble` | 2 | 2G | 0.5 | 512M |
| `dashboard` | 1 | 512M | 0.25 | 128M |

These limits document expected resource needs for the development stack. Adjust them for production workloads or move them to a production override file (`docker-compose.prod.yml`) if a slimmer root compose file is preferred.

### Execution

Start the full stack from the repository root:

```bash
docker compose up -d --build
```

To use custom ports or a container name prefix:

```bash
G8E_HTTP_PORT=3000 G8E_HTTPS_PORT=3443 G8E_PREFIX=myg8e docker compose up -d --build
```

All four containers start, but the operator, dashboard, and ensemble remain not-ready until their platform enrollment requests are approved. Do not use `docker compose up --wait` before approval; that command waits for all services to become healthy and is expected to time out while enrollment is pending. Wait for the gateway health endpoint only, then complete the activation flow below.

### Owner-Approved Platform Activation

After `docker compose up -d --build`, the gateway is healthy but the platform workloads are not. Activate them by enrolling the first owner and approving each pending enrollment request:

```bash
# 1. Wait for the gateway to be healthy (poll the health endpoint).
until curl -fsS http://localhost:8080/api/v1/health >/dev/null 2>&1; do sleep 2; done

# 2. Enroll the first owner. This creates the first user and a usable CLI mTLS identity.
#    -e is the gateway HTTP discovery endpoint (host or host:port); the coordinator
#    derives the HTTP port. --port defaults to 8443 for mTLS and does not need to be set.
./g8e auth enroll user -e localhost

# 3. List pending platform enrollment requests (authenticated via the CLI mTLS identity).
./g8e auth pending-platform-enrollments

# 4. Approve each request by exact request ID. The recommended order is operator, dashboard, ensemble.
./g8e auth approve-platform-enrollment <operator-request-id> --yes
./g8e auth approve-platform-enrollment <dashboard-request-id> --yes
./g8e auth approve-platform-enrollment <ensemble-request-id> --yes

# 5. Wait for the workloads to become healthy.
docker compose ps
```

The operator → dashboard → ensemble order is an operational recommendation, not a security invariant. The gateway does not enforce a prerequisite ordering between component approvals. If strict ordering becomes a product requirement, add explicit singleton activation records and reject out-of-order decisions; do not rely on documentation or compose timing.

The approval commands operate on request IDs only. They never print or accept requester tokens, token hashes, CSR PEM, or certificates. Pending discovery and approval both go through authenticated HTTPS using the CLI mTLS identity created in step 2.

Alternatively, use the gateway console at `https://localhost:8443/console/` to list pending requests and approve them in a browser. The console signs in via the same first-owner WebAuthn passkey created in step 2.

### Headless Gateway-Only Deployment

To start only the gateway while preserving the full-stack default behavior, select the gateway service explicitly:

```bash
docker compose up -d --build --no-deps g8e-gateway
./g8e auth enroll user --headless -e localhost
```

`--no-deps g8e-gateway` starts only the gateway service without its dependents. Docker Compose profiles are additive and cannot exclude unprofiled services that start by default, so headless deployment uses explicit service selection rather than a `headless` profile. The `--headless` flag on `auth enroll user` skips the browser-based passkey flow and is suitable for SSH or automation environments.

### Demo Environments

Functional demo environments are located in the `demos/` directory. These configurations demonstrate multi-network isolation and specialized doctrine enforcement:

- **Healthcare**: FHIR R4 compliance and PHI protection.
- **Finance**: High-integrity ledger and audit requirements.
- **DHS**: Persistent sovereign capability for coalition data handling with USPER PII minimization, cross-domain release control, and consensus posture.
- **FedRAMP**: Sovereign cloud governance with audit integrity, access control, cross-domain protection, and human-in-the-loop resource destruction approval.
- **Frontend**: Third-party frontend application enrollment with WebAuthn passkey authentication, CORS, and SSE live event streaming.

To run a demo:

```bash
cd demos/healthcare
docker compose up -d --build
```

Each demo uses its own `compose.yml` file with isolated networks, volumes, and doctrine bind mounts. Demos build from the repo-root `Dockerfile` (the same production image, always-FIPS) via `context: ../..` in each compose `build` stanza; the image compiles from source in-container, so no pre-built binary is required. See [demos/README.md](../../demos/README.md) for the full demo environment guide.

Demo stacks use the same owner-approved platform enrollment activation flow as the root compose file. The operator and any services that depend on it start unprofiled and remain not-ready until the operator's platform enrollment request is approved. Services with `depends_on: operator: condition: service_healthy` wait for the operator healthcheck, which stays not-ready until the operator is enrolled. After `docker compose up -d --build`, enroll the first owner against the demo gateway and approve the operator's pending request before the dependent services can become ready. The `g8e demos start` and `g8e demos run` commands print the exact activation instructions, including the demo gateway port and the `g8e auth approve-platform-enrollment <request-id>` command to run.

### Consensus Bootstrap Files

Demos that use consensus or notary posture (DHS, FedRAMP) require a bootstrap config file:

- **`consensus-bootstrap.json`**: Defines the consensus policy, member app IDs, quorum threshold, and cryptographic seeds for member signing keys. The file supports two key modes:
  - **`member_seeds`** (preferred): A map of member app IDs to individual hex-encoded Ed25519 seeds. Each member gets its own key pair, so a single compromised key cannot satisfy a multi-member quorum.
  - **`seed_hex`** (legacy fallback): A single hex-encoded Ed25519 seed shared across all members. When `member_seeds` is omitted but `seed_hex` is provided, the same key is registered for every member.

When `member_seeds` is present, it takes precedence over `seed_hex`. If both are omitted, a fresh key pair is generated and shared across members.

The gateway container mounts `consensus-bootstrap.json`, which seeds the ConsensusPolicy and trusted signer registry at startup. The agent container also mounts `consensus-bootstrap.json` to reconstruct the per-member signing keys for L2 votes:

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

When `member_seeds` is used, each member signs votes with its own distinct key, and the gateway verifies each signature against the registered public key. This makes Byzantine quorum meaningful: a single compromised key cannot forge enough votes to meet quorum.

If the agent does not mount `consensus-bootstrap.json`, votes are signed with a default key ID that does not match the gateway's registered members. The gateway rejects these votes, resulting in zero affirmative votes and quorum failure.

### Gateway Posture in Demos

The DHS and FedRAMP demos boot the gateway in `consensus` posture via the `G8E_GATEWAY_POSTURE` environment variable (default: `consensus`) and keep it there for the entire run. The gateway is not restarted or recreated mid-demo; all scenarios execute under the same posture. Notary-tier scenarios remain in the harness registry for manual testing against a separately started demo with a manually enrolled passkey, but the automated `g8e demos run` orchestration excludes them.

## Configuration

### Port Mapping

The gateway exposes two primary ports:
- **8080**: HTTP for bootstrap, health checks, and catch-all redirect to HTTPS.
- **8443**: HTTPS for MCP, A2A, governance envelopes, document store, WebSocket pub/sub, and console SPA.

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
  gateway:
    ports:
      - "3000:8080"
      - "3443:8443"
    command: ["gw", "start", "-f", "--posture", "consensus"]
```

The `gw start` subcommand accepts `--posture`, `--http-port`, and `--https-port` flags. The `-f` flag runs the gateway in the foreground instead of the background, which is required for container usage. Container ports remain 8080 and 8443; only the host-side mapping changes.

### Data Persistence

The gateway maintains state in `/root/.g8e` within the container. This directory contains:
- **`data/`**: SQLite database.
- **`pki/`**: TLS certificates and trust bundles.
- **`secrets/`**: Platform secret keys.
- **`vault/`**: Encrypted storage for sensitive materials.

Mount a persistent volume to preserve state across container lifecycles:

```bash
docker run -v g8e-data:/root/.g8e g8e-gateway:latest gw start -f --posture doctrine
```

### Governance Posture

Specify the security posture at startup using the `--posture` flag. Three values are valid:

- **`--posture doctrine`**: L1 enforced; L2/L3 audited.
- **`--posture consensus`**: L1/L2 enforced; L3 audited.
- **`--posture notary`**: L1/L2/L3 strictly enforced.

## Architecture

### Multi-Stage Build

The `Dockerfile` employs a multi-stage build process:

1. **Build Stage**: Uses a pinned Go toolchain image to compile the `g8e` binary.
2. **Runtime Stage**: Uses a pinned Debian Bookworm image for the execution environment.

The image includes:
- The `g8e` binary at `/g8e`.
- Protocol constants at `/protocol/constants`.
- Reference data at `/docs/reference` (KSI catalog and COSAiS overlays for the compliance CLI).
- Required utilities including `curl`, `wget`, and `ca-certificates`.

The build always links the Go Cryptographic Module v1.0.0 (CMVP Cert #5247) and enters FIPS 140-3 approved mode on startup. Enforcement is a runtime setting, off by default: non-approved primitives such as Ed25519 and ChaCha20-Poly1305 still work. Operators who need strict enforcement set `GODEBUG=fips140=only` in the container environment. The FIPS compliance claim is restricted to `linux/amd64`, which the Dockerfile hardcodes.

The protocol constants are bundled from the `protocol/` directory. The Python package (`g8e`) is not included in the Docker image by default. If you need Python protocol access inside a container, install it separately:

```bash
pip install g8e==1.7.5
```

Or set `G8E_PROTOCOL_DIR=/protocol` to point the Python package at the bundled constants. See the [Protocol Library documentation](../architecture/protocol.md) for details.

### Health Monitoring

The `Dockerfile` does not define an image-level health check. The same image runs as both the gateway (an HTTP server on port 8080) and the operator (an outbound client that listens on nothing), so a baked-in health check would always fail for the operator. Health checks are declared per-service in `docker-compose.yml`, where each service expresses its own readiness signal. The root compose file defines a `wget` probe against the `/api/v1/health` endpoint for the gateway service, an `operator.crt` existence check for the operator, a FastAPI `/health` probe for the ensemble, and an Express `/` probe for the dashboard.

The operator, ensemble, and dashboard healthchecks are readiness checks, not liveness checks. They stay not-ready while platform enrollment is pending and become healthy only after the owner approves the enrollment request and the component completes enrollment. This is truthful readiness: `docker compose ps` shows the workloads as unhealthy until activation is complete.

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

# Compose logs
docker compose logs -f
```

### Service Status

Verify the internal state of the gateway:

```bash
docker exec g8e-gateway /g8e gw status
```

## Production Considerations

- **Base Image**: The image uses Debian Bookworm. Maintain regular updates to the base image for security patches.
- **Resource Constraints**: The root `docker-compose.yml` defines per-service CPU and memory limits (see the Resource Limits section above). Adjust these for production workloads.
- **Vault Management**: The vault auto-initializes on first run with a generated key. Use `G8E_VAULT_KEY` to override the default key file path, or `--vault-key` to specify it via CLI. The vault must be unlocked at startup; if the key cannot be read, the gateway fails to start.
- **Certificates**: By default, the gateway generates self-signed certificates. For production, provide valid certificates via the `--pki-dir` volume or use `--cert-mode full` with appropriate hostnames.
- **Operator Connectivity**: The operator connects to the gateway via `g8e.local`, resolved through the Docker network alias on the shared bridge network. Ensure this hostname matches the gateway certificate SANs in production deployments. On first start with no installed credentials, the operator performs owner-approved platform enrollment against this endpoint and waits for approval before it becomes ready. See the Owner-Approved Platform Activation section above.
