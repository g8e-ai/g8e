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
- **Injection Point**: `build_modular_system_prompt(agent_name=AgentName.SAGE, ...)` prepends `get_agent_persona("sage").get_system_prompt()` immediately after `core/identity.txt`.
- **Routing**: `ChatPipelineService` selects Sage when `triage_result.complexity == COMPLEX` (i.e. the main model tier was chosen).
- **Migration Status**: Complete

### 3. Dash (Assistant AI)
- **Icon**: `zap`
- **Role**: Responder
- **Model Tier**: Assistant
- **Purpose**: Fast-path AI for simple tasks. Same tool set as Sage but no `<agentic_reasoning>` block — Dash's value is latency and minimum viable work.
- **Prompt Source**: Persona voice in `shared/constants/agents.json` (`dash`) + modular doctrine / mode / context sections in `components/g8ee/app/prompts_data/`.
- **Injection Point**: `build_modular_system_prompt(agent_name=AgentName.DASH, ...)` prepends `get_agent_persona("dash").get_system_prompt()` immediately after `core/identity.txt`.
- **Routing**: `ChatPipelineService` selects Dash when `triage_result.complexity == SIMPLE` (i.e. the assistant model tier was chosen).
- **Handoff Note**: Dash does **not** hand off to Sage mid-turn — no runtime mechanism exists for that. When a turn outgrows Dash, its persona directs it to name the mismatch, answer only what a single tool call can answer correctly, and stop; the next user turn is re-triaged and typically re-routes to Sage.
- **Migration Status**: Complete

### 4. Tribunal
- **Icon**: `users`
- **Role**: Arbitrator
- **Model Tier**: Assistant
- **Purpose**: Syntactic refinement and validation for shell commands through a five-member panel
- **Migration Status**: Complete
- **Usage**: Documentation record only. Runtime loads `axiom`, `concord`, `variance`, `pragma`, and `nemesis` directly via `get_tribunal_member(...)` and `verifier` via `get_agent_persona("auditor")`. The base `tribunal` entry describes the shared output contract and is not injected into any prompt.
- **Prompt Source**: `shared/constants/agents.json` (`tribunal`)

**Tribunal Members**:
- **Axiom** (icon: `minimize`) — The Minimalist. Pass 0. Proposes the smallest command that satisfies intent.
- **Concord** (icon: `shield`) — The Guardian. Pass 1. Proposes the safest command that satisfies intent.
- **Variance** (icon: `layers`) — The Exhaustive. Pass 2. Proposes a command that handles edge cases.
- **Pragma** (icon: `code`) — The Conventional. Pass 3. Proposes the command the target system's community would produce.
- **Nemesis** (icon: `warning`) — The Adversary. Pass 4. Proposes a plausible-but-subtly-wrong command to test for attack surfaces.

### 5. Auditor (Verifier)
- **Icon**: `gavel`
- **Role**: Validator / Final Judgment
- **Model Tier**: Assistant
- **Purpose**: The last voice before the command reaches user approval. Produces one of two verdicts in a strict wire contract: the literal string `ok` when the Tribunal winner is correct as-is, or a single corrected command string when a specific nameable flaw requires revision. Defaults to confirming; revises only on concrete errors, never on general unease.
- **Migration Status**: Complete (voice sharpened; `ok`/corrected-command wire contract preserved for `_run_verifier` in `command_generator.py`)
- **Usage**: `get_agent_persona("auditor")` in `command_generator.py`

### 6. Scribe (Title Generator)
- **Icon**: `type`
- **Role**: Summarizer
- **Model Tier**: Assistant
- **Purpose**: Generates concise case titles
- **Migration Status**: Complete
- **Usage**: `get_agent_persona("scribe")` in `title_generator.py`

