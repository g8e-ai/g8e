---
title: Storage Architecture
parent: Architecture
---

# Storage Architecture

Last Updated: 2026-09-23
Version: v2.1.12

## Overview

g8e separates platform coordination state from host-local execution evidence. Each Gateway and outbound Operator opens a local canonical SQLite database at `.g8e/data/g8e.db`. The Gateway uses it for platform coordination state and Gateway-runtime audit evidence. An outbound Operator opens its own copy for the shared state-root schema, replay-independent execution services, and authoritative evidence from operations executed in that Operator runtime.

Additional stores have separate lifecycles. An outbound Operator maintains `.g8e/data/execution_vault.db`, `.g8e/data/replay_store.db`, and, when Git integration is enabled, Git-backed ledgers under `.g8e/data/ledger/`. Both Gateway and outbound Operator modes open `.g8e/data/suspended_transactions.db` for pending L3 approvals. Native and campaign evaluation artifacts persist under `.g8e/data/eval/` on the host that owns the evaluation CLI; model inventories and the rollout queue use the project-root `.g8e/eval/` paths. See [Evaluations](./evals.md) and the [Unified Docker Stack Guide](../guides/unified_stack.md). A remote Operator remains authoritative for its local execution evidence; its publication of signed receipts to the Gateway is a best-effort mirror.

See [Encryption Architecture](./encryption.md) for vault and keystore protection, [Gateway Architecture](./gateway.md) for Gateway service assembly, [Operator Architecture](./operator.md) for host-local execution, and [Event and Action Protocol](./events.md) for the audit-chain and receipt-projection contract.

## Persistence Topology

| Store | Gateway | Outbound Operator | Default path and purpose |
| --- | --- | --- | --- |
| `g8e.db` | Yes | Yes | `.g8e/data/g8e.db`: platform documents, key-value and blob state, state roots, Gateway replay nonces, SSE events, audit events, receipts, and commitments |
| Suspended-transaction database | Yes | Yes | `.g8e/data/suspended_transactions.db`: transactions and proof material awaiting L3 approval |
| Execution vault | Not a separate Gateway service | Yes | `.g8e/data/execution_vault.db`: command output and file-diff content plus searchable execution metadata |
| Replay database | Gateway replay uses `g8e.db` | Yes | `.g8e/data/replay_store.db`: durable nonce reservation for the Operator's independent L4 verification |
| File ledger | Not a separate Gateway service | Optional | `.g8e/data/ledger/`: default and per-session file snapshots, commit history, diffs, and restoration |
| Evaluation artifacts | Host-side evaluation CLI | Host-side evaluation CLI | `.g8e/data/eval/` for run and campaign evidence; project-root `.g8e/eval/` for inventories and rollout state |

All of these files are local to the runtime that opens them. The presence of `g8e.db` on an outbound Operator does not make the Operator a central platform database. During outbound verification, the Operator obtains the Gateway's current state root; its local database is not substituted for that Gateway root. The Gateway receipt mirror does not replace the Operator's local record.

In the root Compose deployment, `g8e-gateway`, `g8e-operator`, and `g8e-inference-operator` use separate named runtime volumes (`g8e-gateway-data`, `g8e-operator-data`, and `g8e-inference-data`). The cross-enrollment secondary gateway and the `g8ellama` inference topology also use separate volumes. The shared `/tmp` volume is not a storage-owner boundary for these databases. Removing a component volume removes that component's local database, PKI, vault, and evidence; it does not remove another component's stores. See [Docker Gateway Guide](../guides/docker_gateway.md) and [Unified Docker Stack Guide](../guides/unified_stack.md) for the complete Compose topology.

## Canonical Database

The canonical database opens in SQLite WAL mode with foreign-key enforcement, a busy timeout, bounded retries for lock contention, and incremental vacuum support. Database-file permission changes are owned by the runtime file service and the explicit SQLite open boundary; the shared SQLite layer does not create parent directories or mutate database-file permissions on open. Gateway and audit services use separate connection pools to the same `g8e.db` file.

### Platform Documents

