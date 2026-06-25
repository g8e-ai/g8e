# Transaction Process: End-to-End Flow

Last Updated: 2026-06-25
Version: v1.2.1

This document walks through the complete transaction process in the g8e governance system, explaining each step from initial intent to final execution and audit. The process is designed to ensure security, accountability, and sovereignty throughout.

## Overview

The g8e system implements a five-layer verification pipeline (L1-L5) that governs every transaction. Transactions flow from AI clients through a governance gateway to governed operators, where they undergo rigorous verification before execution on target systems. The pipeline is governed by a configurable **GovernancePosture** that determines which layers are enforced as fail-closed gates versus audited only.

### GovernanceEnvelope Protobuf Schema

The canonical transaction container is defined in `protocol/proto/g8e/common/v1/common.proto` as the `GovernanceEnvelope` message. It binds identity, intent, state, and governance proofs into a single transaction:

- **Identity fields**: `operator_id`, `operator_session_id`, `web_session_id`, `cli_session_id`, `requestor_user_id`, `acting_app_id`
- **Intent fields**: `event_type`, `payload` (raw protobuf bytes), `action_type`, `target_resource`, `intent_data`
- **State and replay protection**: `state_merkle_root`, `nonce`, `transaction_hash`, `protocol_version`
- **Governance proofs**: `GovernanceMetadata` containing `L1Metadata`, `L2Metadata` (tribunal votes), and `L3Metadata` (WebAuthn or mTLS proof)
- **Application context**: `case_id`, `investigation_id`, `task_id`, `system_fingerprint`, `tenant_id`, `binding_persona`

### Governance Postures

The `GovernancePosture` interface (`internal/services/governance/posture.go`) defines which verification layers are enforced as fail-closed gates versus audited. Three postures are defined, each adding a stricter layer of enforcement:

- **doctrine**: L1 enforced, L2/L3 audited (minimum)
- **consensus**: L1+L2 enforced, L3 audited
- **notary**: L1+L2+L3 strictly enforced (maximum)

The `NewGovernancePosture` factory function panics on unrecognized posture names so that misconfigured deployments fail at startup rather than silently running under a weaker posture. The posture is queried by the L4 Warden during verification and by the L5 Actuator when generating receipt metadata.

---

## Phase 1: Intent Submission

### Step 1: Principal Initiates Request

A **Principal** (human user or AI agent) submits an intent to perform an action. This intent can be submitted through multiple channels:

- **MCP client**: Using Claude, Cursor, Windsurf, or other MCP-compatible AI IDEs
- **Agentic ensemble**: Through A2A (Agent-to-Agent) protocols or tool calls
- **Native application**: Direct integration with g8e protocols

The intent represents what the principal wants to accomplish, for example, "read a file," "deploy a container," or "query a database."

### Step 2: Producer Wraps Intent

The **Producer** (g8e-compatible agentic ensemble, BYO agent, or MCP client) receives the raw intent and begins the governance process:

1. **Reach Consensus (L2)**: The producer sends the envelope to the **TribunalService** (`internal/services/tribunal/service.go`) for deliberation. Each tribunal member independently evaluates the payload using the L1 Doctrine and signs an Ed25519 vote over `<transaction_hash>|<decision>`. The `Deliberate` method populates `L2Metadata` with the `tribunal_id` and the collected `L2Vote` set. For single-binary deployments, the `LocalDeliberator` adapter calls `TribunalService.Deliberate` in-process without an HTTP round-trip.
2. **Create GovernanceEnvelope**: The producer wraps the intent in a `GovernanceEnvelope`, which includes:
   - The original intent as a typed protobuf payload
   - Tribunal L2 votes proving consensus
   - L3 proof (WebAuthn assertion or mTLS certificate fingerprint)
   - Metadata about the request (timestamp, principal identity, nonce, state root)
   - Cryptographic proofs for verification

The signed envelope is now ready for submission to the governance gateway.

---

## Phase 2: Gateway Admission

### Step 3: Envelope Submission to Gateway

