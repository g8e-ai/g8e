# Prompts

g8ee's system prompts and tool descriptions are plain-text files under `components/g8ee/app/prompts_data/`, loaded at runtime by a small file loader and composed into the final LLM system prompt by `build_modular_system_prompt`. This document covers file layout, loader mechanics, assembly order, mode selection, the tool-description pipeline, and the authoring conventions every prompt file is expected to follow.

---

## What Counts as a Prompt

Two categories of text file are treated as prompts by g8ee:

- **System-prompt fragments** — identity, safety, loyalty, dissent, per-mode capabilities/execution/tools, response constraints, Sentinel-mode notice. These are concatenated by `build_modular_system_prompt` into the `system` message of every LLM call.
- **Tool descriptions** — one file per registered tool, loaded into the `description` field of each `ToolDeclaration`. These are rendered to the model as part of the tool schema, not the system prompt, but they operate on the model with the same semantics and therefore follow the same authoring conventions.

A third category, **analysis prompts** (`prompts_data/analysis/*.txt`), contains templated prompts used by Tribunal command analysis, file-risk analysis, and error suggestion. They are loaded through the same `load_prompt` machinery but fed into separate single-purpose LLM calls, not the agent's system prompt.

---

## File Layout

```
components/g8ee/app/prompts_data/
├── __init__.py
├── loader.py                         # load_prompt, load_mode_prompts, list_prompts, clear_cache
├── core/                             # Always loaded, in this order
│   ├── identity.txt                  # Who the agent is
│   ├── safety.txt                    # Absolute forbidden operations
│   ├── loyalty.txt                   # Mission-over-moment doctrine
│   └── dissent.txt                   # Warning protocol, denial memory, escalation response
├── system/                           # Conditionally loaded
│   ├── response_constraints.txt      # Appended after investigation_context on every turn
│   └── sentinel_mode.txt             # Only when investigation.sentinel_mode is True
├── modes/                            # Exactly one subtree loaded per turn
│   ├── operator_bound/
│   │   ├── capabilities.txt
│   │   ├── execution.txt
│   │   └── tools.txt
│   ├── operator_not_bound/
│   │   ├── capabilities.txt
│   │   ├── execution.txt
│   │   ├── tools.txt
│   │   ├── capabilities_no_search.txt
│   │   └── execution_no_search.txt
│   └── cloud_operator_bound/
│       ├── capabilities.txt
│       ├── execution.txt
│       └── tools.txt
├── tools/                            # One file per registered tool (see Tool Descriptions)
│   └── *.txt
└── analysis/                         # Standalone, non-agent prompts
    ├── command_risk.txt
    ├── error_analysis.txt
    └── file_risk.txt
```

Every file under `prompts_data/` has a corresponding entry in the `PromptFile` enum at `components/g8ee/app/constants/prompts.py`. The enum value is the path relative to `prompts_data/`; nothing else in the codebase references these files by string path.

---

## Loader

`components/g8ee/app/prompts_data/loader.py` exposes three public functions.

| Function | Purpose |
|----------|---------|
| `load_prompt(prompt_file: PromptFile) -> str` | Read a single prompt file. Raises `ResourceNotFoundError` when the file is missing. Cached via `@lru_cache(maxsize=128)`. |
| `load_mode_prompts(operator_bound, is_cloud_operator=False, g8e_web_search_available=True) -> dict[str, str]` | Resolve the active `AgentMode` and load its three section files (`capabilities`, `execution`, `tools`) into a dict keyed by `PromptSection`. Cached via `@lru_cache(maxsize=16)`. |
| `clear_cache()` | Invalidate both caches. Intended for test harnesses and hot-reload paths. |

A fourth helper, `list_prompts(subdirectory="")`, exists for introspection and is not used at runtime.

The loader reads UTF-8 files and returns their raw contents. **It does no templating, no XML parsing, no interpolation** — the text is handed to the LLM verbatim, concatenated with newlines by `build_modular_system_prompt`. This is intentional: prompt composition lives in Python; the files are inert content.

