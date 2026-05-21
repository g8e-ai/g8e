---
title: About
parent: Architecture
---

# About g8e

g8e is a zero-trust execution Gateway for agentic infrastructure. It gives AI systems, BYO frontends, BYO agents, and standard tool protocols a governed way to mutate real machines.

The core invariant is narrow: every mutation is a typed, signed, state-bound `GovernanceEnvelope`, and every envelope must clear a fail-closed L1/L2/L3 verification gauntlet before the host executes.

MCP, A2A, OpenAI-style tool calls, and future agent protocols are payload formats. g8e is the governance envelope and sovereign execution boundary around those payloads.

## Core Architecture

1. **Protocol Gateway** - the domain-agnostic wire contract, schemas, transaction hash, state binding, receipt model, and L1/L2/L3 verification rules.
2. **Governance Gateway (`g8eg`)** - the reference Policy Decision Point (PDP), running in `--listen` mode for mTLS APIs, PKI, replay defense, transaction suspension, state roots, and dispatch.
3. **Governed Operator (`g8eo`)** - the host-resident Policy Execution Point (PEP), MCP server, Sentinel scrubber, local audit authority, and Warden execution boundary.
4. **Application layer** - optional producers and consumers, including the reference Engine (`g8ee`), BYO frontends, BYO agents, MCP clients, A2A clients, and native g8e applications.
