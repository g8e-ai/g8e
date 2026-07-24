---
title: Glossary
---

# g8e Glossary

Last Updated: 2026-07-24
Version: v1.6.2

Core terminology for the g8e protocol, Governance Gateway, Governed Operator, and ecosystem integration (MCP, A2A). Terms are organized alphabetically.

---

## A2A (Agent2Agent)

A protocol for agent-to-agent communication and orchestration. g8e supports A2A as a payload type within the GovernanceEnvelope, enabling standard AI agents to interact with the Governed Operator through the g8e protocol translation layer.

---

## Actuator (L5Actuator)

The **L5Actuator** is the execution boundary in the Governed Operator that performs actual command execution after L4 Warden verification. It is the only component permitted to mutate host state. The Actuator ensures that every execution is preceded by a signed intent to execute and followed by a signed **ActionReceipt**. It also handles local rehydration of tokens just before execution.

---

## Acting App ID

A field in the **GovernanceEnvelope** (`acting_app_id`) that identifies the application or tool acting on behalf of the user. This establishes a delegation chain for auditing and policy enforcement.

---

## Capability

A just-in-time, single-action, self-dissolving permission derived from a verified governance envelope. The L5 Actuator mints a capability before execution and dissolves it immediately after, enforcing zero standing privileges. The capability binds the action type, target resource, transaction hash, and expiry. Downstream handlers can extract and verify the capability from the request context.

---

## Coordination Store

The embedded SQLite database used by the Governance Gateway for durable storage of users, operators, and platform data. The Governance Gateway running in gateway mode is the single source of truth: a single SQLite database in WAL mode shared by all components via the Governance Gateway's document store, KV, and pub/sub APIs. BYO agentic clients and other components are stateless with respect to persistence and access all data through the Governance Gateway's HTTP API.

Collections include users, operators, organizations, operator_sessions, cli_sessions, web_sessions, bound_sessions, cases, investigations, tasks, memories, personas, app_policies, trusted_signers, tribunals, enrollment_tokens, revoked_certificates, reputation_state, reputation_commitments, stake_resolutions, agent_activity_metadata, account_locks, login_audit, auth_admin_audit, console_audit, operator_usage, passkey_challenges, settings, and others. The database includes a `state_tier` column to distinguish bound-state from observed-state entries.

---

## Encrypted KV Adapter

A bridge between the canonical gateway DB's KV store and the TokenStore interface expected by the ScrubbingService. Values are encrypted at rest via the vault. Entries are written as `state_tier='observed'` so they do not participate in the bound state root hash. This replaced the standalone TokenStoreService and its separate `token_store.db` file.

---

## Enrollment Token

A one-time, cryptographically random token used for secure passkey enrollment. The gateway generates a 32-byte token with a 5-minute TTL and persists it with the user's session identifiers. The CLI opens a browser with the token in the URL fragment, and the console SPA submits it to the public validation endpoint. The token is consumed on first use and cleaned up after expiration, preventing exposure of raw session identifiers in browser history or referrer headers.

---

## Execution Vault

An embedded SQLite database on the Governed Operator that stores command execution results and file diffs locally. Part of the Local-First Audit Architecture. The Execution Vault Service provides encrypted storage for execution logs and file mutation tracking. Data is encrypted at rest when configured with the encryption vault. The vault supports retention-based pruning and size limits.

---

## Governance Envelope

The canonical Protobuf container (`g8e.common.v1.GovernanceEnvelope`) for all g8e protocol mutations. It binds identity, intent, state, and governance proofs into one transaction. Fields include:
- Identity: `id`, `timestamp`, `expires_at`, `source_component`, `operator_id`, `operator_session_id`, `web_session_id`, `cli_session_id`, `tenant_id`, `binding_persona`, `requestor_user_id`, `acting_app_id`
- Intent: `event_type`, `payload` (typed protobuf bytes), `intent_data` (structured JSON), `action_type`, `target_resource`
- State: `state_merkle_root`, `nonce`, `transaction_hash`, `protocol_version`
- Governance: `governance` (L1/L2/L3 metadata)
- Context: `case_id`, `investigation_id`, `task_id`, `system_fingerprint`

The wire format is canonical JSON (protojson) for client-facing surfaces; signing is based on the deterministic `transaction_hash` computed from normalized envelope fields.

---

## Gateway Peer

A gateway instance in a federated deployment that can communicate with other gateways. Gateway peers have their own SPIFFE identity (`spiffe://g8e.local/gateway/<gateway_id>`) and PKI support for peer-to-peer mTLS connections. Gateway peer functionality is part of the federation architecture for multi-gateway deployments.

