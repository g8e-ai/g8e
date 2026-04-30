---
title: g8e Help
---

# g8e CLI

The `g8e` command is the unified entry point for the g8e AI governance platform. It manages the full lifecycle of the platform, from local development and testing to fleet-wide operator deployment.

## CORE PRINCIPLES

- **AUTHORITY**: Every action is gated by FIDO2/mTLS. Human judgment is the final, non-bypassable security layer.
- **TRUST**: Zero standing credentials. Privileges are earned per-action and mathematically bound to sessions.
- **STRUCTURE**: Safety is enforced at the binary and network layers, not via fragile LLM prompts.
- **SOVEREIGNTY**: You own your data. Operational data stays on the remote host; only scrubbed context reaches the AI.
- **PRESENCE**: Tiny, outbound-only operator binary with no inbound ports. It vanishes when the process ends.
- **AUDIT**: Immutable, local, git-backed ledgers provide a complete record of every change.
- **ISOLATION**: Fully self-hosted and air-gap capable. No SaaS dependencies or mandatory telemetry.
- **AGNOSTIC**: Swap models (Anthropic, OpenAI, local) or infrastructure at will. Governance is the constant.

## COMPONENT ARCHITECTURE

- **g8ed**: Dashboard & API Gateway (Node.js). The central interface for humans.
- **g8ee**: Reasoning Engine (Python). Orchestrates agents (Sage, Dash, Triage, etc.).
- **g8es**: Platform Persistence & Pub/Sub (Go). Handles the blob store and event bus.
- **g8eo**: Remote Operator (Go). The execution agent deployed to target hosts.
- **g8el**: Local Inference (llama.cpp). Optional local model server for air-gapped use.
- **g8ep**: Operational Pod. The container where CLI commands and investigations execute.

### Terminology Mapping
- **Triage**: (GDD: Dash) The initial classifier that reads complexity and intent.
- **Dash**: (GDD: Fast-path) The high-speed responder for simple, single-step requests.
- **Sage**: The senior reasoning agent for complex, multi-step investigations.
- **Tribunal**: The 5-member ensemble (Axiom, Concord, Variance, Pragma, Nemesis) that translates intent to commands.

---

## COMMAND REFERENCE

### identity
Manage local authentication and session state.
- `./g8e login`: Authenticate and save a session token to `~/.g8e/credentials`.
- `./g8e logout`: Clear the local session and credentials.

### platform
Manage the local g8e stack lifecycle.
- `./g8e platform setup`: Initial bootstrap and non-cached container build.
- `./g8e platform start [--dev]`: Bring up the managed services. Use `--dev` for hot-reload.
- `./g8e platform status`: View service health, ports, and component versions.
- `./g8e platform logs [service]`: Stream aggregated, time-ordered logs.
- `./g8e platform update`: Pull the latest code from git and rebuild.
- `./g8e platform settings`: Manage global platform configuration.
- `./g8e platform rebuild [svc]`: Rebuild specific images without wiping volumes.
- `./g8e platform reset`: Wipe all data volumes and rebuild from scratch.
- `./g8e platform wipe`: Clear app data from the database while preserving settings and certs.
- `./g8e platform clean`: Nuke all g8e Docker resources (containers, volumes, images).

### operator
Build and deploy the `g8eo` operator binary.
- `./g8e operator build`: Build the amd64 operator for the current host.
- `./g8e operator build-all`: Build and compress binaries for all target architectures (amd64, arm64, 386).
- `./g8e operator deploy <host>`: SCP/SSH deployment and launch of the operator.
- `./g8e operator stream <host...>`: High-concurrency streaming injection of the operator.
- `./g8e operator reauth`: Request a fresh operator session for a specific user.
- `./g8e operator ssh-config`: Manage SSH identities for fleet operations.

### test
Run tests in isolated, pre-configured test-runner containers.
- `./g8e test g8ee [path]`: Python backend tests (pytest, ruff, pyright).
- `./g8e test g8ed [path]`: Dashboard and API tests (Vitest).
- `./g8e test g8eo [path]`: Go operator tests with race detection.

### security
Audit and manage the platform's security posture.
- `security validate`: Check TLS integrity and volume mount permissions.
- `security certs generate|rotate|status`: Manage the platform CA and mTLS certificates.
- `security passkeys`: Manage FIDO2/WebAuthn credentials.
- `security rotate-internal-token`: Refresh the shared secret between components.
- `security scan-licenses`: Run compliance scans on all dependencies.
- `security mtls-test`: Verify mTLS connectivity between g8ep and g8ed.

### data
Direct interaction with the persistence layer via `g8ep`.
- `data users|operators|store`: Query or modify the primary collections.
- `data settings|audit|device-links`: Manage platform config and LFAA audit logs.

### llm
Configure LLM providers and model selection.
- `llm setup`: Interactive provider configuration (Anthropic, OpenAI, etc.).
- `llm show|get|set <key>`: View or update specific LLM variables.
- `llm restart`: Bounce the inference engine to apply new settings.

### demo
Manage the 10-node "broken fleet" simulation.
- `demo up|down|status`: Manage the demo infrastructure.
- `demo profile list|switch|create`: Manage different demo scenarios (e.g., "fleet", "db-fail").
- `demo shell <node>`: Drop into a specific simulation node.
- `demo dashboard`: Access the standalone demo status dashboard.

### evals
Real-operator evaluation fleet management.
- `evals up|down|status`: Manage evaluation nodes.
- `evals run --gold-set <path>`: Execute a benchmark against a defined gold set.
- `evals logs <node>`: View logs for a specific evaluation node.

### mcp | search | ssh | aws
External integration and credential management.
- `mcp config|test|status`: Integrate with external AI tools via Model Context Protocol.
- `search setup|disable`: Configure Vertex AI Search for the `search_web` tool.
- `ssh setup`: Mount host SSH keys into g8ep for fleet operations.
- `aws setup`: Mount AWS credentials into g8ep for AWS-integrated tools.

---

## DETAILED HELP
For detailed flags and usage examples for any subcommand:
- `./g8e platform --help`
- `./g8e operator --help`
- `./g8e test --help`
- `./g8e demo --help`
