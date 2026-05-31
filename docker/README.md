# g8e Docker Setup

This directory contains Docker configurations for running the g8e platform in containers.

## Components

- **Dockerfile.gateway**: Builds the g8e gateway (Policy Decision Point) in gateway mode with L1 enforcement, L2/L3 auditing
- **Dockerfile.operator**: Builds the g8e operator (Policy Execution Point) in listen mode
- **docker-compose.yml**: Orchestrates gateway and operator services with proper networking and dependencies

## Quick Start

### Build the g8e Binary First

The Dockerfiles copy the pre-built binary from the host. Build it first:

```bash
make build
```

### Build and Start Services

```bash
cd docker
docker-compose up -d
```

This will:
1. Build both gateway and operator images (copying the pre-built binary)
2. Start the gateway service on ports 9000-9002
3. Start the operator service on ports 9010-9012 (offset to avoid conflicts)
4. Create persistent volumes for data, PKI, and secrets

### Service Endpoints

**Gateway:**
- HTTPS mTLS API: `https://localhost:9000`
- Bootstrap TLS (CSR enrollment): `https://localhost:9001`
- Public browser/BYO bootstrap: `https://localhost:9002`

**Operator:**
- HTTPS mTLS API: `https://localhost:9010`
- Bootstrap TLS (CSR enrollment): `https://localhost:9011`
- Public browser/BYO bootstrap: `https://localhost:9012`

### Authentication Flow

1. Gateway starts first and becomes healthy
2. Operator starts and depends on gateway health
3. Operator authenticates with gateway via `G8E_GATEWAY_ENDPOINT=https://gateway:9000`

### View Logs

```bash
# All services
docker-compose logs -f

# Specific service
docker-compose logs -f gateway
docker-compose logs -f operator
```

### Stop Services

```bash
docker-compose down
```

To also remove volumes:
```bash
docker-compose down -v
```

## Configuration

Environment variables can be customized in `docker-compose.yml`:

**Gateway:**
- `G8E_LOG_LEVEL`: Log level (default: info)
- `G8E_GATEWAY_HTTP_PORT`: HTTPS mTLS API port (default: 9000)
- `G8E_GATEWAY_BOOTSTRAP_PORT`: Bootstrap TLS port (default: 9001)
- `G8E_GATEWAY_PUBLIC_PORT`: Public bootstrap port (default: 9002)
- `G8E_DATA_DIR`: Data directory (default: /g8e/.g8e/data)
- `G8E_PKI_DIR`: PKI directory (default: /g8e/.g8e/pki)
- `G8E_SECRETS_DIR`: Secrets directory (default: /g8e/.g8e/secrets)

**Operator:**
- `G8E_LOG_LEVEL`: Log level (default: info)
- `G8E_LISTEN_HTTP_PORT`: HTTPS mTLS API port (default: 9000)
- `G8E_LISTEN_BOOTSTRAP_PORT`: Bootstrap TLS port (default: 9001)
- `G8E_LISTEN_PUBLIC_PORT`: Public bootstrap port (default: 9002)
- `G8E_DATA_DIR`: Data directory (default: /g8e/.g8e/data)
- `G8E_PKI_DIR`: PKI directory (default: /g8e/.g8e/pki)
- `G8E_SECRETS_DIR`: Secrets directory (default: /g8e/.g8e/secrets)
- `G8E_GATEWAY_ENDPOINT`: Gateway endpoint for authentication (default: https://gateway:9000)

## Gateway Postures

The gateway runs in `--doctrine` mode by default (L1 enforced, L2/L3 audited). To change the posture, modify the `ENTRYPOINT` in `Dockerfile.gateway`:

- `--doctrine`: L1 enforced, L2/L3 audited (default)
- `--consensus`: L1/L2 enforced, L3 audited
- `--notary`: L1/L2/L3 strictly enforced

## Building Images Individually

First build the binary on the host:

```bash
make build
```

Then build the Docker images:

```bash
# Build gateway image
docker build -f docker/Dockerfile.gateway -t g8e-gateway:latest ..

# Build operator image
docker build -f docker/Dockerfile.operator -t g8e-operator:latest ..
```

## Development

For development with live code changes, mount the source directory:

```yaml
volumes:
  - ../:/build
```

Then rebuild the binary inside the container or use a development-focused compose override.

## Security Notes

- Both services run as non-root user (g8e:1000)
- mTLS is required for API communication
- PKI material and secrets are stored in Docker volumes
- Health checks ensure services are ready before accepting traffic
