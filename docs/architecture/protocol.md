---
title: Protocol
parent: Architecture
---

# g8e Protocol

Last Updated: 2026-05-12
Version: v0.2.4

The g8e Protocol is the substrate of the platform: a **UAP JSON** wire contract for secure, auditable interaction between any g8e client and any host-side **Operator** implementation. An Operator is a role defined by this protocol — receive signed transactions, enforce L1/L2/L3 verification, execute through a defensive boundary, emit signed receipts anchored to a local ledger.

The reference Operator implementation is **`g8eo`** (Go), and the bundled reference application includes **Engine (`g8ee`)** and **Dashboard (``)**. The protocol is designed for any Bring-Your-Own (BYO) Operator, frontend, or agent system; the bundled components are illustrative, not privileged.

# Core Invariants

1. **Canonical JSON Wire Format**: All mutation commands MUST use canonical JSON (protojson) `GovernanceEnvelope` on all client-facing surfaces (HTTPS APIs, WSS pub/sub command channels, receipts, audit exports). Binary protobuf bytes on the wire are rejected with a clear error. Schema source of truth is `.proto` files (typed, versioned, L1 field-option reflection).
2. **Hash-Based Signing**: The signing basis is a deterministic `transaction_hash` computed from normalized envelope fields (action_type, target_resource, payload as base64, state_merkle_root, nonce, expires_at, intent_data). Wire encoding is irrelevant because the verifier enforces `id == computed transaction_hash`. Canonicalization rules: field names in proto definition order, strings as UTF-8, numbers as decimal integers, absent optional fields omitted, nested messages recursed, bytes as base64, result hashed with SHA-256.
3. **Identity Persistence**: Every message must carry `id`, `operator_id`, and `operator_session_id`.
4. **Immutable Governance**: Governance metadata (L1/L2/L3) is baked into the envelope and verified by the Operator before any action is taken.
5. **BFT State Binding**: Commands are bound to a specific fleet state via `state_merkle_root`. The root is content-authoritative, calculated deterministically over stable business data (Documents, KV, Blobs) while excluding volatile metadata (nonces, SSE events) and metadata-only timestamps.
6. **Unified Transaction Audit**: Every mutation accepted or rejected by the Operator emits an authoritative `ActionReceipt`. These are stored in a dedicated `receipts` table and are queryable via transaction-native APIs.
7. **No Private Channels**: Bundled apps use the same public protocol surface as BYO clients. There is no internal "trust" shortcut for bundled components.

---

# Message Models

The canonical schema files live in `@/home/bob/g8e/shared/proto/`:

| File | Purpose |
|------|---------|
| `common.proto` | Defines component identity enums and custom protobuf options (forbidden_patterns). |
| `operator.proto` | Defines typed request/result payloads (commands, file ops, heartbeats, PKI, device-links, passkeys). |

## UAP JSON Mutation Envelope

The UAP envelope is the only canonical mutation transport. It is JSON on the wire for AI-agent interoperability, while `payload` is the required base64-encoded serialized Protobuf action message that is the sole authority for execution.

| Field | Role |
|-------|------|
| `protocol_version` | Protocol version string. |
| `id` | SHA-256 transaction hash over critical intent/context fields. |
| `timestamp` | UTC creation time. |
| `expires_at` | UTC timestamp after which the transaction is invalid. |
| `nonce` | Random salt or monotonic counter for replay protection. |
| `metadata.sender_id` | Operator identity (mTLS cert signature or equivalent). |
| `intent.action_type` | Action type (e.g., "EXECUTE_BASH", "QUERY_DB"). |
| `intent.target_resource` | Target resource identifier. |
| `context.data_format` | Data format (e.g., "markdown", "raw", "json"). |
| `intent_data` | Structured intent parameters for audit visibility only. |
| `payload` | Base64 binary Protobuf message. **SOLE authority for execution**. |
| `governance` | L1/L2/L3 metadata for security enforcement. |
| `state_merkle_root` | Fleet state root for BFT verification. |
| `case_id`, `investigation_id`, `task_id` | Application-layer identifiers for tracking. |

---

# Protocol Lifecycle

## 1. Request Phase (Client -> Operator)

1. **Generation**: A client (e.g., `g8ee` or a BYO agent) generates a typed payload (e.g., `CommandRequestPayload`).
2. **Envelope Building**: The payload is wrapped in a UAP JSON `GovernanceEnvelope`.
3. **L2 Signing**: The trusted L2 signer signs `transaction_hash|true` with ED25519 and sets `governance.l2.key_id` plus `governance.l2.tribunal_signature`.
4. **Publication**: The serialized envelope is published to the Operator's command channel (WSS) or submitted via HTTPS.

## 2. Verification Phase (Operator)

Upon receiving an envelope, a conforming Operator must perform these checks before any action. In the reference Operator (`g8eo`), they live in `@/home/bob/g8e/components/g8eo/internal/services/pubsub/pubsub_commands.go`:

1. **Parsing**: Rejects any payload that is not a valid UAP JSON `GovernanceEnvelope`.
2. **Expiry Check**: Rejects any transaction if `expires_at` is in the past.
3. **Replay Protection**: Rejects missing or reused `nonce` values using the durable replay store.
4. **State Check**: Requires `state_merkle_root` and compares it against the Operator-local current state root. Missing or mismatched roots reject.
5. **L1 Check**: Uses Protobuf reflection to find fields marked with `forbidden_patterns`. If a regex matches (e.g., `sudo`), the command is rejected.
6. **L2 Check**: Verifies the ED25519 signature against the Operator-owned `SignerStore` (database-backed). Filesystem-only signer management is deprecated.
7. **L3 Check**: For mutation requests, verifies the `L3Proof` (WebAuthn assertion). Missing verifier configuration rejects.
8. **Execution**: The Operator's execution boundary (the **Warden** in the reference implementation) receives the verified transaction and routes exactly one typed action to the appropriate handler.

## 3. Result Phase (Operator -> Client)

1. **Execution**: The handler executes the requested action and captures results.
2. **Envelope Building**: The Operator wraps the result payload (e.g., `CommandResult`) in a UAP JSON `GovernanceEnvelope`.
3. **Publication**: The result envelope is published to the results channel for the subscribed client(s).

---

# Governance Layers

### L1: Technical Bedrock (Hard Gates)
Enforced by the Operator's transaction verifier via Protobuf reflection before execution. In the reference Operator this is `TransactionVerifier`.
- **Mechanism**: Custom field option `forbidden_patterns`.
- **Scope**: Applied to fields like `CommandRequested.command`.
- **Default Patterns**: `sudo`, `su`, `rm -rf /`.

### L2: Consensus (Consensus)
Enforced by the Operator's transaction verifier. Reference implementation: `@/home/bob/g8e/components/g8eo/internal/services/governance/transaction_verifier.go`.
- **Signer**: Any trusted consensus agent or validator (e.g., `g8ee` in the reference application, or a BYO consensus implementation).
- **Verifier**: The Operator (reference implementation: `g8eo`).
- **Mechanism**: ED25519 asymmetric signatures. There is no HMAC fallback.
- **Attribution**: Uses `key_id` in `GovernanceMetadata` for O(1) public key lookup.
- **Material**: `transaction_hash|true`.

### L3: Authorization (Approval)
Human-in-the-loop authorization via Passkeys or BYO approval systems.
- **Mechanism**: Hardware-bound signatures or verifiable approval proofs (`governance.l3.proof`).
- **Auto-Approval**: Benign commands (e.g., `uptime`) can be marked `auto_approved` by the Consensus layer, but L3 NEVER bypasses L1 or L2.

---

# API Reference (Substrate)

## Audit & Verification

### `GET /api/audit/receipts`
Query for signed execution proof (ActionReceipts).
- **Parameters**: `transaction_id`, `operator_session_id`, `limit`, `offset`.
- **Auth**: Requires mTLS or valid session.

### `GET /api/audit/receipts/export`
SIEM-ready NDJSON export of action receipts.
- **Parameters**: `since` (RFC3339 or sqlite format), `limit`.
- **Auth**: Requires mTLS or valid session.
- **Content-Type**: `application/x-ndjson`

### `GET /api/governance/signers`
List all trusted L2 signers.

### `POST /api/governance/signers`
Add a new trusted L2 signer.
- **Body**: `{ "id": "agent-1", "public_key_hex": "...", "enabled": true }`

### `DELETE /api/governance/signers/{id}`
Revoke/delete a trusted L2 signer.

### `GET /health`
Returns Operator status and the current `state_merkle_root`.

---

# Data Conversions

When the reference Engine (`g8ee`) decodes a result from the reference Operator (`g8eo`), it parses the UAP JSON `GovernanceEnvelope` and then parses the typed payload. A BYO client performs the same steps:

Compatibility field shims are not part of the substrate protocol.

---

# Implementation Map

The protocol is the authoritative contract. The files below are the reference implementations bundled in this repository.

| Responsibility | Authoritative File |
|----------------|--------------------|
| **Schemas (substrate)** | `@/home/bob/g8e/shared/proto/` |
| **Event Registry (substrate)** | `@/home/bob/g8e/shared/constants/events.json` |
| **Reference Request Builder (PY, g8ee)** | `@/home/bob/g8e/components/g8ee/app/utils/envelope_builder.py` |
| **Reference Inbound Dispatch (GO, g8eo)** | `@/home/bob/g8e/components/g8eo/internal/services/pubsub/pubsub_commands.go` |
| **Reference L1/L2/L3 Verification (GO, g8eo)** | `@/home/bob/g8e/components/g8eo/internal/services/governance/transaction_verifier.go` |
| **Reference Result Publisher (GO, g8eo)** | `@/home/bob/g8e/components/g8eo/internal/services/pubsub/pubsub_results.go` |

---

# Related Documentation

- [Events](events.md) documents canonical event naming and lifecycle semantics.
- [Pub/Sub Channels](pubsub.md) documents channel-level routing and subscription behavior.
- [Governance & Mechanism Design](governance.md) documents the L1/L2/L3 safety model.
- [Security](security.md) documents platform security mechanisms around execution and auditability.
