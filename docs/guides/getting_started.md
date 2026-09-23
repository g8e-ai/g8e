---
title: Getting Started
parent: Guides
---

# Getting Started

Last Updated: 2026-09-23
Version: v2.1.12

---

## Overview

g8e is a zero-trust execution platform for agentic infrastructure. Its two core execution components are:

- **g8e Gateway**, the central Policy Decision Point (PDP): PKI authority, state store, pub/sub broker, and admission APIs.
- **g8e Operator**, the Policy Execution Point (PEP) for the runtime where that Operator process runs: an outbound-only mTLS connection to the Gateway, local audit vault, and governed tool execution.

Both roles use the same `g8e` binary, selected by the `gw` or `operator` subcommand. The unified stack also includes the first-party Agentic Ensemble (g8ee) and Dashboard (g8ed).

---

## Quick Start (Docker Compose)

The recommended path to launch g8e is the unified Docker Compose stack from the repository root. Building and running the stack requires Docker 24.0+ with the Docker Compose v2 plugin. No local Go compiler is required for the container build, but the owner-enrollment steps require a current `./g8e` CLI binary and a browser unless you choose headless enrollment.

The root image builds only the runtime binary for the image target platform (`linux/amd64` or `linux/arm64`). Run `make build-all` on the host when you need the full Linux, Windows, and macOS deployment matrix for remote Operator deployment. Linux binaries link the Go Cryptographic Module through `GOFIPS140=v1.0.0`; the FIPS 140-3 claim is scoped to linux/amd64, and strict runtime enforcement requires `GODEBUG=fips140=only`. Inspect the deployed binary with `g8e version --fips`; the build setting alone does not enable runtime enforcement.

### 1. Clone and start the Gateway

```bash
git clone https://github.com/g8e-ai/g8e.git
cd g8e
cp .env.example .env
docker compose up -d --build
```

The Compose file contains optional profiles whose services use the required `G8E_OLLAMA_ENDPOINT` interpolation. Compose therefore requires that variable even when those profiles are inactive; the copied `.env.example` supplies a placeholder value. Replace it with the approved remote Ollama URL before enabling the `evaluation` or `g8ellama` profile. Do not treat `localhost` as a remote provider unless Ollama is reachable from the relevant container network.

`docker compose up -d` starts only the unprofiled Gateway service. It publishes port 8080 for limited plain-HTTP health, bootstrap, PKI discovery, and enrollment flows, and port 8443 for the HTTPS APIs, MCP, A2A, pub/sub, and Web Console. The `bootstrapped` profile adds the Data Operator, ensemble, and dashboard after owner enrollment. The `evaluation` profile adds the Inference Operator and requires an approved remote Ollama endpoint. See the [Unified Docker Stack Guide](unified_stack.md) for profiles, volumes, network namespaces, and evaluation topology.

### 2. Get the CLI binary

The CLI binary must run on the same workstation where you complete the browser-based WebAuthn passkey ceremony in Step 3, because `auth enroll user` opens a browser on the local machine. If your workstation with a browser is the same host where the gateway is running, copy the binary out of the gateway container:

```bash
docker cp "${G8E_PREFIX:-g8e}-gateway:/g8e" ./g8e
```

If your workstation is on a different host than the gateway, download the binary over HTTP from the gateway's bootstrap endpoint instead. The gateway serves all platform binaries built by the Dockerfile at `/.well-known/g8e/bin/{filename}` on the HTTP discovery port (8080 by default), with no authentication required so that the g8e binary can be placed on remote hosts as soon as the Gateway is started:

```bash
# From your workstation, targeting the gateway host's HTTP port
curl -fSLO http://<gateway-host>:8080/.well-known/g8e/bin/g8e-linux-amd64
chmod +x g8e-linux-amd64
```

On Windows PowerShell, `curl` is an alias for `Invoke-WebRequest` and does not accept curl-style flags. Use the real `curl.exe` (ships with Windows 10+) or `Invoke-WebRequest` directly:

```powershell
# Option 1: real curl (Windows 10+)
curl.exe -fSLO http://<gateway-host>:8080/.well-known/g8e/bin/g8e-windows-amd64.exe

# Option 2: Invoke-WebRequest
Invoke-WebRequest http://<gateway-host>:8080/.well-known/g8e/bin/g8e-windows-amd64.exe -OutFile g8e-windows-amd64.exe
```

Replace `g8e-linux-amd64` (or `g8e-windows-amd64.exe`) with the binary matching your workstation: `g8e-linux-amd64`, `g8e-linux-arm64`, `g8e-linux-386`, `g8e-darwin-amd64`, `g8e-darwin-arm64`, `g8e-windows-amd64.exe`, or `g8e-windows-arm64.exe`. Binary downloads are served from the gateway's plain-HTTP discovery port, even though authenticated APIs and the Web Console use HTTPS.

### 3. Enroll the first owner

Authenticate the CLI to bootstrap the gateway PKI hierarchy, install the root CA into the OS trust store, and complete the browser-based WebAuthn passkey ceremony:

```bash
./g8e auth enroll user -e localhost
```

By default, the command installs the Gateway Root CA in the workstation's OS trust store before opening the browser. Follow the browser prompt to create the passkey. Once enrollment completes, the CLI holds mTLS credentials bound to the first-owner identity. For a CLI-only owner, use `--headless`; it skips the browser and OS trust installation and cannot authenticate to the Web Console. Use `--no-system-trust` only when an administrator has already installed the Root CA. See [Authentication and Authorization](../architecture/auth.md) for recovery, rotation, and identity details.

### 4. Start the platform workloads

With the owner identity established, bring up the Operator, Agentic Ensemble (g8ee), and Dashboard (g8ed). See the [g8ee documentation](../ensemble/index.md) and the [g8ed documentation](../dashboard/index.md) for component details:

```bash
docker compose --profile bootstrapped up -d
```

### 5. Review platform workload enrollments

List pending platform enrollment requests and approve or deny each workload using your authenticated CLI session:

```bash
# List pending enrollment requests
./g8e auth enroll pending

# Approve the operator, dashboard, and ensemble
./g8e auth enroll approve <operator-request-id> --yes
./g8e auth enroll approve <dashboard-request-id> --yes
./g8e auth enroll approve <ensemble-request-id> --yes

# Reject a request instead of approving it
./g8e auth enroll deny <request-id> --yes
```

You can also view and decide pending enrollments in your browser via the Gateway Web Console at `https://localhost:8443/console/`.

### 6. Verify stack health

```bash
docker compose ps
./g8e gw status
```

Service endpoints:
- **Gateway bootstrap, health, and PKI discovery:** `http://localhost:8080`
- **Gateway HTTPS/mTLS API, MCP, and A2A:** `https://localhost:8443` (`https://localhost:8443/mcp` for MCP)
- **Gateway Web Console:** `https://localhost:8443/console/`
- **Dashboard static host:** `http://localhost:3000`
- **Ensemble API:** `http://localhost:8000`
- **Public spectator private ingest:** `http://127.0.0.1:8081` (loopback only)
- **Public spectator anonymous read/SSE:** `http://127.0.0.1:8082` (loopback only)
- **Evaluation explorer:** `http://127.0.0.1:5173` (loopback only; available when its assets are built)

The Dashboard is a static browser host; the browser authenticates directly to the Gateway. The Dashboard container identity is separate from the browser session and does not authorize browser actions.

### CLI-managed alternative

If a current `g8e` binary is already available on the workstation, `./g8e docker start --full` starts the `bootstrapped` profile, enrolls or reuses the CLI owner interactively, and prompts for platform workload approvals. It does not start the `evaluation` profile. Use `./g8e docker start --full --skip-enroll` only when enrollment and approvals are managed separately. For automated evaluation bootstrap, use `./g8e docker init` with `G8E_OLLAMA_ENDPOINT` set in the repository-root `.env`; see the [Unified Docker Stack Guide](unified_stack.md).

