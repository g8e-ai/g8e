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

### 1. Triage (The Interrogator)
- **Icon**: `scan-eye`
- **Role**: `classifier`
- **Model Tier**: `lite`
- **Purpose**: First-turn classification of complexity (`simple`/`complex`), intent, and user posture (`normal`/`escalated`/`adversarial`/`confused`). It may also serve as an interrogator, producing clarifying questions when intent confidence is low.
- **Naming Note**: Implements the "Dash" interrogator role from GDD. Renamed to `triage` in code to avoid collision with the `dash` fast-path agent.
- **Data Flow**: Routes complex turns to **Sage** and simple turns to **Dash**. Emits clarifying questions to the user if needed. Posture is consumed by the dissent protocol to calibrate agent friction.
- **Staking Logic**: Stakes per question on information yield (engagement, discrimination, downstream utility, redundancy penalty). User click-through behavior is a revealed-preference stake.
- **Data Access**: Sees user message and brief conversation history. Does NOT know Sage/Tribunal/Auditor exist (Vortex Principle).
- **Invariants**: First-turn messages CANNOT be `adversarial`.

### 2. Sage (Primary AI)
- **Icon**: `cpu`
- **Role**: `reasoner`
- **Model Tier**: `primary`
- **Purpose**: Senior reasoning agent for complex investigations. Produces intent document (goals, constraints, success criteria) as the sole input the Tribunal sees.
- **Execution**: Drives multi-step tool loops and articulates intent to the Tribunal.
- **Staking Logic**: Stakes on one-shot sufficiency. Win if Round 1 passes AND Auditor rules ok. Graduated loss: R2 converges < Auditor revises < Auditor swaps < round fails.
- **Data Access**: Sees user message + Triage Q&A (as "user context"). Does NOT know Triage is a distinct agent; does NOT know Auditor has memory (Vortex Principle).
- **System Prompt**: Assembled via `build_modular_system_prompt` in `@/home/bob/g8e/components/g8ee/app/llm/prompts.py`, prepending the persona to core safety, loyalty, and dissent doctrine.

### 3. Dash (Assistant AI)
- **Icon**: `zap`
- **Role**: `responder`
- **Model Tier**: `assistant`
- **Purpose**: Fast-path agent for simple requests. Note: This is distinct from the GDD's "Dash" interrogator role, which maps to the Triage agent.
- **Posture**: Surgical tool use ("one well-aimed call beats a chain").
- **Handoff**: If a turn outgrows its lane, Dash names the mismatch and stops, allowing re-triage to Sage on the next turn.

### 4. Tribunal Members
- **Model Tier**: `lite` (but runtime-routed to `assistant`)
- **Common Contract**: Every member emits exactly a shell command string with no prose or markdown fences.
- **Staking Logic (Honest Four - Axiom, Concord, Variance, Pragma)**: Stake per-lens. Auditor annotates why a command won and resolves against dimensions owned by each persona. In Round 2, rewarded for "held correctly" vs. "absorbed correctly" per Auditor.
- **Staking Logic (Nemesis)**: Stakes on calibration using a proper scoring rule. Attack on flawed consensus confirmed = large gain. Abstain on clean consensus = small gain. Attack on clean (false alarm) = large loss. Abstain on flawed (miss) = large loss. Dominant strategy: honest calibration.
- **Data Access**: See user message (folded into intent). See only request + guidelines + system context. Do NOT know Nemesis is among them. Do NOT know cross-chain grounding exists. Do NOT know other agents have reputation (Vortex Principle).
- **Voices**:
    - **Axiom** (Composer): Pressure for composition over fragmentation.
    - **Concord** (Guardian): Pressure for safety and defensive discipline.
    - **Variance** (Exhaustive): Pressure for robustness against edge cases.
    - **Pragma** (Conventional): Pressure for idiomatic patterns.
    - **Nemesis** (Adversary): Injected flaws for immune system stress-testing.

