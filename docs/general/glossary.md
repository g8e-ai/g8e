---
title: Glossary
---

# g8e Glossary

Last Updated: 2026-05-11
Version: v0.2.5

Essential terminology for understanding the g8e platform. Terms are organized alphabetically.

---

## Absolute Timeout

A security mechanism that automatically terminates web sessions after 24 hours regardless of activity. Ensures sessions cannot remain active indefinitely and requires users to re-authenticate periodically.

---

## Access Control List (ACL)

A set of rules that determines which users or systems can access specific resources. In g8e, ACLs are implemented through the Intent-Based Policy System for the Cloud Operator for AWS and user-level permissions for all Operators.

---

## Anchored Operator Terminal

The pinned terminal at the bottom of the chat interface in  for direct SSH-like command execution without AI involvement. Features command history (up/down arrows), collapsible view, real-time output via SSE, and displays AI-initiated events (approvals, execution results). Provides direct user input bypassing the AI while showing events routed from the AI execution flow.

---

## Approval UI

The interactive interface that presents proposed commands, file modifications, or permission requests to users for explicit consent. Shows the exact operation to be performed, its potential impact, and requires user confirmation before execution. Part of the Human-in-the-Loop security model.

---

## Audit Trail

A chronological record of all actions performed within the g8e platform. Includes command executions, file modifications, permission changes, and user interactions. Stored locally in the Audit Vault on the Operator.

---

## Audit Vault

An embedded SQLite database (`./.g8e/data/g8e.db`) that stores all session history, command executions, and file mutations locally on the Operator. Part of the Local-First Audit Architecture (LFAA). Contains tables for `sessions`, `events`, and `file_mutation_log`.

---

## Auditor

A verifier persona within the g8ee Tribunal stage (L2 Governance). The Auditor evaluates the consensus command winner against the user's original intent. If verified, the Auditor signs a **Merkle Commitment** that binds the current **Reputation Scoreboard** state to the verdict, creating a tamper-evident cryptographic chain of all agent actions. Performs the final consistency check before a command proceeds to user approval.

---

## Authentication Token

A cryptographic credential used to establish identity between components. Includes API keys for Operators, session tokens for web sessions, and device link tokens for deployment. All tokens have limited TTL and are revoked on suspicious activity.

---

## Auto-Approval

A security mechanism in g8ee that allows benign, read-only diagnostic commands (e.g., `uptime`, `df -h`, `ls`) to bypass manual human approval. Defined in `auto_approved.json`, these commands are still subjected to L1 and L2 governance. Auto-approval reduces click fatigue without sacrificing the technical or consensus safety gates.

---

## Binding

The process of connecting an Operator to a web session, enabling command execution on that system. Users manually bind Operators via the Operator Panel. Multiple Operators can be bound to a single web session simultaneously (Multi-Operator Binding), but each Operator can only be bound to one web session at a time.

---

## Bootstrap Process

The initial setup process for g8e, where the platform discovers the Internal Auth Token and configures the Operator's connection to the control plane.

---

## Case ID

A unique identifier for each investigation (chat session). Used to track conversation history, operator context, and state across sessions. Enables users to resume previous conversations and maintain continuity of operations.

---

## Cloud Operator

An Operator binary started with `--cloud` (which defaults to `true`). Unlocks cloud CLI tools (`aws`, `gcloud`, `az`, `terraform`, `kubectl`, `helm`, `ansible`, etc.) and switches the AI to cloud-specific reasoning mode. Use `--provider aws|gcp|azure` to specify the cloud. Cloud Operators run on any Linux system with port 443 outbound. See **Cloud Operator for AWS** for the variant with Zero Standing Privileges and intent-based IAM access.

---

## Cloud Operator for AWS

An Operator started with `--cloud --provider aws` that implements **Zero Standing Privileges** and **Just-in-Time Access**. The Operator launches with zero AWS permissions beyond self-discovery and dynamically requests permissions through user-approved Intents via the Intent-Based Policy System. Features a two-role IAM separation of execution from authority (Operator Role + Escalation Role), 1-hour TTL on granted permissions, and instant revocation capability.

