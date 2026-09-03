---
title: About
parent: Core
---

# About g8e

g8e is a zero-trust execution platform for agentic infrastructure. It sits between the human, AI, and real-world devices, enforcing a fail-closed verification pipeline before any state change occurs. The platform reduces cloud providers to stateless reasoning co-processors: the model reasons over tokenized projections and cryptographic commitments, while state, keys, and raw data remain on the host that owns them. The reference Operator and Governance Gateway are implemented as a single static Go binary. All dependencies are resolved at build time; the compiled binary is statically linked and has zero runtime dependencies.

The core invariant is narrow: every mutation is a typed, signed, state-bound `GovernanceEnvelope` serialized as canonical JSON. Every envelope must clear a fail-closed five-layer verification pipeline before execution. The operator re-derives every proof from scratch against its own local state before any mutation occurs.

g8e functions as a secure perimeter for tool-calling standards. It treats MCP and A2A tool calls as unverified payloads and wraps them in a strict, canonical `GovernanceEnvelope`. The gateway provides a unified MCP endpoint with JWT authentication, just-in-time user provisioning, SSE streaming, and Document, KV, and Blob stores with MCP and A2A protocol translation. The operator compiles 32 native tools for database triage, log digestion, process governance, network validation, system introspection, file operations, cloud metadata lookup, Git operations, Kubernetes inspection, shell execution, remote operator deployment, and governed audit receipt queries. See the [Position Paper](./position_paper.md) for the full argument on why this separation of reasoning and state resolves the forced choice between frontier reasoning and data sovereignty.

## Architectural Differentiators

- **Cloud as Stateless Co-Processor:** The cloud reasoning layer receives tokenized projections and cryptographic commitments, never raw data. Rehydration to real values happens only at the L5 Actuator, at the instant of execution, on the host where the data already lives. See [Encryption](../architecture/encryption.md).
- **State Remains Local:** Canonical state resides within the [Local-First Audit Architecture (LFAA)](../architecture/storage.md) on the host. The cloud provider sees transaction hashes, state roots, and tokenized projections. The hash-chained ledger serves as state history and is maintained on the host.
- **Keys Owned by Data Owners:** Vault keys are generated, imported, and controlled by the data owner. All sensitive data at rest is encrypted with AES-256-GCM using keys that never leave the host in plaintext. A compromised cloud or gateway cannot decrypt host data because the keys were never shared. See [Encryption](../architecture/encryption.md).
- **Outbound-Only Operator:** The host-resident Governed Operator connects via an outbound-only mTLS tunnel to the Governance Gateway. It listens on no ports, accepts no inbound connections, and bypasses NAT and firewalls. The gateway cannot reach into the operator; the operator pulls work when it chooses. See [Network Architecture](../architecture/network.md).
- **Zero Standing Privileges:** The operator holds no permanent administrative credentials. Permissions are minted just-in-time from the verified intent inside the governance envelope, scoped to a single action, and dissolved on completion. A compromise of any layer cannot exfiltrate persistent credentials because none exist. See [Governance](../architecture/governance.md).
- **Unified Context and Control Plane:** Every admitted action writes its complete signed receipt to the host-local SQLite audit store before execution, appends a signed attestation to the SQLite commitment chain, and records governed file snapshots in the git-backed ledger. Agents derive context from these linked stores and verify it against live host state through governed tools. See [Storage Architecture](../architecture/storage.md).
- **Proof of Human Presence:** High-risk mutations require a WebAuthn/FIDO2 passkey assertion computed over the transaction hash. The approval is bound to one action, one moment, and one host: it cannot be transplanted, replayed, or harvested. See [Authentication](../architecture/auth.md).
- **5-Layer Verification Sequence:** Mutations sequentially traverse Doctrine (L1), Consensus (L2), Notary (L3), and Warden (L4) at the Operator boundary before reaching the Actuator (L5) execution boundary. The gateway delegates required L2 deliberation to an enrolled Consensus service that produces signed Ed25519 votes over the transaction hash. Universal checks and every proof required by the active posture fail closed; optional L2 and L3 results are audited without gating execution. See [Governance](../architecture/governance.md) and [Consensus](../architecture/consensus.md).
- **Zero Standing Dependencies:** The reference Governed Operator is a single, statically compiled binary, making the platform air-gap capable for deployment in isolated infrastructure perimeters.
- **Console SPA & Dashboard:** The g8e Console provides a browser-based interface for passkey enrollment, interactive L3 transaction approval, and real-time SSE audit streaming, alongside the operator dashboard for fleet and session management. See [Dashboard Architecture](../architecture/dashboard.md) and [Authentication](../architecture/auth.md).

## Core Architecture

