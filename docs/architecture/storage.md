# Storage Architecture

Last Updated: 2026-08-07
Version: v1.6.6

## Overview

The g8e storage layer is split into discrete services, each responsible for a specific persistence concern. Most services use SQLite as their backing store; the Ledger uses git for file version control, and the History Handler is a stateless coordinator that delegates to the Audit Store and Ledger. Sensitive data is encrypted at rest; see [Encryption Architecture](./encryption.md) for vault and key management.

Services that handle sensitive content require an unlocked vault. The Audit Store, Execution Vault, and Token Store fail closed when the vault is locked, returning errors rather than writing plaintext. The Ledger encrypts file copies when the vault is unlocked and fails closed for file retrieval and restoration. Services that store only public or non-sensitive data (nonces, governance envelopes, attestations) do not require encryption.

In gateway mode, the Audit Store, Token Store, Replay Store, Document Store, Key-Value Store, Blob Store, and SSE Event Buffer operate on the shared gateway database (`g8e.db`), managed by the canonical database service. The canonical database service owns the database connection, vault, and background maintenance, while domain logic is delegated to individual store services. In outbound mode, the Replay Store operates as a standalone SQLite database.

The storage layer consists of the following primary services:

- **Audit Store**: Append-only audit logging for operator sessions, events, file mutations, and signed action receipts
- **Ledger**: Git-backed version control for file modifications, with per-session isolation
- **Execution Vault**: Encrypted and compressed storage for command execution results and file diffs
- **Token Store**: Encrypted key-value persistence for Sentinel tokens, backed by the shared gateway database
- **Replay Store**: Nonce-based replay protection for governance transactions
- **Suspended Transaction Store**: Persistence for transactions awaiting [L3 approval](./auth.md#layer-3-notary-l3notary)
- **Commitment Ledger**: Commitment attestations with chain integrity verification
- **History Handler**: Unified history retrieval combining the Audit Store and Ledger
- **Canonical Gateway Persistence**: Document, key-value, blob, and event stream persistence in the shared database

---

## Services

### Audit Store

The Audit Store is the authoritative append-only record of operator sessions, events, file mutations, and signed action receipts. It tracks system activity and provides the data backbone for audit queries and compliance reporting. The Audit Store operates on the shared gateway database alongside the KV store, document store, and other gateway services.

**Capabilities:**

- Records operator and app sessions, events, file mutations, and signed action receipts
- Encrypts event content, command stdout, and command stderr at rest using AES-256-GCM
- Fails closed when the vault is locked: events are not recorded until the vault is unlocked
- Truncates large command outputs to prevent database bloat
- Validates that events reference an existing session before recording
- Supports atomic batch event insertion within a single transaction
- Stores signed action receipts with idempotent updates keyed by transaction ID
- Prunes old events, file mutations, receipts, and orphaned sessions on a configurable schedule

---

### Ledger

The Ledger provides git-backed version control for all file modifications performed by the operator. Each operator session maintains its own isolated git repository, ensuring that file changes from different sessions do not interfere.

**Two-Phase Commit:**

File operations follow a two-phase commit pattern. The Ledger snapshots the pre-mutation state, the operator performs the filesystem mutation, then the Ledger completes the commit by copying the post-mutation file and committing to git. This produces accurate diff content and statistics for each change. The three supported operations are write, delete, and create.

**Capabilities:**

- Encrypts file copies at rest using AES-256-GCM when the vault is unlocked
- Enforces a size limit on encrypted copies to prevent memory exhaustion during encryption
- Fails closed for file retrieval and restoration when the vault is locked
- Normalizes file paths across platforms for consistent history
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
- Stores execution and file diff records with idempotent updates keyed by stable string IDs
- Supports retrieving file diffs scoped to an operator session

---

### Token Store

The Token Store provides encrypted key-value persistence for Sentinel tokens used by the scrubbing service. It is implemented as an adapter on the shared gateway key-value store, using the same canonical database as the gateway rather than maintaining its own separate database.

**Capabilities:**

- Encrypts all values with AES-256-GCM via the vault before writing to the shared key-value table
- Fails closed on all operations when the vault is locked
- Supports TTL-based expiration for token entries
- Supports prefix-based scanning to retrieve all tokens matching a namespace
- Uses namespaced keys to avoid collisions with other entries in the shared table

---

### Replay Store

The Replay Store provides nonce-based replay protection for governance transactions. Nonce reservation is atomic, detecting duplicates without application-level locking.

In gateway mode, the Replay Store operates on the shared gateway database alongside the Audit Store and Token Store. In outbound mode, the Replay Store uses a standalone SQLite database with its own schema.

**Capabilities:**

- Atomically reserves nonces to detect duplicates without application-level locking
- Supports a nonce lifecycle: reserve, then either finalize (mark as used) or release (delete on transaction failure)
- Fails closed on any error during cleanup or insertion, preventing replay protection from being silently bypassed
- Provides expired nonce cleanup to prevent stale entries from accumulating
- Provides stale reservation cleanup for recovery after crashes (outbound mode)
- Does not require encryption, as nonce data contains no sensitive content
- Does not start a background pruner; callers invoke pruning and cleanup directly

---

### Suspended Transaction Store

The Suspended Transaction Store persists governance transactions that are awaiting [L3 human approval](./auth.md#layer-3-notary-l3notary). It stores the full governance envelope, approval metadata, and cryptographic proof material until the transaction is approved or rejected.

**Capabilities:**

- Stores the complete governance envelope as text for replay after approval
- Supports idempotent updates keyed by transaction hash, allowing re-submission with updated approval metadata
- Tracks approval status, approver identity, cryptographic signature, expected certificate fingerprint, and Ed25519 public key for verification at L3 notary time
- Supports both Ed25519 CLI-based and passkey WebAuthn-based approval proofs
- Filters out expired transactions on retrieval
- Optionally filters pending transactions by user ID
- Retrieves expired transactions for audit before cleanup
- Prunes expired records automatically via a background pruner
- Does not require encryption, as governance envelope data is treated as public audit content

---

### Commitment Ledger

The Commitment Ledger stores commitment attestations as raw JSON with atomic append operations that enforce chain integrity. It is backed by an externally provided SQLite database, with the schema created by the caller before use.

**Capabilities:**

- Enforces chain integrity by verifying that each new attestation's prior hash matches the current latest hash within a single transaction
- Prevents two concurrent attestations from chaining to the same prior hash
- Stores the raw attestation JSON alongside structured fields extracted at insert time
- Tracks all signature digests and the auditor signature as discrete columns
- Does not require encryption, as attestation JSON is treated as public audit data

---

### History Handler

The History Handler is a thin coordinator that combines the Audit Store and Ledger to serve protobuf-encoded history requests. It provides a unified interface for retrieving event history, file history, and performing file restoration.

**Capabilities:**

- Fetches audit events for a session and attaches file mutations for completed file edits
- Delegates file history queries to the Ledger's git log
- Delegates point-in-time file retrieval and file restoration to the Ledger
- Scopes all operations to an operator session ID
- Uses dependency injection for the Audit Store and Ledger, enabling unit testing with mocks

---

### Canonical Gateway Persistence

The canonical gateway database (`g8e.db`) provides unified storage primitives used by higher-level gateway services:

- **Document Store**: Stores JSON documents keyed by collection and identifier, tracking state versions for Merkle root computation
- **Key-Value Store**: Provides TTL-bearing key-value persistence with cache invalidation triggers
- **Blob Store**: Manages raw binary attachments with namespace isolation and TTL
- **SSE Event Buffer**: Maintains a per-routing-target buffer enabling reconnection replay across web sessions, CLI sessions, and user streams

---

## Runtime File I/O Abstraction

All storage services that interact with the filesystem do so through a shared file I/O abstraction scoped to the `.g8e/` runtime directory. This abstraction provides atomic file writes, path resolution that confines operations within the runtime directory, and permission enforcement for directories and files. The Ledger and Audit Store both rely on this abstraction for all filesystem operations, ensuring consistent path handling and preventing writes outside the runtime directory.

---

## Retention and Pruning

Most services run a background pruner that periodically deletes records older than the configured retention threshold and removes the oldest rows when the database exceeds the configured size limit. Pruning reclaims space without a full database lock.

Two services do not start a background pruner:
- **Replay Store**: Callers invoke pruning and stale reservation cleanup directly.
- **Commitment Ledger**: Attestations are permanent audit records and are not pruned.

---

## Cross-Platform Path Handling

All storage services construct filesystem paths through a shared path utility layer that normalizes separators and prevents invalid path joins on Windows. Configuration paths for databases and directories can be either relative or absolute. Relative paths are resolved against a base data directory; absolute paths are respected and used without modification, allowing operators to place individual databases on separate volumes or drives. The Ledger normalizes paths for consistent file history across platforms.

---

## Security Properties

1. **Encryption at rest**: Sensitive fields are encrypted using AES-256-GCM. The Audit Store, Execution Vault, and Token Store use fail-closed semantics, returning errors when the vault is locked.
2. **Fail-closed replay protection**: Nonce reservation returns an error on any failure, preventing replay protection from being silently bypassed.
3. **Commitment chain integrity**: The Commitment Ledger verifies the prior commitment hash within a transaction to prevent chain forks under concurrent writes.
4. **Session validation**: Audit events must reference an existing session.
5. **Path traversal protection**: The Ledger normalizes paths before constructing ledger-relative paths.
6. **Cross-platform path safety**: The shared path utility layer prevents invalid path joins on Windows and normalizes separators across platforms.
7. **Size limits for encrypted copies**: The Ledger enforces a cap on encrypted file copies to prevent memory exhaustion during encryption.
8. **Atomic nonce reservation**: Nonce reservation is atomic without application-level locking.
9. **Runtime directory confinement**: The shared file I/O abstraction resolves all paths relative to the `.g8e/` runtime directory, preventing writes outside the intended scope.

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

The governance layer processes each transaction through the [five-layer interlock](./auth.md): L1 Doctrine (technical safety), L2 Consensus (multi-agent approval), L3 Notary (human authorization), L4 Warden (final verification), and L5 Actuator (execution). The storage layer participates as follows:

1. The L4 Warden reserves the nonce via the Replay Store as stage 1 of L4 processing.
2. The Execution Vault stores the execution result.
3. The Audit Store records the signed action receipt.
4. The Replay Store finalizes the nonce.
5. The Commitment Ledger appends the attestation.

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
- [**Gateway Architecture**](./gateway.md): CanonicalDBService and gateway-mode service initialization
- [**Network Architecture**](./network.md): Mutual TLS and identity binding
- [**g8e Protocol**](../../protocol/docs/spec.md): The wire contract and governance hierarchy
