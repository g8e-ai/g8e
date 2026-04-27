---
title: Storage
parent: Architecture
---

# Storage Architecture

This document explains how the g8e platform stores data across its components. It focuses on the **why** and **what** — the architectural decisions, data flows, and invariants — rather than implementation details.

## Core Principles

**Operator-First Storage**: The Operator (g8eo) is the authoritative system of record for all operational data. Platform components (g8ee, g8ed) are stateless — they hold no databases and rely entirely on g8es for platform-side persistence.

**Local-First Audit Architecture (LFAA)**: Every file mutation and command execution on a managed host is recorded locally in the Operator's Audit Vault and Ledger. The platform receives only Sentinel-scrubbed metadata; raw data never leaves the host unless explicitly retrieved by the customer.

**Cache-Aside Pattern**: The document store (SQLite) is the authoritative source of truth. The KV store provides fast read caching with TTL-based expiration. Writes always go to the document store first, then invalidate or update the cache.

**Three-Tier Data Separation**:
1. **Platform Store (g8es)**: Shared state for users, sessions, operators, cases — data accessible to both g8ed and g8ee
2. **Operator Local Storage**: Operational audit logs, file history, and raw command output — lives exclusively on the managed host
3. **Ledger**: Git-backed version control of all file mutations — cryptographic history for rollback and forensics

## Storage Architecture at a Glance

The g8e platform uses a dual-storage architecture:

**Platform Side (Self-Hosted Hub)**:
- g8es (g8eo in `--listen` mode) provides a single SQLite database at `/data/g8e.db`
- Stores: users, sessions, operators, cases, investigations, settings
- g8ed and g8ee are stateless clients that read/write via HTTP API
- Cache-aside pattern: document store (authoritative) + KV store (fast cache with TTL)

**Operator Side (Managed Hosts)**:
- g8eo maintains four separate storage systems under `.g8e/`:
  - **Scrubbed Vault** (`local_state.db`): Sentinel-processed output for AI access
  - **Raw Vault** (`raw_vault.db`): Unscrubbed command output and diffs for customer forensics
  - **Audit Vault** (`data/g8e.db`): Encrypted session history and event log
  - **Ledger** (`data/ledger`): Git repository tracking all file mutations

**Key Invariant**: Raw operational data never leaves the managed host unless the customer explicitly retrieves it. The platform only receives Sentinel-scrubbed metadata.

---

### Storage Topology

```
┌─────────────────────────────────────────────────────────────────────┐
│                      g8e platform components                    │
│                                                                     │
│  ┌──────────────────────┐      ┌──────────────────────────────────┐ │
│  │         g8ee          │      │              g8ed                │ │
│  │  (stateless)         │      │  (stateless)                     │ │
│  │                      │      │                                  │ │
│  │  DBClient         │      │  G8esDocumentClient             │ │
│  │  (document store)    │      │  (document store)                │ │
│  │                      │      │                                  │ │
│  │  KVClient       │      │  KVClient                   │ │
│  │  (KV + pub/sub)      │      │  (KV store)                      │ │
│  │                      │      │                                  │ │
│  │                      │      │  G8esPubSubClient               │ │
│  │                      │      │  (pub/sub WebSocket)             │ │
│  │                      │      │                                  │ │
│  │                      │      │  g8esBlobClient                 │ │
│  │                      │      │  (binary attachments)            │ │
│  │                      │      │                                  │ │
│  └──────────┬───────────┘      └──────────┬──────────┘             │
│             │  HTTP / WebSocket           │  HTTP                    │
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
│  │  domain data     │  Device tokens   │Sub│                       │
│  │  (JSON docs)     │  Att. index/meta │   │                       │
│  │                  │  Blob metadata   │   │                       │
│  │  ─────────────── │  ──────────────  │   │                       │
│  │  Blob Store      │  SSE Event Buffer│   │                       │
│  │  (Binary data)   │  (Ring buffer)   │   │                       │
│  └─────────────────────────────────────────┘                         │
└─────────────────────────────────────────────────────────────────────┘
                              │
                    Gateway Protocol (WebSocket + mTLS)
                              │
┌─────────────────────────────────────────────────────────────────────┐
│                    OPERATOR (g8eo binary)                             │
│                                                                     │
│  ┌──────────────────┐  ┌──────────────────┐  ┌──────────────────┐  │
│  │   Scrubbed Vault │  │    Raw Vault     │  │   Audit Vault    │  │
│  │  .g8e/       │  │  .g8e/       │  │  .g8e/data/  │  │
│  │  local_state.db  │  │  raw_vault.db    │  │  g8e.db      │  │
│  │                  │  │                  │  │                  │  │
│  │  Sentinel-       │  │  Unscrubbed      │  │  Operator session│  │
│  │  processed       │  │  command output  │  │  history, cmds,  │  │
│  │  output (AI-     │  │  & file diffs    │  │  file mutations  │  │
│  │  accessible)     │  │  (customer-only) │  │  (encrypted)     │  │
│  └──────────────────┘  └──────────────────┘  └──────────────────┘  │
│                                                                     │
│  ┌──────────────────────────────────────────────────────────────┐   │
│  │                      Ledger                                   │   │
│  │            {workdir}/.g8e/data/ledger (Git)              │   │
│  │   Git-backed version control for all operator-modified files │   │
│  │   Every file write is committed to git history               │   │
│  └──────────────────────────────────────────────────────────────┘   │
└─────────────────────────────────────────────────────────────────────┘
```

