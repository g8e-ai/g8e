# Storage Architecture

## Overview

The g8e storage architecture provides a comprehensive, multi-layered persistence system designed for security, auditability, and governance. The storage layer is split into specialized services, each handling a specific aspect of the system's data needs:

- **Audit Store**: Append-only audit logging with encryption at rest
- **Ledger**: Git-backed version control for file modifications
- **Execution Vault**: Command execution results and file diffs
- **Token Store**: Key-value storage for Sentinel token persistence
- **Replay Store**: Nonce-based replay protection
- **Suspended Transaction Store**: L3 approval workflow transactions
- **Commitment Ledger**: Commitment attestations with chain integrity
- **History Handler**: Unified history retrieval combining audit and ledger

All storage services share common patterns:
- SQLite-based with `sqliteutil.DB` wrapper for retry logic
- Encryption at rest via `vault.Vault` (AES-256-GCM) where applicable
- Automatic pruning based on retention policies and size limits
- Background cleanup via `sqliteutil.Pruner`
- Thread-safe operations with proper synchronization

## Core Services

### Audit Store (`audit_store.go`)

The `SQLAuditStore` provides append-only audit logging for all system events. It is the authoritative source of truth for operator sessions, events, file mutations, and action receipts.

**Schema Tables:**
- `sessions`: Operator/app session metadata (id, title, session_type, created_at, user_identity)
- `events`: Audit events with encrypted content (content_text, command_stdout, command_stderr, encrypted flag)
- `file_mutation_log`: File operation records linked to events
- `receipts`: Signed action receipts for transaction-native audit records

**Key Features:**
- **Encryption at rest**: `content_text`, `command_stdout`, and `command_stderr` are encrypted using the vault
- **Output truncation**: Large outputs are truncated using head/tail strategy (configurable thresholds)
- **Session validation**: Events must reference existing sessions (auto-created for app sessions)
- **Batch recording**: `RecordEvents` supports atomic batch inserts
- **Retention pruning**: Automatic cleanup of old events, file mutations, and receipts
- **Action receipts**: Transaction-native audit records with cryptographic signatures

**Configuration:**
```go
type AuditStoreConfig struct {
    DataDir                   string  // Base directory for data
    DBPath                    string  // Database filename
    MaxDBSizeMB               int64   // Maximum database size
    RetentionDays             int     // Data retention period
    PruneIntervalMinutes      int     // Pruning interval
    Enabled                   bool    // Enable/disable audit store
    OutputTruncationThreshold int     // Threshold for output truncation
    HeadTailSize              int     // Size of head/tail for truncation
    EncryptionVault           *vault.Vault  // Required for encryption
}
```

**Key Methods:**
- `CreateSession(id, sessionType, title, userIdentity)`: Create a new session
- `GetOperatorSession(id)`: Retrieve a session by ID
- `RecordEvent(event)`: Record a single event with encryption
- `RecordEvents(events)`: Batch record multiple events atomically
- `RecordActionReceipt(record)`: Record a signed action receipt
- `GetEvents(sessionID, limit, offset)`: Retrieve events for a session with decryption
- `RecordFileMutation(mutation)`: Record a file mutation linked to an event
- `GetFileMutations(eventID)`: Retrieve file mutations for an event
- `GetActionReceipt(transactionID)`: Retrieve a single action receipt
- `ListActionReceipts(sessionID, limit, offset)`: List action receipts with pagination
- `ListActionReceiptsSince(since, limit)`: List action receipts newer than a timestamp

### Ledger (`ledger.go`)

The `GitLedgerService` provides git-backed version control for all file modifications. It maintains a complete history of file changes with cryptographic integrity.

**Key Features:**
- **Two-phase commit**: File operations use a two-phase pattern (begin operation, complete operation)
- **Session isolation**: Each operator session has its own git repository under `sessions/{sessionID}/`
- **Encryption at rest**: Files are encrypted when vault is unlocked (stored with `.enc` extension)
- **Streaming for unencrypted files**: Large files are streamed to prevent OOM
- **Size limits for encrypted files**: 100MB safety limit for encrypted file copies
- **Diff generation**: Full diff content and statistics between commits
- **File restoration**: Restore files to any previous commit state
- **Cross-platform path handling**: Normalizes paths for Windows/Linux compatibility
- **Native go-git implementation**: Uses go-git library for git operations (not git binary)

