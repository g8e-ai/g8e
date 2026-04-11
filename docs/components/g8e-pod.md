# g8e node

Drop Pod is the always-on sidecar container that serves as the unified test environment for the g8e platform — a hermetic container for running all component tests (VSE/Python, VSOD/Node.js, VSA/Go) locally and in CI.

It runs as a managed service alongside `vsodb`, `vse`, and `vsod` in `docker-compose.yml`. Tests are never run directly on the host — all test execution goes through the g8e-pod.

**Operator binary:** The `g8e.operator` binary lives at `/home/g8e/g8e.operator`. When the binary is not present, `fetch-key-and-run.sh` automatically downloads the platform-matching binary from the VSODB blob store (`/blob/operator-binary/linux-{arch}`). VSODB bakes and uploads binaries for all architectures on startup, so a fresh `./g8e platform setup` or `./g8e platform up` is sufficient. To compile a fresh binary manually, use `./g8e operator build`.

---

## Location

```
components/g8e-pod/
├── Dockerfile                    # Container definition (ubuntu:24.04 base, Python 3.12, Node 22, Go 1.24.1)
├── reports/                      # Scan output (gitignored, .gitkeep preserves dir)
└── scripts/
    ├── entrypoint.sh             # Writes supervisor config, execs supervisord as PID 1
    ├── fetch-key-and-run.sh      # Downloads operator binary from blob store if absent, fetches API key, execs operator
    └── security/                 # Security scan scripts (tools lazy-installed at runtime, not baked into image)
        ├── install-scan-tools.sh # Lazy-installs Nuclei, testssl.sh, Trivy, Grype on first use
        ├── run-full-audit.sh     # Orchestrates all scanners against a target
        ├── scan-tls.sh           # TLS/SSL configuration audit via testssl.sh
        ├── scan-nuclei.sh        # Template-based web vulnerability scan via Nuclei
        ├── scan-containers.sh    # Container image CVE scan via Trivy
        ├── scan-dependencies.sh  # Dependency CVE scan via Grype
        └── fetch-public-grades.sh # Queries SSL Labs, Mozilla Observatory, SecurityHeaders.com
```

---

## Container Image

**Base:** `ubuntu:24.04`

**Runtimes installed at build time:**

| Runtime | Version |
|---------|---------|
| Python | 3.12 (`python3.12` APT package) |
| Node.js | 22 (ARG `NODE_VERSION`, via NodeSource) |
| Go | 1.24.1 (ARG `GO_VERSION`, auto-detected arch — supports `amd64` and `arm64`) |
| AWS CLI | Not installed (official installer removed) |

**Tools installed at build time:**

| Tool | Version | Purpose |
|------|---------|---------|
| **System Base** |||
| bash | system | Shell environment |
| ca-certificates | system | CA trust store |
| gnupg | system | GPG for package verification |
| jq | system | JSON processing |
| make | system | Build automation |
| openssl | system | SSL/TLS toolkit |
| python3.12 | system | Python runtime (with dev, pip, venv) |
| supervisor | system | Process supervisor — manages the operator as a service |
| uuid-runtime | system | UUID generation |
| **Network Tools** |||
| curl | system | HTTP client |
| dnsutils | system | DNS troubleshooting (dig, nslookup) |
| iperf3 | system | Network bandwidth testing |
| ipcalc | system | IP address calculator |
| iproute2 | system | Network routing (ip, ss commands) |
| iputils-ping | system | Ping utility |
| iftop | system | Real-time network bandwidth monitor |
| mtr | system | Network diagnostic (traceroute + ping) |
| nethogs | system | Per-process network bandwidth monitor |
| netcat-openbsd | system | Network debugging (nc) |
| net-tools | system | Netstat, ifconfig legacy tools |
| nmap | system | Network discovery and security scanning |
| socat | system | Multipurpose socket relay |
| tcpdump | system | Packet capture and analysis |
| telnet | system | Remote terminal client |
| traceroute | system | Route tracing utility |
| whois | system | Domain registration lookup |
| **Development** |||
| gcc/g++ | system | C/C++ compiler (Go race detector, Python C extensions) |
| git | system | Version control |
| wget | system | HTTP/FTP download utility |
| **System Utilities** |||
| htop | system | Interactive process viewer |
| lsof | system | List open files |
| rsync | system | File synchronization |
| ssh-client | system | SSH remote access client |
| strace | system | System call tracer |
| unzip/zip | system | Archive utilities |
| **Docker** |||
| docker-ce-cli | latest | Docker client (daemon via host socket) |
| docker-compose-plugin | latest | Docker Compose plugin |
| **Go Tools** |||
| gotestsum | v1.12.1 | Structured Go test output (pinned) |

