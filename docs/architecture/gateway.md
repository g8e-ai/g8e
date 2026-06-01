---
title: Governance Gateway
---

# g8e Gateway - Governance Gateway

The g8e Protocol platform is composed of two logically distinct roles, both implemented by the reference `g8e` binary:

1.  **Governance Gateway (g8eg)** (Policy Decision Point / PDP): Serves as the central, BFT-governed coordinator for the platform.
2.  **g8e Operator (g8eo)** (Policy Execution Point / PEP): Runs on target hosts as the sovereign execution boundary and MCP server.

---

## Core Principles

- **5-Layer Governance Bedrock**: Every transaction must pass through five mandatory, fail-closed layers sequentially:
    - **L1 Doctrine**: Technical Bedrock (Hard Gates) code pattern matching and threat analysis defined in `internal/services/governance/l1_doctrine.go`.
    - **L2 Consensus**: Multi-agent consensus signature verification using Ed25519 cryptography defined in `internal/services/governance/l2_consensus.go`.
    - **L3 Notary**: Human-in-the-loop authorization (utilizing WebAuthn or cryptographically signed CLI proofs) defined in `internal/services/governance/l3_notary.go`.
    - **L4 Warden**: Pre-dispatch verification gating (validating signatures, replay prevention, expiry, nonces, and state Merkle root) defined in `internal/services/governance/l4_warden.go`.
    - **L5 Actuator**: Isolated boundary tool dispatch (via MCP/A2A) and signed receipt production defined in `internal/services/governance/l5_actuator.go`.
- **mTLS-Everywhere**: All communication is strictly gated by Gateway-owned mutual TLS. No inbound ports are required on managed hosts.
- **Local-First Audit (LFAA)**: The target host remains the source of truth for command history and file mutations, stored in a tamper-evident local ledger.
- **Canonical JSON (GovernanceEnvelope)**: Every mutation action is governed by a canonical JSON `GovernanceEnvelope` (protojson). This is the single canonical container for all g8e mutations, binding identity, intent, state, and governance proofs into one transaction.
- **Transaction Invariants**: Every transaction is identified by a deterministic `transaction_hash` computed from its content. The envelope `id` must match this hash for the transaction to be valid.
- **Protocol vs Implementation**: The protocol is the Gateway. Conforming implementations of the Governance Gateway (g8eg) and g8e Operator (g8eo) enforce these invariants.
- **Sovereign Authority (PKI)**: The Governance Gateway (g8eg) owns the platform's PKI and is the only entity permitted to sign certificates.
- **CSR-Based Enrollment**: Participants enroll by submitting a Certificate Signing Request (CSR) to the Governance Gateway (g8eg). Identities are encoded as SPIFFE URI SANs.

---

## Architecture Overview

The g8e platform is built on the g8e Protocol. Conforming gateway and operator implementations make that protocol live.

- **Governance Gateway (g8eg)** (PDP): The `g8e` binary run in **Gateway mode** (`--doctrine`, `--consensus`, or `--notary`). It acts as the platform's backbone; protocol hub, policy decision point, persistence layer (SQLite), pub/sub broker, root CA, and audit authority.
- **g8e Operator (g8eo)** (PEP): The `g8e` binary run in **Standard Mode**. It acts as the sovereign tool execution boundary on a managed host, executing actions only after they carry a valid, signed gateway lease. Gateway mode operators automatically expose MCP endpoints.

