---
title: Storage
parent: Architecture
---

# Storage Architecture

Last Updated: 2026-05-13
Version: v0.2.5

This document explains the unified storage architecture for the g8e platform. It focuses on the **why** and **what** — the architectural decisions, data flows, and invariants — rather than low-level implementation.

## Core Principles

- **Operator-Hub Persistence**: The Operator (`g8eo`) running in `--listen` mode is the authoritative system of record and the primary **Substrate**. It provides the Coordination Store API for all clients. Optional application-layer adapters like the Dashboard (`g8ed`) and Engine (`g8ee`) are completely stateless and rely on the Operator for persistence.
- **Local-First Audit Architecture (LFAA)**: Every file mutation and command execution on a managed host is recorded locally in the Operator's Audit Vault and Ledger. The platform substrate receives only Sentinel-scrubbed metadata; raw data never leaves the host unless explicitly retrieved.
- **Unified Coordination Store**: A single SQLite database on the platform hub (provided by `g8eo --listen`) provides Document, KV, SSE, and Blob storage services to stateless clients.
- **Data Sovereignty**: Raw operational data (passwords, secrets, PII) is quarantined on the managed host. The platform only ever receives Sentinel-scrubbed summaries and metadata in the Scrubbed Vault.

## Storage Tiers

1.  **Coordination Store (Platform Hub)**: Shared state for users, sessions, operators, cases, and configuration. Centralized persistence for stateless components.
2.  **LFAA Vaults (Managed Hosts)**:
    *   **Audit Vault**: Cryptographically signed, append-only record of all session activity (encrypted at rest).
    *   **Scrubbed Vault**: Sentinel-processed output for AI context and platform-side reporting.
    *   **Raw Vault**: Unscrubbed command output for deep forensic analysis (customer-access only).
3.  **The Ledger (Managed Hosts)**: Git-backed version control of all file mutations providing cryptographic history and instant rollback.

---

## Storage Architecture at a Glance

### Platform Hub (g8eo --listen)
- **Component**: `g8eo` (the "Platform Hub" and "Substrate")
- **Persistence**: Single SQLite database at `.g8e/data/g8e.db` (The "Coordination Store").
- **Stateless Clients**: Bundled apps (`g8ed`, `g8ee`) and BYO clients read/write via public HTTPS/WSS APIs.
- **Subsystems**:
    - **Document Store**: JSON document CRUD using a Collection/ID pattern with `json_extract` query support.
    - **KV Store**: High-speed ephemeral data with TTL support and read cache.
    - **Blob Store**: Binary storage for investigation attachments and large objects.
    - **SSE Event Buffer**: Ring buffer for Server-Sent Events reconnection replay.

### Managed Hosts (g8eo)
- **Component**: `g8eo` (the "Operator")
- **Scrubbed Vault** (`.g8e/local_state.db`): Sentinel-processed output for AI context.
- **Raw Vault** (`.g8e/raw_vault.db`): Unscrubbed command output for forensic investigations.
- **Audit Vault** (`.g8e/data/g8e.db`): Encrypted append-only event log and session history.
- **Ledger** (`.g8e/data/ledger`): Git repository tracking every file mutation with cryptographic integrity.

---

## Storage Topology

