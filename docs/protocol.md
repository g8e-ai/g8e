---
title: g8e Protocol
---

# g8e Protocol

Last Updated: 2026-05-21

The **g8e Protocol** is a governance and compliance standard. It ingests payloads from open ecosystems (MCP, A2A, OpenAI tool calls, LangChain, etc.) at the Local Operator's admission boundary and forces them through a fail-closed verification gauntlet - envelope integrity, typed-payload decode, L1 forbidden patterns, hash binding, freshness (`expires_at` + nonce/replay), host state-root match, L2 Tribunal signature against a trusted signer, and (for mutations) an L3 WebAuthn proof bound to the same hash. Non-conformant payloads are rejected at the boundary: they never reach the execution boundary (the Actuator) and they never touch the host. Admitted payloads produce a cryptographically provable audit trail with local-first persistence at the site of execution.

Rather than competing with tool-calling standards, g8e functions as a secure perimeter. It treats standard JSON-RPC tools as unverified payloads (the "what") and wraps them in a strict, canonical `GovernanceEnvelope` (the "how").

The protocol is the only mandatory layer of g8e. Any conforming implementation - Local Operator, Remote Operator, client, or BYO frontend - interoperates by speaking this contract. The reference Local Operator (`g8eg`), Remote Operator (`g8eo`), and the reference **g8e Agentic Ensemble** (`g8ee`) are interchangeable with anything that produces and verifies the same envelopes.

---

## Architectural Differentiators

*   **Outbound-Only Reverse Tunnel:** The host-resident Operator binary (`g8eo`) connects via an outbound-only tunnel to the platform hub. This architecture completely bypasses NAT and enterprise firewalls, eliminating the operational necessity of opening dangerous inbound listening ports.
*   **Protocol-First Zero Trust:** Every system component inherently distrusts all other components. The execution gateway boundary actively handles workloads via mTLS and device-link tokens, ensuring no unauthenticated or unverified component holds privileged trust.
*   **Byzantine Fault Tolerant (BFT) Safety:** Agentic automation is treated as a distributed consensus problem. The Quorum (L2) Tribunal is fully provider-agnostic, combining heterogeneous models (e.g., Anthropic, OpenAI, local models) to outvote individual hallucinations or poisonings.
*   **Deterministic Intent Validation:** Execution authority does not rely on fluid natural language strings. The protocol enforces that explicit execution intent is serialized into a typed Protobuf payload, base64-encoded, and locked into the transaction hash of the envelope.
*   **3-Layer Inline Governance Gate:** Every mutation must sequentially pass Doctrine (L1Doctrine) Technical Bedrock (Hard Gates), Quorum (L2Consensus) Consensus (Tribunal), and Notary (L3Notary) Authorization (WebAuthn/Passkey) at the Operator boundary before hitting the host shell.
*   **Local-First Data Sovereignty (LFAA):** All raw data, system roots, and execution histories are isolated locally on the managed host. Every file mutation triggers a two-phase Git-backed commit tracking pre-mutation and post-mutation states, guaranteeing a tamper-evident history trail and instant rollbacks.
*   **Zero Standing Dependencies:** The reference Operator is a single, statically compiled Go binary. The entire platform can deploy in highly hostile, isolated, or air-gapped infrastructure perimeters.

---

## Core Invariants

1. **Canonical JSON wire format** - All client-facing surfaces (HTTP, WSS pub/sub, receipts, audit exports) carry the `GovernanceEnvelope` as canonical JSON (protojson). Binary protobuf is reserved for internal storage. JSON-on-the-wire is what makes the protocol interoperable with MCP (JSON-RPC), A2A (JSON/HTTP), and LLM tool-calling ecosystems.
2. **Hash-based signing** - A deterministic `transaction_hash` is computed from normalized envelope fields. The verifier enforces `id == transaction_hash == SHA256(canonical_fields)`. Wire encoding is irrelevant to the security invariant.
3. **Fail-closed verification** - Any malformed envelope, expired transaction, reused nonce, stale state root, or missing proof is rejected immediately. The system never fails open.
4. **Body-embedded context** - Business and execution context (`web_session_id`, `cli_session_id`, `operator_session_id`, `user_id`, `case_id`, `investigation_id`, etc.) lives inside the envelope body via a typed `RequestContext`. HTTP headers are reserved for protocol-level metadata and mTLS-bound identity.
5. **BFT state binding** - Mutations carry a `state_merkle_root` that the Operator compares against its current host state. Stale-state transactions are rejected.
6. **Signed receipts** - Every accepted mutation produces a Actuator-signed `ActionReceipt` containing status, `state_root_before`, `state_root_after`, and a key-id-bound Ed25519 signature.
7. **Operator sovereignty** - No bundled component has privileged channels. The Remote Operator (`g8eo`) is the only execution boundary, and its rules apply uniformly to BYO and reference clients.

