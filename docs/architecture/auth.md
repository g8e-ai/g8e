# Authentication & Authorization

This document details the authentication and authorization architecture of the g8e substrate. g8e is built as a zero-trust execution environment where every mutation is typed, signed, and governed via a deterministic verification pipeline.

## Overview

The g8e security model is founded on two core pillars:
1.  **Identity-Bound Communication (mTLS)**: Every connection within the platform, whether from a CLI, a Dashboard, or an AI Agent, must be authenticated via mutual TLS (mTLS) with a verified SPIFFE workload identity.
2.  **5-Layer Verification Sequence**: Every mutation (command execution, file edit, tool call) must pass through the sequential 5-layer verification pipeline before execution.

## 1. Authentication & Workload Identity

g8e uses an internal PKI (Public Key Infrastructure) to issue and manage certificates. The **Governance Gateway** acts as the Certificate Authority (CA).

### Workload Identity (SPIFFE)

Every component in the system is assigned a SPIFFE ID, which is embedded as a Uniform Resource Identifier (URI) in the Subject Alternative Name (SAN) of its mTLS certificate.

| Workload Type | SPIFFE ID Format |
| :--- | :--- |
| **Operator** | `spiffe://g8e.local/operator/<org_id>/<operator_id>/<operator_session_id>` |
| **CLI / BYO** | `spiffe://g8e.local/cli/<user_id>/<cli_session_id>` |
| **App / Agent** | `spiffe://g8e.local/app/<operator_id>` |
| **Gateway Hub** | `spiffe://g8e.local/hub/operator-listen` |

### mTLS Enforcement

The Governance Gateway enforces TLS 1.3 for all L7 communication.
-   **Strict mTLS**: The gateway requires and verifies client certificates (`tls.RequireAndVerifyClientCert`).
-   **Revocation**: The `PKIAuthority` service supports certificate revocation via the `RevokeCertificate` method. Revoked certificates are tracked in the `revoked_certificates` collection and checked during session validation.
-   **Identity Binding**: Middleware verifies that the SPIFFE ID in the client certificate matches the identity claims (e.g., `operator_session_id`) inside the `GovernanceEnvelope`.

### Enrollment & Bootstrap (CSR-based)

Clients enroll in the platform using a Certificate Signing Request (CSR) bootstrap flow:
1.  **CA Discovery**: Clients fetch the platform's root CA bundle from `/.well-known/g8e/pki/ca-bundle`.
2.  **CSR Submission**: Clients generate a local key pair (ECDSA P-384) and submit a CSR to `/api/pki/sign-csr`.
3.  **Registration**: The Gateway validates the CSR and binds the certificate to a user identity via invitation-based JIT provisioning.
4.  **Session Issuance**: Upon successful enrollment, the Gateway issues an `operator_session_id` or `cli_session_id`.

## 2. 5-Layer Verification Sequence

g8e uses a 5-layer governance model enforced by the **Governance Substrate**. This model replaces traditional RBAC with a cryptographic proof-of-intent verification sequence.

### Layer 1: Technical Bedrock (L1Doctrine)
*Implementation: `/home/bob/g8e/internal/services/governance/l1_doctrine.go`*

L1 is the foundational layer that executes deterministic security rules. It is always active and cannot be bypassed.
-   **Forbidden Patterns**: Uses Protobuf field options (`(g8e.common.v1).forbidden_patterns`) to reject strings like `sudo`, `rm -rf /`, etc.
-   **MITRE Threat Detection**: Analyzes payloads against MITRE ATT&CK patterns (e.g., discovery, persistence, exfiltration).
-   **Hard Gates**: Rejects any transaction that violates doctrine before it reaches L2 or L3.

### Layer 2: Consensus (L2Consensus)
*Implementation: `/home/bob/g8e/internal/services/governance/l4_warden.go` (verifyL2Posture)*

L2 provides cryptographic signature verification for consensus-based authorization.
-   **Ed25519 Signatures**: Verifies ED25519 signatures on the envelope's `governance.l2.consensus_signature` field.
-   **Trusted Signers**: Loads trusted L2 signer public keys from a `SignerStore` (filesystem or in-memory).
-   **Posture-Aware Enforcement**: Depending on the configured governance posture, L2 signatures may be optional (Doctrine posture) or required (Consensus/Notary postures).
-   **App Policy Bypass**: External applications with `AppPolicy` can bypass L3 requirements for specific intents via `auto_approve_intents`.

### Layer 3: Notary (Authorization)
*Implementation: `/home/bob/g8e/internal/services/governance/l3_notary.go`*

L3 is the final human-in-the-loop gate, ensuring explicit human authorization for mutations.
-   **Suspension**: The Gateway suspends any transaction requiring L3 approval, storing it in the `suspended_transactions` pool.
-   **Out-of-Band (OOB) Approval**: The user approves the transaction via:
    - WebAuthn/Passkey-secured approval page at `/approve/{txHash}`
    - HTTP API endpoint at `/api/approve/{txHash}` for programmatic approval
-   **L3 Proof Binding**: A successful approval generates an `L3Proof` containing a WebAuthn signature or mTLS certificate fingerprint, cryptographically bound to the `transaction_hash`.

### Layer 4: Warden (Pre-dispatch Verification)
*Implementation: `/home/bob/g8e/internal/services/governance/l4_warden.go`*

The Warden performs a strict sequence of checks on every `GovernanceEnvelope`:
1.  **Stateless Validation**: Verifies structural integrity, payload decoding, and L1Doctrine compliance.
2.  **Hash Verification**: Recomputes the `transaction_hash` from the envelope fields and verifies it matches the `id`.
3.  **Stateful Validation**: Checks for expiration (`expires_at`) and verifies the `state_merkle_root` matches the current system state.
4.  **Replay Protection**: Verifies the `nonce` has not been used before using the `ReplayStore`.
5.  **Posture Enforcement**: Enforces the presence and validity of L2 signatures and L3 proofs based on the configured posture.

### Layer 5: Actuator (Execution Boundary)
*Implementation: `/home/bob/g8e/internal/services/governance/l5_actuator.go`*

Once verified, the **L5 Actuator** executes the payload within a sovereign boundary.
-   **Rehydration**: Sensitive tokens (PII, API keys) scrubbed by the `Sovereignty Boundary Plane` are re-injected just before execution.
-   **Egress Dispatch**: Dispatches the verified payload to downstream executors (Shell, MCP servers, A2A agents).
-   **Signed Action Receipts**: Upon completion, the Actuator issues an `ActionReceipt` signed by the Operator's identity, providing an immutable proof of execution and result.

## 3. Governance Postures

Postures define which layers of the bedrock are enforced as "fail-closed" gates.

| Posture | L1 (Doctrine) | L2 (Consensus) | L3 (Notary) | Use Case |
| :--- | :---: | :---: | :---: | :--- |
| **Doctrine** | Required | Optional | Optional | Local development / Diagnostics |
| **Consensus** | Required | Required | Optional | Automated pipelines / Trusted agent ensembles |
| **Notary** | Required | Required | Required | **Production / High-stakes environment (Default)** |

## 4. Sovereignty Boundary Plane

The **Sovereignty Boundary Plane** handles data privacy through the transformation of the `GovernanceEnvelope`:
-   **Scrubbing**: Private data is replaced with opaque tokens before the envelope is sent to external LLMs for reasoning.
-   **Deterministic Rehydration**: The L5 Actuator performs local rehydration of tokens, ensuring that the LLM never sees raw secrets while the execution remains fully governed.
