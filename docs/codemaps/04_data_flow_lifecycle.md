# Data Flow and Request Lifecycle Codemap

## Overview

This codemap traces the complete lifecycle of a mutation request from intent to execution and audit. It covers both the Ensemble (g8ee) path and the BYO client path, showing how requests flow through the governance gauntlet.

## High-Level Request Flow

```text
┌─────────────────────────────────────────────────────────────────────┐
│                         REQUEST ORIGIN                              │
│  (Human via CLI, AI Agent via MCP, BYO Client via A2A/tool call)   │
└─────────────────────────────────────────────────────────────────────┘
                                    │
                                    ▼
┌─────────────────────────────────────────────────────────────────────┐
│                    ENVELOPE FORMATION (L2)                          │
│  (g8ee: Tribunal consensus + L2 signature OR BYO: direct envelope)  │
└─────────────────────────────────────────────────────────────────────┘
                                    │
                                    ▼
┌─────────────────────────────────────────────────────────────────────┐
│                      GATEWAY SUBMISSION (g8eg)                       │
│  (Admission API, mTLS authentication, replay protection)           │
└─────────────────────────────────────────────────────────────────────┘
                                    │
                                    ▼
┌─────────────────────────────────────────────────────────────────────┐
│                    OPERATOR PULL (g8eo via mTLS)                     │
│  (Outbound-only tunnel, envelope fetch, state root sync)            │
└─────────────────────────────────────────────────────────────────────┘
                                    │
                                    ▼
┌─────────────────────────────────────────────────────────────────────┐
│                  GOVERNANCE GAUNTLET (L1/L2/L3)                      │
│  (Envelope integrity → State freshness → L1 → L2 → L3)              │
└─────────────────────────────────────────────────────────────────────┘
                                    │
                                    ▼
┌─────────────────────────────────────────────────────────────────────┐
│                      ACTUATOR EXECUTION                              │
│  (Fail-closed dispatch, command execution, output capture)          │
└─────────────────────────────────────────────────────────────────────┘
                                    │
                                    ▼
┌─────────────────────────────────────────────────────────────────────┐
│                    AUDIT VAULT ANCHORING                             │
│  (SQLite audit event, Git-backed ledger commit, signed receipt)     │
└─────────────────────────────────────────────────────────────────────┘
                                    │
                                    ▼
┌─────────────────────────────────────────────────────────────────────┐
│                   RECEIPT RETURN (via Gateway)                       │
│  (Sentinel-scrubbed output, signed receipt, audit reference)        │
└─────────────────────────────────────────────────────────────────────┘
```

## Detailed Flow: Ensemble Path (g8ee)

### Phase 1: Intent Reception and Triage

```
HTTP Request (FastAPI)
    ↓
routers/chat_router.py
    ↓
services/ai/chat_pipeline.py
    ↓
Triage Agent Classification
    ├─ Simple/Diagnostic → Fast path (auto-approval)
    └─ Complex → Full governance path
```

**Key Components**:
- `app/routers/chat_router.py` - HTTP endpoint
- `app/services/ai/chat_pipeline.py` - Orchestration
- `app/services/ai/generator.py` - LLM generation
- `app/middleware/context.py` - RequestContext injection

### Phase 2: Sage Reasoning

```
Triage (Complex)
    ↓
Sage Agent (reasoner)
    ↓
Generate proposed action
    ↓
Tool call parsing
```

**Key Components**:
- `app/services/ai/generator.py` - Sage orchestration
- `app/llm/providers/` - LLM provider abstraction
- `app/models/` - Pydantic models for validation

### Phase 3: Tribunal Consensus (L2)

```
Sage proposal
    ↓
services/ai/tribunal/
    ↓
Parallel proposal generation (5 heterogeneous models)
    ↓
Consensus verification (k-of-n)
    ↓
services/ai/tribunal/stages/auditor.py
    ↓
services/ai/auditor_service.py
    ↓
Historical context retrieval
    ↓
Proposal validation
    ↓
Ed25519 signature generation (L2)
```

**Key Components**:
- `app/services/ai/tribunal/` - Consensus orchestration (multi-stage)
- `app/services/ai/tribunal/stages/auditor.py` - Auditor stage
- `app/services/ai/auditor_service.py` - L2 signature generation
- `app/llm/providers/` - Heterogeneous model ensemble
- `protocol/proto/common.proto` - L2Signature schema

### Phase 4: Warden Validation

```
Tribunal consensus
    ↓
services/ai/tribunal/stages/warden.py
    ↓
Heuristic risk assessment
    ↓
Two-strike circuit breaker
    ↓
Pass/Fail decision
```

**Key Components**:
- `app/services/ai/tribunal/stages/warden.py` - Circuit breaker logic
- Heuristic risk scoring
- Strike tracking

### Phase 5: Envelope Formation

```
Warden pass
    ↓
services/ai/auditor_service.py
    ↓
app/utils/envelope_builder.py
    ↓
GovernanceEnvelope formation
    ↓
L2 signature attachment
    ↓
transaction_hash computation
    ↓
id field set to hash
    ↓
protojson encoding
```