### 7. Codex (Memory Generator)
- **Icon**: `brain`
- **Role**: Analyzer
- **Model Tier**: Assistant
- **Purpose**: Analyzes conversation history to extract user preferences and investigation summaries
- **Migration Status**: Complete
- **Usage**: `get_agent_persona("codex")` in `memory_generation_service.py`
- **Prompt Source**: `shared/constants/agents.json` (`codex`)

### 8. Judge (Eval Judge)
- **Icon**: `gavel`
- **Role**: Evaluator
- **Model Tier**: Primary
- **Purpose**: Evaluates AI agent performance against gold standard criteria
- **Migration Status**: Complete
- **Usage**: `get_agent_persona("judge")` in `eval_judge.py`
- **Prompt Source**: `shared/constants/agents.json` (`judge`)

### 9. Warden (Response Analyzer)
- **Icon**: `shield`
- **Role**: Defender
- **Model Tier**: Assistant
- **Purpose**: Defensive analysis of AI responses (command risk, error analysis, file operation risk)
- **Migration Status**: Complete - Uses sub-agent pattern
- **Usage**: `get_agent_persona("warden_command_risk")`, etc. in `response_analyzer.py`
- **Prompt Source**: `shared/constants/agents.json` (`warden`, `warden_command_risk`, `warden_error`, `warden_file_risk`)

## Persona Loader Utility

The utility `components/g8ee/app/utils/agent_persona_loader.py` provides:

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
`<operator_context>`) lives in the shared `TRIBUNAL_PROMPT_TEMPLATE` and is
rendered around the persona voice per pass:

```python
from app.services.ai.command_generator import TRIBUNAL_PROMPT_TEMPLATE, _prompt_fields
from app.utils.agent_persona_loader import get_tribunal_member

member = _member_for_pass(pass_index)
member_persona = get_tribunal_member(member.value)
fields = _prompt_fields(operator_context, request=request, guidelines=guidelines)

prompt = TRIBUNAL_PROMPT_TEMPLATE.format(
    voice=member_persona.get_system_prompt(),
    command_constraints_message=command_constraints_message,
    **fields,
)
```

### Verifier (command_generator.py)

The Auditor (Verifier) persona is also pure voice. `TRIBUNAL_VERIFIER_TEMPLATE`
adds `<candidate_command>` in place of the member-specific closer and shares
the same context fields:

```python
from app.services.ai.command_generator import TRIBUNAL_VERIFIER_TEMPLATE, _prompt_fields
from app.utils.agent_persona_loader import get_agent_persona

verifier_persona = get_agent_persona("auditor")
fields = _prompt_fields(operator_context, request=request, guidelines=guidelines)

prompt = TRIBUNAL_VERIFIER_TEMPLATE.format(
    voice=verifier_persona.get_system_prompt(),
    command_constraints_message=command_constraints_message,
    candidate_command=candidate_command,
    **fields,
)
```

### Memory Generation (memory_generation_service.py)
```python
from app.utils.agent_persona_loader import get_agent_persona

memory_persona = get_agent_persona("codex")
system_instructions = f"You are analyzing a technical support conversation for case: {memory.case_title}. {memory_persona.get_system_prompt()}"
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

## Remaining Migration Work

Stage 2 completed migration of Sage and Dash to the centralized persona
system. Remaining code smells tracked for follow-up:

1. **Warden sub-agents** (`warden_command_risk`, `warden_error`,
   `warden_file_risk`) still embed `str.format` placeholders
   (`{command}`, `{exit_code}`, `{stdout}`, etc.) directly in the persona
   text. Each has a single caller today, so the blast radius is low, but
   the doctrine that scaffolding belongs in the consumer template — not
   the persona — applies here as well. Track via
   `_PERSONA_PLACEHOLDER_ALLOWLIST` in
   `tests/unit/test_prompt_alignment.py`.
2. **Codex** and **Judge** output-schema instructions are baked into the
   persona with the same placeholder pattern. Same recommendation: move
   the schema scaffolding into a template on the consumer side.
