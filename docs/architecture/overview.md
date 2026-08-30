---
title: Platform Architecture Overview
parent: Architecture
---

# Platform Architecture Overview

Last Updated: 2026-08-30
Version: v2.1.0

## What g8e Is

g8e is a zero-trust execution platform that sits between AI agents, human operators, and target hosts. An AI agent never mutates a host directly. The agent formulates intent, and the platform translates that intent into a typed, signed, verifiable `GovernanceEnvelope` that passes through five fail-closed governance layers before any action runs. Every AI client is treated as an untrusted principal; every host is sovereign over its own audit ledger and state root.

The platform ships as a polyglot monorepo. The gateway and operator are a single static Go binary that runs in two modes:

- **Governance Gateway (Policy Decision Point / PDP)**: The central coordinator that admits transactions, manages PKI, enforces L1 through L3 governance, brokers pub/sub channels to operators, and acts as the platform's persistence layer, root CA, and audit authority. The gateway runs an in-process Operator substrate (L4 Warden and L5 Actuator) for operations targeting the gateway host itself.
- **Governed Operator (Policy Execution Point / PEP)**: The same binary run in operator mode on target hosts. It requires no installation, opens no inbound ports, initiates an outbound-only mTLS tunnel to the gateway, pulls work from a unique pub/sub channel, re-verifies every proof locally, and is the only component authorized to mutate the host.

Two first-party components ship alongside the Go binary and complete the platform:

- **Ensemble (g8ee)**: The first g8e-compatible agentic ensemble. A Python 3.12 / FastAPI service that connects to the gateway over mTLS, submits governed intents through the MCP surface, and streams progress and results back through the SSE event bridge. See [Ensemble (g8ee)](./ensemble.md).
- **Dashboard (g8ed)**: The operator dashboard. A Node.js 22 / Express web application that provides the operator-facing UI for driving ensembles, managing operators, inspecting audit trails, and configuring the platform. See [Dashboard (g8ed)](./dashboard.md).

`docker compose up` from the repo root brings up the whole stack end to end: gateway, operator, ensemble, and dashboard. See the [Unified Docker Stack guide](../guides/unified_stack.md).

For the full service stacks, see [Gateway Architecture](./gateway.md) and [Operator Architecture](./operator.md). For a visual map of the system, see the [50k system diagram](../diagrams/graph-system-50k.md) and the [system overview flowchart](../diagrams/flowchart-system-overview-lr.md).

---

## The Five-Layer Governance Pipeline

Every agent-originated action passes through the same five fail-closed layers. The gateway owns L1-L3 as policy decisions; the operator owns L4-L5 as execution gates.

| Layer | Owner | Responsibility |
| --- | --- | --- |
| **L1 Doctrine** | Gateway | Forbidden pattern matching and MITRE ATT&CK heuristics detect reverse shells, privilege escalation, and destructive disk operations. Enforced in every posture. The operator re-runs L1 validation locally before execution. |
| **L2 Consensus** | Gateway | Multi-signature Ed25519 votes over `<transaction_hash>\|<decision>` from an enrolled body of members. The reference implementation signs deterministic L1-doctrine evaluations. A quorum of distinct, valid affirmative signatures is required under the `consensus` and `notary` postures. |
| **L3 Notary** | Gateway | Human-in-the-loop authorization. In gateway mode the proof is a WebAuthn/FIDO2 passkey assertion over the transaction hash; CLI callers additionally bind to an mTLS session. In outbound operator mode the proof is a suspended-transaction approval and Ed25519 signature. Mutations are blocked until a valid proof is presented; read-only actions do not require L3. |
| **L4 Warden** | Operator | Final pre-dispatch gate. Recomputes and compares the transaction hash, reserves the nonce, checks expiry, validates the state Merkle root, and verifies L2 and L3 proofs. Any mismatch or missing proof fails closed. |
| **L5 Actuator** | Operator | Singular execution boundary. Signs an `EXECUTING` receipt, rehydrates scrubbed sensitive data at the execution site using local vault keys, mints a just-in-time capability bound to the transaction hash, dispatches the action, dissolves the capability, and signs a final `COMPLETED` or `FAILED` receipt. |

The pipeline is fail-closed at every layer: a failed check rejects the transaction and releases its nonce reservation. For the full interlock sequence, posture configurations, and the canonical `GovernanceEnvelope` container, see [Governance](./governance.md). For L2 enrollment, deliberation, and member key management, see [Consensus](./consensus.md). For the L3 notary modes and out-of-band approval flow, see [Authentication & Authorization](./auth.md).

