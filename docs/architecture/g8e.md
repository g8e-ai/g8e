---
title: g8e Protocol
---

# g8e Protocol

Last Updated: 2026-05-31

The **g8e Protocol** is a zero-trust execution platform and compliance standard for agentic infrastructure. It defines the canonical `GovernanceEnvelope` that wraps all mutations passing through the g8e platform, enforcing fail-closed verification through the sequential 5-Layer interlock sequence. The platform uses `g8e.local` as the default internal hostname and canonical alias for all mesh communication.

---

## Protocol Overview

The g8e Protocol is the foundational wire contract for all mutations in the g8e platform. It provides a typed, signed, state-bound transaction envelope that binds identity, intent, state, and governance proofs into a single verifiable unit.

### Core Design Principles

- **Canonical JSON Wire Format**: All client-facing surfaces (HTTP, WSS pub/sub, receipts, audit exports) carry the `GovernanceEnvelope` as canonical JSON (protojson).Node Node Binary protobuf is strictly reserved for internal storage.
- **g8e.local Canonical Alias**: The platform uses `g8e.local` as the stable internal hostname. The gateway translates this alias to installation-specific peer identity and endpoint data.
- **Hash-Based Signing**: A deterministic `transaction_hash` is computed from normalized envelope fields. The verifier enforces `id == transaction_hash == SHA256(canonical_fields)`.
- **Fail-Closed Verification**: Any malformed envelope, expired transaction, reused nonce, stale state root, or missing proof is rejected immediately before execution.
- **Body-Embedded Context**: Business and execution context (`web_session_id`, `cli_session_id`, `operator_session_id`, `user_id`) lives inside the envelope via typed fields.
- **BFT State Binding**: Mutations carry a `state_merkle_root` that the Operator compares against its current host state.
- **Operator Sovereignty**: No bundled component has privileged channels. The g8e Operator is the only execution boundary, enforcing rules uniformly.

### Protocol Translation

The g8e Protocol does not compete with tool-calling standards. Instead, it wraps standard JSON-RPC tools (MCP, A2A, OpenAI tool calls, LangChain) as unverified payloads inside the strict `GovernanceEnvelope`:

1. **Inbound**: Client ecosystem generates typed payload (e.g., `CommandRequested`, `McpCallRequested`).
2. **Envelope Construction**: Payload embedded in `GovernanceEnvelope` with `nonce`, `expires_at`, `state_merkle_root`.
3. **Governance Proofs**: L2 Consensus signature and L3 Notary proof attached over `transaction_hash`.
4. **Verification**: Envelope passes through the 5-Layer interlock sequence (L1/L2/L3/L4).
5. **Execution**: Verified payload dispatched to L5Actuator for execution and receipt issuance.

---

## GovernanceEnvelope Schema

The `GovernanceEnvelope` is the single canonical container for all g8e mutations. The schema source of truth is defined in `../../protocol/proto/g8e/common/v1/common.proto`.

### Envelope Fields

| Field | Type | Description |
|---|---|---|
| `id` | string | Transaction identifier; must exactly match `transaction_hash` |
| `timestamp` | google.protobuf.Timestamp | Envelope creation time |
| `expires_at` | google.protobuf.Timestamp | UTC timestamp after which envelope is void |
| `source_component` | Component | Source component identifier (COMPONENT_AGENT, COMPONENT_G8EO, COMPONENT_CLIENT) |
| `operator_id` | string | Operator instance identifier |
| `operator_session_id` | string | Host-side agent session identifier |
| `web_session_id` | string | Browser frontend session identifier |
| `cli_session_id` | string | CLI/BYO client session identifier |
| `event_type` | string | Canonical event name from protocol/constants/events.json |
| `payload` | bytes | Raw protobuf payload containing execution instruction |
| `intent_data` | google.protobuf.Struct | Structured JSON-first view of intent |
| `action_type` | string | UAP-compatible action type (e.g., EXECUTE_BASH) |
| `target_resource` | string | UAP-compatible target resource |
| `state_merkle_root` | string | Expected host state root at time of signing |
| `nonce` | string | Unique replay-protection token |
| `transaction_hash` | string | SHA-256 over canonical envelope fields |
| `protocol_version` | string | UAP-compatible protocol version (e.g., "1.0") |
| `governance` | GovernanceMetadata | L1/L2/L3 governance proofs and status |
| `case_id` | string | Optional case identifier for application context |
| `investigation_id` | string | Optional investigation identifier |
| `task_id` | string | Optional task identifier |
| `system_fingerprint` | string | Optional system fingerprint |
| `tenant_id` | string | Optional tenant identifier |
| `binding_persona` | string | Optional binding persona |