Placeholder substitution (`{command}`, `{stdout}`, `{working_dir}`, etc.) in `analysis/*.txt` is performed by the caller of `load_prompt` via standard `str.format`, not by the loader.

---

## Agent Modes

`AgentMode` (defined in `components/g8ee/app/constants/prompts.py`) has three members:

| Mode | When selected |
|------|---------------|
| `OPERATOR_BOUND` (`g8e.bound`) | At least one non-cloud Operator is bound to the investigation. |
| `CLOUD_OPERATOR_BOUND` (`cloud.g8e.bound`) | At least one bound Operator has `is_cloud_operator=True`. |
| `OPERATOR_NOT_BOUND` (`g8e.not.bound`) | No Operator is bound. Advisory mode. |

`load_mode_prompts` applies precedence `cloud > bound > not-bound`: a mixed binding with one cloud and one standard Operator resolves to `CLOUD_OPERATOR_BOUND`.

`AGENT_MODE_PROMPT_FILES` in `components/g8ee/app/constants/prompts.py` maps each mode to its three section files:

| Mode | Capabilities | Execution | Tools |
|------|--------------|-----------|-------|
| `OPERATOR_BOUND` | `modes/operator_bound/capabilities.txt` | `modes/operator_bound/execution.txt` | `modes/operator_bound/tools.txt` |
| `OPERATOR_NOT_BOUND` | `modes/operator_not_bound/capabilities.txt` | `modes/operator_not_bound/execution.txt` | `modes/operator_not_bound/tools.txt` |
| `CLOUD_OPERATOR_BOUND` | `modes/cloud_operator_bound/capabilities.txt` | `modes/cloud_operator_bound/execution.txt` | `modes/cloud_operator_bound/tools.txt` |

### No-search variant

When `operator_bound=False` **and** `g8e_web_search_available=False`, `load_mode_prompts` swaps the capabilities and execution files for their `_no_search` variants. The tools section stays unchanged — `build_modular_system_prompt` suppresses it entirely in this configuration so the model never sees an empty tools block. `g8e_web_search_available` is sourced from `AIToolService.g8e_web_search_available`, which is true only when a `WebSearchProvider` was injected and `g8e_web_search` is registered in `_tool_declarations`.

---

## Assembly Pipeline

`build_modular_system_prompt` in `components/g8ee/app/llm/prompts.py` is the sole entry point for constructing a system prompt. Every other service that needs a system prompt must call it — direct string assembly is forbidden.

Sections are concatenated in this fixed order:

| # | Section | Source | Conditional on |
|---|---------|--------|----------------|
| 1 | `identity` | `PromptFile.CORE_IDENTITY` | always |
| 2 | `safety` | `PromptFile.CORE_SAFETY` | always |
| 3 | `loyalty` | `PromptFile.CORE_LOYALTY` | always |
| 4 | `dissent` | `PromptFile.CORE_DISSENT` | always |
| 5 | `capabilities` | mode file | loaded file is non-empty |
| 6 | `execution` | mode file | loaded file is non-empty |
| 7 | `tools` | mode file | `operator_bound or g8e_web_search_available` |
| 8 | `system_context` | rendered from `OperatorContext` | an Operator context was provided |
| 9 | `sentinel_mode` | `PromptFile.SYSTEM_SENTINEL_MODE` | `investigation.sentinel_mode is True` |
| 10 | `triage_context` | rendered from `TriageResult` | a triage result was provided |
| 11 | `investigation_context` | rendered from `EnrichedInvestigationContext` | investigation context provided |
| 12 | `response_constraints` | `PromptFile.SYSTEM_RESPONSE_CONSTRAINTS` | always |
| 13 | `learned_context` | rendered from user + case memories | any memory is present |

