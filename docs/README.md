---
title: Documentation
has_children: false
permalink: /docs/
---

# g8e Documentation Index

Last Updated: 2026-05-12
Version: v0.2.4

The **g8e Protocol** is the substrate: a domain-agnostic, zero-trust wire contract for human-verified action by autonomous systems. An **Operator** is any host-side implementation that speaks the protocol; **g8eo** is the reference Operator implementation in Go. **g8ee** (Engine) is a reference application component that demonstrates an AI-powered infrastructure-management use case on top of the protocol. This index maps high-level concepts to the protocol substrate, the reference Operator, and the optional reference application adapters.

## Platform Pillars

Foundational architecture documents defining the safety, reasoning, and environment models.

| Document | Description |
|----------|-------------|
| [architecture/security.md](architecture/security.md) | **Zero-Trust & Governance**: LFAA (Logs, Files, Audit, Activity) encryption, Sentinel traffic analysis, and human-in-the-loop controls. |
| [architecture/governance.md](architecture/governance.md) | **3-Layer Governance**: The L1/L2/L3 validation hierarchy (Bedrock, Consensus, Authorization) and the Warden circuit breaker. |
| [architecture/air_gap.md](architecture/air_gap.md) | **Air-Gap Operations**: Principles for fully disconnected environments, local inference, and zero-config bootstrap. |
| [architecture/operator.md](architecture/operator.md) | **Reference Operator (g8eo)**: Lifecycle, vault management, and real-time command tunneling in the reference Go implementation of the Operator role. |
| [architecture/storage.md](architecture/storage.md) | **Persistence**: Reference Operator Listen Mode using the coordination store (SQLite), KV, and LFAA Audit Vault. |

## Reference Application Layer

Optional reference components that demonstrate AI-powered infrastructure management on top of the g8e Protocol. They consume the public protocol surface on equal footing with any BYO client.

| Document | Description |
|----------|-------------|
| [architecture/ai_agents.md](architecture/ai_agents.md) | **Agentic AI**: Optional application-layer LLM adapter behavior, the 5-member Tribunal ensemble, and tool-loop proposal generation. |
| [architecture/prompts.md](architecture/prompts.md) | **Prompt Engineering**: Schema and logic for agent persona assembly. |
| [architecture/thinking.md](architecture/thinking.md) | **Thinking & Reasoning**: Dual-layer architecture for structural reasoning (Tribunal) and provider-native reasoning (thinking levels). |
| [architecture/agent_personas.md](architecture/agent_personas.md) | **Personas**: Gallery of bundled agent personas and their specialized capabilities. |

## Implementation Reference

Technical deep-dives into the reference Operator and reference application components shipped in this repository.

| Component | Role | Primary Implementation |
|-----------|------|------------------------|
| [**g8eo**](components/g8eo.md) | **Reference Operator** | Go-based secure execution agent that implements the Operator role: protocol verification, hub/listen-mode persistence, pub/sub runtime, policy enforcement, and audit. Replaceable by any conforming Operator. |
| [**g8ee**](components/g8ee.md) | **Reference AI Engine** | Python (FastAPI) reference application component for the AI-powered infrastructure-management use case: LLM orchestration, proposal generation, and L2 consensus proof production. |

## Guides & Standards

| Document | Description |
|----------|-------------|
| [developer.md](developer.md) | Environment setup, service bootstrap, and development workflows. |
| [testing.md](testing.md) | CI/CD, unit/integration testing, and gold-set validation. |
| [g8e-help.md](g8e-help.md) | CLI reference for the `./g8e` management tool. |
| [docs-guidelines.md](docs-guidelines.md) | Standards for documentation structure and code-first discovery rules. |
| [glossary.md](glossary.md) | Canonical platform terminology. |

## Internal Protocol Reference

| Document | Description |
|----------|-------------|
| [architecture/protocol.md](architecture/protocol.md) | **g8e Protocol**: UAP JSON mutation envelope, typed operator payloads, and governance metadata. |
| [reference/events.md](reference/events.md) | **Wire Protocol**: Registry of all internal pub/sub and SSE events. |
| [architecture/scripts.md](architecture/scripts.md) | **Management CLI**: Architecture of the core platform orchestration scripts. |
