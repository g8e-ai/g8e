---
title: g8e CLI
---

# g8e Platform CLI

Last Updated: 2026-05-11
Version: v0.2.3

The `g8e` command is the unified entry point for the g8e AI governance platform. It orchestrates the full lifecycle of a self-hosted, human-in-the-loop AI operations system.

## Usage

Running `g8e` without arguments launches the **Interactive Platform Manager**. 

Alternatively, use direct commands for automation and specific tasks:
```bash
./g8e <command> [subcommand] [options]
```

## Core Principles

The platform is built on security-first architectural invariants that cannot be bypassed:

- **3-Layer Governance Bedrock**: Every action is gated by a hierarchical validation system: L1 (Technical Bedrock), L2 (Consensus/Tribunal), and L3 (Human Authorization).
- **Zero Trust**: No standing credentials. Privileges are ephemeral and mathematically bound to sessions via mTLS and internal auth tokens.
- **Binary Safety**: Security is enforced at the binary and network layers, not via fragile LLM prompts.
- **Data Sovereignty**: Operational data stays on the remote host; only scrubbed context reaches the AI.
- **Immutable Audit**: Git-backed ledgers and Merkle commitments provide a tamper-evident record of every change and agent verdict.
- **Air-Gap Capable**: Fully self-hosted with no SaaS dependencies or mandatory telemetry.
- **Provider Agnostic**: Swap LLM providers (Gemini, Anthropic, OpenAI, Ollama) at will. Governance is the constant.

## Component Architecture

The platform consists of exactly three components running natively on the host:

| Component | Language | Purpose |
|-----------|----------|---------|
| **g8ed** | Node.js | Dashboard & API Gateway. Authentication, session management, SSE relay, and operator lifecycle. |
| **g8ee** | Python | Reasoning Engine. Orchestrates AI agents and enforces the 3-Layer Governance Bedrock. |
| **Operator** | Go | Persistence & Pub/Sub. When running in `--listen` mode, it acts as the platform's storage and event bus. |

### Agent Terminology

The AI reasoning engine uses specialized agents with distinct roles:

- **Triage**: The initial classifier that determines complexity, intent, and user posture.
- **Dash**: High-efficiency responder for simple, single-step requests.
- **Sage**: Senior reasoning agent for complex, multi-step investigations and command orchestration.
- **Tribunal**: 5-member ensemble (Axiom, Concord, Variance, Pragma, Nemesis) that translates Sage's intent into hardened shell commands through consensus.
- **Warden**: Defensive circuit breaker that performs risk assessment. Triggers a two-strike lockout on repeated high-risk detections.
- **Auditor**: The final technical gatekeeper that verifies Tribunal output against Sage's intent and manages agent reputation.

## The Request Lifecycle

A user request moves through the **3-Layer Governance Bedrock**:

1. **Ingress**: `g8ed` authenticates the session and relays the request to `g8ee`.
2. **Triage**: The message is classified as `simple` (Dash) or `complex` (Sage).
3. **L1: Technical Bedrock**: Initial scrubbing and validation against forbidden patterns (sudo, etc.).
4. **L2: Consensus (Tribunal)**: Intent is translated into commands by the ensemble. The Warden checks for risk, and the Auditor verifies technical correctness.
5. **L3: Authorization**: State-changing operations halt for human approval via the dashboard. Benign commands may use auto-approval if configured.
6. **Execution**: Approved commands execute on the operator. Results are scrubbed and committed to the git ledger.

## Operational Modes

### Operator Bound Mode
When at least one g8eo operator is connected and bound to the session:
- Full tool suite: command execution, file operations, web search.
- Human-in-the-loop: All state-changing operations require explicit approval.
- Multi-operator support: AI selects targets per command; batch operations fan out with unified approval.

### Advisory Mode
When no operator is connected:
- Limited tools: `search_web` only.
- No execution: AI provides guidance and suggested commands but cannot act on infrastructure.

## Platform Lifecycle

### Daily Operations
```bash
./g8e platform start    # Start g8ed, g8ee, and Operator listen mode
./g8e platform status   # Check service health and PIDs
./g8e platform logs     # Stream aggregated logs
./g8e platform settings # View or update platform configuration
```