### GovernanceMetadata

The `governance` field encapsulates all three governance layers:

| Field | Type | Description |
|---|---|---|
| `l1` | L1Metadata | L1 Doctrine status (validated flag, violations list) |
| `l2` | L2Metadata | L2 Consensus signature, agent IDs, and key ID |
| `l3` | L3Metadata | L3 Notary proof and auto-approval flag |
| `gateway_signed` | bool | True if signed by local gateway without full L2 consensus |

### Canonical JSON Wire Format

All envelopes use canonical JSON (protojson) encoding for client-facing surfaces:

- **Schema source of truth**: `.proto` files in `../../protocol/proto/`
- **Wire format**: canonical JSON (protojson)
- **Signing basis**: deterministic `transaction_hash` computed from normalized envelope fields
- **Internal storage**: protobuf bytes (implementation detail)

This ensures compatibility with JSON-based ecosystems while maintaining typed schema validation.

---

## Transaction Lifecycle

The transaction lifecycle follows a strict sequence from intent to audited execution.

### Request Phase (Client -> Gateway -> Operator)

1. A client ecosystem generates a typed protobuf payload (e.g., `CommandRequested`).
2. The gateway detects the machine's network identity (IPs, hostnames, and aliases) using the detector in `../../internal/services/network/identity.go`.
3. The payload is embedded into a `GovernanceEnvelope` alongside `nonce`, `expires_at`, and `state_merkle_root`.
4. An L2 Consensus producer computes the `transaction_hash` and attaches a signature.
5. For mutations, an L3 Notary (human) signs the same hash via WebAuthn, unless auto-approval policy applies.
6. The client submits the canonical-JSON envelope over mTLS to the g8e Gateway, which validates and dispatches it to the target g8e Operator over WSS. Remote peers are resolved via `g8e.local` translation.

### Verification Phase (L4Warden)

The `L4Warden` operates as the primary pre-dispatch validation gate, executing the following checks sequentially:

1. **Integrity**: Enforces `id == transaction_hash == SHA256(canonical_fields)`.
2. **Freshness**: Verifies `expires_at` and ensures the `nonce` is not in the replay store.
3. **State Binding**: Validates that the `state_merkle_root` matches the local ledger root.
4. **L1 Doctrine**: Scans the decoded typed payload against reflected `forbidden_patterns` and threat rules.
5. **L2 Consensus**: Verifies the Ed25519 signature against the Operator's trusted `SignerStore`.
6. **L3 Notary**: Validates the WebAuthn proof or applies explicit auto-approval policy for the action.

### Execution & Receipt Phase (L5Actuator)

1. The `L5Actuator` signs an executing-state `ActionReceipt` and writes it to the fail-closed `AuditVault`.
2. The typed payload is dispatched to its execution handler (e.g., shell executor, file edit handler).
3. The `L5Actuator` updates the receipt with the final status (`COMPLETED` or `FAILED`), the post-state root, and a fresh signature.
4. The Operator publishes a result envelope carrying the typed result and signed receipt back to the Gateway.

---

## 5-Layer interlock sequence

Every mutation must pass through five independent layers in order. A failure at any layer is an immediate rejection.

