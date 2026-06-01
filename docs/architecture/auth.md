# Authentication & Authorization

This document details the authentication and authorization architecture of the g8e platform. The platform is built as a zero-trust execution environment where every mutation is typed, signed, and governed via a deterministic verification pipeline.

## Overview

The platform security model is founded on two core pillars:
1. **Identity-Bound Communication (mTLS)**: Every connection within the platform, whether from a CLI, a Dashboard, or an AI Agent, must be authenticated via mutual TLS (mTLS) with a verified SPIFFE workload identity.
2. **5-Layer Verification Sequence**: Every mutation (command execution, file edit, tool call) must pass through the sequential 5-layer verification pipeline before execution.

---

## 1. Authentication & Workload Identity

The platform uses an internal Public Key Infrastructure (PKI) to issue and manage certificates. The **Governance Gateway (g8eg)** acts as the Certificate Authority (CA) and enforces identity validation.

### Workload Identity (SPIFFE)

Each system component receives a SPIFFE ID, embedded as a Uniform Resource Identifier (URI) in the Subject Alternative Name (SAN) of its mTLS certificate. The generation logic is defined in `protocol/workload_identity.go`.

| Workload Type | SPIFFE ID Format | Reference |
| :--- | :--- | :--- |
| **g8e Operator (g8eo)** | `spiffe://g8e.local/operator/<organization_id>/<operator_id>/<operator_session_id>` | `protocol/workload_identity.go:35-39` |
| **CLI / BYO Client** | `spiffe://g8e.local/cli/<user_id>/<cli_session_id>` | `protocol/workload_identity.go:46-50` |
| **Application / Agent** | `spiffe://g8e.local/app/<operator_id>` | `protocol/workload_identity.go:57-61` |
| **Governance Gateway (g8eg)** | `spiffe://g8e.local/hub/operator-listen` | `protocol/workload_identity.go:68-72` |

### mTLS Enforcement

The Governance Gateway (g8eg) enforces TLS 1.3 for all L7 communication.
- **Strict mTLS**: The gateway requires and verifies client certificates using `tls.RequireAndVerifyClientCert`.
- **Revocation**: Certificates are checked against a database-backed revoked certificates store. Revocation is enforced at the Gateway.
- **Identity Binding**: Middleware verifies that the SPIFFE ID in the client certificate matches the specific session identifier (such as `operator_session_id` or `cli_session_id`) inside the `GovernanceEnvelope`.

### PKI Hierarchy & Trust Domain

The platform uses a three-tier PKI hierarchy issued by the Governance Gateway (g8eg):

| Tier | Certificate | Purpose | Validity |
| :--- | :--- | :--- | :--- |
| **Root CA** | `g8e Root CA` | Trust anchor for the entire platform | 3650 days |
| **Hub Intermediate CA** | `g8e Hub Intermediate CA` | Signs the gateway serving certificate | 3650 days |
| **Operator Intermediate CA** | `g8e Operator Intermediate CA` | Signs all leaf certificates (operator, CLI, app) | 3650 days |
| **Leaf Certificates** | operator-gateway, operator, CLI, app | End-entity identities for services and clients | 7 days |

**Intermediate Split Rationale**: The hub and operator intermediate CAs are kept separate to enforce a clean blast-radius boundary. The hub intermediate signs only the gateway's serving identity, while the operator intermediate signs delegated workload leaves. This separation allows the operator-issuing key to be rotated or revoked without touching the gateway's serving trust, and vice versa.

**Curve Policy**: All certificates (root, intermediates, serving, and leaves) use ECDSA P-256 for maximum interoperability with SPIFFE/SPIRE and TLS 1.3 stacks.

**Revocation**: Certificate revocation is enforced via a database-backed denylist checked per-request in the mTLS middleware. A standard X.509 CRL signed by the operator intermediate CA is served at `/.well-known/g8e/pki/crl` for external consumption.

### Enrollment & Bootstrap (CSR-based)

Clients enroll in the platform using a Certificate Signing Request (CSR) bootstrap flow:
1. **CA Discovery**: Clients fetch the platform root CA bundle from the endpoint `/.well-known/g8e/pki/ca-bundle`.
2. **CSR Submission**: Clients generate a local ECDSA P-256 key pair and submit a CSR to `/api/v1/pki/csr/sign`.
3. **Registration**: The Governance Gateway (g8eg) validates the CSR and binds the certificate to a user identity via invitation-based Just-In-Time (JIT) provisioning.
4. **Session Issuance**: Upon successful enrollment, the Governance Gateway (g8eg) issues a specific `operator_session_id` or `cli_session_id`.

### Windows Certificate Store Enrollment

