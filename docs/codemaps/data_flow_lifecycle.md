# Data Flow and Request Lifecycle Codemap

## Overview

This codemap traces the complete lifecycle of a mutation request from envelope submission to execution and audit. It documents the actual implementation in the g8eo Operator substrate, showing how UAP JSON envelopes flow through the governance verification gauntlet to the Actuator execution boundary.

## High-Level Request Flow

```text
┌─────────────────────────────────────────────────────────────────────┐
│                         ENVELOPE SUBMISSION                          │
│  (POST /api/governance/envelope or pub/sub message)                  │
│  (Canonical protojson UAP JSON GovernanceEnvelope)                   │
└─────────────────────────────────────────────────────────────────────┘
                                    │
                                    ▼
┌─────────────────────────────────────────────────────────────────────┐
│                  PUBSUBCOMMANDSERVICE.DISPATCH                        │
│  (ProcessEnvelope() or handleCommandPayload())                       │
│  (Payload size validation, protojson decode)                         │
└─────────────────────────────────────────────────────────────────────┘
                                    │
                                    ▼
┌─────────────────────────────────────────────────────────────────────┐
│              TRANSACTIONVERIFIER.VERIFYENVELOPE                       │
│  (In-flight nonce tracking → Nonce reservation)                      │
│  (Stateless: hash, L1 doctrine, payload decode)                      │
│  (Stateful: state root freshness)                                    │
│  (Posture: L2/L3 based on governance posture)                        │
└─────────────────────────────────────────────────────────────────────┘
                                    │
                                    ▼
┌─────────────────────────────────────────────────────────────────────┐
│                    ACTUATOR.EXECUTE                                  │
│  (Sign initial receipt → Log receipt → Execute handler)              │
│  (Update receipt with final status → Return signed receipt)           │
└─────────────────────────────────────────────────────────────────────┘
                                    │
                                    ▼
┌─────────────────────────────────────────────────────────────────────┐
│                    AUDIT VAULT ANCHORING                             │
│  (SQLite audit event write, Git-backed ledger commit)                │
└─────────────────────────────────────────────────────────────────────┘
                                    │
                                    ▼
┌─────────────────────────────────────────────────────────────────────┐
│                   RECEIPT RETURN                                      │
│  (Signed ActionReceipt with execution status)                        │
└─────────────────────────────────────────────────────────────────────┘
```

## Detailed Flow: Operator Processing

### Phase 1: Envelope Dispatch

**Entry Points**:
- `PubSubCommandService.ProcessEnvelope()` - Synchronous HTTP API entry point (POST /api/governance/envelope)
- `PubSubCommandService.handleCommandPayload()` - Asynchronous pub/sub message handler

**Process**:
1. Payload size validation (rejects oversized payloads)
2. Protojson decode to `UAPEnvelope` (rejects non-JSON formats)
3. Dispatch to verification gauntlet

**Key Components**:
- `services/g8eo/internal/services/pubsub/pubsub_commands.go` - Dispatch logic
- `services/g8eo/pkg/uap/` - UAP envelope handling
- Wire format: canonical protojson JSON (not binary protobuf)

### Phase 2: Transaction Verification

**Entry Point**: `TransactionVerifier.VerifyEnvelope()`

**Verification Stages**:

1. **In-Flight Tracking** (early race prevention):
   - Track nonce in concurrent-safe in-flight map
   - Reject if same nonce already processing

2. **Nonce Reservation** (durable replay protection):
   - Reserve nonce in SQLite replay store
   - Check expiry timestamp
   - Reject if nonce already used (replay attack)
   - Nonce remains reserved until execution completes or fails

3. **Stateless Validation** (no external state required):
   - Validate required fields (id, transaction_hash, payload)
   - Decode typed protobuf payload based on action_type
   - Compute transaction hash from normalized envelope fields
   - Verify id == computed hash (hash binding invariant)
   - L1 Doctrine validation via protobuf field options (forbidden patterns)
   - Extended L1 validation for MCP/A2A argument payloads via Sentinel

