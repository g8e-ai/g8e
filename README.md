# g8e

**Byzantine Fault Tolerant (BFT) Governance Substrate for Agentic Infrastructure.**

g8e is a zero-trust execution substrate that forces AI tool calls into a governance envelope. It physically separates intent generation from execution, requiring a compliant agentic system to reach structural consensus before mutating state.

---

## Technical Architecture

The platform operates on a strict zero-trust model where components distrust each other. State changes require cryptographic proof of consensus.

*   **The Protocol (Wire Contract)**: A zero-trust [GovernanceEnvelope](protocol/proto/common.proto) (protojson) that enforces human control, cryptographic signatures (L2/L3), and state verification (L1).
*   **The Operator (Substrate)**: The host-resident binary (`g8eo`) running in `--listen` mode. It is the fail-closed execution boundary. It rejects commands lacking L2 structural consensus or L3 human authorization, enforces L1 hard-gates, and writes an immutable audit ledger (LFAA).
*   **The Engine (Optional App)**: A reference AI orchestrator (`g8ee`) that fulfills intent via a ReAct loop. It generates the required cryptographic proofs by forcing its internal agents to reach structural consensus via a blind Tribunal.
*   **The Principal (Intent)**: The entity requesting the action (e.g., a human via WebAuthn/Passkey or an upstream AI agent).

---

## Payload vs. Envelope (MCP / A2A)

g8e does not replace tool-calling protocols like Anthropic's Model Context Protocol (MCP) or A2A standards; it provides the mandatory security perimeter for them.

*   **MCP is the Payload**: Defines *what* the tool call or context fetch is.
*   **g8e is the Envelope**: Wraps the MCP JSON-RPC payload in a BFT `GovernanceEnvelope`.

Any MCP application can be integrated; g8e forces it to become Byzantine Fault Tolerant by requiring external validation before execution.

---

## Governance Hierarchy

1.  **L1: Technical Bedrock**: Hard-coded gates (e.g., forbidden patterns like `sudo`) and state-root verification.
2.  **L2: Consensus (Tribunal)**: Agentic consensus where multiple independent agents must sign off on the intent.
3.  **L3: Authorization (Human)**: Human-in-the-loop via WebAuthn signatures. Benign diagnostic commands can be auto-approved via policy.

---

## Quick Start

Prerequisites: Go 1.22+, Python 3.12+ (for optional Engine).

```bash
git clone https://github.com/g8e-ai/g8e.git && cd g8e

# Start the mandatory Operator substrate
./g8e platform start

# (Optional) Start the reference AI Engine
./g8e apps start g8ee
```

1.  **Bootstrap**: Follow the CLI instructions to initialize the operator and generate a device-link token.
2.  **Login**: `./g8e login` to authenticate the CLI via mTLS.
3.  **Audit**: View real-time transaction logs in `.g8e/logs/operator-listen.log`.

---

## Documentation

*   [**Protocol Substrate**](docs/protocol/README.md): Wire format, transaction hashes, and L1/L2/L3 definitions.
*   [**Operator (g8eo)**](docs/g8eo/README.md): Execution boundary, listener modes, and host storage.
*   [**Engine (g8ee)**](docs/g8ee/README.md): Reference AI application and agentic orchestration.
*   [**Developer Guide**](docs/developer/README.md): Build instructions and testing workflows.

**License**: Apache 2.0
**Built by**: [Lateralus Labs](https://lateraluslabs.com)