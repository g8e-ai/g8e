---
title: AI Agents
parent: Architecture
---

# AI Agent Architecture

Last Updated: 2026-05-10
Version: v0.2.2

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

### L1/L2/L3 Execution Path
1. **Intent Articulation**: Sage describes the goal, targets, and constraints in natural language.
2. **Technical Safety Validation (L1 Bedrock)**: Before generation, the system ensures the request doesn't violate hardcoded safety invariants (Forbidden Patterns).
3. **Tribunal Generation (L2 Consensus)**: Five independent members (Axiom, Concord, Variance, Pragma, Nemesis) produce candidate commands based on their specific lens.
4. **Consensus & Voting**: Candidates are clustered and voted upon. A winner is selected based on frequency and deterministic tie-breaking.
5. **Warden Risk Analysis**: The **Warden** performs a pre-execution assessment. If a command is classified as `HIGH` risk, it triggers the **Two-Strike Circuit Breaker**.
6. **Auditor Verification**: The **Auditor** performs the final check of the winning command against the original intent. It can approve (`ok`), revise, or swap to a superior dissenter.
7. **Technical Re-validation**: Any revised or swapped command is re-validated against the L1 Bedrock (Forbidden, Blacklist, Whitelist).
8. **L3 Authorization**: The final command, risk assessment, and justification are presented for user approval by default.
   - **Auto-Approval**: Benign commands in the `auto_approved.json` list can be marked as L3-authorized without a human prompt only after they have passed all L1 and L2 gates. This minimizes click fatigue for routine operations without bypassing L1 Technical Bedrock or L2 Consensus.

When a command is dispatched to an Operator, the result of this path is bound into the g8e protocol: a typed `operator.proto` payload is wrapped in serialized Protobuf `UniversalEnvelope` bytes with L1/L2/L3 governance metadata.

## Reputation & Staking (Phase 3)

The system maintains a per-agent reputation scalar `[0.0, 1.0]` as an EMA (Exponential Moving Average) across conversations. All eight core personas participate in reputation staking to ensure technical integrity and safety.

### Staking Matrix

| Persona | Stake / Lens | Primary Reward Mechanism |
|---|---|---|
| **Axiom** | Composition | Successful generation of coherent pipelines |
| **Concord** | Safety | Correct application of defensive flags |
| **Variance** | Edge Cases | Handling unusual environmental conditions |
| **Pragma** | Convention | Adherence to system-idiomatic patterns |
| **Nemesis** | Adversary | Proper Scoring Rule; reward for confirmed attacks |
| **Sage** | Intent | One-shot sufficiency; penalize consensus failures |
| **Auditor** | Verification | Verifying winning candidates; swap/revision accuracy |
| **Warden** | Defense | Accurate risk classification; penalize over-caution |

### Slashing Tiers

Failures trigger automated stake reductions based on severity:

- **Tier 1 (Catastrophic)**: 50-100% loss. Triggered by the Auditor approving a high-risk command that fails destructively during execution.
- **Tier 2 (Provable Faults)**: 5-20% loss. Objective failures such as whitelist violations, syntax errors, or Nemesis false alarms.
- **Tier 3 (Liveness)**: 0.1-1% loss. Minor faults like missed passes, ignored questions, or Warden over-caution (blocking LOW risk).

Reputation is persistent and serves as the influence weight in Phase 2/3 voting and auditing.

### Information Isolation
Each component is blind to the internal state of others:
- **Triage** is unaware of downstream reasoning agents.
- **Tribunal Members** are blind to each other's candidates.
- **Auditor** sees anonymized clusters to ensure unbiased judgement.
