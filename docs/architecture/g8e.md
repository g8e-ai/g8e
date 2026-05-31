---
title: g8e Protocol
---

# g8e Protocol

Last Updated: 2026-05-29

The **g8e Protocol** is a zero-trust execution platform and compliance standard for agentic infrastructure. It defines the canonical `GovernanceEnvelope` that wraps all mutations passing through the g8e platform, enforcing fail-closed verification through the sequential 5-Layer interlock sequence.

---

## Protocol Overview

The g8e Protocol is the foundational wire contract for all mutations in the g8e platform. It provides a typed, signed, state-bound transaction envelope that binds identity, intent, state, and governance proofs into a single verifiable unit.

### Core Design Principles

- **Canonical JSON Wire Format**: All client-facing surfaces (HTTP, WSS pub/sub, receipts, audit exports) carry the `GovernanceEnvelope` as canonical JSON (protojson). Binary protobuf is strictly reserved for internal storage.
- **Hash-Based Signing**: A deterministic `transaction_hash` is computed from normalized envelope fields. The verifier enforces `id == transaction_hash == SHA256(canonical_fields)`.
- **Fail-Closed Verification**: Any malformed envelope, expired transaction, reused nonce, stale state root, or missing proof is rejected immediately before execution.
- **Body-Embedded Context**: Business and execution context (`web_session_id`, `cli_session_id`, `operator_session_id`, `user_id`) lives inside the envelope via typed fields.
- **BFT State Binding**: Mutations carry a `state_merkle_root` that the Operator compares against its current host state.
- **Operator Sovereignty**: No bundled component has privileged channels. The Operator (`g8eo`) is the only execution boundary, enforcing rules uniformly.

### Protocol Translation

The g8e Protocol does not compete with tool-calling standards. Instead, it wraps standard JSON-RPC tools (MCP, A2A, OpenAI tool calls, LangChain) as unverified payloads inside the strict `GovernanceEnvelope`:

1. **Inbound**: Client ecosystem generates typed payload (e.g., `CommandRequested`, `McpCallRequested`).
2. **Envelope Construction**: Payload embedded in `GovernanceEnvelope` with `nonce`, `expires_at`, `state_merkle_root`.
3. **Governance Proofs**: L2 Consensus signature and L3 Notary proof attached over `transaction_hash`.
4. **Verification**: Envelope passes through the 5-Layer interlock sequence (L1/L2/L3/L4).
5. **Execution**: Verified payload dispatched to L5Actuator for execution and receipt issuance.

---

## GovernanceEnvelope Schema

The `GovernanceEnvelope` is the single canonical container for all g8e mutations. The schema source of truth is protocol/proto/g8e/common/v1/common.proto.

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

- **Schema source of truth**: `.proto` files in protocol/proto/
- **Wire format**: canonical JSON (protojson)
- **Signing basis**: deterministic `transaction_hash` computed from normalized envelope fields
- **Internal storage**: protobuf bytes (implementation detail)

This ensures compatibility with JSON-based ecosystems while maintaining typed schema validation.

---

## Transaction Lifecycle

The transaction lifecycle follows a strict sequence from intent to audited execution.

### Request Phase (Client -> Gateway -> Operator)

1. A client ecosystem generates a typed protobuf payload (e.g., `CommandRequested`).
2. The payload is embedded into a `GovernanceEnvelope` alongside `nonce`, `expires_at`, and `state_merkle_root`.
3. An L2 Consensus producer computes the `transaction_hash` and attaches a signature.
4. For mutations, an L3 Notary (human) signs the same hash via WebAuthn, unless auto-approval policy applies.
5. The client submits the canonical-JSON envelope over mTLS to the Governance Gateway (`g8eg`), which validates and dispatches it to the target Operator (`g8eo`) over WSS.

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
Static, deterministic checks enforced before any code executes. Validated using doctrines sourced from protocol/constants/doctrine/doctrine_registry.json. Code pattern matching and threat analysis are defined in internal/services/governance/l1_doctrine.go.
- **Forbidden Patterns**: The custom protobuf field option `(g8e.common.v1.forbidden_patterns)` is reflected at runtime to scan typed payloads.
- **Threat Detection**: L1 Doctrine analyzes command inputs for MITRE ATT&CK patterns, reverse shells, and injection vectors.
- **Allow/Deny Lists**: Enforces per-host policy and user settings.

### L2 Consensus: Distributed Agreement
A cryptographic proof that an independent ensemble agreed on the instruction. Signature verification using Ed25519 cryptography is defined in internal/services/governance/l2_consensus.go.
- An Ed25519 signature is generated over `transaction_hash | decision`.
- Verified against the Operator-owned `SignerStore`.
- Gateway mode may sign locally (`gateway_signed=true`) for single-agent MCP clients.

### L3 Notary: Human Authorization
Hardware-bound proof of human presence. Human-in-the-loop authorization (utilizing WebAuthn or cryptographically signed CLI proofs) is defined in internal/services/governance/l3_notary.go.
- **Web Sessions**: Real WebAuthn/FIDO2 proof with the transaction hash as the assertion challenge.
- **CLI Sessions**: Authenticates via mTLS certificates with SPIFFE URI SANs. The L3 proof is the SHA-256 fingerprint of the mTLS certificate.
- **Auto-Approval**: Explicit policy permits auto-approval for benign diagnostic verbs. Auto-approval never bypasses L1 or L2 gates.

