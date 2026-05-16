---
title: Pub/Sub & Protocol
parent: Architecture
---

# g8e Pub/Sub Architecture

Last Updated: 2026-05-16
Version: v0.4.0

The g8e platform utilizes a high-performance, WebSocket-based Pub/Sub system for all real-time communication between the substrate (Operator) and application-layer adapters (Engine, Dashboard, or BYO clients). 

The host listener is the **Operator binary running in `--listen` mode**, which acts as the WebSocket broker and governance gate for the platform.

---

## The Lifecycle of a Transaction

Every action in the g8e ecosystem is a **Transaction** wrapped in a `GovernanceEnvelope`. In the Go substrate, this is often referred to by its alias, `UAPEnvelope`.

### 1. Intent & Packaging
A client (Engine or BYO) packages an intent (e.g., `EXECUTE_BASH`) into a `GovernanceEnvelope`. 
- **Wire Format**: Canonical JSON (`protojson`) is the **mandatory** wire format for all client-facing surfaces. Binary protobuf and legacy formats are explicitly rejected.
- **Identity**: Every envelope must include a valid `operator_id` and `operator_session_id`.

### 2. Dispatch
The JSON envelope is published to the Operator's command channel: `cmd:{operator_id}:{operator_session_id}`.

### 3. Verification (The Fail-Closed Gate)
Upon receipt, the Operator's `PubSubCommandService` performs strict validation via the `TransactionVerifier`:
- **L1 Technical Bedrock**: Checks for forbidden patterns and blacklisted commands (hard gates).
- **L2 Consensus (Tribunal)**: Verifies ED25519 signatures from trusted signers against the `transaction_hash`.
- **L3 Authorization (Approval)**: Verifies human presence (WebAuthn) or auto-approval policy.

If any check fails, or if the `TransactionVerifier` is unavailable, the transaction is rejected immediately and recorded as a `BLOCKED` action receipt.

### 4. Execution & Receipt
The **Warden** service acts as the final execution boundary. Only after successful verification does the `Warden` execute the payload. 
- **Signed Action Receipt**: Upon completion, the Warden emits a signed receipt containing the outcome and state proofs.
- **Fail-Closed Execution**: If the `Warden` service is missing, no execution occurs.

### 5. Results Delivery
Execution output (stdout, stderr, exit codes) is published back to the client via the `results:{operator_id}:{operator_session_id}` channel as a JSON-encoded `GovernanceEnvelope`.

---

## Channel Taxonomy

Canonical prefixes and formats are defined in `@/protocol/constants/channels.json`. 

### Per-Operator-Session Channels
Format: `{prefix}:{operator_id}:{operator_session_id}`

| Prefix | Source | Destination | Purpose |
| :--- | :--- | :--- | :--- |
| `cmd` | Client | `g8eo` | Inbound mutations and control requests. |
| `results` | `g8eo` | Client | Outbound stdout, stderr, and artifacts. |
| `heartbeat` | `g8eo` | Client | Signal of life and resource utilization metrics. |

### Platform Broadcast Channels
Broadcasting system-wide state changes to all authorized listeners.

| Channel | Purpose |
| :--- | :--- |
| `operator_heartbeats` | Aggregated stream of all heartbeats for fleet monitoring. |
| `sse_events` | Real-time event stream for browser-based UI updates. |
| `system_events` | High-priority system notifications. |

---

## Wire Format & Governance

### Canonical JSON (protojson)
JSON is the canonical wire format for the `GovernanceEnvelope`. This ensures compatibility with:
- **MCP**: Model Context Protocol (JSON-RPC).
- **A2A**: Agent2Agent protocols (JSON/HTTP).
- **LLM Tooling**: OpenAI, Anthropic, and LangChain ecosystems.

### Signing Invariant
Identity and integrity are maintained through a deterministic **Message ID** (and `transaction_hash`) computed from normalized envelope fields. The wire encoding is irrelevant to security because the verifier enforces that the `id` matches the computed hash.

**Canonicalization Rules**:
1. Fields processed in proto definition order.
2. Strings as UTF-8.
3. Bytes as base64.
4. Result hashed with SHA-256.

### Fail-Closed Design
- **Missing IDs**: Any envelope missing a `message_id` or `operator_session_id` is rejected.
- **Unknown Types**: If no handler is registered for an `event_type`, the message is dropped.
- **Missing Verifiers**: If the `TransactionVerifier` or `Warden` is nil, the service rejects all inbound commands.

---

## Technical Invariants

### 1. Separation of Concerns
- **Substrate (g8eo)**: Owns the broker, identity, and governance gates.
- **Application Layer**: Consumers (e.g., `g8ee`) have no privileged access and must use the public protocol surface.

### 2. Single Source of Truth
- **Channels**: Use `@/protocol/constants/channels.json`.
- **Events**: Use `@/protocol/constants/events.json` for all `event_type` values.
- **Models**: Use `GovernanceEnvelope` defined in `@/protocol/proto/common.proto`.

### 3. Bounded Parsing
The `operator_session_id` may contain separators. Always use a **bounded split** with a maximum of 2 splits (3 parts) when parsing channels.

```go
// Canonical Go parsing
parts := strings.SplitN(channel, ":", 3)
```

### 4. Zero-Trust Networking
Operators require outbound WSS connectivity to the platform. No inbound ports are opened, and all inputs are distrusted until verified by the 3-layer governance hierarchy.
