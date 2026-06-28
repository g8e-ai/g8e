---
title: About
parent: Core
---

# About g8e

g8e is a sovereign execution platform that delivers frontier AI reasoning to the edge without surrendering data custody. It reduces cloud providers to stateless co-processors: the model reasons over tokenized projections and cryptographic commitments, while state, keys, and raw data remain on the host that owns them. The platform is implemented as a single static Go binary with zero external dependencies.

The core invariant is narrow: every mutation is a typed, signed, state-bound `GovernanceEnvelope` serialized as canonical JSON. Every envelope must clear a fail-closed verification pipeline of Doctrine (L1Doctrine), Consensus (L2Consensus), Notary (L3Notary), and Warden (L4Warden) before the Actuator (L5Actuator) executes. The operator re-derives every proof from scratch against its own local state before any mutation occurs.

g8e functions as a secure perimeter for tool-calling standards such as MCP and A2A. It treats standard JSON-RPC tools as unverified payloads and wraps them in a strict, canonical `GovernanceEnvelope`. See the [Position Paper](./position_paper.md) for the full argument on why this separation of reasoning and state resolves the forced choice between frontier reasoning and data sovereignty.

## Architectural Differentiators

*   **Cloud as Stateless Co-Processor:** The cloud reasoning layer receives tokenized projections and cryptographic commitments, never raw data. Rehydration to real values happens only at the L5 Actuator, at the instant of execution, on the host where the data already lives. See [Encryption](../architecture/encryption.md).
*   **State Remains Local:** Canonical state resides within the [Local-First Audit Architecture (LFAA)](../architecture/storage.md) on the host. The cloud provider sees transaction hashes, state roots, and tokenized projections. The hash-chained ledger serves as state history and is maintained on the host.
*   **Keys Owned by Data Owners:** Vault keys are generated, imported, and controlled by the data owner. All sensitive data at rest is encrypted with AES-256-GCM using keys that never leave the host in plaintext. A compromised cloud or gateway cannot decrypt host data because the keys were never shared. See [Encryption](../architecture/encryption.md).
*   **Outbound-Only Operator:** The host-resident g8e Operator connects via an outbound-only mTLS tunnel to the g8e Gateway. It listens on no ports, accepts no inbound connections, and bypasses NAT and firewalls. The gateway cannot reach into the operator; the operator pulls work when it chooses. See [Network Architecture](../architecture/network.md).
*   **Zero Standing Privileges:** The operator holds no permanent administrative credentials. Permissions are minted just-in-time from the verified intent inside the governance envelope, scoped to a single action, and dissolved on completion. A compromise of any layer cannot exfiltrate persistent credentials because none exist. See [Governance](../architecture/governance.md).
*   **Unified Context and Control Plane:** The hash-chained ledger that governs execution also serves as the context substrate. Every admitted action writes a signed receipt to a host-local, git-backed, hash-chained ledger before the side effect is executed. Agents derive context from this chain and verify it against live host state through governed tools. See [Storage Architecture](../architecture/storage.md).
*   **Proof of Human Presence:** High-risk mutations require a WebAuthn/FIDO2 passkey assertion computed over the transaction hash. The approval is bound to one action, one moment, and one host: it cannot be transplanted, replayed, or harvested. See [Authentication](../architecture/auth.md).
*   **5-Layer Verification Sequence:** Mutations must sequentially pass Doctrine (L1Doctrine), Consensus (L2Consensus), Notary (L3Notary), and Warden (L4Warden) at the Operator boundary before hitting the Actuator (L5Actuator) execution boundary. Every layer fails closed. See [Governance](../architecture/governance.md).
*   **Zero Standing Dependencies:** The reference g8e Operator is a single, statically compiled binary, making the platform air-gap capable for deployment in isolated infrastructure perimeters.

## Core Architecture

1. **g8e Protocol** - the domain-agnostic wire contract, schemas, transaction hash, state binding, receipt model, and the L1-L5 governance verification rules. See [Protocol Specification](../../protocol/docs/spec.md).
2. **g8e Gateway** - the reference Policy Decision Point (PDP). It admits signed envelopes, manages PKI, enforces freshness and replay defense, and relays to operators. It does not initiate connections to operators and does not possess execution authority. The gateway can run in the cloud or on-premises. See [Gateway Architecture](../architecture/gateway.md).
3. **g8e Operator** - the host-resident Policy Execution Point (PEP). It initiates outbound-only mTLS connections to the gateway, re-verifies all proofs locally against its own state, and is the only component authorized to mutate the host. The operator runs at the site of the data owner. See [Operator Architecture](../architecture/operator.md).
4. **Application Layer** - optional producers and consumers, including g8e-compatible agentic ensembles, BYO frontends, BYO agents, MCP clients, A2A clients, and native g8e applications. g8e is actor-agnostic and governs actions rather than actors. See [Connecting Applications](../guides/connect_apps_to_gateway.md).
