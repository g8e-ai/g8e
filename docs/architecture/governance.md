# Governance

Last Updated: 2026-07-03
Version: v1.3.6

## Overview

The g8e system implements a five-layer verification pipeline (L1-L5) that governs every transaction. Transactions flow from AI clients through a governance gateway to governed operators, where they undergo rigorous verification before execution on target systems. The pipeline is governed by a configurable **GovernancePosture** that determines which layers are enforced as fail-closed gates versus audited only.

The posture is set at startup via `--posture <doctrine|consensus|notary>` and cannot be changed at runtime. The posture interface is defined in `internal/services/governance/posture.go` and queried at two enforcement points:

1. **L4 Warden** (`internal/services/governance/l4_warden.go`): gates transaction dispatch based on L2/L3 verification results.
2. **L5 Actuator** (`internal/services/governance/l5_actuator.go`): records L2/L3 status in the signed `ActionReceipt`.

A third enforcement point exists at startup:

3. **Gateway startup** (`internal/cli/serve/gateway.go`): validates consensus and notary posture prerequisites before any services start.

### GovernanceEnvelope Protobuf Schema

The canonical transaction container is defined in `protocol/proto/g8e/common/v1/common.proto` as the `GovernanceEnvelope` message. It binds identity, intent, state, and governance proofs into a single transaction:

- **Identity fields**: `operator_id`, `operator_session_id`, `web_session_id`, `cli_session_id`, `requestor_user_id`, `acting_app_id`
- **Intent fields**: `event_type`, `payload` (raw protobuf bytes), `action_type`, `target_resource`, `intent_data`
- **State and replay protection**: `state_merkle_root`, `nonce`, `transaction_hash`, `protocol_version`
- **Governance proofs**: `GovernanceMetadata` containing `L1Metadata`, `L2Metadata` (tribunal votes), and `L3Metadata` (WebAuthn or mTLS proof)
- **Application context**: `case_id`, `investigation_id`, `task_id`, `system_fingerprint`, `tenant_id`, `binding_persona`

---

## Posture Definitions

### Doctrine (default)

**Configuration**: `--posture doctrine`

**Interface implementation** (`internal/services/governance/posture.go:47-52`):
- `RequiresL2Signature()` → `false`
- `RequiresL3Proof()` → `false`

**What is enforced (fail-closed, all postures)**:
- **L1 Doctrine validation**: protobuf `forbidden_patterns` field option regex checks and MITRE-based threat detection via `L1Doctrine.ValidatePayload()` (`internal/services/governance/l1_doctrine.go:50`). Any violation returns `ErrTxL1ValidationFailed` and the transaction is rejected (`l4_warden.go:466-469`).
- **Transaction hash integrity**: `envelope.id` and `envelope.transaction_hash` must both match the recomputed hash (`l4_warden.go:477-493`).
- **Nonce replay protection**: nonces are atomically reserved in SQLite before any further checks (`l4_warden.go:325-359`).
- **Expiry enforcement**: expired transactions are rejected (`l4_warden.go:334-341`).
- **State Merkle root validation**: `envelope.state_merkle_root` must match the current state root (`l4_warden.go:500-526`).
- **Action type validation**: unknown action types are rejected (`l4_warden.go:447-450`).
- **Payload decoding**: payloads must decode to the correct protobuf type for the action (`l4_warden.go:458-462`).

**What is audited but NOT gated**:
- **L2 Consensus votes**: if L2 votes are present, they are verified and the result is recorded in the `VerifiedTransaction.L2Valid` field and the `ActionReceipt.L2Status` field, but a missing or invalid L2 does **not** reject the transaction (`l4_warden.go:550-555`).
- **L3 Notary proofs**: if an L3 proof is present, it is verified and the result is recorded in `VerifiedTransaction.L3Valid` and `ActionReceipt.L3Status`, but a missing or invalid L3 does **not** reject the transaction, even for mutations (`l4_warden.go:641-676`).

**Default posture**: Doctrine is the default for gateway mode when no `--posture` flag is provided (`internal/config/config.go:398`). Outbound (operator) mode defaults to notary (`internal/config/config.go:534`).

---

### Consensus

**Configuration**: `--posture consensus`

**Interface implementation** (`internal/services/governance/posture.go:56-61`):
- `RequiresL2Signature()` → `true`
- `RequiresL3Proof()` → `false`

**What is enforced (fail-closed)**:
- Everything from doctrine posture (L1, hash, nonce, expiry, state root, action type, payload decoding).
- **L2 Consensus signature verification**: the following checks are all fail-closed gates under consensus posture:

  - **Vote presence**: The envelope must include `L2Metadata` with at least one vote. Missing votes → `ErrTxL2SignatureMissing` (`l4_warden.go:550-554`).
  - **Signer store configured**: If `signerStore` is nil → `ErrTxL2SignerStoreNotConfigured` (`l4_warden.go:560-565`).
  - **Tribunal store configured**: If `tribunalStore` is nil → `ErrTxL2TribunalNotConfigured` (`l4_warden.go:568-573`).
  - **Tribunal policy exists and is enabled**: The `TribunalPolicy` for `L2.tribunal_id` must exist and have `Enabled = true` (`l4_warden.go:576-590`).
  - **Member validation**: Votes from `signer_key_id` values not in the tribunal policy's `MemberAppIDs` are silently excluded from quorum count (`l4_warden.go:601-603`).
  - **Duplicate signer detection**: If `policy.RequireDistinct` is true, duplicate `signer_key_id` values → `ErrTxL2DuplicateSigner` (`l4_warden.go:604-609`).
  - **Signature verification**: Each vote's `consensus_signature` (Ed25519 over `<transaction_hash>|<decision>`) is verified against the trusted public key from the `SignerStore`. Invalid signatures are excluded from quorum count (`l4_warden.go:611-623`).
  - **Quorum check**: The number of affirmative (decision = true) votes from valid, distinct members must be >= `policy.Quorum`. If not → `ErrTxL2QuorumNotMet` (`l4_warden.go:630-636`).