### L1 Doctrine: Technical Bedrock
Static, deterministic checks enforced before any code executes. Validated using doctrines sourced from `../../protocol/constants/doctrine/doctrine_registry.json`. Code pattern matching and threat analysis are defined in `../../internal/services/governance/l1_doctrine.go`.
- **Forbidden Patterns**: The custom protobuf field option `(g8e.common.v1.forbidden_patterns)` is reflected at runtime to scan typed payloads.
- **Threat Detection**: L1 Doctrine analyzes command inputs for MITRE ATT&CK patterns, reverse shells, and injection vectors.
- **Allow/Deny Lists**: Enforces per-host policy and user settings.

### L2 Consensus: Distributed Agreement
A cryptographic proof that an independent ensemble agreed on the instruction. Signature verification using Ed25519 cryptography is defined in `../../internal/services/governance/l2_consensus.go`.
- An Ed25519 signature is generated over `transaction_hash | decision`.
- Verified against the Operator-owned `SignerStore`.
- Gateway mode may sign locally (`gateway_signed=true`) for single-agent MCP clients.

### L3 Notary: Human Authorization
Hardware-bound proof of human presence. Human-in-the-loop authorization (utilizing WebAuthn or cryptographically signed CLI proofs) is defined in `../../internal/services/governance/l3_notary.go`.
- **Web Sessions**: Real WebAuthn/FIDO2 proof with the transaction hash as the assertion challenge.
- **CLI Sessions**: Authenticates via mTLS certificates with SPIFFE URI SANs. The L3 proof includes the SHA-256 fingerprint of the mTLS certificate and an optional `cli_signature` over the `transaction_hash`.
- **Auto-Approval**: Explicit policy permits auto-approval for benign diagnostic verbs. Auto-approval never bypasses L1 or L2 gates.

### L4 Warden: Pre-dispatch Verification
The central Policy Execution Point (PEP) that validates the entire transaction proof before dispatch. Pre-dispatch verification gating (validating signatures, replay prevention, expiry, nonces, and state Merkle root) is defined in `../../internal/services/governance/l4_warden.go`.
- **Stateless Validation**: Verifies structural integrity, payload decoding, and L1Doctrine compliance.
- **Cryptographic Integrity**: Validates `transaction_hash` and signatures.
- **Freshness & Replay**: Verifies `expires_at` and the `nonce` replay store.
- **State Binding**: Compares `state_merkle_root` against the host ledger.

### L5 Actuator: Execution Boundary
The single fail-closed execution target that dispatches the verified payload and issues signed receipts. Isolated boundary tool dispatch (via MCP/A2A) and signed receipt production are defined in `../../internal/services/governance/l5_actuator.go`.
- **Rehydration**: Sensitive tokens scrubbed by the Sovereignty Boundary Plane are re-injected.
- **Native Dispatch**: Executes the typed payload (bash, file edit, tool call).
- **Signed Action Receipts**: Issues an immutable `ActionReceipt` proof of execution and result.

---

## Event Types

The protocol defines canonical event types in protocol/constants/events.json. Events are categorized by domain:

### AI Agent Events
- `AiAgentConflictDetected`, `AiAgentConflictResolved`
- `AiAgentContinueApprovalRequested`, `AiAgentContinueApprovalGranted`, `AiAgentContinueApprovalRejected`
- `AiLLMChatIterationStarted`, `AiLLMChatIterationCompleted`, `AiLLMChatIterationFailed`
- `AiLLMChatIterationStreamStarted`, `AiLLMChatIterationStreamDeltaReceived`, `AiLLMChatIterationStreamCompleted`
- `AiLLMChatIterationStreamFailed`, `AiLLMChatIterationTextChunkReceived`, `AiLLMChatIterationTextCompleted`

### Command Execution Events
- `OperatorCommandRequested`, `OperatorCommandStarted`, `OperatorCommandCompleted`, `OperatorCommandFailed`
- `OperatorCommandOutputReceived`, `OperatorCommandResult`

### File System Events
- `OperatorFilesystemReadRequested`, `OperatorFilesystemReadCompleted`, `OperatorFilesystemReadFailed`
- `OperatorFileEditRequested`, `OperatorFileEditCompleted`, `OperatorFileEditFailed`
- `OperatorFileHistoryFetchRequested`, `OperatorFileDiffFetchRequested`, `OperatorFileRestoreRequested`

