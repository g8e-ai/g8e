# Copyright (c) 2026 Lateralus Labs, LLC.
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

"""g8ee AI Agent Prompts Module"""

import logging
from typing import Any, List, Optional

from ..constants import (
    AgentName,
    CloudSubtype,
    FORBIDDEN_COMMAND_PATTERNS,
    OperatorType,
    PromptFile,
    PromptSection,
)
from ..constants.prompts import InvestigationContextLabel
from ..models.agent import OperatorContext
from ..models.agents import TriageResult
from ..models.investigations import EnrichedInvestigationContext
from ..models.memory import InvestigationMemory
from ..prompts_data.loader import load_mode_prompts, load_prompt
from ..utils.agent_persona_loader import get_agent_persona

logger = logging.getLogger(__name__)


def _format_operator_doc(op_doc, index: int) -> str:
    """Format a single operator document for investigation context."""
    sys_info = op_doc.system_info
    op_id = op_doc.id or f"operator_{index + 1}"
    hostname = op_doc.current_hostname or (sys_info.hostname if sys_info else None)
    
    details = [
        f"hostname={hostname}" if hostname else None,
        f"os={sys_info.os}" if sys_info and sys_info.os else None,
        f"arch={sys_info.architecture}" if sys_info and sys_info.architecture else None,
        f"type={op_doc.operator_type}",
        f"session={op_doc.operator_session_id[:12]}..." if op_doc.operator_session_id else None,
    ]
    
    return f"  [{index + 1}] {op_id}: {', '.join(filter(None, details))}"


def build_investigation_context_section(
    investigation: EnrichedInvestigationContext | None
) -> str:
    """Build investigation context with case details and conversation summary."""
    if not investigation:
        return ""

    fields = [
        ("case_title", InvestigationContextLabel.CASE),
        ("case_description", InvestigationContextLabel.DESCRIPTION),
        ("status", InvestigationContextLabel.STATUS),
        ("priority", InvestigationContextLabel.PRIORITY),
        ("severity", InvestigationContextLabel.SEVERITY),
    ]

    context_parts = [f"{label.value}: {getattr(investigation, field)}" 
                     for field, label in fields 
                     if getattr(investigation, field)]

    if investigation.conversation_history:
        context_parts.append("Conversation history is available via query_investigation_context.")

    if investigation.operator_documents:
        context_parts.append(f"Operators: {len(investigation.operator_documents)} bound")
        context_parts.extend(
            _format_operator_doc(op_doc, i) 
            for i, op_doc in enumerate(investigation.operator_documents)
        )

    if not context_parts:
        return ""

    return "<investigation_context>\n" + "\n".join(context_parts) + "\n</investigation_context>\n"


def build_response_constraints_section() -> str:
    """Load response length constraints that guide AI to self-limit before SDK hard cutoff."""
    return load_prompt(PromptFile.SYSTEM_RESPONSE_CONSTRAINTS)


def build_triage_context_section(triage_result: TriageResult | None) -> str:
    """Build the triage_context block so downstream agents can read user posture.

    Carries the Triage agent's `request_posture` read (normal / escalated /
    adversarial / confused) into the system prompt. The dissent protocol in
    core/dissent.txt consumes this tag to calibrate warnings and denial-memory.
    """
    if not triage_result or not triage_result.request_posture:
        return ""

    posture = triage_result.request_posture.value if hasattr(triage_result.request_posture, "value") else str(triage_result.request_posture)
    intent_summary = (triage_result.intent_summary or "").strip()

    lines = [
        "<triage_context>",
        f"request_posture: {posture}",
    ]
    if intent_summary:
        lines.append(f"intent_summary: {intent_summary}")
    lines.append("</triage_context>")
    return "\n".join(lines) + "\n"


def build_learned_context_section(
    user_memories: list[InvestigationMemory],
    case_memories: list[InvestigationMemory]
) -> str:
    """Build learned context from user preferences and past investigations."""
    context_parts = []

    if user_memories:
        for mem in user_memories:
            prefs = []
            if mem.communication_preferences:
                prefs.append(f"Communication: {mem.communication_preferences}")
            if mem.technical_background:
                prefs.append(f"Technical background: {mem.technical_background}")
            if mem.response_style:
                prefs.append(f"Response style: {mem.response_style}")
            if mem.problem_solving_approach:
                prefs.append(f"Problem-solving: {mem.problem_solving_approach}")
            if mem.interaction_style:
                prefs.append(f"Interaction style: {mem.interaction_style}")
            if prefs:
                context_parts.extend(prefs)

    if case_memories:
        for mem in case_memories:
            if mem.investigation_summary:
                context_parts.append(f"Previous investigation: {mem.investigation_summary}")

    if not context_parts:
        return ""

    return "<learned_context>\n" + "\n".join(f"- {part}" for part in context_parts) + "\n</learned_context>\n"


