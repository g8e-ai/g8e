---
title: Getting Started
parent: Guides
---

# Getting Started

Last Updated: 2026-05-25
Version: v0.2.6

---

## Quickstart

**Prerequisites:** Go 1.26+ (required) · Python 3.14+ (optional, only for g8e-compatible agentic ensembles)

### 1. Get the Code

```bash
git clone https://github.com/g8e-ai/g8e.git && cd g8e
```

### 2. Start the Governance Gateway

The Governance Gateway (`g8eg`) acts as the central Policy Decision Point (PDP) and cryptographic backplane.

```bash
# Start the Gateway in default doctrine mode (L1 enforced, L2/L3 audited)
./g8e platform start
```

1. **Bootstrap** — Follow the CLI prompts to initialize the PKI hierarchy and Gateway state.
2. **Login** — Run `./g8e login` to authenticate your CLI session via mTLS.
3. **Audit** — Watch live transaction logs stream to `.g8e/logs/operator-listen.log`.

### 3. Start a g8e Operator on a Remote Host

The Governed Operator (`g8eo`) is the Policy Execution Point (PEP) running on target hosts.

1. **Generate a Device Link Token** on your Gateway:
   ```bash
   ./g8e auth device-link create --name "prod-db-node"
   ```
2. **Launch the Operator** on the remote host, pointing back to your Gateway's endpoint:
   ```bash
   # On the remote host
   ./g8e -e <gateway-ip> -D <your-token>
   ```

### 4. Use Gateway as an MCP / A2A Protocol Translator

The Gateway natively functions as a universal protocol translator. It intercepts standard JSON-RPC tool calls (MCP) and HTTP/JSON requests (A2A), wraps them in a canonical JSON `GovernanceEnvelope`, and enforces the 3-Layer BFT verification gauntlet.

```bash
# The Gateway automatically listens for MCP/A2A traffic on the mTLS API port
./g8e platform start --http-listen-port 8440
```

AI clients can connect directly to the Gateway's HTTP API surface (e.g., `https://localhost:8440/api/mcp/v1/tools/call`) using standard protocols. The Gateway translates these requests into governed operations without the client needing to understand the underlying g8e protocol.

For local editor integrations (like Cursor or Claude Code) that require stdio-based MCP, use the `--mcp-serve` flag. This spins up a local proxy that forwards stdio JSON-RPC calls to the Gateway's mTLS API:

```bash
./g8e --mcp-serve
```

### 5. Use Gateway as an MCP / A2A Gateway with Operators

To execute MCP/A2A operations securely across distributed infrastructure, combine the Gateway translator with remote Operators:

1. **Start the Gateway** (`./g8e platform start`).
2. **Connect Remote Operators** (as shown in Step 3), which act as downstream execution targets.
3. **Dispatch**: When an AI client issues an MCP tool call or A2A skill invocation to the Gateway, the Gateway verifies the transaction (L1 Doctrine, L2 Consensus, L3 Notary). Once verified, the Gateway constructs a typed action (e.g., `McpCallRequested`) and dispatches it over the Pub/Sub broker to the designated remote Operator for execution.

This creates a zero-trust execution substrate where AI clients seamlessly interact with standard MCP tools, while all execution is cryptographically bound, verified, and audited by the Governance Gateway before reaching the remote host.

---

## Detailed Setup

The following sections provide detailed operational steps for deploying and using the g8e platform.

---

## Platform Architecture Overview

g8e is a substrate-first platform with two mandatory components and optional application adapters:

- **Governance Gateway (g8eg)** — Mandatory Policy Decision Point (PDP). Runs in Gateway mode (--doctrine, --consensus, or --notary). Provides PKI, persistence, messaging, and admission APIs.
- **Governed Operator (g8eo)** — Mandatory Policy Execution Point (PEP). Runs on target hosts. Executes mutations through fail-closed L1/L2/L3 verification gates and serves as an MCP server.
- **g8e-Compatible Applications** — Optional producers and consumers. Examples include the reference agentic ensemble (g8ee) and dashboard (g8ed).