### Audit & Governance Events
- `OperatorAuditCommandRecorded`, `OperatorAuditUserRecorded`
- `OperatorBootstrapRequested`, `OperatorBootstrapCompleted`, `OperatorBootstrapFailed`

### MCP/A2A Events
- `OperatorMcpCallRequested`
- `OperatorA2aCallRequested`

---

## Session Management

The protocol enforces strict separation between session types to guarantee context integrity.

| Session | Identifier | Use | Auth |
|---|---|---|---|
| **Operator** | `operator_session_id` | Host-side agent | mTLS (operator cert, URI SAN) |
| **CLI** | `cli_session_id` | BYO/CLI client | mTLS (CLI cert, URI SAN) |
| **Web** | `web_session_id` | Browser frontend | Passkey (WebAuthn) |

Sessions are cryptographically bound to their authentication mechanism and cannot be conflated. SSE and pub/sub routing uses these identifiers to prevent cross-tenant data leakage.

---

## Error Handling

Protocol errors follow standardized JSON-RPC codes for MCP/A2A client compatibility. Codes are defined in `../../internal/cli/errors/errors.go`.

| Code | Label | Meaning |
|---|---|---|
| `-32000` | `ErrCodeInvalidEnvelope` | Malformed JSON, missing ID, or unknown action type. |
| `-32001` | `ErrCodeHashMismatch` | `transaction_hash` is missing or does not match computed SHA-256. |
| `-32002` | `ErrCodeExpired` | `expires_at` timestamp has passed. |
| `-32003` | `ErrCodeReplay` | `nonce` has already been used within the expiry window. |
| `-32004` | `ErrCodeStateMismatch` | `state_merkle_root` does not match the current host state. |
| `-32005` | `ErrCodeL1ValidationFailed`| Payload violates L1 Doctrine forbidden patterns. |
| `-32006` | `ErrCodeL2SignatureInvalid`| L2 Consensus signature is missing, invalid, or from an untrusted key. |
| `-32007` | `ErrCodeL3ProofInvalid` | L3 Notary proof is missing or failed verification. |
| `-32008` | `ErrCodePayloadDecodeFailed`| Failed to decode the base64 `payload` into its typed protobuf message. |
| `-32100` | `ErrCodeResourceNotFound` | Requested resource (e.g., file, session) not found. |
| `-32101` | `ErrCodeGatewayNotReady` | Gateway is still bootstrapping or in an error state. |

---

## Configuration

### Gateway Postures

The g8e Gateway runs with three posture options:

| Mode | Flag | Purpose |
|---|---|---|
| **Doctrine** | `--posture doctrine` | L1 enforced, L2/L3 audited (default) |
| **Consensus** | `--posture consensus` | L1/L2 enforced, L3 audited |
| **Notary** | `--posture notary` | L1/L2/L3 strictly enforced |

### Port Configuration

Default ports (configurable via flags or `../../internal/cli/config/paths.json`):

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

---

## Host Sovereignty & Data Audit

### Multi-Ledger Architecture

Each Operator session owns an isolated, encrypted git repository tracking all mutations with `LedgerHashBefore` and `LedgerHashAfter`. Every file mutation triggers a native Go `go-git` commit.

### Fail-Closed Audit Vault

The SQLite-backed `AuditVaultService` mandates valid session identifiers and rejects malformed events. If audit logging fails, execution is aborted.

### Sovereignty Boundary

Output scrubbing is performed directly at the `L5Actuator` boundary to redact tokens, keys, and PII before any data leaves the host.

---

## Implementation Reference