The producer submits the signed `GovernanceEnvelope` to the **Governance Gateway (g8eg)**. The gateway serves as the Policy Decision Point and acts as the system's PKI authority.

The gateway accepts connections through:
- **HTTP/mTLS universal endpoint**: For MCP clients (Claude, Cursor, Windsurf)
- **Standard protocols**: For agentic ensembles and A2A communications

### Step 4: Gateway Admission Control

The gateway performs initial admission checks on the envelope (`internal/services/gateway/gateway_auth.go`, `internal/services/gateway/governance_envelope.go`, `internal/services/gateway/replay_store_service.go`):

1. **mTLS enforcement**: The HTTPS port uses `tls.VerifyClientCertIfGiven`, accepting and verifying client certificates when present but not requiring them at the TLS layer. mTLS enforcement for protected routes happens at the application layer via `auth.Middleware()`, which checks client cert presence and validity for all routes not in the `PublicRouteRegistry`. Browser clients (console, WebAuthn flows) reach public routes without a client cert. The mTLS middleware extracts operator session IDs from certificate SPIFFE URI SANs and authenticates Operator, CLI, or App identities.
2. **Certificate revocation check**: The gateway verifies that the client certificate is not revoked via the PKI authority.
3. **Transport-to-envelope identity binding**: The `verifyEnvelopeIdentityBinding` function enforces that mTLS certificate URI SANs match envelope identity claims (`operator_session_id`, `operator_id`), preventing impersonation.
4. **Replay protection**: The `ReplayStoreService` atomically reserves nonces in SQLite to prevent replay attacks at the gateway level.
5. **Rate limiting**: The governance envelope submission endpoint (`/api/v1/governance/envelopes`) is rate-limited.

If the envelope passes admission, it is queued for processing. If it fails, the gateway rejects it immediately with a typed error and audit entry.

---

## Phase 3: Operator Retrieval

### Step 5: Operator Establishes Connection

A **Governed Operator (g8eo)** running on a sovereign host establishes an outbound-only mTLS tunnel to the gateway. This is a critical security design:

- **Outbound-only**: The operator initiates the connection; the gateway cannot reach into the operator
- **mTLS encryption**: Mutual TLS ensures both ends authenticate each other
- **Policy Execution Point**: The operator is where policies are actually enforced

This design ensures that operators remain sovereign; they can pull work but cannot be pushed into from the gateway.

### Step 6: Operator Fetches Pending Envelope

The operator polls the gateway for pending envelopes that are assigned to it (based on policy, capacity, or other routing logic). When it finds an envelope, it retrieves it over the secure mTLS tunnel.

The operator now has the signed `GovernanceEnvelope` and begins the verification pipeline.

---

## Phase 4: Verification Pipeline (L1-L5)

The operator runs the envelope through a five-layer verification pipeline orchestrated by the **Warden (L4)**. Each layer must pass; if any layer fails, the transaction fails closed (rejected with audit trail). 

### Step 7: Warden Pre-Dispatch Gate (L4)

The **L4 Warden** (`internal/services/governance/l4_warden.go`) is the primary orchestrator for the verification pipeline. It performs a five-stage verification sequence:

1. **In-flight tracking** (stage 0): Prevents the same nonce from being processed concurrently using a `sync.Map`.
2. **Nonce reservation and expiry** (stage 1): Atomically reserves the nonce in the **Replay Store** (durable SQLite storage) to prevent replay attacks even if the operator crashes mid-execution. Checks expiry relative to an injectable clock.
3. **Stateless validation** (stage 2): Structural checks (transaction ID, action type, payload presence), typed payload decoding, L1 Doctrine validation, and transaction hash recomputation and comparison. The hash is computed by `GenerateMessageID` in `pkg/governance/types.go` over the following fields in proto definition order: `action_type`, `target_resource`, `payload` (base64-encoded), `state_merkle_root`, `nonce`, `expires_at` (UTC RFC3339Nano), `intent_data` (canonicalized), `requestor_user_id`, and `acting_app_id`. L3 proof is intentionally NOT included in the transaction hash so that L2 (machine consensus) can sign the hash before L3 (human notary) is asked. Tamper-evidence for L3 is provided by `verifyL3Posture`, which checks the proof against `envelope.TransactionHash` at verification time.
4. **Stateful validation** (stage 3): State Merkle root validation against the operator's current state root.
5. **Posture validation** (stage 4): L2 and L3 checks gated by the configured `GovernancePosture`.

