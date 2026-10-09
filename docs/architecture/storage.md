---
doc_id: storage
title: Storage Architecture
audience: maintainers and coding agents
status: current
last_updated: 2026-10-06
version: v2.3.2
owners:
  - internal/services/storage/
  - internal/constants/paths.go
related:
  - encryption.md
  - gateway.md
  - operator.md
  - governance.md
  - events.md
  - sse.md
  - network.md
  - evals.md
  - model-provenance.md
when_to_read: Understanding database persistence, retention policies, audit evidence architecture, encryption boundaries, transaction storage, replay protection, and host-local execution evidence.
do_not_use_for:
  - Five-layer governance logic (see governance.md)
  - Network PKI and transport identity (see network.md)
  - Vault encryption algorithms (see encryption.md)
---

# Storage Architecture

## Purpose

Documents g8e's storage architecture: local SQLite databases, persistence topologies, audit evidence layout, encryption boundaries, transaction storage for L3 approvals, replay protection, and retention policies. The Gateway and outbound Operators each maintain a canonical database and optional satellite stores for audit, execution, and ledger evidence. This document maps persistence design to implementation in [internal/services/storage/](../../internal/services/storage/).

## Quick index

- [Purpose](#purpose)
- [Quick index](#quick-index)
- [Invariants](#invariants)
- [Owned surfaces](#owned-surfaces)
- [Procedures](#procedures)
- [Anti-patterns](#anti-patterns)
- [Links out](#links-out)

## Invariants

| ID | Rule |
| --- | --- |
| INV-STOR-KV-01 | The Gateway exposes no HTTP KV API. Session, nonce, scrubbing-token, and other KV keys are private to Gateway services. |
| INV-STOR-KV-02 | No `kv_store` row is a derived copy of a document or query result, and document writes MUST NOT write or delete KV rows. Every `kv_store` row is committed state in the bound or observed tier. Schema initialization drops the legacy document-cache triggers and deletes legacy `g8e:cache:*` rows on existing databases. |
| INV-STOR-KV-03 | g8ee MUST use Gateway storage primitives and MUST NOT emulate atomic counters, hashes, lists, or nonce reservation through client-side read/modify/write. Governed replay protection belongs to the executing runtime's uniqueness-constrained replay store. |

## Owned surfaces

| Surface | Owner | Verification |
| --- | --- | --- |
| Canonical schema and state-commitment dirty-mark triggers | `internal/services/gateway/db/schema.sql`, `CanonicalDBService.initSchema` | `TestStateRootService_LegacyDocumentCacheIsRemovedOnOpen`, `TestStateRootService_LegacyDatabaseIsRebuiltOnOpen` |
| Internal KV state and TTL | `KVStoreService` | Gateway KV integration tests |
| g8ee document access (no app cache) | `ensemble/app/db/document_service.py`, `DocumentService` | `tests/unit/db/test_document_service.py`; every read goes to the Gateway document API |
| Replay nonce reservation | `ReplayStoreService` and outbound `SQLReplayStore` | Replay and L4 verification tests |
| Runtime paths | `RuntimeFileService`, `internal/constants/paths.go` | Isolated runtime fixture tests |

Enrollment admission uses the existing document store and `BEGIN IMMEDIATE` transaction owner. Deduplication, live-request counting, and insertion share one transaction. Partial expression indexes cover enrollment identity, token hash, live capacity, pending expiry, issuance leases, and retention. Token and pending reads decode only matching requests; the cleanup owner explicitly expires abandoned requests and retains completed issuance records. Batch decisions recheck active-owner authority and every fingerprint-bound member in one transaction, recording the same governed receipt ID on every member.

Operator replacement uses `DocumentStoreService.FindOperatorLeases`, an indexed lookup of the approving owner's exact system fingerprint. It includes every non-terminated remote lease regardless of heartbeat staleness and decodes only matching identities. The lookup performs no liveness reconciliation; registry reads retain their existing reconciliation behavior. Session authentication uses `DocumentStoreService.FindOperatorsBySession` and a partial session-ID expression index, reads at most two bindings to detect duplicates, and performs no heartbeat reconciliation.

SQLite retry helpers take an explicit context for connection-pool waits, statement execution, transaction acquisition, and retry backoff. Cancellation ends backoff and prevents another attempt; exhausted attempts return `ErrSQLiteBusy` while preserving the driver's error. Busy classification uses the SQLite error code, including extended busy codes, rather than rendered text. Failed attempts do not sleep after the last attempt. Existing context-free owners supply a background context, and execution-vault evidence remains independent of execution cancellation. The driver's internal busy wait cannot be interrupted by a context, so the default busy timeout is a short 100 ms (`sqliteutil.DefaultBusyTimeoutMs`) and the retry helpers do the longer, cancellable waiting; a deadline overshoots by at most one busy wait. Physical connection initialization and context-free bootstrap/root reads can still outlast cancellation, so context-aware retries alone do not establish an HTTP deadline guarantee. A 5 ms busy timeout was measured and rejected: backoff idled the writer and N=1,000 completions fell from about 111/s to about 68/s.

## Procedures

This document does not define a general SQLite backup or recovery procedure. The supported lifecycle operations are owned by CLI commands; confirm flags and defaults with `./g8e <command> --help`.

- **Reset host runtime state:** `g8e gw clean` stops services and renames `.g8e/` aside to `.g8e-<MMDDHHMM>` (nothing is deleted) after offering an evaluation-evidence backup, so the Gateway restarts with no databases, vault, or PKI. `g8e docker clean` removes the Docker stack volumes. `make clean` removes build and test artifacts only and does not reset runtime state, trust, or databases.
- **Back up and restore evaluation evidence:** `g8e eval backup` and `g8e eval restore` copy and verify `.g8e/data/eval/` and `.g8e/eval/`; see [Evaluation Programs](./evals.md).
- **Verify the audit chain:** `g8e audit verify` walks the hash chain from the latest checkpoint (or genesis) and reports the head sequence and hash.

## Anti-patterns

- Bypassing the RuntimeFileService to open database paths directly.
- Hardcoding database paths instead of using constants from [internal/constants/paths.go](../../internal/constants/paths.go).
- Assuming database file permissions are set by SQL library code (they are owned by RuntimeFileService).
- Placing encrypted content outside the field-level vault boundaries (see Security Properties).

## Overview

g8e separates platform coordination state from host-local execution evidence. The Gateway and each outbound Operator opens a local canonical SQLite database. The Gateway uses it for platform coordination state, enrolled Operator registries, session management, and Gateway-runtime audit evidence. An outbound Operator opens its own copy of the canonical database for the shared state-root schema and its authoritative evidence from operations executed in that Operator runtime.

Satellite stores have separate lifecycles and purposes. An outbound Operator maintains an execution vault for encrypted command output and file diffs, a replay database for independent nonce reservation, and optional Git-backed file ledgers. Both Gateway and outbound Operator modes open a suspended-transaction database for transactions awaiting L3 approval. Native and campaign evaluation artifacts persist under host-local `.g8e/data/eval/` on the evaluation CLI host; model inventories and rollout queues use project-root `.g8e/eval/` paths. A remote Operator remains authoritative for its local execution evidence; its publication of signed receipts to the Gateway is best-effort mirroring.

See [Encryption Architecture](./encryption.md) for vault and keystore protection, [Gateway Architecture](./gateway.md) for Gateway service assembly, [Operator Architecture](./operator.md) for host-local execution, and [Event and Action Protocol](./events.md) for the audit-chain and receipt-projection contract.

## Persistence Topology

Database paths are relative to `.g8e/data/` within the runtime directory. Constants are defined in [internal/constants/paths.go](../../internal/constants/paths.go): `DbFilename`, `ExecutionVaultDBFilename`, `ReplayStoreDBFilename`, `SuspendedTxFilename`.

| Store | Gateway | Outbound Operator | Purpose |
| --- | --- | --- | --- |
| Canonical database (`g8e.db`) | ✓ | ✓ | Platform documents, key-value and blob state, state roots, nonce reservations, SSE events, audit chain, receipts, and commitments. |
| Execution vault (`execution_vault.db`) | — | ✓ | Encrypted command output, stderr, and file-diff content; searchable execution metadata. |
| Replay store (`replay_store.db`) | — | ✓ | Durable nonce reservation for independent L4 verification. Gateway replay uses the canonical database. |
| Suspended-transaction store (`suspended_transactions.db`) | ✓ | ✓ | Transactions and approval proof material awaiting L3 approval. |
| File ledger (`.g8e/data/ledger/`) | — | Optional | Git-backed version control: per-session repositories, file snapshots, commit history, and diffs. |
| Inference evidence files (`.g8e/data/inference/`) | ✓ | Inference role | Canonical protojson files written through RuntimeFileService, not SQLite. The Gateway stores `provider-observer/windows/` and `model-provenance/windows/` attestation windows ingested from Observer and Provenance Operators; an Inference Operator stores provider-attempt records and raw responses under `attempts/`. See [Model Provenance](./model-provenance.md). |
| Evaluation artifacts | Evaluation CLI | Evaluation CLI | `.g8e/data/eval/`: run and campaign evidence. Project-root `.g8e/eval/`: model inventories, rollout queue. |

All databases are local to the runtime that opens them. An outbound Operator's `g8e.db` is not a central platform database; it is authoritative only for evidence produced by that Operator. During L4 verification, the Operator obtains the Gateway's current state root; its local database is not substituted. The Gateway mirrors remote signed receipts on a best-effort basis; mirroring failure does not invalidate the Operator's local evidence.

In the root Compose deployment, `g8e-gateway`, `g8e-data-operator`, `g8e-inference-operator`, and `ensemble` use separate named volumes (`g8e-gateway-data`, `g8e-operator-data`, `g8e-inference-data`, `g8e-ensemble-data`). The cross-enrollment `g8e-gateway-secondary` service and the `g8ellama` User Gateway (`g8e-gateway-user`) each use their own volume. The `g8ellama` Inference Node (`g8e-inference`) mounts the same `g8e-inference-data` volume as `g8e-inference-operator`, so those two services share one runtime directory when the profile is combined with the default stack. The shared `/tmp` volume is not a storage boundary. Removing a component volume removes that component's database, PKI, vault, and evidence only.

## Canonical Database

The canonical database ([internal/services/storage/](../../internal/services/storage/)) opens in SQLite WAL mode with foreign-key enforcement, busy timeout, bounded retries for lock contention, and incremental vacuum. `sqliteutil.OpenDB` applies the settings through the modernc driver's `_pragma` connection parameters, so every pooled connection uses `synchronous=NORMAL`, the configured busy timeout and page cache, foreign-key enforcement, and memory-backed temporary storage. Read-only connections apply `query_only=ON` and the configured busy timeout. Database-file permissions are owned by the RuntimeFileService; the SQLite abstraction does not create parent directories or change permissions on open. Gateway and audit services use separate connection pools against the same `g8e.db` file.

### Platform Documents

The document store persists JSON records by collection and identifier. Gateway services use it for users, sessions, Operators, policies, consensus definitions, signer records, enrollment state, passkeys, revocations, and other platform resources. Document writes touch no key-value rows; each write marks only its own leaf dirty in the incremental state commitment.

### Key-Value and Blob State

The key-value store persists string values with optional expiration. The blob store persists binary content by namespace and identifier with content type, size, and optional expiration. Both stores distinguish bound state from observed state.

There is no HTTP key-value API; Gateway services access KV state directly through `KVStoreService`. The Gateway keeps no derived copy of documents or query results in `kv_store`. g8ee does not maintain a second governed replay guard or emulate Redis structures.

Bound documents, bound key-value entries, and bound blobs contribute to the state root that L4 verifies. Replay nonces and SSE events do not contribute to that root. Observed key-value entries and blobs are excluded from the admission root and can be hashed as a separate observed-state commitment.

A state-root read honors its caller's context, including while it waits for a pooled connection; a canceled read returns an error, and a canceled pending-change fold rolls back without publishing a root. Admission callers (governed dispatch, MCP/A2A, platform enrollment, `/api/v1/state`, and health) pass their request context, and an Operator's remote fetch ends at the earlier of its caller's deadline or 15 seconds. L5 receipt `state_root_before`/`state_root_after` reads ignore caller cancellation, as the receipt writes do.

### Scrubbing Token Store

The scrubbing service stores reversible UEI token values through an encrypted adapter over the canonical key-value store. The adapter adds a dedicated namespace, marks entries as observed state, and applies their TTL. Token values are encrypted before persistence; reads and writes fail when the vault is locked.

### Replay Protection

The Gateway replay service reserves nonces in `g8e.db`. A uniqueness constraint makes concurrent reservation atomic, and database errors prevent admission rather than bypassing replay protection. Gateway maintenance removes expired reservations.

An outbound Operator uses a standalone replay database because it independently performs L4 verification. Reservation also relies on a uniqueness constraint and first removes expired records. Validation failures release a reservation, while a successful transaction leaves its reservation in place until expiration in the current execution path.

### SSE Event Buffer

The SSE event buffer supports reconnection replay for authenticated browser and CLI sessions. Every event belongs to a user and exactly one session route. These events are delivery telemetry, not governance state, and maintenance removes events older than one hour.

## Audit Evidence

### SQL Audit Store

The SQL audit store shares `g8e.db` with canonical platform persistence. It records Operator and app sessions, hash-chained audit events, file-mutation references, signed action receipts, and the commitment chain. Events require an associated session, except the `g8e.v1.platform.audit.chain.checkpointed` anchor; single-event insertion creates an app session when necessary, while batch insertion requires every referenced session to exist and commits atomically. Event, receipt, summary, and report queries take an explicit audit scope of Operator session ID, acting app ID, or both (`--app` on `g8e audit` subcommands and `acting_app_id` on the Gateway audit routes). App identity resolves through the transaction receipt and is never inferred from a session, and an empty scope returns events across all sessions, including events without an Operator session.

The vault encrypts event content, command standard output, and command standard error. Command text, event metadata, file paths, ledger hashes, receipt fields, and canonical receipt JSON remain structured and are not protected by field-level vault encryption. Output above the configured threshold is reduced to head and tail sections before encryption.

#### Hash-chained audit log

Every row in the `events` table is an append-only chain entry. Each entry records `seq`, `prev_hash`, a plaintext `content_digest`, and its own `hash`. Receipt stages, LFAA audit facts, and other persisted events share this chain. Structural verification recomputes hashes without decrypting vault-protected fields; content verification compares plaintext against `content_digest` when the vault is unlocked.

Pruning deletes chained rows only after appending a `g8e.v1.platform.audit.chain.checkpointed` anchor that records the pruned range and terminal hash. `g8e audit verify` and `GET /api/v1/audit/verify` walk the chain from the latest checkpoint (or genesis) and report the head sequence and hash.

#### Receipt projection

The `receipts` table is a latest-stage query projection, not the audit record of record. L5 upserts one row per `transaction_id` as the receipt advances from `EXECUTING` to the signed final receipt that carries the signed persistence attestation. The `EXECUTING` write also appends the transaction's commitment in the same transaction when the commitment ledger is configured. Every stage write also appends a chained `g8e.v1.operator.receipt.recorded` fact whose `content_text` is the canonical protojson receipt for that stage.

Receipt canonicalization and audit-content encryption happen before acquiring the commitment ledger mutex or SQLite write transaction. Nonempty session IDs are checked for surrounding whitespace before taking the ledger lock. Session insertion, the receipt upsert, audit-chain append, and optional commitment append still commit atomically; receipt and event foreign keys enforce session existence without a repeated session lookup; commitment construction uses the head read inside that transaction. Receipt and commitment success logs run only after commit and after releasing the ledger mutex, so log sinks do not extend the chain's critical section.

Compliance operational export carries both the retained receipt body and the matching chained receipt facts. The evidence importer verifies the exported chain segment, cross-links matching receipt projections to their chain entries, and cross-links commitments to receipts through deterministic stage evidence.

For a remote Operator, the local audit store is authoritative. After execution, the Operator publishes the signed receipt to the Gateway receipt channel. The Gateway verifies the signer, mirrors an accepted receipt into its own chain and projection, but publication failure does not invalidate the already persisted local result.

### Commitment Ledger

The commitment ledger is a permanent SQLite hash chain inside `g8e.db`, distinct from the git-backed file ledger. Before execution, L5 builds and signs a `CommitmentAttestation` against the current chain head while SQLite holds the write lock. This serialization prevents concurrent writers from selecting the same predecessor.

The commitment binds the transaction, state root, action and target, prior commitment hash, L2 and L3 signature digests, and the initial Warden-to-Actuator receipt signature digest. L5 records the commitment and prior hashes in deterministic stage evidence and does not execute when commitment persistence fails. Compliance tooling verifies the chain, signatures, structured fields, and links to receipts independently.

## Host-Local Evidence

### Execution Vault

An outbound Operator's execution vault stores command records and file-diff records in a standalone SQLite database. Standard output, standard error, and diff content are encrypted before compression; hashes cover the original content. Commands, file paths, sizes, hashes, exit status, timing, and workflow identifiers remain structured.

The vault must be present, and protected writes fail while it is locked. Execution-vault persistence occurs after execution through the Operator result pipeline and is best-effort, so a storage error is logged but does not reverse an action that already completed.

### File Ledger

When Git ledger support is enabled, the Operator maintains a default repository and an isolated repository for each session. A governed file mutation snapshots the pre-mutation state, performs the host operation, then copies and commits the resulting state. Before and after commit hashes, diff summaries, and diff content link file history to audit and execution-vault records.

The ledger supports file history, point-in-time reads, and restoration. Host paths are normalized to stable repository-relative paths. When the vault is unlocked, mirrored file content is encrypted and stored with an `.enc` suffix; when the vault is locked, the current ledger path writes plaintext. The ledger does not provide the same fail-closed encryption behavior as the audit store and execution vault, which fail closed when vault is locked.

### History Coordination

The history service combines SQL audit events with file-mutation references for completed edits. File history, point-in-time reads, and restoration come from the session's file ledger. When the ledger is disabled, SQL event history remains available but file history and restoration do not.

## Pending L3 Approvals

The suspended-transaction store persists the canonical envelope, tool context, requestor and Operator identity, expiration, approval state, and either signed CLI or WebAuthn proof material. Reads and listings exclude expired records. Approval updates only an unexpired transaction, successful resumed execution removes the record, and a failed resumed execution leaves it available for the caller to inspect or retry according to the remaining validity window.

This standalone database does not apply vault field encryption. Its envelopes and approval proof material therefore rely on runtime-directory access controls and database-file permissions at rest.

## Runtime File I/O

RuntimeFileService ([internal/services/fs/](../../internal/services/fs/)) is the canonical abstraction for paths and file operations inside the `.g8e/` runtime tree. The audit store uses it to establish and verify its data directory before opening the resolved SQLite path. The file ledger uses it for runtime directories and ledger files. File operations outside the runtime tree (host targets, governed mutations) access those paths directly outside the abstraction boundary.

All SQLite services resolve their database paths through RuntimeFileService before opening them. Under default configuration, relative database paths resolve beneath `.g8e/data/`. The abstraction owns file permissions and directory creation; the SQLite layer does not. `WriteFile` writes atomically through a temporary file in the target directory and a rename; on Windows it clears the read-only attribute on an existing destination before the rename and restores the previous mode if the rename fails.

## Retention and Maintenance

Each service runs its own pruner on an interval. Retention is service-specific, not unified across stores:

- **Audit store** ([internal/services/storage/audit_store.go](../../internal/services/storage/audit_store.go)): By default, an hourly background task removes events, file-mutation links, and receipts older than 90 days, then removes sessions with no remaining events or receipts. Commitments remain permanent. The configured database size limit (default 2048 MB) does not trigger size-based deletion; the time-based policy is authoritative.
- **Execution vault** ([internal/services/storage/execution_vault.go](../../internal/services/storage/execution_vault.go)): By default, an hourly background task removes execution and diff records older than 30 days. When the database exceeds 1 GiB, the task removes the oldest tenth of each record set.
- **Suspended transactions** ([internal/services/storage/suspended_transaction_store.go](../../internal/services/storage/suspended_transaction_store.go)): By default, a background task runs every 30 minutes. It removes expired records (expiration governs age-based deletion, not the configured retention-days value) and removes the oldest tenth when the database exceeds 256 MiB.
- **Canonical database (key-value and blobs):** Every 30 seconds, maintenance removes expired key-value entries, blobs, and nonce reservations, plus SSE events older than one hour.
- **Replay store (outbound Operator):** Expired nonces are removed during reservation. No automatic background pruner; stale-reservation and used-nonce pruning operations are available for manual invocation.
- **Commitment and file ledgers:** Commitments remain permanent. File-ledger history has no automatic retention or size pruning.

The audit store, execution vault, and suspended-transaction store run incremental vacuum after scheduled pruning to gradually reclaim free pages.

## Governance Transaction Flow

Every governed operation follows the [five-layer interlock](./governance.md):

1. **L1 Doctrine** validates the typed payload and applies hard gates, forbidden-pattern matching, and MITRE threat detection.
2. **L2 Consensus** verifies Ed25519 consensus votes when the active posture requires them.
3. **L3 Notary** verifies WebAuthn or signed CLI authorization when the active posture requires it. Transactions awaiting Gateway-managed approval are persisted in the suspended-transaction store.
4. **L4 Warden** checks expiry, reserves the nonce, validates payload and transaction hash, verifies the state root, and evaluates required L2 and L3 evidence. Failed validation releases the nonce reservation.
5. **L5 Actuator** signs and persists the `EXECUTING` receipt, appends the signed commitment, rehydrates protected values, invokes the handler, and signs and persists the final receipt and persistence attestation.

Initial receipt and commitment are execution gates. Final receipt persistence occurs after the mutation, so final-persistence failure is returned with receipt evidence but cannot roll back an external side effect. On a remote Operator, publication of the completed receipt to the Gateway is best-effort.

## Security Properties and Limits

- SQLite uniqueness constraints provide durable, concurrent nonce replay detection.
- L5 does not dispatch when initial receipt signing or persistence fails, or when the commitment cannot be appended.
- The commitment ledger serializes chain-head selection and supports independent offline verification.
- The canonical state root binds active authoritative documents, key-value state, and blobs while excluding volatile delivery and replay data.
- Field-level vault encryption protects selected audit content, execution output, file diffs, and scrubbing tokens. It does not encrypt complete databases or all metadata.
- Suspended envelopes, approval proofs, platform documents, SSE payloads, commitment records, and structured metadata are not vault-encrypted.
- Ledger copies are encrypted only while the vault is unlocked; the current ledger path can fall back to plaintext.
- A remote Operator's local receipt is authoritative. Gateway receipt mirroring improves centralized visibility but is not a durability guarantee for remote evidence.
- Retention removes audit receipts while commitments remain permanent, so long-term commitment verification can outlive the locally retained receipt cross-link.

## Links out

- [Governance](./governance.md): Five-layer verification and posture behavior.
- [Encryption Architecture](./encryption.md): Vault and keystore cryptography.
- [Gateway Architecture](./gateway.md): Gateway service assembly and canonical database ownership.
- [Operator Architecture](./operator.md): Host execution and local-first evidence.
- [Network Architecture](./network.md): mTLS, transport identity, and PKI.
- [Event and Action Protocol](./events.md): Audit chain and receipt projection.
- [SSE Streaming](./sse.md): Session routing and event replay.
- [Evaluation Programs](./evals.md): Native and campaign evaluation evidence layout.
- [Model Provenance](./model-provenance.md): Attestation window persistence and preflight.
