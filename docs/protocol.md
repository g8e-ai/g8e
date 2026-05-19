---
title: g8e Protocol
---

# g8e Protocol

Last Updated: 2026-05-18

The **g8e Protocol** is a governance and compliance standard. It ingests payloads from open ecosystems (MCP, A2A, OpenAI tool calls, LangChain, etc.) at the Operator's admission boundary and forces them through a fail-closed verification gauntlet - envelope integrity, typed-payload decode, L1 forbidden patterns, hash binding, freshness (`expires_at` + nonce/replay), host state-root match, L2 Tribunal signature against a trusted signer, and (for mutations) an L3 WebAuthn proof bound to the same hash. Non-conformant payloads are rejected at the substrate boundary: they never reach the application layer (Warden, execution handlers) and they never touch the host. Admitted payloads produce a cryptographically provable audit trail with a deep local-first record at the site of execution.

The protocol is the only mandatory layer of g8e. Any conforming implementation - Operator, client, or BYO frontend - interoperates by speaking this contract. The reference Operator (`g8eo`) and reference Engine (`g8ee`) are interchangeable with anything that produces and verifies the same envelopes.

---

## Core Invariants

1. **Canonical JSON wire format** - All client-facing surfaces (HTTP, WSS pub/sub, receipts, audit exports) carry the `GovernanceEnvelope` as canonical JSON (protojson). Binary protobuf is reserved for internal storage. JSON-on-the-wire is what makes the protocol interoperable with MCP (JSON-RPC), A2A (JSON/HTTP), and LLM tool-calling ecosystems.
2. **Hash-based signing** - A deterministic `transaction_hash` is computed from normalized envelope fields. The verifier enforces `id == transaction_hash == SHA256(canonical_fields)`. Wire encoding is irrelevant to the security invariant.
3. **Fail-closed verification** - Any malformed envelope, expired transaction, reused nonce, stale state root, or missing proof is rejected immediately. The system never fails open.
4. **Body-embedded context** - Business and execution context (`web_session_id`, `cli_session_id`, `operator_session_id`, `user_id`, `case_id`, `investigation_id`, etc.) lives inside the envelope body via a typed `RequestContext`. HTTP headers are reserved for protocol-level metadata and mTLS-bound identity.
5. **BFT state binding** - Mutations carry a `state_merkle_root` that the Operator compares against its current host state. Stale-state transactions are rejected.
6. **Signed receipts** - Every accepted mutation produces a Warden-signed `ActionReceipt` containing status, `state_root_before`, `state_root_after`, and a key-id-bound Ed25519 signature.
7. **Operator sovereignty** - No bundled component has privileged channels. The Operator is the only execution boundary, and its rules apply uniformly to BYO and reference clients.

---

## The Players

The system utilizes specialized AI agents defined in `protocol/constants/agents.json`, each with a distinct lens and responsibility within the co-validated infrastructure.

| Player | Role | ID | Lens / Capability |
|---|---|---|---|
| **Triage** | Gatekeeper | `triage` | Classifies complexity, intent, and user posture. Determines model tier and trajectory. |
| **Sage** | Architect | `sage` | Senior reasoning authority; plans investigations and articulates intent. |
| **Dash** | Fast-Path | `dash` | Surgical responder; handles simple requests with minimum viable latency. |
| **Tribunal** | Ensemble | `tribunal` | Five-member panel that converts intent into executable commands. |
| **Axiom** | Composer | `axiom` | Tribunal member: focuses on elegant composition and efficient pipelines. |
| **Concord** | Guardian | `concord` | Tribunal member: focuses on defensive discipline and minimal risk. |
| **Variance** | Exhaustive | `variance` | Tribunal member: focuses on edge cases (filenames, spaces, symlinks). |
| **Pragma** | Conventional | `pragma` | Tribunal member: focuses on idiomatic tools and community standards. |
| **Nemesis** | Adversary | `nemesis` | Calibrated adversary: proposes subtly flawed candidates to test the system. |
| **Auditor** | Verifier | `auditor` | Final quality gate; verifies intent fidelity and syntax; disambiguates votes. |
| **Warden** | Defender | `warden` | Orchestrates risk assessment and execution. Final gate for all mutations. |
| **Codex** | Memory | `codex` | Extracts durable user preferences and scrubbed summaries from history. |
| **Judge** | Evaluator | `judge` | Dispassionate grader of agent performance against gold-standard rubrics. |
| **User** | Co-validator | `user` | Human domain validator; provides hardware-bound signature to verify intent. |

