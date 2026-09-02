---
title: g8e Gateway
---

# g8e Gateway

Last Updated: 2026-09-02
Version: v2.1.3

The g8e Protocol platform is implemented as a single static binary that operates in two modes:

1.  **Governance Gateway** (Policy Decision Point / PDP): The binary run in Gateway mode (`--posture doctrine`, `--posture consensus`, `--posture ratify`, or `--posture notary`). The Gateway owns the policy decision layers (L1 Doctrine, L2 Consensus, L3 Notary) and serves as the platform's central persistence layer, pub/sub broker, root CA, and governance envelope entry point. The Gateway is governed by a multi-signature Consensus (K-of-N Ed25519 votes), not Byzantine consensus. An in-process Operator substrate (PEP) runs L4 Warden and L5 Actuator locally for operations targeting the gateway host itself.
2.  **Governed Operator** (Policy Execution Point / PEP): The same binary run in Standard Mode (`operator start`). It runs on target hosts as the sovereign execution boundary and MCP server, handling L4 Warden (pre-dispatch verification) and L5 Actuator (execution and signed receipt production) for operations on its own host.

---

## Core Principles

- **5-Layer Governance Bedrock**: Every transaction must pass through five mandatory, fail-closed layers sequentially:
    - **L1 Doctrine**: Technical Bedrock (Hard Gates) code pattern matching and threat analysis.
    - **L2 Consensus**: Consensus-based deliberation producing L2 votes (Ed25519 signatures) over the transaction hash. The gateway delegates L2 deliberation to an enrolled Consensus rather than self-signing.
    - **L3 Notary**: Human-in-the-loop authorization (utilizing WebAuthn passkey proofs with optional CLI mTLS session verification).
    - **L4 Warden**: Pre-dispatch verification gating (validating signatures, replay prevention, expiry, nonces, and state Merkle root).
    - **L5 Actuator**: Isolated boundary tool dispatch (via MCP/A2A) and signed receipt production.
- **mTLS-Everywhere**: All communication is strictly gated by Gateway-owned mutual TLS. No inbound ports are required on managed hosts. The platform uses `g8e.local` as its canonical SPIFFE trust domain for workload identities. See [Network Architecture](./network.md) for detailed mTLS enforcement, PKI hierarchy, and identity management.
- **Local-First Audit Architecture (LFAA)**: The target host remains the source of truth for command history and file mutations, stored in a tamper-evident local ledger and SQL audit store.
- **Canonical JSON (GovernanceEnvelope)**: Every mutation action is governed by a canonical JSON `GovernanceEnvelope` (protojson). This is the single canonical container for all g8e mutations, binding identity, intent, state, and governance proofs into one transaction.
- **Transaction Invariants**: Every transaction is identified by a deterministic `transaction_hash` computed from its content. The envelope `id` must match this hash for the transaction to be valid.
- **Protocol vs Implementation**: The protocol is the Gateway. Conforming implementations of the Governance Gateway and Governed Operator enforce these invariants.
- **Sovereign Authority (PKI)**: The Governance Gateway owns the platform's PKI and is the only entity permitted to sign certificates. PKI files are stored under a platform-managed directory. See [Network Architecture](./network.md) for the complete PKI hierarchy and certificate management.
- **CSR-Based Enrollment**: Participants enroll by submitting a Certificate Signing Request (CSR) to the Governance Gateway. Identities are encoded as SPIFFE URI SANs. See [Network Architecture](./network.md) for detailed enrollment procedures.

---

## Architecture Overview

The g8e platform is built on the g8e Protocol. Conforming gateway and Operator implementations make that protocol live.