---

## Governance Gateway

The central coordinator that admits transactions, manages PKI, enforces the 5-layer governance pipeline, and brokers pub/sub channels to operators. The Gateway is an Operator with additional gateway services; its in-process Operator runs L1-L5 locally for operations on the gateway host. Serves as the Policy Decision Point (PDP). Formerly referred to as "g8e Gateway."

---

## Governance Posture

A configuration that determines which governance layers are required for transaction verification. The three postures are:
- **Doctrine**: Requires only L1 (Technical Bedrock) validation
- **Consensus**: Requires L1 and L2 (Consensus) validation
- **Notary**: Requires L1, L2, and L3 (Notary/Human) validation
The posture is set at startup and affects whether L2 signatures and L3 proofs are required. Posture requirements are enforced by the L4 Warden via the GovernancePosture interface.

---

## Governed Operator

A single static binary deployed on target hosts. It requires no installation, opens no inbound ports, and initiates an outbound-only mTLS tunnel to the Governance Gateway. The operator pulls work from a unique pub/sub channel, re-verifies every proof locally against its own state, and is the only component authorized to mutate the host. Each remote Governed Operator runs L1-L5 locally for operations on its own host. Serves as the Policy Execution Point (PEP) and MCP server. Formerly referred to as "g8e Operator."

---

## g8e Protocol

The canonical protocol definitions (protobuf schemas, constant registries, governance invariants).

---

## Heartbeat

A periodic health telemetry message sent by the Governed Operator to the Governance Gateway. Contains system identity (hostname, OS, architecture), performance metrics (CPU, memory, disk usage), network information, uptime data, and environment details (pwd, lang, timezone, term, container detection, init system). Used for Governed Operator health monitoring and status determination.

---

## L1 Doctrine (L1Doctrine)

The foundational layer of g8e governance (Technical Bedrock). It implements hard-coded technical gates enforced via protobuf field options (forbidden_patterns) and real-time heuristics:
- **Forbidden Patterns**: Regex patterns on protobuf fields (e.g., blocking `sudo`, `su`, `rm -rf /`)
- **Threat Detection**: Command and MCP argument analysis against MITRE ATT&CK indicators via the `AnalyzeCommand` and `AnalyzeMCPArguments` methods.
L1 is foundationally active for every command and cannot be bypassed.

---

## L2 Consensus (L2Consensus)

The second layer of g8e governance (Consensus). A multi-agent consensus system where independent agents vote on command candidates. L2 ensures every command executed is backed by a cryptographic quorum. The Tribunal service deliberates on each governance envelope and produces signed votes. In the g8e protocol, an L2 vote is an Ed25519 signature over the `transaction_hash|decision` in the `GovernanceEnvelope`. Votes are verified by the L4 Warden against the tribunal policy's quorum requirement. L2 requirements are posture-dependent via the GovernancePosture interface (doctrine, consensus, notary).

---

## L3 Notary (L3Notary)

The third layer of g8e governance (Authorization), focusing on human oversight. Every state-changing mutation requires explicit human authorization. The L3 notary uses a layered authorization model with three implementations:
- **Gateway Notary**: The unified notary for gateway mode. Layer 1 requires passkey (WebAuthn) authorization for all callers; proofs without a `credential_id` are rejected. Layer 2 performs CLI mTLS session verification (user match, session validity, certificate fingerprint match) when the proof includes an `mtls_cert_fingerprint` (CLI callers). Browser-only proofs skip Layer 2.
- **Outbound Notary**: The notary for outbound mode. Performs suspended transaction lookup and Ed25519 signature verification over the transaction hash. The signature is verified against the `ApprovalPublicKey` stored on the suspended transaction.
- **CLI Notary**: The notary for gateway CLI mode without a passkey verifier. Performs CLI session verification followed by suspended transaction and Ed25519 signature verification.
The CLI approval flow opens a browser to the console SPA for WebAuthn ceremony and subscribes to the gateway's SSE stream for the `approval.completed` event, then verifies the approval status via the mTLS endpoint (`/api/v1/approvals/status/{tx_hash}`). CLI credentials (mTLS) are required for L3 approval flows. The L4 Warden verifies L3 proofs before allowing execution to proceed. L3 requirements are posture-dependent via the GovernancePosture interface (doctrine, consensus, notary).

---

## L4 Warden (L4Warden)

