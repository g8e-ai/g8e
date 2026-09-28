---
doc_id: events
title: Event and Action Protocol
audience: maintainers and coding agents
status: current
last_updated: 2026-09-28
version: v2.2.3
owners:
  - protocol/constants/events.json
  - internal/services/gateway/
  - internal/tools/constgen/
related:
  - docs/architecture/gateway.md
  - docs/architecture/operator.md
when_to_read: Designing, integrating, or auditing event flow across gateway, operator, ensemble, and dashboard; understanding governance envelopes and audit trails.
do_not_use_for:
  - Gateway API endpoints (docs/architecture/gateway.md)
  - Operator command flow (docs/architecture/operator.md)
  - Storage architecture (docs/architecture/storage.md)
---

# Event and Action Protocol

Defines the vocabulary and ownership boundary for protocol events. [protocol/constants/events.json](protocol/constants/events.json) is the authoritative registry; generated Go, Python, and dashboard views derive from it and do not define independent event names.

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
- **Request event**: an event whose `kind` is `request` and whose wire value ends in `.requested`. It initiates a governed transaction.
- **Outcome event**: an event whose `kind` is `outcome`. It reports success, failure, or progress on a prior request.
- **Fact event**: an event whose `kind` is `fact`. It records something that happened independently of a request; may be persisted in an audit log.
- **Stream event**: an event whose `kind` is `stream`. It carries ephemeral UI fragments (deltas, chunks, thinking state) and is never persisted.
- **Governance envelope**: a protobuf message carrying a request event, identity bindings, and authorization metadata. The Gateway validates the envelope, derives `action_type` from the registry, and enforces policy before forwarding to the operator.
- **Action type**: the governance class of a request. The registry maps request events to `action_type` values; the Gateway uses this to select policy enforcement and audit logging.
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
  “producers”: [“gateway|operator|ensemble|dashboard|cli|mcp”],
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

Wire values use `g8e.v1.<domain>.<entity>[.<qualifier>...].<terminal>`. The five domains are:

| Domain | Purpose |
| --- | --- |
| `operator` | Operator lifecycle, commands, heartbeats, and status. |
| `app` | Application lifecycle and actions. |
| `ai` | AI model invocation, consensus, voting, and agent coordination. |
| `platform` | Platform infrastructure: auth, notifications, governance. |
| `public` | Public-facing surface events (stable, versionable). |

## Event kinds

| Kind | Persistence | Transports | Purpose |
| --- | --- | --- | --- |
| `request` | ephemeral | governed | Initiates a governed transaction; terminal must end in `.requested`. |
| `outcome` | gateway.sse_store, operator.audit_log, or ephemeral | sse, pubsub | Reports success, failure, or progress on a request. |
| `fact` | operator.audit_log, gateway.audit_log, or ephemeral | sse, pubsub | Records something that happened independently; not tied to a request. |
| `stream` | ephemeral | sse only | Ephemeral UI fragment (delta, chunk, thinking state). |

## Governance envelopes

Request events that declare a `governance` block become governed transactions. The Gateway:

1. Receives the protobuf GovernanceEnvelope over mTLS with identity bindings (operator_id, operator_session_id, cli_session_id, acting_app_id, source_component).
2. Looks up the `event_type` in the registry and derives `action_type`.
3. Verifies the envelope's identity bindings match the mTLS certificate's SPIFFE ID and rejects unbound mutations.
4. Enforces policies (ACL, rate limit, quotas) derived from `action_type`.
5. Forwards the envelope to the operator; the operator includes both `event_type` and `action_type` in signed receipts.

The `governance` block specifies:

- **action_type**: a governance class name; used to select policy enforcement and audit logging.
- **payload**: the protobuf message type that carries the request body (e.g., `OperatorCommandPayload`).

Version 2 canonicalization (hash, receipt signature) includes both `event_type` and `action_type` so signed evidence identifies the semantic request, not only its broad governance class. Verifiers retain version 1 only for historical records.

## Transport ownership

- **SSE** (Server-Sent Events): Gateway-to-client delivery. The Gateway validates the registry entry, persists non-ephemeral events in its SSE store, and pushes them to subscribed clients.
- **Governed**: mTLS HTTP POST to Gateway. Wraps a request event in a GovernanceEnvelope for identity binding, policy enforcement, and audit logging. Only used by request events with governance.
- **Pub/sub**: Gateway-to-operator and operator-to-Gateway queues for commands, results, receipts, heartbeats, and audit channels. Ensemble does not publish or subscribe to operator pub/sub channels.

## Storage ownership

- **Gateway**: owns operator documents, heartbeat snapshots, operator audit chain, SSE event store, and docstore.
- **Operator**: owns governed commitments (staged transaction state) and receipt stages, appended as it executes and signed as evidence.
- **Audit flow**: application submits audit records to the Gateway, which verifies operator session binding, forwards to the operator, and receives operator acknowledgement containing sequence and hash.
- **g8ee**: submits application requests and audit records to the Gateway but owns neither operator authority nor operator audit persistence.
- **SSE**: is a client delivery projection, not an audit record; non-persistent copies may be pruned.

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
