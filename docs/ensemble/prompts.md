# Prompts

## Scope and ownership

g8ee assembles prompts from three sources: text files under `ensemble/app/prompts_data/`, typed persona models under `ensemble/app/models/personas/`, and runtime context built by `ensemble/app/llm/prompts.py`. The prompt loader removes prompt prose from Python source and caches loaded text; it does not make prompt content an authorization boundary. g8ee remains outside the trusted execution boundary, and prompt instructions, persona output, Tribunal agreement, and application approvals do not authorize host or platform mutations. Governed operations still cross the Gateway and executing Operator paths described in [Ensemble Architecture](../architecture/ensemble.md).

The main system-prompt entry point is `build_modular_system_prompt()` in `ensemble/app/llm/prompts.py`. Chat uses it for the Sage and Dash reasoning paths. The function returns the complete prompt and a character-count map for the sections that were actually included.

## System-prompt assembly

The builder appends sections in this order. Sections whose inputs are absent, or whose mode-specific content is empty, are omitted.

1. **Safety** — `core/safety.txt`, loaded as `PromptFile.CORE_SAFETY`.
2. **Loyalty** — `core/loyalty.txt`, loaded as `PromptFile.CORE_LOYALTY`.
3. **Dissent** — `core/dissent.txt`, loaded as `PromptFile.CORE_DISSENT`.
4. **Capabilities** — the selected mode's capability file.
5. **Execution** — the selected mode's execution file.
6. **Tools** — the selected mode's high-level tool rules, included only when the mode has tool text and either an Operator is bound or g8e web search is available.
7. **Response constraints** — `system/response_constraints.txt`, loaded as `PromptFile.SYSTEM_RESPONSE_CONSTRAINTS`.
8. **Agent persona or identity** — when `agent_name` is supplied, `get_agent_persona(agent_name.value).get_system_prompt()` produces the persona block. Otherwise, `core/identity.txt`, loaded as `PromptFile.CORE_IDENTITY`, is used as the fallback.
9. **System context** — when present, `_build_system_context_section()` emits a `<system_context>` block for one or more `OperatorContext` values. Multiple contexts are wrapped in indexed `<operator>` blocks. The rendered fields include operator type, granted intents, OS, hostname, username and UID, working directory, container runtime, init system, and other non-empty model fields. Container contexts without systemd also receive a warning not to use `systemctl` or `journalctl`.
10. **Sentinel mode** — `system/sentinel_mode.txt`, loaded as `PromptFile.SYSTEM_SENTINEL_MODE`, is included when the investigation's `sentinel_mode` field is exactly `True`.
11. **Triage context** — when triage provides a request posture, `build_triage_context_section()` emits `<triage_context>` with `request_posture` and, when available, `intent_summary`.
12. **Investigation context** — when an investigation is present, `build_investigation_context_section()` emits case title, description, status, priority, severity, conversation-history availability, and bound-operator summaries when those values exist.
13. **Learned context** — when user or case memories produce content, `build_learned_context_section()` emits `<learned_context>` containing communication preferences, technical background, response style, problem-solving approach, interaction style, and previous investigation summaries.

The first seven sections form the relatively stable shared and mode-dependent prefix. The persona follows that prefix, and runtime system, Sentinel, triage, investigation, and learned context follow the persona so changing per-turn state does not invalidate the shared prefix unnecessarily. Prefix-cache reuse is an implementation goal for providers such as llama.cpp and vLLM, not a provider-side correctness guarantee.

Persona models render the following XML layout: `<role>`, optional `<output_contract>`, `<identity>`, optional `<purpose>`, and optional `<autonomy>`. Sage's identity contains the `<agentic_reasoning>` discipline block; Dash does not. Supplying a persona replaces the fallback identity file, preventing duplicate role blocks.

## Runtime modes

The runtime selects a mode from one boolean: `operator_bound`. `True` selects `AgentMode.G8E_BOUND` and loads `modes/operator_bound/{capabilities,execution,tools}.txt`. `False` selects `AgentMode.G8E_NOT_BOUND` and loads `modes/operator_not_bound/{capabilities,execution,tools}.txt`.

When no Operator is bound and `g8e_web_search_available` is `False`, the loader replaces the standard not-bound capability and execution files with `capabilities_no_search.txt` and `execution_no_search.txt`. The not-bound tools file is still loaded, but the system-prompt builder omits the tools section when web search is unavailable. The registered tool set is separately derived from `ToolSpec.agent_modes` and the web-search registration gate; prompt text does not itself expose or authorize a tool.

`protocol/constants/prompts.json` declares `g8e.cloud.bound` as a protocol constant, but the current Python `AgentMode` and `load_mode_prompts()` implementation do not select a cloud-specific prompt directory. Cloud-provider metadata can appear in operator context, but it does not create a separate prompt mode in this builder.

