---
title: Protocol
parent: Architecture
---

# g8e Protocol

Last Updated: 2026-05-12
Version: v0.2.4

The g8e Protocol is a **Protobuf-first** communication layer designed for secure, auditable, and Byzantine Fault Tolerant (BFT) interaction between any g8e client and a host-local **Operator (g8eo)**.

While g8e includes bundled application-layer adapters—**Dashboard (g8ed)** and **Engine (g8ee)**—the protocol is designed for any Bring-Your-Own (BYO) frontend or agent system.

# Core Invariants

1. **Protobuf-Only Wire Format**: All cross-component operator traffic (commands, results, status updates) MUST use serialized `GovernanceEnvelope` messages. JSON fallbacks are rejected.
2. **Identity Persistence**: Every message must carry an `id`, `operator_id`, and `operator_session_id`.
3. **Immutable Governance**: Governance metadata (L1/L2/L3) is baked into the envelope and verified by the Operator before any action is taken.
4. **BFT State Binding**: Commands are bound to a specific fleet state via `state_merkle_root`.
5. **No Private Channels**: Bundled apps use the same public protocol surface as BYO clients. There is no internal "trust" shortcut for bundled components.

---

# Message Models

The canonical schema files live in `@/home/bob/g8e/shared/proto/`:

| File | Purpose |
|------|---------|
| `common.proto` | Defines `GovernanceEnvelope`, component identity, and governance metadata. |
| `operator.proto` | Defines request/result payloads (commands, file ops, heartbeats, etc.). |
| `pubsub.proto` | Defines the low-level pub/sub carrier messages. |

## GovernanceEnvelope

The `GovernanceEnvelope` is the root container for all protocol traffic. It binds an event name to typed payload bytes and provides the necessary context for governance.

| Field | Role |
|-------|------|
| `id` | Unique UUID v4 for the message. |
| `timestamp` | UTC creation time. |
| `source_component` | Identifies the source (e.g., `COMPONENT_G8EE`, `COMPONENT_G8EO`, `COMPONENT_G8ED`, or BYO client). |
| `event_type` | Canonical event string from `shared/constants/events.json`. |
| `operator_id` | Target or source operator identity. |
| `state_merkle_root` | Fleet state root for BFT verification. |
| `expires_at` | UTC timestamp after which the transaction is invalid. |
| `nonce` | Random salt or monotonic counter for replay protection. |
| `governance` | L1/L2/L3 metadata for security enforcement. |
| `payload` | Serialized bytes of the typed message (e.g., `CommandRequested`). |

---

# Protocol Lifecycle

## 1. Request Phase (Client -> Operator)

1. **Generation**: A client (e.g., `g8ee` or a BYO agent) generates a typed payload (e.g., `CommandRequestPayload`).
2. **Envelope Building**: The payload is wrapped in a `GovernanceEnvelope`.
3. **L2 Signing**: The envelope is signed using the configured L2 mechanism (e.g., HMAC-SHA256 or asymmetric signature). For asymmetric signatures, the material is a pipe-delimited canonical string: `ID | Timestamp | EventType | OperatorID | SessionID | StateRoot | Expiry | Nonce | Payload`.
4. **Publication**: The serialized envelope is published to the Operator's command channel (WSS) or submitted via HTTPS.

## 2. Verification Phase (Operator)

Upon receiving an envelope, `g8eo` performs these checks in `@/home/bob/g8e/components/g8eo/services/pubsub/pubsub_commands.go`:

1. **Parsing**: Rejects any payload that is not a valid `GovernanceEnvelope`.
2. **Expiry Check**: Rejects any transaction if `expires_at` is in the past.
3. **Replay Protection**: Rejects the `nonce` if it has already been processed (tracked in the persistent KV store).
4. **BFT Check**: If `state_merkle_root` is present, it is compared against the local ledger. Mismatches cause immediate rejection.
5. **L1 Check**: Uses Protobuf reflection to find fields marked with `forbidden_patterns`. If a regex matches (e.g., `sudo`), the command is rejected.
6. **L2 Check**: Verifies the signature against its local trust store (e.g., `auditor_hmac_key` or trusted public keys) using the canonical signing payload.
7. **L3 Check**: For mutation requests, verifies the human signature or auto-approval status.
8. **Dispatch**: Decodes the inner payload and routes it to the appropriate service handler.

