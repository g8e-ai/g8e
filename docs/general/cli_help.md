---
title: g8e CLI
---

# g8e Platform CLI

Last Updated: 2026-05-16
Version: v0.2.5

The `g8e` command is the unified entry point for the g8e platform. The platform is built on the **g8e Protocol** as substrate; this CLI manages the reference Operator (`g8eo`) runtime by default and exposes the reference Engine app (`g8ee`) only as an optional application-layer adapter.

## Usage

Running `g8e` without arguments launches the **Interactive Platform Manager**. 

Alternatively, use direct commands for automation and specific tasks:
```bash
./g8e <command> [subcommand] [options]
```

## Core Principles

The platform is built on security-first architectural invariants that cannot be bypassed:

- **3-Layer Governance Bedrock**: Every action is gated by a hierarchical validation system: L1 (Technical Bedrock), L2 (Consensus/Tribunal), and L3 (Human Authorization).
- **Zero Trust**: No standing credentials. Privileges are ephemeral and mathematically bound to locally verifiable protocol proofs.
- **Binary Safety**: Security is enforced at the binary and network layers, not via fragile LLM prompts.
- **Data Sovereignty**: Operational data stays on the remote host; only scrubbed context reaches the AI.
- **Immutable Audit**: Git-backed ledgers and Merkle commitments provide a tamper-evident record of every change and agent verdict.
- **Air-Gap Capable**: Fully self-hosted with no SaaS dependencies or mandatory telemetry.
- **Provider Agnostic**: Swap LLM providers (Gemini, Anthropic, OpenAI, Ollama) at will. Governance is the constant.

## Substrate and Application Layer

The default substrate is the Operator plus the shared protocol. Bundled apps remain in-tree as opt-in reference adapters and must use the same public protocol surface as BYO clients.

| Layer | Component | Language | Purpose |
|-----------|-----------|----------|---------|
| **Substrate** | **Operator (g8eo)** | Go | Protocol hub, policy enforcement, execution, audit, receipts, persistence, and pub/sub in listen mode. |
| **Protocol** | **protocol/proto** | Protobuf | Canonical transaction schemas, typed payloads, and envelope contracts. |
| **Application Layer** | **g8ee** | Python | Optional reference Engine adapter for agentic proposal and L2 proof generation. |

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

1. **Ingress**: A bundled or BYO client builds a typed transaction proposal for the Operator protocol.
2. **Triage**: The message is classified as `simple` (Dash) or `complex` (Sage).
3. **L1: Technical Bedrock**: Initial scrubbing and validation against forbidden patterns (sudo, etc.).
4. **L2: Consensus (Tribunal)**: Intent is translated into commands by the ensemble. The Warden checks for risk, and the Auditor verifies technical correctness.
5. **L3: Authorization**: State-changing operations halt for human approval. Benign commands may use auto-approval if configured.
6. **Execution**: The Operator verifies protocol proofs locally, executes accepted work, and commits receipts to the host-authoritative audit ledger.

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
./g8e platform start    # Start Operator listen mode only (exposes 4 ports: 9001, 9000, 9003, 9000)
./g8e platform status   # Check service health and PIDs (shows all four endpoints)
./g8e platform logs     # Stream aggregated logs
./g8e platform settings # View or update platform configuration
./g8e apps start all    # Start optional bundled Engine adapter
```

### Operator Deployment
```bash
./g8e operator build           # Build for current architecture
./g8e operator deploy user@host # Deploy to remote host
./g8e operator stream host...  # Fleet-wide streaming deployment
```

### Testing & Development
```bash
./g8e test           # Go Operator substrate tests
./g8e test g8eo      # Go Operator substrate tests
./g8e test g8ee      # Optional Python Engine adapter tests
```

## Command Reference

### chat
- `[prompt]`: Start an interactive chat session with the AI Engine. Supports optional initial prompt.

### identity
- `login`: Authenticate and save session to `~/.g8e/credentials`
- `logout`: Clear local session and credentials

### vars
- `list`, `ls`: List all g8e environment variables and their current values
- `set <key> <value>`: Set a variable in `.g8e/.env`
- `get <key>`: Display the value of a specific variable
- `unset <key>`: Remove a variable from `.g8e/.env`

### platform
- `start [-a|--with-apps|--with-g8ee]`: Start Operator listen mode by default; optional apps require explicit opt-in
- `stop`: Stop Operator listen mode and any optional app processes
- `restart [-a|--with-apps|--with-g8ee]`: Restart Operator listen mode by default; optional apps require explicit opt-in
- `status`: Show substrate health first and optional application-layer status separately
- `reset`: Destructive. Wipes Engine data, Operator listen-mode data, and bootstrap secrets while preserving PKI material in `.g8e/pki`
- `wipe`: Clears application data via the Operator listen-mode API. Preserves platform settings, PKI material, secrets, and authentication state
- `clean`: Nuke all processes and the `.g8e` runtime directory
- `logs`: Stream logs from all components
- `settings`: Manage platform configuration (sections: general, llm, etc.)

### apps
- `start [g8ee|all]`: Start optional bundled app adapter
- `stop [g8ee|all]`: Stop optional bundled app adapter
- `restart [g8ee|all]`: Restart optional bundled app adapter
- `status`: Show optional app status alongside substrate status
- `build [g8ee|all]`: Install optional app dependencies

### operator
- `init`: Build local operator binary
- `build`: Build amd64 operator for current host
- `build-all`: Build binaries for all architectures
- `deploy <host>`: SCP/SSH deployment and launch
- `stream <host...>`: High-concurrency fleet-wide injection
- `reauth`: Request fresh operator session for a specific user
- `ssh-config`: Manage SSH identities for fleet operations

### test
- `g8eo [path]`: Go Operator substrate tests with race detection. This is the default when no component is provided.
- `g8ee [path]`: Optional Python Engine adapter tests with LLM provider support.
- `chaos [options]`: Run the g8eo Chaos Tester against the local audit stack (e.g., `--count=100`).

### security
- `validate`: Check TLS integrity and volume permissions
- `certs`: Manage platform CA and certificates (generate, rotate, status, trust)
- `passkeys`: Manage FIDO2/WebAuthn credentials
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
- `bench --suite <suite> --mode <baseline|receipt>`: Run a benchmark suite against the new harness
- `verify-receipts <report-dir>`: Re-verify receipt signatures offline
- `list`: List benchmark suites and bundled gold sets
- `run|status|deploy|down|logs`: (LEGACY) These commands have been removed in favor of `bench`.

**Receipt mode requires a running Operator and a bound `--operator-session-id`/`--operator-id`. Baseline mode runs the SUT without binding.**

#### evals workflow (new harness)
1. `./g8e evals bench --suite ifeval --mode baseline`
2. `./g8e evals bench --suite ifeval --mode receipt --operator-session-id <id> --operator-id <id>`
3. `./g8e evals verify-receipts reports/ifeval-<ts>`

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
