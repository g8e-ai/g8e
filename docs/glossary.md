---
title: Glossary
---

# g8e Glossary

Essential terminology for understanding the g8e platform. Terms are organized alphabetically.

---

## Absolute Timeout

A security mechanism that automatically terminates web sessions after 24 hours regardless of activity. Ensures sessions cannot remain active indefinitely and requires users to re-authenticate periodically.

---

## Access Control List (ACL)

A set of rules that determines which users or systems can access specific resources. In g8e, ACLs are implemented through the Intent-Based Policy System for the Cloud Operator for AWS and user-level permissions for all Operators.

---

## Anchored Operator Terminal

The pinned terminal at the bottom of the chat interface in g8ed for direct SSH-like command execution without AI involvement. Features command history (up/down arrows), collapsible view, real-time output via SSE, and displays AI-initiated events (approvals, execution results). Provides direct user input bypassing the AI while showing events routed from the AI execution flow.

---

## Approval UI

The interactive interface that presents proposed commands, file modifications, or permission requests to users for explicit consent. Shows the exact operation to be performed, its potential impact, and requires user confirmation before execution. Part of the Human-in-the-Loop security model.

---

## Audit Vault

An embedded SQLite database (`./.g8e/data/g8e.db`) that stores all session history, command executions, and file mutations locally on the Operator. Part of the Local-First Audit Architecture (LFAA). Contains tables for `sessions`, `events`, and `file_mutation_log`.

---

## Audit Trail

A chronological record of all actions performed within the g8e platform. Includes command executions, file modifications, permission changes, and user interactions. Stored locally in the Audit Vault on the Operator.

---

## Authentication Token

A cryptographic credential used to establish identity between components. Includes API keys for Operators, session tokens for web sessions, and device link tokens for deployment. All tokens have limited TTL and are revoked on suspicious activity.

---

## Binding

The process of connecting an Operator to a web session, enabling command execution on that system. Users manually bind Operators via the Operator Panel. Multiple Operators can be bound to a single web session simultaneously (Multi-Operator Binding), but each Operator can only be bound to one web session at a time.

---

## Case ID

A unique identifier for each investigation (chat session). Used to track conversation history, operator context, and state across sessions. Enables users to resume previous conversations and maintain continuity of operations.

---

## System Operator

The Operator binary started with `--cloud=false`. Standard shell and system operations only — cloud CLI tools (`aws`, `gcloud`, `az`, `terraform`, `kubectl`, `helm`, `ansible`, etc.) are blocked at the execution layer. This is the security boundary intended for operators that should have no cloud access. Because `--cloud` defaults to `true`, `--cloud=false` must be passed explicitly to enforce System Operator mode.

---

## Cloud Operator

The same ~4MB Operator binary started with `--cloud` (which defaults to `true`). Unlocks cloud CLI tools (`aws`, `gcloud`, `az`, `terraform`, `kubectl`, `helm`, `ansible`, etc.) and switches the AI to cloud-specific reasoning mode. Use `--provider aws|gcp|azure` to specify the cloud. Cloud Operators run on any Linux system with port 443 outbound. See **Cloud Operator for AWS** for the variant with Zero Standing Privileges and intent-based IAM access.

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

## Cryptographic Integrity

The use of cryptographic hashes and signatures to ensure data authenticity and prevent tampering. Applied to file mutations in the Ledger, audit logs in the Audit Vault, and communication between components.

---

## Data Sovereignty

The principle that sensitive data remains within the user's jurisdiction and control. In g8e, command outputs and file contents are stored locally on Operators, with only metadata transmitted to the self-hosted g8e platform for routing purposes.

---

## Defense-in-Depth

A security strategy that implements multiple layers of protection to ensure the security of the system. g8e applies this through authentication layers, network isolation, data filtering, human oversight, and audit controls.

---

## g8e

