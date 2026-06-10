# Docker Gateway Guide

This guide explains how to build and run the g8e Gateway using Docker.

## Quick Start

### Build the Docker Image

```bash
docker build -t g8e-gateway:latest .
```

### Run the Container

```bash
docker run -d \
  --name g8e-gateway \
  -p 8080:8080 \
  -p 8443:8443 \
  -v g8e-data:/root/.g8e \
  g8e-gateway:latest \
  gw start --posture doctrine
```

### Using Docker Compose

The root `docker-compose.yml` configures the g8e Operator service. For complete Gateway and Operator deployments, refer to the demo configurations in `demos/`.

```bash
# Example: Start the healthcare demo with gateway and operator
cd demos/healthcare
docker-compose up -d

# View logs
docker-compose logs -f

# Stop the services
docker-compose down
```

## Configuration

### Custom Ports

You can customize the HTTP and HTTPS ports:

**Via Docker command:**
```bash
docker run -d \
  --name g8e-gateway \
  -p 3000:3000 \
  -p 3443:3443 \
  -v g8e-data:/root/.g8e \
  g8e-gateway:latest \
  gw start --http-port 3000 --https-port 3443
```

**Via Docker Compose:**
Edit the `ports` section in your compose file:
```yaml
ports:
  - "3000:3000"
  - "3443:3443"
command: ["/g8e", "gw", "start", "--http-port", "3000", "--https-port", "3443"]
```

### Volume Mounts

The Dockerfile uses the following volumes:

- **`/root/.g8e`**: Runtime state directory (certificates, database, configuration, vault)
- **`/protocol/constants`**: Protocol constants copied during build (doctrine files, API paths, agents)

The protocol constants are embedded in the image during the build stage and do not require runtime mounting.

### Health Check

The container includes a health check that queries the `/api/v1/health` endpoint every 30 seconds:

```bash
docker inspect --format='{{.State.Health.Status}}' g8e-gateway
```

The health check uses wget to verify the gateway HTTP endpoint is responsive.

## Architecture

The Dockerfile uses a multi-stage build:

1. **Build Stage**: Uses `golang:1.26-alpine` to compile the binary from `./cmd/operator`
2. **Runtime Stage**: Uses `gcr.io/distroless/static-debian12` for minimal attack surface

The resulting image contains:
- The `g8e` binary at `/g8e`
- Protocol constants at `/protocol/constants`
- Exposed ports 8080 (HTTP) and 8443 (HTTPS)
- A health check on `/api/v1/health`

The same binary operates in gateway mode or operator mode depending on the command arguments.

## Troubleshooting

### View Logs

```bash
# Docker
docker logs g8e-gateway

# Docker Compose
docker-compose logs -f g8e-gateway
```

### Check Gateway Status

```bash
docker exec g8e-gateway /g8e gw status
```

### Gateway Posture Modes

The gateway supports three security postures:

- **doctrine**: L1 enforced, L2/L3 audited (default)
- **consensus**: L1/L2 enforced, L3 audited
- **notary**: L1/L2/L3 strictly enforced

Specify the posture at startup:
```bash
docker run -d \
  --name g8e-gateway \
  -p 8080:8080 \
  -p 8443:8443 \
  -v g8e-data:/root/.g8e \
  g8e-gateway:latest \
  gw start --posture consensus
```

### Stop the Gateway

```bash
# Docker
docker stop g8e-gateway
docker rm g8e-gateway

# Docker Compose
docker-compose down
```

### Rebuild After Code Changes

```bash
docker-compose build --no-cache
docker-compose up -d
```

## Production Considerations

- **Security**: The image uses distroless for minimal attack surface
- **Resource Limits**: Adjust CPU/memory limits in docker-compose.yml as needed
- **Persistence**: Use named volumes for `.g8e` data to persist across container restarts
- **Certificates**: The gateway generates certificates on first run. Use `--cert-mode full` for production deployments with proper hostnames
- **Networking**: Consider using a reverse proxy (nginx, traefik) for production deployments
- **Vault**: Ensure the vault is initialized and unlocked before starting the gateway in production
- **Port Consolidation**: The gateway uses a 2-port model (8080 for HTTP, 8443 for HTTPS) as of v1.0.10

## Advanced Usage

### Gateway Configuration File

For complex deployments, use a configuration file:

```bash
docker run -d \
  --name g8e-gateway \
  -p 8080:8080 \
  -p 8443:8443 \
  -v g8e-data:/root/.g8e \
  -v ./config/gateway.yml:/etc/g8e/gateway.yml:ro \
  g8e-gateway:latest \
  gw start --config /etc/g8e/gateway.yml
```

### Additional Flags

The gateway supports additional configuration flags:

- `--data-dir`: Data directory for SQLite database
- `--pki-dir`: Directory for TLS certificates
- `--secrets-dir`: Directory for platform secrets
- `--vault-dir`: Directory for encrypted vault
- `--passkey-rp-id`: RP ID for passkey operations
- `--passkey-rp-name`: RP Name for passkey operations
- `--cert-mode`: Certificate mode (full or localhost)
- `--rate-limit-rps`: Requests per second limit
- `--rate-limit-burst`: Rate limit burst size

Refer to `docs/guides/build_gateway.md` for complete flag documentation.