## 3. Result Phase (Operator -> Client)

1. **Execution**: The handler executes the requested action and captures results.
2. **Envelope Building**: `g8eo` wraps the result payload (e.g., `CommandResult`) in a `GovernanceEnvelope`.
3. **Publication**: The result envelope is published to the results channel for the subscribed client(s).

---

# Governance Layers

### L1: Technical Bedrock (Hard Gates)
Enforced via Protobuf reflection in `@/home/bob/g8e/components/g8eo/services/pubsub/protocol_helpers.go`. 
- **Mechanism**: Custom field option `forbidden_patterns`.
- **Scope**: Applied to fields like `CommandRequested.command`.
- **Default Patterns**: `sudo`, `su`, `rm -rf /`.

### L2: Consensus (Consensus)
Enforced via signatures in `@/home/bob/g8e/components/g8eo/services/pubsub/l2_verifier.go`.
- **Signer**: Any trusted consensus agent or validator (e.g., `g8ee`).
- **Verifier**: `g8eo` (Operator).
- **Mechanism**: ED25519 asymmetric signatures (Legacy HMAC fallback).
- **Attribution**: Uses `key_id` in `GovernanceMetadata` for O(1) public key lookup.
- **Material**: Pipe-delimited string: `ID | Timestamp | EventType | OperatorID | SessionID | StateRoot | Expiry | Nonce | Payload`.

### L3: Authorization (Approval)
Human-in-the-loop authorization via Passkeys or BYO approval systems.
- **Mechanism**: Hardware-bound signatures or verifiable approval proofs (`human_signature`).
- **Auto-Approval**: Benign commands (e.g., `uptime`) can be marked `auto_approved` by the Consensus layer, but L3 NEVER bypasses L1 or L2.

---

# Data Conversions

When `g8ee` decodes a result from `g8eo`, it applies shims for compatibility with Python models in `@/home/bob/g8e/components/g8ee/app/utils/envelope_builder.py`:

| Protobuf Field | Python Model Field | Logic |
|----------------|--------------------|-------|
| `CommandResult.output` | `stdout` | Consistent with `ExecutionResultsPayload`. |
| `CommandResult.exit_code` | `return_code` | Matches `subprocess.CompletedProcess`. |
| `ExecutionStatus` (Enum) | `ExecutionStatus` (StrEnum) | `2` -> `"completed"`, `3` -> `"failed"`. |

---

# Implementation Map

| Responsibility | Authoritative File |
|----------------|--------------------|
| **Schemas** | `@/home/bob/g8e/shared/proto/` |
| **Event Registry** | `@/home/bob/g8e/shared/constants/events.json` |
| **Request Builder (PY)** | `@/home/bob/g8e/components/g8ee/app/utils/envelope_builder.py` |
| **Inbound Dispatch (GO)** | `@/home/bob/g8e/components/g8eo/services/pubsub/pubsub_commands.go` |
| **L1 Enforcement (GO)** | `@/home/bob/g8e/components/g8eo/services/pubsub/protocol_helpers.go` |
| **L2 Verification (GO)** | `@/home/bob/g8e/components/g8eo/services/pubsub/l2_verifier.go` |
| **Result Publisher (GO)** | `@/home/bob/g8e/components/g8eo/services/pubsub/pubsub_results.go` |

---

# Related Documentation

- [Events](events.md) documents canonical event naming and lifecycle semantics.
- [Pub/Sub Channels](pubsub.md) documents channel-level routing and subscription behavior.
- [Governance & Mechanism Design](governance.md) documents the L1/L2/L3 safety model.
- [Security](security.md) documents platform security mechanisms around execution and auditability.