- **Governance Gateway** (PDP): The binary run in **Gateway mode** (`--posture doctrine`, `--posture consensus`, `--posture ratify`, or `--posture notary`). It acts as the platform's backbone: protocol hub, policy decision point, persistence layer (SQLite), pub/sub broker, root CA, and audit authority. The Gateway owns layers L1-L3 (Doctrine, Consensus, Notary) as policy decisions. An in-process Operator substrate (PEP) handles L4-L5 (Warden, Actuator) locally for operations targeting the gateway host itself.
- **Governed Operator** (PEP): The binary run in **Standard Mode** (`operator start`). It acts as the sovereign tool execution boundary on a managed host, executing actions only after they carry a valid, signed gateway lease. Operators automatically serve as MCP servers, exposing tool capabilities through the gateway's unified MCP endpoint. Each remote Governed Operator handles L4-L5 (Warden, Actuator) locally for operations on its own host, re-verifying L1-L3 proofs from the Gateway before execution.

The same five layers run on every conforming host. The gateway applies L1–L3, then either its own in-process Operator or a remote Governed Operator applies L4–L5. Each remote operator re-verifies L1–L3 proofs from the gateway before execution.

For a detailed view of the Gateway service stack and its relationship to the Operator substrate, see the [Gateway Service Stack Diagram](../diagrams/graph-gateway-services.md).

---

## Operating Modes: Gateway Mode (PDP)

By passing `--posture doctrine`, `--posture consensus`, `--posture ratify`, or `--posture notary`, the g8e binary file transforms into the platform's central backbone.

- **Role**: Reference hub for the bundled deployment.
- **Governance Posture**:
    - **Doctrine** (`--posture doctrine`): L1 Doctrine enforced, L2 Consensus / L3 Notary audited.
    - **Consensus** (`--posture consensus`): L1 Doctrine / L2 Consensus enforced, L3 Notary audited.
    - **Ratify** (`--posture ratify`): L1 Doctrine / L3 Notary enforced, L2 Consensus audited.
    - **Notary** (`--posture notary`): L1 Doctrine / L2 Consensus / L3 Notary strictly enforced.
- **Capabilities**:
    - **Gateway API**: The only customer-facing mutation entry point.
    - **Document Store**: JSON document CRUD on a Collection/ID pattern.
    - **KV Store**: TTL-aware ephemeral state with GLOB pattern scanning.
    - **Blob Store**: Binary persistence for attachments and certificate material.
    - **Pub/Sub Broker**: WebSocket fan-out with governed mutation channels.
    - **Root CA / PKI**: Issues mTLS certificates via CSR-based enrollment with SPIFFE URI SAN identity.
    - **Audit Authority**: Append-only encrypted log of every event and signed ActionReceipt.
    - **Unified MCP Endpoint**: Single-URL JSON-RPC dispatch contract for MCP protocol communication.

### Port Topology

The Governance Gateway exposes two logical protocol surfaces. To maintain the mTLS execution boundary, surfaces with different TLS requirements must not share a port. See [Network Architecture](./network.md) for detailed port topology, authentication requirements, and port constraints.

**HTTP Port (8080)**: Plain HTTP for bootstrap enrollment and PKI discovery endpoints only. This port serves CA bundle and fingerprint discovery, CLI recovery request/status/complete (token-scoped, no mTLS required), deploy scripts, and node binary distribution. Legacy trust scripts and the old CLI enrollment handler are removed; OS trust installation is handled by `auth enroll user` directly. No MCP routes are available on this port.

**HTTPS Port (8443)**: mTLS for all routes including API, public, enrollment, and MCP endpoints. MCP endpoints require mTLS authentication (or JWT when JWKS is configured). The HTTPS router also serves the Swagger UI and OpenAPI specification; these documentation endpoints require the same mTLS client authentication as the rest of the API surface.

### Configuration Propagation

Vault and rate-limit settings flow from CLI flags through the gateway configuration loader. Vault paths default to platform infrastructure directories when not explicitly specified. Rate limiting is disabled when the requests-per-second value is zero.

The `--doctrine-dir` flag (env: `G8E_DOCTRINE_DIR`) specifies a directory of JSON doctrine files loaded at startup. File-loaded threat detectors are appended after hardcoded MITRE patterns. The loaded doctrine instance is shared between the MCP Gateway threat scanner (field-value scanning) and L4 Warden (payload validation). Defaults to empty (hardcoded patterns only) for backward compatibility.

### Onboarding Wizard

