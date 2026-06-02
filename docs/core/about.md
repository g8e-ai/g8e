---
title: About
parent: Architecture
---

# About g8e

g8e is a zero-trust execution platform for agentic infrastructure. It provides a governed mechanism for AI systems, clients, and agents to interact with host systems via standardized tool protocols.

The core invariant is narrow: every mutation is a typed, signed, state-bound `GovernanceEnvelope` serialized as canonical JSON. Every envelope must clear a fail-closed verification pipeline of Doctrine (L1Doctrine), Consensus (L2Consensus), Notary (L3Notary), and Warden (L4Warden) before the Actuator (L5Actuator) executes.

Rather than competing with tool-calling standards such as MCP or A2A, g8e functions as a secure perimeter. It treats standard JSON-RPC tools as unverified payloads and wraps them in a strict, canonical `GovernanceEnvelope`.

## Architectural Differentiators

*   **Outbound-Only Reverse Tunnel:** The host-resident g8e Operator connects via an outbound-only tunnel to the g8e Gateway. This architecture bypasses NAT and firewalls, eliminating the requirement for inbound listening ports.
*   **g8e Protocol Zero Trust:** Every system component distrusts all other components. The execution boundary handles workloads via mTLS; no unverified component holds privileged trust.
*   **Byzantine Fault Tolerant (BFT) Safety:** Agentic automation is treated as a distributed consensus problem. The Consensus (L2Consensus) layer is provider-agnostic, running multiple independent agents in parallel. By combining heterogeneous models, a single poisoned or hallucinating model is outvoted by the ensemble.
*   **Deterministic Intent Validation:** Execution authority does not rely on natural language. The protocol enforces that execution intent is serialized into a typed Protobuf payload and locked into the deterministic transaction hash of the `GovernanceEnvelope`.
*   **5-Layer Verification Sequence:** Mutations must sequentially pass Doctrine (L1Doctrine), Consensus (L2Consensus), Notary (L3Notary), and Warden (L4Warden) at the Operator boundary before hitting the Actuator (L5Actuator) execution boundary.
*   **Local-First Data Sovereignty:** Raw data, system roots, and execution histories are isolated locally on the managed host. Every file mutation triggers a two-phase Git-backed commit tracking pre-mutation and post-mutation states, guaranteeing a tamper-evident history trail and rollbacks.
*   **Zero Standing Dependencies:** The reference g8e Operator is a single, statically compiled g8e Node, making the platform air-gap capable for deployment in isolated infrastructure perimeters.

## Core Architecture

1. **g8e Protocol** - the domain-agnostic wire contract, schemas, transaction hash, state binding, receipt model, and the L1-L5 governance verification rules.
2. **g8e Gateway** - the reference Policy Decision Point (PDP), running in gateway mode for mTLS APIs, PKI, replay defense, transaction suspension, state roots, and dispatch.
3. **g8e Operator** - the host-resident Policy Execution Point (PEP), MCP server, local audit authority, and Actuator (L5Actuator) execution boundary.
4. **Application Layer** - optional producers and consumers, including g8e-compatible agentic ensembles, BYO frontends, BYO agents, MCP clients, A2A clients, and native g8e applications.
