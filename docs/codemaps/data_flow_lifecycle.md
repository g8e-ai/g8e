# Data Flow and Request Lifecycle Codemap

## Overview

This codemap traces the complete lifecycle of a mutation request from envelope submission to execution and audit. It documents the actual implementation in the g8eo Operator substrate, showing how canonical JSON GovernanceEnvelope payloads flow through the governance verification gauntlet to the Actuator execution boundary.

The Operator runs in two modes:
- **Outbound mode**: Traditional operator with pub/sub subscription to cloud platform, executes commands locally
- **Gateway mode**: Platform persistence and messaging backbone, serves inbound requests via HTTP/WebSocket, delegates execution to downstream MCP/A2A servers

Both modes share the same fail-closed verification gauntlet and Actuator execution boundary.

## High-Level Request Flow

```text
┌─────────────────────────────────────────────────────────────────────┐
│                         ENVELOPE SUBMISSION                          │
│  Gateway mode: POST /api/governance/envelope (HTTP/WebSocket)        │
│  Outbound mode: pub/sub message from cloud platform                  │
│  Wire format: Canonical protojson GovernanceEnvelope                 │
└─────────────────────────────────────────────────────────────────────┘
                                    │
                                    ▼
┌─────────────────────────────────────────────────────────────────────┐
│                  PUBSUBCOMMANDSERVICE.DISPATCH                        │
│  ProcessEnvelope() (Gateway mode, synchronous)                        │
│  handleCommandPayload() (Outbound mode, async pub/sub)               │
│  Payload size validation, protojson decode                           │
└─────────────────────────────────────────────────────────────────────┘
                                    │
                                    ▼
┌─────────────────────────────────────────────────────────────────────┐
│                  L4WARDEN.VERIFYENVELOPE                       │
│  In-flight nonce tracking → Nonce reservation (SQLite)               │
│  Stateless: hash, L1 doctrine, payload decode                        │
│  Stateful: state root freshness                                       │
│  Posture: L1/L2/L3 based on governance posture                        │
└─────────────────────────────────────────────────────────────────────┘
                                    │
                                    ▼
┌─────────────────────────────────────────────────────────────────────┐
│                    L5ACTUATOR.EXECUTE                                  │
│  Sign initial receipt → Log receipt (SQLite + console_audit)          │
│  Sovereignty payload rehydration (if available)                      │
│  Execute handler (local or MCP/A2A egress)                           │
│  Update receipt with final status → Return signed receipt             │
└─────────────────────────────────────────────────────────────────────┘
                                    │
                                    ▼
┌─────────────────────────────────────────────────────────────────────┐
│                    AUDIT VAULT ANCHORING                             │
│  SQLite receipts table (transaction-native audit)                    │
│  Session-scoped git ledger (go-git) for file mutations              │
└─────────────────────────────────────────────────────────────────────┘
                                    │
                                    ▼
┌─────────────────────────────────────────────────────────────────────┐
│                   RECEIPT RETURN                                      │
│  Signed ActionReceipt with execution status, L2/L3 validity flags    │
└─────────────────────────────────────────────────────────────────────┘
```

## Detailed Flow: Operator Processing

### Phase 1: Envelope Dispatch

**Entry Points**:
- `PubSubCommandService.ProcessEnvelope()` - Synchronous HTTP API entry point for Gateway mode (POST /api/governance/envelope)
- `PubSubCommandService.handleCommandPayload()` - Asynchronous pub/sub message handler for Outbound mode
- `PubSubCommandService.HandleCommandData()` - Gateway mode internal dispatch from HTTP/WebSocket

**Process**:
1. Payload size validation (rejects oversized payloads)
2. Protojson decode to `GovernanceEnvelope` (rejects non-JSON formats)
3. Dispatch to verification gauntlet