| Concern | File |
|---|---|
| Protobuf schemas | `../../protocol/proto/g8e/common/v1/common.proto` |
| Event registry | `../../protocol/constants/events.json` |
| Channel prefixes | `../../protocol/constants/channels.json` |
| Envelope types | `../../pkg/governance/types.go` |
| Warden logic | `../../internal/services/governance/l4_warden.go` |
| Actuator logic | `../../internal/services/governance/l5_actuator.go` |
| Audit storage | `../../internal/services/storage/audit_vault.go` |
| Ledger storage | `../../internal/services/storage/ledger.go` |
| Workload identity | `../../protocol/workload_identity.go` |
| Network identity | `../../internal/services/network/identity.go` |
| Gateway envelope construction | `../../internal/services/gateway/governance_envelope.go` |
| Gateway HTTP routing | `../../internal/services/gateway/gateway_http.go` |
| Pub/Sub command service | `../../internal/services/pubsub/pubsub_commands.go` |
| Pub/Sub results service | `../../internal/services/pubsub/pubsub_results.go` |
| MCP/A2A translation | `../../internal/services/mcp/gateway.go` |
| Session management | `../../internal/services/gateway/session_service.go` |
| CLI L3 verification | `../../internal/services/gateway/cli_l3_notary.go` |
| Composite L3 verifier | `../../internal/services/gateway/composite_l3_verifier.go` |
| Doctrine registry | `../../protocol/constants/doctrine/doctrine_registry.json` |
| MCP vectors doctrine | `../../protocol/constants/doctrine/mcp_vectors_doctrine.json` |
| Internal translation | `../../docs/architecture/g8e_local_translation.md` |

---

## Example GovernanceEnvelope with MCP Payloads

### Example 1: MCP File Read Tool Call

```json
{
  "id": "a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2",
  "timestamp": "2026-06-02T18:27:00Z",
  "expiresAt": "2026-06-02T18:32:00Z",
  "sourceComponent": "COMPONENT_CLIENT",
  "operatorId": "op-prod-12345",
  "operatorSessionId": "sess-abc-789",
  "webSessionId": "web-xyz-456",
  "eventType": "g8e.v1.operator.mcp.call.requested",
  "payload": "CgZmc19yZWFkEglleGVjLTIwMzUSCgoZmlsZTovLy9ob21lL3VzZXIvcmVhZG1lLm1kGgZzY3J1Yg==",
  "intentData": {
    "tool": "fs_read",
    "path": "/home/user/readme.md",
    "reason": "Read deployment documentation"
  },
  "actionType": "MCP_CALL",
  "targetResource": "file:///home/user/readme.md",
  "stateMerkleRoot": "abc123def456abc123def456abc123def456abc123def456abc123def456abc1",
  "nonce": "nonce-1717358820000-abc123",
  "transactionHash": "a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2",
  "protocolVersion": "1.0",
  "governance": {
    "l1": {
      "validated": true,
      "violations": []
    },
    "l2": {
      "consensusSignature": "4a5b6c7d8e9f0a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2...c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2",
      "agentIds": ["agent-ensemble-1", "agent-ensemble-2", "agent-ensemble-3"],
      "keyId": "key-ensemble-prod-abc123"
    },
    "l3": {
      "proof": {
        "clientDataJson": "{\"challenge\":\"a1b2c3d4e5f6\",\"origin\":\"https://g8e.ai\",\"type\":\"webauthn.get\"}",
        "authenticatorData": "SZYN5YgOjGh0NBcPZHZgW4_krrmihjLHmVzzuoMdl2NFAAAAAQ",
        "signature": "MEUCIQDWn3x4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2IgE5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2",
        "credentialId": "abc123def456abc123def456abc123def456abc123def456abc123def456abc1"
      },
      "autoApproved": false
    },
    "gatewaySigned": false
  },
  "caseId": "case-deploy-456",
  "taskId": "task-readme-789",
  "systemFingerprint": "fp-linux-amd64-abc123",
  "tenantId": "tenant-prod-xyz"
}
```

The `payload` field contains base64-encoded protobuf bytes of `McpCallRequested` with:
- `tool_name`: "fs_read"
- `arguments_json`: "{\"path\":\"/home/user/readme.md\"}"
- `execution_id`: "exec-2035"

### Example 2: MCP Database Query Tool Call (Auto-Approved)

