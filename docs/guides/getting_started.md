---
doc_id: getting_started
title: Getting Started
audience: new users and platform evaluators
status: current
last_updated: 2026-09-28
version: v2.2.3
owners:
  - docs/guides/getting_started.md
  - Makefile
  - docker-compose.yml
related:
  - docs/guides/unified_stack.md
  - docs/guides/docker_gateway.md
  - docs/guides/connect_operator_to_gateway.md
  - docs/ensemble/index.md
  - docs/architecture/console.md
  - docs/architecture/auth.md
when_to_read: Setting up g8e for the first time, bootstrapping a demo environment, or understanding the platform's core execution model.
do_not_use_for:
  - Architecture details — see docs/architecture/gateway.md
  - Production deployment — see docs/guides/unified_stack.md
  - Authentication details — see docs/architecture/auth.md
---

# Getting Started

---

## Overview

g8e is a zero-trust execution platform for agentic infrastructure. Its two core execution components are:

- **g8e Gateway**, the central Policy Decision Point (PDP): PKI authority, state store, pub/sub broker, and admission APIs.
- **g8e Operator**, the Policy Execution Point (PEP) for the runtime where that Operator process runs: an outbound-only mTLS connection to the Gateway, local audit vault, and governed tool execution.

Both roles use the same `g8e` binary, selected by the `gw` or `operator` subcommand. The unified stack also includes the first-party Agentic Ensemble (g8ee); the Gateway serves the browser console.

---

## Quick Start (Native Host Build)

**The recommended way to run g8e is natively on your machine — no Docker required.**

### Prerequisites

- Go 1.26.6
- Make
- Git
- Node.js 22+ with npm (for the evaluation explorer)
- A modern browser with WebAuthn support (or use headless enrollment)

If you don't have these tools, run the setup script for your platform:

- **Linux:** `bash scripts/linux-setup.sh`
- **macOS:** `bash scripts/macos-setup.sh`
- **Windows:** `pwsh scripts/windows-setup.ps1`

### 1. Clone the repository

```bash
git clone https://github.com/g8e-ai/g8e.git
cd g8e
```

### 2. Start the Gateway only

The simplest path: build and start the Gateway on localhost.

```bash
make up
```

This is equivalent to:

```bash
make build
./g8e gw start
```

The Gateway starts on:
- **HTTP (discovery):** `http://localhost:8080`
- **HTTPS/mTLS (API):** `https://localhost:8443`

### 3. Or: Start the full stack (Gateway + Operators + Ensemble)

For the complete platform with local Operators and the agentic Ensemble (g8ee):

```bash
make full
```

This interactively prompts you for:
- Operator working directories
- Model storage location (for the Provenance and Inference Operators)
- Ollama endpoint (for LLM backends)

Then it:
- Starts the Gateway in the background
- Launches three Operators (Data, Provenance, Observer) and the Ensemble
- Prints commands to enroll and approve workloads

### 4. Enroll the first owner

From another terminal, authenticate with the Gateway:

```bash
./g8e auth enroll user -e localhost
```

This opens your browser for the WebAuthn passkey ceremony and installs the Gateway Root CA in your OS trust store.

### 5. Approve workload enrollments

List pending enrollment requests and approve them:

```bash
./g8e auth enroll pending
./g8e auth enroll approve <request-id> --yes
```

Or approve all at once:

```bash
./g8e auth enroll approve --all --yes
```

### 6. Verify the stack is healthy

```bash
./g8e gw status
./g8e operator list
./g8e tui
```

Visit the browser console at `https://localhost:8443/console/` to see the passkey sign-in and approvals UI.

### Stop the platform

```bash
make down
```

This stops the Gateway (and all local processes started by `make full`). Runtime state in `.g8e/` is preserved.

---

## Prerequisites (Detailed)

There are two ways to run g8e: **natively on your host** (compile and run directly) or **in Docker** (no local Go toolchain required). This guide focuses on the native path, which is faster for development and evaluation.

### Native host build

| Requirement | Version | Notes |
|---|---|---|
| Go | 1.26.6 | Required to build from source |
| Make | Any recent | Required to run Makefile targets |
| Git | Any recent | Required to clone the repository |
| Node.js and npm | 22+ | Required to build the evaluation explorer (once, at build time) |
| Python | 3.10+ | Optional, only for protocol library development |

### Docker path (no local toolchain required)

| Requirement | Version |
|---|---|
| Docker | 24.0+ |
| Docker Compose | v2 |

The Docker build compiles inside the builder stage. No local Go installation needed.

---

## Get the Source

Clone the repository:

```bash
git clone https://github.com/g8e-ai/g8e.git
cd g8e
```

---

## Build

### Build locally (native host)

```bash
make build
```

This builds the `g8e` binary for your current platform and places it at:
- Repository root: `./g8e`
- Platform-specific: `bin/g8e-<os>-<arch>`

The binary is statically linked with zero runtime dependencies.

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

```bash
make docker-up
```

This runs `docker compose up -d --build`, which starts all platform services in containers.

To get the CLI binary from the container:

```bash
docker cp g8e-gateway:/g8e ./g8e
```

Or download it directly from the gateway's bootstrap endpoint:

```bash
curl -fSLO http://localhost:8080/.well-known/g8e/bin/g8e-linux-amd64
chmod +x g8e-linux-amd64
```

---

## Run the Gateway

### Run natively on localhost

**Start the Gateway only:**

```bash
make up
```

