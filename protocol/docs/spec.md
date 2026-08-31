---
title: g8e Protocol
---

# g8e Protocol

Last Updated: 2026-08-30
Version: v2.1.0

The **g8e Protocol** is a zero-trust execution platform and compliance standard for agentic infrastructure. It defines the canonical `GovernanceEnvelope` that wraps all mutations passing through the g8e platform, enforcing fail-closed verification through the sequential 5-Layer interlock sequence. The platform uses `g8e.local` as the default internal hostname and canonical alias for all mesh communication.

---

## Protocol Overview

The g8e Protocol is the foundational wire contract for all mutations in the g8e platform. It provides a typed, signed, state-bound transaction envelope that binds identity, intent, state, and governance proofs into a single verifiable unit.

### Core Design Principles

- **Canonical JSON Wire Format**: All client-facing surfaces (HTTP, WSS pub/sub, receipts, audit exports) carry the `GovernanceEnvelope` as canonical JSON (protojson). Node binary protobuf is strictly reserved for internal storage.
- **g8e.local Canonical Alias**: The platform uses `g8e.local` as the stable internal hostname. The gateway translates this alias to installation-specific peer identity and endpoint data (see [Network Architecture](../../docs/architecture/network.md)).
- **Hash-Based Signing**: A deterministic `transaction_hash` is computed from normalized envelope fields. The verifier enforces `id == transaction_hash == SHA256(canonical_fields)`.
- **Fail-Closed Verification**: Any malformed envelope, expired transaction, reused nonce, stale state root, or missing proof is rejected immediately before execution.
- **Body-Embedded Context**: Business and execution context (`web_session_id`, `cli_session_id`, `operator_session_id`, `user_id`) lives inside the envelope via typed fields.
- **BFT State Binding**: Mutations carry a `state_merkle_root` that the Operator compares against its current host state.
- **Operator Sovereignty**: No bundled component has privileged channels. The g8e Operator is the only execution boundary, enforcing rules uniformly.

### Protocol Translation

The g8e Protocol does not compete with tool-calling standards. Instead, it wraps standard JSON-RPC tools (MCP, A2A, OpenAI tool calls, LangChain) as unverified payloads inside the strict `GovernanceEnvelope`:

1. **Inbound**: Client ecosystem generates typed payload (e.g., `CommandRequested`, `McpCallRequested`).
2. **Envelope Construction**: Payload embedded in `GovernanceEnvelope` with `nonce`, `expires_at`, `state_merkle_root`.
3. **Governance Proofs**: L2 Consensus signature and L3 Notary proof attached over `transaction_hash`.
4. **Verification**: Envelope passes through the 5-Layer interlock sequence (L1/L2/L3/L4).
5. **Execution**: Verified payload dispatched to L5Actuator for execution and receipt issuance.

---

## GovernanceEnvelope Schema

The `GovernanceEnvelope` is the single canonical container for all g8e mutations. The schema source of truth is defined in `../../protocol/proto/g8e/common/v1/common.proto`.

### Envelope Fields

| Field | Type | Description |
|---|---|---|
| `id` | string | Transaction identifier; must exactly match `transaction_hash` |
| `timestamp` | google.protobuf.Timestamp | Envelope creation time |
| `expires_at` | google.protobuf.Timestamp | UTC timestamp after which envelope is void |
| `source_component` | Component | Source component identifier (COMPONENT_AGENT, COMPONENT_G8EO, COMPONENT_CLIENT) |
| `operator_id` | string | Operator instance identifier |
| `operator_session_id` | string | Host-side agent session identifier |
| `web_session_id` | string | Browser frontend session identifier |
| `cli_session_id` | string | CLI/BYO client session identifier |
| `requestor_user_id` | string | The human user who authorized the action (delegator) |
| `acting_app_id` | string | The app/tool acting on behalf of the user (delegate) |
| `event_type` | string | Canonical event name from protocol/constants/events.json |
| `payload` | bytes | Raw protobuf payload containing execution instruction |
| `intent_data` | google.protobuf.Struct | Structured JSON-first view of intent |
| `action_type` | string | UAP-compatible action type (e.g., EXECUTE_BASH) |
| `target_resource` | string | UAP-compatible target resource |
| `state_merkle_root` | string | Expected host state root at time of signing |
| `nonce` | string | Unique replay-protection token |
| `transaction_hash` | string | SHA-256 over canonical envelope fields |
| `protocol_version` | string | UAP-compatible protocol version (e.g., "1.0") |
| `governance` | GovernanceMetadata | L1/L2/L3 governance proofs and status |
| `case_id` | string | Optional case identifier for application context |
| `investigation_id` | string | Optional investigation identifier |
| `task_id` | string | Optional task identifier |
| `system_fingerprint` | string | Optional system fingerprint |
| `tenant_id` | string | Optional tenant identifier |
| `binding_persona` | string | Optional binding persona |

### GovernanceMetadata

The `governance` field encapsulates all three governance layers:

| Field | Type | Description |
|---|---|---|
| `l1` | L1Metadata | L1 Doctrine status (validated flag, violations list) |
| `l2` | L2Metadata | L2 Consensus votes (consensus_set_id, signed L2Vote list) |
| `l3` | L3Metadata | L3 Notary proof (WebAuthn or CLI signature) |

### L2Vote

Each Consensus member produces a signed vote over the transaction hash:

| Field | Type | JSON Name | Description |
|---|---|---|---|
| `signer_key_id` | string | `signerKeyId` | Member app ID; must match a trusted signer in the SignerStore |
| `consensus_signature` | string | `consensusSignature` | Ed25519 signature over `<transaction_hash>|<decision>` |
| `decision` | bool | `decision` | Member vote: true (safe) or false (unsafe) |