---

## The Governance Envelope

The `GovernanceEnvelope` is the single canonical container for every g8e mutation. The schema lives in `@/home/bob/g8e/protocol/proto/common.proto`.

| Field | Purpose |
|---|---|
| `id` | Transaction identifier; must match `transaction_hash`. |
| `event_type` | Canonical event name from `protocol/constants/events.json`. |
| `payload` | Base64-encoded binary protobuf message - the **sole authority for execution**. |
| `intent_data` | `google.protobuf.Struct` view for visibility/audit. Never used as a fallback for execution. |
| `transaction_hash` | SHA-256 over: `action_type | target_resource | payload_base64 | state_root | nonce | expires_at | intent_data`. |
| `governance` | L1 status, L2 Tribunal signature, L3 human proof. |
| `state_merkle_root` | Expected host state root at signing time. |
| `nonce` | Unique replay-protection token. |
| `expires_at` | UTC timestamp after which the envelope is void. |

The schema source of truth lives under `@/home/bob/g8e/protocol/proto/`:

| File | Purpose |
|---|---|
| `common.proto` | `GovernanceEnvelope`, `GovernanceMetadata`, L1/L2/L3 substructures. |
| `operator.proto` | Typed mutation payloads (`CommandRequested`, `FileEditRequested`, `ActionReceipt`, etc.). |
| `pubsub.proto` | Envelope-aware pub/sub message types. |

---

## 3-Layer Governance Bedrock

Every mutation must pass three independent layers in order. A failure at any layer is an immediate rejection.

### L1: Technical Bedrock (Hard Gates)

Static, deterministic checks enforced before any code executes.

- **Forbidden patterns** - Custom protobuf field option `(g8e.common.v1.forbidden_patterns)` is reflected at runtime to scan typed payloads (e.g., `command` field) for `sudo`, `su`, `rm -rf /`, etc.
- **Sentinel pre-execution analysis** - Regex matching against 90+ MITRE ATT&CK threat patterns (reverse shells, privilege escalation, exfiltration).
- **Allow/deny lists** - Per-host policy in `protocol/constants/` and per-user `command_validation` settings.

### L2: Consensus (Tribunal)

A cryptographic proof that an independent ensemble agreed on the instruction.

- **Mechanism** - Ed25519 signature over `transaction_hash | decision`.
- **Trust** - The Operator maintains an Operator-owned `SignerStore`; missing or unknown keys cause rejection.
- **Producer** - Any conforming L2 producer (the bundled Engine, a BYO multi-agent system, or a single signer for low-stakes flows).
- **Reference Engine producer** - g8ee runs its own internal Byzantine cascade upstream of the L2 signature: Triage → Dash/Sage (intent articulation) → 5-member Tribunal generation → R1 vote → optional R2 anonymized peer review → Warden risk analysis (Two-Strike Circuit Breaker) → Auditor verification + Merkle reputation commitment. The Engine signs only after Auditor passes. The Operator does not assume any of this; it re-runs every gate below independently. See [g8ee Governance & Safety](g8ee.md) and [position paper §2.3](position_paper.md).

### L3: Authorization (Human)

Hardware-bound proof of human presence, except where policy explicitly permits auto-approval.

