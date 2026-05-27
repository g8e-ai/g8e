---
title: Glossary
---

# g8e Glossary

Last Updated: 2026-05-25
Version: v1.0.0

Core terminology for the g8e protocol, Governance Gateway, Governed Operator, and ecosystem integration (MCP, A2A). Terms are organized alphabetically.

---

## A2A (Agent2Agent)

A Google protocol for agent-to-agent communication and orchestration. g8e supports A2A as a payload type within the GovernanceEnvelope, enabling standard AI agents to interact with the Governed Operator through the g8e protocol translation layer.

---

## Actuator

The L5 execution boundary in the Governed Operator that performs actual command execution after L4 Warden verification. The Actuator enforces the **Two-Strike Circuit Breaker** for risk classification:
- **Strike 1**: If a command is classified as HIGH risk, the Actuator blocks execution and provides contextual feedback.
- **Strike 2**: If a second HIGH risk command is proposed in the same turn, the Actuator halts the pipeline requiring human intervention.
Successful command execution resets the strike counter.

---

## Audit Vault

An embedded SQLite database on the Governed Operator that stores all operator session history, command executions, and file mutations locally. Part of the Local-First Audit Architecture. Contains tables for `sessions`, `events`, and `file_mutation_log`. The Audit Vault is fail-closed: it rejects events with missing or malformed `operator_session_id` and unknown sessions.

---

## Coordination Store

The embedded SQLite database used by the Governance Gateway for durable storage of users, operators, and platform data. The Gateway running in Gateway mode is the single source of truth - a single SQLite database in WAL mode shared by all components via the Gateway's document store, KV, and pub/sub APIs. BYO agentic clients and other components are stateless with respect to persistence and access all data through the Gateway's HTTP API.

---

## Governance Envelope

The canonical Protobuf container (`g8e.common.v1.GovernanceEnvelope`) for all g8e protocol mutations. It binds identity, intent, state, and governance proofs into one transaction. Fields include:
- Identity: `id`, `timestamp`, `expires_at`, `source_component`, `operator_id`, `operator_session_id`, `web_session_id`, `cli_session_id`
- Intent: `event_type`, `payload` (typed protobuf bytes), `intent_data` (structured JSON), `action_type`, `target_resource`
- State: `state_merkle_root`, `nonce`, `transaction_hash`, `protocol_version`
- Governance: `governance` (L1/L2/L3 metadata)
- Context: `case_id`, `investigation_id`, `task_id`, `system_fingerprint`

The wire format is canonical JSON (protojson) for client-facing surfaces; signing is based on the deterministic `transaction_hash` computed from normalized envelope fields.

---

## Governance Gateway (g8eg)

The central, BFT-governed Policy Decision Point (PDP) running in Gateway mode (--doctrine, --consensus, or --notary). Built as the `g8e` binary from the Go codebase. It acts as the platform's cryptographic backplane, providing central persistence (SQLite Coordination Store), PKI/CA certificate issuance, a secure pub/sub broker, Server-Sent Events (SSE) buffering, replay protection, and the authoritative audit event vault.

---

## Governed Operator (g8eo)

The host-resident execution agent and Policy Execution Point (PEP). Built as the `g8e` binary from the Go codebase. Running on target hosts, `g8eo` connects outbound-only over mTLS to `g8eg`. It exposes host tools as a Model Context Protocol (MCP) Server, verifies incoming GovernanceEnvelope transactions against local L1/L2/L3 gates via the L4 Warden, executes actions strictly through the L5 Actuator boundary, and records tamper-evident local audit logs (Audit Vault, Scrubbed Vault, and git-backed session ledgers).

---

## g8e Protocol

The core substrate protocol defining how mutations flow through the g8e governance system. A typed, signed, state-bound transaction reaches a sovereign host agent that distrusts upstream inputs and refuses to mutate reality unless every independent proof checks out. The protocol uses canonical JSON (protojson) GovernanceEnvelope on the wire, with L1/L2/L3 governance metadata, state binding, replay protection, and cryptographic signing.

---

## Heartbeat