`g8e gw start --interactive` (or `-i`) launches an interactive TUI wizard before gateway startup. The `g8e gw setup` command launches the same wizard without starting the gateway, producing a resolved configuration for inspection. The wizard guides users through four steps: Network & Identity, Security & Governance Posture, Agent Tooling & Routing, and Review & Confirm. The wizard produces a focused configuration containing only wizard-owned fields; the gateway startup process merges the result into resolved CLI flags, preserving non-wizard flags (ports, directories, log level, rate limits). Cancellation returns without starting the gateway. The wizard is explicit opt-in only; existing flags and automation continue to work unchanged.

---

## HTTP Router Architecture

The gateway exposes two logical HTTP surfaces.

- **HTTP surface (plain text)**: Used only for initial bootstrap, PKI discovery, CLI recovery request/status/complete, node binary download, deploy scripts, and a catch-all redirect to HTTPS. It does not serve mTLS or MCP routes.
- **HTTPS surface (mTLS)**: Carries all API, console, passkey, MCP/A2A, and operator management traffic.

The HTTPS router classifies each route into one of four auth modes:

- **Public**: health, state, PKI discovery, landing, logout, console passkey registration, bootstrap enrollment, device/CSR enrollment, and token-scoped CLI recovery.
- **mTLS only**: data, blob, and KV stores, operator management, governance, consensus, audit, pub/sub, SSE push, PKI management, passkey CLI status, enrollment token generation, CLI rotation, and CLI recovery approval via the headless `approve-cli` endpoint.
- **Web session only**: user profile, approvals, passkey credential management, and CLI recovery approval via the browser Console SPA.
- **Dual**: SSE stream and event endpoints accept either a valid client certificate or a web-session cookie.

Application certificates are blocked from privileged governance and query paths. Exact paths and auth mode assignments are part of the wire contract; see the [g8e Protocol specification](../../protocol/docs/spec.md) for the full list.

### Console SPA

The console SPA is an embedded single-page application served at `/console/`. The SPA provides browser-based passkey registration, authentication, credential management, OOB transaction approval, and a live audit stream. The SPA auto-detects approval hash fragments in the URL and triggers the WebAuthn approval flow after successful authentication.

---

## MCP Endpoint Architecture

The Governance Gateway (g8eg) implements a unified MCP (Model Context Protocol) endpoint. This endpoint provides a single-URL JSON-RPC dispatch contract that standard MCP clients expect.

### Unified Endpoint Contract

The unified endpoint accepts POST requests containing JSON-RPC 2.0 messages and dispatches by the `method` field. The endpoint implements the MCP protocol handshake via the `initialize` method and negotiates protocol version with clients.

### Supported Methods

The unified endpoint dispatches the following MCP methods:
- `initialize`: Protocol handshake and capability negotiation
- `ping`: Liveness probe
- `tools/list`: Tool catalog discovery
- `tools/call`: Tool execution
- `resources/list`: Resource catalog discovery
- `resources/templates/list`: Resource template discovery
- `resources/read`: Resource content retrieval
- `prompts/list`: Prompt catalog discovery
- `prompts/get`: Prompt content retrieval
- `a2a/call`: Agent-to-agent communication

The endpoint also supports SSE (Server-Sent Events) via GET requests for streaming capabilities.

### Native Tool Registry

The gateway maintains a centralized tool registry. The tool registry provides thread-safe tool registration and lookup, enforcing tool name validation and input schema compliance. All native tools implement a common interface providing name, description, input schema, and execution capabilities.

### Input Validation Framework

The gateway implements an input validation system with fail-closed security principles:
- **SQL Query Validation**: Rejects empty queries, trailing semicolons to prevent statement chaining
- **URL Validation**: Parses and validates URLs, restricting to http/https schemes, rejecting localhost and loopback addresses, and blocking private IP ranges to prevent SSRF attacks
- **Protocol Validation**: Validates network protocol strings (tcp, udp, tcp6, udp6, raw) for socket audit operations to prevent path traversal
- **Git Path Validation**: Validates repository paths and references to prevent path traversal and command injection
- **Kubernetes Validation**: Validates resource names and namespaces against K8s naming conventions
- **Cloud Metadata Validation**: Validates cloud metadata operation types
- **File Path Validation**: Validates file paths to prevent directory traversal and null byte injection
- **SSH Path Validation**: Validates SSH config and known hosts paths
- **Hostname Validation**: Validates hostnames to prevent shell injection
- **Operator Validation**: Validates operator binary paths and arguments to prevent command injection

