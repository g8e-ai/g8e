---
title: Events
parent: Architecture
---

# g8e Event Specification

Last Updated: 2026-05-16
Version: v0.3.0

The g8e platform is a transaction-driven system where every state change is mediated by the **Operator protocol**. All mutations are wrapped in a **Governance Envelope** (UAP JSON), providing a verifiable audit trail from intent to execution.

---

## Architecture & Transport

Events are the primary mechanism for cross-component communication. The architecture follows a substrate-first model where a sovereign host agent (the Operator, `g8eo`) enforces governance before allowing any mutation of the host system.

### 1. The Substrate: `g8eo`
The reference Operator in **Listen Mode** serves as the authoritative gateway.
- **Protocol Boundary**: `g8eo` is the only component with permission to execute shell commands or mutate the filesystem.
- **Fail-Closed Verification**: Every inbound event must pass L1/L2/L3 verification gates before dispatch.
- **Signed Receipts**: Upon completion, the Operator's **Warden** service emits a signed `ActionReceipt` proving the outcome.

### 2. Canonical Wire Format: UAP JSON
To support "Bring Your Own" (BYO) clients and frontends, the canonical wire format for all client-facing surfaces is **JSON (protojson)**.
- **Schema Source**: Defined in `protocol/proto/common.proto` as `GovernanceEnvelope`.
- **Signing Basis**: Identity, intent, and state are bound into a `transaction_hash` which is signed by the Tribunal (L2) and optionally the Human (L3).
- **No Binary on the Wire**: While internal storage may use binary protobuf, the public-facing Pub/Sub and HTTP APIs use JSON for interoperability with MCP, A2A, and LLM tool-calling ecosystems.

### 3. Routing & Context
Every event carries a routing tuple to ensure it reaches the correct execution context:
- `operator_id`: Routes to a specific remote host or slot.
- `operator_session_id`: Correlates with a specific process lifecycle.
- `web_session_id`: Routes back to a specific browser/client connection.
- `case_id` / `investigation_id`: Correlates events with a specific logical thread of work.

---

## The Governance Bedrock

The Operator enforces a 3-layer validation hierarchy before any execution occurs.

### L1: Technical Bedrock (Hard Gates)
Static analysis of the payload. `g8eo` uses Protobuf reflection to inspect `forbidden_patterns` (e.g., `sudo`, `rm -rf /`) on message fields. Violations result in immediate rejection.

### L2: Consensus (The Tribunal)
A cryptographic proof that a group of independent agents agreed on the instruction. The Tribunal attaches an ED25519 signature over the `transaction_hash`. `g8eo` verifies this against its trusted signer store.

### L3: Authorization (Human-in-the-loop)
Explicit human consent for sensitive operations. Carries real **WebAuthn** proofs (Passkeys). Benign diagnostic commands (e.g., `uptime`, `df`) may be auto-approved by policy but still require L1/L2 signatures.

### BFT State Verification
To prevent replay attacks or "stale state" reasoning, the envelope carries a `state_merkle_root`. The Operator compares this with its local ledger; if they mismatch, the command is rejected as the AI's premise was based on outdated reality.

---

## Event Lifecycle

1. **Emission**: A producer (e.g., `g8ee`) serializes a `GovernanceEnvelope` containing a typed payload (e.g., `CommandRequested`).
2. **Ingestion**: The Operator receives the JSON envelope via Pub/Sub or HTTP.
3. **Verification**: `TransactionVerifier` checks L1 patterns, L2/L3 signatures, and the state root.
4. **Warden Dispatch**: If verified, the **Warden** service takes ownership of execution.
5. **Execution**: The local handler executes the intent (e.g., shell fork).
6. **Receipt**: The Warden emits a signed `ActionReceipt` containing the result summary and state root changes.

---

## Event Name Specification

Events follow a hierarchical, dot-separated naming convention where every leaf is a **past-tense** action or state.

### Name Format
```
g8e.v1.<domain>.<resource>[.<sub-resource>...].<action>
```

### Canonical Truth
- `protocol/constants/events.json`: The single source of truth for event name strings.
- `protocol/proto/`: The canonical schema source for envelopes and payloads.

### Core Domain Mappings
| Domain | Purpose | Example |
|--------|---------|---------|
| `app` | Logical application state (cases, tasks). | `g8e.v1.app.case.created` |
| `operator` | Host mutations and lifecycle. | `g8e.v1.operator.command.completed` |
| `ai` | Reasoning and reasoning lifecycle. | `g8e.v1.ai.llm.chat.iteration.started` |
| `platform` | Infrastructure and auth signals. | `g8e.v1.platform.auth.login.succeeded` |

---

## Common Event Patterns

### LLM Chat Pipeline
Exposes the internal reasoning turns of the AI.
- `g8e.v1.ai.llm.chat.iteration.started`
- `g8e.v1.ai.llm.chat.iteration.thinking.started`
- `g8e.v1.ai.llm.chat.iteration.text.chunk.received` (Streaming tokens)
- `g8e.v1.ai.llm.chat.iteration.completed`

### Operator Command Pipeline
Standardized request/response flow for all host mutations.
- `g8e.v1.operator.command.requested` (Inbound Intent)
- `g8e.v1.operator.command.status.updated.running` (Stdout/Stderr increments)
- `g8e.v1.operator.command.completed` (Final Result)
- `g8e.v1.operator.command.failed` (Error Result)

---

## Adding New Events

1. **Update `protocol/constants/events.json`**: Define the new event string.
2. **Update `protocol/proto/*.proto`**: Define the typed payload message.
3. **Map Action Types**: If the event is a mutation, add it to `services/g8eo/internal/mappings/action_types.go`.
4. **Register Handler**: Update `services/g8eo/internal/services/pubsub/pubsub_commands.go`.


