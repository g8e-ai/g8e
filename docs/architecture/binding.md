---
title: Session & Identity Binding
---

# Session & Identity Binding

Last Updated: 2026-06-24
Version: v1.2.0

Binding is the cryptographic and stateful linkage between platform sessions (web, CLI, operator) and the identities, agents, and Operator instances they authorize. It is the mechanism that answers: *"Which Operator is allowed to act on behalf of which session?"* and *"Which app is allowed to push events to which target?"*

---

## Overview

The g8e platform uses binding in five distinct contexts:

| Context | Purpose | Storage |
| :--- | :--- | :--- |
| **Session Binding (Web ↔ Operator)** | Links a browser session to one or more Operator sessions | KV store + doc store |
| **CLI Cert Binding** | Verifies a CLI mTLS cert is linked to a specific Operator session | Doc store (CLI sessions) |
| **Binding Persona** | Maps JWT roles to an internal persona for authorization context | Request context |
| **PKI Credential Binding** | Binds app + human requestor identity into a delegated credential | Certificate SANs |
| **Envelope Binding** | Stamps app/operator/user identity onto governance envelopes | Envelope fields |

---

## 1. Session Binding: Web ↔ Operator

### 1.1 Concept

When a user interacts with the g8e Dashboard (web session), they may control multiple Operators running on different hosts. **Session binding** is the bidirectional linkage that records which Operator sessions are associated with a given web session. This linkage is required before:

- SSE events can be pushed to a web session target
- An Operator can be set as the active target context
- Commands can be dispatched from a web session to an Operator

### 1.2 KV Key Scheme

Binding uses the platform KV store with three key prefixes defined in `internal/services/gateway/registration_service.go`:

```
g8e:sessions:web:{web_session_id}:bind       → JSON array of operator_session_id strings
g8e:sessions:operator:{operator_session_id}:bind  → web_session_id string
g8e:sessions:cli:{cli_session_id}:bind        → (CLI binding prefix, reserved)
```

The web→operator direction is a **one-to-many** mapping (one web session can bind multiple operators). The operator→web direction is **one-to-one** (an operator session is bound to at most one web session at a time).

Helper functions:

```go
func sessionWebBindKey(webSessionID string) string {
    return sessionWebBindPrefix + webSessionID + sessionBindSuffix
}

func sessionOperatorBindKey(operatorSessionID string) string {
    return sessionOperatorBindPrefix + operatorSessionID + sessionBindSuffix
}
```

### 1.3 Durable Document: `BoundSessionsDocumentGo`

In addition to the KV store (which is optimized for fast lookups), binding state is persisted in the `bound_sessions` document collection as a `BoundSessionsDocumentGo` (`internal/models/auth.go`):

| Field | Type | Description |
| :--- | :--- | :--- |
| `id` | string | Document ID (equals `web_session_id`) |
| `web_session_id` | string | The browser session |
| `user_id` | string | Owning user |
| `operator_session_ids` | []string | All bound Operator session IDs |
| `operator_ids` | []string | All bound Operator IDs |
| `bound_at` | time.Time | Initial binding timestamp |
| `last_updated_at` | time.Time | Last modification timestamp |
| `status` | OperatorStatus | `active` when operators are bound, `terminated` when all unbound |

This document serves as the **durability layer** — if the KV store is lost, the bound sessions document can reconstruct bindings.

### 1.4 API Endpoints

| Endpoint | Method | Description |
| :--- | :--- | :--- |
| `/api/v1/operators/bind` | POST | Bind one or more operators to a web session |
| `/api/v1/operators/unbind` | POST | Unbind one or more operators from a web session |
| `/api/v1/operators/target` | POST | Set the active target Operator for a web session (binds if not already bound) |

### 1.5 Bind Operation (`BindOperators`)

Location: `internal/services/gateway/registration_service.go:448`

For each operator ID in the request:

1. **Validate** — fetch the Operator document, verify it belongs to the requesting user, and confirm it has an active `OperatorSessionID`.
2. **KV: operator→web** — set `g8e:sessions:operator:{op_session_id}:bind` → `web_session_id`.
3. **KV: web→operator** — fetch existing `g8e:sessions:web:{web_session_id}:bind`, append the operator session ID if not already present, write back.
4. **Durable doc** — create or update the `BoundSessionsDocumentGo` in the `bound_sessions` collection, adding the operator ID and session ID.
5. **Operator doc** — stamp `bound_web_session_id` on the Operator document for UI consumption.

**Request/Response types** (`internal/models/auth.go`):

