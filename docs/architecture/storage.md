---
title: Storage
parent: Architecture
---

# Storage Architecture

Last Updated: 2026-05-07
Version: v0.2.0

This document explains how the g8e platform stores data across its components. It focuses on the **why** and **what** — the architectural decisions, data flows, and invariants — rather than implementation details.

## Core Principles

- **Operator-First Storage**: The Operator (`g8eo`) is the authoritative system of record for all operational data. Platform components (`g8ee`, `g8ed`) are stateless — they rely entirely on `operator` for platform-side persistence.
- **Local-First Audit Architecture (LFAA)**: Every file mutation and command execution on a managed host is recorded locally in the Operator's Audit Vault and Ledger. The platform receives only Sentinel-scrubbed metadata; raw data never leaves the host unless explicitly retrieved by the customer.
- **Cache-Aside Pattern**: The `operator` Document Store (SQLite) is the authoritative source of truth for platform domain data. Stateless components (`g8ee`, `g8ed`) use a KV store for fast read caching with TTL-based expiration. Writes go to the Document Store first, which then triggers cache invalidation.
- **Data Sovereignty**: Raw operational data (passwords, secrets, PII) never leaves the host. The platform only ever receives Sentinel-scrubbed summaries and metadata.

## Storage Tiers

1. **Platform Store (operator)**: Shared state for users, sessions, operators, cases, and configuration. Centralized persistence for stateless components.
2. **Operator Local Storage**:
    - **Audit Vault**: Cryptographically signed, append-only record of all session activity.
    - **Ledger**: Git-backed version control of all file mutations providing cryptographic history and rollback.
    - **Vaults**: Tiered SQLite storage for scrubbed (AI-ready) and raw (forensic) execution records.

---

## Storage Architecture at a Glance

### Platform Side (Self-Hosted Hub)
- **Component**: `operator` (running `g8eo --listen`)
- **Persistence**: Single SQLite database at `/data/g8e.db` (The "Coordination Store").
- **Stateless Clients**: `g8ed` and `g8ee` read/write via HTTPS/WSS using `DBClient` (Python) or `OperatorDocumentClient` (JS).
- **Subsystems**:
    - **Document Store**: JSON document CRUD (Collection/ID pattern).
    - **KV Store**: High-speed ephemeral data, TTL support, and read cache.
    - **Blob Store**: Binary storage for investigation attachments.
    - **SSE Event Buffer**: Ring buffer for Server-Sent Events reconnection replay.