The platform name. g8e is an open-source, air-gapped capable AI governance platform that connects Operators to an AI control plane capable of reasoning about system state, executing commands, analyzing results, and performing multi-step operational workflows through natural language.

---

## Device Link

A pre-authorized deployment method for installing Operators on one or many systems from a single token. Users generate a Device Link from the Operator Panel with configurable `max_uses` (1–10,000, default 1) and expiry (1 minute to 7 days, default 1 hour). The token (`dlk_` prefix) is distributed via Ansible, SSH, or configuration management as `g8e.operator --device-token dlk_xxx`. Each system auto-registers: the platform claims an existing AVAILABLE Operator slot for that user, or creates one on demand if none exist. No browser approval required — the link itself is the authorization. Operator slots are the accounting unit — each registered device consumes one slot.

---

## g8e Sentinel

A platform-wide, multi-layer security system that protects the user's remote system and data across both the Operator (`g8eo`) and the AI Engine (`g8ee`). Sentinel stands guard on the Operator side with **pre-execution protection** (46 threat detectors across MITRE ATT&CK-mapped categories) and **egress data scrubbing** to ensure sensitive information never leaves the host. It adds another layer of defense on the AI Engine side with **ingress data scrubbing**, protecting Operator data with redundant patterns before it is transmitted to any model provider. Scrubbing patterns cover credentials (AWS keys, API tokens), PII (emails, phone numbers), network identifiers, and cloud resources, replacing sensitive values with safe placeholders like `[AWS_KEY]` or `[EMAIL]`. Sentinel also includes indirect prompt injection defense to detect command output attempting to manipulate AI behavior. See [docs/architecture/storage.md](architecture/storage.md) for full details.

---

## Encryption at Rest

The protection of data stored on disk using AES-256-GCM encryption. Applied to the Audit Vault, Scrubbed Vault, and Ledger databases on Operators. Ensures data remains confidential even if physical storage is compromised.

---

## g8e Security

The comprehensive security model designed for organizations requiring regulatory compliance, audit trails, and granular access controls. Includes Zero Standing Privileges, data sovereignty, human oversight, and compliance documentation.

---

## Environment

The runtime context of the system where an Operator is running, as reported by the Operator via heartbeat telemetry. Captured in `HeartbeatEnvironment` and includes: current working directory (`pwd`), locale (`lang`), timezone, terminal type (`term`), container detection (`is_container`, `container_runtime`, `container_signals`), and init system (`init_system`). Used by g8ee to provide the AI with accurate context about the Operator's execution environment — for example, whether to avoid `systemctl` commands in a containerized environment. Distinct from any platform-level deployment concept; g8e is self-hosted and has a single deployment environment.

---

## Escalation Role

An AWS IAM role used in Cloud Operator for AWS deployments that can only attach or detach pre-defined intent policies to the Operator Role. Cannot perform other AWS actions, ensuring controlled permission escalation.

---

## FedRAMP Architecture

The security architecture aligned with Federal Risk and Authorization Management Program requirements. Includes documentation, controls, and monitoring suitable for government deployments and federal agencies.

---

## Tool Calling Loop

The execution pattern used by g8ee where the AI generates tool calls to interact with Operators, receives results, and generates subsequent calls based on the outcomes. Enables complex multi-step workflows and adaptive reasoning about system state.

---

## Coordination Store (SQLite)

The embedded SQLite database used for durable storage of users, operators, investigations, chat history, and platform data. g8es is the single source of truth — a single SQLite database in WAL mode shared by all components via g8es's document store, KV, and pub/sub APIs. g8ee and g8ed are stateless with respect to persistence and access all data through the g8ed HTTP API. Replaces the former Google Cloud Firestore dependency.

---

## HIPAA Ready

The compliance state where g8e architecture supports Healthcare Insurance Portability and Accountability Act requirements. Includes data sovereignty, audit trails, access controls, and security documentation for healthcare environments.

---

## Human Oversight

