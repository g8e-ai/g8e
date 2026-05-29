---
title: MCP Protocol
---

# MCP Protocol

Last Updated: 2026-05-25

The g8e Operator in gateway mode supports Model Context Protocol (MCP) integration. MCP clients send JSON-RPC tool calls to the gateway, which wraps them in the g8e governance envelope, runs them through the 3-layer BFT verification sequence (L1Doctrine/L2Consensus/L3Notary), and dispatches verified payloads to downstream MCP servers or to the in-process execution service for local execution.

---

## Protocol Overview

MCP is a JSON-RPC 2.0 protocol for AI tool invocation. MCP clients connect to the gateway via standard JSON-RPC endpoints with structured tool invocation payloads.

### Request Structure

MCP requests follow a JSON-RPC 2.0 pattern:

- **Transport**: JSON-RPC 2.0 over HTTP
- **Authentication**: mTLS certificate for MCP ingress routes
- **Payload**: JSON-RPC request with method, params, and id

### Gateway Integration

The gateway translates MCP tool invocations into governance envelopes:

1. **Inbound**: MCP client sends JSON-RPC tool invocation to gateway
2. **Envelope Construction**: Gateway wraps payload in `GovernanceEnvelope` with action type `MCP_CALL`, `MCP_RESOURCE_READ`, `MCP_RESOURCE_LIST`, `MCP_PROMPT_GET`, or `MCP_PROMPT_LIST`
3. **Verification**: Envelope passes through L1/L2/L3 verification gates
4. **Dispatch**: Verified envelope forwarded to downstream MCP server or local execution

### Local Tool Execution

The gateway handles certain tools locally without downstream proxy:

- **read_field**: JIT field resolution from governed collections with L1 field path validation, L3 session validation, and audit vault logging. Requires `collection`, `document_id`, `field_path`, and `operator_session_id` parameters.

---

## MCP Payload Types

### MCP_CALL

The gateway maps MCP tool invocations to the `MCP_CALL` action type with the `McpCallRequested` proto payload:

| Field | Type | Description |
|---|---|---|
| `tool_name` | string | Name of the tool to invoke (with L1 forbidden patterns) |
| `arguments_json` | string | JSON-encoded arguments object exactly as client supplied |
| `execution_id` | string | Optional client-supplied invocation identifier |

### MCP_RESOURCE_READ

Resource read operations map to `MCP_RESOURCE_READ` with `McpResourceReadRequested`:

| Field | Type | Description |
|---|---|---|
| `uri` | string | Resource URI to read (e.g., "file:///path", "memory://var") |
| `execution_id` | string | Optional client-supplied invocation identifier |

### MCP_PROMPT_GET

Prompt template retrieval maps to `MCP_PROMPT_GET` with `McpPromptGetRequested`:

| Field | Type | Description |
|---|---|---|
| `name` | string | Prompt name/template identifier |
| `execution_id` | string | Optional client-supplied invocation identifier |

### MCP_RESOURCE_LIST

Resource listing maps to `MCP_RESOURCE_LIST` with `McpResourceListRequested`:

| Field | Type | Description |
|---|---|---|
| `execution_id` | string | Optional client-supplied invocation identifier |

### MCP_PROMPT_LIST

Prompt listing maps to `MCP_PROMPT_LIST` with `McpPromptListRequested`:

| Field | Type | Description |
|---|---|---|
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

### MCP Client Connection

MCP clients connect to the gateway via:

- **JSON-RPC 2.0**: Standard JSON-RPC over HTTP to gateway endpoints
- **Authentication**: mTLS certificate (RequireAndVerifyClientCert) for MCP ingress routes

### Tool Invocation

Invoke MCP tools via JSON-RPC POST to `/api/mcp/v1/tools/call` or `/api/mcp/v1/tools/call/sse` for streaming:

```json
{
  "jsonrpc": "2.0",
  "method": "tools/call",
  "params": {
    "name": "example_tool",
    "arguments": {
      "param1": "value1",
      "param2": "value2"
    }
  },
  "id": 1
}
```

### Tool Discovery

The gateway proxies tool discovery to the configured downstream MCP server:

- `tools/list`: Returns available tools with schemas
- `prompts/list`: Returns available prompt templates
- `resources/list`: Returns available resources

Discovery endpoints are proxied verbatim from the downstream server. Tool calls are wrapped in GovernanceEnvelope before verification.

### Tool Schema

Tools are defined by the downstream MCP server with schemas that include:

- **name**: Tool identifier
- **description**: Human-readable description
- **inputSchema**: JSON Schema for input validation

The Gateway applies L1 forbidden pattern checks to tool names before forwarding requests.

---

## Security and Verification

### L1 Doctrine (Hard Gates)

- **Forbidden patterns**: Protobuf field options with regex constraints on tool names and URIs
- **Runtime scanning**: Runtime scanning of field values for forbidden patterns (sudo, password, api_key, etc.)
- **Field path validation**: Allowlist/denylist enforcement for `read_field` tool via `FieldPathRegistry`

### L2 Consensus