Deployment targets:
- **g8ep** (local dev) — always started as Cloud Operator for AWS; credentials from `~/.aws` mount
- **EC2 in VPC** — credentials from EC2 instance profile (IMDS); two-role IAM setup via CloudFormation

---

## Command Approval

The process where users review and authorize proposed commands before execution. Includes command preview, impact assessment, and explicit consent mechanism. Required for all state-changing operations in the Human-in-the-Loop security model.

---

## Compliance Framework

The set of standards and regulations that g8e adheres to, including NSA ZIG alignment, Zero Trust Architecture principles, HIPAA readiness, and FedRAMP architecture. Includes audit trails, data sovereignty controls, and security documentation.

---

## Coordination Store (SQLite)

The embedded SQLite database used for durable storage of users, operators, investigations, chat history, and platform data. The Operator binary running in `--listen` mode is the single source of truth — a single SQLite database in WAL mode shared by all components via the Operator's document store, KV, and pub/sub APIs. g8ee and  are stateless with respect to persistence and access all data through the  HTTP API.

---

## Cryptographic Integrity

The use of cryptographic hashes and signatures to ensure data authenticity and prevent tampering. Applied to file mutations in the Ledger, audit logs in the Audit Vault, and communication between components.

---

## Data Sovereignty

The principle that sensitive data remains within the user's jurisdiction and control. In g8e, command outputs and file contents are stored locally on Operators, with only metadata transmitted to the self-hosted g8e platform for routing purposes.

---

## Defense-in-Depth

A security strategy that implements multiple layers of protection to ensure the security of the system. g8e applies this through authentication layers, network isolation, data filtering, human oversight, and audit controls.

---

## Device Link

A pre-authorized deployment method for installing Operators on one or many systems from a single token. Users generate a Device Link from the Operator Panel with configurable `max_uses` (1–10,000, default 1) and expiry (1 minute to 7 days, default 1 hour). The token (`dlk_` prefix) is distributed via Ansible, SSH, or configuration management as `g8e.operator --device-token dlk_xxx`. Each system auto-registers: the platform claims an existing AVAILABLE Operator slot for that user, or creates one on demand if none exist. No browser approval required — the link itself is the authorization. Operator slots are the accounting unit — each registered device consumes one slot.

**Authority Split:**  is authoritative for device link documents (usage tracking, exhaustion checking, claims management); g8ee is authoritative for operator documents (slot management, lifecycle operations).

---

## Encryption at Rest

The protection of data stored on disk using AES-256-GCM encryption. Applied to the Audit Vault, Scrubbed Vault, and Ledger databases on Operators. Ensures data remains confidential even if physical storage is compromised.

---

## Environment

The runtime context of the system where an Operator is running, as reported by the Operator via heartbeat telemetry. Captured in `HeartbeatEnvironment` and includes: current working directory (`pwd`), locale (`lang`), timezone, terminal type (`term`), container detection (`is_container`, `container_runtime`, `container_signals`), and init system (`init_system`). Used by g8ee to provide the AI with accurate context about the Operator's execution environment.

---

## Escalation Role

An AWS IAM role used in Cloud Operator for AWS deployments that can only attach or detach pre-defined intent policies to the Operator Role. Cannot perform other AWS actions, ensuring controlled permission escalation.

---

## FedRAMP Architecture

The security architecture aligned with Federal Risk and Authorization Management Program requirements. Includes documentation, controls, and monitoring suitable for government deployments and federal agencies.

---

## g8e

The platform name. g8e is an open-source, air-gapped capable AI governance platform that connects Operators to an AI control plane capable of reasoning about system state, executing commands, analyzing results, and performing multi-step operational workflows through natural language.

---

## 

The Node.js/Express web frontend component. Handles user authentication (passkey/FIDO2/WebAuthn), session management, the chat interface, Operator Panel, and SSE streaming to browsers. Routes messages between users and g8ee.

---

## g8ee (g8e Engine)

