---
title: Protocol
parent: Architecture
---

# g8e Protocol

Last Updated: 2026-05-18
Version: v1.1.0

The g8e Protocol is the mandatory substrate for all interactions within the platform. It defines a secure, auditable, and platform-agnostic contract between **Clients** (BYO agents, frontends) and **Operators** (sovereign host agents).

The reference Operator implementation is **`g8eo`** (Go), and the protocol ensures that no privileged "backdoors" exist for bundled components like **Engine (`g8ee`)** or **Dashboard (`g8ed`)**.

---

# Core Invariants

1.  **Canonical JSON Wire Format**: All client-facing mutation paths (HTTPS APIs, WSS pub/sub) MUST use canonical JSON (**protojson**) for the `GovernanceEnvelope`. Binary protobuf is reserved for internal storage and specific high-performance peer-to-peer sync.
2.  **Hash-Based Signing**: The `transaction_hash` is the sole authority for identity and intent. It is a SHA-256 hash computed over canonicalized fields in a specific order. The verifier enforces that `envelope.id == envelope.transaction_hash == computed_hash`.
3.  **Fail-Closed Verification**: An Operator MUST reject any transaction that fails any layer of the 3-Layer Governance (L1/L2/L3), has an expired timestamp, or a reused nonce.
4.  **Body-Embedded Context**: Business and execution context (session IDs, user IDs, investigation/task IDs) MUST be embedded in request bodies via `RequestContext` objects. HTTP headers are reserved for protocol-level metadata and mTLS-bound identity.
5.  **Authoritative Persistence**: Mutations are written to the Hub's Document Store. While a KV cache exists for high-performance lookups, application-layer adapters default to a **Write-Only** cache policy to guarantee every read is satisfied by the authoritative database.
6.  **BFT State Binding**: Mutations are bound to a specific fleet state via `state_merkle_root`. This ensures that an agent is acting on the same reality the Operator currently perceives.
7.  **Signed Receipts**: Every mutation result MUST be accompanied by an `ActionReceipt`, signed by the Operator's Warden, providing cryptographic proof of execution or rejection.

---

# Message Models

The canonical schema source of truth lives in `@/home/bob/g8e/protocol/proto/`.

| File | Purpose |
|------|---------|
| `common.proto` | Defines `GovernanceEnvelope` and `GovernanceMetadata` (L1/L2/L3). |
| `operator.proto` | Defines typed payloads for actions (`CommandRequested`, `FileEditRequested`, etc.). |

## The Governance Envelope

The `GovernanceEnvelope` is the single canonical container for all g8e mutations.

| Field | Description |
|-------|-------------|
| `id` | The unique transaction ID (must match `transaction_hash`). |
| `event_type` | Canonical event name from `protocol/constants/events.json`. |
| `payload` | **Base64-encoded binary Protobuf message**. This is the SOLE authority for execution. |
| `transaction_hash` | SHA-256 hash of: `action_type | target_resource | payload_base64 | state_root | nonce | expires_at | intent_data`. |
| `governance` | Cryptographic proofs for L1, L2, and L3 verification layers. |
| `state_merkle_root` | The expected state root of the host at the time of signing. |
| `nonce` | Unique string for replay protection. |
| `expires_at` | UTC timestamp after which the transaction is void. |

---

# 3-Layer Governance Bedrock

### L1: Technical Bedrock (Hard Gates)
Enforced via Protobuf reflection before any code is executed.
-   **Mechanism**: Custom field option `(g8e.common.v1.forbidden_patterns)`.
-   **Scope**: Applied to fields containing raw input (e.g., shell commands).
-   **Enforcement**: The `TransactionVerifier` regex-matches input against patterns like `sudo`, `su`, or `rm -rf /`.

### L2: Consensus (Tribunal)
Enforced by the Operator via asymmetric signature verification.
-   **Mechanism**: ED25519 signature over `transaction_hash | decision` (where decision is usually `true`).
-   **Trust**: The Operator maintains a `SignerStore` of trusted public keys.
-   **Material**: Signed by a consensus agent (like `g8ee`) after verifying the intent.

### L3: Authorization (Approval)
Human-in-the-loop authorization via hardware-bound proofs.
-   **Mechanism**: WebAuthn `L3Proof` (clientDataJSON, authenticatorData, signature).
-   **Policy**: Mandatory for all mutation requests unless explicitly bypassed by an `auto_approved` policy for benign commands (e.g., `uptime`). L3 NEVER bypasses L1 or L2.

---

# Transaction Lifecycle

## 1. Request Phase (Client -> Operator)
1.  **Payload Generation**: Client creates a typed Protobuf message (e.g., `CommandRequested`).
2.  **Envelope Construction**: Client wraps the payload in a `GovernanceEnvelope`, populating metadata, nonce, and state root.
3.  **L2 Signing**: The Client (or a trusted consensus sidecar) computes the `transaction_hash` and signs it with an L2 private key.
4.  **Submission**: The Client submits the JSON-encoded envelope to the Operator.

## 2. Verification Phase (Operator)
The Operator's `TransactionVerifier` performs the following checks in order:
1.  **Integrity**: Verifies `id == transaction_hash == SHA256(canonical_fields)`.
2.  **Freshness**: Checks `expires_at` and verifies the `nonce` is not in the replay store.
3.  **State**: Compares `state_merkle_root` against the local host's current root.
4.  **L1**: Scans the typed payload for `forbidden_patterns`.
5.  **L2**: Verifies the `tribunal_signature` against the `SignerStore`.
6.  **L3**: For mutations, verifies the WebAuthn `L3Proof`.

## 3. Execution & Result Phase (Operator -> Client)
1.  **Execution**: The **Warden** routes the payload to the appropriate handler (e.g., Bash executor).
2.  **Receipt Generation**: After execution, the Warden generates an `ActionReceipt` containing the status, state root delta, and a signature.
3.  **Result Publication**: The Operator publishes a result envelope (also a `GovernanceEnvelope`) containing the typed result (e.g., `CommandResult`) and the `ActionReceipt`.

---

# Implementation Reference

| Responsibility | Authoritative File |
|----------------|--------------------|
| **Protobuf Schemas** | `@/home/bob/g8e/protocol/proto/` |
| **Event Registry** | `@/home/bob/g8e/protocol/constants/events.json` |
| **Verification Logic** | `@/home/bob/g8e/services/g8eo/internal/services/governance/transaction_verifier.go` |
| **Envelope Types** | `@/home/bob/g8e/services/g8eo/pkg/uap/types.go` |
| **Audit Storage** | `@/home/bob/g8e/services/g8eo/internal/services/storage/audit_vault.go` |
