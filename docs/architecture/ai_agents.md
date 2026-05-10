---
title: AI Agents
parent: Architecture
---

# AI Agent Architecture

Last Updated: 2026-05-10
Version: v0.2.2

The g8e platform utilizes a specialized multi-agent system designed for autonomous infrastructure management. The architecture prioritizes safety, cryptographic auditability, and human-in-the-loop control while maintaining high reasoning performance at fleet scale.

## Core Principles

- **3-Layer Governance Bedrock**: Every action is gated by a hierarchical validation system (L1 Technical Bedrock, L2 Consensus, L3 Authorization).
- **Intent-Driven Execution**: Reasoning agents (Sage/Dash) never write shell commands directly; they articulate natural language intent to the Tribunal.
- **Ensemble Consensus**: The Tribunal uses an independent five-member ensemble with unique technical "lenses" to translate intent into commands.
- **Cryptographic Auditability**: The Auditor binds every verdict to a signed Merkle commitment of the agent reputation scoreboard.
- **Interrogation Gate**: Agents can pause execution to ask clarifying questions via structured `<interrogation>` blocks, preventing "guessing" when context is missing.

## The Agent Pipeline

Every user message triggers a structured pipeline managed by the `ChatPipelineService`.

### 1. Triage (The Gatekeeper)
Incoming messages are first processed by the **Triage Agent** (`lite` model tier). It emits a `TriageResult` with three classifications:
- **Complexity**: `simple` requests route to **Dash**; `complex` requests route to **Sage**.
- **Intent**: Categorizes the goal as `information`, `action`, or `unknown`.
- **Posture**: Gauges user mindset (`normal`, `escalated`, `adversarial`, `confused`) to calibrate downstream behavior.

### 2. Context Assembly
Before a reasoning agent receives the task, the system assembles a comprehensive world view:
- **Fleet & Operator Context**: Real-time system metadata from operator heartbeats.
- **Investigation Context**: Full conversation history and active tool outputs.
- **Memory (Codex)**: Durable user preferences and investigation summaries extracted asynchronously by the **Codex** agent.

### 3. Reasoning Agents
- **Dash (Fast-path)**: A high-efficiency responder for simple tasks. Focused on surgical tool calls and direct answers.
- **Sage (Primary Reasoner)**: The senior reasoner for complex, multi-step investigations. Operates in a **ReAct loop**, planning operations and articulating intent.

## Governance & Safety: The L1/L2/L3 Hierarchy

To ensure the platform remains a "safe pair of hands," every command passes through three distinct layers of validation.

### Layer 1: Technical Bedrock (Hard Gates)
The foundation of the safety system, enforced by the `SafetyService`.
- **Forbidden Patterns**: Global blocks on `sudo`, `su`, and other prohibited shell patterns.
- **Blacklist**: Denies specific dangerous commands or path substrings.
- **Whitelist**: Restricts execution to a known-safe set of base commands and arguments.

### Layer 2: Consensus (The Tribunal)
When Sage or Dash articulates an intent, the **Tribunal** ensemble converts it into a command.
- **Generation**: Five independent members (Axiom, Concord, Variance, Pragma, Nemesis) produce candidates based on their specific lens (Composition, Safety, Edge Cases, Convention, Adversary).
- **The Warden (Circuit Breaker)**: Performs risk analysis on the consensus winner. If a command is classified as `HIGH` risk twice in a single investigation, the **Two-Strike Circuit Breaker** triggers an `AGENT_CONFLICT` and stops execution.
- **The Auditor**: The final technical gate. Verifies the winner against Sage's intent. The Auditor can approve (`ok`), `swap` to a superior dissenting candidate, or `revise` the command.

### Layer 3: Authorization (Human-in-the-loop)
The final layer before execution on the host.
- **Human Approval**: By default, every command, risk assessment, and justification is presented for user approval.
- **Auto-Approval**: Benign commands defined in `auto_approved.json` can be marked as L3-authorized automatically *only if* they have passed all L1 and L2 gates.

## Reputation & Staking

The system maintains a per-agent reputation scalar `[0.0, 1.0]` as an EMA across conversations. Personas participate in **Reputation Staking** to ensure technical integrity.

| Persona | Stake / Lens | Primary Reward Mechanism |
|---|---|---|
| **Axiom** | Composition | Coherent, elegant pipeline generation |
| **Concord** | Safety | Correct application of defensive flags |
| **Variance** | Edge Cases | Handling unusual environmental conditions |
| **Pragma** | Convention | Adherence to system-idiomatic patterns |
| **Nemesis** | Adversary | Reward for confirmed attacks; Proper Scoring Rule |
| **Auditor** | Verification | Accurate swaps and revisions |
| **Warden** | Defense | Precise risk classification |

### Merkle Commitments
To prevent tampering with reputation scores, the Auditor writes a signed Merkle commitment of the scoreboard after every verdict. This chain of commitments ensures that the platform's governance history is cryptographically verifiable.

## Information Isolation
To prevent collusion and prompt injection spread, agents operate in sealed environments:
- **Triage** is unaware of downstream reasoning.
- **Tribunal Members** are blind to each other's candidates (Information Isolation).
- **Auditor** sees anonymized clusters to ensure unbiased judgement.
