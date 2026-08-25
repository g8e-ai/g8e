# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

from enum import StrEnum

from g8e.constants import prompt as _g8e_prompt


class InvestigationContextLabel(StrEnum):
    """Investigation context field labels for UI formatting (g8ee-specific)."""

    CASE = "Case"
    DESCRIPTION = "Description"
    STATUS = "Status"
    PRIORITY = "Priority"
    SEVERITY = "Severity"


class AgentMode(StrEnum):
    """Agent execution modes.

    Values sourced from g8e.constants.PROMPTS to stay in sync with the protocol.
    """

    G8E_BOUND = _g8e_prompt("AgentModeG8eBound")
    CLOUD_OPERATOR_BOUND = _g8e_prompt("AgentModeCloudOperatorBound")
    G8E_NOT_BOUND = _g8e_prompt("AgentModeG8eNotBound")


class PromptSection(StrEnum):
    """Prompt section identifiers.

    Shared section values are sourced from g8e.constants.PROMPTS. g8ee-specific
    sections not in the protocol (SENTINEL_MODE) are defined locally.
    """

    CAPABILITIES = _g8e_prompt("SectionCapabilities")
    EXECUTION = _g8e_prompt("SectionExecution")
    TOOLS = _g8e_prompt("SectionTools")
    RESPONSE_CONSTRAINTS = _g8e_prompt("SectionResponseConstraints")
    AGENT_PERSONA = _g8e_prompt("SectionAgentPersona")
    IDENTITY = _g8e_prompt("SectionIdentity")
    SYSTEM_CONTEXT = _g8e_prompt("SectionSystemContext")
    TRIAGE_CONTEXT = _g8e_prompt("SectionTriageContext")
    INVESTIGATION_CONTEXT = _g8e_prompt("SectionInvestigationContext")
    LEARNED_CONTEXT = _g8e_prompt("SectionLearnedContext")
    SAFETY = _g8e_prompt("SectionSafety")
    LOYALTY = _g8e_prompt("SectionLoyalty")
    DISSENT = _g8e_prompt("SectionDissent")
    VAULT_MODE = _g8e_prompt("SectionVaultMode")

    # g8ee-specific sections not in the g8e protocol
    SENTINEL_MODE = "sentinel_mode"


