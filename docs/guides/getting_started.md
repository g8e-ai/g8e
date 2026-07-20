---
title: Getting Started
parent: Guides
---

# Getting Started

Last Updated: 2026-07-19
Version: v1.5.9

---

## Overview

g8e is a zero-trust execution platform for agentic infrastructure. It consists of two components:

- **g8e Gateway**, the central Policy Decision Point (PDP): PKI authority, state store, pub/sub broker, admission APIs.
- **g8e Operator**, the host-side Policy Execution Point (PEP): outbound-only mTLS tunnel to the gateway, local audit vault, MCP server.

Both roles are served by the same `g8e` binary. The mode is set via the command-line subcommand.

---

## Requirements

There are two ways to run g8e: **entirely in Docker** (no local toolchain required) or **natively** (compile and run directly on your machine). Choose the path that fits your environment.

### Docker path (no local toolchain required)

| Requirement | Version |
|---|---|
| Docker | 24.0+ |
| Docker Compose | v2 (included in Docker Desktop 4.x and Docker Engine 24.0+) |

Everything, including the Go compiler and build dependencies, runs inside the container. No local toolchain is needed to build or run the gateway with Docker.

### Local path (build and run natively)

| Requirement | Notes |
|---|---|
| Go | 1.26.5, required to build from source |
| Make | Any recent version, required to run build targets |
| Git | Any recent version, required to clone the repository |
| Python | 3.10+, optional, required only for protocol library development |

> **Don't have `make` or `go` installed?** Run the setup script for your platform to detect and install them automatically (see [scripts.md](../architecture/scripts.md) for details):
> - **Linux:** `bash scripts/linux-setup.sh`
> - **macOS:** `bash scripts/macos-setup.sh`
> - **Windows:** `pwsh scripts/windows-setup.ps1`

---

## Get the Source

Both paths start with cloning the repository:

```bash
git clone https://github.com/g8e-ai/g8e.git
cd g8e
```

---

## Use the Protocol Library (Go Module or Python Package)

If you only need the g8e wire protocol — constants, models, enums, or protobuf definitions — for your own client or service, you can consume the published packages without building the full platform. Both packages share the same version number as the platform binary.

### Go module

As of v1.5.0, the protocol is part of the root Go module. Add it to your project:

```bash
go get github.com/g8e-ai/g8e@v1.5.8
```

Import the protocol packages in your Go code:

```go
import (
    "github.com/g8e-ai/g8e/protocol"
    "github.com/g8e-ai/g8e/protocol/proto/g8e/common/v1"
)
```

> **Migrating from v1.4.x?** The previous `go get github.com/g8e-ai/g8e/protocol@vX.Y.Z` is no longer needed. The root module now includes all protocol packages. See the [v1.5.0 release notes](../release_notes/v1.5.x/v1.5.0.md) for the full migration guide.

### Python package

Install from PyPI:

```bash
pip install g8e
```

Pinned to a specific version:

```bash
pip install g8e==1.5.8
```

The package provides:
- `g8e.constants` — JSON protocol constants (events, status, collections, headers, etc.)
- `g8e.enums` — Dynamic `StrEnum` and `IntEnum` generation from protocol constants
- `g8e.models` — Pydantic v2 models for protocol data structures

```python
from g8e.constants import EVENTS, ComponentName
from g8e.models import RequestContext

print(ComponentName.CLIENT)  # "client"
```

Requires Python 3.10+. See the [Protocol Library documentation](../architecture/protocol.md) for the full API reference and usage examples.

---

## Build

### Build locally

Requires `make` and Go 1.26+ installed on your machine. If you're not sure, run the [setup script](#local-path-build-and-run-natively) to check and install them automatically.

```bash
make build
```

Produces the `g8e` binary in the repository root and platform-specific binaries in `bin/`. All dependencies are resolved at build time; the compiled binary is statically linked (`CGO_ENABLED=0`) and has zero runtime dependencies. No Go toolchain, OpenSSL, Git, or other external tools are needed on the target host.

Additional build targets:

| Target | Description |
|---|---|
| `make build` | Build for current OS/architecture |
| `make build-all` | Build for all platforms (linux, windows, darwin) |
| `make build-linux` | Linux: amd64, arm64, 386 |
| `make build-darwin` | macOS: amd64, arm64 |
| `make build-windows` | Windows: amd64, arm64 |

