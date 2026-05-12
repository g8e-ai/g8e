---
title: About
parent: Architecture
---

# g8e: The Governed Transaction Layer

g8e is a governed transaction runtime for agentic systems. It provides a data-sovereign, AI-agnostic governance substrate between humans, AI agents, and production infrastructure.

The platform is split into two distinct tiers:

1.  **Substrate (Mandatory)**: The **Operator (g8eo)** and the **g8e Protocol**. The Operator is a sovereign host agent that verifies, executes, and audits transactions. It is the authority for host state and policy.
2.  **Application Layer (Optional)**: Reference adapters like the **Dashboard (g8ed)** and **Engine (g8ee)**. These are opt-in components that consume the public protocol surface on equal footing with Bring-Your-Own (BYO) frontends and agents.

The core product invariant is that a typed, signed, state-bound transaction reaches a sovereign host agent that distrusts upstream inputs and refuses to mutate reality unless every independent proof (L1/L2/L3) checks out.

---

# Origins & Architecture

Last Updated: 2026-05-12
Version: v0.2.4

For thirty years, my entire world has been managing and protecting data across remote systems... unstructured, structured, blob - nfs, smb, https, s3, ssh - linux, unix, windows - wan, lan... and all bits and pieces of the business side in-between - security reviews/audits, sales cycles, painful conversations with customers, on-site visits, RCAs, mission-critical service design... but one thing I hang my hat on is knowing all the people who I took that particular burden away from, so they can just get on with their other jobs and lives.

I spent so much time on remote calls putting out fires in production with people who were mostly checked out while working on other stuff - people have shit to do and are multi-tasking - "I have the expert on the phone who has that thing under control, let me just work on this other thing with a deadline."

My mom would say, 'treat people the way you want to be treated'... I ask myself, how would I want to be helped in this situation? I've been in those folks' shoes - vendor on the call, deadline looming, people arm-grabbing you... your production storage array is down, hours before a major company event, and those who can't do anything about it are panicking.

I want someone to just fix it for me, with receipts, so I can just forward an email to management when we got off the call. So, that's how I operate. If it has an operating system, I get stuff to work on it - applications, network, data, whatever... and show my work - for the humans with real lives and families, who are counting on me to help with some of their biggest challenges at work, and those people usually don't REALLY give a shit about it, they're just doing it to feed their families and pay rent.

The best way that I can help people is typically by gathering as much context as I possibly can - directly on their systems, while asking high signal questions, heavily leaning on those meticulous notes for grounding. I would ask if they mind if I drove - 90+% of people were cool with it - most were stoked... I'd be typing away under their credentials while they worked on other things.

Why trust me? I was the person that their company policy required them to escalate to, and we both wanted the same exact outcome - so our incentives were nearly perfectly aligned.

I wanted folks to have a guy like me in their pocket, powered by safe and reliable AI, not rely on anyone else - ever.

So, I built a sovereign and agnostic system of highly incentivized AI agents to safely, securely, and reliably work like me; in a react loop - gathering as much context as possible from remote systems and user, converging over the ideal next steps, proposing (with justification) to the person with the most at stake before state changes. Once that person approves, cleanly execute, prove it's working, and follow-up end-to-end.

That's g8e... if you look deeper, g8e is a data sovereign and AI agnostic governance layer between the human, AI, and real world devices. The current Operator binary is just my reference implementation in Go, but the Operator could be anything that speaks the g8e protocol (soon MCP and ADA via translation).

I know this workflow applies to much more than SRE / infrastructure.

If you work in an industry that could use a fully self-hosted, data sovereign, AI-provider agnostic, 'leave only footprints' way to deliver AI into messy production environments, IoT devices, etc - please join me. I would love to see some outside PRs or Discussions. I want smart people to join me and don't give a flying fuck about formalities... if you care about safe, responsible AI, I want to help you or partner with you.

Hit me up: danny@g8e.ai


## The Architecture at a Glance

