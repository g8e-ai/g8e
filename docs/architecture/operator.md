---
title: g8e Operator
---

# g8e Operator

Last Updated: 2026-05-25

The **g8e Operator** is the host-side, sovereign agent role defined by the g8e Protocol: a daemon that functions as the remote execution target and universal protocol translator under the security guarantees of the platform. An Operator receives transactions, enforces L1/L2/L3 verification, executes through a defensive boundary, and emits signed receipts anchored to a host-local ledger.

The reference Operator is **`g8eo`** (built as the `g8e` binary). It functions as a sovereign, **Governed Operator** and **Model Context Protocol (MCP) Server**, serving as the Policy Execution Point (PEP). The exact same compiled Go codebase is used to power both sides of the governance boundary:

- **Governance Gateway (PDP)**: When run in Gateway mode (`--doctrine`, `--consensus`, `--notary`), it acts as the central Policy Decision Point (PDP) with platform persistence and in-process pub/sub brokering.
- **g8e Operator (PEP)**: When run as a host agent, it acts as the Policy Execution Point (PEP) and MCP server.

This document focuses on the **Governed Operator** (PEP) role.

---

## 1. Introduction

The core invariant of the Operator is absolute defense-in-depth: a typed, signed, state-bound transaction reaches a sovereign host agent that distrusts upstream inputs and refuses to mutate reality unless every independent proof checks out. 

The Operator is the only component capable of mutating the host. It executes remote-operations work—running shell commands, editing files, interacting with cloud CLIs, and reading file history—but only after translating the request into a canonical governance transaction and verifying it locally.

---

## 2. The 5-Layer Governance Gauntlet

When a command targets an Operator, it progresses through a strict, fail-closed pipeline consisting of five distinct layers of verification and execution:

### L1: Doctrine (Technical Bedrock)
The **L1Doctrine** layer provides foundational hard gates. It utilizes Protobuf field-option extensions (`forbidden_patterns`) to block malicious strings and executes real-time MITRE ATT&CK heuristics to detect threats like reverse shells, privilege escalation, and destructive disk operations. L1 is the first line of defense and cannot be bypassed.

### L2: Consensus
The **L2Consensus** layer verifies the intent of the request via a Byzantine Fault Tolerant (BFT) quorum. It validates Ed25519 signatures from independent reasoning agents against the Operator's locally trusted `SignerStore`. This ensures that no single upstream agent can unilaterally mutate the host. The specific consensus implementation (e.g., Tribunal) is an application-layer concern.

### L3: Notary (Authorization)
The **L3Notary** layer enforces human-in-the-loop authorization. For web-based sessions, it validates FIDO2/WebAuthn (Passkey) proofs. For CLI or BYO client sessions, it validates mTLS certificate fingerprints. Mutations are blocked until a valid L3 proof is presented, unless specifically exempted by an `AutoApprove` policy for benign diagnostic commands.

### L4: Warden (Pre-dispatch Gate)
The **L4Warden** is the final verification gate before execution. It enforces:
1. **Integrity**: Validates that `id == transaction_hash == SHA256(canonical_fields)`. The wire format is canonical JSON (`protojson`), but the signing basis is a deterministic hash of normalized fields.
2. **Freshness**: Enforces `expires_at` and checks for replay attacks via a local `ReplayStore`.
3. **State Binding**: Validates that the `state_merkle_root` matches the host's current ledger root.
4. **Quorum**: Confirms that L1, L2, and L3 proofs meet the current **Governance Posture** (`doctrine`, `consensus`, or `notary`).

### L5: Actuator (Execution Boundary)
The **L5Actuator** is the singular execution boundary permitted to mutate host state. It dispatches verified payloads to internal handlers (shell, file edit, etc.) and uses a **dual-receipt model**:
1. **Pre-execution**: Signs an `ActionReceipt` with status `EXECUTING` and commits it to the local `AuditVaultService`.
2. **Rehydration**: Restores sensitive data (PII, credentials) that was scrubbed upstream, using local tokens from the **Sovereignty Boundary Plane**.
3. **Execution**: Dispatches to the handler and captures the output.
4. **Post-execution**: Signs a final `ActionReceipt` with status `COMPLETED` or `FAILED`, captures the new `state_root_after`, and publishes the signed result back to the Gateway.

---

## 3. Core Subsystems

### Universal Protocol Translator
By exposing standard MCP and A2A interfaces (`--mcp-serve`), the Operator acts as the admission gate for BYO (Bring-Your-Own) AI clients. It isolates the complex requirements of the `GovernanceEnvelope` (such as transaction hashing and L2/L3 signature collection) behind a standardized tool-calling facade, mapping native JSON-RPC requests directly to governed `ActionType` mutations.

### Identity, PKI, and mTLS
The Operator establishes workload identity bound to SPIFFE-style URI SANs, strictly enforced over mutual TLS (mTLS):
- **Operator Identity**: `spiffe://g8e.local/operator/<organization_id>/<operator_id>/<operator_session_id>`
- **CLI/BYO Client**: `spiffe://g8e.local/cli/<user_id>/<cli_session_id>`
- **App Workload**: `spiffe://g8e.local/app/<operator_id>`

