# Protocol

## Overview

The g8e protocol is the canonical wire contract and governance specification governing all interactions between the Agentic Ensemble (`g8ee`), the Governance Gateway (`g8eg`), and the Governed Operator (`g8eo`). All mutations across the platform flow through typed, verifiable transactions encapsulated in canonical `GovernanceEnvelope` containers.

The protocol provides schema definitions, JSON constant registries, Pydantic models, SPIFFE workload identity helpers, and deterministic transaction hashing to enforce fail-closed verification across the platform's 5-layer interlock sequence.

## GovernanceEnvelope

The `GovernanceEnvelope` is the single canonical container for all mutations in the g8e platform. It binds identity, intent, state, replay-protection material, and multi-layer governance proofs into a single verifiable unit.

### Schema Fields

| Field | Type | Description |
| --- | --- | --- |
| `id` | `str` | Transaction identifier; must exactly match the computed `transaction_hash` |
| `timestamp` | `UTCDatetime` | Envelope creation timestamp in UTC |
| `expires_at` | `UTCDatetime` | Expiry timestamp after which the envelope is void |
| `source_component` | `str` | Proto enum name identifying the originating component (`COMPONENT_AGENT`, `COMPONENT_CLIENT`, `COMPONENT_G8EO`) |
| `operator_id` | `str \| None` | Target operator instance identifier |
| `operator_session_id` | `str \| None` | Target host-side agent session identifier |
| `web_session_id` | `str \| None` | Browser frontend session identifier |
| `cli_session_id` | `str \| None` | CLI/BYO client session identifier |
| `requestor_user_id` | `str \| None` | Human user who authorized or initiated the action (delegator) |
| `acting_app_id` | `str \| None` | App or agent acting on behalf of the user (`spiffe://g8e.local/app/g8ee`) |
| `event_type` | `str` | Canonical event name from protocol constant registry |
| `payload` | `str` | Base64-encoded serialized protobuf bytes containing the execution instruction |
| `intent_data` | `dict[str, Any]` | Structured key-value map representing the intent context |
| `action_type` | `str` | UAP-compatible action type (e.g., `EXECUTE_BASH`, `FILE_EDIT`, `DOCUMENT_UPDATE`) |
| `target_resource` | `str` | Target resource path or identifier |
| `state_merkle_root` | `str` | Expected host state root at time of transaction construction |
| `nonce` | `str` | Cryptographically secure unique replay-protection token |
| `transaction_hash` | `str \| None` | SHA-256 digest over normalized envelope fields |
| `protocol_version` | `str` | Protocol version string (e.g., `"1.0"`) |
| `governance` | `GovernanceMetadata` | Container for L1, L2, and L3 governance proofs |
| `case_id` | `str \| None` | Optional case identifier for application context |
| `investigation_id` | `str \| None` | Optional investigation identifier |
| `task_id` | `str \| None` | Optional task identifier |
| `system_fingerprint` | `str \| None` | Optional system fingerprint |
| `tenant_id` | `str \| None` | Optional tenant identifier |
| `binding_persona` | `str \| None` | Optional persona identifier |
| `posture` | `str \| None` | Governance posture at envelope construction (`doctrine`, `consensus`, `ratify`, `notary`) |

### GovernanceMetadata and Proofs

The `governance` field encapsulates validation artifacts across the first three governance layers:

- **`l1` (`GovernanceL1`)** — Technical bedrock validation status, containing `validated` (`bool`) and `violations` (`list[str]`).
- **`l2` (`GovernanceL2`)** — Consensus proof, containing `consensus_set_id` (`str`) and a list of `votes` (`list[GovernanceL2Vote]`). Each vote record contains `signer_key_id` (`str`), `consensus_signature` (`str`, Ed25519 signature over `<transaction_hash>|<decision>`), and `decision` (`bool`).
- **`l3` (`GovernanceL3`)** — Human notary proof (`GovernanceL3Proof`), containing WebAuthn assertion data (`client_data_json`, `authenticator_data`, `signature`, `credential_id`) for web sessions or mTLS certificate proof (`mtls_cert_fingerprint`, `cli_signature`) for CLI sessions.

### Deterministic Transaction Hash

The transaction hash is computed using SHA-256 over canonicalized envelope fields in strict protocol field order:

```
action_type|target_resource|payload|state_merkle_root|nonce|expires_at|intent_data|requestor_user_id|acting_app_id|
```

