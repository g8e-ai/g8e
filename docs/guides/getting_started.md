---
title: Getting Started
parent: Guides
---

# Getting Started

Last Updated: 2026-05-25
Version: v1.0.0

---

## Protocol Overview

g8e is a zero-trust execution substrate for agentic infrastructure. The platform enforces a core invariant: a typed, signed, state-bound transaction reaches a sovereign host agent that distrusts upstream inputs and refuses to mutate reality unless every independent proof checks out.

The substrate consists of two mandatory components:

### Governance Gateway (g8eg)

The Governance Gateway serves as the central Policy Decision Point (PDP). It provides:

- **PKI and Trust Management**: Acts as the platform Certificate Authority, issuing and revoking mTLS certificates bound to URI SANs for workload identity.
- **Persistence Layer**: Maintains the canonical state store via SQLite, including user accounts, device-link tokens, operator registrations, and governance state.
- **Messaging Broker**: Serves as the Pub/Sub broker for real-time event fan-out between clients and operators.
- **Admission APIs**: Exposes HTTP endpoints for envelope submission, device-link enrollment, and trust bundle distribution.
- **Protocol Translation**: Translates standard MCP (Model Context Protocol) and A2A (Agent-to-Agent) requests into canonical JSON GovernanceEnvelope format.

The Gateway runs in one of three modes, each enforcing different layers of the 3-Layer Governance model:

- **Doctrine Mode**: Enforces L1 technical bedrock (forbidden patterns, blacklist, whitelist). L2 and L3 are audited but not enforced.
- **Consensus Mode**: Enforces L1 and L2 (multi-model Byzantine consensus). L3 is audited but not enforced.
- **Notary Mode**: Enforces L1, L2, and L3 (human-in-the-loop via WebAuthn/FIDO2). This is the most secure mode.

### Governed Operator (g8eo)

The Governed Operator serves as the Policy Execution Point (PEP) running on target hosts. It provides:

- **Fail-Closed Execution**: Executes mutations only through the Actuator, the single dispatch path that enforces L1/L2/L3 verification gates.
- **Outbound-Only Connectivity**: Dials out to the Gateway via mTLS reverse tunnel, exposing no inbound ports or remote attack surface.
- **Local-First Audit**: Writes all audit entries to a host-local Git-backed vault before execution, preserving raw data and forensic context on the host.
- **MCP Server**: Exposes tools to standard local clients as a Model Context Protocol server for AI agent integration.
- **State Binding**: Verifies transaction state roots and enforces replay defense before executing any mutation.

The Operator distrusts all upstream inputs. It validates every envelope independently, checks cryptographic proofs, and refuses execution if any verification fails.

### Protocol Flow

1. **Client Submission**: An AI client submits a mutation request via MCP or A2A protocol to the Gateway.
2. **Envelope Construction**: The Gateway wraps the request in a canonical JSON GovernanceEnvelope with typed payload, transaction hash, nonce, expiry, and state root.
3. **Verification Gauntlet**: The Gateway enforces the 3-Layer BFT verification:
   - L1 (Technical Bedrock): Forbidden patterns, blacklist, whitelist checks.
   - L2 (Consensus): Multi-model Byzantine consensus signatures (in consensus/notary modes).
   - L3 (Authorization): Human-in-the-loop approval via WebAuthn (in notary mode).
4. **Dispatch**: Verified envelopes are dispatched over the Pub/Sub broker to target Operators.
5. **Execution**: The Operator receives the envelope, re-verifies all proofs, and executes the mutation through the Actuator.
6. **Receipt**: The Operator emits a signed receipt and writes an audit entry to the local Git-backed vault.

---

## Quick Start

**Prerequisites:** Go 1.26+ (required) · Python 3.14+ (optional, only for g8e-compatible agentic ensembles)

### 1. Get the Code

```bash
git clone https://github.com/g8e-ai/g8e.git && cd g8e
```

### 2. Build the Binaries

```bash
make build
```

This produces `g8e.gateway` (Governance Gateway) and `g8e.operator` (Governed Operator) binaries.

### 3. Start the Governance Gateway

```bash
./g8e platform start
```

This starts the Gateway in doctrine mode (L1 enforced, L2/L3 audited). Follow the CLI prompts to initialize the PKI hierarchy and Gateway state.

### 4. Authenticate

```bash
./g8e login
```

This generates an mTLS client certificate and stores it in `.g8e/pki/client.crt`.

### 5. Start a Governed Operator

For local development:

```bash
./g8e operator start --gateway-url https://localhost:8440
```

For remote deployment, generate a device-link token on the Gateway:

```bash
./g8e auth device-link create --name "prod-db-node"
```

Then start the Operator on the remote host with the token:

```bash
./g8e.operator start --gateway-url https://<gateway-ip>:8440 --device-token <token>
```

### 6. Use as Protocol Translator

The Gateway automatically translates MCP and A2A requests. AI clients can connect directly to the Gateway's HTTP API:

```bash
curl -X POST https://localhost:8440/api/mcp/v1/tools/call \
  --cert .g8e/pki/client.crt \
  --key .g8e/pki/client.key \
  -H "Content-Type: application/json" \
  -d '{"tool": "shell.execute", "arguments": {"command": "ls -la"}}'
```

For stdio-based MCP (required by editors like Cursor or Claude Code):

```bash
./g8e --mcp-serve
```

---

## Next Steps

- **[Build Gateway](build_gateway.md)** — Build a custom g8e-compatible Governance Gateway.
- **[Connect Apps to Gateway](connect_apps_to_gateway.md)** — Connect to, authenticate, use, maintain, and pull reports from a Gateway.
- **[Build Operator](build_operator.md)** — Build a custom g8e-compatible Governed Operator.
- **[Connect Operator to Gateway](connect_operator_to_gateway.md)** — Deploy and use a Governed Operator.
- **[Build Apps](build_apps.md)** — Build g8e-compatible applications using a Gateway.