**Two-Phase Commit Pattern:**
```go
// Write operation
result, err := ledger.LedgerFileWrite(sessionID, filePath)
// ... perform file write ...
err = ledger.CompleteMirrorWrite(result, sessionID)

// Delete operation
result, err := ledger.MirrorFileDelete(sessionID, filePath)
// ... perform file deletion ...
err = ledger.CompleteMirrorDelete(result, sessionID)

// Create operation
result, err := ledger.MirrorFileCreate(sessionID, filePath)
// ... perform file creation ...
err = ledger.CompleteMirrorCreate(result, sessionID)
```

**Key Methods:**
- `GetSessionLedgerPath(sessionID)`: Get or create session-specific git repository
- `LedgerFileWrite(sessionID, filePath)`: Begin two-phase write operation
- `CompleteMirrorWrite(result, sessionID)`: Complete write operation
- `MirrorFileDelete(sessionID, filePath)`: Begin two-phase delete operation
- `CompleteMirrorDelete(result, sessionID)`: Complete delete operation
- `MirrorFileCreate(sessionID, filePath)`: Begin two-phase create operation
- `CompleteMirrorCreate(result, sessionID)`: Complete create operation
- `GetFileHistory(filePath, limit, sessionID)`: Retrieve git history for a file
- `GetFileAtCommit(filePath, commitHash, sessionID)`: Get file content at specific commit
- `RestoreFileFromCommit(filePath, commitHash, sessionID)`: Restore file to previous state
- `GetStateMerkleRoot()`: Get current git commit hash as state merkle root
- `GetDiffContent(hashBefore, hashAfter, sessionID)`: Get full diff between commits
- `GetDiffStat(hashBefore, hashAfter, sessionID)`: Get diff statistics

**Path Normalization:**
The ledger uses `normalizeToGitPath` to convert file paths to git-relative paths with forward slashes, handling Windows drive letters and backslashes for cross-platform compatibility.

### Execution Vault (`execution_vault.go`)

The `ExecutionVaultService` provides SQLite storage for command execution results and file diffs. This is the execution vault - all data encrypted at rest when configured.

**Schema Tables:**
- `execution_log`: Command execution records (id, timestamp_utc, command, exit_code, duration_ms, stdout_compressed, stderr_compressed, stdout_hash, stderr_hash, stdout_size, stderr_size, user_id, case_id, task_id, investigation_id, operator_id)
- `file_diff_log`: File diff records (id, timestamp_utc, file_path, operation, ledger_hash_before, ledger_hash_after, diff_stat, diff_compressed, diff_hash, diff_size, operator_session_id, user_id, case_id, operator_id)

**Key Features:**
- **Compression**: stdout, stderr, and diffs are compressed before storage
- **Encryption at rest**: Compressed data is encrypted using the vault
- **Hash verification**: Content hashes stored for integrity verification
- **Size tracking**: Tracks original and compressed sizes
- **Case/task/investigation linking**: Supports investigation workflow metadata
- **Retention pruning**: Automatic cleanup based on retention and size limits

**Configuration:**
```go
type ExecutionVaultConfig struct {
    DBPath               string  // Database path
    MaxDBSizeMB          int64   // Maximum database size
    RetentionDays        int     // Data retention period
    PruneIntervalMinutes int     // Pruning interval
    Enabled              bool    // Enable/disable execution vault
}
// EncryptionVault is required (passed to constructor)
```

**Key Methods:**
- `StoreExecution(ctx, record)`: Store command execution result with encryption/compression
- `GetExecution(ctx, executionID)`: Retrieve execution with decryption/decompression
- `StoreFileDiff(ctx, record)`: Store file diff with encryption/compression
- `GetFileDiff(ctx, diffID)`: Retrieve file diff with decryption/decompression
- `GetFileDiffsBySession(ctx, sessionID, limit)`: Retrieve all diffs for a session

