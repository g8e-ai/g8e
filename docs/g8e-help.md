---
title: g8e CLI
---

# g8e Platform CLI

The `g8e` command is the unified entry point for the g8e AI governance platform. It orchestrates the full lifecycle of a self-hosted, human-in-the-loop AI operations system.

## Core Principles

The platform is built on security-first architectural invariants that cannot be bypassed:

- **Authority**: Every action is gated by FIDO2/mTLS. Human judgment is the final, non-bypassable security layer.
- **Zero Trust**: No standing credentials. Privileges are earned per-action and mathematically bound to sessions.
- **Binary Safety**: Security is enforced at the binary and network layers, not via fragile LLM prompts.
- **Data Sovereignty**: Operational data stays on the remote host; only scrubbed context reaches the AI.
- **Ephemeral Presence**: Tiny, outbound-only operator binary with no inbound ports. It vanishes when the process ends.
- **Immutable Audit**: Git-backed ledgers on each operator provide a complete, tamper-evident record of every change.
- **Air-Gap Capable**: Fully self-hosted with no SaaS dependencies or mandatory telemetry.
- **Provider Agnostic**: Swap LLM providers (Gemini, Anthropic, OpenAI, Ollama, local) at will. Governance is the constant.

## Component Architecture

The platform is composed of specialized components, each with a single responsibility:

| Component | Language | Purpose |
|-----------|----------|---------|
| **g8ed** | Node.js | Dashboard & API Gateway. Authentication, session management, SSE relay, operator lifecycle. |
| **g8ee** | Python | Reasoning Engine. Orchestrates AI agents (Triage, Sage, Dash, Tribunal) and enforces governance. |
| **g8es** | Go | Platform Persistence & Pub/Sub. SQLite-based blob store, KV cache, and event bus. |
| **g8eo** | Go | Remote Operator. Execution agent deployed to target hosts with LFAA audit trails. |
| **g8el** | C | Local Inference. Optional llama.cpp-based local model server for air-gapped deployments. |
| **g8ep** | N/A | Operational Pod. Container where CLI commands execute and investigations run. |

### Agent Terminology

The AI reasoning engine uses specialized agents with distinct roles:

- **Triage**: The initial classifier that reads message complexity, intent, and user posture.
- **Dash**: Fast-path responder for simple, single-step requests (e.g., status checks, greetings).
- **Sage**: Senior reasoning agent for complex, multi-step investigations and command orchestration.
- **Tribunal**: 5-member ensemble (Axiom, Concord, Variance, Pragma, Nemesis) that translates Sage's intent into hardened shell commands through Byzantine consensus.
- **Warden**: Defensive coordinator that performs pre-execution risk assessment on commands, files, and errors.
- **Auditor**: Final quality gate that verifies Tribunal output against Sage's intent with dissent awareness.

## The Request Lifecycle

A single user message moves through six distinct phases:

1. **Ingress**: g8ed receives the request via HTTPS, authenticates the session, and relays to g8ee with full context.
2. **Triage**: g8ee classifies the message as `simple` (Dash, lite model) or `complex` (Sage, primary model).
3. **Orchestration**: The selected agent runs a ReAct loop, calling tools as needed. Gated tools route through the Tribunal.
4. **Governance**: Every command passes through Sentinel (scrubbing), Tribunal (translation), Warden (risk analysis), and Auditor (verification).
5. **Approval**: State-changing operations halt for human approval via the dashboard.
6. **Execution & Audit**: Approved commands execute on the operator. Results are scrubbed, persisted to local vaults, and committed to the git ledger.

## Operational Modes

### Operator Bound Mode
When at least one g8eo operator is connected and bound to the session:

- Full tool suite: command execution, file operations, directory listing, port checks, web search
- Human-in-the-loop: All state-changing operations require explicit approval
- Multi-operator support: AI selects targets per command; batch operations fan out with unified approval
- Intent permissions: AWS-type operators use JIT permission escalation via the Intent System

### Advisory Mode
When no operator is connected:

- Limited tools: `search_web` only (if Vertex AI Search is configured)
- No execution: AI provides guidance and suggested commands but cannot act on infrastructure
- Behavior: The system automatically swaps to no-search prompt variants to prevent hallucinated tool calls

## Platform Lifecycle

### Initial Setup
```bash
./g8e platform setup    # Bootstrap and build all containers
./g8e platform start    # Bring up the managed services
```

### Daily Operations
```bash
./g8e platform status   # Check service health
./g8e platform logs     # Stream aggregated logs
./g8e platform settings # View or update configuration
```

### Operator Deployment
```bash
./g8e operator build           # Build for current architecture
./g8e operator deploy user@host # Deploy to remote host
./g8e operator stream host...  # Fleet-wide streaming deployment
```

### Testing & Development
```bash
./g8e test g8ee tests/unit     # Python backend tests
./g8e test g8ed test/unit      # Dashboard tests
./g8e test g8eo ./...          # Go operator tests
```

