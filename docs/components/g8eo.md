---
title: g8eo
parent: Components
---

# g8eo — g8e Operator

g8eo is the Go-based reference implementation of the Operator for the g8e platform. It provides language-agnostic, platform-agnostic, secure, real-time command execution and file management for remote system operations. Any client that adheres to the g8e events protocol can act as an Operator.

> For deep-reference security documentation — CA trust bootstrap, mTLS, fingerprint binding, replay protection, operator binding, Sentinel pre-execution threat detection, output scrubbing patterns, LFAA vault encryption, and the Ledger — see [architecture/security.md](../architecture/security.md).

---

## Core Principles

- **Zero-trust security**: Every operation requires authentication; nothing is implicitly trusted
- **Human-in-the-loop**: Every command requires explicit user approval before execution
- **Data sovereignty**: Command output stays local by default; only metadata travels to the cloud
- **Defense in depth**: Multiple security layers — mTLS, certificate pinning, Sentinel platform-wide protection (threat detection + multi-layer scrubbing)
- **Outbound-only connectivity**: g8eo initiates all connections; no inbound ports required

---

## Lifecycle

### Outbound Mode (Default)

g8eo operates as a remote Operator that initiates outbound connections to the platform on port 443 (WSS/HTTPS). The lifecycle follows this sequence:

1. **Bootstrap**: Load CA certificate (local-first from volume mounts, fallback to HTTPS fetch), authenticate with g8ed via API key or device token, receive operator configuration (operator ID, session ID, heartbeat interval, mTLS cert)
2. **Service Initialization**: Instantiate ExecutionService, FileEditService, storage services (LocalStore, RawVault, AuditVault, Ledger), PubSubClient, PubSubResultsService, PubSubCommandService, and Sentinel
3. **Command Loop**: Subscribe to `cmd:{operator_id}:{session}` channel via WebSocket, receive commands, run Sentinel pre-execution analysis, execute locally, publish results to `results:{operator_id}:{session}` channel
4. **Heartbeat**: Publish system metrics to `heartbeat:{operator_id}:{session}` channel at configured interval (default 30s)
5. **Shutdown**: Gracefully stop all services on SIGINT/SIGTERM or remote shutdown request

### Listen Mode (`--listen`)

In Listen Mode, g8eo acts as g8es — the platform's central backend and pub/sub broker. It listens for inbound connections from g8ee, g8ed, and outbound Operators on port 443. It provides SQLite document/KV/blob storage and WebSocket pub/sub messaging but does not execute commands.

### OpenClaw Mode (`--openclaw`)

Connects to an OpenClaw Gateway as a Node Host. No g8e infrastructure required — the Operator advertises shell capabilities and executes commands on behalf of the Gateway.

### MCP Satellite Mode

The Operator supports the Model Context Protocol (MCP) as a native satellite through a protocol translator layer (`services/mcp/translator.go`) that maps MCP tool names to internal g8e event types. MCP payloads are subject to the same Sentinel pre-execution analysis and output scrubbing as native events.

---

## Architecture

```
┌─────────────────────────────────────────────────────────────────┐
│                         g8eo (Operator)                          │
├─────────────────────────────────────────────────────────────────┤
│  Config          │  Models        │  Services                   │
│  - Environment   │  - Commands    │  - Gateway Protocol (WS)   │
│  - CLI flags     │  - File edits  │  - Command execution       │
│  - .env support  │  - Heartbeat   │  - File operations         │
│                  │  - Wire format │  - Directory listing       │
│                  │                │  - Sentinel (threat det.   │
│                  │                │    + output scrubbing)     │
│                  │                │  - Scrubbed/raw vault      │
│                  │                │  - Audit vault + ledger    │
│                  │                │  - History handler         │
└─────────────────────────────────────────────────────────────────┘
                              │
                              ▼ WebSocket (mTLS)
┌─────────────────────────────────────────────────────────────────┐
│                           g8ed                                  │
│     Gateway Protocol — bridges Operators ↔ g8es pub/sub        │
└─────────────────────────────────────────────────────────────────┘
                              │
                              ▼
┌─────────────────────────────────────────────────────────────────┐
│                         g8es                                   │
│  ┌─────────────┐  ┌─────────────┐  ┌─────────────────────────┐ │
│  │  Pub/Sub    │  │  Document   │  │  KV Store               │ │
│  │  (channels) │  │  Store      │  │  (operator sessions)    │ │
│  └─────────────┘  └─────────────┘  └─────────────────────────┘ │
└─────────────────────────────────────────────────────────────────┘
                              │
              ┌───────────────┴───────────────┐
              ▼                               ▼
┌─────────────────────────┐       ┌─────────────────────────┐
│       g8ee (AI Engine)   │       │   g8ed (Dashboard)      │
│  - Command dispatch     │       │  - Operator panel UI    │
│  - Result aggregation   │       │  - SSE broadcast        │
│  - Session management   │       │  - Status computation   │
└─────────────────────────┘       └─────────────────────────┘
```