**Application dependencies pre-installed at build time** (layer-cached):
- Python: `components/vse/requirements.txt` → `pip install` into a venv at `/opt/venv` (required by PEP 668 on Ubuntu 24.04 — system-wide pip installs are blocked)
- Node: `components/vsod/package*.json` → `npm ci`
- Go: `components/vsa/go.mod` + `go.sum` + `vendor/` → vendored, no network download

Component source directories are **volume-mounted** at runtime — code changes never require a rebuild.

---

## Docker Compose Integration

Drop Pod is a managed service in `docker-compose.yml`, started alongside the core platform. It has no `depends_on` constraints and no `restart` policy — it stays running as a sidecar.

**Healthcheck:** `pgrep -x supervisord` — passes once supervisord is running.

**Process model:** `supervisord` runs as PID 1 and manages the `operator` program. The operator service is configured with `autostart=false` — it starts only when VSOD persists the operator API key to the `platform_settings` document in VSODB and then signals supervisor via XML-RPC. The supervisor `command=` delegates to `fetch-key-and-run.sh`, which fetches the API key from VSODB (with retry and exponential backoff for transient unavailability), validates it, and execs the operator binary. Operator stdout/stderr routes to Docker log output (visible via `./g8e platform logs g8e-pod` or `docker logs g8e-pod`).

**Volume mounts (runtime):**

| Host path | Container path | Notes |
|-----------|---------------|-------|
| `./components/vse/app` | `/app/components/vse/app` | VSE application source |
| `./components/vse/config` | `/app/components/vse/config` | VSE configuration |
| `./components/vse/tests` | `/app/components/vse/tests` | VSE tests |
| `./components/vse/pyproject.toml` | `/app/components/vse/pyproject.toml` | VSE dependencies |
| `./components/vsod` | `/app/components/vsod` | VSOD source |
| (named volume) `g8e-dashboard-node-modules` | `/app/components/vsod/node_modules` | Node modules isolated in a named volume |
| `./components/vsa` | `/app/components/vsa` | VSA source (operator) |
| `./components/g8e-pod/scripts` | `/app/components/g8e-pod/scripts` | g8e node scripts |
| `./shared` | `/app/shared` | Shared models and constants |
| `./scripts` | `/app/scripts` | Platform scripts |
| `/var/run/docker.sock` | `/var/run/docker.sock` | Docker socket |
| (named volume) `g8e-data-ssl` | `/vsodb` | VSODB SSL volume — read-only |

**Environment variables (set in the service block — always present):**

| Variable | Value | Purpose |
|----------|-------|---------|
| `HOME` | `/home/g8e` | User home directory |
| `G8E_DB_PATH` | `/data/g8e.db` | VSE local SQLite path (for VSE tests) |
| `G8E_SSL_DIR` | `/vsodb` | Path to the SSL directory |
| `G8E_PUBSUB_CA_CERT` | `/vsodb/ca.crt` | CA cert for pub/sub TLS |
| `G8E_SSL_CERT_FILE` | `/vsodb/ca.crt` | System trust store injection |
| `REQUESTS_CA_BUNDLE` | `/vsodb/ca.crt` | Python requests CA trust |
| `RUNNING_IN_DOCKER` | `1` | Signals container context to scripts |
| `G8E_INTERNAL_AUTH_TOKEN` | — | Shared secret for inter-service authentication. Loaded from `/vsodb/internal_auth_token` by entrypoint. |
| `G8E_SESSION_ENCRYPTION_KEY` | — | Session encryption key. Loaded from `/vsodb/session_encryption_key` by entrypoint. |

