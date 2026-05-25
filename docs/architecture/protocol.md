---
title: g8e Protocol
---

# g8e Protocol

Last Updated: 2026-05-25

## 1. Introduction

The **g8e Protocol** is a zero-trust execution substrate and compliance standard for agentic infrastructure. It ingests payloads from open ecosystems (MCP, A2A, OpenAI tool calls, LangChain) at the admission boundary and forces them through a fail-closed verification gauntlet: envelope integrity, typed-payload decoding, L1 forbidden patterns, hash binding, freshness (`expires_at` + nonce/replay), host state-root validation, L2 Consensus signature, and an L3 Authorization proof bound to the same hash.

Rather than competing with tool-calling standards, the protocol wraps standard JSON-RPC tools as unverified payloads (the "what") inside a strict, canonical `GovernanceEnvelope` (the "how").

### Core Invariants

- **Canonical JSON Wire Format**: All client-facing surfaces (HTTP, WSS pub/sub, receipts, audit exports) carry the `GovernanceEnvelope` as canonical JSON (protojson). Binary protobuf is strictly reserved for internal storage.
- **Hash-Based Signing**: A deterministic `transaction_hash` is computed from normalized envelope fields. The verifier enforces `id == transaction_hash == SHA256(canonical_fields)`.
- **Fail-Closed Verification**: Any malformed envelope, expired transaction, reused nonce, stale state root, or missing proof is rejected immediately before execution.
- **Body-Embedded Context**: Business and execution context (`web_session_id`, `cli_session_id`, `operator_session_id`, `user_id`) lives inside the envelope via a typed `RequestContext`.
- **BFT State Binding**: Mutations carry a `state_merkle_root` that the Operator compares against its current host state.
- **Operator Sovereignty**: No bundled component has privileged channels. The Operator (`g8eo`) is the only execution boundary, enforcing rules uniformly.

## 2. The Lifecycle/Pipeline

The transaction lifecycle follows a strict sequence from intent to audited execution.

### Request Phase (Client -> Gateway -> Operator)

1. A client ecosystem generates a typed protobuf payload (e.g., `CommandRequested`).
2. The payload is embedded into a `GovernanceEnvelope` alongside `nonce`, `expires_at`, and `state_merkle_root`.
3. An L2 Consensus producer computes the `transaction_hash` and attaches a signature.
4. For mutations, an L3 Notary (human) signs the same hash via WebAuthn, unless auto-approval policy applies.
5. The client submits the canonical-JSON envelope over mTLS to the Governance Gateway (`g8eg`), which validates and dispatches it to the target Operator (`g8eo`) over WSS.

### Verification Phase (L4Warden)

The `L4Warden` operates as the primary validation gate, executing the following checks sequentially:

1. **Integrity**: Enforces `id == transaction_hash == SHA256(canonical_fields)`.
2. **Freshness**: Verifies `expires_at` and ensures the `nonce` is not in the replay store.
3. **State Binding**: Validates that the `state_merkle_root` matches the local ledger root.
4. **L1 Doctrine**: Scans the decoded typed payload against reflected `forbidden_patterns` and threat rules.
5. **L2 Consensus**: Verifies the Ed25519 signature against the Operator's trusted `SignerStore`.
6. **L3 Posture**: Validates the WebAuthn proof or applies explicit auto-approval policy for the action.

### Execution & Receipt Phase (L5Actuator)

1. The `L5Actuator` signs an executing-state `ActionReceipt` and writes it to the fail-closed `AuditVault`.
2. The typed payload is dispatched to its execution handler (e.g., shell executor, file edit handler).
3. The `L5Actuator` updates the receipt with the final status (`COMPLETED` or `FAILED`), the post-state root, and a fresh signature.
4. The Operator publishes a result envelope carrying the typed result and signed receipt back to the Gateway.

## 3. Core Subsystems

### The Governance Envelope

The `GovernanceEnvelope` is the single canonical container for every mutation. The schema source of truth is `protocol/proto/g8e/common/v1/common.proto`.

| Field | Purpose |
|---|---|
| `id` | Transaction identifier; must exactly match `transaction_hash`. |
| `event_type` | Canonical event name from `protocol/constants/events.json`. |
| `payload` | Base64-encoded binary protobuf message containing the execution instruction. |
| `transaction_hash` | SHA-256 over: `action_type | target_resource | payload_base64 | state_root | nonce | expires_at | intent_data`. |
| `governance` | Encompasses L1 Doctrine status, L2 Consensus signature, and L3 Notary human proof. |
| `state_merkle_root` | Expected host state root at the time of signing. |
| `nonce` | Unique replay-protection token. |
| `expires_at` | UTC timestamp after which the envelope is strictly void. |