```mermaid
flowchart TD
    subgraph Hub ["Governance Gateway (g8eg) (PDP)"]
        direction TB
        subgraph Layers ["5-Layer Governance"]
            L1["L1 Doctrine"]
            L2["L2 Consensus"]
            L3["L3 Notary"]
            L4["L4 Warden"]
            L5["L5 Actuator"]
            
            L1 --> L2 --> L3 --> L4 --> L5
        end
        db[("SQLite / KV")]
        ps[["Pub/Sub Broker"]]
        ca["Root CA / PKI"]
        
        L5 --- db
        L5 --- ps
        L5 --- ca
    end

    subgraph Apps ["Reference Applications"]
        ensemble["g8e-compatible agentic ensemble"]
    end

    ensemble -- "mTLS JSON" --> L1

    subgraph Host_A ["Managed Host A"]
        g8eoA["g8e Operator (g8eo) (PEP)"] --- LFAA_A["LFAA Ledger & Vault"]
    end

    g8eoA -- "mTLS WSS (JSON)" --> ps
```

---

## Operating Modes: Gateway Mode (PDP)

By passing `--doctrine`, `--consensus`, or `--notary`, the binary transforms into the platform's central backbone.

- **Role**: Reference hub for the bundled deployment.
- **Governance Posture**:
    - **Doctrine** (`--doctrine`): L1 Doctrine enforced, L2 Consensus / L3 Notary audited.
    - **Consensus** (`--consensus`): L1 Doctrine / L2 Consensus enforced, L3 Notary audited.
    - **Notary** (`--notary`): L1 Doctrine / L2 Consensus / L3 Notary strictly enforced.
- **Capabilities**:
    - **Gateway API**: `POST /api/v1/governance/envelopes` is the only customer-facing mutation entry point.
    - **Document Store**: JSON document CRUD on a Collection/ID pattern via `/api/v1/db/*`.
    - **KV Store**: TTL-aware ephemeral state with `GLOB` pattern scanning via `/api/v1/kv/*`.
    - **Blob Store**: Binary persistence for attachments and certificate material via `/api/v1/blob/*`.
    - **Pub/Sub Broker**: High-performance WebSocket fan-out via `/ws/v1/pubsub`. Mutation channels (`cmd:*`) are governed.
    - **Root CA / PKI**: Issues mTLS certificates via CSR-based enrollment with SPIFFE URI SAN identity.
    - **Audit Authority**: Append-only encrypted log of every event and signed `ActionReceipt`.

### Port Topology

The Governance Gateway (g8eg) exposes three logical protocol surfaces. To maintain the mTLS execution boundary, surfaces with different TLS requirements must not share a port.

Default ports are sourced from `internal/constants/ports.go`:

| Surface | Port (default) | Auth | Purpose |
|---|---|---|---|
| **mTLS API + Pub/Sub** | `8440` (mTLS) | mTLS + URI SAN | `/api/v1/governance/envelopes`, `/api/v1/db/*`, `/api/v1/kv/*`, `/api/v1/blob/*`, `/api/v1/pubsub/publish`, and `/ws/v1/pubsub` real-time fan-out. |
| **Bootstrap Port** | `8441` (plain HTTP) | CSR Enrollment | Certificate Signing Requests, CA bundle discovery, and initial provisioning. |
| **Public Port** | `8443` (mTLS) | mTLS + URI SAN | Public mTLS surface for external app enrollment and BYO bootstrap. |

#### Port Constraints

- **mTLS Surface** (`8440`): Requires `tls.RequireAndVerifyClientCert`. This is the primary execution boundary.
- **Bootstrap Surface** (`8441`): Serves plain HTTP for initial CA discovery and bootstrap (no TLS).
- **Public Surface** (`8443`): Requires `tls.RequireAndVerifyClientCert` for mTLS-based external app enrollment.
- **Collision Prevention**: The gateway fails startup if incompatible surfaces (e.g., mTLS and Bootstrap) are assigned to the same port, as this forces a downgrade to `VerifyClientCertIfGiven`.

---

## 5-Layer Verification Sequence

Every transaction submitted to `POST /api/v1/governance/envelopes` must pass through the following layers sequentially:

### L1 Doctrine (Technical Bedrock)
Defined in `internal/services/governance/l1_doctrine.go`. Enforces forbidden patterns (such as `sudo` or `rm -rf /`), blacklists, and whitelists. It also performs MITRE threat detection on incoming payloads.