**Key Components**:
- `internal/services/pubsub/pubsub_commands.go` - Dispatch logic
- Wire format: canonical protojson JSON (not binary protobuf)
- Gateway mode: No pub/sub subscription, commands arrive via HTTP/WebSocket
- Outbound mode: Pub/sub subscription to cloud platform command channel

### Phase 2: Transaction Verification

**Entry Point**: `L4Warden.VerifyEnvelope()`

**Verification Stages**:

1. **In-Flight Tracking** (early race prevention):
   - Track nonce in concurrent-safe in-flight map (sync.Map)
   - Reject if same nonce already processing

2. **Nonce Reservation** (durable replay protection):
   - Reserve nonce in SQLite replay store (atomic CheckAndSetNonce)
   - Check expiry timestamp
   - Reject if nonce already used (replay attack)
   - Nonce remains reserved until execution completes or fails
   - Release nonce on verification failure (allows retry)

3. **Stateless Validation** (no external state required):
   - Validate required fields (id, transaction_hash, payload)
   - Decode typed protobuf payload based on action_type
   - Compute transaction hash from normalized envelope fields
   - Verify id == computed hash (hash binding invariant)
   - L1 Doctrine validation via protobuf field options (forbidden patterns)
   - Extended L1 validation for MCP/A2A argument payloads via L1Doctrine (recursive threat analysis)

4. **Stateful Validation** (requires external state):
   - Verify state_merkle_root matches current state root
   - Reject stale state roots (prevents replay against old state)
   - State root provided by StateRootProvider (GatewayDBService in both modes)

5. **Posture Validation** (governance posture-aware):
   - L2 signature verification (if required by posture)
   - L3 proof verification (if required by posture and action is mutation)
   - Support for external app policy L3 bypass (auto-approve intents)
   - Governance postures: doctrine (L1 enforced, L2/L3 audited), consensus (L1/L2 enforced, L3 audited), notary (L1/L2/L3 strictly enforced)

**Key Components**:
- `internal/services/governance/l4_warden.go` - Verification logic
- `internal/services/storage/replay_store.go` - Nonce replay protection (SQLite)
- `protocol/constants/status.json` - Action type definitions and mutation flags
- `protocol/proto/common.proto` - GovernanceEnvelope schema with L1 field options
- `internal/services/governance/l1_doctrine.go` - MCP/A2A argument threat analysis
- Governance postures: doctrine, consensus, notary (configurable via --doctrine, --consensus, --notary flags)

### Phase 3: Actuator Execution

**Entry Point**: `L5Actuator.Execute()`

**Process**:
1. Prepare initial ActionReceipt with EXECUTING status
2. Sign receipt with Actuator's Ed25519 key (fail-closed if signing fails)
3. Log receipt to audit vault before execution (fail-closed if logging fails)
4. Sentinel payload rehydration (if available, restores scrubbed content)
5. Dispatch to registered ExecutionHandler based on action_type
   - Local execution: CommandService, FileOpsService, PortService, HistoryHandler
   - MCP/A2A egress: MCPGateway.DispatchToDownstream / DispatchToA2ADownstream
6. Capture execution result (success/failure/timeout)
7. Update receipt with final status and state root after execution
8. Sign final receipt
9. Return signed ActionReceipt (even on execution failure)

**Key Components**:
- `internal/services/governance/l5_actuator.go` - Execution boundary
- `internal/services/execution/` - Local execution handlers (command, file edit, fs operations)
- `internal/services/pubsub/` - CommandService, FileOpsService, PortService, HistoryHandler
- `internal/services/mcp/gateway.go` - MCP/A2A protocol translation egress
- `protocol/proto/operator.proto` - ActionReceipt schema
- Fail-closed invariant: no execution without signed receipt
- Receipt includes L2Valid and L3Valid flags for governance posture tracking

### Phase 4: Audit Anchoring

**Entry Points**:
- `AuditVaultService.RecordActionReceipt()` - SQLite receipts table (transaction-native audit)
- `AuditVaultService.RecordEvent()` - SQLite events table (legacy command-centric audit)
- `LedgerService` - Session-scoped git-backed file mutation ledger (go-git)