### The Players

The system defines specialized AI agents and roles in `protocol/constants/agents.json`.

| Role | Responsibility |
|---|---|
| **Triage** | Classifies complexity, intent, and user posture. Determines model tier and trajectory. |
| **Sage** | Senior reasoning authority; plans investigations and articulates intent. |
| **Dash** | Surgical responder; handles simple requests with minimum viable latency. |
| **Consensus Members** | Ensemble including Axiom, Concord, Variance, Pragma, and Nemesis that convert intent into commands. |
| **Auditor** | Final quality gate; verifies intent fidelity and disambiguates votes. |
| **L5Actuator** | Orchestrates execution. Final execution boundary for all mutations. |
| **User** | Human domain validator; provides hardware-bound signature to verify intent. |

### Session Separation

The protocol enforces strict separation between session types to guarantee context integrity.

| Session | Identifier | Use | Auth |
|---|---|---|---|
| **Operator** | `operator_session_id` | Host-side agent | mTLS (operator cert, URI SAN) |
| **CLI** | `cli_session_id` | BYO/CLI client | mTLS (CLI cert, URI SAN) |
| **Web** | `web_session_id` | Browser frontend | Passkey (WebAuthn) |

### JSON-RPC Error Mapping

The Operator proxy exposes Gateway verification failures back to MCP/A2A clients via standardized JSON-RPC codes (defined in `internal/responder/responder.go`):

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

## 4. Governance & Safety

### 3-Layer Governance Bedrock

Every mutation must pass three independent layers in order. A failure at any layer is an immediate rejection.

**1. L1 Doctrine: Technical Bedrock**
Static, deterministic checks enforced before any code executes. Validated using doctrines sourced from `protocol/constants/doctrine/doctrine_registry.json`.
- **Forbidden Patterns**: The custom protobuf field option `(g8e.common.v1.forbidden_patterns)` is reflected at runtime to scan typed payloads.
- **Threat Detection**: Sentinel threat logic runs within L1 Doctrine to analyze command inputs for MITRE ATT&CK patterns, reverse shells, and injection vectors.
- **Allow/Deny Lists**: Enforces per-host policy and user settings.

**2. L2 Consensus: Distributed Agreement**
A cryptographic proof that an independent ensemble agreed on the instruction.
- An Ed25519 signature is generated over `transaction_hash | decision`.
- Verified against the Operator-owned `SignerStore`.
- The reference implementation runs a Byzantine cascade: Triage -> Dash/Sage -> 5-member Consensus generation -> Auditor verification -> Signature.

**3. L3 Notary: Human Authorization**
Hardware-bound proof of human presence.
- **Web Sessions**: Real WebAuthn/FIDO2 proof with the transaction hash as the assertion challenge.
- **CLI Sessions**: Authenticates via mTLS certificates with SPIFFE URI SANs. The L3 proof is the SHA-256 fingerprint of the mTLS certificate.
- **Auto-Approval**: Explicit policy permits auto-approval for benign diagnostic verbs. Auto-approval never bypasses L1 or L2 gates.

### Host Sovereignty & Data Audit

- **Multi-Ledger Architecture**: Each operator session owns an isolated, encrypted git repository (`.g8e/data/ledger/sessions/<id>/`). Every file mutation triggers a native Go `go-git` commit tracking the `LedgerHashBefore` and `LedgerHashAfter`.
- **Fail-Closed Audit Vault**: The SQLite-backed `AuditVaultService` mandates valid session identifiers and rejects malformed events. If audit logging fails, execution is aborted.
- **Sovereignty Boundary**: Output scrubbing is performed directly at the `L5Actuator` boundary to redact tokens, keys, and PII before any data leaves the host.

---

### Implementation Reference

| Concern | Authoritative file |
|---|---|
| Protobuf schemas | `protocol/proto/` |
| Event registry | `protocol/constants/events.json` |
| Channel prefixes | `protocol/constants/channels.json` |
| Envelope types | `pkg/uap/types.go` |
| Warden logic | `internal/services/governance/l4_warden.go` |
| Actuator logic | `internal/services/governance/l5_actuator.go` |
| Audit storage | `internal/services/storage/audit_vault.go` |
| Workload identity| `protocol/workload_identity.go` |
