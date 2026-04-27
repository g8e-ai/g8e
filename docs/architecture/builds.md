---
title: Builds
parent: Architecture
---

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
- A **TLS certificate authority** — generates and serves the platform CA cert at `/ssl/ca.crt` at startup; this cert is the root of trust for all mTLS in the stack.

g8ee and g8ed wait for g8es to be healthy before starting. g8ep waits for g8ed to be healthy before starting. Without g8es, there is no database, no pub/sub, and no TLS trust anchor.

**Standard mode (g8eo — deployed Operators):** The Operator runs on a remote target system and connects outbound to g8es. It executes commands, manages files, streams heartbeat telemetry, and maintains the local audit vault and ledger on the target machine.

Operator binaries for remote deployment (linux/amd64, linux/arm64, linux/386) are cross-compiled and UPX-compressed at g8es image build time and baked into the image. On container startup, g8es uploads all 3 binaries to the blob store automatically. g8ed serves them on demand from the blob store. The `./g8e operator build` and `./g8e operator build-all` commands build fresh binaries inside the `g8eo-test-runner` container (the only image with a Go toolchain) and upload them to the blob store, overriding the baked versions.

---

## Component Overview

| Container | Internal name | Image built from | What it is |
|---|---|---|---|
| `g8es` | g8es | `components/g8es/Dockerfile` | Operator binary in `--listen` mode; platform DB + pub/sub + blob store (including operator binaries) |
| `g8ee` | g8ee | `components/g8ee/Dockerfile` | Python/FastAPI AI backend |
| `g8ed` | g8ed | `components/g8ed/Dockerfile` | Node.js web frontend; single external HTTPS entry point |
| `g8ep` | g8ep | `components/g8ep/Dockerfile` | Alpine sidecar; supervises a local operator process and runs platform tooling. Contains no Go toolchain — the operator binary is fetched from the g8es blob store at runtime (or rebuilt via `g8eo-test-runner` when `./g8e login` then `./g8e operator build` is invoked). |

---

## Build Dependency Chain

All images build in parallel — no component has a build-time dependency on any other. The sequencing constraint is purely at container startup time.

```
[1] ALL images build in parallel:
        g8es      (multi-stage: Go builder cross-compiles amd64/arm64/386 + UPX -> alpine runtime)
        g8ee      (no build deps)
        g8ed      (no build deps)
        g8ep      (no build deps)

[2] g8es container starts first
        |
        |  execs: g8e.operator --listen --data-dir /data --ssl-dir /ssl
        |                      --http-listen-port 9000 --wss-listen-port 9001
        |    -> generates platform CA cert -> writes to g8es-ssl volume
        |    -> opens SQLite store, starts HTTPS and WSS servers
        |    -> background job uploads 3 operator binaries to the blob store
        |    -> health check passes
        v
[3] g8ee, g8ed start in parallel (depends_on: g8es healthy):
        g8ee       -- waits for g8es /health, reads CA from /g8es/ca.crt
        g8ed       -- waits for g8es /health and platform_settings, reads certs from /g8es/

[4] g8ep starts (depends_on: g8ed healthy):
        -> supervisord PID 1 starts; [program:operator] autostarts
        -> fetch-key-and-run.sh pulls the operator API key from g8es platform_settings
        -> if /home/g8e/g8e.operator is missing, it is downloaded from the g8es blob store
        -> operator launches with --endpoint g8e.local
```

---

## Step-by-Step: What Happens and Why

### Step 1 — Build all images in parallel

All component images have no build-time dependencies on each other and build in parallel.

