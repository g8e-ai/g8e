# g8e

**The Governance and Audit Substrate for Agentic Infrastructure.**

A zero-trust execution substrate that forces AI tool calls into a governance envelope, physically requiring a compliant agentic system to reach structural consensus before touching reality.

If an autonomous system is mutating state (infrastructure, financials, IoT, healthcare records), it needs a structural boundary. g8e is that boundary. It treats agentic execution as a Byzantine Fault Tolerance (BFT) problem, physically separating intent generation from execution.

---

## The Mutual-Adversary Architecture

The platform operates on a strict zero-trust model. The components fundamentally distrust each other, and neither side can execute a state change without the cryptographic proof of the others. There are four distinct actors:

* **The Protocol (The Wire Contract)**
* The rules. A zero-trust UAP JSON `GovernanceEnvelope` that physically enforces human control, cryptographic signatures, and state verification. It binds typed payloads to fleet state-roots and hierarchical signatures (L1/L2/L3).


* **The Operator (The Reality Portal)**
* *Why it distrusts:* AI agents hallucinate; humans get fatigued and rubber-stamp prompts.
* *What it does:* The host-side binary (e.g., `g8eo`) running on remote satellites. It is the ultimate fail-closed reality portal. It rejects any command lacking an L2 structural consensus signature or L3 human authorization. It enforces L1 hard-gates (blocking `sudo`), scrubs egress data via Sentinel, and executes the action, writing an immutable record to a local Git/SQLite ledger (LFAA).


* **The Engine (The Orchestrator)**
* *Why it distrusts:* Single-agent LLMs suffer auto-regressive collapse. User prompts are often ambiguous, context-blind, or dangerous.
* *What it does:* The reference AI application (e.g., `g8ee`). It uses a ReAct loop and 13 specialized agents to fulfill intent. It forces the system to reach *structural consensus* (via a blind Tribunal and an internal calibrated adversary) to generate the cryptographic proofs demanded by the Operator's envelope.


* **The Principal (The Intent)**
* *Why it distrusts:* The machine will confidently output a mathematically perfect script that destroys production.
* *What it does:* The entity requesting the action. Today, this is a human user holding a hardware-bound FIDO2 passkey, refusing to sign the envelope until the machine proves safety. Tomorrow, it is an upstream AI agent passing a markdown request.



---

## Payload vs. Envelope (MCP / A2A Integration)

g8e does not replace data pipes or schemas like Anthropic’s Model Context Protocol (MCP) or open A2A routing standards. It acts as the mandatory security perimeter for them.

* **MCP is the Payload:** It defines *what* the tool call or context fetch is.
* **g8e is the Envelope:** We wrap that MCP JSON-RPC payload in our BFT `GovernanceEnvelope`.

**Bidirectional Translation:**
Principals connecting via an MCP Gateway do not need to understand the underlying cryptographic substrate:

1. **Ingress:** The Principal sends a standard MCP tool call.
2. **Translation & Consensus:** The Engine interprets the intent, forces its internal agentic system to reach structural consensus, and wraps the MCP payload in the `GovernanceEnvelope` with the required cryptographic proofs.
3. **Verification:** Before execution, the Operator verifies the envelope's L1/L2/L3 signatures and fleet state-root. If the math checks out, it unwraps the payload and executes.
4. **Egress:** The Operator emits a Sentinel-scrubbed receipt. The Engine translates this back into standard MCP and returns it to the Principal.

You can bring any MCP application; g8e forces it to become Byzantine Fault Tolerant.

---

## The Agent Fabric Vision

This protocol is built for the future of agentic swarms. As systems scale, a Principal will not just be a single human—it will be an interconnected fabric of AI systems asking each other to fulfill tasks.

g8e is the substrate that runs underneath that fabric. It ensures that no matter how deep the AI-to-AI communication goes, every single action that touches the real world is cryptographically audited locally, rigidly governed by structural consensus, and ultimately rolls up to a human Principal who authorized the overarching intent.

---

## Quick Start

Prerequisites: Go and `curl` available on the host. Python 3.12+ (for the optional AI Engine adapter).

```bash
git clone https://github.com/g8e-ai/g8e.git && cd g8e

# Start the Operator hub and optional AI Engine
./g8e platform start --with-apps

```

1. **Trust the platform CA** on your workstation:
* macOS / Linux: `curl -fsSL http://localhost:8080/trust | sudo sh`


2. **Register a passkey** at `https://localhost:9000`.
3. **Generate a device-link token** via the CLI: `./g8e login`.
4. **Deploy the Operator** to any host you want to manage:
```bash
curl -fsSL http://<hub>:8080/g8e | sh -s -- <device-link-token>

```



---

## Documentation Reference

| Document | Description |
| --- | --- |
| [Position Paper](docs/architecture/position_paper.md) | The thesis on BFT Agentic AI and the Agent Fabric. |
| [Protocol](docs/architecture/protocol.md) | The UAP JSON `GovernanceEnvelope` wire contract. |
| [Governance](docs/architecture/governance.md) | Structural consensus, L1/L2/L3 validation, and the Tribunal. |
| [Security](docs/architecture/security.md) | Sentinel egress scrubbing, LFAA multi-ledger audit, and mTLS. |
| [Operator](docs/architecture/operator.md) | Execution boundary, modes, deployment, and on-host storage. |

**License:** Apache 2.0

**Built by:** [Lateralus Labs, LLC](https://lateraluslabs.com) — *A Certified Veteran-Owned Small Business.*