### Component Storage Summary

| Component | Storage Technology | Volume / Path | Role |
|---|---|---|---|
| **g8es (DB)** | SQLite (via g8eo `--listen`) | `g8es-data` → `/data` | Sole platform persistence layer: document store, KV, blob store, SSE buffer, pub/sub broker. Wiped by `platform reset`. |
| **g8es (SSL)** | TLS certs (auto-generated) | `g8es-ssl` → `/ssl` | Platform CA and server certificates. **Never wiped** — survives `reset`, `wipe`, and `rebuild`. |
| **g8ee** | None (g8es client) | — | Stateless; reads/writes all data via g8ed HTTP API |
| **g8ed** | None (g8es client) | — | Stateless; document/KV data via g8es |
| **g8eo (Scrubbed Vault)** | SQLite | `{workdir}/.g8e/local_state.db` | Sentinel-processed output for AI access |
| **g8eo (Raw Vault)** | SQLite | `{workdir}/.g8e/raw_vault.db` | Unscrubbed output for customer forensics |
| **g8eo (Audit Vault)** | SQLite (encrypted) | `{workdir}/.g8e/data/g8e.db` | LFAA: session history, command logs, file mutations |
| **g8eo (Ledger)** | Git | `{workdir}/.g8e/data/ledger` | LFAA: cryptographic file version history |

---

## Platform Store (g8es)

g8es is the platform's shared persistence layer — the g8eo binary running in `--listen` mode with a single SQLite database at `/data/g8e.db`. It serves both g8ed (dashboard) and g8ee (AI engine) via HTTP.

### Why g8es Exists

**Statelessness**: g8ed and g8ee are designed to be stateless. They can be restarted, scaled, or redeployed without data loss because all persistent state lives in g8es.

**Single Source of Truth**: By centralizing platform data in one database, we avoid distributed consistency problems. Both components see the same view of users, sessions, operators, and cases.

**Operational Simplicity**: Backup, migration, and disaster recovery are simplified when there's only one database to manage for platform state.

### Security Architecture

The SSL volume (`g8es-ssl`) is mounted separately from the data volume (`g8es-data`). This separation is critical:

- **Bootstrap Secrets**: `internal_auth_token`, `session_encryption_key`, and `auditor_hmac_key` are generated on first boot and stored in the SSL volume
- **Volume Authority**: The SSL volume is the authoritative source. If the database is wiped but the SSL volume survives, the platform retains its identity
- **Survivability**: Platform identity survives `platform reset` operations as long as the SSL volume is preserved

On each boot, g8es synchronizes secrets from the SSL volume into the database and KV cache. If the volume and database diverge, the volume value wins.

### Storage Subsystems

g8es provides four storage subsystems:

**Document Store**: JSON documents organized by collection (users, sessions, operators, cases, etc.). Supports collection/id CRUD with query capabilities.

**KV Store**: Key/value pairs with optional TTL expiration. Used for ephemeral state (sessions, device tokens, nonces) and as a read cache for document store data.

**Blob Store**: Binary data keyed by namespace + ID with optional TTL. Used for file attachments and binary payloads.

**SSE Event Buffer**: Per-session ring buffer for Server-Sent Events reconnection replay. Enables clients to catch up on missed events after reconnecting.

**PubSub Broker**: In-memory WebSocket pub/sub for real-time messaging. No persistence — messages are only delivered to connected clients.

### Data Model

The document store uses a simple collection/document model:

- **Documents**: JSON objects identified by `{collection}/{id}` pairs
- **Collections**: Logical groupings (users, sessions, operators, cases, investigations, etc.)
- **Metadata**: Each document tracks `created_at` and `updated_at` timestamps separately from the JSON payload
- **Queries**: Filter documents by JSON field values using standard comparison operators

### Cache-Aside Pattern

The KV store provides a read cache with TTL-based expiration:

- **Read Path**: Check KV cache first (~1-5ms). On miss, read from document store and warm the cache.
- **Write Path**: Write to document store first (authoritative), then invalidate or update the cache key.
- **TTL Expiration**: Keys expire automatically; background cleanup purges expired entries every 30 seconds.
- **Consistency Invariant**: The document store is always the source of truth. Cache invalidation on writes prevents stale data.

### Configuration Management

Platform configuration follows a precedence chain (highest priority wins):

1. **User Settings (DB)**: Individual user overrides
2. **Platform Settings (DB)**: Global platform configuration
3. **Environment Variables**: Runtime `G8E_*` variables
4. **Schema Defaults**: Hardcoded safe defaults

