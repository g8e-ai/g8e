---
doc_id: getting_started
title: Getting Started
audience: new users and platform evaluators
status: current
last_updated: 2026-10-06
version: v2.3.2
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
- Node.js 22+ with npm (only to rebuild the Console or evaluation explorer; a fresh clone builds from the committed embeds)
- A modern browser with WebAuthn support (or use headless enrollment)

### Clone the repository

```bash
git clone --depth 1 https://github.com/g8e-ai/g8e.git
cd g8e
```

If build tools are missing, run the setup script from this checkout:

- **Linux:** `bash scripts/linux-setup.sh --build-only`
- **macOS:** `bash scripts/macos-setup.sh --build-only`
- **Windows:** `pwsh scripts/windows-setup.ps1`

For a Gateway in WSL that LAN Operators must reach, see the Windows-to-WSL port-forwarding helper `scripts/configure-gateway-lan.ps1` in [Connect Operator to Gateway](connect_operator_to_gateway.md).

Linux and macOS setup without `--build-only` also installs the contributor toolchain, including Python test and lint dependencies. Pass `-y` to accept installs without prompting, for example `bash scripts/linux-setup.sh --build-only -y`. After setup, open a new terminal or source the shell profile printed by the script, then return to the repository root.

### Gateway-only track

The Gateway-only track has no Python, Ollama, or model SDK requirement. It
requires Git, Go 1.26.6, and Make. When `console/dist` and
`evaluation-explorer/dist` are absent (a fresh clone), `make build` uses the
committed Console and explorer embeds, so Node.js is not needed.
Build and start the Gateway on localhost:

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

### Full platform track: Gateway + Operators + Ensemble

For the complete platform with local Operators and the agentic Ensemble
(g8ee), install Python 3.12+ dependencies once, then start the workloads:

```bash
make ensemble-env # runtime only; bootstraps uv and installs Python 3.12+ as needed
make full-setup
```

Use `make dev-python` instead when you also need the Ensemble test and lint
dependencies. The Gateway and Operator build alone does not need either Python
target.

This interactively prompts you for:
- Operator working directories
- Model storage location (for the Provenance and Inference Operators)
- Ollama endpoint (for LLM backends)

Then it:
- Starts the Gateway in the background
- Launches four Operators (Provenance, Observer, Inference, Data) and the Ensemble
- Prints commands to enroll and approve workloads

### Enroll the first owner

From another terminal, authenticate with the Gateway:

```bash
./g8e auth enroll user -e localhost
```

This opens your browser for the WebAuthn passkey ceremony and installs the Gateway Root CA in your OS trust store.

No browser on this machine (SSH session, container, CI)? Enroll headless instead:

```bash
./g8e auth enroll user -e localhost --headless
```

This creates an mTLS-only CLI identity with no passkey ceremony and no OS
trust-store changes. It cannot sign in to the browser console, but it fully
drives the CLI.

### Approve workload enrollments

List pending enrollment requests and approve them:

```bash
./g8e auth enroll pending
./g8e auth enroll approve <request-id> --yes
```

Or approve all at once:

```bash
./g8e auth enroll approve --all --yes
```

### Verify the stack is healthy

```bash
make status
./g8e gw status
./g8e operator list
./g8e tui
```

`g8e tui` needs an enrolled CLI (`g8e auth enroll user`). It lists pending L3 approvals and can approve them: press `tab` to focus the queue, then `enter` to open the browser WebAuthn page.

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
| Node.js and npm | 22+ | Required only to rebuild the Console or evaluation explorer embeds; `make build` and `make up` use the committed embeds when `dist/` is absent |
| Python | 3.12+ | Required for `make full` / `make full-setup` (Ensemble); `make ensemble-env` installs runtime dependencies, and `make dev-python` adds test and lint dependencies. Also used for protocol library development |

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
git clone --depth 1 https://github.com/g8e-ai/g8e.git
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

**Start the Gateway with its embedded Operator acting as every role:**

```bash
make ensemble-env # one-time runtime setup
make gw
```

The embedded Operator always includes Data, so chat and governed commands work against `embedded-operator` even when `--roles embedded` is the only role. When a dedicated Data Operator is also connected (for example under `make full`), eval and discovery prefer it over the embedded one. `make gw` starts only the Gateway, in a single process, with `--roles provenance,observer,inference,data` on its embedded Operator. There are no separate Operator processes, working directories, or enrollments, and g8ee is not started. It reads `G8E_HOSTNAME` and `G8E_OLLAMA_ENDPOINT` from `.env` the same way `make full` does, except that an unset Ollama endpoint keeps the Gateway's default (`http://127.0.0.1:11434`) instead of failing. The provenance model store is detected from host storage. Override it, or preview the command, with:

```bash
make gw GW_ARGS='--model-storage-root /srv/ollama/models'
make gw GW_ARGS='--dry-run'
```

Roles are fixed when the Gateway starts. `make gw` stops the launcher-managed local Operators and g8ee, then stops and starts the Gateway with every capability on its embedded Operator. Runtime state and enrolled identities are preserved. Startup ends with the same output as `gw status`, which `make full` also prints. `gw status` has one output format whichever target started the Gateway: a Gateway row, an Operators table with one row per connected Operator ID, an Operator flags table, enrollment counts (`?` means unavailable), and the Console URL.

