# Builds, Dependencies, and Startup Sequence

This document explains the g8e component dependency chain, the reason each component exists in the order it does, and what every build and startup step actually produces. Understanding why the order matters requires understanding what each component is and what it provides to the rest of the stack.

---

## The Operator is the Heart of the Platform

Every piece of g8e infrastructure ultimately depends on the `g8e.operator` binary. The same binary runs in two fundamentally different modes:

**`--listen` mode (VSODB — `g8e-data`):** The Operator binary becomes the platform's central persistence layer. It runs as a local server inside the `g8e-data` container and provides:
- A SQLite-backed **document store** (`/db/{collection}/{id}`) — all domain data: users, sessions, operators, cases, investigations, memories. **Authenticated via `X-Internal-Auth`.**
- A **KV store** (`/kv/{key}`) — platform settings, binary blobs, ephemeral session data, attachments. **Authenticated via `X-Internal-Auth`.**
- A **blob store** (`/blob/{namespace}/{id}`) — large file storage with TTL support, including operator binaries for remote deployment (namespace `operator-binary`). **Authenticated via `X-Internal-Auth`.**
- A **pub/sub WebSocket broker** (`/ws/pubsub`) — the real-time message bus connecting operators on remote machines back to VSE and VSOD. **Connection authenticated via `X-Internal-Auth`.**
- A **TLS certificate authority** — generates and serves the platform CA cert at `/ssl/ca.crt` at startup; this cert is the root of trust for all mTLS in the stack

VSE and VSOD wait for VSODB to be healthy before starting. g8e-pod starts independently and builds its own operator binary from source. Without VSODB, there is no database, no pub/sub, and no TLS trust anchor.

**Standard mode (VSA — deployed Operators):** The Operator runs on a remote target system and connects outbound to VSODB. It executes commands, manages files, streams heartbeat telemetry, and maintains the local audit vault and ledger on the target machine.

Operator binaries for remote deployment (linux/amd64, linux/arm64, linux/386) are cross-compiled and UPX-compressed at VSODB image build time and baked into the image. On container startup, VSODB uploads all 3 binaries to the blob store automatically. VSOD serves them on demand from the blob store. The `./g8e operator build` and `./g8e operator build-all` commands in g8e-pod can override the baked binaries by uploading fresh builds to the blob store. g8e-pod builds its own amd64 binary from source using the VSA Makefile at container startup — this keeps g8e-pod self-contained as the test and local-operator runner.

---

## Component Overview

| Container | Internal name | Image built from | What it is |
|---|---|---|---|
| `g8e-data` | VSODB | `components/vsodb/Dockerfile` | Operator binary in `--listen` mode; platform DB + pub/sub + blob store (including operator binaries) |
| `g8e-engine` | VSE | `components/vse/Dockerfile` | Python/FastAPI AI backend |
| `g8e-dashboard` | VSOD | `components/vsod/Dockerfile` | Node.js web frontend; single external HTTPS entry point |
| `g8e-pod` | g8e-pod | `components/g8e-pod/Dockerfile` | Ubuntu sidecar; builds operator binary from source, runs tests, streams operators to remote hosts |

---

## Build Dependency Chain

All images build in parallel — no component has a build-time dependency on any other. The sequencing constraint is purely at container startup time.

```
[1] ALL images build in parallel:
        vsodb      (multi-stage: Go builder cross-compiles amd64/arm64/386 + UPX → alpine runtime)
        vse        (no build deps)
        vsod       (no build deps)
        g8e-pod   (no build deps)

[2] vsodb container starts first
        │
        │  execs: g8e.operator --listen
        │    → generates platform CA cert → writes to vsodb-data volume
        │    → opens SQLite store, starts HTTPS and WSS servers
        │    → health check passes
        ▼
[3] vse, vsod start in parallel (depends_on: vsodb healthy):
        vse       — connects to VSODB pub/sub, reads settings from KV store
        vsod      — reads TLS certs from vsodb-data volume

[3] g8e-pod starts independently (no depends_on):
        → go build ./components/vsa → /home/g8e/g8e.operator
        → starts supervisord
```

---

## Step-by-Step: What Happens and Why

### Step 1 — Build all images in parallel

All component images have no build-time dependencies on each other and build in parallel.

