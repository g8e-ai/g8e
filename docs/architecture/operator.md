---
title: g8e Operator
---

# g8e Operator

Last Updated: 2026-06-10

The **g8e Operator** is the host-side, sovereign agent role defined by the g8e Protocol: a daemon that functions as the remote execution target and universal protocol translator under the security guarantees of the platform. An Operator receives transactions, enforces L1/L2/L3 verification, executes through a defensive boundary, and emits signed receipts anchored to a host-local ledger.

The reference implementation of a g8e-compliant Policy Execution Point (PEP) is the g8e Node. It functions as both a **g8e Operator** and a **Model Context Protocol (MCP) Server**. The same Go codebase provides the logic for both the g8e Gateway (PDP) and the g8e Operator (PEP), differentiated by runtime configuration as seen in `../../cmd/operator/main.go`:

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
The **L1Doctrine** layer provides foundational hard gates. It utilizes Protobuf field-option extensions (`forbidden_patterns`) to block malicious strings at the schema level and executes real-time MITRE ATT&CK heuristics to detect threats like reverse shells, privilege escalation, and destructive disk operations. L1 is the first line of defense, cannot be bypassed, and is defined in `../../internal/services/governance/l1_doctrine.go`.

### L2: Consensus
The **L2Consensus** layer verifies the intent of the request via a Byzantine Fault Tolerant (BFT) quorum. It validates Ed25519 signatures from independent reasoning agents (the **Tribunal**) against the Operator's locally trusted `SignerStore`. This ensures that no single upstream agent can unilaterally mutate the host. The consensus mechanism is defined in `../../internal/services/governance/l2_consensus.go`.

### L3: Notary (Authorization)
The **L3Notary** layer enforces human-in-the-loop authorization. For web-based sessions, it validates FIDO2/WebAuthn (Passkey) proofs. For CLI sessions, it validates mTLS certificate fingerprints and cryptographic signatures over the transaction hash. For operator sessions, it validates mTLS certificate fingerprints only (passkey auth is not available for operators). Mutations are blocked until a valid L3 proof is presented, unless specifically exempted by an `AutoApprove` policy for benign diagnostic commands. The notary verification logic is defined in `../../internal/services/governance/l3_notary.go`.

### L4: Warden (Pre-dispatch Gate)
The **L4Warden** is the final verification gate before execution, defined in `../../internal/services/governance/l4_warden.go`. It enforces:
1. **Integrity**: Validates that `id == transaction_hash == SHA256(canonical_fields)`. The wire format is canonical JSON (`protojson`), but the signing basis is a deterministic hash of normalized fields.
2. **Freshness**: Enforces `expires_at` and checks for replay attacks via a local `ReplayStore`.
3. **State Binding**: Validates that the `state_merkle_root` matches the host's current ledger root.
4. **Quorum**: Confirms that L1, L2, and L3 proofs meet the current **Governance Posture** (`doctrine`, `consensus`, or `notary`).

### L5: Actuator (Execution Boundary)
The **L5Actuator** is the singular execution boundary permitted to mutate host state, defined in `../../internal/services/governance/l5_actuator.go`. It dispatches verified payloads to internal handlers (shell, file edit, etc.) and uses a **dual-receipt model**:
1. **Pre-execution**: Signs an `ActionReceipt` with status `EXECUTING` and commits it to the local `SQLAuditStore`.
2. **Rehydration**: Restores sensitive data (PII, credentials) that was scrubbed upstream by the **Sovereign Execution Boundary**, using local tokens.
3. **Execution**: Dispatches to the handler and captures the output.
4. **Post-execution**: Signs a final `ActionReceipt` with status `COMPLETED` or `FAILED`, captures the new `state_root_after`, and publishes the signed result back to the Gateway.

---

## 3. Core Subsystems

### Universal Protocol Translator
By exposing standard MCP and A2A interfaces, the Operator acts as the admission gate for BYO (Bring-Your-Own) AI clients. It isolates the complex requirements of the `GovernanceEnvelope` (such as transaction hashing and L2/L3 signature collection) behind a standardized tool-calling facade, mapping native JSON-RPC/HTTP requests directly to governed mutations.

