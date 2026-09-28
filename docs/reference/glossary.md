---
doc_id: glossary
title: g8e Glossary
audience: developers, maintainers, and coding agents
status: current
last_updated: 2026-09-28
version: v2.2.3
owners:
  - protocol/
  - internal/services/
  - internal/operator/
  - ensemble/
related:
  - ../architecture/gateway.md
  - ../architecture/operator.md
  - ../architecture/governance.md
  - ../devs/codemap.md
when_to_read: Looking up core terminology, cryptographic bindings, runtime boundaries, and protocol contracts across the g8e Governance Suite.
do_not_use_for:
  - Developer and coding invariants (docs/devs/devs.md)
  - Package and runtime ownership maps (docs/devs/codemap.md)
  - Step-by-step test execution (docs/devs/tests.md)
---

# g8e Glossary

Last Updated: 2026-09-28
Version: v2.2.3

Core terminology for the g8e Governance Suite, including the protocol, Governance Gateway, Governed Operator, g8ee ensemble, g8ed dashboard, compliance evidence, and MCP and A2A integrations. Terms are organized alphabetically. The Gateway and Operator entries describe separate runtime boundaries; see [Gateway Architecture](../architecture/gateway.md) and [Operator Architecture](../architecture/operator.md) for deployment-specific ownership.

---

## A2A (Agent2Agent)

A JSON-RPC protocol surface for agent-to-agent skill invocation exposed at `POST /api/v1/a2a/call`. The Gateway converts an `a2a/call` request into a typed `g8e.operator.v1.A2aCallRequested` (`A2ACallRequested`) protobuf payload, associates it with action type `A2A_CALL` and event type `g8e.v1.operator.a2a.call.requested`, wraps it inside a **Governance Envelope**, and processes it through the governance pipeline. It returns either a signed receipt or a structured suspension response containing an approval URL (`/approval/:txHash`). When admitted under the active governance posture, the request dispatches to the configured downstream A2A server.

---

## Acting App ID

The `acting_app_id` field in a **Governance Envelope**. It identifies the application, tool, or agent acting as the user's delegate (typically a SPIFFE URI such as `spiffe://g8e.local/app/<operator_id>` or `spiffe://g8e.local/app/g8ee`) and complements `requestor_user_id`, which identifies the human authority.

---

## Action Receipt

The canonical protobuf `g8e.operator.v1.ActionReceipt`, signed with Ed25519 by the **L5 Actuator** as evidence of an executing, completed, or failed transaction. It binds the transaction ID and hash, execution status (`EXECUTION_STATUS_EXECUTING`, `EXECUTION_STATUS_COMPLETED`, or `EXECUTION_STATUS_FAILED`), failure code (`ReceiptFailureCode`), result summary, state roots before and after execution, execution duration and completion timestamp, signer key ID, signature, L2 and L3 verification status, deterministic stage evidence, and persistence attestation. L5 signs and persists an initial `EXECUTING` receipt to the SQLite audit store before dispatch (failing closed on signing or storage error), then signs and persists the final receipt and attaches a **Receipt Persistence Attestation** upon completion.

---

## Actuator (L5 Actuator)

The singular execution boundary in the Operator substrate (`internal/services/governance/l5_actuator.go`). After the **L4 Warden** returns a verified transaction, L5 executes a fail-closed sequence: it signs and persists an initial `EXECUTING` **Action Receipt**, appends a signed **Commitment Attestation** to the SQLite **Commitment Ledger**, rehydrates scrubbed tokens locally through the **Sovereign Execution Boundary**, mints a just-in-time single-action **Capability**, dispatches the action to the registered execution handler, captures deterministic stage evidence, dissolves the capability, finalizes and signs the final receipt, and persists a signed **Receipt Persistence Attestation**. Signing, initial receipt persistence, or commitment persistence failure halts execution immediately.

---

## Assessment Scope

A typed compliance record defined in `g8e.compliance.v1.AssessmentScope` that fixes the deployment, organization, product version, build identity, source revision, image digests, component inventory, network topology hash, configuration hashes, doctrine bundle hashes, consensus policy hashes, trust anchors, cryptographic mode, customer responsibilities, and assessment time window to which evidence and conclusions apply.

---

## Audit Store

The append-only SQLite record (`internal/services/storage/audit_store.go`, `SQLAuditStore`) of Operator sessions, events, file mutations, complete canonical **Action Receipts**, deterministic stage evidence, and persistence attestations. Sensitive fields (`content_text`, `command_stdout`, `command_stderr`) are encrypted at rest using AES-256-GCM through the local vault (`vault.Vault`), records are scoped to valid Operator sessions, and retention rules prune eligible records based on size and age limits. In Gateway mode, these tables are part of `g8e.db`; each remote Governed Operator retains its own authoritative local audit database.