The requirement that all significant operations be reviewed and approved by authorized personnel. Implemented through the Approval UI, command approval workflow, and permission escalation controls.

---

## IAM Role

AWS Identity and Access Management role used by Cloud Operators for AWS to define permissions. Includes the Operator Role (starts with zero permissions) and Escalation Role (manages intent policy attachments).

---

## Idle Timeout

A security mechanism that automatically terminates web sessions after 8 hours of inactivity. Prevents unauthorized access through abandoned browser sessions and requires users to re-authenticate when returning.

---

## Identity Verification

The process of confirming the authenticity of users, Operators, and systems. Includes passkey (FIDO2/WebAuthn) authentication for users, API keys for Operators, and system fingerprinting for device binding.

---

## Intent Policy

A pre-defined AWS IAM policy that grants specific permissions for a particular service or action. Cloud Operators for AWS request these policies through the Intent-Based Policy System when additional permissions are needed.

---

## Investigation Management

The process of creating, tracking, and resuming chat sessions (investigations) with proper context preservation, operator binding, and state management across browser sessions.

---

## Heartbeat

A periodic health telemetry message sent by the Operator to the control plane every 30 seconds. Contains system identity (hostname, OS, architecture), performance metrics (CPU, memory, disk usage), network information, and uptime data. Used for Operator health monitoring and status determination.

---

## Human-in-the-Loop

A security principle requiring explicit user approval before any state-changing operation executes. All destructive commands, file modifications, and permission escalations require user consent via the approval UI. Read-only operations execute automatically.

---

## Intent

A pre-defined AWS permission set that Cloud Operators for AWS can request via the Intent-Based Policy System. Examples include `ec2_discovery` (view EC2 instances), `s3_read` (read S3 objects), and `rds_management` (manage RDS instances). Intents are organized into discovery (read-only) and management (read-write) categories.

---

## Intent-Based Policy System

The security framework that governs Cloud Operator for AWS permissions through pre-defined Intent sets. When a Cloud Operator for AWS lacks required permissions for an operation, it requests the appropriate Intent from the user, who can approve or deny the permission escalation.

---

## Least Privilege

A security principle where entities receive only the minimum permissions necessary to perform their functions. g8e implements this through Zero Standing Privileges for the Cloud Operator for AWS and user-level permissions for all Operators.

---

## Local Storage

The practice of storing sensitive data on the local Operator rather than in the g8e platform. Applied to command outputs, file contents, and audit logs through the Audit Vault and Scrubbed Vault.

---

## Malware Detection

The capability of g8e Sentinel to identify malicious code, viruses, and security threats in command outputs and file modifications. Uses pattern matching against MITRE ATT&CK framework indicators.

---

## Man-in-the-Middle Prevention

Security measures that prevent attackers from intercepting or modifying communication between components. Implemented through certificate pinning, mTLS, and encrypted channels.

---

## Message Triage

The classification of incoming user messages as "simple" or "complex" using the lightweight assistant model (`LLM_ASSISTANT_MODEL`) before deciding which LLM to invoke. A short-circuit decision tree first checks for OPERATOR_BOUND workflow (always escalates), attachments (always escalates), or empty messages (always escalates). If none apply, the assistant model receives the last 6 conversation history messages (`TRIAGE_CONVERSATION_TAIL_LIMIT = 6`) plus the current message and responds with a structured JSON classification containing complexity, intent, confidence, and follow-up question. "simple" uses the assistant model; "complex" escalates to the main model. Any ambiguous or error response defaults to complex (fail-safe escalation).

---

## Metadata Transmission

The practice of sending only non-sensitive metadata (hashes, sizes, timestamps) to the g8e platform while keeping actual data content local on the Operator. Under the Local-First Audit Architecture (LFAA), the AI retrieves command output on-demand via `fetch_execution_output` rather than receiving it directly.

---

## Multi-Operator Binding

