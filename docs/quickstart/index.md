# Quickstart

**Prerequisites:** Go 1.26+ (required) · Python 3.14+ (optional, only for g8e-compatible agentic ensembles)

## 1. Get the Code

```bash
git clone https://github.com/g8e-ai/g8e.git && cd g8e
```

## 2. Start the Governance Gateway

The Governance Gateway (`g8eg`) acts as the central Policy Decision Point (PDP) and cryptographic backplane.

```bash
# Start the Gateway in default doctrine mode (L1 enforced, L2/L3 audited)
./g8e platform start
```

1. **Bootstrap** — Follow the CLI prompts to initialize the PKI hierarchy and Gateway state.
2. **Login** — Run `./g8e login` to authenticate your CLI session via mTLS.
3. **Audit** — Watch live transaction logs stream to `.g8e/logs/operator-listen.log`.

## 3. Start a g8e Operator on a Remote Host

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

## 4. Use Gateway as an MCP / A2A Protocol Translator

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

## 5. Use Gateway as an MCP / A2A Gateway with Operators

To execute MCP/A2A operations securely across distributed infrastructure, combine the Gateway translator with remote Operators:

1. **Start the Gateway** (`./g8e platform start`).
2. **Connect Remote Operators** (as shown in Step 3), which act as downstream execution targets.
3. **Dispatch**: When an AI client issues an MCP tool call or A2A skill invocation to the Gateway, the Gateway verifies the transaction (L1 Doctrine, L2 Consensus, L3 Notary). Once verified, the Gateway constructs a typed action (e.g., `McpCallRequested`) and dispatches it over the Pub/Sub broker to the designated remote Operator for execution.

This creates a zero-trust execution substrate where AI clients seamlessly interact with standard MCP tools, while all execution is cryptographically bound, verified, and audited by the Governance Gateway before reaching the remote host.

<!-- ============================================================= -->
<!-- INSERT: SCREENSHOT — `./g8e platform start` running, with the -->
<!-- live audit log streaming a couple of transactions. Proves     -->
<!-- it's real and self-hosted. -->
<!-- ============================================================= -->

> *Insert screenshot of the running Operator + live audit log here.*