**Environment variables (set in the shell or via `docker-compose.yml` `environment:` — configured per deployment):**

| Variable | Typical test value | Purpose |
|----------|--------------------|---------|
| `ENVIRONMENT` | `test` | Runtime environment flag |
| `CI` | `false` | CI flag |
| `G8E_INTERNAL_HTTP_URL` | `https://vsodb` | VSODB HTTP endpoint |
| `G8E_INTERNAL_PUBSUB_URL` | `wss://vsodb` | VSODB pub/sub endpoint for internal services (VSE) |
| `G8E_OPERATOR_PUBSUB_URL` | `wss://g8e.local:443` | VSODB pub/sub endpoint for VSA operator tests — uses the external address via `extra_hosts: g8e.local→host-gateway` to simulate a real remote operator |
| `LLM_ENDPOINT` | — | LLM inference endpoint |
| `APP_URL` | — | Application URL (required — set in `docker-compose.yml` or shell) |

---

## Operator in g8e node

The g8e-pod container hosts the `g8e.operator` binary at `/home/g8e/g8e.operator`. The binary must be built explicitly via `./g8e operator build` before use — it is never built automatically. VSOD runs the binary inside the container via `docker exec` in two scenarios:

1. **Login-triggered activation** — automatically on every user login or registration (see below).
2. **Fleet streaming** — on-demand via the `operator stream` command.

**Environment variable:**

| Variable | Default | Purpose |
|----------|---------|-------|
| `G8E_GATEWAY_OPERATOR_ENDPOINT` | `g8e.local` | `--endpoint` passed to the operator binary when launched by VSOD |

The VSOD service exposes `g8e.local` as a network alias on `vso-network`. The g8e-pod container shares that network, so an operator running inside it resolves `g8e.local` to the VSOD container and reaches VSODB on port 443 exactly as a real remote operator would.

### Login-Triggered Activation (`G8ENodeOperatorService`)

On every successful login or registration, VSOD's `G8ENodeOperatorService` fires a non-blocking activation flow so that the user's g8e-pod operator is `ACTIVE` and ready to be bound by the time their browser finishes loading.

**Flow:**

```
POST /api/auth/login (or /register)
  └─ session created
       └─ fire-and-forget: activateG8ENodeOperatorForUser(user_id, org_id, web_session_id)
            │
            ├─ 1. getG8ENodeOperatorForUser(user_id)
            │       → queryOperators: returns the g8e-pod slot for this user
            │       → if already ACTIVE/BOUND: done (idempotent)
            │       → if no slot: done (graceful no-op)
            │
            └─ 2. launchG8ENodeOperator(apiKey)
                    → reads operator.api_key from the operator document
                    → savePlatformSettings({ g8e_pod_operator_api_key: apiKey })
                    → XML-RPC supervisor.startProcess('operator')
                    → supervisord runs fetch-key-and-run.sh:
                          curl VSODB /db/settings/platform_settings (with retry)
                          → if binary absent: downloads from blob store (/blob/operator-binary/linux-{arch})
                          → extracts g8e_pod_operator_api_key
                          → exec g8e.operator --endpoint g8e.local --working-dir /home/g8e
                          → operator discovers CA cert locally at /vsodb/ca.crt (no network fetch)
                    → binary authenticates with API key, slot is claimed, operator goes ACTIVE
                    → SSE delivers OPERATOR_STATUS_UPDATED to the browser
```

Failures at any step are caught and logged as warnings — they never propagate to the login response.

