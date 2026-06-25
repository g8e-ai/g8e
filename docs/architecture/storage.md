# Storage Architecture

Last Updated: 2026-06-25
Version: v1.2.1

## Overview

The g8e storage layer is split into discrete services, each responsible for a specific persistence concern. Following the v1.0.10 refactor, the monolithic `AuditVaultService` is consolidated into `SQLAuditStore`, `GitLedgerService`, and `HistoryHandler`. All services reside under `internal/services/storage/`.

Mandatory [encryption at rest](./encryption.md) is enforced across the storage layer. Services require an unlocked vault for secure operations, with specific fail-open or fail-closed behaviors depending on the criticality of data continuity versus confidentiality.

- **Audit Store**: Append-only audit logging with encryption at rest
- **Ledger**: git-backed version control for file modifications, session-isolated
- **Execution Vault**: Compressed and encrypted storage for command execution results and file diffs
- **Token Store**: Interface for encrypted Sentinel token persistence, implemented by `gateway.EncryptedKVAdapter` on the shared gateway KV store
- **Replay Store**: Nonce-based replay protection
- **Suspended Transaction Store**: [L3 approval workflow](./auth.md#layer-3-notary-l3notary) transaction persistence
- **Commitment Ledger**: Commitment attestations with chain integrity verification
- **History Handler**: Unified history retrieval combining audit store and ledger

Shared implementation patterns across the services:

- SQLite-based with `sqliteutil.DB` wrapper providing retry logic, transactions, and incremental vacuum
- Mandatory encryption at rest via `vault.Vault` (AES-256-GCM) where applicable
- Background cleanup via `sqliteutil.Pruner` for most services
- Thread-safe operations via `sync.WaitGroup` and `sync.Once`
- Standardized retention and database size pruning policies

---

## Core Services

### Audit Store (`internal/services/storage/audit_store.go`)

`SQLAuditStore` provides append-only audit logging for all system events. It is the authoritative record of operator sessions, events, file mutations, and signed action receipts.

**Schema Tables:**

- `sessions`: Operator and app session metadata (`id`, `title`, `session_type`, `created_at`, `user_identity`)
- `events`: Audit events with optionally encrypted content (`content_text`, `command_stdout`, `command_stderr`, `encrypted` flag)
- `file_mutation_log`: File operation records linked to events
- `receipts`: Signed action receipts for transaction-native audit records

**Key Features:**

- **Encryption at rest**: `content_text`, `command_stdout`, and `command_stderr` are encrypted using the vault when it is unlocked. If the vault is locked, content is stored as plaintext to ensure audit records are never blocked by vault state (fail-open for audit continuity). The `encrypted` column tracks the encryption status of each event.
- **Mandatory Vault**: `NewSQLAuditStore` requires an `EncryptionVault` in its configuration and returns an error if it is missing.
- **Output truncation**: Large outputs are truncated using a head/tail strategy to prevent database bloat; thresholds are configurable.
- **Session validation**: Events must reference a pre-existing session row. App sessions are auto-created on first write to satisfy the foreign key constraint.
- **Batch recording**: `RecordEvents` performs atomic batch inserts within a single transaction.
- **Retention pruning**: Background `sqliteutil.Pruner` deletes old events, file mutations, and receipts, then removes orphaned sessions.
- **Action receipts**: Signed `ActionReceiptRecord` entries stored with upsert semantics on `transaction_id`.

**Configuration:**

```go
type AuditStoreConfig struct {
    DataDir                   string
    DBPath                    string
    MaxDBSizeMB               int64
    RetentionDays             int
    PruneIntervalMinutes      int
    OutputTruncationThreshold int
    HeadTailSize              int
    EncryptionVault           *vault.Vault // required
}
```

`EncryptionVault` is required; `NewSQLAuditStore` returns an error if it is nil.

Default values (`DefaultAuditStoreConfig`): `DataDir: paths.Infra.DataDir` (resolves to `{baseDir}/.g8e/data`), `DBPath: constants.DbFilename` (`"g8e.db"`, resolved to `{DataDir}/g8e.db` via `pathutil.ResolveDBPath`), `MaxDBSizeMB: 2048`, `RetentionDays: 90`, `PruneIntervalMinutes: 60`, `OutputTruncationThreshold: 102400`, `HeadTailSize: 51200`.

**Key Methods:**

- `CreateSession(id, sessionType, title, userIdentity)`: Create a new session row.
- `GetOperatorSession(id)`: Retrieve a session by ID.
- `RecordEvent(event)`: Record a single event; returns the new event ID.
- `RecordEvents(events)`: Batch-record multiple events atomically.
- `RecordActionReceipt(record)`: Upsert a signed action receipt.
- `GetEvents(sessionID, limit, offset)`: Retrieve events for a session with decryption applied.
- `RecordFileMutation(mutation)`: Record a file mutation linked to an event.
- `GetFileMutations(eventID)`: Retrieve file mutations for an event.
- `GetActionReceipt(transactionID)`: Retrieve a single action receipt.
- `ListActionReceipts(sessionID, limit, offset)`: List action receipts with optional session filter and pagination.
- `ListActionReceiptsSince(since, limit)`: List action receipts newer than a timestamp.
- `ListSessions(limit, offset)`: Retrieve all sessions with pagination, ordered by `created_at` ASC.
- `ListEvents(sessionID, limit, offset)`: Retrieve events with optional session filter and pagination, ordered by timestamp ASC.
- `ListFileMutations(limit, offset)`: Retrieve file mutations with pagination, ordered by `id` ASC.
- `GetDataDir()`: Return the configured data directory.
- `GetEncryptionVault()`: Return the encryption vault instance.
- `Wait()`: Block until all in-flight writes complete.
- `Close()`: Stop the pruner and close the database. Idempotent.

---

### Ledger (`internal/services/storage/ledger.go`)

`GitLedgerService` provides git-backed version control for file modifications. Each operator session has its own isolated git repository. The go-git library is used directly; no git binary is invoked.

**Key Features:**

- **Session isolation**: Each operator session maintains its own git repository under `{BaseDir}/sessions/{sessionID}/`. An empty session ID falls back to `{BaseDir}/files`.
- **Two-phase commit**: File operations snapshot state before and after the mutation to produce diff content and statistics. Diff generation is performed using the `go-git` Patch API.
- **Encryption at rest**: When the vault is unlocked, file copies are encrypted using AES-256-GCM and stored with an `.enc` extension. The service enforces a 100 MB size limit on encrypted copies to prevent memory exhaustion during the full-read required by the encryption cipher.
- **Streaming**: Unencrypted files are streamed to the ledger using `io.Copy` to prevent memory exhaustion.
- **Fail-closed Retrieval**: `GetFileAtCommit` and `RestoreFileFromCommit` require an unlocked vault and return an error if the vault is locked.
- **Path normalization**: `normalizeToGitPath` removes Windows drive letters and converts backslashes so paths are consistent across platforms.
- **State merkle root**: `GetStateMerkleRoot` returns the HEAD commit hash of the global `files` ledger as a BFT-verifiable snapshot.

**Configuration:**

```go
type LedgerConfig struct {
    BaseDir         string       // base directory for all session ledgers
    GitPath         string       // non-empty string enables git operations
    EncryptionVault *vault.Vault // required
}
```

`EncryptionVault` is required; `NewGitLedgerService` returns an error if it is nil. Git operations are only performed when `GitPath` is non-empty (`gitReady()` check).

**Two-Phase Commit Pattern:**

```go
// Write
result, err := ledger.LedgerFileWrite(sessionID, filePath)
// ... perform host file write ...
err = ledger.CompleteMirrorWrite(result, sessionID)

// Delete
result, err := ledger.MirrorFileDelete(sessionID, filePath)
// ... perform host file deletion ...
err = ledger.CompleteMirrorDelete(result, sessionID)

// Create
result, err := ledger.MirrorFileCreate(sessionID, filePath)
// ... perform host file creation ...
err = ledger.CompleteMirrorCreate(result, sessionID)
```

**Key Methods:**

- `GetSessionLedgerPath(sessionID)`: Return or initialize the session-specific git repository path.
- `LedgerFileWrite(sessionID, filePath)`: Begin two-phase write; snapshots pre-mutation state.
- `CompleteMirrorWrite(result, sessionID)`: Copy post-mutation file into ledger and commit.
- `MirrorFileDelete(sessionID, filePath)`: Begin two-phase delete; snapshots pre-deletion state.
- `CompleteMirrorDelete(result, sessionID)`: Remove mirror file from ledger and commit.
- `MirrorFileCreate(sessionID, filePath)`: Begin two-phase create; snapshots pre-creation state.
- `CompleteMirrorCreate(result, sessionID)`: Copy created file into ledger and commit.
- `GetFileHistory(filePath, limit, sessionID)`: Return git commit history entries for a file.
- `GetFileAtCommit(filePath, commitHash, sessionID)`: Retrieve and decrypt file content at a specific commit. Requires unlocked vault.
- `RestoreFileFromCommit(filePath, commitHash, sessionID)`: Decrypt and write a file back to its on-disk path from a prior commit.
- `GetStateMerkleRoot()`: Return the HEAD commit hash of the global files ledger.
- `GetDiffContent(hashBefore, hashAfter, sessionID)`: Return the full patch string between two commits.
- `GetDiffStat(hashBefore, hashAfter, sessionID)`: Return a summary stat string between two commits.
- `ListCommits(sessionID, limit)`: List commits from the session ledger (or global files ledger if sessionID is empty), ordered oldest-first.

---

### Execution Vault (`internal/services/storage/execution_vault.go`)

`ExecutionVaultService` stores command execution results and file diffs. All content is encrypted then compressed before being written to SQLite. The vault must be unlocked for any write or read; the service is fail-closed.

**Schema Tables:**

- `execution_log`: Command execution records (`id`, `timestamp_utc`, `command`, `exit_code`, `duration_ms`, `stdout_compressed`, `stderr_compressed`, `stdout_hash`, `stderr_hash`, `stdout_size`, `stderr_size`, `user_id`, `case_id`, `task_id`, `investigation_id`, `operator_id`)
- `file_diff_log`: File diff records (`id`, `timestamp_utc`, `file_path`, `operation`, `ledger_hash_before`, `ledger_hash_after`, `diff_stat`, `diff_compressed`, `diff_hash`, `diff_size`, `operator_session_id`, `user_id`, `case_id`, `operator_id`)

**Key Features:**

- **Encryption then compression**: Content is encrypted with the vault (AES-256-GCM), then compressed via `sqliteutil.Compress` before storage.
- **Hash verification**: Content hashes are stored alongside compressed blobs for integrity checks.
- **Case/task/investigation linking**: Execution records carry workflow metadata fields.
- **Retention pruning**: Background `sqliteutil.Pruner` deletes records older than the retention threshold and prunes the oldest 10% of rows when the database exceeds the size limit.
- **Fail-closed**: The service is fail-closed; `encryptContent` returns an error if the vault is locked, with no plaintext fallback.
- **ID Persistence**: Both `execution_log` and `file_diff_log` use stable string IDs as primary keys.

**Configuration:**

```go
type ExecutionVaultConfig struct {
    DBPath               string
    MaxDBSizeMB          int64
    RetentionDays        int
    PruneIntervalMinutes int
}
```

`vault.Vault` is passed as a constructor argument and is required. `NewExecutionVaultService` returns an error if it is nil.

Default values (`DefaultExecutionVaultConfig`): `DBPath: constants.ExecutionVaultDBPath` (`".g8e/execution_vault.db"`; overridden at runtime to `paths.Infra.ExecutionVaultDBPath`, which resolves to `{baseDir}/.g8e/data/execution_vault.db`), `MaxDBSizeMB: 1024`, `RetentionDays: 30`, `PruneIntervalMinutes: 60`.

**Key Methods:**

- `StoreExecution(ctx, record)`: Encrypt and compress execution output, then insert into `execution_log`.
- `GetExecution(ctx, executionID)`: Retrieve, decompress, and decrypt an execution record.
- `StoreFileDiff(ctx, record)`: Encrypt and compress diff content, then insert into `file_diff_log`.
- `GetFileDiff(ctx, diffID)`: Retrieve, decompress, and decrypt a file diff record.
- `GetFileDiffsBySession(ctx, sessionID, limit)`: Retrieve all diff metadata (without compressed blob) for a session.
- `ListExecutions(ctx, limit, offset)`: Retrieve execution log records with pagination (no blobs), ordered by `timestamp_utc` ASC.
- `ListFileDiffs(ctx, limit, offset)`: Retrieve file diff log records with pagination (no blobs), ordered by `timestamp_utc` ASC.
- `Wait()`: Block until all in-flight writes complete.
- `Close()`: Stop the pruner and close the database.

`ExecutionVaultService` implements the `storage.ExecutionVault` interface.

---

### Token Store (`internal/services/storage/token_store.go`)

`TokenStore` is an interface defining encrypted key-value persistence for Sentinel tokens. The interface is consumed by `ScrubbingService` and implemented by `gateway.EncryptedKVAdapter` (`internal/services/gateway/encrypted_kv_adapter.go`).

**Interface Definition:**

```go
type TokenStore interface {
    KVSet(ctx context.Context, key, value string, ttlSeconds int) error
    KVGet(ctx context.Context, key string) (string, error)
    KVScanPrefix(ctx context.Context, prefix string) (map[string]string, error)
}
```

**Canonical Implementation: `EncryptedKVAdapter`**

`EncryptedKVAdapter` bridges `KVStoreService` (in the canonical gateway database, `g8e.db`) to the `TokenStore` interface. Values are encrypted at rest via `vault.Vault`. Entries are written with `state_tier='observed'` so they do not participate in the bound state root hash. Sentinel keys are namespaced with the `g8e:sentinel:` prefix to avoid collisions with cache and document invalidation entries in the same `kv_store` table.

**Key Features:**

- **Encryption at rest**: All values are encrypted with AES-256-GCM via the vault before being written to `kv_store`. `KVSet` returns an error if the vault is locked. `KVGet` returns an error if the vault is locked. `KVScanPrefix` silently skips entries that cannot be decrypted.
- **TTL support**: TTL is delegated to `KVStoreService.KVSetObserved`, which stores the expiration timestamp.
- **Prefix scanning**: `KVScanPrefix` retrieves all keys matching the prefix, decrypting each value. Decryption failures are silently skipped.
- **No standalone database**: The adapter uses the shared `CanonicalDBService` database connection; no separate SQLite database or background pruner is required.

**Constructor:**

```go
func NewEncryptedKVAdapter(kv *KVStoreService, v *vault.Vault) *EncryptedKVAdapter
```

`EncryptedKVAdapter` implements `storage.TokenStore`.

---

### Replay Store (`internal/services/storage/replay_store.go`)

`SQLReplayStore` provides nonce-based replay protection. It uses SQLite's UNIQUE constraint on the `nonce` column for atomic replay detection.

**Schema Tables:**

- `nonce_usage`: Nonce lifecycle tracking (`nonce`, `reserved_at`, `used_at`, `expires_at`, `status`)

**Key Features:**

- **Atomic replay detection**: The `INSERT` on `nonce_usage` relies on the UNIQUE constraint to detect duplicates without a separate read-then-write race.
- **Nonce lifecycle**: Reserve, then either Finalize (mark as `used`) or Release (delete reservation on transaction failure).
- **Fail-closed design**: `ReserveNonce` fails closed on any SQLite error during cleanup or insertion.
- **Automatic expiry cleanup**: `cleanupExpiredNonces` runs on each `ReserveNonce` call.
- **Stale reservation cleanup**: `CleanupStaleReserved` removes reservations older than a configurable duration that were never finalized, for example after a crash.
- **No encryption required**: Nonce data does not contain sensitive content.

**Configuration:**

```go
type ReplayStoreConfig struct {
    DBPath string
}
```

Default value (`DefaultReplayStoreConfig`): `DBPath: constants.ReplayStoreDBPath` (`".g8e/replay_store.db"`; overridden at runtime to `paths.Infra.ReplayStoreDBPath`, which resolves to `{baseDir}/.g8e/data/replay_store.db`).

No background pruner is started; callers invoke `Prune` and `CleanupStaleReserved` directly.

**Key Methods:**

- `ReserveNonce(nonce, expiresAt)`: Atomically reserve a nonce. Returns `true` if replay is detected.
- `FinalizeNonce(nonce)`: Transition a reserved nonce to `used` status.
- `ReleaseNonce(nonce)`: Delete a reserved nonce on transaction failure.
- `ListNonces(limit, offset)`: Retrieve nonce records with pagination, ordered by `reserved_at` ASC.
- `CleanupStaleReserved(maxReservedDuration)`: Remove stale reservations older than the given duration.
- `Prune(retentionDays)`: Delete used nonce records older than the retention period.
- `Close()`: Close the database connection.

---

### Suspended Transaction Store (`internal/services/storage/suspended_transaction_store.go`)

`SuspendedTransactionService` stores governance transactions awaiting L3 human approval.

**Schema Tables:**

- `suspended_transactions`: Approval workflow records (`transaction_hash`, `envelope`, `created_at`, `expires_at`, `tool_name`, `tool_arguments`, `user_id`, `operator_id`, `approved`, `approved_at`, `approved_by`, `approval_signature`, `expected_cert_fingerprint`, `approval_public_key`)

**Key Features:**

- **Envelope storage**: The full governance envelope is stored as text for replay after approval.
- **Approval tracking**: Records approval status, approver identity, cryptographic signature, expected certificate fingerprint, and Ed25519 public key for cryptographic verification at L3 notary verification time.
- **Expiration**: Transactions carry an `expires_at` timestamp; `GetSuspendedTransaction` and `ListSuspendedTransactions` filter out expired records.
- **User filtering**: `ListSuspendedTransactions` optionally filters by `user_id`.
- **Automatic cleanup**: The background pruner and `CleanupExpiredSuspendedTransactions` remove expired records.
- **No encryption required**: Governance envelope data is not encrypted at rest.

**Configuration:**

```go
type SuspendedTransactionConfig struct {
    DBPath               string
    MaxDBSizeMB          int64
    RetentionDays        int
    PruneIntervalMinutes int
}
```

Default values (`DefaultSuspendedTransactionConfig`): `DBPath: constants.SuspendedTransactionDBPath` (`".g8e/suspended_transactions.db"`; overridden at runtime to `paths.Infra.SuspendedTransactionsDBPath`, which resolves to `{baseDir}/.g8e/data/suspended_transactions.db`), `MaxDBSizeMB: 256`, `RetentionDays: 7`, `PruneIntervalMinutes: 30`.

**Key Methods:**

- `StoreSuspendedTransaction(ctx, tx)`: Upsert a transaction awaiting approval.
- `GetSuspendedTransaction(ctx, txHash)`: Retrieve a transaction by hash. Returns `(nil, false, nil)` if not found or expired.
- `ListSuspendedTransactions(ctx, userID)`: List non-expired transactions, optionally filtered by user.
- `ApproveSuspendedTransaction(ctx, txHash, approvedBy, approvalSignature, certFingerprint, approvalPublicKey)`: Mark a transaction as approved with cryptographic signature and Ed25519 public key.
- `DeleteSuspendedTransaction(ctx, txHash)`: Remove a transaction after approval or rejection.
- `CleanupExpiredSuspendedTransactions(ctx)`: Delete expired records; returns the count deleted and any error.
- `GetExpiredSuspendedTransactions(ctx)`: Retrieve expired transactions for audit purposes.
- `Wait()`: Block until all in-flight writes complete.
- `Close()`: Stop the pruner and close the database.

`SuspendedTransactionService` implements the `storage.SuspendedTransactionStore` interface.

---

### Commitment Ledger (`internal/services/storage/commitment_ledger.go`)

`CommitmentLedger` stores commitment attestations as raw JSON with atomic append operations that enforce chain integrity. It is constructed with an externally provided `sqliteutil.DB`; the `commitment_ledger` table must be created by the caller before use.

**Schema Table (`commitment_ledger`):**

Columns: `id` (auto-increment primary key, mapped to `CommitmentRow.Seq`), `transaction_id`, `transaction_hash`, `prior_commitment_hash`, `state_root_at_commit`, `l2_signature_digest`, `Actuator_intent_signature_digest`, `human_signature_digest`, `action_type`, `target_resource`, `committed_at_unix_ms`, `auditor_key_id`, `signature`, `hash`, `attestation_json`.

**Key Features:**

- **Chain integrity**: `AppendCommitmentJSON` runs inside a transaction, reads the current latest `hash`, and verifies it matches the supplied `priorHash` before inserting.
- **Atomic append**: The transactional check-then-insert prevents two concurrent attestations from chaining to the same `prior_hash`.
- **JSON storage**: The raw attestation JSON is stored in `attestation_json`; structured fields are extracted into individual columns at insert time.
- **Signature tracking**: All signature digests and the auditor signature are stored as discrete columns.
- **No encryption required**: Attestation JSON is treated as public audit data.

**Constructor:**

```go
func NewCommitmentLedger(db *sqliteutil.DB, logger *slog.Logger) *CommitmentLedger
```

**Key Methods:**

- `GetLatestCommitmentJSON()`: Return the most recent attestation as raw JSON, ordered by `committed_at_unix_ms`. Returns `(nil, nil)` when the ledger is empty.
- `ListCommitments()`: Retrieve all commitments ordered by `committed_at_unix_ms` ASC (chain order), returning structured `CommitmentRow` records.
- `AppendCommitmentJSON(attestationJSON, priorHash, hash)`: Atomically append a new commitment with chain verification.

---

### History Handler (`internal/services/storage/history_handler.go`)

`HistoryHandler` is a thin coordinator that combines `SQLAuditStore` and `GitLedgerService` to serve protobuf-encoded history requests.

**Key Features:**

- **Protobuf interface**: All request and response types are protobuf messages from `protocol/proto/g8e/operator/v1`.
- **Unified event history**: `HandleFetchHistory` fetches audit events and attaches file mutations for `FileEdit.Completed` events.
- **File history**: `HandleFetchFileHistory` delegates to the ledger's `GetFileHistory`.
- **File restoration**: `HandleRestoreFile` delegates to the ledger's `RestoreFileFromCommit`.
- **Session context**: All operations are scoped to an `operator_session_id`.

**Constructor:**

```go
func NewHistoryHandler(auditStore auditStoreInterface, ledger ledgerInterface, logger loggerInterface) *HistoryHandler
```

The constructor accepts interface types for dependency injection and unit testing. The `auditStoreInterface` requires `GetOperatorSession`, `GetEvents`, and `GetFileMutations`. The `ledgerInterface` requires `GetFileHistory`, `RestoreFileFromCommit`, `GetFileAtCommit`, and two-phase commit methods.

**Key Methods:**

- `HandleFetchHistory(requestBytes)`: Unmarshal a `FetchHistoryRequested` protobuf, retrieve events with file mutations, return a `FetchHistoryResult`.
- `HandleFetchFileHistory(requestBytes)`: Unmarshal a `FetchFileHistoryRequested` protobuf, retrieve git history, return a `FetchFileHistoryResult`.
- `HandleRestoreFile(requestBytes)`: Unmarshal a `RestoreFileRequested` protobuf, restore the file, return a `RestoreFileResult`.
- `GetFileAtCommit(filePath, commitHash, sessionID)`: Delegate to the ledger's `GetFileAtCommit`.

---

## Common Patterns

### Encryption at Rest

Services requiring encryption use `vault.Vault` (AES-256-GCM). Following v1.0.10, [encryption at rest](./encryption.md) is mandatory for sensitive data storage.

Behavior when the vault is locked varies by service:

- **Audit Store**: Stores plaintext and sets `encrypted = 0`. Audit continuity takes precedence over encryption (fail-open).
- **Execution Vault**: Returns an error if the vault is locked. No plaintext fallback (fail-closed).
- **Token Store (`EncryptedKVAdapter`)**: `KVSet` and `KVGet` return an error if the vault is locked. `KVScanPrefix` silently skips entries that cannot be decrypted. No plaintext fallback (fail-closed).
- **Ledger**: `GetFileAtCommit` and `RestoreFileFromCommit` return an error if the vault is locked.
- **Replay Store**: No encryption required as nonces contain no sensitive content.
- **Suspended Transaction Store**: No encryption required for governance envelopes.
- **Commitment Ledger**: No encryption required for public audit attestations.

### SQLite Utilities

All SQLite-backed services use `sqliteutil.DB`, which provides:

- Retry logic via `ExecWithRetry` and `QueryRowWithRetry`
- Transactional execution via `ExecInTxWithRetry`
- Row materialization via `MaterializeRows`
- Timestamp formatting and parsing
- Compression via `Compress` and `Decompress`
- Content hashing via `HashBytes`
- Database size monitoring via `GetSizeBytes`
- Incremental vacuum via `RunIncrementalVacuum`

### Pruning

Most services start a `sqliteutil.Pruner` in their constructor:

```go
pruner := sqliteutil.NewPruner(db, logger, interval, pruneFunc)
pruner.Start()
```

Prune functions typically:

1. Delete records older than the retention cutoff.
2. Delete the oldest 10% of records when the database exceeds the configured size limit.
3. Run incremental vacuum to reclaim space without a full database lock.

`SQLReplayStore` does not start a background pruner; callers invoke `Prune` and `CleanupStaleReserved` directly.

---

## Testing

The storage package contains comprehensive unit tests co-located with each service implementation:

- `audit_store_unit_test.go`: Unit tests for `SQLAuditStore` session, event, mutation, receipt, encryption, and config validation logic.
- `ledger_test.go`, `ledger_diffcontent_test.go`, `ledger_diffstat_test.go`, `ledger_git_test.go`: Unit tests for `GitLedgerService` two-phase commit, diff generation, and git operations.
- `execution_vault_test.go`: Unit tests for `ExecutionVaultService` encryption, compression, storage, and retrieval.
- `replay_store_test.go`: Unit tests for `SQLReplayStore` nonce reservation, finalization, release, and cleanup.
- `suspended_transaction_store_test.go`: Unit tests for `SuspendedTransactionService` store, retrieve, approve, and expire operations.
- `commitment_ledger_test.go`: Unit tests for `CommitmentLedger` chain integrity and atomic append.
- `history_handler_test.go`, `history_handler_unit_test.go`: Unit and integration tests for `HistoryHandler` protobuf request handling.
- `storage_test_helpers_test.go`: Shared test helper utilities.

The `storagetest` subdirectory (`internal/services/storage/storagetest/`) contains test-only implementations:

- `audit_vault.go`: `TestSQLAuditStore`, a monolithic test implementation that integrates audit storage with a git ledger in a single struct. It includes an additional `chaos_events` table and `RecordChaosEvent`/`RecordChaosEvents` methods not present in production code.
- `helpers.go`: `CreateTestVault` helper for initializing an unlocked `vault.Vault` in tests.
- `audit_store_*_test.go`: Dedicated test suites for audit store config, encryption, events, mutations, receipts, sessions, and end-to-end flows.

These implementations are kept separate from production code to avoid import cycles.

---

## Security Properties

1. **Encryption at rest**: Sensitive fields are encrypted at rest using AES-256-GCM. The Audit Store uses fail-open semantics to ensure continuity; the Execution Vault and Token Store use fail-closed semantics.
2. **Fail-closed replay protection**: `SQLReplayStore.ReserveNonce` returns an error on any SQLite failure, preventing replay protection from being silently bypassed.
3. **Commitment chain integrity**: `CommitmentLedger` verifies `prior_commitment_hash` inside a transaction to prevent chain forks under concurrent writes.
4. **Session validation**: Audit events must reference an existing session row. Foreign key constraints are enforced at the schema level.
5. **Path traversal protection**: `GitLedgerService.normalizeToGitPath` strips drive letters and leading separators before constructing ledger-relative paths.
6. **Size limits for encrypted copies**: The ledger enforces a 100 MB cap on encrypted file copies to prevent OOM during the full-read required by AES-GCM.
7. **Streaming for unencrypted copies**: The ledger streams unencrypted file copies using `io.Copy` to prevent OOM.
8. **Atomic nonce reservation**: The UNIQUE constraint on `nonce_usage.nonce` provides atomicity without application-level locking.

---

## Data Flow

### File Mutation Flow

1. Ledger begins two-phase commit, snapshots pre-mutation state, returns `LedgerResult` with `LedgerHashBefore`.
2. Operator performs the file write, delete, or create on the host filesystem.
3. Ledger completes the commit, copies the post-mutation file, commits to git, records `LedgerHashAfter`, `DiffStat`, and `DiffContent` in the result.
4. Audit store records the event with encrypted content.
5. Audit store records the file mutation linked to the event.
6. Execution vault stores the compressed and encrypted diff.

### Transaction Flow

1. Replay store reserves the nonce atomically.
2. Governance layer executes the transaction.
3. Execution vault stores the execution result.
4. Audit store records the action receipt.
5. Replay store finalizes the nonce.
6. Commitment ledger appends the attestation.

### Approval Flow (L3)

1. Suspended transaction store persists the transaction awaiting human [L3 approval](./auth.md#layer-3-notary-l3notary).
2. CLI lists pending transactions for the user.
3. User approves with a cryptographic signature.
4. Suspended transaction store marks the record as approved.
5. Governance layer executes the approved transaction.
6. Suspended transaction store deletes the record after execution.

---

## Related Documentation

- [**Authentication & Authorization**](./auth.md): Governance sequence and L3 Interlock
- [**Encryption Architecture**](./encryption.md): Vault subsystem and mandatory encryption at rest
- [**Network Architecture**](./network.md): Mutual TLS and identity binding
- [**g8e Protocol**](./protocol.md): The wire contract and governance hierarchy