### Token Store (`token_store.go`)

The `TokenStoreService` provides key-value storage for Sentinel token persistence with TTL support. This service implements the `TokenStore` interface.

**Schema Tables:**
- `kv`: Key-value pairs with optional expiration (key, value, expires_at)

**Key Features:**
- **TTL support**: Keys can have time-to-live expiration
- **Encryption at rest**: Values are encrypted using the vault
- **Prefix scanning**: Support for prefix-based key queries
- **Automatic cleanup**: Expired keys are pruned automatically
- **Size-based pruning**: Oldest keys pruned when size limit exceeded

**Configuration:**
```go
type TokenStoreConfig struct {
    DBPath               string  // Database path
    MaxDBSizeMB          int64   // Maximum database size
    RetentionDays        int     // Data retention period
    PruneIntervalMinutes int     // Pruning interval
    Enabled              bool    // Enable/disable token store
}
// EncryptionVault is required (passed to constructor)
```

**Key Methods:**
- `KVSet(ctx, key, value, ttlSeconds)`: Set key-value pair with optional TTL
- `KVGet(ctx, key)`: Retrieve value by key (honors TTL)
- `KVScanPrefix(ctx, prefix)`: Retrieve all key-value pairs with given prefix
- `KVDelete(key)`: Delete a key-value pair

### Replay Store (`replay_store.go`)

The `SQLReplayStore` provides nonce-based replay protection using SQLite. It ensures that nonces cannot be reused within their validity window.

**Schema Tables:**
- `nonce_usage`: Nonce lifecycle tracking (nonce, reserved_at, used_at, expires_at, status)

**Key Features:**
- **Atomic replay detection**: Uses SQLite UNIQUE constraint for atomic detection
- **Nonce lifecycle**: Reserve → Finalize or Release
- **Fail-closed design**: Any error during cleanup or reservation returns an error
- **Automatic cleanup**: Expired nonces are cleaned up on each reservation
- **Stale reservation cleanup**: Removes reservations that were never finalized
- **No encryption required**: Nonce data does not contain sensitive content

**Nonce Lifecycle:**
1. `ReserveNonce(nonce, expiresAt)`: Atomically reserve a nonce (returns true if replay detected)
2. `FinalizeNonce(nonce)`: Mark a reserved nonce as fully consumed
3. `ReleaseNonce(nonce)`: Remove reservation for a failed transaction

**Key Methods:**
- `ReserveNonce(nonce, expiresAt)`: Reserve nonce for replay protection
- `FinalizeNonce(nonce)`: Mark nonce as used
- `ReleaseNonce(nonce)`: Release nonce reservation
- `CleanupStaleReserved(maxReservedDuration)`: Clean up stale reservations
- `Prune(retentionDays)`: Remove old nonce records

### Suspended Transaction Store (`suspended_transaction_store.go`)

The `SuspendedTransactionService` provides storage for L3 approval workflow transactions. It stores transactions awaiting human approval.

**Schema Tables:**
- `suspended_transactions`: Approval workflow transactions (transaction_hash, envelope, created_at, expires_at, tool_name, tool_arguments, user_id, operator_id, approved, approved_at, approved_by, approval_signature, expected_cert_fingerprint)

**Key Features:**
- **Envelope storage**: Stores full governance envelope for approval
- **Approval tracking**: Tracks approval status, approver identity, and signature
- **Expiration**: Transactions have expiration times
- **Certificate fingerprinting**: Expected cert fingerprint for approval validation
- **Automatic cleanup**: Expired transactions are pruned automatically
- **User filtering**: Can list transactions filtered by user

**Configuration:**
```go
type SuspendedTransactionConfig struct {
    DBPath               string  // Database path
    MaxDBSizeMB          int64   // Maximum database size
    RetentionDays        int     // Data retention period
    PruneIntervalMinutes int     // Pruning interval
    Enabled              bool    // Enable/disable suspended transaction store
}
```

