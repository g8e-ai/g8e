---
title: g8ep
parent: Components
---

# g8ep

g8ep is the always-on sidecar container for operator management, platform scripts, and security tooling.

It runs as a managed service alongside `g8es`, `g8ee`, and `g8ed` in `docker-compose.yml`. Test running is handled by dedicated per-component test-runner containers; g8ep focuses on operator process supervision, Docker CLI access, network troubleshooting tools, and security scanning.

## Core Responsibilities

- **Operator Process Supervision**: Runs supervisord as PID 1 to manage the g8ep operator process
- **Operator Binary Management**: Downloads and caches the operator binary from g8es blob store
- **Platform Scripts**: Provides Python runtime for management scripts in `scripts/`
- **Security Tooling**: Lazy-installs and runs security scanners (Nuclei, Trivy, Grype, testssl.sh)
- **Network Diagnostics**: Full suite of network tools for troubleshooting

## Process Ownership

**g8ee owns the g8ep operator lifecycle.** While g8ed initiates operator activation after login, it delegates all process management to g8ee via the internal HTTP API. g8ee's `OperatorLifecycleService` and `SupervisorService` handle:

- Starting the operator via XML-RPC to supervisord
- Stopping and restarting the operator
- Persisting the operator API key to platform_settings
- Resetting the operator slot with a fresh API key

This separation ensures a single writer for the operator document (g8ee) while allowing g8ed to trigger lifecycle events.

---

## Container Image

**Base:** `python:3.13-alpine`

The image provides Python 3.13, supervisord for process management, Docker CLI, network diagnostics tools, and security scanning utilities. Test runtimes (Node.js, Go) have been moved to dedicated test-runner containers.

### Key Tools

| Category | Tools |
|----------|-------|
| **Process Management** | supervisor |
| **Network Diagnostics** | curl, wget, nmap, tcpdump, iperf3, mtr, traceroute, bind-tools (dig, nslookup), iproute2, net-tools, socat, whois, ipcalc |
| **System Utilities** | bash, git, rsync, openssh-client, htop, lsof, strace, jq, openssl, uuidgen, unzip, zip |
| **Docker** | docker-cli, docker-cli-compose |
| **Python** | requests, aiohttp |

Component source directories are volume-mounted at runtime — code changes never require a rebuild.

---

## Docker Compose Integration

g8ep is a managed service in `docker-compose.yml`, started alongside the core platform. It depends on `g8ed` being healthy and runs continuously as a sidecar.

**Healthcheck:** `pgrep -x supervisord` — passes once supervisord is running.

**Volume mounts (runtime):**

| Host path | Container path | Purpose |
|-----------|---------------|---------|
| `./components/g8ep/scripts` | `/app/components/g8ep/scripts` | g8ep scripts |
| `./shared` | `/app/shared` | Shared models and constants |
| `./scripts` | `/app/scripts` | Platform scripts |
| `/var/run/docker.sock` | `/var/run/docker.sock` | Docker socket |
| `g8es-ssl` | `/g8es` | SSL certificates (read-only) |
| `~/.ssh/config` | `/home/g8e/.ssh/config` | SSH config for streaming |
| `~/.ssh/known_hosts` | `/home/g8e/.ssh/known_hosts` | SSH known hosts |

**Environment variables:** Loaded from the g8es SSL volume at container startup:

- `G8E_INTERNAL_AUTH_TOKEN` — Inter-service authentication secret
- `G8E_SESSION_ENCRYPTION_KEY` — Session encryption key
- `G8E_SSL_DIR` — Path to SSL directory
- `G8E_PUBSUB_CA_CERT` — CA certificate for pub/sub TLS
- `G8E_OPERATOR_PUBSUB_URL` — g8es pub/sub endpoint

---

## Operator Lifecycle

### The g8ep Operator

The g8ep operator is a special cloud operator (`--cloud --provider g8ep`) that runs inside the g8ep container. Each user has exactly one g8ep operator slot, identified by the `is_g8ep` flag on the operator document.

**Key characteristics:**

- Authenticates via `operator_api_key` (no device link required)
- Connects to `g8e.local` (network alias for g8ed on g8e-network)
- Discovers CA certificate locally at `/g8es/ca.crt`
- Managed by supervisord running as PID 1 in g8ep

### Binary Management

