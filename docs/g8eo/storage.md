---
title: Storage
parent: Architecture
---

## Storage Architecture

Last Updated: 2026-05-16
Version: v0.4.0

This document explains the unified storage architecture for the g8e platform. It focuses on the **why** and **what** — the architectural decisions, data flows, and invariants — rather than low-level implementation.

## Core Principles

- **Operator-Hub Persistence**: The Operator (`g8eo`) running in `--listen` mode is the authoritative system of record and the primary **Substrate**. It provides the Unified Coordination Store API for all clients. Application-layer adapters (like Dashboards or specialized Agents) are completely stateless and rely on the Hub for persistence.
- **Local-First Audit Architecture (LFAA)**: Every file mutation and command execution on a managed host is recorded locally in the Operator's Audit Vault and Ledger. The platform substrate receives only Sentinel-scrubbed metadata; raw data never leaves the host unless explicitly requested by an authorized auditor.
- **Unified Coordination Store**: A single SQLite database on the platform hub (`.g8e/data/g8e.db`) provides Document, KV, SSE, and Blob storage services to stateless clients.
- **State Merkle Root Invariant**: All Hub data (Documents, KV, Blobs) is anchored by a platform-wide Merkle state root. This root is used to verify the state-binding of governance transactions before execution.

## Storage Tiers

1.  **Coordination Store (Platform Hub)**: Shared state for users, sessions, operators, cases, and configuration. Centralized persistence for stateless components.
2.  **LFAA Vaults (Managed Hosts)**:
    *   **Audit Vault**: Cryptographically signed, append-only record of all session activity (encrypted at rest).
    *   **Scrubbed Vault**: Sentinel-processed output for AI context and platform-side reporting.
    *   **Raw Vault**: Unscrubbed command output for deep forensic analysis (customer-access only).
3.  **The Ledger (Managed Hosts)**: Multi-Ledger Architecture — a fleet of per-session isolated git repositories providing cryptographic history and instant rollback. Each operator session owns its own git repo under `.g8e/data/ledger/sessions/<operator_session_id>/`.

---

## Storage Architecture at a Glance

### Platform Hub (g8eo --listen)
- **Component**: `g8eo` (the "Platform Hub" or "Substrate")
- **Persistence**: Unified SQLite database at `.g8e/data/g8e.db` (The "Coordination Store").
- **Stateless Clients**: BYO Frontends and Agents read/write via HTTPS/WSS (mTLS) APIs.
- **Subsystems**:
    - **Document Store**: JSON document CRUD using a Collection/ID pattern with `json_extract` query support.
    - **KV Store**: High-speed ephemeral data with TTL support and `GLOB` pattern scanning.
    - **Blob Store**: Binary storage for investigation attachments, certificates, and large objects.
    - **SSE Event Buffer**: Ring buffer for Server-Sent Events reconnection replay.
    - **State Root**: Deterministic Merkle state root across all authoritative hub data.
    - **Nonce Manager**: Replay protection for governance transactions.

### Managed Hosts (g8eo)
- **Component**: `g8eo` (the "Operator")
- **Scrubbed Vault** (`.g8e/local_state.db`): Sentinel-processed command output (`execution_log`) and file diffs (`file_diff_log`) for AI context.
- **Raw Vault** (`.g8e/raw_vault.db`): Unscrubbed command output (`raw_execution_log`) and file diffs (`raw_file_diff_log`) for forensic investigations. Accessible only to humans.
- **Audit Vault** (`.g8e/data/g8e.db`): Encrypted append-only event log (`events`), session history (`sessions`), and file mutation metadata (`file_mutation_log`).
- **Ledger** (`.g8e/data/ledger/`): Multi-Ledger Architecture. A global bootstrap root plus per-session isolated git repositories at `sessions/<operator_session_id>/`. Each session ledger is initialized lazily on first file mutation for that session. Files are mirrored into the session ledger with a two-phase commit (pre/post snapshots).

---

## Storage Topology

```
┌─────────────────────────────────────────────────────────────────────┐
│                      g8e platform hub (Self-Hosted)                 │
│                                                                     │
│  ┌──────────────────────┐      ┌──────────────────────────────────┐ │
│  │         g8ee          │      │                              │ │
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
│  │                  Ledger (Multi-Ledger)                        │   │
│  │         data/ledger/sessions/<session_id>/ (Git)              │   │
│  │   Per-session isolated git repos; two-phase commits          │   │
│  └──────────────────────────────────────────────────────────────┘   │
└─────────────────────────────────────────────────────────────────────┘
```

---

## Component Storage Summary

| Component | Technology | Path/Volume | Role |
|---|---|---|---|
| **Platform Hub (Substrate)** | SQLite | `.g8e/data/g8e.db` | Central platform state (Coordination Store). |
| **Platform Hub (PKI)** | TLS Certs | `.g8e/pki` | CA hierarchy, intermediate CAs, and trust bundles. **Root of Trust**. |
| **Platform Hub (Secrets)** | Bootstrap Secrets | `.g8e/secrets` | Master secrets with tamper-evident manifest (`bootstrap_digest.json`). |
| **Stateless Adapters** | None | - | BYO Frontends/Agents; use `DocumentStore` and `KVStore` APIs. |
| **Operator (Audit Vault)** | SQLite (Enc) | `.g8e/data/g8e.db` | LFAA encrypted append-only log (`events`, `receipts`, `file_mutation_log`). |
| **Operator (Scrubbed Vault)** | SQLite | `.g8e/local_state.db` | Sentinel-scrubbed AI context (`execution_log`, `file_diff_log`). |
| **Operator (Raw Vault)** | SQLite | `.g8e/raw_vault.db` | Customer-only unscrubbed forensic record (`raw_execution_log`, `raw_file_diff_log`). |
| **Operator (Ledger)** | Git | `.g8e/data/ledger/sessions/<session_id>/` | Multi-Ledger: per-session isolated git repos for cryptographic file history, diff, and rollback. |

