# Storage Architecture

Last Updated: 2026-07-12
Version: v1.4.0

## Overview

The g8e storage layer is split into discrete services, each responsible for a specific persistence concern. All services use SQLite as their backing store, with mandatory [encryption at rest](./encryption.md) enforced for sensitive data.

Services that handle sensitive content require an unlocked vault and operate with fail-closed semantics: they return errors rather than writing plaintext when the vault is locked. Services that store only public or non-sensitive data (nonces, governance envelopes, attestations) do not require encryption.

The storage layer consists of the following services:

- **Audit Store**: Append-only audit logging for operator sessions, events, file mutations, and signed action receipts
- **Ledger**: Git-backed version control for file modifications, with per-session isolation
- **Execution Vault**: Encrypted and compressed storage for command execution results and file diffs
- **Token Store**: Encrypted key-value persistence for Sentinel tokens, backed by the shared gateway database
- **Replay Store**: Nonce-based replay protection for governance transactions
- **Suspended Transaction Store**: Persistence for transactions awaiting [L3 approval](./auth.md#layer-3-notary-l3notary)
- **Commitment Ledger**: Commitment attestations with chain integrity verification
- **History Handler**: Unified history retrieval combining the audit store and ledger

---

## Services

### Audit Store

The Audit Store is the authoritative append-only record of operator sessions, events, file mutations, and signed action receipts. It tracks all system activity and provides the data backbone for audit queries and compliance reporting.

**Capabilities:**

- Records operator and app sessions, events, file mutations, and signed action receipts
- Encrypts event content, command stdout, and command stderr at rest using AES-256-GCM
- Fails closed when the vault is locked: events are not recorded until the vault is unlocked
- Truncates large command outputs using a head/tail strategy to prevent database bloat
- Validates that events reference an existing session before recording
- Supports atomic batch event insertion within a single transaction
- Stores signed action receipts with upsert semantics on transaction ID
- Prunes old events, file mutations, receipts, and orphaned sessions on a configurable schedule

---

### Ledger

The Ledger provides git-backed version control for all file modifications performed by the operator. Each operator session maintains its own isolated git repository, ensuring that file changes from different sessions do not interfere. The go-git library is used directly; no external git binary is invoked.

**Two-Phase Commit:**

File operations follow a two-phase commit pattern. The ledger snapshots the pre-mutation state, the operator performs the filesystem mutation, then the ledger completes the commit by copying the post-mutation file and committing to git. This produces accurate diff content and statistics for each change. The three supported operations are write, delete, and create.

**Capabilities:**

- Encrypts file copies at rest using AES-256-GCM when the vault is unlocked
- Enforces a 100 MB size limit on encrypted copies to prevent memory exhaustion during encryption
- Streams unencrypted file copies to avoid loading entire files into memory
- Fails closed for file retrieval and restoration when the vault is locked
- Normalizes file paths across platforms by stripping Windows drive letters and converting backslashes
- Exposes the HEAD commit hash of the global files ledger as a BFT-verifiable state snapshot
- Supports file history queries, point-in-time retrieval, and restoration from prior commits

---

### Execution Vault

The Execution Vault stores command execution results and file diffs. All content is encrypted with AES-256-GCM and then compressed before being written to SQLite. The vault must be unlocked for any read or write; the service is fail-closed with no plaintext fallback.

**Capabilities:**

- Encrypts then compresses all content before storage
- Stores content hashes alongside compressed blobs for integrity verification
- Links execution records to workflow metadata such as case, task, and investigation IDs
- Prunes records older than the retention threshold and removes the oldest rows when the database exceeds the configured size limit
- Uses stable string IDs as primary keys for both execution and file diff records

---

### Token Store

The Token Store provides encrypted key-value persistence for Sentinel tokens used by the scrubbing service. It is implemented as an adapter on the shared gateway key-value store, using the same canonical database as the gateway rather than maintaining its own separate database.

**Capabilities:**

- Encrypts all values with AES-256-GCM via the vault before writing to the shared key-value table
- Fails closed on all operations when the vault is locked
- Supports TTL-based expiration for token entries
- Supports prefix-based scanning to retrieve all tokens matching a namespace
- Sentinel keys are namespaced with a dedicated prefix to avoid collisions with cache and document invalidation entries in the same table

---

### Replay Store

The Replay Store provides nonce-based replay protection for governance transactions. It relies on SQLite's UNIQUE constraint on the nonce column for atomic replay detection, avoiding race conditions from separate read-then-write patterns.

**Capabilities:**

- Atomically reserves nonces using a UNIQUE constraint to detect duplicates without application-level locking
- Supports a nonce lifecycle: reserve, then either finalize (mark as used) or release (delete on transaction failure)
- Fails closed on any SQLite error during cleanup or insertion, preventing replay protection from being silently bypassed
- Cleans up expired nonces automatically on each reservation
- Provides stale reservation cleanup for recovery after crashes
- Does not require encryption, as nonce data contains no sensitive content
- Does not start a background pruner; callers invoke pruning and cleanup directly

---

### Suspended Transaction Store

The Suspended Transaction Store persists governance transactions that are awaiting [L3 human approval](./auth.md#layer-3-notary-l3notary). It stores the full governance envelope, approval metadata, and cryptographic proof material until the transaction is approved or rejected.

**Capabilities:**

- Stores the complete governance envelope as text for replay after approval
- Tracks approval status, approver identity, cryptographic signature, expected certificate fingerprint, and Ed25519 public key for verification at L3 notary time
- Supports both Ed25519 CLI-based and passkey WebAuthn-based approval proofs
- Filters out expired transactions on retrieval
- Optionally filters pending transactions by user ID
- Prunes expired records automatically via a background pruner
- Does not require encryption, as governance envelope data is treated as public audit content

---

### Commitment Ledger

The Commitment Ledger stores commitment attestations as raw JSON with atomic append operations that enforce chain integrity. It is backed by an externally provided SQLite database, with the schema created by the caller before use.

**Capabilities:**

- Enforces chain integrity by verifying that each new attestation's prior hash matches the current latest hash, all within a single transaction
- Prevents two concurrent attestations from chaining to the same prior hash via transactional check-then-insert
- Stores the raw attestation JSON alongside structured fields extracted at insert time
- Tracks all signature digests and the auditor signature as discrete columns
- Does not require encryption, as attestation JSON is treated as public audit data

---

### History Handler

The History Handler is a thin coordinator that combines the Audit Store and Ledger to serve protobuf-encoded history requests. It provides a unified interface for retrieving event history, file history, and performing file restoration.

**Capabilities:**

- Fetches audit events for a session and attaches file mutations for completed file edits
- Delegates file history queries to the ledger's git log
- Delegates file restoration to the ledger's point-in-time retrieval
- Scopes all operations to an operator session ID
- Uses dependency injection for the audit store and ledger, enabling unit testing with mocks

---

## Encryption and Vault Behavior

Services that handle sensitive content use AES-256-GCM encryption via the [vault](./encryption.md). Following v1.0.10, encryption at rest is mandatory for sensitive data storage.

Behavior when the vault is locked varies by service:

- **Audit Store**: Fails closed. Events are not recorded until the vault is unlocked.
- **Execution Vault**: Fails closed. No plaintext fallback for reads or writes.
- **Token Store**: Fails closed on all operations. No plaintext fallback.
- **Ledger**: File retrieval and restoration fail closed. New file copies are encrypted when the vault is unlocked.
- **Replay Store**: No encryption required. Nonces contain no sensitive content.
- **Suspended Transaction Store**: No encryption required. Governance envelopes are public audit data.
- **Commitment Ledger**: No encryption required. Attestations are public audit data.

## Retention and Pruning

Most services run a background pruner that periodically deletes records older than the configured retention threshold and removes the oldest rows when the database exceeds the configured size limit. Pruning also runs incremental vacuum to reclaim space without a full database lock.

The Replay Store is the exception: it does not start a background pruner. Callers invoke pruning and stale reservation cleanup directly.

---

## Cross-Platform Path Handling

All storage services construct filesystem paths through a shared path utility layer that prevents a Windows-specific double-join issue: when two absolute paths are joined with standard library functions, the result is an invalid concatenated path (for example, `C:\temp\C:\temp\data.db`). The utility layer detects absolute paths in the joined elements and uses them as-is instead of concatenating.

Configuration paths for databases and directories can be either relative or absolute. Relative paths are resolved against a base data directory. Absolute paths are respected and used without modification, allowing operators to place individual databases on separate volumes or drives.

Paths are normalized for platform-appropriate separators on Windows, converting forward slashes to backslashes and removing redundant separators. The Ledger additionally strips Windows drive letters and leading separators before constructing ledger-relative paths, ensuring that file history is consistent across platforms.

---

## Security Properties

1. **Encryption at rest**: Sensitive fields are encrypted using AES-256-GCM. The Audit Store, Execution Vault, and Token Store all use fail-closed semantics, returning errors when the vault is locked.
2. **Fail-closed replay protection**: Nonce reservation returns an error on any SQLite failure, preventing replay protection from being silently bypassed.
3. **Commitment chain integrity**: The Commitment Ledger verifies the prior commitment hash inside a transaction to prevent chain forks under concurrent writes.
4. **Session validation**: Audit events must reference an existing session row. Foreign key constraints are enforced at the schema level.
5. **Path traversal protection**: The Ledger strips drive letters and leading separators before constructing ledger-relative paths.
6. **Cross-platform path safety**: The shared path utility layer prevents double-joining of absolute paths on Windows and normalizes separators across platforms. See [Cross-Platform Path Handling](#cross-platform-path-handling).
7. **Size limits for encrypted copies**: The Ledger enforces a 100 MB cap on encrypted file copies to prevent OOM during the full-read required by AES-GCM.
8. **Streaming for unencrypted copies**: The Ledger streams unencrypted file copies to prevent OOM.
9. **Atomic nonce reservation**: The UNIQUE constraint on the nonce column provides atomicity without application-level locking.

---

## Data Flow

### File Mutation Flow

1. The Ledger begins a two-phase commit, snapshots pre-mutation state, and returns the hash of the pre-mutation commit.
2. The operator performs the file write, delete, or create on the host filesystem.
3. The Ledger completes the commit, copies the post-mutation file, commits to git, and records the post-mutation hash, diff stat, and diff content.
4. The Audit Store records the event with encrypted content.
5. The Audit Store records the file mutation linked to the event.
6. The Execution Vault stores the compressed and encrypted diff.

### Transaction Flow

1. The Replay Store atomically reserves the nonce.
2. The governance layer executes the transaction through the [five-layer interlock](./auth.md).
3. The Execution Vault stores the execution result.
4. The Audit Store records the signed action receipt.
5. The Replay Store finalizes the nonce.
6. The Commitment Ledger appends the attestation.

### Approval Flow (L3)

1. The Suspended Transaction Store persists the transaction awaiting human [L3 approval](./auth.md#layer-3-notary-l3notary).
2. The CLI lists pending transactions for the user.
3. The user approves with a cryptographic signature (Ed25519 CLI or passkey WebAuthn).
4. The Suspended Transaction Store marks the record as approved.
5. The governance layer executes the approved transaction.
6. The Suspended Transaction Store deletes the record after execution.

---

## Related Documentation

- [**Authentication & Authorization**](./auth.md): Governance sequence and L3 Interlock
- [**Encryption Architecture**](./encryption.md): Vault subsystem and mandatory encryption at rest
- [**Network Architecture**](./network.md): Mutual TLS and identity binding
- [**g8e Protocol**](../../protocol/docs/spec.md): The wire contract and governance hierarchy

