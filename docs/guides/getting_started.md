---
title: Getting Started
parent: Guides
---

# Getting Started

Last Updated: 2026-06-02
Version: v1.0.8

---

## Protocol Overview

g8e is a zero-trust execution platform for agentic infrastructure. The platform enforces a core invariant: a typed, signed, state-bound transaction reaches a sovereign host agent that distrusts upstream inputs and refuses to mutate reality unless every independent proof checks out.

The platform consists of two mandatory components:

### g8e Gateway

The g8e Gateway serves as the central Policy Decision Point (PDP). It provides:

- **PKI and Trust Management**: Acts as the platform Certificate Authority, issuing and revoking mTLS certificates bound to URI SANs for workload identity.
- **Persistence Layer**: Maintains the canonical state store via SQLite, including user accounts, Operator registrations, and governance state. Implements incremental state tracking for efficient state root calculation.
- **Messaging Broker**: Serves as the Pub/Sub broker for real-time event fan-out between clients and operators.
- **Admission APIs**: Exposes HTTP endpoints for envelope submission and trust bundle distribution.
- **Protocol Translation**: Translates standard MCP (Model Context Protocol) and A2A (Agent-to-Agent) requests into canonical JSON GovernanceEnvelope format via a unified endpoint architecture.

The g8e Gateway runs in one of three modes, each enforcing different layers of the 5-layer verification sequence:

- **Doctrine Mode**: Enforces L1Doctrine (technical bedrock: forbidden patterns, blacklist, whitelist). L2/L3 signatures not required.
- **Consensus Mode**: Enforces L1Doctrine and L2Consensus (multi-model Byzantine consensus). L3 signature not required.
- **Notary Mode**: Enforces L1Doctrine, L2Consensus, and L3Notary (human-in-the-loop via WebAuthn/FIDO2). L4Warden and L5Actuator are always active for execution. This is the most secure mode.

### g8e Operator

The g8e Operator serves as the Policy Execution Point (PEP) running on target hosts. It provides:

- **Fail-Closed Execution**: Executes mutations only through L5Actuator, the single dispatch path that enforces L1/L2/L3 verification gates.
- **Outbound-Only Connectivity**: Dials out to the Gateway via mTLS reverse tunnel, exposing no inbound ports or remote attack surface.
- **Local-First Audit**: Writes all audit entries to a host-local Git-backed vault before execution, preserving raw data and forensic context on the host.
- **MCP Server**: Exposes tools to standard local clients as a Model Context Protocol server for AI agent integration. Includes 13 native tools for database operations, filesystem analysis, network diagnostics, process management, and system monitoring.
- **State Binding**: Verifies transaction state roots and enforces replay defense before executing any mutation.

The g8e Operator distrusts all upstream inputs. It validates every envelope independently, checks cryptographic proofs, and refuses execution if any verification fails.

### Protocol Flow

1. **Client Submission**: An AI client submits a mutation request via MCP or A2A protocol to the Gateway.
2. **Envelope Construction**: The g8e Gateway wraps the request in a canonical JSON GovernanceEnvelope with typed payload, transaction hash, nonce, expiry, and state root.
3. **Verification Sequence**: The platform enforces the 5-layer verification sequence:
   - L1Doctrine: Forbidden patterns, blacklist, whitelist checks.
   - L2Consensus: Multi-model Byzantine consensus signatures (in consensus/notary modes).
   - L3Notary: Human-in-the-loop approval via WebAuthn (in notary mode).
   - L4Warden: Pre-dispatch integrity and state-root verification.
   - L5Actuator: Sovereign execution boundary and signed receipt issuance.
4. **Dispatch**: Verified envelopes are dispatched over the Pub/Sub broker to target g8e Operators.
5. **Execution**: The g8e Operator receives the envelope, re-verifies all proofs, and executes the mutation through L5Actuator.
6. **Receipt**: The g8e Operator emits a signed receipt and writes an audit entry to the local Git-backed vault.

---

## Quick Start

**Prerequisites:** Go 1.26+ · (Optional) Python 3.14+ for agent ensembles.

The platform is a single g8e Node. No runtime, no interpreter, no sidecar.

### Quick launch (pre-built binaries)

**Quick launch (Linux)**

```bash
curl -fsSL https://g8e.ai/g8e-linux-amd64 -o g8e && chmod +x g8e && ./g8e gw start
```

**Quick launch (macOS)**

```bash
curl -fsSL https://g8e.ai/g8e-darwin-amd64 -o g8e && chmod +x g8e && ./g8e gw start
```

**Quick launch (Windows)**

