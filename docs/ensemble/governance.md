# Governance

## Overview

The g8e platform enforces zero-trust autonomous infrastructure management through a five-layer verification pipeline (L1 through L5). AI clients and agentic ensembles are treated as untrusted principals: they formulate operational intent, but cannot directly execute mutations against target hosts. All state changes, tool executions, and data modifications must be encapsulated in typed, deterministic `GovernanceEnvelope` transactions and pass through fail-closed verification gates before execution.

The governance architecture divides responsibilities between two primary components:

- **Governance Gateway (Policy Decision Point / PDP)** — The central coordinator that enforces transport security, manages PKI, runs L1 Doctrine validation, brokers L2 Consensus deliberation, and manages L3 Notary human-in-the-loop authorization.
- **Governed Operator (Policy Execution Point / PEP)** — The host-side agent that maintains local sovereignty, re-verifies all proofs, runs the L4 Warden pre-dispatch gate, and executes actions within the isolated L5 Actuator boundary.

The g8e Agentic Ensemble (`g8ee`) acts as a first-party producer within this framework. Internally, `g8ee` runs an multi-agent generation and auditing pipeline (Tribunal, Auditor, and Warden risk analyzers) to synthesize and sanitize commands. Externally, `g8ee` dispatches host-level operations via `CommandIntent` messages and executes platform mutations directly via `GovernanceClient` using signed `GovernanceEnvelope` transactions.

## The Five-Layer Verification Pipeline

Every mutation passing through the platform traverses the five-layer interlock sequence in strict order. Universal checks and proofs required by the active posture fail closed: any failed required validation check or missing required proof immediately rejects the transaction and releases any reserved nonces. Optional L2 and L3 results remain audit evidence and do not gate execution.

```
L1 Doctrine (Bedrock) → L2 Consensus → L3 Notary → L4 Warden → L5 Actuator
```

### L1: Technical Bedrock (Doctrine)

L1 Doctrine is the deterministic, non-negotiable hard gate enforced via protobuf reflection and pattern-matching engines. It operates before any consensus deliberation or human authorization:

- **Pattern Matching and Rule Sets** — Evaluates execution payloads against compiled blacklist patterns, whitelist rules, OWASP Core Rule Set (CRS) definitions, and MCP vector rules.
- **MITRE ATT&CK Threat Detection** — Detects malicious behaviors including reverse shell patterns, privilege escalation heuristics, unsanctioned network listeners, and destructive disk commands.
- **Stateless Validation** — Enforces action type schemas, required fields, and structural payload integrity.
- **Enforcement Scope** — Enforced as a mandatory fail-closed gate in every governance posture. The Governed Operator re-evaluates L1 Doctrine locally before execution to prevent gateway tampering.

### L2: Multi-Agent Consensus

L2 Consensus verifies cryptographic multi-signature approval over the transaction payload. It ensures that critical actions receive distributed consensus before reaching human review or execution:

- **Tribunal Evaluation** — Enrolled consensus members independently evaluate the envelope payload against doctrine and policy rules.
- **Ed25519 Vote Signing** — Each member signs an Ed25519 vote over the canonical transaction hash and decision string: `<transaction_hash>|<decision>`.
- **Quorum Enforcement** — The Gateway and Operator verify that affirmative votes meet the configured quorum threshold (`K-of-N`) from distinct, trusted keys enrolled in the signer store.
- **Postures** — Enforced as a fail-closed requirement under `consensus` and `notary` postures; verified and recorded as an audited proof under `doctrine` and `ratify` postures.

### L3: Notary Authorization

L3 Notary provides human-in-the-loop authorization and cryptographic session binding for state-changing operations:

- **WebAuthn / FIDO2 Passkeys** — In gateway browser sessions, human operators provide hardware-bound cryptographic assertions over the transaction hash to authorize high-risk mutations.
- **mTLS Transport Proofs** — In CLI and agent workloads, L3 notary proofs are bound to the SHA-256 certificate fingerprint (`mtls_cert_fingerprint`) of the authenticated client mTLS transport certificate.
- **Suspended Transaction Flow** — When an L3 proof is required but absent, the Gateway suspends the transaction, issues an out-of-band approval challenge URL, and resumes processing once approved.
- **Mutation Scoping** — Enforced for state-changing mutations under `ratify` and `notary` postures. Read-only actions (such as filesystem inspection or log queries) do not require L3 authorization.

### L4: Warden Pre-Dispatch Gate

The L4 Warden runs on the Governed Operator (and in-process on the Gateway for local actions) as the final pre-dispatch verification gate:

- **In-Flight Tracking** — Rejects concurrent transaction submissions bearing the same nonce to prevent double-spend attacks.
- **Nonce Reservation and Expiry** — Atomically reserves the unique 32-byte nonce in durable storage and validates that the transaction has not expired (`expires_at > now_utc`).
- **Transaction Hash Integrity** — Recomputes the deterministic SHA-256 transaction hash across canonical fields and verifies an exact match with the envelope `id`.
- **State Merkle Root Verification** — Compares the envelope's `state_merkle_root` against the operator's current local state ledger, rejecting stale transactions (`TX_STATE_MISMATCH`).
- **Posture-Gated Proof Evaluation** — Enforces L2 Consensus and L3 Notary verification according to the active `GovernancePosture`.

### L5: Actuator Execution Boundary

The L5 Actuator is the singular execution boundary authorized to interact with the underlying host:

- **Initial Receipt Signing** — Signs an initial `ActionReceipt` with status `EXECUTING` and commits it to the local audit vault before executing any instruction; execution halts if signing fails.
- **PII Scrubbing and Rehydration** — Sensitive credentials and host parameters scrubbed at the gateway boundary are rehydrated strictly inside the local operator vault using host-local keys.
- **Just-in-Time Capability Minting** — Mints a short-lived, single-action capability scoped precisely to the transaction hash and dissolves it immediately upon completion.
- **Tool Dispatch** — Executes the requested tool inside a memory-safe, isolated execution boundary.
- **Final Receipt Production** — Mints a final signed `ActionReceipt` containing execution results, exit codes, state root transitions (`state_root_before`, `state_root_after`), and L2/L3 governance audit status, signed with the operator's private Ed25519 actuator key.

## Governance Postures

Platform governance enforcement is configured at startup via the `--posture <doctrine|consensus|ratify|notary>` flag. Postures define which verification layers operate as blocking fail-closed gates versus audited proofs.

| Posture | L1 Doctrine | L2 Consensus | L3 Notary | Typical Use Case |
| --- | --- | --- | --- | --- |
| **`doctrine`** | Enforced | Audited | Audited | Local development, test environments, and automated CI pipelines |
| **`consensus`** | Enforced | Enforced | Audited | Multi-agent automated operations requiring distributed consensus |
| **`ratify`** | Enforced | Audited | Enforced (mutations) | Human-authorized operations without L2 consensus |
| **`notary`** | Enforced | Enforced | Enforced (mutations) | Production environments requiring consensus and human oversight for host mutations |

Universal baseline checks—including L1 Doctrine, deterministic transaction hash verification, nonce replay defense, expiry validation, state Merkle root integrity, action type validation, and payload decoding—fail closed across all postures.

## Ensemble Pre-Governance Pipeline

Before any command reaches the platform governance pipeline, `g8ee` runs an internal multi-agent generation, consensus, and risk assessment gauntlet.

### 1. Intent vs. Command Separation

Reasoning agents such as Sage formulate high-level intent (`SageOperatorRequest`) without proposing raw shell syntax. This separation ensures that strategic planning cannot inject arbitrary or unsafe shell constructs directly into execution pipelines.

### 2. Tribunal Consensus Generation

The natural-language intent is dispatched to the 5-member Tribunal (`role="arbitrator"`). All five members evaluate the intent independently and in parallel under strict Information Isolation:

- **Axiom (`axiom`)** — Composition lens focusing on cohesive multi-stage shell pipelines.
- **Concord (`concord`)** — Safety lens prioritizing defensive flags, explicit paths, and fail-safe pipeline chaining.
- **Variance (`variance`)** — Edge-case lens handling environment hazards, file path spaces, null delimiters, and locale boundaries.
- **Pragma (`pragma`)** — Convention lens selecting idiomatic modern tooling (`ss`, `journalctl`, `kubectl`).
- **Nemesis (`nemesis`)** — Adversary lens proposing stress-testing edge cases or cleanly abstaining when commands are sound.

### 3. Weighted Voting and Peer Review

Candidate command strings are clustered and evaluated using `weighted_vote()`:

- **Threshold Enforcement** — Requires a consensus strength meeting `TRIBUNAL_MIN_CONSENSUS` (minimum 3 agreeing members).
- **Round 2 Peer Review** — If consensus strength is insufficient in Round 1, an anonymized summary of candidate clusters is distributed for a second round of peer review.
- **Consensus Failure** — If agreement cannot be reached after Round 2, the pipeline fails closed with `TribunalConsensusFailedError`.

### 4. Machine-Domain Auditor

The Auditor (`auditor`, `model_tier="primary"`) reviews the winning candidate cluster against Sage's intent, operator context, whitelists, and blacklists. The Auditor stakes reputation and returns one of three verdicts:

- `ok` — Approves the candidate command unchanged.
- `revised:<command>` — Corrects syntax errors, missing flags, or whitelist constraints.
- `swap:<cluster_id>` — Selects a superior dissenting candidate cluster.

### 5. LLM Risk Filter (Warden Sub-Agents)

Three specialized risk analyzers evaluate operational blast radius and stake reputation on their assessments:

- **`warden_command_risk`** — Evaluates shell command blast radius, reversibility, and system impact (`LOW`, `MEDIUM`, `HIGH`).
- **`warden_file_risk`** — Assesses file mutation risks based on target path sensitivity and Git tracking state.
- **`warden_error`** — Classifies execution failures into recovery strategies (`AUTO_FIXABLE`, `ESCALATE`, `RETRY_LIMIT`).

Ambiguous or indeterminate risk evaluations fail closed to `HIGH` risk.

## Transaction Dispatch Mechanisms

`g8ee` interacts with the g8e platform through two distinct dispatch paths depending on the operational target.

### Operator Command Intent Dispatch

For host-side actions (shell execution, file reads, file edits, directory listings):

1. `g8ee` packages the verified instruction into a protobuf payload (`CommandRequested`, `FileEditRequested`, `FsReadRequested`, etc.) and encodes it as base64 ASCII.
2. `g8ee` publishes a `CommandIntent` model to the operator's dedicated pub/sub channel: `cmd:<operator_id>:<operator_session_id>`.
3. The Gateway consumes the `CommandIntent`, verifies the operator session binding, fetches the current state Merkle root, constructs the canonical `GovernanceEnvelope`, and routes it through L1–L3 gates.
4. The governed envelope is published to the operator's command queue for L4 Warden validation and L5 Actuator execution.

### Direct Governance Envelope Submission

For platform-level state mutations (creating or updating cases, investigations, memories, and reputation records):

1. **State Root Acquisition** — `GovernanceClient` (`app/clients/governance_client.py`) fetches the current state Merkle root from the Gateway (`GET /healthz`).
2. **Deterministic Construction** — Builds a `GovernanceEnvelope` containing normalized timestamps, a random 32-byte nonce, base64-encoded protobuf payload, and the computed transaction hash.
3. **Transport Proof Binding** — Binds the mTLS client certificate SHA-256 fingerprint into the L3 notary metadata.
4. **Submission and State Retry** — Posts the envelope to `POST /api/v1/governance/envelopes`. If a concurrent transaction changes the state root before submission, producing a `TX_STATE_MISMATCH` (HTTP 403), `GovernanceClient` automatically re-fetches the updated state root and retries up to three times (`_STATE_ROOT_MAX_RETRIES`).
5. **Receipt Verification** — Upon successful execution, `GovernanceClient` verifies the Ed25519 signature on the returned `ActionReceipt` against the platform actuator public key (`verify_receipt_signature()`).

## Deterministic Transaction Hashing

The `GovernanceEnvelope.id` must match the deterministic SHA-256 hash computed over canonicalized fields in strict protocol order:

```
action_type|target_resource|payload|state_merkle_root|nonce|expires_at|intent_data|requestor_user_id|acting_app_id|
```

The hashing algorithm adheres to strict canonicalization rules:

- **Field Omission** — Empty strings, `None` values, or empty dictionaries are omitted entirely without placeholder tokens or trailing separators.
- **Pipe Separation** — Exactly one pipe delimiter (`|`) is appended after each present field.
- **Timestamp Normalization** — `expires_at` is normalized to fixed 6-digit microsecond UTC format (`YYYY-MM-DDTHH:MM:SS.ffffffZ`).
- **Intent Canonicalization** — `intent_data` dictionaries are serialized into sorted, comma-delimited `key=value` strings.
- **L3 Proof Exclusion** — L3 notary metadata is intentionally excluded from the transaction hash so that L2 consensus members can sign the payload before human authorization is requested.

## Security and Sovereignty Guarantees

- **Fail-Closed by Design** — Every verification layer, risk analyzer, and cryptographic check fails closed on errors or ambiguous data.
- **Host Sovereignty** — Raw operational data and detailed audit logs remain on the sovereign host. The gateway and external clients receive only sanitized, signed execution receipts and Merkle root commitments.
- **Zero Standing Privileges** — The operator possesses no permanent execution privileges; execution capabilities are minted just-in-time per transaction hash and dissolved immediately upon completion.
- **Cryptographic Audit Trail** — Every executed mutation produces a hash-chained audit record and a signed `ActionReceipt` verifiable with Ed25519 public keys.
- **Workload Identity Binding** — All network communications require mutual TLS, and the Gateway verifies that envelope identity claims match the SPIFFE URI SANs embedded in client transport certificates.

## Related

- [Agents](agents.md) — Multi-agent roster, persona models, and Tribunal consensus roles
- [Architecture](architecture.md) — Platform components, protocol surfaces, and model hierarchy
- [Protocol](protocol.md) — Canonical `GovernanceEnvelope` schemas, dispatch models, and hashing specifications
- [Thinking](thinking.md) — Provider reasoning tokens and cryptographic thought signatures
- [PKI & Trust](pki.md) — Public Key Infrastructure, trust bundles, and workload enrollment
- [Storage](storage.md) — State Merkle roots, local audit vaults, and data sovereignty
- [Evals](evals.md) — Benchmark evaluation suites and Judge scoring rubrics
- [Constants](constants.md) — Protocol constant registries and action types
