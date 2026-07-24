# Governance

Last Updated: 2026-07-24
Version: v1.6.2

## Overview

The g8e system implements a five-layer verification pipeline (L1-L5) that governs every transaction. Transactions flow from AI clients through a governance gateway to governed operators, where they undergo verification before execution on target systems. The pipeline is governed by a configurable **GovernancePosture** that determines which layers are enforced as fail-closed gates versus audited only.

The posture is set at startup via `--posture <doctrine|consensus|notary>` and cannot be changed at runtime. The posture is queried at two enforcement points:

1. **L4 Warden**: gates transaction dispatch based on L2/L3 verification results.
2. **L5 Actuator**: records L2/L3 status in the signed action receipt.

A third check point exists at startup:

3. **Gateway startup**: logs advisory warnings for consensus and notary postures if tribunal prerequisites are not yet configured. The gateway boots regardless; L2 enforcement happens at transaction time via L4Warden.

### GovernanceEnvelope

The canonical transaction container is the **GovernanceEnvelope**, a protobuf message that binds identity, intent, state, and governance proofs into a single transaction. It carries:

- **Identity fields**: operator ID, session IDs (operator, web, CLI), requestor user ID, acting app ID
- **Intent fields**: event type, payload (typed protobuf bytes), action type, target resource, intent data
- **State and replay protection**: state Merkle root, nonce, transaction hash, protocol version
- **Governance proofs**: L1 metadata, L2 metadata (tribunal votes), L3 metadata (WebAuthn or mTLS proof)
- **Application context**: case ID, investigation ID, task ID, system fingerprint, tenant ID, binding persona

---

## Posture Definitions

### Doctrine (default)

**Configuration**: `--posture doctrine`

**What is enforced (fail-closed, all postures)**:
- **L1 Doctrine validation**: forbidden pattern checks and MITRE-based threat detection. Any violation rejects the transaction.
- **Transaction hash integrity**: the envelope ID and transaction hash must both match the recomputed hash.
- **Nonce replay protection**: nonces are atomically reserved in durable storage before any further checks.
- **Expiry enforcement**: expired transactions are rejected.
- **State Merkle root validation**: the envelope state root must match the current state root.
- **Action type validation**: unknown action types are rejected.
- **Payload decoding**: payloads must decode to the correct protobuf type for the action.

**What is audited but NOT gated**:
- **L2 Consensus votes**: if L2 votes are present, they are verified and the result is recorded in the receipt, but a missing or invalid L2 does not reject the transaction.
- **L3 Notary proofs**: if an L3 proof is present, it is verified and the result is recorded, but a missing or invalid L3 does not reject the transaction, even for mutations.

**Default posture**: Doctrine is the default for gateway mode when no `--posture` flag is provided. Outbound (operator) mode defaults to notary.

---

### Consensus

**Configuration**: `--posture consensus`

**What is enforced (fail-closed)**:
- Everything from doctrine posture (L1, hash, nonce, expiry, state root, action type, payload decoding).
- **L2 Consensus signature verification**: the following checks are all fail-closed gates under consensus posture:
  - **Vote presence**: the envelope must include L2 metadata with at least one vote.
  - **Signer store configured**: the trusted signer store must be available.
  - **Tribunal store configured**: the tribunal policy store must be available.
  - **Tribunal policy exists and is enabled**: the tribunal policy for the L2 tribunal ID must exist and be enabled.
  - **Member validation**: votes from signer key IDs not in the tribunal policy's member list are silently excluded from quorum count.
  - **Duplicate signer detection**: if the policy requires distinct signers, duplicate signer key IDs are rejected.
  - **Signature verification**: each vote's Ed25519 signature over `<transaction_hash>|<decision>` is verified against the trusted public key. Invalid signatures are excluded from quorum count.
  - **Quorum check**: the number of affirmative votes from valid, distinct members must meet or exceed the policy's quorum threshold.

**Startup validation**: The gateway logs advisory warnings at startup for consensus and notary postures if the tribunal is not yet configured:
- If the tribunal ID is empty, a warning is logged.
- If the tribunal policy does not exist or is disabled, a warning is logged.
- L2 enforcement happens at transaction time via L4Warden; L2-gated transactions are rejected until a tribunal is properly enrolled.
- If the tribunal ID is set and the policy exists and is enabled, the Tribunal service is bootstrapped in-process and wired as both the mTLS HTTP handler and the local deliberator.

**Tribunal bootstrap**: The Tribunal service is constructed from the tribunal policy stored in the database. For single-member tribunals, the gateway's actuator Ed25519 key is reused as the member signing key. For multi-member tribunals, member keys are loaded from disk. Members whose keys cannot be resolved are included without a private key; they can participate in policy but cannot sign votes, and a warning is logged.