### 5. Auditor
- **Role**: `auditor`
- **Model Tier**: `primary`
- **Purpose**: Final judge of Tribunal candidates. Operates in `unanimous`, `majority`, or `tied` modes. Only agent with cross-conversation memory.
- **Staking Logic**: Stakes on downstream truth. Cannot stake on own verdict in the moment. Stakes on execution outcomes + forward-hook hit rate + peer-review agreement. Bonds 2-3x any Tribunal member. Earns a cut of everyone's reward, paid only on downstream success.
- **Data Access**: Sees user message, Triage Q&A, Sage intent, persona signatures, and has full memory access (cross-chain precedent). Only agent that reads `reputation_state` (cross-chain memory) and writes `reputation_commitment`.
- **Autonomy**: Verdict is final and cryptographically bound to the reputation scoreboard via Merkle commitments.

### 6. Specialists (Scribe, Codex, Judge)
- **Scribe** (`summarizer`): Generates 3-7 word case titles.
- **Codex** (`analyzer`): Async extraction of user preferences and scrubbed investigation summaries.
- **Judge** (`evaluator`): Post-hoc performance grading against gold-standard rubrics.

### 7. Warden (Defensive Sub-Agents)
- **Role**: `defender`
- **Model Tier**: `lite`
- **Purpose**: Collective role for specialized risk analysis sub-agents. Coordinates findings to inform human approval and Auditor decisions.
- **Sub-Agents**:
    - `warden_command_risk`: Classifies shell command risk (LOW/MEDIUM/HIGH).
    - `warden_error`: Analyzes failures for `AUTO_FIXABLE` or `ESCALATE`.
    - `warden_file_risk`: Evaluates file operation safety, factoring in git state.
- **Implementation**: Managed by the `AIResponseAnalyzer` service in `g8ee`.

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

## The Vortex Principle (Tiered Information Quarantine)

The Tribunal game design enforces a critical safety mechanism: tiered information quarantine. Each agent operates in a sealed information environment, believing they're playing a smaller game than they're actually in:

- **Triage (GDD's "Dash")** believes it's playing optimized 20-questions; doesn't know Sage/Tribunal/Auditor exist
- **Sage** believes it's producing intent for a translation layer; doesn't know Triage is a distinct agent (its Q&A appears as "user context"); doesn't know Auditor has memory
- **Tribunal** believes they're proposing commands for a merit judge; doesn't know Nemesis is among them, doesn't know cross-chain grounding exists, doesn't know other agents have reputation
- **Auditor** is the only agent with full visibility (persona signatures, cross-chain memory, reputation state)

This tiered information quarantine is load-bearing. Collapsing it collapses the diversity and honesty that produce safety. The vortex is enforced through prompt-layer design: each agent's persona prompt receives only the information appropriate to its role.

## Data Access Matrix

| Role | User msg | Triage Q&A | Sage intent | Own rep | Others' rep | Peer candidates | Persona signatures | Cross-chain memory |
|---|---|---|---|---|---|---|---|---|
| Triage | ✓ | writes | ✗ | ✓ | ✗ | ✗ | ✗ | ✗ |
| Sage | ✓ | ✓ (as "user context") | writes | ✓ | ✓ | ✗ | ✗ | ✗ |
| Tribunal (5) | ✓ | folded into intent | ✓ | ✓ | ✗ | R2 only, anonymized | ✗ | ✗ |
| Auditor | ✓ | ✓ | ✓ | ✓ + slash history | ✓ | ✓ | ✓ | ✓ |
| Operator | ✗ | ✗ | ✗ | ✗ | ✗ | ✗ | ✗ | ✗ |
| Human | ✓ | ✓ (their own) | ✓ summary | ✗ by default | ✗ | optional expand | ✗ | ✗ |

## Technical Invariants

- **Model Tier Mapping**: `primary`, `assistant`, `lite`. Verified via `test_model_tier_is_valid_value`.
- **Runtime Alignment**: Production routing tiers are pinned to `agents.json` declarations via `test_model_tier_matches_runtime_routing`.
- **XML Scaffolding**: All prompt components are wrapped in hard XML boundaries to prevent data bleeding.