### L4 Warden: Pre-dispatch Verification
The central Policy Execution Point (PEP) that validates the entire transaction proof before dispatch. Pre-dispatch verification gating (validating signatures, replay prevention, expiry, nonces, and state Merkle root) is defined in internal/services/governance/l4_warden.go.
- **Stateless Validation**: Verifies structural integrity, payload decoding, and L1Doctrine compliance.
- **Cryptographic Integrity**: Validates `transaction_hash` and signatures.
- **Freshness & Replay**: Verifies `expires_at` and the `nonce` replay store.
- **State Binding**: Compares `state_merkle_root` against the host ledger.

### L5 Actuator: Execution Boundary
The single fail-closed execution target that dispatches the verified payload and issues signed receipts. Isolated boundary tool dispatch (via MCP/A2A) and signed receipt production are defined in internal/services/governance/l5_actuator.go.
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

### Command Execution Events
- `CommandRequested`, `CommandStarted`, `CommandCompleted`, `CommandFailed`
- `CommandOutputReceived`, `CommandErrorReceived`

### File System Events
- `FileReadRequested`, `FileReadCompleted`, `FileReadFailed`
- `FileWriteRequested`, `FileWriteCompleted`, `FileWriteFailed`
- `FileHistoryRequested`, `FileDiffRequested`, `FileRestoreRequested`

### Audit & Governance Events
- `AuditEventRecorded`, `AuditQueryRequested`
- `GovernanceEnvelopeReceived`, `GovernanceEnvelopeVerified`, `GovernanceEnvelopeRejected`

### MCP/A2A Events
- `McpCallRequested`, `McpCallCompleted`, `McpCallFailed`
- `McpResourceReadRequested`, `McpResourceReadCompleted`
- `A2aCallRequested`, `A2aCallCompleted`, `A2aCallFailed`

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

Protocol errors follow standardized JSON-RPC codes for MCP/A2A client compatibility:

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

### Gateway Modes

The Operator runs in gateway mode with three posture options:

| Mode | Flag | Purpose |
|---|---|---|
| **Doctrine** | `--doctrine` | L1 enforced, L2/L3 audited (default) |
| **Consensus** | `--consensus` | L1/L2 enforced, L3 audited |
| **Notary** | `--notary` | L1/L2/L3 strictly enforced |

### Port Configuration

Default ports (configurable via flags or internal/cli/config/paths.json):

| Port | Purpose | Auth |
|---|---|---|
| `8440` | mTLS API + Pub/Sub | mTLS (RequireAndVerifyClientCert) |
| `8441` | Bootstrap enrollment | Plain HTTP (no TLS) |
| `8443` | Public web session | TLS (no client cert) |

### Configuration

The g8e platform uses **ZERO environment variables** for production configuration. All paths are computed relative to project root, and all configuration is via CLI flags:

- `--data-dir <dir>`: Data directory for SQLite database (default: `.g8e/data` in working directory)
- `--pki-dir <dir>`: Directory for TLS certificates (default: `.g8e/pki`)
- `--secrets-dir <dir>`: Directory for platform secrets (default: `.g8e/secrets`)
- `--http-port <port>`: mTLS API port (default: 8440)
- `--bootstrap-port <port>`: Bootstrap enrollment port (default: 8441)
- `--public-port <port>`: Public web session port (default: 8443)

---

## Host Sovereignty & Data Audit

### Multi-Ledger Architecture

Each operator session owns an isolated, encrypted git repository tracking all mutations with `LedgerHashBefore` and `LedgerHashAfter`. Every file mutation triggers a native Go `go-git` commit.

### Fail-Closed Audit Vault

The SQLite-backed `AuditVaultService` mandates valid session identifiers and rejects malformed events. If audit logging fails, execution is aborted.

### Sovereignty Boundary

Output scrubbing is performed directly at the `L5Actuator` boundary to redact tokens, keys, and PII before any data leaves the host.

---

## Implementation Reference

| Concern | File |
|---|---|
| Protobuf schemas | protocol/proto/g8e/common/v1/common.proto |
| Event registry | protocol/constants/events.json |
| Channel prefixes | protocol/constants/channels.json |
| Envelope types | pkg/governance/types.go |
| Warden logic | internal/services/governance/l4_warden.go |
| Actuator logic | internal/services/governance/l5_actuator.go |
| Audit storage | internal/services/storage/audit_vault.go |
| Ledger storage | internal/services/storage/ledger.go |
| Workload identity | protocol/workload_identity.go |
| Gateway envelope construction | internal/services/gateway/governance_envelope.go |
| Gateway HTTP routing | internal/services/gateway/gateway_http.go |
| Pub/Sub command service | internal/services/pubsub/pubsub_commands.go |
| Pub/Sub results service | internal/services/pubsub/pubsub_results.go |
| MCP/A2A translation | internal/services/mcp/gateway.go |
| Session management | internal/services/gateway/session_service.go |
| CLI L3 verification | internal/services/gateway/cli_l3_notary.go |
| Composite L3 verifier | internal/services/gateway/composite_l3_verifier.go |
| Doctrine registry | protocol/constants/doctrine/doctrine_registry.json |
| MCP vectors doctrine | protocol/constants/doctrine/mcp_vectors_doctrine.json |

---

## Related Documentation

- [**Operator (g8eo)**](docs/architecture/operator.md) - Operator architecture and execution boundary
- [**Gateway (g8eg)**](docs/architecture/gateway.md) - Governance Gateway architecture
- [**MCP Protocol**](docs/protocols/mcp/mcp.md) - MCP protocol specification and integration
- [**A2A Protocol**](docs/protocols/a2a/a2a.md) - A2A protocol specification and integration