1. **g8e Protocol** - The domain-agnostic wire contract, schemas, transaction hash, state binding, receipt model, and L1-L5 governance verification rules. See [Protocol Specification](../../protocol/docs/spec.md).
2. **Governance Gateway** - The reference Policy Decision Point (PDP). It admits signed envelopes, manages PKI, enforces freshness and replay defense, and relays to operators. It provides a unified MCP endpoint with JWT authentication, just-in-time user provisioning, SSE streaming, and Document, KV, and Blob stores with MCP and A2A protocol translation. It does not initiate connections to operators. The gateway can run in the cloud or on-premises. See [Gateway Architecture](../architecture/gateway.md).
3. **Governed Operator** - The host-resident Policy Execution Point (PEP). It initiates outbound-only mTLS connections to the gateway, re-verifies all proofs locally against its own state, and is the only component authorized to mutate the host. The operator compiles 32 native tools for database triage, log digestion, process governance, network validation, system introspection, file operations, cloud metadata lookup, Git operations, Kubernetes inspection, shell execution, remote operator deployment, and governed audit receipt queries. The operator runs at the site of the data owner. See [Operator Architecture](../architecture/operator.md).
4. **Consensus** - An enrolled service that evaluates envelopes and produces signed Ed25519 votes over the canonical SHA-256 transaction hash for L2 consensus. The gateway delegates L2 deliberation to the Consensus and never self-signs. See [Consensus Architecture](../architecture/consensus.md) and [Governance](../architecture/governance.md).
5. **Application Layer** - Optional producers and consumers, including g8e-compatible agentic ensembles (such as Ensemble `g8ee`), operator dashboards (`g8ed`), BYO frontends, BYO agents, MCP clients, A2A clients, and native g8e applications. g8e is actor-agnostic and governs actions rather than actors. See [Connecting Applications](../guides/connect_apps_to_gateway.md), [Ensemble Architecture](../architecture/ensemble.md), [AI Agents Boundary](../architecture/agents.md), the [g8ee documentation](../ensemble/index.md), and the [g8ed documentation](../dashboard/index.md) for the first-party component details.

## Posture Configurations

The gateway supports four posture configurations that control which verification layers are enforced versus audited:

| Posture | L1 Doctrine | L2 Consensus | L3 Notary | Typical Use |
| --- | --- | --- | --- | --- |
| `doctrine` (default) | Enforced | Audited | Audited | Local development and CI |
| `consensus` | Enforced | Enforced | Audited | Automated workflows with multi-agent review |
| `ratify` | Enforced | Audited | Enforced (mutations only) | Human-authorized workflows without multi-agent review |
| `notary` | Enforced | Enforced | Enforced (mutations only) | Production with multi-agent review and human authorization |

L4 Warden and L5 Actuator are always active in all configurations. The following checks are enforced as fail-closed gates in every posture: L1 Doctrine validation, transaction hash integrity, nonce replay protection, expiry enforcement, state Merkle root validation, action type validation, and payload decoding. See [Governance](../architecture/governance.md) for posture configuration.

## Operational Philosophy

g8e is built for operators who manage remote systems under real-world pressure: production fires, looming deadlines, and multi-tasking stakeholders. The platform mirrors the workflow of a trusted expert who gathers maximum context directly on the target systems, asks high-signal questions, converges on the ideal next step, and proposes action with justification before the person with the most at stake approves. Once approved, the operator executes cleanly, proves the result, and follows up end-to-end.

The same binary, protocol, and verification pipeline governs actions across domains. What changes between domains is the doctrine configuration, the target data, and the governance posture. The data owner configures these to match their regulatory and operational requirements. The platform does not need domain-specific code. It needs domain-specific doctrine, which is data, not code.

## Related Documentation

- [Position Paper](./position_paper.md): The full argument for the sovereignty inversion.
- [Platform Overview](../architecture/overview.md): High-level system architecture, components, and verification pipeline.
- [Gateway Architecture](../architecture/gateway.md): Gateway role, capabilities, and port topology.
- [Operator Architecture](../architecture/operator.md): Operator role, native tools, and local audit.
- [AI Agents and Governance Boundary](../architecture/agents.md): Untrusted AI client interaction, native tools, and execution flow.
- [Consensus Architecture](../architecture/consensus.md): L2 multi-signature consensus, enrollment, and deliberation flow.
- [Authentication](../architecture/auth.md): mTLS, SPIFFE, PKI, WebAuthn, and the five-layer verification sequence.
- [Encryption](../architecture/encryption.md): Vault architecture, key hierarchy, and cryptographic primitives.
- [Storage Architecture](../architecture/storage.md): Audit store, ledger, execution vault, and data flow.
- [Network Architecture](../architecture/network.md): PKI, mTLS, enrollment, and outbound-only connectivity.
- [Governance](../architecture/governance.md): Five-layer pipeline, posture configurations, and transaction flow.
- [SSE Streaming](../architecture/sse.md): SSE event stream, real-time approvals, and audit telemetry.
- [Dashboard Architecture](../architecture/dashboard.md): Operator dashboard, passkey enrollment, and management interface.
- [Ensemble Architecture](../architecture/ensemble.md): First-party agentic ensemble service and execution workflow.
- [g8ee Documentation](../ensemble/index.md): Detailed g8ee component documentation — agents, governance, prompts, SSE, storage, and evals.
- [g8ed Documentation](../dashboard/index.md): Detailed g8ed component documentation — architecture, auth, gateway integration, SSE, and operator surfaces.
- [Protocol Specification](../../protocol/docs/spec.md): Wire contract, schemas, and verification rules.