### L2 Consensus (Consensus Verification)
Defined in `internal/services/governance/l2_consensus.go`. Verifies multi-agent consensus signature using Ed25519 cryptography. In Gateway mode, this requires Ed25519 signatures from trusted consensus agents.

### L3 Notary (Human Authorization)
Defined in `internal/services/governance/l3_notary.go`. Enforces human-in-the-loop authorization using a cryptographic proof of human intent:
- **BYO Clients**: Use WebAuthn or Passkey proofs (FIDO2).
- **CLI Sessions**: Use mTLS certificate fingerprints or Ed25519 signatures bound to the session.

### L4 Warden (Pre-Dispatch Gating)
Defined in `internal/services/governance/l4_warden.go`. Enforces final pre-execution verification gates:
- **Transaction Hash**: The `envelope.id` must match the deterministic transaction hash computed from its content.
- **Expiry**: The `expires_at` timestamp must be in the future.
- **Nonce/Replay**: The `nonce` must not have been used previously (sliding-window protection).
- **State Root**: The `state_merkle_root` (if provided) must match the current state root of the gateway.
- **Signer Trust**: Verifies L2 Consensus / L3 Notary signatures against trusted keys.

### L5 Actuator (Execution and Receipt)
Defined in `internal/services/governance/l5_actuator.go`. Performs isolated boundary tool dispatch (via MCP/A2A) and signed receipt production:
- **Execution**: Dispatches the verified payload to the downstream execution handler (such as an MCP server).
- **Audit**: Persists a `console_audit` record and a signed `ActionReceipt`.
- **Receipt**: Generates a deterministic, signed receipt containing the result and state transitions.

---

## Out-of-Band (OOB) Suspension & WebAuthn Approval Flow

When a standard AI client (such as Claude or Cursor) requests a mutation, it typically cannot generate an L3 Notary human signature.

1.  **Suspension**: The gateway detects missing L3 Notary proof and suspends the transaction in the SQLite `suspended_transactions` store.
2.  **Challenge**: The gateway returns an OOB WebAuthn challenge URL to the AI client.
3.  **Approval**: The human opens the URL, authenticates with a passkey, and approves the specific transaction.
4.  **Resumption**: The gateway attaches the resulting WebAuthn proof to the envelope and resumes the L4 Warden and L5 Actuator flow.

---

## JWT Authentication & JIT User Provisioning

The Governance Gateway (g8eg) provides JWT authentication and Just-In-Time (JIT) user provisioning flows that fully isolate the downstream g8e Operator (g8eo) from Identity Providers (IdP). The Governance Gateway (g8eg) acts as the authentication brain, while the g8e Operator (g8eo) receives a pre-validated, enriched payload via the pub/sub pipe.

### 4-Step JWT Flow

**Step 1: Inbound HTTP Handshake & JWT Verification**
The Governance Gateway (g8eg) intercepts inbound `Authorization: Bearer <JWT>` tokens on public MCP endpoints before routing to downstream execution logic. The middleware cryptographically verifies the JWT signature using JWKS or static public keys, validates `exp` and `iss` claims, and extracts identity claims (`sub`, `tenant_id`, `roles`).

**Step 2: Edge Validation & JIT Account Management**
Following successful token validation, the Governance Gateway (g8eg) ensures the user exists locally and maps their roles:
- **JIT Provisioning**: Checks the SQLite `users` collection for the `sub` (User ID). If the user does not exist, dynamically creates their user account record with default active status.
- **Persona Mapping**: Loads declarative Persona manifests (e.g., YAML definitions representing `security-analyst`, `admin`). Evaluates the JWT `roles` against these manifests to determine the active `binding_persona`.
- **Context Injection**: Stores the resolved `binding_persona` and `tenant_id` into the request context.