The substrate enforces the core invariant: a typed, signed, state-bound transaction reaches a sovereign host agent that distrusts upstream inputs and refuses to mutate reality unless every independent proof checks out.

---

## Prerequisites

### System Requirements
- **Go 1.26+** — Required for building the Gateway and Operator binaries.
- **Python 3.14+** — Optional, only required for g8e-compatible agentic ensembles.
- **OpenSSL** — Required for PKI operations and mTLS certificate generation.
- **Git** — Required for the audit vault's Git-backed commit history.

### Network Requirements
- **Local deployment:** All components communicate over localhost or a local network.
- **Remote deployment:** Operators dial out to the Gateway via mTLS. No inbound ports are required on the Operator.
- **Air-gap:** Build dependencies must be cached on a connected machine; runtime requires zero internet connectivity.

---

## Installation

### 1. Clone the Repository

```bash
git clone https://github.com/g8e-ai/g8e.git && cd g8e
```

### 2. Build the Binaries

The Gateway and Operator are compiled from a single Go codebase:

```bash
make build
```

This produces:
- `g8e.gateway` — The Governance Gateway binary
- `g8e.operator` — The Governed Operator binary

The binaries are statically linked and require no runtime dependencies.

### 3. Initialize the Platform Runtime

```bash
./g8e platform init
```

This creates the `.g8e` directory structure:
- `.g8e/pki/` — PKI hierarchy (CA, certificates, keys)
- `.g8e/data/` — SQLite database for Gateway persistence
- `.g8e/logs/` — Operator and Gateway logs
- `.g8e/secrets/` — Encrypted vault for platform secrets

---

## Starting the Governance Gateway

The Gateway acts as the central Policy Decision Point. Start it in the appropriate mode for your use case:

### Doctrine Mode (Default)

Enforces L1 technical bedrock (forbidden patterns, blacklist, whitelist). L2 and L3 are audited but not enforced.

```bash
./g8e platform start --mode doctrine
```

### Consensus Mode

Enforces L1 and L2 (multi-model Byzantine consensus). L3 is audited but not enforced.

```bash
./g8e platform start --mode consensus
```

### Notary Mode

Enforces L1, L2, and L3 (human-in-the-loop via WebAuthn/FIDO2). This is the most secure mode.

```bash
./g8e platform start --mode notary
```

### Gateway Ports

The Gateway exposes four logical surfaces:

| Surface | Port (default) | Auth | Purpose |
|---|---|---|---|
| **Bootstrap** | 8441 (plain HTTP) | None | Trust bundle download, device-link enrollment, CSR signing |
| **Public Port** | 8442 (TLS) | Web session | Browser login, WebAuthn challenge, PKI discovery |
| **mTLS API + Pub/Sub** | 8440 (TLS) | mTLS + URI SAN | `/api/governance/envelope`, `/db`, `/ws/pubsub` |

Ports can be customized via CLI flags or environment variables.

---

## Authenticating to the Gateway

### CLI Authentication

The CLI uses mTLS for authentication. Generate a client certificate:

```bash
./g8e login
```

This:
1. Generates a CSR (Certificate Signing Request)
2. Submits it to the Gateway's bootstrap endpoint
3. Receives a signed client certificate
4. Stores it in `.g8e/pki/client.crt`

### Browser Authentication

For web-based interactions (dashboard, device-link enrollment), the Gateway supports WebAuthn/FIDO2:

1. Navigate to `https://localhost:8442` (or your configured public port)
2. Follow the on-screen prompts to register a security key
3. Use the key for subsequent authentication

---

## Deploying Operators

Operators run on target hosts and execute mutations. They dial out to the Gateway via mTLS.

### Local Operator

For development or single-host deployments:

```bash
./g8e operator start --gateway-url https://localhost:8440
```

### Remote Operator

For distributed infrastructure:

1. **Generate a Device Link Token** on the Gateway:
   ```bash
   ./g8e auth device-link create --name "prod-db-node"
   ```

2. **Copy the binary and token** to the remote host.

3. **Start the Operator** on the remote host:
   ```bash
   ./g8e.operator start --gateway-url https://<gateway-ip>:8440 --device-token <token>
   ```

The Operator will:
- Establish an outbound-only mTLS tunnel to the Gateway
- Subscribe to command events on the Pub/Sub broker
- Execute mutations through the L1/L2/L3 gauntlet
- Write audit entries to the local Git-backed vault

---

## Using the Gateway as a Protocol Translator

The Gateway natively translates standard MCP and A2A protocols into governed operations.

### MCP (Model Context Protocol)

AI clients can connect to the Gateway's MCP endpoint:

```bash
# The Gateway listens for MCP traffic on the mTLS API port
curl -X POST https://localhost:8440/api/mcp/v1/tools/call \
  --cert .g8e/pki/client.crt \
  --key .g8e/pki/client.key \
  -H "Content-Type: application/json" \
  -d '{"tool": "shell.execute", "arguments": {"command": "ls -la"}}'
```

For stdio-based MCP (required by some editors like Cursor or Claude Code):

```bash
./g8e --mcp-serve
```

This spins up a local proxy that forwards stdio JSON-RPC calls to the Gateway's mTLS API.

### A2A (Agent-to-Agent)

A2A skill invocations are similarly translated:

```bash
curl -X POST https://localhost:8440/api/a2a/v1/skills/execute \
  --cert .g8e/pki/client.crt \
  --key .g8e/pki/client.key \
  -H "Content-Type: application/json" \
  -d '{"skill": "file.read", "parameters": {"path": "/etc/hosts"}}'
```

The Gateway wraps these requests in a canonical JSON `GovernanceEnvelope`, enforces the 3-Layer BFT verification gauntlet, and dispatches verified payloads to downstream Operators.

---

## Verifying Installation

### Check Gateway Status

```bash
./g8e platform status
```

This reports:
- Gateway process status
- Listening ports
- PKI hierarchy status
- Connected Operators

### Check Operator Status

```bash
./g8e operator status
```

This reports:
- Operator process status
- Gateway connection status
- Subscription status
- Local audit vault health

### Run Substrate Tests

```bash
./g8e test g8eo
```

This runs the Operator test suite, verifying core functionality including:
- Pub/Sub command dispatch
- Audit vault writes
- Ledger commits
- L1/L2/L3 verification gates

---

## Next Steps

- **[Air-Gap Deployment](air_gap.md)** — Deploy g8e in environments with zero internet connectivity.
- **[g8e-Compatible Applications](g8e-compatible-apps.md)** — Build conforming producers and consumers of the protocol.
- **[Troubleshooting](troubleshooting.md)** — Resolve common setup and operational issues.
- **[Developer Guidelines](../devs.md)** — Contribute to the platform or extend it with custom components.

---

## Security Considerations

### PKI and mTLS

- The Gateway acts as the platform's Certificate Authority (CA).
- All inter-component traffic uses mTLS with ECDSA P-384 certificates.
- Client certificates are bound to URI SANs for workload identity.
- Certificate revocation is enforced at the Gateway boundary.

### Fail-Closed Execution

- The Operator executes mutations only through the Actuator, the single fail-closed dispatch path.
- Any failure in L1, L2, or L3 results in a typed rejection and audit entry.
- No fallback paths or silent retries exist.

### Local-First Audit

- All audit entries are written to a host-local Git-backed vault before execution.
- Raw data, forensic context, and execution history never leave the host.
- Only Sentinel-scrubbed projections cross the wire to the Gateway.

### Outbound-Only Operators

- Operators open an mTLS reverse tunnel to the Gateway and listen on nothing.
- No inbound ports, NAT traversal, or remote attack surface on the execution boundary.
- This enables governed execution on unreachable or firewalled hosts.