---

## Requirements

There are two ways to run g8e: **entirely in Docker** (no local Go toolchain required) or **natively** (compile and run directly on your machine). Choose the path that fits your environment. Run repository commands from the repository root.

### Docker path (no local toolchain required)

| Requirement | Version |
|---|---|
| Docker | 24.0+ |
| Docker Compose | v2 |

The Docker build runs the Go compiler and build dependencies inside the builder stage. No local Go or Make installation is needed. You still need the repository, Docker, the resulting `./g8e` CLI binary for enrollment and management, and a local browser for passkey enrollment. Browser-based enrollment may require administrator permission to install the Gateway Root CA in the workstation's OS trust store. Use `--headless` when a browser is unavailable.

### Local path (build and run natively)

| Requirement | Notes |
|---|---|
| Go | 1.26.6, required to build from source |
| Make | Any recent version, required to run build targets |
| Git | Any recent version, required to clone the repository |
| Python | 3.10+, optional, required only for protocol library development |
| Node.js and npm | Node.js 22+, required to build the embedded evaluation explorer before a local `make build` |

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
go get github.com/g8e-ai/g8e/v2@v2.1.12
```

Import the protocol packages in your Go code:

```go
import (
    "github.com/g8e-ai/g8e/v2/protocol"
    "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/common/v1"
)
```

> **Migrating from v2.0.0?** The Go module path changed from `github.com/g8e-ai/g8e` to `github.com/g8e-ai/g8e/v2` in v2.0.1. Update your import paths and `go get` commands to include the `/v2` suffix. See the [v2.0.1 release notes](../release_notes/v2.0.x/v2.0.1.md) for the full migration guide.
>
> **Migrating from v1.4.x?** The previous `go get github.com/g8e-ai/g8e/protocol@vX.Y.Z` is no longer needed. The root module now includes all protocol packages. See the [v1.5.0 release notes](../release_notes/v1.5.x/v1.5.0.md) for the full migration guide.

### Python package

Install from PyPI:

```bash
pip install g8e
```

Pinned to a specific version:

```bash
pip install g8e==2.1.12
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

Requires `make`, Go 1.26.6, and Node.js 22+ with npm. Before `make build`, build the embedded evaluation explorer so the Makefile can package its static assets:

```bash
cd dashboard/g8e-adapter/evaluation-explorer
npm ci
npm run build
cd ../../..
make build
```

The build produces the `g8e` binary in the repository root and a platform-specific binary in `bin/`. The compiled Go binary is statically linked and has zero runtime dependencies; the Node.js toolchain is needed only to build the embedded explorer. If you're not sure whether the local tools are installed, run the [setup script](#local-path-build-and-run-natively) for the Go and Make prerequisites, then see [Build a g8e-Compatible Frontend](build_frontend.md) for frontend tooling.

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

Build the images and start the gateway phase of the unified stack:

```bash
make up
```