---

## Canonical Truths

All constants, event types, and status values are defined in shared JSON files that g8eo mirrors as compile-time Go constants. Contract tests enforce alignment across components.

- **Event types**: `shared/constants/events.json` — canonical source for all event type strings (e.g., `g8e.v1.operator.command.requested`)
- **Status values**: `shared/constants/status.json` — canonical source for status enums (e.g., `execution.status`, `g8e.status`, `vault.mode`)
- **Channels**: `shared/constants/channels.json` — canonical source for channel naming patterns (e.g., `cmd:{operator_id}:{session}`)
- **Agent definitions**: `shared/constants/agents.json` — canonical source for AI agent personas and tool configurations

g8eo's `constants/` package duplicates these as Go constants for compile-time safety. The `contracts/` directory contains cross-component contract tests that verify the Go constants match the JSON source of truth.

---

## Data Flow

### Command Execution

```
User approves command
        │
        ▼
g8ee ──publishes──► g8es pub/sub: cmd:{id}:{session}
        │
        ▼
g8ed ──bridges──► g8eo WebSocket
        │
        ▼
g8eo executes command locally
        │
        ▼
g8eo ──publishes──► g8es pub/sub: results:{id}:{session}
        │
        ▼
g8ed ──bridges──► g8ee
        │
        ▼
g8ee returns result to AI
```

### Heartbeat Flow

g8eo calls `buildHeartbeat()` at the configured interval (default 30 seconds; overridable via `--heartbeat-interval` flag at startup or `HeartbeatIntervalSeconds` in bootstrap config), collects system metrics, then calls `PublishHeartbeat()` to send the payload over the Gateway WebSocket to g8es pub/sub. From there, g8ee and g8ed handle persistence and SSE fan-out.

---

## Storage

g8eo maintains four independent local stores on the Operator machine. All are SQLite-based (WAL mode) except the Ledger, which is a Git repository. All data resides in the `.g8e/` directory within the working directory.

| Store | Path | Purpose |
|---|---|---|
| Scrubbed Vault | `.g8e/local_state.db` | Sentinel-scrubbed command output and file diffs; AI reads from here |
| Raw Vault | `.g8e/raw_vault.db` | Unscrubbed full output; never transmitted to the platform |
| Audit Vault | `.g8e/data/g8e.db` | LFAA structured event timeline; sensitive fields encrypted at rest; **Head/Tail truncation** for outputs >100KB |
| Ledger | `.g8e/data/ledger` | Git-backed cryptographic version history for all Operator-modified files |

Local storage is enabled by default (`-s`). Disabled with `-s=false` (full output sent to cloud instead). The Ledger requires a functional `git` binary; disabled via `--no-git`.

For complete schema DDL, exact column definitions, retention policies, vault encryption details, and data flow specifics, see [architecture/storage.md](../architecture/storage.md).

---

## Security

For full details on every g8eo security layer — CA trust bootstrap, mTLS, fingerprint binding, replay protection, operator binding, Sentinel pre-execution threat detection, output scrubbing, LFAA vault encryption, and the Ledger — see [architecture/security.md](../architecture/security.md).

### Defense Layers

1. **API Key Authentication** — Required for all operations before any bootstrap config is returned
2. **mTLS (Mutual TLS)** — Both sides present certificates; g8eo rejects the connection if the server certificate isn't signed by the pinned CA
3. **CA Trust Bootstrap** — Local-first discovery: scans volume mount paths (`/ssl/ca.crt`, `/g8es/ca.crt`, `/g8es/ssl/ca.crt`, `/data/ssl/ca.crt`) before falling back to an HTTPS fetch from `https://<endpoint>/ssl/ca.crt`. Only trusts that exact CA; never embedded in the binary
4. **TLS Kill Switch** — Self-terminates with exit code 7 (`ExitCertTrustFailure`) on cert verification failure; connection never downgraded
5. **Fingerprint Binding** — System fingerprint permanently locked to the Operator slot on first auth; mismatches rejected
6. **Replay Protection** — `X-Request-Timestamp` (±5 min window) + optional `X-Request-Nonce` validated against nonce cache
7. **Explicit Session Binding** — Operator cannot receive commands until a user explicitly binds their web session
8. **Sentinel Pre-Execution** — Dangerous commands blocked on the host before execution (46 MITRE ATT&CK-mapped categories). Part of the platform-wide Sentinel protection that also includes multi-layer data scrubbing on both the Operator and AI Engine.
9. **Human Approval** — Every state-changing command requires explicit user consent
10. **LFAA Audit Logging** — All operations logged to the local audit vault
11. **Process Sovereignty** — The user terminates the process; no platform action can keep a dead Operator alive

