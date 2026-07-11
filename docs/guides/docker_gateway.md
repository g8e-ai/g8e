# Docker Gateway Guide

Last Updated: 2026-07-08
Version: v1.3.8

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
  gw serve --posture doctrine
```

The `gw serve` subcommand runs the gateway in foreground mode, blocking until shutdown. It is a hidden subcommand that serves as the re-exec target for `gw start`. For direct `docker run` usage, `gw serve` is suitable because it runs the gateway in the foreground without spawning a subprocess. The root `docker-compose.yml` uses `gw start -f` instead, which runs the gateway in the foreground directly (same process, no subprocess). Both approaches keep the container running.

## Docker Compose Deployment

The repository includes a root `docker-compose.yml` that deploys both the Gateway and an Operator for testing purposes. Both services utilize the same `Dockerfile`.

### Core Services

- **g8e-gateway**: Provides the persistence layer and governance enforcement. Starts via the `gw start -f` CLI subcommand, which defaults to `doctrine` posture.
- **g8e-operator**: Connects to the gateway to provide execution capabilities. Uses the `operator start -e g8e.local` command with `extra_hosts` mapping `g8e.local` to the host gateway, ensuring the operator resolves the gateway by the hostname matching its certificate SANs.

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
- **Government**: Public sector data residency and access control.
- **Finance**: High-integrity ledger and audit requirements.
- **Secure Data**: Governed data migration with a two-operator pipeline and chain-of-custody receipts.
- **Department of War (DoW)**: Tactical edge operations with SWaP-constrained sensors, BFT spoofing defense, and autonomous SIGINT-to-EO/IR cross-cueing.
- **Swarm**: Drone swarm simulation with 20 autonomous operators, battlefield intelligence, and doctrine-based weapons control.
- **DHS**: Persistent sovereign capability for coalition data handling with USPER PII minimization, cross-domain release control, and consensus posture.

To run a demo:

```bash
cd demos/healthcare
docker compose up -d
```

Each demo uses its own `compose.yml` file with isolated networks, volumes, and doctrine bind mounts. Demos use a separate `demos/Dockerfile` that copies a pre-built binary into a minimal Debian image without compilation. Run `make build` first to produce the binary. See [demos/README.md](../../demos/README.md) for the full demo environment guide.

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
  gw serve --posture doctrine --http-port 3000 --https-port 3443
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

The `gw start` and `gw serve` subcommands accept the same `--posture`, `--http-port`, and `--https-port` flags. The `-f` flag (available only on `gw start`) runs the gateway in the foreground instead of the background — Ctrl+C stops the gateway directly. Container ports remain 8080 and 8443; only the host-side mapping changes.

### Data Persistence

The gateway maintains state in `/root/.g8e` within the container. This directory contains:
- **`data/`**: SQLite database.
- **`pki/`**: TLS certificates and trust bundles.
- **`secrets/`**: Platform secret keys.
- **`vault/`**: Encrypted storage for sensitive materials.

Mount a persistent volume to preserve state across container lifecycles:

```bash
docker run -v g8e-data:/root/.g8e g8e-gateway:latest gw serve --posture doctrine
```

### Governance Posture

Specify the security posture at startup using the `--posture` flag. Three values are valid:

- **`--posture doctrine`**: L1 enforced; L2/L3 audited.
- **`--posture consensus`**: L1/L2 enforced; L3 audited.
- **`--posture notary`**: L1/L2/L3 strictly enforced.

## Architecture

### Multi-Stage Build

The `Dockerfile` employs a multi-stage build process:

1. **Build Stage**: Uses a pinned Go toolchain image to compile the `g8e` binary from `cmd/g8e`.
2. **Runtime Stage**: Uses a pinned Debian Bookworm image for the execution environment.

The image includes:
- The `g8e` binary at `/g8e`.
- Protocol constants at `/protocol/constants`.
- Required utilities including `curl`, `wget`, and `ca-certificates`.

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
- **Vault Management**: The vault must be initialized for production operations. Use `G8E_VAULT_REQUIRE_UNLOCK=true` to ensure the gateway only starts when the vault is available.
- **Certificates**: By default, the gateway generates self-signed certificates. For production, provide valid certificates via the `--pki-dir` volume or use `--cert-mode full` with appropriate hostnames.
- **Operator Connectivity**: The operator connects to the gateway via `g8e.local`, resolved through Docker `extra_hosts` mapping to `host-gateway`. Ensure this hostname matches the gateway certificate SANs in production deployments.
