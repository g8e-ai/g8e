---
title: Documentation
has_children: false
permalink: /docs/
---

# g8e Documentation Index

g8e is a Zero-Trust AI platform for secure infrastructure operations. This index maps high-level concepts to their technical implementations.

## Platform Pillars

| Document | Description |
|----------|-------------|
| [architecture/security.md](architecture/security.md) | **Zero-Trust & Governance**: LFAA (Logs, Files, Audit, Activity) encryption, Sentinel traffic analysis, and human-in-the-loop controls. |
| [architecture/ai_agents.md](architecture/ai_agents.md) | **Agentic AI**: Multi-provider LLM abstraction (g8ee), tool-loop execution, and secure command dispatch. |
| [architecture/operator.md](architecture/operator.md) | **Secure Execution**: The g8eo Operator lifecycle, vault management, and real-time command tunneling. |
| [architecture/storage.md](architecture/storage.md) | **Persistence**: g8es (Data Bus) architecture using SQLite, KV stores, and secure blob storage. |

## Component Reference

Technical deep-dives into the services that comprise the g8e stack.

| Component | Role | Primary Implementation |
|-----------|------|------------------------|
| [**g8eo**](components/g8eo.md) | Operator | Go-based secure execution agent with git-backed ledger (LFAA). |
| [**g8ee**](components/g8ee.md) | Engine | Python (FastAPI) agentic orchestrator for LLM interactions and tool dispatch. |
| [**g8ed**](components/g8ed.md) | Terminal | Node.js (Express) management plane, SSE fan-out, and session binding. |
| [**g8es**](components/g8es.md) | Data Bus | g8eo in `--listen` mode providing unified persistence and pub/sub. |
| [**g8el**](components/g8el.md) | Local LLM | llama.cpp sidecar for air-gapped or local model execution. |
| [**g8ep**](components/g8ep.md) | Sidecar | Always-on management node for fleet operations and security scanning. |

## Guides & Standards

| Document | Description |
|----------|-------------|
| [developer.md](developer.md) | Environment setup, service bootstrap, and development workflows. |
| [testing.md](testing.md) | CI/CD, unit/integration testing on g8ep, and gold-set validation. |
| [g8e-help.md](g8e-help.md) | CLI reference for the `/home/bob/g8e/g8e` management tool. |
| [docs-guidelines.md](docs-guidelines.md) | Standards for documentation structure and code-first discovery rules. |
| [glossary.md](glossary.md) | Canonical platform terminology. |

## Internal Protocol Reference

| Document | Description |
|----------|-------------|
| [reference/events.md](reference/events.md) | **Wire Protocol**: Registry of all internal pub/sub and SSE events. |
| [architecture/prompts.md](architecture/prompts.md) | **Prompt Engineering**: Schema and logic for agent persona assembly. |
| [architecture/scripts.md](architecture/scripts.md) | **Management CLI**: Architecture of the core platform orchestration scripts. |