### Exit Codes

| Code | Meaning | Retryable |
|------|---------|-----------|
| 0 | Success | — |
| 1 | General error | Maybe |
| 2 | Auth failure (API Key / Device Token) | No |
| 3 | Permission denied | No |
| 4 | Network error | Yes |
| 5 | Config error | No |
| 6 | Storage error | No |
| 7 | TLS cert failure | No |

---

## Authentication

### Methods

| Method | Use Case | How It Works |
|--------|----------|--------------|
| **API Key** | Traditional | Pass `--key` or set `G8E_OPERATOR_API_KEY` |
| **Device Link** | Deployment | Auth via `--device-token` or `G8E_DEVICE_TOKEN`; returns session ID |

### Session Naming

- All session ID fields use `snake_case` in JSON/API payloads (`operator_session_id`)
- Channel pattern: `cmd:{operator_id}:{operator_session_id}`
- Canonical channel listing and wire format: `shared/constants/channels.json`

### Multi-Operator Binding

Multiple g8eo instances can bind to the same operator session, enabling:
- Single conversation interface for multiple systems
- Batch execution across all bound operators after one approval
- Targeting by hostname, operator ID, or list index

### Internal Platform Authentication

g8e components (g8ed, g8ee, and g8eo in `--listen` mode) communicate over an internal network using a shared secret called the `internal_auth_token`.

- **Authoritative Source**: The `g8es-ssl` volume (mounted at `/g8es/ssl`) is the sole authoritative source of truth. The token is stored in plain text at `/g8es/ssl/internal_auth_token`.
- **Generation**: On the first platform start, g8es (g8eo in `--listen` mode) generates a cryptographically secure 32-byte hex token if one does not exist and writes it to the SSL volume.
- **Discovery**: g8ed and g8ee discover this token by reading the file from the shared volume at startup.
- **Enforcement**: Every internal HTTP request must include the `x-internal-auth` header matching this token.
- **Diagnostics**: The `./g8e platform settings` command displays a truncated version of the active token (e.g., `f5037487...6c5f`) for verification.

---

## CLI Reference

### Core Options

| Flag | Env Var | Default | Description |
|------|---------|---------|-------------|
| `-k` / `--key` | `G8E_OPERATOR_API_KEY` | — | API key for auth |
| `-S` / `--operator_session` | `G8E_OPERATOR_SESSION_ID` | — | Pre-authorized session ID |
| `-D` / `--device-token` | `G8E_DEVICE_TOKEN` | — | Device link token |
| `-e` / `--endpoint` | `G8E_OPERATOR_ENDPOINT` | g8e.local | Operator endpoint |
| `--ca-url` | — | — | Override URL for hub CA certificate fetch |
| `--wss-port` | — | 443 | WSS port to dial on g8es for pub/sub |
| `--http-port` | — | 443 | HTTPS port to dial for auth/bootstrap |
| `-s` / `--local-storage` | — | true | Local storage mode — store output locally instead of cloud |
| `-l` / `--log` | `G8E_LOG_LEVEL` | info | Log level |
| `-G` / `--no-git` | — | false | Disable Ledger (git-backed file versioning) |
| `--heartbeat-interval` | — | 30s | Heartbeat interval (e.g. `60s`, `2m`); overrides the 30s default |
| `--working-dir` | — | launch dir | Working directory for commands and data storage |
| `-c` / `--cloud` | — | true | Cloud Operator mode — unlocks cloud CLI tools. Always set on g8ep. |
| `-p` / `--provider` | — | — | Cloud provider: `aws`, `gcp`, `azure`. Required for Cloud Operator. |
| `-v` / `--version` | — | — | Show version |

### Listen Mode

| Flag | Default | Description |
|------|---------|-------------|
| `--listen` | false | Enable listen mode |
| `--wss-listen-port` | 443 | WSS/TLS port for operator pub/sub connections |
| `--http-listen-port` | 443 | HTTPS port for internal g8ee/g8ed traffic |
| `--data-dir` | `.g8e/data` in working directory | SQLite data directory |
| `--ssl-dir` | `data-dir/ssl` | Directory for TLS certificates |
| `--tls-cert` | — | Path to TLS certificate file |
| `--tls-key` | — | Path to TLS private key file |

