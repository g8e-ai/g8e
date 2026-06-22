# Transaction Process: End-to-End Flow

This document walks through the complete transaction process in the g8e governance system, explaining each step from initial intent to final execution and audit. The process is designed to ensure security, accountability, and sovereignty throughout.

## Overview

The g8e system implements a five-layer verification pipeline (L1-L5) that governs every transaction. Transactions flow from AI clients through a governance gateway to governed operators, where they undergo rigorous verification before execution on target systems.

---

## Phase 1: Intent Submission

### Step 1: Principal Initiates Request

A **Principal** (human user or AI agent) submits an intent to perform an action. This intent can be submitted through multiple channels:

- **MCP client**: Using Claude, Cursor, Windsurf, or other MCP-compatible AI IDEs
- **Agentic ensemble**: Through A2A (Agent-to-Agent) protocols or tool calls
- **Native application**: Direct integration with g8e protocols

The intent represents what the principal wants to accomplish—for example, "read a file," "deploy a container," or "query a database."

### Step 2: Producer Wraps Intent

The **Producer** (g8e-compatible agentic ensemble, BYO agent, or MCP client) receives the raw intent and begins the governance process:

1. **Reach Consensus (L2)**: The producer coordinates with a tribunal of signers to reach consensus on the intent. This ensures that no single entity can unilaterally authorize actions.
2. **Create GovernanceEnvelope**: The producer wraps the intent in a signed `GovernanceEnvelope`, which includes:
   - The original intent
   - Tribunal signatures proving consensus
   - Metadata about the request (timestamp, principal identity, etc.)
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

The gateway performs initial admission checks on the envelope:

1. **Signature verification**: Validates that the envelope is properly signed by recognized tribunal members
2. **PKI validation**: Confirms the certificates used are from the trusted PKI hierarchy
3. **Envelope structure validation**: Ensures the envelope conforms to the expected schema
4. **Replay protection**: Checks that this envelope hasn't been processed before

If the envelope passes admission, it is queued for processing. If it fails, the gateway rejects it immediately with a typed error and audit entry.

---

## Phase 3: Operator Retrieval

### Step 5: Operator Establishes Connection

A **Governed Operator (g8eo)** running on a sovereign host establishes an outbound-only mTLS tunnel to the gateway. This is a critical security design:

- **Outbound-only**: The operator initiates the connection; the gateway cannot reach into the operator
- **mTLS encryption**: Mutual TLS ensures both ends authenticate each other
- **Policy Execution Point**: The operator is where policies are actually enforced

This design ensures that operators remain sovereign—they can pull work but cannot be pushed into from the gateway.

### Step 6: Operator Fetches Pending Envelope

The operator polls the gateway for pending envelopes that are assigned to it (based on policy, capacity, or other routing logic). When it finds an envelope, it retrieves it over the secure mTLS tunnel.

The operator now has the signed `GovernanceEnvelope` and begins the verification pipeline.

---

## Phase 4: Verification Pipeline (L1-L5)

The operator runs the envelope through a five-layer verification pipeline orchestrated by the **Warden (L4)**. Each layer must pass; if any layer fails, the transaction fails closed (rejected with audit trail). 

### Step 7: Warden Pre-Dispatch Gate (L4)

The **L4 Warden** is the primary orchestrator for the verification pipeline. Before any deeper checks, it performs:

1. **In-flight tracking**: Prevents the same nonce from being processed concurrently.
2. **Nonce reservation**: Atomically reserves the nonce in the **Replay Store** (durable SQLite storage) to prevent replay attacks even if the operator crashes mid-execution.
3. **Expiry check**: Ensures the transaction hasn't expired relative to the operator's system clock.

If these pass, the Warden proceeds with the multi-layer pipeline.

### Layer 1: Technical Bedrock (L1)

**Purpose**: Ensure the transaction doesn't violate fundamental technical or safety constraints.

**Checks** (handled by **L1 Doctrine**):
- **Protobuf Field Validation**: Checks for `forbidden_patterns` defined in the protocol schema.
- **MITRE-based Threat Detection**: Scans command strings, MCP arguments, and file content for known malicious patterns (e.g., reverse shells, privilege escalation, data destruction).
- **Critical Path Scrutiny**: Elevated verification for modifications to critical system files (e.g., `/etc/shadow`, `/etc/sudoers`).

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

**Checks**:
- **Tribunal signature**: Validates that the envelope includes signatures from a quorum of trusted tribunal members.
- **Signer Store verification**: Cryptographically verifies signatures against the local **Signer Store** (trusted public keys).

**Outcome**:
- **Passed**: Proceeds to L3
- **Invalid/Missing**: Transaction fails closed with typed rejection and audit entry

### Layer 3: Authorization (L3)

**Purpose**: Ensure the principal is authorized and present for the action.

**Checks** (handled by **L3 Notary**):
- **Human presence verification**: 
    - **Gateway Mode**: Validates WebAuthn passkey signatures.
    - **Outbound Mode**: Validates CLI-based approvals (`g8e approve`) using cryptographic signatures over the transaction hash.
- **Mutation enforcement**: Actions classified as mutations (state-changing) strictly require L3 proof.

**Outcome**:
- **Authorized**: Proceeds to L5 (Actuator)
- **Denied**: Transaction fails closed with typed rejection and audit entry

---

## Phase 5: Execution

