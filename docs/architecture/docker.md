# Docker Architecture

## Overview

g8e runs as a multi-service Docker Compose stack. Every service is built from a dedicated Dockerfile with a hardened runtime configuration: non-root users, read-only filesystems where possible, and capability/privilege restrictions on most services.

## Compose Files

| File | Purpose |
|------|---------|
| `docker-compose.yml` | Production configuration. Image-baked code, non-root users, security hardening. Used by `./g8e platform *` and CI. |

**Development usage:**
g8e does not use a separate `docker-compose.dev.yml` file. Development workflows are handled via the `./g8e` CLI or by setting environment variables.

## Services

### VSE (`g8e-engine`)

AI backend. Python/FastAPI.

- **User:** `g8e` (uid 1001, gid 1001)
- **Read-only filesystem:** yes — tmpfs at `/tmp`, `/var/tmp`
- **Capabilities:** none (`cap_drop: ALL`)
- **Writable volumes:** `vse-data:/data` only
- **Config/shared mounts:** `./shared:ro`, `vsodb-ssl:/vsodb:ro`
- **Internal Auth:** Receives `INTERNAL_AUTH_TOKEN` via environment during bootstrap; discovers authoritative token from VSODB/SSL volume at runtime.
- **Security:** `cap_drop: ALL`, `no-new-privileges:true`, hardened sysctls

### VSOD (`g8e-dashboard`)

Web frontend and single external entry point. Node.js.

- **User:** `g8e` (uid 1001, gid 1001)
- **Read-only filesystem:** yes — tmpfs at `/tmp`, `/var/tmp`
- **Capabilities:** none (`cap_drop: ALL`)
- **Writable volumes:** `vsod-data:/data`
- **Config/shared mounts:** `./shared:ro`, `vsodb-ssl:/vsodb:ro`, `./docs:ro`, `./README.md:ro`, `./components/vsod/views:ro`
- **Internal Auth:** Discovers authoritative token from VSODB/SSL volume (`vsodb-ssl:/vsodb:ro`) at runtime. No `G8E_INTERNAL_AUTH_TOKEN` environment variable.
- **Security:** `cap_drop: ALL`, `no-new-privileges:true`, hardened sysctls, read-only root filesystem

### VSODB (`g8e-data`)

Platform persistence and pub/sub broker. Runs the `g8e.operator` binary in `--listen` mode.

- **User:** `g8e` (uid 1001, gid 1001)
- **Read-only filesystem:** yes — tmpfs at `/tmp`, `/var/tmp`
- **Capabilities:** none (no `cap_drop` or `cap_add` directives in compose)
- **Writable volumes:** `vsodb-data:/data`, `vsodb-ssl:/ssl`
- **Internal Auth:** Authoritative generator and enforcer of `X-Internal-Auth` token. Receives `G8E_INTERNAL_AUTH_TOKEN` via environment. Persists secrets exclusively to the `g8e-data-ssl` volume.
- **Security:** read-only root filesystem (no `cap_drop`, `no-new-privileges`, or `sysctls` directives in compose)
- **Ports:** Exposes 9000 (HTTPS) and 9001 (WSS) for internal communication (no external ports)

### g8e node

Unified test environment with Python, Node, and Go. Always running alongside core services.