**Declarative tribunal seeding** (`--tribunal-bootstrap`): When the `--tribunal-bootstrap` flag is set to a JSON config file path, the gateway seeds trusted signers and a tribunal policy at startup, before consensus validation runs. The config file contains `tribunal_id`, `member_app_ids`, `quorum`, and optional `seed_hex` (a hex-encoded Ed25519 seed). If `seed_hex` is provided, the corresponding public key is derived and registered as a trusted signer for each member. If omitted, a fresh key pair is generated. This is idempotent: if the tribunal already exists, the bootstrap is skipped. This enables deterministic demo deployments where the gateway and agent harness share the same Ed25519 seed.

**What is audited but NOT gated**:
- **L3 Notary proofs**: same behavior as doctrine. Verified if present, recorded in receipt, but not required for mutations.

---

### Notary

**Configuration**: `--posture notary`

**What is enforced (fail-closed)**:
- Everything from doctrine posture (L1, hash, nonce, expiry, state root, action type, payload decoding).
- Everything from consensus posture (all L2 checks: vote presence, signer store, tribunal store, tribunal policy, member validation, duplicate detection, signature verification, quorum).
- **L3 Notary proof verification for mutations**: the following checks are fail-closed gates under notary posture, but only for mutation action types:
  - **L3 proof presence**: for mutation actions, the envelope must include L3 metadata with a non-nil proof.
  - **L3 notary configured**: the L3 notary must be available.
  - **L3 proof validity**: the L3 proof verification must succeed. Any failure for a mutation rejects the transaction.

**Mutation classification**: The following action types are classified as mutations and require L3 proof under notary posture:
- `A2A_CALL`
- `CANCEL`
- `EXECUTE_BASH`
- `FILE_EDIT`
- `MCP_CALL`
- `RESTORE_FILE`
- `SHUTDOWN`

Non-mutation actions (e.g., `FS_READ`, `FS_LIST`, `FETCH_LOGS`, `HEARTBEAT`) do not require L3 proof even under notary posture.

**Startup validation**: Same as consensus posture; advisory warnings if tribunal is not configured. L2 enforcement happens at transaction time via L4Warden. If the tribunal ID is set and the policy exists and is enabled, the Tribunal service is bootstrapped in-process.

**Tribunal bootstrap**: Same as consensus posture. The Tribunal service is constructed and wired as both the mTLS HTTP handler and the local deliberator. The in-process local deliberator signs L2 votes with the gateway's actuator key for single-member tribunals.

**Default for outbound mode**: Notary is the default posture for outbound (operator) mode. This ensures that operators running in outbound mode require full L1/L2/L3 enforcement by default. Since the L3 notary is nil in outbound mode, mutations fail-closed unless an L3 notary is explicitly configured.

---

## Enforcement Point Summary

| Check | Doctrine | Consensus | Notary |
|---|---|---|---|
| L1 Doctrine validation | **Enforced** | **Enforced** | **Enforced** |
| Transaction hash integrity | **Enforced** | **Enforced** | **Enforced** |
| Nonce replay protection | **Enforced** | **Enforced** | **Enforced** |
| Expiry enforcement | **Enforced** | **Enforced** | **Enforced** |
| State Merkle root validation | **Enforced** | **Enforced** | **Enforced** |
| Action type validation | **Enforced** | **Enforced** | **Enforced** |
| L2 vote presence | Audited | **Enforced** | **Enforced** |
| L2 signer store configured | Audited | **Enforced** | **Enforced** |
| L2 tribunal store configured | Audited | **Enforced** | **Enforced** |
| L2 tribunal policy exists + enabled | Audited | **Enforced** | **Enforced** |
| L2 duplicate signer detection | Audited | **Enforced** | **Enforced** |
| L2 signature verification | Audited | **Enforced** | **Enforced** |
| L2 quorum met | Audited | **Enforced** | **Enforced** |
| L3 proof present (mutations only) | Audited | Audited | **Enforced** |
| L3 notary configured (mutations only) | Audited | Audited | **Enforced** |
| L3 proof valid (mutations only) | Audited | Audited | **Enforced** |
| L2/L3 status in receipt | Recorded | Recorded | Recorded |
| Startup: tribunal ID required | - | Advisory | Advisory |
| Startup: quorum >= 1 | - | Advisory | Advisory |
| Startup: tribunal policy exists + enabled | - | Advisory | Advisory |
| Invalid posture name | **Enforced** | **Enforced** | **Enforced** |

**Enforced** = fail-closed gate; transaction is rejected if the check fails.
**Audited** = result is verified if present and recorded in the receipt, but does not gate execution.
**Recorded** = L2/L3 status is reflected in the action receipt as enum values.
**Advisory** = a warning is logged at startup but the gateway boots regardless; enforcement happens at transaction time via L4Warden.

### Posture Enforcement Matrix

