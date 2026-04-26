---
title: Prompts
parent: Architecture
---

# Prompts

g8ee's prompt system is a hybrid of static file-based fragments (`components/g8ee/app/prompts_data/`) and canonical persona definitions (`shared/constants/agents.json`). This modular architecture is designed for prefix-cache reuse in reasoning models and strict structural enforcement through XML-like scaffolding.

---

## Prompt Categories

Three categories of content are composed into the final prompts:

- **System-prompt fragments** — Static doctrinal files (safety, loyalty, dissent, capabilities, execution, tools, response constraints). These are the shared "base layers" of the system.
- **Agent Personas** — Structured identity, role, and purpose definitions from `agents.json`. Every agent (Sage, Dash, Triage, etc.) has a unique persona block.
- **Tool Descriptions** — Individual files under `prompts_data/tools/` that populate the `description` field of `ToolDeclaration` schemas.
- **Ensemble Prompts** — Single-purpose templates for the Tribunal (generation, auditing) and Warden (risk analysis), often containing complex `{placeholders}` for turn-specific context.

---

## File Layout

### 1. Static Fragments (`components/g8ee/app/prompts_data/`)
```
components/g8ee/app/prompts_data/
├── loader.py                         # load_prompt, load_mode_prompts, list_prompts
├── core/                             # Always loaded (Safety, Loyalty, Dissent)
├── system/                           # Global constraints (Response length, Sentinel mode)
├── modes/                            # Context-dependent behavior (Bound vs. Not Bound)
├── tools/                            # One file per registered tool
└── tribunal/                         # Special-purpose templates for ensemble members
    ├── generator.txt                 # Round 1 generation
    ├── generator_round_2.txt         # Round 2 peer-review base
    ├── auditor.txt                   # Auditor verification
    └── round_2/                      # Persona-specific Round 2 overrides
        └── {axiom, concord, ...}.txt
```

### 2. Canonical Personas (`shared/constants/agents.json`)
This file is the single source of truth for agent identities across all components.
- **`agent.metadata`**: Maps agent IDs (e.g., `sage`, `dash`, `triage`) to their `identity`, `purpose`, and `autonomy` blocks.
- **Terminology Mapping**: Per the Tribunal GDD (§14.1), the code uses specific mappings to resolve naming collisions:
  - **GDD Dash** (Interrogator) -> `triage` agent.
  - **GDD Sage** (Investigator) -> `primary` model tier (using `sage` persona).
  - **Code Dash** -> `assistant` model tier (using `dash` persona), the fast-path responder.

---

## Loader

`components/g8ee/app/prompts_data/loader.py` provides:
- `load_prompt(PromptFile) -> str`: Reads a file from `prompts_data/`. Cached via `@lru_cache`.
- `load_mode_prompts(...) -> dict[str, str]`: Resolves the active `AgentMode` and loads the associated `capabilities`, `execution`, and `tools` files.

**No Interpolation**: The loader returns raw text. Composition (interpolation of `{request}`, etc.) is performed by the calling service (e.g., `llm/prompts.py`) using standard Python `str.format`.

---

## Assembly Pipeline (The Modular System Prompt)

`build_modular_system_prompt` in `components/g8ee/app/llm/prompts.py` is the entry point for all agentic turns. It concatenates sections in a **fixed order optimized for llama.cpp prefix-cache reuse**. Static, unchanging blocks appear first; dynamic, per-turn data appears last.

| # | Section | Content Source | Priority |
|---|---------|----------------|----------|
| 1 | **Safety** | `core/safety.txt` | Static (Shared) |
| 2 | **Loyalty** | `core/loyalty.txt` | Static (Shared) |
| 3 | **Dissent** | `core/dissent.txt` | Static (Shared) |
| 4 | **Capabilities** | `modes/{mode}/capabilities.txt` | Static (Per Mode) |
| 5 | **Execution** | `modes/{mode}/execution.txt` | Static (Per Mode) |
| 6 | **Tools** | `modes/{mode}/tools.txt` | Static (Per Mode) |
| 7 | **Response Constraints** | `system/response_constraints.txt` | Static (Shared) |
| 8 | **Agent Persona** | `agents.json` (wrapped in tags) | Static (Per Agent) |
| 9 | **System Context** | `OperatorContext` metadata | Dynamic (Per Turn) |
| 10 | **Sentinel Mode** | `system/sentinel_mode.txt` | Dynamic (Per Case) |
| 11 | **Triage Context** | `TriageResult` (request_posture) | Dynamic (Per Turn) |
| 12 | **Investigation Context** | `EnrichedInvestigationContext` | Dynamic (Per Turn) |
| 13 | **Learned Context** | User/Case Memories | Dynamic (Per User) |

---

## Authoring Conventions

Every prompt file must adhere to these conventions. Reviews reject violations.

### 1. Positive, Authorizing Language
Tell the model what it **is authorized** to do. Positive framing improves reliability and aligns with the platform's governance posture.
- **Do**: `Authoritative tool for X.`, `The authorized path is to Y.`
- **Avoid**: `Do not X.`, `Never need to Z.` (Unless in a `<never>` block).

### 2. XML Scaffolding
Prompt sections use XML-like tags to enforce structural boundaries. `AgentPersona.format_xml_tag` enforces a canonical "scaffolding" pattern:
- Opening tag on its own line.
- Content on its own line.
- Closing tag on its own line.

Conventional tags:
- `<identity>`, `<role>`, `<purpose>`, `<autonomy>`: Persona structure.
- `<never>`: Consolidated prohibitions (always at the end of the file).
- `<system_context>`, `<investigation_context>`, `<learned_context>`: Dynamic sections.

### 3. The `<never>` Block
Hard prohibitions (statements of the form "Never X") live in a dedicated `<never>` block at the end of a file. This isolates absolute rules from descriptive prose, making them easier for the model to attend to and for humans to audit.

### 4. Signal Discipline
- **Tense**: Present tense, active voice.
- **Formatting**: Backticks for tool names, parameter names, file paths, shell commands.
- **No Emojis**: Emojis are strictly forbidden in prompt files.
- **Time/Date**: Reference the current year (2026) and knowledge cutoff (Jan 2025) if timing is critical.

---

## Adding a New Agent or Prompt

1. **For a new Agent**: Add a entry to `shared/constants/agents.json`. Use `id`, `role`, `identity`, `purpose`, and `autonomy`.
2. **For a new Fragment**: Add the `.txt` file to `prompts_data/` and register it in `PromptFile` (`constants/prompts.py`).
3. **For a new Section**: Register a `PromptSection` member and wire it into `build_modular_system_prompt`.
4. **Verification**: Update unit tests in `components/g8ee/tests/unit/llm/` to assert on section order and content.
