---
title: Platform Architecture Overview
parent: Architecture
---

# Platform Architecture Overview

Last Updated: 2026-09-23
Version: v2.1.12

## What g8e Is

g8e is a zero-trust governance and execution platform that sits between AI agents, human operators, and target runtimes. A governed client supplies intent; the platform constructs or verifies a typed, canonical `GovernanceEnvelope`, and the applicable path passes through five governance layers before an action runs. Universal checks and proofs required by the active posture fail closed. Client-native tools, external MCP wrappers, and other side channels remain outside this boundary. Each executing runtime is sovereign over its local execution evidence and state, while the Gateway owns platform coordination state.

The platform ships as a polyglot monorepo. The gateway and operator are a single static Go binary that runs in two modes:

- **Governance Gateway (Policy Decision Point / PDP)**: The central coordinator that admits transactions, manages PKI, coordinates L1 through L3 governance, brokers pub/sub channels to operators, and owns Gateway-local coordination state and evidence. The Gateway runs an in-process Operator substrate (L4 Warden and L5 Actuator) for operations targeting the Gateway runtime itself.
- **Governed Operator (Policy Execution Point / PEP)**: The same binary run in operator mode on a target runtime. It requires no inbound management listener, initiates an outbound-only mTLS connection to the Gateway, pulls work from an exact session-specific pub/sub channel, re-verifies each envelope locally, and is the governed execution boundary for mutations in its own runtime.

Two first-party components ship alongside the Go binary and complete the platform:

- **Ensemble (g8ee)**: The optional first-party agentic ensemble. A Python 3.12 / FastAPI service that connects to the Gateway over mTLS, sends host commands through the exact Operator command-relay path, submits protected application records through the governance endpoint, and publishes progress and results through the SSE event bridge. Its model reasoning, Tribunal, application approvals, and memory remain outside protocol authorization. See [Ensemble (g8ee)](./ensemble.md).
- **Dashboard (g8ed)**: The first-party browser interface. A framework-free JavaScript SPA served by a minimal Node.js 22 / Express 5 static host; the browser authenticates and calls the Gateway directly, while the container independently enrolls an owner-approved workload identity that the running static host does not use for outbound requests. See [Dashboard (g8ed)](./dashboard.md) and the [Dashboard documentation](../dashboard/index.md).

`docker compose up` from the repo root starts the Gateway only. The `bootstrapped` profile adds the Data Operator, ensemble, and dashboard after owner enrollment; the `evaluation` profile adds the Inference Operator. See the [Unified Docker Stack guide](../guides/unified_stack.md).

For the full service stacks, see [Gateway Architecture](./gateway.md) and [Operator Architecture](./operator.md). For a visual map of the system, see the [50k system diagram](../diagrams/graph-system-50k.md) and the [system overview flowchart](../diagrams/flowchart-system-overview-lr.md).

---

## The Five-Layer Governance Pipeline

Every operation that enters a governed platform path reaches the five-layer interlock. Client-native tools, external MCP wrappers, discovery methods, and other side channels do not. Universal checks and posture-required proofs fail closed, while optional L2 and L3 results remain audit evidence. The Gateway owns client-facing admission and L1-L3 coordination; the executing Operator, including the embedded Gateway Operator, independently applies L4-L5 in its own runtime.

| Layer | Owner | Responsibility |
| --- | --- | --- |
| **L1 Doctrine** | Gateway and Operator | Forbidden pattern matching and MITRE ATT&CK heuristics detect reverse shells, privilege escalation, and destructive disk operations. Enforced in every posture. The Gateway screens applicable admission paths and the executing Operator re-runs L1 validation locally before execution. |
| **L2 Consensus** | Gateway and Operator | Multi-signature Ed25519 votes over `<transaction_hash>\|<decision>` from an enrolled body of members. The reference implementation signs deterministic L1-doctrine evaluations. The Gateway coordinates deliberation when configured and the Operator verifies the evidence locally. A quorum of distinct, valid affirmative signatures is required under the `consensus` and `notary` postures. |
| **L3 Notary** | Gateway and Operator | Human authorization for mutation actions under `ratify` and `notary`. Gateway-managed MCP/A2A flows can suspend for WebAuthn approval; outbound-operator verification accepts the approved suspended-transaction proof and Ed25519 signature, with CLI identity binding where applicable. Direct envelopes and governed HTTP dispatch do not manufacture or suspend for missing L3 proof. L3 is audited under `doctrine` and `consensus`; read-only actions do not require L3. |
| **L4 Warden** | Executing Operator | Final pre-dispatch gate. Recomputes and compares the transaction hash, reserves the nonce, checks expiry, validates the state Merkle root, and verifies L2 and L3 proofs. Universal mismatches and missing posture-required proofs fail closed; optional proof failures remain non-gating audit evidence. |
| **L5 Actuator** | Executing Operator | Singular execution boundary. Signs an `EXECUTING` receipt, rehydrates scrubbed sensitive data at the execution site using local vault keys, mints a just-in-time capability bound to the transaction hash, dispatches the action, dissolves the capability, and signs a final `COMPLETED` or `FAILED` receipt. |