### L3Proof

The L3 proof structure supports both WebAuthn and CLI-based authentication:

| Field | Type | JSON Name | Description |
|---|---|---|---|
| `client_data_json` | string | `clientDataJSON` | WebAuthn client data JSON (for web sessions) |
| `authenticator_data` | string | `authenticatorData` | WebAuthn authenticator data (for web sessions) |
| `signature` | string | `signature` | WebAuthn signature (for web sessions) |
| `credential_id` | string | `credentialId` | WebAuthn credential ID (for web sessions) |
| `mtls_cert_fingerprint` | string | `mtlsCertFingerprint` | SHA-256 fingerprint of mTLS certificate (for CLI sessions) |
| `cli_signature` | string | `cliSignature` | Ed25519 signature over transaction_hash (for CLI sessions) |

### Canonical JSON Wire Format

All envelopes use canonical JSON (protojson) encoding for client-facing surfaces:

- **Schema source of truth**: `.proto` files in `../../protocol/proto/`
- **Wire format**: canonical JSON (protojson)
- **Signing basis**: deterministic `transaction_hash` computed from normalized envelope fields in proto field order
- **Internal storage**: protobuf bytes (implementation detail)

The transaction hash is computed from the following fields in order: `action_type`, `target_resource`, `payload` (base64-encoded), `state_merkle_root`, `nonce`, `expires_at` (UTC fixed microsecond precision), `intent_data` (canonicalized map with sorted keys), `requestor_user_id`, and `acting_app_id`. The result is hashed with SHA-256 and hex-encoded. The L3 proof is intentionally excluded from the hash so that L2 consensus can sign before the human notary is asked to authorize.

This ensures compatibility with JSON-based ecosystems while maintaining typed schema validation.

---

## Transaction Lifecycle

The transaction lifecycle follows a strict sequence from intent to audited execution.

### Request Phase (Client -> Gateway -> Operator)

1. A client ecosystem generates a typed protobuf payload (e.g., `CommandRequested`).
2. The gateway detects the machine's network identity (IPs, hostnames, and aliases) using the detector in `../../internal/services/network/identity.go`.
3. The payload is embedded into a `GovernanceEnvelope` alongside `nonce`, `expires_at`, and `state_merkle_root`.
4. An L2 Consensus producer computes the `transaction_hash` and attaches a signature.
5. For mutations, an L3 Notary (human) signs the same hash via WebAuthn, unless auto-approval policy applies.
6. The client submits the canonical-JSON envelope over mTLS to the g8e Gateway, which validates and dispatches it to the target g8e Operator over WSS. Remote peers are resolved via `g8e.local` translation (see [Network Architecture](../../docs/architecture/network.md)).

### Verification Phase (L4Warden)

The `L4Warden` operates as the primary pre-dispatch validation gate, executing the following checks sequentially:

1. **Freshness**: Verifies `expires_at` and durably reserves the `nonce` in the replay store to prevent concurrent double-processing.
2. **L1 Doctrine**: Validates the action type, decodes the typed payload, and scans it against reflected `forbidden_patterns` and threat rules.
3. **Integrity**: Enforces `id == transaction_hash == SHA256(canonical_fields)`.
4. **State Binding**: Validates that the `state_merkle_root` matches the local ledger root.
5. **L2 Consensus**: Verifies Ed25519 signatures against the Operator's trusted `SignerStore` and checks quorum against the consensus policy.
6. **L3 Notary**: Validates the WebAuthn or CLI proof, or applies explicit auto-approval policy for the action.

### Execution & Receipt Phase (L5Actuator)

1. L4 emits signed-input-bound deterministic evidence for doctrine, consensus, notary, and final verification, including monotonic timing, identities, state roots, doctrine version, and signature digests.
2. The `L5Actuator` signs an executing-state `ActionReceipt` whose signature binds the canonical receipt fields and the hash of its deterministic stage evidence, then writes the complete protojson receipt to the fail-closed `SQLAuditStore`.
3. The actuator atomically appends a signed `CommitmentAttestation` to the commitment ledger and records the chain hashes in the receipt evidence.
4. Sensitive tokens scrubbed by the Sovereign Execution Boundary are re-injected via payload rehydration.
5. A JIT execution capability is minted, scoping the dispatch to the verified action type and target resource.
6. The typed payload is dispatched to its execution handler (for example, shell execution or file editing), and the capability is dissolved after dispatch.
7. The `L5Actuator` adds L5 outcome evidence, updates the receipt with the final status (`COMPLETED` or `FAILED`) and post-state root, and signs the updated receipt.
8. After the final receipt is durably stored, the actuator attaches a signed `ReceiptPersistenceAttestation` that binds the transaction, receipt-signature digest, audit record, signer, and persistence timestamp, then persists the resulting receipt.
9. The Operator publishes a result envelope carrying the typed result and signed receipt back to the Gateway.

---

## 5-Layer interlock sequence

Every mutation must pass through five independent layers in order. A failure at any layer is an immediate rejection.

### L1 Doctrine: Technical Bedrock
Static, deterministic checks enforced before any code executes. Validated using doctrines sourced from `../../protocol/constants/doctrine/doctrine_registry.json`. Code pattern matching and threat analysis are defined in `../../internal/services/governance/l1_doctrine.go`.
- **Forbidden Patterns**: The custom protobuf field option `(g8e.common.v1.forbidden_patterns)` is reflected at runtime to scan typed payloads.
- **Threat Detection**: L1 Doctrine analyzes command inputs for MITRE ATT&CK patterns, reverse shells, injection vectors, data destruction, system tampering, security bypass, malware deployment, credential access, persistence, lateral movement, defense evasion, reconnaissance, resource hijacking, and network manipulation.
- **Critical System File Protection**: Blocks modifications to critical system paths including `/etc/passwd`, `/etc/shadow`, `/etc/sudoers`, `/etc/ssh/`, `/etc/pam.d/`, `/etc/ld.so.*`, system binaries in `/bin/`, `/sbin/`, `/usr/bin/`, `/usr/sbin/`, and boot configurations.
- **Allow/Deny Lists**: Enforces per-host policy and user settings.