1. **Field Omission** — Empty or `None` fields are omitted entirely from the input string (no placeholder value, no trailing separator).
2. **Field Termination** — A single pipe character (`|`) is appended after each present field.
3. **Timestamp Normalization** — `expires_at` is normalized to fixed 6-digit microsecond UTC format (`YYYY-MM-DDTHH:MM:SS.ffffffZ`).
4. **Intent Canonicalization** — `intent_data` is converted to sorted, comma-delimited `key=value` format, recursing into nested structures.
5. **L3 Exclusion** — The L3 proof is intentionally excluded from the hash so that L2 consensus members can sign before the human notary is challenged.

Cross-language test suites verify that the Python implementation (`compute_transaction_hash`) and Go implementation (`GenerateMessageID`) produce identical hashes across shared test vector suites.

## Dispatch Mechanisms

The ensemble interacts with the platform using two distinct dispatch patterns based on the mutation target.

### Operator Command Intent Dispatch

For host-level mutations (executing shell commands, editing files, inspecting filesystems), the ensemble constructs a `CommandIntent` model:

1. The ensemble packages the execution request into a protobuf payload and encodes the serialized bytes as base64 ASCII.
2. The ensemble publishes the `CommandIntent` to the operator's dedicated command channel (`cmd:<operator_id>:<operator_session_id>`).
3. The Gateway consumes the intent, validates the operator session binding, fetches the current state Merkle root, and constructs the canonical `GovernanceEnvelope`.
4. The Gateway routes the envelope through L1 Doctrine, L2 Consensus, and L3 Notary gates before publishing the governed transaction to the operator for execution.

### Direct Governance Envelope Submission

For platform-level state mutations (creating or modifying cases, investigations, memories, and reputation records), the ensemble's `GovernanceClient` constructs and submits the `GovernanceEnvelope` directly:

1. `GovernanceClient` fetches the current state Merkle root from the Gateway's `/api/v1/state` endpoint.
2. The client generates a cryptographic nonce, normalizes timestamps, and builds the `GovernanceEnvelope`.
3. The client computes the deterministic transaction hash via `compute_transaction_hash`.
4. The client attaches agent signatures and submits the envelope to `POST /api/v1/governance/envelopes` over mTLS.
5. The Gateway verifies the envelope against L1/L2/L3 policies and returns a signed `ActionReceipt`.

## Gateway Communication Surfaces

The Governance Gateway exposes two network surfaces with distinct security and protocol requirements.

### Plain HTTP Surface (Port 8080)

The plaintext HTTP surface handles initial bootstrap, certificate lifecycle onboarding, and redirects:

- **Trust Bundle Distribution** — `/.well-known/g8e/pki/ca-bundle` and `/pki/trust/g8eg-ca-bundle.pem` provide the platform CA bundle.
- **Platform Device Enrollment** — `/api/v1/pki/devices/enroll` and `/api/v1/auth/platform-enrollments/*` coordinate passkey-authenticated device binding.
- **CSR Signing** — `/api/v1/pki/csr/sign` accepts Certificate Signing Requests from operators and CLI clients during bootstrap.
- **Health Verification** — `/api/v1/health` provides unauthenticated liveness checks.
- **Deployment Script Bootstrap** — `/g8e-deploy.sh` and `/g8e-deploy.ps1` serve platform bootstrap scripts for Linux and Windows.

### mTLS HTTPS Surface (Port 8443)

The HTTPS surface requires mutual TLS (mTLS) with SPIFFE identity verification and serves all operational APIs:

- **Governance Submission** — `POST /api/v1/governance/envelopes` for direct envelope submission and verification.
- **Operator Session Management and Validation** — `/api/v1/operators/bind`, `/api/v1/operators/unbind`, and `/api/v1/operators/session/*` manage active bindings; `POST /api/v1/operators/validate` validates the exact active operator-session, CLI-session, and user tuple for enrolled applications over mTLS.
- **Document Store Operations** — `/api/v1/data/*` and `/api/v1/data/items` for querying and mutating platform documents.
- **Key-Value Cache Operations** — `/api/v1/kv/*` for fast ephemeral session state and coordination keys.
- **Blob Storage Operations** — `/api/v1/blobs/*` for encrypted artifact and attachment storage.
- **Pub/Sub Messaging** — `POST /api/v1/pubsub/publish`, `GET /api/v1/pubsub/stream`, and `/ws/pubsub` for pub/sub message transport.
- **Server-Sent Events (SSE)** — `POST /api/v1/sse/push` for publishing ensemble telemetry, `GET /api/v1/sse/stream` for real-time streaming, and `GET /api/v1/sse/events` for event polling.
- **MCP and A2A Endpoints** — `POST /mcp` for Model Context Protocol JSON-RPC requests and `POST /api/v1/a2a/call` for Agent-to-Agent skill invocations.
- **Authentication and Approvals** — `/api/v1/auth/passkeys/*` for WebAuthn challenges and `/api/v1/approvals/*` for out-of-band approval flows.
- **Audit Receipts** — `/api/v1/audit/receipts` and `/api/v1/audit/receipts/export` for querying signed execution receipts.