---

## Bound State Root

The admission-gating **State Root** computed by `StateRootService.calculateStateRoot()`. It is derived from a deterministic SHA-256 Merkle aggregation over: all authoritative `documents` rows (`collection`, `id`, `data` ordered by `collection, id`), active bound-tier `kv_store` entries (`state_tier = 'bound'`, excluding ephemeral cache keys prefixed with `g8e:cache:*`, ordered by `key`), active bound-tier `blobs` entries (`state_tier = 'bound'`, ordered by `namespace, id`), and the token keymap hash provided by `KeymapHashProvider` when the scrubbing service is attached. It explicitly excludes ephemeral caches, nonces, SSE events, and observed-tier rows (`state_tier = 'observed'`) so telemetry does not continuously invalidate in-flight envelopes.

---

## Canonical Gateway Database

The Gateway's primary SQLite database, `g8e.db`, opened in WAL mode via `sqliteutil` and managed by `CanonicalDBService`. It embeds the canonical schema from `internal/services/gateway/db/schema.sql` and stores platform state across `documents`, `kv_store`, `blobs`, `state_root`, `state_version`, `nonces`, and `sse_events`. It is distinct from the git-backed file **Ledger**, the **Execution Vault** (`execution_vault.db`), the **Suspended Transaction Store** (`suspended_transactions.db`), and remote Operator audit databases, each of which maintains an independent lifecycle and sovereignty boundary.

---

## Capability

A just-in-time, single-action, self-dissolving permission minted by the L5 Actuator (`internal/services/governance/capability.go`) from a verified transaction. It binds the transaction hash, action type, target resource, Operator identity and session, envelope expiry, a random 32-byte single-use token, and the Actuator key ID. The token and transaction hash are signed with the Actuator's private key. L5 places the capability into the Go execution context via `ContextWithCapability` and dissolves it immediately after dispatch completes or fails; handlers verify it before performing operations.

---

## CLI Operator Session Binding

The persisted pair of `operator_session_id` and `operator_id` stamped on an authenticated CLI session. Running `g8e operator bind <operator-session-id>` pins the CLI session to an active operator session owned by the same user via `POST /api/v1/auth/cli/bind`; `g8e operator bind list` inspects bindings; `g8e operator bind unbind` clears the binding via `POST /api/v1/auth/cli/unbind`. Binding changes issue a replacement CLI session server-side and update local credentials. Session refresh, rotation, and recovery inherit the prior binding when present. The unified auth middleware derives operator identity from the session record and rejects contradictory request headers.

---

## Command Intent

The deprecated pre-governance protobuf `g8e.common.v1.CommandIntent`. Prior to v2.1.14, enrolled applications could publish this shape to an Operator's `cmd:` channel; the Gateway validated the Operator-session binding, obtained the current state root, and converted the intent into a **Governance Envelope**. That WebSocket publish path was removed in v2.1.14 to ensure all mutations enter through validated ingress. Enrolled applications now dispatch host operations through `POST /api/v1/operators/commands` with a registered request `event_type` and serialized protobuf payload bytes, or submit pre-formed envelopes to `POST /api/v1/governance/envelopes`.

---

## Commitment Attestation

The canonical protobuf `g8e.operator.v1.CommitmentAttestation`. Before execution, the L5 Auditor signs a record binding the transaction ID and hash, state root at commit, action type, target resource, canonical digests of L2 consensus votes, Warden intent signatures, and human Notary signatures, the prior commitment hash, committed timestamp, and Auditor key ID. Its canonical SHA-256 hash becomes the cryptographic link chained by the next attestation.

---

## Commitment Ledger

The append-only SQLite hash chain of signed **Commitment Attestations** (`internal/services/storage/commitment_ledger.go`, table `commitment_ledger`). L5 builds and appends each commitment against the current chain head while holding the database write lock, preventing concurrent writers from selecting the same predecessor. Compliance and reporting tools independently verify chain order, hashes, signatures, structured columns, and receipt cross-links. This is distinct from the git-backed **Ledger** used for file history.

---

## Compliance Bundle

A portable, content-addressed release package (`internal/services/compliance/report/bundle.go`) containing a manifest (`manifest.json`), canonical compliance report formats (JSON, OSCAL, Markdown, HTML, CSV, CLI), assertion assessments, framework control assessments, evidence index, source artifacts, catalogs, crosswalks, checksum roots, and cryptographic signatures. Verification (`VerifyComplianceReportBundle`) independently checks allowed paths, digests, signatures, references, scope, versions, and assessment consistency without trusting rendered text or terminal output.

