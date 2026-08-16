# Storage Architecture

Last Updated: 2026-08-16

## Overview

The g8e storage layer is the persistence foundation for the five-layer governance pipeline. It records every operator session, command execution, file change, governance transaction, and audit attestation so the platform can replay history, verify state, and prove what happened.

The layer is split into specialized services. Sensitive services encrypt content at rest through the vault and fail closed when the vault is locked. Services that store public or non-sensitive data, such as replay nonces and suspended governance envelopes, do not require encryption.

In gateway mode the canonical SQLite database `g8e.db` hosts the audit log, action receipts, key-value store, document store, blob store, replay nonces, and SSE event buffer. The ledger, execution vault, and suspended transaction store use their own files. In outbound/operator mode the replay store, execution vault, and suspended transaction store run as standalone SQLite databases; the audit store and ledger remain local to the operator.

See [Encryption Architecture](./encryption.md) for vault and key management, and [Gateway Architecture](./gateway.md) for service initialization in gateway mode.

## Storage Services

### Audit Store

The audit store is the append-only record of operator sessions, events, file mutations, and signed action receipts. It stores event content, command output, and action receipts, encrypting sensitive fields through the vault. New events must reference an existing session. Batch insertion is atomic. Records older than the configured retention window are pruned automatically.

### Ledger

The ledger provides git-backed version control for all file modifications. Each operator session keeps an isolated repository. File changes follow a two-phase pattern: the ledger snapshots the pre-mutation state, the operator performs the write, delete, or create, and the ledger commits the post-mutation state and diff. File copies are encrypted when the vault is unlocked. The HEAD commit of the ledger is exposed as a verifiable state snapshot. The ledger also supports history queries, point-in-time retrieval, and restoration.

### Execution Vault

The execution vault stores command results and file diffs. It encrypts then compresses content before writing, and it stores content hashes for integrity checks. It links execution records to workflow identifiers such as user, case, task, and investigation. Old records are pruned on a configurable schedule and when the database exceeds a size limit. Reads and writes fail closed if the vault is locked.

### Token Store

The token store provides encrypted key-value persistence for Sentinel tokens used by the scrubbing service. It is an adapter over the shared gateway key-value store. Token values are encrypted through the vault before writing and fail closed when the vault is locked. Entries carry time-to-live values and are namespaced to avoid collisions.

### Replay Store

The replay store provides nonce-based replay protection for governance transactions. Nonce reservation is atomic, using the database's uniqueness guarantees. Reserved nonces can be finalized when a transaction completes or released when a transaction fails. In gateway mode the store operates on the canonical `g8e.db` database and is cleaned as part of routine maintenance. In outbound mode it uses a standalone SQLite database and callers run cleanup.

### Suspended Transaction Store

The suspended transaction store persists governance transactions awaiting [L3 approval](./auth.md). It stores the full envelope, approval metadata, and proof material. The store tracks approval status and supports both Ed25519 CLI and passkey WebAuthn proof types. It filters expired transactions, lists pending items, and prunes expired records automatically.

### Commitment Ledger

The commitment ledger stores attestation records with chain-integrity protection. Each new attestation must chain to the latest prior record within a single atomic operation, preventing concurrent forks. It stores the raw attestation alongside structured fields extracted from it. Commitments are treated as permanent audit records and are not pruned.

### History Handler

The history handler is a coordinator that unifies the audit store and the ledger for history requests. It fetches audit events for a session and attaches file mutations for completed edits. File history, point-in-time retrieval, and file restoration are delegated to the ledger. All operations are scoped to an operator session.

### Canonical Gateway Persistence

The canonical gateway database `g8e.db` provides shared persistence primitives for the gateway:

- **Document Store**: JSON documents keyed by collection and identifier, with state-change tracking for Merkle root computation.
- **Key-Value Store**: TTL-bearing key-value storage with pattern scanning, supporting both bound state and observed telemetry.
- **Blob Store**: Binary attachments keyed by namespace, with TTL and observed-state support.
- **SSE Event Buffer**: Per-routing-target event storage for reconnection replay across web sessions, CLI sessions, and user streams.

## Runtime File I/O

The ledger and audit store use the runtime file service for all filesystem operations within the `.g8e/` directory. The service resolves paths relative to the runtime directory, prevents traversal outside it, and enforces consistent file and directory permissions. Other storage services use configured database paths or the shared `g8e.db` connection rather than direct filesystem I/O.

## Retention and Maintenance

Most storage services run a background maintenance task that deletes records older than the configured retention threshold and removes the oldest records when the database exceeds its configured size limit. Pruning reclaims space without requiring a full database lock. The replay store in outbound mode and the commitment ledger are not pruned; commitment records are permanent audit data. The canonical gateway maintenance task also cleans expired KV entries, blobs, nonces, and SSE events.

## Security Properties

1. **Encryption at rest**: The audit store, execution vault, token store, and ledger encrypt sensitive content through the vault.
2. **Fail-closed encryption**: Sensitive services return errors when the vault is locked; there is no plaintext fallback.
3. **Fail-closed replay protection**: Nonce reservation returns an error on any failure, so replay protection is never silently bypassed.
4. **Commitment chain integrity**: The commitment ledger verifies each new record chains to the latest prior record atomically.
5. **Session validation**: Audit events must reference an existing session.
6. **Path confinement**: Ledger and audit file operations are confined to the `.g8e/` runtime directory.
7. **Size limits for encrypted copies**: The ledger caps encrypted file copies to prevent memory exhaustion during encryption.
8. **Atomic nonce reservation**: Nonce reservation is atomic without application-level locking.
9. **Cross-platform path safety**: Path normalization and validation keep file history consistent across platforms.

## Data Flows

### File Mutation Flow

1. The ledger begins a two-phase commit and snapshots the pre-mutation state.
2. The operator writes, deletes, or creates the file on the host filesystem.
3. The ledger completes the commit, copies the post-mutation file, commits to git, and records the post-mutation hash, diff stat, and diff content.
4. The audit store records the event with encrypted content.
5. The audit store records the file mutation linked to the event.
6. The execution vault stores the encrypted and compressed diff.

### Transaction Flow

Each governance transaction passes through the [five-layer interlock](./auth.md): L1 Doctrine, L2 Consensus, L3 Notary, L4 Warden, and L5 Actuator. The storage layer participates as follows:

1. The L4 Warden reserves a nonce through the replay store as the first stage of L4 processing.
2. The execution vault stores the execution result.
3. The audit store records the signed action receipt.
4. The replay store finalizes the nonce.
5. The commitment ledger appends the attestation.

### Approval Flow (L3)

1. The suspended transaction store persists the transaction awaiting human [L3 approval](./auth.md).
2. The CLI lists pending transactions for the user.
3. The user approves with an Ed25519 CLI or passkey WebAuthn proof.
4. The suspended transaction store marks the record as approved.
5. The governance layer executes the approved transaction.
6. The suspended transaction store removes the record after execution.

## Related Documentation

- [Authentication & Authorization](./auth.md): Governance sequence and L3 Interlock
- [Encryption Architecture](./encryption.md): Vault subsystem and mandatory encryption at rest
- [Gateway Architecture](./gateway.md): Canonical database service and gateway-mode service initialization
- [Network Architecture](./network.md): Mutual TLS and identity binding
- [g8e Protocol](../../protocol/docs/spec.md): The wire contract and governance hierarchy
