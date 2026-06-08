---
title: A2A Protocol
---

# A2A Protocol

Last Updated: 2026-05-31

The g8e Operator in gateway mode supports Agent-to-Agent (A2A) protocol integration. A2A agents send HTTP/JSON skill invocation requests to the g8e Gateway, which wraps them in the g8e governance envelope, runs them through the 5-layer verification sequence (L1Doctrine, L2Consensus, L3Notary, L4Warden, L5Actuator), and dispatches verified payloads to downstream A2A servers or to the in-process execution service for local execution.

---

## Protocol Overview

A2A is an HTTP/JSON protocol for agent skill invocation. A2A agents connect to the gateway via skill invocation endpoints with JSON payload structure.

### Request Structure

A2A requests follow an HTTP/JSON pattern:

- **Transport**: HTTP/JSON
- **Authentication**: mTLS certificate or API key depending on configuration
- **Payload**: JSON structure with skill_name and parameters

### Gateway Integration

The g8e Gateway translates A2A skill invocations into governance envelopes:

1. **Inbound**: A2A agent sends HTTP/JSON skill invocation to gateway
2. **Envelope Construction**: Gateway wraps payload in `GovernanceEnvelope` with action type `A2A_CALL`
3. **Verification**: Envelope passes through L1/L2/L3/L4 verification gates
4. **Dispatch**: Verified envelope forwarded to L5Actuator for execution to downstream A2A server or local execution

---

## A2A Payload Types

### A2A_CALL

The gateway maps A2A skill invocations to the `A2A_CALL` action type with the `A2aCallRequested` proto payload:

| Field | Type | Description |
|---|---|---|
| `skill_name` | string | Name of the skill to invoke (L1 forbidden patterns: sudo, su) |
| `payload_json` | string | JSON-encoded A2A task payload |
| `execution_id` | string | Optional client-supplied invocation identifier |

### Canonical JSON Wire Format

All envelopes use canonical JSON (protojson) encoding for client-facing surfaces:

- Schema source of truth: `.proto` files in `protocol/proto/`
- Wire format: canonical JSON (protojson)
- Signing basis: deterministic `transaction_hash` computed from normalized envelope fields
- Internal storage: protobuf bytes (implementation detail)

This ensures compatibility with JSON-based ecosystems while maintaining typed schema validation.

---

## Client Integration

### A2A Agent Connection

A2A agents connect to the gateway via:

- **HTTP/JSON**: Skill invocation endpoints with JSON payload structure
- **Authentication**: mTLS certificate or API key depending on configuration

### Skill Invocation

Invoke A2A skills via POST to `/api/a2a/v1/call`:

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

Bring-your-own clients can integrate by:

1. Submitting standard A2A requests to the g8e Gateway HTTP endpoints
2. Receiving `A2ASuccessResponse` or `A2ASuspensionResponse` with verification proofs
3. Trusting the Gateway's cryptographic guarantees without implementing full protocol

#### Response Types

- **A2ASuccessResponse**: Returned when A2A call succeeds
  - `id`: Transaction hash
  - `result`: ActionReceipt with execution status and result summary

- **A2ASuspensionResponse**: Returned when A2A call is suspended for L3 approval
  - `id`: Transaction hash
  - `status`: "suspended"
  - `tx_hash`: Transaction hash
  - `approval_url`: URL for WebAuthn authorization
  - `message`: "Execution paused for L3 authorization"

---

## Security and Verification

### L1 Doctrine (Hard Gates)

- **Forbidden patterns**: Protobuf field options with regex constraints on skill names (e.g., sudo, su)
- **Runtime scanning**: Gateway validates skill names against L1 forbidden patterns before envelope construction
- **Field validation**: Payload parameters are validated against allowlist/denylist where configured

### L2 Consensus

- **Ed25519 signatures**: Consensus agents sign envelopes with their private keys
- **Signer verification**: Gateway verifies signatures against trusted signers in SQLite store
- **Gateway bypass**: In doctrine mode, Gateway signs envelopes locally (single-agent consensus)
- **Reputation staking**: Signers stake reputation on their decisions

### L3 Notary (Authorization)

- **mTLS fingerprint**: CLI sessions use mTLS certificate fingerprint as proof via CLIL3Notary
- **Composite verifier**: CompositeL3Verifier handles both web and CLI session types
- **Auto-approval**: Benign diagnostic commands may skip human prompt after L1/L2 pass

---

## Error Handling

A2A protocol errors follow gateway error conventions:

- **Invalid request**: Standard JSON-RPC error codes
- **Authentication failure**: mTLS certificate validation failure
- **Authorization failure**: L3 verification failure
- **Circuit breaker**: Downstream server temporarily unavailable

