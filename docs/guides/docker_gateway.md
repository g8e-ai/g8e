# Docker Gateway Guide

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
  --doctrine
```

## Docker Compose Deployment

The repository includes a root `docker-compose.yml` that deploys both the Gateway and an Operator for testing purposes. Both services utilize the same `Dockerfile`.

### Core Services

- **g8e-gateway**: Provides the persistence layer and governance enforcement.
- **g8e-operator**: Connects to the gateway to provide execution capabilities.

### Execution

Start the services from the repository root:

```bash
docker compose up -d
```

### Demo Environments

Functional demo environments are located in the `demos/` directory. These configurations demonstrate multi-network isolation and specialized doctrine enforcement:

- **Healthcare**: FHIR R4 compliance and PHI protection.
- **Government**: Public sector data residency and access control.
- **Finance**: High-integrity ledger and audit requirements.

To run a demo:

```bash
cd demos/healthcare
docker compose up -d
```

## Configuration

### Port Mapping

The gateway exposes two primary ports:
- **8080**: HTTP for bootstrap, MCP, and health checks.
- **8443**: HTTPS for mTLS API and administrative interface.

Custom ports are configured via CLI flags or environment variables:

**CLI Example:**
```bash
docker run -d \
  g8e-gateway:latest \
  --doctrine --http-port 3000 --https-port 3443
```

**Compose Example:**
```yaml
services:
  gateway:
    ports:
      - "3000:3000"
      - "3443:3443"
    command: ["--doctrine", "--http-port", "3000", "--https-port", "3443"]
```

### Data Persistence

The gateway maintains state in `/root/.g8e` within the container. This directory contains:
- **`data/`**: SQLite database.
- **`pki/`**: TLS certificates and trust bundles.
- **`secrets/`**: Platform secret keys.
- **`vault/`**: Encrypted storage for sensitive materials.

Mount a persistent volume to preserve state across container lifecycles:

```bash
docker run -v g8e-data:/root/.g8e g8e-gateway:latest --doctrine
```

### Governance Posture

Specify the security posture at startup using a mutually exclusive flag:

- **`--doctrine`**: L1 enforced; L2/L3 audited.
- **`--consensus`**: L1/L2 enforced; L3 audited.
- **`--notary`**: L1/L2/L3 strictly enforced.

## Architecture

### Multi-Stage Build

The `Dockerfile` employs a multi-stage build process:

1. **Build Stage**: Utilizes `golang:1.26` to compile the `g8e` binary from `cmd/operator`.
2. **Runtime Stage**: Utilizes `debian:bookworm` for the execution environment.

The image includes:
- The `g8e` binary at `/g8e`.
- Protocol constants at `/protocol/constants`.
- Required utilities including `curl`, `wget`, and `ca-certificates`.

### Health Monitoring

The container defines a health check that queries the `/api/v1/health` endpoint:

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
- **Resource Constraints**: Define CPU and memory limits in production compose files to prevent resource exhaustion.
- **Vault Management**: The vault must be initialized for production operations. Use `G8E_VAULT_REQUIRE_UNLOCK=true` to ensure the gateway only starts when the vault is available.
- **Certificates**: By default, the gateway generates self-signed certificates. For production, provide valid certificates via the `--pki-dir` volume or use `--cert-mode full` with appropriate hostnames.