def build_command_constraints_message(
    whitelisting_enabled: bool,
    blacklisting_enabled: bool,
    whitelisted_commands: List[dict[str, Any]] | None,
    blacklisted_commands: List[dict[str, str]] | None,
) -> str:
    """Generate a human-readable message describing active command constraints.
    
    Args:
        whitelisting_enabled: Whether whitelist enforcement is active
        blacklisting_enabled: Whether blacklist enforcement is active
        whitelisted_commands: List of command metadata dicts with safe_options and validation patterns
        blacklisted_commands: List of blacklisted command dicts
    """
    parts = []
    if whitelisting_enabled:
        if whitelisted_commands:
            command_names = [cmd.get("command", "unknown") for cmd in whitelisted_commands]
            parts.append(f"Whitelisting is ENABLED. Only these commands are allowed: {', '.join(command_names)}")
            
            # Add detailed constraint information for each command
            constraint_details = []
            for cmd in whitelisted_commands:
                cmd_name = cmd.get("command", "unknown")
                safe_options = cmd.get("safe_options", [])
                validation = cmd.get("validation", {})
                
                if safe_options or validation:
                    details = f"{cmd_name}:"
                    if safe_options:
                        details += f" safe_options={safe_options}"
                    if validation:
                        details += f" validation_patterns={list(validation.keys())}"
                    constraint_details.append(details)
            
            if constraint_details:
                parts.append("Command-specific constraints: " + "; ".join(constraint_details))
        else:
            parts.append("Whitelisting is ENABLED, but no commands are whitelisted. ALL commands will be rejected.")
    
    if blacklisting_enabled:
        if blacklisted_commands:
            blacklisted_names = [c.get("command", "unknown") for c in blacklisted_commands]
            parts.append(f"Blacklisting is ENABLED. These commands are FORBIDDEN: {', '.join(blacklisted_names)}")
        else:
            parts.append("Blacklisting is ENABLED, but no commands are blacklisted.")

    if not parts:
        return "No whitelist or blacklist constraints are active."

    return " ".join(parts)


def build_forbidden_patterns_message() -> str:
    """Generate a message listing all forbidden command patterns."""
    patterns = sorted(list(FORBIDDEN_COMMAND_PATTERNS))
    return f"The following patterns are FORBIDDEN and will be rejected: {', '.join(patterns)}"


def build_tribunal_operator_context_string(operator_context: OperatorContext | None) -> str:
    """Build a formatted string of operator context for Tribunal prompts."""
    if not operator_context:
        return "No operator context available"

    parts: list[str] = []
    if operator_context.hostname:
        parts.append(f"Hostname: {operator_context.hostname}")
    if operator_context.os:
        parts.append(f"OS: {operator_context.os}")
    if operator_context.architecture:
        parts.append(f"Architecture: {operator_context.architecture}")
    if operator_context.username:
        uid_suffix = f" (uid={operator_context.uid})" if operator_context.uid is not None else ""
        parts.append(f"User: {operator_context.username}{uid_suffix}")
    if operator_context.shell:
        parts.append(f"Shell: {operator_context.shell}")
    if operator_context.working_directory:
        parts.append(f"Working Directory: {operator_context.working_directory}")
    if operator_context.operator_type:
        parts.append(f"Operator Type: {operator_context.operator_type}")
    if operator_context.is_cloud_operator:
        parts.append("Cloud Operator: Yes")
        if operator_context.cloud_subtype:
            parts.append(f"Cloud Subtype: {operator_context.cloud_subtype}")
        if operator_context.granted_intents:
            parts.append(f"Granted Intents: {operator_context.granted_intents}")
    if operator_context.is_container:
        parts.append("Container Environment: Yes")
        if operator_context.container_runtime:
            parts.append(f"Container Runtime: {operator_context.container_runtime}")
        if operator_context.init_system:
            parts.append(f"Init System: {operator_context.init_system}")
    elif operator_context.init_system:
        parts.append(f"Init System: {operator_context.init_system}")

    return "\n".join(parts) if parts else "No operator details available"