If any stage fails, the nonce reservation is released and the transaction is rejected with a typed error. The nonce remains reserved through successful verification and is finalized after execution completes.

### Layer 1: Technical Bedrock (L1)

**Purpose**: Ensure the transaction doesn't violate fundamental technical or safety constraints.

**Checks** (handled by **L1 Doctrine** in `internal/services/governance/l1_doctrine.go`):
- **Protobuf Field Validation**: Reflects over protobuf message fields and checks the `forbidden_patterns` field option extension (defined in `protocol/proto/g8e/common/v1/common.proto`) against string values using regex matching.
- **MITRE-based Threat Detection**: Scans command strings (`CommandRequested`), MCP tool arguments (`McpCallRequested`), A2A payloads (`A2ACallRequested`), and file edit content (`FileEditRequested`) for known malicious patterns. Detectors cover data destruction (T1485, T1561.001), reverse shells, privilege escalation, and other attack categories with MITRE ATT&CK mappings.
- **Critical System File Protection**: The `isCriticalSystemFile` check blocks modifications to sensitive paths (e.g., `/etc/shadow`, `/etc/sudoers`).

**Outcome**:
- **Passed**: Proceeds to State Check
- **Violated**: Transaction fails closed with typed rejection and audit entry

### State Check: Merkle Root Freshness

**Purpose**: Ensure the transaction is based on the current system state.

**Checks**:
- **Merkle root validation**: Compares the `StateMerkleRoot` in the envelope against the operator's **Canonical DB** state root.
- **Consistency verification**: Rejects stale transactions to prevent race conditions on shared state.

**Outcome**:
- **Fresh**: Proceeds to L2
- **Stale**: Transaction fails closed with typed rejection and audit entry

### Layer 2: Consensus Verification (L2)

**Purpose**: Verify that the transaction has proper tribunal consensus.

**Checks** (performed by `verifyL2Posture` in the L4 Warden):
- **Vote presence**: If the posture requires L2 signatures (`RequiresL2Signature()`), the envelope must include `L2Metadata` with at least one vote.
- **Tribunal policy lookup**: Loads the `TribunalPolicy` by `tribunal_id` from the `TribunalStore`. The policy must exist and be enabled.
- **Member validation**: Votes from `signer_key_id` values not in the tribunal policy's `MemberAppIDs` are silently excluded; only member votes count toward quorum.
- **Duplicate signer detection**: If the policy requires distinct signers, duplicate `signer_key_id` values are rejected.
- **Signature verification**: Each vote's `consensus_signature` (Ed25519 over `<transaction_hash>|<decision>`) is verified against the trusted public key from the `SignerStore`.
- **Quorum check**: The number of affirmative (safe) votes must meet or exceed the policy's `Quorum` threshold.

**Posture behavior**:
- **doctrine**: L2 results are recorded for audit but do not gate execution.
- **consensus/notary**: L2 signature verification is a fail-closed gate.

**Outcome**:
- **Passed**: Proceeds to L3
- **Invalid/Missing**: Transaction fails closed with typed rejection and audit entry

### Layer 3: Authorization (L3)

**Purpose**: Ensure the principal is authorized and present for the action.

**L3 Notary implementations** (interface defined in `internal/services/governance/l3_notary.go`):

The `L3Notary` interface is implemented by the `outboundL3Notary` struct, which supports three operational modes via different constructor functions:

- **PasskeyService** (`internal/services/gateway/passkey_service.go`): Gateway mode for web sessions. Verifies WebAuthn assertions using the `transaction_hash` as the challenge. Validates credential ID, client data JSON, authenticator data, and signature against registered passkey credentials. Implements the `L3Notary` interface directly and is injected as the `passkeyVerifier` in `NewGatewayL3Notary`.