### L2 Consensus

L2 Consensus is a protocol concept defined by the g8e protocol (`L2Metadata`, `L2Vote`). It requires a cryptographic proof that an independent consensus set deliberated on the instruction and produced signed votes. The protocol defines the wire format generically; any implementation that produces valid `L2Metadata` with signed `L2Vote` entries can satisfy L2 Consensus.

#### Reference Implementation: Consensus

The Consensus service is the reference implementation of L2 Consensus shipped with g8e. It is an enrolled body of agentic applications that independently evaluate each transaction's payload. Consensus service logic is defined in `../../internal/services/consensus/service.go`.
- Each Consensus member independently evaluates the payload and signs an `L2Vote` over `transaction_hash | decision` using Ed25519.
- The `L2Metadata` contains the `consensus_set_id` and a list of `L2Vote` entries (signer key ID, signature, decision).
- The gateway never self-signs L2 votes. Under `consensus` and `notary` postures, the gateway calls the Consensus service's `/consensus/v1/deliberate` endpoint before dispatch.
- L4 Warden verifies quorum: at least `K` affirmative distinct signatures from the ConsensusPolicy's member set, checked against the `SignerStore`.
- For single-member consensus (degenerate case), the gateway's actuator signing key may be used as the member key. Multi-member consensus requires a separate key provisioning flow.

### L3 Notary: Human Authorization
Hardware-bound proof of human presence. Human-in-the-loop authorization (utilizing WebAuthn or cryptographically signed CLI proofs) is defined in `../../internal/services/governance/l3_notary.go`.
- **Web Sessions**: Real WebAuthn/FIDO2 proof with the transaction hash as the assertion challenge. WebAuthn passkey bootstrap provides secure initial enrollment without password dependencies.
- **CLI Sessions**: Authenticates via mTLS certificates with SPIFFE URI SANs. The L3 proof includes the SHA-256 fingerprint of the mTLS certificate (`mtls_cert_fingerprint`) and a cryptographic `cli_signature` over the `transaction_hash`. In outbound mode, transactions requiring L3 are suspended and must be approved via CLI commands such as `g8e approve <tx_hash>`. The suspended transaction records the `submitter_cli_session_id` so that the resulting `approval.completed` SSE event is scoped to that specific CLI session, preventing cross-session approval leakage.
- **Auto-Approval**: Explicit policy permits auto-approval for benign diagnostic verbs. Auto-approval never bypasses L1 or L2 gates.

### L4 Warden: Pre-dispatch Verification
The central Policy Execution Point (PEP) that validates the entire transaction proof before dispatch. Pre-dispatch verification gating (validating signatures, replay prevention, expiry, nonces, and state Merkle root) is defined in `../../internal/services/governance/l4_warden.go`.
- **Freshness & Replay**: Verifies `expires_at` and durably reserves the `nonce` in the replay store with early durable reservation to prevent concurrent double-processing.
- **Stateless Validation**: Validates the action type, decodes the typed payload, validates L1 Doctrine compliance, and enforces `id == transaction_hash`.
- **State Binding**: Compares `state_merkle_root` against the host ledger.
- **Posture-Aware Validation**: Enforces L2 (Ed25519 signature verification against `SignerStore` with ConsensusPolicy quorum) and L3 (WebAuthn or CLI proof) requirements based on governance posture (doctrine, consensus, ratify, or notary).

### L5 Actuator: Execution Boundary
The single fail-closed execution target that dispatches the verified payload and issues signed receipts. Isolated boundary tool dispatch (via MCP/A2A) and signed receipt production are defined in `../../internal/services/governance/l5_actuator.go`.
- **Rehydration**: Sensitive tokens scrubbed by the Sovereign Execution Boundary are re-injected.
- **JIT Capability Minting**: Mints a scoped execution capability bound to the verified action type and target resource, dissolved after dispatch.
- **Native Dispatch**: Executes the typed payload (bash, file edit, tool call).
- **Signed Action Receipts**: Issues an immutable `ActionReceipt` proof of execution and result.

---

## Event Types

The protocol defines canonical event types in `../../protocol/constants/events.json`. Events are categorized by domain:

### AI Agent Events
- `AiAgentConflictDetected`, `AiAgentConflictResolved`
- `AiAgentContinueApprovalRequested`, `AiAgentContinueApprovalGranted`, `AiAgentContinueApprovalRejected`
- `AiLLMChatIterationStarted`, `AiLLMChatIterationCompleted`, `AiLLMChatIterationFailed`, `AiLLMChatIterationRetry`, `AiLLMChatIterationStopped`
- `AiLLMChatIterationStreamStarted`, `AiLLMChatIterationStreamDeltaReceived`, `AiLLMChatIterationStreamCompleted`, `AiLLMChatIterationStreamFailed`
- `AiLLMChatIterationTextChunkReceived`, `AiLLMChatIterationTextCompleted`, `AiLLMChatIterationTextReceived`, `AiLLMChatIterationTextTruncated`
- `AiLLMChatIterationThinkingStarted`, `AiLLMChatIterationThinkingUpdate`, `AiLLMChatIterationThinkingEnd`
- `AiLLMChatIterationCitationsReceived`
- `AiLlmChatIterationThinkingStopped`
- `AiLLMChatFilterEvent`, `AiLLMChatMessageDeadLettered`, `AiLLMChatMessageProcessingFailed`, `AiLLMChatMessageReplayed`, `AiLLMChatMessageSent`
- `AiLLMChatStopHide`, `AiLLMChatStopShow`, `AiLLMChatSubmitted`
- `AiLLMConfigFailed`, `AiLLMConfigReceived`, `AiLLMConfigRequested`
- `AiLLMLifecycleCompleted`, `AiLLMLifecycleErrorOccurred`, `AiLLMLifecycleFailed`, `AiLLMLifecycleRequested`, `AiLLMLifecycleStarted`, `AiLLMLifecycleStopped`
- `AiLLMToolG8eCommandConstraintsCompleted`, `AiLLMToolG8eCommandConstraintsFailed`, `AiLLMToolG8eCommandConstraintsReceived`, `AiLLMToolG8eCommandConstraintsRequested`
- `AiLLMToolG8eInvestigationQueryCompleted`, `AiLLMToolG8eInvestigationQueryFailed`, `AiLLMToolG8eInvestigationQueryReceived`, `AiLLMToolG8eInvestigationQueryRequested`
- `AiLLMToolG8eWebSearchCompleted`, `AiLLMToolG8eWebSearchFailed`, `AiLLMToolG8eWebSearchReceived`, `AiLLMToolG8eWebSearchRequested`
- `AiReputationStateUpdated`
- `AiTriageClarificationAnswered`, `AiTriageClarificationQuestions`, `AiTriageClarificationSkipped`, `AiTriageClarificationTimeout`
- `AiConsensusSessionStarted`, `AiConsensusSessionCompleted`, `AiConsensusSessionDisabled`, `AiConsensusSessionGenerationFailed`, `AiConsensusSessionModelNotConfigured`, `AiConsensusSessionProviderUnavailable`, `AiConsensusSessionSystemError`, `AiConsensusSessionAuditorFailed`, `AiConsensusSessionWardenBlocked`
- `AiConsensusVotingPassCompleted`, `AiConsensusVotingConsensusReached`, `AiConsensusVotingConsensusNotReached`, `AiConsensusVotingConsensusFailed`, `AiConsensusVotingRoundStarted`, `AiConsensusVotingRoundCompleted`, `AiConsensusVotingRound2Started`, `AiConsensusVotingRound2ConsensusReached`, `AiConsensusVotingRound2ConsensusFailed`, `AiConsensusVotingDissentRecorded`, `AiConsensusVotingAuditStarted`, `AiConsensusVotingAuditCompleted`

### Application Events
- `AppCaseAssigned`, `AppCaseCleared`, `AppCaseClosed`, `AppCaseCreated`, `AppCaseCreationRequested`, `AppCaseDeleted`, `AppCaseEscalated`, `AppCaseResolved`, `AppCaseSelected`, `AppCaseSwitched`, `AppCaseUpdateRequested`, `AppCaseUpdated`
- `AppInvestigationChatMessageAi`, `AppInvestigationChatMessageSystem`, `AppInvestigationChatMessageUser`
- `AppInvestigationClosed`, `AppInvestigationCreated`, `AppInvestigationDeleted`, `AppInvestigationEscalated`, `AppInvestigationListCompleted`, `AppInvestigationListFailed`, `AppInvestigationListReceived`, `AppInvestigationListRequested`, `AppInvestigationLoaded`, `AppInvestigationRequested`, `AppInvestigationStarted`
- `AppInvestigationStatusUpdatedClosed`, `AppInvestigationStatusUpdatedEscalated`, `AppInvestigationStatusUpdatedOpen`, `AppInvestigationStatusUpdatedResolved`, `AppInvestigationUpdated`
- `AppMemoryCreated`, `AppMemoryUpdated`
- `AppTaskAssigned`, `AppTaskCompleted`, `AppTaskCreated`, `AppTaskFailed`, `AppTaskStarted`, `AppTaskUpdated`

### Command Execution Events
- `OperatorCommandRequested`, `OperatorCommandStarted`, `OperatorCommandCompleted`, `OperatorCommandFailed`, `OperatorCommandExecution`, `OperatorCommandResult`
- `OperatorCommandOutputReceived`
- `OperatorCommandApprovalRequested`, `OperatorCommandApprovalPreparing`, `OperatorCommandApprovalGranted`, `OperatorCommandApprovalRejected`
- `OperatorCommandCancelRequested`, `OperatorCommandCancelAcknowledged`, `OperatorCommandCancelFailed`, `OperatorCommandCancelled`
- `OperatorCommandStatusUpdatedQueued`, `OperatorCommandStatusUpdatedRunning`, `OperatorCommandStatusUpdatedCompleted`, `OperatorCommandStatusUpdatedFailed`, `OperatorCommandStatusUpdatedCancelled`