---

## Consensus

A named L2 governance body defined by a `ConsensusPolicy` (`internal/models/auth.go`). Its enrolled members correspond to trusted signer identities (`member_app_ids`), and the policy defines the affirmative quorum threshold (`quorum`), distinct-signature requirement (`require_distinct`), and enabled state. Members sign Ed25519 votes over `<transaction_hash>|<decision>`. L2 gates execution under `consensus` and `notary` postures and remains audited under `doctrine` and `ratify`.

---

## Control Assertion

A framework-neutral, atomic statement of technical behavior defined in the protocol assertion catalog (`protocol/constants/compliance/assertion-catalog.json`, catalog version 2.0.0). Each assertion declares its identifier, version, title, statement, category, component scope, responsibility, applicable action classes, required evidence types, required grader and verifier references, minimum evidence level, validation cycle, missing-evidence policy, and passing rule. Typed crosswalks map these assertions to external framework controls (e.g. FedRAMP 20x CR26, NIST SP 800-53 Rev. 5).

---

## Danny-as-Code (DaC)

A tongue-in-cheek counterpart to Infrastructure-as-Code (IaC): the operating methodology developed over thirty years of incident response, expressed as a machine-verifiable separation of intent proposal, authorization, execution, and evidence. The "Danny" is the author, who is real.

---

## Deterministic Stage Evidence

The canonical protobuf `g8e.operator.v1.DeterministicStageEvidence`, which records a typed governance or execution stage with monotonic timing (`monotonic_start_ns`, `monotonic_end_ns`), clock domain, timing source, outcome (`DeterministicStageOutcome`), transaction and identity bindings, state roots before and after execution, signer and signature digests, commitment hashes, doctrine bundle identity, audit record references, and parent-stage linkage. Stage kinds cover `L1_DOCTRINE`, `PROTOCOL_L2`, `L3_NOTARY`, `L4_VERIFICATION`, `RECEIPT_PERSISTENCE`, `COMMITMENT_APPEND`, and `L5_EXECUTION`.

---

## Encrypted KV Adapter

The adapter (`internal/services/gateway/encrypted_kv_adapter.go`) connecting the Gateway KV store service to the `storage.TokenStore` interface required by the scrubbing service. It encrypts token values using the local vault with AES-256-GCM, namespaces them with `constants.SentinelKeyPrefix` (`g8e:sentinel:token:`), and writes them as `state_tier='observed'`. This excludes encrypted token storage rows from the bound state root while the deterministic in-memory token keymap hash is bound into the state root separately.

---

## Enrollment Token

A one-time token (`internal/models/auth.go`, `EnrollmentToken`) used to bridge passkey enrollment from the CLI to the browser without placing raw session identifiers in a URL. The Gateway generates 32 cryptographically random bytes, hex-encodes them, associates the token with the user and CLI session, and applies a five-minute expiration. The browser receives it in the URL fragment, and validation consumes it once. This differs from the credentials used by **Platform Enrollment**.

---

## Evidence Graph

The immutable, typed directed graph constructed by the compliance subsystem (`internal/services/compliance/`). It connects assessment scopes, control assertions, framework controls, content-addressed artifacts, verifier results, and assertion assessments. Compliance graders, profilers, and renderers evaluate this graph rather than scraping logs or inferring outcomes from filenames.

---

## Evidence Level

The standardized strength hierarchy assigned to compliance evidence:
- **L0 (Documented)**: Documented policy, architectural narrative, or procedural claim.
- **L1 (Implemented)**: Implemented code path, configuration mechanism, or structural boundary.
- **L2 (Deterministically Evaluated)**: Replicated unit, integration, or schema evaluation producing deterministic evidence.
- **L3 (Demonstrated)**: Live execution demonstrated against an integrated runtime stack with observed state mutations.
- **L4 (Continuously Evidenced)**: Continuous automated evaluation evidenced across an assessment time window.
- **L5 (Externally Attested)**: Cryptographically verified external attestation or independent third-party assessment.

---

## Execution Vault

A dedicated local SQLite service (`internal/services/storage/execution_vault.go`, `execution_vault.db`) that stores command execution results (`execution_logs`) and file diffs (`file_diffs`), links records to workflow identifiers, and records content hashes. It encrypts sensitive content through the local vault with AES-256-GCM, compresses payloads with zlib, and enforces configurable retention (`ExecutionVaultRetentionDays`) and database size limits (`ExecutionVaultMaxSizeMB`).

---

## g8e Binary