**Key Methods:**
- `StoreSuspendedTransaction(ctx, tx)`: Store transaction awaiting approval
- `GetSuspendedTransaction(ctx, txHash)`: Retrieve suspended transaction (returns false if expired)
- `ListSuspendedTransactions(ctx, userID)`: List non-expired transactions (optionally filtered by user)
- `ApproveSuspendedTransaction(ctx, txHash, approvedBy, signature, certFingerprint)`: Mark transaction as approved
- `DeleteSuspendedTransaction(ctx, txHash)`: Remove transaction after approval/rejection
- `CleanupExpiredSuspendedTransactions(ctx)`: Remove expired transactions

### Commitment Ledger (`commitment_ledger.go`)

The `CommitmentLedger` provides SQLite-backed storage for commitment attestations. It stores raw JSON attestations with atomic append operations to guarantee chain integrity under concurrent writes.

**Schema Tables:**
- `commitment_ledger`: Commitment attestations (transaction_id, transaction_hash, prior_commitment_hash, state_root_at_commit, l2_signature_digest, Actuator_intent_signature_digest, human_signature_digest, action_type, target_resource, committed_at_unix_ms, auditor_key_id, signature, hash, attestation_json)

**Key Features:**
- **Chain integrity**: Verifies prior_commitment_hash matches current latest commitment
- **Atomic append**: Transactional insertion prevents concurrent attestations from chaining to same prior_hash
- **JSON storage**: Stores raw JSON attestation while extracting structured fields
- **Signature tracking**: Stores all signature digests and auditor signature

**Key Methods:**
- `GetLatestCommitmentJSON()`: Retrieve most recent commitment as raw JSON (returns nil if empty)
- `AppendCommitmentJSON(attestationJSON, priorHash, hash)`: Atomically append new commitment with chain verification

### History Handler (`history_handler.go`)

The `HistoryHandler` combines the audit store and ledger to provide unified history retrieval functionality. It handles protobuf-based requests for history operations.

**Key Features:**
- **Protobuf interface**: Handles protobuf-encoded requests/responses
- **Unified history**: Combines audit events with file mutations
- **File history**: Retrieves git history for specific files
- **File restoration**: Restores files to previous commit states
- **Session context**: All operations scoped to operator sessions

**Key Methods:**
- `HandleFetchHistory(requestJSON)`: Fetch audit events for a session with file mutations
- `HandleFetchFileHistory(requestJSON)`: Fetch git history for a specific file
- `HandleRestoreFile(requestJSON)`: Restore a file to a previous commit state
- `GetFileAtCommit(filePath, commitHash, sessionID)`: Get file content at specific commit
- `IsEnabled()`: Check if history handler is enabled

## Common Patterns

### Encryption at Rest

All storage services that handle sensitive data use the `vault.Vault` for encryption at rest:

```go
// Encryption
encrypted, err := vault.Encrypt([]byte(content))

// Decryption
decrypted, err := vault.Decrypt(encryptedData)
```

Services that require encryption:
- Audit Store: content_text, command_stdout, command_stderr (EncryptionVault required in config)
- Ledger: File content (when vault is unlocked, EncryptionVault required in config)
- Execution Vault: stdout, stderr, file diffs (EncryptionVault passed to constructor)
- Token Store: All values (EncryptionVault passed to constructor)
- Replay Store: No encryption required (nonce data is not sensitive)
- Suspended Transaction Store: No encryption required (envelope contains governance data)
- Commitment Ledger: No encryption required (attestation JSON is public)

### SQLite Utilities

All SQLite-based stores use the `sqliteutil.DB` wrapper which provides:
- Retry logic for transient failures
- Transaction support with `ExecInTxWithRetry`
- Row materialization with `MaterializeRows`
- Timestamp formatting/parsing
- Compression/decompression utilities
- Hash computation
- Database size monitoring
- Incremental vacuum for space reclamation

### Pruning

Most storage services use `sqliteutil.Pruner` for background cleanup:

```go
pruner := sqliteutil.NewPruner(db, logger, interval, pruneFunc)
pruner.Start()
```

Prune functions typically:
1. Delete records older than retention period
2. Delete records when database size exceeds limit
3. Run incremental vacuum to reclaim space