### Layer 5: Actuator (L5)

**Purpose**: Execute the approved transaction and generate signed cryptographic evidence.

**Process**:
1. **Initial Receipt**: The Actuator signs an initial receipt with `EXECUTION_STATUS_EXECUTING` and logs it to the **Local Audit Vault** *before* starting execution. This ensures evidence of the attempt is preserved even if execution crashes.
2. **Sovereignty Rehydration**: If the payload was scrubbed for sovereignty, the **Scrubbing Service** rehydrates it using local tokens before execution.
3. **Execution**: The Actuator dispatches the action to the appropriate internal service:
   - **Command Execution**: Bash/Shell commands via `ExecutionService`.
   - **File Operations**: Scoped reads, writes, and edits via `FileEditService`.
   - **Protocol Egress**: MCP or A2A tool calls via the **MCP Gateway**.
4. **Result Capture**: Output, errors, and updated Merkle roots are captured.
5. **Final Receipt**: A final `ActionReceipt` is generated, containing:
   - Execution results (or failure summary)
   - `StateRootBefore` and `StateRootAfter`
   - Operator signature (proving authorized dispatch)
6. **Sovereignty Scrubbing**: Sensitive host data is scrubbed from the result before returning it to the gateway.

**Outcome**: Signed final receipt is generated and anchored to the local ledger.

---

## Phase 6: Audit and Completion

### Step 8: Local Audit Vault Logging

The operator anchors the transaction to the **Local Audit Vault** on the sovereign host. This architecture, known as **Local-First Audit Architecture (LFAA)**, ensures:

- **Immutable record**: The transaction cannot be altered after the fact.
- **Local sovereignty**: Audit data stays on the host; raw data never leaves.
- **Cryptographic integrity**: Each entry is signed and chained.
- **Multi-layered storage**: 
    - **SQL Audit Store**: Stores structured event data, receipts, and session metadata in an encrypted SQLite database.
    - **Git Ledger Service**: Provides immutable versioning for file mutations, allowing full history reconstruction.

The vault records:
- The original envelope
- Verification layer results (pass/fail for each layer)
- Execution results and state root transitions
- Signed receipts (both intent and final result)
- Timestamps and session metadata

**Note**: Even failed transactions are logged to the audit vault for complete transparency.

### Step 9: Receipt Return to Gateway

The operator pushes the sovereignty-scrubbed signed receipt back to the gateway over the mTLS tunnel. The receipt:

- Confirms successful execution (or captures the failure)
- Provides the results (if authorized for the principal)
- Maintains the audit trail for the entire pipeline
- Contains no sensitive host data

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
Every verification layer fails closed—if any check fails, the transaction is rejected immediately. Crucially, the **Actuator** will not execute a mutation if it fails to sign or log the initial "intent to execute" receipt.

### Sovereignty
- Raw data and audit logs stay on the sovereign host.
- Operators initiate outbound-only connections to the gateway.
- Sensitive data is scrubbed/rehydrated at the execution boundary.

### Cryptographic Integrity
- Every envelope is signed by the tribunal (L2).
- Every receipt is signed by the operator (L5).
- Audit entries are stored in encrypted databases.
- mTLS protects all network communications.

### Defense in Depth
- **L1 Doctrine**: Deep threat detection and schema validation.
- **L2 Consensus**: Multi-signature tribunal verification.
- **L3 Notary**: Mandatory human-in-the-loop for mutations.
- **L4 Warden**: Replay protection and state root consistency.
- **L5 Actuator**: Signed execution boundary.

### Accountability
- Every transaction is logged with a unique `TransactionHash`.
- Every failure is recorded with typed rejection.
- Principal identity is verified at L3.

---

## Component Summary

| Component | Role | Key Characteristics |
|-----------|------|---------------------|
| **Principal** | Initiates intent | Human or AI agent, authenticated via WebAuthn or CLI. |
| **Producer** | Wraps intent in envelope | Reaches L2 consensus, creates signed GovernanceEnvelope. |
| **Governance Gateway** | Policy Decision Point | PKI authority, admission control, universal endpoint. |
| **L4 Warden** | Verification Orchestrator | Performs replay protection, L1-L4 verification. |
| **L1 Doctrine** | Technical Bedrock | Protobuf field validation, MITRE-based threat detection. |
| **L3 Notary** | Authorization Engine | Verifies human presence (WebAuthn or CLI signatures). |
| **L5 Actuator** | Execution Gateway | Dual receipt signing, rehydration, execution dispatch. |
| **Local Audit Vault** | Immutable Ledger | Consists of **SQL Audit Store** and **Git Ledger Service**. |

---

## Transaction Flow Summary

1. **Principal** submits intent
2. **Producer** reaches L2 consensus and creates signed envelope
3. **Gateway** admits envelope after PKI and admission validation
4. **Operator** fetches envelope via outbound mTLS
5. **Warden (L4)** performs replay protection, L1 doctrine, and L2/L3 verification
6. **Actuator (L5)** signs intent, rehydrates, and executes approved transaction
7. **Local Audit Vault** (LFAA) logs complete transaction to SQL and Git
8. **Operator** signs final receipt and returns scrubbed receipt to gateway
9. **Gateway** returns final output to principal

This end-to-end process ensures that every transaction is governed, verified, executed safely, and audited while maintaining system sovereignty and security.