The statically linked Go executable (`cmd/g8e/main.go`) that operates as a **Governance Gateway** (`g8e gw`), **Governed Operator** (`g8e operator`), or unified administrative tool (`g8e auth`, `mcp`, `vault`, `test`, `audit`, `report`, `compliance`, `eval`, `tui`). The selected subcommand determines the role; both Gateway and Operator share the same underlying protocol contracts and governance substrate.

---

## g8e Protocol

The canonical wire contract and invariant set for g8e (`protocol/`). It defines protobuf schemas (`g8e.common.v1`, `g8e.operator.v1`, `g8e.compliance.v1`, `g8e.eval.v1`), Go and Python protocol libraries, JSON constant registries (`events.json`, `actions.json`), JSON model schemas, doctrine definitions, SPIFFE workload-identity helpers, receipt verification routines, and compliance crosswalks. Client-facing protobuf messages use protojson on wire boundaries.

---

## g8ed

The first-party browser dashboard (`dashboard/`). It provides passkey authentication and operator-facing visibility while remaining strictly outside the governance Policy Decision Point and Policy Execution Point boundaries. The browser authenticates directly to the Gateway using WebAuthn and a Gateway-issued HttpOnly session cookie; the dashboard service runs under its own enrolled application identity (`spiffe://g8e.local/app/g8ed`).

---

## g8ee

The first-party reference agentic ensemble (`ensemble/`). It translates user requests into governed intent, executes multi-agent reasoning loops and Tribunal workflows, constructs conformant **Governance Envelopes**, and publishes typed progress and result telemetry to the Gateway. It runs under enrolled application identity `spiffe://g8e.local/app/g8ee` and acts as a supported producer rather than a required Gateway dependency.

---

## Gateway Peer

A workload identity reserved for Gateway-to-Gateway trust, with SPIFFE URI `spiffe://g8e.local/gateway/<gateway_id>`. The PKI issues certificates from a dedicated `gateway-peer` intermediate CA and includes the CA in the canonical trust bundle. These identity and certificate primitives provide the trust foundation for federated deployments.

---

## Governance Envelope

The canonical protobuf container `g8e.common.v1.GovernanceEnvelope` for all governed mutations. It binds:
- **Identity**: `id`, timestamps (`timestamp`, `expires_at`), `source_component`, `operator_id`, `operator_session_id`, `web_session_id`, `cli_session_id`, `requestor_user_id`, and `acting_app_id`.
- **Intent**: routing `event_type`, typed protobuf `payload` bytes, structured `intent_data`, `action_type`, and `target_resource`.
- **State and replay defense**: `state_merkle_root`, `nonce`, `transaction_hash`, and `protocol_version`.
- **Governance proofs**: `governance` (`L1Metadata`, `L2Metadata`, `L3Metadata`).
- **Application context**: `case_id`, `investigation_id`, `task_id`, `system_fingerprint`, `tenant_id`, and `binding_persona`.
- **Policy metadata**: the Gateway-selected governance `posture`.

Client-facing surfaces serialize the envelope with protojson. The deterministic transaction hash covers normalized intent, state, and identity fields; posture is policy metadata and is excluded from transaction hash canonicalization.

---

## Governance Gateway

The platform coordinator and Policy Decision Point (PDP) (`g8e gw`). It authenticates workloads, manages the PKI hierarchy, persists shared platform state in `g8e.db`, admits envelopes, executes L1 through L3 policy decisions, and brokers Operator pub/sub channels. For actions targeting the Gateway host, an in-process Operator substrate runs L4 and L5 locally. The Gateway holds no privileged bypass around the governance pipeline and never initiates inbound connections to remote Operators.

---

## Governance Posture

The startup-selected policy mode that determines whether L2 Consensus and L3 Notary operate as fail-closed gates or audited observations. L1 Doctrine and universal integrity checks remain enforced in every posture.

| Posture | L1 Doctrine | L2 Consensus | L3 Notary (Mutations) |
| --- | --- | --- | --- |
| **Doctrine** | Enforced | Audited | Audited |
| **Consensus** | Enforced | Enforced | Audited |
| **Ratify** | Enforced | Audited | Enforced |
| **Notary** | Enforced | Enforced | Enforced |

Read-only actions do not require L3 proof in any posture. The Gateway stamps the active posture into `envelope.posture` at admission time; the L4 Warden evaluates against that envelope field during verification.

---

## Governed Operator

The Policy Execution Point (PEP) deployed on a target host (`g8e operator`). It opens no inbound listening ports, establishes an outbound-only mTLS connection to the Gateway, pulls work from its session-specific channel (`cmd:<operator_id>:<operator_session_id>`), and re-runs L1 Doctrine and verifies L2 Consensus and L3 Notary locally before executing through L4 Warden and L5 Actuator. It is the sole component authorized to mutate its host and retains host-authoritative audit and file-history evidence.

