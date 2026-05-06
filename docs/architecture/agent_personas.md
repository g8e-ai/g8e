---
title: Agent Personas
parent: Architecture
---

# Agent Persona System

## Overview

The g8e platform utilizes a centralized agent persona system to define AI identities, roles, and behavioral constraints. This system ensures consistency across Python (`g8ee`) and Node.js (`g8ed`) components while optimizing for prefix-cache performance.

- **Triage** (classifier): The entry point. Decides if a request is `simple` or `complex` and assesses user posture.
- **Sage** (reasoner): Senior reasoning authority for `complex` paths. Plans multi-step investigations.
- **Dash** (responder): Fast-path responder for `simple` paths. Minimizes latency with surgical action.
- **The Tribunal** (tribunal_member): Five-member ensemble (Axiom, Concord, Variance, Pragma, Nemesis) that translates intent into shell commands.
- **Warden** (defender): Defensive coordinator (Command, File, and Error analyzers) for local risk assessment.
- **Auditor** (auditor): Final judge and Merkle-root committer of Tribunal candidates.
- **Specialists**: Scribe (summarizer), Codex (analyzer), and Judge (evaluator).

---

## Authority & Registry

The canonical truth for agent personas resides in **Python models** located in `@/components/g8ee/app/models/personas/`. These models enforce field presence and alignment via Pydantic and provide structured system prompt generation.

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
    - `output_contract`: (Optional) Strict wire format definition.

---

## Core Pipeline Agents

### 1. Triage (The Gatekeeper)
- **Role**: `classifier` | **Tier**: `lite`
- **Purpose**: First-turn classification of complexity, intent, and user posture.
- **Complexity Calibration**: 
    - `simple`: Routine inquiries, status checks, single-step tasks.
    - `complex`: Multi-step investigations, ambiguity, or security-sensitive paths.
- **Posture Analysis**: Gauges `normal`, `escalated`, `adversarial`, or `confused` mindset. 
- **Invariants**: First-turn messages CANNOT be `adversarial`. Security overrides ALWAYS force `complex`.
- **Constraint**: Classifier ONLY. Does not generate text or questions; interrogation is deferred to reasoning agents.

### 2. Sage (Reasoning Authority)
- **Role**: `reasoner` | **Tier**: `primary`
- **Purpose**: Architect of complex investigations. Drives multi-step tool loops.
- **Intent Articulation**: Describes functional goals (e.g., "Determine if nginx errors started before the 14:20 deploy") without naming shell tools or syntax.
- **Interrogation Protocol**: If ambiguity persists, issues exactly three binary (YES/NO) questions to the user.
- **Approval Density**: Maximizes the value of every user interaction by proposing high-density intents to minimize approval cycles.

### 3. Dash (Fast-Path Responder)
- **Role**: `responder` | **Tier**: `assistant` | **Interrogator**: `simple` turns
- **Purpose**: Resolves straightforward requests with minimal latency.
- **Operating Mode**: Surgical tool use ("one well-aimed call beats a chain"). 
- **Escalation**: Hands off to **Sage** if multi-step planning or deep reasoning is required.
- **Interrogation Protocol**: Owns interrogation for `simple` turns. Emits an `<interrogation>` block to suppress tool execution until the user responds.

---

## The Tribunal & Safety

### 4. The Tribunal
A five-member ensemble translating Sage's intent into executable commands through ideological consensus.

- **Axiom** (`call_merge`): The Composer. Focuses on elegant pipeline composition.
- **Concord** (`verified_user`): The Guardian. Focuses on safety and defensive discipline.
- **Variance** (`call_split`): The Exhaustive. Focuses on edge cases (spaces, null input, symlinks).
- **Pragma** (`menu_book`): The Conventional. Focuses on idiomatic patterns (e.g., `journalctl` vs `tail`).
- **Nemesis** (`gpp_maybe`): The Adversary. Injects subtle flaws to test the system's immune system.

**Common Contract**: Members emit *only* a shell command string. No explanation or commentary is permitted.

### 5. Warden (Defensive Coordination)
- **Role**: `defender` | **Tier**: `lite`
- **Purpose**: Local risk assessment performing pre-execution analysis.
- **Sub-Agents**:
    - `warden_command_risk`: Classifies blast radius (LOW/MEDIUM/HIGH).
    - `warden_file_risk`: Evaluates file operation sensitivity and git-reversibility.
    - `warden_error`: Analyzes failures for `AUTO_FIXABLE` or `ESCALATE`.
- **Stake Reputation**: Stakes reputation on accuracy. Over-caution (blocking safe tasks) or under-caution (allowing danger) costs reputation.

### 6. Auditor
- **Role**: `auditor` | **Tier**: `primary`
- **Purpose**: Final judge of Tribunal candidates. 
- **Responsibility**: Judges Tribunal candidates after Warden clearance. Awards Nemesis for successful "Warden tricks" but rejects the flawed command. 
- **Reputation**: The only agent authorized to write `reputation_commitment` to the global Merkle-root scoreboard.

---

## Specialty Agents

- **Scribe** (`summarizer`): Generates concise 3-7 word case titles.
- **Codex** (`analyzer`): Async extraction of durable user preferences and scrubbed summaries.
- **Judge** (`evaluator`): Dispassionate post-hoc grading against gold-standard rubrics.

---

## The Information Isolation Principle

Agents operate in a sealed environment to preserve diversity and technical honesty (GDD §3).

| Role | Information Visible | Isolation Goal |
|---|---|---|
| **Triage** | User message + brief history | Prevent routing bias |
| **Sage/Dash** | Triage result + history | Focus on reasoning/speed |
| **Tribunal** | Sage's intent + OS context | Prevent ideological collision |
| **Auditor** | Intent + Candidates + Reputation | Converge on technical truth |

---

## Canonical Prompt Layout

System prompts are constructed via `@/components/g8ee/app/models/personas/base.py` following this XML-tagged pattern:

1. `<role>`: The behavioral category.
2. `<output_contract>`: Strict wire format (if applicable).
3. `<identity>`: Core personality.
4. `<purpose>`: Technical objectives.
5. `<autonomy>`: Agency directive.

This structure optimizes prefix-cache effectiveness in `llama.cpp` by placing shared doctrine before agent-specific identity.

---

## Reputation & Staking

The platform uses a Phase 3 Reputation System (GDD §14.5) to enforce performance:
- **EMA Updating**: Agent scalars are updated using an Exponential Moving Average (default half-life: 50).
- **Slashing**: Catastrophic failures result in tiered scalar retention (Tier 1: 25% retention).
- **Proper Scoring**: Nemesis stakes are calibrated so that honest calibration is the dominant strategy.
- **Merkle Commitments**: Auditor's reputation updates are cryptographically bound to a global scoreboard.