---

## Governance Postures

A configurable **GovernancePosture** determines which layers are enforced as fail-closed gates versus audited only. The posture is set at startup via `--posture <doctrine|consensus|notary>` and cannot be changed at runtime. The gateway boots regardless of posture; layer enforcement happens at transaction time.

| Posture | L1 Doctrine | L2 Consensus | L3 Notary | Typical Use |
| --- | --- | --- | --- | --- |
| **Doctrine** (default) | Enforced | Audited | Audited | Local development and CI |
| **Consensus** | Enforced | Enforced | Audited | Automated workflows with multi-agent review |
| **Notary** | Enforced | Enforced | Enforced (mutations only) | Production with human authorization |

The following checks are enforced as fail-closed gates in every posture: L1 Doctrine validation, transaction hash integrity, nonce replay protection, expiry enforcement, state Merkle root validation, action type validation, and payload decoding. See [Governance](./governance.md) for the posture selection semantics and per-layer enforcement matrix.

---

## From Intent to Execution

The practical flow for an AI client is:

1. The AI client submits an intent through the MCP or A2A endpoint.
2. The gateway translates the intent into a canonical `GovernanceEnvelope` carrying the typed payload, identity, nonce, expiry, and state root.
3. Under `consensus` or `notary` posture, the gateway sends the envelope to the enrolled consensus for L2 votes.
4. If L3 is required and missing, the gateway suspends the transaction and sends an approval challenge to the human.
5. After L1-L3 pass, the gateway publishes the envelope to the unique pub/sub channel for the bound operator.
6. The bound operator pulls the envelope, the L4 Warden re-verifies L1-L4, and the L5 Actuator executes the action.
7. The operator writes the signed receipt to the local audit vault and publishes it back to the gateway.
8. The gateway returns the receipt to the AI client through the original MCP/A2A response or SSE channel.

Only the operator bound to the envelope receives the work. The gateway binds the envelope to the authenticated operator session and publishes to that operator's unique command channel. No broadcast occurs. For the visual sequence, see the [principal-ensemble-gateway-operator sequence diagram](../diagrams/sequence-principal-ensemble-gateway-operator-v3.md). For the agent-facing surface, see [AI Agents and the g8e Governance Boundary](./agents.md).

---

## AI Client Surface

g8e exposes two standard protocols for AI clients:

- **MCP (Model Context Protocol)**: A unified JSON-RPC endpoint that lets standard MCP clients such as Claude Code, Codex, Goose, or Gemini CLI discover tools, call them, and receive typed results. The gateway translates each MCP `tools/call` into a canonical `GovernanceEnvelope` and routes it through the governance pipeline.
- **A2A (Agent-to-Agent)**: A JSON-RPC endpoint for direct A2A skill invocations. The gateway wraps the skill request in an envelope and either executes it through a governed operator or forwards it to a configured downstream A2A server, depending on posture and authorization.

The governed operator ships with native, memory-safe tools that agents invoke through the MCP surface: filesystem reads, shell command execution, database triage, log filtering, process inspection, network probes, cloud metadata lookup, Git state, and Kubernetes inspection. Each native tool accepts a typed request, performs read-only or governed-mutation operations, and returns a scrubbed result. See [AI Agents and the g8e Governance Boundary](./agents.md) for the client surface and [Operator Architecture](./operator.md) for the complete native tool playbook.

---

## Network and Identity

The platform uses a zero-trust networking model where all communication is authenticated via mutual TLS (mTLS) with verified SPIFFE workload identities. The platform uses `g8e.local` as the SPIFFE trust domain and as the TLS ServerName for connections that resolve the gateway by IP.

The gateway operates a four-tier PKI hierarchy: a self-signed Root CA signs a Hub Intermediate CA (which signs the gateway serving certificate), an Operator Intermediate CA (which signs operator, CLI, and app leaf certificates), and a Gateway Peer Intermediate CA (which signs gateway peer certificates for multi-host deployments). All certificates use ECDSA P-256 and carry SPIFFE URI SANs under the `g8e.local` trust domain. Certificate revocation is enforced per-request during mTLS verification, with a standard X.509 CRL served at `/.well-known/g8e/pki/crl`.

