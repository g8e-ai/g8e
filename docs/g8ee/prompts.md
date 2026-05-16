---
title: Prompt Architecture
parent: Architecture
---

# Prompt Architecture

Last Updated: 2026-05-13
Version: v0.3.0

The g8e prompt system is a modular architecture implemented in the **optional Engine (`g8ee`) adapter**. It is designed for **prefix-cache reuse** and **strict structural enforcement**. It composes system prompts from shared fragments, canonical persona definitions, and dynamic turn-specific context.

As a **BYO (Bring Your Own) client** to the `g8eo` substrate, the Engine uses this system to translate user intent into the signed JSON transactions required by the protocol.

---

## The Assembly Pipeline

The system prompt is built by `build_modular_system_prompt` in `@/home/bob/g8e/services/g8ee/app/llm/prompts.py:494`. Sections are concatenated in a fixed order based on their stability to optimize prefix caching (e.g., llama.cpp, vLLM).

### Section Order & Rationale

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

## Canonical Invariants

### 1. XML Scaffolding
All sections are wrapped in XML tags to enforce hard structural boundaries. This is guaranteed by `AgentPersona.format_xml_tag` in `@/home/bob/g8e/services/g8ee/app/utils/agent_persona_loader.py:62`. This prevents "prompt leakage" where one section's instructions bleed into another.

### 2. Prefix Cache Optimization
By placing the most stable sections (Safety, Loyalty, Dissent) at the very beginning, the system maximizes KV-cache hits. This significantly reduces latency when switching between different agents (e.g., Triage to Sage) within the same conversation.

### 3. Decoupled Reasoning
The prompt system does not execute actions. It generates **intent**. This intent is then passed to the **Tribunal** for translation into a command, which is finally wrapped in a signed `UniversalEnvelope` and sent to the `g8eo` substrate for execution.

---

## Dynamic Context Blocks

### System Context (`<system_context>`)
Injected via `_build_system_context_section`. It includes:
- **Operator Type**: Standard vs. Cloud (least-privilege intent-based).
- **Environment**: OS, Hostname, User (UID), Working Directory.
- **Isolation**: Container runtime and Init system (e.g., warning if `systemd` is missing).

### Triage Context (`<triage_context>`)
Injected via `build_triage_context_section`. It carries the Triage agent's classification:
- **Request Posture**: `normal`, `escalated`, `adversarial`, or `confused`.
- **Intent Summary**: The high-level objective of the turn.

### Investigation Context (`<investigation_context>`)
Injected via `build_investigation_context_section`. It binds the prompt to the current case:
- **Case Metadata**: Title, Description, Status, Priority, Severity.
- **Bound Operators**: A list of all operators currently connected to the investigation.

---

## The Dissent Protocol

Defined in `core/dissent.txt`, the Dissent Protocol is the "moral compass" of the agent. It instructs agents to:
1.  **Verify Posture**: Read the `<triage_context>` to calibrate suspicion.
2.  **Issue Warnings**: If a request is risky but valid, warn the user.
3.  **Refuse Violations**: If a request violates L1/L2 safety or loyalty, refuse with a clear reason.
4.  **Memory of Denials**: Past denials are carried in context to prevent "jailbreaking" through persistence.

---

## Tribunal Prompts

The Tribunal uses a specialized set of prompts for its consensus-generation workflow:
- **Generator (`tribunal/generator.txt`)**: Instructs the five personas (Axiom, Concord, Variance, Pragma, Nemesis) to translate intent into a specific command.
- **Auditor (`tribunal/auditor.txt`)**: Evaluates the candidates against the original intent to select the most robust implementation.

---

## Operational Guide

### Adding a New Agent
1. Define a Pydantic model in `@/home/bob/g8e/services/g8ee/app/models/personas/`.
2. Register it in `PERSONA_REGISTRY` in `@/home/bob/g8e/services/g8ee/app/models/personas/__init__.py`.
3. The prompt system will automatically pick up the new persona.

### Modifying Fragments
1. Edit the `.txt` file in `@/home/bob/g8e/services/g8ee/app/prompts_data/`.
2. If adding a new file, register it in the `PromptFile` enum in `@/home/bob/g8e/services/g8ee/app/constants/prompts.py`.

### Verification
Run the prompt alignment suite:
```bash
./g8e test g8ee -- tests/unit/llm/test_prompts.py
```