def build_tribunal_prompt_fields(
    operator_context: OperatorContext | None,
    request: str,
    guidelines: str,
    default_os: str,
    default_shell: str,
    default_working_directory: str,
) -> dict[str, str]:
    """Build the common template kwargs used by every Tribunal persona prompt.
    
    Returns a dict with keys: os, shell, working_directory, user_context,
    operator_context, forbidden_patterns_message, request, guidelines.
    """
    os_name = (operator_context.os if operator_context else None) or default_os
    shell = (operator_context.shell if operator_context else None) or default_shell
    working_directory = (
        operator_context.working_directory if operator_context else None
    ) or default_working_directory
    username = operator_context.username if operator_context else None
    uid = operator_context.uid if operator_context else None
    if username and uid is not None:
        user_context = f"{username} (uid={uid})"
    else:
        user_context = username or "unknown"
        
    return {
        "os": os_name,
        "shell": shell,
        "working_directory": working_directory,
        "user_context": user_context,
        "operator_context": build_tribunal_operator_context_string(operator_context),
        "forbidden_patterns_message": build_forbidden_patterns_message(),
        "request": request.strip() if request else "",
        "guidelines": guidelines.strip() if guidelines else "(none)",
    }


def build_tribunal_auditor_context(
    mode: str,
    winner: str | None,
    clusters: List[dict[str, Any]],
) -> str:
    """Build the mode-specific context for the auditor prompt.
    
    Args:
        mode: "unanimous", "majority", or "tied"
        winner: The winning command string
        clusters: List of dicts with keys: cluster_id, command, support_count
    """
    parts = []
    
    if mode == "unanimous":
        parts.append(f"<candidate_command>\n{winner}\n</candidate_command>")
        parts.append("\nUNANIMOUS CONSENSUS: All Tribunal members produced the command above.")
        parts.append("Validate it for syntactic correctness, safety, and alignment with the request.")
        parts.append("\nResponse format:")
        parts.append("- status: \"ok\" (if correct) or \"revised\" (if needs fix)")
        parts.append("- revised_command: the corrected command string (only if status is \"revised\")")
        
    elif mode == "majority":
        parts.append("<candidates_by_cluster>")
        for c in clusters:
            parts.append(f"[{c['cluster_id']}] (support: {c['support_count']})\n{c['command']}")
        parts.append("</candidates_by_cluster>")
        parts.append(f"\nMAJORITY WINNER: [{clusters[0]['cluster_id']}]")
        parts.append("\nObserve the winner and the dissenting clusters above. You may approve the winner, swap to a dissenter, or issue a revision.")
        parts.append("\nResponse format:")
        parts.append("- status: \"ok\" (approve winner), \"swap\" (pick another cluster), or \"revised\"")
        parts.append("- swap_to_cluster: e.g. \"cluster_b\" (only if status is \"swap\")")
        parts.append("- revised_command: corrected string (only if status is \"revised\")")
        
    elif mode == "tied":
        parts.append("<tied_candidates>")
        for c in clusters:
            parts.append(f"[{c['cluster_id']}] (support: {c['support_count']})\n{c['command']}")
        parts.append("</tied_candidates>")
        parts.append("\nVOTING TIED: The tie-break ladder could not resolve a single winner.")
        parts.append("YOU MUST DISAMBIGUATE. Pick the best cluster from the tied set or provide a revision.")
        parts.append("\nResponse format:")
        parts.append("- status: \"swap\" (pick one) or \"revised\"")
        parts.append("- swap_to_cluster: e.g. \"cluster_a\" (required for status \"swap\")")
        parts.append("- revised_command: corrected string (only if status is \"revised\")")
        
    return "\n".join(parts)


def build_tribunal_generator_prompt(
    request: str,
    guidelines: str,
    forbidden_patterns_message: str,
    command_constraints_message: str,
    os: str,
    shell: str,
    user_context: str,
    working_directory: str,
    operator_context_str: str,
) -> str:
    """Build the prompt for a Tribunal generation pass."""
    template = load_prompt(PromptFile.TRIBUNAL_GENERATOR)
    return template.format(
        request=request,
        guidelines=guidelines,
        forbidden_patterns_message=forbidden_patterns_message,
        command_constraints_message=command_constraints_message,
        os=os,
        shell=shell,
        user_context=user_context,
        working_directory=working_directory,
        operator_context=operator_context_str,
    )


def build_tribunal_auditor_prompt(
    request: str,
    guidelines: str,
    forbidden_patterns_message: str,
    command_constraints_message: str,
    os: str,
    user_context: str,
    operator_context_str: str,
    auditor_context: str,
) -> str:
    """Build the prompt for the Tribunal auditor."""
    template = load_prompt(PromptFile.TRIBUNAL_AUDITOR)
    return template.format(
        request=request,
        guidelines=guidelines,
        forbidden_patterns_message=forbidden_patterns_message,
        command_constraints_message=command_constraints_message,
        os=os,
        user_context=user_context,
        operator_context=operator_context_str,
        auditor_context=auditor_context,
    )


