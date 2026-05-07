---
title: Protocol
parent: Architecture
---

# g8e Protocol

Last Updated: 2026-05-07
Version: v0.2.0

# Protocol Invariants

## 1. No Backwards Compatibility
The g8e platform follows a strict **NO BACKWARDS COMPATIBILITY** policy. Protocol updates (e.g., migrating from JSON to Protobuf) are breaking changes. Components MUST reject legacy data formats with clear error messages. Users are expected to recreate resources if a protocol change makes existing data unreadable.

## 2. Protobuf-First
All cross-component operator traffic (commands, results, status updates) MUST use serialized `UniversalEnvelope` Protobuf messages. JSON fallbacks are forbidden.

---

## Core Model

g8e treats cross-component operator work as typed protocol messages, not ad hoc JSON documents. The canonical schema files live in `shared/proto/`:

| File | Purpose |
|------|---------|
| `shared/proto/common.proto` | Defines `UniversalEnvelope`, component identity, and governance metadata. |
| `shared/proto/operator.proto` | Defines operator request and result payloads for commands, file operations, filesystem operations, history, logs, ports, audit, shutdown, and heartbeats. |
| `shared/proto/pubsub.proto` | Defines the low-level pub/sub carrier messages whose `data` fields carry opaque bytes. |

The canonical event registry remains `shared/constants/events.json`. Event strings identify what happened or what is requested; Protobuf messages define the binary payload shape for protocol paths that carry typed operator traffic.

---

## UniversalEnvelope

`UniversalEnvelope` is the root container for Protobuf-first cross-component operator messages.

| Field | Role |
|-------|------|
| `id` | Unique message identifier. In command flows this commonly matches the execution correlation key. |
| `timestamp` | Message creation time. |
| `source_component` | Publishing component enum: `COMPONENT_G8EE`, `COMPONENT_G8EO`, or `COMPONENT_G8ED`. |
| `event_type` | Canonical event string from `shared/constants/events.json`. |
| `operator_id` | Operator identity associated with the message. |
| `operator_session_id` | Concrete operator session that receives or produced the message. |
| `case_id` | Case correlation key for audit and UI grouping. |
| `investigation_id` | Investigation correlation key for chat and execution history. |
| `task_id` | Task correlation key for threaded command/result flows. |
| `web_session_id` | Browser session correlation key when a human-interactive session is known. |
| `system_fingerprint` | Hardware/system fingerprint reported by `g8eo`. |
| `state_merkle_root` | Fleet state root used by BFT stale-state checks when populated. |
| `governance` | L1/L2/L3 metadata attached to the transaction. |
| `payload` | Serialized bytes of the typed message selected by `event_type`. |

The envelope does not replace event names. It binds a canonical event name to typed payload bytes and the governance evidence required to decide whether the transaction can proceed.

---

## Wire Flow

The operator command/result path uses the same two-level shape in both directions:

```text
g8ee or g8eo
  -> typed operator payload from operator.proto
  -> serialized payload bytes
  -> UniversalEnvelope.payload
  -> serialized UniversalEnvelope bytes
  -> g8es pub/sub data
```

For inbound operator requests, `g8ee` builds a typed payload from a `G8eMessage` payload model, serializes it, wraps it in `UniversalEnvelope`, signs the L2 metadata, and publishes the envelope bytes through g8es pub/sub. `g8eo` rejects command bytes that cannot be unmarshaled as `UniversalEnvelope`.

For outbound operator results, `g8eo` builds typed result payloads, wraps them in `UniversalEnvelope`, and publishes the envelope bytes through g8es pub/sub. Runtime result paths for command completion, cancellation, file operations, filesystem operations, logs/history, status updates, and heartbeats use the Protobuf envelope path.

Channel names, subscription lifecycles, and broker behavior are documented in the pub/sub architecture and g8es component docs. This page owns the protocol message contract, not the channel taxonomy.

---

## Payload Selection

`event_type` determines how the receiver interprets `payload`.

Representative request payloads include:

| Event | Payload message |
|-------|-----------------|
| `g8e.v1.operator.command.requested` | `CommandRequested` |
| `g8e.v1.operator.command.cancel.requested` | `CommandCancelRequested` |
| `g8e.v1.operator.file.edit.requested` | `FileEditRequested` |
| `g8e.v1.operator.filesystem.list.requested` | `FsListRequested` |
| `g8e.v1.operator.filesystem.read.requested` | `FsReadRequested` |
| `g8e.v1.operator.filesystem.grep.requested` | `FsGrepRequested` |
| `g8e.v1.operator.network.port.check.requested` | `CheckPortRequested` |
| `g8e.v1.operator.logs.fetch.requested` | `FetchLogsRequested` |
| `g8e.v1.operator.history.fetch.requested` | `FetchHistoryRequested` |
| `g8e.v1.operator.file.history.fetch.requested` | `FetchFileHistoryRequested` |
| `g8e.v1.operator.file.diff.fetch.requested` | `FetchFileDiffRequested` |
| `g8e.v1.operator.file.restore.requested` | `RestoreFileRequested` |
| `g8e.v1.operator.heartbeat.requested` | `HeartbeatRequested` |
| `g8e.v1.operator.shutdown.requested` | `ShutdownRequested` |
| `g8e.v1.operator.audit.user.recorded` | `AuditMsgRequested` |
| `g8e.v1.operator.audit.ai.recorded` | `AuditMsgRequested` |
| `g8e.v1.operator.audit.direct.command.recorded` | `DirectCommandAuditRequested` |
| `g8e.v1.operator.audit.direct.command.result.recorded` | `DirectCommandResultAuditRequested` |

