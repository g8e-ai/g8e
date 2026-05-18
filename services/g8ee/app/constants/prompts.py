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

from enum import StrEnum

# Prompt enums sourced from protocol/constants/prompts.json (single source of truth)
from g8e_protocol.constants import AgentMode, PromptFile, PromptSection


class InvestigationContextLabel(StrEnum):
    """Investigation context field labels for UI formatting (g8ee-specific)."""
    CASE = "Case"
    DESCRIPTION = "Description"
    STATUS = "Status"
    PRIORITY = "Priority"
    SEVERITY = "Severity"


# Add path property to PromptFile for backward compatibility
# This is a g8ee-specific convenience property
PromptFile.path = property(lambda self: self.value)


# AGENT_MODE_PROMPT_FILES mapping - derived from protocol JSON structure
# Maps AgentMode values to PromptSection -> PromptFile mappings
AGENT_MODE_PROMPT_FILES = {
    AgentMode.OPERATOR_BOUND: {
        PromptSection.CAPABILITIES: PromptFile.MODES_OPERATOR_BOUND_CAPABILITIES,
        PromptSection.EXECUTION: PromptFile.MODES_OPERATOR_BOUND_EXECUTION,
        PromptSection.TOOLS: PromptFile.MODES_OPERATOR_BOUND_TOOLS,
    },
    AgentMode.OPERATOR_NOT_BOUND: {
        PromptSection.CAPABILITIES: PromptFile.MODES_OPERATOR_NOT_BOUND_CAPABILITIES,
        PromptSection.EXECUTION: PromptFile.MODES_OPERATOR_NOT_BOUND_EXECUTION,
        PromptSection.TOOLS: PromptFile.MODES_OPERATOR_NOT_BOUND_TOOLS,
    },
    AgentMode.CLOUD_OPERATOR_BOUND: {
        PromptSection.CAPABILITIES: PromptFile.MODES_CLOUD_OPERATOR_BOUND_CAPABILITIES,
        PromptSection.EXECUTION: PromptFile.MODES_CLOUD_OPERATOR_BOUND_EXECUTION,
        PromptSection.TOOLS: PromptFile.MODES_CLOUD_OPERATOR_BOUND_TOOLS,
    },
}