class PromptFile(StrEnum):
    """Prompt file identifiers with paths."""

    CORE_SAFETY = "core/safety.txt"
    CORE_LOYALTY = "core/loyalty.txt"
    CORE_DISSENT = "core/dissent.txt"
    CORE_IDENTITY = "core/identity.txt"
    SYSTEM_RESPONSE_CONSTRAINTS = "system/response_constraints.txt"
    SYSTEM_SENTINEL_MODE = "system/sentinel_mode.txt"

    TRIBUNAL_GENERATOR = "tribunal/generator.txt"
    TRIBUNAL_GENERATOR_ROUND_2 = "tribunal/generator_round_2.txt"
    TRIBUNAL_AUDITOR = "tribunal/auditor.txt"
    TRIBUNAL_ROUND_2_AXIOM = "tribunal/round_2/axiom.txt"
    TRIBUNAL_ROUND_2_CONCORD = "tribunal/round_2/concord.txt"
    TRIBUNAL_ROUND_2_VARIANCE = "tribunal/round_2/variance.txt"
    TRIBUNAL_ROUND_2_PRAGMA = "tribunal/round_2/pragma.txt"
    TRIBUNAL_ROUND_2_NEMESIS = "tribunal/round_2/nemesis.txt"

    MODES_OPERATOR_BOUND_CAPABILITIES = "modes/operator_bound/capabilities.txt"
    MODES_OPERATOR_BOUND_EXECUTION = "modes/operator_bound/execution.txt"
    MODES_CLOUD_OPERATOR_BOUND_CAPABILITIES = "modes/cloud_operator_bound/capabilities.txt"
    MODES_CLOUD_OPERATOR_BOUND_EXECUTION = "modes/cloud_operator_bound/execution.txt"
    MODES_OPERATOR_NOT_BOUND_CAPABILITIES = "modes/operator_not_bound/capabilities.txt"
    MODES_OPERATOR_NOT_BOUND_EXECUTION = "modes/operator_not_bound/execution.txt"
    MODES_OPERATOR_NOT_BOUND_CAPABILITIES_NO_SEARCH = (
        "modes/operator_not_bound/capabilities_no_search.txt"
    )
    MODES_OPERATOR_NOT_BOUND_EXECUTION_NO_SEARCH = (
        "modes/operator_not_bound/execution_no_search.txt"
    )
    MODES_OPERATOR_BOUND_TOOLS = "modes/operator_bound/tools.txt"
    MODES_CLOUD_OPERATOR_BOUND_TOOLS = "modes/cloud_operator_bound/tools.txt"
    MODES_OPERATOR_NOT_BOUND_TOOLS = "modes/operator_not_bound/tools.txt"

    TOOLS_FILE_READ = "tools/file_read_on_operator.txt"
    TOOLS_FILE_UPDATE = "tools/file_update_on_operator.txt"
    TOOLS_REVOKE_INTENT = "tools/revoke_intent_permission.txt"
    TOOLS_STREAM_OPERATOR = "tools/stream_operator_to_ssh_fleet.txt"
    TOOLS_SSH_INVENTORY = "tools/list_ssh_inventory.txt"
    TOOLS_GET_COMMAND_CONSTRAINTS = "tools/get_command_constraints.txt"
    TOOLS_GRANT_INTENT = "tools/grant_intent_permission.txt"
    TOOLS_QUERY_INVESTIGATION_CONTEXT = "tools/query_investigation_context.txt"
    TOOLS_G8E_WEB_SEARCH = "tools/g8e_web_search.txt"
    TOOLS_CHECK_PORT = "tools/check_port_status.txt"
    TOOLS_FETCH_FILE_DIFF = "tools/fetch_file_diff.txt"
    TOOLS_LIST_FILES = "tools/list_files_and_directories_with_detailed_metadata.txt"
    TOOLS_FILE_WRITE = "tools/file_write_on_operator.txt"
    TOOLS_FETCH_FILE_HISTORY = "tools/fetch_file_history.txt"
    TOOLS_FILE_CREATE = "tools/file_create_on_operator.txt"
    TOOLS_RUN_COMMANDS = "tools/run_commands_with_operator.txt"
    TOOLS_RECURSIVE_GREP = "tools/recursive_grep_search.txt"
    TOOLS_READ_FILE_CONTENT = "tools/read_file_content.txt"

    @property
    def path(self) -> str:
        """Return the file path for this prompt."""
        return self.value


# Agent mode prompt file mappings
AGENT_MODE_PROMPT_FILES = {
    AgentMode.G8E_BOUND: {
        PromptSection.CAPABILITIES: PromptFile.MODES_OPERATOR_BOUND_CAPABILITIES,
        PromptSection.EXECUTION: PromptFile.MODES_OPERATOR_BOUND_EXECUTION,
        PromptSection.TOOLS: PromptFile.MODES_OPERATOR_BOUND_TOOLS,
    },
    AgentMode.CLOUD_OPERATOR_BOUND: {
        PromptSection.CAPABILITIES: PromptFile.MODES_CLOUD_OPERATOR_BOUND_CAPABILITIES,
        PromptSection.EXECUTION: PromptFile.MODES_CLOUD_OPERATOR_BOUND_EXECUTION,
        PromptSection.TOOLS: PromptFile.MODES_CLOUD_OPERATOR_BOUND_TOOLS,
    },
    AgentMode.G8E_NOT_BOUND: {
        PromptSection.CAPABILITIES: PromptFile.MODES_OPERATOR_NOT_BOUND_CAPABILITIES,
        PromptSection.EXECUTION: PromptFile.MODES_OPERATOR_NOT_BOUND_EXECUTION,
        PromptSection.TOOLS: PromptFile.MODES_OPERATOR_NOT_BOUND_TOOLS,
    },
}


__all__ = [
    "AGENT_MODE_PROMPT_FILES",
    "AgentMode",
    "InvestigationContextLabel",
    "PromptFile",
    "PromptSection",
]