### Native Tool Execution
The g8e Operator compiles native tool playbooks directly into the g8e Node to provide memory-safe, boundary-enforced execution for common operational tasks. These tools execute within the g8e Operator's execution boundary locally, without proxying to downstream MCP servers. AI agents interact with clean JSON schemas while the internal memory-safe execution layer enforces hard boundaries.

#### Database Triage & Performance Playbook
- **db_discover_topology**: Automatically scans database schemas, tables, and column data types, returning a highly compressed JSON map. AI agents need this first to prevent hallucinated queries.
- **db_query_validate**: Intercepts any AI-generated SQL and runs it through EXPLAIN QUERY PLAN natively. If the engine flags an unindexed, full-table scan on a production dataset, the g8e Node rejects the task before execution.
- **db_isolated_read**: Executes SELECT statements using a database handle opened strictly with SQLITE_OPEN_READONLY. This prevents the AI from executing destructive injections (e.g., ; DROP TABLE...).
- **db_index_triage**: Queries internal fragmentation statistics and indexes to diagnose slow queries without letting the AI guess the performance bottleneck.

#### Telemetry & Log Digestion Playbook
- **log_stream_filter**: Reads native log paths or standard buffers, applies a regex match requested by the AI, runs the matched chunks through the scrubbing engine to redact secrets/PII, and pushes only the sanitized fragments.
- **sys_oom_detect**: Directly parses /var/log/dmesg or system logs to scan for Out-Of-Memory (OOM) killer events, process kills, or core panic dumps, isolating the exact failing PID.
- **config_diff_mask**: Compares application configuration states against environmental baselines. It strips out actual passwords, tokens, and salts inside the g8e Node before outputting the structural differences to the AI.

#### Resource & Process Governance Playbook
- **proc_metric_top**: Directly parses the Linux /proc filesystem in memory to extract process IDs, memory maps, and CPU tracking. It returns a tightly structured JSON array of the top resource-hogging processes.
- **fs_disk_profile**: Recursively calculates directory sizes natively (equivalent to an optimized du --max-depth=2) starting from an approved path root. It instantly isolates unrotated log files or bloated tmp directories.
- **proc_signal_safe**: Allows the AI to send explicit termination signals (SIGTERM, SIGKILL) to a process, but enforces a strict g8e Node-level denylist (e.g., rejecting attempts to kill PID 1, system init, or the g8e Node itself).

#### Network & Connectivity Validation Playbook
- **net_socket_audit**: Directly inspects active network sockets (/proc/net/tcp and /proc/net/udp) to map established connections and confirm if expected internal microservices are actually listening.
- **net_endpoint_ping**: Initiates native TCP handshakes or ICMP requests to defined target host/port combinations to verify local network routing and DNS resolution performance.
- **net_http_probe**: Performs a lightweight native HTTP request (similar to curl -I) to internal API endpoints, returning only the status codes, headers, and latency metrics while discarding heavy response payloads.

### Identity, PKI, and mTLS
The g8e Operator establishes workload identity bound to SPIFFE-style URI SANs, strictly enforced over mutual TLS (mTLS). See [Network Architecture](./network.md) for complete SPIFFE ID formats, PKI hierarchy, mTLS enforcement policies, and certificate revocation mechanisms.

