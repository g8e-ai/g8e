# Agent Persona System

## Overview

The g8e platform uses multiple AI agents across the stack, each with a specific role and purpose. This document describes the centralized persona system for defining and managing agent identities, prompts, and metadata.

## Agent Registry

All agent definitions are centralized in `shared/constants/agents.json`. This file contains:

- **Metadata**: id, display_name, icon, description, role, model_tier, temperature (persona-level override; when `null`, runtime resolves via the precedence below), tools
- **Identity**: Who the agent is
- **Purpose**: What the agent does
- **Autonomy**: How much independence the agent has (fully_autonomous, human_approved)
- **Persona**: The full prompt template for the agent (TODO placeholders to be filled in)

### Persona-driven temperature precedence

Runtime temperature resolution for Tribunal and Verifier calls (see `_resolve_temperature` in `components/g8ee/app/services/ai/command_generator.py`):

1. **Persona `temperature`** when explicitly set in `agents.json`. Reserved for the rare case a persona has a concrete reason to pin a specific value; all current Tribunal personas leave it `null`.
2. **Model `default_temperature`** from `components/g8ee/app/models/model_configs.py`. For the Gemini 3 family this is `1.0`, per Google's own guidance that changing it can cause looping or degraded performance.
3. **`LLM_DEFAULT_TEMPERATURE`** as a last-resort global default when neither of the above is set.

The Tribunal's behavioral diversity (Axiom / Concord / Variance / Verifier) is carried by **the prompt language of each persona**, not by per-pass numeric temperature skew. The earlier `_temperature_for_pass` function has been removed; there is no per-pass variation.

## Current Agents

### 1. Triage
- **Role**: Classifier / Gatekeeper
- **Model Tier**: Primary
- **Purpose**: Routes user messages by **complexity**, **intent**, and **request_posture** (`normal` / `escalated` / `adversarial` / `confused`). Posture is the loyal-friction signal the responding agent uses to calibrate dissent and denial-memory behavior.
- **Migration Status**: Complete
- **Usage**: `get_agent_persona("triage")` in `triage.py`
- **Prompt Source**: `shared/constants/agents.json` (`triage`)
- **Note**: `adversarial` requires prior-turn context (a first-turn message cannot be adversarial). See `core/dissent.txt` for how downstream agents consume the posture tag.

### 2. Primary AI
- **Role**: Reasoner
- **Model Tier**: Primary
- **Purpose**: Main reasoning AI for complex tasks
- **Prompt Source**: Modular system in `components/g8ee/app/prompts_data/`
- **Migration Status**: TODO - Uses modular prompt system, not yet centralized

### 3. Assistant AI
- **Role**: Responder
- **Model Tier**: Assistant
- **Purpose**: Fast-path AI for simple tasks
- **Prompt Source**: Modular system in `components/g8ee/app/prompts_data/`
- **Migration Status**: TODO - Uses modular prompt system, not yet centralized

### 4. Tribunal
- **Role**: Arbitrator
- **Model Tier**: Assistant
- **Purpose**: Syntactic refinement and validation for shell commands
- **Migration Status**: Complete
- **Usage**: Documentation record only. Runtime loads `axiom`, `concord`, and `variance` directly via `get_tribunal_member(...)` and `verifier` via `get_agent_persona("verifier")`. The base `tribunal` entry describes the shared output contract and is not injected into any prompt.
- **Prompt Source**: `shared/constants/agents.json` (`tribunal`)

### 5. Verifier
- **Role**: Validator / Final Judgment
- **Model Tier**: Assistant
- **Temperature**: `null` — uses the configured model's `default_temperature`. Convergent behavior is enforced by the persona voice and the strict `ok`/corrected-command wire contract, not by a hardcoded temperature (some providers, e.g. Gemini 3+, reject non-default temperatures).
- **Purpose**: The last voice before the command reaches user approval. Produces one of two verdicts in a strict wire contract: the literal string `ok` when the Tribunal winner is correct as-is, or a single corrected command string when a specific nameable flaw requires revision. Defaults to confirming; revises only on concrete errors, never on general unease.
- **Migration Status**: Complete (voice sharpened; `ok`/corrected-command wire contract preserved for `_run_verifier` in `command_generator.py`)
- **Usage**: `get_agent_persona("verifier")` in `command_generator.py`