### Native Tool Ecosystem

The gateway provides a set of native tools covering database operations, filesystem analysis, network diagnostics, process management, and system monitoring. All tools are registered at startup via explicit registration.

**Database Tools**:
- `db_discover_topology`: Database schema discovery
- `db_index_triage`: Index analysis and optimization recommendations
- `db_isolated_read`: Isolated database read operations with query validation
- `db_query_validate`: SQL query validation and analysis

**Filesystem Tools**:
- `fs_disk_profile`: Disk usage and filesystem profiling
- `fs_disk_usage`: Disk usage analysis
- `fs_file_checksum`: File integrity verification via checksum
- `read_file`: Read file contents with validation
- `log_stream_filter`: Log stream filtering and analysis

**Network Tools**:
- `net_endpoint_ping`: Network endpoint reachability testing
- `net_http_probe`: HTTP endpoint probing with SSRF protection
- `net_socket_audit`: Socket state auditing with protocol validation
- `net_dns_resolve`: DNS resolution and query
- `net_ssh_known_hosts`: SSH known hosts management
- `tls_cert_inspect`: TLS certificate inspection and validation

**Process Tools**:
- `proc_metric_top`: Process resource metrics and top-like analysis
- `proc_signal_safe`: Safe process signal handling
- `proc_tree`: Process tree visualization and analysis

**System Tools**:
- `sys_oom_detect`: Out-of-memory detection and analysis
- `sys_info`: System information gathering
- `sys_env_vars`: Environment variable inspection
- `sys_service_status`: System service status checking
- `sys_container_status`: Container status monitoring
- `sys_time_clock`: System time and clock information

**Configuration Tools**:
- `config_diff_mask`: Configuration diffing with sensitive data masking

**Cloud Tools**:
- `cloud_metadata`: Cloud provider metadata retrieval

**Kubernetes Tools**:
- `k8s_inspect`: Kubernetes resource inspection

**Git Tools**:
- `git_ops`: Git repository operations

**Operator Tools**:
- `operator_deploy`: Operator deployment and management

**Execution Tools**:
- `run_shell_command`: Safe shell command execution

**Audit Tools**:
- `audit_receipt_list`: Lists signed ActionReceipt records from the operator audit vault, scoped to an operator session, with optional action_type and not_before filtering and limit/offset pagination
- `audit_receipt_get`: Retrieves a single signed ActionReceipt by transaction_id from the operator audit vault

---

## Incremental State Tracking

The gateway implements incremental state tracking to optimize performance by avoiding full state recomputation. The database schema includes:

### State Version Table

A monotonically increasing counter (`state_version`) tracks changes across all data stores. Triggers automatically increment this counter on document, KV store, and blob mutations. This allows the gateway to detect when state has changed without scanning entire tables.

### Change Tracking Mechanisms

Triggers on the `documents`, `kv_store`, and `blobs` tables increment the state version on insert, update, and delete operations. The gateway queries the current version before performing expensive state root calculations, skipping computation when the version has not changed.

### Bound vs Observed State Tiering

The gateway distinguishes between two state tiers for Merkle root computation:

- **Bound State**: Authoritative state (documents, bound KV entries, bound blobs, token keymap) that gates transaction admission. The bound state root is the freshness root that in-flight envelopes depend on.
- **Observed State**: Telemetry and environmental readings stored as observed KV entries and blobs. These are hashed into a separate observed state root that is chained into the audit ledger but does not gate transaction admission.

This tiering prevents observed-state churn (continuous telemetry updates) from invalidating in-flight envelopes, which would degrade fail-closed into fail-always.

---

## Health Endpoint Consolidation

The gateway exposes two health endpoints, one per protocol surface:

### Main Health Check (HTTPS)

