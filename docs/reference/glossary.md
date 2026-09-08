---
title: Glossary
---

# g8e Glossary

Last Updated: 2026-09-08
Version: v2.1.7

Core terminology for the g8e Governance Suite, including the protocol, Governance Gateway, Governed Operator, g8ee ensemble, g8ed dashboard, compliance evidence, and MCP and A2A integrations. Terms are organized alphabetically.

---

## A2A (Agent2Agent)

A JSON-RPC protocol surface for agent-to-agent skill invocation. The Gateway converts an `a2a/call` request into a typed `A2ACallRequested` payload inside a **Governance Envelope**, processes it through the governance pipeline, and returns either a signed receipt or a structured suspension response containing an approval URL. A configured downstream A2A server can execute the admitted request.

---

## Action Receipt

The canonical protobuf `g8e.operator.v1.ActionReceipt`, signed by the **L5 Actuator** as evidence of an executing, completed, or failed transaction. It binds the transaction ID and hash, execution status and summary, state roots before and after execution, L2 and L3 status, deterministic stage evidence, signer identity, and signature. L5 signs and persists an `EXECUTING` form before dispatch, then signs and persists the final form and attaches a **Receipt Persistence Attestation**.

---

## Acting App ID

The `acting_app_id` field in a **Governance Envelope**. It identifies the application or tool acting as the user's delegate and complements `requestor_user_id`, which identifies the human authority.

---

## Actuator (L5 Actuator)

The singular execution boundary in the Operator substrate. After the **L4 Warden** returns a verified transaction, L5 persists a signed pre-execution **Action Receipt**, appends a signed **Commitment Attestation**, rehydrates scrubbed tokens locally, mints a just-in-time **Capability**, dispatches the action, dissolves the capability, and persists the signed final receipt and its persistence attestation. Signing, initial receipt persistence, or commitment persistence failure stops execution.

---

## Assessment Scope

A typed compliance record that fixes the deployment, organization, product version, build identity, source revision, network topology, cryptographic mode, component inventory, configuration hashes, doctrine bundle hashes, consensus policy hashes, trust anchors, and assessment time window to which evidence and conclusions apply.

---

## Audit Store

The append-only SQLite record of Operator sessions, events, file mutations, complete canonical **Action Receipts**, deterministic stage evidence, and persistence attestations. Sensitive content is encrypted through the local vault, records are scoped to valid Operator sessions, and retention rules prune eligible records. In gateway mode these tables are part of `g8e.db`; each remote Operator also retains its own authoritative local audit evidence.

---

## Bound State Root

The admission-gating **State Root** computed from all documents, active bound-tier KV and blob rows, and the token keymap hash when the scrubbing service is attached. It excludes cache entries, nonces, SSE events, and observed-tier rows so telemetry does not continuously invalidate in-flight envelopes.

---

## Canonical Gateway Database

The Gateway's SQLite database, `g8e.db`, opened in WAL mode and used by the document, KV, blob, audit, commitment-ledger, replay, and SSE-buffer services. It stores platform state such as users, organizations, sessions, operators, policies, consensus definitions, enrollment records, and revocations. It is not the only database in the system: the git-backed file ledger, execution vault, suspended-transaction store, and Operator-side stores remain separate where their lifecycle or sovereignty boundary requires it.

---

## Capability

A just-in-time, single-action permission minted by L5 from a verified transaction. It binds the transaction hash, action type, target resource, Operator identity and session, envelope expiry, a random single-use token, and the Actuator key identity. L5 places it in the execution context and dissolves it immediately after dispatch succeeds or fails; handlers can verify it before performing work.

---

## Command Intent

The pre-governance protobuf `g8e.common.v1.CommandIntent` published by an app workload to a bound Operator's command channel. It carries routing identity, action classification, typed Operator payload bytes, and application context. The Gateway validates the Operator-session binding, obtains the current state root, and converts the intent into a **Governance Envelope**; a command intent is not itself authorization to execute.

---

## Commitment Attestation

The canonical protobuf `g8e.operator.v1.CommitmentAttestation`. Before execution, the L5 Auditor signs a record binding the transaction, state root, action and target, digests of L2, Warden-intent, and human signatures, and the prior commitment hash. Its canonical SHA-256 hash becomes the link used by the next attestation.

---

## Commitment Ledger

The permanent SQLite hash chain of signed **Commitment Attestations**. L5 builds and appends each commitment against the current head while holding the database write lock, preventing concurrent writers from selecting the same predecessor. Compliance and reporting tools independently verify chain order, hashes, signatures, structured columns, and receipt cross-links. This is distinct from the git-backed **Ledger** used for file snapshots.

