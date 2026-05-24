---
title: About
parent: Architecture
---

# About g8e

g8e is a zero-trust execution Gateway for agentic infrastructure. It gives AI systems, BYO frontends, BYO agents, and standard tool protocols a governed way to mutate real machines.

The core invariant is narrow: every mutation is a typed, signed, state-bound `GovernanceEnvelope`, and every envelope must clear a fail-closed Doctrine (L1), Quorum (L2), and Notary (L3) verification gauntlet before the host executes.

Rather than competing with tool-calling standards like Anthropic’s Model Context Protocol (MCP) or A2A, g8e functions as a secure perimeter. It treats standard JSON-RPC tools as unverified payloads (the "what") and wraps them in a strict, canonical `GovernanceEnvelope` (the "how").

## Architectural Differentiators

*   **Outbound-Only Reverse Tunnel:** The host-resident Operator binary (`g8eo`) connects via an outbound-only tunnel to the platform hub. This architecture completely bypasses NAT and enterprise firewalls, eliminating the operational necessity of opening dangerous inbound listening ports.
*   **Protocol-First Zero Trust:** Every system component inherently distrusts all other components. The execution gateway boundary actively handles workloads via mTLS and device-link tokens, ensuring no unverified component holds privileged trust.
*   **Byzantine Fault Tolerant (BFT) Safety:** Agentic automation is treated as a distributed consensus problem. The Quorum (L2) Tribunal is fully provider-agnostic, running multiple independent agents in parallel and blind to each other. By combining heterogeneous models (e.g., Anthropic, OpenAI, local open-source models), a single poisoned or hallucinating model is simply outvoted by the ensemble.
*   **Deterministic Intent Validation:** Execution authority does not rely on fluid natural language strings. The protocol enforces that explicit execution intent is serialized into a typed Protobuf payload, base64-encoded, and locked into the transaction hash of the envelope.
*   **3-Layer Inline Governance Gate:** Every mutation must sequentially pass Doctrine (L1) Technical Bedrock (Hard Gates), Quorum (L2) Consensus (Tribunal), and Notary (L3) Authorization (WebAuthn/Passkey) at the Operator boundary before hitting the host shell.
*   **Local-First Data Sovereignty (LFAA):** All raw data, system roots, and execution histories are isolated locally on the managed host. Every file mutation triggers a two-phase Git-backed commit tracking pre-mutation and post-mutation states, guaranteeing a tamper-evident history trail and instant rollbacks.
*   **Zero Standing Dependencies:** The reference Operator is a single, statically compiled Go binary, making the entire platform air-gap capable for deployment in highly hostile, isolated infrastructure perimeters.

## Core Architecture

1. **Protocol Gateway** - the domain-agnostic wire contract, schemas, transaction hash, state binding, receipt model, and Doctrine (L1)/Quorum (L2)/Notary (L3) verification rules.
2. **Governance Gateway (`g8eg`)** - the reference Policy Decision Point (PDP), running in Gateway mode (--doctrine, --consensus, or --notary) for mTLS APIs, PKI, replay defense, transaction suspension, state roots, and dispatch.
3. **Governed Operator (`g8eo`)** - the host-resident Policy Execution Point (PEP), MCP server, Sentinel scrubber, local audit authority, and Actuator execution boundary.
4. **Application layer** - optional producers and consumers, including g8e-compatible agentic ensembles, BYO frontends, BYO agents, MCP clients, A2A clients, and native g8e applications.