**Startup validation**: The gateway performs additional validation at startup for consensus and notary postures (`internal/cli/serve/gateway.go`):
- `tribunalID` must be non-empty → `ErrConfigTribunalIDRequired` (`internal/config/config.go:292-293`).
- The `TribunalPolicy` must exist in the database and be enabled.
- **Quorum must be >= 1** → `ErrConfigTribunalQuorumLow` (`internal/config/config.go:295-296`). Quorum=1 is valid for single-member ensembles (e.g., demo deployments with a single-key ensemble).
- The Tribunal service is bootstrapped in-process and wired as both the mTLS HTTP handler and the local deliberator.

**Tribunal bootstrap** (`internal/cli/serve/gateway.go:465-492`): The `BootstrapTribunal` function constructs a `TribunalService` from the `TribunalPolicy` stored in the database. For single-member tribunals, the gateway's actuator Ed25519 key is reused as the member signing key (Option C). For multi-member tribunals, member keys are loaded from disk via `FileKeyProvider` (`internal/services/tribunal/factory.go:85-130`), which reads hex-encoded Ed25519 seeds from `{secretsDir}/{prefix}{tribunalID}_{appID}.key` files. Members whose keys cannot be resolved are included without a private key; they can participate in policy but cannot sign votes, and a warning is logged.

**Declarative tribunal seeding** (`--tribunal-bootstrap`): When the `--tribunal-bootstrap` flag is set to a JSON config file path, `bootstrapTribunalPolicy` (`internal/cli/serve/gateway.go`) seeds trusted signers and a TribunalPolicy at startup, before consensus validation runs. The config file contains `tribunal_id`, `member_app_ids`, `quorum`, and optional `seed_hex` (a hex-encoded Ed25519 seed). If `seed_hex` is provided, the corresponding public key is derived and registered as a trusted signer for each member. If omitted, a fresh key pair is generated. The TribunalPolicy is then created in the database. This is idempotent: if the tribunal already exists, the bootstrap is skipped. This enables deterministic demo deployments where the gateway and agent harness share the same Ed25519 seed.

**What is audited but NOT gated**:
- **L3 Notary proofs**: same behavior as doctrine: verified if present, recorded in receipt, but not required for mutations (`l4_warden.go:641-676`).

---

### Notary

**Configuration**: `--posture notary`

**Interface implementation** (`internal/services/governance/posture.go:65-70`):
- `RequiresL2Signature()` → `true`
- `RequiresL3Proof()` → `true`

**What is enforced (fail-closed)**:
- Everything from doctrine posture (L1, hash, nonce, expiry, state root, action type, payload decoding).
- Everything from consensus posture (all L2 checks: vote presence, signer store, tribunal store, tribunal policy, member validation, duplicate detection, signature verification, quorum).
- **L3 Notary proof verification for mutations**: the following checks are fail-closed gates under notary posture, but **only for mutation action types**:

  - **L3 proof presence**: For mutation actions, the envelope must include `L3Metadata` with a non-nil `Proof`. Missing proof → `ErrTxL3ProofMissing` (`l4_warden.go:646-650`).
  - **L3 notary configured**: If `l3Notary` is nil and the action is a mutation → `ErrTxL3NotaryNotConfigured` (`l4_warden.go:654-659`).
  - **L3 proof validity**: The `L3Notary.VerifyL3Proof` call must return `(true, nil)`. Any failure or error for a mutation → `ErrTxL3ProofInvalid` (`l4_warden.go:670-673`).

**Mutation classification** (`internal/constants/action_types.go:84-97`): The following action types are classified as mutations and require L3 proof under notary posture:
- `A2A_CALL`
- `CANCEL`
- `EXECUTE_BASH`
- `FILE_EDIT`
- `MCP_CALL`
- `RESTORE_FILE`
- `SHUTDOWN`

Non-mutation actions (e.g., `FS_READ`, `FS_LIST`, `FETCH_LOGS`, `HEARTBEAT`) do not require L3 proof even under notary posture. This is enforced by the `isMutation(actionType)` check that gates all L3 fail-closed paths (`l4_warden.go:647, 655, 670`).

**Startup validation**: Same as consensus posture — notary requires L2 signatures, so the gateway validates `tribunalID`, `TribunalPolicy` existence, and quorum at startup, and bootstraps the Tribunal service in-process (see consensus startup validation above).

**Tribunal bootstrap**: Same as consensus posture — `BootstrapTribunal` constructs the `TribunalService` and wires it as both the mTLS HTTP handler and the local deliberator via `SetTribunalDeliberator` (`internal/cli/serve/gateway.go`). The in-process `LocalDeliberator` signs L2 votes with the gateway's actuator key for single-member tribunals.

**Default for outbound mode**: Notary is the default posture for outbound (operator) mode (`internal/config/config.go:534`). This ensures that operators running in outbound mode require full L1/L2/L3 enforcement by default, since the L3Notary is nil and mutations will fail-closed with `ErrTxL3NotaryNotConfigured` unless an L3 notary is explicitly configured.