## Command Reference

### identity
Authentication and session management.
- `login`: Authenticate and save session to `~/.g8e/credentials`
- `logout`: Clear local session and credentials

### platform
Manage the local Docker stack lifecycle.
- `setup`: Initial bootstrap and non-cached container build
- `start [--dev]`: Bring up managed services (use `--dev` for hot-reload)
- `status`: View service health, ports, and component versions
- `logs [service]`: Stream aggregated, time-ordered logs
- `update`: Pull latest code from git and rebuild
- `settings`: Manage global platform configuration
- `rebuild [svc]`: Rebuild specific images without wiping volumes
- `reset`: Wipe all data volumes and rebuild from scratch
- `wipe`: Clear app data from database while preserving settings and certs
- `clean`: Remove all g8e Docker resources (containers, volumes, images)
- `demo`: Start platform and demo environment together

### operator
Build and deploy g8eo operators.
- `init`: Build operator binary in test-runner container
- `build`: Build amd64 operator for current host
- `build-all`: Build and compress binaries for all architectures (amd64, arm64, 386)
- `deploy <host>`: SCP/SSH deployment and launch with flags for arch, endpoint, ports
- `stream <host...>`: High-concurrency streaming injection across fleet
- `reauth --user-id|--email`: Request fresh operator session for specific user
- `ssh-config`: Manage SSH identities for fleet operations

### test
Run tests in isolated test-runner containers.
- `g8ee [path]`: Python tests (pytest, ruff, pyright) with LLM provider flags
- `g8ed [path]`: Dashboard and API tests (Vitest)
- `g8eo [path]`: Go operator tests with race detection

### security
Audit and manage security posture.
- `validate`: Check TLS integrity and volume mount permissions
- `certs generate|rotate|status|trust`: Manage platform CA and mTLS certificates
- `passkeys`: Manage FIDO2/WebAuthn credentials
- `rotate-internal-token`: Refresh shared secret between components
- `scan-licenses`: Run compliance scans on dependencies
- `mtls-test`: Verify mTLS connectivity between g8ep and g8ed

### data
Direct interaction with persistence layer via g8ep.
- `users|operators`: Query or modify user and operator documents
- `store <collection> list|get|stats|network|find|kv|wipe|get-setting`: Access blob store
- `settings`: Manage platform configuration
- `audit`: View LFAA audit logs
- `device-links`: Manage device link tokens

### llm
Configure LLM providers and model selection.
- `setup`: Interactive provider configuration
- `show|get|set <key>`: View or update LLM variables
- `restart`: Restart inference engine to apply settings

### demo
Manage the "broken fleet" simulation for AI operator training and evaluation.
- `up [-n <count>] [-d <token>]`: Build and start N devices (default: 10). Pass a `DEVICE_TOKEN` to auto-attach operators.
- `down`: Stop all simulation nodes and dashboards.
- `status`: View container status, node counts, and dashboard availability.
- `clean`: Forcefully remove all demo containers, images, and networks.
- `health`: Run diagnostic checks (e.g., Flask/Nginx status) across the active fleet.
- `profile [list|switch P=<name>]`: Manage demo scenarios. Profiles include `acme-corp` (1000-node regional fleet), `nginx` (broken web apps), and `fleet`.
- `shell <node>`: Drop into a specific simulation node's shell for manual verification.
- `devices`: List all discovered device hostnames in the active profile.
- `broken`: List devices currently in a non-healthy state (e.g., SSL expired, 502 Bad Upstream).
- `operators`: Show the status of g8e operator processes across the fleet.
- `deploy -d <token>`: Push and launch operators to all nodes via pubsub.
- `stream -d <token>`: Inject operators via SSH streaming (requires `g8ep` access).
- `vanish`: Zero-trace removal of all operator processes and binaries from the fleet.
- `dashboard`: Access the standalone, profile-specific status dashboard (e.g., ACME Global Monitor).

### evals
Real-operator evaluation fleet management.
- `run --gold-set <path>`: Execute benchmark against gold set
- `list`: List available evaluation scenarios
- `up --nodes <n> --device-token <tok>`: Bring up eval nodes
- `down`: Tear down eval fleet
- `status`: View eval node status
- `logs <node>`: View logs for specific node

### mcp
Model Context Protocol integration.
- `config`: Configure MCP servers
- `test`: Test MCP connectivity
- `status`: View MCP connection status

### search
Vertex AI Search configuration for web search tool.
- `setup`: Configure search provider
- `disable`: Disable search integration

### ssh
SSH credential management for fleet operations.
- `setup`: Mount host SSH keys into g8ep

### aws
AWS credential management for AWS-integrated tools.
- `setup`: Mount AWS credentials into g8ep

## Detailed Help

For detailed flags and usage examples:
```bash
./g8e platform --help
./g8e operator --help
./g8e test --help
./g8e demo --help
```