**Step 3: Enriched Pub/Sub Handoff (GovernanceEnvelope)**
The Governance Gateway (g8eg) strips the heavy JWT and injects the evaluated security requirements directly into the canonical mutation envelope before passing it to the pub/sub broker:
- The `GovernanceEnvelope` carries `tenant_id` and `binding_persona` as typed fields.
- The pub/sub payload is strictly a canonical `GovernanceEnvelope` carrying typed payloads (e.g., `McpCallRequested`) alongside the validated security metadata.
- The heavy JWT is discarded, reducing payload size.

**Step 4: Native Execution & Data Scrubbing (g8e Operator)**
When the outbound g8e Operator (g8eo) pulls the message off the pub/sub queue, it acts natively on the injected security metadata without second-guessing the Governance Gateway (g8eg):
- The g8e Operator (g8eo) decodes the `GovernanceEnvelope` and extracts `tenant_id` and `binding_persona`.
- These fields propagate into the execution context.
- Native tool isolation applies column masks or data redaction (e.g., stripping `password_hash`, masking emails) directly based on the Persona before returning results.

### Operator Isolation from IdP

This architecture ensures the g8e Operator (g8eo) never requires outbound internet access to verify tokens or manage user state. The Governance Gateway (g8eg) handles all IdP communication, JWT validation, and user lifecycle management. The g8e Operator (g8eo) receives only the pre-validated, enriched security metadata needed for execution.

---

## Session Types

| Session Type | Identifier | Purpose | Authentication |
|---|---|---|---|
| **Operator Session** | `operator_session_id` | Authenticates a specific **g8e Operator (g8eo)** (PEP). | mTLS (Operator Cert) |
| **CLI Session** | `cli_session_id` | Authenticates a **BYO/CLI client**. | mTLS (CLI Cert) |
| **Web Session** | `web_session_id` | Authenticates a **browser-based client**. | Passkey (WebAuthn) |
| **JWT Session** | `sub` (User ID) | Authenticates via external IdP JWT. | JWT (validated at Gateway) |

---

## Implementation Reference

| Concern | File |
|---|---|
| Gateway mode entry | `cmd/operator/main.go` (runGatewayMode) |
| Gateway service | `internal/services/gateway/gateway_service.go` |
| Coordination Store | `internal/services/gateway/gateway_db.go` |
| Pub/Sub broker | `internal/services/gateway/gateway_pubsub.go` |
| L1 Doctrine | `internal/services/governance/l1_doctrine.go` |
| L2 Consensus | `internal/services/governance/l2_consensus.go` |
| L3 Notary | `internal/services/governance/l3_notary.go` |
| L4 Warden | `internal/services/governance/l4_warden.go` |
| L5 Actuator | `internal/services/governance/l5_actuator.go` |
| PKI / CertStore | `internal/services/gateway/gateway_certs.go` |
| Secret Manager | `internal/services/gateway/secret_manager.go` |
| Workload identity | `protocol/workload_identity.go` |
| Collections registry | `internal/constants/collections.go` |

---

## Canonical Collections

| Collection | Description |
|---|---|
| **Authentication & Sessions** | `users`, `web_sessions`, `operator_sessions`, `cli_sessions`, `bound_sessions`, `passkey_challenges` |
| **Organizations & Tenants** | `organizations`, `invitations` |
| **Audit & Security** | `login_audit`, `auth_admin_audit`, `account_locks`, `console_audit`, `revoked_certificates` |
| **Operators & Usage** | `operators`, `operator_usage` |
| **Cases & Investigations** | `cases`, `investigations`, `tasks` |
| **Governance & Reputation** | `reputation_state`, `reputation_commitments`, `stake_resolutions`, `trusted_signers`, `app_policies` |
| **AI & Context** | `memories`, `agent_activity_metadata`, `personas` |
| **Configuration** | `settings` |
| **Testing** | `chaos_events` |

---

## Related Documentation

- [**g8e Protocol**](./g8e.md) - The wire contract and governance hierarchy.
- [**g8e Operator**](./operator.md) - Sovereign host-side execution agent and MCP server.