---

## Compliance Bundle

A portable, content-addressed package containing a canonical compliance report, assessments, evidence index, source artifacts, catalogs, crosswalks, trust material, and manifest. Bundle verification checks allowed paths, digests, signatures, references, scope, versions, and assessment consistency without trusting rendered Markdown, HTML, or terminal output.

---

## Consensus

A named L2 governance body defined by a `ConsensusPolicy`. Its enrolled members correspond to trusted signer identities, and the policy defines the member set, affirmative quorum, distinct-signature requirement, and enabled state. Members sign Ed25519 votes over `<transaction_hash>|<decision>`. L2 gates execution under `consensus` and `notary` postures and remains audited under `doctrine` and `ratify`.

---

## Control Assertion

A framework-neutral, atomic statement of technical behavior in the protocol compliance assertion catalog. Each assertion declares its scope, responsibility, applicable actions, required evidence and verifier types, minimum evidence level, validation cycle, missing-evidence policy, and passing rule. Typed crosswalks map these assertions to external framework controls without treating a supporting mapping as certification.

---

## Deterministic Stage Evidence

The canonical protobuf `g8e.operator.v1.DeterministicStageEvidence`, which records a typed governance or execution stage with monotonic timing, outcome, transaction and identity bindings, state roots, signer and signature digests, doctrine bundle identity, audit references, and parent-stage linkage. Current stage kinds cover L1, protocol L2, L3, L4 verification, receipt persistence, commitment append, and L5 execution.

---

## Encrypted KV Adapter

The adapter from the Gateway KV service to the token-store interface used by the scrubbing service. It namespaces and encrypts token values through the vault and writes them as observed-tier KV entries, excluding their encrypted storage rows from the bound state root. The deterministic in-memory token keymap hash is bound into the bound state root separately.

---

## Enrollment Token

A one-time token used to transfer passkey enrollment from the CLI to the browser without placing raw session identifiers in a URL. The Gateway generates 32 cryptographically random bytes, encodes them as hexadecimal, stores the token with the user and CLI session, and applies a five-minute expiry. The browser receives it in the URL fragment, and validation consumes it once. This differs from the token used by **Platform Enrollment**.

---

## Evidence Graph

The immutable, typed graph that connects assessment scope, control assertions, framework controls, content-addressed artifacts, verifier results, and assessments. Compliance graders and renderers consume this graph rather than scraping logs or inferring results from filenames.

---

## Evidence Level

The strength assigned to compliance evidence: L0 documented, L1 implemented, L2 deterministically evaluated, L3 demonstrated against a real stack, L4 continuously evidenced across an assessment window, and L5 externally attested. A result does not inherit a higher level merely from lower-level evidence.

---

## Execution Vault

A local SQLite service that stores command execution results and file diffs, links records to workflow identifiers, and records content hashes. It encrypts content through the local vault and then compresses it for storage, and it enforces configurable retention and database-size limits. Writes fail when content cannot be encrypted; reads log decryption or decompression failures and return the record without the unavailable decoded content.

---

## g8e Binary

The statically linked Go executable that runs either as a **Governance Gateway** (`g8e gw`) or a **Governed Operator** (`g8e operator`). The selected command determines the role; both roles use the same protocol and Operator substrate.

---

## g8e Protocol

The canonical wire contract and invariant set for g8e. It includes protobuf schemas, Go and Python protocol packages, JSON constant registries, JSON model schemas, doctrine definitions, SPIFFE workload-identity helpers, receipt verification, compliance types, and conformance vectors. Client-facing protobuf messages use protojson on the wire.

---

## g8ed

The first-party browser dashboard. It provides passkey authentication and operator-facing visibility while remaining outside the governance and execution authority. The browser authenticates directly to the Gateway with WebAuthn and a Gateway-issued HttpOnly session cookie; the dashboard workload uses its own enrolled app identity.

---

## g8ee

The first-party reference agentic ensemble. It converts user requests into governed intent, runs its multi-agent reasoning and Tribunal workflow, constructs conformant **Governance Envelopes**, and publishes typed progress and result events through the Gateway. It is a supported producer, not a required Gateway dependency.

---

## Gateway Peer

A workload identity reserved for Gateway-to-Gateway trust, with SPIFFE form `spiffe://g8e.local/gateway/<gateway_id>`. The PKI creates a dedicated Gateway-peer intermediate CA and includes it in the canonical trust bundle. These identity and certificate primitives support federated deployments; they do not by themselves establish a federation protocol or peer replication behavior.

---

## Governance Envelope

