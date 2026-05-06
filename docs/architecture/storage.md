---
title: Storage
parent: Architecture
---

# Storage Architecture

This document explains how the g8e platform stores data across its components. It focuses on the **why** and **what** — the architectural decisions, data flows, and invariants — rather than implementation details.

## Core Principles

- **Operator-First Storage**: The Operator (`g8eo`) is the authoritative system of record for all operational data. Platform components (`g8ee`, `g8ed`) are stateless — they rely entirely on `g8es` for platform-side persistence.
- **Local-First Audit Architecture (LFAA)**: Every file mutation and command execution on a managed host is recorded locally in the Operator's Audit Vault and Ledger. The platform receives only Sentinel-scrubbed metadata; raw data never leaves the host unless explicitly retrieved by the customer.
- **Cache-Aside Pattern**: The `g8es` Document Store (SQLite) is the authoritative source of truth. The KV store provides fast read caching with TTL-based expiration. Writes go to the Document Store first, then invalidate the cache.
- **Zero-Trust Security**: Sentinel analyzes all actions (commands and file edits) *before* execution to block threats and scrubs sensitive data *after* execution before it leaves the host.

## Storage Tiers

1. **Platform Store (g8es)**: Shared state for users, sessions, operators, and cases. Centralized persistence for stateless components.
2. **Operator Local Storage**: Operational audit logs, file history, and raw command output living exclusively on the managed host.
3. **Ledger**: Git-backed version control of all file mutations providing cryptographic history and rollback.

---

## Storage Architecture at a Glance

### Platform Side (Self-Hosted Hub)
- **Component**: `g8es` (running `g8eo --listen`)
- **Persistence**: Single SQLite database at `/data/g8e.db`.
- **Stateless Clients**: `g8ed` and `g8ee` read/write via HTTPS/WSS.
- **Subsystems**: Document Store (JSON), KV Store (TTL), Blob Store (Binary), SSE Event Buffer.

### Operator Side (Managed Hosts)
- **Component**: `g8eo`
- **Scrubbed Vault** (`local_state.db`): Sentinel-processed output for AI context.
- **Raw Vault** (`raw_vault.db`): Unscrubbed command output for forensics.
- **Audit Vault** (`data/g8e.db`): Encrypted session history and LFAA event log.
- **Ledger** (`data/ledger`): Git repository tracking every file mutation.

**Key Invariant**: Raw operational data (passwords, secrets, PII) never leaves the host. The AI engine (`g8ee`) only ever sees Sentinel-scrubbed data.

---

## Storage Topology

```
┌─────────────────────────────────────────────────────────────────────┐
│                      g8e platform components                    │
│                                                                     │
│  ┌──────────────────────┐      ┌──────────────────────────────────┐ │
│  │         g8ee          │      │              g8ed                │ │
│  │  (stateless AI)       │      │  (stateless UI)                  │ │
│  │                      │      │                                  │ │
│  │  DBClient         │      │  G8esDocumentClient             │ │
│  │  (JSON documents)    │      │  (JSON documents)                │ │
│  │                      │      │                                  │ │
│  │  KVClient       │      │  KVClient                   │ │
│  │  (Cache + Pub/Sub)    │      │  (Cache + Session)               │ │
│  │                      │      │                                  │ │
│  │  BlobClient     │      │  g8esBlobClient                 │ │
│  │  (Attachments)        │      │  (Binary data)                   │ │
│  │                      │      │                                  │ │
│  └──────────┬───────────┘      └──────────┬──────────┘             │
│             │  HTTPS / WSS (mTLS)         │  HTTPS                   │
│             └──────────────────┬──────────┘                          │
│                                ▼                                     │
│  ┌─────────────────────────────────────────┐                         │
│  │                 g8es                   │                         │
│  │    g8eo binary in --listen mode          │                         │
│  │    SQLite (g8e.db) at               │                         │
│  │    /data/g8e.db                     │                         │
│  │                                         │                         │
│  │  Document Store  │  KV Store (TTL)  │   │                       │
│  │  ─────────────── │  ──────────────  │   │                       │
│  │  All platform    │  Sessions        │Pub│                       │
│  │  domain data     │  Nonces/Tokens   │Sub│                       │
│  │  (JSON docs)     │  Read Cache      │   │                       │
│  │  ─────────────── │  ──────────────  │   │                       │
│  │  Blob Store      │  SSE Event Buffer│   │                       │
│  │  (Binary data)   │  (Ring buffer)   │   │                       │
│  └─────────────────────────────────────────┘                         │
└─────────────────────────────────────────────────────────────────────┘
                              │
                    Gateway Protocol (mTLS)
                              │
┌─────────────────────────────────────────────────────────────────────┐
│                    OPERATOR (g8eo binary)                             │
│                                                                     │
│  ┌──────────────────┐  ┌──────────────────┐  ┌──────────────────┐  │
│  │   Scrubbed Vault │  │    Raw Vault     │  │   Audit Vault    │  │
│  │  local_state.db  │  │  raw_vault.db    │  │  data/g8e.db     │  │
│  │                  │  │                  │  │                  │  │
│  │  Sentinel-       │  │  Unscrubbed      │  │  LFAA Encrypted  │  │
│  │  scrubbed        │  │  Full forensic   │  │  Session History │  │
│  │  AI context      │  │  Record          │  │  & Audit Log     │  │
│  └──────────────────┘  └──────────────────┘  └──────────────────┘  │
│                                                                     │
│  ┌──────────────────────────────────────────────────────────────┐   │
│  │                      Ledger                                   │   │
│  │               data/ledger (Git)                          │   │
│  │   Cryptographic version control for every file mutation      │   │
│  └──────────────────────────────────────────────────────────────┘   │
└─────────────────────────────────────────────────────────────────────┘
```