Revocation is checked on every handshake. Every `ActionReceipt` is signed by a host-unique Ed25519 key, providing a cryptographic proof of host execution.

### JWT Authentication Isolation
The Operator is fully isolated from Identity Providers (IdP). The Gateway handles all JWT validation, user provisioning, and role mapping. JIT provisioning is **owner-controlled** and requires an active invitation:
- **Owner-Centric Model**: All authentication requires owner approval via device links. The platform owner creates invitations for specific identities (IdP `sub` or email) before JIT provisioning can occur.
- **Invitation-Based JIT**: When a JWT is presented, the Gateway validates the signature and checks for an active invitation. If no invitation exists, authentication is rejected (403 Forbidden). If a valid invitation exists, the user is provisioned and bound to the owner's organization, then the invitation is consumed.
- **Strict TTL**: Device links and sessions have a 1-hour TTL by default. Long-lived access requires programmatic renewal or re-authentication via the device-link flow.
- **Gateway Responsibility**: The Gateway validates inbound `Authorization: Bearer <JWT>` tokens, performs invitation-gated JIT user provisioning, maps JWT roles to Personas, and injects `tenant_id` and `binding_persona` into the `GovernanceEnvelope`.
- **Operator Responsibility**: The Operator receives only the pre-validated, enriched security metadata in the envelope. It decodes `tenant_id` and `binding_persona` from the envelope, propagates them into the execution context, and applies Persona-based data scrubbing (column masks, redaction) before returning results.
- **No IdP Dependency**: The Operator never requires outbound internet access to verify tokens or manage user state. This enables air-gapped and high-security deployments where the Operator has no external network connectivity.

### Local-First Audit Architecture (LFAA)
The host is the authoritative source of truth for all mutations.
- **AuditVaultService**: An append-only SQLite log of every event and signed `ActionReceipt`. It is fail-closed: events missing a valid `operator_session_id` are rejected.
- **Scrubbed vs. Raw Logs**: Sovereignty scrubbing separates logs into a **Scrubbed Vault** (safe for AI reading) and a **Raw Vault** (unscrubbed forensic record for human security audits).
- **Git-Backed Ledger**: Implements a two-phase commit (`state_root_before` / `state_root_after`) for file mutations using native `go-git`. Files are mirrored and can be restored to any prior state.

---

## 4. Governance & Safety

- **Sovereignty Boundary Plane**: Data sovereignty is enforced at the boundary. Sensitive data is scrubbed before leaving the host and replaced with tokens (`{{UEI_N}}`). These tokens are rehydrated by the `L5Actuator` only at the moment of execution.
- **Strict Canonical JSON**: While schemas are defined in Protobuf, the wire format for all client-facing surfaces is strictly canonical JSON (`protojson`) for maximum ecosystem compatibility.
- **No Backward Compatibility**: The Operator enforces the current strict 3-Layer governance protocol. Legacy formats, HMAC fallbacks, and unsigned inputs are rejected. 

---

## 5. Current Implementation Status

The reference implementation (`g8eo`) currently supports:

- **Universal Protocol Translation** — Functional MCP and A2A gateway mapping standard tool calls to signed `GovernanceEnvelope` mutations.
- **Fail-Closed 5-Layer Gauntlet** — L1 (Doctrine), L2 (Consensus), and L4 (Warden) gates are fully enforced on every transaction.
- **Outbound-Only mTLS Connectivity** — Dial-out reverse tunnels with zero inbound port requirements.
- **Local-First Audit Vault** — Git-backed ledger and fail-closed SQLite audit vault enforcing session existence for all writes.
- **Deterministic Hash Binding** — SHA-256 transaction hash integrity enforced across all wire formats.
- **Sovereignty Boundary** — Automated scrubbing and rehydration of sensitive data during the execution lifecycle.
- **Host-Unique Signing** — Cryptographic Action Receipts signed by host-specific keys.
- **Zero-Dependency Binary** — Statically compiled Go binary for air-gapped and high-security deployments.

---

## 6. Implementation Reference

| Concern | Authoritative file |
|---|---|
| Ingress Verification (`L4Warden`) | `internal/services/governance/l4_warden.go` |
| Execution Boundary (`L5Actuator`) | `internal/services/governance/l5_actuator.go` |
| Sovereignty (Data Scrubbing) | `internal/services/sovereignty/boundary.go` |
| Technical Bedrock (`L1Doctrine`) | `internal/services/governance/l1_doctrine.go` |
| Consensus (`L2Consensus`) | `internal/services/governance/l2_consensus.go` |
| Notary (`L3Notary`) | `internal/services/governance/l3_notary.go` |
| Local Audit Vault | `internal/services/storage/audit_vault.go` |
| Native Git Ledger | `internal/services/storage/ledger.go` |
| Operator Entrypoint | `cmd/g8eo/main.go` |
| Protocol Definitions | `protocol/proto/g8e/common/v1/common.proto` |
| Operator Protocol | `protocol/proto/g8e/operator/v1/operator.proto` |
| Workload Identity | `protocol/workload_identity.go` |
| Event Constants | `protocol/constants/events.json` |
| Port Constants | `protocol/constants/ports.json` |

See also: [g8e Protocol](./protocol.md), [Governance Gateway](./gateway.md).