The fail-closed transaction verification gate in the Governance Gateway that enforces L1/L2/L3 governance before any execution. The Warden performs:
- Envelope integrity and decoding validation
- Typed payload validation and action type matching
- L1 forbidden pattern validation via L1Doctrine
- Transaction hash verification (both `id` and `transaction_hash` must match the computed hash)
- Freshness (expiry) and replay protection (nonce)
- State root matching
- L2 signature verification (when required by posture)
- L3 Notary proof verification (when required by posture)
The Warden rejects transactions that fail any check; only verified transactions proceed to the L5 Actuator for execution.

---

## Ledger

The file-mutation audit layer of the Local-First Audit Architecture. The Governed Operator implements a Multi-Ledger Architecture: each Operator session receives its own isolated git repository at `.g8e/data/ledger/sessions/<operator_session_id>/`. Every file mutation follows a two-phase commit: the Ledger snapshots the file's state before mutation (`LedgerHashBefore`), the Governed Operator executes, then the Ledger snapshots the result (`LedgerHashAfter`). Each phase produces a git commit with a timestamped message referencing the Operator session ID. The resulting git hash pair provides a cryptographically verifiable diff, enabling time-travel, rollback, and cross-session forensic comparison.

---

## Local-First Audit Architecture

An architecture where the Governed Operator is the System of Record for all execution logs and file mutations. The Governance Gateway acts as a stateless relay with no sensitive operational data persisting in Gateway storage. Core philosophy: "The Governance Gateway handles routing. The Governed Operator handles retention."

---

## MCP (Model Context Protocol)

An open protocol for standardizing AI model interactions with tools and data sources. The Governed Operator exposes host tools as an MCP Server, enabling standard AI clients (OpenAI, Anthropic, LangChain, etc.) to execute commands through the g8e governance envelope. MCP tool calls are translated into GovernanceEnvelope transactions with typed `operator.proto` payloads.

---

## Merkle Commitment

A cryptographic artifact produced during L2 Consensus verification. It is a SHA-256 Merkle root computed over the sorted (agent_id, scalar) leaves of the Reputation Scoreboard. Each commitment includes the `prev_root` of the previous commitment, forming a tamper-evident hash chain of agent performance.

---

## Mutual TLS (mTLS)

Two-way TLS authentication where both client and server verify each other's certificates. Used between Governed Operators and the Governance Gateway to ensure g8e binary file authenticity and prevent forged connections. The Governance Gateway operates as a Certificate Authority (CA) issuing Operator certificates with SPIFFE URI SAN identity.

---

## g8e binary file

The pre-compiled g8e binary file that can be launched as a Governance Gateway or Governed Operator. The same artifact serves both roles, selected by command-line flags.

---

## Operator Session

A unique execution context for a running Governed Operator instance. Identified by `operator_session_id`. Each session has its own isolated git-backed ledger for file mutation tracking. The Execution Vault is keyed by Operator session ID and enforces session validation before recording events.

---

## PKI (Public Key Infrastructure)

The cryptographic infrastructure managed by the Governance Gateway for issuing and revoking Operator certificates. The Governance Gateway acts as a Certificate Authority (CA), issuing certificates with workload identity (URI SAN) for operators. Certificates are revoked via the Governance Gateway's revocation service, and operators fetch the revocation bundle to enforce revocation. TLS 1.3-only is enforced. The PKI also supports gateway peer certificates for federated deployments.

---

## Replay Protection

Security mechanisms that prevent captured requests from being replayed by attackers. Implemented through nonce tracking in the Governance Gateway's replay store, timestamp validation (expiry), and transaction hash verification. The L4 Warden checks nonce uniqueness before accepting any transaction.

---

## Reputation Staking

The mechanism by which L2 Consensus agents earn or lose standing based on the quality of their contributions. Each agent is assigned a reputation scalar (0.0 to 1.0) on the Reputation Scoreboard. Scalars are updated via an Exponential Moving Average (EMA) based on consensus participation and the eventual success or failure of the commands they proposed. Agents can be "slashed" for proposing high-risk or failing commands.

---

## Requestor User ID

A field in the **GovernanceEnvelope** (`requestor_user_id`) that identifies the human user who authorized the action. This is the ultimate authority in the delegation chain.

---

## Observed-State Root

A separate Merkle root commitment computed over observed-tier KV and blob entries in the canonical gateway DB. Observed-state rows (those with `state_tier='observed'`) are excluded from the bound state root to prevent churning in-flight envelopes. The observed-state root does not gate transaction admission but is chained into the audit ledger so observed evidence is tamper-evident without breaking the bound root. Computed by the StateRootService.

---

## Scrubbed Vault

