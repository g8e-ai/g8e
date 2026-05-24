---
title: Documentation
has_children: false
permalink: /docs/
---

# g8e Documentation Index

Last Updated: 2026-05-20
Version: v0.2.6

The documentation is organized around the g8e execution Gateway: a typed, signed, state-bound `GovernanceEnvelope`, a fail-closed gateway, and a sovereign host Operator. Application layers, frontends, agents, and tool protocols are replaceable producers and consumers of that contract.

## 1. Protocol Gateway
The domain-agnostic wire contract and governance model that every conforming component must follow.

- [**Protocol Gateway**](concepts/protocol.md) - The `GovernanceEnvelope`, transaction flow, L1Doctrine/L2Consensus/L3Notary gates, receipts, and session rules.
- [**Governance Hierarchy**](concepts/protocol.md#3-layer-governance-bedrock) - L1Doctrine/L2Consensus/L3Notary validation model.
- [**Security Principles**](concepts/protocol.md#host-sovereignty--audit) - Host sovereignty, local-first audit, and fail-closed execution.

## 2. Gateway Components
The reference Go implementation compiles from a single codebase into two role-specific binaries:

- [**Governance Gateway (g8eg)**](concepts/g8eg.md) (`g8e.gateway`) - The central PDP / BFT-governed Policy Decision Point running in Gateway mode (--doctrine, --consensus, or --notary).
- [**Governed Operator (g8eo)**](concepts/operator.md) (`g8e.operator`) - The host-side PEP / sovereign execution agent and MCP Server.

## 3. g8e-Compatible Applications
Optional producers and consumers demonstrating the protocol in action.

- [**g8e-Compatible Applications**](concepts/g8e-compatible-apps.md) - How to build conforming producers and consumers of the protocol.

## 4. Developer Resources
Guides for setting up, testing, and contributing to the platform.

- [**Developer Guidelines](devs.md)** - Environment setup, development workflows, and testing standards.
- [**Developer Troubleshooting**](guides/troubleshooting.md) - Common setup failures and recovery checks.
- [**CLI Reference**](reference/cli.md) - Help for the `./g8e` management tool.

## 5. General Reference
Broad architectural context and platform thesis.

- [**About g8e**](concepts/about.md) - Compact architecture framing.
- [**Position Paper**](concepts/position_paper.md) - Thesis on BFT governance at the AI execution boundary.
- [**Glossary**](reference/glossary.md) - Canonical platform terminology.