---

## Component Storage Summary

| Component | Technology | Path/Volume | Role |
|---|---|---|---|
| **g8es (DB)** | SQLite | `g8es-data` -> `/data` | Central platform state. Wiped by `platform reset`. |
| **g8es (SSL)** | TLS Certs | `g8es-ssl` -> `/ssl` | CA, identity, and bootstrap secrets. **Survives reset**. |
| **g8ee** | None | - | Stateless; uses `DBClient`, `KVClient`, `BlobClient`. |
| **g8ed** | None | - | Stateless; uses `G8esDocumentClient`, `KVClient`. |
| **g8eo (Scrubbed)** | SQLite | `.g8e/local_state.db` | Sanity-checked AI context. |
| **g8eo (Raw)** | SQLite | `.g8e/raw_vault.db` | Customer-only forensic record. |
| **g8eo (Audit)** | SQLite (Enc) | `.g8e/data/g8e.db` | LFAA encrypted append-only event log. |
| **g8eo (Ledger)** | Git | `.g8e/data/ledger` | Cryptographic file history. |

---

## Platform Persistence (g8es)

`g8es` is the platform's central coordination point. It is an instance of `g8eo` running in `--listen` mode.

### Subsystems
- **Document Store**: Unified storage for JSON documents. Clients use a collection/ID pattern.
- **KV Store**: High-speed ephemeral data and read cache. Supports TTL and patterns.
- **Blob Store**: Binary storage for investigation attachments.
- **SSE Buffer**: A ring buffer for Server-Sent Events, ensuring clients can catch up after disconnects.
- **PubSub Broker**: Real-time message distribution for coordination.

### The SSL Volume Authority
The `g8es-ssl` volume is the platform's root of trust. It stores:
1. **CA Certificates**: Root and intermediate certificates for mTLS.
2. **Bootstrap Secrets**: `internal_auth_token`, `session_encryption_key`, and `auditor_hmac_key`.

On startup, `g8es` synchronizes these secrets into the database. If a conflict occurs, the SSL volume is authoritative.

### Cache-Aside Consistency
`g8ee` and `g8ed` implement a cache-aside pattern:
1. **Read**: Check KV cache first. On miss, fetch from Document Store and populate KV.
2. **Write**: Write to Document Store first (atomic), then delete the KV cache key.
3. **TTL**: KV entries have a default 10-minute TTL to ensure eventual consistency if invalidation fails.

---

## Local-First Audit Architecture (LFAA)

`g8eo` implements LFAA to ensure data sovereignty and tamper-evident auditing.

### Sentinel Defense & Scrubbing
Sentinel operates in two phases:
1. **Defense (Pre-Execution)**: Analyzes commands and file edits *before* they occur. Matches against 50+ threat patterns (e.g., reverse shells, system tampering).
2. **Scrubbing (Post-Execution)**: Removes 27+ types of sensitive data (API keys, PII, connection strings) from output before it is stored in the Scrubbed Vault or sent to the platform.

### The Ledger
The Ledger is a Git repository located at `.g8e/data/ledger`.
- **Atomic Commits**: Every file modification is committed with pre/post hashes.
- **Tamper Evidence**: Uses Git's cryptographic Merkle tree to guarantee history integrity.
- **Rollback**: Enables instantaneous restoration of any file to a previous state.

### Vault Encryption
The Audit Vault (`.g8e/data/g8e.db`) is encrypted using AES-256-GCM when an encryption vault is configured. Sensitive fields like `content_text`, `stdout`, and `stderr` are never stored in plain text.

### Querying the LFAA Audit Vault

The LFAA Audit Vault can be queried directly using SQLite commands for forensic analysis and audit review.

**Database Location:**
- Container: `/opt/g8e/.g8e/data/g8e.db`
- Host (after copy): Copy from container via `docker cp g8es:/opt/g8e/.g8e/data/g8e.db /tmp/g8e.db`

