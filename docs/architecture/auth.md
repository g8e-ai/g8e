# Authentication & Authorization

This document details the authentication and authorization architecture of the g8e platform. The platform is built as a zero-trust execution environment where every mutation is typed, signed, and governed via a deterministic verification pipeline.

## Overview

The platform security model is founded on two core pillars:
1. **Identity-Bound Communication (mTLS)**: Every connection within the platform, whether from a CLI, a Dashboard, or an AI Agent, must be authenticated via mutual TLS (mTLS) with a verified SPIFFE workload identity.
2. **5-Layer Verification Sequence**: Every mutation (command execution, file edit, tool call) must pass through the sequential 5-layer verification pipeline before execution.

---

## 1. Authentication & Workload Identity

The platform uses an internal Public Key Infrastructure (PKI) to issue and manage certificates. The **g8e Gateway** acts as the Certificate Authority (CA) and enforces identity validation.

### Workload Identity (SPIFFE)

Each system component receives a SPIFFE ID, embedded as a Uniform Resource Identifier (URI) in the Subject Alternative Name (SAN) of its mTLS certificate. See [Network Architecture](./network.md) for the complete SPIFFE ID format reference and implementation details.

### mTLS Enforcement

The g8e Gateway enforces TLS 1.3 for all L7 communication with strict mTLS requirements. See [Network Architecture](./network.md) for detailed mTLS enforcement policies, revocation mechanisms, and identity binding procedures.

### PKI Hierarchy & Trust Domain

The platform uses a four-tier PKI hierarchy issued by the g8e Gateway. See [Network Architecture](./network.md) for the complete PKI hierarchy, intermediate CA split rationale, curve policy, and revocation details.

### Enrollment & Bootstrap

Clients enroll in the platform using a Certificate Signing Request (CSR) bootstrap flow. See [Network Architecture](./network.md) for detailed enrollment procedures, including CSR-based enrollment and Windows Certificate Store enrollment with TPM-backed keys.

---

## 2. Network Security Foundation

The authentication architecture is built on a zero-trust networking model where all communication is authenticated via mutual TLS (mTLS) with verified SPIFFE workload identities. For detailed information on:

- PKI hierarchy and certificate management
- Workload identity (SPIFFE) formats
- mTLS enforcement and revocation
- Certificate enrollment and bootstrap flows
- Port topology and communication patterns
- g8e.local internal translation layer

See [Network Architecture](./network.md).

---

## 3. 5-Layer Verification Sequence (Interlock)

The platform implements a deterministic 5-layer governance sequence. Every mutation must pass through all active layers before execution. The structural schema is defined as `GovernanceEnvelope` in `protocol/proto/g8e/common/v1/common.proto:79-115`.

### Layer 1: Technical Bedrock (L1Doctrine)
*Implementation: `internal/services/governance/l1_doctrine.go:50`*

L1 is the foundational layer that executes deterministic security rules.
- **Forbidden Patterns**: Uses Protobuf field options (`forbidden_patterns`) to reject strings matching dangerous patterns.
- **MITRE Threat Detection**: Analyzes payloads against MITRE ATT&CK patterns for reverse shells, privilege escalation, credential access, and other threats.
- **Input Validation Framework**: Comprehensive validation system (`internal/services/mcp/validation.go`) with fail-closed security principles for MCP tool inputs, including SQL query validation, URL validation to prevent SSRF attacks, and protocol validation to prevent path traversal.
- **Hard Gates**: Rejects transactions immediately upon violation; cannot be bypassed by L2 or L3.

### Layer 2: Consensus (L2Consensus)
*Implementation: `internal/services/governance/l2_consensus.go:45`*

L2 provides multi-agent cryptographic verification of intent.
- **Ed25519 Signatures**: Verifies Ed25519 signatures over the `transaction_hash|decision` format.
- **Trusted Signers**: Requires signatures from trusted agents listed in the `SignerStore`.
- **Posture-Aware Enforcement**: Enforces signature requirements based on the configured `GovernancePosture`.

### Layer 3: Notary (L3Notary)
*Implementation: `internal/services/governance/l3_notary.go:32`*