The main health endpoint on the HTTPS port performs full readiness checks:
- Service readiness via an optional readiness callback
- Platform settings document availability
- State root calculation success

The response includes `status`, `mode`, `version`, `pid` (OS process ID), `governance_ready` (whether the governance pipeline is initialized), and `state_merkle_root` (the current state root). This endpoint is unauthenticated to bypass authentication middleware for monitoring purposes.

### Bootstrap Health Check (HTTP)

The bootstrap health endpoint on the HTTP port is a lighter check that verifies only the readiness callback. It skips platform settings and state root verification, making it suitable for initialization monitoring before the database is fully configured. The response includes `status`, `mode`, `version`, `pid`, and `governance_ready`.

### Operational Status (`g8e gw status`)

`g8e gw status` reports the health of both the localhost Gateway and the Docker Compose stack in a single command. When the Gateway is running in Docker Compose, the command probes the containerized gateway endpoint in addition to the localhost endpoint and surfaces the Docker Compose service status. The TUI diagnoses Docker gateway state when its configured endpoint is unreachable, suppresses duplicate usage output on errors, and records machine-observable scenario success, failure, and cancellation checkpoints.

---

## 5-Layer Verification Sequence

Every transaction submitted to `POST /api/v1/governance/envelopes` must pass through five layers sequentially. The Gateway (PDP) owns layers L1-L3 as policy decisions. The Operator substrate (PEP), whether in-process on the Gateway host or remote on a managed host, owns layers L4-L5 as enforcement and execution. Remote Governed Operators re-verify L1-L3 proofs from the Gateway before running L4-L5 locally (see [Operator Architecture](./operator.md)).

### L1 Doctrine (Technical Bedrock) - Gateway (PDP)
Enforces forbidden patterns (such as `sudo` or `rm -rf /`), blacklists, and whitelists. It also performs MITRE threat detection on incoming payloads.

### L2 Consensus (Consensus Deliberation) - Gateway (PDP)
The gateway delegates L2 deliberation to an enrolled Consensus service rather than self-signing votes. The Consensus evaluates the transaction and produces `L2Vote` entries (Ed25519 signatures over the transaction hash) from its member agents. Under `consensus` and `notary` postures, the gateway calls the Consensus's `Deliberate` endpoint (via in-process deliberation) and attaches the returned L2 votes to the envelope. The L4 Warden then verifies the quorum of valid signatures against the `ConsensusPolicy` stored in the consensus store.

### L3 Notary (Human Authorization) - Gateway (PDP)
The gateway notary enforces human-in-the-loop authorization using a layered model: passkey authorization is required for all proofs, and CLI mTLS session verification is applied as an additional transport-auth layer when `mtls_cert_fingerprint` is present.
- **Web Sessions**: Use WebAuthn passkey proofs (FIDO2).
- **CLI Sessions**: Use WebAuthn passkey proofs with additional mTLS certificate fingerprint verification. The CLI session verifier checks user active status, session ownership, fingerprint match (constant-time compare), session active and expiry, and certificate revocation.
- **Operator Sessions**: Use mTLS certificate fingerprints only (passkey auth is not available for operators).
- **JWT Sessions**: Use JWT tokens validated at the gateway with JIT user provisioning.

### L4 Warden (Pre-Dispatch Gating) - Operator (PEP)
Runs on the Operator substrate (in-process for gateway-host operations, remote for managed-host operations). Enforces final pre-execution verification gates:
- **Transaction Hash**: The `envelope.id` must match the deterministic transaction hash computed from its content.
- **Expiry**: The `expires_at` timestamp must be in the future.
- **Nonce/Replay**: The `nonce` must not have been used previously (sliding-window protection) via the replay store.
- **State Root**: The `state_merkle_root` (if provided) must match the current state root of the gateway.
- **Signer Trust**: Verifies L2 Consensus / L3 Notary signatures against trusted keys in the signer store.