The document store persists JSON records by collection and identifier. Gateway services use it for users, sessions, Operators, policies, consensus definitions, signer records, enrollment state, passkeys, revocations, and other platform resources. Document writes invalidate related key-value cache entries and advance the state version used to cache state-root calculations.

### Key-Value and Blob State

The key-value store persists string values with optional expiration. The blob store persists binary content by namespace and identifier with content type, size, and optional expiration. Both stores distinguish bound state from observed state.

Bound documents, active bound key-value entries, and active bound blobs contribute to the state root that L4 verifies. Cache entries, replay nonces, and SSE events do not contribute to that root. Observed key-value entries and blobs are excluded from the admission root and can be hashed as a separate observed-state commitment.

### Scrubbing Token Store

The scrubbing service stores reversible UEI token values through an encrypted adapter over the canonical key-value store. The adapter adds a dedicated namespace, marks entries as observed state, and applies their TTL. Token values are encrypted before persistence; reads and writes fail when the vault is locked.

### Replay Protection

The Gateway replay service reserves nonces in `g8e.db`. A uniqueness constraint makes concurrent reservation atomic, and database errors prevent admission rather than bypassing replay protection. Gateway maintenance removes expired reservations.

An outbound Operator uses a standalone replay database because it independently performs L4 verification. Reservation also relies on a uniqueness constraint and first removes expired records. Validation failures release a reservation, while a successful transaction leaves its reservation in place until expiration in the current execution path.

### SSE Event Buffer

The SSE event buffer supports reconnection replay for authenticated browser and CLI sessions. Every event belongs to a user and exactly one session route. These events are delivery telemetry, not governance state, and maintenance removes events older than one hour.

## Audit Evidence

### SQL Audit Store

The SQL audit store shares `g8e.db` with canonical platform persistence. It records Operator and app sessions, hash-chained audit events, file-mutation references, signed action receipts, and the commitment chain. Events require an associated session; single-event insertion creates an app session when necessary, while batch insertion requires every referenced session to exist and commits atomically.

The vault encrypts event content, command standard output, and command standard error. Command text, event metadata, file paths, ledger hashes, receipt fields, and canonical receipt JSON remain structured and are not protected by field-level vault encryption. Output above the configured threshold is reduced to head and tail sections before encryption.

#### Hash-chained audit log

Every row in the `events` table is an append-only chain entry. Each entry records `seq`, `prev_hash`, a plaintext `content_digest`, and its own `hash`. Receipt stages, LFAA audit facts, and other persisted events share this chain. Structural verification recomputes hashes without decrypting vault-protected fields; content verification compares plaintext against `content_digest` when the vault is unlocked.

Pruning deletes chained rows only after appending a `g8e.v1.platform.audit.chain.checkpointed` anchor that records the pruned range and terminal hash. `g8e audit verify` and `GET /api/v1/audit/verify` walk the chain from the latest checkpoint (or genesis) and report the head sequence and hash.

#### Receipt projection

The `receipts` table is a latest-stage query projection, not the audit record of record. L5 upserts one row per `transaction_id` as the receipt advances from `EXECUTING` through the signed final receipt to the receipt that carries the signed persistence attestation. Every stage write also appends a chained `g8e.v1.operator.receipt.recorded` fact whose `content_text` is the canonical protojson receipt for that stage.

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

When file ledger support is enabled, the Operator maintains a default git repository and an isolated repository for each Operator session. A governed file mutation snapshots the pre-mutation state, performs the host operation, then copies and commits the resulting state. The resulting before and after commit hashes, diff summary, and diff content link file history to audit and execution-vault records.

The ledger supports file history, point-in-time reads, and restoration. Host paths are normalized to stable repository-relative paths, and encrypted file copies are limited to 100 MiB. When the vault is unlocked, mirrored file content is encrypted and stored with an `.enc` suffix; the current ledger copy path writes plaintext when the supplied vault is locked, so the ledger does not provide the same fail-closed encryption behavior as the audit store, execution vault, and token adapter.

### History Coordination

The history service combines SQL audit events with file-mutation references for completed edits. File history, point-in-time reads, and restoration come from the session's file ledger. When the ledger is disabled, SQL event history remains available but file history and restoration do not.