**Process**:
1. Validate operator_session_id (must reference pre-created session in sessions table)
2. Write ActionReceipt to SQLite receipts table with execution metadata
3. Write ActionReceipt document to console_audit collection (document store)
4. For file mutations: commit to session-scoped git ledger with diff
5. Store tamper-evident history with commit hashes per session

**Key Components**:
- `internal/services/storage/audit_vault.go` - SQLite vault (receipts, events, sessions tables)
- `internal/services/storage/ledger.go` - Git ledger (go-git, session-scoped)
- `.g8e/data/g8e.db` - SQLite database location
- `.g8e/data/ledger/.git` - Global git repository
- `.g8e/data/ledger/sessions/{session_id}/.git` - Session-scoped git repositories
- Fail-closed: reject events with unknown sessions, never auto-create sessions
- Receipts table provides transaction-native audit with L2/L3 validity flags

### Phase 5: Receipt Return

**Process**:
1. Actuator returns signed ActionReceipt to caller
2. Receipt contains execution status, state roots, signature, and L2/L3 validity flags
3. Caller (HTTP API or pub/sub) returns receipt to client
4. Receipt serves as cryptographic proof of execution attempt
5. Receipt returned even on execution failure (status=FAILED)

**Key Components**:
- `operatorv1.ActionReceipt` - Protobuf receipt schema
- Ed25519 signature verification by clients
- CanonicalizeActionReceipt for deterministic signing/verification
- Receipt includes gateway_signed flag for Gateway mode tracking

## Critical Decision Points

### Fail-Closed Points

1. **In-Flight Nonce**: Same nonce already processing → reject (early race prevention)
2. **Nonce Replay**: Nonce already used in replay store → reject (durable replay protection)
3. **Envelope Decode**: Non-JSON payload → reject (only protojson accepted)
4. **Transaction Hash**: id != computed hash → reject (hash binding invariant)
5. **Expiry**: Transaction expired → reject
6. **State Root**: state_merkle_root != current state root → reject
7. **L1 Doctrine**: Forbidden pattern in typed payload → reject
8. **L1 Sentinel**: MCP/A2A argument threat detected → reject (recursive analysis)
9. **L2 Signature**: Invalid or missing signature (if required by posture) → reject
10. **L3 Proof**: Invalid or missing proof (if required by posture and mutation) → reject
11. **Actuator Signing**: Cannot sign receipt → reject (no execution without signed receipt)
12. **Audit Logging**: Cannot log receipt → reject (no execution without audit)
13. **Session Validation**: Unknown operator_session_id → reject (audit vault fail-closed)

### Audit Points

1. **Nonce Reservation**: Durable SQLite write before expensive verification
2. **Verification Decisions**: Log each gate decision with nonce and reason
3. **Blocked Transactions**: Log blocked transactions to receipts table with BLOCKED status
4. **Receipt Signing**: Log initial receipt (EXECUTING status) before execution
5. **Execution**: Log command execution with stdout/stderr (truncated)
6. **Receipt Finalization**: Log final receipt with execution status
7. **Error**: Log all failures with context and nonce

## Storage Flow

### Replay Store (SQLite)
```
Nonce reservation → ReserveNonce() → SQLite INSERT
→ FinalizeNonce() on success → ReleaseNonce() on failure
→ Prevents replay attacks across Operator restarts
→ Atomic CheckAndSetNonce for early durable commitment
```

### Audit Vault (SQLite)
```
ActionReceipt → RecordActionReceipt() → Session validation
→ SQLite INSERT to receipts table → Transaction-native audit
→ Event → RecordEvent() → Session validation
→ SQLite INSERT to events table → Legacy command-centric audit
→ Fail-closed: rejects unknown sessions, never auto-creates
```