### Operator Deployment
```bash
./g8e operator build           # Build for current architecture
./g8e operator deploy user@host # Deploy to remote host
./g8e operator stream host...  # Fleet-wide streaming deployment
```

### Testing & Development
```bash
./g8e test g8ee      # Python backend tests
./g8e test g8ed      # Dashboard tests
./g8e test g8eo      # Go operator tests
```

## Command Reference

### identity
- `login`: Authenticate and save session to `~/.g8e/credentials`
- `logout`: Clear local session and credentials

### vars
- `list`, `ls`: List all g8e environment variables and their current values
- `set <key> <value>`: Set a variable in `.g8e/.env`
- `get <key>`: Display the value of a specific variable
- `unset <key>`: Remove a variable from `.g8e/.env`

### platform
- `start`: Start all platform components (g8ed, g8ee, Operator)
- `stop`: Stop all platform components
- `restart`: Restart all platform components
- `status`: Show component health, versions, and PIDs
- `reset`: Reset application data (preserves SSL)
- `wipe`: Clear app data while preserving platform settings and SSL
- `clean`: Nuke all processes and the `.g8e` runtime directory
- `logs`: Stream logs from all components
- `settings`: Manage platform configuration (sections: general, llm, etc.)

### operator
- `init`: Build local operator binary
- `build`: Build amd64 operator for current host
- `build-all`: Build binaries for all architectures
- `deploy <host>`: SCP/SSH deployment and launch
- `stream <host...>`: High-concurrency fleet-wide injection
- `reauth`: Request fresh operator session for a specific user
- `ssh-config`: Manage SSH identities for fleet operations

### test
- `g8ee [path]`: Python tests with LLM provider support
- `g8ed [path]`: Vitest dashboard and API tests
- `g8eo [path]`: Go operator tests with race detection

### security
- `validate`: Check TLS integrity and volume permissions
- `certs`: Manage platform CA and certificates (generate, rotate, status, trust)
- `passkeys`: Manage FIDO2/WebAuthn credentials
- `rotate-internal-token`: Refresh shared secret between components
- `mtls-test`: Verify mTLS connectivity

### data
- `users|operators`: Query or modify user and operator documents
- `store <collection> list|get`: Access the SQLite-based blob store
- `settings`: Low-level platform configuration management
- `audit`: View LFAA audit logs
- `device-links`: Manage device link tokens

### llm
- `setup`: Interactive provider configuration
- `show|get|set`: View or update LLM variables
- `restart`: Restart inference engine to apply settings

### demo
- `deploy [-n <count>] -d <token>`: Start and authenticate a simulated fleet of N devices
- `down`: Stop all simulation nodes
- `status`: View container status and node counts
- `clean`: Forcefully remove all demo artifacts
- `profile [list|switch]`: Manage demo scenarios (e.g., acme-corp, nginx)
- `shell <node>`: Drop into a simulation node's shell
- `devices|broken`: List discovered or unhealthy devices
- `operators`: Show status of g8e operator processes in the fleet

**To start a demo, use `deploy -d <token>`. This will automatically bring up the fleet and authenticate the operators.**

### evals
- `deploy -d <token>`: Start and authenticate eval operators with a dashboard-issued device link token
- `run --gold-set <path>`: Execute benchmark against web-session-bound eval operators
- `list`: List available evaluation scenarios
- `down|status|logs`: Manage the evaluation fleet

**Eval operators must be manually bound to your web session in the dashboard before they can be used for benchmarking. This ensures a human is present during execution.**

#### evals workflow
1. `./g8e evals deploy -d <token>`
2. **Open the Dashboard and bind the eval operators to your session**
3. `./g8e evals run --gold-set <path>`

### Integration Tools
- `mcp`: Model Context Protocol integration (config, test, status)
- `search`: Vertex AI Search configuration (setup, disable)
- `ssh`: Manage host SSH key mounts
- `aws`: Manage AWS credential mounts

## Detailed Help

For detailed flags and usage examples:
```bash
./g8e platform --help
./g8e operator --help
./g8e test --help
./g8e demo --help
```