The AI engine component with LLM provider abstraction supporting OpenAI, Anthropic, Gemini, and Ollama providers. Processes natural language requests, reasons about system state, generates commands, and manages investigations. Implements the tool calling loop for Operator interactions and the Intent-Based Policy System for Cloud Operators.

---

## g8eo (g8e Operator)

The Go-based reference implementation of the Operator. A lightweight (~4MB) binary that provides language-agnostic, platform-agnostic execution, file operations, local storage, and heartbeat monitoring. When started with `--listen`, it acts as the platform's central **Coordination Store**.

---

## g8e Security

The comprehensive security model designed for organizations requiring regulatory compliance, audit trails, and granular access controls. Includes Zero Standing Privileges, data sovereignty, human oversight, and compliance documentation.

---

## g8e Sentinel

A platform-wide, multi-layer security system that protects the user's remote system and data across both the Operator (`g8eo`) and the AI Engine (`g8ee`). Sentinel stands guard on the Operator side with **pre-execution protection** (threat detectors across MITRE ATT&CK-mapped categories) and **egress data scrubbing** to ensure sensitive information never leaves the host. It adds another layer of defense on the AI Engine side with **ingress data scrubbing**, protecting Operator data with redundant patterns before it is transmitted to any model provider. Scrubbing patterns cover credentials (AWS keys, API tokens), PII (emails, phone numbers), network identifiers, and cloud resources, replacing sensitive values with safe placeholders like `[AWS_KEY]` or `[EMAIL]`. Sentinel also includes indirect prompt injection defense to detect command output attempting to manipulate AI behavior.

---

## Heartbeat

A periodic health telemetry message sent by the Operator to the control plane every 30 seconds. Contains system identity (hostname, OS, architecture), performance metrics (CPU, memory, disk usage), network information, and uptime data. Used for Operator health monitoring and status determination.

---

## HIPAA Ready

The compliance state where g8e architecture supports Healthcare Insurance Portability and Accountability Act requirements. Includes data sovereignty, audit trails, access controls, and security documentation for healthcare environments.

---

## Human Oversight

The requirement that all significant operations be reviewed and approved by authorized personnel. Implemented through the Approval UI, command approval workflow, and permission escalation controls.

---

## Human-in-the-Loop

A security principle requiring explicit user approval before any state-changing operation executes. All destructive commands, file modifications, and permission escalations require user consent via the approval UI. Read-only operations execute automatically.

---

## IAM Role

AWS Identity and Access Management role used by Cloud Operators for AWS to define permissions. Includes the Operator Role (starts with zero permissions) and Escalation Role (manages intent policy attachments).

---

## Identity Verification

The process of confirming the authenticity of users, Operators, and systems. Includes passkey (FIDO2/WebAuthn) authentication for users, API keys for Operators, and system fingerprinting for device binding.

---

## Idle Timeout

A security mechanism that automatically terminates web sessions after 8 hours of inactivity. Prevents unauthorized access through abandoned browser sessions and requires users to re-authenticate when returning.

---

## Information Isolation Principle

A load-bearing safety property in g8e's mechanism design where AI agents operate in a sealed, tiered information environment. Each agent (Triage, Sage, Tribunal members, Auditor) has a quarantined view of the pipeline to prevent collusion and ensure honest participation.

---

## Intent

A pre-defined AWS permission set that Cloud Operators for AWS can request via the Intent-Based Policy System. Examples include `ec2_discovery`, `s3_read`, and `rds_management`. Intents are organized into discovery (read-only) and management (read-write) categories.

---

## Intent Policy

A pre-defined AWS IAM policy that grants specific permissions for a particular service or action. Cloud Operators for AWS request these policies through the Intent-Based Policy System when additional permissions are needed.

---

## Intent-Based Policy System

The security framework that governs Cloud Operator for AWS permissions through pre-defined Intent sets. When a Cloud Operator for AWS lacks required permissions for an operation, it requests the appropriate Intent from the user, who can approve or deny the permission escalation.

---

## Interrogation Gate

A safety mechanism in g8ee that detects `<interrogation>` blocks in LLM responses. When a reasoning agent (Sage or Dash) determines it lacks sufficient information to proceed safely, it emits an interrogation block containing clarifying questions. The Interrogation Gate suppresses all tool/command execution for that turn and defers to the user for answers, ensuring the AI never "guesses" when intent is ambiguous.