| Posture | L1 Doctrine | L2 Consensus | L3 Notary |
|---|---|---|---|
| Doctrine | **Enforced** | Audited | Audited |
| Consensus | **Enforced** | **Enforced** | Audited |
| Notary | **Enforced** | **Enforced** | **Enforced** |

---

## Transaction Process: End-to-End Flow

This section walks through the complete transaction process from initial intent to final execution and audit. The process is designed to ensure security, accountability, and sovereignty throughout.

### Phase 1: Intent Submission

#### Step 1: Principal Initiates Request

A **Principal** (human user or AI agent) submits an intent to perform an action. This intent can be submitted through multiple channels:

- **MCP client**: Using Claude Code, Codex, Goose, Gemini CLI, or other MCP-compatible AI agents
- **Agentic ensemble**: Through A2A (Agent-to-Agent) protocols or tool calls
- **Native application**: Direct integration with g8e protocols

The intent represents what the principal wants to accomplish, for example, "read a file," "deploy a container," or "query a database."

#### Step 2: Producer Wraps Intent

The **Producer** (g8e-compatible agentic ensemble, BYO agent, or MCP client) receives the raw intent and begins the governance process:

1. **Create GovernanceEnvelope**: The producer wraps the intent in a GovernanceEnvelope, which includes:
   - The original intent as a typed protobuf payload
   - Metadata about the request (timestamp, principal identity, nonce, state root)
   - Cryptographic proofs for verification

2. **L2 Consensus (gateway-mediated)**: Under `consensus` and `notary` postures, the gateway's `processGatewayTransaction` automatically sends the envelope to the Tribunal for L2 deliberation before dispatch. Each tribunal member independently evaluates the payload using the L1 Doctrine and signs an Ed25519 vote over `<transaction_hash>|<decision>`. For single-binary deployments, the local deliberator calls the Tribunal Service in-process without an HTTP round-trip. Under `doctrine` posture, L2 votes are not required (audited only) and the Tribunal is not constructed in-process.

