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

"""``get_command_constraints`` tool — surface active whitelist/blacklist state to the LLM."""

from __future__ import annotations

import logging
from typing import TYPE_CHECKING

import app.llm.llm_types as types
from app.constants.prompts import PromptFile
from app.constants.settings import DEFAULT_OS_NAME
from app.constants.status import OperatorToolName
from app.llm.prompts import load_prompt
from app.models.http_context import G8eHttpContext
from app.models.investigations import EnrichedInvestigationContext
from app.models.settings import G8eeUserSettings
from app.models.tool_results import CommandConstraintsResult, ToolResult
from app.models.whitelist import WhitelistedCommand
from app.services.investigation.investigation_service import (
    extract_single_operator_context,
)
from app.utils.safety import map_os_string_to_platform
from app.utils.csv_commands import parse_command_csv

if TYPE_CHECKING:
    from app.services.ai.tool_service import AIToolService

logger = logging.getLogger(__name__)


def build() -> types.ToolDeclaration:
    declaration = types.ToolDeclaration(
        name=OperatorToolName.GET_COMMAND_CONSTRAINTS,
        description=load_prompt(PromptFile.TOOL_GET_COMMAND_CONSTRAINTS),
        parameters=types.Schema(type=types.Type.OBJECT, properties={}, required=None),
    )
    return declaration


async def handle(
    svc: "AIToolService",
    tool_args: dict[str, object],
    investigation: EnrichedInvestigationContext,
    g8e_context: G8eHttpContext,
    request_settings: G8eeUserSettings,
    execution_id: str,
) -> ToolResult:
    logger.info("[GET_COMMAND_CONSTRAINTS] Retrieving command constraints")

    user_settings = request_settings
    cv = user_settings.command_validation if user_settings else None
    whitelisting_enabled = cv.enable_whitelisting if cv else False
    blacklisting_enabled = cv.enable_blacklisting if cv else False
    auto_approve_enabled = cv.enable_auto_approve if cv else False
    auto_approved_commands: list[str] = []
    auto_approved_sources: list[dict[str, str]] = []
    if cv and auto_approve_enabled:
        # Union of JSON-configured platform defaults and per-user CSV override.
        # Order: JSON entries first (platform-blessed), then any extras from CSV.
        seen: set[str] = set()
        for name in svc.auto_approved_validator.get_auto_approved_command_names():
            if name not in seen:
                seen.add(name)
                auto_approved_commands.append(name)
                auto_approved_sources.append({"command": name, "source": "platform"})
        for name in parse_command_csv(cv.auto_approved_commands):
            if name not in seen:
                seen.add(name)
                auto_approved_commands.append(name)
                auto_approved_sources.append({"command": name, "source": "user"})
            else:
                # Command exists in both JSON and CSV; CSV override takes precedence for source attribution
                for source_entry in auto_approved_sources:
                    if source_entry["command"] == name and source_entry["source"] == "platform":
                        source_entry["source"] = "user"
                        break

    whitelisted_commands: list[WhitelistedCommand] = []
    global_forbidden_patterns: list[str] = []
    global_forbidden_directories: list[str] = []
    if whitelisting_enabled:
        primary_operator = (
            investigation.operator_documents[0]
            if investigation.operator_documents
            else None
        )
        operator_context = (
            extract_single_operator_context(primary_operator)
            if primary_operator
            else None
        )
        os_name = operator_context.os if operator_context else DEFAULT_OS_NAME
        platform = map_os_string_to_platform(os_name)

        csv_override = cv.whitelisted_commands if cv else None
        csv_commands = (
            parse_command_csv(csv_override) if csv_override else []
        )
        if csv_commands:
            whitelisted_commands = [WhitelistedCommand(command=cmd) for cmd in csv_commands]
        else:
            whitelisted_commands = svc.whitelist_validator.get_available_commands_with_metadata(
                platform
            )
        global_forbidden_patterns = list(svc.whitelist_validator.forbidden_patterns)
        global_forbidden_directories = list(
            svc.whitelist_validator.forbidden_directories
        )

    blacklisted_commands: list[dict[str, str]] = []
    blacklisted_substrings: list[dict[str, str]] = []
    blacklisted_patterns: list[dict[str, str]] = []
    if blacklisting_enabled:
        blacklisted_commands = svc.blacklist_validator.get_forbidden_commands()
        blacklisted_substrings = svc.blacklist_validator.get_forbidden_substrings()
        blacklisted_patterns = svc.blacklist_validator.get_forbidden_patterns()

    parts: list[str] = []
    if not whitelisting_enabled and not blacklisting_enabled and not auto_approve_enabled:
        parts.append(
            "No command constraints are currently enforced. All commands require human approval."
        )
    else:
        if whitelisting_enabled:
            parts.append(
                f"Whitelisting ENABLED: only the {len(whitelisted_commands)} listed commands are permitted. "
                "Each command has strict 'safe_options' and 'validation' patterns that MUST be followed. "
                "Any command or argument not explicitly allowed by these rules will be blocked by the technical (L1) validator."
            )
        if blacklisting_enabled:
            parts.append(
                "Blacklisting ENABLED: commands matching blacklisted entries will be blocked."
            )
        if auto_approve_enabled:
            if auto_approved_commands:
                platform_count = sum(1 for s in auto_approved_sources if s["source"] == "platform")
                user_count = sum(1 for s in auto_approved_sources if s["source"] == "user")
                source_breakdown = ""
                if platform_count > 0 and user_count > 0:
                    source_breakdown = f" ({platform_count} platform defaults + {user_count} user-configured)"
                elif platform_count > 0:
                    source_breakdown = f" ({platform_count} platform defaults)"
                elif user_count > 0:
                    source_breakdown = f" ({user_count} user-configured)"
                parts.append(
                    f"Auto-approve ENABLED: the {len(auto_approved_commands)} listed base commands "
                    f"{source_breakdown} ({', '.join(auto_approved_commands)}) skip the human approval prompt — the user has "
                    "rubber-stamped them as benign. All other commands still require human approval. "
                    "Auto-approve does NOT widen the whitelist or bypass the blacklist."
                )
            else:
                parts.append(
                    "Auto-approve ENABLED but auto_approved_commands list is empty: all commands still require human approval."
                )

    result = CommandConstraintsResult(
        success=True,
        whitelisting_enabled=whitelisting_enabled,
        blacklisting_enabled=blacklisting_enabled,
        auto_approve_enabled=auto_approve_enabled,
        whitelisted_commands=whitelisted_commands,
        blacklisted_commands=blacklisted_commands,
        blacklisted_substrings=blacklisted_substrings,
        blacklisted_patterns=blacklisted_patterns,
        auto_approved_commands=auto_approved_commands,
        auto_approved_sources=auto_approved_sources,
        global_forbidden_patterns=global_forbidden_patterns,
        global_forbidden_directories=global_forbidden_directories,
        message=" ".join(parts),
    )
    logger.info(
        "[GET_COMMAND_CONSTRAINTS] whitelisting=%s blacklisting=%s auto_approve=%s "
        "whitelist_count=%d blacklist_count=%d auto_approve_count=%d",
        whitelisting_enabled, blacklisting_enabled, auto_approve_enabled,
        len(whitelisted_commands), len(blacklisted_commands), len(auto_approved_commands),
    )
    return result