Universal checks and posture-required proofs fail closed throughout the pipeline: a failed required check rejects the transaction and releases its nonce reservation, while optional L2 and L3 results remain audit evidence. For the full interlock sequence, posture configurations, and the canonical `GovernanceEnvelope` container, see [Governance](./governance.md). For L2 enrollment, deliberation, and member key management, see [Consensus](./consensus.md). For the L3 notary modes and out-of-band approval flow, see [Authentication & Authorization](./auth.md).

---

## Governance Postures

A configurable **GovernancePosture** determines which layers are enforced as fail-closed gates versus audited only. Gateway mode selects the posture at startup via `--posture <doctrine|consensus|ratify|notary>` and cannot change it at runtime. Outbound Operators receive the posture in each envelope and fail closed if it is absent or invalid. The Gateway can boot with incomplete consensus or notary configuration, but transactions requiring those services remain unavailable until they are configured.

| Posture | L1 Doctrine | L2 Consensus | L3 Notary | Typical Use |
| --- | --- | --- | --- | --- |
| **Doctrine** (default) | Enforced | Audited | Audited | Local development and CI |
| **Consensus** | Enforced | Enforced | Audited | Automated workflows with multi-agent review |
| **Ratify** | Enforced | Audited | Enforced (mutations only) | Human-authorized workflows without multi-agent review |
| **Notary** | Enforced | Enforced | Enforced (mutations only) | Production with multi-agent review and human authorization |

The following checks are enforced as fail-closed gates in every posture: L1 Doctrine validation, transaction hash integrity, nonce replay protection, expiry enforcement, state Merkle root validation, action type validation, and payload decoding. See [Governance](./governance.md) for the posture selection semantics and per-layer enforcement matrix.

---

## From Intent to Execution

For a Gateway-constructed MCP or A2A operation, the practical flow is:

1. The AI client submits an intent through the MCP or A2A endpoint.
2. The gateway translates the intent into a canonical `GovernanceEnvelope` carrying the typed payload, identity, nonce, expiry, and state root.
3. Under `consensus` or `notary` posture, the Gateway sends the envelope to its configured consensus deliberator for L2 votes; if deliberation is unavailable, the required proof is absent and the transaction fails closed at verification.
4. If L3 is required and missing, a supported Gateway MCP/A2A flow suspends the transaction and sends an approval challenge to the human. Direct envelope and governed HTTP dispatch paths must provide required L3 evidence themselves.
5. After the applicable Gateway admission and proof steps pass, the Gateway publishes the envelope to the exact session-specific command channel for the bound Operator.
6. The bound operator pulls the envelope, the L4 Warden re-verifies L1-L4, and the L5 Actuator executes the action.
7. The operator writes the signed receipt to the local audit vault and publishes it back to the gateway.
8. The gateway returns the receipt to the AI client through the original MCP/A2A response or SSE channel.

Only the operator bound to the envelope receives the work. The gateway binds the envelope to the authenticated operator session and publishes to that operator's unique command channel. No broadcast occurs. For the visual sequence, see the [principal-ensemble-gateway-operator sequence diagram](../diagrams/sequence-principal-ensemble-gateway-operator-v3.md). For the agent-facing surface, see [AI Agents and the g8e Governance Boundary](./agents.md).