---

## Investigation

A chat session or conversation between a user and the AI control plane. Investigations contain message history, operator context, and state. Each investigation has a unique `case_id` and can be resumed across sessions.

---

## Investigation Management

The process of creating, tracking, and resuming chat sessions (investigations) with proper context preservation, operator binding, and state management across browser sessions.

---

## Just-in-Time Access

A security model exclusive to the Cloud Operator for AWS where permissions are granted only when needed for a specific operation, only after explicit user approval, and with automatic expiration. Upon approval, the permission is attached with a **default 1-hour TTL** and automatically revoked when it expires.

---

## L1 Technical Bedrock

The first and foundation layer of g8e governance. It implements hard-coded technical gates: **Forbidden Patterns** (blocking commands like `sudo` or `su`), a **Blacklist** (denylist of specific binaries), and a **Whitelist** (allowlist of permitted operations). L1 is foundationally active for every command, regardless of L2 consensus or L3 approval status.

---

## L2 Consensus (Tribunal)

The second layer of g8e governance. A heterogeneous multi-model ensemble of 5 independent agents (Axiom, Concord, Variance, Pragma, Nemesis) that produces and votes on command candidates. Includes the **Warden** for risk analysis and the **Auditor** for final verification. L2 ensures that every command executed is the result of a rigorous consensus process rather than a single model's output.

---

## L3 Authorization (Approval)

The third layer of g8e governance, focusing on human oversight. By default, every state-changing command requires explicit user authorization via the **Approval UI**. Benign diagnostic commands may be covered by **Auto-Approval**, but L3 never bypasses the safety requirements of L1 or L2.

---

## Least Privilege

A security principle where entities receive only the minimum permissions necessary to perform their functions. g8e implements this through Zero Standing Privileges for the Cloud Operator for AWS and user-level permissions for all Operators.

---

## Ledger

The file-mutation audit layer of the LFAA. The Operator implements a **Multi-Ledger Architecture**: each operator session receives its own isolated git repository initialized on first use at `.g8e/data/ledger/sessions/<operator_session_id>/`. A global ledger at `.g8e/data/ledger/` acts as the bootstrap root, but all runtime file-mutation history is written into the session-scoped sub-repository.

Every file mutation follows a two-phase commit: the Ledger snapshots the file's state before the mutation (`LedgerHashBefore`), the Operator executes, then the Ledger snapshots the result (`LedgerHashAfter`). Each phase produces a git commit with a timestamped message referencing the operator session ID. The resulting git hash pair provides a cryptographically verifiable diff, enabling time-travel, rollback, and cross-session forensic comparison.

Session ledgers are created lazily and protected by a double-checked lock so concurrent sessions never interfere. The Ledger is disabled gracefully when git is unavailable (`--no-git`); the Audit Vault continues operating.

See also: **Audit Vault**, **Local-First Audit Architecture (LFAA)**, **Time-Travel**.

---

## Local Storage

The practice of storing sensitive data on the local Operator rather than in the g8e platform. Applied to command outputs, file contents, and audit logs through the Audit Vault and Scrubbed Vault.

---

## Local-First Audit Architecture (LFAA)

An architecture where the Operator is the System of Record for all chat history, execution logs, and file mutations. The self-hosted g8e platform acts as a stateless relay with no sensitive operational data persisting in platform storage. Core philosophy: "The Platform handles routing. The Operator handles retention."

---

## Malware Detection

The capability of g8e Sentinel to identify malicious code, viruses, and security threats in command outputs and file modifications. Uses pattern matching against MITRE ATT&CK framework indicators.

---

## Man-in-the-Middle Prevention

Security measures that prevent attackers from intercepting or modifying communication between components. Implemented through certificate pinning, mTLS, and encrypted channels.

---

## Merkle Commitment

A cryptographic artifact produced by the **Auditor** during the Tribunal's verification step. It is a SHA-256 Merkle root computed over the sorted (agent_id, scalar) leaves of the **Reputation Scoreboard**. Each commitment includes the `prev_root` of the previous commitment, forming a tamper-evident hash chain.