## Pub/Sub Channels

All inter-component event distribution uses structured channel names defined in `protocol/constants/channels.json`:

| Channel Pattern | Direction | Description |
| --- | --- | --- |
| `cmd:<operator_id>:<operator_session_id>` | Ensemble → Gateway → Operator | Inbound command intent and governed dispatch channel |
| `results:<operator_id>:<operator_session_id>` | Operator → Gateway → Ensemble | Execution results channel carrying stdout, stderr, and exit codes |
| `receipts:<operator_id>:<operator_session_id>` | Operator → Gateway → Ensemble | Signed action receipts channel (`ActionReceipt`) |
| `heartbeat:<operator_id>:<operator_session_id>` | Operator → Gateway | Periodic operator health and telemetry pulses |
| `governance` | Gateway ↔ Components | Platform-wide governance notifications and state updates |
| `sse_event` | Ensemble → Gateway → Clients | UI telemetry, reasoning traces, and turn lifecycle events |
| `storage_document` | Gateway → Components | Document creation, mutation, and deletion notifications |
| `storage_kv` | Gateway → Components | Key-value cache state change notifications |
| `storage_blob` | Gateway → Components | Blob storage upload and removal notifications |

## Workload Identity

The platform enforces zero-trust identity using SPIFFE IDs within the `spiffe://g8e.local/` trust domain:

- **Agentic Ensemble (`App`)** — `spiffe://g8e.local/app/g8ee` (`EnsembleAppID`) identifies the centralized ensemble service.
- **Governed Operator (`Operator`)** — `spiffe://g8e.local/operator/<org_id>/<operator_id>/<session_id>` identifies specific operator instances.
- **Command Line Interface (`CLI`)** — `spiffe://g8e.local/cli/<user_id>/<session_id>` identifies authenticated CLI sessions.
- **Human User (`User`)** — `spiffe://g8e.local/user/<user_id>` identifies human operator accounts.
- **Gateway Hub (`Hub`)** — `spiffe://g8e.local/hub/operator-listen` identifies central gateway listener endpoints.
- **Peer Gateway (`GatewayPeer`)** — `spiffe://g8e.local/gateway/<gateway_id>` identifies peer gateways in distributed deployments.

Identity validators on both Gateway and Operator verify SPIFFE URI SANs during the mTLS handshake before admitting requests.

## Five-Layer Governance Lifecycle

Every mutation passes through five sequential governance layers. Universal checks and proofs required by the active posture fail closed; optional L2 and L3 results remain audit evidence:

```
[Agent Intent] ──> L1 Doctrine ──> L2 Consensus ──> L3 Notary ──> L4 Warden ──> L5 Actuator ──> [Host Execution & Receipt]
```

1. **L1 Doctrine (Technical Bedrock)** — Deterministic hard gates scan payloads against forbidden pattern registries (`doctrine_registry.json`), blacklist rules, and MITRE ATT&CK heuristics before any consensus or human involvement.
2. **L2 Consensus** — Enrolled members evaluate the transaction and produce Ed25519 signatures over `<transaction_hash>|<decision>`. L2 quorum is required under consensus and notary postures and audited under doctrine and ratify.
3. **L3 Notary (Human Authorization)** — Hardware-bound human approval uses a WebAuthn passkey assertion or mTLS CLI signed proof. L3 is required for mutations under ratify and notary postures and audited under doctrine and consensus; read-only actions do not require L3.
4. **L4 Warden (Pre-Dispatch Gate)** — Operator-side verification re-computes the transaction hash, verifies nonce reservation, validates state Merkle root alignment, and checks posture-required L2/L3 signatures against trusted verification state.
5. **L5 Actuator (Sovereign Execution)** — The actuator signs and persists a complete pre-execution `ActionReceipt`, atomically appends a signed `CommitmentAttestation` against the current chain head, and records commitment linkage in deterministic stage evidence. It then rehydrates scrubbed sensitive tokens, mints a scoped Just-In-Time capability, executes the operation, dissolves the capability, signs and persists the final receipt, and attaches a signed `ReceiptPersistenceAttestation` proving durable association with its audit record.