### L5 Actuator (Execution and Receipt) - Operator (PEP)
Runs on the Operator substrate and fails closed across evidence, persistence, commitment, and dispatch:
- **Pre-execution evidence**: Signs and persists the complete protojson `ActionReceipt` with deterministic L4 stage evidence before any side effect.
- **Commitment**: Builds and appends a signed `CommitmentAttestation` against the current SQLite chain head under the write lock, then records both chain hashes in receipt evidence.
- **Execution**: Dispatches the verified payload through a scoped capability to the downstream execution handler (such as an MCP server).
- **Final evidence**: Adds L5 outcome evidence and state transitions, signs and persists the final receipt, and attaches a signed `ReceiptPersistenceAttestation` proving its durable audit-record association.

---

## Out-of-Band (OOB) Suspension & WebAuthn Approval Flow

When a standard AI client (such as Claude Code, Codex, Goose, Gemini CLI, or Devin CLI) requests a mutation, it typically cannot generate an L3 Notary human signature.

1.  **Suspension**: The gateway detects missing L3 Notary proof and suspends the transaction in the SQLite `suspended_transactions` store.
2.  **Challenge**: The gateway returns an OOB WebAuthn challenge URL to the AI client.
3.  **Approval**: The human opens the URL, authenticates with a passkey, and approves the specific transaction.
4.  **Resumption**: The gateway attaches the resulting WebAuthn proof to the envelope and resumes the L4 Warden and L5 Actuator flow.

---

## JWT Authentication & JIT User Provisioning

The Governance Gateway (g8eg) provides JWT authentication and Just-In-Time (JIT) user provisioning flows that fully isolate the downstream Governed Operator (g8eo) from Identity Providers (IdP). The Governance Gateway (g8eg) acts as the authentication brain, while the Governed Operator (g8eo) receives a pre-validated, enriched payload via the pub/sub pipe.

### 4-Step JWT Flow
The JWT authentication logic is implemented in the gateway auth middleware.

**Step 1: Inbound HTTP Handshake and JWT Verification** The Governance Gateway (g8eg) intercepts inbound `Authorization: Bearer <JWT>` tokens on public MCP endpoints before routing to downstream execution logic. The middleware cryptographically verifies the JWT signature using JWKS, validates `exp`, `nbf`, `iss`, and `aud` claims, and extracts identity claims (`sub`, `tenant_id`, `roles`).

**Step 2: Edge Validation and JIT Account Management** Following successful token validation, the Governance Gateway (g8eg) ensures the user exists locally and maps their roles:
- **JIT Provisioning**: Checks the SQLite `users` collection for the `sub` (User ID). If the user does not exist, provisions the user account subject to platform owner authorization.
- **Persona Mapping**: Loads declarative Persona definitions (e.g., `security-analyst`, `admin`) from the `personas` collection in the document store. Evaluates the JWT `roles` against these persona definitions to determine the active `binding_persona`.
- **Context Injection**: Stores the resolved `binding_persona` and `tenant_id` into the request context.

**Step 3: Enriched Pub/Sub Handoff (GovernanceEnvelope)** The Governance Gateway (g8eg) strips the heavy JWT and injects the evaluated security requirements directly into the canonical mutation envelope before passing it to the pub/sub broker:
- The `GovernanceEnvelope` carries `tenant_id` and `binding_persona` as typed fields.
- The pub/sub payload is strictly a canonical `GovernanceEnvelope` carrying typed payloads (e.g., `McpCallRequested`) alongside the validated security metadata.
- The heavy JWT is discarded, reducing payload size.

**Step 4: Native Execution and Data Scrubbing (Governed Operator)** When the outbound Governed Operator (g8eo) pulls the message off the pub/sub queue, it acts natively on the injected security metadata without second-guessing the Governance Gateway (g8eg):
- The Governed Operator (g8eo) decodes the `GovernanceEnvelope` and extracts `tenant_id` and `binding_persona`.
- These fields propagate into the execution context.
- Native tool isolation applies column masks or data redaction (e.g., stripping `password_hash`, masking emails) directly based on the Persona before returning results.

### Operator Isolation from IdP