- **User:** `g8e` (uid 1001, gid 1001)
- **Base image:** `ubuntu:24.04`
- **Read-only filesystem:** no — test and build workflows write to `/app/components` and Go build cache
- **Bind mounts:** `components/vse/`, `components/vsod/`, `components/vsa/`, `components/g8e-pod/scripts/`, `shared/`, `scripts/` — the full repo root is not mounted
- **Capabilities:** `cap_drop: ALL` — no capabilities added back
- **Security:** `cap_drop: ALL`, `no-new-privileges: true` (no `sysctls` directives in compose)
- **Docker socket:** see [Docker Socket Threat Model](#docker-socket-threat-model) below
- **Go toolchain:** `GOPATH`, `GOBIN`, and `GOCACHE` are all set under `/home/g8e/` so the Go build cache and installed binaries are owned by `g8e` from the start. `gotestsum` is installed as `g8e` (after `USER g8e`) so no root-owned cache files are created.
- **Notable env vars:** `RUNNING_IN_DOCKER=1`, `HOME=/home/g8e` signals to test and tool scripts that they are executing inside the container

## Non-Root Users

All production services (VSE, VSOD, VSODB) run as dedicated non-root users created in their respective Dockerfiles. The `user:` directive in compose reinforces this by specifying the numeric uid:gid directly — the image cannot override it.

| Service | User | UID | GID |
|---------|------|-----|-----|
| VSE | `g8e` | 1001 | 1001 |
| VSOD | `g8e` | 1001 | 1001 |
| VSODB | `g8e` | 1001 | 1001 |
| g8e node | `g8e` | 1001 | 1001 |

Dockerfile patterns:

**Debian (VSE) — `python:3.13-slim` base:**
```dockerfile
RUN groupadd -g 1001 g8e && \
    useradd -u 1001 -g g8e -M -s /sbin/nologin g8e
USER g8e
```

**Alpine (VSOD, VSODB) — `node:22-alpine3.21` / `alpine:3.21` base:**
```dockerfile
RUN addgroup -g 1001 g8e && \
    adduser -u 1001 -G g8e -H -D -s /sbin/nologin g8e
USER g8e
```

**Ubuntu (Drop Pod) — `ubuntu:24.04` base:**
```dockerfile
RUN groupadd -g 1001 g8e && \
    useradd -u 1001 -g g8e -m -s /bin/bash g8e
USER g8e
```

## Security Hardening

### Resource Constraints

All services implement physical resource limits to prevent Denial of Service (DoS) from compromised or runaway processes:

- **Memory Limits:** Ranging from 512MB (VSODB) to 4GB (Drop Pod).
- **PID Limits:** Restricts the number of concurrent processes to prevent fork bombs.

### Volume Security Options

Writable data volumes use native Docker mount options to restrict behavior:

- **`noexec`:** Prevents execution of binaries from the volume.
- **`nosuid`:** Prevents `setuid` bits from being respected.
- **`nodev`:** Prevents the creation of device nodes.

Applied to: `vse-data`, `vsod-data`, `vsodb-data`, `vsodb-ssl`.

### Network Isolation

The backend network (`vso-network`) uses a standard bridge driver:

- **Bridge network:** All services communicate over the `g8e-network` bridge. The network is not marked `internal: true` — external routing is not blocked at the Docker network level.
- **Gateway:** VSOD is the only service with published host ports (443, 80), making it the single external entry point by design.
- **Sysctls:** Hardened kernel parameters (`accept_redirects=0`, `send_redirects=0`) are applied to VSE and VSOD. VSODB and g8e node do not have `sysctls` directives.

### `no-new-privileges`

Applied to VSE, VSOD, and g8e node:

```yaml
security_opt:
  - no-new-privileges:true
```

Prevents any process inside the container from gaining additional privileges via `setuid`/`setgid` binaries or file capabilities, even if a vulnerability allows code execution as an unexpected user.

Not applied to:
- **VSODB** — no `security_opt` directive in compose.

### Capability Dropping

VSE, VSOD, and g8e node g8e all capabilities:

```yaml
cap_drop:
  - ALL
```

Drop Pod drops all capabilities with no additions.

VSODB does not have a `cap_drop` directive in compose.

### Read-Only Filesystems

Services that only write to mounted volumes get a read-only root filesystem:

```yaml
read_only: true
tmpfs:
  - /tmp
  - /var/tmp
```

Applied to: **VSE**, **VSOD**, **VSODB**

Not applied to: **Drop Pod** (build and test workflows require writes throughout the container).

## Docker Socket Threat Model

One service mounts `/var/run/docker.sock`: g8e node.

**The threat:** The Docker socket is equivalent to root on the host. A process with socket access can start privileged containers, read host filesystem paths, and escape the container isolation boundary.

**Why g8e node needs it:**

Drop Pod uses `docker exec` to run test suites and operator workflows against live service containers. It is a dev/test tool, never public-facing.

Mitigation: g8e node runs as uid 1001 (not root). The `group_add: ${DOCKER_GID}` directive adds the host docker group to the container user, granting socket access without requiring root.

**How VSOD manages the g8e node operator (without the socket):**

VSOD's `G8ENodeOperatorService` manages operator processes inside the g8e-pod container via Supervisor XML-RPC over the internal network — it does not use `docker exec` or mount the Docker socket. The XML-RPC interaction is:

- Isolated to a single internal service (`G8ENodeOperatorService`)
- Never triggered by unauthenticated requests — operator sessions require a valid authenticated user session
- Not exposed on any public API path

## Volume Strategy

Volumes are categorized by write requirement:

**Production (`docker-compose.yml`):**

| Mount | Mode | Services |
|-------|------|----------|
| `vse-data:/data` | read-write | VSE |
| `vsod-data:/data` | read-write | VSOD |
| `vsodb-data:/data` | read-write | VSODB |
| `vsodb-ssl:/ssl` | read-write | VSODB |
| `vsod-node-modules:/app/components/vsod/node_modules` | read-write | g8e node |
| `./components/vse:/app/components/vse` | read-write | g8e node |
| `./components/vsod:/app/components/vsod` | read-write | g8e node |
| `./components/vsa:/app/components/vsa` | read-write | g8e node |
| `./components/g8e-pod/scripts:/app/components/g8e-pod/scripts` | read-write | g8e node |
| `./scripts:/app/scripts` | read-write | g8e node |
| `./components/g8e-pod/reports:/reports` | read-write | g8e node |
| `./shared:/app/shared` | read-only | VSE |
| `./shared:/app/shared` | read-write | g8e node |
| `./shared:/shared` | read-only | VSOD |
| `./components/vsod/views:/app/views` | read-only | VSOD |
| `vsodb-ssl:/vsodb` | read-only | VSE, VSOD, g8e node |
| `./docs:/docs` | read-only | VSOD |
| `./README.md:/readme/README.md` | read-only | VSOD |
| `/var/run/docker.sock` | read-write | g8e node |

**Development additions:**
Development mode is handled via the `./g8e` CLI and by passing specific environment variables or Docker Compose profiles.

## Build Context

All Dockerfiles use the repo root as the build context (`context: .` in compose). This is required because each service copies from multiple top-level directories:

- **VSE** — `components/vse/`, `shared/`
- **VSOD** — `components/vsod/`, `shared/`
- **VSODB** — `components/vsa/`, `components/vsodb/` (pre-built operator binaries for linux/amd64, arm64, 386 are generated during build)
- **Drop Pod** — `components/vse/`, `components/vsod/`, `components/vsa/`, `components/g8e-pod/`

No `.dockerignore` file exists at the repo root; the full build context (minus `.gitignore` patterns) is sent to the daemon.