### Vault Management

| Flag | Description |
|------|-------------|
| `--rekey-vault` | Re-encrypt vault with new key |
| `--old-key` | Old API key (required for rekey) |
| `--verify-vault` | Verify vault integrity |
| `--reset-vault` | Destroy all vault data |

---

## Dependencies

### Runtime (Compiled into Binary)

| Package | Purpose |
|---------|---------|
| `gorilla/websocket` | WebSocket client for Gateway Protocol |
| `modernc.org/sqlite` | Pure Go SQLite (no CGO) |
| `golang.org/x/crypto` | Encryption + SSH for `stream` subcommand |

### Test-Only

| Package | Purpose |
|---------|---------|
| `stretchr/testify` | Test assertions and mocking |

---

## Testing

See [testing.md — g8eo](../testing.md#g8eo--go) for test infrastructure, `testutil/` helpers, mock locations, contract tests, and how to run tests.

### Coverage Goals

- **config**: >80%
- **models**: 100%
- **services**: >70%
- **overall**: >65%

---

## Service Instantiation

g8eo startup is a two-phase process:

### Phase 1 — `NewG8eoService` (pre-auth)

Only `BootstrapService` is constructed. Before this, the CA is loaded using a local-first strategy — scanning well-known volume mount paths (`/ssl/ca.crt`, `/g8es/ca.crt`, `/g8es/ssl/ca.crt`, `/data/ssl/ca.crt`) before falling back to `FetchAndSetCA` for a remote HTTPS fetch. The mTLS HTTP client is then built using that CA via `certs.GetTLSConfig`. For testing, `BootstrapService` provides `SetHTTPClient` to allow mocking the authentication endpoint.

### Phase 2 — `G8eoService.Start()` (post-auth)

After `BootstrapService.RequestBootstrapConfig()` authenticates with g8ed and `ApplyBootstrapConfig()` applies the returned configuration (operator ID, session ID, heartbeat interval, per-operator mTLS cert), the remaining services are instantiated in order:

1. **`ExecutionService`** — shell command execution, concurrency-controlled via semaphore
2. **`FileEditService`** — file read/write/patch operations with automatic backups
3. **`LocalStoreService`** *(if local storage enabled)* — scrubbed vault SQLite
4. **`RawVaultService`** *(if local storage enabled)* — unscrubbed vault SQLite
5. **`AuditVaultService`** — LFAA audit log; always initialized
6. **`LedgerService`** *(if git available and not disabled)* — wraps `AuditVaultService`
7. **`HistoryHandler`** — wraps `AuditVaultService` + optional `LedgerService`
8. **`PubSubClient`** — persistent WebSocket connection to g8es pub/sub endpoint. Implements the `PubSubClient` interface to allow for test doubles (e.g. `MockG8esPubSubClient`). Injected via `SetPubSubClient` if a custom implementation is required for testing.
9. **`PubSubResultsService`** — publishes to `results:{operator_id}:{session}` channel
10. **`PubSubCommandService`** — subscribes to `cmd:{operator_id}:{session}` channel; orchestrates all sub-services. Created via `NewPubSubCommandService(CommandServiceConfig{...})`.
11. **`Sentinel`** — pre-execution threat detector + post-execution scrubber; injected via `CommandServiceConfig`.

All storage and security services are now injected into `PubSubCommandService` at construction time via `CommandServiceConfig`. The legacy setter pattern has been removed to ensure a fully initialized state.

---

## Project Structure

```
g8eo/
├── certs/                 # CA trust store (fetched at startup), TLS config
├── cmd/                   # CLI subcommands (stream)
├── config/                # Configuration management
├── constants/             # Exit codes, channels, events (mirrors shared JSON)
├── contracts/             # Cross-component contract tests
├── httpclient/            # mTLS HTTP/WebSocket client factory
├── models/                # Data structures
├── services/
│   ├── auth/              # Bootstrap, device auth, fingerprint
│   ├── execution/         # Command execution, file edit, fs_list
│   ├── listen/            # Listen mode (DB, HTTP, pub/sub broker)
│   ├── mcp/               # MCP protocol translator
│   ├── openclaw/          # OpenClaw Node Host
│   ├── pubsub/            # g8es WebSocket transport
│   ├── sentinel/          # Pre-execution threat detection + post-execution scrubbing
│   ├── sqliteutil/        # Shared SQLite helpers
│   ├── storage/           # Scrubbed vault, raw vault, audit vault, ledger, history handler
│   ├── system/            # Host info, git resolution, system metrics
│   └── vault/             # Encrypted DEK-based storage
├── testutil/              # Test helpers
└── main.go                # Entry point
```