This architecture ensures the Governed Operator (g8eo) never requires outbound internet access to verify tokens or manage user state. The Governance Gateway (g8eg) handles all IdP communication, JWT validation, and user lifecycle management. The Governed Operator (g8eo) receives only the pre-validated, enriched security metadata needed for execution.

---

## Session Types

| Session Type | Identifier | Purpose | Authentication |
|---|---|---|---|
| **Operator Session** | `operator_session_id` | Authenticates a specific **Governed Operator (g8eo)** (PEP). | mTLS (Operator Cert) |
| **CLI Session** | `cli_session_id` | Authenticates a **BYO/CLI client**. | mTLS (CLI Cert) |
| **Web Session** | `web_session_id` | Authenticates a **browser-based client**. | Passkey (WebAuthn) |
| **JWT Session** | `sub` (User ID) | Authenticates via external IdP JWT. | JWT (validated at Gateway) |

---

## Agent Integration

The Governance Gateway provides zero-config ingress for agentic CLI coding tools (Claude Code, OpenAI Codex, Goose, Gemini CLI, Devin CLI) through the MCP agent subcommands. Each supported agent has its native/built-in tools disabled at launch, forcing all I/O through the g8e MCP gateway for full L1-L5 governance enforcement.

### Agent Subcommands

The agent integration provides the following subcommands:

**`g8e mcp agent list`**: Lists all supported agent binaries that g8e supports for MCP integration: Claude Code, OpenAI Codex, Devin CLI, Goose, and Gemini CLI.

**`g8e mcp agent show <agent>`**: Prints MCP client configuration for connecting to the Governance Gateway from local coding tools. Displays three configuration matrices:
- g8e.local (mTLS): For production environments with DNS configured
- IP Address (mTLS): For environments without DNS or direct IP access
- Stdio Transport: For direct native tool access without gateway

**`g8e mcp agent run [--url <url>] [--verify] [-- <command> [args...]]`**: Launches an AI agent or wraps an external MCP server with g8e governance. Supports:
- Launching supported agents (claude, codex, devin, goose, gemini) with automatic MCP configuration and native tool disabling
- Wrapping external MCP servers via HTTP or subprocess for governance reverse proxy
- Forwarding extra arguments to the agent binary
- Runtime verification (`--verify`, enabled by default): before launching the agent, g8e verifies that the agent's tool-disabling configuration was correctly written, checking MCP config files, CLI flags, and agent-specific settings (e.g., `tools.core: []` for Gemini, `--disallowed-tools` for Claude/Codex, `--no-profile` for Goose). Use `--verify=false` to skip verification (e.g., for CI/CD pipelines where the config is pre-validated).

### Stdio Proxy

The stdio proxy bridges stdio MCP transport to the gateway mTLS HTTPS endpoint:
- Accepts JSON-RPC 2.0 requests over stdin/stdout
- Proxies requests to the gateway HTTPS endpoint with mTLS
- Identity is carried in the delegated mTLS certificate's URI SANs (no session headers needed)
- Detects L3 approval responses and subscribes to SSE for completion
- Auto-opens browser for L3 approval URLs
- Re-sends the original request once the approval.completed SSE event arrives

### L3 Approval SSE Notification

When the gateway returns an L3 approval response, the stdio proxy:
1. Extracts the approval URL from the response (structured field or text content)
2. Opens the browser automatically using the cross-platform browser utility
3. Subscribes to the gateway's SSE stream (`GET /api/v1/sse/stream`) scoped to the user
4. Waits for the `approval.completed` SSE event with a matching transaction hash
5. Re-sends the original request and returns the result

CLI credentials (mTLS) are required for L3 approval flows. The SSE wait has a 3-minute timeout (the gateway's approval request TTL is 2 minutes).

### Browser Utility

The browser utility provides cross-platform browser opening for L3 approval URLs, supporting macOS, Linux, and Windows.

---

## Related Documentation

- [**g8e Protocol**](../../protocol/docs/spec.md) - The wire contract and governance hierarchy.
- [**g8e Operator**](./operator.md) - Sovereign host-side execution agent and MCP server.
- [**Getting Started**](../guides/getting_started.md) - CLI commands, agent integration, and setup guides.
