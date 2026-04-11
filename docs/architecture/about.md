# About g8e

g8e is the governance layer for AI-powered operations in mission-critical infrastructure. It provides the control, safety, and oversight needed to deploy AI into production without compromising security or human authority.

## The Vision

The problem in modern operations isn't a shortage of tooling—it's the friction between intent and execution. g8e is designed to bridge that gap by providing a fully self-hosted, air-gap capable platform with zero cloud dependencies. You run the platform on your hardware, using your preferred LLM, maintaining absolute control over your data and your environment.

## Core Principles

Every architectural decision in g8e is an expression of these eight core principles:

1.  **Human Authority** — AI proposes, you decide. Nothing executes without your explicit approval. Human judgment is the security model.
2.  **Earned Authority** — Standing trust is a liability. Trust is scoped to sessions, earned per-action, and never self-granted. Execution and authorization are separated by design.
3.  **Layered Enforcement** — Governance is enforced at every boundary (Sentinel, Tribunal, approval, audit) so a failure at one layer doesn't compromise the platform.
4.  **Source Available** — Security through obscurity is false security. All enforcement logic and threat detection patterns are readable, auditable, and criticizable by anyone.
5.  **Local-First Audit** — An append-only, encrypted audit trail is maintained at the site of execution. Accountability lives where the action happened.
6.  **Data Sovereignty** — Sensitive data is scrubbed before any AI sees it. Raw output never leaves the operator; only sanitized context crosses component boundaries.
7.  **Minimal Footprint** — Outbound-only, no root required, and zero dependencies. A single 4MB process that vanishes the second you kill it.
8.  **Universal Runtime** — Any model, any provider, any OS. Governance is the constant; your infrastructure and AI choices are yours.

## Human Agency is the Point

g8e is the governance layer between intent and production execution. It stands between your environment and AI, ensuring safe and responsible deployments in real production infrastructure where humans stay in control. It's not an AI assistant—it's an AI defense force that safely binds AI to reality. It never runs autonomously, never increases attack surface, and leaves only footprints.

g8e is developed by [Lateralus Labs, LLC](https://lateraluslabs.com), a Certified Veteran Owned Small Business (VOSB) dedicated to AI governance and safety research.