### Operator Side (Managed Hosts)
- **Component**: `g8eo`
- **Scrubbed Vault** (`local_state.db`): Sentinel-processed output for AI context.
- **Raw Vault** (`raw_vault.db`): Unscrubbed command output for customer forensics.
- **Audit Vault** (`data/g8e.db`): Encrypted session history and LFAA event log.
- **Ledger** (`data/ledger`): Git repository tracking every file mutation with cryptographic integrity.

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
│  │  DBClient            │      │  OperatorDocumentClient              │ │
│  │  (JSON documents)    │      │  (JSON documents)                │ │
│  │                      │      │                                  │ │
│  │  KVService           │      │  OperatorKvCacheClient               │ │
│  │  (Cache + Pub/Sub)    │      │  (Cache + Session)               │ │
│  │                      │      │                                  │ │
│  │  BlobClient          │      │  OperatorHttpClient                  │ │
│  │  (Attachments)        │      │  (Binary data)                   │ │
│  │                      │      │                                  │ │
│  └──────────┬───────────┘      └──────────┬──────────┘             │
│             │  HTTPS / WSS (mTLS)         │  HTTPS                   │
│             └──────────────────┬──────────┘                          │
│                                ▼                                     │
│  ┌─────────────────────────────────────────┐                         │
│  │                 operator                   │                         │
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
│  │               data/ledger (Git)                               │   │
│  │   Cryptographic version control for every file mutation      │   │
│  └──────────────────────────────────────────────────────────────┘   │
└─────────────────────────────────────────────────────────────────────┘
```

---

## Component Storage Summary

| Component | Technology | Path/Volume | Role |
|---|---|---|---|
| **operator (DB)** | SQLite | `operator-data` -> `/data/g8e.db` | Central platform state (Coordination Store). |
| **operator (SSL)** | TLS Certs | `operator-ssl` -> `/ssl` | CA, identity, and bootstrap secrets. **Survives reset**. |
| **g8ee** | None | - | Stateless; uses `DBClient`, `KVService`, `BlobClient`. |
| **g8ed** | None | - | Stateless; uses `OperatorDocumentClient`, `OperatorKvCacheClient`. |
| **g8eo (Scrubbed)** | SQLite | `.g8e/local_state.db` | Sentinel-scrubbed AI context. |
| **g8eo (Raw)** | SQLite | `.g8e/raw_vault.db` | Customer-only unscrubbed forensic record. |
| **g8eo (Audit)** | SQLite (Enc) | `.g8e/data/g8e.db` | LFAA encrypted append-only event log. |
| **g8eo (Ledger)** | Git | `.g8e/data/ledger` | Cryptographic file history and rollback. |

---

## Platform Persistence (operator)

`operator` is the platform's central coordination point. It is an instance of `g8eo` running in `--listen` mode.

### Subsystems
- **Document Store**: Unified storage for JSON documents. Clients use a collection/ID pattern.
- **KV Store**: High-speed ephemeral data and read cache. Supports TTL, patterns, and complex types (emulated).
- **Blob Store**: Binary storage for investigation attachments and large objects.
- **SSE Buffer**: A ring buffer for Server-Sent Events, ensuring clients can catch up after disconnects.
- **PubSub Broker**: Real-time message distribution for coordination.

### The SSL Volume Authority
The `operator-ssl` volume is the platform's root of trust. It stores:
1. **CA Certificates**: Root and intermediate certificates for mTLS.
2. **Bootstrap Secrets**: `internal_auth_token`, `session_encryption_key`, and `auditor_hmac_key`.

On startup, `operator` synchronizes these secrets into its database. If a conflict occurs, the SSL volume is authoritative.

### Cache-Aside Consistency
`g8ee` and `g8ed` implement a cache-aside pattern for performance:
1. **Read**: Check KV cache first. On miss, fetch from Document Store and populate KV.
2. **Write**: Write to Document Store first (authoritative), then delete/invalidate the KV cache key.
3. **TTL**: KV entries have collection-specific TTLs (e.g., settings=default, api_keys=long) to ensure eventual consistency.

---

## Local-First Audit Architecture (LFAA)

`g8eo` implements LFAA to ensure data sovereignty and tamper-evident auditing on managed hosts.

### Sentinel Defense & Scrubbing
Sentinel operates in two phases to protect both the host and data privacy:
1. **Defense (Pre-Execution)**: Analyzes commands and file edits *before* they occur, blocking threat patterns.
2. **Scrubbing (Post-Execution)**: Removes sensitive data (API keys, PII) from output before it is stored in the Scrubbed Vault or sent to the platform.

### The Ledger
The Ledger is a Git repository located at `.g8e/data/ledger`.
- **Atomic Commits**: Every file modification is committed with pre/post hashes.
- **Tamper Evidence**: Uses Git's cryptographic Merkle tree to guarantee history integrity.
- **Rollback**: Enables instantaneous restoration of any file to a previous state.

### Vault Encryption
The Audit Vault (`.g8e/data/g8e.db`) is encrypted using AES-256-GCM when an encryption vault is configured. Sensitive event fields are never stored in plain text.

### Querying the LFAA Audit Vault
The LFAA Audit Vault can be queried directly via SQLite for forensic analysis.

**Database Location:** `.g8e/data/g8e.db`

**Schema Tables:**
- `sessions` — Web session records (id, title, created_at, user_identity)
- `events` — Event logs (id, operator_session_id, timestamp, type, content_text, command_raw, command_exit_code, command_stdout, command_stderr, execution_duration_ms, stored_locally, encrypted)
- `file_mutation_log` — File mutation records (id, event_id, filepath, operation, ledger_hash_before, ledger_hash_after, diff_stat)

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
| `investigations` | Detailed forensic investigation records including history trails and chat. |
| `tasks` | Task queue and execution status. |
| **AI & Context** |
| `memories` | AI-generated long-term context. |
| `tribunal_commands` | History of commands reviewed by the Tribunal. |
| `agent_activity_metadata` | Execution context and performance metrics. |
| **Configuration** |
| `settings` | Global and user-level overrides (PLATFORM_SETTINGS_DOC, USER_SETTINGS_DOC_PREFIX). |
| **Reputation System** |
| `reputation_state` | Reputation scores and state. |
| `reputation_commitments` | Reputation stake commitments. |
| `stake_resolutions` | Reputation stake resolution records. |
| **Security & Compliance** |
| `revoked_certificates` | Serial numbers and reasons for certificate revocations. |

---

## Related Documentation

- [../components/g8eo.md](../components/g8eo.md) — g8eo component reference
- [../components/g8ee.md](../components/g8ee.md) — g8ee component reference
- [../components/g8ed.md](../components/g8ed.md) — g8ed component reference
- [../components/operator.md](../components/operator.md) — operator component reference
- [../architecture/security.md](security.md) — Full security model: mTLS, Sentinel patterns, LFAA encryption, threat detection
- [../glossary.md](../glossary.md) — Platform terminology
