---
doc_id: ensemble_prompts
title: Prompt Assembly and Management
audience: maintainers and coding agents
status: current
last_updated: 2026-09-28
version: v2.2.3
owners:
  - ensemble/app/llm/prompts.py
  - ensemble/app/prompts_data/
  - ensemble/app/utils/agent_persona_loader.py
  - ensemble/app/constants/prompts.py
related:
  - docs/ensemble/architecture/ensemble.md
  - docs/ensemble/agents.md
  - docs/ensemble/protocol.md
  - docs/ensemble/governance.md
  - docs/ensemble/thinking.md
  - docs/ensemble/llm-providers.md
when_to_read: Implementing, auditing, or extending system prompts, persona models, Tribunal prompts, mode-specific rules, tool descriptions, prompt loading, or protocol alignment.
do_not_use_for:
  - Runtime chat flow and message handling (docs/ensemble/architecture/ensemble.md)
  - Agent and persona roster definitions (docs/ensemble/agents.md)
  - Gateway protocol integration (docs/ensemble/protocol.md)
  - Verification and approval flow (docs/ensemble/governance.md)
  - Provider-specific generation and tool declarations (docs/ensemble/llm-providers.md)
---

# Prompt Assembly and Management

## Purpose

Defines how g8ee assembles modular system prompts from text files and typed models, manages mode-specific capabilities and tools, and structures Tribunal prompts for command generation. The prompt builder is the single authority for system prompt composition; prompt text does not itself authorize operations or cross trust boundaries.

## Quick index

