---
title: Protocol
parent: Architecture
---

# g8e Protocol

Last Updated: 2026-05-11
Version: v0.2.3

The g8e Protocol is a **Protobuf-first** communication layer designed for secure, auditable, and Byzantine Fault Tolerant (BFT) interaction between the three platform components: **Dashboard (g8ed)**, **Engine (g8ee)**, and **Operator (g8eo)**.

# Core Invariants

1. **Protobuf-Only Wire Format**: All cross-component operator traffic (commands, results, status updates) MUST use serialized `UniversalEnvelope` messages. JSON fallbacks are rejected.
2. **Identity Persistence**: Every message must carry an `id`, `operator_id`, and `operator_session_id`.
3. **Immutable Governance**: Governance metadata (L1/L2/L3) is baked into the envelope and verified before any action is taken.
4. **BFT State Binding**: Commands are bound to a specific fleet state via `state_merkle_root`.

---

# Message Models

The canonical schema files live in `@/home/bob/g8e/shared/proto/`:

| File | Purpose |
|------|---------|
| `common.proto` | Defines `UniversalEnvelope`, component identity, and governance metadata. |
| `operator.proto` | Defines request/result payloads (commands, file ops, heartbeats, etc.). |
| `pubsub.proto` | Defines the low-level pub/sub carrier messages. |

## UniversalEnvelope

The `UniversalEnvelope` is the root container for all protocol traffic. It binds an event name to typed payload bytes and provides the necessary context for governance.

| Field | Role |
|-------|------|
| `id` | Unique UUID v4 for the message. |
| `timestamp` | UTC creation time. |
| `source_component` | `COMPONENT_G8EE`, `COMPONENT_G8EO`, or `COMPONENT_G8ED`. |
| `event_type` | Canonical event string from `shared/constants/events.json`. |
| `operator_id` | Target or source operator identity. |
| `state_merkle_root` | Fleet state root for BFT verification. |
| `governance` | L1/L2/L3 metadata for security enforcement. |
| `payload` | Serialized bytes of the typed message (e.g., `CommandRequested`). |

---

# Protocol Lifecycle

## 1. Request Phase (Engine -> Operator)

1. **Generation**: `g8ee` generates a `G8eMessage` containing a typed payload (e.g., `CommandRequestPayload`).
2. **Envelope Building**: `@/home/bob/g8e/components/g8ee/app/utils/envelope_builder.py` converts the model to a `UniversalEnvelope`.
3. **L2 Signing**: The envelope is signed using HMAC-SHA256 over `event_type || "\n" || payload_bytes` using the `auditor_hmac_key`.
4. **Publication**: The serialized envelope is published to the operator's command channel.

## 2. Verification Phase (Operator)

Upon receiving an envelope, `g8eo` performs these checks in `@/home/bob/g8e/components/g8eo/services/pubsub/pubsub_commands.go`:

1. **Parsing**: Rejects any payload that is not a valid `UniversalEnvelope`.
2. **BFT Check**: If `state_merkle_root` is present, it is compared against the local ledger. Mismatches cause immediate rejection.
3. **L1 Check**: Uses Protobuf reflection to find fields marked with `forbidden_patterns`. If a regex matches (e.g., `sudo`), the command is rejected.
4. **L2 Check**: Verifies the Tribunal signature against its local `auditor_hmac_key`.
5. **Dispatch**: Decodes the inner payload and routes it to the appropriate service handler.

## 3. Result Phase (Operator -> Engine)

1. **Execution**: The handler executes the requested action and captures results.
2. **Envelope Building**: `g8eo` wraps the result payload (e.g., `CommandResult`) in a `UniversalEnvelope`.
3. **Publication**: The result envelope is published to the results channel for the Engine/Dashboard.

---

# Governance Layers

### L1: Technical Bedrock (Hard Gates)
Enforced via Protobuf reflection in `@/home/bob/g8e/components/g8eo/services/pubsub/protocol_helpers.go`. 
- **Mechanism**: Custom field option `forbidden_patterns`.
- **Scope**: Applied to fields like `CommandRequested.command`.
- **Default Patterns**: `sudo`, `su`, `rm -rf /`.

### L2: Consensus (Tribunal)
Enforced via HMAC-SHA256 signatures in `@/home/bob/g8e/components/g8eo/services/pubsub/l2_verifier.go`.
- **Signer**: `g8ee` (Engine).
- **Verifier**: `g8eo` (Operator).
- **Material**: `event_type + "\n" + payload_bytes`.

### L3: Authorization (Approval)
Human-in-the-loop authorization via Passkeys.
- **Mechanism**: Hardware-bound signatures (`human_signature`).
- **Auto-Approval**: Benign commands (e.g., `uptime`) can be marked `auto_approved` by the Tribunal, but L3 NEVER bypasses L1 or L2.

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