### Standalone Reauth (`operator reauth`)

To force a reauth outside of the login flow — for example when the g8e-pod operator is stuck or unresponsive — use:

```bash
./g8e operator reauth --email user@example.com
./g8e operator reauth --user-id <user-id>
```

This calls `POST /api/internal/operators/user/:userId/reauth` on VSOD, which:

1. Stops the supervised operator service via XML-RPC `supervisor.stopProcess` (no-op if already stopped)
2. Resets the operator slot to `AVAILABLE` (full delete + recreate via `resetOperator`)
3. Reads the fresh `operator_api_key` returned by `resetOperator`
4. Persists the new API key to the `platform_settings` document in VSODB and signals `supervisor.startProcess` via XML-RPC

The operator re-authenticates and goes `ACTIVE` within seconds. The operation is idempotent — safe to call whether or not the operator is currently running.

> The g8e-pod operator is a **system operator** (`operator_type: system`). It authenticates using its `operator_api_key` from the operator document — no device link, no `--cloud` flag. AWS CLI is **not installed** in the container; however, if it were, the host `~/.aws` directory would typically be mounted for credentials.

---

## Running Tests

The g8e-pod container runs `entrypoint.sh` on startup, writes the supervisor config, and execs `supervisord` as PID 1. Tests are executed via `docker exec` — the `g8e` bash script routes `test` commands into the g8e-pod and invokes `run_tests.sh` directly inside it.

See [testing.md](../testing.md) for complete test execution documentation — g8e-pod environment details, all `./g8e test` commands, component-specific guides, and CI workflows.

---

## Security Scans

Security scan scripts live in `components/g8e-pod/scripts/security/` and are volume-mounted into the container at runtime. The scanning tools (Nuclei, testssl.sh, Trivy, Grype) are **not** pre-installed in the image — they are lazy-installed on first use by `install-scan-tools.sh`. All network utilities required by the scan scripts (`wget`, `unzip`, `nmap`, etc.) are pre-installed in the base image.

See [testing.md](../testing.md) for security scan commands and script documentation.

---

## Managing the g8e node Image

Source code changes never require a rebuild. Rebuild only when the image definition changes:

| Changed file | Action |
|-------------|--------|
| `components/g8e-pod/Dockerfile` | `./g8e platform rebuild g8e-pod` |
| `components/vse/requirements.txt` | `./g8e platform rebuild g8e-pod` |
| `components/vsod/package*.json` | `./g8e platform rebuild g8e-pod` |
| `components/vsa/go.mod` / `go.sum` | `./g8e platform rebuild g8e-pod` |

```bash
# Rebuild g8e-pod image only
./g8e platform rebuild g8e-pod

# Clean g8e-pod image (full removal)
./g8e platform clean --clean-g8e-pod
```

---

## Ephemeral SSH Deployment (`operator stream`)

The `stream` subcommand is a Go-native concurrent SSH engine built directly into the `g8e.operator` binary. It injects the operator to one or more remote hosts simultaneously without ever writing to local disk, using `golang.org/x/crypto/ssh` — no system `ssh` binary, no external dependencies.

**Architecture:**

```
g8e-pod container
  │
  ├─ Load linux/<arch>/g8e.operator into RAM (once)
  │
  └─ Goroutine pool (default: 50 concurrent)
       │
       ├─ Parse ~/.ssh/config (inline parser, no external deps)
       │   → resolves HostName, User, Port, IdentityFile per host
       │
       ├─ Dial via crypto/ssh + agent / identity files
       │
       ├─ Pipe binary buffer to remote stdin
       │
       └─ Remote: mktemp → cat > $B → chmod +x → trap EXIT → run & wait
                                                               ↑
                                                    binary deleted on any exit
```

The binary is loaded into memory once, then fanned out across all goroutines. Each goroutine opens an SSH session, pipes the buffer to stdin, and runs the trap-based ephemeral script. When the operator exits or the connection drops, the remote `EXIT` trap fires and deletes the tmpfile. Context cancellation (Ctrl+C) propagates to all goroutines simultaneously.