```go
type BindOperatorsRequest struct {
    OperatorIDs  []string `json:"operator_ids"`
    UserID       string   `json:"user_id"`
    WebSessionID string   `json:"web_session_id"`
}

type BindOperatorsResponse struct {
    Success           bool     `json:"success"`
    BoundCount        int      `json:"bound_count"`
    FailedCount       int      `json:"failed_count"`
    BoundOperatorIDs  []string `json:"bound_operator_ids"`
    FailedOperatorIDs []string `json:"failed_operator_ids"`
    Error             string   `json:"error,omitempty"`
}
```

### 1.6 Unbind Operation (`UnbindOperators`)

Location: `internal/services/gateway/registration_service.go:624`

For each operator ID:

1. **Validate** — fetch the Operator document, verify ownership.
2. **KV: operator→web** — delete `g8e:sessions:operator:{op_session_id}:bind`.
3. **KV: web→operator** — fetch the web bind key, remove the operator session ID from the array. If the array is now empty, delete the key entirely; otherwise write back the reduced array.
4. **Durable doc** — remove the operator from `BoundSessionsDocumentGo`. If no operators remain, set status to `terminated`.
5. **Operator doc** — clear `bound_web_session_id` (set to empty string).

### 1.7 Target Context (`SetTargetContext`)

Location: `internal/services/gateway/registration_service.go:762`

Sets the active target Operator for a web session. If the Operator is not already bound to the web session (`op.BoundWebSessionID != req.WebSessionID`), it calls `BindOperators` first to establish the binding. This ensures target context operations always operate over a bound relationship.

### 1.8 Operator Document Fields

The Operator document (`internal/models/auth.go`) carries binding state:

| Field | JSON Key | Description |
| :--- | :--- | :--- |
| `BoundWebSessionID` | `bound_web_session_id` | Web session the Operator is currently bound to. Set during bind, cleared on unbind. |
| `OperatorSessionID` | `operator_session_id` | The Operator's own session ID, used as the KV key for binding lookups. |

### 1.9 Operator Status: `bound`

The `bound` status (`internal/constants/status.go:74`) is one of the valid Operator lifecycle states:

```
available → bound → active → stopped → terminated
                 ↘ offline
                 ↘ stale
```

When an Operator is bound to a web session, its status transitions to `bound`. The corresponding status-updated event is `g8e.v1.operator.status.updated.bound` (`protocol/constants/events.json`).

---

## 2. CLI Cert Binding to Operator

### 2.1 Concept

A CLI client (`g8e login`) receives its own mTLS certificate with a SPIFFE URI SAN identifying the CLI session. The CLI session is itself linked to an Operator session via the `OperatorSessionID` field on the `CLISession` model. This creates a chain:

```
CLI mTLS cert (SPIFFE URI) → CLI session → OperatorSessionID → Operator session
```

### 2.2 `cliCertBoundToOperator`

Location: `internal/services/gateway/gateway_auth.go:660`

This function verifies that a presented client certificate belongs to a CLI session whose `OperatorSessionID` matches the claimed operator session. It is used during authentication to allow CLI clients to call internal APIs scoped by `cli_session_id` while presenting their CLI mTLS cert and the linked operator session as a Bearer token.

**Algorithm:**

1. Extract SPIFFE URIs from the client certificate's SANs.
2. Match against `WorkloadIdentity.MatchesCLI(uri, userID, cliSessionID)` or `MatchesCLISessionOnly(uri, cliSessionID)`.
3. Load the CLI session document from the `cli_sessions` collection.
4. Check that the session has not expired.
5. Return `cliSession.OperatorSessionID == operatorSessionID`.

### 2.3 CLI Session Persistence

Location: `internal/services/gateway/cli_session_service.go:44`

`PersistCLISession` creates the CLI session document with the `OperatorSessionID` field that establishes the binding:

```go
cliSession := models.CLISession{
    ID:                cliSessionID,
    UserID:            userID,
    OperatorSessionID: operatorSessionID,  // ← the binding link
    SystemFingerprint: systemFingerprint,
    CertFingerprint:   certFingerprint,
    CertSerial:        certSerial,
    ...
}
```

---

## 3. Binding Persona (JWT Auth Context)

### 3.1 Concept

When external Identity Provider (IdP) JWT tokens are used for authentication, the JWT `roles` claim is mapped to an internal **binding persona** string. This persona is stamped into the request context and used downstream by the governance envelope builder.

### 3.2 Flow

Location: `internal/services/gateway/gateway_auth.go:857-873`

1. JWT is validated against the configured JWKS endpoint.
2. `PersonaService.MapRolesToPersona(jwt.Roles)` maps the JWT roles to a persona string. If mapping fails, `"default"` is used.
3. The persona is stamped into the request context via `ContextKeyBindingPersona`.
4. The MCP gateway envelope builder (`internal/services/mcp/gateway.go:1294`) reads the persona from context and sets `env.BindingPersona` on the governance envelope.

### 3.3 Context Key

Defined in `protocol/constants/auth.json`:

```json
"binding_persona": {
    "_go_const": "ContextKeyBindingPersona",
    "value": "binding_persona",
    "description": "Stores the binding persona identifier in context"
}
```

---

## 4. PKI Credential Binding (Delegated App Credentials)

### 4.1 Concept

When `g8e mcp agent run` launches an AI agent, it requests a short-lived delegated credential from the Gateway's `/api/v1/pki/apps/delegated` endpoint. This credential **binds both** the app identity and the human requestor identity into the certificate's SANs.

### 4.2 Implementation

Location: `internal/services/gateway/pki_controller.go:357`

The `handlePKIAppsDelegated` handler:

1. Requires mTLS authentication from a human CLI session (not an app cert).
2. Extracts the user ID from the CLI certificate.
3. Mints a short-lived (1-hour) certificate with dual URI SANs (`internal/services/gateway/gateway_certs.go:818-828`):
   - App identity: `spiffe://g8e.local/app/<app_name>`
   - Requestor identity: `spiffe://g8e.local/user/<user_id>`
4. The resulting credential is injected into the agent subprocess via `G8E_APP_CERT` / `G8E_APP_KEY` environment variables.

### 4.3 Envelope Binding

Location: `internal/services/mcp/gateway.go:1290-1314`

The governance envelope builder binds both identities to the envelope:

- `env.OperatorId` / `env.OperatorSessionId` / `env.ActingAppId` — set from the app identity (for delegated credentials) or operator identity (for operator sessions).
- `env.RequestorUserId` — set from the human user ID.
- `env.BindingPersona` — set from the binding persona context key.
- `env.TenantId` — set from the tenant ID context key.

This ensures every governance envelope carries the full identity chain: who requested it (human), what executed it (app/operator), and under which persona/tenant.

---

## 5. SSE Authorization via Binding

### 5.1 Push Authorization (App → Target)

Location: `internal/services/gateway/gateway_http_sse.go:126-224`

When an app workload pushes an SSE event to a target session, the Gateway verifies the app is authorized for that target by checking the binding:

**Web session target:**
1. Look up `sessionWebBindKey(web_session_id)` in KV → get list of bound operator session IDs.
2. For each bound operator session, check if the app's SPIFFE ID matches via `WorkloadIdentity.MatchesApp(appID, operatorID)`.
3. If no match, return `403 Forbidden`.

**CLI session target:**
1. Load the CLI session document.
2. Get the `OperatorSessionID` from the CLI session.
3. Load the Operator document for that session.
4. Verify `WorkloadIdentity.MatchesApp(appID, operatorID)`.
5. If no match, return `403 Forbidden`.

### 5.2 Stream Authorization (Consumer → Events)

Location: `internal/services/gateway/gateway_http_sse.go:285-336`

When a consumer connects to the SSE stream, the Gateway verifies the Operator session is bound to the declared routing target:

- **CLI session target:** Load the CLI session, verify `cliSess.OperatorSessionID == operatorSessionID`.
- **Web session target:** Look up `sessionOperatorBindKey(operatorSessionID)` in KV, verify the returned `web_session_id` matches the requested `route.WebSessionID`.
- **User-scoped target:** Validate the Operator session, verify `op.UserID == route.UserID`.

---

## 6. Protocol Constants & Event Taxonomy

### 6.1 KV Key Constants

Defined in `protocol/constants/kv_keys.json`:

| Constant | Key Pattern | Purpose |
| :--- | :--- | :--- |
| `KVKeySessionOperatorBind` | `g8e:sessions:operator:{operator.session.id}:bind` | Operator → web session |
| `KVKeySessionWebBind` | `g8e:sessions:web:{web.session.id}:bind` | Web session → operator sessions |

### 6.2 HTTP Header

| Constant | Header | Purpose |
| :--- | :--- | :--- |
| `HeaderBoundOperators` | `X-G8E-Bound-Operators` | Communicates bound operator info in HTTP responses |

### 6.3 Operator Events

Defined in `internal/constants/events.go` and `protocol/constants/events.json`:

| Constant | Value | Description |
| :--- | :--- | :--- |
| `EventOperatorBound` | `g8e.v1.operator.bound` | Operator bound event |
| `EventOperatorUnbound` | `g8e.v1.operator.unbound` | Operator unbound event |
| `EventOperatorStatusUpdatedBound` | `g8e.v1.operator.status.updated.bound` | Operator status changed to bound |
| `HistoryEventTypeBound` | `bound` | History event type for binding (`internal/constants/status.go`) |

### 6.4 Agent Modes

Defined in `internal/constants/prompts.go`:

| Constant | Value | Description |
| :--- | :--- | :--- |
| `AgentModeG8eBound` | `g8e.bound` | Operator is bound |
| `AgentModeCloudOperatorBound` | `g8e.cloud.bound` | Cloud operator is bound |
| `AgentModeG8eNotBound` | `g8e.not.bound` | Operator is not bound |