4. **Stateful Validation** (requires external state):
   - Verify state_merkle_root matches current state root
   - Reject stale state roots (prevents replay against old state)

5. **Posture Validation** (governance posture-aware):
   - L2 signature verification (if required by posture)
   - L3 proof verification (if required by posture and action is mutation)
   - Support for external app policy L3 bypass (auto-approve intents)

**Key Components**:
- `services/g8eo/internal/services/governance/transaction_verifier.go` - Verification logic
- `services/g8eo/internal/services/storage/replay_store.go` - Nonce replay protection
- `protocol/constants/status.json` - Action type definitions and mutation flags
- `protocol/proto/common.proto` - GovernanceEnvelope schema with L1 field options
- Governance postures: doctrine, consensus, notary (configurable)

### Phase 3: Actuator Execution

**Entry Point**: `Actuator.Execute()`

**Process**:
1. Prepare initial ActionReceipt with EXECUTING status
2. Sign receipt with Actuator's Ed25519 key (fail-closed if signing fails)
3. Log receipt to audit vault before execution (fail-closed if logging fails)
4. Dispatch to registered ExecutionHandler based on action_type
5. Capture execution result (success/failure/timeout)
6. Update receipt with final status and state root after execution
7. Sign final receipt
8. Return signed ActionReceipt

**Key Components**:
- `services/g8eo/internal/services/governance/actuator.go` - Execution boundary
- `services/g8eo/internal/services/execution/` - Execution handlers (command, file edit, fs operations)
- `protocol/proto/operator.proto` - ActionReceipt schema
- Fail-closed invariant: no execution without signed receipt

### Phase 4: Audit Anchoring

**Entry Points**:
- `AuditVaultService.RecordEvent()` - SQLite audit event write
- `LedgerService` - Git-backed file mutation ledger

**Process**:
1. Validate operator_session_id (must reference pre-created session)
2. Write audit event to SQLite with execution metadata
3. For file mutations: commit to git ledger with diff
4. Store tamper-evident history with commit hashes

**Key Components**:
- `services/g8eo/internal/services/storage/audit_vault.go` - SQLite vault
- `services/g8eo/internal/services/storage/ledger.go` - Git ledger (go-git)
- `.g8e/audit/` - SQLite database location
- `.g8e/ledger/.git` - Git repository location
- Fail-closed: reject events with unknown sessions, never auto-create sessions

### Phase 5: Receipt Return

**Process**:
1. Actuator returns signed ActionReceipt to caller
2. Receipt contains execution status, state roots, and signature
3. Caller (HTTP API or pub/sub) returns receipt to client
4. Receipt serves as cryptographic proof of execution attempt

**Key Components**:
- `operatorv1.ActionReceipt` - Protobuf receipt schema
- Ed25519 signature verification by clients
- Receipt returned even on execution failure (status=FAILED)

## Critical Decision Points

### Fail-Closed Points

1. **In-Flight Nonce**: Same nonce already processing → reject (early race prevention)
2. **Nonce Replay**: Nonce already used in replay store → reject (durable replay protection)
3. **Envelope Decode**: Non-JSON payload → reject (only protojson accepted)
4. **Transaction Hash**: id != computed hash → reject (hash binding invariant)
5. **Expiry**: Transaction expired → reject
6. **State Root**: state_merkle_root != current state root → reject
7. **L1 Doctrine**: Forbidden pattern in typed payload → reject
8. **L2 Signature**: Invalid or missing signature (if required by posture) → reject
9. **L3 Proof**: Invalid or missing proof (if required by posture and mutation) → reject
10. **Actuator Signing**: Cannot sign receipt → reject (no execution without signed receipt)
11. **Audit Logging**: Cannot log receipt → reject (no execution without audit)
12. **Session Validation**: Unknown operator_session_id → reject (audit vault fail-closed)