After owner enrollment, start the remaining workloads with `docker compose --profile bootstrapped up -d`, as shown in the [Quick Start](#quick-start-docker-compose).

To obtain a host-side CLI binary without a local Go toolchain, copy it out of the running gateway container:

```bash
docker cp "${G8E_PREFIX:-g8e}-gateway:/g8e" ./g8e
```

The Dockerfile builder stage produces only the image target platform binary. Run `make build-all` on the host when you need the full deployment matrix for `/.well-known/g8e/bin/{filename}` downloads. Linux builds link the pinned Go Cryptographic Module; run `g8e version --fips` to inspect module status. The project's FIPS 140-3 compliance claim applies only to linux/amd64.

Related Docker Compose lifecycle targets:

| Target | Description |
|---|---|
| `make up` | Build images and start the unprofiled gateway (`docker compose up -d --build`) |
| `make down` | Stop services from all profiles and preserve volumes |
| `make clean-docker` | Stop services from all profiles and remove volumes |

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
./g8e gw start --posture ratify      # L1/L3 enforced, L2 audited
./g8e gw start --posture notary      # L1/L2/L3 strictly enforced
```

The default plain-HTTP port is `8080` and serves only bootstrap and PKI discovery routes. The default HTTPS port is `8443` and serves authenticated APIs, MCP, and the Web Console. Override them with `--http-port` and `--https-port`.

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
  --workdir /root \
  -p 8080:8080 \
  -p 8443:8443 \
  -v g8e-data:/root/.g8e \
  g8e-gateway:latest \
  gw start -f --posture doctrine --cert-mode localhost
```

`--cert-mode localhost` makes this standalone example suitable for local access. Use the default `full` certificate mode only when the container can read the host identity inputs required for the Gateway serving certificate. The named volume `g8e-data` persists Gateway-local runtime state, including PKI, databases, and vault data, across container restarts.

Check gateway health from the host:

```bash
curl -fsS http://localhost:8080/api/v1/health
```

The image has no image-level health check because the same image also runs the Operator, which has no inbound HTTP listener. Docker Compose defines service-specific health checks.

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

See [Demo scenarios and ports](#demo-scenarios-and-ports) below for each demo's ports.

### Start a remote operator

To connect an Operator on a remote host to the Gateway, use a bare hostname that resolves on the Operator host and appears in the Gateway serving certificate:

```bash
./g8e operator start -e <gateway-hostname>
```

When `--endpoint` (or `-e`) is provided, the Operator automatically initiates platform enrollment with the Gateway if credentials are not installed. The Gateway holds the enrollment request in pending state until the enrolled owner verifies and decides it with `./g8e auth enroll pending`, then `./g8e auth enroll approve <request-id> --yes` or `./g8e auth enroll deny <request-id> --yes`. After approval, the Operator receives signed mTLS credentials, opens its outbound connection to the Gateway on port 8443, and begins executing governed actions in its own runtime. The Operator does not expose an inbound management port. See [Connect Operator to Gateway](./connect_operator_to_gateway.md) for hostname, certificate, enrollment, and remote deployment requirements.

### Run the gateway and operator in Docker

The root `docker-compose.yml` deploys the full platform stack on a shared `g8e-net` bridge network: `g8e-gateway` (PDP), `g8e-operator` (PEP), `ensemble` (g8ee), and `dashboard` (g8ed). See the [g8ee documentation](../ensemble/index.md) and the [g8ed documentation](../dashboard/index.md) for the first-party component details. The stack uses a two-phase startup model in which `docker compose up -d` starts only the unprofiled gateway service. After enrolling the first owner, start the remaining platform workloads under the `bootstrapped` profile and approve all three enrollment requests:

```bash
# Phase 1: Start the gateway
docker compose up -d

# Phase 2: Enroll the first owner
./g8e auth enroll user -e localhost

# Phase 3: Start workloads and approve every enrollment request
docker compose --profile bootstrapped up -d
./g8e auth enroll pending
./g8e auth enroll approve <operator-request-id> --yes
./g8e auth enroll approve <dashboard-request-id> --yes
./g8e auth enroll approve <ensemble-request-id> --yes
```

The gateway exposes plain-HTTP bootstrap and discovery on port 8080 and HTTPS/mTLS APIs and MCP on port 8443. The operator resolves the gateway through the internal Docker network alias `g8e.local`. See [Unified Docker Stack Guide](unified_stack.md) and [Docker Gateway Guide](docker_gateway.md) for full configuration options.

---

## MCP Agent Integration

g8e integrates with popular AI agent binaries (Claude Code, Codex, Devin CLI, Goose, Gemini CLI) to provide governed MCP tool access.

### Launch an agent with governance

Launch a supported agent with g8e as its MCP server. Claude, Codex, Goose, and Gemini launch profiles disable or exclude their native tools so supported I/O routes through the g8e MCP pipeline:

```bash
./g8e mcp agent run claude
./g8e mcp agent run goose
```

The launcher cannot disable Devin's native tools, and clients can still use side channels such as direct filesystem, shell, network, or other MCP access when those capabilities remain enabled. Only requests sent through g8e cross the governance boundary. See [AI Agents and the g8e Governance Boundary](../architecture/agents.md) for the launcher matrix and external MCP wrapper limitations.

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

The `demos/` directory contains four Docker Compose environments. Each uses isolated networks, volumes, doctrine, and scenario data:

- **Healthcare**: HIPAA/PHI governance and prior-authorization workflows
- **Finance**: trading controls
- **DHS**: coalition data-plane governance, cross-domain release control, and receipted destruction
- **FedRAMP**: sovereign cloud governance, audit integrity, access control, and cross-domain protection

The demo images compile from source in Docker. Running `make build` also copies the host CLI to `./g8e` at the repository root for `g8e demos` commands, but that copy is not required to build the containers. See the [Demos README](../../demos/README.md) for each environment's topology and services.

### Run a demo

Start from the repository root. The start command prints the exact HTTP and HTTPS ports required by the owner-enrollment commands:

```bash
./g8e demos list
./g8e demos start healthcare

# Follow the enrollment commands printed by `demos start`, then check readiness.
./g8e demos status healthcare

# Run one scenario or all scenarios.
./g8e demos run healthcare 1
./g8e demos run healthcare

# Stop the environment while preserving its volumes.
./g8e demos stop healthcare
```

Every demo starts with zero users. Its Operator submits a platform enrollment request and remains not-ready until the first owner enrolls and approves that request. For healthcare, the enrollment flow is:

```bash
./g8e auth enroll user -e localhost:8081 --port 8444
./g8e auth enroll pending -e localhost:8081 --port 8444
./g8e auth enroll approve <operator-request-id> --yes -e localhost:8081 --port 8444
./g8e demos status healthcare
```

Use the ports printed by `demos start` for another environment. `demos clean` and `demos reset` remove persisted demo state; `demos rebuild` stops the environment, rebuilds its images, and restarts it while preserving volumes. Use `./g8e demos <command> --help` before running these lifecycle commands.

### Demo scenarios and ports

| Demo | Scenarios | HTTP | HTTPS | Additional UI |
|---|---:|---:|---:|---:|
| healthcare | 1-4 | 8081 | 8444 | 3001 |
| finance | 1 | 8082 | 8445 | 3002 |
| dhs | 1-4 | 8087 | 8450 | - |
| fedramp | 1-4 | 8088 | 8451 | - |

Run `./g8e demos run <demo> --help` for current scenario names and `./g8e demos pull` to pre-pull the pinned external images used for air-gapped deployment.

---

## Stop, restart, or reset the Compose stack

Stop the stack while preserving named volumes, credentials, and component-local state:

```bash
docker compose --profile bootstrapped --profile evaluation down
```

Start it again with the profiles required by the deployment. After a volume-preserving stop, approved workload identities normally remain available; check `docker compose ps` and `./g8e auth enroll pending` if a workload is not ready.

`docker compose down -v` removes the named volumes and therefore destroys Gateway, Operator, Ensemble, and Dashboard local state, including credentials and enrollment state. Use it only for an intentional cold reset; the workloads must enroll again. The CLI equivalent `./g8e docker clean` is also destructive. See [Docker Gateway Guide](docker_gateway.md) for lifecycle options and [Unified Docker Stack Guide](unified_stack.md) for profile-specific state ownership.

## Post-Bootstrap Actions

After the Gateway is running and the CLI is authenticated:

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
| Ratify | Enforced | Audited | Enforced | `--posture ratify` |
| Notary | Enforced | Enforced | Enforced | `--posture notary` |

These postures select enforcement behavior for gateway admission layers L1-L3. Admitted actions continue through L4 Warden verification and L5 Actuator dispatch. See [Gateway Architecture](../architecture/gateway.md) for the complete five-layer sequence.

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