### 6. Title Generator
- **Role**: Summarizer
- **Model Tier**: Assistant
- **Temperature**: `null` — uses the configured model's `default_temperature`.
- **Purpose**: Generates concise case titles
- **Migration Status**: Complete
- **Usage**: `get_agent_persona("title_generator")` in `title_generator.py`

### 7. Axiom (Tribunal Member)
- **Role**: Tribunal Member — The Minimalist
- **Model Tier**: Assistant
- **Temperature**: `null` — uses the model default. Minimalist character comes from the persona, not a numeric temperature.
- **Purpose**: Proposes the simplest command that correctly accomplishes the intent. Complexity is a liability; every extra flag or pipe is a surface for bugs or misreading. Does not add defensive flags — that is Concord's and Variance's job.
- **Pass Assignment**: Pass 0 in voting swarm
- **Migration Status**: Complete (voice sharpened; tribunal `.format()` contract preserved)
- **Usage**: `get_tribunal_member("axiom")` in `command_generator.py`

### 8. Concord (Tribunal Member)
- **Role**: Tribunal Member — The Archivist
- **Model Tier**: Assistant
- **Temperature**: `null` — uses the model default. Archivist character comes from the persona.
- **Purpose**: Proposes the command that reads cleanest in an audit log six months later. Optimizes for institutional memory and retrospective legibility: explicit flags, absolute paths, named resources over wildcards, optional trailing comments. Distinct from Variance — Concord assumes the reader is careless, not that the writer is compromised.
- **Pass Assignment**: Pass 1 in voting swarm
- **Migration Status**: Complete (voice sharpened; tribunal `.format()` contract preserved)
- **Usage**: `get_tribunal_member("concord")` in `command_generator.py`

### 9. Variance (Tribunal Member)
- **Role**: Tribunal Member — The Adversary
- **Model Tier**: Assistant
- **Temperature**: `null` — uses the model default. Adversarial character comes from the persona.
- **Purpose**: Proposes the command as if an attacker had authored it and the user had not noticed. Simulates adversarial origin per-command: compromised terminal, pasted snippet, social engineering. Defensive additions must do real work — no security theater. Couples directly to Triage's `adversarial` `request_posture` signal.
- **Pass Assignment**: Pass 2 in voting swarm
- **Migration Status**: Complete (voice sharpened; tribunal `.format()` contract preserved)
- **Usage**: `get_tribunal_member("variance")` in `command_generator.py`

### 10. Memory Generator
- **Role**: Analyzer
- **Model Tier**: Assistant
- **Purpose**: Analyzes conversation history to extract user preferences and investigation summaries
- **Migration Status**: Complete
- **Usage**: `get_agent_persona("memory_generator")` in `memory_generation_service.py`
- **Prompt Source**: `shared/constants/agents.json` (`memory_generator`)

### 11. Eval Judge
- **Role**: Evaluator
- **Model Tier**: Primary
- **Purpose**: Evaluates AI agent performance against gold standard criteria
- **Migration Status**: Complete
- **Usage**: `get_agent_persona("eval_judge")` in `eval_judge.py`
- **Prompt Source**: `shared/constants/agents.json` (`eval_judge`)

### 12. Response Analyzer
- **Role**: Defender
- **Model Tier**: Assistant
- **Purpose**: Defensive analysis of AI responses (command risk, error analysis, file operation risk)
- **Migration Status**: Complete - Uses sub-agent pattern
- **Usage**: `get_agent_persona("response_analyzer_command_risk")`, etc. in `response_analyzer.py`
- **Prompt Source**: `shared/constants/agents.json` (`response_analyzer`, `response_analyzer_command_risk`, `response_analyzer_error`, `response_analyzer_file_risk`)

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
print(persona.temperature)   # None
print(persona.tools)         # ["run_commands_with_operator", ...]

# For Tribunal members specifically
member_persona = get_tribunal_member("axiom")
```

The loader uses Pydantic validation to ensure all agent entries are well-formed at load time.

## Sub-Agent Pattern

Some agents delegate to specialized sub-agents for different analysis types. The Response Analyzer is the current example:

**Parent Agent**: `response_analyzer`
- Delegates to three sub-agents for different analysis types
- Metadata only, no active persona

**Sub-Agents**:
- `response_analyzer_command_risk` - Analyzes shell command risk levels (LOW/MEDIUM/HIGH)
- `response_analyzer_error` - Analyzes command failures and determines auto-fixability
- `response_analyzer_file_risk` - Analyzes file operation safety and risk levels

**Usage in Service**:
```python
from app.utils.agent_persona_loader import get_agent_persona