The industry's current trajectory for agentic AI on infrastructure is structurally broken. Relying on a single Large Language Model creates a system vulnerable to auto-regressive collapse. Conversely, traditional "Human-in-the-Loop" setups rapidly degrade into alert fatigue, where human approval becomes a rubber-stamp. 

To solve this, g8e centers the product boundary on an Operator/protocol substrate, with optional Dashboard and Engine adapters around four core mechanisms:

### 1. The Reality Portal: Sovereign Execution (Substrate)
SaaS-based agent architectures pull your authoritative state into their cloud. We inverted this. The execution plane is the **Operator**: a statically compiled Go binary that runs on your managed host and provides the **mandatory substrate** for the platform.

The Operator treats all upstream inputs as inherently untrusted. Command traffic isn't ad hoc JSON; it is serialized `GovernanceEnvelope` bytes carrying typed Protobuf payloads. Before a single bit moves on the host OS, the Operator rejects malformed envelopes, applies protocol-level L1 checks, verifies L2 Consensus signatures, and routes the payload through a Sentinel layer enforcing strict allowlist/denylist controls and 46 MITRE ATT&CK detectors.

### 2. The BFT Control Plane: Governance (Application Layer)
Any state-changing intent can be evaluated by a consensus panel (e.g., the bundled 5-node **Engine** adapter). Operating under strict information isolation, they evaluate intent in a vacuum. Because they cannot socially engineer each other, they cannot sycophantically agree.

A calibrated adversarial agent (**Nemesis**) continuously attempts to trick the platform's risk-assessment Wardens with flawed-but-plausible commands. We replaced external audits with continuous, mathematically bounded adversarial pressure.

### 3. Execution is a Side-Effect of the Audit Log (LFAA)
In g8e, auditability is the literal nervous system. Utilizing a Local-First Audit Architecture (LFAA), every intent, Tribunal verdict, risk assessment, and raw command output is anchored to an encrypted, Git-backed SQLite ledger on the host *before and during* execution. The cloud can disappear, and your history doesn't.

### 4. Proof of Human Presence (Application-Layer Approval)
The machine handles what is machine-checkable. The human handles what is strictly human-checkable: intent fidelity, contextual stakes, and the acceptance of consequences. A bundled Dashboard or BYO frontend can collect approval UX, but the protocol explicitly binds this Layer 3 authorization state (`GovernanceEnvelope.governance.l3`) into the payload envelope for Operator-side verification. No "internal" trust shortcuts exist for bundled components.

---

## Origins: Building the Governor

g8e was built from scratch with the help of AI coding agents — the very kind of agents this platform is designed to govern. 

The entire stack (Go, Python, Node.js, SQLite, React, Docker) was architected, tested, and shipped by leveraging AI as a force multiplier. But we never let it drive unsupervised. We experienced firsthand the sheer power of LLM-assisted engineering, alongside the terrifying reality of what happens when an AI confidently proposes a destructive command or silently hallucinates a dependency. 

We realized that to safely use AI to manage infrastructure, we needed a dedicated platform that assumes the AI is simultaneously brilliant and dangerous. A platform where AI is treated as an untrusted advisory input, not a trusted administrative user.

## About the Architect

g8e was designed and developed by **Danny Barbour**, founder of [Lateralus Labs, LLC](https://lateraluslabs.com), a Certified Veteran-Owned Small Business (VOSB) and SAM.gov registered federal contractor. 

The instinct to design for adversarial conditions—including those inside an LLM—wasn't born in a lab. It was earned by standing on the bridge of a Navy vessel watching complex systems fail under pressure, and later serving as the lead on-call engineer responsible for global enterprise backup environments where failure to restore simply wasn't an option. 

When you build systems where "down" means disaster, you don't bolt governance on at the end. You build it into the wire protocol. The g8e architecture reflects a career spent bridging the gap between cutting-edge systems engineering and zero-trust, TS-eligible, mission-critical infrastructure operations.

*Danny is currently open to new opportunities, consulting, and systems/platform engineering roles. If your team is tackling AI governance, distributed systems, or mission-critical infrastructure, [reach out to build defensible AI infrastructure together](mailto:danny@g8e.ai).*