**g8es image build:** Uses a multi-stage Dockerfile. The builder stage (`golang:1.26-alpine3.23`) installs UPX, then cross-compiles the `g8e.operator` binary for all 3 target architectures (`linux/amd64`, `linux/arm64`, `linux/386`) with `-trimpath`, `-buildvcs=false`, and UPX `--best --lzma` compression. The platform version is injected via `-ldflags "-X main.version=${VERSION}"` — the `VERSION` build arg defaults to `${G8E_VERSION:-dev}` in compose (`build.sh` exports `G8E_VERSION` from the `VERSION` file). The final stage is a minimal `alpine:3.23` image that copies the amd64 binary to `/usr/local/bin/g8e.operator` (to run g8es itself in `--listen` mode) and all 3 compressed binaries to `/opt/operator-binaries/` (for blob store upload at startup). The Go toolchain is not present in the runtime image.

**g8ee image build:** Multi-stage Dockerfile based on `python:3.12-slim`. The builder stage installs Python dependencies into a prefix which is copied into the final runtime stage.

**g8ed image build:** Multi-stage Dockerfile based on `node:22-alpine3.23`. The builder stage runs `npm ci` (falling back to `npm install` if no lockfile) and `npm prune --omit=dev`. The final stage installs only curl (for healthchecks) — npm is not present in the runtime image.

**g8ep image build:** Based on `python:3.13-alpine`. Installs network/security tooling (supervisor, Docker CLI, nmap, tcpdump, bind-tools, etc.) plus `requests` and `aiohttp`. **Go is not installed in g8ep.** Operator binaries for this container come from the g8es blob store at runtime. When `./g8e login` then `./g8e operator build` or `./g8e operator build-all` is invoked, the compilation happens inside `g8eo-test-runner` (image: `golang:1.26-alpine3.23`), which uploads the result to the g8es blob store.

**Trigger:**
```bash
./g8e platform build        # Rebuild using layer cache
./g8e platform setup        # Build + start (first-time setup)
```

---

### Step 2 — Start g8es (`g8es`)

**Why first among running services:** g8es is the dependency of everything else. It provides:

1. **The TLS CA** — on first start, the Operator binary (in `--listen` mode) generates a self-signed CA and writes it to `/ssl/` inside the container. That directory is backed by the `g8es-ssl` Docker volume, which every other container mounts read-only at `/g8es`. Other containers read the CA cert from `/g8es/ca.crt`. Without this, mTLS cannot be established between any components. See [Security Architecture > Workstation CA Trust](security.md#workstation-ca-trust) for how users trust this CA.

2. **The document store** — g8ee and g8ed read and write all domain data (users, sessions, operators, cases, investigations) exclusively through g8es's HTTP API. All requests are authenticated via the `X-Internal-Auth` header. Neither component holds its own relational database.

3. **The KV and Blob stores** — platform settings, session data, operator binary blobs, file attachment metadata, and large binary blobs are all stored here. All requests are authenticated via the `X-Internal-Auth` header.

4. **The pub/sub broker** — operators on remote machines maintain a persistent WebSocket connection to g8es (WSS/TLS). Command dispatch and result delivery for the AI flow through this broker. g8ee and g8ed also connect to this broker for real-time event distribution.

5. **The operator binaries** — cross-compiled and UPX-compressed at image build time, baked into the image at `/opt/operator-binaries/`. On container startup, the entrypoint uploads all 3 binaries (linux-amd64, linux-arm64, linux-386) to the blob store (namespace `operator-binary`) via `PUT /blob/operator-binary/linux-{arch}`. Served on demand via `GET /blob/operator-binary/linux-{arch}`. The `./g8e operator build` commands (after `./g8e login`) override these by uploading fresh builds from `g8eo-test-runner`.

**What it runs:**
```
/entrypoint.sh
  -> exec g8e.operator --listen --data-dir /data --ssl-dir /ssl
             --http-listen-port 9000 --wss-listen-port 9001
```

Port 9000 (HTTPS) is used by g8ee and g8ed for all internal API traffic. Port 9001 (WSS/TLS) is used by remote Operators for the pub/sub connection. The container is internal-only and exposes no host ports. It runs as non-root user `g8e` (UID 1001).

**Health check:** `curl -f --cacert /ssl/ca.crt https://localhost:9000/health` — passes when the HTTP server is ready and the SQLite store is open. Operator binary uploads to the blob store run in the background after health is reached.

---

### Step 3 — Start g8ee and g8ed in parallel (`depends_on: g8es healthy`)

Both services start simultaneously once g8es is healthy.

---

### Step 4 — Start g8ee (`g8ee`)

**Why after g8es:**

- g8ee's entrypoint loops on `https://g8es:9000/health` (up to 30 retries, 2s apart) using `/g8es/ca.crt` as the CA bundle. It will not start uvicorn until g8es responds.
- Once running, g8ee reads all platform settings (LLM provider, API keys, feature flags) from g8es's document store and opens a persistent WebSocket connection to the g8es pub/sub broker.

**What it runs:**
```
exec uvicorn app.main:app --host 0.0.0.0 --port 443 \
    --ssl-keyfile "${SSL_DIR}/server.key" \
    --ssl-certfile "${SSL_DIR}/server.crt"
```

g8ee is an internal-only service. g8ed proxies all traffic to it; g8ee is never exposed on a public port. It runs as non-root user `g8e` (UID 1001) with `read_only: true`, `cap_drop: ALL`, and `no-new-privileges: true`.

**Health check:** `curl -f --cacert /g8es/ca.crt https://localhost/health`.

---

### Step 5 — Start g8ed (`g8ed`)

**Why after g8es:**

- g8ed reads its TLS certificates from the `g8es-ssl` volume (mounted read-only at `/g8es`) to terminate HTTPS on ports 443/80. Without g8es having initialized and written those certs, g8ed cannot serve TLS.
- g8ed's entrypoint additionally verifies that `platform_settings` has been initialized in g8es by fetching `https://g8es:9000/db/settings/platform_settings` with the internal auth token before launching `node server.js`.
- At runtime, g8ed proxies all AI requests to g8ee's internal HTTPS API and subscribes to g8es pub/sub to receive operator events (heartbeats, command results, status changes) for SSE fan-out to the browser.

**What it runs:**
```
CMD ["node", "server.js"]
```

g8ed is the only service with external ports (`443:443`, `80:80`). It terminates TLS, handles passkey authentication, manages operator WebSocket connections (Gateway Protocol — bridging remote g8eo operators to g8es pub/sub), and serves the browser dashboard. It runs as non-root user `g8e` (UID 1001) with `read_only: true`, `cap_drop: ALL`, and `no-new-privileges: true`. g8ed's `G8ENodeOperatorService` manages the g8ep operator process via Supervisor XML-RPC over the internal network (port 443), not via `docker exec`.

**Health check:** `curl -f --cacert /g8es/ca.crt https://localhost/health`.

---

### Step 6 — Start g8ep (`g8ep`, `depends_on: g8ed healthy`)

g8ep waits for g8ed to be healthy before starting.

**What it does on startup:**
1. `scripts/entrypoint.sh` loads the internal auth token, session encryption key, and CA cert paths from `/g8es/` (the read-only `g8es-ssl` volume mount).
2. It writes a supervisord config to `/tmp/g8e.operator.conf` with a `[program:operator]` entry (`autostart=true`, `autorestart=true`) whose command is `/app/components/g8ep/scripts/fetch-key-and-run.sh`, and an `[inet_http_server]` block listening on port 443 with username `g8e-internal` and password `${G8E_INTERNAL_AUTH_TOKEN}`.
3. It execs `supervisord -c /tmp/g8e.operator.conf` as PID 1. Supervisor immediately starts the operator program.
4. `fetch-key-and-run.sh` retrieves the operator API key from g8es `platform_settings` with exponential backoff, then — if `/home/g8e/g8e.operator` is missing — downloads the operator binary for the container's architecture from `https://g8es:9000/blob/operator-binary/linux-{arch}`.
5. The operator launches with `--endpoint g8e.local --working-dir /home/g8e --no-git --log info --cloud --provider g8ep`.

**Why g8ep does not build a binary:** g8ep has no Go toolchain; keeping it lean avoids shipping a compiler in the sidecar. The binary is always available because g8es bakes and publishes it at startup. `./g8e operator build` (which compiles in `g8eo-test-runner` and uploads to g8es) can force a fresh build at any time — g8ep will pick it up on next operator restart (`./g8e operator reauth`). g8ep runs as non-root user `g8e` (UID 1001) with `cap_drop: ALL` (capabilities `NET_RAW`, `NET_ADMIN`, `SYS_PTRACE`, `SETUID`, `SETGID` are added back for operator network tooling) and `no-new-privileges: true`.

**Health check:** `pgrep -x supervisord`.

---

## Runtime Startup Order (Enforced by `depends_on`)

Docker Compose enforces this dependency graph via `condition: service_healthy`:

```
g8es (healthy)
    |-- g8ee  (depends_on: g8es healthy)
    '-- g8ed  (depends_on: g8es healthy)
            '-- g8ep  (depends_on: g8ed healthy)
```

The `build.sh` script (`scripts/core/build.sh`) manages the full lifecycle. It ensures that when `rebuild`, `reset`, or `setup` is called, `g8es` is started first and its health is verified before dependent services are awaited.

---

### CI Workflow (`build-and-test.yml`)

Triggered on every push to `main` and on pull requests to `main`.

1. **Platform Setup:** Executes `./g8e platform setup` to build all core images (`g8es`, `g8ee`, `g8ed`, `g8ep`) and start the full platform with health checks.
2. **Component Tests:** Runs `./g8e test g8ee`, `./g8e test g8ed`, and `./g8e test g8eo`. Each invocation lazily builds the matching per-component test-runner image (`g8ee-test-runner`, `g8ed-test-runner`, `g8eo-test-runner`) on first use via `docker compose up -d --wait`.

Test-runner containers are not built by `platform setup`; they are built on demand when `./g8e test <component>` first requires them.

---

## Build vs. Start Commands

| Command | What it does |
|---|---|
| `./g8e platform setup` | First-time setup: builds all core images (layer cache is used), force-recreates and starts `g8es`, `g8ee`, `g8ed`, `g8ep`, waits for health checks. Does not wipe data volumes. |
| `./g8e platform build` | Alias for `rebuild`: stops containers, rebuilds all core images in parallel (with cache), starts them with health checks |
| `./g8e platform start` | `up -d` with no rebuild; images must already exist |
| `./g8e platform stop` | Stops core containers; preserves all volumes and images |
| `./g8e platform restart` | Stop + start without rebuilding; picks up config changes that don't require a new image |
| `./g8e platform rebuild [component ...]` | Rebuild one or more specific components, e.g. `./g8e platform rebuild g8ed`. Valid components include test-runners (`g8ee-test-runner`, `g8ed-test-runner`, `g8eo-test-runner`). |
| `./g8e platform reset` | Wipes `g8es-data`, `g8ee-data`, `g8ed-data` volumes and rebuilds core images; **`g8es-ssl` is preserved**, so the platform CA stays trusted by previously configured devices. |
| `./g8e platform wipe` | Clears application data from the g8es DB via the internal API (`manage-g8es.py store wipe`); preserves platform settings, SSL certs, and the internal auth token. |
| `./g8e platform clean` | Remove all managed Docker resources: containers, images, volumes, networks (filtered by the `io.g8e.managed=true` label), plus orphaned networks and dangling images. |

---

## First-Time Setup Order

On a fresh checkout with no existing images:

```
1. ./g8e platform setup   (or: docker compose up -d --build)
   '-- [1] all core images build in parallel (layer cache is used)
          g8es cross-compiles all 3 operator arches + UPX compression (takes longest)
   '-- [2] g8es starts
          -> generates platform CA cert -> writes to g8es-ssl volume
          -> uploads 3 compressed operator binaries to blob store (background)
          -> health check passes
   '-- [3] g8ee, g8ed start in parallel (depends_on: g8es)
   '-- [4] g8ep starts (depends_on: g8ed)
          -> supervisord starts operator; fetch-key-and-run.sh pulls the binary
             from the g8es blob store on first run
   '-- [5] all health checks pass
   '-- https://localhost is live

2. Open https://localhost
   '-- setup wizard configures: hostname, AI provider, admin account, passkey
```

Works identically with Docker Desktop — no `./g8e` CLI required for first-time setup.

---

## The g8ep Operator and Supervisord

The g8ep container runs `supervisord` as PID 1 with a config written at startup to `/tmp/g8e.operator.conf`. The `[program:operator]` entry has `autostart=true`, so the operator process starts automatically when supervisord launches. Supervisor also exposes `[inet_http_server]` on port 443 (inside the container) with HTTP Basic auth using the internal auth token as the password.

When a user launches a local operator session from the dashboard, g8ed:
1. Persists the operator API key to the `platform_settings` document in g8es.
2. Calls Supervisor XML-RPC (via the network, not `docker exec`) to start or restart the operator process in g8ep.

The operator process then runs inside g8ep as `g8e.operator --endpoint g8e.local --working-dir /home/g8e --no-git --log info --cloud --provider g8ep`. The API key is fetched from g8es `platform_settings` by `fetch-key-and-run.sh` before the binary is exec'd. The CA certificate is read from `/g8es/ca.crt` — the operator discovers it automatically without a network fetch. From this point it is indistinguishable from any other operator at the protocol level — heartbeats flow through g8es pub/sub and commands are dispatched via the same channels.

g8ep mounts `/var/run/docker.sock` to support operator streaming/deploy workflows (e.g. invoking the `g8e.operator stream` binary inside g8ep to inject operators onto remote hosts). The `./g8e operator build` build path does not use this socket — builds happen in `g8eo-test-runner`.

---

## Platform Network

All services run on a single internal Docker bridge network: `g8e-network`.

g8ed is given network aliases `localhost` and `g8e.local` so that the Operator binary (running inside g8ep) can reach g8ed at `g8e.local:443` — the default platform endpoint used by operators when no `--endpoint` flag is given.

External traffic enters only through g8ed on ports 443 and 80. All other services (`g8es`, `g8ee`, `g8ep`) are internal-only and not exposed on any host port.

---

## Volume Dependencies

The platform uses two separate g8es-owned volumes so that TLS material can survive a data reset.

**`g8es-ssl` volume (the single most critical volume):**

| Path inside g8es | Mounted elsewhere as | Written by | Read by | Why |
|---|---|---|---|---|
| `/ssl/ca.crt` | `/g8es/ca.crt` (`ro`) | g8es (on first init) | g8ee, g8ed, g8ep, operator binary | Platform TLS CA; root of trust for all mTLS |
| `/ssl/server.crt`, `/ssl/server.key` | `/g8es/server.{crt,key}` (`ro`) | g8es | g8ee, g8ed | TLS server cert used by uvicorn (g8ee) and g8ed's HTTPS listener |
| `/ssl/internal_auth_token` | `/g8es/internal_auth_token` (`ro`) | g8es | g8ee, g8ed, g8ep | Shared secret for `X-Internal-Auth` |

**`g8es-data` volume:**

| Path inside g8es | Written by | Read by | Why |
|---|---|---|---|
| `/data/g8e.db` | g8es | g8es only (via HTTP API) | All platform domain data |

Operator binaries are baked into the g8es image at `/opt/operator-binaries/linux-{amd64,arm64,386}/g8e.operator` (cross-compiled and UPX-compressed at image build time). On container startup, g8es uploads them to the blob store and serves them via `GET /blob/operator-binary/linux-{arch}`. g8ep fetches its binary from the blob store on first operator start (it does not build locally).

`./g8e platform reset` wipes `g8es-data`, `g8ee-data`, and `g8ed-data` but preserves `g8es-ssl` — users do not need to re-trust the platform CA after a reset. `./g8e platform clean` removes everything, including `g8es-ssl`, at which point a fresh CA is generated on the next startup and all previously trusted clients must re-trust.
