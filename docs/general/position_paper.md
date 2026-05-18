**Byzantine Fault Tolerance at the Execution Boundary: Securing Agentic Infrastructure**

*Danny Barbour*
*May 2026*

### Abstract

The industry is rapidly converging on standardized data pipes and routing protocols—such as Anthropic’s Model Context Protocol (MCP) and open Agent-to-Agent (A2A) standards—to connect Large Language Models (LLMs) to production environments. However, treating infrastructure mutation as a simple JSON-RPC tool call is operational malpractice. Autonomous systems are inherently vulnerable to auto-regressive collapse, hallucination, and sycophancy. In high-stakes environments, agentic execution is not a prompt engineering challenge; it is a Byzantine Fault Tolerance (BFT) problem.

This paper outlines a mutual-adversary governance architecture. We propose a strict physical separation between intent generation, communication protocols, and host execution. By forcing all state-changing payloads into a cryptographically signed, state-bound `GovernanceEnvelope`, we ensure that no autonomous system can access reality without structural consensus, host-local verification, and explicit, hardware-bound human authorization.

---

### 1. The Operational Reality of Agentic AI

Current agentic architectures optimize for capability and developer velocity. An LLM reasoning loop is given a set of executable tools, context is piped in via protocols like MCP, and the model autonomously invokes functions via JSON-RPC.

In read-only environments, this is highly effective. In state-changing infrastructure, it is a severe anti-pattern.

A single LLM is a non-deterministic probabilistic text generator. If it hallucinates a faulty assumption early in its reasoning chain, it will mathematically justify its own mistake across thousands of subsequent tokens. Retrofitting safety via a Human-in-the-Loop (HITL) confirmation dialog merely shifts liability to a fatigued operator who lacks the context to manually verify the blast radius of a generated script.

Furthermore, protocols like MCP and A2A are designed to standardize the *payload* (the context fetch or the tool execution). They do not provide execution governance. If an architecture pipes unstructured JSON-RPC directly into a root shell or a cloud API, it is operating entirely on implicit trust.

### 2. A Mutual-Adversary Architecture

To safely deploy autonomous agents, we must discard implicit trust. The system must operate on a **mutual-adversary model**, assuming that the AI control plane is compromised by default, that human operators are prone to fatigue, and that the execution environment must fail-closed.

This architecture isolates the system into four distinct components:

**I. The Principal (The Intent)**
The entity requesting the state change. Today, this is a human operator. As systems scale, the Principal will increasingly be an upstream AI agent passing a high-level request down the chain.

* **The vulnerability:** Principals provide ambiguous, context-blind, or highly dangerous instructions.
* **The constraint:** The Principal holds the hardware-bound FIDO2 passkey. The system cannot execute a mutation without this cryptographic Proof of Human Presence (PHP) anchoring the final intent.

**II. The Engine (The Orchestrator)**
The reasoning layer. Instead of a single agent, the Engine utilizes a structurally blind multi-agent ensemble (the Tribunal) operating in a ReAct loop.

* **The vulnerability:** Single agents suffer from sycophancy and auto-regressive collapse.
* **The constraint:** The Engine is strictly stateless regarding the host. It cannot execute anything. It must reach a plurality consensus across ideologically opposed agents (including a calibrated adversary) and attach an Ed25519 signature to its proposed command string before transmitting it.

**III. The Protocol (The Wire Contract)**
The governance boundary. The protocol is not a data pipe; it is an armored transport envelope (`GovernanceEnvelope`).

* **The vulnerability:** Raw tool calls lack state awareness and cryptographic proof.
* **The constraint:** The protocol binds the typed payload to a deterministic transaction hash, the L1/L2/L3 signatures, and the `state_merkle_root` of the target host.

**IV. The Operator (The Reality Portal)**
The host-side binary. The Operator is the sovereign system of record and the physical execution boundary.

* **The vulnerability:** Upstream AI or compromised transit networks injecting malicious payloads.
* **The constraint:** The Operator distrusts all inbound traffic. It enforces L1 hard-gates (e.g., regex blocking `sudo`), verifies the L2 Tribunal signature, demands the L3 Human signature, and checks the transaction's state root against the host's actual state to prevent the execution of stale context.

### 3. Payload vs. Envelope: Interoperability with MCP

This architecture does not replace context standards like MCP; it secures them.

MCP is highly effective at standardizing *what* a tool call is. But an MCP server natively executing those calls possesses no structural defense against an AI logic failure.

In a mutual-adversary architecture, MCP is treated strictly as the **Payload**. The execution substrate wraps that payload inside the **Envelope**.

When an LLM client issues a standard MCP `call_tool` request, the gateway intercepts the JSON-RPC message, maps the arguments into a base64 Protobuf payload, and seals it within the `GovernanceEnvelope`. The host Operator then forces that envelope through the L1/L2/L3 cryptographic gauntlet. If the cryptography and state-roots validate, the Operator unwraps the MCP payload and allows the native execution. If validation fails, the payload is dropped at the boundary.

This separation of concerns allows engineering teams to leverage open standards for capability discovery while enforcing Byzantine Fault Tolerance at the execution boundary.

### 4. Execution as a Side-Effect of the Audit Log

In standard SaaS deployments, auditability is a telemetry stream emitted after the fact. In a zero-trust substrate, execution is a side-effect of the audit log.

The Operator implements a Local-First Audit Architecture (LFAA). Before a single bit flips on the host, the intent, the consensus proof, and the human signature are anchored to a host-local, AES-256-GCM encrypted SQLite vault. Furthermore, every file mutation utilizes a multi-ledger two-phase commit into an isolated Git repository on the host, generating a cryptographic hash of the pre- and post-execution state.

The AI control plane only receives metadata scrubbed of credentials and PII. The authoritative, cryptographic truth of the state change never leaves the host.

### 5. The Agent Fabric Vision

We are moving away from monolithic assistants toward an "Agent Fabric"—an interconnected mesh of micro-agents delegating tasks to one another across organizational boundaries.

If this fabric relies on implicit trust and naked tool calls, the blast radius of a single failure will cascade unpredictably.

A mutual-adversary substrate ensures that no matter how deep the AI-to-AI delegation goes, the ultimate interaction with physical reality remains governed. Every action is cryptographically audited on the local host, validated against the immediate state, and definitively rolls up to a human Principal who authorized the overarching intent.

### 6. Conclusion

Infrastructure engineering requires properties that consumer AI does not. Mistakes are persistent, and the cost of an hallucinated action is real. We cannot bolt safety onto agentic systems after the fact by simply adding a confirmation button to an MCP tool call.

We must build safety into the substrate. By treating AI execution as a consensus problem, physically separating reasoning from execution, and requiring cryptographic proof of state and intent, we can deploy autonomous systems into high-stakes environments without surrendering operational sovereignty.