Windows users can enroll via the Windows Certificate Store for seamless browser authentication:
1. **CLI Enrollment**: Run `./g8e auth enroll-windows [--tpm]` to generate an ECDSA P-256 keypair in the Windows Personal store.
2. **CSR Signing**: The CLI submits a CSR to the Gateway and receives a signed certificate with SPIFFE URI SAN.
3. **Certificate Import**: The signed certificate is imported to `Cert:\CurrentUser\My` in the Windows Certificate Store.
4. **Browser Authentication**: Chrome and Edge automatically present certificates from the Windows Personal store when the Gateway issues a TLS CertificateRequest.
5. **Session Binding**: The Gateway extracts the SPIFFE URI SAN from the client certificate and creates a `web_session_id` bound to the user identity.

**TPM-Backed Keys**: The `--tpm` flag uses the Microsoft Platform Crypto Provider KSP to generate keys backed by Windows Hello for Business, providing hardware-bound L3 presence proofs.

---

## 2. 5-Layer Verification Sequence (Interlock)

The platform implements a deterministic 5-layer governance sequence. Every mutation must pass through all active layers before execution. The structural schema is defined as `GovernanceEnvelope` in `protocol/proto/g8e/common/v1/common.proto:81-115`.

### Layer 1: Technical Bedrock (L1Doctrine)
*Implementation: `internal/services/governance/l1_doctrine.go:35-50`*

L1 is the foundational layer that executes deterministic security rules.
- **Forbidden Patterns**: Uses Protobuf field options (`forbidden_patterns`) to reject strings like `sudo` or `su`.
- **MITRE Threat Detection**: Analyzes payloads against MITRE ATT&CK patterns.
- **Hard Gates**: Rejects transactions immediately upon violation; cannot be bypassed by L2 or L3.

### Layer 2: Consensus (L2Consensus)
*Implementation: `internal/services/governance/l2_consensus.go:27-45`*

L2 provides multi-agent cryptographic verification of intent.
- **Ed25519 Signatures**: Verifies Ed25519 signatures over the `transaction_hash|decision` format.
- **Trusted Signers**: Requires signatures from trusted agents listed in the `SignerStore`.
- **Posture-Aware Enforcement**: Enforces signature requirements based on the configured `GovernancePosture`.

### Layer 3: Notary (L3Notary)
*Implementation: `internal/services/governance/l3_notary.go:29-35`*

L3 ensures explicit human authorization for mutations.
- **Suspension**: The Governance Gateway (g8eg) suspends transactions requiring L3 approval, storing them in the `suspended_transactions` pool.
- **Out-of-Band (OOB) Approval**: The user approves via WebAuthn/Passkey at the `/api/v1/approve/{txHash}` endpoint, or via mTLS certificate fingerprint.
- **L3Proof**: A successful approval generates an `L3Proof` cryptographically bound to the `transaction_hash`.

### Layer 4: Warden (L4Warden)
*Implementation: `internal/services/governance/l4_warden.go:306-320`*

The Warden is the final fail-closed gate before execution. It verifies:
1. **Structural Integrity**: Structural integrity, payload decoding, and L1Doctrine compliance.
2. **Hash Verification**: Matches the `id` and `transaction_hash` fields against the recomputed SHA-256 hash.
3. **State Root Consistency**: Ensures the `state_merkle_root` matches the current platform state.
4. **Replay Protection**: Verifies the `nonce` using the `ReplayStore`.
5. **Posture Enforcement**: Enforces L2 and L3 requirements based on the configured `GovernancePosture`.

### Layer 5: Actuator (L5Actuator)
*Implementation: `internal/services/governance/l5_actuator.go:51-70`*

The Actuator represents the execution boundary and final audit commitment.
- **Egress Dispatch**: Dispatches the verified payload to downstream executors (Shell, MCP, A2A).
- **Action Receipts**: Issues a signed `ActionReceipt` providing immutable proof of the outcome.
- **Commitment**: Records the transaction in the `AuditVaultService` and chains it to the ledger.

---

## 3. Governance Postures

Postures define which layers of the bedrock are enforced as fail-closed gates.

| Posture | L1 (Doctrine) | L2 (Consensus) | L3 (Notary) | Use Case |
| :--- | :---: | :---: | :---: | :--- |
| **Doctrine** | Required | Optional | Optional | Local Dev / CI |
| **Consensus** | Required | Required | Optional | Automated Workflows |
| **Notary** | Required | Required | Required | **Production (Default)** |

---

## 4. Sovereignty Boundary Plane

Handling sensitive data without leaking it to upstream models is handled by the Sovereignty Boundary Plane:
- **Scrubbing**: Private data is replaced with opaque tokens before sending to external LLMs.
- **Deterministic Rehydration**: The L5 Actuator performs local rehydration of tokens just before execution.
- **Data Sovereignty**: Raw secrets never leave the sovereign host environment.