### Error Mapping

Gateway verification errors are mapped to g8e custom JSON-RPC error codes via `mapGatewayError`:

- **Invalid envelope**: `-32000` (ErrCodeInvalidEnvelope) - malformed GovernanceEnvelope, missing ID, or unknown action type
- **Payload decode failed**: `-32008` (ErrCodePayloadDecodeFailed) - protobuf unmarshaling error
- **Hash mismatch**: `-32001` (ErrCodeHashMismatch) - transaction_hash validation failure
- **L1 rejection**: `-32000` (ErrCodeInvalidEnvelope) - forbidden pattern violation
- **L2 rejection**: `-32001` (ErrCodeHashMismatch) - signature verification failure
- **L3 rejection**: `-32001` (ErrCodeHashMismatch) - missing or invalid L3 proof
- **Downstream unavailable**: `-32003` (Internal error) - circuit breaker open

---

## Configuration

### Gateway Postures

The g8e Gateway supports three governance postures (configured via CLI flags):

| Posture | Configuration | Purpose |
|---|---|---|
| **PostureDoctrine** | `doctrine` | L1 enforced, L2/L3 signature not required (default) |
| **PostureConsensus** | `consensus` | L1/L2 enforced, L3 signature not required |
| **PostureNotary** | `notary` | L1/L2/L3 strictly enforced (default for outbound mode) |

### Port Configuration

Default ports (configurable via config or paths.json):

| Port | Purpose | Auth |
|---|---|---|
| `8080` | HTTP (bootstrap + MCP) | Plain HTTP (no TLS) |
| `8443` | HTTPS (mTLS API + public) | mTLS (RequireAndVerifyClientCert) |

### Configuration

The g8e platform uses **ZERO environment variables** for production configuration. All paths are computed relative to project root, and all configuration is via CLI flags:

- `--data-dir <dir>`: Data directory for SQLite database (default: `.g8e/data` in working directory)
- `--pki-dir <dir>`: Directory for TLS certificates (default: `.g8e/pki`)
- `--secrets-dir <dir>`: Directory for platform secrets (default: `.g8e/secrets`)
- `--http-port <port>`: HTTP port for bootstrap and MCP routes (default: 8080)
- `--https-port <port>`: HTTPS port for mTLS API and public surface (default: 8443)

### Circuit Breaker

The Gateway implements a circuit breaker for downstream A2A servers:

- **Max failures**: 5 consecutive failures before opening circuit
- **Cooldown**: 1 minute before attempting recovery
- **Behavior**: Rejects requests with error when circuit is open

---

## Session Management

The Gateway enforces strict session separation via SQLite-backed session store:

| Session Type | Identifier | Authentication | Use Case |
|---|---|---|---|
| **Web Session** | `web_session_id` | WebAuthn (passkey) | Browser-based clients |
| **CLI Session** | `cli_session_id` | mTLS certificate | CLI/BYO clients |
| **Operator Session** | `operator_session_id` | mTLS certificate | In-process execution context |

Sessions are cryptographically bound to their authentication mechanism and cannot be conflated. SSE and pub/sub routing uses these identifiers to prevent cross-tenant data leakage.

---

## Implementation Reference

| Concern | File |
|---|---|
| Gateway entry | `cmd/operator/main.go` (runGatewayMode) |
| Gateway service | `internal/services/gateway/gateway_service.go` |
| HTTP routing | `internal/services/gateway/gateway_http.go` |
| A2A translation | `internal/services/mcp/gateway.go` (HandleA2aCall) |
| Envelope construction | `internal/services/mcp/gateway.go` (processGatewayTransaction) |
| Transaction verification | `internal/services/governance/l4_warden.go` |
| Envelope processor | `internal/services/governance/processor.go` |
| Pub/Sub command service | `internal/services/pubsub/pubsub_commands.go` |
| Session management | `internal/services/gateway/session_service.go` |
| CLI L3 verification | `internal/services/gateway/cli_l3_notary.go` |
| Composite L3 verifier | `internal/services/gateway/composite_l3_verifier.go` |
| Error mapping | `internal/services/mcp/gateway.go` (mapGatewayError) |
| Circuit breaker | `internal/services/mcp/gateway.go` (isCircuitOpen, recordFailure, recordSuccess) |
| Response models | `internal/services/mcp/models.go` (A2ASuccessResponse, A2ASuspensionResponse) |

---

## Related Documentation

- [**g8e Protocol**](../../architecture/protocol.md) - The wire contract and governance hierarchy
- [**Operator (g8eo)**](../../architecture/operator.md) - Operator architecture and gateway mode
- [**MCP Protocol**](../mcp/mcp.md) - MCP protocol specification and integration