---

## The Players

The system utilizes specialized AI agents defined in `services/g8eo/internal/constants/agents.go`, each with a distinct lens and responsibility within the co-validated infrastructure.

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
| **Actuator** | Defender | `Actuator` | Orchestrates risk assessment and execution. Final gate for all mutations. |
| **Codex** | Memory | `codex` | Extracts durable user preferences and scrubbed summaries from history. |
| **Judge** | Evaluator | `judge` | Dispassionate grader of agent performance against gold-standard rubrics. |
| **User** | Co-validator | `user` | Human domain validator; provides hardware-bound signature to verify intent. |

---

## The Governance Envelope

The `GovernanceEnvelope` is the single canonical container for every g8e mutation. The schema lives in `@/home/bob/g8e/protocol/proto/common.proto`.

| Field | Purpose |
|---|---|
| `id` | Transaction identifier; must match `transaction_hash`. |
| `event_type` | Canonical event name from `services/g8eo/internal/constants/events.go`. |
| `payload` | Base64-encoded binary protobuf message - the **sole authority for execution**. |
| `intent_data` | `google.protobuf.Struct` view for visibility/audit. Never used as a fallback for execution. |
| `transaction_hash` | SHA-256 over: `action_type | target_resource | payload_base64 | state_root | nonce | expires_at | intent_data`. |
| `governance` | Doctrine (L1Doctrine) status, Quorum (L2Consensus) Tribunal signature, Notary (L3Notary) human proof. |
| `state_merkle_root` | Expected host state root at signing time. |
| `nonce` | Unique replay-protection token. |
| `expires_at` | UTC timestamp after which the envelope is void. |

The schema source of truth lives under `@/home/bob/g8e/protocol/proto/`:

| File | Purpose |
|---|---|
| `common.proto` | `GovernanceEnvelope`, `GovernanceMetadata`, Doctrine (L1Doctrine)/Quorum (L2Consensus)/Notary (L3Notary) substructures. |
- `operator.proto` | Typed mutation payloads (`CommandRequested`, `FileEditRequested`, `ActionReceipt`, etc.).
- `pubsub.proto` | Envelope-aware pub/sub message types.

---

## JSON-RPC Error Mapping

For BYO clients using the MCP or A2A protocol translation gateway, `g8eg` (serving as Gateway) and `g8eo` (serving as MCP Server) provide granular JSON-RPC error codes to disambiguate Gateway verification failures. These codes reside in the reserved range `-32000` to `-32099`.

| Code | Label | Meaning |
|---|---|---|
| `-32000` | `ERR_INVALID_ENVELOPE` | Malformed UAP JSON, missing ID, or unknown action type. |
| `-32001` | `ERR_HASH_MISMATCH` | `transaction_hash` is missing or does not match computed SHA-256. |
| `-32002` | `ERR_EXPIRED` | `expires_at` timestamp has passed. |
| `-32003` | `ERR_REPLAY` | `nonce` has already been used within the expiry window. |
| `-32004` | `ERR_STATE_MISMATCH` | `state_merkle_root` does not match the current host state. |
| `-32005` | `ERR_L1_FAILED` | Typed payload violates Doctrine (L1Doctrine) forbidden patterns or Sentinel rules. |
| `-32006` | `ERR_L2_FAILED` | Quorum (L2Consensus) Tribunal signature is missing, invalid, or from an untrusted key. |
| `-32007` | `ERR_L3_FAILED` | Notary (L3Notary) WebAuthn proof is missing or failed verification. |
| `-32008` | `ERR_PAYLOAD_DECODE` | Failed to decode the base64 `payload` into its typed protobuf message. |
| `-32101` | `ERR_Gateway_NOT_READY` | Governance Gateway (Actuator/Verifier) is not initialized. |
| `-32603` | `INTERNAL_ERROR` | Unhandled internal error or execution failure. |

---

## 3-Layer Governance Bedrock

Every mutation must pass three independent layers in order. A failure at any layer is an immediate rejection.

### Doctrine (L1Doctrine): Technical Bedrock (Hard Gates)

Static, deterministic checks enforced before any code executes.