The gateway exposes two ports: a plain HTTP port for unauthenticated discovery and bootstrap (health and state checks, CA bundle and fingerprint discovery, bootstrap, CLI recovery and platform enrollment request/status/complete, deploy scripts, and node binary distribution), and an HTTPS port enforcing TLS 1.3 that serves the Console, browser WebAuthn endpoints, CA bundle and CRL endpoints, and all governed execution routes. The HTTPS port accepts optional client certificates at the transport layer; authentication middleware classifies routes by auth mode and enforces application-layer mTLS verification for governed operator API, CSR signing, and MCP/A2A routes. Operators initiate outbound-only mTLS tunnels to the gateway and pull work from unique pub/sub channels; the gateway never reaches into operators. See [Network Architecture](./network.md) for the full PKI hierarchy, port topology, SPIFFE identity formats, and enrollment procedures.

---

## Authentication and Authorization

The platform security model is built on two core principles: identity-bound communication via mTLS, and the five-layer verification sequence. Authentication methods vary by surface:

- **CLI**: mTLS certificates issued through an enrollment state machine that classifies local identity as complete, absent, partial, or corrupt and routes to reuse, bootstrap, recovery, or rotation accordingly. The passkey ceremony runs through a browser; the `--headless` flag produces an mTLS-only identity that skips the browser.
- **Console SPA**: Web session cookie authenticated by a WebAuthn/FIDO2 passkey registered during enrollment.
- **AI Agent / App**: mTLS with app workload identity, enrolled via the PKI API. Identity-only by default; L2 signer capability requires explicit admin registration.
- **Operator**: mTLS with operator workload identity bound to an organization, operator, and session.

Enrollment uses a one-time token flow so raw session identifiers never appear in browser history or referrer headers. Recovery of partial or corrupt credentials requires one-time human approval through the Console SPA or, under `--headless`, through an already-enrolled CLI via the mTLS approve-cli endpoint. See [Authentication & Authorization](./auth.md) for the enrollment state machine, recovery and rotation flows, headless enrollment, and the L3 notary modes.

---

## Encryption

g8e enforces mandatory encryption for all sensitive data at rest and mTLS for all network communication. The encryption system consists of a per-host vault providing AES-256-GCM primitives, a three-tier key hierarchy (private key, HKDF-derived Key Encryption Key, and wrapped Data Encryption Key), a platform keystore backed by OS keyrings for secret storage, and the PKI hierarchy for certificate-based mTLS.