```
┌─────────────────────────────────────────────────────────────────────┐
│                      g8e platform hub (Self-Hosted)                 │
│                                                                     │
│  ┌──────────────────────┐      ┌──────────────────────────────────┐ │
│  │         g8ee          │      │              g8ed                │ │
│  │  (stateless Engine)   │      │  (stateless Dashboard)           │ │
│  │                      │      │                                  │ │
│  │  DBClient            │      │  OperatorDocumentClient          │ │
│  │  (JSON documents)    │      │  (JSON documents)                │ │
│  │                      │      │                                  │ │
│  │  KVCacheClient       │      │  KVCacheClient                   │ │
│  │  (Cache + Pub/Sub)    │      │  (Cache + Session)               │ │
│  │                      │      │                                  │ │
│  │  BlobClient          │      │  OperatorBlobClient              │ │
│  │  (Attachments)        │      │  (Binary data)                   │ │
│  │                      │      │                                  │ │
│  └──────────┬───────────┘      └──────────┬──────────┘             │
│             │  HTTPS / WSS (mTLS)         │  HTTPS                   │
│             └──────────────────┬──────────┘                          │
│                                ▼                                     │
│  ┌─────────────────────────────────────────────────────────────────┐ │
│  │                         g8eo --listen                           │ │
│  │                (Unified Coordination Store)                     │ │
│  │                                                                 │ │
│  │  Document Store  │  KV Store (TTL)  │  SSE Buffer  │  Blob Store│ │
│  │  ─────────────── │  ──────────────  │  ──────────  │  ──────────│ │
│  │  All platform    │  Sessions        │  SSE Replay  │  Binaries  │ │
│  │  domain data     │  Nonces/Tokens   │  Ring Buffer │  Certs     │ │
│  │  (JSON docs)     │  Read Cache      │              │            │ │
│  │  ─────────────── │  ──────────────  │  ──────────  │  ──────────│ │
│  │                   SQLite (.g8e/data/g8e.db)                     │ │
│  └─────────────────────────────────────────────────────────────────┘ │
└─────────────────────────────────────────────────────────────────────┘
                              │
                    Gateway Protocol (mTLS)
                              │
┌─────────────────────────────────────────────────────────────────────┐
│                    OPERATOR (Managed Host)                           │
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
| **Platform Hub (Substrate)** | SQLite | `.g8e/data/g8e.db` | Central platform state (Coordination Store). |
| **Platform Hub (PKI)** | TLS Certs | `.g8e/pki` | CA hierarchy, intermediate CAs, trust bundles. **Root of Trust**. |
| **Platform Hub (Secrets)** | Bootstrap Secrets | `.g8e/secrets` | Session encryption key with tamper-evidence manifest. |
| **Optional Adapters** | None | - | Stateless clients; use `DBClient` and `KVCacheClient`. |
| **Operator (Scrubbed)** | SQLite | `.g8e/local_state.db` | Sentinel-scrubbed AI context. |
| **Operator (Raw)** | SQLite | `.g8e/raw_vault.db` | Customer-only unscrubbed forensic record. |
| **Operator (Audit)** | SQLite (Enc) | `.g8e/data/g8e.db` | LFAA encrypted append-only event log. |
| **Operator (Ledger)** | Git | `.g8e/data/ledger` | Cryptographic file history and rollback. |

---

## Coordination Store (g8eo --listen)

The Coordination Store is the platform's central coordination point. It is implemented in the `g8eo` binary when running in `--listen` mode.

### Subsystems
- **Document Store**: Unified storage for JSON documents. Clients use a collection/ID pattern. All documents include `created_at` and `updated_at` timestamps managed by the store.
- **KV Store**: High-speed ephemeral data and read cache. Supports TTL, `GLOB` pattern matching, and cursor-based scanning (`KVScan`).
- **Blob Store**: Binary storage for investigation attachments, large objects, and certificate material.
- **SSE Buffer**: A ring buffer for Server-Sent Events, ensuring clients can catch up after disconnects.
- **PubSub Broker**: Real-time message distribution for coordination between the Engine and Dashboard.

### The PKI and Secrets Directories (Root of Trust)
The `.g8e/pki` and `.g8e/secrets` directories form the platform's root of trust.

**PKI Directory (`.g8e/pki/`)** stores:
1. **CA Hierarchy**: Root CA, intermediate CAs (hub, operator, bootstrap), and trust bundles.
2. **Issued Certificates**: Server and workload certificates signed by intermediate CAs.

**Secrets Directory (`.g8e/secrets/`)** stores:
1. **Bootstrap Secrets**: `session_encryption_key`, `warden_signing_key`, and `warden_key_id`.
2. **Tamper-Evidence Manifest**: `bootstrap_digest.json` with SHA-256 digests of each secret.

On startup, `g8eo` SecretManager validates that secrets match the bootstrap digest manifest. If a conflict occurs, startup fails hard with actionable error messages.

### Cache-Aside Consistency
`g8ee` and `g8ed` implement a cache-aside pattern for performance:
1. **Read**: Check KV cache first. On miss, fetch from Document Store and populate KV.
2. **Write**: Write to Document Store first (authoritative), then delete/invalidate the KV cache key.
3. **TTL**: KV entries have collection-specific TTLs to ensure eventually consistency.

---

## Local-First Audit Architecture (LFAA)

`g8eo` implements LFAA to ensure data sovereignty and tamper-evident auditing on managed hosts.

### Sentinel Defense & Scrubbing
Sentinel protects data privacy in two phases:
1. **Defense (Pre-Execution)**: Analyzes commands and file edits *before* they occur, blocking threat patterns.
2. **Scrubbing (Post-Execution)**: Removes sensitive data (API keys, PII) from output before it is stored in the Scrubbed Vault or sent to the platform.

### The Ledger (Git)
The Ledger is a Git repository located at `.g8e/data/ledger`.
- **Atomic Commits**: Every file modification is committed with pre/post hashes.
- **Tamper Evidence**: Uses Git's Merkle tree to guarantee history integrity.
- **Rollback**: Enables restoration of any file to any previous state in history.

### Vault Strategy
- **Audit Vault** (`.g8e/data/g8e.db`): Encrypted using AES-256-GCM (if configured). Stores the definitive session history and event log.
- **Scrubbed Vault** (`.g8e/local_state.db`): Stores the "AI-ready" view of the host, including execution logs and file diffs that have been processed by Sentinel.
- **Raw Vault** (`.g8e/raw_vault.db`): Stores unscrubbed forensic records, accessible only to authorized customer auditors.

---

## Canonical Collections

The following collections are defined in `shared/constants/collections.json` and are used across the platform for Document Store organization:

| Collection | Description |
|---|---|
| **Authentication & Sessions** |
| `users` | Account data and credential metadata. |
| `web_sessions` | UI sessions (encrypted). |
| `operator_sessions` | Operator CLI sessions (encrypted). |
| `cli_sessions` | CLI tool sessions (encrypted). |
| `bound_sessions` | Sessions cryptographically bound to a specific device/origin. |
| `api_keys` | API key credentials for external integrations. |
| `passkey_challenges` | Passkey authentication challenges. |
| **Organizations & Tenants** |
| `organizations` | Tenant isolation and policy grouping. |
| **Audit & Security** |
| `login_audit` | Login attempt history and security events. |
| `auth_admin_audit` | Administrative authentication actions. |
| `account_locks` | Temporary account locks due to security policy violations. |
| `console_audit` | Console command execution audit trail. |
| `revoked_certificates` | List of revoked mTLS/SSL certificates. |
| **Operators & Usage** |
| `operators` | Registration and heartbeat status for managed hosts. |
| `operator_usage` | Operator resource usage metrics. |
| **Cases & Investigations** |
| `cases` | Support cases and forensic investigations. |
| `investigations` | Detailed forensic investigation records including history trails and chat. |
| `tasks` | Background tasks and long-running operations. |
| **Governance & Reputation** |
| `tribunal_commands` | History of commands reviewed by the Tribunal. |
| `reputation_state` | Current reputation scores and metadata for agents and operators. |
| `reputation_commitments` | Merkle commitments for reputation state transitions. |
| `stake_resolutions` | Outcomes of reputation staking and slashed stakes. |
| **AI & Context** |
| `memories` | AI-generated long-term context. |
| `agent_activity_metadata` | Execution context and performance metrics. |
| **Configuration** |
| `settings` | Global and user-level overrides. |

---

## Related Documentation

- [../components/g8eo.md](../components/g8eo.md) — g8eo component reference
- [../components/g8ee.md](../components/g8ee.md) — g8ee component reference
- [../components/g8ed.md](../components/g8ed.md) — g8ed component reference
- [security.md](security.md) — Full security model: mTLS, Sentinel patterns, LFAA encryption, threat detection
- [protocol.md](protocol.md) — Governance Envelope and communication protocol