---

## Heartbeat

Periodic health and status telemetry sent by a Governed Operator to the Gateway over its outbound connection. Wire messages (`HeartbeatRequested` and `HeartbeatResult`, `g8e.operator.v1.HeartbeatResult`) report operator identity, uptime, OS, architecture, CPU count, memory and disk metrics, network interfaces, runtime environment, capability flags, and system fingerprint. The Gateway monitors heartbeats to maintain operator liveness records.

---

## Inference Dispatch (Governed Inference)

The governed model-inference dispatch subsystem (`internal/services/inference/`). It routes model generation requests (`InferenceDispatchRequest`) across configured LLM providers and backends. The dispatcher validates model roles (`primary`, `assistant`, `lite`), enforces model variant assignments, verifies model image digests, and captures execution metrics in an `InferenceDispatchResult`. Dispatched inference requests remain subject to L1 Doctrine payload scanning and model-boundary token scrubbing.

---

## KSI (Key Security Indicator)

A typed, machine-evaluable security requirement defined in the FedRAMP 20x catalog (`docs/reference/ksi-catalog.json`). KSIs span ten functional categories: `CED` (Cybersecurity Education), `CMT` (Change Management), `CNA` (Cloud Native Architecture), `IAM` (Identity and Access Management), `INR` (Incident Response), `MLA` (Monitoring, Logging, and Auditing), `PIY` (Policy and Inventory), `RCP` (Recovery Planning), `SVC` (Service Configuration), and `TPR` (Supply Chain Risk). The KSI evaluator derives status (`satisfied`, `not_satisfied`, `unverifiable`, `not_applicable`) from registered automated methods and typed evidence artifacts.

---

## L1 Doctrine (L1Doctrine)

The technical hard-gate governance layer (`internal/services/governance/l1_doctrine.go`). It validates decoded typed payloads against protobuf field-option regex rules (`forbidden_patterns`), bundled JSON doctrine sets, and deterministic threat analysis covering shell commands and MCP arguments, including MITRE ATT&CK patterns. L1 is enforced across all governance postures at Gateway admission and is re-run independently on the Operator prior to dispatch.

---

## L2 Consensus (L2Consensus)

The multi-signature governance layer (`internal/services/governance/l2_consensus.go`). Each enrolled member signs an Ed25519 vote over `<transaction_hash>|<decision>`. The L4 Warden verifies signer trust, consensus policy membership, signature validity, signature distinctness, and affirmative quorum. L2 is a required gate under `consensus` and `notary` postures; under `doctrine` and `ratify`, available L2 votes are verified and recorded in the audit trail without gating execution.

---

## L3 Notary (L3Notary)

The human-authorization layer for mutations under `ratify` and `notary` postures (`internal/services/governance/l3_notary.go`). Gateway mode verifies a WebAuthn passkey assertion against the transaction hash challenge; CLI-originated proofs additionally bind the authenticated CLI session ID and certificate fingerprint. Outbound mode verifies an approved suspended transaction record and its Ed25519 approval signature over the transaction hash. Read-only actions do not require L3 proof in any posture; `doctrine` and `consensus` postures audit L3 proofs when provided without gating execution.

---

## L4 Warden (L4Warden)

The fail-closed pre-dispatch verifier on the Operator substrate (`internal/services/governance/l4_warden.go`). It executes a strict five-step verification sequence:
1. Reserves the in-flight lock and durably reserves the nonce in the SQLite replay store (`ReserveNonce`), verifying expiry.
2. Performs stateless validation: verifies envelope structure, decodes payload, ensures action type and payload match, recomputes the transaction hash, and re-runs L1 Doctrine.
3. Performs stateful validation: checks that `envelope.state_merkle_root` matches the current authoritative Bound State Root.
4. Performs posture validation: reads `envelope.posture` and verifies L2 Consensus votes and L3 Notary proofs according to posture rules.
5. Emits linked `DeterministicStageEvidence` and returns a `VerifiedTransaction` to L5 Actuator.

Verification failure at any step releases the nonce reservation and halts the transaction.

---

## L5 Actuator (L5Actuator)

See **Actuator**.

---

## Ledger

The git-backed file-mutation history maintained by the Governed Operator (`internal/services/storage/ledger.go`, `GitLedgerService`). Each Operator session operates within an isolated git repository beneath the runtime ledger directory. For every governed file mutation, the ledger creates a pre-mutation snapshot and commit, applies the mutation, commits the post-mutation state, and records commit hashes, diff stats, and diff content. The ledger supports history inspection, point-in-time retrieval, and file restoration. It is distinct from the SQLite **Commitment Ledger**.

