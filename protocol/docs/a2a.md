---
title: A2A Protocol
---

# A2A Protocol

Last Updated: 2026-07-29
Version: v1.6.6

The g8e Operator supports Agent-to-Agent (A2A) protocol integration. A2A agents submit HTTP/JSON skill invocation requests to the g8e Gateway, which encapsulates them in a governance envelope, executes the 5-layer verification sequence (L1 Doctrine, L2 Consensus, L3 Notary, L4 Warden, L5 Actuator), and dispatches verified payloads to a configured downstream A2A server.

---

## Protocol Overview

A2A is an HTTP/JSON protocol for agent skill invocation. A2A agents connect to the gateway via skill invocation endpoints using a JSON payload structure.

### Request Structure

A2A requests follow an HTTP/JSON pattern:

- **Transport**: HTTP/JSON
- **Authentication**: mTLS certificate (HTTPS port) or JWT (HTTP port, when JWKS is configured)
- **Payload**: JSON-RPC 2.0 structure with `skill_name`, `payload`, and optional `execution_id` parameters

### Gateway Integration

The g8e Gateway translates A2A skill invocations into governance envelopes:

1. **Inbound**: A2A agent sends HTTP/JSON skill invocation to the gateway.
2. **Envelope Construction**: Gateway wraps the payload in a `GovernanceEnvelope` with action type `A2A_CALL`.
3. **Verification**: The envelope passes through L1-L4 verification gates.
4. **Dispatch**: Verified envelopes are forwarded to the L5 Actuator, which dispatches the call to a configured downstream A2A server via `DispatchToA2ADownstream`.

---

## A2A Payload Types

### A2A_CALL

The gateway maps A2A skill invocations to the `A2A_CALL` action type (defined in `internal/constants/action_types.go`). The `A2aCallRequested` protobuf payload is defined in `protocol/proto/g8e/operator/v1/operator.proto`, and the corresponding event type `g8e.v1.operator.a2a.call.requested` is registered in `protocol/constants/events.json`:

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
- **Authentication**: mTLS certificate (HTTPS port) or JWT (HTTP port, when JWKS is configured)

### Skill Invocation

Invoke A2A skills via POST to `/api/v1/a2a/call` (registered in `internal/constants/api_paths.go`). The request is wrapped in a JSON-RPC 2.0 envelope:

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

A2A calls are also dispatched through the unified MCP endpoint at `/mcp` (`HandleMCP` in `internal/services/mcp/mcp_endpoint.go`), which routes the `a2a/call` method to the same `a2aCall` handler.

### Skill Discovery

Skill discovery is not currently implemented. The A2A downstream URL is configured via `internal/config/config.go` (`GatewayConfig.A2ADownstreamURL`) and passed to the gateway service through the `Dependencies.A2ADownstreamURL` field in `NewGatewayService`, but no discovery endpoint exists. Skills must be known a priori or discovered through out-of-band mechanisms.

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
  - `message`: Directive string from `approvalPausedMessage` containing the approval URL and a 2-minute TTL deadline

---

## Security and Verification

The g8e platform enforces security across five layers:

### L1 Doctrine (Hard Gates)

- **Forbidden patterns**: Protobuf field options with regex constraints on skill names defined in `protocol/proto/g8e/operator/v1/operator.proto` (pattern: (?i)^(sudo|su)$).
- **Runtime scanning**: L1 validation runs during `VerifyEnvelope` in `internal/services/governance/l4_warden.go`, which calls `L1Doctrine.ValidatePayload` in `internal/services/governance/l1_doctrine.go` after payload decoding. The `ValidatePayload` method checks forbidden patterns on protobuf string fields.
- **MITRE-based threat detection**: The `payload_json` field is analyzed via `AnalyzeMCPArguments` for MITRE ATT&CK threat indicators, including reverse shells, privilege escalation, credential access, data exfiltration, and system tampering patterns.

### L2 Consensus

- **Ed25519 signatures**: Consensus agents sign envelopes with Ed25519 private keys.
- **Signer verification**: The gateway verifies signatures against trusted signers in the SQLite store.
- **Doctrine mode**: L2 signatures are not required; the L2 status is recorded as `L2_STATUS_NOT_REQUIRED`.
- **L2 status tracking**: `ActionReceipt.l2_status` distinguishes between `L2_STATUS_NOT_REQUIRED`, `L2_STATUS_REQUIRED_VALID`, and `L2_STATUS_REQUIRED_FAILED`.

### L3 Notary (Authorization)

- **mTLS fingerprint**: CLI sessions use mTLS certificate fingerprints as proof. The `CLISessionVerifier` in `internal/services/gateway/cli_session_verifier.go` performs user active, session validity, and certificate revocation checks.
- **Unified notary**: `NewGatewayL3Notary` in `internal/services/governance/l3_notary.go` creates a composite `L3Notary` that uses a layered model: passkey (WebAuthn) verification is always required first, and CLI mTLS session verification is additionally performed when `mtls_cert_fingerprint` is present in the proof.
- **Suspension**: Transactions requiring L3 approval are suspended and stored for later resumption via WebAuthn or CLI proof.
- **L3 status tracking**: `ActionReceipt.l3_status` distinguishes between `L3_STATUS_NOT_REQUIRED`, `L3_STATUS_REQUIRED_VALID`, and `L3_STATUS_REQUIRED_FAILED`.

### L4 Warden (Pre-dispatch)

- **Verification**: `internal/services/governance/l4_warden.go` validates signatures, replay prevention, expiry, nonces, and the state Merkle root.

### L5 Actuator (Dispatch)

