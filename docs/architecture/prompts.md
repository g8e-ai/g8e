---
title: Prompts
parent: Architecture
---

# Prompt System

The g8e prompt system is a modular architecture designed for **prefix-cache reuse** and **strict structural enforcement**. It composes final system prompts from shared fragments, canonical persona definitions, and dynamic turn-specific context.

The system is optimized for high-reasoning models (llama.cpp, vLLM) by placing static content at the beginning of the prompt to maximize KV-cache hits across agent turns.

---

## The Assembly Pipeline

The primary system prompt is built by `build_modular_system_prompt` in `@/home/bob/g8e/components/g8ee/app/llm/prompts.py:496`. Sections are concatenated in a fixed order based on their stability to optimize prefix caching.

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

## Core Components

### 1. Static Fragments (`prompts_data/`)
Located in `@/home/bob/g8e/components/g8ee/app/prompts_data/`, these `.txt` files provide the doctrinal foundation.
- **`core/`**: Safety, Loyalty, and Dissent fragments shared by all agents.
- **`modes/`**: Context-dependent files based on `AgentMode` (`operator_bound`, `operator_not_bound`, `cloud_operator_bound`).
- **`system/`**: Global constraints and special states like Sentinel Mode.
- **`tools/`**: Individual descriptions used to populate tool declarations.

### 2. Canonical Personas (`agents.json`)
`@/home/bob/g8e/shared/constants/agents.json:54` is the single source of truth for AI identities.
- **Triage**: Classifier (`lite` tier). Maps to \"Dash\" in GDD §14.1. Determines complexity and posture.
- **Dash**: Fast-path responder (`assistant` tier). Resolves simple, single-step tasks.
- **Sage**: Senior reasoner (`primary` tier). Architect of multi-step investigations.
- **The Tribunal**: Ensemble members (`axiom`, `concord`, `variance`, `pragma`, `nemesis`) that translate intent into shell commands.

### 3. The Tribunal Ensemble
The Tribunal uses specialized prompts in `prompts_data/tribunal/` to ensure technical integrity:
- **Generator**: Round 1 and Round 2 templates. Round 2 provides anonymized clusters to encourage convergence.
- **Persona Overrides**: Each member (e.g., `axiom`, `concord`) has a specific Round 2 prompt to sharpen their unique lens during peer review.
- **Auditor**: A high-reasoning prompt that evaluates candidate clusters against the original intent.

### 4. Warden (Defensive Analysis)
Warden is the defensive coordinator that performs pre-execution risk assessment.
- **Command Risk**: Classifies shell commands as LOW, MEDIUM, or HIGH.
- **File Risk**: Evaluates the cost and reversibility of file writes.
- **Error Analyzer**: Determines if a failure is `AUTO_FIXABLE` or requires `ESCALATE`.
- **Structured Parsing**: Uses `@/home/bob/g8e/components/g8ee/app/llm/structured.py:54` to ensure robust recovery from local models that struggle with JSON formatting.

---

## Authoring Principles

### 1. XML Scaffolding
All sections must be wrapped in XML-like tags to enforce structural boundaries. Use `AgentPersona.format_xml_tag` in `@/home/bob/g8e/components/g8ee/app/utils/agent_persona_loader.py:64` to guarantee consistent formatting.

### 2. Signal Discipline
- **Voice**: Present tense, active voice, technical and direct.
- **Formatting**: Use backticks for symbols (tools, variables, paths).
- **No Emojis**: Strictly forbidden in all prompt files.
- **Positive Framing**: Authorize behavior (\"You are authorized to...\") rather than just prohibiting it.

### 3. Prefix-Cache Optimization
Always maintain the section order defined in `build_modular_system_prompt`. Shared static blocks must remain at the top to ensure high performance across multiple agents sharing the same mode.

---

## Operational Guide

### Adding a New Agent
1. Define the metadata in `@/home/bob/g8e/shared/constants/agents.json:54`.
2. Register the ID in `ReasoningAgent` and update `@/home/bob/g8e/components/g8ee/app/utils/agent_persona_loader.py:27`.

### Adding a Prompt Fragment
1. Create the `.txt` file in `@/home/bob/g8e/components/g8ee/app/prompts_data/`.
2. Register the file in `@/home/bob/g8e/components/g8ee/app/constants/prompts.py:46`.
3. If it's a new section, update `@/home/bob/g8e/components/g8ee/app/llm/prompts.py:496`.

### Verification
Verify prompt assembly and alignment by running:
```bash
/home/bob/g8e/g8e test g8ee -- tests/unit/llm/test_prompts.py
```
