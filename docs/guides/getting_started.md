---
title: Getting Started
parent: Guides
---

# Getting Started

Last Updated: 2026-08-25
Version: v2.0.0

---

## Overview

g8e is a zero-trust execution platform for agentic infrastructure. It consists of two components:

- **g8e Gateway**, the central Policy Decision Point (PDP): PKI authority, state store, pub/sub broker, admission APIs.
- **g8e Operator**, the host-side Policy Execution Point (PEP): outbound-only mTLS tunnel to the gateway, local audit vault, MCP server.

Both roles are served by the same `g8e` binary. The mode is set via the command-line subcommand.

---

## Quick Start (Docker Compose)

The recommended path to launch g8e is using the unified Docker Compose stack from the repository root. This requires only Docker 24.0+ and no local Go compiler.

Building the gateway container image compiles all platform binaries inside the image (linux/amd64 with FIPS 140-3 approved mode, arm64, 386, windows/amd64, windows/arm64, darwin/amd64, darwin/arm64) via `make build-all` in the builder stage. The gateway hosts and serves these binaries via `/.well-known/g8e/bin/{filename}` for node and remote operator deployment.

### 1. Clone and start the Gateway

```bash
git clone https://github.com/g8e-ai/g8e.git
cd g8e
docker compose up -d --build
```

`docker compose up -d` starts the central Policy Decision Point (`g8e-gateway`) on port 8080 (HTTP discovery, MCP) and port 8443 (HTTPS/mTLS, Web Console). Platform workloads (`g8e-operator`, `ensemble`, `dashboard`) belong to the `bootstrapped` profile and wait for owner enrollment before starting.

### 2. Extract the CLI binary

Copy the compiled CLI binary from the running gateway container to your host:

```bash
docker cp g8e-gateway:/g8e ./g8e
```

### 3. Enroll the first owner

Authenticate the CLI to bootstrap the gateway PKI hierarchy, install the root CA into the OS trust store, and complete the browser-based WebAuthn passkey ceremony:

```bash
./g8e auth enroll user -e localhost
```

Follow the browser prompt to create your passkey. Once enrolled, the CLI holds mTLS credentials bound to the root owner identity.

### 4. Start the platform workloads

With the owner identity established, bring up the Operator, Agentic Ensemble (g8ee), and Dashboard (g8ed):

```bash
docker compose --profile bootstrapped up -d
```

### 5. Approve platform workload enrollments

List pending platform enrollment requests and approve each workload using your authenticated CLI session:

```bash
# List pending enrollment requests
./g8e auth pending-platform-enrollments

# Approve the operator, dashboard, and ensemble
./g8e auth approve-platform-enrollment <operator-request-id> --yes
./g8e auth approve-platform-enrollment <dashboard-request-id> --yes
./g8e auth approve-platform-enrollment <ensemble-request-id> --yes
```

You can also view and approve pending enrollments in your browser via the Gateway Web Console at `https://localhost:8443/console/`.

### 6. Verify stack health

```bash
docker compose ps
./g8e gw status
```

Service endpoints:
- **Gateway HTTP & MCP:** http://localhost:8080
- **Gateway HTTPS & Web Console:** https://localhost:8443 (Web Console: `https://localhost:8443/console/`)
- **Operator Dashboard:** http://localhost:3000
- **Ensemble API:** http://localhost:8000

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
| Go | 1.26.6, required to build from source |
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

If you only need the g8e wire protocol, constants, models, enums, or protobuf definitions, for your own client or service, you can consume the published packages without building the full platform. Both packages share the same version number as the platform binary.

### Go module

As of v1.5.0, the protocol is part of the root Go module. Add it to your project:

```bash
go get github.com/g8e-ai/g8e@v2.0.0
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
pip install g8e==2.0.0
```

The package provides:
- `g8e.constants`: JSON protocol constants (events, status, collections, headers, etc.)
- `g8e.enums`: Dynamic `StrEnum` and `IntEnum` generation from protocol constants
- `g8e.models`: Pydantic v2 models for protocol data structures

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