For cross-compilation:

```bash
GOOS=linux GOARCH=amd64 make build
GOOS=darwin GOARCH=arm64 make build
GOOS=windows GOARCH=amd64 make build
```

### Build in Docker

Requires only Docker 24.0+. No local Go installation needed.

Build the binary for Linux (amd64):

```bash
make build-docker
```

This builds a `g8e-builder` Docker image using the `builder` stage of the Dockerfile, then runs the Go compiler inside it. The output binary lands in `bin/g8e-linux-amd64`.

Additional Docker build targets:

| Target | Description |
|---|---|
| `make build-docker` | Linux amd64 only |
| `make build-linux-docker` | Linux: amd64, arm64, 386 |
| `make build-darwin-docker` | macOS: amd64, arm64 |
| `make build-windows-docker` | Windows: amd64, arm64 |
| `make build-all-docker` | All platforms |

Binaries are placed in `bin/` with `.sha256` checksums alongside each one.

---

## Run the Gateway

### Run the gateway locally

After building with `make build`:

```bash
./g8e gw start
```

The gateway starts in Doctrine mode (L1 enforced, L2/L3 audited). To specify a security posture:

```bash
./g8e gw start --posture doctrine    # default
./g8e gw start --posture consensus   # L1/L2 enforced, L3 audited
./g8e gw start --posture notary      # L1/L2/L3 strictly enforced
```

Default ports: `8080` (HTTP) and `8443` (HTTPS/mTLS). Override with `--http-port` and `--https-port`.

Runtime state is written to `.g8e/` in the working directory:

| Path | Contents |
|---|---|
| `.g8e/pki/` | CA hierarchy and trust bundles |
| `.g8e/secrets/` | Bootstrap secrets and vault key |
| `.g8e/data/` | SQLite databases and blobs |
| `.g8e/vault/` | Encrypted audit vault |
| `.g8e/logs/` | Component logs |

Check gateway health:

```bash
./g8e gw status
```

View logs in real time:

```bash
./g8e gw logs -f
```

### Run the gateway in Docker

Requires Docker 24.0+. No local binary needed; the Docker image builds and bundles the binary.

Build the gateway image:

```bash
docker build -t g8e-gateway:latest .
```

Run the container:

```bash
docker run -d \
  --name g8e-gateway \
  -p 8080:8080 \
  -p 8443:8443 \
  -v g8e-data:/root/.g8e \
  g8e-gateway:latest \
  gw start --posture doctrine
```

The named volume `g8e-data` persists all runtime state (PKI, database, vault) across container restarts.

Check gateway health:

```bash
docker inspect --format='{{.State.Health.Status}}' g8e-gateway
```

View logs:

```bash
docker logs -f g8e-gateway
```

Run gateway management commands inside the container:

```bash
docker exec g8e-gateway /g8e gw status
docker exec g8e-gateway /g8e gw logs
```

Stop and remove the container:

```bash
docker stop g8e-gateway && docker rm g8e-gateway
```

---

## Connect an Operator

### Authenticate the CLI

After the gateway is running (locally or in Docker), authenticate the CLI to bootstrap the PKI hierarchy and issue mTLS credentials:

```bash
./g8e auth enroll
```

For Docker demos where HTTP and HTTPS are mapped to different host ports, use the split endpoint flags:

```bash
./g8e auth enroll -e localhost:<httpPort> --port <httpsPort>
```