Section order is deliberate. Identity and safety come first because everything after them should be interpreted inside that envelope. Loyalty and dissent come before mode-specific capabilities so that the doctrine governs how capabilities are exercised. `response_constraints` comes last among the static sections so its formatting rules are the most recent instruction in the prompt before learned context and the user turn.

---

## Tool Descriptions

Tool descriptions are prompt files loaded into the `description` field of each `ToolDeclaration` in `components/g8ee/app/services/ai/tool_service.py`. Each `_build_*_tool` method calls `load_prompt(PromptFile.TOOL_*)` and attaches the result to the declaration:

```python
declaration = types.ToolDeclaration(
    name=OperatorToolName.RUN_COMMANDS,
    description=load_prompt(PromptFile.TOOL_RUN_COMMANDS),
    parameters=schema_from_model(OperatorCommandArgs, required_override=["command", "justification"]),
)
```

The model sees these descriptions as part of the tool schema, but they operate on the model's behaviour the same way a system-prompt fragment does. **Tool description files follow every authoring convention defined below.**

Tool advertisement is filtered per-mode by `AIToolService.get_tools(agent_mode, model_to_use)`, which walks `TOOL_SPECS` (see `components/g8ee/app/services/ai/tool_registry.py`) and returns only tools whose `agent_modes` contains the active mode and whose name is currently registered. `OPERATOR_TOOLS` and `AI_UNIVERSAL_TOOLS` (frozensets of tool names) are also exported by `tool_registry.py`, derived from the `TOOL_SPECS` table.

---

## Authoring Conventions

Every prompt file in `prompts_data/` is written in a single consistent voice. The conventions below are enforceable — reviews reject violations.

### Positive, authorizing language

Prompts tell the model what **is authorized** rather than what is prohibited. The model performs better on positive framings, and the voice matches the governance posture of the platform: g8ee authorizes the agent to act, it does not merely prohibit misbehaviour.

Preferred patterns:

- `Authoritative tool for X.` — states the tool's authorized scope.
- `The authorized path is to Y.` — names the sanctioned procedure.
- `Only after Z has completed is the call authorized.` — gates an action by a condition.
- `Treat the scrubbed output as authoritative for analysis.` — elevates a default channel.

Avoid:

- `Do not X.` — shaming.
- `X instead of Y.` / `Preferred over Y.` — defining by exclusion.
- `X, not Y.` — rhetorical contrast that doubles the model's working set.
- `Never need to Z.` — hedged prohibition.

### The `<never>` block convention

Genuine prohibitions — statements of the form "Never X" — live in a dedicated `<never>` XML block, one per prompt file, typically at the end of the file. Each item starts with `Never …` and is a single self-contained directive.

Example (from `components/g8ee/app/prompts_data/modes/operator_bound/tools.txt`):

```xml
<never>
- Never invoke a tool silently; the conversational text response that precedes the call is mandatory.
- Never skip the post-tool analysis; every tool result is paired with an explanatory text response.
- Never call `run_commands_with_operator` before `get_command_constraints` has returned for this turn.
</never>
```

This convention has three benefits:

- **Isolation.** Prohibitions are one coherent list rather than scattered across prose the model has to parse line-by-line.
- **Declarative form.** `Never X` is absolute and unambiguous; the model handles it reliably.
- **Auditability.** A reviewer can answer "what are the hard rules for this file?" by reading one block.

### XML tag conventions

Prompt files use XML-like tags to structure content. The loader does not parse them — they are there for the model. Conventional tags:

| Tag | Purpose |
|-----|---------|
| `<identity>`, `<platform>`, `<role>`, `<operating_principles>` | Top-level identity structure in `core/identity.txt`. |
| `<safety>`, `<execution_posture>`, `<credential_handling>`, `<sensitive_data_protection>`, `<forbidden_operations>` | Safety-doctrine structure in `core/safety.txt`. |
| `<loyalty_doctrine>`, `<principle id="N" name="...">`, `<behavioral_rules>`, `<authorized_patterns>` | Loyalty-doctrine structure in `core/loyalty.txt`. |
| `<never>` | Consolidated prohibitions block. One per file, typically at the end. |
| `<capabilities mode="...">`, `<execution mode="...">`, `<tools mode="...">` | Mode-specific section wrappers. The `mode` attribute is informational; the loader selects files by mode, not by the attribute. |
| `<parameters>`, `<returns>`, `<behavior>`, `<examples>`, `<example>` | Standard per-tool description structure. |
| `<scope>` | Replaces older `<caution>` — describes the tool's authorized operating scope. |