Produces the `g8e` binary in the repository root and platform-specific binaries in `bin/`. All dependencies are resolved at build time; the compiled binary is statically linked and has zero runtime dependencies. No Go toolchain, OpenSSL, Git, or other external tools are needed on the target host.

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

Build and start the full stack (the Dockerfile builder stage compiles all platform binaries inside the image):

```bash
make up
```

To obtain a host-side CLI binary without a local Go toolchain, copy it out of the running gateway container:

```bash
docker cp g8e-gateway:/g8e ./g8e
```

The Dockerfile builder stage produces all platform binaries (linux/amd64, linux/arm64, linux/386, windows/amd64, windows/arm64, darwin/amd64, darwin/arm64); the gateway serves them via `/.well-known/g8e/bin/{filename}` for node deployment. Linux binaries are built with FIPS 140-3 approved mode enabled via `GOFIPS140=v1.0.0`.

Related Docker Compose lifecycle targets:

| Target | Description |
|---|---|
| `make up` | Build and start the full stack (`docker compose up -d --build`) |
| `make down` | Stop the stack, preserving volumes (`docker compose down`) |
| `make clean-docker` | Stop the stack and remove volumes (`docker compose down -v`) |

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
  gw start -f --posture doctrine
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
./g8e auth enroll user
```

The `auth enroll user` command installs the gateway Root CA into the OS trust store before opening the browser for the passkey ceremony. Before installation, it checks for stale g8e Root CA anchors from previous gateway instances and prompts for removal if found. If trust installation fails, the browser does not open; resolve the trust issue and re-run. Use `--no-system-trust` only if an administrator has already installed the Root CA. After trust installation or stale anchor removal, close all open browser windows before clicking the enrollment link so the browser opens a fresh session that recognizes the new trust anchor.

For Docker demos where HTTP and HTTPS are mapped to different host ports, use the split endpoint flags:

```bash
./g8e auth enroll user -e localhost:<httpPort> --port <httpsPort>
```

See [Demo port mappings](#demo-port-mappings) below for each demo's ports.

### Start a remote operator

To connect an operator on a remote host to the gateway:

```bash
./g8e operator start -e <gateway-ip>
```

When `--endpoint` (or `-e`) is provided, the operator automatically initiates platform enrollment with the gateway if credentials are not yet installed. The gateway holds the enrollment request in pending state until the enrolled owner approves it via `./g8e auth pending-platform-enrollments` and `./g8e auth approve-platform-enrollment <request-id> --yes` (or via the gateway web console at `https://<gateway-ip>:8443/console/`). Once approved, the operator receives signed mTLS credentials, connects to the gateway pub/sub broker on port 8443, and begins executing governed actions. See [Connect Operator to Gateway](./connect_operator_to_gateway.md) for full enrollment and remote deployment options.

### Run the gateway and operator in Docker

The root `docker-compose.yml` deploys the full platform stack on a shared `g8e-net` bridge network: `g8e-gateway` (PDP), `g8e-operator` (PEP), `ensemble` (g8ee), and `dashboard` (g8ed). The stack uses a two-phase startup model where `docker compose up -d` starts only the unprofiled gateway service. After enrolling the first owner, start the remaining platform workloads under the `bootstrapped` profile and approve their enrollment requests:

```bash
# Phase 1: Start the gateway
docker compose up -d

# Phase 2: Enroll the first owner
./g8e auth enroll user -e localhost

# Phase 3: Start workloads and approve enrollment requests
docker compose --profile bootstrapped up -d
./g8e auth pending-platform-enrollments
./g8e auth approve-platform-enrollment <operator-request-id> --yes
```

The gateway exposes ports 8080 (HTTP) and 8443 (HTTPS/mTLS) on the host. The operator resolves the gateway via the internal Docker network alias `g8e.local`. See [Unified Docker Stack Guide](unified_stack.md) and [Docker Gateway Guide](docker_gateway.md) for full configuration options.

---

## MCP Agent Integration

g8e integrates with popular AI agent binaries (Claude Code, Codex, Devin CLI, Goose, Gemini CLI) to provide governed MCP tool access.

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