- [Purpose](#purpose)
- [Invariants](#invariants)
- [Owned surfaces](#owned-surfaces)
- [Procedures](#procedures)
- [Anti-patterns](#anti-patterns)
- [Links out](#links-out)

Invariant groups: [System prompt composition](#system-prompt-composition-inv-prompt-comp), [Prompt loading and caching](#prompt-loading-and-caching-inv-prompt-load), [Tribunal prompts](#tribunal-prompts-inv-tribunal), [Tool descriptions and mode rules](#tool-descriptions-and-mode-rules-inv-tools).

## Invariants

Ids are stable. Append the next free number in a topic. Do not renumber.

### System prompt composition (`INV-PROMPT-COMP`)

| ID | Rule |
| --- | --- |
| INV-PROMPT-COMP-01 | The system prompt builder `build_modular_system_prompt()` in ensemble/app/llm/prompts.py is the sole authority on section order, inclusion conditions, and content. It assembles sections in order: safety, loyalty, dissent, capabilities, execution, tools, response constraints, identity/persona, system context, sentinel mode, triage context, investigation context, learned context. Sections with empty inputs or absent mode content are omitted. |
| INV-PROMPT-COMP-02 | The builder returns a tuple of (complete_prompt_string, context_sizes_dict) where context_sizes maps section label to character count. The shared platform-static prefix (safety, loyalty, dissent, mode files, response constraints) appears first to enable llama.cpp and vLLM prefix-cache reuse; per-turn dynamic context follows to avoid invalidating the cached prefix. |
| INV-PROMPT-COMP-03 | Persona models render XML layout in order: <role>, optional <output_contract>, <identity>, optional <purpose>, optional <autonomy>. Sage's <identity> contains the <agentic_reasoning> discipline block; Dash does not. Supplying a persona replaces the fallback core/identity.txt, preventing duplicate role blocks. |
| INV-PROMPT-COMP-04 | Mode selection is binary: when operator_bound is True, load AgentMode.G8E_BOUND mode files; when False, load AgentMode.G8E_NOT_BOUND. When no Operator is bound and g8e_web_search_available is False, substitute capabilities_no_search.txt and execution_no_search.txt for the standard not-bound files. The tools file is always loaded but the tools section is omitted from the prompt when no Operator is bound and no web search is available. |
| INV-PROMPT-COMP-05 | Container contexts without systemd receive an explicit warning: "WARNING: systemd is NOT available - do NOT use systemctl, journalctl, or other systemd commands". Non-container contexts with init systems render the init system name without the warning. |
| INV-PROMPT-COMP-06 | System context fields are rendered in order: operator type, granted intents, OS, hostname, username (with uid), working directory, container environment and runtime, init system, then all other non-empty model fields. Multiple operator contexts are wrapped in indexed <operator> blocks; single contexts omit the wrapper. |

### Prompt loading and caching (`INV-PROMPT-LOAD`)

| ID | Rule |
| --- | --- |
| INV-PROMPT-LOAD-01 | load_prompt(prompt_file: PromptFile) requires a PromptFile enum member, reads UTF-8 text from ensemble/app/prompts_data/, caches up to 128 distinct enum arguments with @lru_cache, and raises TypeError for non-PromptFile arguments or ResourceNotFoundError when the path does not exist. |
| INV-PROMPT-LOAD-02 | load_mode_prompts(operator_bound: bool, g8e_web_search_available: bool = True) caches up to 16 argument combinations. It loads capabilities, execution, and tools for the selected bound/not-bound mode, including no-search variants when applicable. A missing mode file is logged and represented by an empty string rather than aborting the load. |
| INV-PROMPT-LOAD-03 | list_prompts(subdirectory: str = "") returns a dict mapping underscore-normalized relative names to Path objects for discovered .txt files under ensemble/app/prompts_data/. A missing directory returns an empty dict. |
| INV-PROMPT-LOAD-04 | clear_cache() clears both the prompt and mode-prompt caches. Used by tests and hot-reloading workflows. |

### Tribunal prompts (`INV-TRIBUNAL`)

| ID | Rule |
| --- | --- |
| INV-TRIBUNAL-01 | Tribunal prompt builders (build_tribunal_generator_prompt(), build_tribunal_auditor_prompt()) are separate from the conversational system-prompt builder. They format requests, operator context, constraints, and (for the auditor) candidate-cluster context. All Tribunal members receive the same request intent; member-specific R2 prompts exist for Axiom, Concord, Variance, Pragma, and Nemesis. |
| INV-TRIBUNAL-02 | Round one uses ensemble/app/prompts_data/tribunal/generator.txt, formatting the request, guidelines, target OS, shell, user context, working directory, operator context, forbidden patterns, and active whitelist/blacklist constraints. Tribunal members return one exact command string or the template-defined error form, without commentary or Markdown. |
| INV-TRIBUNAL-03 | Round two uses ensemble/app/prompts_data/tribunal/generator_round_2.txt or the member-specific file under ensemble/app/prompts_data/tribunal/round_2/ when a recognized member ID is supplied. It additionally formats the anonymized candidate-cluster context. |
| INV-TRIBUNAL-04 | The auditor uses ensemble/app/prompts_data/tribunal/auditor.txt and context produced by build_tribunal_auditor_context(), which distinguishes unanimous, majority, and tied outcomes and specifies permitted response fields: ok, revised, or swap. The caller validates the Auditor's structured response. |
| INV-TRIBUNAL-05 | Tribunal model agreement is application reasoning; it does not create protocol L2 signatures or replace Gateway and Operator verification. |

### Tool descriptions and mode rules (`INV-TOOLS`)

| ID | Rule |
| --- | --- |
| INV-TOOLS-01 | Per-tool descriptions are loaded by each tool module's build() function from the corresponding PromptFile under ensemble/app/prompts_data/tools/. The description is placed in the provider-facing ToolDeclaration; the typed tool schema is built separately from the tool's Pydantic argument model. |
| INV-TOOLS-02 | Registered tools and their execution handlers are declared in ensemble/app/services/ai/tool_registry.py through TOOL_SPECS, which also defines scope, supported agent modes, display metadata, and the web-search condition. |
| INV-TOOLS-03 | Mode tools.txt files (for bound and not-bound modes) contain shared advertisements and usage rules, not per-tool parameter schemas. Parameter, return, behavior, examples, Sentinel, or data-sovereignty guidance belongs in the individual tool description when applicable. Alignment tests enforce that mode files do not embed <parameters>, <returns>, or <behavior> blocks or parameter-bullet specifications, and that registered active tools have description files. |

## Owned surfaces

| Claim | Path | Verify |
| --- | --- | --- |
| System prompt builder | ensemble/app/llm/prompts.py:508 | Section order, inclusion conditions, return type match code |
| Prompt text files | ensemble/app/prompts_data/ | Files exist for all paths in PromptFile enum |
| Prompt loading API | ensemble/app/prompts_data/loader.py | load_prompt, load_mode_prompts, list_prompts, clear_cache implementations |
| Persona rendering | ensemble/app/utils/agent_persona_loader.py:81 | XML tag ordering and content structure match code |
| Tribunal builders | ensemble/app/llm/prompts.py:354 | Template loading, member-specific R2 selection, context formatting match code |
| System context builder | ensemble/app/llm/prompts.py:439 | Field ordering, operator wrapping, container warning logic match code |
| Prompt constants | ensemble/app/constants/prompts.py | AgentMode, PromptFile, PromptSection enums match section names and file paths |

## Procedures

### Building a modular system prompt

1. Call build_modular_system_prompt(operator_bound, system_context, user_memories, case_memories, investigation, g8e_web_search_available, triage_result, agent_name) from the chat pipeline.
2. The builder loads the static shared prefix: safety, loyalty, dissent, mode capabilities/execution/tools, and response constraints.
3. The builder appends the agent-specific persona (if agent_name is supplied) or the fallback identity.
4. The builder appends per-turn dynamic context if present: system context, sentinel mode, triage, investigation, learned context.
5. The builder logs section counts and character sizes, then returns the complete prompt and a context_sizes dict.

### Loading a prompt file

1. Acquire a PromptFile enum member (e.g., PromptFile.CORE_SAFETY).
2. Call load_prompt(prompt_file).
3. The loader resolves the path relative to ensemble/app/prompts_data/, reads UTF-8 text, and returns it. The result is cached.
4. On missing file, raises ResourceNotFoundError. On non-PromptFile argument, raises TypeError.

### Loading mode-specific prompts

1. Call load_mode_prompts(operator_bound, g8e_web_search_available).
2. The loader selects the appropriate mode (G8E_BOUND or G8E_NOT_BOUND).
3. If not bound and search is unavailable, the loader substitutes the no-search variants.
4. The loader returns a dict keyed by PromptSection string with loaded prompt content. Missing files are logged and represented as empty strings.

### Rendering a persona

1. Retrieve the AgentPersonaModel for the agent ID via get_agent_persona(agent_id).
2. Call get_system_prompt() on the persona.
3. The persona formats XML tags in order: <role>, optional <output_contract>, <identity>, optional <purpose>, optional <autonomy>.
4. Return the formatted XML string.

### Building a Tribunal prompt (round one)

1. Acquire the request, guidelines, operator context, and constraints.
2. Call build_tribunal_prompt_fields() to build common template kwargs.
3. Call build_tribunal_generator_prompt(..., round_num=1).
4. The builder loads tribunal/generator.txt, formats it with the kwargs, and returns the prompt string.

### Building a Tribunal prompt (round two)

1. Acquire the same fields as round one, plus anonymized cluster context.
2. Call build_tribunal_generator_prompt(..., round_num=2, cluster_context=..., member=...).
3. If a member ID is supplied, the builder maps it to the member-specific R2 file; otherwise uses the generic R2 file.
4. The builder formats the template with all fields including cluster_context and returns the prompt string.

### Building a Tribunal auditor prompt

1. Acquire the request, guidelines, operator context, constraints, and audit mode (unanimous/majority/tied).
2. Call build_tribunal_auditor_context() to produce the mode-specific context block.
3. Call build_tribunal_auditor_prompt().
4. The builder loads tribunal/auditor.txt, formats it with all fields, and returns the prompt string.

### Clearing the prompt cache

1. Call clear_cache() to flush both the prompt and mode-prompt caches.
2. Used in test teardown or hot-reloading workflows.

## Anti-patterns

- Hand-editing generated protobuf references or Swagger JSON without updating sources (docs/devs/docs.md).
- Calling build_modular_system_prompt() with mismatched types (e.g., passing a string instead of OperatorContext for system_context).
- Loading prompt files directly with open() instead of load_prompt(PromptFile....) — bypasses caching and validation.
- Adding prompt content to Python code instead of text files under ensemble/app/prompts_data/ — defeats the purpose of the loader.
- Duplicating persona content in multiple prompt files — use the persona model as the SSOT.
- Embedding per-tool parameter schemas in mode tools.txt files — move them to individual tool descriptions.
- Assuming prompt sections are unconditionally included — verify omission conditions for each section.
- Calling build_tribunal_generator_prompt() without validating the member ID — invalid IDs fall back to the generic R2 file silently.

## Links out

- [Ensemble Architecture](../architecture/ensemble.md) — Runtime boundary, chat flow, tool execution, governance limits.
- [Agents](agents.md) — Persona roster, Tribunal roles, operational flow.
- [Protocol](protocol.md) — g8ee's Gateway-facing protocol integration.
- [Governance](governance.md) — Five-layer verification and envelope lifecycle.
- [Thinking](thinking.md) — Provider reasoning budgets and thought signatures.
- [LLM Providers](llm-providers.md) — Provider interfaces, tool declarations, generation configuration.
- [Developer Guidelines](../devs/docs.md) — Documentation audit and ownership rules.