---

## Local-First Audit Architecture (LFAA)

The architectural model in which each target host remains the authoritative root of trust for its own execution evidence, file history, and host effects. The local audit database, commitment ledger, execution vault, and git file ledger preserve the raw forensic artifacts required to reconstruct and verify execution. The Gateway coordinates platform workflows and relays commitments, while sensitive execution details remain bound to the Operator host.

---

## Marshal

The g8ee application-layer defender persona (`marshal`, *The Order Keeper*). Marshal coordinates pre-envelope risk analysis for shell commands, file modifications, and error states before a `GovernanceEnvelope` is constructed. Specialized sub-agents (`marshal_command`, `marshal_error`, `marshal_file`) emit advisory `LOW`, `MEDIUM`, or `HIGH` risk labels, drive the user approval interface, and stake reputation on classification accuracy. Marshal is distinct from the protocol **L4 Warden**, which performs deterministic pre-dispatch verification on the Operator substrate.

---

## MCP (Model Context Protocol)

The JSON-RPC tool protocol exposed by g8e for AI clients (`internal/services/mcp/`). The Gateway handles tool discovery and calls, converts tool requests into typed Operator protobuf payloads wrapped in **Governance Envelopes**, and returns structured results or approval suspension responses. Native Operator tools and configured downstream MCP servers execute only after passing through the governance pipeline.

---

## Model Roles (Primary, Assistant, Lite)

The three independently configurable chat and inference roles across g8ee and governed inference dispatch. Each role uses a strict lowercase wire identifier in APIs, configuration, telemetry, and database records, and a title-case display label in user-facing copy:

| Wire value | Display label | Responsibility |
| --- | --- | --- |
| `primary` | Primary | Complex chat turns, tool-capable agent loops, and primary reasoning work |
| `assistant` | Assistant | Bounded technical tasks and delegated sub-agent turns |
| `lite` | Lite | Triage, Tribunal generation, risk classification, title generation, and structured evaluations |

Synonyms such as "Light" for `lite` are prohibited. See [LLM Providers](../ensemble/llm-providers.md) for provider configuration and routing.

---

## Mutual TLS (mTLS)

Bidirectional TLS authentication enforced across all protected transport surfaces. g8e requires TLS 1.3 and uses certificate URI Subject Alternative Names (SANs) under the `g8e.local` trust domain to authenticate workload and session identities, encrypt data in transit, bind callers to envelope claims, and enforce certificate revocation. Workload certificates authenticate communicating processes, while binary build provenance is verified separately.

---

## Observed-State Root

A deterministic SHA-256 Merkle root computed by `StateRootService.calculateObservedStateRoot()` over active observed-tier `kv_store` and `blobs` rows (`state_tier = 'observed'`). It does not gate transaction admission freshness, allowing high-frequency telemetry and environmental observations to remain tamper-evident in audit records without invalidating in-flight envelopes.

---

## Operator Run Dispatch

The enrolled-CLI automation path for governed shell execution across one or more remote Operators (`g8e operator run`). The CLI posts dispatch requests to `POST /api/v1/operators/commands` with explicit target operator session IDs, fanning out `EXECUTE_BASH` in parallel. The Gateway constructs the envelope, screens the payload with L1 Doctrine, applies posture-aware L3 gating, publishes to each target's `cmd:` channel, and returns stdout, stderr, and exit codes to the caller upon completion.

---

## Operator Session

The runtime authorization and execution context of an active Governed Operator, identified by `operator_session_id`. It scopes the Operator's SPIFFE workload identity, pub/sub communication channels (`cmd:<operator_id>:<operator_session_id>`), audit records, execution vault logs, and isolated git ledger repository.

---

## OSCAL (Open Security Controls Assessment Language)

The NIST machine-readable XML/JSON format supported by the compliance reporting subsystem. g8e projects canonical compliance assessments into OSCAL 1.1.2 assessment results and validates output against official OSCAL schemas. OSCAL is an output projection format; assessment decisions are derived from canonical typed evidence graphs.

---

## PKI (Public Key Infrastructure)

The Gateway-managed X.509 certificate authority tree (`internal/services/pki/`). It establishes root and intermediate authorities, issues short-lived workload certificates with SPIFFE URI SANs, publishes trust and revocation bundles, and manages Operator, CLI, app, user, hub, and Gateway-peer identities. Enrolling workloads generate and retain their own private keys.

---

## Platform Enrollment