**Key Components**:
- `app/services/ai/auditor_service.py` - Envelope formation
- `app/utils/envelope_builder.py` - Envelope building utilities
- `app/proto/` - Generated protobuf code
- `protocol/proto/common.proto` - GovernanceEnvelope schema

### Phase 6: Gateway Submission

```
Envelope ready
    ↓
services/operator/operator_data_service.py
    ↓
Gateway submission (HTTP/mTLS)
    ↓
Replay protection check
    ↓
Session validation
    ↓
Envelope queued for Operator
```

**Key Components**:
- `app/services/operator/operator_data_service.py` - Gateway client
- `app/clients/http_client.py` - HTTP client
- `protocol/constants/api_paths.json` - API path mappings

### Phase 7: Response Handling

```
Gateway acknowledgment
    ↓
Receipt handling
    ↓
Response streaming to client
    ↓
Investigation update
    ↓
Memory association
```

**Key Components**:
- `app/services/investigation/investigation_service.py` - Investigation lifecycle
- `app/services/operator/stream_executor.py` - Streaming execution
- `app/db/models.py` - Persistence

## Detailed Flow: BYO Client Path

### Phase 1: Client Intent

```
BYO Client (MCP/A2A/tool call)
    ↓
Local envelope formation
    ↓
L2 signature (if client has consensus)
    ↓
protojson GovernanceEnvelope
```

**Key Components**:
- Client-side envelope formation
- Protocol compliance (must match `protocol/proto/common.proto`)
- Optional L2 signature (if client implements consensus)

### Phase 2: Gateway Submission

```
Envelope ready
    ↓
Gateway admission API (HTTP/mTLS)
    ↓
Client authentication (mTLS + device-link)
    ↓
Replay protection check
    ↓
Envelope validation
    ↓
Envelope queued for Operator
```

**Key Components**:
- Gateway admission API
- mTLS authentication
- Device-link validation
- Replay protection

### Phase 3-7: Same as Ensemble Path
(Operator pull, governance gauntlet, execution, audit, receipt return)

## Detailed Flow: Operator Side (g8eo)

### Phase 1: Gateway Connection

```
Operator startup (--listen mode)
    ↓
services/gateway/
    ↓
Open outbound-only mTLS tunnel to Gateway
    ↓
Certificate validation
    ↓
Connection established
```

**Key Components**:
- `services/g8eo/internal/services/gateway/` - Gateway client
- `services/g8eo/internal/security/` - mTLS configuration
- Outbound-only tunnel (no inbound listeners)

### Phase 2: Envelope Fetch

```
Connection ready
    ↓
services/gateway/
    ↓
Fetch pending envelopes from Gateway
    ↓
Envelope received (protojson)
    ↓
Unmarshal to GovernanceEnvelope
```

**Key Components**:
- `services/g8eo/internal/services/gateway/` - Envelope fetch
- `services/g8eo/internal/marshaler/` - Envelope unmarshaling
- `services/g8eo/internal/protocol/proto/` - Generated protobuf code

### Phase 3: Pre-Governance Checks

```
Envelope received
    ↓
services/governance/
    ↓
Envelope integrity check
    ↓
Typed payload decode
    ↓
Hash binding verification (id == computed hash)
    ↓
Freshness check (nonce + expiry)
    ↓
State root freshness (expected vs current)
```

**Key Components**:
- `services/g8eo/internal/services/governance/` - Verification logic
- `services/g8eo/internal/protocol/types/` - Type adapters
- `services/g8eo/internal/services/storage/` - State root retrieval

### Phase 4: L1 Doctrine

```
Pre-checks pass
    ↓
services/governance/
    ↓
L1 Doctrine validation
    ↓
Forbidden patterns check (constants/doctrine/forbidden_patterns.json)
    ↓
Blacklist check (constants/doctrine/blacklist.json)
    ↓
Whitelist check (constants/doctrine/whitelist.json)
    ↓
Pass/Fail decision
```

**Key Components**:
- `services/g8eo/internal/services/governance/` - L1 validation
- `protocol/constants/doctrine/` - Doctrine rules
- Reflection-based pattern matching

### Phase 5: L2 Quorum

```
L1 pass
    ↓
services/governance/
    ↓
L2 signature verification
    ↓
Ed25519 signature check
    ↓
Consensus verification (k-of-n)
    ↓
Reputation stake verification
    ↓
Pass/Fail decision
```

**Key Components**:
- `services/g8eo/internal/services/governance/` - L2 verification
- `services/g8eo/internal/security/` - Ed25519 verification
- `protocol/constants/agents.json` - Agent definitions

### Phase 6: L3 Notary

```
L2 pass
    ↓
services/governance/
    ↓
L3 authorization check
    ↓
WebAuthn/FIDO2 signature verification
    ↓
Human authorization confirmation
    ↓
Transaction hash challenge verification
    ↓
Pass/Fail decision
```

**Key Components**:
- `services/g8eo/internal/services/governance/` - L3 verification
- `services/g8eo/internal/services/auth/` - WebAuthn verification
- OOB approval flow (if not pre-approved)

### Phase 7: Actuator Execution