- **Mechanism** - Real WebAuthn/FIDO2 `L3Proof` (clientDataJSON, authenticatorData, signature) with the transaction hash as the assertion challenge.
- **Auto-approval** - Benign diagnostic verbs (e.g., `uptime`, `df`) may be marked L3-authorized via policy. **L3 auto-approval never bypasses L1 or L2.**

---

## Transaction Lifecycle

### Request Phase (Client → Operator)

1. Client builds a typed protobuf payload (e.g., `CommandRequested`).
2. Client embeds the payload in a `GovernanceEnvelope`, populating `nonce`, `expires_at`, and `state_merkle_root`.
3. The L2 producer computes `transaction_hash` and attaches a Tribunal signature.
4. The L3 actor (human) signs the same hash via WebAuthn.
5. Client submits canonical-JSON envelope over mTLS to the Operator.

### Verification Phase (Operator)

The `TransactionVerifier` runs the following gates in order:

1. **Integrity** - `id == transaction_hash == SHA256(canonical_fields)`.
2. **Freshness** - `expires_at` not passed; `nonce` not in the replay store.
3. **State** - `state_merkle_root` matches local ledger root.
4. **L1** - Reflected `forbidden_patterns` over the typed payload + Sentinel threat analysis.
5. **L2** - Tribunal signature verified against the trusted `SignerStore`.
6. **L3** - WebAuthn `L3Proof` verified for mutations (or auto-approval policy applied after L1/L2 pass).

### Execution & Receipt Phase (Operator → Client)

1. The **Warden** signs an executing-state `ActionReceipt` and writes it to the AuditVault. If logging fails, execution is aborted.
2. The Warden dispatches the typed payload to its execution handler (e.g., shell executor, file edit handler).
3. The Warden updates the receipt with the final status (`COMPLETED` / `FAILED`), the post-state root, and a fresh signature.
4. The Operator publishes a result envelope (also a `GovernanceEnvelope`) carrying the typed result and the signed receipt.

---

## Event Specification

All cross-component messages are events with hierarchical, dot-separated, past-tense names:

```
g8e.v1.<domain>.<resource>[.<sub-resource>...].<action>
```

| Domain | Purpose | Example |
|---|---|---|
| `app` | Logical application state | `g8e.v1.app.case.created` |
| `operator` | Host mutations and lifecycle | `g8e.v1.operator.command.completed` |
| `ai` | Reasoning lifecycle | `g8e.v1.ai.llm.chat.iteration.text.chunk.received` |
| `platform` | Infrastructure / auth signals | `g8e.v1.platform.auth.login.succeeded` |

Canonical truth lives in:

- `@/home/bob/g8e/protocol/constants/events.json` - string names
- `@/home/bob/g8e/protocol/proto/` - typed payload schemas
- `@/home/bob/g8e/protocol/constants/channels.json` - pub/sub channel prefixes

### Adding a new event

1. Add the string to `protocol/constants/events.json`.
2. Define a typed payload in `protocol/proto/`.
3. If it is a mutation, add an action-type mapping in `services/g8eo/internal/mappings/action_types.go`.
4. Register a handler in `services/g8eo/internal/services/pubsub/pubsub_commands.go`.

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

## Pub/Sub Transport

The Operator's `--listen` mode is the WSS broker and governance gate.

### Channel taxonomy

Per-operator-session: `{prefix}:{operator_id}:{operator_session_id}`

| Prefix | Source | Destination | Purpose |
|---|---|---|---|
| `cmd` | Client | `g8eo` | Inbound mutations and control requests. |
| `results` | `g8eo` | Client | Stdout/stderr/artifacts. |
| `heartbeat` | `g8eo` | Client | Liveness and resource utilization. |

Platform broadcast: `operator_heartbeats`, `sse_events`, `system_events`.

### Wire rules

- All envelopes are canonical JSON (protojson).
- `operator_session_id` may contain separators; always parse with a bounded split (`SplitN(channel, ":", 3)`).
- Missing `message_id`, `operator_session_id`, or unknown `event_type` → rejected/dropped at the broker.
- `/pubsub/publish` is restricted to non-mutation fan-out (`heartbeat:*`, `results:*`, `sse:*`, `internal:*`). Mutations must use `POST /api/governance/envelope` and return `409 Conflict` if attempted on `cmd:*` directly.

