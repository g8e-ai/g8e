---
doc_id: governance
title: Governance Architecture
audience: maintainers and coding agents
status: current
last_updated: 2026-09-28
version: v2.2.3
owners:
  - internal/services/governance/
  - internal/governance/envelope.go
  - protocol/proto/g8e/common/v1/common.proto
  - protocol/proto/g8e/operator/v1/operator.proto
  - internal/services/gateway/dispatch_service.go
related:
  - docs/architecture/consensus.md
  - docs/architecture/gateway.md
  - docs/architecture/operator.md
  - docs/architecture/auth.md
  - docs/architecture/encryption.md
  - docs/architecture/network.md
  - docs/architecture/events.md
  - docs/architecture/agents.md
  - docs/architecture/storage.md
when_to_read: Understanding the five-layer verification pipeline (L1-L5), governance postures, transaction envelope canonicalization and hashing, fail-closed pre-dispatch and execution gates, deterministic stage evidence, and receipt attestation.
do_not_use_for:
  - Deep-dive into L2 machine consensus algorithms, member keys, and quorum policies (docs/architecture/consensus.md)
  - Gateway HTTP routes, WebSocket pub/sub tunneling, and network topology (docs/architecture/gateway.md and docs/architecture/network.md)
  - Operator runtime daemon, process management, and local audit storage (docs/architecture/operator.md)
  - WebAuthn passkey enrollment and mTLS session verification (docs/architecture/auth.md)
  - Event catalog taxonomy and payload schema definitions (docs/architecture/events.md)
---

# Governance Architecture

## Purpose

Documents the governance architecture of the g8e platform: the five-layer verification pipeline (L1 through L5), immutable governance postures (`doctrine`, `consensus`, `ratify`, `notary`), canonical transaction container (`GovernanceEnvelope`) hashing and canonicalization, pre-dispatch verification in the L4 Warden, fail-closed execution in the L5 Actuator, just-in-time capability lifecycle, and deterministic protocol stage evidence attestation.

## Quick index