### File System Events
- `OperatorFilesystemReadRequested`, `OperatorFilesystemReadStarted`, `OperatorFilesystemReadCompleted`, `OperatorFilesystemReadFailed`, `OperatorFilesystemReadReceived`
- `OperatorFilesystemListRequested`, `OperatorFilesystemListStarted`, `OperatorFilesystemListCompleted`, `OperatorFilesystemListFailed`, `OperatorFilesystemListReceived`
- `OperatorFilesystemGrepRequested`, `OperatorFilesystemGrepStarted`, `OperatorFilesystemGrepCompleted`, `OperatorFilesystemGrepFailed`, `OperatorFilesystemGrepReceived`
- `OperatorFileEditRequested`, `OperatorFileEditStarted`, `OperatorFileEditCompleted`, `OperatorFileEditFailed`, `OperatorFileEditTimeout`
- `OperatorFileEditApprovalRequested`, `OperatorFileEditApprovalFeedback`, `OperatorFileEditApprovalGranted`, `OperatorFileEditApprovalRejected`
- `OperatorFileHistoryFetchRequested`, `OperatorFileHistoryFetchStarted`, `OperatorFileHistoryFetchCompleted`, `OperatorFileHistoryFetchFailed`, `OperatorFileHistoryFetchReceived`
- `OperatorFileDiffFetchRequested`, `OperatorFileDiffFetchStarted`, `OperatorFileDiffFetchCompleted`, `OperatorFileDiffFetchFailed`, `OperatorFileDiffFetchReceived`
- `OperatorFileRestoreRequested`, `OperatorFileRestoreReceived`, `OperatorFileRestoreCompleted`, `OperatorFileRestoreFailed`

### Audit & Governance Events
- `OperatorAuditCommandRecorded`, `OperatorAuditUserRecorded`, `OperatorAuditAiRecorded`
- `OperatorAuditDirectCommandRecorded`, `OperatorAuditDirectCommandResultRecorded`, `OperatorAuditMcpCallRecorded`
- `OperatorBootstrapRequested`, `OperatorBootstrapReceived`, `OperatorBootstrapConfigReceived`, `OperatorBootstrapCompleted`, `OperatorBootstrapFailed`

### MCP/A2A Events
- `OperatorMcpCallRequested`
- `OperatorA2aCallRequested`

### Network Events
- `OperatorNetworkPingRequested`, `OperatorNetworkPingReceived`, `OperatorNetworkPingCompleted`, `OperatorNetworkPingFailed`
- `OperatorNetworkPortCheckRequested`, `OperatorNetworkPortCheckReceived`, `OperatorNetworkPortCheckStarted`, `OperatorNetworkPortCheckCompleted`, `OperatorNetworkPortCheckFailed`
- `OperatorPortCheckRequested`

### Operator Lifecycle Events
- `OperatorBound`, `OperatorDeviceRegistered`, `OperatorUnbound`
- `OperatorHeartbeatRequested`, `OperatorHeartbeatSent`, `OperatorHeartbeatReceived`, `OperatorHeartbeatMissed`
- `OperatorShutdownRequested`, `OperatorShutdownAcknowledged`
- `OperatorStatusUpdatedAvailable`, `OperatorStatusUpdatedActive`, `OperatorStatusUpdatedBound`, `OperatorStatusUpdatedOffline`, `OperatorStatusUpdatedStale`, `OperatorStatusUpdatedStopped`, `OperatorStatusUpdatedTerminated`, `OperatorStatusUpdatedUnavailable`
- `OperatorSlotInitializationFailed`
- `OperatorContextChanged`

### Notary Events
- `OperatorNotaryApprovalRequested`, `OperatorNotaryTransactionExpired`

### Other Events
- `OperatorEvalAnswerRequested`
- `OperatorFieldReadRequested`, `OperatorFieldReadAccessGranted`, `OperatorFieldReadAccessDenied`
- `OperatorIntentRequested`, `OperatorIntentGranted`, `OperatorIntentDenied`, `OperatorIntentRevoked`, `OperatorIntentRevokeRequested`
- `OperatorIntentApprovalRequested`, `OperatorIntentApprovalGranted`, `OperatorIntentApprovalRejected`
- `OperatorLogsFetchRequested`, `OperatorLogsFetchReceived`, `OperatorLogsFetchCompleted`, `OperatorLogsFetchFailed`
- `OperatorHistoryFetchRequested`, `OperatorHistoryFetchReceived`, `OperatorHistoryFetchCompleted`, `OperatorHistoryFetchFailed`
- `OperatorPanelListUpdated`
- `OperatorStreamApprovalRequested`, `OperatorStreamApprovalGranted`, `OperatorStreamApprovalRejected`
- `OperatorTerminalApprovalDenied`, `OperatorTerminalAuthStateChanged`, `OperatorTerminalThinkingAppend`, `OperatorTerminalThinkingComplete`

### Platform Events
- `PlatformAuthComponentInitializedAuthstate`, `PlatformAuthComponentInitializedChat`, `PlatformAuthComponentInitializedOperator`
- `PlatformAuthInfo`
- `PlatformAuthLoginFailed`, `PlatformAuthLoginRequested`, `PlatformAuthLoginSucceeded`
- `PlatformAuthLogoutFailed`, `PlatformAuthLogoutRequested`, `PlatformAuthLogoutSucceeded`
- `PlatformAuthSessionExpired`, `PlatformAuthSessionValidationFailed`, `PlatformAuthSessionValidationRequested`, `PlatformAuthSessionValidationSucceeded`
- `PlatformAuthUserAuthenticated`, `PlatformAuthUserUnauthenticated`
- `PlatformConsoleLogConnectedConfirmed`, `PlatformConsoleLogEntryReceived`
- `PlatformExternalServiceConfigured`
- `PlatformNotification`
- `PlatformVaultModeChanged`
- `PlatformSseConnectionClosed`, `PlatformSseConnectionError`, `PlatformSseConnectionEstablished`, `PlatformSseConnectionFailed`, `PlatformSseConnectionOpened`, `PlatformSseKeepaliveSent`
- `PlatformTelemetryAuditLogged`, `PlatformTelemetryErrorLogged`, `PlatformTelemetryHealthReported`, `PlatformTelemetryPerformanceRecorded`
- `PlatformTerminalClosed`, `PlatformTerminalMaximized`, `PlatformTerminalMinimized`, `PlatformTerminalOpened`
- `PlatformUsageUpdated`

