# Authentication & Authorization

This document details the authentication and authorization architecture of the g8e substrate. g8e is built as a zero-trust execution environment where every mutation is typed, signed, and governed via a deterministic verification pipeline.

## Overview

The g8e security model is founded on two core pillars:
1.  **Identity-Bound Communication (mTLS)**: Every connection within the platform, whether from a CLI, a Dashboard, or an AI Agent, must be authenticated via mutual TLS (mTLS) with a verified SPIFFE workload identity.
2.  **5-Layer Verification Sequence**: Every mutation (command execution, file edit, tool call) must pass through the sequential 5-layer verification pipeline before execution.

## 1. Authentication & Workload Identity

g8e uses an internal PKI (Public Key Infrastructure) to issue and manage certificates. The **Governance Gateway** (PDP) acts as the Certificate Authority (CA).

### Workload Identity (SPIFFE)

Every component in the system is assigned a SPIFFE ID, which is embedded as a Uniform Resource Identifier (URI) in the Subject Alternative Name (SAN) of its mTLS certificate.

| Workload Type | SPIFFE ID Format |
| :--- | :--- |
| **Operator** | `spiffe://g8e.local/operator/<org_id>/<operator_id>/<operator_session_id>` |
| **CLI / BYO** | `spiffe://g8e.local/cli/<user_id>/<cli_session_id>` |
| **App / Agent** | `spiffe://g8e.local/app/<operator_id>` |
| **Gateway / Hub** | `spiffe://g8e.local/hub/operator-listen` |

### mTLS Enforcement

The Governance Gateway enforces TLS 1.3 for all L7 communication.
-   **Strict mTLS**: The gateway requires and verifies client certificates (`tls.RequireAndVerifyClientCert`).
-   **Revocation**: Certificates are checked against a `revoked_certificates` collection. Revocation is enforced at the Gateway.
-   **Identity Binding**: Middleware verifies that the SPIFFE ID in the client certificate matches the identity claims (e.g., `operator_session_id`) inside the `GovernanceEnvelope`.

### Enrollment & Bootstrap (CSR-based)

Clients enroll in the platform using a Certificate Signing Request (CSR) bootstrap flow:
1.  **CA Discovery**: Clients fetch the platform's root CA bundle from `/.well-known/g8e/pki/ca-bundle`.
2.  **CSR Submission**: Clients generate a local key pair (ECDSA P-384) and submit a CSR to `/api/pki/sign-csr`.
3.  **Registration**: The Gateway validates the CSR and binds the certificate to a user identity via invitation-based JIT provisioning.
4.  **Session Issuance**: Upon successful enrollment, the Gateway issues an `operator_session_id` or `cli_session_id`.

## 2. 5-Layer Verification Sequence (The Gauntlet)

g8e implements a deterministic 5-layer governance sequence. Every mutation must pass through all active layers before execution.

### Layer 1: Technical Bedrock (L1Doctrine)
*Implementation: `internal/services/governance/l1_doctrine.go`*

L1 is the foundational layer that executes deterministic security rules.
-   **Forbidden Patterns**: Uses Protobuf field options (`forbidden_patterns`) to reject strings like `sudo`, `rm -rf /`, etc.
-   **MITRE Threat Detection**: Analyzes payloads against MITRE ATT&CK patterns.
-   **Hard Gates**: Rejects transactions immediately upon violation; cannot be bypassed by L2 or L3.

### Layer 2: Consensus (L2Consensus)
*Implementation: `internal/services/governance/l2_consensus.go`*

L2 provides multi-agent cryptographic verification of intent.
-   **Ed25519 Signatures**: Verifies ED25519 signatures over the `transaction_hash|decision`.
-   **Trusted Signers**: Requires signatures from trusted agents listed in the `SignerStore`.
-   **Posture-Aware Enforcement**: Based on the `GovernancePosture`, L2 signatures may be required.

### Layer 3: Notary (Authorization)
*Implementation: `internal/services/governance/l3_notary.go`*

L3 ensures explicit human authorization for mutations.
-   **Suspension**: The Gateway suspends transactions requiring L3 approval, storing them in the `suspended_transactions` pool.
-   **Out-of-Band (OOB) Approval**: The user approves via WebAuthn/Passkey at `/approve/{txHash}` or mTLS certificate fingerprint.
-   **L3Proof**: A successful approval generates an `L3Proof` cryptographically bound to the `transaction_hash`.

### Layer 4: Warden (Pre-dispatch Verification)
*Implementation: `internal/services/governance/l4_warden.go`*

The Warden is the final fail-closed gate before execution. It verifies:
1.  **Structural Integrity**:Structural integrity, payload decoding, and L1Doctrine compliance.
2.  **Hash Verification**: `id` and `transaction_hash` must match the recomputed SHA-256 hash.
3.  **State Root Consistency**: `state_merkle_root` must match the current system state.
4.  **Replay Protection**: Verifies the `nonce` using the `ReplayStore`.
5.  **Posture Enforcement**: Enforces L2 and L3 requirements based on the configured `GovernancePosture`.

### Layer 5: Actuator (Execution Boundary)
*Implementation: `internal/services/governance/l5_actuator.go`*

The Actuator represents the execution boundary and final audit commitment.
-   **Egress Dispatch**: Dispatches the verified payload to downstream executors (Shell, MCP, A2A).
-   **Action Receipts**: Issues a signed `ActionReceipt` providing immutable proof of the outcome.
-   **Commitment**: Records the transaction in the `AuditVaultService` and chains it to the ledger.

## 3. Governance Postures

Postures define which layers of the bedrock are enforced as "fail-closed" gates.

| Posture | L1 (Doctrine) | L2 (Consensus) | L3 (Notary) | Use Case |
| :--- | :---: | :---: | :---: | :--- |
| **Doctrine** | Required | Optional | Optional | Local Dev / CI |
| **Consensus** | Required | Required | Optional | Automated Workflows |
| **Notary** | Required | Required | Required | **Production (Default)** |

## 4. Sovereignty Boundary Plane

Handling sensitive data without leaking it to upstream models:
-   **Scrubbing**: Private data is replaced with opaque tokens before sending to external LLMs.
-   **Deterministic Rehydration**: The L5 Actuator performs local rehydration of tokens just before execution.
-   **Data Sovereignty**: Raw secrets never leave the sovereign host environment.
