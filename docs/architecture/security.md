---
title: Security
parent: Architecture
---

# Security Architecture

Last Updated: 2026-05-13
Version: v0.2.5

g8e is a Zero-Trust Operator/protocol substrate for secure infrastructure operations. Security is not an "add-on" but the core constraint: the platform assumes the AI control plane is potentially adversarial or error-prone and enforces safety at the infrastructure level through a 3-layer governance hierarchy and a unified protocol architecture.

The platform is explicitly split into a mandatory **Substrate** (g8eo/Operator) and optional **Application-Layer Adapters** (g8ed/Dashboard, g8ee/Engine). The Substrate owns all security, trust, and execution responsibilities.

## Bedrock Principles

1.  **Proof of Human Presence (PHP)**: The AI proposes; the human signs. No state-changing operation executes without an explicit, hardware-bound signature (Passkey/WebAuthn) appended to the transaction envelope.
2.  **Substrate Sovereignty**: The Operator (`g8eo`) is the mandatory substrate. It is the final arbiter of execution, enforcing hard gates and protocol invariants locally on the target host.
3.  **Protocol-First Zero Trust**: No component or connection is implicitly trusted. Every request is carried by a Protobuf `UniversalEnvelope` that binds identity, context, and cryptographic governance evidence.
4.  **Fail-Closed Invariants**: Malformed envelopes, invalid signatures, or stale state roots result in immediate rejection. The system never "fails open".

## Technical Positioning

-   **vs. SSH**: SSH is a secure pipe; g8e is a **governor**. g8e uses the pipe to enforce a governance model (scrubbing, consensus) that SSH cannot.
-   **vs. Teleport / Boundary**: These manage **human** access. g8e manages **AI-powered automation** acting on behalf of humans.
-   **vs. Ansible / Terraform**: These are deterministic. g8e is for **non-deterministic** investigation where the AI reasons about real-time state before proposing actions.

## The Execution Boundary (Warden)

All system mutations must pass through the **Warden** — the final stop for all transactions. The Warden enforces the "Execution Boundary" pattern: no code path can trigger a shell command or file edit without passing through the Warden's authorization gate.

### Fail-Closed Transaction Gate
Before reaching the Warden, every inbound request passes through a strict `TransactionVerifier` that enforces:
-   **Protobuf Integrity**: Decodes the canonical `UniversalEnvelope`.
-   **Action Type Validation**: Rejects unknown or unauthorized action types.
-   **Hash Verification**: Validates the `id` (hash) of the entire envelope.
-   **Replay Protection**: Enforces expiry (`expires_at`) and nonce uniqueness.
-   **State Binding**: Rejects transactions with a `state_merkle_root` that does not match the current host state.

## 3-Layer Governance Hierarchy

### L1: Technical Bedrock (Hard Gates)
L1 provides hardcoded, non-negotiable safety invariants enforced at the Operator boundary via Protobuf reflection.
-   **Forbidden Patterns**: Global rejection of dangerous shell patterns (e.g., `sudo`, `su`, `rm -rf /`).
-   **Allowlist/Denylist**: Configurable filters for binary names and substrings.
-   **Reflected Validation**: `g8eo` uses Protobuf options to validate typed payloads before they even reach the execution service.

### L2: Consensus (Tribunal)
The Consensus layer converts intent into executable commands using a verifiable proof of consensus from an ensemble of independent agents.
-   **Tribunal Signature**: An Ed25519 signature from a trusted L2 signer (e.g., an agent ensemble).
-   **Trusted Signers**: `g8eo` loads trusted public keys from `.g8e/pki/trusted_signers/*.pub`. A first-boot Operator can start before signers are provisioned, but every transaction with a missing or unknown L2 key is rejected.
-   **Quorum Enforcement**: The Warden ensures that the required number of valid consensus votes are present.

### L3: Authorization (PHP Gate)
L3 involves explicit human authorization, governed by the **Auditor-User Partition**.
-   **Proof of Human Presence (PHP)**: Hardware-bound Passkey/WebAuthn signatures.
-   **Auto-Approval**: Benign diagnostic commands (e.g., `uptime`, `df`) can be auto-approved, but only *after* passing L1 and L2. Auto-approval **NEVER** bypasses hard gates.

## The Universal Envelope

The `UniversalEnvelope` is the single canonical BFT transaction container for all g8e mutations.

| Field | Purpose |
|---|---|
| `id` | Unique hash of the envelope for tracking and correlation. |
| `state_merkle_root` | Binds the command to a specific host state root. |
| `governance` | Carries L1 status, L2 Tribunal signatures, and L3 Human signatures. |
| `payload` | Serialized bytes of the typed Protobuf message (e.g., `CommandRequested`). |
| `nonce` | Unique string to prevent replay attacks. |
| `expires_at` | RFC3339 timestamp after which the transaction is void. |

## Network & PKI

### Operator-Owned PKI
g8e operates its own internal PKI anchored in `.g8e/pki`:
-   **TLS 1.3 Only**: All communication is secured via TLS 1.3.
-   **mTLS Everywhere**: Mutual authentication is mandatory for all component communication.
-   **Short-Lived Certs**: The Operator issues short-lived certificates for workload identity.
-   **Revocation**: Strict enforcement of certificate revocation lists (CRLs).

### Secrets Management
Authoritative secrets are stored in `.g8e/secrets` and managed via a dedicated `Vault` service:
-   **Encrypted at Rest**: Sensitive data (audit logs, secrets) is encrypted using **AES-256-GCM**.
-   **No Secret Mounts**: Application-layer adapters (g8ed, g8ee) do not have access to substrate secrets.
-   **Tamper Evidence**: Startup checks verify SHA-256 digests of all secret material.

## Sovereignty & Audit

### Encrypted Audit Vault
Every action is recorded in an encrypted SQLite database on the Operator host. Output is scrubbed by **Sentinel** for credentials and PII before being recorded or transmitted.

### Git Ledger
Every file mutation is mirrored into a hidden `.g8e/ledger` Git repository, providing a verifiable, diffable history of all AI-driven edits.

### Output Scrubbing (Sentinel)
Sentinel performs dual-role analysis:
1.  **Defense**: Analyzes input commands for MITRE ATT&CK patterns before execution.
2.  **Sovereignty**: Scrubs output for tokens, keys, and PII before it leaves the host substrate.

