---
title: About
parent: Architecture
---

# Origins, Governance, and Philosophy

g8e is the governance layer for AI-powered operations in mission-critical infrastructure. It provides the cryptographic control, threat detection, and non-bypassable human oversight needed to deploy AI into production without compromising security or handing over the keys.

## Origins: Building the Governor with the Governed

g8e was built from scratch with the help of AI coding agents — the very kind of agents this platform is designed to govern. 

The entire stack (Go, Python, Node.js, SQLite, React, Docker) was written, tested, and shipped by leveraging AI as a force multiplier. But we never let it drive unsupervised. We experienced firsthand the sheer power of LLM-assisted engineering, alongside the terrifying reality of what happens when an AI confidently proposes a destructive command or silently hallucinates a dependency.

We realized that to safely use AI to manage infrastructure, we needed a dedicated platform that assumes the AI is simultaneously brilliant and dangerous. A platform where AI is treated as an untrusted advisory input, not a trusted administrative user. 

That platform is g8e.

## The Vision

The problem in modern operations isn't a shortage of tooling—it's the friction between intent and execution, compounded by the massive risk of giving autonomous agents write access to production.

g8e is designed to bridge that gap by providing a fully self-hosted, air-gap capable platform with zero cloud dependencies. You run the platform on your hardware, using your preferred LLM, maintaining absolute control over your data and your environment.

## Core Principles

Every architectural decision in g8e is an expression of these eight core principles:

1.  **AUTHORITY** — Every action is gated by FIDO2. Human judgment is the final, non-bypassable security layer. AI proposes, but humans always decide.
2.  **TRUST** — Zero standing credentials. Privileges are earned per-action, mathematically bound to sessions, and expire automatically.
3.  **STRUCTURE** — Safety is enforced at the binary and network layers, not via fragile LLM prompts. Prompt injection cannot bypass cryptographic execution constraints.
4.  **SOVEREIGNTY** — You own your data. The remote host is the system of record; raw operational data never leaves the host, and AI only receives scrubbed context.
5.  **PRESENCE** — A tiny, outbound-only binary with no inbound ports and no dependencies. It leaves no footprint and vanishes the moment the process is killed.
6.  **AUDIT** — Accountability lives where execution happens. Encrypted, local, git-backed ledgers provide an immutable record of every change without needing a cloud backend.
7.  **ISOLATION** — Fully self-hosted and air-gap capable. No SaaS dependencies, no mandatory telemetry, and no "phone-home" behavior. You hold the keys.
8.  **AGNOSTIC** — Swap models, providers, or infrastructure at will. Governance is the constant; the choice of intelligence is yours.

## Human Agency is the Point

If you give an AI an API key with write access to AWS, you no longer control your infrastructure—the AI's prompt does. Prompt engineering is not a security boundary.

g8e is the governance layer between intent and production execution. It stands between your environment and the AI, ensuring safe and responsible deployments in real production infrastructure where humans stay in control. 

It is not an AI assistant—it is an AI defense force that securely binds LLM reasoning to reality. It never runs autonomously, never bypasses human judgment, never increases your inbound attack surface, and leaves only an encrypted audit trail behind.

---

g8e is developed by [Lateralus Labs, LLC](https://lateraluslabs.com), a Certified Veteran Owned Small Business (VOSB) dedicated to AI governance and safety research.