Representative result payloads include `CommandResult`, `ExecutionStatusUpdate`, `FileEditResult`, `FsListResult`, `FsReadResult`, `FsGrepResult`, `PortCheckResult`, `FetchLogsResult`, `FetchHistoryResult`, `FetchFileHistoryResult`, `FetchFileDiffResult`, `RestoreFileResult`, and `HeartbeatResult`.

`g8eo` fails closed when the envelope itself cannot be decoded or is missing `id`. For recognized first-class request events, `g8eo` decodes `UniversalEnvelope.payload` into the expected `operator.proto` message before dispatch, enforces reflected L1 gates when decoding succeeds, and each handler rejects payloads that cannot be decoded as its expected message. Unknown event types do not dispatch because no handler is registered.

---

## Governance Metadata

Governance is part of the protocol envelope. It is not optional side-channel state.

### L1: Technical Bedrock

L1 enforcement uses Protobuf reflection over custom field options defined in `common.proto`. `operator.proto` marks safety-sensitive string fields with `g8e.common.v1.forbidden_patterns`.

`CommandRequested.command` currently carries forbidden patterns for `sudo`, `su`, and `rm -rf /`. `g8eo` unmarshals recognized typed payloads and rejects the command before dispatch when a reflected field violates its configured patterns.

### L2: Consensus

`g8ee` signs outbound command envelopes with an HMAC-SHA256 Tribunal signature over this canonical byte sequence:

```text
event_type || "\n" || payload_bytes
```

The hex digest is stored in `governance.l2.tribunal_signature`. `g8eo` computes the same digest with the configured auditor HMAC key and rejects commands with missing or invalid signatures when L2 verification is configured.

### L3: Authorization

`common.proto` defines `L3Metadata` with `human_signature`, `public_key`, and `auto_approved`. The current Protobuf schema can carry L3 authorization evidence, but command acceptance is not currently gated by a runtime L3 verifier in `g8eo`.

Auto-approval never bypasses L1 or L2. It only represents L3 authorization state when the surrounding approval flow populates and verifies it.

---

## State Root Verification

`UniversalEnvelope.state_merkle_root` binds a generated transaction to the fleet state observed at generation time. When an inbound command carries a non-empty state root and `g8eo` has a ledger service available, `g8eo` compares the envelope root to its current ledger root.

A mismatch is a hard reject because the command was generated against stale state. If the ledger service is absent or cannot provide a root, `g8eo` logs that BFT root verification cannot be completed for that message.

---

## Strict Protocol Compliance

The v0.2.0 operator protocol is Protobuf-first on command and result pub/sub paths.

Current compliance rules:

- `g8ee` command publication builds serialized `UniversalEnvelope` bytes; it does not fall back to JSON when envelope construction fails.
- `g8eo` inbound command handling rejects payloads that cannot be parsed as `UniversalEnvelope`.
- Typed operator handlers decode `UniversalEnvelope.payload` as the expected `operator.proto` message.
- Runtime result publishers emit serialized `UniversalEnvelope` bytes containing typed result payloads.

Protocol changes must update `shared/proto/`, generated language artifacts, event constants when event names change, and every doc that owns the affected behavior.

---

## Field Mappings and Enum Conversions

When `g8ee` decodes a `UniversalEnvelope` from `g8eo`, it performs several transformations to ensure compatibility with internal Pydantic models and Python-idiomatic types.

### 1. Enum Conversions
Protobuf numeric enums are converted to Python string-based `StrEnum` values:

| Protobuf Enum | Python Enum | Mapping Logic |
|---------------|-------------|---------------|
| `g8e.operator.v1.ExecutionStatus` | `app.constants.status.ExecutionStatus` | `EXECUTION_STATUS_COMPLETED` (2) -> `"completed"` |
| `UniversalEnvelope.event_type` | `app.constants.events.EventType` | Raw string -> `EventType` member |

### 2. Field Boundary Shims
To maintain compatibility with internal Pydantic models while using standardized Protobuf field names, the following mappings are applied during decoding in `app.utils.envelope_builder.py`:

| Protobuf Field | Python Model Field | Context |
|----------------|--------------------|---------|
| `CommandResult.output` | `stdout` | Consistent with `ExecutionResultsPayload` |
| `CommandResult.exit_code` | `return_code` | Consistent with Python `subprocess.CompletedProcess` |

---

## Implementation Map

| Responsibility | Authoritative implementation |
|----------------|------------------------------|
| Envelope schema | `shared/proto/common.proto` |
| Operator payload schema | `shared/proto/operator.proto` |
| Pub/sub carrier schema | `shared/proto/pubsub.proto` |
| Event registry | `shared/constants/events.json` |
| Python envelope builder and L2 signer | `components/g8ee/app/utils/envelope_builder.py` |
| Python command publication | `components/g8ee/app/clients/pubsub_client.py` |
| Go inbound envelope parsing and governance dispatch | `components/g8eo/services/pubsub/pubsub_commands.go` |
| Go L1 reflection enforcement | `components/g8eo/services/pubsub/protocol_helpers.go` |
| Go L2 signature verification | `components/g8eo/services/pubsub/l2_verifier.go` |
| Go result envelope publication | `components/g8eo/services/pubsub/pubsub_results.go` |
| Generated Protobuf artifacts | `components/g8ee/app/proto/` and `components/g8eo/shared/proto/` |

---

## Related Documentation

- [Events](events.md) documents canonical event naming and lifecycle semantics.
- [Pub/Sub Channels](pubsub.md) documents channel-level routing and subscription behavior.
- [Governance & Mechanism Design](governance.md) documents the L1/L2/L3 safety model.
- [Security](security.md) documents platform security mechanisms around execution and auditability.