## Action Types and Mappings

The protocol maps between internal protobuf event types and UAP action types:

| Protobuf Event Type | Action Type | Result Action Type |
| --- | --- | --- |
| `OPERATOR_COMMAND_REQUESTED` | `EXECUTE_BASH` | `EXECUTE_BASH_RESULT` |
| `OPERATOR_FILE_EDIT_REQUESTED` | `FILE_EDIT` | `FILE_EDIT_RESULT` |
| `OPERATOR_FILESYSTEM_LIST_REQUESTED` | `FS_LIST` | `FS_LIST_RESULT` |
| `OPERATOR_FILESYSTEM_READ_REQUESTED` | `FS_READ` | `FS_READ_RESULT` |
| `OPERATOR_FILESYSTEM_GREP_REQUESTED` | `FS_GREP` | `FS_GREP_RESULT` |
| `OPERATOR_LOGS_FETCH_REQUESTED` | `FETCH_LOGS` | `FETCH_LOGS_RESULT` |
| `OPERATOR_HISTORY_FETCH_REQUESTED` | `FETCH_HISTORY` | `FETCH_HISTORY_RESULT` |
| `OPERATOR_FILE_HISTORY_FETCH_REQUESTED` | `FETCH_FILE_HISTORY` | `FETCH_FILE_HISTORY_RESULT` |
| `OPERATOR_FILE_RESTORE_REQUESTED` | `RESTORE_FILE` | `RESTORE_FILE_RESULT` |
| `OPERATOR_NETWORK_PORT_CHECK_REQUESTED` | `PORT_CHECK` | `PORT_CHECK_RESULT` |
| `OPERATOR_HEARTBEAT_REQUESTED` | `HEARTBEAT` | `HEARTBEAT_RESULT` |
| `OPERATOR_SHUTDOWN_REQUESTED` | `SHUTDOWN` | `SHUTDOWN_RESULT` |
| `OPERATOR_MCP_CALL_REQUESTED` | `MCP_CALL` | `MCP_CALL_RESULT` |
| `OPERATOR_A2A_CALL_REQUESTED` | `A2A_CALL` | `A2A_CALL_RESULT` |
| `OPERATOR_INTENT_REQUESTED` | `GRANT_INTENT` | `GRANT_INTENT_RESULT` |
| `OPERATOR_INTENT_REVOKE_REQUESTED` | `REVOKE_INTENT` | `REVOKE_INTENT_RESULT` |
| `OPERATOR_EVAL_ANSWER_REQUESTED` | `EVAL_ANSWER` | `EVAL_ANSWER_RESULT` |
| `APP_CASE_CREATED` / `APP_CASE_UPDATED` | `DOCUMENT_UPDATE` | `DOCUMENT_UPDATE_RESULT` |
| `APP_CASE_DELETED` | `DOCUMENT_DELETE` | `DOCUMENT_DELETE_RESULT` |
| `APP_INVESTIGATION_CREATED` / `APP_INVESTIGATION_UPDATED` | `DOCUMENT_UPDATE` | `DOCUMENT_UPDATE_RESULT` |
| `APP_INVESTIGATION_DELETED` | `DOCUMENT_DELETE` | `DOCUMENT_DELETE_RESULT` |
| `APP_MEMORY_CREATED` / `APP_MEMORY_UPDATED` | `DOCUMENT_UPDATE` | `DOCUMENT_UPDATE_RESULT` |

## Related

- [Architecture](architecture.md) — System architecture, protocol surfaces, and model hierarchy
- [Governance](governance.md) — Five-layer verification pipeline and envelope validation
- [Agents](agents.md) — Agent hierarchy, personas, and Tribunal consensus
- [Constants](constants.md) — Protocol constants, channels, and action type mappings
- [PKI & Trust](pki.md) — Public Key Infrastructure, trust bundles, and workload identity
- [Storage](storage.md) — Storage tiers and data sovereignty
- [Thinking](thinking.md) — Consensus deliberation and cryptographic thought signatures