### JWT Authentication Isolation
The g8e Operator is fully isolated from Identity Providers (IdP). The g8e Gateway handles all JWT validation, user provisioning, and role mapping. JIT provisioning is **owner-controlled** and requires an active invitation:
- **Owner-Centric Model**: All authentication requires owner approval via invitations. The platform owner creates invitations for specific identities (IdP `sub` or email) before JIT provisioning can occur.
- **Invitation-Based JIT**: When a JWT is presented, the g8e Gateway validates the signature and checks for an active invitation. If no invitation exists, authentication is rejected (403 Forbidden). If a valid invitation exists, the user is provisioned and bound to the owner's organization, then the invitation is consumed.
- **Strict TTL**: Sessions have a 1-hour TTL by default. Long-lived access requires programmatic renewal or re-authentication.
- **g8e Gateway Responsibility**: The g8e Gateway validates inbound `Authorization: Bearer <JWT>` tokens, performs invitation-gated JIT user provisioning, maps JWT roles to Personas, and injects `tenant_id` and `binding_persona` into the `GovernanceEnvelope`.
- **g8e Operator Responsibility**: The g8e Operator receives only the pre-validated, enriched security metadata in the envelope. It decodes `tenant_id` and `binding_persona` from the envelope, propagates them into the execution context, and applies Persona-based data scrubbing (column masks, redaction) before returning results.
- **No IdP Dependency**: The Operator never requires outbound internet access to verify tokens or manage user state. This enables air-gapped and high-security deployments where the Operator has no external network connectivity.

### Local-First Audit Architecture (LFAA)
The host is the authoritative source of truth for all mutations.
- **SQLAuditStore**: An append-only SQLite log of every event and signed `ActionReceipt`. It is fail-closed: events missing a valid `operator_session_id` are rejected as defined in `../../internal/services/storage/audit_store.go`. All sensitive content fields (content_text, command_stdout, command_stderr) are encrypted at rest using the vault subsystem (mandatory since v1.0.10).
- **Scrubbed vs. Raw Logs**: Sensitive data scrubbing separates logs into a **Scrubbed Vault** (safe for AI reading) and a **Raw Vault** (unscrubbed forensic record for human security audits).
- **Git-Backed Ledger**: Implements a two-phase commit (`state_root_before` / `state_root_after`) for file mutations using native `go-git`. Files are mirrored and can be restored to any prior state using `../../internal/services/storage/ledger.go`. The ledger also encrypts mirrored files at rest when the vault is unlocked (mandatory since v1.0.10).

---

## 4. Governance & Safety

- **Sovereign Execution Boundary**: Data sovereignty is enforced at the boundary. Sensitive data is scrubbed before leaving the host and replaced with tokens (`{{UEI_N}}`). These tokens are rehydrated by the `L5Actuator` only at the moment of execution.
- **Strict Canonical JSON**: While schemas are defined in Protobuf, the wire format for all client-facing surfaces is strictly canonical JSON (`protojson`) for maximum ecosystem compatibility.
- **Strict Protocol Enforcement**: The Operator enforces the current 5-layer verification protocol. Outdated formats, HMAC fallbacks, and unsigned inputs are rejected.

---

## 5. Current Implementation Status

The reference implementation (`g8eo`) currently supports:

- **Universal Protocol Translation**: Functional MCP and A2A gateway mapping standard tool calls to signed `GovernanceEnvelope` mutations.
- **Fail-Closed 5-Layer Verification**: L1 (Doctrine), L2 (Consensus), L3 (Notary), L4 (Warden), and L5 (Actuator) gates are fully enforced on every transaction.
- **Outbound-Only mTLS Connectivity**: Dial-out reverse tunnels with zero inbound port requirements. See [Network Architecture](./network.md) for detailed communication patterns and port topology.
- **Local-First Audit Vault**: Git-backed ledger and fail-closed SQLite audit vault enforcing session existence for all writes. Mandatory encryption at rest for all storage services (since v1.0.10).
- **Deterministic Hash Binding**: SHA-256 transaction hash integrity enforced across all wire formats.
- **Sovereign Execution Boundary**: Automated scrubbing and rehydration of sensitive data during the execution lifecycle.
- **Host-Unique Signing**: Cryptographic Action Receipts signed by host-specific keys.
- **Zero-Dependency Node Binary**: Statically compiled Go binary for air-gapped and high-security deployments.
- **Expanded Native Tool Catalog**: 26 native tools compiled into the binary for memory-safe, boundary-enforced execution across database triage, log digestion, process governance, network validation, system introspection, and cloud metadata operations.

