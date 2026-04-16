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

from enum import Enum

class AgentMode(str, Enum):
    OPERATOR_BOUND = "g8e.bound"
    OPERATOR_NOT_BOUND = "g8e.not.bound"
    CLOUD_OPERATOR_BOUND = "cloud.g8e.bound"

class PromptSection(str, Enum):
    IDENTITY = "identity"
    SAFETY = "safety"
    LOYALTY = "loyalty"
    DISSENT = "dissent"
    CAPABILITIES = "capabilities"
    EXECUTION = "execution"
    TOOLS = "tools"
    DOCS = "docs"
    SYSTEM_CONTEXT = "system_context"
    SENTINEL_MODE = "sentinel_mode"
    TRIAGE_CONTEXT = "triage_context"
    INVESTIGATION_CONTEXT = "investigation_context"
    RESPONSE_CONSTRAINTS = "response_constraints"
    LEARNED_CONTEXT = "learned_context"


class InvestigationContextLabel(str, Enum):
    CASE = "Case"
    DESCRIPTION = "Description"
    STATUS = "Status"
    PRIORITY = "Priority"
    SEVERITY = "Severity"

class PromptFile(str, Enum):
    # Core
    CORE_IDENTITY = "core/identity.txt"
    CORE_SAFETY = "core/safety.txt"
    CORE_LOYALTY = "core/loyalty.txt"
    CORE_DISSENT = "core/dissent.txt"
    
    # System
    SYSTEM_RESPONSE_CONSTRAINTS = "system/response_constraints.txt"
    SYSTEM_SENTINEL_MODE = "system/sentinel_mode.txt"
    
    # Modes - Operator Bound
    MODE_OPERATOR_BOUND_CAPABILITIES = "modes/operator_bound/capabilities.txt"
    MODE_OPERATOR_BOUND_EXECUTION = "modes/operator_bound/execution.txt"
    MODE_OPERATOR_BOUND_TOOLS = "modes/operator_bound/tools.txt"
    
    # Modes - Operator Not Bound
    MODE_OPERATOR_NOT_BOUND_CAPABILITIES = "modes/operator_not_bound/capabilities.txt"
    MODE_OPERATOR_NOT_BOUND_EXECUTION = "modes/operator_not_bound/execution.txt"
    MODE_OPERATOR_NOT_BOUND_TOOLS = "modes/operator_not_bound/tools.txt"
    MODE_OPERATOR_NOT_BOUND_CAPABILITIES_NO_SEARCH = "modes/operator_not_bound/capabilities_no_search.txt"
    MODE_OPERATOR_NOT_BOUND_EXECUTION_NO_SEARCH = "modes/operator_not_bound/execution_no_search.txt"
    
    # Modes - Cloud Operator Bound
    MODE_CLOUD_OPERATOR_BOUND_CAPABILITIES = "modes/cloud_operator_bound/capabilities.txt"
    MODE_CLOUD_OPERATOR_BOUND_EXECUTION = "modes/cloud_operator_bound/execution.txt"
    MODE_CLOUD_OPERATOR_BOUND_TOOLS = "modes/cloud_operator_bound/tools.txt"
    
    # Tools
    TOOL_RUN_COMMANDS = "tools/run_commands_with_operator.txt"
    TOOL_FILE_CREATE = "tools/file_create_on_operator.txt"
    TOOL_FILE_WRITE = "tools/file_write_on_operator.txt"
    TOOL_FILE_READ = "tools/file_read_on_operator.txt"
    TOOL_FILE_UPDATE = "tools/file_update_on_operator.txt"
    TOOL_SEARCH_WEB = "tools/g8e_web_search.txt"
    TOOL_CHECK_PORT = "tools/check_port_status.txt"
    TOOL_LIST_FILES = "tools/list_files_and_directories_with_detailed_metadata.txt"
    TOOL_GRANT_INTENT = "tools/grant_intent_permission.txt"
    TOOL_REVOKE_INTENT = "tools/revoke_intent_permission.txt"
    TOOL_FETCH_EXECUTION_OUTPUT = "tools/fetch_execution_output.txt"
    TOOL_FETCH_SESSION_HISTORY = "tools/fetch_session_history.txt"
    TOOL_FETCH_FILE_HISTORY = "tools/fetch_file_history.txt"
    TOOL_RESTORE_FILE = "tools/restore_file.txt"
    TOOL_FETCH_FILE_DIFF = "tools/fetch_file_diff.txt"
    TOOL_READ_FILE_CONTENT = "tools/read_file_content.txt"
    TOOL_QUERY_INVESTIGATION_CONTEXT = "tools/query_investigation_context.txt"
    TOOL_GET_COMMAND_CONSTRAINTS = "tools/get_command_constraints.txt"

    # Analysis
    ANALYSIS_COMMAND_RISK = "analysis/command_risk.txt"
    ANALYSIS_ERROR_SUGGESTION = "analysis/error_analysis.txt"
    ANALYSIS_FILE_RISK = "analysis/file_risk.txt"

    @property
    def path(self) -> str:
        """Get the relative path for the prompt file."""
        return self.value

AGENT_MODE_PROMPT_FILES = {
    AgentMode.OPERATOR_BOUND: {
        PromptSection.CAPABILITIES: PromptFile.MODE_OPERATOR_BOUND_CAPABILITIES,
        PromptSection.EXECUTION: PromptFile.MODE_OPERATOR_BOUND_EXECUTION,
        PromptSection.TOOLS: PromptFile.MODE_OPERATOR_BOUND_TOOLS,
    },
    AgentMode.OPERATOR_NOT_BOUND: {
        PromptSection.CAPABILITIES: PromptFile.MODE_OPERATOR_NOT_BOUND_CAPABILITIES,
        PromptSection.EXECUTION: PromptFile.MODE_OPERATOR_NOT_BOUND_EXECUTION,
        PromptSection.TOOLS: PromptFile.MODE_OPERATOR_NOT_BOUND_TOOLS,
    },
    AgentMode.CLOUD_OPERATOR_BOUND: {
        PromptSection.CAPABILITIES: PromptFile.MODE_CLOUD_OPERATOR_BOUND_CAPABILITIES,
        PromptSection.EXECUTION: PromptFile.MODE_CLOUD_OPERATOR_BOUND_EXECUTION,
        PromptSection.TOOLS: PromptFile.MODE_CLOUD_OPERATOR_BOUND_TOOLS,
    },
}