- **NewOutboundL3Notary** (`internal/services/governance/l3_notary.go`): Outbound mode for operator-side approval. Verifies that a transaction exists in the `SuspendedTransactionStore`, is marked as approved, has a valid CLI signature over the transaction hash, and has not exceeded the 30-minute approval window. No CLI session or certificate revocation checks are performed in this mode.

- **NewCLIL3Notary** (`internal/services/governance/l3_notary.go`): Gateway mode for CLI sessions. Extends outbound mode with a `CLISessionVerifier` that checks user active status, session ownership, certificate fingerprint match, session expiry, and certificate revocation via the PKI authority. The `CLISessionVerifier` interface is implemented by `cliSessionVerifier` in `internal/services/gateway/cli_session_verifier.go`.

- **NewGatewayL3Notary** (`internal/services/governance/l3_notary.go`): Unified gateway mode that handles both CLI (mTLS) and passkey (WebAuthn) proofs. When `VerifyL3Proof` is called, proofs containing `mtls_cert_fingerprint` route to the CLI verification path; all others delegate to the injected `passkeyVerifier` (PasskeyService). This is the constructor used in `GetGovernanceDeps` (`internal/services/gateway/gateway_service.go`) for gateway-mode deployments.

- **VerifyCLICertificate** (`internal/services/gateway/cli_cert.go`): Standalone function for real-time mTLS certificate validation during request authentication, distinct from the L3 notary verification path.

**Posture behavior**:
- **doctrine/consensus**: L3 results are recorded for audit but do not gate execution.
- **notary**: L3 proof is a fail-closed gate for mutation actions.

**Mutation enforcement**: The `isMutation` check determines whether an action type is state-changing. Only mutations require L3 proof under the notary posture.

**No bypass field**: L3 is satisfied only by a verified proof. There is no `AutoApproved` or equivalent bypass; the Warden re-derives whether L3 is required from the action type and posture, and if required, demands a real proof. Out-of-band approvals use the `outboundL3Notary` + `SuspendedTransactionStore` path with a verifiable CLI signature, not a producer-supplied flag.

**Outcome**:
- **Authorized**: Proceeds to L5 (Actuator)
- **Denied**: Transaction fails closed with typed rejection and audit entry

---

## Phase 5: Execution

### Layer 5: Actuator (L5)

**Purpose**: Execute the approved transaction and generate signed cryptographic evidence.

**Process** (`internal/services/governance/l5_actuator.go`):
1. **Initial Receipt**: The Actuator signs an initial receipt with `EXECUTION_STATUS_EXECUTING` using Ed25519 and logs it to the **Local Audit Vault** *before* starting execution. The receipt is canonicalized using `CanonicalizeActionReceipt` (deterministic JSON with fixed field order) before signing. This ensures evidence of the attempt is preserved even if execution crashes. If signing or logging fails, execution does NOT proceed (fail-closed).
2. **L2/L3 Status Recording**: The receipt includes `L2Status` and `L3Status` fields reflecting whether each layer was required by the posture and whether it passed validation.
3. **Sovereignty Rehydration**: If the payload was scrubbed for sovereignty, the **Scrubbing Service** rehydrates it using local tokens before execution.
4. **JIT Capability Minting**: The Actuator mints a just-in-time, single-action, self-dissolving **Capability** (`MintCapability` in `internal/services/governance/capability.go`) scoped to the transaction hash, action type, and target resource. The capability is injected into the execution context for downstream handlers. No standing credentials exist outside the lifetime of a single `Execute` call. The capability is dissolved immediately after execution completes or fails (zero standing privileges).
5. **Execution**: The Actuator dispatches the action through the `ExecutionHandler` interface (implemented by `OperatorPubSubService`), which routes to the appropriate handler based on event type:
   - **Command Execution**: Bash/Shell commands via `ExecutionService`.
   - **File Operations**: Scoped reads, writes, and edits via `FileEditService`.
   - **Protocol Egress**: MCP or A2A tool calls via the **MCP Gateway**.
   - **Synchronous handlers**: `EVAL_ANSWER`, `MCP_CALL`, and `A2A_CALL` return results directly as the receipt summary.
