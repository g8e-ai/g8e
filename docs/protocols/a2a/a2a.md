---
title: A2A Protocol
---

# A2A Protocol

Last Updated: 2026-06-15

The g8e Operator supports Agent-to-Agent (A2A) protocol integration. A2A agents submit HTTP/JSON skill invocation requests to the g8e Gateway, which encapsulates them in a governance envelope, executes the 5-layer verification sequence (L1 Doctrine, L2 Consensus, L3 Notary, L4 Warden, L5 Actuator), and dispatches verified payloads to downstream A2A servers or the in-process execution service.

---

## Protocol Overview

A2A is an HTTP/JSON protocol for agent skill invocation. A2A agents connect to the gateway via skill invocation endpoints using a JSON payload structure.

### Request Structure

A2A requests follow an HTTP/JSON pattern:

- **Transport**: HTTP/JSON
- **Authentication**: mTLS certificate or JWT (when JWKS is configured) or API key depending on configuration
- **Payload**: JSON-RPC 2.0 structure with `skill_name` and `payload` parameters

### Gateway Integration

The g8e Gateway translates A2A skill invocations into governance envelopes:

1. **Inbound**: A2A agent sends HTTP/JSON skill invocation to the gateway.
2. **Envelope Construction**: Gateway wraps the payload in a `GovernanceEnvelope` with action type `A2A_CALL`.
3. **Verification**: The envelope passes through L1-L4 verification gates.
4. **Dispatch**: Verified envelopes are forwarded to the L5 Actuator for execution to a downstream A2A server or local execution.

---

## A2A Payload Types

### A2A_CALL

The gateway maps A2A skill invocations to the `A2A_CALL` action type. The `A2aCallRequested` protobuf payload is defined in `protocol/proto/g8e/operator/v1/operator.proto`:

| Field | Type | Description |
|---|---|---|
| `skill_name` | string | Name of the skill to invoke (L1 forbidden patterns: (?i)^(sudo|su)$) |
| `payload_json` | string | JSON-encoded A2A task payload |
| `execution_id` | string | Optional client-supplied invocation identifier |

### Canonical JSON Wire Format

All envelopes use canonical JSON (protojson) encoding for client-facing surfaces:

- **Schema source of truth**: `protocol/proto/g8e/operator/v1/operator.proto`
- **Wire format**: Canonical JSON (protojson)
- **Signing basis**: Deterministic `transaction_hash` computed from normalized envelope fields
- **Internal storage**: Protobuf bytes (implementation detail)

This ensures compatibility with JSON-based ecosystems while maintaining typed schema validation.

---

## Client Integration

### A2A Agent Connection

A2A agents connect to the gateway via:

- **HTTP/JSON**: Skill invocation endpoints with JSON-RPC 2.0 payload structure
- **Authentication**: mTLS certificate or JWT (when JWKS is configured) or API key depending on configuration

### Skill Invocation

Invoke A2A skills via POST to `/api/v1/a2a/call`:

```json
{
  "jsonrpc": "2.0",
  "id": 1,
  "method": "a2a/call",
  "params": {
    "skill_name": "example_skill",
    "payload": {
      "param1": "value1",
      "param2": "value2"
    },
    "execution_id": "optional_execution_id"
  }
}
```

### Skill Discovery

Skill discovery is not currently implemented. The A2A downstream URL is configured but no discovery endpoint exists. Skills must be known a priori or discovered through out-of-band mechanisms.

### BYO Clients

Bring-your-own (BYO) clients integrate by:

1. Submitting standard A2A requests to the g8e Gateway HTTP endpoints.
2. Receiving `A2ASuccessResponse` or `A2ASuspensionResponse` with verification proofs.
3. Trusting the gateway's cryptographic guarantees without implementing the full protocol.

#### Response Types

- **A2ASuccessResponse**: Returned when the A2A call succeeds.
  - `id`: Transaction hash
  - `result`: `ActionReceipt` with execution status and result summary

- **A2ASuspensionResponse**: Returned when the A2A call is suspended for L3 approval.
  - `id`: Transaction hash
  - `status`: "suspended"
  - `tx_hash`: Transaction hash
  - `approval_url`: URL for WebAuthn authorization
  - `message`: "Execution paused for L3 authorization"

---

## Security and Verification

The g8e platform enforces security across five layers:

### L1 Doctrine (Hard Gates)

- **Forbidden patterns**: Protobuf field options with regex constraints on skill names defined in `protocol/proto/g8e/operator/v1/operator.proto` (pattern: (?i)^(sudo|su)$).
- **Runtime scanning**: The gateway validates skill names against L1 forbidden patterns before envelope construction.
- **Field validation**: Payload parameters are validated against allowlist/denylist where configured.

### L2 Consensus

- **Ed25519 signatures**: Consensus agents sign envelopes with Ed25519 private keys.
- **Signer verification**: The gateway verifies signatures against trusted signers in the SQLite store.
- **Gateway bypass**: In doctrine mode, the gateway signs envelopes locally (single-agent consensus).
- **L2 status tracking**: `ActionReceipt.l2_status` distinguishes between `L2_STATUS_NOT_REQUIRED`, `L2_STATUS_REQUIRED_VALID`, and `L2_STATUS_REQUIRED_FAILED`.

### L3 Notary (Authorization)

