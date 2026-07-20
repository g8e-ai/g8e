---
title: g8e Operator
---

# g8e Operator

Last Updated: 2026-07-19
Version: v1.5.9

The **g8e Operator** is the host-side, sovereign agent role defined by the g8e Protocol: a daemon that functions as the remote execution target and universal protocol translator under the security guarantees of the platform. An Operator receives transactions, enforces L1/L2/L3 verification, executes through a defensive boundary, and emits signed receipts anchored to a host-local ledger.

The reference implementation of a g8e-compliant Policy Execution Point (PEP) is the g8e binary file. It functions as both a **g8e Operator** and a **Model Context Protocol (MCP) Server**. The same Go codebase provides the logic for both the g8e Gateway (PDP) and the g8e Operator (PEP), differentiated by CLI subcommands (`gw start` vs `operator start`):

- **g8e Gateway (PDP)**: When run in Gateway mode (utilizing `L1Doctrine`, `L2Consensus`, and `L3Notary` as a central authority), it acts as the central Policy Decision Point (PDP) with platform persistence and pub/sub brokering.
- **g8e Operator (PEP)**: When run as a host agent, it acts as the Policy Execution Point (PEP) and MCP server, enforcing local verification before host mutation.

This document focuses on the **g8e Operator** (PEP) role.

---

## 1. Introduction

The core invariant of the Operator is absolute defense-in-depth: a typed, signed, state-bound transaction reaches a sovereign host agent that distrusts upstream inputs and refuses to mutate reality unless every independent proof checks out. 

The Operator is the only component capable of mutating the host. It executes remote operations, such as running shell commands, editing files, interacting with cloud CLIs, and reading file history, but only after translating the request into a canonical `GovernanceEnvelope` transaction and verifying it locally.

---

## 2. 5-Layer Verification Sequence

When a command targets an Operator, it progresses through a strict, fail-closed pipeline consisting of five distinct layers of verification and execution:

### L1: Doctrine (Technical Bedrock)
The **L1Doctrine** layer provides foundational hard gates. It blocks malicious strings at the schema level and executes real-time MITRE ATT&CK heuristics to detect threats like reverse shells, privilege escalation, and destructive disk operations. L1 is the first line of defense and cannot be bypassed.

### L2: Consensus
The **L2Consensus** layer verifies the intent of the request via a Byzantine Fault Tolerant (BFT) quorum. It validates Ed25519 signatures from independent reasoning agents (the **Tribunal**) against the Operator's locally trusted signer store. This ensures that no single upstream agent can unilaterally mutate the host.