A periodic health telemetry message sent by the Governed Operator to the Gateway. Contains system identity (hostname, OS, architecture), performance metrics (CPU, memory, disk usage), network information, uptime data, and environment details (pwd, lang, timezone, term, container detection, init system). Used for Operator health monitoring and status determination.

---

## L1 Technical Bedrock (Doctrine)

The first and foundational layer of g8e governance. It implements hard-coded technical gates enforced via protobuf field options:
- **Forbidden Patterns**: Regex patterns on string fields (e.g., blocking `sudo`, `su`, `rm -rf /`)
- **MITRE-based threat detection**: Command and MCP argument analysis against ATT&CK indicators
L1 is foundationally active for every command, regardless of L2 consensus or L3 approval status. Implemented in `L1Doctrine` within the L4 Warden verification flow.

---

## L2 Consensus

The second layer of g8e governance. A multi-agent consensus system that produces and votes on command candidates. L2 ensures every command executed is the result of a rigorous consensus process backed by a single L2 Ed25519 signature over the transaction hash, rather than a single model's output. The signature is verified in the L4 Warden. Consensus implementations are application-layer concerns, not protocol requirements.

---

## L3 Authorization (Notary)

The third layer of g8e governance, focusing on human oversight. By default, every state-changing command requires explicit user authorization via WebAuthn (passkey) proof. Benign diagnostic commands may be covered by auto-approval policies, but L3 never bypasses the safety requirements of L1 or L2. The L3 proof is verified in the L4 Warden.

---

## L4 Warden

The fail-closed transaction verification gate in the Governed Operator that enforces L1/L2/L3 governance before any execution. The Warden performs:
- Envelope integrity and decoding validation
- Typed payload validation and action type matching
- L1 forbidden pattern validation via L1Doctrine
- Transaction hash verification
- Freshness (expiry) and replay protection (nonce)
- State root matching
- L2 signature verification (when required by posture)
- L3 WebAuthn proof verification (when required by posture)
The Warden rejects transactions that fail any check; only verified transactions proceed to the L5 Actuator for execution.

---

## L5 Actuator

The execution boundary in the Governed Operator that performs actual command execution after L4 Warden verification. The Actuator enforces risk classification and the Two-Strike Circuit Breaker. It is the only component that mutates host state.

---

## Ledger

The file-mutation audit layer of the Local-First Audit Architecture. The Governed Operator implements a Multi-Ledger Architecture: each operator session receives its own isolated git repository at `.g8e/data/ledger/sessions/<operator_session_id>/`. Every file mutation follows a two-phase commit: the Ledger snapshots the file's state before mutation (`LedgerHashBefore`), the Operator executes, then the Ledger snapshots the result (`LedgerHashAfter`). Each phase produces a git commit with a timestamped message referencing the operator session ID. The resulting git hash pair provides a cryptographically verifiable diff, enabling time-travel, rollback, and cross-session forensic comparison.

---

## Local-First Audit Architecture

An architecture where the Governed Operator is the System of Record for all execution logs and file mutations. The Governance Gateway acts as a stateless relay with no sensitive operational data persisting in Gateway storage. Core philosophy: "The Gateway handles routing. The Operator handles retention."

---

## MCP (Model Context Protocol)

An open protocol for standardizing AI model interactions with tools and data sources. The Governed Operator exposes host tools as an MCP Server, enabling standard AI clients (OpenAI, Anthropic, LangChain, etc.) to execute commands through the g8e governance envelope. MCP tool calls are translated into GovernanceEnvelope transactions with typed `operator.proto` payloads.

---

## Merkle Commitment

A cryptographic artifact produced during L2 Consensus verification. It is a SHA-256 Merkle root computed over the sorted (agent_id, scalar) leaves of the Reputation Scoreboard. Each commitment includes the `prev_root` of the previous commitment, forming a tamper-evident hash chain of agent performance.

---

## Mutual TLS (mTLS)

Two-way TLS authentication where both client and server verify each other's certificates. Used between Governed Operators and the Governance Gateway to ensure binary authenticity and prevent forged connections. The Gateway operates as a Certificate Authority (CA) issuing operator certificates.

