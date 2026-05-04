---
title: Agent Personas
parent: Architecture
---

# Agent Persona System

## Overview

The g8e platform utilizes a centralized agent persona system to define and manage AI identities, roles, and behavioral constraints. This system ensures consistency across the Python (`g8ee`) and Node.js (`g8ed`) components while providing high-performance, validated persona loading.

## Authority & Registry

The canonical truth for agent personas resides in **Python models** located in `@/components/g8ee/app/models/personas/`. These models enforce field presence and alignment via Pydantic.

- **Authoritative Source**: `@/components/g8ee/app/models/personas/` (Python classes).
- **Derivative JSON**: `@/shared/constants/agents.json` (Generated for Node.js/`g8ed` consumption).
- **Metadata Fields**:
    - `id`: Unique identifier (e.g., `triage`, `sage`).
    - `display_name`: Human-readable name.
    - `icon`: Material Design icon name for the UI.
    - `description`: Brief role summary.
    - `role`: Behavioral category (`classifier`, `reasoner`, `responder`, `auditor`, `tribunal_member`, `summarizer`, `analyzer`, `evaluator`, `defender`).
    - `model_tier`: Runtime routing target (`primary`, `assistant`, `lite`).
    - `tools`: List of permitted tool names.
    - `identity`: Core personality and behavioral instructions.
    - `purpose`: Technical objectives and pipeline role.
    - `autonomy`: Directive prose affirming the agent's agency.
    - `output_contract`: (Optional) Strict wire format definition (Required for Tribunal and Triage).

## Current Agents

### 1. Triage (The Gatekeeper/Classifier)
- **Icon**: `manage_search`
- **Role**: `classifier`
- **Model Tier**: `lite`
- **Purpose**: First-turn classification of complexity (`simple`/`complex`), intent, and user posture.
- **Scope**: Per GDD §14.1, Triage is a classifier ONLY. It does NOT generate questions or interrogations — that responsibility belongs to the reasoning agents (Dash/Sage) per the Interrogation Protocol.
- **Invariants**: First-turn messages CANNOT be `adversarial`. Security-sensitive requests are ALWAYS `complex`.
- **Data Flow**: Routes `complex` turns to **Sage** and `simple` turns to **Dash**.

### 2. Sage (Reasoning Authority)
- **Icon**: `psychology`
- **Role**: `reasoner`
- **Model Tier**: `primary`
- **Purpose**: Senior reasoning agent for complex investigations. Drives multi-step tool loops and articulates intent to the Tribunal.
- **Intent Articulation**: Sage describes the *goal* and *semantics* of a command without naming shell tools or syntax, allowing the Tribunal to translate.
- **Staking**: Stakes on one-shot sufficiency. Win if Round 1 passes AND Warden clears it AND Auditor rules `ok`.

### 3. Dash (Fast-Path Responder)
- **Icon**: `bolt`
- **Role**: `responder`
- **Model Tier**: `assistant`
- **Purpose**: Resolves straightforward requests with minimal latency.
- **Operating Mode**: Surgical tool use ("one well-aimed call beats a chain"). Escalates multi-step or deeply ambiguous requests to **Sage**.

### 4. The Tribunal
A five-member panel that translates Sage's intent into an executable command through ideological consensus.

- **Axiom** (`call_merge`): The Composer. Focuses on composition and coherent pipelines.
- **Concord** (`verified_user`): The Guardian. Focuses on safety and defensive discipline.
- **Variance** (`call_split`): The Exhaustive. Focuses on robustness against edge cases (spaces, null input, symlinks).
- **Pragma** (`menu_book`): The Conventional. Focuses on idiomatic patterns for the target system.
- **Nemesis** (`gpp_maybe`): The Adversary. Injects plausible-but-subtle flaws to stress-test the ensemble.

**Common Contract**: Every member emits exactly a shell command string. Disagreement is ideological, not statistical.

### 5. Warden (Defensive Coordination)
- **Icon**: `shield`
- **Role**: `defender`
- **Model Tier**: `lite`
- **Purpose**: Orchestrates specialized risk analysis sub-agents to produce a consolidated safety verdict for the Operator. The Warden validates the safety of a command *before* the Auditor cryptographically commits. Stakes reputation on accurate risk assessment.
- **Sub-Agents**:
    - `warden_command_risk`: Classifies command blast radius (LOW/MEDIUM/HIGH).
    - `warden_file_risk`: Evaluates file operation sensitivity and git-reversibility.
    - `warden_error`: Analyzes failures for `AUTO_FIXABLE` or `ESCALATE`.
- **Staking**: Warden stakes reputation on accurate risk classification. It earns reputation for correctly identifying dangerous commands and loses reputation for blocking safe operations (over-caution) or allowing dangerous ones (under-caution).
- **Two-Strike Circuit Breaker**: When Warden blocks a command:
    - **First Strike**: An assistant-tier model generates contextual feedback explaining why the command was blocked and suggesting safer alternatives. Sage receives this feedback and can propose a revised command.
    - **Second Strike**: If Warden blocks Sage's revised command, the system triggers an `AI_AGENT_CONFLICT_DETECTED` event, halts the ReAct loop, and surfaces an "Agent Conflict" dialog to the user for human intervention.

### 6. Auditor
- **Icon**: `fact_check`
- **Role**: `auditor`
- **Model Tier**: `primary`
- **Purpose**: Final judge of Tribunal candidates. Operates in `unanimous`, `majority`, or `tied` modes. Only once the Warden has cleared the command does the Auditor perform the final consistency check and Merkle commitment.
- **Reputation**: The only agent that reads `reputation_state` (cross-chain memory) and writes `reputation_commitment` via Merkle roots.
- **Output**: `ok` or `reject`.

### 7. Specialists
- **Scribe** (`title`): Generates concise 3-7 word case titles.
- **Codex** (`neurology`): Async extraction of durable user preferences and scrubbed investigation summaries.
- **Judge** (`gavel`): Post-hoc performance grading against gold-standard rubrics.

## The Information Isolation Principle (Information Quarantine)

GDD §3 defines Information Isolation: agents operate in a sealed information environment to prevent collapsing diversity and honesty.

| Role | Information Visible |
|---|---|
| **Triage** | User message + brief history. Doesn't know Sage/Tribunal exist. |
| **Sage** | User message + Triage classification. Doesn't know Triage is a separate agent. |
| **Tribunal** | Intent from Sage + OS/Shell context. Doesn't know Nemesis is present. |
| **Auditor** | Full visibility: User msg, Sage intent, Persona signatures, Reputation state. |

## Canonical Prompt Layout

All system prompts are constructed via `@/components/g8ee/app/utils/agent_persona_loader.py` following this XML-tagged pattern:

1. `<role>`: The behavioral category.
2. `<output_contract>`: Strict wire format (if applicable).
3. `<identity>`: Core personality.
4. `<purpose>`: Technical objectives.
5. `<autonomy>`: Agency directive.

This structure ensures prefix-cache effectiveness in `llama.cpp` by placing shared doctrine (Safety, Loyalty, Dissent) before agent-specific identity.

## Reputation & Staking

The platform uses a Phase 3 Reputation System (GDD §14.5) to enforce performance:
- **EMA Updating**: Agent scalars are updated using an Exponential Moving Average (default half-life: 50).
- **Slashing**: Catastrophic failures result in tiered scalar retention (Tier 1: 25% retention).
- **Proper Scoring**: Nemesis stakes are calibrated so that honest calibration is the dominant strategy.
- **Merkle Commitments**: Auditor's reputation updates are cryptographically bound to a global scoreboard.