6. **Result Capture**: Output, errors, and updated Merkle roots are captured.
7. **Final Receipt**: A final `ActionReceipt` is generated, containing:
   - Execution results (or failure summary)
   - `StateRootBefore` and `StateRootAfter`
   - Operator signature (Ed25519 over canonical receipt JSON)
   - `L2Status` and `L3Status` reflecting posture enforcement
8. **Sovereignty Scrubbing**: Sensitive host data is scrubbed from the result before returning it to the gateway.

**Outcome**: Signed final receipt is generated and anchored to the local ledger.

---

## Phase 6: Audit and Completion

### Step 8: Local Audit Vault Logging

The operator anchors the transaction to the **Local Audit Vault** on the sovereign host. This architecture, known as **Local-First Audit Architecture (LFAA)**, ensures:

- **Immutable record**: The transaction cannot be altered after the fact.
- **Local sovereignty**: Audit data stays on the host; raw data never leaves.
- **Cryptographic integrity**: Each entry is signed and chained.
- **Multi-layered storage**:
    - **SQL AuditStore** (`internal/services/storage/audit_store.go`): Stores structured event data, `ActionReceipt` records, and file mutation records in an encrypted SQLite database. Receipts are stored with upsert semantics and support pagination for history queries.
    - **Git LedgerService** (`internal/services/storage/ledger.go`): Provides immutable versioning for file mutations using a git-backed ledger with two-phase commit. Files are optionally encrypted before copying to the ledger. Git state is snapshotted before and after mutations, with diffs calculated for audit logging. The current state Merkle root is the git commit hash.

The vault records:
- The original envelope
- Verification layer results (pass/fail for each layer)
- Execution results and state root transitions
- Signed receipts (both intent and final result)
- Timestamps and session metadata

**Note**: Even failed transactions are logged to the audit vault for complete transparency. The `HistoryHandler` (`internal/services/storage/history_handler.go`) integrates both stores to service `FetchHistory` requests, returning audit events with associated file mutation details.

### Step 9: Receipt Return to Gateway

The operator returns the sovereignty-scrubbed signed receipt to the gateway. In synchronous gateway mode, the `ProcessEnvelope` method (`internal/services/pubsub/pubsub_commands.go`) returns the receipt directly to the HTTP caller. In outbound mode, the receipt is pushed over the mTLS tunnel. The receipt:

- Confirms successful execution (or captures the failure)
- Provides the results (if authorized for the principal)
- Maintains the audit trail for the entire pipeline
- Contains no sensitive host data

The receipt is returned even on execution failure (status=FAILED) so callers receive cryptographic evidence of the attempt. A nil receipt is only returned when verification fails before execution begins.

### Step 10: Gateway Returns Final Output

The gateway receives the receipt and returns the final safe output to the principal:

- **Success case**: Returns the execution results.
- **Failure case**: Returns the typed rejection with explanation and receipt evidence.
- **Audit reference**: Provides a reference to the audit entry for traceability.

The principal now has confirmation of the transaction outcome.

---

## Security Properties

Throughout this process, several key security properties are maintained:

### Fail-Closed Design
Every verification layer fails closed: if any check fails, the transaction is rejected immediately and the nonce reservation is released. Crucially, the **Actuator** will not execute a mutation if it fails to sign or log the initial "intent to execute" receipt. The `NewGovernancePosture` factory panics on invalid posture names to prevent misconfigured deployments from silently running under a weaker posture.

### Sovereignty
- Raw data and audit logs stay on the sovereign host.
- Operators initiate outbound-only connections to the gateway.
- Sensitive data is scrubbed/rehydrated at the execution boundary.