### Configuration

All storage services follow a similar configuration pattern:

```go
type Config struct {
    DBPath               string  // Database path
    MaxDBSizeMB          int64   // Maximum database size
    RetentionDays        int     // Data retention period
    PruneIntervalMinutes int     // Pruning interval
    Enabled              bool    // Enable/disable service
    // ... service-specific fields
}
```

Default configurations are provided via `Default*Config()` functions.

## Testing

Test implementations are provided in the `storagetest` subdirectory:

- `audit_vault.go`: Test-only monolithic audit service with Git ledger integration
- `helpers.go`: Test helper functions
- Various test files: E2E tests, encryption tests, event tests, mutation tests, receipt tests, session tests

The test implementations are separate from production code to avoid import cycles and provide flexible testing scenarios.

## Security Considerations

1. **Encryption at Rest**: Sensitive data is encrypted at rest using AES-256-GCM via the vault where applicable (Audit Store, Ledger, Execution Vault, Token Store)
2. **Fail-Closed Design**: Replay store fails closed on any error during nonce operations
3. **Chain Integrity**: Commitment ledger verifies prior_hash to prevent chain forks
4. **Session Validation**: Audit events must reference existing sessions
5. **Path Traversal Protection**: Ledger normalizes paths to prevent directory traversal
6. **Size Limits**: Encrypted file copies have size limits to prevent OOM
7. **Atomic Operations**: Replay detection uses UNIQUE constraints for atomicity
8. **Streaming**: Large unencrypted files are streamed to prevent memory exhaustion
9. **Vault Requirements**: Audit Store and Ledger require EncryptionVault in config; Execution Vault and Token Store require vault passed to constructor

## Performance Considerations

1. **Batch Operations**: Audit store supports batch event recording for efficiency
2. **Compression**: Execution vault compresses data before encryption
3. **Streaming**: Ledger streams unencrypted files to prevent OOM
4. **Incremental Vacuum**: Pruning runs incremental vacuum to reclaim space without full database lock
5. **Indexing**: All tables have appropriate indexes for query performance
6. **Two-Phase Commit**: Ledger uses two-phase commit to minimize lock duration
7. **Background Pruning**: Cleanup runs in background to avoid blocking operations

## Data Flow

### File Mutation Flow

1. **Begin Operation**: Ledger begins two-phase commit (snapshots pre-mutation state)
2. **File Operation**: Operator performs file write/delete/create
3. **Complete Operation**: Ledger completes commit (snapshots post-mutation state, generates diff)
4. **Record Event**: Audit store records event with encrypted content
5. **Record Mutation**: Audit store records file mutation linked to event
6. **Store Diff**: Execution vault stores compressed/encrypted diff

### Transaction Flow

1. **Reserve Nonce**: Replay store reserves nonce for replay protection
2. **Execute Transaction**: Governance layer executes transaction
3. **Store Execution**: Execution vault stores execution result
4. **Record Receipt**: Audit store records action receipt
5. **Finalize Nonce**: Replay store marks nonce as used
6. **Append Commitment**: Commitment ledger appends attestation

### Approval Flow

1. **Suspend Transaction**: Suspended transaction store stores transaction awaiting approval
2. **List Pending**: CLI lists pending transactions for user
3. **Approve Transaction**: User approves transaction with signature
4. **Update Status**: Suspended transaction store marks as approved
5. **Execute Transaction**: Governance layer executes approved transaction
6. **Cleanup**: Transaction is removed from suspended store

## Migration and Upgrades

The storage layer uses SQLite which provides:
- Schema initialization via `CREATE TABLE IF NOT EXISTS` statements
- Backward compatibility through careful schema evolution
- Transactional schema changes
- Easy backup and restore

Schema changes should:
1. Use `CREATE TABLE IF NOT EXISTS` for new tables
2. Use `ALTER TABLE` for additive changes
3. Avoid destructive changes (DROP COLUMN, etc.)
4. Provide migration paths for existing data

Note: The storage layer does not currently use a formal migration system. Schema changes are applied via initialization statements in each service's schema constant.