- **Dispatch**: The Actuator egress handler `handleA2aCallRequestSync` in `internal/services/pubsub/pubsub_commands.go` unmarshals the `A2aCallRequested` payload and calls `DispatchToA2ADownstream` to forward the call to the configured downstream A2A server.
- **Downstream request**: The gateway sends an `A2ADownstreamRequest` (defined in `internal/services/mcp/models.go`) containing `skill_name` and `payload` as JSON to the downstream URL via HTTP POST with a 30-second timeout. The `execution_id` field is present in the struct but is not populated during downstream dispatch.
- **Downstream response**: The gateway parses the response for `result`, `summary`, or `error` fields. If `summary` is present, it is used; otherwise `result` is used; otherwise the gateway returns "completed".
- **Receipt bounding**: The receipt summary is truncated to `ReceiptSummaryMaxBytes` (4096 bytes, defined in `internal/constants/pubsub.go`) to prevent unbounded growth.
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

The g8e platform uses CLI flags for production configuration. See [g8e Protocol](./spec.md) for the full environment variable list. A2A-relevant flags:

- `--a2a-downstream-url <url>`: URL of a downstream A2A server to proxy execution to (default: none).
- `--public-base-url <url>`: Public base URL for L3 approval links (e.g., `https://demo.g8e.ai`). Used in `A2ASuspensionResponse` approval URLs.
- `--http-port <port>`: HTTP port for bootstrap, MCP, and A2A routes (flag default: 0, auto-resolved from `constants.Ports`; effective default: 8080).
- `--https-port <port>`: HTTPS port for mTLS API and public surface (flag default: 0, auto-resolved from `constants.Ports`; effective default: 8443).
- `--data-dir <dir>`: Data directory for SQLite database (default: `.g8e/data`).
- `--pki-dir <dir>`: Directory for TLS certificates (default: `.g8e/pki`).
- `--secrets-dir <dir>`: Directory for platform secrets (default: `.g8e/secrets`).

### Circuit Breaker

The Gateway implements a shared circuit breaker for downstream MCP and A2A servers. The A2A dispatch path (`DispatchToA2ADownstream`) uses the same circuit breaker state as MCP downstream dispatch:

- **Max failures**: 5 consecutive failures before opening circuit (default, set in `NewGatewayService`)
- **Cooldown**: 1 minute before attempting recovery (half-open state)
- **Behavior**: Rejects requests with `ErrGatewayDownstreamUnavailable` when circuit is open
- **Failure recording**: HTTP connection failures and 5xx responses from the downstream server increment the failure count; successful requests reset it

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
| Gateway entry | `cmd/g8e/main.go` |
| Gateway service | `internal/services/gateway/gateway_service.go` |
| HTTP routing | `internal/services/gateway/gateway_http_router.go` |
| A2A REST handler | `internal/services/mcp/gateway.go` (`HandleA2aCall`) |
| Unified MCP/A2A dispatcher | `internal/services/mcp/mcp_endpoint.go` (`HandleMCP`) |
| A2A call logic | `internal/services/mcp/gateway.go` (`a2aCall`) |
| Downstream dispatch | `internal/services/mcp/gateway.go` (`DispatchToA2ADownstream`) |
| Actuator egress | `internal/services/pubsub/pubsub_commands.go` (`handleA2aCallRequestSync`) |
| Response models | `internal/services/mcp/models.go` |
| Envelope construction | `internal/services/mcp/gateway.go` (`processGatewayTransaction`) |
| L1 doctrine validation | `internal/services/governance/l1_doctrine.go` |
| Transaction verification | `internal/services/governance/l4_warden.go` |
| Envelope processor interface | `internal/services/governance/types.go` (`EnvelopeProcessor`) |
| Envelope processor impl | `internal/services/pubsub/pubsub_commands.go` (`OperatorPubSubService.ProcessEnvelope`) |
| Session management | `internal/services/gateway/cli_session_service.go`, `web_session_service.go`, `operator_session_service.go` |
| Action type constant | `internal/constants/action_types.go` |
| API path constant | `internal/constants/api_paths.go` |
| Error codes | `internal/constants/rpc_errors.go` |
| Receipt summary limit | `internal/constants/pubsub.go` (`ReceiptSummaryMaxBytes`) |
| Gateway configuration | `internal/config/config.go` (`GatewayConfig.A2ADownstreamURL`) |
| Event type registry | `protocol/constants/events.json` |
| Proto schema | `protocol/proto/g8e/operator/v1/operator.proto` |

---

## Reference Artifacts

The following reference artifacts reside alongside this document in `protocol/docs/`:

- **`a2a.proto`**: The upstream A2A protocol protobuf definition (package `lf.a2a.v1`). This is the canonical schema for the Agent-to-Agent protocol surface, including `SendMessage`, `SendStreamingMessage`, `GetTask`, `ListTasks`, `CancelTask`, `SubscribeToTask`, push notification configuration, and `AgentCard` discovery. The g8e Gateway does not implement the full upstream A2A service surface; it integrates A2A skill invocations via the `A2A_CALL` action type and the `/api/v1/a2a/call` endpoint.
- **`a2a.json`**: A non-normative JSON Schema bundle extracted from the proto definitions. This artifact provides schema validation for JSON-based tooling and is not used at runtime by the gateway.

---

## Related Documentation

- [**g8e Protocol**](./spec.md) - The wire contract and governance hierarchy.
- [**Operator (g8eo)**](../../docs/architecture/operator.md) - Operator architecture and gateway mode.
- [**MCP Protocol**](./mcp.md) - MCP protocol specification and integration.