The owner-approved protocol for enrolling dashboard, ensemble, and Operator component instances (`internal/services/gateway/platform_enrollment_service.go`). A component generates private keys, submits Certificate Signing Requests (CSRs) and fingerprints, and awaits an approval decision. To complete an approved request, the component proves possession of every private key by signing a canonical completion transcript (`PlatformEnrollmentCompletionTranscript`) that binds the protocol version, request ID, token hash, component kind, instance ID, and key fingerprints before certificates are issued.

---

## Principal

The human user, AI agent, application, CI/CD pipeline, or automated process that originates an intent. g8e governs the requested action rather than granting a trusted principal an unmonitored bypass; identity claims (`requestor_user_id`, `acting_app_id`) remain explicitly bound in the envelope.

---

## Producer

A client or service that translates a principal's intent into a conformant **Governance Envelope**. Producers include the Gateway's MCP and A2A translators, g8ee, the CLI dispatch service, and native third-party clients. Producing an envelope initiates governance evaluation; it does not authorize execution.

---

## Receipt Failure Code

A strongly typed classification enum (`ReceiptFailureCode`, `protocol/proto/g8e/operator/v1/operator.proto`) stamped on an `ActionReceipt` when execution status is `EXECUTION_STATUS_FAILED`. Values distinguish governance rejections (`RECEIPT_FAILURE_CODE_GOVERNANCE_REJECTED`), model configuration errors (`RECEIPT_FAILURE_CODE_MODEL_OVERRIDE_DENIED`, `RECEIPT_FAILURE_CODE_ROLE_INVALID`, `RECEIPT_FAILURE_CODE_MODEL_REF_INVALID`), inference backend faults (`RECEIPT_FAILURE_CODE_BACKEND_UNAVAILABLE`, `RECEIPT_FAILURE_CODE_BACKEND_TIMEOUT`, `RECEIPT_FAILURE_CODE_GENERATE_FAILED`), missing models (`RECEIPT_FAILURE_CODE_MODEL_NOT_FOUND`), provider format errors (`RECEIPT_FAILURE_CODE_PROVIDER_RESPONSE_INVALID`), and general execution failures (`RECEIPT_FAILURE_CODE_EXECUTION_FAILED`). The failure code is bound into the receipt signature.

---

## Receipt Persistence Attestation

The canonical protobuf `g8e.operator.v1.ReceiptPersistenceAttestation`, signed by the Actuator key after the final `ActionReceipt` is durably stored in the SQLite audit store. It binds the transaction ID, receipt signature digest, persistence timestamp, audit record ID, signer key ID, and signature. Upstream receipt relays verify both the receipt signature and this persistence attestation.

---

## Replay Protection

The multi-tiered defense that prevents duplicate or replayed transactions from executing. The L4 Warden reserves each envelope's nonce in-flight, durably records it in the SQLite `nonces` table with expiration tracking (`ReserveNonce`), recomputes and verifies the deterministic transaction hash, and rejects expired or duplicated transactions. Failed transactions release their nonce reservations.

---

## Reputation Commitment

A g8ee Auditor record (`app.models.reputation.ReputationCommitment`) that cryptographically binds a Tribunal verdict to the agent reputation scoreboard. It contains a SHA-256 Merkle root over sorted `(agent_id, scalar)` pairs, the preceding reputation root, leaf count, verdict ID, and an Auditor HMAC-SHA256 signature. This ensemble-internal scoreboard chain is distinct from the L5 **Commitment Ledger**.

---

## Reputation Staking

The g8ee mechanism that adjusts each agent persona's reputation scalar (ranging from 0.0 to 1.0) using an exponential moving average following task outcomes. Stake-resolution records capture rewards, slash penalties, and unbonding states. Reputation governs ensemble voting weights and prompt influence; it does not alter cryptographic signature verification at the L4 Warden.

---

## Requestor User ID

The `requestor_user_id` field in a **Governance Envelope**. It identifies the human delegator who authorized the action and pairs with `acting_app_id`, which identifies the delegated application, tool, or agent persona.

---

## Scrubbed Vault

The storage mode in execution and file vaults where sensitive tokens have passed through the **Sovereign Execution Boundary** before persistence. Sensitive values are replaced with reversible `{{UEI_N}}` tokens or irreversible redactions depending on detector rules. The contrasting `raw` mode stores unsanitized forensic records on the local host and is prohibited from being transmitted to external models or cloud services.

---

## Sovereign Execution Boundary

The Operator-local scrubbing and rehydration architecture implemented by `ScrubbingService` (`internal/services/scrubbing/`). Egress traffic leaves the host only after sensitive values are tokenized (`{{UEI_N}}`) or redacted; persistent token mappings are encrypted at rest with the local vault; the token keymap hash can participate in the Bound State Root; and L5 rehydrates reversible tokens strictly on the target execution host immediately before command dispatch.