- [Purpose](#purpose)
- [Quick index](#quick-index)
- [Invariants](#invariants)
- [Owned surfaces](#owned-surfaces)
- [Procedures](#procedures)
  - [The Five-Layer Interlock Sequence](#the-five-layer-interlock-sequence)
  - [Governance Postures and Enforcement Matrix](#governance-postures-and-enforcement-matrix)
  - [Canonical Transaction Envelope and Deterministic Hashing](#canonical-transaction-envelope-and-deterministic-hashing)
  - [Layer 1 (L1) Doctrine: Technical Bedrock](#layer-1-l1-doctrine-technical-bedrock)
  - [Layer 2 (L2) Consensus: Multi-Agent Quorum](#layer-2-l2-consensus-multi-agent-quorum)
  - [Layer 3 (L3) Notary: Human Authorization](#layer-3-l3-notary-human-authorization)
  - [Layer 4 (L4) Warden: Pre-Dispatch Verification Pipeline](#layer-4-l4-warden-pre-dispatch-verification-pipeline)
  - [Layer 5 (L5) Actuator: Fail-Closed Execution Boundary](#layer-5-l5-actuator-fail-closed-execution-boundary)
  - [Deterministic Protocol Chain and Evidence Graph](#deterministic-protocol-chain-and-evidence-graph)
  - [End-to-End Transaction Lifecycles](#end-to-end-transaction-lifecycles)
- [Anti-patterns](#anti-patterns)
- [Links out](#links-out)

Key invariant groups: [Posture and Enforcement](#posture-and-enforcement-inv-gov-post), [Canonical Envelope and Hashing](#canonical-envelope-and-hashing-inv-gov-env), [Bedrock Layers and Posture Rules](#bedrock-layers-and-posture-rules-inv-gov-lay), [L4 Warden Pre-Dispatch](#l4-warden-pre-dispatch-inv-gov-ward), [L5 Actuator and Execution](#l5-actuator-and-execution-inv-gov-act), [Audit Trail and Evidence Chain](#audit-trail-and-evidence-chain-inv-gov-evid).

## Invariants

Ids are stable. Append the next free number within each group; do not renumber.

### Posture and Enforcement (`INV-GOV-POST`)

| ID | Rule |
| --- | --- |
| INV-GOV-POST-01 | The four canonical governance postures (`doctrine`, `consensus`, `ratify`, `notary`) MUST be parsed via `ParseGovernancePosture` and fail closed on unrecognized names with an immediate validation error or panic. |
| INV-GOV-POST-02 | The Gateway MUST inject the configured governance posture into `GovernanceEnvelope.Posture` at construction time. The L4 Warden MUST read posture per-envelope from `envelope.Posture` and fail closed with `ErrEnvelopePostureMissing` if absent. |
| INV-GOV-POST-03 | Posture configuration is immutable for the lifetime of the process. In gateway mode, `--posture` defaults to `doctrine`. In outbound operator mode, posture is authoritative from the received envelope. |
| INV-GOV-POST-04 | L2 consensus is enforced under `consensus` and `notary` postures (`RequiresL2Signature() == true`). L3 notary proof is enforced for mutation action types under `ratify` and `notary` postures (`RequiresL3Proof() == true`). |

### Canonical Envelope and Hashing (`INV-GOV-ENV`)

| ID | Rule |
| --- | --- |
| INV-GOV-ENV-01 | All new ingress transactions MUST declare protocol version `"2"` (`GovernanceProtocolVersionV2`). Envelopes with version `"1.0"` or other values MUST fail closed at L4 with `ErrTxProtocolVersionUnsupported`. |
| INV-GOV-ENV-02 | The canonical transaction hash (`GenerateMessageID`) for protocol version 2 MUST hash the prefix `g8e-tx-v2|` followed by `action_type|event_type|` and the canonical v1 string representation with SHA-256 encoded as lowercase hex. |
| INV-GOV-ENV-03 | The v1 canonical string MUST serialize fields in strict documented order: `action_type`, `target_resource`, base64-encoded `payload`, `state_merkle_root`, `nonce`, RFC3339 `expires_at`, canonicalized `intent_data`, `requestor_user_id`, `acting_app_id`, `operator_id`, `operator_session_id`, `case_id`, `investigation_id`, `task_id`, `web_session_id`, and `cli_session_id`, delimited by `|`. |
| INV-GOV-ENV-04 | `envelope.Id` and `envelope.TransactionHash` MUST both equal the recomputed transaction hash. Any mismatch MUST be rejected at L4 with `ErrTxTransactionIDMismatch` or `ErrTxTransactionHashMismatch`. |
| INV-GOV-ENV-05 | Posture and L2/L3 governance proofs MUST NOT be included in the transaction hash canonicalization, allowing L2 consensus members to sign before human notary approval and allowing the gateway to set envelope posture without invalidating hashes. |

### Bedrock Layers and Posture Rules (`INV-GOV-LAY`)

| ID | Rule |
| --- | --- |
| INV-GOV-LAY-01 | L1 Doctrine MUST validate protobuf field options `(g8e.common.v1).forbidden_patterns` on string fields and run MITRE ATT&CK threat detectors across command, MCP argument, A2A payload, and critical system file edit requests. Any violation MUST reject the transaction fail closed with `ErrTxL1ValidationFailed` across all postures. |
| INV-GOV-LAY-02 | L2 Consensus member votes MUST be Ed25519 signatures over `<transaction_hash>|<decision>`. Under `consensus` and `notary` postures, affirmative votes (`decision=true`) from distinct members (when `require_distinct` is true) MUST meet policy quorum, or reject with `ErrTxL2QuorumNotMet`. |
| INV-GOV-LAY-03 | Platform bootstrap action types (`actionType.IsBootstrapAction()`) MUST be exempt from L2 consensus gating across all postures, allowing platform enrollment before consensus members are enrolled. |
| INV-GOV-LAY-04 | L3 Notary verification MUST gate mutation actions under `ratify` and `notary` postures (`ErrTxL3ProofMissing`). Read-only actions MUST NOT require an L3 proof under any posture. |
| INV-GOV-LAY-05 | In gateway mode, L3 Notary requires a WebAuthn passkey assertion with challenge matching the transaction hash (`ErrPasskeyProofRequired`). In outbound operator mode, L3 Notary verifies an approved suspended transaction within the 30-minute window (`L3ApprovalWindow`) signed by the operator private key with matching certificate fingerprint. |

### L4 Warden Pre-Dispatch (`INV-GOV-WARD`)

| ID | Rule |
| --- | --- |
| INV-GOV-WARD-01 | L4 Warden MUST track nonces in memory (`inFlight`) before stateful operations to prevent race conditions, and MUST durably reserve the nonce in `ReplayStore` before payload validation. |
| INV-GOV-WARD-02 | Expiry (`envelope.ExpiresAt`) MUST be strictly checked against the clock before nonce reservation. Expired transactions MUST fail closed with `ErrTxTransactionExpired`. |
| INV-GOV-WARD-03 | Stateless checks MUST run before stateful checks: protocol version, L1 doctrine presence, non-empty ID, event type validation, known action type, non-empty payload (except heartbeat), typed payload decoding, document collection scope, execution target ownership, L1 doctrine scanning, and dual hash matching. |
| INV-GOV-WARD-04 | The state Merkle root (`envelope.StateMerkleRoot`) MUST match the current root from `StateRootProvider` (or the pre-fetched root in context for in-process gateway builds); mismatches MUST fail closed with `ErrTxStateRootMismatch`. |
| INV-GOV-WARD-05 | Any validation failure occurring after nonce reservation MUST release the nonce reservation via `replayStore.ReleaseNonce` so that non-admitted transactions do not leave dangling replay locks. |
| INV-GOV-WARD-06 | A `DOCUMENT_UPDATE` or `DOCUMENT_DELETE` payload MUST target a collection marked `_governed` in `protocol/constants/collections.json` (`CollectionName.IsGovernedDocument`); any other collection MUST fail closed with `ErrTxDocumentCollectionNotGoverned` before execution. The governed document store is the Gateway's platform document store, which also holds `users`, `trusted_signers`, `app_policies`, and other authority records that only their owning Gateway services write. |
| INV-GOV-WARD-07 | After document collection scope and before doctrine scanning, L4 MUST require `ExecutionTarget.ExecutesFor(envelope.OperatorId)`: an outbound runtime executes only for its own enrolled Operator ID, and the Gateway executes only for the embedded Operator owned by `embedded.Service`. A missing target dependency, empty target on a host action, or foreign target fails closed with `ErrTxTargetOperatorMismatch` (HTTP 403 for direct envelope submission). The INV-AUTH-ID-05 unbound app document-write path is exempt from the Operator ID comparison; the target dependency remains required. Gateway-internal enrollment envelopes name the embedded Operator, including bootstrap before that record is claimed. |

### L5 Actuator and Execution (`INV-GOV-ACT`)

| ID | Rule |
| --- | --- |
| INV-GOV-ACT-01 | The L5 Actuator MUST enforce fail-closed execution: receipt signing key, non-empty event type, and execution handler MUST be present. When SQL audit is enabled, auditor signing key, key ID, and commitment ledger MUST be present. |
| INV-GOV-ACT-02 | The Actuator MUST sign and persist an `EXECUTING` status `ActionReceipt` to the audit stores before invoking the execution handler. Failure to sign or persist MUST abort execution without invoking the handler (`ErrL5ActuatorLogReceipt`). |
| INV-GOV-ACT-03 | When `SQLAuditStore` is configured, the Actuator MUST append a signed `CommitmentAttestation` to the SQLite hash-chained commitment ledger against the current chain head before execution. |
| INV-GOV-ACT-04 | The Actuator MUST mint a transaction-scoped, single-action `Capability` bound to the transaction hash, action type, target resource, operator ID, session, and expiry before execution, and MUST dissolve the capability immediately after execution completes or fails. |
| INV-GOV-ACT-05 | The Actuator MUST finalize the `ActionReceipt` with status (`COMPLETED` or `FAILED`), state root after, execution timestamp, and typed `ReceiptFailureCode`. It MUST sign the final receipt, log it, and attach a signed `ReceiptPersistenceAttestation`. |
| INV-GOV-ACT-06 | Remote receipt publication to the Gateway (`PublishActionReceipt`) is best effort and MUST NOT alter or gate local execution outcome. The executing runtime's local SQLite audit record remains authoritative. |

### Audit Trail and Evidence Chain (`INV-GOV-EVID`)

| ID | Rule |
| --- | --- |
| INV-GOV-EVID-01 | Deterministic stage evidence MUST maintain strict monotonic ordering: `L1_DOCTRINE`, `PROTOCOL_L2`, `L3_NOTARY`, `L4_VERIFICATION`, `RECEIPT_PERSISTENCE`, `COMMITMENT_APPEND`, and `L5_EXECUTION`. |
| INV-GOV-EVID-02 | Stage parentage MUST bind strictly: L1, L2, and L3 stages declare parent ID equal to the L4 stage ID; L4, receipt persistence, and commitment append declare parent ID equal to the L5 stage ID. |
| INV-GOV-EVID-03 | Identity fields (`OperatorID`, `OperatorSessionID`, `RequestorUserID`, `ActingAppID`, `CaseID`, `InvestigationID`, `TaskID`) MUST remain identical across all deterministic stages in a transaction. |
| INV-GOV-EVID-04 | The final `ReceiptPersistenceAttestation` MUST bind the transaction ID, audit record ID, signer key ID, timestamp, and SHA-256 digest of the final receipt signature, signed by the Actuator's private key. |
| INV-GOV-EVID-05 | Relayed receipts at the Gateway MUST be verified against both the Actuator public key signature and the persistence attestation before acceptance; invalid attestations MUST cause rejection. |

## Owned surfaces

| Claim | Path | Verify |
| --- | --- | --- |
| Governance posture definitions and factory | `internal/services/governance/posture.go` | Unit tests in `internal/services/governance/l4_warden_test.go` |
| Envelope proto definition and schema | `protocol/proto/g8e/common/v1/common.proto`, `protocol/models/governance.json` | `make proto` and schema validation tests |
| Envelope hashing and canonicalization | `internal/governance/envelope.go` | Cross-language parity tests in `internal/governance/envelope_hash_parity_test.go` |
| L1 Doctrine validation and threat detectors | `internal/services/governance/l1_doctrine.go` | Unit tests in `internal/services/governance/l1_doctrine_test.go` |
| L2 Consensus verification | `internal/services/governance/l2_consensus.go` | Unit tests in `internal/services/governance/l4_warden_consensus_test.go` |
| L3 Notary authorization | `internal/services/governance/l3_notary.go` | Unit tests in `internal/services/governance/l3_notary_test.go` |
| L4 Warden pre-dispatch pipeline | `internal/services/governance/l4_warden.go` | Unit and edge tests in `internal/services/governance/l4_warden_test.go` |
| L5 Actuator execution boundary | `internal/services/governance/l5_actuator.go` | Unit and integration tests in `internal/services/governance/l5_actuator_test.go` |
| Just-in-time capability lifecycle | `internal/services/governance/capability.go` | Unit tests in `internal/services/governance/capability_test.go` |
| Deterministic protocol chain verification | `internal/services/governance/protocol_chain.go` | Chain tests in `internal/services/governance/protocol_chain_test.go` |
| Gateway dispatch envelope construction | `internal/services/gateway/dispatch_service.go` | Unit tests in `internal/services/gateway/dispatch_service_test.go` |
| Operator execution receipts proto | `protocol/proto/g8e/operator/v1/operator.proto` | `make proto` and protocol reference tests |

## Procedures

### The Five-Layer Interlock Sequence

Every transaction submitted to the g8e platform passes through a five-layer verification pipeline (L1 through L5) before execution can occur in any runtime. The layers interlock under a defense-in-depth model where each layer operates independently and assumes prior stages could be flawed or adversarial:

```
[ Ingress: HTTP / MCP / A2A / PubSub ]
                 │
                 ▼
         1. L1 Doctrine
   (Hard static gates & MITRE threat scans)
                 │
                 ▼
         2. L2 Consensus
   (K-of-N Ed25519 multi-agent quorum)
                 │
                 ▼
         3. L3 Notary
   (Human-in-the-loop WebAuthn / Passkey / mTLS)
                 │
                 ▼
         4. L4 Warden
   (Replay check, state Merkle root, hash binding, posture gating)
                 │
                 ▼
         5. L5 Actuator
   (Fail-closed EXECUTING receipt, commitment append, JIT capability, dispatch, attested receipt)
```

1. **L1 Doctrine**: Technical bedrock. Hard static analysis gates that inspect field annotations for forbidden regex patterns and run MITRE ATT&CK threat detectors across command strings, MCP arguments, A2A payloads, and critical system file edits. Enforced fail-closed across all postures.
2. **L2 Consensus**: Multi-agent machine quorum. Verifies protocol-level K-of-N threshold signatures from enrolled members over the exact transaction hash and safety decision. Independent from application-level ensemble voting. Enforced under `consensus` and `notary` postures; audited under `doctrine` and `ratify`.
3. **L3 Notary**: Human-in-the-loop authorization. Verifies cryptographic proof of human presence for mutation operations. In gateway mode, requires a WebAuthn passkey assertion; in outbound operator mode, verifies an approved suspended transaction signed by an authorized key within a 30-minute approval window. Enforced for mutations under `ratify` and `notary` postures; audited under `doctrine` and `consensus`.
4. **L4 Warden**: Pre-dispatch verification. Verifies transaction expiration, reserves nonces durably in SQLite storage to prevent replay attacks, confirms protocol version 2, validates dual transaction hash and ID equality, validates the state Merkle root, and enforces posture-required L2 and L3 proofs. Any failure after nonce reservation releases the lock.
5. **L5 Actuator**: Execution boundary. Establishes zero standing privileges. Signs and logs an `EXECUTING` receipt to local SQLite audit storage, appends a signed `CommitmentAttestation` to the hash-chained commitment ledger, rehydrates sovereignty-scrubbed values, mints a single-action just-in-time capability, invokes the isolated execution handler, dissolves the capability immediately, finalizes the receipt with typed failure classification, logs final execution evidence, and signs a durable `ReceiptPersistenceAttestation`.

### Governance Postures and Enforcement Matrix

Governance postures define which layers are enforced as fail-closed admission gates versus which are audited for compliance evidence. When a layer is audited, verification runs if evidence is present and records the result in the receipt, but missing or invalid proofs do not block execution.

| Posture | L1 Doctrine | L2 Consensus | L3 Notary (Mutations) | L3 Notary (Read-Only) | Typical Use |
| --- | --- | --- | --- | --- | --- |
| **`doctrine`** | Enforced | Audited | Audited | Not Required | Local development and automated testing |
| **`consensus`** | Enforced | Enforced | Audited | Not Required | Autonomous agent ensembles with peer review |
| **`ratify`** | Enforced | Audited | Enforced | Not Required | Human-authorized operations without peer consensus |
| **`notary`** | Enforced | Enforced | Enforced | Not Required | Production environments requiring both peer review and human approval |

#### Posture Gating Details

- **Doctrine (default)**: Configured via `--posture doctrine` on `g8e gw start`. Enforces L1 Doctrine, expiry, nonce replay, state root, and transaction hash integrity. L2 votes and L3 proofs are recorded in receipts if present, but neither gates transaction dispatch.
- **Consensus**: Configured via `--posture consensus`. Enforces L1 Doctrine and L2 consensus signature verification. Requires that the consensus policy store is configured, the consensus policy exists and is enabled, member signatures verify against `TrustedSigner` public keys, and affirmative distinct votes meet quorum. Platform bootstrap actions are exempt from L2 gating. L3 proofs remain audited only.
- **Ratify**: Configured via `--posture ratify`. Enforces L1 Doctrine and L3 notary proof verification for all mutation action types (`actionType.IsMutation() == true`). Read-only actions bypass L3 enforcement. L2 consensus votes remain audited only.
- **Notary**: Configured via `--posture notary`. Strictly enforces L1 Doctrine, L2 consensus quorum, and L3 notary proof verification for mutations. Read-only actions require valid L1 and L2, but do not require L3.

#### Gateway Posture Ingestion and Operator Propagation

1. **Gateway Startup**: The Gateway parses `--posture` using `governance.ParseGovernancePosture`. Invalid posture names fail startup fast.
2. **Startup Advisories**: If a posture requires L2 consensus (`consensus` or `notary`), the Gateway checks whether `--consensus-id` is provided and resolves to an enabled policy. If unconfigured or disabled, the Gateway logs an advisory warning (`L2 posture requires consensus but policy not found or disabled`) and continues booting; L2-gated transactions fail closed at transaction time.
3. **Envelope Posture Injection**: At transaction creation, the Gateway writes its configured posture string into `GovernanceEnvelope.Posture`.
4. **Authoritative Envelope Posture**: When the L4 Warden evaluates a transaction in an Operator runtime, it reads `envelope.Posture` via `postureFromEnvelope`. The envelope is authoritative; if the field is empty, the Warden rejects the transaction with `ErrEnvelopePostureMissing`.

### Canonical Transaction Envelope and Deterministic Hashing

The `GovernanceEnvelope` is the universal container for all governed platform intents. It binds identity, state, payload, and governance proofs:

```protobuf
message GovernanceEnvelope {
  string id = 1;
  google.protobuf.Timestamp timestamp = 2;
  google.protobuf.Timestamp expires_at = 3;
  Component source_component = 4;
  string operator_id = 5;
  string operator_session_id = 6;
  string web_session_id = 7;
  string cli_session_id = 22;
  string requestor_user_id = 25;
  string acting_app_id = 26;
  string event_type = 8;
  bytes payload = 9;
  google.protobuf.Struct intent_data = 10;
  string action_type = 19;
  string target_resource = 20;
  string state_merkle_root = 11;
  string nonce = 12;
  string transaction_hash = 13;
  string protocol_version = 21;
  GovernanceMetadata governance = 14;
  string case_id = 15;
  string investigation_id = 16;
  string task_id = 17;
  string system_fingerprint = 18;
  string tenant_id = 23;
  string binding_persona = 24;
  string posture = 27;
}
```

#### Deterministic Hash Calculation (`GenerateMessageID`)

All new transactions require `protocol_version = "2"` (`GovernanceProtocolVersionV2`). Version 2 canonical hashing is calculated as follows:

```
canonical_v2 = "g8e-tx-v2|" + action_type + "|" + event_type + "|" + canonical_v1
transaction_hash = sha256_hex(canonical_v2)
```

The embedded `canonical_v1` string serializes fields in strict canonical order with `|` delimiters:

1. `action_type` (UTF-8 string)
2. `target_resource` (UTF-8 string)
3. `payload` (Base64 standard encoding)
4. `state_merkle_root` (UTF-8 string)
5. `nonce` (UTF-8 string)
6. `expires_at` (RFC3339 formatted via `timesvc.FormatTimestamp`)
7. `intent_data` (recursively sorted keys: `key=val,key2=val2` via `canonicalizeStruct`)
8. `requestor_user_id` (UTF-8 string)
9. `acting_app_id` (UTF-8 string)
10. `operator_id` (UTF-8 string)
11. `operator_session_id` (UTF-8 string)
12. `case_id` (UTF-8 string)
13. `investigation_id` (UTF-8 string)
14. `task_id` (UTF-8 string)
15. `web_session_id` (UTF-8 string)
16. `cli_session_id` (UTF-8 string)

Absent optional fields append nothing prior to their terminating delimiter. Posture (`posture`), envelope ID (`id`), transaction hash (`transaction_hash`), timestamp (`timestamp`), and governance metadata (`governance`) are excluded from hashing. This guarantees that:
- L2 consensus members can sign the transaction hash before human L3 notary approval is gathered.
- The Gateway can stamp the posture into `envelope.Posture` without altering the cryptographic signature.
- Both `envelope.Id` and `envelope.TransactionHash` MUST match `transaction_hash`.

### Layer 1 (L1) Doctrine: Technical Bedrock

L1 Doctrine performs technical verification on the raw intent without external network or identity dependencies:

1. **Protobuf Field Option Screening**: Scans fields of decoded protobuf messages for the `(g8e.common.v1).forbidden_patterns` custom extension. When declared, comma-separated regular expressions are evaluated against string field values. Matches cause immediate rejection.
2. **Payload Threat Detection**:
   - `operatorv1.CommandRequested`: Command lines are scanned via `L1Doctrine.AnalyzeCommand` against built-in MITRE ATT&CK detectors and directory-loaded doctrine rules.
   - `operatorv1.McpCallRequested`: Arguments JSON strings are decoded and recursively scanned for injection patterns, shell metacharacters, and path traversals via `L1Doctrine.AnalyzeMCPArguments`.
   - `operatorv1.A2ACallRequested`: Sub-agent JSON skill payloads are recursively scanned for malicious tool invocation vectors.
   - `operatorv1.FileEditRequested`: Verifies that `FilePath` does not target critical system files (`/etc/passwd`, `/etc/shadow`, `/etc/sudoers`, `.ssh/authorized_keys`, root filesystem boundaries) and scans file contents for embedded threat patterns.
3. **Directory Doctrine Files**: `NewL1DoctrineFromDir` loads external JSON doctrine catalogs (e.g. `blacklist_doctrine.json`, `gitleaks_doctrine.json`, `owasp_crs_doctrine.json`). Loaded patterns combine with hardcoded MITRE detectors.
4. **Doctrine Bundle Identity**: Computes a deterministic SHA-256 `doctrine_bundle_hash` and `doctrine_bundle_version` (`g8e-l1-doctrine-v1+<source@version>`) recorded in deterministic stage evidence for auditing.

### Layer 2 (L2) Consensus: Multi-Agent Quorum

L2 Consensus authenticates machine review by requiring independent affirmative cryptographic votes:

1. **Vote Format**: Each member vote is an Ed25519 signature over the UTF-8 payload `<transaction_hash>|<decision>`, where decision is `true` or `false`.
2. **Policy Evaluation**: The Warden resolves the policy ID from `envelope.Governance.L2.ConsensusSetId`. The policy defines `MemberKeyIDs`, `Quorum`, `RequireDistinct`, and `Enabled`.
3. **Distinct Member Verification**: If `RequireDistinct` is true, multiple votes from the same `signer_key_id` reject the transaction with `ErrTxL2DuplicateSigner`.
4. **Public Key Resolution**: Each signer key is resolved from `SignerStore.GetTrustedSigner`. Unregistered or inactive signers are rejected.
5. **Quorum Evaluation**: Only affirmative (`decision=true`) votes signed by enrolled, trusted members count toward quorum. Negative votes record authenticated dissent. If affirmative votes < `Quorum`, the transaction is rejected with `ErrTxL2QuorumNotMet`.
6. **Bootstrap Action Exemption**: Platform bootstrap actions (actions where `actionType.IsBootstrapAction() == true`, such as initial platform enrollment) are exempt from L2 enforcement. They establish the initial consensus tribunal before signers exist. L2 votes are verified if present and recorded as evidence, but missing votes do not reject bootstrap envelopes.

### Layer 3 (L3) Notary: Human Authorization

L3 Notary provides non-repudiable human-in-the-loop authorization for mutations (`actionType.IsMutation() == true`):

1. **Mutation Classification**: Action types carry an intrinsic `_mutation` boolean in `protocol/constants/status.json`. Read-only operations (`FS_READ`, `FS_LIST`, `MCP_PROMPT_GET`, etc.) bypass L3 enforcement.
2. **Gateway Mode (`gatewayNotary`)**:
   - **Passkey Authorization**: Requires a WebAuthn passkey assertion. The browser client receives a challenge matching the transaction hash and returns an assertion containing `credential_id`, `authenticator_data`, `client_data_json`, and `signature`. Verified by `PasskeyVerifier`.
   - **CLI mTLS Session Layer**: CLI callers additionally include `mtls_cert_fingerprint` and `cli_signature`. The notary calls `CLISessionVerifier` to confirm the user is active, the CLI session is valid, and the certificate is not revoked.
3. **Outbound Operator Mode (`outboundNotary`)**:
   - Checks `SuspendedTransactionStore` for an approved record matching the transaction hash.
   - Requires `Approved == true` and verifies the approval was granted within the 30-minute window (`L3ApprovalWindow`).
   - Verifies an Ed25519 `cli_signature` over the transaction hash using the stored approval public key, and ensures `mtls_cert_fingerprint` matches the expected certificate.
4. **Suspension and Resume Mechanics**:
   - On Gateway MCP and A2A ingress, transactions lacking L3 proofs under `ratify` or `notary` postures are suspended. The Gateway stores the envelope in `SuspendedTransactionStore` and returns an approval challenge/URL to the caller.
   - Once approved via passkey WebAuthn, the transaction resumes execution with the minted proof.
   - On direct envelope submission (`/governance/v1/envelope`) and Gateway CLI dispatch, transactions lacking required L3 proofs are not suspended; they fail closed with `ErrTxL3ProofUnmintable` or `ErrTxL3ProofMissing`.

### Layer 4 (L4) Warden: Pre-Dispatch Verification Pipeline

The L4 Warden executes ordered pre-dispatch verification before any mutation reaches the execution runtime:

```
[ Incoming Envelope ]
        │
        ▼
0. Memory In-Flight Lock (sync.Map trackInFlight)
        │
        ▼
1. Durable Nonce Reservation (ReplayStore.ReserveNonce + Expiry Check)
        │
        ▼
2. Stateless Validation (Version 2, L1 Doctrine, Typed Decode, Document Collection Scope, Execution Target, Hash Match)
        │ ──[Failure]──► Release Nonce Reservation & Reject
        ▼
3. Stateful Validation (State Merkle Root Verification)
        │ ──[Failure]──► Release Nonce Reservation & Reject
        ▼
4. Posture Validation (L2 Consensus Quorum + L3 Notary Human Proof)
        │ ──[Failure]──► Release Nonce Reservation & Reject
        ▼
5. VerifiedTransaction Produced (Nonce Remains Reserved)
```

1. **Step 0: In-Flight Lock**: `trackInFlight(envelope.Nonce)` stores the nonce in a memory `sync.Map`. If another concurrent goroutine is processing the same nonce, it returns `ErrTxInFlight`.
2. **Step 1: Expiry and Durable Nonce Reservation**:
   - Confirms `envelope.ExpiresAt != nil` and `tv.clock.Now().Before(expiresAt)`. Expired nonces return `ErrTxTransactionExpired`.
   - Confirms `envelope.Nonce != ""` and reserves it in SQLite via `replayStore.ReserveNonce(nonce, expiresAt)`. Duplicate nonces return `ErrTxTransactionReplay`.
   - Releases the memory in-flight lock now that SQLite holds the durable lock.
3. **Step 2: Stateless Validation**:
   - Protocol version must equal `"2"`.
   - L1 Doctrine validator must be configured.
   - Envelope ID must not be empty.
   - Event type must be valid in the event registry.
   - Action type must be recognized in `knownActionTypes`.
   - Payload must be present (unless action type is `HEARTBEAT`).
   - Typed payload decoded via `DecodePayloadForAction`.
   - Document update/delete collection checked against the governed set.
   - Runtime execution target checked against `operator_id` (INV-GOV-WARD-07).
   - Decoded payload evaluated against L1 Doctrine (`ValidatePayload`).
   - Transaction hash computed via `GenerateMessageID`. Both `envelope.TransactionHash` and `envelope.Id` must match the computed hash.
4. **Step 3: Stateful Validation**:
   - State Merkle root must not be empty.
   - Current root fetched from `StateRootProvider` (or context override on in-process builds).
   - Envelope state root must match the current system state root.
5. **Step 4: Posture Validation**:
   - Reads posture from `envelope.Posture` via `postureFromEnvelope`.
   - Evaluates L2 Consensus votes (`verifyL2Posture`).
   - Evaluates L3 Notary proof (`verifyL3Posture`).
6. **Fail-Closed Nonce Release**:
   - If any check in Steps 2, 3, or 4 fails, `releaseNonceReservation(envelope.Nonce)` is invoked immediately so the nonce is not permanently locked by an unadmitted transaction.

### Layer 5 (L5) Actuator: Fail-Closed Execution Boundary

The L5 Actuator owns the execution boundary. It guarantees zero standing privileges and creates non-repudiable audit receipts:

1. **Pre-Execution Invariants**: Verifies `ExecutionHandler` is set, `SigningKey` is present, `EventType` is non-empty, and when `SQLAuditStore` is enabled, `AuditorSigningKey` and `AuditorKeyID` are present and match.
2. **Initial Receipt Logging**: Constructs an initial `ActionReceipt` with status `EXECUTION_STATUS_EXECUTING`, state root before, and L2/L3 status. Signs the receipt with `SigningKey` and logs it to `ConsoleAuditStore` and `SQLAuditStore`. If signing or logging fails, execution aborts immediately. Stage: `DETERMINISTIC_STAGE_KIND_RECEIPT_PERSISTENCE`.
3. **Commitment Append**: When `SQLAuditStore` is active, appends a signed `CommitmentAttestation` to the SQLite hash-chained commitment ledger. The attestation binds the transaction ID, hash, prior commitment hash, state root, L2 signature digest, warden intent signature digest, human signature digest, action type, target resource, and auditor key ID. Stage: `DETERMINISTIC_STAGE_KIND_COMMITMENT_APPEND`.
4. **Payload Rehydration**: Rehydrates sovereignty-scrubbed tokens into execution payloads in memory via `ScrubbingService`.
5. **JIT Capability Lifecycle**:
   - Mints a single-action capability (`MintCapability`) binding transaction hash, action type, target resource, operator ID, session ID, expiry, and an Ed25519-signed single-use token.
   - Attaches capability to the execution context (`ContextWithCapability`).
   - Dispatches execution to `ExecutionHandler.ExecuteVerifiedTransaction`.
   - Dissolves the capability (`cap.Dissolve()`) in a defer-safe block immediately after execution completes or fails. Expired or dissolved capabilities cannot be reused. Stage: `DETERMINISTIC_STAGE_KIND_L5_EXECUTION`.
6. **Receipt Finalization and Failure Classification**:
   - Updates receipt status to `EXECUTION_STATUS_COMPLETED` or `EXECUTION_STATUS_FAILED`.
   - Captures state root after execution.
   - Maps errors to typed `ReceiptFailureCode` (e.g. `RECEIPT_FAILURE_CODE_MODEL_OVERRIDE_DENIED`, `RECEIPT_FAILURE_CODE_BACKEND_TIMEOUT`, `RECEIPT_FAILURE_CODE_EXECUTION_FAILED`).
7. **Final Persistence Attestation**:
   - Signs the final receipt and logs it to the audit stores.
   - Generates a `ReceiptPersistenceAttestation` containing the transaction ID, final receipt signature digest, audit record ID, signer key ID, and timestamp, signed by the Actuator's private key.
   - Stores the attestation in `receipt.FinalPersistenceAttestation` and persists the updated record.
8. **Asynchronous Gateway Relay**: If a `ReceiptPublisher` is wired (outbound operator mode), publishes the signed `ActionReceipt` to the Gateway's `receipts:` pub/sub channel. Relay errors are logged as warnings and do not fail the execution; the local SQLite record remains authoritative.

### Deterministic Protocol Chain and Evidence Graph

Every completed or failed transaction that reaches the Actuator carries a deterministic evidence chain (`DeterministicProtocolChain`):

```
                       ┌─────────────────────────┐
                       │   L5 Execution Stage    │
                       └────────────┬────────────┘
                                    │ (parent)
          ┌─────────────────────────┼─────────────────────────┐
          │                         │                         │
          ▼                         ▼                         ▼
┌──────────────────┐      ┌──────────────────┐      ┌──────────────────┐
│   L4 Warden      │      │ Receipt Persist  │      │ Commitment Append│
│Verification Stage│      │      Stage       │      │      Stage       │
└─────────┬────────┘      └──────────────────┘      └──────────────────┘
          │ (parent)
   ┌──────┴──────┬─────────────┐
   ▼             ▼             ▼
┌──────┐      ┌──────┐      ┌──────┐
│  L1  │      │  L2  │      │  L3  │
│Doctr.│      │Consen│      │Notary│
└──────┘      └──────┘      └──────┘
```

1. **Ordered Stages**: The protocol chain defines seven strictly ordered stage kinds:
   - `DETERMINISTIC_STAGE_KIND_L1_DOCTRINE`
   - `DETERMINISTIC_STAGE_KIND_PROTOCOL_L2`
   - `DETERMINISTIC_STAGE_KIND_L3_NOTARY`
   - `DETERMINISTIC_STAGE_KIND_L4_VERIFICATION`
   - `DETERMINISTIC_STAGE_KIND_RECEIPT_PERSISTENCE`
   - `DETERMINISTIC_STAGE_KIND_COMMITMENT_APPEND`
   - `DETERMINISTIC_STAGE_KIND_L5_EXECUTION`
2. **Parentage Relationships**: L1, L2, and L3 stages declare `ParentStageId` equal to the L4 stage ID. L4 verification, receipt persistence, and commitment append declare `ParentStageId` equal to the L5 stage ID. The L5 execution stage has an empty parent ID.
3. **Identity Invariance**: All stages must declare identical values across all non-empty identity fields: `OperatorId`, `OperatorSessionId`, `RequestorUserId`, `ActingAppId`, `CaseId`, `InvestigationId`, and `TaskId`.
4. **Content Addressing**: Canonical stages serialize deterministically into a content reference string with prefix `urn:g8e:evidence:stages:sha256:<hex>`.

### End-to-End Transaction Lifecycles

#### Synchronous Gateway Execution (Embedded Operator)

In synchronous gateway mode, the Gateway's embedded Operator executes transactions in-process:

1. Client submits intent to the Gateway via HTTP or MCP.
2. Gateway builds the `GovernanceEnvelope` via `BuildGovernanceEnvelope`, validates L1 Doctrine, fetches current state Merkle root, injects posture, and hashes the transaction.
3. L4 Warden performs in-flight tracking, reserves the nonce in SQLite, verifies stateless properties, state root, and posture (L2/L3).
4. L5 Actuator signs initial receipt, appends commitment attestation, rehydrates payload, mints capability, executes the command handler, dissolves the capability, updates the receipt, signs persistence attestation, and persists to local SQLite audit stores.
5. Signed `ActionReceipt` returns directly in the HTTP or MCP response.

#### Outbound Operator WebSocket Dispatch

In outbound operator mode, sovereign Operators connect to the Gateway over outbound-only mTLS WebSockets:

1. Client submits intent to the Gateway.
2. Gateway constructs the `GovernanceEnvelope`, screening L1 Doctrine and verifying that mutations under `ratify`/`notary` postures contain valid L3 proofs (or rejecting with `ErrTxL3ProofUnmintable`).
3. Gateway publishes the envelope to the operator's dedicated session channel: `cmd:<operator-id>:<operator-session-id>`.
4. Operator receives the envelope over the WebSocket, runs its local L4 Warden, and reserves the nonce in its local SQLite replay store.
5. Operator's L5 Actuator executes the command locally, records the receipt in its local SQLite audit store, and appends the commitment to its local commitment ledger.
6. Operator publishes the result back to the Gateway via `res:<operator-id>:<operator-session-id>` and relays the signed receipt to `receipts:<operator-id>`.
7. Gateway verifies the relay signature and persistence attestation before recording the mirrored receipt in its Gateway audit store.

## Anti-patterns

- **Trusting Client-Supplied Posture**: Never allow clients to define or override the governance posture per request. Posture is set by Gateway configuration at startup and stamped into the envelope.
- **Executing Before Initial Receipt Persistence**: Never invoke an execution handler before the initial `EXECUTING` receipt and commitment attestation are persisted to durable storage.
- **Retaining Standing Privileges**: Never maintain long-lived capabilities or credentials. Capabilities must be minted just-in-time at L5 and dissolved immediately upon handler return.
- **Hashing Posture or Proofs**: Never include `posture`, `governance.l2`, or `governance.l3` in `GenerateMessageID`. Hashing governance metadata prevents L2 members from signing prior to human authorization.
- **Dangling Nonce Reservations**: Never fail an envelope verification without releasing the nonce reservation in `ReplayStore`.
- **Assuming Gateway Receipt Mirror is Authoritative**: Never treat Gateway-relayed receipts as primary execution proof. The sovereign Operator's local SQLite database is the authoritative system of record.
- **Bypassing Bootstrap Action Exemption**: Never block platform enrollment actions on L2 consensus quorums. Bootstrap actions must succeed to create the consensus tribunal.

## Links out

- [Consensus Architecture](consensus.md): L2 multi-member Ed25519 policies, deliberation mechanics, and quorum verification.
- [Gateway Architecture](gateway.md): Gateway operational modes, reverse proxy routing, and admission control.
- [Operator Architecture](operator.md): Operator runtime daemon, command execution boundary, and local audit storage.
- [Authentication & Authorization](auth.md): mTLS identity binding, WebAuthn passkey enrollment, and CLI session verification.
- [Encryption Architecture](encryption.md): Cryptographic primitives, key hierarchies, and TLS 1.3 configuration.
- [Network Architecture](network.md): PKI topology, SPIFFE identities, and pub/sub communication channels.
- [Event and Action Protocol](events.md): Canonical event taxonomy, routing keys, and action classifications.
- [AI Agents and Governance Boundary](agents.md): Agent ingress paths, downstream MCP egress, and tool call constraints.
- [Storage Architecture](storage.md): SQLite schema layouts, commitment ledgers, and document stores.
- [Documentation Guide](../devs/docs.md): Documentation invariants, audit procedures, and metadata standards.
