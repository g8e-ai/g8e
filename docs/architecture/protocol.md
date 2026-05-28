---
title: g8e Protocol
---

# g8e Protocol

Last Updated: 2026-05-28

## Overview

The **g8e Protocol** is a zero-trust execution substrate for agentic infrastructure. It defines a typed, signed, state-bound transaction envelope that admits payloads from open ecosystems (MCP, A2A, OpenAI tool calls, LangChain) through a fail-closed verification pipeline.

The protocol wraps standard JSON-RPC tools as unverified payloads (the "what") inside a strict, canonical `GovernanceEnvelope` (the "how"). The envelope carries cryptographic proofs that must pass the 5-layer verification sequence before any execution occurs.

### Core Invariants

- **Canonical JSON Wire Format**: All client-facing surfaces (HTTP, WSS pub/sub, receipts, audit exports) carry the `GovernanceEnvelope` as canonical JSON (protojson). Binary protobuf is strictly reserved for internal storage.
- **Hash-Based Signing**: A deterministic `transaction_hash` is computed from normalized envelope fields. The verifier enforces `id == transaction_hash == SHA256(canonical_fields)`.
- **Fail-Closed Verification**: Any malformed envelope, expired transaction, reused nonce, stale state root, or missing proof is rejected immediately before execution.
- **Body-Embedded Context**: Execution context (`web_session_id`, `cli_session_id`, `operator_session_id`, `operator_id`) lives inside the envelope as first-class fields.
- **BFT State Binding**: Mutations carry a `state_merkle_root` that the Operator compares against its current host state.
- **Operator Sovereignty**: The Operator (`g8eo`) is the only execution boundary, enforcing rules uniformly. No component has privileged bypass channels.

## Protocol Components

### GovernanceEnvelope

The `GovernanceEnvelope` is the single canonical container for every mutation. It carries the typed payload, cryptographic proofs, and execution context in a verifiable unit.

See [GovernanceEnvelope Schema](../protocols/g8e/g8e.md#governanceenvelope-schema) for detailed field specifications.

### 5-Layer Verification Sequence

Every mutation must pass through a strict, fail-closed pipeline of five independent layers. A failure at any layer results in immediate rejection.

- **L1 Doctrine** (L1Doctrine): Technical Bedrock - Static analysis and forbidden pattern enforcement (e.g., `sudo`, `rm -rf /`).
- **L2 Consensus** (L2Consensus): Distributed Agreement - Multi-agent verification via Ed25519 signatures from an independent ensemble.
- **L3 Notary** (L3Notary): Human Authorization - Hardware-bound proof of human presence (WebAuthn/Passkey).
- **L4 Warden** (L4Warden): Pre-dispatch Verification - Integrity, freshness (expiry/nonce), and state-root matching.
- **L5 Actuator** (L5Actuator): Execution Boundary - Single fail-closed dispatch path that signs and emits `ActionReceipts`.

## Transaction Lifecycle

The transaction lifecycle follows a strict sequence from intent to audited execution:

1. **Request Phase**: Client generates typed payload, embeds in envelope, attaches L2/L3 signatures, submits to Gateway.
2. **Verification Phase**: Operator executes integrity, freshness, state binding, and the 5-layer verification checks at **L4Warden**.
3. **Execution & Receipt Phase**: **L5Actuator** signs executing receipt, dispatches payload, updates receipt with final status, publishes result.

## Session Management

The protocol enforces strict separation between session types to guarantee context integrity:

- **Operator Session** (`operator_session_id`): Host-side agent, authenticated via mTLS with operator cert URI SAN.
- **CLI Session** (`cli_session_id`): BYO/CLI client, authenticated via mTLS with CLI cert URI SAN.
- **Web Session** (`web_session_id`): Browser frontend, authenticated via Passkey (WebAuthn).

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

## Host Sovereignty & Data Audit

- **Multi-Ledger Architecture**: Each operator session owns an isolated, encrypted git repository tracking all mutations with `LedgerHashBefore` and `LedgerHashAfter`.
- **Fail-Closed Audit Vault**: SQLite-backed service mandates valid session identifiers and rejects malformed events. If audit logging fails, execution is aborted.
- **Sovereignty Boundary**: Output scrubbing is performed at the execution boundary to redact tokens, keys, and PII before any data leaves the host.

## Deep-Dive Reference

For comprehensive protocol specifications, schema details, event types, configuration, and implementation references, see the [g8e Protocol Specification](../protocols/g8e/g8e.md).

## Related Documentation

- [**Operator (g8eo)**](operator.md) - Operator architecture and execution boundary
- [**Gateway (g8eg)**](gateway.md) - Governance Gateway architecture
- [**MCP Protocol**](../protocols/mcp/mcp.md) - MCP protocol specification and integration
- [**A2A Protocol**](../protocols/a2a/a2a.md) - A2A protocol specification and integration
