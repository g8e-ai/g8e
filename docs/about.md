---
title: About
parent: Architecture
---

# About g8e

g8e is a governed transaction runtime for agentic systems. It provides a data-sovereign, AI-agnostic governance substrate between humans, AI agents, and the systems they act on.

The core product invariant is that a typed, signed, state-bound transaction reaches a sovereign host implementation that distrusts upstream inputs and refuses to mutate reality unless every independent proof (L1/L2/L3) checks out.

For more on the origins, philosophy, and future of g8e, please visit [g8e.ai/blog](https://g8e.ai/blog).

## Core Architecture

1.  **Protocol (Substrate)**: A domain-agnostic wire contract - a typed, signed, state-bound `GovernanceEnvelope` carrying L1/L2/L3 evidence.
2.  **Operator (Role)**: A host-side implementation that speaks the protocol. It receives signed transactions, enforces L1/L2/L3 verification, executes through a defensive boundary, and emits signed receipts. `g8eo` is the reference implementation.
3.  **Application Layer (Optional)**: Components like the **Engine (g8ee)** and **Dashboard (g8ed)** which consume the public protocol.
