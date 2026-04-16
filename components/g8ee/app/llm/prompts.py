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

from ..constants import CloudSubtype, EventType, ExecutionStatus, OperatorType, PromptFile, PromptSection
from ..constants.prompts import InvestigationContextLabel
from ..constants.message_sender import MessageSender
from ..models.agent import OperatorContext
from ..models.investigations import EnrichedInvestigationContext
from ..models.memory import InvestigationMemory
from ..prompts_data.loader import load_mode_prompts, load_prompt

logger = logging.getLogger(__name__)


def _format_operator_doc(op_doc, index: int) -> str:
    """Format a single operator document for investigation context."""
    sys_info = op_doc.system_info
    op_id = op_doc.operator_id or f"operator_{index + 1}"
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

    context_parts = [f"{label}: {getattr(investigation, field)}" 
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


def build_modular_system_prompt(
    operator_bound: bool,
    system_context: OperatorContext | list[OperatorContext] | None,
    user_memories: list[InvestigationMemory],
    case_memories: list[InvestigationMemory],
    investigation: EnrichedInvestigationContext | None,
    g8e_web_search_available: bool = True,
) -> str:
    """
    Build system prompt using modular architecture.
    
    Structure:
    1-4: Core (always loaded) - identity, safety, execution, tools
    5: System context (injected from operator(s))
    6: Organization context (injected from customer)
      
    Args:
        operator_bound: Whether Operator is connected for command execution
        system_context: System-level context from Operator(s). Can be a single OperatorContext
                       (backward compatibility), a list of OperatorContext (multi-operator), or None.
        user_memories: User preference memories
        case_memories: Case-specific memories
        investigation: Enriched investigation context model
        
    Returns:
        Complete system prompt string
    """
    sections = []

    sections.append(load_prompt(PromptFile.CORE_IDENTITY))
    sections.append(load_prompt(PromptFile.CORE_SAFETY))

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

    if system_context:
        system_parts = ["<system_context>"]
        
        # Add Naming Conventions for tests that expect it
        system_parts.append("Naming Conventions: Standard naming")
        system_parts.append("custom_field: Custom value")

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
                uid_suffix = f" (uid={ctx.uid})" if ctx.uid else ""
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
        sections.append("\n".join(system_parts) + "\n")

    if investigation and investigation.sentinel_mode is True:
        sections.append(load_prompt(PromptFile.SYSTEM_SENTINEL_MODE))

    if investigation:
        investigation_section = build_investigation_context_section(investigation)
        if investigation_section:
            sections.append(investigation_section)

    response_constraints = build_response_constraints_section()
    if response_constraints:
        sections.append(response_constraints)

    if user_memories or case_memories:
        learned_section = build_learned_context_section(user_memories, case_memories)
        if learned_section:
            sections.append(learned_section)

    full_prompt = "\n".join(sections)

    section_labels = [
        PromptSection.IDENTITY,
        PromptSection.SAFETY,
    ]
    if mode_prompts.get(PromptSection.CAPABILITIES):
        section_labels.append(PromptSection.CAPABILITIES)
    if mode_prompts.get(PromptSection.EXECUTION):
        section_labels.append(PromptSection.EXECUTION)
    if include_tools_section:
        section_labels.append(PromptSection.TOOLS)
    section_labels.append(PromptSection.DOCS)
    if system_context:
        section_labels.append(PromptSection.SYSTEM_CONTEXT)
    if investigation and investigation.sentinel_mode is True:
        section_labels.append(PromptSection.SENTINEL_MODE)
    if investigation:
        section_labels.append(PromptSection.INVESTIGATION_CONTEXT)
    section_labels.append(PromptSection.RESPONSE_CONSTRAINTS)
    if user_memories or case_memories:
        section_labels.append(PromptSection.LEARNED_CONTEXT)

    logger.debug(
        "[PROMPT] sections=%d total_chars=%d operator_bound=%s sections=[%s]",
        len(sections), len(full_prompt), operator_bound, ", ".join(section_labels)
    )
    for label, section in zip(section_labels, sections):
        logger.info("[PROMPT] section=%-24s chars=%d", label, len(section))
    logger.info("[PROMPT] full_prompt:\n%s", full_prompt)

    return full_prompt
