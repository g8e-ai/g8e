---
title: AI Agents
parent: Architecture
---

# AI Agent Architecture

The g8e platform utilizes a specialized multi-agent system designed for autonomous infrastructure management. The architecture prioritizes safety, cryptographic auditability, and human-in-the-loop control while maintaining high reasoning performance at fleet scale.

## Core Principles

- **Structural Safety**: Invariants are enforced by code models (Pydantic/LiteLLM), not just prompts.
- **Intent-Driven Execution**: Reasoning agents (Sage/Dash) never write shell commands directly; they articulate intent to the Tribunal.
- **Information Quarantine**: Agents operate in sealed environments with limited visibility into the overall pipeline to prevent collusion and prompt injection spread.
- **Ensemble Consensus**: The Tribunal uses an independent five-member ensemble to translate intent into commands, verified by an Auditor.

## The Agent Pipeline

Every user message triggers a structured pipeline that routes the request based on complexity and state.

### 1. Triage (The Gatekeeper)
Incoming messages are first processed by the **Triage Agent** (using a `lite` model tier). It performs three critical classifications:
- **Complexity**: Routes `simple` requests (status checks, basic lookups) to **Dash** and `complex` requests (fleet operations, multi-step maintenance) to **Sage**.
- **Intent**: Categorizes the goal as `information`, `action`, or `unknown`.
- **Posture**: Gauges user mindset (`normal`, `escalated`, `adversarial`, `confused`) to calibrate downstream behavior.

**Interrogation Protocol**: If a reasoning agent (**Dash** or **Sage**) requires more information, it emits clarifying questions. Tool execution is deferred until the user responds, and the question-answer pairs are persisted as structured context.

### 2. Context Enrichment
Before an agent receives the task, the system assembles a comprehensive "world view":
- **Fleet & Operator Context**: Real-time system metadata extracted from `OperatorDocument` heartbeats and inventory.
- **Investigation Context**: Conversation history, triage results, and active tool outputs.
- **Memory (Codex)**: Durable user preferences and technical background extracted asynchronously by **Codex**.

### 3. Reasoning Agents
- **Dash (Fast-path)**: A high-efficiency responder for `simple` tasks. It handles surgical tool calls or direct answers with minimal ceremony.
- **Sage (Primary Reasoner)**: The senior reasoner for `complex` tasks. It operates in a **ReAct loop**, planning operations, proposing intents, and interpreting results.

## The Tribunal: Command Translation

To prevent hallucinations and ensure safety, agents never write shell commands. Instead, they emit a `SageOperatorRequest` containing an **Operational Intent**.

### Execution Protocol
1. **Intent Articulation**: Sage describes the goal, targets, and constraints in natural language.
2. **Tribunal Generation**: Five independent members (Axiom, Concord, Variance, Pragma, Nemesis) produce candidate commands based on their specific lens (Composition, Safety, Edge Cases, Convention, Adversarial).
3. **Consensus & Voting**: Candidates are clustered and voted upon. A winner is selected based on frequency and deterministic tie-breaking.
4. **Warden Risk Analysis**: The **Warden** performs a pre-execution assessment. If a command is classified as `HIGH` risk, it triggers the **Two-Strike Circuit Breaker**.
5. **Auditor Verification**: The **Auditor** performs the final check of the winning command against the original intent. It can approve (`ok`), revise, or swap to a superior dissenter.
6. **Human-in-the-Loop**: The final command, risk assessment, and justification are presented for user approval before execution on the Operator.

## Security & Governance

### The Warden Circuit Breaker
The Warden-Sage interaction prevents infinite loops and ensures safety backpressure:
- **First Strike**: Warden blocks a `HIGH` risk command; Sage receives contextual feedback and suggestions to revise.
- **Second Strike**: If a second `HIGH` risk command is blocked in the same turn, an `AI_AGENT_CONFLICT_DETECTED` event is raised, halting the loop for human intervention.

### Reputation & Audit
- **Reputation Staking**: Tribunal members and Warden agents stake reputation on accuracy. Failures (e.g., approving a flawed command) result in reputation slashing.
- **LFAA (Local-First Audit Architecture)**: All activity is persisted in an encrypted, tamper-evident audit vault on the target host, with file mutations backed by a local Git repository.

### Information Isolation
Each component is blind to the internal state of others:
- **Triage** is unaware of downstream reasoning agents.
- **Tribunal Members** are blind to each other's candidates.
- **Auditor** sees anonymized clusters to ensure unbiased judgement.