The ability to bind multiple Operators to a single web session simultaneously. Enables coordinated operations across multiple systems while maintaining individual Operator accountability.

---

## Mutual TLS (mTLS)

Two-way TLS authentication where both client and server verify each other's certificates. Used between Operators and g8e Cloud to ensure binary authenticity and prevent forged connections.

---

## Internal Auth Token

The shared secret (`X-Internal-Auth`) used for all service-to-service communication between g8ed, g8ee, and g8es. Generated by g8es on first boot and persisted exclusively in the `g8es-ssl` volume at `/ssl/internal_auth_token`. Discovered by g8ee and g8ed at startup by reading the shared volume. This secret is **never stored in the database**, ensuring platform identity is decoupled from the database lifecycle. Strictly enforced by g8es for all HTTP and WebSocket routes.

---

## Investigation

A chat session or conversation between a user and the AI control plane. Investigations contain message history, operator context, and state. Each investigation has a unique `case_id` and can be resumed across sessions.

---

## Just-in-Time Access

A security model exclusive to the Cloud Operator for AWS where permissions are granted only when needed for a specific operation, only after explicit user approval, and with automatic expiration. When the Cloud Operator lacks required permissions, it requests the appropriate Intent from the user. Upon approval, the permission is attached with a **default 1-hour TTL** and automatically revoked when it expires. Permissions can also be revoked at any time through conversation. Works in conjunction with Zero Standing Privileges to ensure the AI never accumulates persistent access.

---

## Local-First Audit Architecture (LFAA)

An architecture where the Operator is the System of Record for all chat history, execution logs, and file mutations. The self-hosted g8e platform acts as a stateless relay with no sensitive operational data persisting in platform storage. Core philosophy: "The Platform handles routing. The Operator handles retention."

---

## Ledger

A git-backed version control system (`./.g8e/data/ledger`) that maintains cryptographic integrity and restoration capability for all files modified by the Operator. Every file mutation creates a git commit with hash references, enabling time-travel and rollback functionality.

---

## Network Isolation

The security practice of separating network segments to prevent lateral movement. g8e achieves this through outbound-only connectivity, eliminating the need for inbound ports or network exposure.

---

## NSA ZIG Alignment

Compliance with the National Security Agency's Zero Trust Implementation Guidelines (ZIG). g8e **exceeds requirements in 6 of 7 pillars** (User, Device, Application, Data, Automation, Visibility) with the Network & Environment pillar fully compliant. The platform addresses Discovery Phase, Phase One, and Phase Two ZIG requirements (91 activities across 7 pillars). Key capabilities include human-in-the-loop for all operations, Zero Standing Privileges with 1-hour TTL, Sentinel bidirectional protection, Device Links with configurable max_uses for single and mass deployment, and SIEM-ready threat signals with MITRE ATT&CK mapping.

---

## Operator Role

The AWS IAM role used by Cloud Operators for AWS for executing actions. Starts with zero permissions and gains capabilities only through approved intent policy attachments.

---

## Permission Boundary

An AWS IAM policy that defines the maximum permissions that can be granted to a role. g8e uses this to prevent admin-level access and enforce least-privilege principles.

---

## PII Scrubbing

The removal of Personally Identifiable Information from data before platform transmission. g8e Sentinel replaces names, emails, phone numbers, and other PII with safe placeholders.

---

## Pinned Certificate

A server certificate embedded in the Operator binary at compile time. Prevents man-in-the-middle attacks by ensuring Operators only connect to legitimate g8e servers.

---

## ReAct

A reasoning pattern used by g8ee where the LLM cycles through Think → Act → Observe → Repeat. The AI generates a thought, executes an action (command, file operation, tool call), observes the result, and uses that observation to inform the next reasoning step. This enables complex multi-step workflows where the AI can adapt its approach based on command outputs and system state. The Tribunal refines command syntax within this loop without re-invoking the main LLM.

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

