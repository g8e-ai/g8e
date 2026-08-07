# Docker Gateway Guide

Last Updated: 2026-08-07
Version: v1.7.0

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

The repository includes a root `docker-compose.yml` that deploys both the Gateway and an Operator for testing purposes. Both services utilize the same `Dockerfile`.

### Core Services

- **g8e-gateway**: Provides the persistence layer and governance enforcement. Starts via the `gw start -f` CLI subcommand, which defaults to `doctrine` posture.
- **g8e-operator**: Connects to the gateway to provide execution capabilities. Uses the `operator start -e g8e.local` command. The gateway registers `g8e.local` as a network alias on the shared `g8e-net` bridge network, so the operator resolves the gateway by the hostname matching its certificate SANs.

The operator service depends on the gateway health check passing before starting (`depends_on` with `condition: service_healthy`).

### Environment Variables

The compose file supports the following environment variable overrides:

- **`G8E_PREFIX`**: Prefix for container names (default: `g8e`). Containers are named `${G8E_PREFIX}-gateway` and `${G8E_PREFIX}-operator`.
- **`G8E_HTTP_PORT`**: Host port mapped to the gateway HTTP port 8080 (default: `8080`).
- **`G8E_HTTPS_PORT`**: Host port mapped to the gateway HTTPS port 8443 (default: `8443`).

### Resource Limits

Both services define CPU and memory constraints in the compose file:

- **Limits**: 2 CPUs, 1G memory per service.
- **Reservations**: 0.5 CPUs, 256M memory per service.

### Execution

Start the services from the repository root:

```bash
docker compose up -d
```

To use custom ports or a container name prefix:

```bash
G8E_HTTP_PORT=3000 G8E_HTTPS_PORT=3443 G8E_PREFIX=myg8e docker compose up -d
```

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
docker compose up -d
```

Each demo uses its own `compose.yml` file with isolated networks, volumes, and doctrine bind mounts. Demos use a separate `demos/Dockerfile` that copies a pre-built binary into a minimal Debian image without compilation. Run `make build` first to produce the binary. See [demos/README.md](../../demos/README.md) for the full demo environment guide.

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

### Posture Switching During Demos

The DHS and FedRAMP demos dynamically switch the gateway's posture mid-scenario. The gateway container is stopped and recreated with a new `G8E_GATEWAY_POSTURE` environment variable, then the demo waits for the gateway to become healthy before continuing.

This allows a single demo to test multiple posture modes (e.g., consensus, then notary, then consensus) without restarting the entire Docker Compose stack. Other services (operator, agent, actuators) remain running during the switch.

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
- Required utilities including `curl`, `wget`, and `ca-certificates`.

The protocol constants are bundled from the `protocol/` directory. The Python package (`g8e`) is not included in the Docker image by default. If you need Python protocol access inside a container, install it separately:

```bash
pip install g8e==1.6.10
```

Or set `G8E_PROTOCOL_DIR=/protocol` to point the Python package at the bundled constants. See the [Protocol Library documentation](../architecture/protocol.md) for details.

### Health Monitoring

The container defines a health check that queries the `/api/v1/health` endpoint. The `Dockerfile` uses `curl` for this check; the root `docker-compose.yml` overrides this with `wget`.

```bash
docker inspect --format='{{.State.Health.Status}}' g8e-gateway
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
- **Resource Constraints**: The root `docker-compose.yml` defines CPU and memory limits (2 CPUs / 1G per service with 0.5 CPU / 256M reservations). Adjust these for production workloads.
- **Vault Management**: The vault auto-initializes on first run with a generated key. Use `G8E_VAULT_KEY` to override the default key file path, or `--vault-key` to specify it via CLI. The vault must be unlocked at startup; if the key cannot be read, the gateway fails to start.
- **Certificates**: By default, the gateway generates self-signed certificates. For production, provide valid certificates via the `--pki-dir` volume or use `--cert-mode full` with appropriate hostnames.
- **Operator Connectivity**: The operator connects to the gateway via `g8e.local`, resolved through the Docker network alias on the shared bridge network. Ensure this hostname matches the gateway certificate SANs in production deployments.
