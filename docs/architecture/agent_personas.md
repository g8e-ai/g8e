---
title: Agent Personas
parent: Architecture
---

# Agent Persona System

## Overview

The g8e platform uses multiple AI agents across the stack, each with a specific role and purpose. This document describes the centralized persona system for defining and managing agent identities, prompts, and metadata.

## Agent Registry

All agent definitions are centralized in `shared/constants/agents.json`. This file contains:

- **Metadata**: id, display_name, icon, description, role, model_tier, tools
- **Identity**: Who the agent is
- **Purpose**: What the agent does
- **Autonomy**: Empowering directive prose affirming the agent's maximum agency within its role — blended from its identity, purpose, and persona. Free-form string, not an enum.
- **Persona**: The full prompt template for the agent (TODO placeholders to be filled in)

## Current Agents

### 1. Triage
- **Icon**: `scan-eye`
- **Role**: Classifier / Gatekeeper
- **Model Tier**: Primary
- **Purpose**: Routes user messages by **complexity**, **intent**, and **request_posture** (`normal` / `escalated` / `adversarial` / `confused`). Posture is the loyal-friction signal the responding agent uses to calibrate dissent and denial-memory behavior.
- **Migration Status**: Complete
- **Usage**: `get_agent_persona("triage")` in `triage.py`
- **Prompt Source**: `shared/constants/agents.json` (`triage`)
- **Note**: `adversarial` requires prior-turn context (a first-turn message cannot be adversarial). See `core/dissent.txt` for how downstream agents consume the posture tag.

### 2. Sage (Primary AI)
- **Icon**: `cpu`
- **Role**: Reasoner
- **Model Tier**: Primary
- **Purpose**: Main reasoning AI for complex tasks. Carries the `<agentic_reasoning>` discipline block that was previously inlined in `modes/operator_bound/capabilities.txt`.
- **Prompt Source**: Persona voice in `shared/constants/agents.json` (`sage`) + modular doctrine / mode / context sections in `components/g8ee/app/prompts_data/`.
- **Injection Point**: `build_modular_system_prompt(agent_name=AgentName.SAGE, ...)` uses the persona's system prompt (which includes role, identity, purpose, autonomy) in place of `core/identity.txt`. When no agent_name is provided, CORE_IDENTITY is used as fallback.
- **Routing**: `ChatPipelineService` selects Sage when `triage_result.complexity == COMPLEX` (i.e. the main model tier was chosen).
- **Migration Status**: Complete

### 3. Dash (Assistant AI)
- **Icon**: `zap`
- **Role**: Responder
- **Model Tier**: Assistant
- **Purpose**: Fast-path AI for simple tasks. Carries a surgical tool-bearing posture — Dash has access to the full tool surface (13 tools, same as Sage) but operates under a "one well-aimed call beats a chain" discipline. Dash's value is latency and minimum viable work; when a turn requires multi-step reasoning or chaining, its persona directs it to name the mismatch, answer only what a single tool call can answer correctly, and stop. The next user turn is re-triaged and typically re-routes to Sage.
- **Prompt Source**: Persona voice in `shared/constants/agents.json` (`dash`) + modular doctrine / mode / context sections in `components/g8ee/app/prompts_data/`. Dash uses a slim builder variant that omits `<agentic_reasoning>` and other heavyweight sections to minimize prefill cost.
- **Injection Point**: `build_modular_system_prompt(agent_name=AgentName.DASH, ...)` uses the persona's system prompt (which includes role, identity, purpose, autonomy) in place of `core/identity.txt`. The Dash path skips heavy sections (learned context, full tool descriptions) to reduce prompt size.
- **Routing**: `ChatPipelineService` selects Dash when `triage_result.complexity == SIMPLE` (i.e. the assistant model tier was chosen).
- **Handoff Note**: Dash does **not** hand off to Sage mid-turn — no runtime mechanism exists for that. When a turn outgrows Dash, its persona directs it to name the mismatch, answer only what a single tool call can answer correctly, and stop; the next user turn is re-triaged and typically re-routes to Sage.
- **Migration Status**: Complete