**VSODB image build:** Uses a multi-stage Dockerfile. The builder stage installs Go and UPX, then cross-compiles the `g8e.operator` binary for all 3 target architectures (`linux/amd64`, `linux/arm64`, `linux/386`) with `-trimpath`, `-buildvcs=false`, and UPX `--best --lzma` compression. The platform version is injected via `-ldflags "-X main.version=${VERSION}"` — the `VERSION` build arg is set from the `G8E_VERSION` environment variable (read from the `VERSION` file by `build.sh`). The final stage is a minimal Alpine image that copies the amd64 binary to `/usr/local/bin/` (to run VSODB itself in `--listen` mode) and all 3 compressed binaries to `/opt/operator-binaries/` (for blob store upload at startup). The Go toolchain is not present in the runtime image.

**All other images** (g8e-pod, vse, vsod) build independently with no dependency on vsodb.

**VSE image build:** Uses a multi-stage Dockerfile based on Python 3.13-slim. It installs dependencies into a prefix in the builder stage to keep the final runtime image minimal.

**VSOD image build:** Uses a multi-stage Dockerfile based on Node.js 22-alpine. It installs `docker-cli` in the final stage to allow interaction with the host Docker daemon.

**g8e-pod image build:** Installs Go 1.24.1, Node.js 22, Python 3.12, and all test tooling. The operator binary is **not** built at image build time — it is compiled from the vendored source in `/app/components/vsa` at container startup (or on demand via `./g8e operator build`) using the VSA Makefile, so it always reflects the current source tree.

**Trigger:**
```bash
./g8e platform build
```

---

### Step 2 — Start VSODB (`g8e-data`)

**Why first among running services:** VSODB is the dependency of everything else. It provides:

1. **The TLS CA** — on first start, the Operator binary (in `--listen` mode) generates a self-signed CA and writes it to `/data/ssl/`. All other containers mount `vsodb-data:/vsodb:ro` and read the CA cert from `/vsodb/ssl/ca.crt`. Without this, mTLS cannot be established between any components. See [Security Architecture > Workstation CA Trust](security.md#workstation-ca-trust) for how users trust this CA.

2. **The document store** — VSE and VSOD read and write all domain data (users, sessions, operators, cases, investigations) exclusively through VSODB's HTTP API. All requests are authenticated via the `X-Internal-Auth` header. Neither component holds its own relational database.

3. **The KV and Blob stores** — platform settings, session data, operator binary blobs, file attachment metadata, and large binary blobs are all stored here. All requests are authenticated via the `X-Internal-Auth` header.

4. **The pub/sub broker** — operators on remote machines maintain a persistent WebSocket connection to VSODB (WSS/TLS). Command dispatch and result delivery for the AI agent flow through this broker. VSE and VSOD also connect to this broker for real-time event distribution.

5. **The operator binaries** — cross-compiled and UPX-compressed at image build time, baked into the image at `/opt/operator-binaries/`. On container startup, the entrypoint uploads all 3 binaries (linux-amd64, linux-arm64, linux-386) to the blob store (namespace `operator-binary`). Served on demand via `GET /blob/operator-binary/{os}-{arch}`. The `./g8e operator build` commands in g8e-pod can override these by uploading fresh builds.

**What it runs:**
```
/entrypoint.sh
  → exec g8e.operator --listen --data-dir /data --ssl-dir /ssl
             --http-listen-port 9000 --wss-listen-port 9001
```

Port 9000 (HTTPS) is used by VSE and VSOD for all internal API traffic. Port 9001 (WSS/TLS) is used by remote Operators for the pub/sub connection.

**Health check:** `curl https://localhost/health` — passes when the HTTP server is ready and the SQLite store is open.

---

### Step 3 — Start VSE, VSOD, and g8e-pod in parallel (`depends_on: vsodb healthy`)

All three services start simultaneously once vsodb is healthy.

---

### Step 4 — Start VSE (`g8e-engine`)

**Why after VSODB:**

- VSE's startup sequence opens a persistent WebSocket connection to VSODB's pub/sub broker to subscribe to heartbeat and command-result channels. If VSODB is not healthy, this connection fails and VSE will not start successfully.
- VSE reads all platform settings (LLM provider, API keys, feature flags) from VSODB's KV store at startup.

**What it runs:**
```
CMD ["sh", "-c", "exec uvicorn app.main:app --host 0.0.0.0 --port ${PORT:-443}"]
```

VSE is an internal-only service. VSOD proxies all traffic to it; VSE is never exposed on a public port. It runs as non-root user `g8e` (UID 1001).

**Health check:** `curl https://localhost/health/store` — passes when VSODB document store connectivity is confirmed.

---

### Step 5 — Start VSOD (`g8e-dashboard`)

**Why after VSODB (and implicitly after VSE):**

- VSOD reads its TLS certificates from the `vsodb-data` volume (`/vsodb/ssl/`) to terminate HTTPS on ports 443/80. Without VSODB having initialized and written those certs, VSOD cannot serve TLS.
- VSOD proxies all AI requests to VSE's internal HTTP API. While VSOD can start before VSE is fully ready, the setup wizard and dashboard will not function correctly until VSE is healthy.
- VSOD subscribes to VSODB pub/sub to receive operator events (heartbeats, command results, status changes) for SSE fan-out to the browser.

**What it runs:**
```
CMD ["node", "server.js"]
```

VSOD is the only service with external ports (`443:443`, `80:80`). It terminates TLS, handles passkey authentication, manages operator WebSocket connections (Gateway Protocol — bridging remote VSA operators to VSODB pub/sub), and serves the browser dashboard. It runs as non-root user `g8e` (UID 1001).

VSOD also mounts `/var/run/docker.sock` to manage operator sessions inside the g8e-pod container via `docker exec`. This is required for the g8e-pod operator feature — VSOD calls `supervisorctl start operator` inside the g8e-pod container when a user launches a local operator session.

**Health check:** `curl https://localhost/health/store` — passes when connectivity to VSODB is confirmed. External TLS health is checked via `curl https://localhost/health`.

---

### Step 6 — Start g8e-pod

**Starts independently** — g8e-pod has no `depends_on` constraint. It does not require vsodb to build its binary.

**What it does on startup:**
1. Compiles the `g8e.operator` binary from source (`/app/components/vsa`) using the VSA Makefile, outputting to `/home/g8e/g8e.operator`. If `/vsodb/ssl/ca.crt` is already present (vsodb initialized), it is used to configure the operator's trust anchor.
2. Starts `supervisord` as PID 1. The `[program:operator]` entry is registered but `autostart=false` — the operator process does not start automatically. VSOD starts it on demand via `docker exec g8e-pod supervisorctl start operator` when a user launches a local operator session.

**Why g8e-pod builds its own binary:** g8e-pod is the test runner for VSA (`./g8e test vsa`). Having Go installed and building from source using the VSA Makefile ensures the binary used for local operator sessions always matches the current source, and that `go build` is verified as part of the development workflow. The `./g8e operator build` command can force a rebuild at any time. It runs as non-root user `g8e` (UID 1001) with `cap_drop: ALL` and `no-new-privileges: true`.

**Health check:** `pgrep -x supervisord`

---

## Runtime Startup Order (Enforced by `depends_on`)

Docker Compose enforces this dependency graph via `condition: service_healthy`:

```
vsodb (healthy)
    ├── vse      (depends_on: vsodb healthy)
    └── vsod     (depends_on: vsodb healthy)

g8e-pod  — no depends_on; builds its own binary from source at startup
```

The `build.sh` script (`scripts/core/build.sh`) manages the full lifecycle. It ensures that when `rebuild` or `reset` is called, `vsodb` is started first and its health is verified before starting dependent services.

---

### CI Workflow (`ci.yml`)
Triggered on every push to `main` and pull requests to `main` or `dev`.
1. **Platform Build:** Executes `./g8e platform build` to verify all components compile and start correctly.
2. **Component Tests:** Runs `vse`, `vsod`, and `vsa` test suites inside `g8e-pod`.
3. **Multi-Arch Verification:** Explicitly builds the `g8e.operator` for `amd64`, `arm64`, and `386` architectures.

---

## Build vs. Start Commands

| Command | What it does |
|---|---|
| `./g8e platform setup` | Full first-time setup: no-cache build of all images (VSODB cross-compiles all operator binaries), start platform, wait for health checks. Does not wipe data volumes. Recommended for first-time setup. |
| `./g8e platform build` | Rebuild with layer cache: stops containers, rebuilds all images in parallel (with cache), starts vsodb first then all remaining services, waits for health checks |
| `./g8e platform start` | Start existing images with no rebuild — services must already be built |
| `./g8e platform stop` | Stop containers; preserves all volumes and images |
| `./g8e platform restart` | Stop and start without rebuilding; picks up config changes that don't require a new image |
| `./g8e platform rebuild [component]` | Rebuild one or more specific components, e.g. `./g8e platform rebuild vsod` |
| `./g8e platform reset` | Wipe data volumes + full rebuild — equivalent to starting fresh from scratch |
| `./g8e platform wipe` | Remove data volumes and restart — images are reused, data is erased |
| `./g8e platform clean` | Remove all managed Docker resources: containers, images, volumes, networks, build cache |

---

## First-Time Setup Order

On a fresh checkout with no existing images:

```
1. ./g8e platform setup   (or: docker compose up)
   └─ [1] all images build in parallel (no cache)
          vsodb cross-compiles all 3 operator arches + UPX compression (takes longest)
   └─ [2] vsodb starts
          → generates platform CA cert → writes to vsodb-data volume
          → uploads 3 compressed operator binaries to blob store
          → health check passes
   └─ [3] vse, vsod start in parallel (depends_on: vsodb)
          g8e-pod starts independently: make build → /home/g8e/g8e.operator
   └─ [4] all health checks pass
   └─ https://localhost is live

2. Open https://localhost
   └─ setup wizard configures: hostname, AI provider, admin account, passkey
```

Works identically with Docker Desktop — no `./g8e` CLI required for first-time setup.

---

## The g8e-pod Operator and Supervisord

The g8e-pod container runs `supervisord` as PID 1. The supervisor config pre-registers a `[program:operator]` entry but sets `autostart=false` — the operator process does not start automatically.

When a user launches a local operator session from the dashboard, VSOD:
1. Writes the user's device token to `/run/operator-token` inside the g8e-pod container via `docker exec`
2. Calls `docker exec g8e-pod supervisorctl start operator`

The Operator process then starts inside g8e-pod with `--endpoint g8e.local`. It reads the API key from the `G8E_OPERATOR_API_KEY` environment variable (fetched from VSODB platform_settings by `fetch-key-and-run.sh`). The CA certificate is loaded from the local SSL volume at `/vsodb/ca.crt` — the operator discovers it automatically without a network fetch. From this point it is indistinguishable from any other operator — heartbeats flow through VSODB pub/sub, commands are dispatched via the same channels.

This is why VSOD mounts `/var/run/docker.sock`. There is no alternative mechanism; `docker exec` into a running container requires socket access. VSOD's `G8ENodeOperatorService` handles the interaction with the Docker API.

---

## Platform Network

All services run on a single internal Docker bridge network: `g8e-network` (`vso-network` in compose).

VSOD is given network aliases `localhost` and `g8e.local` so that the Operator binary (running inside g8e-pod) can reach VSOD at `g8e.local:443` — the default platform endpoint used by operators when no `--endpoint` flag is given.

External traffic enters only through VSOD on ports 443 and 80. All other services (VSODB, VSE) are internal-only and not exposed on any host port.

---

## Volume Dependencies

`vsodb-data` is the single most critical volume in the platform. Its contents gate multiple service startups:

**`vsodb-data` volume (`g8e-data-data`):**

| Path | Written by | Read by | Why |
|---|---|---|---|
| `/data/ssl/ca.crt` | VSODB (on first init) | VSE, VSOD, g8e-pod, Operator binary | Platform TLS CA; root of trust for all mTLS |
| `/data/g8e.db` | VSODB | VSODB only (via HTTP API) | All platform domain data |

Operator binaries are baked into the VSODB image at `/opt/operator-binaries/linux-{amd64,arm64,386}/g8e.operator` (cross-compiled and UPX-compressed at image build time). On container startup, VSODB uploads them to the blob store and serves them via `GET /blob/operator-binary/{os}-{arch}`. g8e-pod builds its own amd64 binary independently from source.

If `vsodb-data` is wiped, every dependent service loses its TLS configuration and must re-initialize. This is why `./g8e platform wipe` stops all services before removing volumes — partial re-initialization with stale CA certs in other volumes would leave the stack in an inconsistent state.