```
L3 pass
    ↓
services/execution/
    ↓
Fail-closed dispatch
    ↓
Command execution
    ↓
Output capture
    ↓
Timeout enforcement
    ↓
Execution complete
```

**Key Components**:
- `services/g8eo/internal/services/execution/` - Actuator
- Command dispatch and execution
- Output capture and streaming
- Resource limits

### Phase 8: Sentinel Scrubbing

```
Execution complete
    ↓
services/sentinel/
    ↓
PII detection and redaction
    ↓
Secret detection and redaction
    ↓
Safe output projection
    ↓
Scrubbed output ready
```

**Key Components**:
- `services/g8eo/internal/services/sentinel/` - Scrubbing logic
- Pattern-based detection
- Safe output generation

### Phase 9: Audit Vault Anchoring

```
Scrubbed output ready
    ↓
services/storage/audit_vault.go
    ↓
SQLite audit event write
    ↓
Session validation
    ↓
Event insertion
    ↓
services/storage/ledger.go
    ↓
Git commit (go-git)
    ↓
Tamper-evident history
```

**Key Components**:
- `services/g8eo/internal/services/storage/audit_vault.go` - SQLite vault
- `services/g8eo/internal/services/storage/ledger.go` - Git ledger
- `services/g8eo/internal/services/system/` - Git operations (go-git)
- `.g8e/audit/` - SQLite database
- `.g8e/ledger/.git` - Git repository

### Phase 10: Receipt Generation

```
Audit anchored
    ↓
services/execution/
    ↓
ActionReceipt formation
    ↓
Execution metadata
    ↓
Output hash
    ↓
Signer signature
    ↓
protojson encoding
```

**Key Components**:
- `services/g8eo/internal/services/execution/` - Receipt generation
- `protocol/proto/common.proto` - ActionReceipt schema
- Ed25519 signing

### Phase 11: Receipt Return

```
Receipt ready
    ↓
services/gateway/
    ↓
Push receipt to Gateway (mTLS)
    ↓
Gateway acknowledges
    ↓
Receipt stored
```

**Key Components**:
- `services/g8eo/internal/services/gateway/` - Receipt push
- mTLS tunnel
- Gateway storage

## Critical Decision Points

### Fail-Closed Points

1. **Envelope Integrity**: Invalid envelope → reject
2. **Hash Binding**: id != computed hash → reject
3. **Freshness**: Stale nonce/expiry → reject
4. **State Root**: Stale state root → reject
5. **L1 Doctrine**: Forbidden pattern → reject
6. **L2 Quorum**: Invalid signature → reject
7. **L3 Notary**: Unauthorized → reject
8. **Execution Failure**: Command error → audit + reject

### Audit Points

1. **Envelope Receipt**: Log incoming envelope
2. **L1/L2/L3 Decisions**: Log each gate decision
3. **Execution**: Log command execution
4. **Output**: Log scrubbed output
5. **Receipt**: Log signed receipt
6. **Error**: Log all failures with context

## Storage Flow

### SQLite Audit Vault
```
Event → services/storage/audit_vault.go → SQLite INSERT
→ Session validation → Row committed → Queryable audit log
```

### Git Ledger
```
Event → services/storage/ledger.go → Git staging
→ Git commit (go-git) → Commit hash → Tamper-evident history
→ Diff computation → Rollback capability
```

## Error Handling

### Envelope Rejection
```
Any gate fail → Typed rejection → Audit event → Receipt (rejection)
→ Return to Gateway → Gateway returns to client
```

### Execution Failure
```
Command error → Audit event → Receipt (error) → Return to Gateway
→ Gateway returns to client → Client handles error
```

### System Failure
```
System error → Audit event → Receipt (system error) → Return to Gateway
→ Gateway returns to client → Client handles error
```

## Performance Considerations

### Parallel Execution
- Tribunal consensus: Parallel model calls
- L2 verification: Parallel signature checks
- Audit writes: Batch inserts

### Caching
- State root: Cached and refreshed periodically
- Doctrine rules: Loaded at startup
- Agent configurations: Loaded at startup

### Streaming
- LLM generation: Streamed responses
- Command output: Streamed capture
- Receipt return: Streamed to Gateway

## Security Boundaries

### Trust Boundaries
1. **Client → Gateway**: mTLS + device-link
2. **Gateway → Operator**: mTLS + outbound-only tunnel
3. **Operator → Execution**: Fail-closed Actuator
4. **Execution → Output**: Sentinel scrubbing
5. **All → Audit**: Tamper-evident storage

### Data Sovereignty
- Raw data: Never leaves host
- Scrubbed output: Crosses trust boundary
- Audit events: Host-local only
- State root: Distributed via Gateway

## Key Invariants

1. **Fail-closed**: All gates default to reject
2. **Audit-first**: All decisions audited before side effects
3. **Hash binding**: Transaction hash independent of wire encoding
4. **State binding**: Envelope bound to current state root
5. **Local audit**: Audit vault host-local only
6. **Sentinel scrubbing**: All output scrubbed before exposure
7. **Outbound-only**: Operator has no inbound listeners
8. **Protocol-first**: All envelopes follow protocol schema