---

## Message Triage

The classification of incoming user messages as "simple" or "complex" using a lightweight lite model. Triage acts as a gatekeeper, emitting structured metadata:
- **Complexity**: Decides whether to route to Dash (Assistant) or Sage (Primary).
- **Intent**: Classifies the category of user intent (Action, Question, Unknown).
- **Posture**: Assesses the user's state (Normal, Frustrated, etc.) to calibrate downstream agent behavior.

---

## Metadata Transmission

The practice of sending only non-sensitive metadata (hashes, sizes, timestamps) to the g8e platform while keeping actual data content local on the Operator. Under the Local-First Audit Architecture (LFAA), the AI retrieves command output on-demand via `fetch_execution_output` rather than receiving it directly.

---

## MITRE ATT&CK Framework

A globally-accessible knowledge base of adversary tactics and techniques based on real-world observations. g8e Sentinel maps threat detection patterns to this framework to provide standardized security monitoring and incident response capabilities.

---

## Multi-Operator Binding

The ability to bind multiple Operators to a single web session simultaneously. Enables coordinated operations across multiple systems while maintaining individual Operator accountability.

---

## Mutual TLS (mTLS)

Two-way TLS authentication where both client and server verify each other's certificates. Used between Operators and g8e Cloud to ensure binary authenticity and prevent forged connections.

---

## Network Isolation

The security practice of separating network segments to prevent lateral movement. g8e achieves this through outbound-only connectivity, eliminating the need for inbound ports or network exposure.

---

## NSA ZIG Alignment

Compliance with the National Security Agency's Zero Trust Implementation Guidelines (ZIG). g8e **exceeds requirements in 6 of 7 pillars** (User, Device, Application, Data, Automation, Visibility) with the Network & Environment pillar fully compliant.

---

## Ollama (Remote)

The remote LLM inference component. g8e supports any remote Ollama server reachable via its native `/api/chat` surface. Used as an LLM backend for g8ee.

---

## Operator

The language-agnostic, platform-agnostic execution binary that runs on target systems and receives commands from the g8e control plane. The current ~4MB Go binary (`g8eo`) is the reference implementation for Linux and macOS. When running in `--listen` mode, the Operator serves as the platform's **Coordination Store**, providing the document store, KV, and pub/sub broker for  and g8ee.

Operator command/result traffic follows the g8e protocol: UAP JSON `GovernanceEnvelope` bytes carry typed `operator.proto` payloads and L1/L2/L3 governance metadata over the pub/sub transport.

---

## Operator Panel

The web UI component that displays all Operators belonging to a user, showing their status, hostname, and metrics. Users bind/unbind Operators to their web session through this panel.

---

## Operator Role

The AWS IAM role used by Cloud Operators for AWS for executing actions. Starts with zero permissions and gains capabilities only through approved intent policy attachments.

---

## Operator Slot

The accounting unit for Operators. Each running Operator occupies one slot. Slots are created explicitly at provisioning time or on demand when a Device Link is claimed and no available slot exists. Cloud providers like `g8ep` use `cloud_subtype='g8ep'` for identification.

---

## Permission Boundary

An AWS IAM policy that defines the maximum permissions that can be granted to a role. g8e uses this to prevent admin-level access and enforce least-privilege principles.

---

## Permission Escalation

The process by which Cloud Operators for AWS request additional AWS permissions beyond their current scope. Requires explicit user approval through the Intent-Based Policy System.

---

## PII Scrubbing

The removal of Personally Identifiable Information from data before platform transmission. g8e Sentinel replaces names, emails, phone numbers, and other PII with safe placeholders.

---

## Pinned Certificate

A server certificate embedded in the Operator binary at compile time. Prevents man-in-the-middle attacks by ensuring Operators only connect to legitimate g8e servers.

---

## ReAct

A reasoning pattern used by g8ee where the LLM cycles through Think → Act → Observe → Repeat. The AI generates a thought, executes an action, observes the result, and uses that observation to inform the next reasoning step. The Tribunal refines command syntax within this loop without re-invoking the main LLM.