### Reputation Events
- `OperatorReputationCommitmentCreated`, `OperatorReputationCommitmentFailed`, `OperatorReputationCommitmentVerified`
- `OperatorReputationSlashTier1`, `OperatorReputationSlashTier2`, `OperatorReputationSlashTier3`, `OperatorReputationStateUpdated`
- `ReputationStateUpdated`

### Source Events
- `SourceAiAssistant`, `SourceAiPrimary`, `SourceAiTriage`
- `SourceSystem`
- `SourceUserChat`, `SourceUserTerminal`

---

## Session Management

The protocol enforces strict separation between session types to guarantee context integrity.

| Session | Identifier | Use | Auth |
|---|---|---|---|
| **Operator** | `operator_session_id` | Host-side agent | mTLS (operator cert, URI SAN) |
| **CLI** | `cli_session_id` | BYO/CLI client | mTLS (CLI cert, URI SAN) |
| **Web** | `web_session_id` | Browser frontend | Passkey (WebAuthn) |

Sessions are cryptographically bound to their authentication mechanism and cannot be conflated.

### SSE & Pub/Sub Routing

SSE and pub/sub routing uses a two-dimensional model that separates ownership from delivery:

- **Ownership dimension**: `user_id` is always required. It identifies the human user who owns the event stream and is bound by the authenticated identity (mTLS certificate or web session cookie). `user_id` alone is not a valid delivery target.
- **Delivery dimension**: Exactly one of `web_session_id` or `cli_session_id` must be set. This identifies the specific session that should receive the event. The two session IDs are mutually exclusive on the delivery axis.

The prior three-way mutually-exclusive routing model (`user_id` / `web_session_id` / `cli_session_id` — pick exactly one) is removed. The `user_id`-only fan-out route is removed entirely, so approval events are scoped to the specific CLI session that submitted the transaction, eliminating cross-session leakage.

Routing and identity identifiers are carried in headers and auth context, not in URL query strings. For mTLS clients, `user_id` is derived from the certificate and the session ID is sent via the `X-G8E-CLI-Session-ID` or `X-G8E-Web-Session-ID` header. For cookie-authenticated browser clients, `web_session_id` is derived from the session cookie. This matches the transport model used by every other gateway controller and prevents identity/routing IDs from leaking into access logs, browser history, and referrer headers.

See [SSE Streaming](../../docs/architecture/sse.md) for the full endpoint and security model.

---

## Error Handling

Protocol errors follow standardized JSON-RPC codes for MCP/A2A client compatibility. Codes are defined in `../../internal/constants/rpc_errors.go`.

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

---

## Configuration

### Gateway Postures

The g8e Gateway runs with four posture options:

| Mode | Flag | Purpose |
|---|---|---|
| **Doctrine** | `--posture doctrine` | L1 enforced, L2/L3 audited (default) |
| **Consensus** | `--posture consensus` | L1/L2 enforced, L3 audited |
| **Ratify** | `--posture ratify` | L1/L3 enforced, L2 audited |
| **Notary** | `--posture notary` | L1/L2/L3 strictly enforced |

### Port Configuration

The g8e Gateway exposes two logical protocol surfaces in a consolidated 2-port configuration:
- **HTTP port 8080**: Bootstrap, CLI recovery discovery, deploy scripts, node binary download, health checks, and a catch-all redirect to HTTPS
- **HTTPS port 8443**: mTLS API and public surface, including MCP/A2A ingress, SSE, console, and authenticated API routes

See [Network Architecture](../../docs/architecture/network.md) for detailed port topology, authentication requirements, and port constraints.

### Configuration

The g8e platform uses CLI flags for production configuration. All paths are computed relative to project root. The following environment variables are supported:

- `G8E_CONSENSUS_ID`: ID of the ConsensusPolicy for L2 consensus (required for `--consensus` posture)
- `G8E_CONSENSUS_URL`: URL of the Consensus service for L2 deliberation (e.g. `https://localhost:8443/consensus/v1/deliberate`)
- `G8E_CONSENSUS_BOOTSTRAP`: Path to a JSON file that seeds a ConsensusPolicy and trusted signers at startup (overrides `--consensus-bootstrap`)
- `G8E_DOCTRINE_DIR`: Directory containing doctrine JSON files for L1 threat detection (overrides `--doctrine-dir`)
- `G8E_VAULT_DIR`: Directory for vault data (overrides `--vault-dir`)
- `G8E_VAULT_KEY`: Path to vault private key (overrides `--vault-key`)
- `G8E_PASSKEY_RP_ID`: RP ID for passkey operations (overrides `--passkey-rp-id`)
- `G8E_PASSKEY_RP_NAME`: RP Name for passkey operations (overrides `--passkey-rp-name`)
- `G8E_PASSKEY_RP_ORIGINS`: Comma-separated additional RP origins for passkey operations (overrides `--passkey-rp-origin`)
- `G8E_PUBLIC_BASE_URL`: Public base URL for approval links and host validation (overrides `--public-base-url`)
- `G8E_ALLOWED_ORIGINS`: Comma-separated allowed CORS origins for cross-origin browser access (overrides `--cors-origin`)

CLI flags:

- `--posture <mode>`: Gateway posture: doctrine (L1 enforced, L2/L3 audited), consensus (L1/L2 enforced, L3 audited), ratify (L1/L3 enforced, L2 audited), notary (L1/L2/L3 strictly enforced) (default: doctrine)
- `--data-dir <dir>`: Data directory for SQLite database (default: `.g8e/data` in working directory)
- `--pki-dir <dir>`: Directory for TLS certificates (default: `.g8e/pki`)
- `--secrets-dir <dir>`: Directory for platform secrets (default: `.g8e/secrets`)
- `--vault-dir <dir>`: Directory for vault data (default: `.g8e/vault`)
- `--vault-key <path>`: Path to vault private key (default: `.g8e/vault/key`)
- `--http-port <port>`: HTTP port for bootstrap and MCP routes (flag default: 0, auto-resolved from `constants.Ports`; effective default: 8080)
- `--https-port <port>`: HTTPS port for mTLS API and public surface (flag default: 0, auto-resolved from `constants.Ports`; effective default: 8443)
- `--passkey-rp-id <id>`: RP ID for passkey operations (default: localhost)
- `--passkey-rp-name <name>`: RP Name for passkey operations (default: g8e)
- `--rate-limit-rps <n>`: Gateway requests per second limit (set to 0 to disable)
- `--rate-limit-burst <n>`: Gateway rate limit burst size
- `--log <level>`: Log level: info, error, debug (default: info)
- `--cert-mode <mode>`: Certificate mode: full (all hostnames/IPs), localhost (only localhost)
- `--consensus-id <id>`: ID of the ConsensusPolicy for L2 consensus (required for `--consensus` posture, env: `G8E_CONSENSUS_ID`)
- `--consensus-url <url>`: URL of the Consensus service for L2 deliberation (env: `G8E_CONSENSUS_URL`)
- `--consensus-bootstrap <path>`: Path to a JSON file that seeds a ConsensusPolicy and trusted signers at startup (env: `G8E_CONSENSUS_BOOTSTRAP`)
- `--doctrine-dir <dir>`: Directory containing doctrine JSON files for L1 threat detection (default: hardcoded MITRE patterns only, env: `G8E_DOCTRINE_DIR`)
- `--mcp-downstream-url <url>`: URL of a downstream MCP server to proxy discovery and execution to (default: none)
- `--a2a-downstream-url <url>`: URL of a downstream A2A server to proxy execution to (default: none)
- `--public-base-url <url>`: Public base URL for approval links and host validation (env: `G8E_PUBLIC_BASE_URL`)
- `--cors-origin <origin>`: Allowed CORS origin for cross-origin browser access (repeatable, env: `G8E_ALLOWED_ORIGINS`)
- `--passkey-rp-origin <origin>`: Additional RP origin for passkey operations (repeatable, env: `G8E_PASSKEY_RP_ORIGINS`)
- `--follow`: Run gateway in foreground (Ctrl+C stops gateway)
- `--interactive` / `-i`: Launch interactive onboarding wizard before starting the gateway

---

## Host Sovereignty & Data Audit

### Multi-Ledger Architecture

The storage layer has been refactored into specialized services for separation of concerns:

- **GitLedgerService** (`ledger.go`): Maintains git-backed version control of all file mutations with `LedgerHashBefore` and `LedgerHashAfter`. Every file mutation triggers a native Go `go-git` commit.
- **ExecutionVaultService** (`execution_vault.go`): SQLite-backed storage for command execution results and file diffs, with encryption at rest.
- **CommitmentLedger** (`commitment_ledger.go`): SQLite-backed storage for commitment attestations with atomic append operations.
- **SQLAuditStore** (`audit_store.go`): Fail-closed audit logging for events, sessions, and action receipts with mandatory encryption of sensitive fields.

### Fail-Closed Audit Store

The SQLite-backed `SQLAuditStore` mandates valid session identifiers and rejects malformed events. Sensitive fields (`content_text`, `command_stdout`, `command_stderr`) are encrypted at rest using the required `vault.Vault`. Encryption at rest is mandatory for all storage services. If audit logging fails, execution is aborted.

### Sovereign Execution Boundary

Output scrubbing is performed directly at the `L5Actuator` boundary to redact tokens, keys, and PII before any data leaves the host.

---

## Test Coverage & Code Quality

### Test Coverage Status

The codebase maintains comprehensive test coverage across all layers:

- **Governance Layer**: Unit tests for L1 Doctrine (`l1_doctrine_test.go`), L2 Consensus (`l4_warden_consensus_test.go`), L3 Notary (`l3_notary_test.go`, `l3_notary_integration_test.go`), L4 Warden (`l4_warden_test.go`), and L5 Actuator (`l5_actuator_test.go`, `l5_actuator_integration_test.go`)
- **Storage Layer**: Extensive test suites for SQLAuditStore (in `storagetest/`), GitLedgerService (`ledger_test.go`, `ledger_git_test.go`), ExecutionVaultService (`execution_vault_test.go`), and supporting stores (replay, token, suspended transaction)
- **Gateway Layer**: Comprehensive tests for envelope construction (`governance_envelope_test.go`, `governance_envelope_quality_test.go`), authentication (`gateway_auth_test.go`, `bootstrap_controller_test.go`, `enrollment_token_controller_test.go`, `session_controller_test.go`, `user_controller_test.go`), and session management
- **MCP/A2A Layer**: Integration tests for MCP gateway translation and native tools
- **E2E Tests**: Docker-based end-to-end tests supporting both root and demo environments via `G8E_TEST_ENV` environment variable

All tests follow a Tier 1 philosophy where possible (no external network/DB requirements) to ensure fast, reliable CI/CD execution.

## Implementation Reference