- **Ed25519 signatures**: Consensus agents sign envelopes with their private keys
- **Signer verification**: Gateway verifies signatures against trusted signers in SQLite store
- **Gateway signing**: In doctrine mode, Gateway signs envelopes locally (single-agent consensus) with `gateway_signed=true` flag
- **Reputation staking**: Signers stake reputation on their decisions

### L3 Notary (Authorization)

- **mTLS fingerprint**: CLI sessions use mTLS certificate fingerprint as proof via CLIL3Notary
- **WebAuthn**: Browser-based clients authenticate with passkeys via PasskeyService
- **Composite verifier**: CompositeL3Verifier handles both web and CLI session types
- **Auto-approval**: Benign diagnostic commands may skip human prompt after L1/L2 pass

---

## Error Handling

MCP protocol errors follow JSON-RPC 2.0 conventions:

- **Parse error**: `-32700` (Parse error)
- **Invalid request**: `-32600` (Invalid Request)
- **Method not found**: `-32601` (Method not found)
- **Invalid params**: `-32602` (Invalid params)
- **Internal error**: `-32603` (Internal error)

### Gateway-Specific Error Codes

The gateway uses reserved error codes in the `-32000` to `-32099` range for governance verification failures:

- **Invalid envelope**: `-32000` (Invalid envelope structure or missing fields)
- **Hash mismatch**: `-32001` (Transaction hash mismatch)
- **Expired**: `-32002` (Transaction expired)
- **Replay**: `-32003` (Transaction replay detected)
- **State mismatch**: `-32004` (State root mismatch)
- **L1 validation failed**: `-32005` (L1 forbidden pattern violation)
- **L2 signature invalid**: `-32006` (L2 signature verification failed)
- **L3 proof invalid**: `-32007` (L3 WebAuthn proof verification failed)
- **Payload decode failed**: `-32008` (Failed to decode protobuf payload)
- **Resource not found**: `-32100` (Requested resource not found)
- **Gateway not ready**: `-32101` (Gateway not ready to process requests)

### Error Mapping

Gateway verification errors are mapped to JSON-RPC error responses:

- **Invalid envelope**: `-32000` (Invalid envelope structure)
- **L1 rejection**: `-32005` (L1 forbidden pattern violation)
- **L2 rejection**: `-32006` (L2 signature verification failed)
- **L3 rejection**: `-32007` (L3 proof verification failed)
- **Downstream unavailable**: `-32603` (Internal error, circuit breaker)

### L3 Suspension

When L3 proof is missing (`ErrL3ProofMissing`), the gateway suspends the transaction and returns an approval URL for out-of-band WebAuthn authorization. The client must retry after approval.

---

## Configuration

### Gateway Modes

The Operator runs in gateway mode with three posture options:

| Mode | Flag | Purpose |
|---|---|---|
| **Doctrine** | `--doctrine` | L1 enforced, L2/L3 audited (default) |
| **Consensus** | `--consensus` | L1/L2 enforced, L3 audited |
| **Notary** | `--notary` | L1/L2/L3 strictly enforced |

### Port Configuration

Default ports (configurable via flags or paths.json):

| Port | Purpose | Auth |
|---|---|---|
| `8441` | Bootstrap enrollment | TLS (no client cert) |
| `8443` | Public web session | TLS (no client cert) |
| `8440-8443`, `18789` | Operator mTLS API + Pub/Sub | mTLS (RequireAndVerifyClientCert) |

### Environment Variables

- `G8E_PKI_DIR`: PKI hierarchy directory (default: `.g8e/pki`)
- `G8E_DATA_DIR`: SQLite persistence directory (default: `.g8e/data`)
- `G8E_SECRETS_DIR`: Platform secrets directory (default: `.g8e/secrets`)

### Circuit Breaker

The Gateway implements a circuit breaker for downstream MCP servers:

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
| Gateway entry | `cmd/g8eo/main.go` (runGatewayMode) |
| Gateway service | `internal/services/gateway/gateway_service.go` |
| HTTP routing | `internal/services/gateway/gateway_http.go` |
| MCP/A2A translation | `internal/services/mcp/gateway.go` |
| MCP models | `internal/services/mcp/models.go` |
| Field path registry | `internal/services/mcp/field_parser.go` |
| Envelope construction | `internal/services/mcp/gateway.go` (processGatewayTransaction) |
| Transaction verification | `internal/services/governance/l4_warden.go` |
| Envelope processor | `internal/services/governance/processor.go` |
| Pub/Sub command service | `internal/services/pubsub/pubsub_commands.go` |
| Session management | `internal/services/gateway/session_service.go` |
| CLI L3 verification | `internal/services/gateway/cli_l3_notary.go` |
| Composite L3 verifier | `internal/services/gateway/composite_l3_verifier.go` |
| Passkey L3 brokerage | `internal/services/gateway/passkey_service.go` |
| Error mapping | `internal/services/mcp/gateway.go` (mapGatewayError) |
| Suspended transaction store | `internal/services/gateway/gateway_db_service.go` |

---

## Related Documentation

- [**g8e Protocol**](../../architecture/g8e.md) - The wire contract and governance hierarchy
- [**Operator (g8eo)**](../../architecture/operator.md) - Operator architecture and gateway mode
- [**A2A Protocol**](../a2a/a2a.md) - A2A protocol specification and integration