### L3: Notary (Authorization)
The **L3Notary** layer enforces human-in-the-loop authorization. For web-based sessions, it validates FIDO2/WebAuthn (Passkey) proofs. For CLI sessions, it validates mTLS certificate fingerprints and cryptographic signatures over the transaction hash. For operator sessions (outbound mode), it validates mTLS certificate fingerprints and cryptographic signatures without CLI session or certificate revocation checks (passkey auth is not available for operators). Mutations are blocked until a valid L3 proof is presented. Non-mutation actions (as classified by the action type's intrinsic mutation property) do not require L3 proof. L3 approval notifications use Server-Sent Events (SSE) for real-time delivery: the Gateway emits an `approval.completed` event scoped to the submitting user when a passkey approval is verified, and CLI clients subscribe to the SSE stream rather than polling. See [SSE Streaming](./sse.md) for the full SSE architecture.

### L4: Warden (Pre-dispatch Gate)
The **L4Warden** is the final verification gate before execution. It enforces:
1. **Integrity**: Validates that `id == transaction_hash == SHA256(canonical_fields)`. The wire format is canonical JSON, but the signing basis is a deterministic hash of normalized fields.
2. **Freshness**: Enforces `expires_at` and checks for replay attacks via a local replay protection store.
3. **State Binding**: Validates that the `state_merkle_root` matches the host's current ledger root.
4. **Quorum**: Confirms that L1, L2, and L3 proofs meet the current **Governance Posture** (`doctrine`, `consensus`, or `notary`).

### L5: Actuator (Execution Boundary)
The **L5Actuator** is the singular execution boundary permitted to mutate host state. It dispatches verified payloads to internal handlers (shell, file edit, etc.) and uses a **dual-receipt model** with **JIT capability minting**:
1. **Pre-execution**: Signs an `ActionReceipt` with status `EXECUTING` and commits it to the local audit vault.
2. **Rehydration**: Restores sensitive data (PII, credentials) that was scrubbed upstream by the **Sovereign Execution Boundary**, using local tokens.
3. **JIT Capability Minting**: Mints a scoped, single-action, self-dissolving `Capability` bound to the transaction hash and action type, enforcing zero standing privileges. The capability is injected into the execution context for downstream handlers.
4. **Execution**: Dispatches to the handler and captures the output.
5. **Capability Dissolution**: Dissolves the JIT capability immediately after execution completes or fails, preventing reuse.
6. **Post-execution**: Signs a final `ActionReceipt` with status `COMPLETED` or `FAILED`, captures the new `state_root_after`, and publishes the signed result back to the Gateway.

---

## 3. Core Subsystems

### Universal Protocol Translator
By exposing standard MCP and A2A interfaces, the Operator acts as the admission gate for BYO (Bring-Your-Own) AI clients. It isolates the complex requirements of the `GovernanceEnvelope` (such as transaction hashing and L2/L3 signature collection) behind a standardized tool-calling facade, mapping native JSON-RPC/HTTP requests directly to governed mutations.

### Native Tool Execution
The g8e Operator compiles native tool playbooks directly into the g8e binary file to provide memory-safe, boundary-enforced execution for common operational tasks. These tools execute within the g8e Operator's execution boundary locally, without proxying to downstream MCP servers. AI agents interact with clean JSON schemas while the internal memory-safe execution layer enforces hard boundaries.

#### Database Triage & Performance Playbook
- **db_discover_topology**: Automatically scans database schemas, tables, and column data types, returning a highly compressed JSON map. AI agents need this first to prevent hallucinated queries.
- **db_query_validate**: Intercepts any AI-generated SQL and runs it through EXPLAIN QUERY PLAN natively. If the engine flags an unindexed, full-table scan on a production dataset, the g8e binary file rejects the task before execution.
- **db_isolated_read**: Executes SELECT statements in read-only mode to prevent destructive injections (e.g., ; DROP TABLE...).
- **db_index_triage**: Queries internal fragmentation statistics and indexes to diagnose slow queries without letting the AI guess the performance bottleneck.

#### Telemetry & Log Digestion Playbook
- **log_stream_filter**: Reads native log paths or standard buffers, applies a regex match requested by the AI, runs the matched chunks through the scrubbing engine to redact secrets/PII, and pushes only the sanitized fragments.
- **sys_oom_detect**: Parses system logs to scan for Out-Of-Memory (OOM) killer events, process kills, or core panic dumps, isolating the exact failing PID.
- **config_diff_mask**: Compares application configuration states against environmental baselines. It strips out actual passwords, tokens, and salts inside the g8e binary file before outputting the structural differences to the AI.

#### Resource & Process Governance Playbook
- **proc_metric_top**: Extracts process IDs, memory maps, and CPU tracking from the host. It returns a tightly structured JSON array of the top resource-hogging processes.
- **fs_disk_profile**: Recursively calculates directory sizes natively (equivalent to an optimized du --max-depth=2) starting from an approved path root. It instantly isolates unrotated log files or bloated tmp directories.
- **proc_signal_safe**: Allows the AI to send explicit termination signals (SIGTERM, SIGKILL) to a process, but enforces a strict g8e binary file-level denylist (e.g., rejecting attempts to kill critical system processes or the g8e Operator itself).
- **proc_tree**: Inspects the process hierarchy to map parent-child relationships and identify process trees for targeted operations.

#### Network & Connectivity Validation Playbook
- **net_socket_audit**: Inspects active network sockets to map established connections and confirm if expected internal microservices are actually listening.
- **net_endpoint_ping**: Initiates native TCP handshakes or ICMP requests to defined target host/port combinations to verify local network routing and DNS resolution performance.
- **net_http_probe**: Performs a lightweight native HTTP request (similar to curl -I) to internal API endpoints, returning only the status codes, headers, and latency metrics while discarding heavy response payloads.
- **net_dns_resolve**: Performs DNS lookups to verify domain resolution and identify DNS server issues.
- **tls_cert_inspect**: Inspects TLS certificates from endpoints to validate expiration, chain of trust, and certificate metadata.
- **net_ssh_known_hosts**: Manages SSH known_hosts entries for secure remote access validation.

#### System Introspection Playbook
- **sys_info**: Returns comprehensive system information including OS version, kernel, architecture, and hardware details.
- **sys_env_vars**: Lists environment variables with optional filtering and masking of sensitive values.
- **sys_service_status**: Checks the status of system services (systemd, init.d) to determine if services are running, stopped, or failed.
- **sys_container_status**: Inspects container runtime status (Docker, containerd) to identify running containers and their health.
- **sys_time_clock**: Reports system time, timezone, and clock synchronization status (NTP).

#### File Operations Playbook
- **fs_file_checksum**: Calculates cryptographic checksums (SHA256, MD5) of files to verify integrity and detect changes.
- **fs_disk_usage**: Reports disk usage statistics for mounted filesystems to identify capacity issues.
- **read_file**: Reads file contents with optional line range limits and encoding detection for safe file inspection.

#### Cloud & Orchestration Playbook
- **cloud_metadata**: Retrieves cloud provider metadata (AWS, GCP, Azure) to identify instance identity, region, and availability zone.
- **k8s_inspect**: Queries Kubernetes API to inspect pod status, deployments, and cluster health.
- **git_ops**: Performs Git operations (status, log, diff) to inspect repository state and changes.
- **operator_deploy**: Deploys or updates g8e operators on remote hosts via secure channels.

#### Shell Execution Playbook
- **run_shell_command**: Executes shell commands within the g8e execution boundary with strict argument validation and output capture.

### Identity, PKI, and mTLS
The g8e Operator establishes workload identity bound to SPIFFE-style URI SANs, strictly enforced over mutual TLS (mTLS). See [Network Architecture](./network.md) for complete SPIFFE ID formats, PKI hierarchy, mTLS enforcement policies, and certificate revocation mechanisms.

### JWT Authentication Isolation
The g8e Operator is fully isolated from Identity Providers (IdP). The g8e Gateway handles all JWT validation, user provisioning, and role mapping. JIT provisioning is **owner-controlled**:
- **Owner-Centric Model**: The first human to authenticate becomes the Platform Owner. Starting the Gateway is the owner's act of authorization -- no standing invite codes or manual approval steps are required for subsequent CSR enrollment.
- **CSR-Based Enrollment**: For mTLS-based authentication, clients enroll via Certificate Signing Request (CSR) where they generate their own key pair and the Gateway acts as a Certificate Authority (CA) to sign the certificate. No shared secrets, no API keys to leak.
- **JWT-Based JIT**: When a JWT is presented, the g8e Gateway validates the signature and provisions the user subject to platform owner authorization. The user is bound to the owner's organization.
- **Strict TTL**: Sessions have a 1-hour TTL by default. Long-lived access requires programmatic renewal or re-authentication.
- **g8e Gateway Responsibility**: The g8e Gateway validates inbound `Authorization: Bearer <JWT>` tokens, performs JIT user provisioning subject to owner authorization, maps JWT roles to Personas, and injects `tenant_id` and `binding_persona` into the `GovernanceEnvelope`.
- **g8e Operator Responsibility**: The g8e Operator receives only the pre-validated, enriched security metadata in the envelope. It decodes `tenant_id` and `binding_persona` from the envelope, propagates them into the execution context, and applies Persona-based data scrubbing (column masks, redaction) before returning results.
- **No IdP Dependency**: The Operator never requires outbound internet access to verify tokens or manage user state. This enables air-gapped and high-security deployments where the Operator has no external network connectivity.

### Local-First Audit Architecture (LFAA)
The host is the authoritative source of truth for all mutations.
- **Audit Vault**: An append-only audit log of every event and signed `ActionReceipt`. It is fail-closed: events missing a valid operator session ID are rejected. All sensitive content fields are [encrypted at rest](./encryption.md) using the vault subsystem.
- **Scrubbed vs. Raw Logs**: Sensitive data scrubbing separates logs into a **Scrubbed Vault** (safe for AI reading) and a **Raw Vault** (unscrubbed forensic record for human security audits).
- **Git-Backed Ledger**: Implements a two-phase commit (`state_root_before` / `state_root_after`) for file mutations. Files are mirrored and can be restored to any prior state. The ledger also encrypts mirrored files at rest when the vault is unlocked.

---

## 4. Governance & Safety

- **Sovereign Execution Boundary**: Data sovereignty is enforced at the boundary. Sensitive data is scrubbed before leaving the host and replaced with tokens. These tokens are rehydrated by the Actuator only at the moment of execution.
- **Strict Canonical JSON**: While schemas are defined in Protobuf, the wire format for all client-facing surfaces is strictly canonical JSON for maximum ecosystem compatibility.
- **Strict Protocol Enforcement**: The Operator enforces the current 5-layer verification protocol. Outdated formats, HMAC fallbacks, and unsigned inputs are rejected.

---

## 5. Current Implementation Status

The reference implementation currently supports:

- **Universal Protocol Translation**: Functional MCP and A2A gateway mapping standard tool calls to signed `GovernanceEnvelope` mutations.
- **Fail-Closed 5-Layer Verification**: L1 (Doctrine), L2 (Consensus), L3 (Notary), L4 (Warden), and L5 (Actuator) gates are fully enforced on every transaction.
- **SSE-Based L3 Approvals**: L3 notary approvals use Server-Sent Events for real-time notification delivery, replacing polling-based waiting. CLI clients subscribe to the SSE stream and receive `approval.completed` events when passkey verification succeeds.
- **Outbound-Only mTLS Connectivity**: Dial-out reverse tunnels with zero inbound port requirements. See [Network Architecture](./network.md) for detailed communication patterns and port topology.
- **Local-First Audit Vault**: Git-backed ledger and fail-closed audit vault enforcing session existence for all writes. Encryption at rest for all storage services.
- **Deterministic Hash Binding**: SHA-256 transaction hash integrity enforced across all wire formats.
- **Sovereign Execution Boundary**: Automated scrubbing and rehydration of sensitive data during the execution lifecycle.
- **Host-Unique Signing**: Cryptographic Action Receipts signed by host-specific keys.
- **Zero-Dependency Binary**: Statically compiled Go binary for air-gapped and high-security deployments.
- **Expanded Native Tool Catalog**: 30 native tools compiled into the binary for memory-safe, boundary-enforced execution across database triage, log digestion, process governance, network validation, system introspection, file operations, cloud metadata, and shell execution.

---

## 6. Post-Bootstrap Workflow

After completing platform bootstrap via `./g8e auth enroll`, follow this workflow to begin using the Operator. Enrollment automatically registers a passkey via browser after successful CLI session enrollment, streamlining the onboarding flow.

### 1. Verify Gateway Health

Confirm the g8e Gateway is running and accessible:

```bash
./g8e gw status
```

### 2. Enroll Remote Operators (Multi-Host Setups)

For distributed enforcement across multiple hosts, enroll each remote operator:

```bash
./g8e gw security pki enroll -e <gateway-ip>
```

Each Operator receives a unique SPIFFE workload identity bound to its mTLS certificate. To deploy the binary to remote hosts, use `./g8e operator deploy` or `./g8e operator stream`.

### 3. Configure AI Client Integration

Configure your AI client to connect to the Gateway's universal HTTP MCP endpoint:

```bash
# Generate MCP configuration for a specific agent
./g8e mcp agent show claude

# The command outputs JSON configurations for three transport modes:
#   g8e.local (mTLS), IP Address (mTLS), and Stdio Transport
# Copy the appropriate JSON configuration to your MCP client's config file
```

**Protocol Integration:**
- **All Clients**: Use the universal HTTP endpoint with mTLS authentication
- **Supported Agents (Claude Code, Codex, Goose, Gemini CLI)**: Configure MCP client with HTTP transport
- **Custom BYO Clients**: Use HTTP MCP or A2A protocol endpoints

### 4. Test with a Simple Mutation

Execute a benign diagnostic command to verify the verification sequence:

```bash
# Via MCP client: request a tool call
# Example: db_discover_topology or sys_oom_detect
```

The Operator will:
1. Translate the request into a GovernanceEnvelope
2. Enforce L1 (Technical Bedrock) checks
3. Verify L2 (Consensus) signatures if in consensus/notary mode
4. Require L3 (Notary) approval if in notary mode
5. Execute through the Actuator boundary
6. Emit a signed ActionReceipt

### 5. Review Audit Trail

Query the audit vault to verify governance enforcement:

```bash
# List signed receipts (auto-discovers session from credentials)
./g8e audit receipts

# Query raw audit events with optional session filter
./g8e audit events --limit 100

# Generate a compliance report (JSON)
./g8e audit report

# Export the full receipts bundle for archival
./g8e audit export --out receipts.json

# Aggregate summary by event type and receipt status
./g8e audit summary
```

Each receipt includes:
- Transaction hash
- L2/L3 verification status
- Signed ActionReceipt
- State root before/after
- Operator session ID

### 6. Explore Native Tools

The Operator compiles native tool playbooks for common operational tasks:
- **Database Triage**: Schema discovery, query validation, isolated reads, index triage
- **Log Digestion**: Stream filtering, OOM detection, config diffing
- **Process Governance**: Resource profiling, safe signal handling, process tree inspection
- **Network Validation**: Socket auditing, endpoint probing, HTTP health checks, DNS resolution, TLS certificate inspection, SSH known hosts management
- **System Introspection**: System information, environment variables, time/clock, service status, container status
- **File Operations**: File checksumming, disk usage analysis, file reading
- **Cloud & Orchestration**: Cloud metadata, Kubernetes inspection, Git operations, operator deployment
- **Shell Execution**: Safe shell command execution

See [Native Tool Execution](#native-tool-execution) for the complete tool catalog.

---

## 7. See Also

- [g8e Protocol](../../protocol/docs/spec.md) for protocol definitions and wire formats
- [g8e Gateway](./gateway.md) for PDP architecture and communication patterns
- [Network Architecture](./network.md) for SPIFFE ID formats, PKI hierarchy, mTLS enforcement, and port topology
- [Governance](./governance.md) for posture configuration and tribunal setup
- [Auth Architecture](./auth.md) for enrollment, passkey, and session management details
- [Getting Started](../guides/getting_started.md) for initial setup and usage examples
- [Connect Operator to Gateway](../guides/connect_operator_to_gateway.md) for remote operator enrollment
- [SSE Streaming](./sse.md) for real-time event delivery architecture
- [Scripts](./scripts.md) for bootstrap and deploy script reference
- [Storage Architecture](./storage.md) for audit vault and ledger internals
- [Encryption](./encryption.md) for encryption at rest details