---

## Enforcement Point Summary

The posture is checked at the following code locations. Each check is a fail-closed gate when the posture requires it:

| Check | Code Location | Doctrine | Consensus | Notary |
|---|---|---|---|---|
| L1 Doctrine validation | `l4_warden.go:466-469` | **Enforced** | **Enforced** | **Enforced** |
| Transaction hash integrity (incl. L3 proof binding) | `l4_warden.go:477-493`, `internal/governance/envelope.go:107-118` | **Enforced** | **Enforced** | **Enforced** |
| Nonce replay protection | `l4_warden.go:325-359` | **Enforced** | **Enforced** | **Enforced** |
| Expiry enforcement | `l4_warden.go:334-341` | **Enforced** | **Enforced** | **Enforced** |
| State Merkle root validation | `l4_warden.go:500-526` | **Enforced** | **Enforced** | **Enforced** |
| Action type validation | `l4_warden.go:447-450` | **Enforced** | **Enforced** | **Enforced** |
| L2 vote presence | `l4_warden.go:550-554` | Audited | **Enforced** | **Enforced** |
| L2 signer store configured | `l4_warden.go:560-565` | Audited | **Enforced** | **Enforced** |
| L2 tribunal store configured | `l4_warden.go:568-573` | Audited | **Enforced** | **Enforced** |
| L2 tribunal policy exists + enabled | `l4_warden.go:576-590` | Audited | **Enforced** | **Enforced** |
| L2 duplicate signer detection | `l4_warden.go:604-609` | Audited | **Enforced** | **Enforced** |
| L2 signature verification | `l4_warden.go:611-623` | Audited | **Enforced** | **Enforced** |
| L2 quorum met | `l4_warden.go:630-636` | Audited | **Enforced** | **Enforced** |
| L3 proof present (mutations only) | `l4_warden.go:646-650` | Audited | Audited | **Enforced** |
| L3 notary configured (mutations only) | `l4_warden.go:654-659` | Audited | Audited | **Enforced** |
| L3 proof valid (mutations only) | `l4_warden.go:670-673` | Audited | Audited | **Enforced** |
| L2/L3 status in receipt | `l5_actuator.go:109-127` | Recorded | Recorded | Recorded |
| Startup: tribunal ID required | `config.go:292-293` | - | **Enforced** | **Enforced** |
| Startup: quorum >= 1 | `config.go:295-296` | - | **Enforced** | **Enforced** |
| Startup: tribunal policy exists + enabled | `internal/cli/serve/gateway.go` | - | **Enforced** | **Enforced** |
| Invalid posture name → panic | `posture.go:75-81` | **Enforced** | **Enforced** | **Enforced** |

**"Enforced"** = fail-closed gate; transaction is rejected if the check fails.
**"Audited"** = result is verified if present and recorded in the receipt, but does not gate execution.
**"Recorded"** = L2/L3 status is reflected in the `ActionReceipt` as `L2_STATUS_*` / `L3_STATUS_*` enum values.

---

## Transaction Process: End-to-End Flow

This section walks through the complete transaction process from initial intent to final execution and audit. The process is designed to ensure security, accountability, and sovereignty throughout.

### Phase 1: Intent Submission

#### Step 1: Principal Initiates Request

A **Principal** (human user or AI agent) submits an intent to perform an action. This intent can be submitted through multiple channels:

- **MCP client**: Using Claude, Cursor, Windsurf, or other MCP-compatible AI IDEs
- **Agentic ensemble**: Through A2A (Agent-to-Agent) protocols or tool calls
- **Native application**: Direct integration with g8e protocols

The intent represents what the principal wants to accomplish, for example, "read a file," "deploy a container," or "query a database."

#### Step 2: Producer Wraps Intent

The **Producer** (g8e-compatible agentic ensemble, BYO agent, or MCP client) receives the raw intent and begins the governance process:

1. **Reach Consensus (L2)**: The producer sends the envelope to the **TribunalService** (`internal/services/tribunal/service.go`) for deliberation. Each tribunal member independently evaluates the payload using the L1 Doctrine and signs an Ed25519 vote over `<transaction_hash>|<decision>`. The `Deliberate` method populates `L2Metadata` with the `tribunal_id` and the collected `L2Vote` set. For single-binary deployments, the `LocalDeliberator` adapter calls `TribunalService.Deliberate` in-process without an HTTP round-trip.
2. **Create GovernanceEnvelope**: The producer wraps the intent in a `GovernanceEnvelope`, which includes:
   - The original intent as a typed protobuf payload
   - Tribunal L2 votes proving consensus
   - L3 proof (WebAuthn assertion or mTLS certificate fingerprint)
   - Metadata about the request (timestamp, principal identity, nonce, state root)
   - Cryptographic proofs for verification

The signed envelope is now ready for submission to the governance gateway.

### Phase 2: Gateway Admission

#### Step 3: Envelope Submission to Gateway

The producer submits the signed `GovernanceEnvelope` to the **Governance Gateway (g8eg)**. The gateway serves as the Policy Decision Point and acts as the system's PKI authority.

The gateway accepts connections through:
- **HTTP/mTLS universal endpoint**: For MCP clients (Claude, Cursor, Windsurf)
- **Standard protocols**: For agentic ensembles and A2A communications

#### Step 4: Gateway Admission Control

The gateway performs initial admission checks on the envelope (`internal/services/gateway/gateway_auth.go`, `internal/services/gateway/governance_envelope.go`, `internal/services/gateway/replay_store_service.go`):