### 4. Tribunal
- **Icon**: `users`
- **Role**: Arbitrator
- **Model Tier**: Assistant
- **Purpose**: Syntactic refinement and validation for shell commands through a five-member panel.
- **Migration Status**: Complete
- **Usage**: Documentation record only. Runtime loads `axiom`, `concord`, `variance`, `pragma`, and `nemesis` directly via `get_tribunal_member(...)` and `verifier` via `get_agent_persona("auditor")`.
- **Prompt Source**: `shared/constants/agents.json` (`tribunal`)

**Tribunal Members**:
- **Axiom** (icon: `git-merge`) — The Composer. Translates intent into the most coherent composed command that fulfills the full intent in one invocation. Pressure against fragmentation.
- **Concord** (icon: `shield-check`) — The Guardian. Translates intent into the safest command that does the job. Pressure against regret; defensive discipline.
- **Variance** (icon: `git-branch`) — The Exhaustive. Handles edge cases the obvious version misses (filenames with spaces, symlinks, etc.). Pressure against fragility.
- **Pragma** (icon: `book-open`) — The Conventional. Uses idiomatic tools, flags, and patterns for the target system's community. Pressure against novelty.
- **Nemesis** (icon: `shield-alert`) — The Adversary. Always present; produces a plausible-but-flawed command, or honestly abstains when no attack surface exists. immune system of the platform.

### 5. Auditor (Verifier)
- **Icon**: `search-check`
- **Role**: Validator / Final Judgment
- **Model Tier**: Assistant
- **Purpose**: The final checkpoint before the command reaches the human. Operates in three modes (Unanimous, Majority, Tied) with anonymized cluster IDs. Produces `ok`, `revised:<command>`, or `swap:<cluster_id>`. Scrutinizes compositions stage-by-stage.
- **Migration Status**: Complete
- **Usage**: `get_agent_persona("auditor")` in `command_generator.py`

### 6. Scribe (Title Generator)
- **Icon**: `type`
- **Role**: Summarizer
- **Model Tier**: Assistant
- **Purpose**: Generates concise (3-7 words) specific case titles from the opening turn. Ruthlessly compressive.
- **Migration Status**: Complete
- **Usage**: `get_agent_persona("scribe")` in `title_generator.py`

### 7. Codex (Memory Generator)
- **Icon**: `brain`
- **Role**: Analyzer
- **Model Tier**: Assistant
- **Purpose**: Extracts durable user preferences and scrubbed investigation summaries. Redacts identifiers and guards against overfitting to single turns.
- **Migration Status**: Complete
- **Usage**: `get_agent_persona("codex")` in `memory_generation_service.py`

### 8. Judge (Eval Judge)
- **Icon**: `gavel`
- **Role**: Evaluator
- **Model Tier**: Primary
- **Purpose**: Grades agent performance against gold-standard rubrics. Distinguishes between system failures and low scores.
- **Migration Status**: Complete
- **Usage**: `get_agent_persona("judge")` in `eval_judge.py`

### 9. Warden (Defensive Coordinator)
- **Icon**: `shield`
- **Role**: Defender
- **Model Tier**: Assistant
- **Purpose**: Coordinates defensive analysis across three specialist sub-agents. Fails closed (HIGH risk) on inconclusive analysis.
- **Migration Status**: Complete - Uses specialist sub-agent pattern
- **Usage**: `get_agent_persona("warden")` in `response_analyzer.py`

**Warden Specialists**:
- `warden_command_risk` (icon: `shield-alert`) - Classifies shell command risk (LOW/MEDIUM/HIGH) based on blast radius and reversibility.
- `warden_error` (icon: `alert-triangle`) - Analyzes failures to determine if they are `AUTO_FIXABLE`, `ESCALATE`, or `RETRY_LIMIT`.
- `warden_file_risk` (icon: `file-shield`) - Classifies file operation risk, factoring in git working-tree state and backup status.

## Persona Loader Utility

The utility `components/g8ee/app/utils/agent_persona_loader.py` provides:

- **`@lru_cache(maxsize=1)` decorator** on `_load_agents_json()` — the `agents.json` file is cached at process startup for the lifetime of the g8ee service. This ensures persona definitions are immutable during process lifetime and provides O(1) lookup performance. A `clear_agents_json_cache()` function exists for testing purposes only.