---

## SSE (Server-Sent Events)

The Gateway real-time event delivery mechanism for browser and CLI sessions. Applications push authenticated, session-scoped events via `POST /api/v1/sse/push`; consumers stream `/api/v1/sse/stream` or poll `/api/v1/sse/events` using mTLS or web-session cookies. Events are buffered in the `sse_events` table in `g8e.db` for reconnection replay. Governed Operator command transport uses outbound mTLS and pub/sub routing rather than SSE.

---

## State Root

A deterministic SHA-256 digest representing authoritative platform state. Carried in `envelope.state_merkle_root`, it ensures that a transaction is evaluated against the exact platform state under which it was authorized. The L4 Warden rejects envelopes whose state root diverges from the current authoritative root. See **Bound State Root** and **Observed-State Root**.

---

## State Tier

The classification applied to Gateway KV and blob storage rows:
- **Bound** (`state_tier = 'bound'`): Authoritative state included in the admission-gating Bound State Root.
- **Observed** (`state_tier = 'observed'`): Telemetry, evidence, and encrypted token mappings excluded from the bound root and hashed into the separate Observed-State Root.

Documents are authoritative by definition and always participate in the bound root.

---

## Suspended Transaction

A Governance Envelope paused while awaiting required L3 Notary human approval (`internal/services/storage/suspended_transaction_store.go`, stored in `suspended_transactions.db`). The store retains the envelope, approval status, proof metadata, expiry, and caller bindings. Upon human approval via passkey or signature, the Gateway resumes governance evaluation; executed or expired records are pruned.

---

## System Fingerprint

A stable SHA-256 host identifier (`internal/services/auth/fingerprint.go`, `SystemFingerprint`) derived from operating system, CPU architecture, core count, machine ID, and hostname. The generator supports operator-differentiating parameters (`local_dir`, `account`, `port`, `role`) so multiple Operator instances can coexist on the same physical host without collision.

---

## Tactical Governance Console (TUI)

The terminal user interface launched via `g8e tui` (`internal/cli/cmd/tui/`). It renders live operator connections, active governance postures, approval queues, transaction feeds, and forensic metrics directly in the terminal for operators and security teams.

---

## Time-Travel

Point-in-time file retrieval and restoration powered by the git-backed **Ledger** (`internal/services/storage/ledger.go`). Callers can inspect historical file versions, retrieve diff statistics, or restore a file to a specific ledger commit. This is distinct from the append-only SQLite **Commitment Ledger**, which cannot be rewound.

---

## Tool Calling Loop

The operational pattern in which an AI agent selects an MCP tool or A2A skill, submits arguments, receives a governed result or approval suspension, and decides its next action. Every state-modifying iteration executes as an independent, fully evaluated **Governance Envelope** rather than inheriting standing authorization from earlier conversational turns.

---

## Transaction Hash

The deterministic SHA-256 digest computed from normalized **Governance Envelope** fields via `GenerateMessageID(env)`. The envelope `id` and `transaction_hash` must both match this value. The calculation dispatches on protocol version: V1 and V2 (`txHashV2Prefix|action_type|event_type|v1Canonical`). L2 consensus votes, L3 human approvals, replay checks, capabilities, receipts, and commitment attestations bind cryptographically to this hash. Dynamic policy metadata, including the active governance posture, is excluded from hash canonicalization.

---

## Vault

The local AES-256-GCM encryption service and key management hierarchy (`internal/services/vault/`, `vault.Vault`). It secures sensitive audit fields, execution vault logs, and encrypted KV token records. Storage components requiring encryption fail closed if the vault is locked or uninitialized. The encryption vault is distinct from the **Execution Vault**, which is an SQLite database.

---

## Workload Identity

A SPIFFE URI Subject Alternative Name encoded in an X.509 certificate under the `g8e.local` trust domain (`protocol/workload_identity.go`). Formats are:
- **Operator**: `spiffe://g8e.local/operator/<organization_id>/<operator_id>/<operator_session_id>`
- **CLI**: `spiffe://g8e.local/cli/<user_id>/<cli_session_id>`
- **App**: `spiffe://g8e.local/app/<operator_id>` (with `spiffe://g8e.local/app/g8ee` designating the ensemble event broker)
- **User**: `spiffe://g8e.local/user/<user_id>`
- **Hub**: `spiffe://g8e.local/hub/operator-listen`
- **Gateway Peer**: `spiffe://g8e.local/gateway/<gateway_id>`

The transport and authorization layers match these URI SANs against session records, role definitions, and envelope identity fields.