## MITRE ATT&CK Framework

A globally-accessible knowledge base of adversary tactics and techniques based on real-world observations. g8e Sentinel maps threat detection patterns to this framework to provide standardized security monitoring and incident response capabilities.

---

## Operator

The language-agnostic, platform-agnostic execution binary that runs on target systems and receives commands from the g8e control plane. A headless, stateless execution environment that operates with least-privilege principles. Any client that follows the g8e events protocol can act as an Operator. Operators connect via outbound-only WebSocket (Gateway Protocol) to g8ed — no inbound connectivity required. g8ed bridges commands between the internal g8es pub/sub bus and the Operator's WebSocket connection. The current ~4MB Go binary (`g8eo`) is the reference implementation for Linux and macOS.

Three types exist, determined at startup:
- **System Operator** (`--cloud=false`) — cloud CLI tools blocked
- **Cloud Operator** (`--cloud`, the default) — cloud CLI tools enabled
- **Cloud Operator for AWS** (`--cloud --provider aws`) — cloud CLI tools enabled with Zero Standing Privileges and intent-based IAM access

---

## Permission Escalation

The process by which Cloud Operators for AWS request additional AWS permissions beyond their current scope. Requires explicit user approval through the Intent-Based Policy System. All escalation requests are logged and auditable.

---

## Operator Panel

The web UI component that displays all Operators belonging to a user, showing their status, hostname, and metrics. Users bind/unbind Operators to their web session through this panel. Supports Device Link for deployment.

---

## Operator Slot

The accounting unit for Operators. Each running Operator — System or Cloud — occupies one slot. Slots are created explicitly at provisioning time (e.g. initial setup, g8ep reauth) or on demand when a Device Link is claimed and no AVAILABLE slot exists for the user. Slot limits are configurable per deployment (default: unlimited in self-hosted mode).

---

## g8es Pub/Sub

The internal real-time messaging system used for communication between g8ee, g8ed, and Operators. Each Operator has dedicated g8es channels for commands, results, and heartbeats. g8ed's Gateway Connection Manager bridges these g8es channels to the Operator's WebSocket connection via the Gateway Protocol. Operators connect to g8es via WebSocket at the `/ws/pubsub` endpoint; the base URL and port are configurable via `G8E_OPERATOR_PUBSUB_URL`.

---

## Scrubbed Vault

The local SQLite database (`./.g8e/local_state.db`) that stores Sentinel-processed command output and file diffs. AI reads from this vault. Sensitive data is replaced with safe placeholders like `[IP_ADDR]`, `[AWS_KEY]`, etc.

---

## Self-Discovery Permissions

The minimal bootstrap permissions granted to Cloud Operators at launch, allowing them to identify their own AWS role and capabilities without accessing user resources. Includes permissions like `sts:GetCallerIdentity` and `iam:GetRole`.

---

## Tribunal

A heterogeneous multi-model architecture in g8ee for refining command syntax. Implements a multi-stage pipeline that fires only for `run_commands_with_operator` workflows:

1. **Generation** — Up to five independent Small Language Model (SLM) passes produce candidate command strings for the same intent + context. Diversity is driven by distinct member personas: Axiom (The Minimalist), Concord (The Guardian), Variance (The Exhaustive), Pragma (The Conventional), and Nemesis (The Adversary).

2. **Voting** — Candidates are normalized (stripped markdown fences and surrounding whitespace) and grouped by exact value. Each unique string receives a weight based on position-decay weighting (earlier passes carry more weight). The highest aggregate weight wins.

3. **Verification** — A separate convergent verifier persona (The Auditor) evaluates the winner against the original intent and reports either "ok" or a short revised command.

4. **Approval** — The refined command is presented to the user for explicit approval.

Failure modes (missing model, provider error, no consensus, verifier failure) halt the execution and return an error to the AI — there is no fallback to the original reasoning agent because it never proposes a command directly.

