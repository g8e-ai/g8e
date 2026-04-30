---
title: Prompts
parent: Architecture
---

# Prompt System

The g8e prompt system is a hybrid architecture designed for **prefix-cache reuse** and **strict structural enforcement**. It composes a final system prompt from modular fragments, canonical persona definitions, and dynamic turn-specific context.

The system is optimized for high-reasoning models (like those in llama.cpp or vLLM) by placing static, unchanging content at the beginning of the prompt to maximize KV-cache hits across multiple agent turns.

---

## The Assembly Pipeline

The modular system prompt is built by `build_modular_system_prompt` in `components/g8ee/app/llm/prompts.py`. Sections are concatenated in a fixed order based on their stability.

| # | Section | Content Source | Stability | Rationale |
|---|---------|----------------|-----------|-----------|
| 1 | **Safety** | `core/safety.txt` | Global Static | Absolute behavioral guardrails. |
| 2 | **Loyalty** | `core/loyalty.txt` | Global Static | Mission-over-moment doctrine. |
| 3 | **Dissent** | `core/dissent.txt` | Global Static | Protocol for warnings and denials. |
| 4 | **Capabilities** | `modes/{mode}/capabilities.txt` | Per-Mode Static | What the agent can do in this mode. |
| 5 | **Execution** | `modes/{mode}/execution.txt` | Per-Mode Static | How the agent should execute tasks. |
| 6 | **Tools** | `modes/{mode}/tools.txt` | Per-Mode Static | High-level tool usage guidance. |
| 7 | **Response Constraints** | `system/response_constraints.txt` | Global Static | Guidance on response length and style. |
| 8 | **Agent Persona** | `agents.json` via `AgentPersona` | Per-Agent Static | Identity, role, purpose, and autonomy. |
| 9 | **System Context** | `OperatorContext` | Dynamic | Current system state (OS, hostname, etc.). |
| 10 | **Sentinel Mode** | `system/sentinel_mode.txt` | Per-Case Dynamic | Injected only when Sentinel mode is active. |
| 11 | **Triage Context** | `TriageResult` | Per-Turn Dynamic | The classification and posture of the request. |
| 12 | **Investigation Context** | `EnrichedInvestigationContext` | Per-Turn Dynamic | Case details and bound operator list. |
| 13 | **Learned Context** | `InvestigationMemory` | Per-User/Case Dynamic | Durable preferences and past findings. |

---

## Core Components

### 1. Static Fragments (`prompts_data/`)
Located in `components/g8ee/app/prompts_data/`, these `.txt` files provide the doctrinal foundation.
- **`core/`**: Safety, Loyalty, and Dissent fragments shared by all agents.
- **`modes/`**: Context-dependent files based on whether an operator is `bound`, `not_bound`, or a `cloud_operator`.
- **`system/`**: Global constraints and special modes (Sentinel).
- **`tools/`**: Individual descriptions that populate `ToolDeclaration` schemas.

### 2. Canonical Personas (`agents.json`)
`shared/constants/agents.json` is the single source of truth for all AI identities.
- **Identity Mapping**: Per the Tribunal GDD (§14.1), names are mapped to avoid collisions:
  - **GDD Dash** (Interrogator) maps to the `triage` agent.
  - **Code Dash** is the fast-path responder (`assistant` tier).
  - **Sage** is the senior reasoning investigator (`primary` tier).

### 3. The Tribunal Ensemble
The Tribunal uses a different prompt strategy located in `prompts_data/tribunal/`.
- **Generator**: Round 1 and Round 2 templates for member candidates.
- **Persona-specific R2**: `axiom`, `concord`, `variance`, `pragma`, and `nemesis` have unique Round 2 overrides to sharpen their specific lens during peer review.
- **Auditor**: A high-reasoning prompt that evaluates candidate clusters against the original intent.

---

## Authoring Principles

All prompt content must adhere to these invariants to maintain system reliability.

### 1. XML Scaffolding
All sections must be wrapped in XML-like tags to enforce structural boundaries. The `AgentPersona.format_xml_tag` utility in `components/g8ee/app/utils/agent_persona_loader.py` enforces this pattern:
- Opening tag on its own line.
- Content on its own line.
- Closing tag on its own line.

### 2. Positive Framing
Prompts should authorize behavior rather than just prohibiting it.
- **Authorized**: "You are authorized to use X for Y."
- **Positive**: "The standard path for Z is to use the A tool."

### 3. The `<never>` Block
Hard prohibitions are consolidated into `<never>` or `<constraints>` blocks at the end of a fragment. This ensures they are not diluted by descriptive prose and are easily weighted by the model.

### 4. Signal Discipline
- **Voice**: Present tense, active voice, technical and direct.
- **Formatting**: Use backticks for symbols (tools, variables, paths).
- **No Emojis**: Strictly forbidden in all prompt files.
- **Truth over Coherence**: Direct the model to value evidence (command output) over plausible-sounding fabrication.

---

## Operational Guide

### Adding a New Agent
1. Define the metadata in `shared/constants/agents.json`.
2. Add the agent ID to the `ReasoningAgent` enum in `components/g8ee/app/constants/`.

### Adding a Prompt Fragment
1. Create the `.txt` file in the appropriate `prompts_data/` subdirectory.
2. Register the file in the `PromptFile` enum in `components/g8ee/app/constants/prompts.py`.
3. If it's a new section, add it to `PromptSection` and update `build_modular_system_prompt`.

### Verification
Changes to prompts should be verified by running the alignment tests:
```bash
/home/bob/g8e/g8e test g8ee -- tests/unit/llm/test_prompts.py
```