- **Forbidden patterns** - Custom protobuf field option `(g8e.common.v1.forbidden_patterns)` is reflected at runtime to scan typed payloads (e.g., `command` field) for `sudo`, `su`, `rm -rf /`, etc.
- **Sentinel pre-execution analysis** - Regex matching against threat doctrines (reverse shells, privilege escalation, exfiltration).
- **Allow/deny lists** - Per-host policy in `services/g8eo/internal/constants/` and per-user `command_validation` settings.

#### Doctrine Storage

Doctrine definitions are stored in `protocol/constants/doctrine/` as canonical JSON files:

```
protocol/constants/doctrine/
  doctrine_registry.json      # Metadata: sources, versions, last_updated
  owasp_crs_doctrine.json    # OWASP CRS doctrines (RCE, LFI, SQLi, scanner)
  gitleaks_doctrine.json     # Gitleaks secret doctrines
  semgrep_doctrine.json      # Semgrep command injection doctrines (P1)
  mcp_vectors_doctrine.json  # g8e-specific MCP/agentic threat doctrines
```

#### Doctrine Schema

Each doctrine file follows this canonical schema:

```json
{
  "source": "owasp_crs",
  "version": "4.0.0",
  "last_updated": "2026-05-22",
  "license": "Apache-2.0",
  "doctrines": [
    {
      "id": "owasp_crs_932100",
      "name": "RCE: nc -e reverse shell",
      "category": "reverse_shell",
      "severity": "critical",
      "pattern": "(?i)nc\\s+.*-e\\s+(/bin/)?(sh|bash|zsh)",
      "mitre_attack": "T1059.004",
      "mitre_tactic": "Execution",
      "confidence": 0.95,
      "enabled": true
    }
  ]
}
```

#### Doctrine Registry

The `doctrine_registry.json` tracks all doctrine sources:

```json
{
  "version": "1.0.0",
  "last_updated": "2026-05-22",
  "sources": [
    {
      "name": "owasp_crs",
      "url": "https://github.com/coreruleset/coreruleset",
      "version": "4.0.0",
      "license": "Apache-2.0",
      "enabled": true,
      "last_ingested": "2026-05-22"
    }
  ]
}
```

#### Industry Doctrine Sources

- **OWASP CRS** (P0): Apache-2.0 licensed WAF rules for RCE, LFI, SQLi, scanner detection
- **Gitleaks** (P0): MIT licensed secret detection (800+ pattern types)
- **Semgrep** (P1): Command injection patterns from bash/, python/, generic/ rule sets
- **secrets-patterns-db** (P1): CC0 licensed unified pattern database

#### MCP/Agentic-Specific Doctrines

g8e defines unique threat doctrines for agentic execution:

- Tool response injection
- Unsafe argument handling in MCP tools
- Prompt injection via tool outputs
- Credential exposure in tool responses
- GovernanceEnvelope field abuse
- MCP protocol misuse

### Quorum (L2Consensus): Consensus (Tribunal)

A cryptographic proof that an independent ensemble agreed on the instruction.

- **Mechanism** - Ed25519 signature over `transaction_hash | decision`.
- **Trust** - The Governed Operator maintains an Operator-owned `SignerStore`; missing or unknown keys cause rejection.
- **Producer** - Any conforming Quorum (L2Consensus) producer (the bundled **agentic ensemble**, a BYO multi-agent system, or a single signer for low-stakes flows).
- **Reference agentic ensemble producer** - The **agentic ensemble** (`g8ee`) runs its own internal Byzantine cascade upstream of the Quorum (L2Consensus) signature: Triage → Dash/Sage (intent articulation) → 5-member Tribunal generation → R1 vote → optional R2 anonymized peer review → Actuator risk analysis (Two-Strike Circuit Breaker) → Auditor verification + Merkle reputation commitment. The Ensemble signs only after Auditor passes. The Gateway gateway and operator do not assume any of this; they re-run every gate below independently. See [g8ee Governance & Safety](g8ee.md) and [position paper §2.3](position_paper.md).

### Notary (L3Notary): Authorization (Human)

Hardware-bound proof of human presence, except where policy explicitly permits auto-approval.

- **Web sessions (WebAuthn)** - Real WebAuthn/FIDO2 `L3Proof` (clientDataJSON, authenticatorData, signature) with the transaction hash as the assertion challenge. The user authenticates once with a passkey to establish a `web_session` (24-hour TTL). Within an authenticated session, the user can approve multiple mutations without re-authenticating. The L3 proof is per-transaction, but the session provides the authorization context.
- **CLI sessions (mTLS)** - CLI sessions authenticate via mTLS certificates with SPIFFE URI SANs (`spiffe://g8e.local/cli/<user_id>/<cli_session_id>`). The L3 proof for CLI sessions is the SHA-256 fingerprint of the mTLS certificate (`mtls_cert_fingerprint`). The verifier validates the certificate fingerprint, checks revocation status, expiry, and ensures the SPIFFE URI SAN matches the expected CLI session. CLI sessions do not require per-transaction re-authentication once the mTLS certificate is validated.
- **Auto-approval** - Benign diagnostic verbs (e.g., `uptime`, `df`) may be marked Notary (L3Notary)-authorized via policy. **Notary (L3Notary) auto-approval never bypasses Doctrine (L1Doctrine) or Quorum (L2Consensus).**

