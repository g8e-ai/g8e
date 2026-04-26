---
title: AI Agents
parent: Architecture
---

# AI Agent Architecture

The g8e platform utilizes a specialized multi-agent system designed for autonomous technical investigations. The architecture prioritizes safety, cryptographic auditability, and human-in-the-loop control while maintaining high reasoning performance.

## The Agent Lifecycle

Every user message triggers a structured pipeline that ensures complex tasks receive deep reasoning while simple requests remain fast and efficient.

### 1. Triage (The Gatekeeper)
Incoming messages are first processed by the **Triage Agent** (using a lightweight model). It performs three critical classifications:
- **Complexity**: Routes "simple" requests (status checks, basic lookups) to **Dash** and "complex" requests (investigations, multi-step actions) to **Sage**.
- **Intent**: Categorizes the goal as `information`, `action`, or `unknown`.
- **Posture**: Identifies the user's state (e.g., `normal`, `escalated`, `adversarial`) to calibrate downstream agent behavior.

### 2. Context Enrichment
Before an agent receives the task, the system assembles a comprehensive "world view":
- **Investigation History**: The recent conversation log.
- **Operator Context**: Real-time system metadata (OS, shell, architecture, permissions) from the target operator.
- **Memories**: Preferences and summaries extracted by **Codex** in previous turns.

### 3. Reasoning & Tool Loops
- **Dash (Fast-path)**: A direct responder that resolves simple tasks with minimal latency. It can call single tools but escalates to Sage if the plan exceeds a single turn.
- **Sage (Primary Reasoner)**: The senior investigator. It operates in a **ReAct loop**, proposing tools, interpreting results, and refining its plan until the investigation is complete.

## The Tribunal: Intent-Driven Execution

To prevent hallucinations in shell syntax, agents never write commands directly. Instead, they articulate **Investigative Intent**. This intent is translated into executable commands by **The Tribunal**:

1. **Ensemble Generation**: Five specialized members (**Axiom**, **Concord**, **Variance**, **Pragma**, **Nemesis**) generate independent command candidates based on their unique perspectives (e.g., safety, convention, edge cases).
2. **Consensus Voting**: A deterministic voting system selects the winning command cluster.
3. **The Auditor**: A primary-tier model verifies the winner against the original intent, catching subtle flaws or proposing refined versions.
4. **Cryptographic Commitment**: The Auditor signs the result, binding it to the current reputation scoreboard before it reaches the human for approval.

## Memory & Background Learning

Learning happens off the critical path via **Codex**, which runs asynchronously after a turn completes:
- **Preference Extraction**: Updates the user's communication style, technical background, and problem-solving approach.
- **Investigation Summaries**: Maintains high-level progress logs, enabling the system to resume long-running investigations without context loss.

## Governance & Sovereignty

Safety is enforced through the **Local-First Audit Architecture (LFAA)**. The platform acts as a stateless relay; the **Operator** is the system of record.
- **Audit Vault**: All activity (messages, intents, commands, results) is persisted in an encrypted, tamper-evident SQLite database on the target host.
- **Ledger**: Every file mutation is backed by a local Git repository, providing immutable version history and rollback capability.
- **Warden**: Performs pre-execution risk assessment (Command, File, and Error risk) to ensure the user is fully informed before granting approval.