### Cryptographic Integrity
- Every envelope is signed by tribunal members (L2) using Ed25519 over `<transaction_hash>|<decision>`.
- Every receipt is signed by the L5 Actuator using Ed25519 over canonical JSON (`CanonicalizeActionReceipt`).
- Audit entries are stored in encrypted SQLite databases with optional vault encryption.
- File mutations in the git ledger are optionally encrypted before storage.
- mTLS with `tls.VerifyClientCertIfGiven` on the HTTPS port, with application-layer enforcement via `auth.Middleware()` for all non-public routes. The PKI TLSConfig defaults to `RequireAndVerifyClientCert` for operator-side connections.
- Transport-to-envelope identity binding prevents impersonation by matching mTLS certificate SPIFFE URI SANs to envelope identity claims.

### Defense in Depth
- **L1 Doctrine**: Protobuf field option validation and MITRE-based threat detection.
- **L2 Consensus**: Multi-signature tribunal verification with quorum and distinct-signer checks.
- **L3 Notary**: Human-in-the-loop authorization via WebAuthn passkey, CLI mTLS certificate, or outbound CLI approval.
- **L4 Warden**: Replay protection, state root consistency, and posture-gated L2/L3 verification.
- **L5 Actuator**: Fail-closed signed execution boundary with canonical receipt production and zero standing privileges via JIT capability minting.

### Accountability
- Every transaction is logged with a unique `TransactionHash`.
- Every failure is recorded with typed rejection.
- Principal identity is verified at L3.

---

## Component Summary

| Component | Role | Key Characteristics |
|-----------|------|---------------------|
| **Principal** | Initiates intent | Human or AI agent, authenticated via WebAuthn or CLI mTLS. |
| **Producer** | Wraps intent in envelope | Reaches L2 consensus via TribunalService, creates GovernanceEnvelope with L2 votes and L3 proof. |
| **Governance Gateway** | Policy Decision Point | PKI authority, mTLS admission control, identity binding, replay store, universal endpoint. |
| **GovernancePosture** | Verification Policy | Configures which layers are enforced vs audited (doctrine, consensus, notary). |
| **L4 Warden** | Verification Orchestrator | Five-stage verification: in-flight tracking, nonce reservation, stateless validation (L1 Doctrine + hash), stateful validation (state root), posture-gated L2/L3. |
| **L1 Doctrine** | Technical Bedrock | Protobuf `forbidden_patterns` field option validation, MITRE-based threat detection with `ThreatDetector` regex patterns. |
| **L2 Consensus** | Tribunal Verification | Ed25519 vote signatures, quorum and distinct-signer checks, TribunalPolicy enforcement. |
| **L3 Notary** | Authorization Engine | `outboundL3Notary` (`l3_notary.go`) configured via `NewGatewayL3Notary` (gateway), `NewCLIL3Notary` (CLI), or `NewOutboundL3Notary` (operator). Routes WebAuthn proofs to PasskeyService and mTLS proofs to CLI session verification. |
| **L5 Actuator** | Execution Gateway | Fail-closed dual receipt signing with canonical JSON, JIT capability minting/dissolving, rehydration, execution dispatch via ExecutionHandler. |
| **Local Audit Vault** | Immutable Ledger | SQLAuditStore (encrypted SQLite) and GitLedgerService (git-backed, two-phase commit, optional encryption). |

---

## Transaction Flow Summary

1. **Principal** submits intent
2. **Producer** reaches L2 consensus via TribunalService and creates GovernanceEnvelope with L2 votes and L3 proof
3. **Gateway** admits envelope after mTLS, PKI, identity binding, and replay protection
4. **Operator** fetches envelope via outbound mTLS or processes synchronously via `ProcessEnvelope`
5. **Warden (L4)** performs five-stage verification: in-flight tracking, nonce reservation, stateless (L1 doctrine + hash), stateful (state root), and posture-gated L2/L3
6. **Actuator (L5)** signs initial receipt (fail-closed), rehydrates payload, mints JIT capability, executes via ExecutionHandler, dissolves capability, signs final receipt
7. **Local Audit Vault** (LFAA) logs complete transaction to SQLAuditStore and GitLedgerService
8. **Operator** returns signed receipt to gateway (scrubbed of sensitive host data)
9. **Gateway** returns final output to principal

This end-to-end process ensures that every transaction is governed, verified, executed safely, and audited while maintaining system sovereignty and security.