The audit store, execution vault, ledger, and encrypted key-value adapter all encrypt content through the vault when it is unlocked. The gateway refuses to start if the vault cannot be unlocked, and other components fail closed when the vault is locked rather than falling back to plaintext. The keystore encrypts session keys, Ed25519 signing keys, CA private keys, service certificate private keys, API keys, and auditor HMAC keys using a master key stored in the OS-native credential store. The platform links against the Go Cryptographic Module v1.0.0 (CMVP Cert #5247) when built with `GOFIPS140=v1.0.0` for FIPS 140-3 compliance. See [Encryption Architecture](./encryption.md) for the vault lifecycle, key hierarchy, PKI details, and FIPS compliance.

---

## Storage and Audit

The storage layer is the persistence foundation for the governance pipeline. It records every operator session, command execution, file change, governance transaction, and audit attestation so the platform can replay history, verify state, and prove what happened.

In gateway mode the canonical SQLite database `g8e.db` hosts the audit log, searchable receipt columns and complete canonical receipt JSON, the signed commitment ledger, key-value store, document store, blob store, replay nonces, and SSE event buffer. The git-backed file-mutation ledger, execution vault, and suspended transaction store use their own files. In outbound/operator mode the replay store, execution vault, and suspended transaction store run as standalone SQLite databases; the audit store, commitment ledger, and git-backed ledger remain local to the operator.

Specialized services include the audit store (append-only record of sessions, events, file mutations, and signed receipts), the ledger (git-backed version control for file modifications, with the HEAD commit exposed as a verifiable state snapshot), the execution vault (encrypted, compressed command results and file diffs), the replay store (nonce-based replay protection), the suspended transaction store (envelopes awaiting L3 approval), and the commitment ledger (chain-integrity-protected attestations). The target host remains the source of truth for command history and file mutations: this is the Local-First Audit Architecture (LFAA). See [Storage Architecture](./storage.md) for the full service inventory and runtime file I/O model.

---

## SSE Event Bridge

The gateway provides a Server-Sent Events (SSE) streaming infrastructure that enables real-time event delivery from app workloads to browser and CLI clients. g8e-compatible agentic ensembles publish typed events through `POST /api/v1/sse/push` (mTLS with app workload identity), and clients consume historical events via `GET /api/v1/sse/events` and live events via `GET /api/v1/sse/stream` (dual auth: mTLS for CLI or operator, web session cookie for browser). The gateway also produces SSE events internally for platform workflows such as passkey registration and L3 transaction approval. This lets an external ensemble surface progress, questions, and results to the human without exposing raw host data. See [SSE Streaming](./sse.md) for the push, poll, and stream semantics.

---

## Protocol Library

The g8e Protocol Library is the canonical wire contract for all mutations in the platform. It provides schema definitions, JSON constant registries, JSON model schemas, Pydantic models, dynamic enum generation, and SPIFFE workload identity helpers for building compatible clients and services. The protocol publishes as two independent packages sharing a single unified version number with the platform binary: a Go module under the root module path and a Python package. There are no separate protocol-only releases. See [Protocol Library](./protocol.md) for package structure, code generation, and release workflow.

---

## Scripts and Tooling

g8e provides platform-specific bootstrap scripts for local development, gateway-served deploy scripts for remote operator installation, smoke test scripts that verify SDK importability in clean environments, and a CI guard script that validates doctrine detector coverage of finalized COSAiS overlays. The `g8e demos` CLI supports air-gapped image pull, export, import, and listing for demo environments. See [Scripts](./scripts.md) for the full script inventory.

---

## Key Design Principles

- **Do not trust the AI client.** The agent provides intent; the platform verifies and executes. The client has no privileged channel.
- **Do not trust the consensus layer.** Votes are verified against trusted public keys and the transaction hash. A missing or invalid signature fails closed.
- **Do not trust the gateway.** The operator re-derives every proof locally before execution.
- **Multi-signature consensus.** L2 requires K-of-N Ed25519 affirmative votes from distinct members. The reference implementation signs deterministic L1-doctrine evaluations.
- **Doctrine is enforced, not suggested.** Agents can be informed of doctrine, but the L1 gate rejects forbidden actions regardless of compliance.
- **Scope is explicit.** The envelope is bound to the authenticated operator session, and only that operator's command channel receives the dispatched work.
- **Sovereign hosts.** Every operator is authoritative for its own audit ledger and state root. The gateway never reaches into operators.
- **Fail-closed by default.** Encryption is mandatory, missing proofs reject transactions, and no component silently falls back to plaintext or weaker enforcement.

---

## Architecture Documentation Index

| Document | Scope |
| --- | --- |
| [AI Agents and the g8e Governance Boundary](./agents.md) | AI client surface (MCP, A2A), native tool playbook, intent-to-execution flow, security boundaries summary. |
| [Gateway Architecture](./gateway.md) | Gateway service stack, operating modes, port topology, MCP/A2A endpoints, pub/sub brokering, in-process Operator substrate. |
| [Operator Architecture](./operator.md) | Operator execution boundary, L4 Warden and L5 Actuator, native tool playbook, local audit vault. |
| [Ensemble (g8ee)](./ensemble.md) | First-party agentic ensemble: role, connection model, in-tree protocol dependency, build and test. |
| [Dashboard (g8ed)](./dashboard.md) | Operator dashboard: role, connection model, build and test. |
| [Governance](./governance.md) | Five-layer interlock sequence, GovernanceEnvelope structure, posture configurations, transaction flow. |
| [Consensus](./consensus.md) | L2 consensus policy, declarative bootstrap, member key management, deliberation, L4 vote verification. |
| [Authentication & Authorization](./auth.md) | CLI enrollment state machine, recovery and rotation, headless enrollment, WebAuthn notary, session binding. |
| [Network Architecture](./network.md) | PKI hierarchy, SPIFFE workload identity, mTLS enforcement, port topology, enrollment procedures. |
| [Encryption Architecture](./encryption.md) | Vault lifecycle, three-tier key hierarchy, platform keystore, TLS and mTLS, FIPS 140-3 compliance. |
| [Storage Architecture](./storage.md) | Audit store, ledger, execution vault, replay store, suspended transaction store, commitment ledger, runtime file I/O. |
| [SSE Streaming](./sse.md) | SSE push, poll, and stream endpoints for agentic ensembles and platform workflows. |
| [Protocol Library](./protocol.md) | Go and Python protocol packages, constants registries, JSON model schemas, protobuf code generation, release workflow. |
| [Scripts](./scripts.md) | Dev bootstrap, smoke test, CI guard, remote deploy, and air-gapped demo scripts. |

For developer guidelines, test patterns, and contribution conventions, see [Developer Guidelines](../devs/devs.md). For building g8e-compatible applications and agentic ensembles, see [Build Apps](../guides/build_apps.md).
