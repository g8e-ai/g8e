---
title: g8e Help
---

# g8e CLI

The `g8e` command is the unified entry point for managing the g8e AI governance platform. It abstracts Docker complexity and ensures that all operations—from local development to fleet-wide deployment—are performed within a secure, authenticated, and consistent environment.

## CORE PRINCIPLES

- **Docker-Native**: Requires only Docker on the host. All toolchains (Go, Python, Node) are isolated in containers.
- **Security-First**: mTLS by default. All container-side operations require valid session tokens.
- **Canonical Truth**: Constants like `agents.json` and `models` are shared across components.
- **Zero-Footprint**: Fleet operations use SSH streaming to execute without persistent installation.

## THE OPERATIONAL LIFECYCLE

### identity

Sessions are saved locally in `~/.g8e/credentials` for a seamless experience.
- `./g8e login --api-key <key>`: Authenticate via API key.
- `./g8e login --device-token <token>`: Authenticate via terminal link.
- `./g8e logout`: Clear local credentials.

### platform

Manage the local g8e stack (g8es, g8ee, g8ed, g8ep).
- `./g8e platform setup`: Initial bootstrap and container build.
- `./g8e platform start [--dev]`: Bring up the stack. Use `--dev` for hot-reload in `g8ee`.
- `./g8e platform status`: View service health and component versions.
- `./g8e platform logs [service]`: Aggregated, time-ordered logs across the fleet.

### operator

Build and deploy the `g8eo` operator binary.
- `./g8e operator build`: Compile the operator for the host architecture.
- `./g8e operator deploy <host>`: Standard SCP/SSH deployment.
- `./g8e operator stream <host...>`: High-concurrency streaming injection.

### test

Run tests in isolated, pre-configured test-runner containers.
- `./g8e test g8ee [path]`: Python tests with pytest, ruff, and pyright.
- `./g8e test g8ed [path]`: Node.js tests with Vitest.
- `./g8e test g8eo [path]`: Go tests with race detection.

### security

- `security validate`: Check TLS and volume mount integrity.
- `security certs generate|rotate`: Manage the platform CA and certificates.
- `security scan-licenses`: Run license compliance scans.
- `security rotate-internal-token`: Refresh the shared platform secret.

### data

- `data users|operators|store`: Direct persistence layer interaction.
- `data settings|audit|device-links`: Configuration and LFAA vault queries.

### llm

- `llm setup|show|restart`: Configure LLM provider settings (Anthropic, OpenAI, etc.).
- `llm get|set <key>`: Fine-grained control of LLM configuration variables.

### mcp

- `mcp config|test|status`: MCP client integration for external AI tools.

### search

- `search setup|disable`: Configure Vertex AI Search for the `search_web` tool.

### ssh

- `ssh setup`: Mount host SSH credentials into g8ep for fleet operations.

### aws

- `aws setup`: Mount AWS credentials into g8ep for AWS-integrated tools.

### demo

- `demo up|down|clean`: Manage the 10-node broken-fleet demo.
- `demo profile list|switch`: Toggle between different demo scenarios.
- `demo shell N=01`: Debug a specific fleet node.

### evals

- `evals up|run|down`: Real-operator evaluation fleet management.
- `evals status|logs`: Monitor evaluation nodes.

## DETAILED HELP
- `./g8e operator --help`
- `./g8e test --help`
- `./g8e platform --help`