### Hard-boundary prohibitions

The `<forbidden_operations>` list in `core/safety.txt` is the one exception to the positive-language rule. Those six items are genuine absolute refusals (`rm -rf /`, data exfiltration, backdoor creation, etc.) and the negative framing is doing real signalling work for the model. Leave that section alone.

### Style

- Present tense. Active voice.
- Backticks for tool names, parameter names, file paths, shell commands.
- No emojis.
- Inline code examples are acceptable in tool-description files when they clarify the parameter shape or the authorized call pattern. Avoid introducing new command/code snippets into `core/` or `system/` files — those files are doctrinal, not operational.
- Tool descriptions open with a single sentence naming the tool's authorized scope, then a `<parameters>` block, then any supplementary sections (`<behavior>`, `<returns>`, `<examples>`, `<never>`).

---

## Adding a New Prompt File

1. Create the `.txt` file under the correct subdirectory of `components/g8ee/app/prompts_data/`.
2. Add a new member to the `PromptFile` enum in `components/g8ee/app/constants/prompts.py`. The enum value is the path relative to `prompts_data/`.
3. For a new mode file, add it to the appropriate inner dict in `AGENT_MODE_PROMPT_FILES`.
4. For a new section that plugs into `build_modular_system_prompt`, add a `PromptSection` member and wire it into the assembly order in `components/g8ee/app/llm/prompts.py`.
5. Write the file following the authoring conventions above. Prohibitions go in a `<never>` block.
6. Update any test in `components/g8ee/tests/` that asserts on section counts or prompt content.

---

## Adding a New Tool Description

1. Create `components/g8ee/app/prompts_data/tools/<tool_name>.txt` following the tool-description structure above.
2. Add a `TOOL_<NAME>` member to the `PromptFile` enum pointing at the new file.
3. Add a `_build_<name>_tool` method on `AIToolService` that loads the description via `load_prompt(PromptFile.TOOL_<NAME>)` and attaches it to the `ToolDeclaration`.
4. Register the tool in `TOOL_SPECS` (`components/g8ee/app/services/ai/tool_registry.py`) with the appropriate `agent_modes`, `builder_attr`, `handler_attr`, and display metadata.
5. Add a `_handle_<name>` handler method on `AIToolService` with the uniform signature: `(self, tool_args, investigation, g8e_context, request_settings) -> ToolResult`.

---

## Caching and Hot Reload

`load_prompt` and `load_mode_prompts` both use `functools.lru_cache`. In production, prompt files are read once per unique argument set for the lifetime of the process. Test harnesses that mutate prompt files on disk must call `clear_cache()` between mutations for changes to be observable.

The entire prompt assembly is recomputed per-turn inside `build_modular_system_prompt` — only the file reads are cached, not the composed system prompt. This is intentional: the composed prompt depends on per-turn inputs (`OperatorContext`, `TriageResult`, `EnrichedInvestigationContext`, user/case memories) that change every turn.

---

## Cross-References

- Mode selection and tool-advertisement rules: `docs/components/g8ee.md` (`## Prompt System`, `#### Tool Availability Model`).
- Tribunal prompts and command-constraint injection: `docs/components/g8ee.md` (`## Tribunal`).
- Triage pipeline producing `TriageResult`: `docs/architecture/ai_agents.md`.
- LLM provider abstraction that consumes the assembled system prompt: `docs/architecture/thinking_levels.md` (thinking translators) and `docs/components/g8ee.md` (provider adapters).