The `g8e.operator` binary lives at `/home/g8e/g8e.operator`. The `fetch-key-and-run.sh` script automatically downloads the platform-matching binary from the g8es blob store when:

- The binary is not present or not executable
- The blob metadata (created_at, size, content_type) differs from the previously downloaded version

The script tracks blob metadata in `/home/g8e/g8e.operator.meta` to detect changes. After running `./g8e operator build`, g8ep automatically re-downloads the updated binary on the next operator start.

### Login-Triggered Activation

On every successful login or registration, g8ed triggers g8ep operator activation via the internal HTTP API. The flow is non-blocking — failures never propagate to the login response.

**Flow:**

```
POST /api/auth/login (or /register)
  └─ session created
       └─ fire-and-forget: activateG8ENodeOperatorForUser(user_id)
            │
            ├─ g8ed delegates to g8ee via InternalHttpClient
            │
            └─ g8ee OperatorLifecycleService.activate_g8ep_operator()
                 │
                 ├─ 1. Query for operator with is_g8ep=true for this user
                 │       → if already ACTIVE/BOUND: done (idempotent)
                 │       → if no slot: done (graceful no-op)
                 │
                 └─ 2. launch_g8ep_operator(api_key)
                      ├─ Persist API key to platform_settings (g8ee authority)
                      ├─ XML-RPC to supervisord: supervisor.startProcess('operator')
                      └─ supervisord runs fetch-key-and-run.sh:
                           ├─ Wait for API key in platform_settings (with retry)
                           ├─ Download binary from blob store if needed
                           └─ exec g8e.operator --endpoint g8e.local --cloud --provider g8ep
```

### Restart Flow

To force a reauth (e.g., when the operator is stuck), the UI or CLI triggers a restart via `POST /api/operators/g8ep/reauth`.

**Flow:**

```
POST /api/operators/g8ep/reauth
  └─ g8ed delegates to g8ee via InternalHttpClient
       └─ g8ee OperatorLifecycleService.relaunch_g8ep_operator()
            │
            ├─ 1. Stop operator via XML-RPC: supervisor.stopProcess('operator')
            │
            ├─ 2. Reset operator slot to AVAILABLE
            │       → Generate new API key
            │       → Clear session bindings
            │       → Update operator document
            │
            └─ 3. Launch with new key
                  → Persist new API key to platform_settings
                  → XML-RPC: supervisor.startProcess('operator')
```

The operation is idempotent — safe to call whether or not the operator is currently running.

---

## Process Management Architecture

**g8ee is the single writer for the g8ep operator lifecycle.** This separation of concerns ensures:

- **Single authority**: g8ee owns operator process management via SupervisorService
- **Delegation pattern**: g8ed triggers lifecycle events but delegates execution to g8ee
- **Clean boundaries**: g8ed handles user-facing auth, g8ee handles process supervision

### SupervisorService

g8ee's `SupervisorService` communicates with supervisord in g8ep via XML-RPC over HTTP:

- Resolves supervisor port and auth token from platform_settings
- Constructs Basic auth header using `g8e-internal:{token}`
- Performs XML-RPC calls to `http://g8ep:{port}/RPC2`
- Handles common supervisor faults (BAD_NAME, ALREADY_STARTED, NOT_RUNNING, SPAWN_ERROR)

### Internal HTTP API

g8ed calls g8ee via `InternalHttpClient` to trigger operator lifecycle operations:

- `POST /api/internal/operators/user/:userId/reauth` — Restart g8ep operator
- Activation happens automatically via g8ee's internal router on login

---

## Test Runners

Test running is handled by dedicated per-component containers:

| Container | Image | Purpose |
|-----------|-------|---------|
| `g8ee-test-runner` | `python:3.12-slim` | g8ee pytest + pyright |
| `g8ed-test-runner` | `node:22-alpine3.23` | g8ed vitest |
| `g8eo-test-runner` | `golang:1.26-alpine3.23` | g8eo tests + operator builds |

The `./g8e test <component>` CLI command routes to the correct test-runner container. The `./g8e operator build` and `./g8e operator stream` commands are also executed through the host CLI, with `stream` running inside `g8ep` via `docker exec`.

See [testing.md](../testing.md) for complete test execution documentation.

---

## Management Commands

The `g8e` CLI script provides several top-level management commands that interact with the `g8ep` environment and platform services:

### LLM Management (`./g8e llm`)
- `llm setup`: Interactive wizard to configure LLM providers (Anthropic, Gemini, OpenAI, Ollama).
- `llm show`: Display current LLM configuration and active models.
- `llm restart`: Force a refresh of the LLM provider state in `g8ee`.

### MCP Management (`./g8e mcp`)
- `mcp config`: Configure external MCP client integrations.
- `mcp status`: Check the status of MCP tool bridging.
- `mcp test`: Run diagnostic tests for MCP client connectivity.

### Web Search Management (`./g8e search`)
- `search setup`: Configure Vertex AI Search for the `search_web` tool.
- `search disable`: Disable the web search capability platform-wide.

### SSH & AWS Credentials
- `ssh setup`: Mounts host SSH credentials (`~/.ssh`) into the `g8ep` container for fleet-wide operator streaming.
- `aws setup`: Mounts host AWS credentials (`~/.aws`) into the `g8ep` container for cloud-integrated operator tools.

---

## Security Scans

Security scan scripts live in `components/g8ep/scripts/security/` and are volume-mounted into the container. Scanning tools (Nuclei, testssl.sh, Trivy, Grype) are lazy-installed on first use by `install-scan-tools.sh` — they are not pre-installed in the image to keep it lean.

All network utilities required by the scan scripts (wget, unzip, nmap, etc.) are pre-installed in the base image.

See [testing.md](../testing.md) for security scan commands and script documentation.

---

## Managing the g8ep Image

Source code changes never require a rebuild. Rebuild only when the image definition changes:

| Changed file | Action |
|-------------|--------|
| `components/g8ep/Dockerfile` | `./g8e platform rebuild g8ep` |

```bash
# Rebuild g8ep image
./g8e platform rebuild g8ep

# Clean g8ep image (full removal)
./g8e platform clean --clean-g8ep
```

---

## Ephemeral SSH Deployment (`operator stream`)

The `stream` subcommand is a Go-native concurrent SSH engine built into the `g8e.operator` binary. It injects the operator to remote hosts without writing to local disk, using `golang.org/x/crypto/ssh`.

**Architecture:**

```
g8ep container
  │
  ├─ Load linux/<arch>/g8e.operator into RAM (once)
  │
  └─ Goroutine pool (default: 50 concurrent)
       │
       ├─ Parse ~/.ssh/config (inline parser)
       ├─ Dial via crypto/ssh + agent / identity files
       ├─ Pipe binary buffer to remote stdin
       └─ Remote: mktemp → cat > $B → chmod +x → trap EXIT → run & wait
                    ↑
              binary deleted on exit
```

The binary is loaded into memory once, then fanned out across goroutines. Each goroutine opens an SSH session, pipes the buffer, and runs a trap-based ephemeral script. Context cancellation (Ctrl+C) propagates to all goroutines.

**Usage:**

```bash
# Build operator binaries first
./g8e operator build

# Stream to hosts
./g8e operator stream host1 host2 --endpoint 10.0.0.1 --device-token dlk_xxx

# Stream from file
./g8e operator stream --hosts /etc/g8e/fleet.txt --concurrency 100 --endpoint 10.0.0.1
```

**SSH config setup:** Configure multiplexing before streaming to many hosts to avoid full key exchange overhead per connection:

```bash
./g8e operator ssh-config  # Applies multiplexing config
```

---

## Operator Panel UI

The g8ep operator appears at the top of the Operator Panel, identified by the `is_g8ep` flag.

**Actions:**

- **Restart g8ep** — `POST /api/operators/g8ep/reauth` (delegates to g8ee)
- **Device Link** — Generate `dlk_` token for remote operators
- **Bind/Unbind** — Bind operator to current web session
- **Stop** — Send shutdown command to running operator

The panel subscribes to SSE events for real-time updates: `OPERATOR_PANEL_LIST_UPDATED`, `OPERATOR_HEARTBEAT_RECEIVED`, and `OPERATOR_STATUS_UPDATED_*` events.

---

## Related Documentation

| Document | Description |
|----------|-------------|
| [testing.md](../testing.md) | Complete testing guide — g8ep environment, test commands, CI workflows |
| [developer.md](../developer.md) | Platform setup, infrastructure, code quality rules |