## Tool descriptions and mode rules

Per-tool descriptions are loaded by each tool module's `build()` function from a corresponding `PromptFile` under `app/prompts_data/tools/`. The description is placed in the provider-facing `ToolDeclaration`; the typed tool schema is built separately from the tool's Pydantic argument model. Registered tools and their execution handlers are declared in `app/services/ai/tool_registry.py` through `TOOL_SPECS`, which also defines scope, supported agent modes, display metadata, and the web-search condition.

Mode `tools.txt` files contain shared advertisements and usage rules, not per-tool parameter schemas. Parameter, return, behavior, examples, Sentinel, or data-sovereignty guidance belongs in the individual tool description when applicable. The alignment tests enforce that mode files do not embed `<parameters>`, `<returns>`, or `<behavior>` blocks or parameter-bullet specifications, and that registered active tools have description files.

## Tribunal prompts

The Tribunal prompt builders are separate from the conversational system-prompt builder:

- `build_tribunal_generator_prompt()` loads `tribunal/generator.txt` for round one. It formats the natural-language request, guidelines, target OS, shell, user context, working directory, operator context, forbidden command patterns, and active whitelist/blacklist constraints. Tribunal members receive the request as intent and return one exact command string or the template-defined error form, without commentary or Markdown.
- For round two, the same builder loads `tribunal/generator_round_2.txt`, or the member-specific file under `tribunal/round_2/` when a recognized member ID is supplied. It additionally formats the anonymized candidate-cluster context.
- `build_tribunal_auditor_prompt()` loads `tribunal/auditor.txt` and adds the request, constraints, operator context, and the context produced by `build_tribunal_auditor_context()`. The context distinguishes `unanimous`, `majority`, and `tied` outcomes and specifies the permitted `ok`, `revised`, or `swap` response fields. The caller parses and validates the Auditor's structured response.

The five Tribunal personas are Axiom, Concord, Variance, Pragma, and Nemesis. Their calls are isolated from one another during candidate generation. Tribunal model agreement is application reasoning; it does not create protocol L2 signatures or replace Gateway and Operator verification.

## Loading and caching

`app.prompts_data.loader` resolves prompt paths relative to `PROMPTS_DIR`, which is the `app/prompts_data` package directory.

- `load_prompt(prompt_file: PromptFile) -> str` requires a `PromptFile` enum member, reads UTF-8 text, caches up to 128 distinct enum arguments with `@lru_cache`, and raises `TypeError` for a non-`PromptFile` argument or `ResourceNotFoundError` when the path does not exist.
- `load_mode_prompts(operator_bound: bool, g8e_web_search_available: bool = True) -> dict[str, str]` caches up to 16 argument combinations. It loads capabilities, execution, and tools for the selected bound/not-bound mode. A missing mode file is logged and represented by an empty string rather than aborting the mode load.
- `list_prompts(subdirectory: str = "")` returns a mapping of underscore-normalized relative names to `Path` objects for discovered `.txt` files. A missing directory returns an empty mapping.
- `clear_cache()` clears both prompt caches and is used for tests and prompt hot-reloading workflows.

## Protocol alignment

Shared prompt identifiers are sourced through `g8e.constants.prompt()` in the Python package, which keeps the Python values aligned with the protocol registry. `AgentMode` currently exposes the bound and not-bound runtime values. `PromptSection` includes the shared safety, loyalty, dissent, capabilities, execution, tools, response-constraint, persona, identity, system-context, triage-context, investigation-context, and learned-context identifiers, plus the protocol-backed `VAULT_MODE` value and the local `SENTINEL_MODE` name for the same `sentinel_mode` value. The protocol registry represents that value through its `SectionVaultMode` entry; it does not imply that a separate cloud prompt loader exists.

Alignment tests cover prompt-file loading and encoding, mode-file availability, section ordering, persona placement, Sage-only reasoning discipline, mode-file boundaries, and the relationship between active tool registrations and prompt descriptions.

## Related

- [Ensemble Architecture](../architecture/ensemble.md) — Runtime boundary, chat flow, tool execution, and governance limits
- [Agents](agents.md) — Persona roster, Tribunal roles, and operational flow
- [Protocol](protocol.md) — g8ee's Gateway-facing protocol integration
- [Governance](governance.md) — Five-layer verification and envelope lifecycle
- [Thinking](thinking.md) — Provider reasoning budgets and thought signatures
- [LLM Providers](llm-providers.md) — Provider interfaces, tool declarations, and generation configuration
- [Documentation Guide](../devs/docs.md) — Documentation audit and ownership rules