```text
Gateway  online (PID 29424)

Operators
  OPERATOR ID        TYPE      STATUS  HOST   HTTP PORT  CAPABILITIES                                 DIRECTORY
  embedded-operator  embedded  active  local  8080       embedded,data,inference,provenance,observer  /home/you/g8e

Operator flags
  OPERATOR ID        CAPABILITY  FLAG                             VALUE
  embedded-operator  inference   --inference-ollama-endpoint      http://192.168.1.2:11434
  embedded-operator  inference   --inference-keep-alive           (default)
  embedded-operator  provenance  --model-storage-root             /mnt/d/ai/Ollama/models
  embedded-operator  provenance  --provenance-operator-id         (default)
  embedded-operator  observer    --provider-boundary-observer-id  (default)

Enrollments  users 1 · pending 0 · apps 0 · dashboards 0

Console  https://localhost:8443/console/
```

The Operators table lists `embedded` and `remote` Operators together, embedded first, then by ID. `HTTP PORT` is the Gateway HTTP port for the embedded Operator and the Gateway port a remote Operator dials. `DIRECTORY` is the Operator working directory. The Operator flags table has one row per start flag: `--log`, `--cloud`, `--provider`, and `--no-git` appear only when set to a non-default value, and each capability's flags appear for every Operator holding that capability. `(default)` means the flag was not set at start. The Gateway lists only values an Operator reported at registration, plus the launch profile for its embedded Operator, so a remote Operator's `--endpoint`, `--provenance-operator-id`, `--provider-boundary-observer-id`, and `--inference-keep-alive` are not listed. Before CLI enrollment, the local embedded Operator is still listed from its launch profile, and the output states that remote Operators and enrollments are unavailable. Authenticated registry records take precedence when available; an unreachable registry is reported as unavailable rather than empty. On a fresh Gateway, enroll the first user to claim the embedded Operator. Stop it with `make down`. Remote Operators must be stopped on their own hosts.

**Start the Gateway + Operators + Ensemble:**

```bash
make ensemble-env # one-time runtime setup
make full
```

`make full` reads the repository-root `.env` and starts without launcher prompts. It stops and starts the Gateway with only the `embedded` role, then launches separate provenance, observer, inference, and data Operators in unique working directories with their own enrollment identities, plus g8ee. This also switches a Gateway previously started by `make gw` back to separate Operators. Set `G8E_OLLAMA_ENDPOINT` to the approved HTTP(S) provider URL. `G8E_HOSTNAME` selects the Gateway hostname for Operators, Ensemble, browser approval links, CORS, and passkey origins; if omitted, the host launcher retains `g8e.local`. Exported process variables take precedence, including empty values. Missing or empty Ollama endpoints and invalid hostnames fail before Gateway startup. The launcher parses quoted values and comments as data, without shell execution or variable interpolation, and leaves `.env` unchanged.

The four roles run locally by default. Optional `G8E_PROVENANCE_HOST`, `G8E_OBSERVER_HOST`, `G8E_INFERENCE_HOST`, and `G8E_DATA_HOST` select remote hosts. Remote roles print commands to run on those hosts; the launcher does not connect to them. The Gateway and Ensemble always run locally. The hostname must resolve to this Gateway, be reachable by the selected hosts, and match its existing TLS certificate.

Operator working directories default to `~/.ollama/g8e/<role>`, and the local Ollama model store is detected from host storage. Paths, ports, and model roles are not read from `.env`, following [INV-ENV-04](../devs/devs.md#environment-inv-env). Override paths with explicit launcher flags:

```bash
make full FULL_ARGS='--model-storage-root /srv/ollama/models --data-working-dir /srv/g8e/data'
make full FULL_ARGS='--dry-run'
```

`--dry-run` previews the Gateway and workloads without launching services or writing runtime state; Make still builds the binary. Use `--<role>-working-dir` for any role and `--env-file` for another dotenv file. Each Operator requires a separate directory. The Ensemble uses `.local.dev/full/ensemble`.

Use `make full-setup` for the previous interactive flow, which prompts for Operator hosts, working directories, model storage, and the Ollama endpoint. It does not read `.env`.

Workloads may still await owner enrollment and approval. Stale local identities fail unattended startup with instructions to use `make full RESET_IDENTITIES=1`; see [workload identity recovery](reset_workload_identity.md).

### Gateway configuration

The gateway starts in Doctrine mode by default (L1 enforced, L2/L3 audited). To specify a different security posture:

```bash
./g8e gw start --posture doctrine    # default
./g8e gw start --posture consensus   # L1/L2 enforced, L3 audited
./g8e gw start --posture ratify      # L1/L3 enforced, L2 audited
./g8e gw start --posture notary      # L1/L2/L3 strictly enforced
```

`make clean` removes build and test artifacts and does not reset runtime state, trust, or databases. To intentionally reset the Gateway runtime, run `./g8e gw clean`, which archives `.g8e/` and requires fresh owner and workload enrollment.

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

Remove volumes (deletes persisted runtime data):

```bash
docker compose down -v
```

---

## Use the Protocol Library (Go Module or Python Package)

If you only need the g8e wire protocol, constants, models, enums, or protobuf definitions for your own client or service, you can consume the published packages without building the full platform.

### Go module

```bash
go get github.com/g8e-ai/g8e/v2@v2.3.2
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
pip install g8e==2.3.2
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

For Docker deployments where the Gateway serves HTTPS on a different port than the default:

```bash
./g8e auth enroll user -e localhost --port 9443
```

Replace `9443` with the published HTTPS port. `--endpoint` (`-e`) selects the HTTP discovery endpoint, not the HTTPS API port; when discovery is also remapped, use `-e localhost:<http-port>`.

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

## Post-Bootstrap Actions

After the Gateway is running and the CLI is authenticated:

```bash
./g8e gw status           # Gateway health, Operators (type, capabilities, start flags), enrollment counts
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