The canonical protobuf container `g8e.common.v1.GovernanceEnvelope` for governed mutations. It binds:

- **Identity**: `id`, timestamps, source component, Operator and session IDs, `requestor_user_id`, and `acting_app_id`.
- **Intent**: routing `event_type`, typed protobuf `payload`, structured `intent_data`, `action_type`, and `target_resource`.
- **State and replay defense**: `state_merkle_root`, `nonce`, `transaction_hash`, and `protocol_version`.
- **Governance**: L1, L2, and L3 metadata.
- **Application context**: case, investigation, task, system fingerprint, tenant, and binding persona.
- **Policy context**: the Gateway-selected governance `posture`.

Client-facing surfaces serialize the envelope with protojson. The deterministic transaction hash covers normalized intent and identity fields; posture is policy metadata and is not part of that hash.

---

## Governance Gateway

The platform coordinator and Policy Decision Point (PDP). It authenticates workloads, manages PKI, persists shared platform state, admits envelopes, runs L1 through L3 policy decisions, and brokers Operator pub/sub channels. For actions targeting the Gateway host, an in-process Operator substrate runs L4 and L5 locally. The Gateway does not hold a privileged path around the governance pipeline and does not initiate connections into remote Operators.

---

## Governance Posture

The startup-selected policy that determines whether L2 and L3 are fail-closed gates or audited results. L1 and universal integrity checks are enforced in every posture.

| Posture | L1 | L2 | L3 for mutations |
| --- | --- | --- | --- |
| **Doctrine** | Enforced | Audited | Audited |
| **Consensus** | Enforced | Enforced | Audited |
| **Ratify** | Enforced | Audited | Enforced |
| **Notary** | Enforced | Enforced | Enforced |

Read-only actions do not require L3 in any posture. The Gateway stamps the posture into each envelope, and the L4 Warden treats that envelope field as authoritative during verification.

---

## Governed Operator

The Policy Execution Point (PEP) deployed on a target host. It opens no inbound service port, establishes an outbound-only mTLS connection to the Gateway, pulls work from its session-specific channel, and re-runs L1 and re-verifies L2 and L3 locally before L4 and L5. It is the only platform component authorized to mutate its host and retains host-local audit and file-history evidence.

---

## Heartbeat

Periodic health telemetry sent by a Governed Operator to the Gateway. The typed payload reports identity, uptime, operating system and architecture, CPU, memory and disk details, network information, environment details, capability flags, and fingerprint details. The Gateway uses it to monitor Operator status.

---

## KSI (Key Security Indicator)

A typed, evaluable security requirement used by the compliance subsystem, particularly the FedRAMP 20x catalog. A KSI declares methods, evidence requirements, certification class, and validation cycle; the evaluator derives status from registered methods and typed evidence rather than prose claims.

---

## L1 Doctrine (L1Doctrine)

The technical hard-gate layer. It validates decoded typed payloads against protobuf forbidden-pattern options, bundled JSON doctrine sets, and deterministic command and MCP-argument threat analysis, including MITRE ATT&CK-oriented patterns. L1 is enforced in every posture and is re-run on the Operator before execution.

---

## L2 Consensus (L2Consensus)

The multi-signature governance layer. Each enrolled member produces an Ed25519 vote over `<transaction_hash>|<decision>`, and L4 verifies signer trust, policy membership, signature validity, distinctness, and affirmative quorum. L2 is required only by `consensus` and `notary`; under `doctrine` and `ratify`, available L2 results are recorded without gating execution.

---

## L3 Notary (L3Notary)

The human-authorization layer for mutations under `ratify` and `notary` postures. Gateway mode verifies a WebAuthn passkey assertion; CLI-originated proofs additionally bind the authenticated CLI session and certificate fingerprint. Outbound mode verifies an approved suspended transaction and its Ed25519 signature over the transaction hash. Read-only actions do not require L3, and `doctrine` and `consensus` audit L3 without requiring it.

---

## L4 Warden (L4Warden)

The fail-closed pre-dispatch verifier on the Operator substrate. It validates envelope structure and typed payload/action agreement, reserves the nonce, checks expiry, recomputes and compares both transaction ID and hash, re-runs L1, verifies the current state root, and applies posture-dependent L2 and L3 checks. It returns a verified transaction and deterministic evidence to L5; it does not execute the action.

---

## L5 Actuator (L5Actuator)

See **Actuator**.

---

## Ledger

The git-backed file-mutation history maintained by the Governed Operator. Each Operator session uses an isolated repository beneath the runtime ledger directory. A governed file mutation snapshots and commits the pre-mutation state, performs the operation, then commits the post-mutation state and records the before and after hashes, diff stat, and diff content. The ledger supports history queries, point-in-time retrieval, and restoration. It is distinct from the SQLite **Commitment Ledger**.