### Git Ledger (go-git, session-scoped)
```
File mutation → LedgerFileWrite() → Session-scoped git staging
→ Git commit (go-git) → Commit hash → Tamper-evident history
→ Diff computation → Rollback capability
→ Session-scoped repos: .g8e/data/ledger/sessions/{session_id}/.git
→ Global repo: .g8e/data/ledger/.git
```

## Error Handling

### Verification Failure
```
L4Warden.VerifyEnvelope() fails
→ Release nonce reservation (allows retry)
→ Return governance.ErrXxx sentinel error
→ Log blocked transaction to receipts table (BLOCKED status)
→ No receipt generated (verification failed before execution)
```

### Execution Failure
```
L5Actuator.Execute() handler fails
→ Update receipt with FAILED status
→ Sign final receipt
→ Log receipt to audit
→ Return signed receipt (status=FAILED)
→ Client receives cryptographic evidence of failure
```

### System Failure
```
L5Actuator signing or audit logging fails
→ Fail-closed: do not execute handler
→ Return error without receipt
→ Client receives verification error
→ No mutation occurs without audit trail
```

## Performance Considerations

### Concurrency
- In-flight nonce tracking: concurrent-safe sync.Map
- Replay store: SQLite with atomic ReserveNonce/FinalizeNonce
- Execution service: semaphore-based concurrency control
- L5Actuator: sync.WaitGroup for graceful shutdown
- Audit vault writes: sync.WaitGroup for concurrent write safety

### Caching
- State root: Provided by StateRootProvider (GatewayDBService in both modes)
- Doctrine rules: Loaded from protobuf field options (no runtime cache)
- L2 signers: Loaded from filesystem at startup (FilesystemSignerStore)
- Governance posture: Configured at startup (doctrine/consensus/notary)
- MCP/A2A downstream URLs: Configured at startup

### Streaming
- Command output: streamingWriter with line-by-line logging
- Output truncation: 10MB limit per stream to prevent OOM
- Pub/sub messages: async goroutine processing with WaitGroup
- File ledger copies: streaming for unencrypted files, in-memory for encrypted (100MB limit)

## Security Boundaries

### Trust Boundaries
1. **Client → Operator**: mTLS authentication (Gateway mode HTTP/WebSocket, Outbound mode pub/sub)
2. **Envelope → Verification**: Fail-closed L4Warden with L1/L2/L3 gates
3. **Verification → L5Actuator**: VerifiedTransaction with L2/L3 validity flags
4. **L5Actuator → Execution**: Fail-closed receipt signing before execution
5. **Execution → Audit**: SQLite + session-scoped Git ledger (host-local only)
6. **Audit → Client**: Signed receipt as cryptographic proof
7. **MCP/A2A Egress**: Downstream server dispatch via MCPGateway (Gateway mode only)

### Data Sovereignty
- Raw execution output: Stored in SQLite audit vault (host-local, encrypted if vault unlocked)
- File mutations: Stored in session-scoped Git ledger (host-local, encrypted if vault unlocked)
- Scrubbed output: Returned in receipt (crosses trust boundary)
- State root: Provided by GatewayDBService (same schema in both modes)
- Nonce replay protection: SQLite replay store (host-local)
- MCP/A2A downstream results: Bounded to 4 KiB in receipt summary

## Key Invariants

1. **Fail-closed**: All verification gates default to reject
2. **Audit-first**: Receipt logged before execution begins
3. **Hash binding**: Transaction hash computed from normalized fields, id == hash
4. **State binding**: Envelope bound to current state root
5. **Local audit**: Audit vault and session-scoped ledger host-local only
6. **Replay protection**: Durable nonce reservation in SQLite with in-flight tracking
7. **Wire format**: Canonical protojson JSON (not binary protobuf)
8. **Session validation**: Audit vault rejects events with unknown sessions, never auto-creates
9. **Governance posture**: L1/L2/L3 enforcement based on doctrine/consensus/notary mode
10. **Receipt on failure**: Signed receipt returned even on execution failure
