# Agent Persona System

## Overview

The g8e platform uses multiple AI agents across the stack, each with a specific role and purpose. This document describes the centralized persona system for defining and managing agent identities, prompts, and metadata.

## Agent Registry

All agent definitions are centralized in `shared/constants/agents.json`. This file contains:

- **Metadata**: id, display_name, icon, description, role, model_tier, temperature, tools
- **Identity**: Who the agent is
- **Purpose**: What the agent does
- **Autonomy**: How much independence the agent has (fully_autonomous, human_approved)
- **Persona**: The full prompt template for the agent (TODO placeholders to be filled in)

## Current Agents

### 1. Triage
- **Role**: Classifier
- **Model Tier**: Primary
- **Purpose**: Routes user messages based on complexity and intent
- **Migration Status**: Complete
- **Usage**: `get_agent_persona("triage")` in `triage.py`
- **Prompt Source**: `shared/constants/agents.json` (`triage`)

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
- **Usage**: `get_agent_persona("tribunal")` in `command_generator.py` (base template)
- **Prompt Source**: `shared/constants/agents.json` (`tribunal`)

### 5. Verifier
- **Role**: Validator
- **Model Tier**: Assistant
- **Temperature**: 0.0 (deterministic)
- **Purpose**: Deterministic command verification
- **Migration Status**: Complete
- **Usage**: `get_agent_persona("verifier")` in `command_generator.py`

### 6. Title Generator
- **Role**: Summarizer
- **Model Tier**: Assistant
- **Temperature**: 0.7
- **Purpose**: Generates concise case titles
- **Migration Status**: Complete
- **Usage**: `get_agent_persona("title_generator")` in `title_generator.py`

### 7. Axiom (Tribunal Member)
- **Role**: Tribunal Member
- **Model Tier**: Assistant
- **Temperature**: 0.0 (deterministic)
- **Purpose**: The Optimizer - statistical probability and resource efficiency
- **Pass Assignment**: Pass 0 in voting swarm
- **Migration Status**: Complete
- **Usage**: `get_tribunal_member("axiom")` in `command_generator.py`

### 8. Concord (Tribunal Member)
- **Role**: Tribunal Member
- **Model Tier**: Assistant
- **Temperature**: 0.4
- **Purpose**: The Ethicist - harm minimization and ethical integrity
- **Pass Assignment**: Pass 1 in voting swarm
- **Migration Status**: Complete
- **Usage**: `get_tribunal_member("concord")` in `command_generator.py`

### 9. Variance (Tribunal Member)
- **Role**: Tribunal Member
- **Model Tier**: Assistant
- **Temperature**: 0.8
- **Purpose**: The Catalyst - edge case hunting through adversarial simulation
- **Pass Assignment**: Pass 2 in voting swarm
- **Migration Status**: Complete
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

## Example Persona Structure

A complete persona should include:

```json
{
  "persona": "<role>\nYou are the [agent name], a [role] that [purpose].\n</role>\n\n<identity>\n[Detailed identity description]\n</identity>\n\n<guidelines>\n- Guideline 1\n- Guideline 2\n</guidelines>\n\n<constraints>\n- Constraint 1\n- Constraint 2\n</constraints>"
}
```

## Current Prompt File Structure

For agents using the modular system (Primary/Assistant), prompts are split across multiple files in `components/g8ee/app/prompts_data/`:

- `core/identity.txt` - Role, platform, operating principles
- `core/safety.txt` - Architecture, execution posture, forbidden operations
- `modes/operator_bound/` - Capabilities, execution, tools (when operator is bound)
- `modes/operator_not_bound/` - Capabilities, execution, tools (when operator is not bound)
- `modes/cloud_operator_bound/` - Cloud-specific variants
- `system/` - Response constraints, sentinel mode
- `tools/` - Individual tool descriptions

**Note**: The `analysis/` directory (command_risk.txt, error_analysis.txt, file_risk.txt) was previously used by Response Analyzer but has been migrated to the persona system as sub-agents. These files can be removed once migration is confirmed stable.

## Remaining Migration Work

The following agents still need migration to the centralized persona system:

1. **Primary AI** - Uses modular prompt system in `prompts_data/`
2. **Assistant AI** - Uses modular prompt system in `prompts_data/`