1. **mTLS enforcement**: The HTTPS port uses `tls.VerifyClientCertIfGiven`, accepting and verifying client certificates when present but not requiring them at the TLS layer. mTLS enforcement for protected routes happens at the application layer via `auth.Middleware()`, which checks client cert presence and validity for all routes not in the `PublicRouteRegistry`. Browser clients (console, WebAuthn flows) reach public routes without a client cert. The mTLS middleware extracts operator session IDs from certificate SPIFFE URI SANs and authenticates Operator, CLI, or App identities.
2. **Certificate revocation check**: The gateway verifies that the client certificate is not revoked via the PKI authority.
3. **Transport-to-envelope identity binding**: The `verifyEnvelopeIdentityBinding` function enforces that mTLS certificate URI SANs match envelope identity claims (`operator_session_id`, `operator_id`), preventing impersonation.
4. **Replay protection**: The `ReplayStoreService` atomically reserves nonces in SQLite to prevent replay attacks at the gateway level.
5. **Rate limiting**: The governance envelope submission endpoint (`/api/v1/governance/envelopes`) is rate-limited.

If the envelope passes admission, it is queued for processing. If it fails, the gateway rejects it immediately with a typed error and audit entry.

### Phase 3: Operator Retrieval

#### Step 5: Operator Establishes Connection

A **Governed Operator (g8eo)** running on a sovereign host establishes an outbound-only mTLS tunnel to the gateway. This is a critical security design:

- **Outbound-only**: The operator initiates the connection; the gateway cannot reach into the operator
- **mTLS encryption**: Mutual TLS ensures both ends authenticate each other
- **Policy Execution Point**: The operator is where policies are actually enforced

This design ensures that operators remain sovereign; they can pull work but cannot be pushed into from the gateway.

#### Step 6: Operator Fetches Pending Envelope

The operator polls the gateway for pending envelopes that are assigned to it (based on policy, capacity, or other routing logic). When it finds an envelope, it retrieves it over the secure mTLS tunnel.

The operator now has the signed `GovernanceEnvelope` and begins the verification pipeline.

### Phase 4: Verification Pipeline (L1-L5)

The operator runs the envelope through a five-layer verification pipeline orchestrated by the **Warden (L4)**. Each layer must pass; if any layer fails, the transaction fails closed (rejected with audit trail).

#### Step 7: Warden Pre-Dispatch Gate (L4)

The **L4 Warden** (`internal/services/governance/l4_warden.go`) is the primary orchestrator for the verification pipeline. It performs a five-stage verification sequence:

1. **In-flight tracking** (stage 0): Prevents the same nonce from being processed concurrently using a `sync.Map`.
2. **Nonce reservation and expiry** (stage 1): Atomically reserves the nonce in the **Replay Store** (durable SQLite storage) to prevent replay attacks even if the operator crashes mid-execution. Checks expiry relative to an injectable clock.
3. **Stateless validation** (stage 2): Structural checks (transaction ID, action type, payload presence), typed payload decoding, L1 Doctrine validation, and transaction hash recomputation and comparison. The hash is computed by `GenerateMessageID` in `pkg/governance/types.go` over the following fields in proto definition order: `action_type`, `target_resource`, `payload` (base64-encoded), `state_merkle_root`, `nonce`, `expires_at` (UTC RFC3339Nano), `intent_data` (canonicalized), `requestor_user_id`, and `acting_app_id`. L3 proof is intentionally NOT included in the transaction hash so that L2 (machine consensus) can sign the hash before L3 (human notary) is asked. Tamper-evidence for L3 is provided by `verifyL3Posture`, which checks the proof against `envelope.TransactionHash` at verification time.
4. **Stateful validation** (stage 3): State Merkle root validation against the operator's current state root.
5. **Posture validation** (stage 4): L2 and L3 checks gated by the configured `GovernancePosture`.

If any stage fails, the nonce reservation is released and the transaction is rejected with a typed error. The nonce remains reserved through successful verification and is finalized after execution completes.

#### Layer 1: Technical Bedrock (L1)

**Purpose**: Ensure the transaction doesn't violate fundamental technical or safety constraints.

**Checks** (handled by **L1 Doctrine** in `internal/services/governance/l1_doctrine.go`):
- **Protobuf Field Validation**: Reflects over protobuf message fields and checks the `forbidden_patterns` field option extension (defined in `protocol/proto/g8e/common/v1/common.proto`) against string values using regex matching.
- **MITRE-based Threat Detection**: Scans command strings (`CommandRequested`), MCP tool arguments (`McpCallRequested`), A2A payloads (`A2ACallRequested`), and file edit content (`FileEditRequested`) for known malicious patterns. Detectors cover data destruction (T1485, T1561.001), reverse shells, privilege escalation, and other attack categories with MITRE ATT&CK mappings.
- **Critical System File Protection**: The `isCriticalSystemFile` check blocks modifications to sensitive paths (e.g., `/etc/shadow`, `/etc/sudoers`).

**Outcome**:
- **Passed**: Proceeds to State Check
- **Violated**: Transaction fails closed with typed rejection and audit entry

#### State Check: Merkle Root Freshness

**Purpose**: Ensure the transaction is based on the current system state.

**Checks**:
- **Merkle root validation**: Compares the `StateMerkleRoot` in the envelope against the operator's **Canonical DB** state root.
- **Consistency verification**: Rejects stale transactions to prevent race conditions on shared state.