| Concern | File |
|---|---|
| Protobuf schemas | `../../protocol/proto/g8e/common/v1/common.proto` |
| Event registry | `../../protocol/constants/events.json` |
| Channel prefixes | `../../protocol/constants/channels.json` |
| Envelope types | `../../internal/governance/envelope.go` |
| L1 Doctrine logic | `../../internal/services/governance/l1_doctrine.go` |
| L3 Notary logic | `../../internal/services/governance/l3_notary.go` |
| L4 Warden logic | `../../internal/services/governance/l4_warden.go` |
| L5 Actuator logic | `../../internal/services/governance/l5_actuator.go` |
| Consensus service | `../../internal/services/consensus/service.go` |
| Audit storage | `../../internal/services/storage/audit_store.go` |
| Git ledger storage | `../../internal/services/storage/ledger.go` |
| Execution vault storage | `../../internal/services/storage/execution_vault.go` |
| Commitment ledger storage | `../../internal/services/storage/commitment_ledger.go` |
| Replay store | `../../internal/services/storage/replay_store.go` |
| Token store | `../../internal/services/storage/token_store.go` |
| Suspended transaction store | `../../internal/services/storage/suspended_transaction_store.go` |
| Network identity detector | `../../internal/services/network/identity.go` |
| Network architecture | [../../docs/architecture/network.md](../../docs/architecture/network.md) |
| Gateway envelope construction | `../../internal/services/gateway/governance_controller.go` |
| Gateway HTTP handler | `../../internal/services/gateway/gateway_http.go` |
| Gateway HTTP routing | `../../internal/services/gateway/gateway_http_router.go` |
| Pub/Sub command service | `../../internal/services/pubsub/pubsub_commands.go` |
| Pub/Sub results service | `../../internal/services/pubsub/pubsub_results.go` |
| MCP/A2A translation | `../../internal/services/mcp/gateway.go` |
| Session management | `../../internal/services/gateway/cli_session_service.go`, `../../internal/services/gateway/operator_session_service.go`, `../../internal/services/gateway/web_session_service.go` |
| CLI session verification | `../../internal/services/gateway/cli_session_verifier.go` |
| Doctrine registry | `../../protocol/constants/doctrine/doctrine_registry.json` |
| MCP vectors doctrine | `../../protocol/constants/doctrine/mcp_vectors_doctrine.json` |
| Gitleaks doctrine | `../../protocol/constants/doctrine/gitleaks_doctrine.json` |
| OWASP CRS doctrine | `../../protocol/constants/doctrine/owasp_crs_doctrine.json` |
| Blacklist doctrine | `../../protocol/constants/doctrine/blacklist_doctrine.json` |
| Whitelist doctrine | `../../protocol/constants/doctrine/whitelist_doctrine.json` |
| RPC error codes | `../../internal/constants/rpc_errors.go` |
| Governance posture | `../../internal/services/governance/posture.go` |

---

## Example GovernanceEnvelope with an MCP Tool Call

### Example 1: MCP File Read Tool Call

```json
{
  "id": "a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2",
  "timestamp": "2026-06-02T18:27:00Z",
  "expiresAt": "2026-06-02T18:32:00Z",
  "sourceComponent": "COMPONENT_CLIENT",
  "operatorId": "op-prod-12345",
  "operatorSessionId": "sess-abc-789",
  "webSessionId": "web-xyz-456",
  "eventType": "g8e.v1.operator.mcp.call.requested",
  "payload": "CgZmc19yZWFkEglleGVjLTIwMzUSCgoZmlsZTovLy9ob21lL3VzZXIvcmVhZG1lLm1kGgZzY3J1Yg==",
  "intentData": {
    "tool": "fs_read",
    "path": "/home/user/readme.md",
    "reason": "Read deployment documentation"
  },
  "actionType": "MCP_CALL",
  "targetResource": "file:///home/user/readme.md",
  "stateMerkleRoot": "abc123def456abc123def456abc123def456abc123def456abc123def456abc1",
  "nonce": "nonce-1717358820000-abc123",
  "transactionHash": "a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2",
  "protocolVersion": "1.0",
  "governance": {
    "l1": {
      "validated": true,
      "violations": []
    },
    "l2": {
      "consensusSetId": "consensus-prod-abc123",
      "votes": [
        {
          "signerKeyId": "agent-ensemble-1",
          "consensusSignature": "4a5b6c7d8e9f0a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2...c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2",
          "decision": true
        },
        {
          "signerKeyId": "agent-ensemble-2",
          "consensusSignature": "5b6c7d8e9f0a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2...d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3",
          "decision": true
        },
        {
          "signerKeyId": "agent-ensemble-3",
          "consensusSignature": "6c7d8e9f0a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2...e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4",
          "decision": true
        }
      ]
    },
    "l3": {
      "proof": {
        "clientDataJSON": "{\"challenge\":\"a1b2c3d4e5f6\",\"origin\":\"https://g8e.ai\",\"type\":\"webauthn.get\"}",
        "authenticatorData": "SZYN5YgOjGh0NBcPZHZgW4_krrmihjLHmVzzuoMdl2NFAAAAAQ",
        "signature": "MEUCIQDWn3x4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2IgE5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2",
        "credentialId": "abc123def456abc123def456abc123def456abc123def456abc123def456abc1"
      }
    }
  },
  "caseId": "case-deploy-456",
  "taskId": "task-readme-789",
  "systemFingerprint": "fp-linux-amd64-abc123",
  "tenantId": "tenant-prod-xyz"
}
```

The `payload` field contains base64-encoded protobuf bytes of `McpCallRequested` with:
- `tool_name`: "fs_read"
- `arguments_json`: "{\"path\":\"/home/user/readme.md\"}"
- `execution_id`: "exec-2035"

---

## Related Documentation

- [**g8e Operator**](../../docs/architecture/operator.md) - Operator architecture and execution boundary
- [**g8e Gateway**](../../docs/architecture/gateway.md) - Gateway architecture
- [**MCP Protocol**](./mcp.md) - MCP protocol specification and integration
- [**A2A Protocol**](./a2a.md) - A2A protocol specification and integration