The `platform_settings` document in the `settings` collection stores global configuration including LLM settings, URLs, and mirrored bootstrap secrets. Bootstrap secrets are marked as `writeOnce` in the g8ed settings model to prevent UI updates from silently diverging from the volume-authoritative values.

---

## Key Collections

The document store organizes data into collections. Key collections include:

**Authentication & Sessions**:
- `users`: User accounts, credentials, roles
- `web_sessions`, `operator_sessions`, `cli_sessions`: Session state with encrypted sensitive fields
- `api_keys`: API key registry with permissions
- `login_audit`, `auth_admin_audit`: Authentication event history
- `account_locks`: Account lockout state
- `passkey_challenges`: WebAuthn challenges (single-use nonces)

**Operators & Cases**:
- `operators`: Operator registration, binding, lifecycle status
- `operator_usage`: Usage metrics
- `cases`, `investigations`, `tasks`: Support case management

**AI & Reputation**:
- `memories`: AI memory entries
- `tribunal_commands`: Tribunal command history
- `agent_activity_metadata`: Agent execution context and performance
- `reputation_state`, `reputation_commitments`, `stake_resolutions`: Reputation system

**Configuration & Audit**:
- `settings`: Platform-wide configuration and user preferences
- `bound_sessions`: Operator-web session bindings
- `console_audit`: Admin action audit trail
- `organizations`: Organization records

**Session Security**: Session documents store sensitive fields (API keys, operator IDs) encrypted with AES-256-GCM using the `session_encryption_key`. Sessions have both absolute (24h) and idle (8h) expiration timers.

---

## Operator Storage (g8eo)

The Operator (g8eo) maintains local storage on managed hosts, separate from the platform store. This implements the Local-First Audit Architecture (LFAA).

### Why Local Storage Exists

**Data Sovereignty**: Raw operational data (command output, file diffs) never leaves the managed host unless the customer explicitly retrieves it. The platform only receives Sentinel-scrubbed metadata.

**Forensics & Compliance**: The Raw Vault preserves unscrubbed data for customer investigation and compliance requirements. The Audit Vault provides an encrypted, tamper-evident session history.

**Offline Capability**: Operators can continue functioning even if disconnected from the platform. Local audit logs ensure no data is lost during outages.

### Storage Systems

The Operator maintains four separate storage systems under `{workdir}/.g8e/`:

**Scrubbed Vault** (`local_state.db`): SQLite database storing Sentinel-processed command output and file diffs. This is the data the AI can access for context and decision-making. Data is compressed, hashed, and pruned based on retention policies.

**Raw Vault** (`raw_vault.db`): SQLite database storing unscrubbed command output and file diffs. This is the customer's authoritative data store for forensics. The AI never reads from this vault. It has a larger retention window (30 days vs 90 days for scrubbed vault).

**Audit Vault** (`data/g8e.db`): Encrypted SQLite database storing the complete LFAA audit trail — session history, command logs, file mutations, and chat messages. Sensitive fields (content, stdout, stderr) are encrypted at rest when an encryption vault is configured.

**Ledger** (`data/ledger`): Git repository tracking all file mutations with cryptographic history. Every file write, delete, and create is committed with pre/post hashes and diffs. Enables rollback to any previous state and provides tamper-evident file history.

### Data Flow

When the Operator executes a command or modifies a file:

1. **Execution**: Command runs, output is captured
2. **Dual Write**: Output is written to both Scrubbed Vault (after Sentinel processing) and Raw Vault (unscrubbed)
3. **File Mutation**: Before/after snapshots are captured, diff is calculated
4. **Ledger Commit**: File mutation is committed to Git with cryptographic hashes
5. **Audit Log**: Event is recorded in Audit Vault with encrypted sensitive fields
6. **Platform Sync**: Sentinel-scrubbed metadata is sent to platform via WebSocket

---

## Sentinel Mode

g8eo supports two vault modes that control data scrubbing and AI access:

**Raw Mode (Default)**: The AI reads from the Raw Vault (unscrubbed data). Both Scrubbed and Raw Vaults are written. Used when full fidelity is required for debugging or forensics.

**Scrubbed Mode**: The AI reads from the Scrubbed Vault (Sentinel-processed data). Only the Scrubbed Vault is written; Raw Vault writes are skipped. Used for production investigations where PII and secrets must be redacted.

Sentinel applies 27+ redaction patterns to remove PII, secrets, and credentials before data is written to the Scrubbed Vault. This ensures the AI never sees sensitive information while still having sufficient context for decision-making.

---

## Related Documentation

- [../components/g8eo.md](../components/g8eo.md) — g8eo component reference
- [../components/g8ee.md](../components/g8ee.md) — g8ee component reference
- [../components/g8ed.md](../components/g8ed.md) — g8ed component reference
- [../components/g8es.md](../components/g8es.md) — g8es component reference
- [../architecture/security.md](security.md) — Full security model: mTLS, Sentinel patterns, LFAA encryption, threat detection
- [../glossary.md](../glossary.md) — Platform terminology