**Schema Tables:**
- `sessions` — Web session records (id, title, created_at, user_identity)
- `events` — Event logs (id, web_session_id, timestamp, type, content_text, command_raw, command_exit_code, command_stdout, command_stderr, execution_duration_ms, stored_locally, stdout_truncated, stderr_truncated, encrypted)
- `file_mutation_log` — File mutation records (id, event_id, filepath, operation, ledger_hash_before, ledger_hash_after, diff_stat)

**Common Queries:**

```sql
-- List all sessions
SELECT id, title, created_at, user_identity FROM sessions ORDER BY created_at DESC LIMIT 50;

-- Get session details
SELECT id, title, created_at, user_identity FROM sessions WHERE id = 'SESSION_ID';

-- Count events by type for a session
SELECT type, COUNT(*) as cnt FROM events WHERE web_session_id = 'SESSION_ID' GROUP BY type;

-- List events for a session
SELECT id, web_session_id, timestamp, type, content_text, command_raw,
command_exit_code, command_stdout, command_stderr, execution_duration_ms,
stored_locally, stdout_truncated, stderr_truncated, encrypted
FROM events WHERE web_session_id = 'SESSION_ID'
ORDER BY timestamp DESC LIMIT 50;

-- Filter events by type (USER_MSG, AI_MSG, CMD_EXEC, FILE_MUTATION)
SELECT * FROM events WHERE web_session_id = 'SESSION_ID' AND type = 'CMD_EXEC'
ORDER BY timestamp DESC LIMIT 50;

-- List file mutations
SELECT fml.id, fml.event_id, fml.filepath, fml.operation,
fml.ledger_hash_before, fml.ledger_hash_after, fml.diff_stat,
e.timestamp, e.web_session_id
FROM file_mutation_log fml
JOIN events e ON fml.event_id = e.id
ORDER BY e.timestamp DESC LIMIT 50;

-- Statistics
SELECT COUNT(*) FROM sessions;
SELECT COUNT(*) FROM events;
SELECT COUNT(*) FROM file_mutation_log;
SELECT type, COUNT(*) as cnt FROM events GROUP BY type ORDER BY cnt DESC;
SELECT COUNT(*) FROM events WHERE encrypted = 1;
SELECT MIN(timestamp), MAX(timestamp) FROM events;
```

**Python CLI Tool:**
For structured queries with formatting, use the management script:
```bash
./g8e data audit --db-path /opt/g8e/.g8e/data/g8e.db sessions
./g8e data audit --db-path /opt/g8e/.g8e/data/g8e.db events --session SESSION_ID
./g8e data audit --db-path /opt/g8e/.g8e/data/g8e.db stats
```

See `scripts/data/manage-lfaa.py` for full CLI reference.

---

## Canonical Collections

| Collection | Description |
|---|---|
| **Authentication & Sessions** |
| `users` | Account data and credential metadata. |
| `web_sessions` | UI sessions (encrypted). |
| `operator_sessions` | Operator CLI sessions (encrypted). |
| `cli_sessions` | Direct CLI sessions (encrypted). |
| `api_keys` | API key credentials for external integrations. |
| `passkey_challenges` | Passkey authentication challenges. |
| `account_locks` | Account lockout status and metadata. |
| **Audit & Security** |
| `login_audit` | Login attempt history and security events. |
| `auth_admin_audit` | Administrative authentication actions. |
| `console_audit` | Console command execution audit trail. |
| `bound_sessions` | Session binding records for security. |
| **Operators & Usage** |
| `operators` | Registration and heartbeat status for managed hosts. |
| `operator_usage` | Operator resource usage metrics. |
| **Organizations** |
| `organizations` | Organization accounts and memberships. |
| **Cases & Investigations** |
| `cases` | Support cases and forensic investigations. |
| `investigations` | Detailed forensic investigation records. |
| `tasks` | Task queue and execution status. |
| **AI & Context** |
| `memories` | AI-generated long-term context. |
| `tribunal_commands` | History of commands reviewed by the Tribunal. |
| `agent_activity_metadata` | Execution context and performance metrics. |
| **Configuration** |
| `settings` | Global and user-level overrides. |
| **Reputation System** |
| `reputation_state` | Reputation scores and state. |
| `reputation_commitments` | Reputation stake commitments. |
| `stake_resolutions` | Reputation stake resolution records. |

---

## Related Documentation

- [../components/g8eo.md](../components/g8eo.md) — g8eo component reference
- [../components/g8ee.md](../components/g8ee.md) — g8ee component reference
- [../components/g8ed.md](../components/g8ed.md) — g8ed component reference
- [../components/g8es.md](../components/g8es.md) — g8es component reference
- [../architecture/security.md](security.md) — Full security model: mTLS, Sentinel patterns, LFAA encryption, threat detection
- [../glossary.md](../glossary.md) — Platform terminology
