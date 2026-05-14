---
title: g8eo
parent: Components
---

# g8eo — g8e Operator

Last Updated: 2026-05-11
Version: v0.2.4

g8eo is the Go-based reference implementation of the Operator for the g8e platform. It provides language-agnostic, secure, real-time command execution and file management for remote system operations. In addition to acting as a remote execution agent, it provides the platform's central persistence and messaging backbone when running in Listen Mode.

> For deep-reference security documentation — CA trust bootstrap, mTLS, fingerprint binding, replay protection, operator binding, Sentinel pre-execution threat detection, output scrubbing patterns, LFAA vault encryption, and the Ledger — see [architecture/security.md](../architecture/security.md).

---

## Core Principles

- **Zero-trust security**: Every operation requires authentication; nothing is implicitly trusted.
- **Protocol-governed execution**: Every command is carried as a UAP JSON `GovernanceEnvelope` with typed `operator.proto` payload bytes and L1/L2/L3 governance metadata.
- **Data sovereignty**: Command output stays local by default; only metadata travels to the cloud.
- **Defense in depth**: Multiple security layers — mTLS, certificate pinning, and Sentinel platform-wide protection.
- **Outbound-only connectivity**: In default mode, g8eo initiates all connections; no inbound ports required.

---

## Operating Modes

g8eo supports four primary operating modes to balance security, performance, and deployment flexibility:

### 1. Outbound Mode (Default)
**The standard deployment.** g8eo acts as a remote Operator that dials into the platform (g8eo listen mode). This enables execution on machines behind strict firewalls without requiring inbound firewall rules.

### 2. Listen Mode (`--listen`)
**Platform Persistence & Messaging.** In this mode, g8eo provides the central document store, KV store, blob storage, and pub/sub broker for application-layer adapters like the Engine (g8ee). It does **not** execute commands or initiate outbound connections.

### 3. SSH Stream Mode (`stream` subcommand)
**Agentless fleet operations.** A Go-native concurrent SSH engine that allows g8e to "stream" itself onto remote hosts. This is used for temporary operations on hosts where a permanent g8eo installation is not desired.

### 4. OpenClaw Mode (`--openclaw`)
**Gateway integration.** Connects directly to an OpenClaw Gateway as a Node Host, allowing g8eo to be used as a high-performance shell execution engine for third-party platforms.

---

## Lifecycle & Pipeline

### Startup Sequence
g8eo initialization ensures security before any core logic is loaded:

1. **Phase 1: Bootstrap (Pre-Auth)**
   - **CA Discovery**: Scans local volume mount paths (`/ssl/ca.crt`, etc.) before falling back to an HTTPS fetch.
   - **Authentication**: Authenticates with the platform using an API key or Device Token.
   - **Configuration**: Receives its Operator ID, Session ID, and a per-operator mTLS certificate.

2. **Phase 2: Service Initialization (Post-Auth)**
   - **Execution Engine**: Starts the shell execution and file editing services.
   - **Storage Layer**: Initializes the local vaults (Scrubbed, Raw, Audit) and the Git-backed Ledger.
   - **Connectivity**: Establishes the persistent WebSocket connection to the pub/sub broker.
   - **Sentinel**: Activates pre-execution threat detection and post-execution output scrubbing.

### Command Pipeline & Governance
g8eo treats all input as untrusted at the protocol boundary. Commands are processed through a strict 3-layer validation hierarchy carried within the `GovernanceEnvelope`:

1. **L1 Technical Bedrock (Hard Gates)**: Enforced via Protobuf reflection. g8eo inspects the `operator.proto` payload for fields with `forbidden_patterns` (e.g., `sudo`, `su`, `rm -rf /`). Violations result in immediate rejection.
2. **L2 Consensus (Tribunal)**: Verified via ED25519 signatures from trusted keys in `.g8e/pki/trusted_signers/`. g8eo rejects any envelope without `governance.l2.key_id` and a valid signature.
3. **L3 Authorization (Approval)**: Human-in-the-loop or auto-approval metadata. Auto-approval is authorization state only and **never** bypasses L1 or L2 gates.

**State Verification**: `state_merkle_root` is mandatory and g8eo verifies it against the Operator-local state root to ensure the command is not based on stale system state.

---

## Storage Architecture

### Local-First Audit Architecture (LFAA)
g8eo maintains four independent local stores in the `.g8e/` directory:

| Store | Purpose | Access |
|---|---|---|
| **Scrubbed Vault** | Sentinel-scrubbed command output and file diffs. | AI Engine (g8ee) |
| **Raw Vault** | Unscrubbed full output for forensics. | Local User Only |
| **Audit Vault** | Append-only event timeline (SQLite). | Platform / Audit |
| **Ledger** | Git-backed cryptographic version history for all modified files. | Platform / Audit |

### Listen Mode API
In Listen Mode, g8eo exposes a hardened internal API for platform components:

- **Document Store (`/db/`)**: JSON document storage with collection-based organization and filtering.
- **KV Store (`/kv/`)**: High-performance key-value storage with TTL support for transient state and locks.
- **Blob Store (`/blob/`)**: Namespace-isolated storage for large payloads (e.g., file contents, logs).
- **Pub/Sub (`/ws/pubsub`)**: Real-time message distribution via WebSockets, secured by mTLS with operator session authentication.

---

## Canonical Truths

The g8e protocol is defined in `shared/proto/`; shared constants JSON registries remain the source for event names, status values, and channel prefixes. g8eo mirrors those registries as compile-time Go constants:

- **Protocol**: Generated Go artifacts under `shared/proto/` mirror `shared/proto/common.proto`, `shared/proto/operator.proto`, and `shared/proto/pubsub.proto`.
- **Events**: `constants/events.go` mirrors `shared/constants/events.json`.
- **Status**: `constants/status.go` mirrors `shared/constants/status.json`.
- **Channels**: `constants/channels.go` mirrors `shared/constants/channels.json`.

---

## Platform Authentication

All platform components authenticate via the public protocol surface:

- **Operator Sessions**: Operators authenticate via mTLS client certificates with URI SAN identity binding (spiffe://g8e.local/operator/<organization_id>/<operator_id>/<operator_session_id>).
- **System Apps**: Reference apps (g8ee, g8ed) authenticate via mTLS with app identity URIs (spiffe://g8e.local/app/<app_id>).
- **Public Enrollment**: New operators enroll via CSR-based enrollment on the bootstrap port (8080) without prior authentication.
- **Revocation Enforcement**: Certificate revocation is checked on every request via the PKI authority.

---

## Operational Reference

### CLI Reference
g8eo provides a comprehensive set of flags for runtime configuration. Use the `--help` flag to see all available options:

```bash
g8e.operator --help
g8e.operator stream --help
```

### Security Exit Codes
If g8eo encounters a critical configuration or security error, it self-terminates with a specific exit code:

| Code | Meaning | Action |
|---|---|---|
| **2** | Auth Failure | Verify API Key / Token |
| **4** | Network Error | Check endpoint connectivity |
| **5** | Config Error | Validate CLI flags / Environment |
| **7** | TLS Cert Failure | CA trust mismatch; check certificates |
