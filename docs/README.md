---
title: Documentation
has_children: false
permalink: /docs/
---

# g8e Documentation Index

Last Updated: 2026-05-12
Version: v0.2.3

g8e is a Zero-Trust Operator/protocol substrate for secure infrastructure operations. This index maps high-level concepts to the mandatory substrate (Operator plus shared protocol) and the optional bundled application-layer adapters (Dashboard and Engine).

## Platform Pillars

Foundational architecture documents defining the safety, reasoning, and environment models.

| Document | Description |
|----------|-------------|
| [architecture/security.md](architecture/security.md) | **Zero-Trust & Governance**: LFAA (Logs, Files, Audit, Activity) encryption, Sentinel traffic analysis, and human-in-the-loop controls. |
| [architecture/governance.md](architecture/governance.md) | **3-Layer Governance**: The L1/L2/L3 validation hierarchy (Bedrock, Consensus, Authorization) and the Warden circuit breaker. |
| [architecture/air_gap.md](architecture/air_gap.md) | **Air-Gap Operations**: Principles for fully disconnected environments, local inference, and zero-config bootstrap. |
| [architecture/operator.md](architecture/operator.md) | **Secure Execution**: The g8eo Operator lifecycle, vault management, and real-time command tunneling. |
| [architecture/storage.md](architecture/storage.md) | **Persistence**: Operator Listen Mode using the coordination store (SQLite), KV, and LFAA Audit Vault. |

## Optional Application Layer

Optional reference adapters that consume the public g8e protocol.

| Document | Description |
|----------|-------------|
| [architecture/dashboard.md](architecture/dashboard.md) | **Dashboard Adapter**: Optional g8ed UI and relay behavior as an application-layer client. |
| [architecture/ai_agents.md](architecture/ai_agents.md) | **Agentic AI**: Optional application-layer LLM adapter behavior, the 5-member Tribunal ensemble, and tool-loop proposal generation. |
| [architecture/prompts.md](architecture/prompts.md) | **Prompt Engineering**: Schema and logic for agent persona assembly. |
| [architecture/thinking.md](architecture/thinking.md) | **Thinking & Reasoning**: Dual-layer architecture for structural reasoning (Tribunal) and provider-native reasoning (thinking levels). |
| [architecture/agent_personas.md](architecture/agent_personas.md) | **Personas**: Gallery of bundled agent personas and their specialized capabilities. |

## Substrate and Application-Layer Reference

Technical deep-dives into the required Operator substrate and optional bundled adapters.

| Component | Role | Primary Implementation |
|-----------|------|------------------------|
| [**g8eo**](components/g8eo.md) | **Substrate Operator** | Go-based secure execution agent, protocol hub, listen-mode persistence, pub/sub runtime, policy enforcement, and audit. |
| [**g8ee**](components/g8ee.md) | **Optional Engine Adapter** | Python (FastAPI) agentic adapter for LLM interactions, proposal generation, and proof production. |
| [**g8ed**](components/g8ed.md) | **Optional Dashboard Adapter** | Node.js (Express) application-layer UI, approval UX, and receipt display. |

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
| [architecture/protocol.md](architecture/protocol.md) | **g8e Protocol**: Protobuf `UniversalEnvelope`, typed operator payloads, and governance metadata. |
| [reference/events.md](reference/events.md) | **Wire Protocol**: Registry of all internal pub/sub and SSE events. |
| [architecture/scripts.md](architecture/scripts.md) | **Management CLI**: Architecture of the core platform orchestration scripts. |
