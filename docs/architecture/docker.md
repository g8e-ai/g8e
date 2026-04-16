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

### g8ee (`g8ee`)

AI backend. Python/FastAPI.

- **User:** `g8e` (uid 1001, gid 1001)
- **Read-only filesystem:** yes — tmpfs at `/tmp`, `/var/tmp`
- **Capabilities:** none (`cap_drop: ALL`)
- **Writable volumes:** `g8ee-data:/data` only
- **Config/shared mounts:** `./shared:/app/shared:ro`, `g8es-ssl:/g8es:ro`
- **Internal Auth:** Receives `G8E_INTERNAL_AUTH_TOKEN` via environment during bootstrap; discovers authoritative token from g8es/SSL volume at runtime.
- **Security:** `cap_drop: ALL`, `no-new-privileges:true`, hardened sysctls (`accept_redirects=0`, `send_redirects=0`)
- **Healthcheck:** `curl -f -k https://localhost/health` (internal port 443)

### g8ed (`g8ed`)

Web frontend and single external entry point. Node.js.

- **User:** `g8e` (uid 1001, gid 1001)
- **Read-only filesystem:** yes — tmpfs at `/tmp`, `/var/tmp`
- **Capabilities:** none (`cap_drop: ALL`)
- **Writable volumes:** `g8ed-data:/data`
- **Config/shared mounts:** `./shared:/shared:ro`, `g8es-ssl:/g8es:ro`, `./docs:/docs:ro`, `./README.md:/readme/README.md:ro`, and specific file mounts from `./components/g8ed/` to `/app/` for hot-reload support.
- **Internal Auth:** Discovers authoritative token from g8es/SSL volume (`g8es-ssl:/g8es:ro`) at runtime.
- **Security:** `cap_drop: ALL`, `no-new-privileges:true`, hardened sysctls (`accept_redirects=0`, `send_redirects=0`), read-only root filesystem
- **Healthcheck:** `curl -f -k https://localhost/health` (internal port 443)

### g8es (`g8es`)

Platform persistence and pub/sub broker. Runs the `g8e.operator` binary in `--listen` mode.

- **User:** `g8e` (uid 1001, gid 1001)
- **Read-only filesystem:** yes — tmpfs at `/tmp`, `/var/tmp`
- **Capabilities:** `cap_add: NET_BIND_SERVICE`, `cap_drop: ALL`
- **Writable volumes:** `g8es-data:/data`, `g8es-ssl:/ssl`
- **Internal Auth:** Authoritative generator and enforcer of `X-Internal-Auth` token. Receives `G8E_INTERNAL_AUTH_TOKEN` via environment. Persists secrets exclusively to the `g8es-ssl` volume.
- **Security:** read-only root filesystem, `cap_add: NET_BIND_SERVICE`, `cap_drop: ALL`
- **Ports:** Exposes 9000 (HTTPS) and 9001 (WSS) for internal communication (no external ports)
- **Healthcheck:** `curl -f -k https://localhost:9000/health`

### g8e node (`g8ep`)

Unified management sidecar with Python and network tools. Always running alongside core services.