---

## Real-Time Monitoring

Continuous observation of system health, performance metrics, and security events. Implemented through 30-second heartbeats, real-time streaming, and instant alerting.

---

## Regulatory Compliance

Adherence to industry-specific regulations and standards. g8e supports frameworks including HIPAA for healthcare, FedRAMP for government, and GDPR for data protection.

---

## Replay Protection

Security mechanisms that prevent captured requests from being replayed by attackers. Implemented through timestamp validation, nonce tracking, and request authentication.

---

## Reputation Staking

The mechanism by which g8ee Tribunal agents earn or lose standing based on the quality of their contributions. Each agent is assigned a reputation scalar (0.0 to 1.0) on the **Reputation Scoreboard**. Scalars are updated via an Exponential Moving Average (EMA) based on consensus participation and the eventual success or failure of the commands they proposed. Agents can be "slashed" for proposing high-risk or failing commands.

---

## Scrubbed Vault

The local SQLite database (`./.g8e/local_state.db`) that stores Sentinel-processed command output and file diffs. AI reads from this vault. Sensitive data is replaced with safe placeholders like `[IP_ADDR]`, `[AWS_KEY]`, etc.

---

## Self-Discovery Permissions

The minimal bootstrap permissions granted to Cloud Operators at launch, allowing them to identify their own AWS role and capabilities without accessing user resources.

---

## SSE (Server-Sent Events)

The streaming protocol used to push real-time events from the backend to the browser. Used for AI response streaming, command execution results, heartbeat updates, and approval requests.

---

## System Fingerprint

A unique identifier generated by each Operator based on system characteristics including hostname, OS, architecture, and network configuration. Used for Operator identification and duplicate detection.

---

## System Operator

The Operator binary started with `--cloud=false`. Standard shell and system operations only — cloud CLI tools are blocked at the execution layer.

---

## Time-Travel

The ability to restore files to any previous state using the Ledger's git history. Users can rollback changes, view historical versions, and recover from unintended modifications.

---

## Tool Calling Loop

The execution pattern used by g8ee where the AI generates tool calls to interact with Operators, receives results, and generates subsequent calls based on the outcomes.

---

## Tribunal

See **L2 Consensus (Tribunal)**.

---

## Trust Portal

A host-local bootstrap interface served by the Operator on Port 80 during initial setup. It provides a secure UI for generating the initial SSL certificates, Internal Auth Token, and other bootstrap secrets.

---

## Unified Approval

The batch execution approval dialog in  that allows a single user approval to cover commands across multiple Operators.

---

## Governance Envelope

The Protobuf root container for cross-component operator protocol messages. It binds a canonical `event_type` to typed payload bytes, operator/session context, optional state root data, and **L1/L2/L3 governance metadata**.

---

## Warden

A defensive coordination agent in g8ee that performs pre-execution risk assessment (LOW/MEDIUM/HIGH) for the consensus winner. It enforces the **Two-Strike Circuit Breaker**:
- **Strike 1**: If a command is classified as HIGH risk, the Warden blocks execution and provides contextual feedback to the reasoning agent to suggest a safer alternative.
- **Strike 2**: If a second HIGH risk command is proposed in the same turn, the Warden triggers an `AGENT_CONFLICT` error, halting the pipeline and requiring human intervention.
Successful command execution resets the strike counter.

---

## WebSession

An authenticated browser session. The **Coordination Store** is the authoritative store; the Operator KV acts as a fast read cache (cache-aside pattern). Contains user identity and session metadata. Sessions use encrypted cookies with idle and absolute timeouts.

---

## Zero Standing Privileges

A security model exclusive to the Cloud Operator for AWS where the Operator launches with zero AWS access beyond bootstrap permissions. Implements a **two-policy separation of execution from authority**.

---

## Zero-Trust AI Architecture

The platform's core security model. g8e assumes no implicit trust in either the AI or the systems it manages. All actions require explicit authorization, all data is filtered through Sentinel, and all execution happens with human oversight.