Configuration via platform settings: `llm_command_gen_passes` (default: 3), `llm_command_gen_verifier` (default: true), `llm_command_gen_enabled` (default: true).

---

## SSE (Server-Sent Events)

The streaming protocol used to push real-time events from the backend to the browser. Used for AI response streaming, command execution results, heartbeat updates, and approval requests.

---

## System Fingerprint

A unique identifier generated by each Operator based on system characteristics including hostname, OS, architecture, and network configuration. Used for Operator identification, duplicate detection, and security monitoring.

---

## Time-Travel

The ability to restore files to any previous state using the Ledger's git history. Users can rollback changes, view historical versions, and recover from unintended modifications through the git-backed version control system.

---

## Unified Approval

The batch execution approval dialog in g8ed that allows a single user approval to cover commands across multiple Operators. When commands need to execute on multiple systems, g8ed displays a unified UI with header "Command Requested (N systems)", a list of target hostnames and Operator types, and a single "Approve for N Systems" button. The approval routes to `/api/operator/approval/respond` once, and g8ee fans out the command to each Operator in parallel (bounded by `command_validation.max_batch_concurrency`), with all per-operator executions correlated back to the approval via a shared `batch_id`.

---

## g8eo (g8e Operator)

Also known as: **g8e.operator**

The Go-based reference implementation of the Operator. A lightweight (~4MB) binary that provides language-agnostic, platform-agnostic execution, file operations, local storage, and heartbeat monitoring. It follows the g8e events protocol, connecting to g8ed via the Gateway Protocol (WebSocket) for command dispatch, result delivery, and heartbeat telemetry.

---

## g8ee

Also known as: **g8ee**

The AI engine component with LLM provider abstraction supporting OpenAI, Anthropic, Gemini, and Ollama providers. Processes natural language requests, reasons about system state, generates commands, and manages investigations. Implements the tool calling loop for Operator interactions and the Intent-Based Policy System for Cloud Operators.

---

## g8ed (g8e Dashboard)

Also known as: **g8ed**

The Node.js/Express web frontend component. Handles user authentication (passkey/FIDO2/WebAuthn), session management, the chat interface, Operator Panel, and SSE streaming to browsers. Routes messages between users and g8ee.

---

## g8es (g8e Data Bus)

Also known as: **g8es**

The Operator binary (`g8e.operator`) running in `--listen` mode. Serves as the platform's single source of truth for persistence and messaging. Provides a document store, KV store, and pub/sub broker via a single static binary with zero external dependencies. See **Coordination Store (SQLite)**.

---

## Ollama (Remote)

The remote LLM inference component. g8e supports any remote Ollama server that provides an API at `/v1`. Used as an LLM backend for g8ee. Configure the endpoint via the setup wizard or `./g8e llm setup`.

---

## WebSession

An authenticated browser session. g8es document store is the authoritative store; g8es KV acts as a fast read cache (cache-aside pattern). Contains user identity and session metadata. Operator–web session bindings are tracked separately by `BoundSessionsService`. Sessions use encrypted cookies with idle timeout (8 hours) and absolute timeout (24 hours).

---

## Zero Standing Privileges

A security model exclusive to the Cloud Operator for AWS where the Operator launches with zero AWS access beyond bootstrap permissions (STS identity, IAM role introspection). Implements a **two-policy separation of execution from authority**: the Operator Role executes actions but cannot modify its own permissions; the Escalation Role can attach/detach pre-defined intent policies but cannot execute AWS actions. This separation prevents the AI from self-escalating privileges. All operational permissions must be explicitly requested and approved by the user through the Intent-Based Policy System.

---

## Zero-Trust AI Architecture

The platform's core security model. g8e assumes no implicit trust in either the AI or the systems it manages. All actions require explicit authorization, all data is filtered through Sentinel before platform transmission, and all execution happens with human oversight.