- **mTLS fingerprint**: CLI sessions use mTLS certificate fingerprints as proof via `internal/services/gateway/cli_l3_notary.go`.
- **Composite verifier**: `internal/services/gateway/composite_l3_verifier.go` handles both web and CLI session types.
- **Suspension**: Transactions requiring L3 approval are suspended and stored for later resumption via WebAuthn or CLI proof.
- **L3 status tracking**: `ActionReceipt.l3_status` distinguishes between `L3_STATUS_NOT_REQUIRED`, `L3_STATUS_REQUIRED_VALID`, and `L3_STATUS_REQUIRED_FAILED`.

### L4 Warden (Pre-dispatch)

- **Verification**: `internal/services/governance/l4_warden.go` validates signatures, replay prevention, expiry, nonces, and the state Merkle root.

### L5 Actuator (Dispatch)

- **Dispatch**: Verified payloads are dispatched to downstream A2A servers or local execution.
- **Receipts**: Produces signed `ActionReceipt` objects upon completion.

---

## Error Handling

A2A protocol errors follow gateway error conventions. Verification errors map to granular JSON-RPC codes defined in `internal/constants/rpc_errors.go`:

- **Invalid envelope**: `-32000` (`ErrCodeInvalidEnvelope`) - malformed `GovernanceEnvelope`, missing ID, or unknown action type.
- **Hash mismatch**: `-32001` (`ErrCodeHashMismatch`) - `transaction_hash` validation failure.
- **Expired**: `-32002` (`ErrCodeExpired`) - transaction expired.
- **Replay**: `-32003` (`ErrCodeReplay`) - replay attack detected.
- **State mismatch**: `-32004` (`ErrCodeStateMismatch`) - state root mismatch.
- **L1 rejection**: `-32005` (`ErrCodeL1ValidationFailed`) - forbidden pattern violation.
- **L2 rejection**: `-32006` (`ErrCodeL2SignatureInvalid`) - signature verification failure.
- **L3 rejection**: `-32007` (`ErrCodeL3ProofInvalid`) - missing or invalid L3 proof.
- **Payload decode failed**: `-32008` (`ErrCodePayloadDecodeFailed`) - protobuf unmarshaling error.

---

## Configuration

### Gateway Postures

The g8e Gateway supports three governance postures (configured via CLI flags):

| Posture | Configuration | Purpose |
|---|---|---|
| **PostureDoctrine** | `doctrine` | L1 enforced, L2/L3 signatures not required (default). |
| **PostureConsensus** | `consensus` | L1/L2 enforced, L3 signature not required. |
| **PostureNotary** | `notary` | L1/L2/L3 strictly enforced. |

### Port Configuration

The platform uses a consolidated 2-port gateway:

| Port | Purpose | Auth |
|---|---|---|
| `8080` | HTTP (bootstrap, MCP, A2A) | Plain HTTP with loopback origin protection. |
| `8443` | HTTPS (mTLS API, public) | mTLS (RequireAndVerifyClientCert). |

### CLI Configuration

The g8e platform uses **ZERO environment variables** for production configuration. All configuration is performed via CLI flags:

- `--data-dir <dir>`: Data directory for SQLite database (default: `.g8e/data`).
- `--pki-dir <dir>`: Directory for TLS certificates (default: `.g8e/pki`).
- `--secrets-dir <dir>`: Directory for platform secrets (default: `.g8e/secrets`).
- `--http-port <port>`: HTTP port for bootstrap, MCP, and A2A routes (default: 8080).
- `--https-port <port>`: HTTPS port for mTLS API and public surface (default: 8443).

### Circuit Breaker

The Gateway implements a circuit breaker for downstream A2A servers:

- **Max failures**: 5 consecutive failures before opening circuit
- **Cooldown**: 1 minute before attempting recovery
- **Behavior**: Rejects requests with error when circuit is open

---

## Session Management

The gateway enforces session separation via a SQLite-backed session store:

| Session Type | Identifier | Authentication | Use Case |
|---|---|---|---|
| **Web Session** | `web_session_id` | WebAuthn (passkey) | Browser-based clients. |
| **CLI Session** | `cli_session_id` | mTLS certificate | CLI/BYO clients. |
| **Operator Session** | `operator_session_id` | mTLS certificate | In-process execution context. |

Sessions are cryptographically bound to their authentication mechanism.

---

## Implementation Reference

| Concern | File |
|---|---|
| Gateway entry | `cmd/operator/main.go` |
| Gateway service | `internal/services/gateway/gateway_service.go` |
| HTTP routing | `internal/services/gateway/gateway_http.go` |
| A2A translation | `internal/services/mcp/gateway.go` |
| Envelope construction | `internal/services/mcp/gateway.go` |
| Transaction verification | `internal/services/governance/l4_warden.go` |
| Envelope processor | `internal/services/governance/processor.go` |
| Downstream dispatch | `internal/services/mcp/gateway.go` |
| Session management | `internal/services/gateway/session_service.go` |
| Error codes | `internal/constants/rpc_errors.go` |
| Proto schema | `protocol/proto/g8e/operator/v1/operator.proto` |

---

## Related Documentation

- [**g8e Protocol**](../../architecture/protocol.md) - The wire contract and governance hierarchy.
- [**Operator (g8eo)**](../../architecture/operator.md) - Operator architecture and gateway mode.
- [**MCP Protocol**](../mcp/mcp.md) - MCP protocol specification and integration.