### Technical Invariants

- **Zero-Trust Networking**: Operators require outbound WSS connectivity. No inbound ports are opened; all inputs are distrusted until verified.
- **Bounded Parsing**: Use `SplitN(channel, ":", 3)` when parsing channels to handle session IDs that may contain separators.
- **Fail-Closed Execution**: If the `Warden` service or `TransactionVerifier` is missing/nil, all inbound commands are rejected.

---

## Host Sovereignty & Audit

### Multi-Ledger Architecture
The Operator implements an isolated, git-based ledger for every session:
- **Isolation**: Each operator session owns a unique git repository at `.g8e/data/ledger/sessions/<id>/`.
- **Verifiable History**: Every file mutation is mirrored via a two-phase commit (`LedgerHashBefore` -> `LedgerHashAfter`).
- **Encryption**: Session ledgers are stored encrypted at rest when the vault is unlocked.

### Encrypted Audit Vault
Every action and receipt is recorded in an encrypted SQLite database. The `AuditVaultService` is fail-closed; it rejects events missing valid session identifiers or malformed metadata.

### Output Scrubbing (Sentinel)
**Sentinel** performs dual-role analysis:
1. **Defense**: Analyzes input commands for MITRE ATT&CK patterns.
2. **Sovereignty**: Scrubs output for tokens, keys, and PII before it leaves the host.

---

## Session Types

The protocol enforces strict separation between disjoint session types. The Operator never falls back to a single session ID; each request must declare its context.

| Session | Identifier | Use | Auth |
|---|---|---|---|
| **Operator** | `operator_session_id` | Host-side agent | mTLS (operator cert, URI SAN) |
| **CLI** | `cli_session_id` | BYO/CLI client (`./g8e chat`) | mTLS (CLI cert, URI SAN) |
| **Web** | `web_session_id` | Browser frontend | Passkey (WebAuthn) |

A `web_session_id` can never receive events scoped to a `cli_session_id`, and vice versa.

---

## Reputation & Stakes

Agent performance is tracked via an EMA scalar `[0.0, 1.0]` in the `reputation_state` collection.

| Player | Staked Lens | Slashing Triggers |
|---|---|---|
| **Axiom** | Composition | Missed pass, Whitelist violation |
| **Concord** | Safety | Missed pass, Whitelist violation |
| **Variance** | Edge Cases | Missed pass, Whitelist violation |
| **Pragma** | Convention | Missed pass, Whitelist violation |
| **Nemesis** | Adversary | False alarm, Abstaining on real flaw |
| **Sage** | Intent | Consensus failure, Heavy Auditor revision |
| **Auditor** | Verification | Destructive approval failure, Auditor error |
| **Warden** | Defense | Missed risk, Over-caution (blocking LOW) |

---

## Implementation Reference

| Concern | Authoritative file |
|---|---|
| Protobuf schemas | `@/home/bob/g8e/protocol/proto/` |
| Event registry | `@/home/bob/g8e/protocol/constants/events.json` |
| Channel prefixes | `@/home/bob/g8e/protocol/constants/channels.json` |
| Envelope types (Go) | `@/home/bob/g8e/services/g8eo/pkg/uap/types.go` |
| Verification logic | `@/home/bob/g8e/services/g8eo/internal/services/governance/transaction_verifier.go` |
| Audit storage | `@/home/bob/g8e/services/g8eo/internal/services/storage/audit_vault.go` |
| Workload identity | `@/home/bob/g8e/protocol/workload_identity.go` |

For the reference Operator implementation see [Operator](operator.md). For the reference Engine application see [g8ee Service](g8ee_service.md). For Hub/data-backplane behavior see [g8eo Service](g8eo_service.md).