**Important**: Tooling that edits `agents.json` and expects to see changes without a process restart will silently get stale data due to the cache. The file is process-lifetime-immutable by design.

```python
from app.utils.agent_persona_loader import get_agent_persona, get_tribunal_member

# Retrieve an agent's persona
persona = get_agent_persona("triage")

# Get the system prompt (falls back to identity/purpose if persona is TODO)
system_prompt = persona.get_system_prompt()

# Access metadata
print(persona.display_name)  # "Triage"
print(persona.role)          # "classifier"
print(persona.tools)         # [] — Triage is a classifier and calls no tools

# For personas that do bind tools, e.g. the Sage (Primary AI):
sage = get_agent_persona("sage")
print(sage.tools)         # ["run_commands_with_operator", "file_create_on_operator", ...]

# For Tribunal members specifically
member_persona = get_tribunal_member("axiom")
```

The loader uses Pydantic validation to ensure all agent entries are well-formed at load time.

## Sub-Agent Pattern

Some agents delegate to specialized sub-agents for different analysis types. The Response Analyzer is the current example:

**Parent Agent**: `warden`
- Delegates to three sub-agents for different analysis types
- Metadata only, no active persona

**Sub-Agents**:
- `warden_command_risk` (icon: `shield-alert`) - Analyzes shell command risk levels (LOW/MEDIUM/HIGH)
- `warden_error` (icon: `alert-triangle`) - Analyzes command failures and determines auto-fixability
- `warden_file_risk` (icon: `file-shield`) - Analyzes file operation safety and risk levels

**Usage in Service**:
```python
from app.utils.agent_persona_loader import get_agent_persona

# Load specific sub-agent for command risk analysis in response_analyzer.py
command_risk_persona = get_agent_persona("warden_command_risk")
prompt = command_risk_persona.persona.format(
    command=command,
    justification=justification,
    working_dir=working_dir
)
```

## Concrete Service Examples

### Tribunal Generation Passes (command_generator.py)

Each Tribunal member's persona is **pure voice** (`<role>`, `<output_contract>`,
`<principles>`, `<method>`) with no embedded `str.format` placeholders. The
scaffolding (`<constraints>`, `<request>`, `<guidelines>`, `<system_context>`,
`<operator_context>`) lives in the shared `TRIBUNAL_GENERATOR` prompt file.
The Tribunal Generator passes the voice as the System Instructions via the LLM API,
and passes the rendered scaffolding as the User Prompt:

```python
from app.llm.prompts import build_tribunal_generator_prompt
from app.utils.agent_persona_loader import get_agent_persona

member = _member_for_pass(pass_index)
member_persona = get_agent_persona(member.value)
fields = build_tribunal_prompt_fields(operator_context, request=request, guidelines=guidelines)

prompt = build_tribunal_generator_prompt(
    request=request,
    guidelines=guidelines,
    forbidden_patterns_message=fields["forbidden_patterns_message"],
    command_constraints_message=command_constraints_message,
    **fields,
)

# Call LLM with member_persona.get_system_prompt() as system instructions
```

### Verifier (command_generator.py)

The Auditor (Verifier) persona is also pure voice. `TRIBUNAL_AUDITOR` prompt
adds `<candidate_command>` in place of the member-specific closer and shares
the same context fields:

```python
from app.llm.prompts import build_tribunal_auditor_prompt
from app.utils.agent_persona_loader import get_agent_persona

verifier_persona = get_agent_persona("auditor")
fields = build_tribunal_prompt_fields(operator_context, request=request, guidelines=guidelines)

prompt = build_tribunal_auditor_prompt(
    request=request,
    guidelines=guidelines,
    forbidden_patterns_message=fields["forbidden_patterns_message"],
    command_constraints_message=command_constraints_message,
    candidate_command=candidate_command,
    **fields,
)

# Call LLM with verifier_persona.get_system_prompt() as system instructions
```

### Memory Generation (memory_generation_service.py)
```python
from app.utils.agent_persona_loader import get_agent_persona

memory_persona = get_agent_persona("codex")
# Pass memory_persona.get_system_prompt() as system instructions
```

## Canonical persona layout

