# Builds, Dependencies, and Startup Sequence

This document explains the g8e component dependency chain, the reason each component exists in the order it does, and what every build and startup step actually produces. Understanding why the order matters requires understanding what each component is and what it provides to the rest of the stack.

---

## The Operator is the Heart of the Platform

Every piece of g8e infrastructure ultimately depends on the `g8e.operator` binary. The same binary runs in two fundamentally different modes:

**`--listen` mode (g8es — `g8es`):** The Operator binary becomes the platform's central persistence layer. It runs as a local server inside the `g8es` container and provides:
- A SQLite-backed **document store** (`/db/{collection}/{id}`) — all domain data: users, sessions, operators, cases, investigations, memories. **Authenticated via `X-Internal-Auth`.**
- A **KV store** (`/kv/{key}`) — platform settings, binary blobs, ephemeral session data, attachments. **Authenticated via `X-Internal-Auth`.**
- A **blob store** (`/blob/{namespace}/{id}`) — large file storage with TTL support, including operator binaries for remote deployment (namespace `operator-binary`). **Authenticated via `X-Internal-Auth`.**
- A **pub/sub WebSocket broker** (`/ws/pubsub`) — the real-time message bus connecting operators on remote machines back to g8ee and g8ed. **Connection authenticated via `X-Internal-Auth`.**
- A **TLS certificate authority** — generates and serves the platform CA cert at `/ssl/ca.crt` at startup; this cert is the root of trust for all mTLS in the stack

g8ee and g8ed wait for g8es to be healthy before starting. g8ep starts independently and builds its own operator binary from source. Without g8es, there is no database, no pub/sub, and no TLS trust anchor.

**Standard mode (g8eo — deployed Operators):** The Operator runs on a remote target system and connects outbound to g8es. It executes commands, manages files, streams heartbeat telemetry, and maintains the local audit vault and ledger on the target machine.

Operator binaries for remote deployment (linux/amd64, linux/arm64, linux/386) are cross-compiled and UPX-compressed at g8es image build time and baked into the image. On container startup, g8es uploads all 3 binaries to the blob store automatically. g8ed serves them on demand from the blob store. The `./g8e operator build` and `./g8e operator build-all` commands in g8ep can override the baked binaries by uploading fresh builds to the blob store. g8ep builds its own amd64 binary from source using the g8eo Makefile at container startup — this keeps g8ep self-contained as the test and local-operator runner.

---

## Component Overview

| Container | Internal name | Image built from | What it is |
|---|---|---|---|
| `g8es` | g8es | `components/g8es/Dockerfile` | Operator binary in `--listen` mode; platform DB + pub/sub + blob store (including operator binaries) |
| `g8ee` | g8ee | `components/g8ee/Dockerfile` | Python/FastAPI AI backend |
| `g8ed` | g8ed | `components/g8ed/Dockerfile` | Node.js web frontend; single external HTTPS entry point |
| `g8ep` | g8ep | `components/g8ep/Dockerfile` | Alpine sidecar; manages operator processes and streams operators to remote hosts. Does not contain a Go toolchain — the operator binary is built in `g8eo-test-runner` when `./g8e operator build` is invoked. |

---

## Build Dependency Chain

All images build in parallel — no component has a build-time dependency on any other. The sequencing constraint is purely at container startup time.

```
[1] ALL images build in parallel:
        g8es      (multi-stage: Go builder cross-compiles amd64/arm64/386 + UPX → alpine runtime)
        g8ee        (no build deps)
        g8ed       (no build deps)
        g8ep   (no build deps)

[2] g8es container starts first
        │
        │  execs: g8e.operator --listen
        │    → generates platform CA cert → writes to g8es-data volume
        │    → opens SQLite store, starts HTTPS and WSS servers
        │    → health check passes
        ▼
[3] g8ee, g8ed start in parallel (depends_on: g8es healthy):
        g8ee       — connects to g8es pub/sub, reads settings from KV store
        g8ed      — reads TLS certs from g8es-data volume

[3] g8ep starts independently (no depends_on):
        → starts supervisord (no Go toolchain in image)
        → operator binary is built on demand in g8eo-test-runner via ./g8e operator build
```

---

## Step-by-Step: What Happens and Why

### Step 1 — Build all images in parallel