L3 ensures explicit human authorization for mutations.
- **Suspension**: The g8e Gateway (g8eg) suspends transactions requiring L3 approval, storing them in the `suspended_transactions` pool.
- **Out-of-Band (OOB) Approval**: The user approves via CLI command (`g8e approve <tx_hash>`) with a cryptographic Ed25519 signature over the transaction hash, or via WebAuthn for web sessions.
- **L3Proof**: A successful approval generates an `L3Proof` containing the cryptographic signature and certificate fingerprint, cryptographically bound to the `transaction_hash`.

### Layer 4: Warden (L4Warden)
*Implementation: `internal/services/governance/l4_warden.go:372`*

The Warden is the final fail-closed gate before execution. It verifies:
1. **In-Flight Tracking**: Prevents concurrent processing of transactions with the same nonce.
2. **Nonce Reservation**: Early durable replay protection via `ReplayStore.ReserveNonce`.
3. **Stateless Validation**: Structural integrity, payload decoding, L1Doctrine compliance, and hash verification.
4. **Stateful Validation**: State root consistency check via `StateRootProvider`.
5. **Posture Validation**: L2 and L3 enforcement based on the configured `GovernancePosture` (Doctrine, Consensus, or Notary).

### Layer 5: Actuator (L5Actuator)
*Implementation: `internal/services/governance/l5_actuator.go:76`*

The Actuator represents the execution boundary and final audit commitment.
- **Egress Dispatch**: Dispatches the verified payload to downstream executors (Shell, MCP, A2A).
- **Sensitive Data Rehydration**: Rehydrates scrubbed placeholders (such as `{{UEI_1}}`) with original sensitive data just before execution via `RehydratePayload`.
- **Action Receipts**: Issues a signed `ActionReceipt` providing immutable proof of the outcome.
- **Commitment**: Records the transaction in the `SQLAuditStore` and chains it to the ledger.

---

## 4. Governance Postures

Postures define which layers of the bedrock are enforced as fail-closed gates.

| Posture | L1 (Doctrine) | L2 (Consensus) | L3 (Notary) | Use Case |
| :--- | :---: | :---: | :---: | :--- |
| **Doctrine** | Required | Optional | Optional | Local Dev / CI |
| **Consensus** | Required | Required | Optional | Automated Workflows |
| **Notary** | Required | Required | Required | **Production (Default)** |

---

## 5. Sovereign Execution Boundary

Handling sensitive data without leaking it to upstream models is managed by the Sovereign Execution Boundary:
- **Scrubbing**: Private data is replaced with opaque tokens (Uniform Element Identifiers, such as `{{UEI_1}}`) before sending to external LLMs.
- **Deterministic Rehydration**: The L5 Actuator performs local rehydration of tokens just before execution via `RehydratePayload`.
- **Data Sovereignty**: Raw secrets never leave the sovereign host environment.

---

## 6. Encryption at Rest

The platform enforces mandatory encryption for all sensitive data at rest. See [Encryption Architecture](./encryption.md) for complete details.

### Vault-Based Encryption

All storage services require an unlocked vault at initialization:
- **SQLAuditStore**: Encrypts audit records, governance envelopes, audit trail, and compliance records
- **ExecutionVaultService**: Encrypts execution results and command outputs
- **TokenStoreService**: Encrypts authentication tokens and session data

### Encryption Guarantees

- **Fail-closed**: Services fail to initialize without a vault
- **AES-256-GCM**: All data encrypted with NIST-approved algorithm
- **Key rotation**: Support for re-keying without data loss
- **Zero-knowledge**: Vault keys never written to disk in plaintext

### Vault Management

Vault operations are managed via CLI commands:
- `./g8e vault init`: Initialize new vault
- `./g8e vault unlock`: Unlock vault with key
- `./g8e vault rekey`: Rotate vault keys
- `./g8e vault status`: Check vault status
- `./g8e vault reset`: Destroy vault (destructive)

### Configuration

Vault paths can be configured via:
- CLI flags: `--vault-dir`, `--vault-key`
- Environment variables: `G8E_VAULT_DIR`, `G8E_VAULT_KEY`
- Configuration file: `paths_default.json`