### 6.5 Agent Activity Metadata

Defined in `protocol/models/agent_activity_metadata.json`:

| Field | Type | Description |
| :--- | :--- | :--- |
| `operator_bound` | boolean | Whether operators were bound |
| `bound_operator_count` | integer | Number of bound operators |

### 6.6 Reputation & Stake Binding

Both `protocol/models/reputation_commitment.json` and `protocol/models/stake_resolution.json` contain a `binding` object that links the record to a specific `investigation_id`, ensuring reputation and stake data are cryptographically bound to the investigation context.

---

## 7. Error Constants

All binding-related errors are defined in `internal/constants/errors.go`:

| Constant | Description |
| :--- | :--- |
| `ErrIdentityBindingFailed` | Identity binding failed |
| `ErrRegistrationFailedToSetKVBinding` | Failed to set KV binding |
| `ErrRegistrationFailedToMarshalSessionIDs` | Failed to marshal session IDs |
| `ErrRegistrationFailedToGetBoundSessions` | Failed to get bound sessions document |
| `ErrRegistrationFailedToMarshalBoundSessions` | Failed to marshal bound sessions document |
| `ErrRegistrationFailedToMarshalExistingDocument` | Failed to marshal existing document |
| `ErrRegistrationFailedToSetBoundSessions` | Failed to set bound sessions document |
| `ErrRegistrationFailedToUnmarshalBoundSessions` | Failed to unmarshal bound sessions document |
| `ErrRegistrationFailedToUpdateBoundSessions` | Failed to update bound sessions document |
| `ErrRegistrationFailedToBindOperator` | Failed to bind Operator for target context |

---

## 8. Data Flow Diagram

```
                    ┌─────────────┐
                    │  Web Browser │
                    │  (Dashboard)  │
                    └──────┬───────┘
                           │ web_session_id
                           ▼
                    ┌──────────────┐
                    │  Web Session  │
                    │  (doc store)  │
                    └──────┬───────┘
                           │
              ┌────────────┼────────────┐
              │ KV: web→op │  Doc:     │
              │ bind key   │  bound_   │
              │            │  sessions │
              ▼            └───────────┘
     ┌────────────────┐
     │ Operator Sess 1 │ ←── KV: op→web bind key
     │ (host A)        │
     └────────────────┘
     ┌────────────────┐
     │ Operator Sess 2 │ ←── KV: op→web bind key
     │ (host B)        │
     └────────────────┘

                    ┌─────────────┐
                    │  CLI Client  │
                    │  (g8e login) │
                    └──────┬───────┘
                           │ mTLS cert (SPIFFE URI)
                           ▼
                    ┌──────────────┐
                    │  CLI Session  │
                    │  (doc store)  │
                    │  .Operator    │
                    │  SessionID    │
                    └──────┬───────┘
                           │
                           ▼
                    ┌──────────────┐
                    │  Operator     │
                    │  Session      │
                    └──────────────┘
```

---

## 9. Security Properties

1. **Bidirectional updates**: Both KV directions (web→operator and operator→web) are updated during bind/unbind. During bind, if the operator→web KV write succeeds but the web→operator write fails, the operator→web binding is not rolled back; the operator is added to the failed list. During unbind, KV deletion failures are logged as warnings and do not prevent the operator from being unbound.

2. **Ownership enforcement**: Bind/unbind operations verify that the Operator belongs to the requesting user (`op.UserID == req.UserID`), preventing cross-user binding attacks.

3. **Session expiry checks**: CLI cert binding checks `cliSession.ExpiresAt` before accepting the binding, preventing expired sessions from being used.

4. **SSE push authorization**: Apps cannot push events to sessions they are not bound to. The binding chain (app → operator → web/CLI session) is verified on every push.

5. **SSE stream authorization**: Consumers cannot read events from sessions their Operator is not bound to. The binding is verified at stream connection time.

6. **Durable recovery**: The `BoundSessionsDocumentGo` document provides a durability layer. If the KV store is lost, bindings can be reconstructed from the document store.

7. **Envelope identity binding**: Every governance envelope carries the full identity chain (app, operator, user, persona, tenant), ensuring auditability of all mutations.

---

## 10. Related Documents

- [Authentication & Authorization](./auth.md) — mTLS, SPIFFE workload identity, JWT/JWKS, passkey bootstrap
- [Operator Architecture](./operator.md) — Operator lifecycle, 5-layer verification, session management
- [SSE Streaming](./sse.md) — Event delivery infrastructure, routing, push/stream authorization
- [Network Architecture](./network.md) — PKI hierarchy, SPIFFE ID formats, mTLS enforcement
- [Storage Architecture](./storage.md) — KV store, document store, collection schemas
