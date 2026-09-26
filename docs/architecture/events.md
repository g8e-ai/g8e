# Event and Action Protocol

This document defines the vocabulary and ownership boundary for protocol
events. `protocol/constants/events.json` is the source of truth; generated
Go, dashboard, and runtime Python views must not introduce independent event
names.

## Glossary

- **Event**: a named, typed message on SSE, pub/sub, or a governed envelope.
  It is identified by `event_type`.
- **Request event**: an event whose terminal is `requested`.
- **Outcome event**: a terminal or progress response to a request.
- **Fact event**: a record of something that happened independently of one
  request.
- **Stream event**: an ephemeral UI fragment that is not persisted.
- **Governed action**: the governance class derived from a request event.
  `action_type` selects payload decoding and policy; callers do not choose it
  independently.
- **Receipt**: signed evidence of one governed transaction stage.
- **Audit record**: a fact event persisted in a host audit log.

The word “action” refers only to the governed action class. UI thinking
updates use `phase`, not `action_type`.

## Event registry

Each registry entry has a wire value and, once enriched, metadata describing
its kind, transports, producers, persistence, governance action, outcomes, and
payload. The metadata is validated by
`protocol/models/event_registry.schema.json`.

Wire values use:

```
g8e.v1.<domain>.<entity>[.<qualifier>...].<terminal>
```

The domains are `operator`, `app`, `ai`, `platform`, and `public`. New
governed requests must use a registered request event and a registry-provided
governance action.

The last wire segment is the terminal. `internal/tools/constgen` enforces the
closed terminal list on every registry entry. Compound forms that remain
legal without an allowlist:

- `g8e.v1.<domain>.<terminal>` for entity-less facts (`operator.bound`,
  `operator.unbound`)
- `*.status.updated.<state>` (operator and investigation status facts)
- stream fragments containing `.stream.`, `.chunk.`, `.delta.`, `.keepalive.`,
  or `.thinking.`

The grammar allowlist is empty. Historical names that used `event`,
`execution`, `result`, `info`, or `notification` as a terminal were renamed:

| Previous | Current |
| --- | --- |
| `g8e.v1.ai.llm.chat.filter.event` | `g8e.v1.ai.llm.chat.filter.updated` |
| `g8e.v1.operator.command.execution` | `g8e.v1.operator.command.execution.started` |
| `g8e.v1.operator.command.result` | `g8e.v1.operator.command.result.completed` |
| `g8e.v1.platform.auth.info` | `g8e.v1.platform.auth.info.updated` |
| `g8e.v1.platform.notification` | `g8e.v1.platform.notification.sent` |

Callers send `event_type` only. Gateway derives `action_type` from the
registry and rejects unknown or non-request events at ingress.

## Transport ownership

- **SSE** is Gateway-to-client. The Gateway validates the registry entry and
  persists non-ephemeral events in its SSE store.
- **Internal HTTP over mTLS** is the g8ee-to-Gateway boundary for dispatch,
  governance envelopes, SSE push, audit ingest, and operator reads.
- **Pub/sub** is Gateway-to-operator transport for command, result, receipt,
  heartbeat, and audit channels. Ensemble does not publish or subscribe to
  operator pub/sub channels.

## Storage ownership

- Gateway owns operator documents and heartbeat snapshots.
- The executing operator owns governed commitments and receipt stages.
- Gateway mirrors operator receipts and owns its own audit chain.
- g8ee owns neither operator authority nor operator audit persistence. It
  submits application requests and audit records to the Gateway.
- SSE is a client delivery projection, not an audit record.

## Hash and receipt versions

New ingress uses protocol version `2`. Version 2 transaction
canonicalization includes both `event_type` and the registry-derived
`action_type`. Receipt canonicalization likewise includes both fields.
Verifiers retain version 1 only for historical records.

The `ActionReceipt` message carries top-level `event_type` and `action_type`
so signed evidence identifies the semantic request, not only its broad
governance class.

## Audit chain

Host audit events are append-only chain entries. Each entry records a sequence,
the previous hash, a plaintext content digest, and its own hash. Receipt
stages are appended as audit events while the receipts table remains a
latest-stage query projection. Pruning requires a checkpoint entry containing
the pruned range and terminal hash.

The audit ingest endpoint is Gateway-owned:
`POST /api/v1/audit/records`. It authenticates the application, verifies the
operator session binding, forwards the fact to the operator, and returns the
operator acknowledgement containing `{seq, hash}`.