```powershell
iwr https://g8e.ai/g8e-windows-amd64.exe -outf g8e.exe; .\g8e.exe gw start
```

**Deploy operators to remote hosts via SSH**

```bash
# Using your existing SSH config, deploy Operators across your fleet
./g8e Operator deploy --hosts host1,host2,host3

# Tool calls accept a list of hosts for simultaneous fan-out execution
```

### Build from source

### 1. Build g8e

```bash
git clone https://github.com/g8e-ai/g8e.git && cd g8e
make build
```

This produces the `g8e` g8e Node. It is self-contained and manages both g8e Gateway (PDP) and g8e Operator (PEP) roles.

**Build Options:**
- `make build`: Standard build (~35-38MB per platform)
- `make build-compressed`: Compressed build (~15-17MB for Linux/Windows AMD64/ARM64, ~35-38MB for macOS and Windows ARM64)

### 2. Start the g8e Gateway

Start the sovereign g8e Gateway (PDP) in **Doctrine Mode** (L1Doctrine enforced). This bootstraps the stateless gateway with PKI, persistence, and pub/sub:

```bash
./g8e gw start
```

### 3. Authenticate CLI to g8e Gateway

Authenticate the CLI to the running g8e Gateway. This bootstraps the PKI hierarchy and issues your initial mTLS credentials. Required before any CLI or Operator can connect:

```bash
./g8e auth login
```

Credentials and trust material are stored in `.g8e/pki` and `.g8e/secrets` in the project directory.

### 4. Deploy an Operator

For remote host enforcement, use CSR-based enrollment:

```bash
./g8e security pki enroll -e <gateway-ip>
```

---

## 5-Layer Verification Sequence

g8e enforces a hierarchical defense-in-depth model:

| Layer | Name | Mechanism | Enforcement |
| :--- | :--- | :--- | :--- |
| **L1** | **Technical Bedrock** | Pattern matching, allow/denylists | Hard gate (always active) |
| **L2** | **Consensus** | Multi-model BFT agreement | Consensus/Notary modes |
| **L3** | **Notary** | Human-in-the-loop (WebAuthn) | Notary mode |
| **L4** | **Warden** | Pre-dispatch verification | Always active |
| **L5** | **Actuator** | Execution boundary | Always active |

## Protocol Integration

### MCP (Model Context Protocol)
Connect AI agents to the g8e Operator's toolset. The g8e Gateway translates JSON-RPC requests into signed GovernanceEnvelope before execution. The unified MCP endpoint architecture provides a single entry point for all MCP operations with comprehensive input validation for security hardening.

### A2A (Agent-to-Agent)
g8e Gateway-mediated communication between sovereign agents. Every interaction is state-bound and audit-logged to the local Git ledger.

---

## Post-Bootstrap Actions

After successful bootstrap, verify the platform and begin integration:

### Verify Platform Status

```bash
./g8e gw status
```

### Explore Available Commands

```bash
./g8e --help
./g8e gw --help
./g8e security --help
./g8e data --help
```

### Configure Remote Operators (Multi-Host Setups)

For distributed enforcement across multiple hosts:

```bash
./g8e security pki enroll -e <gateway-ip>
```

See [Connect Operator to Gateway](./connect_operator_to_gateway.md) for detailed enrollment steps.

### Review Audit Trail

Query the local audit vault to verify g8e governance enforcement:

```bash
./g8e data query --collection audit_vault
```

---

## Integration Guides

- **[MCP Protocol](../protocols/mcp/mcp.md)** — Connect AI clients via Model Context Protocol
- **[A2A Protocol](../protocols/a2a/a2a.md)** — Agent-to-agent communication patterns
- **[Connect Apps to Gateway](./connect_apps_to_gateway.md)** — Integrate application-layer adapters
- **[Native Tools](../architecture/operator.md#native-tool-execution)** — Database triage, log digestion, process governance

---

## Governance Configuration

The g8e Gateway operates in three security postures:

- **Doctrine Mode** (default): L1Doctrine enforced, L2/L3 audited
- **Consensus Mode**: L1Doctrine/L2Consensus enforced, L3 audited
- **Notary Mode**: L1Doctrine/L2Consensus/L3Notary strictly enforced

Configure posture via `./g8e gw start --posture doctrine`, `--posture consensus`, or `--posture notary`.

---

## Deep Dive Documentation

- **[Architecture](../devs/codemap.md)** — Platform component structure
- **[Operator Reference](../architecture/operator.md)** — Execution boundary and verification sequence
- **[Security Model](../architecture/auth.md)** — PKI, mTLS, and WebAuthn details
- **[CLI Reference](../devs/devs.md)** — Comprehensive command documentation
