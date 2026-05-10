# **AI-Powered, Human-Driven Infrastructure**

Last Updated: 2026-05-07
Version: v0.2.0

**A Byzantine Fault Tolerant Architecture for Agentic Automation**

*Danny Barbour · [github.com/g8e-ai/g8e](https://github.com/g8e-ai/g8e)*

**Abstract:** We propose a distributed governance architecture for agentic infrastructure built on mutual adversarial assumption. We treat LLM-driven automation as a Byzantine Fault Tolerance (BFT) problem. The architecture utilizes a stateless reasoning Engine running a consensus protocol over isolated, heterogeneous AI personas, and a single-binary sovereign Operator that runs on managed hosts with a tamper-evident local audit ledger. Governance evidence travels with execution intent in a typed Protobuf `UniversalEnvelope`, binding event names, operator payloads, state roots, and L1/L2/L3 metadata into the same transaction. The AI is structurally prevented from auto-regressive collapse, and the human operator is elevated from a rubber-stamp supervisor to a first-class co-validator whose explicit stake is time.

## **1\. The Fallacy of the Single Agent**

The industry's current trajectory for agentic AI on infrastructure is structurally broken. Every catastrophic AI failure in production shares the same root cause: reliance on a single monolithic agent.

A single Large Language Model is a probabilistic text generator fundamentally vulnerable to auto-regressive collapse. Because it generates output sequentially based on its own preceding context, a single hallucination or bad assumption early in the reasoning chain becomes an unassailable axiom for the rest of the generation. The agent will confidently output thousands of tokens mathematically justifying its own mistake. Adding a "self-reflection" step to a single-agent loop typically results in the LLM aggressively defending its initial flawed logic.

Conversely, the **Human-in-the-Loop (HITL)** pattern attempts to retrofit safety by throwing a confirmation dialog in front of every state change. In infrastructure, this rapidly degrades into alert fatigue. Verifying an LLM's proposed bash script—understanding its side effects, checking flags, assessing blast radius—is cognitively expensive. Clicking "Approve" is cheap. The human is nominally in the loop, but the liability is simply legally shifted to a fatigued operator who inevitably rubber-stamps the output.

Trusting a single agent to mutate state is gross negligence. Trusting a fatigued human to catch the agent's subtle errors is operational suicide.

## **2\. The Reality Portal: Sovereign Execution**

SaaS-based agent architectures pull your authoritative state into their cloud. We inverted this. The execution plane is the **Operator**: a single, statically compiled 4MB Go binary ("Satellite Agent") that runs on the managed host.

In a typical agentic architecture, the execution worker is a dumb terminal that runs whatever payload the cloud orchestrator sends it. In g8e, the Operator is the reality portal. It treats the upstream AI Engine as inherently untrusted and actively expects adversarial inputs. Command and result traffic is not ad hoc JSON; it is serialized `UniversalEnvelope` bytes carrying typed `operator.proto` payloads through the pub/sub transport.

Before a single bit moves on the host OS, the Operator rejects malformed envelopes, applies protocol-level L1 checks, verifies L2 Tribunal signatures when configured, and routes the inbound payload through its **Sentinel** layer: 46 discrete MITRE ATT\&CK detectors and strict command allowlist/denylist enforcement. It executes commands in an isolated process group with a closed stdin.

The Operator requires zero inbound ports, communicating exclusively via outbound mTLS WebSockets. By leveraging a Temporal Privilege Function, it attaches just-in-time IAM scopes based on the parsed intent and drops them post-execution, ensuring zero standing privileges. Locally, the Operator doesn't just execute the platform; it *is* the platform.

## **3\. Execution is a Side-Effect of the Audit Log**

Most platforms treat auditability as a JSON log emitted after the fact. In g8e, auditability is the literal nervous system.

We utilize a **Local-First Audit Architecture (LFAA)**. Every intent, Tribunal verdict, risk assessment, and raw command output is anchored to an encrypted, Git-backed SQLite ledger on the host *before and during* execution. The AI Engine is merely a stateless relay. If the Engine burns down, your local ledger remains the mathematically verifiable truth of what happened.

### **The Economics of Alignment**

To a Staff SRE, fast AI is a threat model. If an agent generates a mutation in 800 milliseconds, it means it didn’t check the fleet history and it didn't verify implicit constraints.

When you issue a command to g8e, it might take 40 seconds to process. **This is not latency; this is async alignment compute.** The system is trading cheap machine compute to protect your non-fungible human time. During those 40 seconds, the Auditor pulls cross-conversation memory across your fleet. If an agent proposes a directory deletion on Host B, the Auditor cross-references an incident from three weeks ago on Host A where a similar operation caused an outage.

You are not babysitting an autonomous bot. You are the final co-validator in a system that has already done the exhausting, cross-contextual research you would have otherwise had to do manually.

## **4\. The BFT Control Plane: The Tribunal**

Any state-changing intent is forced through a 5-node LLM consensus panel: Axiom, Concord, Variance, Pragma, and Nemesis.

Operating under the **Information Isolation Principle**, these agents evaluate the intent in a vacuum. They are blind to one another. They cannot sycophantically agree, and they cannot be socially engineered. You can rage at the prompt or demand destructive actions; the agents will simply refuse, or the **Warden**—armed with three specialized risk-assessment sub-agents—will catch the garbage and fail the transaction closed.

Because the Tribunal generates in strict parallel, the 5-node consensus resolves in roughly one second. The fractional token overhead is immediately offset by the operational ROI of generating a mathematically ideal, highly composed multi-stage command on the first try.

**Nemesis** acts as an endogenous red team, explicitly prompted to craft flawed-but-plausible commands to trick the Warden. If it succeeds but is caught by the final Auditor, it is rewarded. We replaced external audits with continuous, mathematically bounded adversarial pressure.

## **5\. The Governance Gateway and Proof of Human Presence**

The machine handles what is machine-checkable. The human handles what is strictly human-checkable: intent fidelity, contextual stakes, and the acceptance of irreversible real-world consequences.

This is expressed by the **Co-Validation Identity**:

$$
\text{Safe}(a) \iff \sigma_{\text{machine}}(a) \land \sigma_{\text{human}}(a)
$$

Neither signature is sufficient alone. Crucially, we do not allow the human signature to be automated. The industry standard for HITL is a CLI prompt. CLI prompts can be bypassed by a tired developer writing a wrapper script with \--auto-approve.

g8e permanently disables automatic function calling. We enforce explicit friction through **Proof of Human Presence (PHP)**. The **Governance Gateway** is the only path to the human, enforced by FIDO2. It doesn't just show a UI; it records Layer 3 authorization evidence for the transaction. At the protocol layer, `UniversalEnvelope.governance.l3` can carry a human signature and public key, or an `auto_approved` flag for benign commands that have already passed L1 and L2. Auto-approval is not an execution shortcut; it is only Layer 3 authorization state.

We enforce the friction because the friction is the security boundary. The wire protocol makes that boundary explicit instead of relying on side-channel trust.

## **6\. The Receipts: Evals Over Vibes**

The AI ecosystem is currently saturated with vibes-based safety claims. We do not use LLM-as-a-judge to pat ourselves on the back.

Because our platform is built on LFAA, we did not have to invent a telemetry pipeline to measure safety. Our evaluation dataset is queried directly from the cryptographic audit ledgers that the Operator natively produces. The metadata in our evals is the exact same metadata generated in your own host's SQLite vault.

Using a deterministic BenchmarkJudge, we measure exactly how often the Nemesis successfully tricks the Warden, how often the Auditor catches it, and how accurately the Tribunal translates intent into syntax.

More importantly, our evals highlight our deadlock rate. When the Tribunal cannot reach a plurality consensus, or when the Warden flags a critical risk, the circuit breaker trips and the transaction fails closed. We do not claim 100% accuracy. We claim verifiable governance that fails gracefully.

## **7\. Closing**

Infrastructure has properties consumer AI does not. Mistakes are persistent. Blast radius is real. Compliance regimes require auditability and sovereignty.

The current generation of agent platforms cannot satisfy these constraints. Co-validation is the necessary correction. We use Byzantine Fault Tolerance to keep the AI honest, an encrypted ledger to power fleet memory, a sovereign Go binary to keep the state local, and a FIDO2 passkey to keep the human in authority.

If you are building infrastructure that AI agents will operate, build it on a zero-trust governance protocol whose enforcement metadata travels with the command. The ideas are free; the code is public.

*g8e is open-source: [github.com/g8e-ai/g8e](https://github.com/g8e-ai/g8e)*