## Pending L3 Approvals

The suspended-transaction store persists the canonical envelope, tool context, requestor and Operator identity, expiration, approval state, and either signed CLI or WebAuthn proof material. Reads and listings exclude expired records. Approval updates only an unexpired transaction, successful resumed execution removes the record, and a failed resumed execution leaves it available for the caller to inspect or retry according to the remaining validity window.

This standalone database does not apply vault field encryption. Its envelopes and approval proof material therefore rely on runtime-directory access controls and database-file permissions at rest.

## Runtime File I/O

`RuntimeFileService` is the canonical abstraction for paths and file operations inside the `.g8e/` runtime tree. The audit store uses it to establish and verify its data directory before opening the resolved SQLite path. The file ledger uses it for runtime directories and mirrored ledger files, while the execution boundary accesses governed host targets outside the runtime tree.

Standalone SQLite services open their configured database paths through the shared SQLite layer. Production callers resolve each canonical relative database path exactly once through the injected `RuntimeFileService` before opening SQLite; under default configuration those paths resolve beneath the runtime data directory.

## Retention and Maintenance

Retention is service-specific rather than a single policy applied to every store:

- **Audit store:** By default, an hourly task removes events, file-mutation links, and receipts older than 90 days, then removes sessions with no remaining events or receipts. Commitments are not removed. The configured audit database size value does not currently trigger size-based deletion.
- **Execution vault:** By default, an hourly task removes execution and diff records older than 30 days. When the database exceeds 1 GiB, it removes the oldest tenth of each record set.
- **Suspended transactions:** By default, a task runs every 30 minutes, removes expired records, and removes the oldest tenth when the database exceeds 256 MiB. Expiration, rather than the configured retention-days value, controls age-based deletion.
- **Canonical Gateway stores:** Every 30 seconds, maintenance removes expired key-value entries, blobs, and nonces, plus SSE events older than one hour.
- **Standalone Operator replay:** Expired nonces are removed during reservation. Additional stale-reservation and used-nonce pruning operations are available, but no background pruner runs for this database.
- **Commitment and file ledgers:** Commitments are permanent, and file-ledger history has no automatic retention or size pruning.

The audit store, execution vault, and suspended-transaction store run incremental vacuum after scheduled pruning to reclaim free pages gradually.

## Governance Transaction Flow

Every governed operation follows the [five-layer interlock](./governance.md):

1. **L1 Doctrine** validates the typed payload and applies hard gates, forbidden-pattern matching, and MITRE threat detection.
2. **L2 Consensus** verifies Ed25519 consensus votes when the active posture requires them.
3. **L3 Notary** verifies WebAuthn or signed CLI authorization for mutations when the active posture requires it; transactions awaiting Gateway-managed approval enter the suspended-transaction store.
4. **L4 Warden** checks expiry, reserves the nonce, validates the payload and transaction hash, verifies the state root, and evaluates required L2 and L3 evidence. A failed validation releases the nonce reservation.
5. **L5 Actuator** signs and persists the `EXECUTING` receipt, appends the signed commitment, rehydrates protected values, mints a transaction-bound capability, invokes the handler, dissolves the capability, and signs and persists the final receipt and persistence attestation.

The initial receipt and commitment are execution gates. Final receipt persistence occurs after the mutation, so a final persistence failure is returned with the available receipt evidence but cannot roll back an external side effect. On a remote Operator, publication of the completed receipt to the Gateway is also best-effort.

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

## Related Documentation

- [Governance](./governance.md): Five-layer verification and posture behavior
- [Authentication and Authorization](./auth.md): Identity, sessions, and L3 approval
- [Encryption Architecture](./encryption.md): Vault and keystore cryptography and operations
- [Gateway Architecture](./gateway.md): Canonical database ownership and Gateway service assembly
- [Operator Architecture](./operator.md): Host execution and local-first evidence
- [SSE Streaming](./sse.md): Session routing and event replay
- [Network Architecture](./network.md): mTLS and transport identity
- [Evaluations](./evals.md): Native and campaign evaluation evidence layout
- [g8e Protocol](../../protocol/docs/spec.md): Canonical governance messages and wire contract
