---
title: Security
parent: Architecture
---

# Security Architecture

Last Updated: 2026-05-16
Version: v0.3.0

The **g8e Protocol** is a Zero-Trust substrate for human-verified action by autonomous systems. Security is the core constraint: the protocol assumes the AI control plane is potentially adversarial and enforces host safety through a 3-layer governance hierarchy and a unified transaction envelope.

The platform is split into the protocol substrate, an **Operator** role (implemented by [g8eo](../g8eo/README.md)), and optional application-layer adapters ([g8ee](../g8ee/README.md)). The Operator owns all security, trust, and execution responsibilities; the application layer holds no privileged trust.

## Bedrock Principles

1.  **Proof of Human Presence (PHP)**: The AI proposes; the human signs. No state-changing operation executes without an explicit, hardware-bound WebAuthn signature appended to the transaction.
2.  **Operator Sovereignty**: The Operator (`g8eo`) is the final arbiter of execution. It enforces hard gates and protocol invariants locally on the host. No client — bundled or BYO — can bypass these checks.
3.  **Zero-Trust Protocol**: No component or connection is implicitly trusted. Every request is carried by a `GovernanceEnvelope` that binds identity, context, and cryptographic evidence.
4.  **Fail-Closed Invariants**: Malformed envelopes, invalid signatures, or stale state roots result in immediate rejection. The system never "fails open".

## The Governance Pipeline

Every inbound request follows a strict fail-closed pipeline before any execution occurs:

1.  **Ingress & Decode**: The Operator receives a `GovernanceEnvelope` via UAP (JSON/protojson).
2.  **Transaction Verification**:
    *   **Integrity**: Validates the `transaction_hash` against the envelope content.
    *   **Liveness**: Checks `expires_at` and verifies `nonce` uniqueness (replay protection).
    *   **State Binding**: Rejects if the `state_merkle_root` does not match the current host state.
3.  **L1: Technical Bedrock**: Enforces forbidden patterns (e.g., `sudo`) via Protobuf reflection.
4.  **L2: Consensus (Tribunal)**: Verifies the cryptographic signature from a trusted agent ensemble.
5.  **L3: Authorization (PHP)**: For mutations, verifies a hardware-bound WebAuthn proof.
6.  **The Warden**: Final stop. Signs an `ActionReceipt` (intent), executes via the handler, and signs a final `ActionReceipt` (result).

## The Governance Envelope

The `GovernanceEnvelope` is the single canonical BFT transaction container for all g8e mutations.

| Field | Purpose |
|---|---|
| `id` | Unique hash of the envelope (must match `transaction_hash`). |
| `transaction_hash` | Deterministic hash computed from envelope fields for signing. |
| `state_merkle_root` | Binds the command to a specific host state root. |
| `governance` | Carries L1 status, L2 Tribunal signatures, and L3 Human signatures. |
| `payload` | Serialized bytes of the typed Protobuf message (e.g., `CommandRequested`). |
| `nonce` | Unique string to prevent replay attacks. |
| `expires_at` | Timestamp after which the transaction is void. |

## 3-Layer Governance Bedrock

### L1: Technical Bedrock (Hard Gates)
L1 provides non-negotiable safety invariants enforced at the Operator boundary.
-   **Reflected Validation**: `g8eo` uses Protobuf options (`forbidden_patterns`) to validate typed payloads before they reach the execution service.
-   **Command Scrubbing**: Global rejection of dangerous shell patterns and binary names.

### L2: Consensus (Tribunal)
The Consensus layer converts intent into executable commands using a verifiable proof from an ensemble of independent agents.
-   **Tribunal Signature**: An Ed25519 signature over `transaction_hash|decision`.
-   **Trusted Signers**: `g8eo` loads trusted public keys from the Operator-owned `SignerStore`. Transactions with unknown L2 keys are rejected.

### L3: Authorization (PHP Gate)
L3 involves explicit human authorization for all system mutations.
-   **Proof of Human Presence**: Real WebAuthn proofs (`L3Proof`) containing authenticator data and hardware-bound signatures.
-   **Auto-Approval**: Benign diagnostic commands (e.g., `uptime`) can be auto-approved via policy, but only *after* passing L1 and L2.

## The Execution Boundary (Warden)

The **Warden** is the only service permitted to trigger system mutations. It enforces:
-   **Fail-Closed Receipts**: If receipt signing or initial audit logging fails, execution is aborted.
-   **Action Receipts**: Every mutation generates two signed receipts:
    1.  **Executing**: Proof of intent to execute a verified transaction.
    2.  **Completed/Failed**: Final outcome with `state_root_after`.
-   **State Transition Proof**: Captures `state_root_before` and `state_root_after` to prove the exact impact of the mutation.

## Host Sovereignty & Audit

### Multi-Ledger Architecture
The Operator implements an isolated, git-based ledger for every session:
-   **Isolation**: Each operator session owns a unique git repository at `.g8e/data/ledger/sessions/<id>/`.
-   **Verifiable History**: Every file mutation is mirrored via a two-phase commit (`LedgerHashBefore` -> `LedgerHashAfter`).
-   **Encryption**: Session ledgers are stored encrypted at rest when the vault is unlocked.

### Encrypted Audit Vault
Every action and receipt is recorded in an encrypted SQLite database. The `AuditVaultService` is fail-closed; it rejects events missing valid session identifiers or malformed metadata.

### Output Scrubbing (Sentinel)
**Sentinel** performs dual-role analysis:
1.  **Defense**: Analyzes input commands for MITRE ATT&CK patterns.
2.  **Sovereignty**: Scrubs output for tokens, keys, and PII before it leaves the host.

## Network & PKI

### Operator-Owned PKI
g8e operates an internal PKI anchored in `.g8e/pki`:
-   **TLS 1.3 + mTLS**: Mandatory mutual authentication for all components.
-   **Short-Lived Certs**: The Operator issues certificates with workload identity (URI SANs).
-   **Revocation**: Strict enforcement of CRLs.

### Secrets Management
Authoritative secrets are stored in `.g8e/secrets` and managed via a dedicated `Vault`.
-   **No Secret Mounts**: Application adapters (g8ee) never gain access to raw substrate secrets.
-   **Tmpfs Isolation**: Container entrypoints write required material into tmpfs paths to avoid persistence of sensitive keys.

