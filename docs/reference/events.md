---
title: Events
parent: Reference
---

# g8e Event Type Reference

Reference guide for g8e platform event type definitions. These event strings are canonical identifiers used by the g8e protocol and by UI/API event surfaces.

---

## Event Name Format

```
g8e.v<version>.<domain>.<resource>[.<sub-resource>...].<action>
```

- **Protocol prefix** -- `g8e.v1` (current version)
- **Domain** -- top-level namespace: `app`, `operator`, `ai`, `platform`, `source`
- **Resource path** -- dot-separated hierarchy identifying the subject
- **Action** -- past-tense verb or state at the leaf position

---

## Source of Truth

`shared/constants/events.json` is the single canonical definition. Component-level bindings consume it:

| Component | File | Mechanism |
|-----------|------|-----------|
| g8ee (Python) | `components/g8ee/app/constants/events.py` | `EventType(str, Enum)` mirroring wire values |
| g8ed (Node.js server) | `components/g8ed/constants/events.js` | `EventType` frozen object, reads from `events.json` via `shared.js` |
| g8ed (browser client) | `components/g8ed/public/js/constants/events.js` | `EventType` frozen object with hardcoded wire values (subset) |
| g8eo (Go) | `components/g8eo/constants/events.go` | `Event` struct tree with hardcoded wire values (operator subset) |

### Event Definition Files

- **Canonical JSON**: [`shared/constants/events.json`](../../shared/constants/events.json) - Nested hierarchy with wire values
- **Python binding**: [`components/g8ee/app/constants/events.py`](../../components/g8ee/app/constants/events.py) - `EventType` enum
- **Node.js server binding**: [`components/g8ed/constants/events.js`](../../components/g8ed/constants/events.js) - `EventType` frozen object
- **Node.js client binding**: [`components/g8ed/public/js/constants/events.js`](../../components/g8ed/public/js/constants/events.js) - Browser `EventType`
- **Go binding**: [`components/g8eo/constants/events.go`](../../components/g8eo/constants/events.go) - `Event` struct (operator subset)

---

## Naming Rules

1. Wire values use lowercase dot-delimited segments: `g8e.v1.operator.command.started`
2. Constant names use `UPPER_SNAKE_CASE`: `OPERATOR_COMMAND_STARTED`
3. Every leaf is a **past-tense action** (`created`, `failed`, `received`) or a **state** (`active`, `open`)
4. New events must be added to `events.json` first, then propagated to component bindings

---

## Domains

The protocol defines five top-level domains. Total event count: **273**.

| Domain | Count | Description | See Definition |
|--------|-------|-------------|----------------|
| `app` | 35 | Application-layer entities: cases, tasks, investigations | [`events.json`](../../shared/constants/events.json#L8) |
| `operator` | 114 | Operator (g8eo) lifecycle, commands, file ops, network, audit, bootstrap | [`events.json`](../../shared/constants/events.json#L65) |
| `ai` | 83 | LLM chat, streaming, lifecycle, tool calls, tribunal, reputation | [`events.json`](../../shared/constants/events.json#L293) |
| `platform` | 36 | Auth, SSE transport, terminal UI, telemetry, sentinel | [`events.json`](../../shared/constants/events.json#L464) |
| `source` | 5 | Event origin tags for message attribution | [`events.json`](../../shared/constants/events.json#L554) |

---

## Component Coverage

Not every component needs every event. This table shows which domains each component binding covers.

| Domain | `events.json` | g8ee (Python) | g8ed (server JS) | g8ed (client JS) | g8eo (Go) |
|--------|:---:|:---:|:---:|:---:|:---:|
| `app` | 35 | 35 | 35 | 35 | -- |
| `operator` | 114 | 114 | 114 | 114 | 65 |
| `ai` | 83 | 83 | 83 | 83 | -- |
| `platform` | 36 | 36 | 36 | 36 | -- |
| `source` | 5 | 5 | 5 | 5 | -- |
| **Total** | **273** | **273** | **273** | **273** | **65** |

g8eo only binds the operator-domain events it produces or consumes. g8ed client JS mirrors all events with hardcoded values. g8ed server JS and g8ee mirror the full set.

---

## Wire Representation

Event names are identifiers, not the whole wire contract. Operator command/result traffic uses the g8e protocol: serialized Protobuf `UniversalEnvelope` bytes carry `event_type`, typed `operator.proto` payload bytes, operator/session context, state roots, and L1/L2/L3 governance metadata. See [g8e Protocol](../architecture/protocol.md).

Browser-facing SSE and internal UI event surfaces use JSON objects with a `type` field carrying the same canonical event value:

```json
{
  "type": "g8e.v1.operator.command.completed",
  "data": {
    "execution_id": "exec-abc123",
    "output": "command output",
    "exit_code": 0,
    "success": true,
    "investigation_id": "inv-xyz",
    "case_id": "case-456",
    "web_session_id": "ws-789"
  }
}
```

SSE events use this structure, serialized via `G8eBaseModel.forWire()` at the g8ed SSE service boundary.

---

## Adding New Events

1. Add the wire value to [`shared/constants/events.json`](../../shared/constants/events.json) following the nested hierarchy
2. Add the `EventType` enum member to [`components/g8ee/app/constants/events.py`](../../components/g8ee/app/constants/events.py)
3. Add the `EventType` property to [`components/g8ed/constants/events.js`](../../components/g8ed/constants/events.js) (reads from JSON automatically)
4. If the event is needed in the browser, add it to [`components/g8ed/public/js/constants/events.js`](../../components/g8ed/public/js/constants/events.js)
5. If the event is consumed or produced by g8eo, add it to [`components/g8eo/constants/events.go`](../../components/g8eo/constants/events.go)
