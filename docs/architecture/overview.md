---
doc_id: overview
title: Platform Architecture Overview
audience: maintainers and coding agents
status: current
last_updated: 2026-10-01
version: v2.2.6
owners:
  - cmd/g8e/
  - internal/cli/
  - internal/services/
  - protocol/
related:
  - governance.md
  - gateway.md
  - operator.md
  - consensus.md
  - auth.md
  - network.md
  - encryption.md
  - storage.md
  - sse.md
  - evals.md
  - model-provenance.md
  - public_spectator.md
  - ensemble.md
  - dashboard.md
  - protocol.md
  - scripts.md
  - ../devs/docs.md
  - ../devs/devs.md
when_to_read: High-level architectural orientation across the g8e zero-trust governance and execution platform, its five-layer interlock pipeline, node topologies, trust boundaries, client surfaces, storage model, and component interactions.
do_not_use_for:
  - Canonical five-layer interlock verification mechanics and envelope schemas (governance.md)
  - Gateway HTTP/HTTPS routes, session routing, and MCP endpoint internals (gateway.md)
  - Operator execution boundary, native tool catalog, and local audit vault (operator.md)
  - PKI hierarchy, TLS 1.3/mTLS transport, SPIFFE identity, and port topology (network.md)
  - Platform coding invariants and repository standards (../devs/devs.md)
---

# Platform Architecture Overview

## Purpose

Documents the top-level architecture of the g8e zero-trust governance and execution platform. g8e inserts a deterministic, cryptographic, five-layer governance interlock between AI agents, human operators, and execution runtimes. This document defines the system topology, core binary modes, first-party auxiliary services, governance pipeline, trust boundaries, network identities, persistence model, and compliance evidence structures across the repository.

## Quick index

