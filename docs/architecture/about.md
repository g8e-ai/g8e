---
title: About
parent: Architecture
---

# Origins & Architecture

Last Updated: 2026-05-10
Version: v0.2.2

g8e is the governance layer for AI-powered operations in mission-critical infrastructure. It provides the cryptographic control, threat detection, and non-bypassable human oversight needed to deploy AI into production without compromising security or handing over the keys.

## The Architecture at a Glance

The industry's current trajectory for agentic AI on infrastructure is structurally broken. Relying on a single Large Language Model creates a system vulnerable to auto-regressive collapse. Conversely, traditional "Human-in-the-Loop" setups rapidly degrade into alert fatigue, where human approval becomes a rubber-stamp. 

To solve this, g8e implements a three-tier component architecture (Operator, Dashboard, Engine) composed of four core mechanisms:

### 1. The Reality Portal: Sovereign Execution (Operator)
SaaS-based agent architectures pull your authoritative state into their cloud. We inverted this. The execution plane is the **Operator**: a statically compiled Go binary that runs on your managed host.

The Operator treats upstream AI as inherently untrusted. Command traffic isn't ad hoc JSON; it is serialized `UniversalEnvelope` bytes carrying typed Protobuf payloads. Before a single bit moves on the host OS, the Operator rejects malformed envelopes, applies protocol-level L1 checks, verifies L2 Tribunal signatures, and routes the payload through a Sentinel layer enforcing strict allowlist/denylist controls and 46 MITRE ATT&CK detectors.

### 2. The BFT Control Plane: The Tribunal (Engine)
Any state-changing intent is forced through a 5-node LLM consensus panel in the **Engine**. Operating under strict information isolation, they evaluate intent in a vacuum. Because they cannot socially engineer each other, they cannot sycophantically agree.

A calibrated adversarial agent (**Nemesis**) continuously attempts to trick the platform's risk-assessment Wardens with flawed-but-plausible commands. We replaced external audits with continuous, mathematically bounded adversarial pressure.

### 3. Execution is a Side-Effect of the Audit Log (LFAA)
In g8e, auditability is the literal nervous system. Utilizing a Local-First Audit Architecture (LFAA), every intent, Tribunal verdict, risk assessment, and raw command output is anchored to an encrypted, Git-backed SQLite ledger on the host *before and during* execution. The cloud can disappear, and your history doesn't.

### 4. Proof of Human Presence (Dashboard)
The machine handles what is machine-checkable. The human handles what is strictly human-checkable: intent fidelity, contextual stakes, and the acceptance of consequences. Facilitated by the **Dashboard**, g8e disables automatic function calling, enforcing friction through a FIDO2-backed Governance Gateway. The protocol explicitly binds this Layer 3 authorization state (`UniversalEnvelope.governance.l3`) into the payload envelope.

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