- **User:** `g8e` (uid 1001, gid 1001)
- **Base image:** `python:3.13-alpine`
- **Read-only filesystem:** no
- **Bind mounts:** `components/g8ep/scripts/`, `shared/`, `scripts/`
- **Capabilities:** `cap_add: NET_RAW, NET_ADMIN, SYS_PTRACE, SETUID, SETGID`, `cap_drop: ALL`
- **Security:** `cap_add: NET_RAW, NET_ADMIN, SYS_PTRACE, SETUID, SETGID`, `cap_drop: ALL`, `no-new-privileges: true`
- **Docker socket:** see [Docker Socket Threat Model](#docker-socket-threat-model) below
- **Notable env vars:** `RUNNING_IN_DOCKER=1`, `HOME=/home/g8e` signals to platform scripts that they are executing inside the container
- **Healthcheck:** `pgrep -x supervisord`

## Test Runners

g8e uses dedicated per-component test runner containers that are lean and parallel-buildable. These are defined in `docker-compose.yml` as a separate service group outside of the primary application services.

| Service | Component | Purpose |
|---------|-----------|---------|
| `g8ee-test-runner` | g8ee | Python/pytest/pyright unit and integration tests. |
| `g8ed-test-runner` | g8ed | Node.js/vitest unit and integration tests. |
| `g8eo-test-runner` | g8eo | Go/gotestsum tests and operator binary builder. |

These runners share the same user (uid 1001) as production services and mount the relevant component source for fast test cycles.

**Lifecycle Management:**
- Test-runners are excluded from default platform lifecycle commands (`up`, `rebuild`, `setup`, `reset`)
- Build test-runners explicitly via `./g8e platform rebuild test-runners`
- Run tests via `./g8e test <component>` which uses the appropriate test-runner container

## Non-Root Users

All production services (g8ee, g8ed, g8es) run as dedicated non-root users created in their respective Dockerfiles. The `user:` directive in compose reinforces this by specifying the numeric uid:gid directly — the image cannot override it.

| Service | User | UID | GID |
|---------|------|-----|-----|
| g8ee | `g8e` | 1001 | 1001 |
| g8ed | `g8e` | 1001 | 1001 |
| g8es | `g8e` | 1001 | 1001 |
| g8e node | `g8e` | 1001 | 1001 |

Dockerfile patterns:

**Debian (g8ee) — `python:3.13-slim` base:**
```dockerfile
RUN groupadd -g 1001 g8e && \
    useradd -u 1001 -g g8e -M -s /sbin/nologin g8e
USER g8e
```

**Alpine (g8ed, g8es) — `node:22-alpine3.23` / `alpine:3.23` base:**
```dockerfile
RUN addgroup -g 1001 g8e && \
    adduser -u 1001 -G g8e -H -D -s /sbin/nologin g8e
USER g8e
```

**Alpine (g8ep) — `python:3.13-alpine` base:**
```dockerfile
RUN addgroup -g 1001 g8e && \
    adduser -u 1001 -G g8e -D -s /bin/bash g8e && \
    addgroup docker 2>/dev/null || true && \
    addgroup g8e docker 2>/dev/null || true
USER g8e
```

## Security Hardening

### Resource Constraints

All services implement physical resource limits to prevent Denial of Service (DoS) from compromised or runaway processes:

- **Memory Limits:** Ranging from 512MB (g8es) to 4GB (g8ep).
- **PID Limits:** Restricts the number of concurrent processes to prevent fork bombs.

### Volume Security Options

Writable data volumes use native Docker mount options to restrict behavior:

- **`noexec`:** Prevents execution of binaries from the volume.
- **`nosuid`:** Prevents `setuid` bits from being respected.
- **`nodev`:** Prevents the creation of device nodes.

Applied to: `g8ee-data`, `g8ed-data`, `g8es-data`, `g8es-ssl`.

### Network Isolation

The backend network (`g8e-network`) uses a standard bridge driver:

- **Bridge network:** All services communicate over the `g8e-network` bridge. The network is not marked `internal: true` — external routing is not blocked at the Docker network level.
- **Gateway:** g8ed is the only service with published host ports (443, 80), making it the single external entry point by design.
- **Sysctls:** Hardened kernel parameters (`accept_redirects=0`, `send_redirects=0`) are applied to g8ee and g8ed. g8es and g8ep do not have `sysctls` directives.

### `no-new-privileges`

Applied to g8ee, g8ed, and g8ep:

```yaml
security_opt:
  - no-new-privileges:true
```

Prevents any process inside the container from gaining additional privileges via `setuid`/`setgid` binaries or file capabilities, even if a vulnerability allows code execution as an unexpected user.

Not applied to:
- **g8es** — no `security_opt` directive in compose.

### Capability Dropping

g8ee and g8ed drop all capabilities:

```yaml
cap_drop:
  - ALL
```

g8ep adds `NET_RAW`, `NET_ADMIN`, `SYS_PTRACE`, `SETUID`, `SETGID` capabilities and drops all others.

g8es adds `NET_BIND_SERVICE` capability and drops all others.

### Read-Only Filesystems

Services that only write to mounted volumes get a read-only root filesystem:

```yaml
read_only: true
tmpfs:
  - /tmp
  - /var/tmp
```

Applied to: **g8ee**, **g8ed**, **g8es**

Not applied to: **g8ep** and test runners.

## Docker Socket Threat Model

One service mounts `/var/run/docker.sock`: g8ep.

**The threat:** The Docker socket is equivalent to root on the host. A process with socket access can start privileged containers, read host filesystem paths, and escape the container isolation boundary.

**Why g8ep needs it:**

g8ep manages operator build and deployment workflows. It is a management tool, never public-facing.

Mitigation: g8ep runs as uid 1001 (not root). The `group_add: ${DOCKER_GID}` directive adds the host docker group to the container user, granting socket access without requiring root.

**How g8ed manages the g8ep operator (without the socket):**

g8ed's `G8ENodeOperatorService` manages operator processes inside the g8ep container via Supervisor XML-RPC over the internal network — it does not use `docker exec` or mount the Docker socket. The XML-RPC interaction is:

- Isolated to a single internal service (`G8ENodeOperatorService`)
- Never triggered by unauthenticated requests — operator sessions require a valid authenticated user session
- Not exposed on any public API path

## Volume Strategy

Volumes are categorized by write requirement:

**Production (`docker-compose.yml`):**

| Mount | Mode | Services |
|-------|------|----------|
| `g8ee-data:/data` | read-write | g8ee |
| `g8ed-data:/data` | read-write | g8ed |
| `g8es-data:/data` | read-write | g8es |
| `g8es-ssl:/ssl` | read-write | g8es |
| `g8ed-node-modules:/app/node_modules` | read-write | g8ed |
| `g8ed-test-node-modules:/app/components/g8ed/node_modules` | read-write | g8ed-test-runner |
| `./components/g8ee:/app/components/g8ee` | read-write | g8ee-test-runner |
| `./components/g8ed:/app/components/g8ed` | read-write | g8ed-test-runner |
| `./components/g8eo:/app/components/g8eo` | read-write | g8eo-test-runner |
| `./components/g8ep/scripts:/app/components/g8ep/scripts` | read-write | g8ep |
| `./scripts:/app/scripts` | read-write | g8ep, test-runners |
| `./shared:/app/shared` | read-only | g8ee, g8ee-test-runner |
| `./shared:/shared` | read-only | g8ed, g8ed-test-runner, g8eo-test-runner |
| `./components/g8ed/views:/app/views` | read-only | g8ed |
| `g8es-ssl:/g8es` | read-only | g8ee, g8ed, g8ep, test-runners |
| `./docs:/docs` | read-only | g8ed |
| `./README.md:/readme/README.md` | read-only | g8ed |
| `/var/run/docker.sock` | read-write | g8ep |

**Development additions:**
Development mode is handled via the `./g8e` CLI and by passing specific environment variables or Docker Compose profiles.

## Build Context

All Dockerfiles use the repo root as the build context (`context: .` in compose). This is required because each service copies from multiple top-level directories:

- **g8ee** — `components/g8ee/`, `shared/`
- **g8ed** — `components/g8ed/`, `shared/`
- **g8es** — `components/g8eo/`, `components/g8es/` (pre-built operator binaries for linux/amd64, arm64, 386 are generated during build)
- **g8ep** — `components/g8ee/`, `components/g8ed/`, `components/g8eo/`, `components/g8ep/`

No `.dockerignore` file exists at the repo root; the full build context (minus `.gitignore` patterns) is sent to the daemon.