def build_modular_system_prompt(
    operator_bound: bool,
    system_context: OperatorContext | list[OperatorContext] | None,
    user_memories: list[InvestigationMemory],
    case_memories: list[InvestigationMemory],
    investigation: EnrichedInvestigationContext | None,
    g8e_web_search_available: bool = True,
    triage_result: TriageResult | None = None,
    agent_name: AgentName | None = None,
) -> str:
    """
    Build system prompt using modular architecture.

    Section order (Gemini 3 best practices - context first, instructions last):
      1. identity   — who the agent is
      2. system_context (injected from operator(s))
      3. sentinel_mode (if active)
      4. triage_context (request_posture from Triage, if available)
      5. investigation_context
      6. learned_context (user + case memories)
      7. safety     — absolute forbidden operations
      8. loyalty    — mission-over-moment doctrine
      9. dissent    — warning protocol, denial memory, escalation response
      10. capabilities / execution / tools (mode-dependent)
      11. response_constraints

    Args:
        operator_bound: Whether Operator is connected for command execution
        system_context: System-level context from Operator(s). Can be a single OperatorContext
                       (backward compatibility), a list of OperatorContext (multi-operator), or None.
        user_memories: User preference memories
        case_memories: Case-specific memories
        investigation: Enriched investigation context model
        triage_result: Triage classification for this turn. When provided, its
                       request_posture is injected as a triage_context tag that
                       the dissent protocol reads at inference time.

    Returns:
        Complete system prompt string
    """
    sections = []

    if agent_name is not None:
        persona = get_agent_persona(agent_name.value).get_system_prompt()
        if persona:
            sections.append(persona)
    else:
        sections.append(load_prompt(PromptFile.CORE_IDENTITY))

    # Context blocks first (Gemini 3 best practices)
    if system_context:
        system_parts = ["<system_context>"]

        # Handle both single OperatorContext and list of OperatorContext
        contexts_to_render = system_context if isinstance(system_context, list) else [system_context]
        
        for idx, ctx in enumerate(contexts_to_render):
            if not ctx:
                continue
                
            # Wrap each operator's context in operator tags for multi-operator scenarios
            if len(contexts_to_render) > 1:
                system_parts.append(f"<operator index=\"{idx}\">")
            
            if ctx.operator_type:
                if ctx.operator_type == OperatorType.CLOUD:
                    if ctx.cloud_subtype == CloudSubtype.G8E_POD:
                        system_parts.append("Operator Type: g8ep Cloud Operator - Direct system access via G8E_POD")
                    elif ctx.cloud_subtype:
                        system_parts.append(f"Operator Type: Cloud Operator for {ctx.cloud_subtype.upper()} - Least-privilege intent-based access")
                    else:
                        logger.warning("[PROMPT] Cloud operator %s has no cloud_subtype set", ctx.operator_id)
                        system_parts.append("Operator Type: Cloud Operator - Least-privilege intent-based access")
                    granted_intents = ctx.granted_intents or []
                    if granted_intents:
                        system_parts.append(f"granted_intents: {granted_intents}")
                    else:
                        system_parts.append("granted_intents: [] (bootstrap only - ask permission before using cloud APIs)")
                else:
                    system_parts.append("Operator Type: Operator - Standard system access")

            if ctx.os:
                system_parts.append(f"OS: {ctx.os}")
            if ctx.hostname:
                system_parts.append(f"Hostname: {ctx.hostname}")
            if ctx.username:
                uid_suffix = f" (uid={ctx.uid})" if ctx.uid is not None else ""
                system_parts.append(f"User: {ctx.username}{uid_suffix}")
            if ctx.working_directory:
                system_parts.append(f"Working Directory: {ctx.working_directory}")

            if ctx.is_container:
                container_runtime = ctx.container_runtime or ""
                init_system = ctx.init_system or ""
                system_parts.append(f"Container Environment: YES (runtime: {container_runtime})")
                system_parts.append(f"Init System (PID 1): {init_system}")
                if init_system != "systemd":
                    system_parts.append("WARNING: systemd is NOT available - do NOT use systemctl, journalctl, or other systemd commands")
            elif ctx.init_system:
                system_parts.append(f"Init System: {ctx.init_system}")

            excluded_keys = {
                "operator_id", "operator_session_id",
                "os", "hostname", "username", "uid", "working_directory",
                "operator_type", "cloud_subtype",
                "is_cloud_operator", "granted_intents",
                "is_container", "container_runtime", "init_system",
            }
            for key in type(ctx).model_fields:
                if key not in excluded_keys:
                    value = getattr(ctx, key)
                    if value:
                        system_parts.append(f"{key}: {value}")
            
            # Close operator tags for multi-operator scenarios
            if len(contexts_to_render) > 1:
                system_parts.append("</operator>")

        system_parts.append("</system_context>")
        system_context_str = "\n".join(system_parts) + "\n"
        sections.append(system_context_str)
        
        # Log the system context being injected
        logger.info(
            "[PROMPT] system_context injected: operator_count=%d context_len=%d username=%s uid=%s hostname=%s os=%s",
            len(contexts_to_render),
            len(system_context_str),
            contexts_to_render[0].username if contexts_to_render and contexts_to_render[0] else None,
            contexts_to_render[0].uid if contexts_to_render and contexts_to_render[0] else None,
            contexts_to_render[0].hostname if contexts_to_render and contexts_to_render[0] else None,
            contexts_to_render[0].os if contexts_to_render and contexts_to_render[0] else None,
        )
        logger.info("[PROMPT] full system_context:\n%s", system_context_str[:5000])

    if investigation and investigation.sentinel_mode is True:
        sections.append(load_prompt(PromptFile.SYSTEM_SENTINEL_MODE))

    triage_section = build_triage_context_section(triage_result)
    if triage_section:
        sections.append(triage_section)

    if investigation:
        investigation_section = build_investigation_context_section(investigation)
        if investigation_section:
            sections.append(investigation_section)

    if user_memories or case_memories:
        learned_section = build_learned_context_section(user_memories, case_memories)
        if learned_section:
            sections.append(learned_section)

    # Safety and execution instructions at the end (Gemini 3 best practices)
    sections.append(load_prompt(PromptFile.CORE_SAFETY))
    sections.append(load_prompt(PromptFile.CORE_LOYALTY))
    sections.append(load_prompt(PromptFile.CORE_DISSENT))

    # Determine if any operator is a cloud operator for mode selection
    is_cloud_operator = False
    if system_context:
        if isinstance(system_context, list):
            is_cloud_operator = any(ctx.is_cloud_operator for ctx in system_context if ctx)
        else:
            is_cloud_operator = system_context.is_cloud_operator
    
    mode_prompts = load_mode_prompts(
        operator_bound,
        is_cloud_operator=is_cloud_operator,
        g8e_web_search_available=g8e_web_search_available,
    )

    if mode_prompts.get(PromptSection.CAPABILITIES):
        sections.append(mode_prompts[PromptSection.CAPABILITIES])
    if mode_prompts.get(PromptSection.EXECUTION):
        sections.append(mode_prompts[PromptSection.EXECUTION])
    include_tools_section = bool(mode_prompts.get(PromptSection.TOOLS)) and (
        operator_bound or g8e_web_search_available
    )
    logger.info(
        "[PROMPT] tools_section: include=%s operator_bound=%s g8e_web_search_available=%s",
        include_tools_section, operator_bound, g8e_web_search_available
    )
    if include_tools_section:
        sections.append(mode_prompts[PromptSection.TOOLS])

    response_constraints = build_response_constraints_section()
    if response_constraints:
        sections.append(response_constraints)

    full_prompt = "\n".join(sections)

    section_labels = [
        PromptSection.IDENTITY,
        PromptSection.SAFETY,
        PromptSection.LOYALTY,
        PromptSection.DISSENT,
    ]
    if mode_prompts.get(PromptSection.CAPABILITIES):
        section_labels.append(PromptSection.CAPABILITIES)
    if mode_prompts.get(PromptSection.EXECUTION):
        section_labels.append(PromptSection.EXECUTION)
    if include_tools_section:
        section_labels.append(PromptSection.TOOLS)
    if system_context:
        section_labels.append(PromptSection.SYSTEM_CONTEXT)
    if investigation and investigation.sentinel_mode is True:
        section_labels.append(PromptSection.SENTINEL_MODE)
    if triage_section:
        section_labels.append(PromptSection.TRIAGE_CONTEXT)
    if investigation:
        section_labels.append(PromptSection.INVESTIGATION_CONTEXT)
    section_labels.append(PromptSection.RESPONSE_CONSTRAINTS)
    if user_memories or case_memories:
        section_labels.append(PromptSection.LEARNED_CONTEXT)

    logger.info(
        "[PROMPT] sections=%d total_chars=%d operator_bound=%s sections=[%s]",
        len(sections), len(full_prompt), operator_bound, ", ".join(section_labels)
    )
    for label, section in zip(section_labels, sections):
        logger.info("[PROMPT] section=%-24s chars=%d", label, len(section))
    logger.info("[PROMPT] full_prompt:\n%s", full_prompt)

    return full_prompt
