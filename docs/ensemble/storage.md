# Storage

## Overview

The g8e platform enforces strict data sovereignty across all storage tiers: raw command execution outputs, unscrubbed file contents, and host-level mutation records remain exclusively on the Governed Operator host under `.g8e/`, encrypted at rest via the local vault. The g8e Agentic Ensemble (`g8ee`) interacts with Gateway-hosted persistence services (Document Store, Blob Store, Key-Value Store) over mutual TLS (mTLS) on the HTTPS surface (port 8443) for structured workflow state, binary attachments, and ephemeral coordination data.

## Storage Architecture and Tiers

The storage ecosystem is split between Gateway-hosted canonical persistence in `g8e.db` and host-native Operator storage under `.g8e/`.

### Document Store

The Document Store provides JSON document persistence organized into collections in the Gateway SQLite database (`g8e.db`). The ensemble interacts with the Document Store via `DBService` and `DBClient`, which target the `/api/v1/data/` endpoint over HTTPS.

- **Collections** — Persists workflow entities including cases (`cases`), investigations (`investigations`), agent memory records (`memories`), benchmark evaluations (`evaluations`), agent reputation tracking (`reputation`), and platform configuration (`settings`).
- **Query and Filter Engine** — Supports structured collection queries via `POST /api/v1/data/{collection}/_query` with field filters, directional ordering (`order_by`), limit controls, and field projection (`select_fields`).
- **Document Lifecycle** — Direct document retrieval (`GET`), idempotent creation/replacement (`PUT`), and field merging (`PATCH`).
- **Array Operations and Batch Writes** — Supports `ArrayUnion` and `ArrayRemove` list modifications alongside atomic batch write operations (`BatchWriteOperation`).
- **Merkle State Root Integration** — The Document Store tracks state changes to maintain Merkle tree roots for governance validation and transaction verification.
- **Governed Mutations** — Critical state mutations (such as cases, investigations, memories, and reputation updates) originate through `GovernanceClient`, wrapping the update in a signed `GovernanceEnvelope` submitted to `/api/v1/governance/envelopes` for L1/L2/L3 verification before persistence.

### Blob Store

The Blob Store provides binary object storage for large payloads, attachments, and investigation evidence artifacts. The ensemble accesses this tier through `BlobService` and `BlobClient`, which wrap the `/api/v1/blobs/` Gateway API.

- **Namespaced Partitioning** — Blobs are isolated by namespace and object identifier, using the standard attachment namespace format `att:{investigation_id}`.
- **Binary Ingestion and Retrieval** — Direct binary upload with content-type headers (`PUT /api/v1/blobs/{namespace}/{id}`), retrieval (`GET /api/v1/blobs/{namespace}/{id}`), and single object deletion (`DELETE /api/v1/blobs/{namespace}/{id}`).
- **Bulk Cleanup** — Namespace-wide deletion (`DELETE /api/v1/blobs/{namespace}`) for purging all attachment artifacts when an investigation lifecycle concludes.

### Key-Value Store

The Key-Value Store provides high-performance, TTL-aware key-value persistence in the Gateway database for caching, session tracking, and ephemeral coordination. The ensemble interfaces with the KV tier using `KVService` and `KVCacheClient`, which communicate with `/api/v1/kv/`.

- **Key-Value Operations** — Key retrieval (`GET`), expiration-bounded write (`PUT` with `ttl` in seconds), key existence checks, and deletion (`DELETE`).
- **TTL Management** — Direct TTL query (`GET /api/v1/kv/{key}/_ttl`) and dynamic expiration updates (`PUT /api/v1/kv/{key}/_expire`).
- **Pattern Scanning and Purging** — Glob pattern key discovery (`POST /api/v1/kv/_keys`) and batch pattern deletion (`POST /api/v1/kv/_delete_pattern`).
- **Data Structure Emulation** — Client-side abstractions support hash mapping (`hget`, `hset`, `hgetall`, `hdel`), ordered list queues (`rpush`, `lpush`, `lrange`, `llen`, `ltrim`), atomic counters (`incr`, `decr`), and JSON serialization helpers (`get_json`, `set_json`).

### Operator-Side Host-Native Storage

The Governed Operator maintains local, sovereign storage under the `.g8e/` directory on target hosts. The ensemble never reads directly from Operator disk; all host interactions pass through the five-layer governance pipeline and native tools.

- **Audit Store** — Append-only record of operator sessions, execution events, file mutations, and signed action receipts, encrypted at rest through the local vault.
- **Git Ledger** — Git-backed version control tracking file mutations across two-phase commits, capturing pre-mutation snapshots, post-mutation commits, diff stats, and cryptographic content hashes.
- **Execution Vault** — Encrypted and compressed storage for command execution output and diffs, linked to workflow identifiers (user, case, task, investigation).
- **Token Store** — Encrypted key-value persistence for Sentinel tokens used by PII scrubbing and rehydration services.
- **Replay Store** — Standalone SQLite database managing atomic nonce reservation to prevent replay attacks during transaction execution.
- **Suspended Transaction Store** — Local persistence for governance envelopes awaiting human L3 authorization proofs.

## Data Sovereignty Principles

1. **Host Confinement** — Raw command stdout/stderr, unscrubbed tool outputs, and original file contents never leave the Operator host.
2. **Encrypted at Rest** — All sensitive data on the Operator host is encrypted at rest using local vault keys before persistence.
3. **Metadata-Only Platform State** — Platform-side storage on the Gateway contains only structured envelopes, Merkle state roots, consensus signatures, redacted summaries, and scrubbed artifacts.
4. **Autonomous Operator Authority** — Operators retain full authority over their local audit ledgers and filesystem state trees, independent of Gateway connectivity.

## Security and Resilience

- **mTLS Authentication** — Every request from `DBClient`, `BlobClient`, and `KVCacheClient` requires mutual TLS with client certificates signed by the platform CA.
- **Fail-Closed Operation** — Storage clients fail closed on network errors, certificate mismatches, or locked vaults; no plaintext fallback or unverified write paths are permitted.
- **Path Confinement** — Operator runtime file operations are strictly confined within `.g8e/` via `RuntimeFileService` abstractions.

## Related

- [Architecture](architecture.md) — System architecture, protocol surfaces, and service clients
- [Governance](governance.md) — Five-layer verification pipeline and envelope validation
- [PKI & Trust](pki.md) — Mutual TLS certificates and cryptographic identity
- [Protocol](protocol.md) — Gateway API surfaces and transaction lifecycle
- [Constants](constants.md) — Collection names, KV keys, and API paths