All Tribunal / Verifier personas follow the same canonical layout. The
convention is **XML-style tags for hard structural boundaries, Markdown for
content inside each section**. This matches the Gemini 3 prompt-engineering
guidance (see `docs/reference/gemini/prompt_engineering.md`): XML tags keep
template-substituted user data (`{request}`, `{guidelines}`, ...) from
bleeding into instructions, while Markdown inside each section stays
human-readable in source.

Ordering rules (Gemini 3 best practice, applied uniformly):

- Critical behavioral instructions first. `<role>` and `<output_contract>` come
  at the top so the model weights them heavily.
- Context last. `{request}`, `{guidelines}`, `{system_context}`,
  `{candidate_command}` sit at the bottom of the prompt, followed by a short
  transition line that re-anchors the model on the output contract.
- Platform-injected constraints are grouped in a single `<constraints>`
  section so substitution cannot splice into surrounding prose.
- **No embedded shell command examples.** Inline backticks around shell
  snippets (e.g. `` `ls -la` ``, `` `curl | bash` ``) and `<example>` blocks
  showing commands are forbidden. They teach the model to emit markdown or
  prose around commands, which then mis-parses downstream. State behaviors
  instead (e.g. "prefer staged download-inspect-execute over piping remote
  content into a shell").

### Prefix Cache Optimization (Tribunal Templates)

Tribunal generator and auditor templates are ordered **static-prefix-first** to maximize llama.cpp `--cache-reuse` effectiveness. Per-session-stable sections (`<constraints>`, `<system_context>`, `<operator_context>`) precede per-turn dynamic sections (`<guidelines>`, `<request>`, `{auditor_context}`). This ordering ensures that when llama.cpp receives a second request in the same session, the KV cache can reuse the static prefix tokens, reducing prefill cost.

**Generator template** (`prompts_data/tribunal/generator.txt`):
1. `<constraints>` — forbidden patterns, command constraints (session-stable)
2. `<system_context>` — OS, shell, user, working directory (session-stable)
3. `<operator_context>` — bound operator metadata (session-stable)
4. `<guidelines>` — per-turn operator-provided guidance (dynamic)
5. `<request>` — the natural-language intent (dynamic)

**Auditor template** (`prompts_data/tribunal/auditor.txt`):
1. `<constraints>` — forbidden patterns, command constraints (session-stable)
2. `<system_context>` — OS, shell, user (session-stable)
3. `<operator_context>` — bound operator metadata (session-stable)
4. `<guidelines>` — per-turn operator-provided guidance (dynamic)
5. `<request>` — the natural-language intent (dynamic)
6. `{auditor_context}` — candidate command and cluster IDs (dynamic)

**Required llama.cpp flags** for this optimization to be effective:
- `--cache-reuse 256` — enables KV cache reuse up to 256 tokens
- `--keep -1` — keeps the KV cache between requests (default is to clear)
- `--parallel <n_slots ≥ 6>` — Tribunal emits 5 parallel members + 1 auditor in a round; without sufficient parallel slots, cache reuse is defeated by sequential processing

See `docs/components/g8el.md` for full configuration details.

```text
<role>
[one or two sentences: who, what one thing, for whom]
</role>

<output_contract>
[strict wire format — the single most important section for machine-parsed agents]
</output_contract>

<principles>
- **Principle 1** — short clause.
- **Principle 2** — short clause.
</principles>

<constraints>
{forbidden_patterns_message}

{command_constraints_message}
</constraints>

<request>
{request}
</request>

<guidelines>
{guidelines}
</guidelines>

<system_context>
OS: {os}
Shell: {shell}
User: {user_context}
Working directory: {working_directory}
</system_context>

<operator_context>
{operator_context}
</operator_context>

Respond now with [exactly the required output format, one sentence].
```

The Verifier uses the same pattern with `<candidate_command>` in place of
`<operator_context>` and shares the same context fields.

## output_contract Field

The `output_contract` field in persona definitions is the **canonical and only** location for specifying the expected output format. Previously, a regex fallback existed to extract `<output_contract>` tags embedded in the `identity` field. This fallback has been removed — all personas must use the explicit `output_contract` field in `agents.json`.

**Contract test**: `TestOutputContractIsExplicitField` in `tests/unit/test_prompt_alignment.py` enforces this invariant by asserting that no persona embeds the tag in its `identity`. If a violation is introduced, the test fires immediately and points to migrating to the explicit field.

## Model Tier Values and Runtime Alignment

The `model_tier` field in `agents.json` is constrained to three valid values:
- `primary` — high-reasoning model (Sage, Judge, Auditor)
- `assistant` — assistant model (Dash, Tribunal members, Scribe, Codex, Warden)
- `lite` — fast/lightweight model (Triage)

**Contract test**: `test_model_tier_is_valid_value` in `tests/unit/utils/test_shared_agent_constants.py` enforces this by asserting every agent's `model_tier` is one of the three valid values. This catches typos like `"liter"`.

**Runtime-tier alignment**: The declared tier in `agents.json` must match the tier the production path actually requests. A contract test `test_model_tier_matches_runtime_routing` pins per-agent declared tier to the tier its production path requests:
- Triage: declared `lite` → runtime uses `lite`
- Sage: declared `primary` → runtime uses `primary`
- Dash: declared `assistant` → runtime uses `assistant`
- Tribunal members: declared `lite` → runtime uses `assistant` (generator hardcode)
- Auditor: declared `lite` → runtime uses `assistant` (generator hardcode)
- Judge: declared `primary` → runtime uses `primary`
- Scribe/Codex/Warden: declared `assistant` → runtime uses `assistant`

This test catches drift in either direction — if `agents.json` is edited without updating the runtime hardcodes, or vice versa.

## Current Prompt File Structure

For agents using the modular system (Primary/Assistant), prompts are split across multiple files in `components/g8ee/app/prompts_data/`:

- `core/identity.txt` - Role, platform, operating principles
- `core/safety.txt` - Architecture, execution posture, forbidden operations
- `core/loyalty.txt` - Loyal-friction doctrine: mission over moment, frustration is data, memory of refusal, dissent is visible
- `core/dissent.txt` - Warning protocol, denial memory, posture-based escalation response, the right to disagree
- `modes/operator_bound/` - Capabilities, execution, tools (when operator is bound)
- `modes/operator_not_bound/` - Capabilities, execution, tools (when operator is not bound)
- `modes/cloud_operator_bound/` - Cloud-specific variants
- `system/` - Response constraints, sentinel mode
- `tools/` - Individual tool descriptions

## Loyal-Friction Doctrine

The platform's stance against RLHF-induced sycophancy lives in four pieces that cooperate at inference time:

1. **`core/loyalty.txt`** — Principles. The agent's loyalty is to the user's long-term outcome, not their immediate instruction. Frustration is data, not authorization to skip safety work. Denied requests are remembered across turns. Dissent is visible, not hedged.
2. **`core/dissent.txt`** — Protocol. How to emit a single-sentence `<warning>` before a tool call; how to handle denial memory ("earlier we declined X; this is related because Y — decide fresh"); how to adjust behavior when `request_posture` is `escalated` / `adversarial` / `confused`; how to shape explicit disagreement ("I don't think this will get you what you're after").
3. **Triage's `request_posture`** — Upstream read of the user's state (`normal` / `escalated` / `adversarial` / `confused`). Injected as `<triage_context>` into the Primary/Assistant system prompt. `adversarial` requires prior-turn context; first-turn messages cannot be adversarial.
4. **The Tribunal's five sharpened voices** — Axiom (Minimalist), Concord (Guardian), Variance (Exhaustive), Pragma (Conventional), Nemesis (Adversary) produce five ideologically distinct candidate commands. Nemesis specifically couples to `adversarial` posture. The Verifier converges on one final command with a terse `ok`-or-corrected-command verdict.

The design goal is **loyal friction**: the agent visibly cares about the user's outcome and, because it cares, refuses to let the user hurt themselves — while still executing what the user commands within their authority, with the warning logged alongside the execution.

**Note**: The `analysis/` directory (command_risk.txt, error_analysis.txt, file_risk.txt) was previously used by Response Analyzer but has been migrated to the persona system as sub-agents. These files can be removed once migration is confirmed stable.
