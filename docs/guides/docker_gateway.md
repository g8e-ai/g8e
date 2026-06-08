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
  -v $(pwd)/protocol/constants:/protocol/constants:ro \
  -v g8e-data:/root/.g8e \
  g8e-gateway:latest
```

### Using Docker Compose (Recommended)

```bash
# Build and start with default ports (8080/8443)
docker-compose up -d

# Build and start with custom ports
HTTP_PORT=3000 HTTPS_PORT=3443 docker-compose up -d

# View logs
docker-compose logs -f

# Stop the gateway
docker-compose down
```

## Configuration

### Custom Ports

You can customize the HTTP and HTTPS ports in several ways:

**Via Docker command:**
```bash
docker run -d \
  --name g8e-gateway \
  -p 3000:3000 \
  -p 3443:3443 \
  -v $(pwd)/protocol/constants:/protocol/constants:ro \
  -v g8e-data:/root/.g8e \
  g8e-gateway:latest \
  --doctrine gw start --http-port 3000 --https-port 3443
```

**Via Docker Compose:**
```bash
HTTP_PORT=3000 HTTPS_PORT=3443 docker-compose up -d
```

**Or edit docker-compose.yml directly:**
```yaml
ports:
  - "3000:3000"
  - "3443:3443"
environment:
  - G8E_HTTP_PORT=3000
  - G8E_HTTPS_PORT=3443
command: ["--doctrine", "gw", "start", "--http-port", "3000", "--https-port", "3443"]
```

### Volume Mounts

The Dockerfile mounts the following volumes:

- **`/protocol/constants`**: Read-only mount of doctrine constants (required for doctrine mode)
- **`/root/.g8e`**: Runtime state directory (certificates, database, configuration)

### Health Check

The container includes a health check that runs every 30 seconds:

```bash
docker inspect --format='{{.State.Health.Status}}' g8e-gateway
```

## Architecture

The Dockerfile uses a multi-stage build:

1. **Build Stage**: Uses `golang:1.26-alpine` to compile the binary
2. **Runtime Stage**: Uses `gcr.io/distroless/static-debian12` for minimal attack surface

This results in a small, secure image with only the necessary runtime dependencies.

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
- **Persistence**: Use named volumes for .g8e data to persist across container restarts
- **Certificates**: The gateway generates self-signed certificates on first run in localhost mode
- **Networking**: Consider using a reverse proxy (nginx, traefik) for production deployments

## Advanced Usage

### Custom Doctrine Path

If you need to use a custom doctrine location:

```bash
docker run -d \
  --name g8e-gateway \
  -p 8080:8080 \
  -p 8443:8443 \
  -v /path/to/custom/constants:/protocol/constants:ro \
  -v g8e-data:/root/.g8e \
  g8e-gateway:latest
```

### Environment Variables

You can pass additional environment variables:

```bash
docker run -d \
  --name g8e-gateway \
  -p 8080:8080 \
  -p 8443:8443 \
  -v $(pwd)/protocol/constants:/protocol/constants:ro \
  -v g8e-data:/root/.g8e \
  -e G8E_LOG_LEVEL=debug \
  g8e-gateway:latest
```