**Usage:**

```bash
# Build operator binaries first (required before operator stream):
./g8e operator build

# Stream to a single host
./g8e operator stream myhost --endpoint 192.168.1.10 --device-token dlk_xxx

# Stream to multiple hosts concurrently
./g8e operator stream host1 host2 host3 --endpoint 10.0.0.1 --device-token dlk_xxx

# Stream to 1,000 nodes from a file (100 concurrent sessions)
./g8e operator stream --hosts /etc/g8e/fleet.txt \
  --concurrency 100 --endpoint 10.0.0.1 --device-token dlk_xxx

# Read hosts from stdin
cat hosts.txt | ./g8e operator stream --hosts - --endpoint 10.0.0.1 --device-token dlk_xxx

# All flags
./g8e operator stream [host...] \
  --arch amd64|arm64|386        \  # default: amd64
  --hosts <file|->              \  # file of hosts (one per line) or - for stdin
  --concurrency <N>             \  # max parallel SSH sessions (default: 50)
  --timeout <secs>              \  # per-host dial+inject timeout (default: 60)
  --endpoint <host>             \  # starts operator if set
  --device-token <tok>          \  # device link token (`dlk_` prefix)
  --key <apikey>                \  # API key auth
  --no-git                      \  # disable Ledger
  --ssh-config <path>              # custom SSH config path (default: ~/.ssh/config)
```

**JSON lines output (stdout):**

Each host emits a status line as it completes; a summary line is written last. Stderr carries human-readable progress. This separation makes `stream` scriptable and future Web UI-ready.

```jsonl
{"host":"host1","status":"done","size_bytes":15234567,"elapsed_ms":1820,"ts":"2026-03-02T23:04:02Z"}
{"host":"host2","status":"failed","error":"dial tcp: connection refused","elapsed_ms":60001,"ts":"2026-03-02T23:04:03Z"}
{"summary":true,"status":"summary","total":2,"success":1,"failed":1,"total_ms":60043,"ts":"2026-03-02T23:04:03Z"}
```

**Signal propagation / cleanup guarantees:**

| Event | Result |
|-------|--------|
| Normal exit | Operator exits, remote `EXIT` trap fires, binary deleted |
| Network g8e | SSHD kills the remote shell, `EXIT` trap fires, binary deleted |
| Ctrl+C on host | Go context cancels, all SSH sessions close, remote `EXIT` traps fire |

**SSH config resolution:** The inline parser reads `~/.ssh/config` (volume-mounted from the host into g8e-pod at `/root/.ssh`) and resolves `HostName`, `User`, `Port`, and `IdentityFile` per host. Wildcard patterns (`prod-*`, `?`) are supported. SSH agent (`SSH_AUTH_SOCK`) is used when available; identity files fall back to standard paths (`id_ed25519`, `id_ecdsa`, `id_rsa`).

**vs. `operator deploy`:** `deploy` copies the binary to a persistent path on disk via `scp` and optionally starts it with `nohup`. `stream` is zero-footprint — the binary is always volatile and the session is live for its lifetime.

### SSH Config Setup (`operator ssh-config`)

Before streaming to many hosts, your `~/.ssh/config` must be configured for multiplexing — otherwise every connection performs a full key exchange (~1.5s overhead) rather than reusing an existing socket (~milliseconds).

```bash
# Inspect what will be written, without touching anything
./g8e operator ssh-config --print

# Apply: create ~/.ssh/sockets/ and append the g8e stanza
./g8e operator ssh-config

# Replace an existing g8e stanza (e.g. after updating g8e)
./g8e operator ssh-config --force
```

The command is idempotent — running it twice without `--force` is safe; it detects the existing stanza and exits cleanly.

**What it configures (global `Host *` block):**