- [Purpose](#purpose)
- [Quick index](#quick-index)
- [Invariants](#invariants)
- [Owned surfaces](#owned-surfaces)
- [Procedures](#procedures)
  - [What g8e Is: Zero-Trust Governance and Execution Platform](#what-g8e-is-zero-trust-governance-and-execution-platform)
  - [Core Binaries, Operating Modes, and First-Party Services](#core-binaries-operating-modes-and-first-party-services)
  - [The Five-Layer Governance Pipeline](#the-five-layer-governance-pipeline)
  - [Governance Postures and Enforcement Matrix](#governance-postures-and-enforcement-matrix)
  - [From Intent to Governed Execution](#from-intent-to-governed-execution)
  - [AI Client Surface and Protocol Ingress](#ai-client-surface-and-protocol-ingress)
  - [Governed Operator Roles and Multi-Operator Coexistence](#governed-operator-roles-and-multi-operator-coexistence)
  - [Network Architecture, Port Topology, and SPIFFE Identity](#network-architecture-port-topology-and-spiffe-identity)
  - [Authentication, Session Models, and Platform Enrollment](#authentication-session-models-and-platform-enrollment)
  - [Cryptography, Key Custody, and Per-Runtime Vault Hierarchy](#cryptography-key-custody-and-per-runtime-vault-hierarchy)
  - [Local-First Audit Architecture (LFAA) and Storage Model](#local-first-audit-architecture-lfaa-and-storage-model)
  - [SSE Event Streaming Infrastructure](#sse-event-streaming-infrastructure)
  - [Observe Projections, Evaluation Explorer, and Public Spectator Mirror](#observe-projections-evaluation-explorer-and-public-spectator-mirror)
  - [Protocol Wire Contracts and Code Generation](#protocol-wire-contracts-and-code-generation)
  - [Compliance Evidence and KSI Verification Foundation](#compliance-evidence-and-ksi-verification-foundation)
  - [Docker Compose Unified Stack and Deployment Topologies](#docker-compose-unified-stack-and-deployment-topologies)
- [Anti-patterns](#anti-patterns)
- [Links out](#links-out)

Key invariant groups: [Pipeline and Interlock](#pipeline-and-interlock-inv-arch-pipe), [Node Topology and Operating Modes](#node-topology-and-operating-modes-inv-arch-node), [Ingress and Client Boundary](#ingress-and-client-boundary-inv-arch-ingr), [Identity and Trust Boundaries](#identity-and-trust-boundaries-inv-arch-trust), [Storage and Sovereign Audit](#storage-and-sovereign-audit-inv-arch-stor).

## Invariants

Ids are stable. Append the next free number within each group; do not renumber.

### Pipeline and Interlock (`INV-ARCH-PIPE`)

| ID | Rule |
| --- | --- |
| INV-ARCH-PIPE-01 | Every operation entering a governed platform path MUST pass through the five-layer verification pipeline (L1 Doctrine, L2 Consensus, L3 Notary, L4 Warden, L5 Actuator). Universal checks (hash integrity, replay nonce, expiry, state root, payload decode) and posture-required proofs fail closed; optional L2/L3 proofs are verified and recorded as audit evidence without gating execution. |
| INV-ARCH-PIPE-02 | The platform MUST enforce one of four immutable governance postures (`doctrine`, `consensus`, `ratify`, `notary`) selected at Gateway startup via `--posture`. The executing Operator's L4 Warden treats the posture carried in the envelope as authoritative. |
| INV-ARCH-PIPE-03 | The L5 Actuator is the sole execution boundary. It MUST sign an `EXECUTING` receipt, append to the commitment chain when SQL audit is active, rehydrate scrubbed secrets at the execution site, mint a short-lived capability bound to the transaction hash, dispatch the action, dissolve the capability, and persist a final `COMPLETED` or `FAILED` receipt. |

### Node Topology and Operating Modes (`INV-ARCH-NODE`)

| ID | Rule |
| --- | --- |
| INV-ARCH-NODE-01 | The Gateway (PDP) and Operator (PEP) MUST be instantiated from the single static `g8e` Go binary (`g8e gw start` and `g8e operator start`). The Gateway coordinates ingress, PKI, and L1-L3 deliberation; the Operator maintains sovereign execution and local audit evidence. |
| INV-ARCH-NODE-02 | Outbound Operators MUST establish outbound-only mTLS connections to the Gateway and subscribe to exact session-specific command channels (`cmd:<operator_id>:<operator_session_id>`). The Gateway MUST NOT expose or require inbound management listeners into remote Operator environments. |
| INV-ARCH-NODE-03 | Multiple Operator instances running on the same host MUST be disambiguated by a composite SHA-256 system fingerprint combining host properties with `local_dir`, `account`, `port`, and `operator_role` (`inference`, `provenance`, `observer`, `data`). Witness operators (`provenance`, `observer`) MUST hard-reject arbitrary command execution. |
| INV-ARCH-NODE-04 | Every read of an Operator document MUST first move a remote Operator in `active` status with no heartbeat for more than 60 seconds (`constants.OperatorHeartbeatStaleAfter`) to `stale` and persist that transition. The check lives in the Gateway document store, so no reader can observe a silent Operator as `active`. A heartbeat restores `stale` to `active`; it never revives `stopped` or `terminated`. The embedded Operator is exempt. `g8e operator start` MUST reject a `--heartbeat-interval` above half that window so a healthy Operator can miss one beat. |
| INV-ARCH-NODE-05 | Enrolling an Operator whose `system_fingerprint` matches a non-terminated remote Operator the same owner already holds MUST terminate the earlier Operator document and deactivate its Operator session as part of the governed issuance. One Operator identity MUST NOT hold two live leases. |

### Ingress and Client Boundary (`INV-ARCH-INGR`)

| ID | Rule |
| --- | --- |
| INV-ARCH-INGR-01 | Client-native tools, direct shell execution, MCP servers a client reaches directly, and side channels remain outside the governance boundary. Tools are governed ONLY when invoked through a g8e ingress (Gateway MCP, A2A, Governed HTTP dispatch, CLI `operator run`, or direct envelope) and executed via L5 Actuator. |
| INV-ARCH-INGR-02 | Direct envelope submission (`POST /api/v1/governance/envelopes`) and governed HTTP dispatch (`POST /api/v1/operators/commands`) MUST NOT synthesize missing L2 votes or L3 proofs; transactions missing posture-required evidence MUST fail closed at L4 verification. |

### Identity and Trust Boundaries (`INV-ARCH-TRUST`)

| ID | Rule |
| --- | --- |
| INV-ARCH-TRUST-01 | All internal network transport MUST enforce TLS 1.3 with mutual TLS (mTLS) for protected routes and pub/sub channels. Certificates MUST use ECDSA P-256 and carry SPIFFE URI SANs under the `g8e.local` trust domain. |
| INV-ARCH-TRUST-02 | The Gateway PKI hierarchy MUST isolate the Hub Intermediate CA (signing the Gateway serving certificate) from the Operator Intermediate CA (signing workload leaves). Workload leaf certificates MUST NOT exceed 7 days validity. Certificate revocation MUST be verified per request against local CRL storage. |

### Storage and Sovereign Audit (`INV-ARCH-STOR`)

| ID | Rule |
| --- | --- |
| INV-ARCH-STOR-01 | Under the Local-First Audit Architecture (LFAA), each executing runtime (Gateway and remote Operators) is authoritative for its sovereign SQLite audit database, execution vault, and local state root. Gateway receipt copies mirrored over pub/sub are best-effort projections. |
| INV-ARCH-STOR-02 | Sensitive command results, diffs, and governed file contents MUST be encrypted at rest using AES-256-GCM via a three-tier vault key hierarchy (master private key, HKDF-derived KEK, and wrapped DEK). |

## Owned surfaces

| Claim | Path | Verify |
| --- | --- | --- |
| Unified CLI entrypoint and commands | `cmd/g8e/main.go`, `internal/cli/cmd/` | `make build && ./g8e --help` |
| Gateway daemon and lifecycle | `internal/cli/cmd/gw/`, `internal/services/gateway/` | Unit and integration tests in `internal/services/gateway/` |
| Operator daemon and lifecycle | `internal/cli/cmd/operator/`, `internal/services/operator/` | Unit and integration tests in `internal/services/operator/` |
| Five-layer governance pipeline | `internal/services/governance/` | Unit and integration tests in `internal/services/governance/` |
| Machine consensus and deliberation | `internal/services/consensus/`, `internal/services/governance/l2_consensus.go` | Unit and integration tests in `internal/services/consensus/` |
| MCP server and native tool registry | `internal/services/mcp/` | `internal/services/mcp/native_tool_registry_test.go` |
| Network, PKI, and SPIFFE identity | `internal/services/network/`, `protocol/workload_identity.go` | Unit and integration tests in `internal/services/network/` |
| Cryptographic vault and keystore | `internal/services/vault/`, `internal/services/keystore/` | Unit and integration tests in `internal/services/vault/` |
| Sovereign audit and SQLite storage | `internal/services/storage/`, `internal/services/audit/` | Unit and integration tests in `internal/services/storage/` |
| SSE event bridge and pub/sub broker | `internal/services/sse/`, `internal/services/pubsub/` | Unit and integration tests in `internal/services/sse/` |
| Agentic Ensemble service (g8ee) | `ensemble/` | `cd ensemble && pytest` |
| Dashboard static host and SPA (g8ed) | `dashboard/` | `cd dashboard && npm test` |
| Protocol definitions and bindings | `protocol/proto/g8e/`, `protocol/` | `make proto` and `make validate-doctrines` |
| Compliance catalogs and KSI verification | `protocol/compliance/`, `internal/cli/cmd/compliance/` | `g8e compliance demo-run verify` |

## Procedures

### What g8e Is: Zero-Trust Governance and Execution Platform

g8e is a zero-trust governance and execution platform designed for autonomous AI agents, human operators, and distributed target runtimes. Traditional AI agent frameworks rely on client-side sandboxes, unverified tool interfaces, or permissive external wrappers that execute raw commands without cryptographic accountability. g8e inserts a deterministic, cryptographic, five-layer governance interlock between agent intent and runtime execution.

When a governed client submits intent, the platform translates it into or admits a canonical protobuf `GovernanceEnvelope`. The envelope traverses five governance layers before any mutation occurs. Universal checks (hash integrity, replay nonce reservation, expiration, state Merkle root, payload decode) and posture-required proofs fail closed throughout the pipeline: any missing proof or rule violation rejects the transaction. Client-native tools, MCP servers a client reaches directly, and direct host side channels remain outside this governance boundary. Each executing runtime maintains sovereign authority over its local execution evidence, while the Gateway coordinates platform-wide ingress, PKI, and deliberation.

### Core Binaries, Operating Modes, and First-Party Services

The platform ships as a polyglot monorepo centered on a single Go binary, complemented by first-party Python and Node.js services:

1. **Governance Gateway (`g8e gw start`)**: The central Policy Decision Point (PDP) and protocol coordinator. The Gateway admits client transactions, manages PKI, runs L1 Doctrine screening, coordinates L2 Consensus deliberation, manages L3 Notary approval suspensions, brokers outbound-only pub/sub WebSocket channels to Operators, and persists Gateway-local coordination state. For actions targeting the Gateway runtime itself, the Gateway executes through an in-process embedded Operator substrate (`internal/services/gateway/embedded/`). See [Gateway Architecture](gateway.md).
2. **Governed Operator (`g8e operator start`)**: The Policy Execution Point (PEP) for target runtimes. The Operator exposes no inbound management listeners, initiates an outbound-only TLS 1.3 mTLS connection to the Gateway, pulls work exclusively from an exact session-specific command channel, verifies every transaction independently through the L4 Warden, and executes accepted actions through the L5 Actuator in its local runtime. See [Operator Architecture](operator.md).
3. **Agentic Ensemble (`g8ee`)**: The optional first-party agentic ensemble runtime (`ensemble/`). Implemented in Python 3.12 with FastAPI, `g8ee` connects to the Gateway over mTLS with an enrolled app workload identity (`spiffe://g8e.local/app/g8ee`), dispatches governed host commands via the Gateway Operator command-relay endpoint, writes protected application records via the governance endpoint, and publishes real-time progress via the SSE event bridge. Its model reasoning, prompt templating, Tribunal deliberation, application approvals, and investigation memory remain outside protocol authorization. See [Ensemble Architecture](ensemble.md).
4. **Dashboard (`g8ed`)**: The first-party operator interface (`dashboard/`). A framework-free vanilla JavaScript SPA hosted by a minimal Node.js 22 / Express 5 static server on port 3000. The browser authenticates via WebAuthn passkeys and dials the Gateway HTTPS listener directly; the static host container enrolls an owner-approved workload identity that is not used for outbound browser requests. See [Dashboard Architecture](dashboard.md).
5. **Evaluation Explorer**: A specialized web frontend served on port 5173 (`EvalExplorerDefaultPort`) for real-time inspection, progress tracking, model response rendering, and verification of model evaluation campaigns. See [Evaluations](evals.md).

### The Five-Layer Governance Pipeline

Every transaction entering a governed platform path traverses five distinct verification and execution layers:

| Layer | Owner | Responsibility |
| --- | --- | --- |
| **L1 Doctrine** | Gateway and Operator | Forbidden pattern matching, MITRE ATT&CK heuristics, and command safety analysis. Detects reverse shells, privilege escalation, destructive disk operations, and credential theft. Evaluated on admission by the Gateway and independently re-evaluated locally by the executing Operator. |
| **L2 Consensus** | Gateway and Operator | K-of-N multi-signature Ed25519 authorization from enrolled member keys. Member deliberators evaluate the payload deterministically against L1 Doctrine and sign `<transaction_hash>\|<decision>`. Quorum requires distinct, valid affirmative signatures under `consensus` and `notary` postures. |
| **L3 Notary** | Gateway and Operator | Human authorization for mutation actions. In Gateway mode, operations suspend for WebAuthn passkey approval. In outbound Operator mode, verification requires an Ed25519 signature over the transaction hash from an approved suspended transaction. Audited under `doctrine` and `consensus`; read-only actions do not require L3. |
| **L4 Warden** | Executing Operator | Pre-dispatch verification gate. Reserves the transaction nonce in durable storage, checks expiration, decodes the typed protobuf payload, re-runs L1 Doctrine validation, recomputes the transaction hash and compares it against both `transaction_hash` and envelope `id`, verifies the state Merkle root against the Gateway root, and evaluates posture-required L2 and L3 proofs. Generates deterministic stage evidence. |
| **L5 Actuator** | Executing Operator | Singular execution boundary. Signs and persists an `EXECUTING` receipt, appends a signed commitment to the SQL ledger, rehydrates sensitive payload tokens via local vault keys, mints a short-lived capability bound to the transaction hash, executes the typed action handler, dissolves the capability, captures final state, signs a final `COMPLETED` or `FAILED` receipt with persistence attestation, and best-effort mirrors the receipt to the Gateway. |

### Governance Postures and Enforcement Matrix

The platform operates under one of four configurable governance postures. The Gateway selects its posture at startup via `--posture <doctrine|consensus|ratify|notary>` and cannot change it at runtime. The posture is recorded in each `GovernanceEnvelope` and is authoritative at the Operator's L4 Warden:

| Posture | L1 Doctrine | L2 Consensus | L3 Notary | Typical Use |
| --- | --- | --- | --- | --- |
| **Doctrine** (default) | Enforced | Audited | Audited | Local development, CI, and automated unit testing |
| **Consensus** | Enforced | Enforced | Audited | Multi-agent autonomous workflows requiring peer review |
| **Ratify** | Enforced | Audited | Enforced (mutations only) | Human-authorized workflows without multi-agent consensus |
| **Notary** | Enforced | Enforced | Enforced (mutations only) | Production environments requiring both multi-agent and human authorization |

Fail-closed universal gates enforced across every posture: L1 Doctrine evaluation, transaction hash integrity (`transaction_hash == envelope.id == recomputed_hash`), nonce replay reservation, transaction expiry, state Merkle root validation, action type validation, and payload decoding. See [Governance](governance.md) for full interlock specifications.

### From Intent to Governed Execution

For a request originating from an AI agent or operator, the end-to-end execution path flows through the following stages:

```
[ AI Client / Operator ]
           │
           │ (MCP / A2A / HTTP Dispatch / Direct Envelope)
           ▼
┌────────────────────────────────────────────────────────┐
│                   Governance Gateway                   │
│                                                        │
│  1. Ingress Authentication & Session Binding           │
│  2. Construct/Verify Canonical GovernanceEnvelope      │
│  3. L1 Doctrine Screening                              │
│  4. L2 Consensus Deliberation (if required)            │
│  5. L3 Notary Suspension & WebAuthn Approval (if req.) │
│  6. Bind Envelope to Target Operator Session           │
└──────────────────────────┬─────────────────────────────┘
                           │
                           │ Outbound-only mTLS Pub/Sub
                           │ Channel: cmd:<operator_id>:<session_id>
                           ▼
┌────────────────────────────────────────────────────────┐
│                   Governed Operator                    │
│                                                        │
│  L4 Warden:                                            │
│    - Nonce tracking & durable replay reservation       │
│    - Expiry & timestamp verification                   │
│    - Payload decoding & local L1 Doctrine validation   │
│    - Transaction hash recomputation & match check      │
│    - Gateway state Merkle root verification            │
│    - Posture-gated L2 vote & L3 proof verification     │
│                                                        │
│  L5 Actuator:                                          │
│    - Sign & persist EXECUTING receipt                  │
│    - Append signed commitment to SQL ledger            │
│    - Rehydrate scrubbed sensitive data via local vault │
│    - Mint short-lived JIT capability bound to tx hash  │
│    - Dispatch typed action handler                     │
│    - Dissolve capability & capture state root          │
│    - Sign & persist final COMPLETED/FAILED receipt     │
│    - Mirror receipt to Gateway (best-effort)           │
└────────────────────────────────────────────────────────┘
```

1. **Client Ingress**: An AI client invokes a tool via MCP (`POST /mcp` or stdio), A2A (`POST /a2a`), or an enrolled app submits a command to `POST /api/v1/operators/commands`.
2. **Envelope Construction**: The Gateway constructs a canonical `GovernanceEnvelope` binding target operator session, action type, typed protobuf payload, nonce, expiration, identity, and current Gateway state root.
3. **L1-L3 PDP Coordination**: The Gateway screens L1 Doctrine. Under `consensus` or `notary`, it routes the envelope to its configured consensus deliberator for L2 Ed25519 signatures. Under `ratify` or `notary`, if L3 proof is missing for a mutation, supported flows suspend the transaction and await human WebAuthn approval.
4. **Session-Bound Pub/Sub**: The Gateway publishes the verified envelope to the exact session channel `cmd:<operator_id>:<operator_session_id>`. No broadcast occurs.
5. **L4 Operator Verification**: The bound Operator pulls the envelope and executes the five-stage L4 Warden check. If any check fails, the transaction is rejected, the nonce is released, and a failed receipt is recorded locally.
6. **L5 Actuator Execution**: The L5 Actuator records initial execution evidence, rehydrates tokens, mints a scoped capability, executes the action, dissolves the capability, and writes final receipts and commitments to local storage.
7. **Result Delivery**: The Operator publishes the receipt back on `receipts:<operator_id>:<operator_session_id>`. The Gateway mirrors the receipt and returns results to the calling client via HTTP, MCP response, or the SSE event bridge.

### AI Client Surface and Protocol Ingress

g8e provides five distinct ingress paths into the governance pipeline:

1. **Model Context Protocol (MCP)**: Implements standard MCP JSON-RPC over HTTP (`POST /mcp`) and stdio (`g8e mcp serve`). Standard MCP clients (Claude Code, Codex, Goose, Gemini CLI) discover native tools and invoke them. The Gateway translates incoming `tools/call` requests into canonical `GovernanceEnvelope` transactions.
2. **Agent-to-Agent (A2A)**: JSON-RPC endpoint (`POST /a2a`) for direct agent skill invocations. Skill requests are wrapped in envelopes and dispatched to registered downstream A2A handlers.
3. **Governed HTTP Dispatch**: Enrolled applications post registered request event types and serialized protobuf payloads to `POST /api/v1/operators/commands`. The Gateway validates the event, derives `action_type`, binds target session identity, and publishes the envelope.
4. **Direct Envelope Submission**: Authorized CLIs or operators submit pre-constructed envelopes via `POST /api/v1/governance/envelopes`. Missing posture-required L2/L3 proofs are not synthesized.
5. **CLI-Directed Command Dispatch**: Enrolled operators can be pinned via `g8e operator bind` and targeted with parallel `EXECUTE_BASH` commands via `g8e operator run`.

The reference binary compiles a native registry of 32 typed tools in `internal/services/mcp/native_tool_registry.go` spanning database triage, log filtering, system and container inspection, filesystem profiling, network and TLS probing, Git operations, shell execution, cloud/Kubernetes inspection, deployment, and audit receipt queries.

### Governed Operator Roles and Multi-Operator Coexistence

The g8e Operator is a compact binary, and multiple operator processes can execute concurrently on the exact same host system for entirely different purposes. Operators running on the same host are differentiated by four key factors: `local_dir`, `account`, `port`, and `operator_role`.

The canonical `system_fingerprint` is a SHA-256 composite hash combining immutable host properties (`os`, `arch`, `cpu_count`, `machine_id`, `hostname`) with `local_dir`, `account`, `port`, and `operator_role`. This ensures distinct, collision-free identities in the Gateway operator registry.

| Role | Activation Flags | Responsibilities | Execution Boundaries |
| --- | --- | --- | --- |
| **Data Operator** | Default (or `--data-operator-enabled`) | Governed tool execution, local shell commands, filesystem triage, and execution vault | Primary PEP for tool execution. Discovered by Gateway session service for tool and workflow dispatch. |
| **Inference Operator** | `--inference-enabled` | Governed LLM inference backend (g8ellama), model registry synchronization, and Ollama provider dispatch | Serves LLM requests; executes model pull/release/residency commands. Excluded from general tool-execution discovery. |
| **Provenance Operator** | `--provenance-operator-enabled`, `--model-storage-root` | Storage-side model provenance attestation over local model weight files | Read-only witness. Arbitrary command execution is hard-rejected by `ValidateWitnessCommand`. |
| **Observer Operator** | `--provider-boundary-observer-enabled`, `--provider-boundary-observer-id` | Read-only hardware, temperature, power, and residency observation on approved host | Read-only witness. Arbitrary command execution is hard-rejected by `ValidateWitnessCommand`. |

### Network Architecture, Port Topology, and SPIFFE Identity

The platform enforces a zero-trust network model. All protected communications require TLS 1.3 with mutual TLS (mTLS) and SPIFFE workload identities under the `g8e.local` trust domain.

#### Port Topology

| Port | Protocol | Listener | Auth Mode | Purpose |
| --- | --- | --- | --- | --- |
| **8080** | HTTP | Gateway | Public / Token | Plain HTTP discovery (`/.well-known/g8e/pki/*`), health, bootstrap, deploy scripts, and CLI recovery |
| **8443** | HTTPS | Gateway | Route-Gated (mTLS / Web Session / Dual) | Protected APIs, Console SPA, MCP, A2A, pub/sub WebSockets, and governance execution |
| **8081** | HTTP | Gateway | Loopback / Allowlist | Private ingest listener for public spectator mirror export |
| **8082** | HTTP | Public Mirror | Loopback / Anonymous | Public spectator mirror serving anonymous history, SSE, and content-addressed proofs |
| **5173** | HTTP | Eval Explorer | Loopback / Direct | Embedded Evaluation Explorer SPA for live campaign telemetry |
| **8000** | HTTP | Ensemble (g8ee) | mTLS to Gateway | First-party Python agentic ensemble |
| **3000** | HTTP | Dashboard (g8ed) | Direct to Gateway | Node.js static host serving vanilla JavaScript SPA |
| **11434** | HTTP | Ollama Provider | Remote Loopback / LAN | Remote Ollama inference provider dialed by Inference Operator |

#### PKI Hierarchy

The Gateway manages a four-tier PKI hierarchy:
- **Root CA**: Self-signed root CA (`g8e Root CA`) with 3650-day validity.
- **Hub Intermediate CA**: Signed by Root CA (3650 days). Signs only the Gateway serving certificate (`operator-gateway`, 90-day validity).
- **Operator Intermediate CA**: Signed by Root CA (3650 days). Signs all workload leaf certificates (`operator`, `cli`, `app`) with 7-day maximum validity.
- **Gateway Peer Intermediate CA**: Signed by Root CA (3650 days). Signs federated gateway peering certificates (`gateway-peer`, 90-day validity).

All certificates use ECDSA P-256 (`elliptic.P256()`). Revocation is enforced per request against local storage, with an X.509 CRL DER served at `/.well-known/g8e/pki/crl` and `/api/v1/pki/revocation-bundle`. See [Network Architecture](network.md).

#### SPIFFE Workload Identity Formats

- **Operator**: `spiffe://g8e.local/operator/<organization_id>/<operator_id>/<operator_session_id>`
- **CLI**: `spiffe://g8e.local/cli/<user_id>/<cli_session_id>`
- **App Workload**: `spiffe://g8e.local/app/<operator_id>` (Centralized Ensemble: `spiffe://g8e.local/app/g8ee`)
- **Human User**: `spiffe://g8e.local/user/<user_id>`
- **Gateway Hub**: `spiffe://g8e.local/hub/operator-listen`

### Authentication, Session Models, and Platform Enrollment

Authentication mechanisms correspond strictly to workload categories:

1. **CLI Authentication**: Mutual TLS using certificates issued through an enrollment state machine that classifies local credentials as complete, absent, partial, or corrupt. Interactive enrollment uses a browser-based WebAuthn ceremony; headless enrollment (`g8e auth enroll user --headless`) produces an mTLS-only identity requiring approval via `POST /api/v1/auth/approve-cli`.
2. **Console SPA Authentication**: Web session cookies issued after successful WebAuthn/FIDO2 passkey assertion registered during owner bootstrap.
3. **App Workload Authentication**: Mutual TLS with an app workload certificate issued through the platform enrollment flow. Workloads have identity-only access by default; L2 signing requires explicit administrative policy registration.
4. **Operator Authentication**: Mutual TLS with an operator certificate issued through owner-approved platform enrollment. Session tokens are unique per connection.

Revoking an enrollment via `POST /api/v1/auth/platform-enrollments/revoke` writes the certificate serial to the CRL, purges associated policies, and immediately terminates active pub/sub WebSockets. See [Authentication & Authorization](auth.md).

### Cryptography, Key Custody, and Per-Runtime Vault Hierarchy

g8e secures data at rest and in transit through a structured cryptographic hierarchy:

- **Per-Runtime Vault**: An AES-256-GCM encrypted vault hosted in `.g8e/vault/` encrypts sensitive audit content, command output, diffs, governed file copies, and scrubbing tokens.
- **Three-Tier Key Hierarchy**:
  1. *Master Vault Key*: An Ed25519 private key stored in `.g8e/secrets/vault.key`.
  2. *Key Encryption Key (KEK)*: Derived via HKDF-SHA256 from the master key.
  3. *Data Encryption Key (DEK)*: A 256-bit AES key wrapped by the KEK and stored in `.g8e/vault/keys/active.dek`.
- **Platform Keystore**: Stores session signing keys, CA private keys, and HMAC auditor secrets.
- **FIPS 140-3 Compliance**: The platform can link against the Go Cryptographic Module v1.0.0 (CMVP Cert #5247) when compiled with `GOFIPS140=v1.0.0`. See [Encryption Architecture](encryption.md).

### Local-First Audit Architecture (LFAA) and Storage Model

g8e enforces a Local-First Audit Architecture (LFAA). The target host where an action executes is the primary, authoritative source of truth for execution evidence, command history, and file mutations.

- **Primary Database (`g8e.db`)**: A SQLite database stored in `.g8e/data/` manages platform collections, key-value blobs, nonce reservations, audit records, receipts, and commitment chains.
- **Specialized Persistence Services**:
  - *Audit Store*: Append-only SQL store recording session lifecycle, command executions, and signed receipts.
  - *Commitment Ledger*: Hash-chained SQLite ledger linking consecutive execution commitments to prove historical tamper resistance.
  - *Replay Store*: Durable nonce tracking to prevent replay attacks across restarts.
  - *Suspended Transaction Store*: Envelopes suspended awaiting L3 human passkey authorization.
  - *Git-Backed File Ledger*: Optional Git version control tracking every governed file mutation, exposing the repository HEAD commit as a verifiable state snapshot.

Gateway copies of receipts mirrored over pub/sub are best-effort projections; the remote Operator's local persisted evidence is authoritative. See [Storage Architecture](storage.md).

### SSE Event Streaming Infrastructure

The Gateway operates a Server-Sent Events (SSE) streaming infrastructure for real-time telemetry:
- **Event Ingestion**: Enrolled app workloads push typed events via `POST /api/v1/sse/push` using mTLS with app workload identity (`spiffe://g8e.local/app/g8ee`).
- **Client Consumption**: Clients poll historical events via `GET /api/v1/sse/events` and stream live events via `GET /api/v1/sse/stream` under dual authentication (mTLS for CLI/Operator, web session cookie for browsers).
- **Platform Telemetry**: The Gateway produces internal SSE events for passkey registration and L3 notary approval challenges. SSE is delivery telemetry, not durable governance state or authorization. See [SSE Streaming](sse.md).

### Observe Projections, Evaluation Explorer, and Public Spectator Mirror

The platform provides layered observation capabilities separated by strict trust boundaries:

1. **Private Observe Projections**: Authenticated Gateway routes (`/api/v1/observe/...`) serving structured system status, active operators, and transaction history to authorized browser sessions and mTLS clients.
2. **Evaluation Explorer**: Served on loopback port 5173, the Evaluation Explorer displays live model evaluation progress, run boundaries, model responses, failure diagnoses, and hardware telemetry.
3. **Public Spectator Mirror**: An isolated, read-only mirror service. The Gateway exports an allowlisted, signed projection over loopback port 8081 (`PublicSpectatorPrivatePort`). The mirror serves anonymous public traffic on loopback port 8082 (`PublicSpectatorPublicPort`) or via Cloudflare Tunnel. Public clients cannot connect to the private Gateway, and the mirror cannot authorize or execute mutations. See [Public Spectator Architecture](public_spectator.md).

### Protocol Wire Contracts and Code Generation

The g8e Protocol Library (`protocol/`) defines canonical wire contracts, schemas, and models:
- **Protobuf Schemas**: Defined in `protocol/proto/g8e/` and generated via `make proto` using `buf`. Generated Go, Python, and TypeScript bindings provide typed message structures.
- **Constants Registries**: JSON registries in `protocol/constants/` serve as single sources of truth for doctrines, COSAiS overlays, ports, and errors, validated via `make validate-doctrines` and `make validate-cosais`.
- **JSON Model Schemas**: Canonical schemas in `protocol/models/` and `protocol/schemas/` define structures for consensus policies, audit events, and compliance records.
- **Canonical Serialization**: Transactions use canonical protojson serialization for deterministic hashing and cryptographic signatures. See [Protocol Library](protocol.md).

### Compliance Evidence and KSI Verification Foundation

The platform incorporates a protocol-owned compliance evidence foundation:
- **Catalog Infrastructure**: Canonical assertion, framework, crosswalk, and demo-scenario catalogs are digest-verified against FedRAMP 20x Key Security Indicators (KSI).
- **Demo Run Evidence**: Compliance demo runs persist manifests, scenario definitions, receipts, and metric evidence under `.g8e/data/compliance/demo-evidence/<run-id>/`.
- **Deterministic Verification**: The read-only verification command `g8e compliance demo-run verify <run-id>` verifies manifests, SHA-256 provenance hashes, content-addressed artifacts, protocol signatures, and directory integrity, exiting nonzero on any discrepancy.

### Docker Compose Unified Stack and Deployment Topologies

The root `docker-compose.yml` launches the complete platform in the default profile (`docker compose up -d`):
- `g8e-gateway`: Gateway PDP on ports 8080, 8443, 8081, 8082, 5173.
- `g8e-data-operator`: Governed Data Operator running in worker mode (container hostname `data-operator`).
- `g8e-inference-operator`: Governed Inference Operator connecting to remote Ollama.
- `g8e-ensemble`: First-party Python agentic ensemble on port 8000.
- `g8e-dashboard`: Node.js static host and browser frontend on port 3000.

**Binary Precedence Rule**: Host binaries mounted at `./bin:/opt/g8e/bin:ro` take precedence over image baked-in binaries (`/g8e`). Running `make build` and restarting containers immediately updates all services without rebuilding container images. See [Unified Docker Stack Guide](../guides/unified_stack.md).

## Anti-patterns

- **Bypassing the five-layer pipeline for client-native tools**: Assuming client-native tools or MCP servers a client reaches directly provide governance without crossing a g8e ingress (INV-ARCH-INGR-01).
- **Changing governance posture at runtime**: Attempting to weaken or alter `--posture` after Gateway startup without a process restart (INV-ARCH-PIPE-02).
- **Synthesizing missing L2/L3 proofs on direct paths**: Allowing direct envelope submission or governed HTTP dispatch to fabricate consensus votes or notary approvals (INV-ARCH-INGR-02).
- **Treating the Gateway receipt mirror as authoritative execution truth**: Relying on Gateway receipt mirrors rather than the executing Operator's sovereign local audit store (INV-ARCH-STOR-01).
- **Opening inbound management listeners on Operators**: Exposing inbound network ports on remote Operators rather than maintaining outbound-only mTLS pub/sub connections (INV-ARCH-NODE-02).
- **Dispatching mutating commands to witness operators**: Attempting to route general tool execution to Provenance or Observer operators rather than Data operators (INV-ARCH-NODE-03).
- **Connecting public browsers to the private Gateway**: Routing public spectator traffic to port 8443 rather than through the isolated public mirror on port 8082 (INV-ARCH-TRUST-01).
- **Hard-wrapping documentation prose lines or embedding code line numbers**: Violating documentation standards during architecture updates (INV-DOC-STYLE-01, INV-DOC-STYLE-02).

## Links out

- [Governance Architecture](governance.md): Detailed five-layer interlock sequence, envelope structure, and posture semantics.
- [Gateway Architecture](gateway.md): Gateway operating modes, route mapping, session types, and embedded Operator substrate.
- [Operator Architecture](operator.md): Operator execution boundary, native tool catalog, and local audit vault.
- [Consensus Architecture](consensus.md): L2 machine consensus, deliberation mechanics, and signature verification.
- [Authentication & Authorization](auth.md): CLI enrollment, recovery, rotation, and WebAuthn notary approvals.
- [Network Architecture](network.md): PKI hierarchy, TLS 1.3/mTLS transport, SPIFFE identity, and port topology.
- [Encryption Architecture](encryption.md): Vault lifecycle, three-tier key hierarchy, and FIPS compliance.
- [Storage Architecture](storage.md): Audit store, commitment ledger, replay store, and SQLite persistence.
- [SSE Streaming](sse.md): Real-time event streaming and consumer routing.
- [Evaluations](evals.md): Model evaluation framework, campaign scoring, and operator roles.
- [Model Provenance](model-provenance.md): Model weight attestation and chain-of-custody verification.
- [Public Spectator Architecture](public_spectator.md): Read-only spectator mirror and threat model.
- [Ensemble Architecture](ensemble.md): First-party Python agentic ensemble runtime and tool loops.
- [Dashboard Architecture](dashboard.md): Browser static host and SPA identity boundaries.
- [Protocol Library](protocol.md): Protobuf definitions, JSON registries, and code generation.
- [Scripts Reference](scripts.md): Bootstrap, smoke test, deployment, and demo scripts.
- [Documentation Guide](../devs/docs.md): Invariant definitions, metadata specifications, and audit workflows.
- [Developer Guidelines](../devs/devs.md): Repository coding standards and invariant rules.
- [Unified Docker Stack Guide](../guides/unified_stack.md): Container compose setup and campaign execution.
- [Compliance Alignment Reference](../reference/compliance-alignment.md): KSI evaluation framework and control crosswalks.
