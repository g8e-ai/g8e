---
title: About
parent: Architecture
---

# About g8e

g8e is a governed execution substrate for agentic infrastructure. It provides a data-sovereign, protocol-agnostic boundary between AI agents and host systems.

The core invariant is that every transaction must clear a fail-closed verification gauntlet (L1/L2/L3) at the host boundary before execution.

For more on the origins, philosophy, and future of g8e, please visit [g8e.ai/blog](https://g8e.ai/blog).

## Core Architecture

1.  **Protocol (Substrate)**: A domain-agnostic wire contract - a typed, signed, state-bound `GovernanceEnvelope` carrying L1/L2/L3 evidence.
2.  **Governance Gateway (g8eg)**: The central, BFT-governed Policy Decision Point (PDP) running in `--listen` mode to broker, suspend, and validate transactions.
3.  **Governed Operator (g8eo)**: The host-resident execution boundary and Policy Execution Point (PEP). It enforces protocol compliance, verifies L1/L2/L3 signatures, functions as an MCP Server to clients, and executes transactions via the Warden stage.
4.  **Application Layer (Optional)**: Components like the **Engine (g8ee)** and **Dashboard (g8ed)** which consume the public protocol.
