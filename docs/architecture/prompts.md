---
title: Prompt Architecture
parent: Architecture
---

# Prompt Architecture

Last Updated: 2026-05-10
Version: v0.2.3

The g8e prompt system is a modular architecture designed for **prefix-cache reuse** and **strict structural enforcement**. It composes system prompts from shared fragments, canonical persona definitions, and dynamic turn-specific context.

The system is optimized for high-reasoning models (llama.cpp, vLLM) by placing static content at the beginning of the prompt to maximize KV-cache hits across agent turns.

---

## The Assembly Pipeline

The system prompt is built by `build_modular_system_prompt` in `@/home/bob/g8e/components/g8ee/app/llm/prompts.py:494`. Sections are concatenated in a fixed order based on their stability to optimize prefix caching.

| # | Section | Content Source | Stability | Rationale |
|---|---------|----------------|-----------|-----------|
| 1 | **Safety** | `core/safety.txt` | Global Static | Absolute behavioral guardrails. |
| 2 | **Loyalty** | `core/loyalty.txt` | Global Static | Mission-over-moment doctrine. |
| 3 | **Dissent** | `core/dissent.txt` | Global Static | Protocol for warnings and denials. |
| 4 | **Capabilities** | `modes/{mode}/capabilities.txt` | Per-Mode Static | Authorized actions for the current mode. |
| 5 | **Execution** | `modes/{mode}/execution.txt` | Per-Mode Static | How to process tasks and tools. |
| 6 | **Tools** | `modes/{mode}/tools.txt` | Per-Mode Static | High-level tool usage guidance. |
| 7 | **Response Constraints** | `system/response_constraints.txt` | Global Static | Length and style constraints. |
| 8 | **Agent Persona** | `AgentPersona.get_system_prompt()` | Per-Agent Static | Identity and specific mission. |
| 9 | **System Context** | `<system_context>` tag | Per-Turn Dynamic | Host OS, user, and environment details. |
| 10 | **Sentinel Mode** | `system/sentinel_mode.txt` | Per-Turn Dynamic | Injected only during escalated threats. |
| 11 | **Triage Context** | `<triage_context>` tag | Per-Turn Dynamic | User posture and intent classification. |
| 12 | **Investigation Context** | `<investigation_context>` tag | Per-Turn Dynamic | Case details and active operator list. |
| 13 | **Learned Context** | `<learned_context>` tag | Per-Turn Dynamic | Durable preferences and memory. |

---

## Canonical Truths

### 1. Persona Registry
All agent identities are defined as Pydantic models in `@/home/bob/g8e/components/g8ee/app/models/personas/` and registered in `PERSONA_REGISTRY`. This ensures type safety and structural consistency across the platform.

### 2. Agents Shared Constants
The shared JSON file `@/home/bob/g8e/shared/constants/agents.json:1` acts as the cross-component source of truth for metadata, icons, and model tiers.

### 3. XML Scaffolding Invariant
All sections are wrapped in XML tags to enforce hard structural boundaries. This is guaranteed by `AgentPersona.format_xml_tag` in `@/home/bob/g8e/components/g8ee/app/utils/agent_persona_loader.py:62`.

---

## Core Subsystems

### The Triage Pipeline (Interrogator)
Triage (`lite` tier) is the "first read of the room." It classifies requests by:
- **Complexity**: Determines if the request is `simple` (handled by Dash) or `complex` (handled by Sage).
- **Posture**: Gauges user intent (Normal, Escalated, Adversarial, Confused).
- **Intent**: Classifies as `information` or `action`.

### The Tribunal Ensemble (Consensus)
The Tribunal converts Sage's intent into shell commands through a 5-member ensemble:
- **Axiom**: Focuses on **Composition** (elegant pipelines).
- **Concord**: Focuses on **Safety** (defensive discipline).
- **Variance**: Focuses on **Edge Cases** (robust filenames/paths).
- **Pragma**: Focuses on **Convention** (idiomatic system patterns).
- **Nemesis**: The **Adversary** (proposes plausible-but-flawed candidates to stress-test the Auditor).

The **Auditor** (`primary` tier) evaluates these candidates against the original intent to select or revise the final command.

### Warden (Defensive Analysis)
Warden coordinates risk classification before execution:
- **Command Risk**: Labels commands as LOW, MEDIUM, or HIGH risk.
- **File Risk**: Evaluates the cost and reversibility of file operations.
- **Error Analyzer**: Determines if failures are `AUTO_FIXABLE` or require `ESCALATE`.

---

## Governance & Safety

### 1. Dissent Protocol
Embedded in every prompt via `core/dissent.txt`, this protocol instructs agents to issue warnings or denials when requests conflict with safety or loyalty guardrails.

### 2. Reputation Staking
Warden and Tribunal members "stake reputation" on their outputs. High-quality, safe classifications earn reputation, while failures or over-blocking cost it.

### 3. Co-Validation Terminology
All prompts reinforce the **co-validated infrastructure** model, where AI agents and human operators work in tandem, with the AI proposing and the human (or L1/L2 gates) validating.

---

## Operational Guide

### Adding a New Agent
1. Create a new persona model in `@/home/bob/g8e/components/g8ee/app/models/personas/`.
2. Register it in `PERSONA_REGISTRY` in `@/home/bob/g8e/components/g8ee/app/models/personas/__init__.py:35`.
3. Update `@/home/bob/g8e/shared/constants/agents.json:54` with the metadata.

### Modifying Prompt Fragments
1. Edit the relevant `.txt` file in `@/home/bob/g8e/components/g8ee/app/prompts_data/`.
2. Register the file in `PromptFile` enum in `@/home/bob/g8e/components/g8ee/app/constants/prompts.py:46`.

### Verification
Run the prompt alignment suite:
```bash
./g8e test g8ee -- tests/unit/llm/test_prompts.py
```