---

## 6. Post-Bootstrap Workflow

After completing platform bootstrap via `./g8e auth login`, follow this workflow to begin using the Operator:

### 1. Verify Gateway Health

Confirm the g8e Gateway is running and accessible:

```bash
./g8e gw status
```

### 2. Enroll Remote Operators (Multi-Host Setups)

For distributed enforcement across multiple hosts, enroll each remote operator:

```bash
./g8e security pki enroll -e <gateway-ip>
```

Each Operator receives a unique SPIFFE workload identity bound to its mTLS certificate.

### 3. Configure AI Client Integration

Configure your AI client to connect to the Gateway's universal HTTP MCP endpoint:

```bash
# Generate universal HTTP MCP configuration
./g8e gw mcp-config

# Set environment variables for mTLS
export G8E_CLIENT_CERT_PATH=.g8e/pki/client.crt
export G8E_CLIENT_KEY_PATH=.g8e/pki/client.key
export G8E_CA_CERT_PATH=.g8e/pki/ca.crt

# Copy the JSON configuration output to your MCP client's config file
```

**Protocol Integration:**
- **All Clients**: Use the universal HTTP endpoint with mTLS authentication
- **IDE Integration (Cursor, Windsurf, Claude Code)**: Configure MCP client with HTTP transport
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

### 6. Review Audit Trail

Query the local audit vault to verify governance enforcement:

```bash
./g8e data query --collection audit_vault
```

Each entry includes:
- Transaction hash
- L1/L2/L3 verification status
- Signed ActionReceipt
- State root before/after
- Operator session ID

### 7. Explore Native Tools

The Operator compiles native tool playbooks for common operational tasks:
- **Database Triage**: Schema discovery, query validation, isolated reads, index triage
- **Log Digestion**: Stream filtering, OOM detection, config diffing
- **Process Governance**: Resource profiling, safe signal handling, process tree inspection
- **Network Validation**: Socket auditing, endpoint probing, HTTP health checks, DNS resolution, TLS certificate inspection, SSH known hosts management
- **System Introspection**: System information, environment variables, time/clock, service status, container status, disk usage
- **File Operations**: File checksumming, disk profiling
- **Cloud & Orchestration**: Cloud metadata, Kubernetes inspection, Git operations, operator deployment
- **Shell Execution**: Safe shell command execution

See [Native Tool Execution](#native-tool-execution) for the complete tool catalog.

---

## 7. Implementation Reference

| Concern | Authoritative file |
|---|---|
| Ingress Verification (`L4Warden`) | `../../internal/services/governance/l4_warden.go` |
| Execution Boundary (`L5Actuator`) | `../../internal/services/governance/l5_actuator.go` |
| Scrubbing (Data Scrubbing) | `../../internal/services/scrubbing/boundary.go` |
| Technical Bedrock (`L1Doctrine`) | `../../internal/services/governance/l1_doctrine.go` |
| Consensus (`L2Consensus`) | `../../internal/services/governance/l2_consensus.go` |
| Notary (`L3Notary`) | `../../internal/services/governance/l3_notary.go` |
| Local Audit Vault | `../../internal/services/storage/audit_store.go` |
| Native Git Ledger | `../../internal/services/storage/ledger.go` |
| Native Tools | `../../internal/services/mcp/native_tools.go` |
| Native Tool Handlers | `../../internal/services/mcp/native_handlers.go` |
| Operator Entrypoint | `../../cmd/operator/main.go` |
| Protocol Definitions | `../../protocol/proto/g8e/common/v1/common.proto` |
| Operator Protocol | `../../protocol/proto/g8e/operator/v1/operator.proto` |
| Workload Identity | `../../protocol/workload_identity.go` |
| Network architecture | `./network.md` |
| Event Constants | `../../protocol/constants/events.json` |
| Port Constants | `../../protocol/constants/ports.json` |

See also: [g8e Protocol](./protocol.md), [g8e Gateway](./gateway.md).