---

## Local-First Audit Architecture (LFAA)

The architecture in which each target host remains authoritative for raw execution evidence, file history, and the effects applied to that host. The local audit store, commitment ledger, execution vault, and git-backed ledger retain the evidence needed to reconstruct and verify execution. The Gateway is not entirely stateless: it persists platform coordination state and, in gateway mode, local or relayed receipt evidence; sovereignty depends on raw sensitive content remaining on the Operator and only scrubbed results and cryptographic commitments crossing the boundary.

---

## MCP (Model Context Protocol)

The JSON-RPC tool protocol exposed by g8e for compatible AI clients. The Gateway supports tool discovery and calls, converts governed calls into typed Operator protobuf payloads and **Governance Envelopes**, and returns structured results or an approval suspension. Native Operator tools and configured downstream MCP servers execute only after governance admission.

---

## Mutual TLS (mTLS)

TLS authentication in which both peers present and verify certificates. g8e uses mTLS and SPIFFE URI SANs to authenticate workload and session identities, encrypt transport, bind callers to envelope claims, and enforce certificate revocation. Binary provenance is a separate build-signing concern; mTLS authenticates the communicating workload, not the executable file itself. TLS 1.3 is required on protected transport surfaces.

---

## Observed-State Root

A separate SHA-256 commitment over active observed-tier KV and blob rows. It does not gate transaction admission, so telemetry and environmental readings do not make in-flight envelopes stale. Audit evidence can chain this root to make observations tamper-evident.

---

## Operator Session

The execution and authorization context of a running Governed Operator, identified by `operator_session_id`. It scopes the Operator workload identity, pub/sub channel, audit records, execution-vault records, and session-specific git ledger.

---

## OSCAL (Open Security Controls Assessment Language)

The NIST machine-readable format supported by the compliance reporting subsystem. g8e projects its canonical typed assessment record into OSCAL assessment results and validates the output against the supported OSCAL 1.1.2 schema; OSCAL is an output representation, not an independent source of assessment decisions.

---

## PKI (Public Key Infrastructure)

The Gateway-managed certificate hierarchy for protected platform communication. It creates root and intermediate authorities, issues workload certificates carrying SPIFFE URI SANs, publishes trust and revocation bundles, and supports Operator, CLI, app, user, hub, and Gateway-peer identities. Private keys for CSR-based enrollment are generated and retained by the enrolling workload.

---

## Platform Enrollment

The owner-approved protocol for enrolling dashboard, ensemble, and Operator component instances. A component generates distinct app, Operator, and CLI keys as required, submits CSRs and fingerprints, and receives an approval or denial decision. To complete an approved request, the component proves possession of every CSR private key by signing a canonical transcript that binds the protocol version, request, token hash, component identity, and key fingerprints; the Gateway verifies those signatures before issuing certificates.

---

## Principal

The human, AI agent, application, CI/CD workflow, or scheduled process that originates an intent. g8e governs the requested action rather than granting a trusted actor a bypass; the authenticated producer and any human delegator remain explicit in the envelope.

---

## Producer

A client or service that translates a principal's intent into a conformant **Governance Envelope**. Producers include the Gateway's MCP and A2A translators, g8ee, and compatible native integrations. Producing an envelope does not authorize execution; the Gateway and bound Operator still verify it.

---

## Receipt Persistence Attestation

The canonical protobuf `g8e.operator.v1.ReceiptPersistenceAttestation`, signed after the final receipt is durably stored. It binds the transaction ID, digest of the final receipt signature, persistence timestamp, audit record ID, signer key ID, and signature. Receipt relays verify both the receipt signature and this attestation.

---

## Replay Protection

The nonce, expiry, transaction-hash, and in-flight controls that prevent a captured or concurrent transaction from executing again. L4 reserves each nonce atomically in durable storage before stateful verification, rejects duplicates and expired envelopes, and releases the reservation when verification fails.

---

## Reputation Commitment

A g8ee Auditor record that binds a Tribunal verdict to the current agent-reputation scoreboard. It contains a SHA-256 Merkle root over sorted `(agent_id, scalar)` leaves, the preceding reputation root, leaf count, verdict identity, and an Auditor HMAC-SHA256 signature. This ensemble reputation chain is distinct from the L5 **Commitment Ledger**.

---

## Reputation Staking