See [Demo port mappings](#demo-port-mappings) below for each demo's ports.

### Enroll a remote operator

To connect an operator on a remote host to the gateway:

```bash
./g8e gw security pki enroll -e <gateway-ip>
```

See [Connect Operator to Gateway](./connect_operator_to_gateway.md) for full enrollment steps.

### Run the gateway and operator in Docker

The root `docker-compose.yml` defines both a `g8e-gateway` service and a `g8e-operator` service on a shared bridge network (`g8e-net`). The gateway registers a network alias `g8e.local` on `g8e-net`, so the operator can enroll by connecting to `g8e.local` without DNS or `/etc/hosts` configuration. The gateway health check must pass before the operator starts, enforced by `depends_on` with `condition: service_healthy`.

```bash
docker compose up -d
```

Both services use the root `Dockerfile` for image builds. The gateway exposes ports 8080 (HTTP) and 8443 (HTTPS/mTLS) on the host. The `restart: "no"` policy prevents enrollment loops if the gateway is not yet available.

---

## MCP Agent Integration

g8e integrates with popular AI agent binaries (Claude Code, Codex, Goose, Gemini CLI) to provide governed MCP tool access.

### Launch an agent with governance

Launch an agent with native tools disabled, forcing all I/O through the g8e MCP pipeline:

```bash
# Launch Claude Code with L1-L5 governance
./g8e mcp agent run claude

# Launch Goose with g8e MCP configuration
./g8e mcp agent run goose
```

### List supported agents

Print all supported AI agent binaries:

```bash
./g8e mcp agent list
```

### Show agent configurations

Print MCP client configurations for connecting to the g8e Gateway from local coding tools:

```bash
./g8e mcp agent show claude
```

The CLI displays configurations for `g8e.local` (mTLS), IP Address (mTLS), and Stdio Transport. If `g8e.local` resolution fails, the proxy automatically falls back to direct IP access.

---

## Industry Demos

The `demos/` directory contains Docker Compose environments for eight demo environments: **Healthcare** (HIPAA/PHI), **Finance** (trading controls), **Government** (CUI/CMMC), **Secure Data** (governed data migration with two-operator chain-of-custody), **DoW** (Department of War tactical edge with SIGINT, EO/IR, and PNT fusion sensors), **Swarm** (drone swarm with 20 autonomous operators), **DHS** (persistent sovereign capability with coalition data-plane governance, cross-domain release control, and cryptographically receipted destruction), and **Frontend** (third-party frontend enrollment with CORS, passkey, and SSE protection). Each demo is hermetically sealed with its own networks, volumes, and doctrine rules.

### What the demos use Docker for

Each demo spins up a full isolated stack via Docker Compose:

- **Gateway**, runs in a container on `net_perimeter` and `net_internal`
- **Operator**, runs in a container on `net_internal` and `net_secure` (outbound-only to gateway)
- **AI agent runtime**, simulated or real g8e agent on `net_internal`
- **Target system**, mock EHR/trading/classified-doc API on `net_secure`
- **Observability**, log aggregator and audit viewer on `net_mgmt`

All g8e services (gateway, operator, agent runtime) use `demos/Dockerfile` with `build: context: ..`, which copies a pre-built binary from `demos/bin/g8e` into the image. Run `make build` first to compile the binary and copy it to `demos/bin/g8e`. Auxiliary services (target systems, sensors, bad actors, observability) use Alpine, Python, Nginx, or other base images. The secure-data demo deploys two gateway-operator pairs (source and destination domains) on separate subnets. The DoW demo deploys three sensor agent containers (SIGINT, EO/IR, PNT fusion) with SWaP resource limits on all g8e containers. The swarm demo deploys a single gateway with 20 operator containers. The DHS demo deploys a real `agent-coalition` container running `demos scenarios run`, a real `datasvc` Python HTTP actuator on `net_secure`, and display-only source connectors modeling NIPR/SIPR/Mission-Partner/partner-nation sovereignty boundaries.

All demos use the `/api/v1/health` endpoint for gateway health checks.

The `g8e` demos CLI checks for a local binary at `demos/bin/g8e` and prints a warning if it is not found. This binary is required for Docker Compose builds, which copy it into the image via `demos/Dockerfile`.

### Prerequisites for demos

- Docker 24.0+ with Docker Compose v2
- Go 1.26+ (or a pre-built `g8e` binary) to produce `demos/bin/g8e`

Run `make build` first to compile the binary and copy it to `demos/bin/g8e`. Docker Compose copies this binary into each g8e service container via `demos/Dockerfile`.

### Run a demo

Use the `g8e` CLI to manage demo environments:

```bash
# List available demo environments
./g8e demos list

# Start a demo
./g8e demos start healthcare

# Check status
./g8e demos status healthcare

# Run a scenario
./g8e demos run healthcare 1   # Authorized agent submits a FHIR PA request
./g8e demos run healthcare 4   # Bad actor PHI exfiltration blocked

# Stop
./g8e demos stop healthcare

# Remove containers, volumes, and networks
./g8e demos clean healthcare

# Clean and restart in one step
./g8e demos reset healthcare

# Rebuild Docker images and restart
./g8e demos rebuild healthcare

# Pre-pull external images for air-gapped deployment
./g8e demos pull
```

Or use Docker Compose directly:

```bash
cd demos/healthcare
docker compose up -d
docker compose logs -f
docker compose down -v
```

### Demo scenarios

| Demo | Scenario | Description |
|---|---|---|
| healthcare | 1 | Authorized agent submits a FHIR PA request |
| healthcare | 2 | Gold card auto-approval |
| healthcare | 3 | SLA breach and OHA reporting |
| healthcare | 4 | Bad actor PHI exfiltration blocked |
| gov | 1 | CUI exfiltration attempt blocked |
| finance | 1 | Unauthorized trade blocked |
| secure-data | 1 | Governed migration with chain-of-custody receipts |
| secure-data | 2 | Connector bypass attempt blocked |
| secure-data | 3 | Cross-tenant leak doctrine triggered |
| dow | 1 | Autonomous SIGINT-to-EO/IR cross-cueing |
| dow | 2 | BFT spoofing defense |
| dow | 3 | Disconnected operations |
| dhs | 1 | Sovereign multi-source ingest (chain-of-custody) |
| dhs | 2 | Cross-domain release requires Notary authority |
| dhs | 3 | Resilient disconnected operations / continuity of coverage |
| dhs | 4 | Governed predictive cueing (quorum vs veto) |
| dhs | 5 | Sovereign destruction + tamper-proof audit |
| swarm | 1 | Authorized recon mission (governed drone deployment) |
| swarm | 2 | Weapons safety doctrine block |
| swarm | 3 | Navigation boundary violation block |
| frontend | 1 | Third-party frontend enrollment |

### Demo port mappings

Each demo uses distinct host ports to allow simultaneous deployment. The frontend demo shares ports with secure-data and cannot run at the same time:

| Demo | HTTP | HTTPS | Demo UI |
|---|---|---|---|
| gov | 8080 | 8443 | 3000 |
| healthcare | 8081 | 8444 | 3001 |
| finance | 8082 | 8445 | 3002 |
| secure-data (src) | 8083 | 8446 | - |
| secure-data (dst) | 8084 | 8447 | - |
| swarm | 8085 | 8448 | 5005 |
| dow | 8086 | 8449 | - |
| dhs | 8087 | 8450 | - |
| frontend | 8083 | 8446 | 3003 |

When enrolling the CLI against a demo gateway, use the HTTP and HTTPS ports from the table above:

```bash
./g8e auth enroll -e localhost:<httpPort> --port <httpsPort>
```

For example, to enroll against the healthcare demo: `./g8e auth enroll -e localhost:8081 --port 8444`.

---

## Post-Bootstrap Actions

After the gateway is running and the CLI is authenticated:

```bash
./g8e gw status           # Gateway health and endpoint info
./g8e gw data operators   # List enrolled operators
./g8e gw data users       # List users
./g8e gw data audit list  # Inspect the audit vault
./g8e --help              # Full command reference
```

---

## Governance Postures

| Posture | L1 | L2 | L3 | Flag |
|---|---|---|---|---|
| Doctrine (default) | Enforced | Audited | Audited | `--posture doctrine` |
| Consensus | Enforced | Enforced | Audited | `--posture consensus` |
| Notary | Enforced | Enforced | Enforced | `--posture notary` |

---

## Next Steps

- **[Build Gateway](build_gateway.md)**, Full gateway build reference, custom gateway implementations, and CLI flag reference
- **[Build Operator](build_operator.md)**, Build and deploy a custom g8e Operator
- **[Connect Operator to Gateway](connect_operator_to_gateway.md)**, Enrollment, mTLS configuration, and session management
- **[Connect Apps to Gateway](connect_apps_to_gateway.md)**, Integrate application-layer adapters
- **[Docker Gateway Guide](docker_gateway.md)**, Docker-specific configuration, volumes, and production considerations
- **[Architecture](../architecture/gateway.md)**, Platform architecture and 5-layer verification sequence
- **[MCP Protocol](../../protocol/docs/mcp.md)**, Connect AI clients via Model Context Protocol
- **[Protocol Library](../architecture/protocol.md)**, Go module and Python package API reference, constants, models, and usage examples
- **[A2A Protocol](../../protocol/docs/a2a.md)**, Agent-to-agent communication patterns