# Load specific sub-agent for command risk analysis in response_analyzer.py
command_risk_persona = get_agent_persona("response_analyzer_command_risk")
prompt = command_risk_persona.persona.format(
    command=command,
    justification=justification,
    working_dir=working_dir
)
```

## Concrete Service Examples

### Tribunal Generation Passes (command_generator.py)
```python
from app.utils.agent_persona_loader import get_tribunal_member

# Each Tribunal member has a unique persona
member = _member_for_pass(pass_index)
member_persona = get_tribunal_member(member.value)
prompt = member_persona.persona.format(
    forbidden_patterns_message=_format_forbidden_patterns_message(),
    command_constraints_message=command_constraints_message,
    intent=intent,
    os=os_name,
    shell=shell,
    user_context=user_context,
    working_directory=working_directory,
    original_command=original_command,
)
```

### Verifier (command_generator.py)
```python
from app.utils.agent_persona_loader import get_agent_persona

verifier_persona = get_agent_persona("verifier")
prompt = verifier_persona.get_system_prompt().format(
    forbidden_patterns_message=_format_forbidden_patterns_message(),
    command_constraints_message=command_constraints_message,
    intent=intent,
    os=os_name,
    user_context=user_context,
    candidate_command=candidate_command,
)
```

### Memory Generation (memory_generation_service.py)
```python
from app.utils.agent_persona_loader import get_agent_persona

memory_persona = get_agent_persona("memory_generator")
system_instructions = f"You are analyzing a technical support conversation for case: {memory.case_title}. {memory_persona.get_system_prompt()}"
```

## Canonical persona layout

All Tribunal / Verifier personas follow the same canonical layout. The
convention is **XML-style tags for hard structural boundaries, Markdown for
content inside each section**. This matches the Gemini 3 prompt-engineering
guidance (see `docs/reference/gemini/prompt_engineering.md`): XML tags keep
template-substituted user data (`{intent}`, `{original_command}`, ...) from
bleeding into instructions, while Markdown inside each section stays
human-readable in source.

Ordering rules (Gemini 3 best practice, applied uniformly):

- Critical behavioral instructions first. `<role>` and `<output_contract>` come
  at the top so the model weights them heavily.
- Context last. `{intent}`, `{system_context}`, `{original_command}`,
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

<intent>
{intent}
</intent>

<system_context>
OS: {os}
Shell: {shell}
User: {user_context}
Working directory: {working_directory}
</system_context>

<original_command>
{original_command}
</original_command>

Respond now with [exactly the required output format, one sentence].
```

The Verifier uses the same pattern with `<candidate_command>` in place of
`<original_command>` and no `<system_context>` shell/working-directory fields.

## Current Prompt File Structure

For agents using the modular system (Primary/Assistant), prompts are split across multiple files in `components/g8ee/app/prompts_data/`:

- `core/identity.txt` - Role, platform, operating principles
- `core/safety.txt` - Architecture, execution posture, forbidden operations
- `core/loyalty.txt` - Loyal-friction doctrine: kingdom over king, frustration is data, memory of refusal, dissent is visible
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
4. **The Tribunal's three sharpened voices** — Axiom (Minimalist), Concord (Archivist), Variance (Adversary) produce three ideologically distinct candidate commands. Variance specifically couples to `adversarial` posture. The Verifier converges on one final command with a terse `ok`-or-corrected-command verdict.

The design goal is **loyal friction**: the agent visibly cares about the user's outcome and, because it cares, refuses to let the user hurt themselves — while still executing what the user commands within their authority, with the warning logged alongside the execution.

**Note**: The `analysis/` directory (command_risk.txt, error_analysis.txt, file_risk.txt) was previously used by Response Analyzer but has been migrated to the persona system as sub-agents. These files can be removed once migration is confirmed stable.

## Remaining Migration Work

The following agents still need migration to the centralized persona system:

1. **Primary AI** - Uses modular prompt system in `prompts_data/`
2. **Assistant AI** - Uses modular prompt system in `prompts_data/`