```json
{
  "id": "b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3",
  "timestamp": "2026-06-02T18:28:00Z",
  "expiresAt": "2026-06-02T18:33:00Z",
  "sourceComponent": "COMPONENT_AGENT",
  "operatorId": "op-prod-67890",
  "operatorSessionId": "sess-def-012",
  "eventType": "g8e.v1.operator.mcp.call.requested",
  "payload": "CgZxdWVyeRIXZXhlYy1idWlsZC0zNDU2EgoJCXNFTEVDVCBjb3VudCgqKSBGUk9NIHVzZXJzGgZzY3J1Yg==",
  "intentData": {
    "tool": "postgres_query",
    "query": "SELECT count(*) FROM users",
    "reason": "Check user count for health check"
  },
  "actionType": "MCP_CALL",
  "targetResource": "postgres://prod-db.internal/users",
  "stateMerkleRoot": "def456abc123def456abc123def456abc123def456abc123def456abc123def4",
  "nonce": "nonce-1717358880000-def456",
  "transactionHash": "b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3",
  "protocolVersion": "1.0",
  "governance": {
    "l1": {
      "validated": true,
      "violations": []
    },
    "l2": {
      "consensusSignature": "5c6d7e8f0a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2...d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3",
      "agentIds": ["agent-ensemble-1"],
      "keyId": "key-ensemble-prod-def456"
    },
    "l3": {
      "proof": null,
      "autoApproved": true
    },
    "gatewaySigned": true
  },
  "caseId": "",
  "taskId": "task-health-345",
  "systemFingerprint": "fp-linux-amd64-def456",
  "tenantId": "tenant-prod-xyz"
}
```

This example shows a benign diagnostic query with `autoApproved: true` and `gatewaySigned: true`, bypassing the L3 human authorization while still passing L1 and L2 checks.

### Example 3: MCP Resource List Call

```json
{
  "id": "c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4",
  "timestamp": "2026-06-02T18:29:00Z",
  "expiresAt": "2026-06-02T18:34:00Z",
  "sourceComponent": "COMPONENT_CLIENT",
  "operatorId": "op-prod-11111",
  "operatorSessionId": "sess-ghi-345",
  "webSessionId": "web-jkl-678",
  "eventType": "g8e.v1.operator.mcp.resources.list.requested",
  "payload": "CgZleGVjLTQ1NjcG",
  "intentData": {
    "operation": "list_resources",
    "reason": "Discover available MCP resources"
  },
  "actionType": "MCP_RESOURCE_LIST",
  "targetResource": "*",
  "stateMerkleRoot": "efg789abc123efg789abc123efg789abc123efg789abc123efg789abc123efg7",
  "nonce": "nonce-1717358940000-efg789",
  "transactionHash": "c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4",
  "protocolVersion": "1.0",
  "governance": {
    "l1": {
      "validated": true,
      "violations": []
    },
    "l2": {
      "consensusSignature": "6d7e8f0a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2...e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4",
      "agentIds": ["agent-ensemble-1", "agent-ensemble-2"],
      "keyId": "key-ensemble-prod-efg789"
    },
    "l3": {
      "proof": {
        "clientDataJson": "{\"challenge\":\"c3d4e5f6\",\"origin\":\"https://g8e.ai\",\"type\":\"webauthn.get\"}",
        "authenticatorData": "SZYN5YgOjGh0NBcPZHZgW4_krrmihjLHmVzzuoMdl2NFAAAAAQ",
        "signature": "MEYCIQCd4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2IhAOf6a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2",
        "credentialId": "def456abc123def456abc123def456abc123def456abc123def456abc123def4"
      },
      "autoApproved": false
    },
    "gatewaySigned": false
  },
  "caseId": "case-discovery-789",
  "taskId": "task-resources-456",
  "systemFingerprint": "fp-linux-amd64-efg789",
  "tenantId": "tenant-prod-xyz"
}
```

This example demonstrates a resource discovery operation using the `McpResourceListRequested` payload type.

---

## Related Documentation

- [**g8e Operator**](./operator.md) - Operator architecture and execution boundary
- [**g8e Gateway**](./gateway.md) - Gateway architecture
- [**MCP Protocol**](../protocols/mcp/mcp.md) - MCP protocol specification and integration
- [**A2A Protocol**](../protocols/a2a/a2a.md) - A2A protocol specification and integration
