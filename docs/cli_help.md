---
title: g8e CLI
---

# g8e Platform CLI

Last Updated: 2026-05-19
Version: v0.2.6

The `g8e` command is the unified entry point for the g8e platform. The platform is built on the **g8e Protocol**; this CLI manages the Protocol + Local Operator (`g8eg`) by default and exposes the reference **g8e Agentic Ensemble** (`g8ee`) only as an optional application-layer adapter.

## Usage

Running `g8e` without arguments launches the **Interactive Platform Manager** with category-based submenus:

- **Gateway (g8eg)**: Platform lifecycle commands
- **Operator (g8eo)**: Remote operator deployment and fleet management
- **Apps (g8ee)**: Optional application-layer adapters
- **Protocol & Infrastructure**: Authentication, security, data management, and integration tools
- **Testing & Evaluation**: Unit tests, fleet simulation, and benchmark evaluation

Alternatively, use direct commands for automation and specific tasks:
```bash
./g8e <command> [subcommand] [options]
```

## Core Principles

The platform is built on security-first architectural invariants that cannot be bypassed:

- **3-Layer Governance Bedrock**: Every action is gated by a hierarchical validation system: Doctrine (L1Doctrine) Technical Bedrock, Quorum (L2Consensus) Consensus/Tribunal, and Notary (L3Notary) Human Authorization.
- **Zero Trust**: No standing credentials. Privileges are ephemeral and mathematically bound to locally verifiable protocol proofs.
- **Binary Safety**: Security is enforced at the binary and network layers, not via fragile LLM prompts.
- **Data Sovereignty**: Operational data stays on the remote host; only scrubbed context reaches the AI.
- **Immutable Audit**: Git-backed ledgers and Merkle commitments provide a tamper-evident record of every change and agent verdict.
- **Air-Gap Capable**: Fully self-hosted with no SaaS dependencies or mandatory telemetry.
- **Provider Agnostic**: Swap LLM providers (Gemini, Anthropic, OpenAI, Ollama) at will. Governance is the constant.

## Gateway and Application Layer

The default platform is the Local Operator plus the shared protocol. Bundled apps remain in-tree as opt-in reference adapters and must use the same public protocol surface as BYO clients.

| Layer | Component | Language | Purpose |
|-----------|-----------|----------|---------|
| **Protocol (Local)** | **g8eg** | Go | **Local Operator**: Protocol hub, CA/PKI, persistence, and pub/sub broker in listen mode. |
| **Protocol (Remote)**| **g8eo** | Go | **Remote Operator**: Sovereign host execution (Actuator), local git ledger, and MCP Server. |
| **Protocol** | **protocol/proto** | Protobuf | Canonical transaction schemas, typed payloads, and envelope contracts. |
| **Application Layer** | **g8ee** | Python | Optional reference **g8e-compliant agentic ensemble** adapter for agentic proposal and L2Consensus proof generation. |

### Agent Terminology

The **g8e Agentic Ensemble** uses specialized agents with distinct roles:

- **Triage**: The initial classifier that determines complexity, intent, and user posture.
- **Dash**: High-efficiency responder for simple, single-step requests.
- **Sage**: Senior reasoning agent for complex, multi-step investigations and command orchestration.
- **Tribunal**: 5-member **agentic ensemble** (Axiom, Concord, Variance, Pragma, Nemesis) that translates Sage's intent into hardened shell commands through consensus.
- **Actuator**: Defensive circuit breaker that performs risk assessment. Triggers a two-strike lockout on repeated high-risk detections.
- **Auditor**: The final technical gatekeeper that verifies Tribunal output against Sage's intent and manages agent reputation.

## The Request Lifecycle

A user request moves through the **3-Layer Governance Bedrock**:

1. **Ingress**: A bundled or BYO client builds a typed transaction proposal for the Operator protocol.
2. **Triage**: The message is classified as `simple` (Dash) or `complex` (Sage).
3. **Doctrine (L1Doctrine): Technical Bedrock**: Initial scrubbing and validation against forbidden patterns (sudo, etc.).
4. **Quorum (L2Consensus): Consensus (Tribunal)**: Intent is translated into commands by the ensemble. The Actuator checks for risk, and the Auditor verifies technical correctness.
5. **Notary (L3Notary): Authorization**: State-changing operations halt for human approval. Benign commands may use auto-approval if configured.
6. **Execution**: The Governed Operator (`g8eo`) verifies protocol proofs locally, executes accepted work, and commits receipts to the host-authoritative audit ledger.