The local SQLite database on the Governed Operator managed by the **Sovereign Execution Boundary**. It stores command outputs where sensitive data (credentials, PII, network identifiers) has been replaced with safe placeholders like `{{UEI_N}}`. This ensures that raw sensitive data never leaves the sovereign host.

---

## Sovereign Execution Boundary

The data sovereignty and scrubbing system running within the Governed Operator (PEP), implemented as the `ScrubbingService`. It provides:
- **Egress Scrubbing**: Removes sensitive data (PII, credentials) from command output before transmission to the cloud.
- **Local Rehydration**: Restores original tokens just before execution at the L5 Actuator, ensuring the host shell receives the actual required values while the cloud only sees placeholders.
- **Token Persistence**: Maintains consistent mapping of placeholders across sessions via the TokenStore interface, implemented by the EncryptedKVAdapter against the canonical gateway DB.

---

## SSE (Server-Sent Events)

The streaming protocol used to push real-time events from the Governance Gateway to clients. Used for command execution results, heartbeat updates, and approval completion notifications. All clients (CLI, browser, operator) use the unified `/api/v1/sse/stream` endpoint. Approval requests are returned inline in the MCP or A2A response with an `approval_url` field, not pushed via SSE. Approval completions are delivered as `approval.completed` events on the SSE stream.

---

## State Root

A Merkle root representing the current bound state of the Governance Gateway's canonical database (documents, kv_store, blobs). The GovernanceEnvelope includes `state_merkle_root` for state binding. The L4 Warden verifies that the transaction's state root matches the current state root before accepting the transaction, ensuring the transaction is based on the correct state. Only bound-state rows (those with `state_tier='bound'`) are included in the state root computation. Observed-state rows are hashed separately in the observed-state root. The bound root also incorporates the token keymap hash from the ScrubbingService to bind rehydration mappings into the state.

---

## System Fingerprint

A unique identifier generated by each Governed Operator based on system characteristics including hostname, OS, architecture, CPU count, and machine ID. Used for Governed Operator identification and duplicate detection.

---

## Time-Travel

The ability to restore files to any previous state using the Ledger's git history. Users can rollback changes, view historical versions, and recover from unintended modifications by restoring files to specific git commits.

---

## Tool Calling Loop

The execution pattern where BYO AI clients generate tool calls to interact with the Governed Operator via MCP or A2A protocols, receive results, and generate subsequent calls based on the outcomes. The Governed Operator exposes host tools as an MCP Server, enabling standard AI clients to execute commands through the governance envelope.

---

## Tribunal

A multi-member deliberation service that produces L2 consensus votes for governance envelopes. Each tribunal member is an enrolled principal with its own Ed25519 key. The tribunal policy defines the member set, quorum threshold, and whether distinct signatures are required. When the gateway operates under consensus or notary posture, the tribunal deliberates on each envelope and signs a vote indicating whether the action is safe. The gateway bootstraps the tribunal at startup from a policy stored in the coordination store.

---

## Transaction Hash

A deterministic SHA-256 hash computed from normalized GovernanceEnvelope fields. The hash is used for replay protection, state binding, and L2/L3 signature verification. The `transaction_hash` field in the envelope must match the computed hash; mismatch causes rejection by the L4 Warden.

---

## State Tier

A classification for database entries in the canonical gateway DB that determines whether they participate in the bound state root. Two tiers exist:
- **Bound State** (`state_tier='bound'`): Entries that affect transaction freshness and are included in the bound state root. In-flight envelopes depend on this root.
- **Observed State** (`state_tier='observed'`): Entries that are evidence or telemetry data, excluded from the bound state root to prevent churning. These are hashed separately in the observed-state root for audit ledger chaining.
The `state_tier` column was added to `kv_store` and `blobs` tables in v1.1.9.

---

## Workload Identity

The identity of a Governed Operator as encoded in its TLS certificate via URI SAN (Subject Alternative Name). The workload identity is used for authentication and authorization in the mTLS connection between the Governed Operator and the Governance Gateway. The SPIFFE trust domain is `g8e.local`. Supported identity formats include:
- Operator: `spiffe://g8e.local/operator/<organization_id>/<operator_id>/<operator_session_id>`
- CLI: `spiffe://g8e.local/cli/<user_id>/<cli_session_id>`
- App: `spiffe://g8e.local/app/<operator_id>`
- User: `spiffe://g8e.local/user/<user_id>`
- Hub: `spiffe://g8e.local/hub/operator-listen`
- Gateway Peer: `spiffe://g8e.local/gateway/<gateway_id>`