**Outcome**:
- **Fresh**: Proceeds to L2
- **Stale**: Transaction fails closed with typed rejection and audit entry

#### Layer 2: Consensus Verification (L2)

**Purpose**: Verify that the transaction has proper tribunal consensus.

**Checks** (performed by `verifyL2Posture` in the L4 Warden; see [L2 Consensus Verification Detail](#l2-consensus-verification-detail) and [Tribunal](#tribunal) for full details):
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

#### Layer 3: Authorization (L3)

**Purpose**: Ensure the principal is authorized and present for the action.

**L3 Notary implementations** (see [L3 Notary Verification Detail](#l3-notary-verification-detail) for full details):

- **PasskeyService** (`internal/services/gateway/passkey_service.go`): Gateway mode for web sessions. Verifies WebAuthn assertions using the `transaction_hash` as the challenge. Validates credential ID, client data JSON, authenticator data, and signature against registered passkey credentials. Implements the `L3Notary` interface directly and is injected as the `passkeyVerifier` in `NewGatewayL3Notary`.

- **NewOutboundL3Notary** (`internal/services/governance/l3_notary.go`): Outbound mode for operator-side approval. Verifies that a transaction exists in the `SuspendedTransactionStore`, is marked as approved, has a valid CLI signature over the transaction hash, and has not exceeded the 30-minute approval window. No CLI session or certificate revocation checks are performed in this mode.

- **NewCLIL3Notary** (`internal/services/governance/l3_notary.go`): Gateway mode for CLI sessions. Extends outbound mode with a `CLISessionVerifier` that checks user active status, session ownership, certificate fingerprint match, session expiry, and certificate revocation via the PKI authority. The `CLISessionVerifier` interface is implemented by `cliSessionVerifier` in `internal/services/gateway/cli_session_verifier.go`.

- **NewGatewayL3Notary** (`internal/services/governance/l3_notary.go`): Unified gateway mode that requires passkey authorization for all proofs. `gatewayNotary.VerifyL3Proof` always requires a `credential_id` and delegates to the injected `passkeyVerifier` (PasskeyService) first. If the proof includes `mtls_cert_fingerprint` (CLI caller), CLI mTLS session verification runs as an additional transport-auth layer. This is the constructor used in `GetGovernanceDeps` (`internal/services/gateway/gateway_service.go`) for gateway-mode deployments.

- **VerifyCLICertificate** (`internal/services/gateway/cli_cert.go`): Standalone function for real-time mTLS certificate validation during request authentication, distinct from the L3 notary verification path.

**Posture behavior**:
- **doctrine/consensus**: L3 results are recorded for audit but do not gate execution.
- **notary**: L3 proof is a fail-closed gate for mutation actions.

**Mutation enforcement**: The `isMutation` check determines whether an action type is state-changing. Only mutations require L3 proof under the notary posture.

**No bypass field**: L3 is satisfied only by a verified proof. There is no `AutoApproved` or equivalent bypass; the Warden re-derives whether L3 is required from the action type and posture, and if required, demands a real proof. Out-of-band approvals use the `outboundNotary` + `SuspendedTransactionStore` path with a verifiable CLI signature, not a producer-supplied flag.

**Outcome**:
- **Authorized**: Proceeds to L5 (Actuator)
- **Denied**: Transaction fails closed with typed rejection and audit entry

### Phase 5: Execution

#### Layer 5: Actuator (L5)

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

### Phase 6: Audit and Completion

#### Step 8: Local Audit Vault Logging

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

#### Step 9: Receipt Return to Gateway

The operator returns the sovereignty-scrubbed signed receipt to the gateway. In synchronous gateway mode, the `ProcessEnvelope` method (`internal/services/pubsub/pubsub_commands.go`) returns the receipt directly to the HTTP caller. In outbound mode, the receipt is pushed over the mTLS tunnel. The receipt:

- Confirms successful execution (or captures the failure)
- Provides the results (if authorized for the principal)
- Maintains the audit trail for the entire pipeline
- Contains no sensitive host data

The receipt is returned even on execution failure (status=FAILED) so callers receive cryptographic evidence of the attempt. A nil receipt is only returned when verification fails before execution begins.

#### Step 10: Gateway Returns Final Output

The gateway receives the receipt and returns the final safe output to the principal:

- **Success case**: Returns the execution results.
- **Failure case**: Returns the typed rejection with explanation and receipt evidence.
- **Audit reference**: Provides a reference to the audit entry for traceability.

The principal now has confirmation of the transaction outcome.

---

## L2 Consensus Verification Detail

### L2 Signature Verification

The L4 Warden verifies L2 votes in `verifyL2Posture` (`l4_warden.go:549-639`). The verification is identical regardless of posture; the posture only determines whether a failure rejects the transaction or is merely recorded.

**Signature format**: Ed25519 over `<transaction_hash>|<decision>` (boolean string). Verified by `verifyL2Signature` in the L4 Warden.

---

## Tribunal

The Tribunal is the enrolled agentic application that deliberates on governance envelopes and produces L2 consensus votes. It is the core component of the L2 Consensus verification layer. Each member is a distinct enrolled principal with its own Ed25519 key, registered as a `TrustedSigner` in the gateway's `SignerStore`.

### TribunalPolicy

The `TribunalPolicy` (`internal/models/auth.go:502-510`) defines a named consensus body with the following fields:

| Field | Type | Description |
|---|---|---|
| `ID` | `string` | Tribunal name/identifier. Must be non-empty, alphanumeric + hyphens + underscores only. |
| `MemberAppIDs` | `[]string` | List of member App IDs. Each must correspond to an enabled `TrustedSigner`. No duplicates. |
| `Quorum` | `int` | Minimum number of affirmative distinct signatures required. Must be >= 1 and <= member count. Consensus posture enforces >= 1 at startup. |
| `RequireDistinct` | `bool` | If true, duplicate `signer_key_id` values in a vote set are rejected. |
| `Enabled` | `bool` | Whether the tribunal is active. New tribunals must be created with `Enabled=true`. |
| `CreatedAt` | `time.Time` | Creation timestamp (auto-set). |
| `UpdatedAt` | `time.Time` | Last update timestamp (auto-set). |

### TribunalMember

Each `TribunalMember` (`internal/services/tribunal/member.go:26-32`) represents a single member identity in the Tribunal:

- **`AppID`**: The member's enrolled application ID. This is the same ID used to look up the member's trusted public key in the `SignerStore`.
- **`PrivateKey`**: The member's Ed25519 private key, used to sign consensus votes. Members without a private key (key resolution failed during bootstrap) are included in the member list for policy purposes but cannot sign votes; their votes are skipped during deliberation.

Members never share the gateway identity key. Even in single-member tribunals, the member is a distinct principal; the actuator's Ed25519 key is reused as the member signing key (Option C), but the member App ID remains separate from the gateway's actuator key ID.

### TribunalService

The `TribunalService` (`internal/services/tribunal/service.go:32-44`) is the core deliberation engine. It holds:

- **`tribunalID`**: The `TribunalPolicy.ID` this service represents.
- **`members`**: The list of `TribunalMember` structs, each with their private key (if available).
- **`doctrine`**: The shared `L1Doctrine` instance used for deterministic safety evaluation by all members.
- **`logger`** and **`responder`**: Infrastructure for logging and HTTP response writing.

#### Deliberation Flow

The `Deliberate` method (`tribunal/service.go:78-126`) processes a `GovernanceEnvelope` through all tribunal members:

1. **Hash verification**: Recompute the transaction hash via `governance.GenerateMessageID(env)` and verify it matches `envelope.id`. Mismatch → `ErrTribunalHashMismatch` (fail-closed).
2. **Command extraction**: Extract command data and intent from the envelope payload via `extractCommandData()` (`tribunal/member.go:60-76`). If `IntentData` is present, it is marshaled to JSON; otherwise the raw payload bytes are used.
3. **Per-member safety evaluation**: Each member with a non-nil `PrivateKey` independently evaluates safety via `evaluateSafety()` (`tribunal/member.go:39-41`):
   - **MITRE checks**: `L1Doctrine.AnalyzeCommand()` scans the command data for malicious patterns. If any signal has `BlockRecommended = true`, the payload is unsafe.
   - **Fail-closed on nil doctrine**: If doctrine is nil, `runMITREChecks` returns `false` (unsafe) (`tribunal/member.go:46-48`). This ensures that a misconfigured tribunal cannot approve transactions without doctrine validation.
4. **Vote signing**: Each member signs `<transaction_hash>|<decision>` with Ed25519 (`tribunal/member.go:81-88`, implemented in `signDecision`). The decision is a boolean (`true` = safe, `false` = unsafe).
5. **Vote collection**: Votes are collected into `L2Metadata.Votes` with `tribunal_id` set to the service's tribunal ID. The envelope's `Governance.L2` metadata is populated with the votes.

#### HTTP Handler

The `HandleDeliberate` method (`tribunal/service.go:131-173`) is the mTLS-guarded HTTP handler for `POST /tribunal/v1/deliberate`. It:

- Accepts a canonical-JSON `GovernanceEnvelope` (max 1 MiB body).
- Unmarshals via `protojson.Unmarshal`.
- Calls `Deliberate` to produce L2 votes.
- Returns the signed envelope with L2 metadata populated as `protojson` with `Content-Type: application/json` and `X-Content-Type-Options: nosniff` headers.

Errors are mapped to HTTP status codes: `ErrTribunalHashMismatch` → 400, other deliberation failures → 500.

### Key Providers

The `KeyProvider` interface (`internal/services/tribunal/factory.go:31-36`) resolves Ed25519 private keys for tribunal members by App ID. This abstraction allows keys to be sourced from disk, in-process, or any secure backing store.

#### FileKeyProvider

`FileKeyProvider` (`internal/services/tribunal/factory.go:85-130`) loads Ed25519 private keys from disk. Each member's key is stored as a hex-encoded Ed25519 seed in a file named `{prefix}{tribunalID}_{memberAppID}.key` within the secrets directory, where `prefix` is `constants.SecretsFileTribunalMemberKeyPrefix`. Key files are created with `0600` permissions.

The `GetMemberKey` method:
1. Constructs the file path: `filepath.Join(secretsDir, prefix + tribunalID + "_" + appID + ".key")`.
2. Reads the hex-encoded seed from the file.
3. Decodes the hex string and validates the seed length is `ed25519.SeedSize` (32 bytes).
4. Returns `ed25519.NewKeyFromSeed(seed)`.

Errors are returned for missing files, invalid hex, or wrong seed length.

#### SaveMemberKey

`SaveMemberKey` (`internal/services/tribunal/factory.go:134-150`) writes an Ed25519 private key seed to disk for member provisioning. It creates the secrets directory (with `0700` permissions) if it does not exist, encodes the key seed as hex, and writes it with `0600` file permissions.

#### Bootstrap Key Resolution

During `BootstrapTribunal` (`internal/cli/serve/gateway.go:465-492`), a composite `KeyProvider` is constructed:

1. **File-based lookup**: First attempts to load the member key from disk via `FileKeyProvider`.
2. **Actuator fallback**: If the file lookup fails and the member App ID matches the actuator's key ID, the actuator's Ed25519 private key is used (Option C for single-member tribunals).
3. **Failure**: If neither source resolves, the member is included without a private key and a warning is logged. The member can participate in policy but cannot sign votes.

### Tribunal Factory

`NewTribunalFromPolicy` (`internal/services/tribunal/factory.go:45-83`) is the shared factory that constructs a `TribunalService` from a `TribunalPolicy` and a `KeyProvider`. It:

1. Validates that `policy` and `keyProvider` are non-nil (fail-closed with `ErrTribunalFactoryNilPolicy` / `ErrTribunalFactoryNilKeyProvider`).
2. Iterates over `policy.MemberAppIDs`, resolving each member's private key via the `KeyProvider`.
3. If key resolution fails, logs a warning and includes the member with a nil `PrivateKey`.
4. Constructs and returns a `TribunalService` with the resolved members, shared doctrine, logger, and responder.

This factory is used by both production bootstrap (`BootstrapTribunal` in `internal/cli/serve/gateway.go`) and test fixtures (`SetupTribunal` in `test/fixtures/gateway_fixture.go`), eliminating code duplication.

### Tribunal Store Service

The `TribunalStoreService` (`internal/services/gateway/tribunal_store_service.go:29-35`) provides CRUD operations for `TribunalPolicy` records, backed by the SQLite document store. It wraps a `DocumentStoreService` for persistence and a `SignerStoreService` for member validation.

#### Operations

- **`GetTribunal(id)`**: Retrieves a `TribunalPolicy` by ID from the `tribunals` collection. Returns `(nil, nil)` if not found.
- **`AddTribunal(policy)`**: Creates or updates a tribunal policy with fail-closed validation (see below).
- **`ListTribunals()`**: Returns all tribunal policies, ordered by ID.
- **`DeleteTribunal(id)`**: Removes a tribunal policy. Returns `(false, nil)` if not found.

#### AddTribunal Validation

The `AddTribunal` method (`tribunal_store_service.go:79-148`) enforces the following constraints at write time:

- **Tribunal ID**: Non-empty, alphanumeric + hyphens + underscores only (`isValidTribunalID`).
- **Member list**: Non-empty, no empty strings, no duplicate member IDs.
- **Quorum**: >= 1 and <= member count.
- **Trusted signer verification**: Every `MemberAppID` must resolve to an enabled `TrustedSigner` in the `SignerStore`.
- **Existence check**: New tribunals must be created with `Enabled=true` (`ErrTribunalMustBeEnabled`). Existing tribunals may only be updated via `Enabled=false` (disable path). Overwriting an existing enabled tribunal with `Enabled=true` is rejected (`ErrAlreadyExists`).

### Admin API Endpoints

Tribunal policies are managed via admin-only REST endpoints in `AdminController` (`internal/services/gateway/admin_controller.go`):

| Method | Path | Description |
|---|---|---|
| `POST` | `/api/v1/admin/tribunals` | Create a new tribunal policy. Body: `TribunalPolicy` JSON. |
| `GET` | `/api/v1/admin/tribunals` | List all tribunal policies. |
| `DELETE` | `/api/v1/admin/tribunals/{id}` | Delete a tribunal policy by ID. |

All endpoints require an authenticated bootstrap user (admin-only). Non-bootstrap users receive 403 Forbidden.

### LocalDeliberator

The `LocalDeliberator` (`tribunal/service.go:175-204`) is an in-process adapter that satisfies the `mcp.TribunalDeliberator` interface by calling `TribunalService.Deliberate` directly, without an HTTP round-trip. This is used when the Tribunal runs in the same process as the gateway (single-binary deployment).

The `Deliberate` method:
1. Unmarshals the envelope bytes via `protojson.Unmarshal`.
2. Calls `TribunalService.Deliberate` to produce L2 votes.
3. Marshals the result envelope via `protojson` (single-line mode) and returns the bytes.

### Tribunal Bootstrap at Gateway Startup

When the gateway starts in consensus or notary posture, `BootstrapTribunal` (`internal/cli/serve/gateway.go:465-492`) is called to construct and wire the Tribunal service. Under doctrine posture, the Tribunal is not constructed in-process; L2 votes must be obtained from an external Tribunal service when required.

1. **Load policy**: Retrieves the `TribunalPolicy` from the database via `TribunalStore.GetTribunal(tribunalID)`. If the policy is missing, returns `ErrTxL2TribunalNotConfigured`.
2. **Construct key provider**: Creates a `FileKeyProvider` for the configured secrets directory, then wraps it with the actuator key fallback logic (see [Bootstrap Key Resolution](#bootstrap-key-resolution)).
3. **Build service**: Calls `NewTribunalFromPolicy` with the policy, composite key provider, L1 doctrine, logger, and response writer.
4. **Wire handlers**: The resulting `TribunalService` is registered as both:
   - The mTLS HTTP handler for `POST /tribunal/v1/deliberate` (remote deliberation via `HandleDeliberate`).
   - The local deliberator for in-process calls (via `NewLocalDeliberator`).

---

## L3 Notary Verification Detail

### L3 Notary Implementations

The `L3Notary` interface (`internal/services/governance/l3_notary.go:35-39`) is implemented by three structs, configured through three constructors:

- **NewOutboundL3Notary** (`internal/services/governance/l3_notary.go:80-86`): Constructs an `outboundNotary` (line 66). Operator-side approval. Verifies the transaction exists in `SuspendedTransactionStore`, is marked approved, has a valid Ed25519 signature over the transaction hash, matches the expected certificate fingerprint, and is within the 30-minute approval window.
- **NewCLIL3Notary** (`internal/services/governance/l3_notary.go:88-97`): Constructs a `cliNotary` (line 74). Gateway CLI mode. Calls the shared `verifyOutboundProof` function with a `CLISessionVerifier` that checks user active status, CLI session ownership, certificate fingerprint match, session expiry, and certificate revocation before the suspended-transaction and signature verification.
- **NewGatewayL3Notary** (`internal/services/governance/l3_notary.go:99-108`): Constructs a `gatewayNotary` (line 57). Unified gateway mode. Requires passkey authorization for all proofs; `credential_id` must be non-empty or `ErrPasskeyProofRequired` is returned. Passkey verification runs first via the injected `passkeyVerifier`. If `mtls_cert_fingerprint` is present, CLI mTLS session verification runs as an additional transport-auth layer.

The passkey verifier is **PasskeyService** (`internal/services/gateway/passkey_service.go`), which implements `L3Notary` for WebAuthn assertion verification. It uses `transaction_hash` as the challenge and verifies the assertion against the registered passkey credentials for the user.

### L3 and Mutations

L3 fail-closed enforcement applies **only to mutation action types** (`l4_warden.go:647, 655, 670`). The `isMutation` check (`internal/constants/action_types.go:84-97`) classifies the following as mutations:

| Action Type | Mutation |
|---|---|
| `A2A_CALL` | Yes |
| `CANCEL` | Yes |
| `EXECUTE_BASH` | Yes |
| `FILE_EDIT` | Yes |
| `MCP_CALL` | Yes |
| `RESTORE_FILE` | Yes |
| `SHUTDOWN` | Yes |
| `FS_LIST` | No |
| `FS_READ` | No |
| `FS_GREP` | No |
| `PORT_CHECK` | No |
| `FETCH_LOGS` | No |
| `FETCH_HISTORY` | No |
| `FETCH_FILE_HISTORY` | No |
| `FETCH_FILE_DIFF` | No |
| `EVAL_ANSWER` | No |
| `MCP_RESOURCE_LIST` | No |
| `MCP_RESOURCE_READ` | No |
| `MCP_PROMPT_LIST` | No |
| `MCP_PROMPT_GET` | No |
| `HEARTBEAT` | No |
| `INVESTIGATION_CREATE` | No |

Non-mutation actions never require L3 proof, even under notary posture.

---

## Posture Selection and Defaults

| Mode | Default Posture | Configured Via |
|---|---|---|
| Gateway mode | `doctrine` | `--posture` flag; defaults to `PostureDoctrine` (`config.go:398`) |
| Outbound (operator) mode | `notary` | Defaults to `PostureNotary` (`config.go:534`) |

**Posture selection**: The `doctrine` and `consensus` postures allow mutations to execute without human authorization (L3 proof) or multi-party consensus, below the floor defined in the position paper (§12). Choosing such a posture is itself an act of human intent; the gateway logs a warning at startup and proceeds. The `--posture doctrine` or `--posture consensus` flag is the authorization.

**Invalid posture handling**: `NewGovernancePosture()` panics on unrecognized posture names (`posture.go:75-81`). This is intentional; misconfigured deployments fail at startup rather than silently running under a weaker posture. `ParseGovernancePosture()` returns an error for CLI flag validation (`posture.go:86-97`).

---

## Receipt Metadata

The L5 Actuator records posture enforcement results in every `ActionReceipt` (`l5_actuator.go:109-127`):

| Posture | L2Status | L3Status |
|---|---|---|
| Doctrine | `L2_STATUS_NOT_REQUIRED` | `L3_STATUS_NOT_REQUIRED` |
| Consensus | `L2_STATUS_REQUIRED_VALID` or `L2_STATUS_REQUIRED_FAILED` | `L3_STATUS_NOT_REQUIRED` |
| Notary | `L2_STATUS_REQUIRED_VALID` or `L2_STATUS_REQUIRED_FAILED` | `L3_STATUS_REQUIRED_VALID` or `L3_STATUS_REQUIRED_FAILED` |

These values are part of the canonical receipt JSON (`l5_actuator.go:241-254`) and are signed by the actuator's Ed25519 key. They are also persisted in the `ActionReceiptRecord` in the SQL audit store (`l5_actuator.go:297-319`).

---

## Security Properties

Throughout the transaction process, several key security properties are maintained:

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
| **L3 Notary** | Authorization Engine | Three structs in `l3_notary.go`: `gatewayNotary` (line 57, via `NewGatewayL3Notary`), `cliNotary` (line 74, via `NewCLIL3Notary`), `outboundNotary` (line 66, via `NewOutboundL3Notary`). `gatewayNotary` requires passkey first, then CLI session as additional layer. `cliNotary` and `outboundNotary` use shared `verifyOutboundProof` with suspended transaction and Ed25519 signature verification. |
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

---

## Related Documentation

- [Gateway Architecture](./gateway.md): Gateway mode, MCP endpoints, and 5-layer verification sequence.
- [Encryption](./encryption.md): Cryptographic primitives used throughout the pipeline.