3. **L3 Notary proof**: The envelope may include an L3 proof (WebAuthn assertion or mTLS certificate fingerprint) if the producer can generate one. However, standard AI clients (Claude Code, Codex, Goose, Gemini CLI) typically cannot generate an L3 Notary human signature. In this case, the gateway suspends the transaction, sends an OOB WebAuthn challenge URL to the client, the human approves via browser, and the gateway attaches the resulting proof to the envelope before resuming the L4/L5 flow (see [OOB Suspension](#out-of-band-oob-suspension--webauthn-approval-flow) in [Gateway Architecture](./gateway.md)).

The signed envelope is now ready for submission to the governance gateway.

### Phase 2: Gateway Admission

#### Step 3: Envelope Submission to Gateway

The producer submits the signed GovernanceEnvelope to the **Governance Gateway**. The gateway serves as the Policy Decision Point and acts as the system's PKI authority.

The gateway accepts connections through:
- **HTTP/mTLS universal endpoint**: For MCP clients (Claude Code, Codex, Goose, Gemini CLI)
- **Standard protocols**: For agentic ensembles and A2A communications

#### Step 4: Gateway Admission Control

The gateway performs initial admission checks on the envelope:

1. **mTLS enforcement**: The HTTPS port accepts and verifies client certificates when present but does not require them at the TLS layer. mTLS enforcement for protected routes happens at the application layer, which checks client cert presence and validity for all non-public routes. Browser clients (console, WebAuthn flows) reach public routes without a client cert. The mTLS middleware extracts operator session IDs from certificate SPIFFE URI SANs and authenticates Operator, CLI, or App identities.
2. **Certificate revocation check**: The gateway verifies that the client certificate is not revoked via the PKI authority.
3. **Transport-to-envelope identity binding**: The gateway enforces that mTLS certificate URI SANs match envelope identity claims (operator session ID, operator ID), preventing impersonation.
4. **Rate limiting**: The governance envelope submission endpoint is rate-limited.

If the envelope passes admission, it is queued for processing. If it fails, the gateway rejects it immediately with a typed error and audit entry.

### Phase 3: Operator Retrieval

#### Step 5: Operator Establishes Connection

A **Governed Operator** running on a sovereign host establishes an outbound-only mTLS tunnel to the gateway. This is a critical security design:

- **Outbound-only**: The operator initiates the connection; the gateway cannot reach into the operator
- **mTLS encryption**: Mutual TLS ensures both ends authenticate each other
- **Policy Execution Point**: The operator is where policies are actually enforced

This design ensures that operators remain sovereign; they can pull work but cannot be pushed into from the gateway.

#### Step 6: Operator Fetches Pending Envelope

The operator polls the gateway for pending envelopes that are assigned to it (based on policy, capacity, or other routing logic). When it finds an envelope, it retrieves it over the secure mTLS tunnel.

The operator now has the signed GovernanceEnvelope and begins the verification pipeline.

### Phase 4: Verification Pipeline (L1-L5)

The operator runs the envelope through a five-layer verification pipeline orchestrated by the **Warden (L4)**. This pipeline runs on both the Gateway's in-process Operator (for operations targeting the gateway host) and on each remote Governed Operator (for operations targeting their own hosts). Each layer must pass; if any layer fails, the transaction fails closed (rejected with audit trail).

#### Step 7: Warden Pre-Dispatch Gate (L4)

The **L4 Warden** is the primary orchestrator for the verification pipeline. It performs a five-stage verification sequence:

1. **In-flight tracking** (stage 0): Prevents the same nonce from being processed concurrently.
2. **Nonce reservation and expiry** (stage 1): Atomically reserves the nonce in durable storage to prevent replay attacks even if the operator crashes mid-execution. Checks expiry relative to an injectable clock.
3. **Stateless validation** (stage 2): Structural checks (transaction ID, action type, payload presence), typed payload decoding, L1 Doctrine validation, and transaction hash recomputation and comparison. The hash is computed over the following fields in proto definition order: action type, target resource, payload (base64-encoded), state Merkle root, nonce, expiry timestamp (UTC fixed-microsecond format), intent data (canonicalized), requestor user ID, and acting app ID. Intent data canonicalization rejects unsupported types with an error (no silent fallback), ensuring cross-language hash parity between Go and Python. L3 proof is intentionally not included in the transaction hash so that L2 (machine consensus) can sign the hash before L3 (human notary) is asked. Tamper-evidence for L3 is provided at verification time, when the proof is checked against the transaction hash.
4. **Stateful validation** (stage 3): State Merkle root validation against the operator's current state root.
5. **Posture validation** (stage 4): L2 and L3 checks gated by the configured GovernancePosture.

If any stage fails, the nonce reservation is released and the transaction is rejected. The nonce remains reserved through successful verification and is finalized after execution completes.

#### Layer 1: Technical Bedrock (L1)

**Purpose**: Ensure the transaction does not violate fundamental technical or safety constraints.

**Checks**:
- **Protobuf Field Validation**: Reflects over protobuf message fields and checks forbidden pattern field option extensions against string values using regex matching.
- **MITRE-based Threat Detection**: Scans command strings, MCP tool arguments, A2A payloads, and file edit content for known malicious patterns. Detectors cover data destruction, reverse shells, privilege escalation, and other attack categories with MITRE ATT&CK mappings.
- **Critical System File Protection**: Blocks modifications to sensitive paths (e.g., `/etc/shadow`, `/etc/sudoers`).

**Outcome**:
- **Passed**: Proceeds to State Check
- **Violated**: Transaction fails closed with typed rejection and audit entry

#### State Check: Merkle Root Freshness

**Purpose**: Ensure the transaction is based on the current system state.

**Checks**:
- **Merkle root validation**: Compares the state Merkle root in the envelope against the operator's current state root.
- **Consistency verification**: Rejects stale transactions to prevent race conditions on shared state.

**Outcome**:
- **Fresh**: Proceeds to L2
- **Stale**: Transaction fails closed with typed rejection and audit entry

#### Layer 2: Consensus Verification (L2)

**Purpose**: Verify that the transaction has proper L2 consensus.

**Checks** (see [Tribunal](#tribunal) for full details):
- **Vote presence**: If the posture requires L2 signatures, the envelope must include L2 metadata with at least one vote.
- **Tribunal policy lookup**: Loads the tribunal policy by tribunal ID. The policy must exist and be enabled.
- **Member validation**: Votes from signer key IDs not in the tribunal policy's member list are silently excluded; only member votes count toward quorum.
- **Duplicate signer detection**: If the policy requires distinct signers, duplicate signer key IDs are rejected.
- **Signature verification**: Each vote's Ed25519 signature over `<transaction_hash>|<decision>` is verified against the trusted public key.
- **Quorum check**: The number of affirmative (safe) votes must meet or exceed the policy's quorum threshold.

**Posture behavior**:
- **doctrine**: L2 results are recorded for audit but do not gate execution.
- **consensus/notary**: L2 signature verification is a fail-closed gate.

**Outcome**:
- **Passed**: Proceeds to L3
- **Invalid/Missing**: Transaction fails closed with typed rejection and audit entry

#### Layer 3: Authorization (L3)

**Purpose**: Ensure the principal is authorized and present for the action.

**L3 Notary implementations** (see [L3 Notary Verification Detail](#l3-notary-verification-detail) for full details):

- **Passkey Service**: Gateway mode for web sessions. Verifies WebAuthn assertions using the transaction hash as the challenge. Validates credential ID, client data JSON, authenticator data, and signature against registered passkey credentials.

- **Outbound L3 Notary**: Outbound mode for operator-side approval. Verifies that a transaction exists in the suspended transaction store, is marked as approved, has a valid CLI signature over the transaction hash, and has not exceeded the 30-minute approval window. No CLI session or certificate revocation checks are performed in this mode.

- **CLI L3 Notary**: Gateway mode for CLI sessions. Extends outbound mode with CLI session verification that checks user active status, session ownership, certificate fingerprint match, session expiry, and certificate revocation via the PKI authority.

- **Gateway L3 Notary**: Unified gateway mode that requires passkey authorization for all proofs. The gateway notary always requires a credential ID and delegates to the passkey verifier first. If the proof includes an mTLS certificate fingerprint (CLI caller), CLI mTLS session verification runs as an additional transport-auth layer. This is the notary used in gateway-mode deployments.

**Posture behavior**:
- **doctrine/consensus**: L3 results are recorded for audit but do not gate execution.
- **notary**: L3 proof is a fail-closed gate for mutation actions.

**Mutation enforcement**: The system classifies whether an action type is state-changing. Only mutations require L3 proof under the notary posture.

**No bypass field**: L3 is satisfied only by a verified proof. There is no auto-approved bypass. The Warden re-derives whether L3 is required from the action type and posture, and if required, demands a real proof. Out-of-band approvals use the outbound notary with a verifiable CLI signature, not a producer-supplied flag.

**Outcome**:
- **Authorized**: Proceeds to L5 (Actuator)
- **Denied**: Transaction fails closed with typed rejection and audit entry

### Phase 5: Execution

#### Layer 5: Actuator (L5)

**Purpose**: Execute the approved transaction and generate signed cryptographic evidence.

**Process**:
1. **Initial Receipt**: The Actuator signs an initial receipt with status "executing" using Ed25519 and logs it to the **Local Audit Vault** before starting execution. The receipt is canonicalized using deterministic JSON with fixed field order before signing. This ensures evidence of the attempt is preserved even if execution crashes. If signing or logging fails, execution does not proceed (fail-closed).
2. **L2/L3 Status Recording**: The receipt includes L2 and L3 status fields reflecting whether each layer was required by the posture and whether it passed validation.
3. **Sovereignty Rehydration**: If the payload was scrubbed for sovereignty, the Scrubbing Service rehydrates it using local tokens before execution.
4. **JIT Capability Minting**: The Actuator mints a just-in-time, single-action, self-dissolving **Capability** scoped to the transaction hash, action type, and target resource. The capability is injected into the execution context for downstream handlers. No standing credentials exist outside the lifetime of a single execution call. The capability is dissolved immediately after execution completes or fails (zero standing privileges).
5. **Execution**: The Actuator dispatches the action through the execution handler, which routes to the appropriate handler based on event type:
   - **Command Execution**: Bash/Shell commands.
   - **File Operations**: Scoped reads, writes, and edits.
   - **Protocol Egress**: MCP or A2A tool calls via the MCP Gateway.
   - **Synchronous handlers**: `EVAL_ANSWER`, `MCP_CALL`, and `A2A_CALL` return results directly as the receipt summary.
   Execution handlers scrub sensitive host data from results (stdout, stderr, file content) before returning them to the Actuator. The Actuator receives already-scrubbed output.
6. **Result Capture**: Output, errors, and updated Merkle roots are captured.
7. **Final Receipt**: A final action receipt is generated, containing:
   - Execution results (or failure summary)
   - State root before and after
   - Operator signature (Ed25519 over canonical receipt JSON)
   - L2 and L3 status reflecting posture enforcement

**Outcome**: Signed final receipt is generated and anchored to the local ledger.

### Phase 6: Audit and Completion

#### Step 8: Local Audit Vault Logging

The operator anchors the transaction to the **Local Audit Vault** on the sovereign host. This architecture, known as **Local-First Audit Architecture (LFAA)**, ensures:

- **Immutable record**: The transaction cannot be altered after the fact.
- **Local sovereignty**: Audit data stays on the host; raw data never leaves.
- **Cryptographic integrity**: Each entry is signed and chained.
- **Multi-layered storage**:
    - **SQL Audit Store**: Stores structured event data, action receipt records, and file mutation records in an encrypted SQLite database. Receipts are stored with upsert semantics and support pagination for history queries.
    - **Git Ledger**: Provides immutable versioning for file mutations using a git-backed ledger with two-phase commit. Files are optionally encrypted before copying to the ledger. Git state is snapshotted before and after mutations, with diffs calculated for audit logging. The current state Merkle root is the git commit hash.

The vault records:
- The original envelope
- Verification layer results (pass/fail for each layer)
- Execution results and state root transitions
- Signed receipts (both intent and final result)
- Timestamps and session metadata

**Note**: Even failed transactions are logged to the audit vault for complete transparency. The history handler integrates both stores to service history queries, returning audit events with associated file mutation details.

#### Step 9: Receipt Return to Gateway

The operator returns the sovereignty-scrubbed signed receipt to the gateway. In synchronous gateway mode, the receipt is returned directly to the HTTP caller. In outbound mode, the receipt is pushed over the mTLS tunnel. The receipt:

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

The L4 Warden verifies L2 votes with identical logic regardless of posture; the posture only determines whether a failure rejects the transaction or is merely recorded.

**Signature format**: Ed25519 over `<transaction_hash>|<decision>` (boolean string).

---

## Tribunal

The Tribunal is the **reference implementation** of L2 Consensus shipped with g8e. L2 Consensus is a protocol concept defined by the g8e protocol (`L2Metadata`, `L2Vote`); the Tribunal is one concrete implementation. Alternative implementations can be built against the same protocol interfaces. Each member is a distinct enrolled principal with its own Ed25519 key, registered as a trusted signer in the gateway's signer store.

### Tribunal Policy

The tribunal policy defines a named consensus body with the following properties:

- **ID**: Tribunal name/identifier. Must be non-empty, alphanumeric with hyphens and underscores only.
- **Member App IDs**: List of member application IDs. Each must correspond to an enabled trusted signer. No duplicates.
- **Quorum**: Minimum number of affirmative distinct signatures required. Must be >= 1 and <= member count.
- **Require Distinct**: If true, duplicate signer key IDs in a vote set are rejected.
- **Enabled**: Whether the tribunal is active. New tribunals must be created as enabled.

### Tribunal Members

Each tribunal member represents a single member identity:

- **App ID**: The member's enrolled application ID, used to look up the member's trusted public key.
- **Private Key**: The member's Ed25519 private key, used to sign consensus votes. Members without a private key (key resolution failed during bootstrap) are included in the member list for policy purposes but cannot sign votes; their votes are skipped during deliberation.

Members never share the gateway identity key. Even in single-member tribunals, the member is a distinct principal; the actuator's Ed25519 key is reused as the member signing key, but the member App ID remains separate from the gateway's actuator key ID.

### Tribunal Service

The Tribunal Service is the core deliberation engine. It holds the tribunal ID, the list of members with their private keys, the shared L1 Doctrine instance for deterministic safety evaluation, and infrastructure for logging and HTTP response writing.

#### Deliberation Flow

The Tribunal Service processes a GovernanceEnvelope through all tribunal members:

1. **Hash verification**: Recomputes the transaction hash and verifies it matches the envelope ID. Mismatch causes fail-closed rejection.
2. **Command extraction**: Extracts command data and intent from the envelope payload. If intent data is present, it is marshaled to JSON; otherwise the raw payload bytes are used.
3. **Per-member safety evaluation**: Each member with a private key independently evaluates safety:
   - **MITRE checks**: Scans the command data for malicious patterns. If any signal has block-recommended, the payload is unsafe.
   - **Fail-closed on nil doctrine**: If doctrine is nil, the evaluation returns unsafe. This ensures a misconfigured tribunal cannot approve transactions without doctrine validation.
4. **Vote signing**: Each member signs `<transaction_hash>|<decision>` with Ed25519. The decision is a boolean (true = safe, false = unsafe).
5. **Vote collection**: Votes are collected into the L2 metadata with the tribunal ID set to the service's tribunal ID.

#### HTTP Handler

The Tribunal Service exposes an mTLS-guarded HTTP handler for remote deliberation. It accepts a canonical-JSON GovernanceEnvelope (max 1 MiB body), calls the deliberation process, and returns the signed envelope with L2 metadata populated as JSON with appropriate security headers.

### Key Providers

The key provider interface resolves Ed25519 private keys for tribunal members by App ID. This abstraction allows keys to be sourced from disk, in-process, or any secure backing store.

**File-based key provider**: Loads Ed25519 private keys from disk. Each member's key is stored as a hex-encoded Ed25519 seed in a file within the secrets directory. Key files are created with restrictive permissions.

**Save member key**: Writes an Ed25519 private key seed to disk for member provisioning, creating the secrets directory if it does not exist.

**Bootstrap key resolution**: During tribunal bootstrap, a composite key provider is constructed:
1. **File-based lookup**: First attempts to load the member key from disk.
2. **Actuator fallback**: If the file lookup fails and the member App ID matches the actuator's key ID, the actuator's Ed25519 private key is used (for single-member tribunals).
3. **Failure**: If neither source resolves, the member is included without a private key and a warning is logged.

### Tribunal Factory

The tribunal factory constructs a Tribunal Service from a tribunal policy and a key provider. It:

1. Validates that the policy and key provider are non-nil (fail-closed).
2. Iterates over the policy's member App IDs, resolving each member's private key via the key provider.
3. If key resolution fails, logs a warning and includes the member without a private key.
4. Constructs and returns a Tribunal Service with the resolved members, shared doctrine, logger, and responder.

This factory is used by both production bootstrap and test fixtures, eliminating code duplication.

### Tribunal Store Service

The tribunal store service provides CRUD operations for tribunal policy records, backed by the SQLite document store.

**Operations**:
- **Get**: Retrieves a tribunal policy by ID. Returns nil if not found.
- **Add**: Creates or updates a tribunal policy with fail-closed validation.
- **List**: Returns all tribunal policies, ordered by ID.
- **Delete**: Removes a tribunal policy.

**Add validation**: Enforces the following constraints at write time:
- Tribunal ID: non-empty, alphanumeric with hyphens and underscores only.
- Member list: non-empty, no empty strings, no duplicate member IDs.
- Quorum: >= 1 and <= member count.
- Trusted signer verification: every member App ID must resolve to an enabled trusted signer.
- New tribunals must be created as enabled. Existing tribunals may only be disabled.

### Admin API Endpoints

Tribunal policies are managed via admin-only REST endpoints:

| Method | Path | Description |
|---|---|---|
| `POST` | `/api/v1/admin/tribunals` | Create a new tribunal policy |
| `GET` | `/api/v1/admin/tribunals` | List all tribunal policies |
| `DELETE` | `/api/v1/admin/tribunals/{id}` | Delete a tribunal policy by ID |

All endpoints require an authenticated bootstrap user (admin-only).

### Local Deliberator

The local deliberator is an in-process adapter that calls the Tribunal Service directly, without an HTTP round-trip. This is used when the Tribunal runs in the same process as the gateway (single-binary deployment).

### Tribunal Bootstrap at Gateway Startup

When the gateway starts in consensus or notary posture with a non-empty tribunal ID, the Tribunal service is constructed and wired at startup. If the tribunal ID is empty, an advisory warning is logged and the gateway boots without an in-process Tribunal; L2-gated transactions will be rejected by L4Warden until a tribunal is enrolled. Under doctrine posture, the Tribunal is not constructed in-process; L2 votes must be obtained from an external Tribunal service when required.

1. **Load policy**: Retrieves the tribunal policy from the database. If missing, bootstrap fails with an error.
2. **Construct key provider**: Creates a file-based key provider for the configured secrets directory, then wraps it with the actuator key fallback logic.
3. **Build service**: Constructs the Tribunal Service with the policy, composite key provider, shared L1 doctrine, logger, and response writer.
4. **Wire handlers**: The resulting Tribunal Service is registered as both the mTLS HTTP handler for remote deliberation and the local deliberator for in-process calls.

---

## L3 Notary Verification Detail

### L3 Notary Implementations

The L3 Notary interface is implemented by three notary types:

- **Outbound L3 Notary**: Operator-side approval. Verifies the transaction exists in the suspended transaction store, is marked approved, has a valid Ed25519 signature over the transaction hash, matches the expected certificate fingerprint, and is within the 30-minute approval window.
- **CLI L3 Notary**: Gateway CLI mode. Calls the shared verification function with a CLI session verifier that checks user active status, CLI session ownership, certificate fingerprint match, session expiry, and certificate revocation before the suspended-transaction and signature verification.
- **Gateway L3 Notary**: Unified gateway mode. Requires passkey authorization for all proofs; a credential ID must be non-empty. Passkey verification runs first. If an mTLS certificate fingerprint is present, CLI mTLS session verification runs as an additional transport-auth layer.

The passkey verifier verifies WebAuthn assertions using the transaction hash as the challenge and validates against registered passkey credentials for the user.

### L3 and Mutations

L3 fail-closed enforcement applies only to mutation action types. The mutation classification:

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
| Gateway mode | Doctrine | `--posture` flag; defaults to doctrine when not specified |
| Outbound (operator) mode | Notary | Defaults to notary when not specified |

**Posture selection**: The doctrine and consensus postures allow mutations to execute without human authorization (L3 proof) or multi-party consensus. Choosing such a posture is itself an act of human intent; the gateway logs a warning at startup and proceeds. The `--posture doctrine` or `--posture consensus` flag is the authorization.

**Invalid posture handling**: The posture factory panics on unrecognized posture names. This is intentional; misconfigured deployments fail at startup rather than silently running under a weaker posture. CLI flag validation returns a graceful error.

---

## Receipt Metadata

The L5 Actuator records posture enforcement results in every action receipt:

| Posture | L2 Status | L3 Status |
|---|---|---|
| Doctrine | Audited | Audited |
| Consensus | Required (valid or failed) | Audited |
| Notary | Required (valid or failed) | Required (valid or failed) |

These values are part of the canonical receipt JSON and are signed by the actuator's Ed25519 key. They are also persisted in the audit store.

---

## Security Properties

Throughout the transaction process, several key security properties are maintained:

### Fail-Closed Design
Every verification layer fails closed: if any check fails, the transaction is rejected immediately and the nonce reservation is released. Crucially, the Actuator will not execute a mutation if it fails to sign or log the initial "intent to execute" receipt. The posture factory panics on invalid posture names to prevent misconfigured deployments from silently running under a weaker posture.

### Sovereignty
- Raw data and audit logs stay on the sovereign host.
- Operators initiate outbound-only connections to the gateway.
- Sensitive data is scrubbed/rehydrated at the execution boundary.

### Cryptographic Integrity
- Every envelope is signed by tribunal members (L2) using Ed25519 over `<transaction_hash>|<decision>`.
- Every receipt is signed by the L5 Actuator using Ed25519 over canonical JSON.
- Audit entries are stored in encrypted SQLite databases with optional vault encryption.
- File mutations in the git ledger are optionally encrypted before storage.
- mTLS on the HTTPS port, with application-layer enforcement for all non-public routes. The PKI TLS configuration defaults to requiring and verifying client certificates for operator-side connections.
- Transport-to-envelope identity binding prevents impersonation by matching mTLS certificate SPIFFE URI SANs to envelope identity claims.

### Defense in Depth
- **L1 Doctrine**: Protobuf field option validation and MITRE-based threat detection.
- **L2 Consensus**: Multi-signature tribunal verification with quorum and distinct-signer checks.
- **L3 Notary**: Human-in-the-loop authorization via WebAuthn passkey, CLI mTLS certificate, or outbound CLI approval.
- **L4 Warden**: Replay protection, state root consistency, and posture-gated L2/L3 verification.
- **L5 Actuator**: Fail-closed signed execution boundary with canonical receipt production and zero standing privileges via JIT capability minting.

### Accountability
- Every transaction is logged with a unique transaction hash.
- Every failure is recorded with typed rejection.
- Principal identity is verified at L3.

---

## Component Summary

| Component | Role | Key Characteristics |
|-----------|------|---------------------|
| **Principal** | Initiates intent | Human or AI agent, authenticated via WebAuthn or CLI mTLS. |
| **Producer** | Wraps intent in envelope | Creates GovernanceEnvelope with intent, metadata, and optional L3 proof. L2 consensus is obtained by the gateway during transaction processing. |
| **Governance Gateway** | Policy Decision Point | PKI authority, mTLS admission control, identity binding, replay store, universal endpoint. |
| **GovernancePosture** | Verification Policy | Configures which layers are enforced vs audited (doctrine, consensus, notary). |
| **L4 Warden** | Verification Orchestrator | Five-stage verification: in-flight tracking, nonce reservation, stateless validation (L1 + hash), stateful validation (state root), posture-gated L2/L3. |
| **L1 Doctrine** | Technical Bedrock | Forbidden pattern validation, MITRE-based threat detection. |
| **L2 Consensus** | Tribunal Verification | Ed25519 vote signatures, quorum and distinct-signer checks, tribunal policy enforcement. |
| **L3 Notary** | Authorization Engine | Gateway notary (passkey-first, CLI session as additional layer), CLI notary (session verification + suspended transaction), outbound notary (operator-side approval with CLI signature). |
| **L5 Actuator** | Execution Gateway | Fail-closed dual receipt signing with canonical JSON, JIT capability minting/dissolving, rehydration, execution dispatch. |
| **Local Audit Vault** | Immutable Ledger | SQL audit store (encrypted SQLite) and git ledger (git-backed, two-phase commit, optional encryption). |

---

## Transaction Flow Summary

1. **Principal** submits intent
2. **Producer** creates GovernanceEnvelope with intent, metadata, and optional L3 proof. Gateway obtains L2 consensus via Tribunal during transaction processing
3. **Gateway** admits envelope after mTLS, PKI, identity binding, and replay protection
4. **Operator** fetches envelope via outbound mTLS or processes synchronously
5. **Warden (L4)** performs five-stage verification: in-flight tracking, nonce reservation, stateless (L1 doctrine + hash), stateful (state root), and posture-gated L2/L3
6. **Actuator (L5)** signs initial receipt (fail-closed), rehydrates payload, mints JIT capability, executes, dissolves capability, signs final receipt
7. **Local Audit Vault** logs complete transaction to SQL audit store and git ledger
8. **Operator** returns signed receipt to gateway (scrubbed of sensitive host data)
9. **Gateway** returns final output to principal

This end-to-end process ensures that every transaction is governed, verified, executed safely, and audited while maintaining system sovereignty and security.

---

## Related Documentation

- [Gateway Architecture](./gateway.md): Gateway mode, MCP endpoints, and 5-layer verification sequence.
- [Operator Architecture](./operator.md): Operator-side verification pipeline, execution boundary, and local audit vault.
- [Authentication & Authorization](./auth.md): mTLS identity binding, passkey enrollment, and session management.
- [Encryption](./encryption.md): Cryptographic primitives used throughout the pipeline.