### Audit Points

1. **Nonce Reservation**: Durable SQLite write before expensive verification
2. **Verification Decisions**: Log each gate decision with nonce and reason
3. **Receipt Signing**: Log initial receipt (EXECUTING status) before execution
4. **Execution**: Log command execution with stdout/stderr
5. **Receipt Finalization**: Log final receipt with execution status
6. **Error**: Log all failures with context and nonce

## Storage Flow

### Replay Store (SQLite)
```
Nonce reservation → ReserveNonce() → SQLite INSERT
→ FinalizeNonce() on success → ReleaseNonce() on failure
→ Prevents replay attacks across Operator restarts
```

### Audit Vault (SQLite)
```
Execution result → RecordEvent() → Session validation
→ SQLite INSERT → Queryable audit log
→ Fail-closed: rejects unknown sessions
```

### Git Ledger (go-git)
```
File mutation → LedgerFileWrite() → Git staging
→ Git commit (go-git) → Commit hash → Tamper-evident history
→ Diff computation → Rollback capability
```

## Error Handling

### Verification Failure
```
TransactionVerifier.VerifyEnvelope() fails
→ Release nonce reservation
→ Return governance.ErrXxx sentinel error
→ No receipt generated (verification failed before execution)
```

### Execution Failure
```
Actuator.Execute() handler fails
→ Update receipt with FAILED status
→ Sign final receipt
→ Log receipt to audit
→ Return signed receipt (status=FAILED)
```

### System Failure
```
Actuator signing or audit logging fails
→ Fail-closed: do not execute handler
→ Return error without receipt
→ Client receives verification error
```

## Performance Considerations

### Concurrency
- In-flight nonce tracking: concurrent-safe sync.Map
- Replay store: SQLite with atomic ReserveNonce/FinalizeNonce
- Execution service: semaphore-based concurrency control
- Actuator: sync.WaitGroup for graceful shutdown

### Caching
- State root: Provided by StateRootProvider (cached at Gateway level)
- Doctrine rules: Loaded from protobuf field options (no runtime cache)
- L2 signers: Loaded from filesystem at startup (FilesystemSignerStore)
- Governance posture: Configured at startup (doctrine/consensus/notary)

### Streaming
- Command output: streamingWriter with line-by-line logging
- Output truncation: 10MB limit per stream to prevent OOM
- Pub/sub messages: async goroutine processing with WaitGroup

## Security Boundaries

### Trust Boundaries
1. **Client → Operator**: mTLS authentication (listen mode)
2. **Envelope → Verification**: Fail-closed TransactionVerifier
3. **Verification → Actuator**: VerifiedTransaction with L2/L3 validity flags
4. **Actuator → Execution**: Fail-closed receipt signing before execution
5. **Execution → Audit**: SQLite + Git ledger (host-local only)
6. **Audit → Client**: Signed receipt as cryptographic proof

### Data Sovereignty
- Raw execution output: Stored in SQLite audit vault (host-local)
- File mutations: Stored in Git ledger (host-local)
- Scrubbed output: Returned in receipt (crosses trust boundary)
- State root: Distributed via Gateway state sync
- Nonce replay protection: SQLite replay store (host-local)

## Key Invariants

1. **Fail-closed**: All verification gates default to reject
2. **Audit-first**: Receipt logged before execution begins
3. **Hash binding**: Transaction hash computed from normalized fields, id == hash
4. **State binding**: Envelope bound to current state root
5. **Local audit**: Audit vault and ledger host-local only
6. **Replay protection**: Durable nonce reservation in SQLite
7. **Wire format**: Canonical protojson JSON (not binary protobuf)
8. **Protocol-first**: All envelopes follow protobuf schema
9. **Session validation**: Audit vault rejects unknown sessions
10. **Receipt signing**: No execution without signed receipt
