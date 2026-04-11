# g8e Documentation Index

Documentation for the g8e platform, organized by category.

---

## Root Documents

| Document | Description |
|----------|-------------|
| [developer.md](developer.md) | Quick start, infrastructure setup, SSL, code quality rules (VSA/g8ee/VSOD), shared constants and models, project structure |
| [testing.md](testing.md) | Comprehensive testing guide — shared principles, g8e-pod environment, CI workflows, and component-specific guidelines (VSA, VSOD, g8ee) |
| [glossary.md](glossary.md) | Essential terminology for understanding the g8e platform, organized alphabetically |
| [docs-guidelines.md](docs-guidelines.md) | Documentation authoring standards — structure, style, formatting, file locations, ownership rules, and sync rules |

---

## Architecture

Cross-component internals — data flows, protocols, and system-wide design decisions.

| Document | Description |
|----------|-------------|
| [architecture/about.md](architecture/about.md) | Platform origins, story, and governance |
| [architecture/ai_agents.md](architecture/ai_agents.md) | AI agent cross-component architecture — transport, conversation data models, command execution pipeline |
| [architecture/builds.md](architecture/builds.md) | Build system — component builds, operator binary distribution, and CI workflows |
| [architecture/dashboard.md](architecture/dashboard.md) | Dashboard architecture — SSE fan-out, operator panel, and frontend integration |
| [architecture/docker.md](architecture/docker.md) | Docker architecture — service configuration, non-root users, security hardening, capability model, read-only filesystems, docker socket threat model, and dev/prod compose split |
| [architecture/mcp.md](architecture/mcp.md) | MCP (Model Context Protocol) gateway architecture |
| [architecture/operator.md](architecture/operator.md) | Operator architecture — VSA lifecycle, session management, command dispatch, binding protocol, and Sentinel integration |
| [architecture/storage.md](architecture/storage.md) | Data storage architecture — all storage layers, component roles, topology, encryption, and retention |
| [architecture/security.md](architecture/security.md) | Complete security architecture — authentication, session management, operator security, authorization, API security, data protection, LFAA encryption, Sentinel, threat model, and governance |

---

## Components

Technical reference for each platform component.

| Document | Description |
|----------|-------------|
| [components/vsa.md](components/vsa.md) | VSA (Virtual Service Agent) — Go-based operator providing secure, real-time command execution and file management for remote system operations |
| [components/g8ee.md](components/g8ee.md) | g8ee (Virtual Support Engineer) — AI engine providing agentic, LLM-powered interface for infrastructure operations with human-in-the-loop safety controls and multi-provider LLM abstraction |
| [components/vsod.md](components/vsod.md) | VSOD (VSO Dashboard) — authentication, session management, dashboard backend, operator lifecycle, SSE fan-out, and WebSocket proxy |
| [components/vsodb.md](components/vsodb.md) | VSODB — operator binary in `--listen` mode; single source of truth for persistence (SQLite document store, KV store, SSE event buffer, and pub/sub broker) |
| [components/g8e-pod.md](components/g8e-pod.md) | g8e node — always-on sidecar container for running all component tests (g8ee/VSOD/VSA), security scans, and ephemeral SSH deployment |

---

## Scripts

Documentation for the platform management CLI and supporting scripts.

| Document | Description |
|----------|-------------|
| [architecture/scripts.md](architecture/scripts.md) | Unified platform management CLI — complete command reference, workflows, and technical details |

---

## Reference

External resources and reference materials (do not modify).

| Document | Description |
|----------|-------------|
| [reference/core_principles.md](reference/core_principles.md) | Core platform principles and design philosophy |
| [reference/mcp.yaml](reference/mcp.yaml) | MCP (Model Context Protocol) schema and tool definitions |
