---
title: Agent Personas
parent: Architecture
---

# Agent Persona System

## Overview

The g8e platform utilizes a centralized agent persona system to define and manage AI identities, roles, and behavioral constraints. This system ensures consistency across the Python (`g8ee`) and Node.js (`g8ed`) components while providing high-performance, validated persona loading.

## Agent Registry

All agent definitions are centralized in `@/home/bob/g8e/shared/constants/agents.json`. Each entry in `agent.metadata` contains:

- **Identity Metadata**: `id`, `display_name`, `icon`, `description`.
- **Structural Metadata**: `role`, `model_tier`, `tools`.
- **Persona Components**:
    - **identity**: Core personality and behavioral instructions.
    - **purpose**: Technical objectives and pipeline role.
    - **autonomy**: Directive prose affirming the agent's agency.
    - **output_contract**: (Optional) Strict wire format definition (Required for Tribunal members).

## Current Agents

### 1. Triage
- **Icon**: `scan-eye`
- **Role**: `classifier`
- **Model Tier**: `lite`
- **Purpose**: First-turn classification of complexity (`simple`/`complex`), intent, and user posture (`normal`/`escalated`/`adversarial`/`confused`).
- **Data Flow**: Routes complex turns to **Sage** and simple turns to **Dash**. Posture is consumed by the dissent protocol to calibrate agent friction.
- **Invariants**: First-turn messages CANNOT be `adversarial`.

### 2. Sage (Primary AI)
- **Icon**: `cpu`
- **Role**: `reasoner`
- **Model Tier**: `primary`
- **Purpose**: Senior reasoning agent for complex investigations.
- **Execution**: Drives multi-step tool loops and articulates intent to the Tribunal.
- **System Prompt**: Assembled via `build_modular_system_prompt` in `@/home/bob/g8e/components/g8ee/app/llm/prompts.py`, prepending the persona to core safety, loyalty, and dissent doctrine.

### 3. Dash (Assistant AI)
- **Icon**: `zap`
- **Role**: `responder`
- **Model Tier**: `assistant`
- **Purpose**: Fast-path agent for simple requests.
- **Posture**: Surgical tool use ("one well-aimed call beats a chain").
- **Handoff**: If a turn outgrows its lane, Dash names the mismatch and stops, allowing re-triage to Sage on the next turn.

### 4. Tribunal Members
- **Model Tier**: `lite` (but runtime-routed to `assistant`)
- **Common Contract**: Every member emits exactly a shell command string with no prose or markdown fences.
- **Voices**:
    - **Axiom** (Composer): Pressure for composition over fragmentation.
    - **Concord** (Guardian): Pressure for safety and defensive discipline.
    - **Variance** (Exhaustive): Pressure for robustness against edge cases.
    - **Pragma** (Conventional): Pressure for idiomatic patterns.
    - **Nemesis** (Adversary): Injected flaws for immune system stress-testing.

### 5. Auditor
- **Role**: `auditor`
- **Model Tier**: `primary`
- **Purpose**: Final judge of Tribunal candidates. Operates in `unanimous`, `majority`, or `tied` modes.
- **Autonomy**: Verdict is final and cryptographically bound to the reputation scoreboard via Merkle commitments.

### 6. Specialists (Scribe, Codex, Judge)
- **Scribe** (`summarizer`): Generates 3-7 word case titles.
- **Codex** (`analyzer`): Async extraction of user preferences and scrubbed investigation summaries.
- **Judge** (`evaluator`): Post-hoc performance grading against gold-standard rubrics.

### 7. Warden (Defensive Coordinator)
- **Role**: `defender`
- **Model Tier**: `lite`
- **Purpose**: Coordinates specialized risk analysis. Fails closed (HIGH risk) on ambiguity.
- **Sub-Agents**:
    - `warden_command_risk`: Classifies shell command risk (LOW/MEDIUM/HIGH).
    - `warden_error`: Analyzes failures for `AUTO_FIXABLE` or `ESCALATE`.
    - `warden_file_risk`: Evaluates file operation safety, factoring in git state.

## Persona Loader Utility

The `@/home/bob/g8e/components/g8ee/app/utils/agent_persona_loader.py` utility provides centralized loading and validation:

- **Validation**: Uses Pydantic `AgentPersona` model to enforce field presence and alignment.
- **Caching**: `agents.json` is loaded via `@lru_cache(maxsize=1)`, making it process-lifetime immutable for performance.
- **System Prompt Assembly**: `AgentPersona.get_system_prompt()` constructs the canonical XML-tagged block:
    1. `<role>`
    2. `<output_contract>` (if present)
    3. `<identity>`
    4. `<purpose>`
    5. `<autonomy>`

## Canonical Prompt Layout

All agents follow a strict structural pattern to ensure prefix-cache effectiveness and model adherence:

1. **Static Shared Prefix**: Safety, Loyalty, and Dissent doctrine (identical across agents).
2. **Mode-Specific capabilities**: Tools and execution instructions.
3. **Agent Persona**: The XML-tagged identity block from `agents.json`.
4. **Dynamic Context**: Triage posture, investigation status, and user memories.

**Prohibited Patterns**:
- No embedded shell command examples (prevents markdown pollution).
- No `<output_contract>` tags inside the `identity` field; must use the explicit field in `agents.json`.

## Technical Invariants

- **Model Tier Mapping**: `primary`, `assistant`, `lite`. Verified via `test_model_tier_is_valid_value`.
- **Runtime Alignment**: Production routing tiers are pinned to `agents.json` declarations via `test_model_tier_matches_runtime_routing`.
- **XML Scaffolding**: All prompt components are wrapped in hard XML boundaries to prevent data bleeding.