Enrolled CLI owners can also pin their session to a specific operator (`g8e operator bind`) and fan out governed `EXECUTE_BASH` commands to one or more explicit operator sessions in parallel (`g8e operator run`). The gateway dispatch service constructs envelopes and waits for terminal results per target. See [Operator Architecture](./operator.md#3-cli-directed-command-dispatch).

---

## AI Client Surface

g8e exposes two standard client-facing protocols, plus typed operator and direct-envelope paths:

- **MCP (Model Context Protocol)**: A unified JSON-RPC endpoint that lets standard MCP clients such as Claude Code, Codex, Goose, or Gemini CLI discover tools, call them, and receive typed results. Gateway `tools/call` requests are translated into canonical `GovernanceEnvelope` transactions and processed through the Gateway's governed path.
- **A2A (Agent-to-Agent)**: A JSON-RPC endpoint for direct A2A skill invocations. The Gateway wraps the skill request in an envelope and routes it through the Gateway Actuator path to a configured downstream A2A server.
- **Governed HTTP dispatch and direct envelopes**: An enrolled app can dispatch a registered request `event_type` to one exact Operator session through `POST /api/v1/operators/commands`, or an authorized CLI or Operator can submit a complete envelope. These paths do not manufacture missing L2 votes or L3 proofs.

The governed Operator ships with native, typed tools for filesystem reads, shell command execution, database triage, log filtering, process inspection, network probes, cloud metadata lookup, Git state, and Kubernetes inspection. Tools are governed only when invoked through a g8e governed ingress; an external MCP wrapper forwards accepted requests directly and does not add L2-L5 processing or signed Operator receipts. See [AI Agents and the g8e Governance Boundary](./agents.md) for the client surface and [Operator Architecture](./operator.md) for the native tool playbook.

---

## Network and Identity

The platform uses a zero-trust networking model with verified SPIFFE workload identities. Protected HTTPS routes and Operator pub/sub use TLS 1.3 with mTLS where the route requires it; deliberately limited discovery and enrollment routes use plain HTTP, browser routes use web sessions, and configured MCP/A2A routes may use JWT. The platform uses `g8e.local` as the SPIFFE trust domain and as the TLS ServerName for connections that resolve the Gateway by IP.

The gateway operates a four-tier PKI hierarchy: a self-signed Root CA signs a Hub Intermediate CA (which signs the gateway serving certificate), an Operator Intermediate CA (which signs operator, CLI, and app leaf certificates), and a Gateway Peer Intermediate CA (which signs gateway peer certificates for multi-host deployments). All certificates use ECDSA P-256 and carry SPIFFE URI SANs under the `g8e.local` trust domain. Certificate revocation is enforced per-request during mTLS verification, with a standard X.509 CRL served at `/.well-known/g8e/pki/crl`.

The Gateway exposes two ports: a plain HTTP port for limited discovery, health, bootstrap, token-scoped recovery and platform-enrollment flows, deploy scripts, and node distribution, and an HTTPS port enforcing TLS 1.3 for the Console, browser WebAuthn endpoints, APIs, and governed execution routes. HTTPS client certificates are optional at the TLS handshake so browser and bootstrap assets can load; route middleware then applies public, web-session, dual, mTLS, or configured JWT requirements. Operators initiate outbound-only mTLS connections to the Gateway and pull work from exact session-specific channels; the Gateway never reaches into Operators. See [Network Architecture](./network.md) for the full PKI hierarchy, port topology, SPIFFE identity formats, and enrollment procedures.

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

g8e encrypts selected sensitive content at rest and protects applicable network paths with TLS 1.3 and mTLS. The encryption system consists of a per-runtime vault providing AES-256-GCM primitives, a three-tier vault key hierarchy (private key, HKDF-derived Key Encryption Key, and wrapped Data Encryption Key), a platform keystore for long-lived security material, and the PKI hierarchy for certificate-based identity. Structured metadata, suspended envelopes, SSE events, commitment records, and complete databases are not uniformly vault-encrypted.

The vault encrypts selected audit content, command output, file diffs, governed file copies, and scrubbing-token values when it is unlocked. The Gateway opens its vault before initializing services that require protected storage, while individual stores define their own locked-vault and persistence behavior. The keystore protects session material, signing keys, CA private keys, service certificate keys, API keys, and auditor HMAC keys. The platform can link against the Go Cryptographic Module v1.0.0 (CMVP Cert #5247) when built with `GOFIPS140=v1.0.0`; the deployed binary's approved-mode and enforcement state must be checked at runtime. See [Encryption Architecture](./encryption.md) for the vault lifecycle, key hierarchy, PKI details, and FIPS compliance.

---

## Storage and Audit

The storage layer is the persistence foundation for the governance pipeline. It records every operator session, command execution, file change, governance transaction, and audit attestation so the platform can replay history, verify state, and prove what happened.

In Gateway mode the canonical SQLite database `g8e.db` hosts platform documents, key-value and blob state, state-root inputs, replay nonces, SSE events, audit records, receipts, and commitments. The suspended-transaction store and other specialized stores have separate lifecycles. In outbound Operator mode, each runtime opens its own `g8e.db`; replay protection, execution-vault content, suspended transactions, and optional file-ledger data are local to that Operator. A remote Operator's local audit and execution evidence is authoritative; Gateway receipt copies are best-effort mirrors.

Specialized services include the audit store (append-only record of sessions, events, file mutations, and signed receipts), the ledger (git-backed version control for file modifications, with the HEAD commit exposed as a verifiable state snapshot), the execution vault (encrypted, compressed command results and file diffs), the replay store (nonce-based replay protection), the suspended transaction store (envelopes awaiting L3 approval), and the commitment ledger (chain-integrity-protected attestations). The target host remains the source of truth for command history and file mutations: this is the Local-First Audit Architecture (LFAA). See [Storage Architecture](./storage.md) for the full service inventory and runtime file I/O model.

---

## SSE Event Bridge

The gateway provides a Server-Sent Events (SSE) streaming infrastructure that enables real-time event delivery from app workloads to browser and CLI clients. g8e-compatible agentic ensembles publish typed events through `POST /api/v1/sse/push` (mTLS with app workload identity), and clients consume historical events via `GET /api/v1/sse/events` and live events via `GET /api/v1/sse/stream` (dual auth: mTLS for CLI or operator, web session cookie for browser). The gateway also produces SSE events internally for platform workflows such as passkey registration and L3 transaction approval. SSE is delivery telemetry, not governance state, authorization, or durable execution evidence. In the standard dashboard deployment, the static host does not proxy the Gateway SSE endpoint, so browser event delivery requires an explicitly configured reverse proxy or a Gateway-origin frontend. See [SSE Streaming](./sse.md) for the push, poll, and stream semantics and [Dashboard (g8ed)](./dashboard.md) for the current dashboard capability boundary.

---

## Observe and Public Spectator Boundaries

The private Gateway provides credentialed, read-only observe projections for browser sessions and mTLS producers. These projections are distinct from the public spectator mirror. When enabled, the Gateway exports an allowlisted, signed public-safe projection through its private ingest listener; the separate public listener serves anonymous bootstrap, history, SSE, and content-addressed proof downloads. Public browsers never connect to the private Gateway, and the mirror does not authorize or execute mutations. See [Public Spectator Architecture and Threat Model](./public_spectator.md) and [Storage Architecture](./storage.md).

---

## Protocol Library

The g8e Protocol Library is the canonical wire contract for governed operations that enter the platform through a g8e ingress. It provides protobuf schemas and generated bindings, JSON constant registries, JSON model schemas, Python Pydantic models, canonicalization and verification helpers, and SPIFFE workload identity helpers. The Go module and Python package share the repository version, but the Go module follows root release tags and the Python package follows separate `protocol/v*` release tags; private TypeScript bindings are not published. See [Protocol Library](./protocol.md) for package structure, code generation, and release workflow.

---

## Compliance Evidence

The platform ships a protocol-owned compliance evidence foundation. Canonical assertion, framework, crosswalk, and demo-scenario catalogs are digest-verified and validated fail-closed against supported versions, references, evidence levels, responsibilities, mappings, and assessment semantics. Typed protobuf records cover control assertions, assessment scope, content-addressed evidence references, assertion and framework-control assessments, report manifests, verification reports, demo manifests, scenario definitions, step results, scenario results, and metric evidence. Client-facing serialization uses canonical protojson.

Demo runs (healthcare, finance, DHS, FedRAMP) persist a typed manifest and canonical scenario results under `.g8e/data/compliance/demo-evidence/<run-id>/`. The manifest binds scenario definitions, framework controls, execution lanes, required environment, and SHA-256 provenance for the compose file, doctrine, target data, and configuration. Content-addressed artifacts (receipts, persistence attestations, state observations, metric evidence) are persisted under SHA-256-addressed paths so scenario and step records carry resolvable references. The read-only `g8e compliance demo-run verify <run-id>` command emits a typed `ComplianceVerificationReport` and exits nonzero when any manifest, provenance, artifact, signature, protocol-chain, state-observation, metric, or directory-integrity check fails. See [Compliance Alignment Report](../reference/compliance-alignment.md) for the full catalog model, KSI evaluation, and evidence verification behavior.

---

## Scripts and Tooling

g8e provides platform-specific bootstrap scripts for local development, gateway-served deploy scripts for remote operator installation, smoke test scripts that verify SDK importability in clean environments, and a CI guard script that validates doctrine detector coverage of finalized COSAiS overlays. The `g8e demos` CLI supports air-gapped image pull, export, import, and listing for demo environments. See [Scripts](./scripts.md) for the full script inventory.

---

## Key Design Principles

- **Do not trust the AI client.** The agent provides intent; the platform verifies and executes. The client has no privileged channel.
- **Do not trust the consensus layer.** Votes are verified against trusted public keys and the transaction hash. A missing or invalid signature fails closed when L2 is required and remains non-gating audit evidence otherwise.
- **Do not trust the gateway.** The operator re-derives every proof locally before execution.
- **Multi-signature consensus.** L2 requires K-of-N Ed25519 affirmative votes from distinct members. The reference implementation signs deterministic L1-doctrine evaluations.
- **Doctrine is enforced, not suggested.** Agents can be informed of doctrine, but the L1 gate rejects forbidden actions regardless of compliance.
- **Scope is explicit.** The envelope is bound to the authenticated operator session, and only that operator's command channel receives the dispatched work.
- **Sovereign hosts.** Every operator is authoritative for its own audit ledger and state root. The gateway never reaches into operators.
- **Fail-closed where the contract requires it.** Universal checks and posture-required proofs reject transactions when they fail; protected storage and transport paths fail according to their owning service's contract. No ingress silently weakens governance to admit a missing required proof.

---

## Architecture Documentation Index

| Document | Scope |
| --- | --- |
| [AI Agents and the g8e Governance Boundary](./agents.md) | AI client surface (MCP, A2A), native tool playbook, intent-to-execution flow, security boundaries summary. |
| [Gateway Architecture](./gateway.md) | Gateway service stack, operating modes, port topology, MCP/A2A endpoints, pub/sub brokering, in-process Operator substrate. |
| [Operator Architecture](./operator.md) | Operator execution boundary, L4 Warden and L5 Actuator, native tool playbook, local audit vault. |
| [Evaluations](./evals.md) | Go-native execution-boundary suite, model campaign scoring, Observer and Provenance Operator roles, evidence, and verification. |
| [Model Provenance](./model-provenance.md) | Zero-trust model weight attestation, Provenance Operator, and chain-of-custody for scored inference. |
| [Ensemble (g8ee)](./ensemble.md) | First-party agentic ensemble: role, connection model, in-tree protocol dependency, build and test. |
| [Dashboard (g8ed)](./dashboard.md) | Browser dashboard: static-host boundary, browser and container identities, gateway-direct requests, SSE, build, and test. |
| [Governance](./governance.md) | Five-layer interlock sequence, GovernanceEnvelope structure, posture configurations, transaction flow. |
| [Consensus](./consensus.md) | L2 consensus policy, declarative bootstrap, member key management, deliberation, L4 vote verification. |
| [Authentication & Authorization](./auth.md) | CLI enrollment state machine, recovery and rotation, headless enrollment, WebAuthn notary, session binding. |
| [Network Architecture](./network.md) | PKI hierarchy, SPIFFE workload identity, mTLS enforcement, port topology, enrollment procedures. |
| [Encryption Architecture](./encryption.md) | Vault lifecycle, three-tier key hierarchy, platform keystore, TLS and mTLS, FIPS 140-3 compliance. |
| [Storage Architecture](./storage.md) | Audit store, ledger, execution vault, replay store, suspended transaction store, commitment ledger, runtime file I/O. |
| [SSE Streaming](./sse.md) | SSE push, poll, and stream endpoints for agentic ensembles and platform workflows. |
| [Public Spectator Architecture and Threat Model](./public_spectator.md) | Public-mirror observation mode, outbound-only export, closed allowlist, threat model, and availability boundaries. |
| [Protocol Library](./protocol.md) | Go and Python protocol packages, constants registries, JSON model schemas, protobuf code generation, release workflow. |
| [Scripts](./scripts.md) | Dev bootstrap, smoke test, CI guard, remote deploy, and air-gapped demo scripts. |

For developer guidelines, test patterns, and contribution conventions, see [Developer Guidelines](../devs/devs.md). For building g8e-compatible applications and agentic ensembles, see [Build Apps](../guides/build_apps.md).