| Directive | Value | Purpose |
|-----------|-------|---------|
| `ControlMaster` | `auto` | Reuse a single TCP connection per host |
| `ControlPath` | `~/.ssh/sockets/%r@%h:%p` | Socket file path for the multiplexed connection |
| `ControlPersist` | `1h` | Master connection stays open 1 hour after the last session |
| `ServerAliveInterval` | `30` | Keep-alive ping every 30s — prevents firewalls from killing the idle pipe |
| `ServerAliveCountMax` | `3` | Drop after 3 missed pings (90s total dead time) |
| `StrictHostKeyChecking` | `accept-new` | Auto-accept unknown host keys; never prompts — required for scripted 1,000-node runs |
| `Compression` | `yes` | Reduces binary transfer size over the wire |
| `ForwardAgent` | `no` | Never forward the SSH agent to remote hosts |

The stanza is wrapped in `# BEGIN g8e operator-stream config` / `# END g8e operator-stream config` markers so it can be detected and replaced cleanly.

**Per-host overrides** can still be added to `~/.ssh/config` after the global block — SSH applies the first matching rule per directive, so host-specific blocks placed before or after the `Host *` block will take precedence as expected:

```sshconfig
# Example: production nodes behind a jump host
Host prod-*
    User deploy
    IdentityFile ~/.ssh/prod_ed25519
    ProxyJump jump.example.com

# Example: jump host itself
Host jump.example.com
    HostName 1.2.3.4
    User admin
    Port 2222
```

---

## VSOD Operator Panel UI

VSOD manages the g8e-pod operator through the Operator Panel in the browser UI. The panel is implemented in `components/vsod/public/js/components/operator-panel.js` and its mixin modules.

The g8e-pod operator is sorted to the top of the operator list — it is identified by the `is_g8e_pod` flag on the operator document and always renders first, above all remote operators.

**Actions available per operator card in the UI:**

| Button | Action | API call |
|--------|--------|----------|
| Device Link | Generate a `dlk_` token and copy to clipboard | `POST /api/auth/link/generate` (body: `{ operator_id }`) |
| Restart g8e-pod | Stop the supervised process, reset slot, relaunch with fresh token | `POST /api/operators/g8e-pod/reauth` |
| Copy API Key | Copy `operator_api_key` to clipboard | `GET /api/operators/:operatorId/api-key` |
| Refresh API Key | Terminate operator, create new slot with new API key | `POST /api/operators/:operatorId/refresh-key` |
| Bind / Unbind | Bind operator to the current web session (or unbind/clear stale) | `POST /api/operators/:operatorId/bind` / `/unbind` |
| Stop | Send shutdown command to a running operator | `POST /api/operators/:operatorId/stop` |

**Restart g8e-pod flow (UI-triggered):**

`POST /api/operators/g8e-pod/reauth` is an authenticated user-facing route on VSOD — the caller's identity comes from the session (`req.userId`). It calls `relaunchG8ENodeOperatorForUser` in `G8ENodeOperatorService`, which is the same function used by the internal CLI route. No `operator_id` parameter is required because each user has exactly one g8e-pod operator slot.

**SSE updates:** The panel subscribes to `EVENTS.OPERATOR.DATA_UPDATED` events emitted by `OperatorSSEHandler`. Heartbeat events update the metrics display in place; state-change and stale events trigger a full operator list re-render.

---

## Operator Test VMs

Operator test VMs are not defined in `docker-compose.yml`. Multi-operator integration testing is performed by streaming the operator binary to remote hosts via `operator stream`, or by running the binary directly against the VSODB pub/sub endpoint. See the demo under `demo/` for an example multi-operator configuration.

---

## Related Documentation

| Document | Description |
|----------|-------------|
| [testing.md](../testing.md) | Complete testing guide — g8e-pod environment, all test commands, component-specific guides, CI workflows |
| [developer.md](../developer.md) | Platform setup, infrastructure, code quality rules |