The `demos/` directory contains Docker Compose environments for five demo environments. Each demo is isolated with its own networks, volumes, and doctrine rules.

- **Healthcare**: HIPAA/PHI governance
- **Finance**: trading controls
- **DHS**: persistent sovereign capability with coalition data-plane governance, cross-domain release control, and cryptographically receipted destruction
- **FedRAMP**: sovereign cloud governance with CR-26 audit integrity, access control, and cross-domain protection
- **Frontend**: third-party frontend enrollment with CORS, passkey, and SSE protection

### What the demos use Docker for

Each demo spins up a full isolated stack via Docker Compose:

- **Gateway**, runs in a container on `net_perimeter` and `net_internal`
- **Operator**, runs in a container on `net_internal` and `net_secure` (outbound-only to gateway)
- **AI agent runtime**, simulated or real g8e agent on `net_internal`
- **Target system**, mock EHR/trading/classified-doc API on `net_secure`
- **Observability**, log aggregator and audit viewer on `net_mgmt`

All g8e services (gateway, operator, agent runtime) build from the repo-root `Dockerfile` (the same production image, always-FIPS) via `context: ../..` in each demo's `compose.yml`. The image compiles from source in-container, so no pre-built binary is required for Docker Compose builds. Auxiliary services (target systems, sensors, bad actors, observability) use Alpine, Python, Nginx, or other base images.

The DHS and FedRAMP demos additionally deploy real agent runtime containers, Python HTTP actuators on the secure network, and display-only source connectors modeling sovereignty boundaries.

All demos use the `/api/v1/health` endpoint for gateway health checks.

The `g8e` demos CLI checks for a local binary at `demos/bin/g8e` and prints a warning if it is not found. This binary is not required for Docker Compose builds (demos compile from source via the repo-root `Dockerfile`), but the CLI uses it for host-side demo orchestration when present.

### Prerequisites for demos

- Docker 24.0+ with Docker Compose v2
- Go 1.26+ (only needed for host-side `make build`; Docker Compose builds compile from source in-container)

Run `make build` to compile the binary and copy it to `demos/bin/g8e` for host-side demo orchestration. Docker Compose builds do not require this binary; demos build from the repo-root `Dockerfile` via `context: ../..`.

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
| finance | 1 | Unauthorized trade blocked |
| dhs | 1 | Sovereign multi-source ingest (chain-of-custody) |
| dhs | 2 | Resilient disconnected operations / continuity of coverage |
| dhs | 3 | Governed predictive cueing (quorum vs veto) |
| dhs | 4 | Sovereign destruction + tamper-proof audit |
| fedramp | 1 | Governed cloud resource provisioning |
| fedramp | 2 | Unauthorized audit trail destruction blocked |
| fedramp | 3 | Governed configuration revert |
| fedramp | 4 | Gateway audit vault destruction blocked |
| frontend | 1 | Third-party frontend enrollment |

### Demo port mappings

Each demo uses distinct host ports to allow simultaneous deployment:

| Demo | HTTP | HTTPS | Demo UI |
|---|---|---|---|
| healthcare | 8081 | 8444 | 3001 |
| finance | 8082 | 8445 | 3002 |
| dhs | 8087 | 8450 | - |
| fedramp | 8088 | 8451 | - |
| frontend | 8083 | 8446 | 3003 |

When enrolling the CLI against a demo gateway, use the HTTP and HTTPS ports from the table above:

```bash
./g8e auth enroll user -e localhost:<httpPort> --port <httpsPort>
```

For example, to enroll against the healthcare demo: `./g8e auth enroll user -e localhost:8081 --port 8444`.

### Verify all demos

Run the one-command demo verification target to build the binary and run all 5 demos sequentially:

```bash
make demo-verify
```

This target depends on `make build`, then for each demo (healthcare, finance, dhs, fedramp, frontend) it:

1. Stops any running instance and cleans up volumes
2. Runs all scenarios via `g8e demos run <org>`
3. Stops and cleans up after the demo
4. Reports PASS or FAIL

If any demo fails, the target exits immediately with a non-zero status. All 5 demos must pass for the target to succeed.

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