---

## Coordination Store (g8eo --listen)

The Coordination Store is the platform's central coordination point. It is implemented in the `g8eo` binary when running in `--listen` mode.

### Subsystems
- **Document Store**: Unified storage for JSON documents. Clients use a collection/ID pattern. All documents include `created_at` and `updated_at` timestamps managed by the store.
- **KV Store**: High-speed ephemeral data and read cache. Supports TTL, `GLOB` pattern matching, and cursor-based scanning (`KVScan`).
- **Blob Store**: Binary storage for investigation attachments, large objects, and certificate material.
- **SSE Buffer**: A per-session ring buffer for Server-Sent Events, ensuring clients can catch up after disconnects.
- **State Root Provider**: Calculates and maintains the platform-wide Merkle state root, binding all Hub data into a single verifiable hash.
- **Nonce Manager**: Prevents transaction replay by tracking used nonces with sliding-window expiration.

### The PKI and Secrets Directories (Root of Trust)
The `.g8e/pki` and `.g8e/secrets` directories form the platform's root of trust.

**PKI Directory (`.g8e/pki/`)** stores:
1. **CA Hierarchy**: Root CA, intermediate CAs (hub, operator, bootstrap), and trust bundles.
2. **Issued Certificates**: Server and workload certificates signed by intermediate CAs.

**Secrets Directory (`.g8e/secrets/`)** stores:
1. **Bootstrap Secrets**: `session_encryption_key`, `warden_signing_key`, and `warden_key_id`.
2. **Tamper-Evidence Manifest**: `bootstrap_digest.json` with SHA-256 digests of each secret.

On startup, `g8eo` SecretManager validates that secrets match the bootstrap digest manifest. If a conflict occurs, startup fails hard with actionable error messages.

### State Merkle Root Invariant
The platform state is anchored by a Merkle state root calculated across all documents, active KV entries, and blobs.
1. **Deterministic Calculation**: The root is computed by hashing a canonical JSON representation of all authoritative records in the Hub database.
2. **Transaction Binding**: Every governance transaction carries the `state_merkle_root` required for it to be valid.
3. **Fail-Closed Execution**: The Operator verifies that the transaction's state root matches the expected state before execution. Stale or invalid transactions are rejected.

---

## Local-First Audit Architecture (LFAA)

`g8eo` implements LFAA to ensure data sovereignty and tamper-evident auditing on managed hosts.

### Sentinel Defense & Scrubbing
Sentinel protects data privacy in two phases:
1. **Defense (Pre-Execution)**: Analyzes commands and file edits *before* they occur, blocking threat patterns.
2. **Scrubbing (Post-Execution)**: Removes sensitive data (API keys, PII) from output before it is stored in the Scrubbed Vault or sent to the platform.

### The Ledger (Multi-Ledger Architecture)
The Ledger uses a **Multi-Ledger Architecture**: each operator session owns an isolated git repository under `.g8e/data/ledger/sessions/<operator_session_id>/`. A global root at `.g8e/data/ledger/` is initialized at bootstrap but all runtime mutations are written into session-scoped repos.

- **Session Isolation**: Each session ledger is initialized lazily with a double-checked lock on first file mutation. Concurrent sessions never share a git working tree, preventing cross-session interference.
- **Two-Phase Commit**: Every mutation captures `LedgerHashBefore` (pre-mutation git commit) and `LedgerHashAfter` (post-mutation git commit), with the commit message embedding the operator session ID and a UTC timestamp.
- **Tamper Evidence**: Git's Merkle tree guarantees history integrity. The git commit hash is the state root for that mutation boundary.
- **Rollback**: Any file can be restored to any prior state within its session ledger (`RestoreFileFromCommit`).
- **Encrypted Mirror**: When the Encryption Vault is unlocked, mirrored files are stored as `.enc` (AES-256-GCM) and decrypted transparently on retrieval.
- **Graceful Degradation**: When git is unavailable (`--no-git`), the Ledger is disabled. The Audit Vault continues operating.

### Vault Strategy
- **Audit Vault** (`.g8e/data/g8e.db`): Encrypted using AES-256-GCM. Stores the definitive session history and **Action Receipts**.
- **Scrubbed Vault** (`.g8e/local_state.db`): Stores the "AI-ready" view of the host, including execution logs and file diffs processed by Sentinel.
- **Raw Vault** (`.g8e/raw_vault.db`): Stores unscrubbed forensic records, accessible only to authorized humans.

**Fail-Closed Auditing**: Audit events are fail-closed against session identity. `RecordEvent` and `RecordEvents` reject missing, malformed, or unknown `operator_session_id` values. Sessions must be created explicitly by auth lifecycle code before any audit writes are accepted.

---

## Canonical Collections

The following collections are defined in `protocol/constants/collections.json` and are used across the platform for Document Store organization:

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

- [protocol.md](protocol.md) — Governance Envelope and communication protocol
- [security.md](security.md) — mTLS, Sentinel patterns, and LFAA encryption
- [../developer/README.md](../developer/README.md) — Developer onboarding
