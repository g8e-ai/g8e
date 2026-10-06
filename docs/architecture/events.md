---
doc_id: events
title: Event and Action Protocol
audience: maintainers and coding agents
status: current
last_updated: 2026-10-06
version: v2.3.1
owners:
  - protocol/constants/events.json
  - internal/services/gateway/
  - internal/tools/constgen/
related:
  - docs/architecture/gateway.md
  - docs/architecture/operator.md
when_to_read: Designing, integrating, or auditing event flow across gateway, operator, ensemble, and console; understanding governance envelopes and audit trails.
do_not_use_for:
  - Gateway API endpoints (docs/architecture/gateway.md)
  - Operator command flow (docs/architecture/operator.md)
  - Storage architecture (docs/architecture/storage.md)
---

# Event and Action Protocol

Defines the vocabulary and ownership boundary for protocol events. [protocol/constants/events.json](protocol/constants/events.json) is the authoritative registry; generated Go, Python, and console views derive from it and do not define independent event names.

## Purpose

The event registry establishes a closed vocabulary for named, typed messages flowing over SSE, pub/sub, and governed transports. Each event carries metadata describing its kind (request, outcome, fact, stream), who produces it, where it persists, and for governed requests, the policy class and payload schema that govern it.

## Quick index

- [Glossary](#glossary)
- [Event registry](#event-registry)
- [Event kinds](#event-kinds)
- [Governance envelopes](#governance-envelopes)
- [Transport ownership](#transport-ownership)
- [Storage ownership](#storage-ownership)
- [Audit chain](#audit-chain)
- [Wire value grammar](#wire-value-grammar)
- [Links out](#links-out)

## Glossary

- **Event**: a named, typed message identified by `event_type`. It flows over SSE, pub/sub, or governed transport.
- **Event kind**: one of request, outcome, fact, or stream. Defines the event's role and persistence.
- **Request event**: an event whose `kind` is `request` and whose wire value ends in `.requested`. It initiates a request, which may or may not be wrapped in a `GovernanceEnvelope` depending on the registry entry.
- **Outcome event**: an event whose `kind` is `outcome`. It reports success, failure, or progress on a prior request.
- **Fact event**: an event whose `kind` is `fact`. It records something that happened independently of a request; may be persisted in an audit log.
- **Stream event**: an event whose `kind` is `stream`. It carries ephemeral UI fragments (deltas, chunks, thinking state) and is never persisted.
- **Governance envelope**: a protobuf message carrying a request event, identity bindings, and authorization metadata. The Gateway validates the envelope, derives `action_type` from the registry, and enforces policy before forwarding to the operator.
- **Action type**: the governance class of a governed request. The registry maps those request events to `action_type` values; the Gateway uses this to select policy enforcement and audit logging.
- **Receipt**: cryptographic evidence of one stage of a governed transaction, signed by the operator and persisted in the audit chain.
- **Audit record**: a fact event appended to an operator or Gateway audit log with a sequence number, content digest, and hash chain.

## Event registry

Each registry entry describes one event with metadata: kind, wire value, producers, persistence location, allowed transports, and (for requests) governance action and payload schema.

Metadata is validated against [protocol/models/event_registry.schema.json](protocol/models/event_registry.schema.json) during code generation; the generator in [internal/tools/constgen/](internal/tools/constgen/) enforces the closed terminal list and governance constraints.

### Registry entry structure

```json
“EventName”: {
  “_go_const”: “EventName”,
  “value”: “g8e.v1.<domain>.<entity>[.<qualifier>...].<terminal>”,
  “kind”: “request|outcome|fact|stream”,
  “transport”: [“governed|pubsub|sse”],
  “producers”: [“gateway|operator|ensemble|cli|mcp”],
  “persistence”: “operator.audit_log|gateway.audit_log|gateway.sse_store|gateway.operator_docs|gateway.docstore|ephemeral”,
  “governance”: {
    “action_type”: “<string>”,
    “payload”: “<protobuf message name>”
  },
  “outcomes”: [“OutcomeEventName”],
  “reserved”: false
}
```

### Domains

Wire values use `g8e.v1.<domain>.<entity>[.<qualifier>...].<terminal>`. The live registry is the source of truth for which domains are in use today; it is not a closed enum hard-coded in the schema. In the current registry, the active domains are:

| Domain | Purpose |
| --- | --- |
| `operator` | Operator lifecycle, commands, heartbeats, and status. |
| `ai` | AI model invocation, consensus, voting, and agent coordination. |
| `app` | Application lifecycle and actions. |
| `platform` | Platform infrastructure: auth, notifications, governance. |
| `public` | Public-facing surface events that are stable and versioned. |
| `inference` | Inference/session telemetry currently emitted by the ensemble stack. |

The registry itself is maintained in [protocol/constants/events.json](../../protocol/constants/events.json); the schema only enforces the wire-value grammar and field structure, not a fixed domain list.

## Event kinds

| Kind | Persistence | Transports | Purpose |
| --- | --- | --- | --- |
| `request` | ephemeral | governed, sse, or unset | Initiates a request; registry entries with `governance` metadata are processed through the `GovernanceEnvelope` path. |
| `outcome` | gateway.sse_store, operator.audit_log, or ephemeral | sse, pubsub, or unset | Reports success, failure, or progress on a prior request. |
| `fact` | operator.audit_log, gateway.audit_log, or ephemeral | sse, pubsub, or unset | Records something that happened independently; not tied to a request. |
| `stream` | ephemeral | sse only | Ephemeral UI fragment (delta, chunk, thinking state). |

## Governance envelopes

Not every request event is a governed transaction. A request event becomes governed only when its registry entry carries a `governance` block. The current registry distinguishes between:

- UI/session requests that are delivered over SSE or local request channels without a governance wrapper.
- Mutating app and document requests that use the `governed` transport and carry a `GovernanceEnvelope`.

The canonical envelope definition lives in [protocol/proto/g8e/common/v1/common.proto](../../protocol/proto/g8e/common/v1/common.proto). `GovernanceEnvelope` binds identity, intent, payload, timing, and optional L1/L2/L3 governance metadata into a single transaction container. The Gateway validates the envelope against the current event registry, derives the action classification from the registry entry, and enforces the posture-specific checks before forwarding the request to the selected operator session.

The `governance` block in the registry specifies:

- **action_type**: the policy/action classification for the request.
- **payload**: the protobuf payload type that the envelope carries for that request.

The canonical message shape is therefore registry-driven: the event registry identifies the semantic request, while the canonical protobuf message defines the envelope and proofs. The runtime derives action metadata from both sources rather than treating the event name as the only source of truth.

## Transport ownership

The current registry defines three transport classes: `governed`, `pubsub`, and `sse`.

- **SSE**: client-delivery channel for live UI and operator state updates. Many outcome and stream events are emitted here. This transport is not equivalent to a persisted audit log.
- **Governed**: request path used by registry entries whose `governance` metadata is populated. The request is wrapped in a canonical `GovernanceEnvelope` and processed through the gateway policy path before dispatch.
- **Pub/sub**: canonical routing/notification channel for runtime messaging where the registry marks it as such; it is not the same as a persisted storage layer.

The registry is the source of truth for what a given event may traverse. In other words, a given event name is not assumed to be valid for all transports; the metadata in [protocol/constants/events.json](../../protocol/constants/events.json) declares the valid path.

## Storage ownership

The registry records persistence ownership in the `persistence` field. The current allowed values are:

- `operator.audit_log`
- `gateway.audit_log`
- `gateway.sse_store`
- `gateway.operator_docs`
- `gateway.docstore`
- `ephemeral`

This is the authoritative storage contract enforced by the registry and generation tooling. In practical terms:

- **Gateway**: owns the public-facing event projection and document surfaces that the registry marks as `gateway.*`.
- **Operator**: owns the local, append-only evidence that the registry marks as `operator.audit_log`.
- **SSE**: is a delivery projection for client updates, not the canonical evidence ledger.
- **Ephemeral**: used for UI/request-scoped events that are not intended to become audit-state records.

These ownership labels describe the current registry contract; they are not a second, out-of-band store model. Runtime code remains expected to follow the event registry and the runtime-specific storage services rather than invent a parallel naming scheme.

## Audit chain

Host audit events are append-only chain entries. Each entry records:

- Sequence number (monotonic per host).
- Previous hash (linked to the prior entry).
- Plaintext content digest of the event.
- Computed hash of the entry.

Receipt stages are appended as audit events; the receipts table remains a latest-stage query projection. Pruning requires a checkpoint entry containing the pruned range and terminal hash.

## Wire value grammar

Wire values follow the pattern `g8e.v1.<domain>.<entity>[.<qualifier>...].<terminal>` where `<terminal>` is the last segment.

### Allowed terminals

The closed set of allowed terminals is enforced by [internal/tools/constgen/](internal/tools/constgen/) and includes: `requested`, `started`, `received`, `completed`, `failed`, `cancelled`, `timeout`, `recorded`, `created`, `updated`, `deleted`, `granted`, `rejected`, `denied`, `revoked`, `expired`, `acknowledged`, `sent`, `missed`, `bound`, `unbound`, `opened`, `closed`, `established`, `checkpointed`, `exported`, `published`, `rotated`, `available`, `invoked`, `reached`, `detected`, `resolved`, `appended`, `truncated`, `retry`, `heartbeat`, `active`, `ai`, `answered`, `append`, `assigned`, `authenticated`, `blocked`, `changed`, `cleared`, `complete`, `configured`, `confirmed`, `disabled`, `end`, `error`, `escalated`, `feedback`, `lettered`, `loaded`, `logged`, `occurred`, `offline`, `open`, `preparing`, `questions`, `queued`, `registered`, `replayed`, `reported`, `running`, `selected`, `skipped`, `stale`, `stopped`, `submitted`, `succeeded`, `switched`, `system`, `terminated`, `tier1`, `tier2`, `tier3`, `unauthenticated`, `unavailable`, `update`, `user`, `verified`.

### Exempt forms

The following compound forms are exempt from terminal validation:

- `g8e.v1.<domain>.<terminal>` for entity-less facts (e.g., `operator.bound`, `operator.unbound`).
- `*.status.updated.<state>` for operator and investigation status facts.
- Stream fragments containing `.stream.`, `.chunk.`, `.delta.`, `.keepalive.`, or `.thinking.`.

### Historical naming

Prior protocol versions used `event`, `execution`, `result`, `info`, and `notification` as terminals, which are no longer allowed. Migration examples:

| Historical | Current |
| --- | --- |
| `g8e.v1.ai.llm.chat.filter.event` | `g8e.v1.ai.llm.chat.filter.updated` |
| `g8e.v1.operator.command.execution` | `g8e.v1.operator.command.execution.started` |
| `g8e.v1.operator.command.result` | `g8e.v1.operator.command.result.completed` |
| `g8e.v1.platform.auth.info` | `g8e.v1.platform.auth.info.updated` |
| `g8e.v1.platform.notification` | `g8e.v1.platform.notification.sent` |

## Links out

- [Gateway Architecture](gateway.md): event routing, envelope validation, SSE delivery.
- [Operator Architecture](operator.md): command execution, receipt signing, audit log persistence.
- [Storage Architecture](storage.md): docstore, audit log, and versioning.