---

## Operator

The compiled Go codebase which generates the `g8e` binary. The term also refers to the Governed Operator (g8eo) role when running on target hosts as the PEP. Operator command/result traffic follows the g8e protocol: canonical JSON GovernanceEnvelope carries typed `operator.proto` payloads and L1/L2/L3 governance metadata over the pub/sub transport.

---

## Operator Session

A unique execution context for a running Governed Operator instance. Identified by `operator_session_id`. Each session has its own isolated git-backed ledger for file mutation tracking. The Audit Vault is keyed by operator session ID and enforces session validation before recording events.

---

## PKI (Public Key Infrastructure)

The cryptographic infrastructure managed by the Governance Gateway for issuing and revoking operator certificates. The Gateway acts as a Certificate Authority (CA), issuing certificates with workload identity (URI SAN) for operators. Certificates are revoked via the Gateway's revocation service, and operators fetch the revocation bundle to enforce revocation. TLS 1.3-only is enforced.

---

## Replay Protection

Security mechanisms that prevent captured requests from being replayed by attackers. Implemented through nonce tracking in the Governance Gateway's replay store, timestamp validation (expiry), and transaction hash verification. The L4 Warden checks nonce uniqueness before accepting any transaction.

---

## Reputation Staking

The mechanism by which L2 Consensus agents earn or lose standing based on the quality of their contributions. Each agent is assigned a reputation scalar (0.0 to 1.0) on the Reputation Scoreboard. Scalars are updated via an Exponential Moving Average (EMA) based on consensus participation and the eventual success or failure of the commands they proposed. Agents can be "slashed" for proposing high-risk or failing commands.

---

## Scrubbed Vault

The local SQLite database on the Governed Operator that stores Sentinel-processed command output and file diffs. Sensitive data (credentials, PII, network identifiers) is replaced with safe placeholders like `[IP_ADDR]`, `[AWS_KEY]`, `[EMAIL]`. AI clients read from this vault rather than raw output.

---

## Sentinel

The data sovereignty and threat detection system running within the Governed Operator. Sentinel provides pre-execution protection (MITRE ATT&CK-mapped threat detectors) and egress data scrubbing to ensure sensitive information never leaves the host. Scrubbing patterns cover credentials, PII, network identifiers, and cloud resources. Sentinel also includes indirect prompt injection defense to detect command output attempting to manipulate AI behavior.

---

## SSE (Server-Sent Events)

The streaming protocol used to push real-time events from the Governance Gateway to clients. Used for command execution results, heartbeat updates, and approval requests in BYO agentic client integrations.

---

## State Root

A Merkle root representing the current state of the Governed Operator's data stores (Audit Vault, Ledger, etc.). The GovernanceEnvelope includes `state_merkle_root` for state binding. The L4 Warden verifies that the transaction's state root matches the current state root before accepting the transaction, ensuring the transaction is based on the correct state.

---

## System Fingerprint

A unique identifier generated by each Governed Operator based on system characteristics including hostname, OS, architecture, CPU count, and machine ID. Used for Operator identification and duplicate detection.

---

## Time-Travel

The ability to restore files to any previous state using the Ledger's git history. Users can rollback changes, view historical versions, and recover from unintended modifications by restoring files to specific git commits.

---

## Tool Calling Loop

The execution pattern where BYO AI clients generate tool calls to interact with the Governed Operator via MCP or A2A protocols, receive results, and generate subsequent calls based on the outcomes. The Governed Operator exposes host tools as an MCP Server, enabling standard AI clients to execute commands through the governance envelope.

---

## Transaction Hash

A deterministic SHA-256 hash computed from normalized GovernanceEnvelope fields. The hash is used for replay protection, state binding, and L2/L3 signature verification. The `transaction_hash` field in the envelope must match the computed hash; mismatch causes rejection by the L4 Warden.

---

## Workload Identity

The identity of a Governed Operator as encoded in its TLS certificate via URI SAN (Subject Alternative Name). The workload identity is used for authentication and authorization in the mTLS connection between the Operator and the Gateway.