**Start the Gateway + Operators + Ensemble:**

```bash
make full
```

`make full` prompts interactively for operator configuration, then launches everything in the background.

### Gateway configuration

The gateway starts in Doctrine mode by default (L1 enforced, L2/L3 audited). To specify a different security posture:

```bash
./g8e gw start --posture doctrine    # default
./g8e gw start --posture consensus   # L1/L2 enforced, L3 audited
./g8e gw start --posture ratify      # L1/L3 enforced, L2 audited
./g8e gw start --posture notary      # L1/L2/L3 strictly enforced
```

The default plain-HTTP port is `8080` and serves bootstrap and PKI discovery. The default HTTPS port is `8443` and serves authenticated APIs, MCP, and the Web Console.

### Monitor the Gateway

Check health:

```bash
./g8e gw status
```

View logs in real time:

```bash
./g8e gw logs -f
```

### Runtime state

All runtime state is written to `.g8e/` in the working directory:

| Path | Contents |
|---|---|
| `.g8e/pki/` | CA hierarchy and trust bundles |
| `.g8e/secrets/` | Bootstrap secrets and vault key |
| `.g8e/data/` | SQLite databases and blobs |
| `.g8e/vault/` | Encrypted audit vault |
| `.g8e/logs/` | Component logs |

### Stop the Gateway

```bash
make down
```

---

## Run in Docker Compose

The root `docker-compose.yml` starts the full unified stack: Gateway, Data Operator, Inference Operator, and Agentic Ensemble.

### Start the stack

```bash
cp .env.example .env
docker compose up -d --build
```

### Enroll and approve

Get the CLI binary:

```bash
docker cp g8e-gateway:/g8e ./g8e && chmod +x ./g8e
```

Authenticate:

```bash
./g8e auth enroll user -e localhost
```

List and approve:

```bash
./g8e auth enroll pending
./g8e auth enroll approve <request-id> --yes
```

### Verify the stack

```bash
docker compose ps
./g8e gw status
```

Service endpoints:

- **Gateway bootstrap and health:** `http://localhost:8080`
- **Gateway HTTPS/mTLS API and MCP:** `https://localhost:8443` (MCP at `/mcp`)
- **Console (browser):** `https://localhost:8443/console/`
- **Ensemble API:** `http://localhost:8000`
- **Public spectator private ingest:** `http://127.0.0.1:8081`
- **Public spectator anonymous read/SSE:** `http://127.0.0.1:8082`
- **Evaluation explorer:** `http://127.0.0.1:5173`

### Stop the stack

```bash
docker compose down
```

Preserve volumes:

```bash
docker compose down -v
```

---

## Use the Protocol Library (Go Module or Python Package)

If you only need the g8e wire protocol, constants, models, enums, or protobuf definitions for your own client or service, you can consume the published packages without building the full platform.

### Go module

```bash
go get github.com/g8e-ai/g8e/v2@v2.2.3
```

Import in your Go code:

```go
import (
    "github.com/g8e-ai/g8e/v2/protocol"
    "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/common/v1"
)
```

### Python package

```bash
pip install g8e==2.2.3
```

Or pinned to latest:

```bash
pip install g8e
```

Use in your Python code:

```python
from g8e.constants import EVENTS, ComponentName
from g8e.models import RequestContext

print(ComponentName.CLIENT)  # "client"
```

Requires Python 3.10+. See the [Protocol Library documentation](../architecture/protocol.md) for the full API reference.

---

## Authenticate the CLI

After the gateway is running (locally or in Docker), authenticate to bootstrap the PKI hierarchy and issue mTLS credentials:

```bash
./g8e auth enroll user
```

This installs the gateway Root CA into your OS trust store and opens the browser for the WebAuthn passkey ceremony.

For Docker deployments with different HTTP and HTTPS ports:

```bash
./g8e auth enroll user -e localhost:8080 --port 8443
```

---

## Connect an Operator

### Start a remote operator

To connect an Operator on a remote host to the Gateway, provide the gateway hostname:

```bash
./g8e operator start -e <gateway-hostname>
```

The Operator will automatically initiate platform enrollment. The gateway holds the enrollment request in pending state until an enrolled owner approves it.

---

## MCP Agent Integration

g8e integrates with popular AI agent binaries to provide governed MCP tool access.

### Launch an agent with governance

```bash
./g8e mcp agent run claude
./g8e mcp agent run goose
```

### List supported agents

```bash
./g8e mcp agent list
```

### Show agent configurations

```bash
./g8e mcp agent show claude
```

---

## Industry Demos

The `demos/` directory contains four Docker Compose environments: healthcare, finance, DHS, and FedRAMP.

### Run a demo

```bash
./g8e demos list
./g8e demos start healthcare

# Follow the enrollment commands, then check readiness
./g8e demos status healthcare

# Run scenarios
./g8e demos run healthcare 1
./g8e demos run healthcare
```

Demo ports:

| Demo | HTTP | HTTPS | Additional UI |
|---|---|---|---|
| healthcare | 8081 | 8444 | 3001 |
| finance | 8082 | 8445 | 3002 |
| dhs | 8087 | 8450 | - |
| fedramp | 8088 | 8451 | - |

See [demos/README.md](../../demos/README.md) for full details.

---

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

These postures select enforcement behavior for gateway admission layers L1-L3. Admitted actions continue through L4 Warden verification and L5 Actuator dispatch. See [Gateway Architecture](../architecture/gateway.md) for details.

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