The g8ee mechanism that updates each agent's reputation scalar in the range 0.0 through 1.0 using an exponential moving average after outcomes resolve. Typed stake-resolution records capture rewards, slash tiers, and unbonding state; reputation affects ensemble influence, not L2 cryptographic signature validity in the platform Warden.

---

## Requestor User ID

The `requestor_user_id` field in a **Governance Envelope**. It identifies the human delegator who authorized the action and pairs with `acting_app_id`, which identifies the delegated application or tool.

---

## Scrubbed Vault

The `scrubbed` storage mode for execution and file-operation results whose sensitive content has passed through the **Sovereign Execution Boundary** before persistence. Sensitive values can become reversible UEI tokens or non-reversible typed redactions, depending on the detector and policy. `raw` mode stores the unsanitized local forensic form and does not make that content safe for model or cloud access.

---

## Sovereign Execution Boundary

The Operator-local scrubbing and rehydration boundary implemented by the `ScrubbingService` and L5 integration. Egress scrubbing removes or tokenizes sensitive content before it leaves the host; persistent token mappings are encrypted; the token keymap hash can participate in the bound state root; and L5 rehydrates reversible tokens only on the execution host immediately before dispatch.

---

## SSE (Server-Sent Events)

The Gateway event surface for real-time delivery from app workloads and internal Gateway producers to browser and CLI sessions. Apps push authenticated, session-scoped events with `POST /api/v1/sse/push`; consumers poll `/api/v1/sse/events` or stream `/api/v1/sse/stream` using mTLS or a web-session cookie. Approval and passkey completion events use this path. Governed Operator command transport uses pub/sub rather than SSE.

---

## State Root

A deterministic SHA-256 digest representing the current authoritative bound state used for transaction freshness. The Gateway's `StateRootService` hashes documents, active bound-tier KV and blob rows, and the scrubbing token keymap hash when configured. The envelope carries this value in `state_merkle_root`, and L4 rejects a mismatch with the current authoritative root. See also **Bound State Root** and **Observed-State Root**.

---

## State Tier

The classification that controls whether a Gateway KV or blob row participates in transaction freshness:

- **Bound** (`state_tier='bound'`): authoritative state included in the admission-gating root.
- **Observed** (`state_tier='observed'`): telemetry or evidence excluded from the bound root and hashed into the separate observed-state root.

Documents are authoritative and always participate in the bound root; the `state_tier` column applies to KV and blob rows.

---

## Suspended Transaction

A Governance Envelope paused while it awaits required L3 authorization. The suspended-transaction store retains the envelope, approval status, proof material, expiry, and caller binding. After successful approval, the Gateway resumes governance processing and removes the record after execution; expired records are pruned.

---

## System Fingerprint

A stable SHA-256 host identifier derived from operating system, architecture, CPU count, machine identifier, and hostname. It supports Operator identification and duplicate detection and is carried in Operator records and envelope context.

---

## Time-Travel

Point-in-time file retrieval and restoration using the git-backed **Ledger**. A caller can inspect file history or restore a governed file from a selected ledger commit; this term does not refer to rewriting the SQLite commitment chain.

---

## Tool Calling Loop

The client pattern in which an AI model selects an MCP tool or A2A skill, submits arguments, consumes the governed result or approval state, and chooses the next call. Each state-changing iteration remains a separate governed transaction rather than inheriting authorization from the surrounding conversation.

---

## Transaction Hash

The deterministic SHA-256 digest computed from normalized **Governance Envelope** fields. The envelope `id` and `transaction_hash` must both equal the computed value. L2 votes, L3 authorization, replay checks, capabilities, receipts, and commitment evidence bind to this hash. Policy metadata such as the envelope posture is excluded from hash canonicalization.

---

## Vault

The local encryption service and key hierarchy used by storage and scrubbing components. Protected data uses AES-256-GCM, and services that require encryption fail closed when the vault is unavailable or locked rather than falling back to plaintext. The encryption vault is distinct from the **Execution Vault**, which is a SQLite data store.

---

## Workload Identity

A SPIFFE-style identity encoded in a certificate URI SAN under the `g8e.local` trust domain. Current formats are:

- Operator: `spiffe://g8e.local/operator/<organization_id>/<operator_id>/<operator_session_id>`
- CLI: `spiffe://g8e.local/cli/<user_id>/<cli_session_id>`
- App: `spiffe://g8e.local/app/<operator_id>`
- User: `spiffe://g8e.local/user/<user_id>`
- Hub: `spiffe://g8e.local/hub/operator-listen`
- Gateway peer: `spiffe://g8e.local/gateway/<gateway_id>`

The transport and application layers match these identities to sessions, roles, and envelope claims.
