---
title: Documentation
has_children: false
permalink: /docs/
---

# g8e Documentation Index

Last Updated: 2026-05-20
Version: v0.2.6

The documentation is organized around the g8e execution substrate: a typed, signed, state-bound `GovernanceEnvelope`, a fail-closed gateway, and a sovereign host Operator. Application layers, frontends, agents, and tool protocols are replaceable producers and consumers of that contract.

## 1. Protocol Substrate
The domain-agnostic wire contract and governance model that every conforming component must follow.

- [**Protocol Substrate**](protocol.md) - The `GovernanceEnvelope`, transaction flow, L1/L2/L3 gates, receipts, and session rules.
- [**Governance Hierarchy**](protocol.md#3-layer-governance-bedrock) - L1/L2/L3 validation model.
- [**Security Principles**](protocol.md#host-sovereignty--audit) - Host sovereignty, local-first audit, and fail-closed execution.

## 2. Substrate Components
The reference Go implementation compiles from a single codebase into two role-specific binaries:

- [**Governance Gateway (g8eg)**](g8eg.md) (`g8e.gateway`) - The central PDP / BFT-governed Policy Decision Point running in `--listen` mode.
- [**Governed Operator (g8eo)**](operator.md) (`g8e.operator`) - The host-side PEP / sovereign execution agent and MCP Server.

## 3. Reference Applications
Optional producers and consumers demonstrating the protocol in action.

- [**g8ee Engine**](g8ee.md) - Reference AI reasoning and Tribunal orchestration.

## 4. Developer Resources
Guides for setting up, testing, and contributing to the platform.

- [**Contribution Guide**](../CONTRIBUTING.md) - Environment setup, development workflows, and testing standards.
- [**Developer Troubleshooting**](troubleshooting.md) - Common setup failures and recovery checks.
- [**CLI Reference**](cli_help.md) - Help for the `./g8e` management tool.

## 5. General Reference
Broad architectural context and platform thesis.

- [**About g8e**](about.md) - Compact architecture framing.
- [**Position Paper**](position_paper.md) - Thesis on BFT governance at the AI execution boundary.
- [**Glossary**](glossary.md) - Canonical platform terminology.