All component images have no build-time dependencies on each other and build in parallel.

**g8es image build:** Uses a multi-stage Dockerfile. The builder stage installs Go and UPX, then cross-compiles the `g8e.operator` binary for all 3 target architectures (`linux/amd64`, `linux/arm64`, `linux/386`) with `-trimpath`, `-buildvcs=false`, and UPX `--best --lzma` compression. The platform version is injected via `-ldflags "-X main.version=${VERSION}"` — the `VERSION` build arg is set from the `G8E_VERSION` environment variable (read from the `VERSION` file by `build.sh`). The final stage is a minimal Alpine image that copies the amd64 binary to `/usr/local/bin/` (to run g8es itself in `--listen` mode) and all 3 compressed binaries to `/opt/operator-binaries/` (for blob store upload at startup). The Go toolchain is not present in the runtime image.

**All other images** (g8ep, g8ee, g8ed) build independently with no dependency on g8es.

**g8ee image build:** Uses a multi-stage Dockerfile based on Python 3.13-slim. It installs dependencies into a prefix in the builder stage to keep the final runtime image minimal.

**g8ed image build:** Uses a multi-stage Dockerfile based on Node.js 22-alpine. The builder stage runs `npm ci` and prunes dev dependencies. The final stage installs only curl (for healthchecks) — npm is not present in the runtime image.

**g8ep image build:** Based on `python:3.13-alpine`. Installs Python and network/security tooling (Docker CLI, supervisor, etc.). Go is **not** installed in g8ep. The operator binary is compiled on demand in the `g8eo-test-runner` container (Go 1.26-alpine3.23) via `./g8e operator build`, which uploads the fresh build to the g8es blob store.

**Trigger:**
```bash
./g8e platform build
```

---

### Step 2 — Start g8es (`g8es`)

**Why first among running services:** g8es is the dependency of everything else. It provides:

1. **The TLS CA** — on first start, the Operator binary (in `--listen` mode) generates a self-signed CA and writes it to `/data/ssl/`. All other containers mount `g8es-data:/g8es:ro` and read the CA cert from `/g8es/ssl/ca.crt`. Without this, mTLS cannot be established between any components. See [Security Architecture > Workstation CA Trust](security.md#workstation-ca-trust) for how users trust this CA.

2. **The document store** — g8ee and g8ed read and write all domain data (users, sessions, operators, cases, investigations) exclusively through g8es's HTTP API. All requests are authenticated via the `X-Internal-Auth` header. Neither component holds its own relational database.

3. **The KV and Blob stores** — platform settings, session data, operator binary blobs, file attachment metadata, and large binary blobs are all stored here. All requests are authenticated via the `X-Internal-Auth` header.

4. **The pub/sub broker** — operators on remote machines maintain a persistent WebSocket connection to g8es (WSS/TLS). Command dispatch and result delivery for the AI flow through this broker. g8ee and g8ed also connect to this broker for real-time event distribution.

5. **The operator binaries** — cross-compiled and UPX-compressed at image build time, baked into the image at `/opt/operator-binaries/`. On container startup, the entrypoint uploads all 3 binaries (linux-amd64, linux-arm64, linux-386) to the blob store (namespace `operator-binary`). Served on demand via `GET /blob/operator-binary/{os}-{arch}`. The `./g8e operator build` commands in g8ep can override these by uploading fresh builds.

**What it runs:**
```
/entrypoint.sh
  → exec g8e.operator --listen --data-dir /data --ssl-dir /ssl
             --http-listen-port 9000 --wss-listen-port 9001
```

Port 9000 (HTTPS) is used by g8ee and g8ed for all internal API traffic. Port 9001 (WSS/TLS) is used by remote Operators for the pub/sub connection.

**Health check:** `curl https://localhost/health` — passes when the HTTP server is ready and the SQLite store is open.

---

### Step 3 — Start g8ee, g8ed, and g8ep in parallel (`depends_on: g8es healthy`)

All three services start simultaneously once g8es is healthy.

---

### Step 4 — Start g8ee (`g8ee`)

**Why after g8es:**

- g8ee's startup sequence opens a persistent WebSocket connection to g8es's pub/sub broker to subscribe to heartbeat and command-result channels. If g8es is not healthy, this connection fails and g8ee will not start successfully.
- g8ee reads all platform settings (LLM provider, API keys, feature flags) from g8es's KV store at startup.

**What it runs:**
```
exec uvicorn app.main:app --host 0.0.0.0 --port 443 \
    --ssl-keyfile "${SSL_DIR}/server.key" \
    --ssl-certfile "${SSL_DIR}/server.crt"
```

g8ee is an internal-only service. g8ed proxies all traffic to it; g8ee is never exposed on a public port. It runs as non-root user `g8e` (UID 1001).

**Health check:** `curl -f -k https://localhost/health` — passes when g8es connectivity is confirmed.

---

### Step 5 — Start g8ed (`g8ed`)

**Why after g8es (and implicitly after g8ee):**

- g8ed reads its TLS certificates from the `g8es-data` volume (`/g8es/ssl/`) to terminate HTTPS on ports 443/80. Without g8es having initialized and written those certs, g8ed cannot serve TLS.
- g8ed proxies all AI requests to g8ee's internal HTTP API. While g8ed can start before g8ee is fully ready, the setup wizard and dashboard will not function correctly until g8ee is healthy.
- g8ed subscribes to g8es pub/sub to receive operator events (heartbeats, command results, status changes) for SSE fan-out to the browser.

**What it runs:**
```
CMD ["node", "server.js"]
```

g8ed is the only service with external ports (`443:443`, `80:80`). It terminates TLS, handles passkey authentication, manages operator WebSocket connections (Gateway Protocol — bridging remote g8eo operators to g8es pub/sub), and serves the browser dashboard. It runs as non-root user `g8e` (UID 1001).

g8ep mounts `/var/run/docker.sock` for operator builds and streaming. g8ed manages the g8ep operator process via Supervisor XML-RPC over the internal network (port 443), not via docker exec.

**Health check:** `curl -f -k https://localhost/health` — passes when connectivity to g8es is confirmed. External TLS health is checked via the same endpoint.

---

### Step 6 — Start g8ep

**Starts independently** — g8ep has no `depends_on` constraint. It does not require g8es to build its binary.

**What it does on startup:**
1. Compiles the `g8e.operator` binary from source (`/app/components/g8eo`) using the g8eo Makefile, outputting to `/home/g8e/g8e.operator`. If `/g8es/ssl/ca.crt` is already present (g8es initialized), it is used to configure the operator's trust anchor.
2. Starts `supervisord` as PID 1. The `[program:operator]` entry is registered with `autostart=true` — the operator process starts automatically when supervisord launches. g8ed controls it via Supervisor XML-RPC over the internal network (port 443) when a user launches a local operator session.

**Why g8ep builds its own binary:** Having Go installed and building from source using the g8eo Makefile ensures the binary used for local operator sessions always matches the current source. The `./g8e operator build` command can force a rebuild at any time. It runs as non-root user `g8e` (UID 1001) with `cap_drop: ALL` and `no-new-privileges: true`.

**Health check:** `pgrep -x supervisord`

---

## Runtime Startup Order (Enforced by `depends_on`)

Docker Compose enforces this dependency graph via `condition: service_healthy`:

```
g8es (healthy)
    ├── g8ee      (depends_on: g8es healthy)
    └── g8ed     (depends_on: g8es healthy)

g8ep  — no depends_on; builds its own binary from source at startup
```

The `build.sh` script (`scripts/core/build.sh`) manages the full lifecycle. It ensures that when `rebuild` or `reset` is called, `g8es` is started first and its health is verified before starting dependent services.

---

### CI Workflow (`build-and-test.yml`)
Triggered on every push to `main` and on pull requests to `main`.
1. **Platform Setup:** Executes `./g8e platform setup` to build all images and start the full platform (g8es, g8ee, g8ed, g8ep) with health checks.
2. **Test Runner Build:** Executes `./g8e platform rebuild test-runners` to build dedicated per-component test-runner containers.
3. **Component Tests:** Runs `g8ee`, `g8ed`, and `g8eo` test suites via `./g8e test <component>` inside their respective test-runner containers.

---

## Build vs. Start Commands

| Command | What it does |
|---|---|
| `./g8e platform setup` | Full first-time setup: no-cache build of all images (g8es cross-compiles all operator binaries), start platform, wait for health checks. Does not wipe data volumes. Recommended for first-time setup. |
| `./g8e platform build` | Rebuild with layer cache: stops containers, rebuilds all images in parallel (with cache), starts g8es first then all remaining services, waits for health checks |
| `./g8e platform start` | Start existing images with no rebuild — services must already be built |
| `./g8e platform stop` | Stop containers; preserves all volumes and images |
| `./g8e platform restart` | Stop and start without rebuilding; picks up config changes that don't require a new image |
| `./g8e platform rebuild [component]` | Rebuild one or more specific components, e.g. `./g8e platform rebuild g8ed` |
| `./g8e platform reset` | Wipe data volumes + full rebuild — equivalent to starting fresh from scratch |
| `./g8e platform wipe` | Remove data volumes and restart — images are reused, data is erased |
| `./g8e platform clean` | Remove all managed Docker resources: containers, images, volumes, networks, build cache |

---

## First-Time Setup Order

On a fresh checkout with no existing images:

```
1. ./g8e platform setup   (or: docker compose up)
   └─ [1] all images build in parallel (no cache)
          g8es cross-compiles all 3 operator arches + UPX compression (takes longest)
   └─ [2] g8es starts
          → generates platform CA cert → writes to g8es-data volume
          → uploads 3 compressed operator binaries to blob store
          → health check passes
   └─ [3] g8ee, g8ed start in parallel (depends_on: g8es)
          g8ep starts independently: make build → /home/g8e/g8e.operator
   └─ [4] all health checks pass
   └─ https://localhost is live

2. Open https://localhost
   └─ setup wizard configures: hostname, AI provider, admin account, passkey
```

Works identically with Docker Desktop — no `./g8e` CLI required for first-time setup.

---

## The g8ep Operator and Supervisord

The g8ep container runs `supervisord` as PID 1. The supervisor config pre-registers a `[program:operator]` entry with `autostart=true` — the operator process starts automatically when supervisord launches.

When a user launches a local operator session from the dashboard, g8ed:
1. Persists the operator API key to the platform_settings document in g8es
2. Calls Supervisor XML-RPC to start the operator process over the internal network (port 443)

The Operator process then starts inside g8ep with `--endpoint g8e.local`. It reads the API key from the `G8E_OPERATOR_API_KEY` environment variable (fetched from g8es platform_settings by `fetch-key-and-run.sh`). The CA certificate is loaded from the local SSL volume at `/g8es/ca.crt` — the operator discovers it automatically without a network fetch. From this point it is indistinguishable from any other operator — heartbeats flow through g8es pub/sub, commands are dispatched via the same channels.

This is why g8ep mounts `/var/run/docker.sock` — for operator builds and streaming. g8ed's `G8ENodeOperatorService` manages the g8ep operator via Supervisor XML-RPC over the internal network, which does not require Docker socket access.

---

## Platform Network

All services run on a single internal Docker bridge network: `g8e-network` (`g8e-network` in compose).

g8ed is given network aliases `localhost` and `g8e.local` so that the Operator binary (running inside g8ep) can reach g8ed at `g8e.local:443` — the default platform endpoint used by operators when no `--endpoint` flag is given.

External traffic enters only through g8ed on ports 443 and 80. All other services (G8es, g8ee) are internal-only and not exposed on any host port.

---

## Volume Dependencies

`g8es-data` is the single most critical volume in the platform. Its contents gate multiple service startups:

**`g8es-data` volume (`g8es-data`):**

| Path | Written by | Read by | Why |
|---|---|---|---|
| `/data/ssl/ca.crt` | g8es (on first init) | g8ee, g8ed, g8ep, Operator binary | Platform TLS CA; root of trust for all mTLS |
| `/data/g8e.db` | g8es | g8es only (via HTTP API) | All platform domain data |

Operator binaries are baked into the g8es image at `/opt/operator-binaries/linux-{amd64,arm64,386}/g8e.operator` (cross-compiled and UPX-compressed at image build time). On container startup, g8es uploads them to the blob store and serves them via `GET /blob/operator-binary/{os}-{arch}`. g8ep builds its own amd64 binary independently from source.

If `g8es-data` is wiped, every dependent service loses its TLS configuration and must re-initialize. This is why `./g8e platform wipe` stops all services before removing volumes — partial re-initialization with stale CA certs in other volumes would leave the stack in an inconsistent state.
