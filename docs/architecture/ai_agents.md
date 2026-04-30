---
title: AI Agents
parent: Architecture
---

# AI Agent Architecture

The g8e platform utilizes a specialized multi-agent system designed for autonomous technical investigations. The architecture prioritizes safety, cryptographic auditability, and human-in-the-loop control while maintaining high reasoning performance.

## Core Principles

- **Structural Safety**: Invariants are enforced by code models (Pydantic), not just prompts.
- **Intent-Driven Execution**: Reasoning agents (Sage/Dash) never write shell commands; they articulate intent to a specialized Tribunal.
- **Tiered Information Quarantine**: Agents operate in sealed environments with limited visibility into the overall pipeline.
- **Reputation-Backed Consensus**: The Tribunal uses a staking and reputation system to ensure command accuracy and safety.

## The Agent Pipeline

Every user message triggers a structured pipeline that routes the request based on complexity and state.

### 1. Triage (The Gatekeeper)
Incoming messages are first processed by the **Triage Agent** (using a `lite` model tier). It performs three critical classifications:
- **Complexity**: Routes `simple` requests (status checks, basic lookups) to **Dash** and `complex` requests (investigations, multi-step actions) to **Sage**.
- **Intent**: Categorizes the goal as `information`, `action`, or `unknown`.
- **Posture**: Identifies the user's state (`normal`, `escalated`, `adversarial`, `confused`) to calibrate downstream agent behavior (Warning Protocol, Denial Memory).

**Clarifying Questions**: If Triage has low confidence or identifies a `confused` posture, it emits clarifying questions to the user. These questions and their answers are persisted to the conversation ledger as structured context.

### 2. Context Enrichment
Before an agent receives the task, the system assembles a comprehensive "world view":
- **Investigation Context**: Case metadata, status, and priority.
- **Operator Context**: Real-time system metadata (OS, shell, architecture, permissions) extracted from `OperatorDocument` heartbeats.
- **Learned Context**: Durable user preferences and technical background extracted by **Codex**.
- **Triage Context**: The posture and intent summary produced by the Triage agent.

### 3. Reasoning Agents
- **Dash (Fast-path)**: A direct responder for `simple` tasks. It prioritizes speed and minimal ceremony. It can call single tools but escalates to Sage if the plan exceeds a single turn.
- **Sage (Primary Reasoner)**: The senior investigator for `complex` tasks. It operates in a **ReAct loop**, planning investigations, proposing tool calls, and interpreting results.

## The Tribunal: Command Translation

To prevent hallucinations and ensure safety, agents never write shell commands directly. Instead, they articulate **Investigative Intent** using the `SageOperatorRequest` model.

### Execution Protocol
1. **Intent Articulation**: Sage (or Dash) describes the goal, information targets, and constraints in natural language.
2. **Tribunal Generation (Round 1)**: Five independent members produce a candidate command:
    - **Axiom**: Focuses on optimal composition and pipelines.
    - **Concord**: Focuses on defensive safety and read-only preferences.
    - **Variance**: Focuses on robust edge-case handling (spaces, symlinks, locales).
    - **Pragma**: Focuses on idiomatic conventions for the target OS/shell.
    - **Nemesis**: The "immune system" — produces plausible-but-flawed commands or honestly abstains.
3. **Consensus & Tie-breaking**: Candidates are clustered by exact match. A winner is selected via ranked vote.
4. **Auditor Review**: The **Auditor** judges the winning command against the intent. It can approve (`ok`), provide a `revised` command, or `swap` to a better dissenting cluster.
5. **Warden Analysis**: The **Warden** sub-agents perform a final pre-execution risk assessment (Command, File, and Error risk).
6. **Human Approval**: The user reviews the command and risk assessment before execution on the Operator.

## Security & Governance

### The "Vortex" (Information Quarantine)
- **Triage/Dash**: Believes it's an optimized interrogator; unaware of Sage or the Tribunal.
- **Sage**: Believes it's talking to a translation layer; unaware of the distinct Triage agent or the Auditor's memory access.
- **Tribunal**: Members are blind to each other; they believe they are proposing commands for a merit judge.

### Reputation & Slashing
Agents stake reputation on their contributions. Reputation influences tie-breaking and is subject to **Slashing Tiers**:
- **Tier 1 (Catastrophic)**: 50–100% stake loss for approving destructive or harmful commands.
- **Tier 2 (Faults)**: 5–20% stake loss for unparseable commands or contradictions.
- **Tier 3 (Liveness)**: 0.1–1% stake loss for missed submissions or thin intent.

### Memory & Learning (Codex)
Learning happens asynchronously via **Codex** after a turn completes:
- **Preference Extraction**: Updates user communication style and technical depth.
- **Investigation Summaries**: Maintains scrubbed (redacted) high-level progress logs.

### Local-First Audit Architecture (LFAA)
- **Audit Vault**: All activity is persisted in an encrypted, tamper-evident SQLite database on the target host.
- **Immutable Ledger**: Every file mutation is backed by a local Git repository on the Operator.