---

## Transaction Lifecycle

### Request Phase (Client → Operator)

1. Client builds a typed protobuf payload (e.g., `CommandRequested`).
2. Client embeds the payload in a `GovernanceEnvelope`, populating `nonce`, `expires_at`, and `state_merkle_root`.
3. The Quorum (L2Consensus) producer computes `transaction_hash` and attaches a Tribunal signature.
4. The Notary (L3Notary) actor (human) signs the same hash via WebAuthn.
5. Client submits canonical-JSON envelope over mTLS to the Governance Gateway (`g8eg`), which validates, records/suspends as needed, and dispatches to the target Governed Operator (`g8eo`) over secure mTLS WSS.

### Verification Phase (Gateway & Operator)

The `TransactionVerifier` on both `g8eg` and `g8eo` runs the following gates in order:

1. **Integrity** - `id == transaction_hash == SHA256(canonical_fields)`.
2. **Freshness** - `expires_at` not passed; `nonce` not in the replay store.
3. **State** - `state_merkle_root` matches local ledger root.
4. **Doctrine (L1Doctrine)** - Reflected `forbidden_patterns` over the typed payload + Sentinel threat analysis.
5. **Quorum (L2Consensus)** - Tribunal signature verified against the trusted `SignerStore`.
6. **Notary (L3Notary)** - WebAuthn `L3Proof` verified for mutations (or auto-approval policy applied after Doctrine (L1Doctrine)/Quorum (L2Consensus) pass).

### Execution & Receipt Phase (Operator → Client)

The **Actuator** on the Remote Operator signs an executing-state `ActionReceipt` and writes it to the AuditVault. If logging fails, execution is aborted.
2. The Actuator dispatches the typed payload to its execution handler (e.g., shell executor, file edit handler).
3. The Actuator updates the receipt with the final status (`COMPLETED` / `FAILED`), the post-state root, and a fresh signature.
4. The Remote Operator publishes a result envelope (also a `GovernanceEnvelope`) carrying the typed result and the signed receipt back to the Local Operator.

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

- `@/home/bob/g8e/services/g8eo/internal/constants/events.go` - string names
- `@/home/bob/g8e/protocol/proto/` - typed payload schemas
- `@/home/bob/g8e/services/g8eo/internal/constants/channels.go` - pub/sub channel prefixes

### Adding a new event

1. Add the string to `services/g8eo/internal/constants/events.go`.
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
- **Fail-Closed Execution**: If the `Actuator` service or `TransactionVerifier` is missing/nil, all inbound commands are rejected.

---

## Host Sovereignty & Audit

### Multi-Ledger Architecture
The Operator implements an isolated, git-based ledger for every session:
- **Isolation**: Each operator session owns a unique git repository at `.g8e/data/ledger/sessions/<id>/`.
- **Persistence**: Every file mutation is mirrored via a two-phase commit (`LedgerHashBefore` -> `LedgerHashAfter`).
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
| **Actuator** | Defense | Missed risk, Over-caution (blocking LOW) |

---

## Implementation Reference

| Concern | Authoritative file |
|---|---|
| Protobuf schemas | `@/home/bob/g8e/protocol/proto/` |
| Event registry | `@/home/bob/g8e/services/g8eo/internal/constants/events.go` |
| Channel prefixes | `@/home/bob/g8e/services/g8eo/internal/constants/channels.go` |
| Envelope types (Go) | `@/home/bob/g8e/services/g8eo/pkg/uap/types.go` |
| Verification logic | `@/home/bob/g8e/services/g8eo/internal/services/governance/transaction_verifier.go` |
| Audit storage | `@/home/bob/g8e/services/g8eo/internal/services/storage/audit_vault.go` |
| Workload identity | `@/home/bob/g8e/protocol/workload_identity.go` |

For the reference Remote Operator implementation see [Operator](operator.md). For the reference **g8e Agentic Ensemble** application see [g8e Agentic Ensemble](g8ee.md). For Hub/data-backplane behavior see [Local Operator (g8eg)](g8eg.md).