## Operational Modes

### Operator Bound Mode
When at least one g8eo operator is connected and bound to the web session:
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
./g8e setup             # Configure local environment (Protocol + Local Operator vs **g8e Agentic Ensemble**)
./g8e platform start    # Start Local Operator (g8eg) in listen mode (default ports: <!-- g8e:port:operator_http -->8440<!-- /g8e:port --> multiplexed TLS, <!-- g8e:port:operator_bootstrap -->8441<!-- /g8e:port --> plain Bootstrap)
./g8e platform status   # Check service health and PIDs (shows all four endpoints)
./g8e platform logs     # Stream aggregated logs
./g8e platform settings # View or update platform configuration
./g8e apps start g8ee   # Start optional reference **g8e-compliant agentic ensemble** app
```

### Operator Deployment
```bash
./g8e operator build           # Build for current architecture
./g8e operator deploy user@host # Deploy to remote host
./g8e operator stream host...  # Fleet-wide streaming deployment
```

### Testing & Development
```bash
./g8e test           # Remote Operator tests
./g8e test g8eo      # Remote Operator tests
./g8e test g8ee      # Optional Python Ensemble adapter tests
```

## Command Reference

### Gateway (g8eg)
The Gateway is the local Protocol hub that runs in listen mode, managing CA/PKI, persistence, and pub/sub broker.

**platform** (Gateway lifecycle)
- `start [-a|--with-g8ee]`: Start Gateway (g8eg) in listen mode by default; optional **g8e Agentic Ensemble** (g8ee) requires explicit opt-in
- `stop`: Stop Gateway and any optional app processes
- `restart [-a|--with-g8ee]`: Restart Gateway listen mode by default; optional **g8e Agentic Ensemble** (g8ee) requires explicit opt-in
- `status`: Show Gateway health first and optional **g8e Agentic Ensemble** status separately
- `reset`: Destructive. Wipes Ensemble data, Gateway listen-mode data, and bootstrap secrets while preserving PKI material in `.g8e/pki` (prompts for confirmation; bypass with `-y`, `--yes`, or `--force`)
- `clean`: Nuke all processes and the `.g8e` runtime directory (prompts for confirmation; bypass with `-y`, `--yes`, or `--force`)
- `logs`: Stream logs from all components
- `settings`: Manage platform configuration (sections: general, llm, etc.)

### Operator (g8eo)
The Operator is the sovereign host execution (Actuator), local git ledger, and MCP Server that runs on remote hosts.

**operator** (Remote Operator lifecycle)
- `init`: Build local operator binary
- `build`: Build amd64 operator for current host
- `build-all`: Build binaries for all architectures
- `deploy <host>`: SCP/SSH deployment and launch
- `stream <host...>`: High-concurrency fleet-wide injection
- `reauth`: Request fresh operator session for a specific user
- `ssh-config`: Manage SSH identities for fleet operations

### Apps (g8ee)
Apps are optional application-layer adapters that use the public protocol surface. The reference app is g8ee (Python Agentic Ensemble).

**apps** (Optional app lifecycle)
- `start [g8ee]`: Start optional reference **g8e-compliant agentic ensemble** app
- `stop [g8ee]`: Stop optional reference **g8e-compliant agentic ensemble** app
- `restart [g8ee]`: Restart optional reference **g8e-compliant agentic ensemble** app
- `status`: Show optional g8ee status alongside Gateway status
- `build [g8ee]`: Install optional **g8e-compliant agentic ensemble** dependencies

**chat** (Interactive interface)
- `[prompt]`: Start an interactive web session with the **g8e Agentic Ensemble**. Supports optional initial prompt.

### Protocol & Infrastructure
Protocol-level commands for authentication, configuration, security, and data management.

**identity** (Authentication)
- `login [--email <email>] [--count <n>] [--ttl <seconds>]`: Authenticate and save operator session to `~/.g8e/credentials`. In sandbox mode `--email` is optional and defaults to the bootstrap superuser (`superadmin@g8e.local`); pass it explicitly only to switch to a non-default user. Optional count (default 1) and TTL (default 3600).
- `logout`: Clear local operator session and credentials

**vars** (Environment variables)
- `list`, `ls`: List all g8e environment variables and their current values
- `set <key> <value>`: Set a variable in `.g8e/.env`
- `get <key>`: Display the value of a specific variable
- `unset <key>`: Remove a variable from `.g8e/.env`

**security** (Security validation)
- `validate`: Check TLS integrity and volume permissions
- `passkeys`: Manage FIDO2/WebAuthn credentials
- `mtls-test`: Verify mTLS connectivity

**data** (Data management)
- `users|operators`: Query or modify user and operator documents
- `store <collection> list|get`: Access the SQLite-based blob store
- `settings`: Low-level platform configuration management
- `audit`: View LFAA audit logs
- `device-links`: Manage device link tokens

**llm** (LLM configuration)
- `setup`: Interactive provider configuration
- `show|get|set`: View or update LLM variables
- `restart`: Restart Ensemble to apply settings

**mcp** (Model Context Protocol)
Generates configs for and interacts with the Operator MCP translation gateway.

Usage:
  `./g8e mcp config`      - Generate an IDE-compatible mcpServers configuration block
  `./g8e mcp status`      - Check the health of the MCP gateway
  `./g8e mcp test`        - Run test tools/list and tools/call requests against the operator
  `./g8e mcp serve`       - Start the MCP stdio gateway and proxy requests to the operator via mTLS

**Integration Tools**
- `search`: Vertex AI Search configuration (setup, disable)
- `ssh`: Manage host SSH key mounts
- `aws`: Manage AWS credential mounts

### Testing & Evaluation
Testing and evaluation tools for the substrate and applications.

**test** (Unit tests)
- `g8eo [path]`: Remote Operator tests with race detection. Run `./g8e test g8eo -h` for unique options. This is the default when no component is provided.
- `g8ee [path]`: Optional Python Ensemble adapter tests with LLM provider support. Run `./g8e test g8ee -h` for unique options.
- `ci`: Run all CI workflow steps locally (proto verify, lint, vulncheck, Operator tests, app tests). Run `./g8e test ci -h` for details.
- `chaos [options]`: Run the g8eo Chaos Tester against the local audit stack. Run `./g8e test chaos -h` for options.

**demo** (Fleet simulation)
- `deploy [-n <count>] -d <token>`: Start and authenticate a simulated fleet of N devices
- `down`: Stop all simulation nodes
- `status`: View container status and node counts
- `clean`: Forcefully remove all demo artifacts
- `profile [list|switch]`: Manage demo scenarios (e.g., acme-corp, nginx)
- `shell <node>`: Drop into a simulation node's shell
- `devices|broken`: List discovered or unhealthy devices
- `operators`: Show status of g8e operator processes in the fleet

**To start a demo, use `deploy -d <token>`. This will automatically bring up the fleet and authenticate the operators.**

**evals** (Benchmark evaluation)
- `bench --suite <suite> --mode <baseline|receipt>`: Run a benchmark suite against the new harness
- `verify-receipts <report-dir>`: Re-verify receipt signatures offline
- `list`: List benchmark suites and bundled gold sets

**Receipt mode requires a running Operator and an authenticated CLI session. Baseline mode runs the SUT without binding.**

#### evals workflow (new harness)
1. `./g8e login` (zero-arg in sandbox; mints CLI mTLS cert + session)
2. `./g8e evals bench --suite ifeval --mode baseline`
3. `./g8e evals bench --suite ifeval --mode receipt`
4. `./g8e evals verify-receipts reports/ifeval-<ts>`

**NOTE: G8E_OPERATOR_SESSION_ID is loaded from `~/.g8e/credentials` automatically after `./g8e login`. Do not pass `--operator-session-id` unless you are explicitly overriding the cached session - passing the wrong UUID (e.g. `G8E_OPERATOR_ID`) is rejected with a hard error.**

### setup
- `[--quick|--advanced]`: Launch the Environment Setup wizard.
  - `quick`: Set up Protocol + Gateway (Listen Mode) or Protocol + Gateway + **g8e Agentic Ensemble** (g8ee).
  - `advanced`: Configure custom paths and external provider settings.

## Detailed Help

For detailed flags and usage examples:
```bash
./g8e platform --help
./g8e operator --help
./g8e test --help
./g8e demo --help
```
