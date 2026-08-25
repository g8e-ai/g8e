# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

"""``check_port_status`` tool - TCP/UDP reachability probe via the operator."""

from __future__ import annotations

import logging
from typing import TYPE_CHECKING

import app.llm.llm_types as types
from app.constants.prompts import PromptFile
from app.constants.generated_status import OperatorToolName
from app.llm.llm_types import schema_from_model
from app.llm.prompts import load_prompt
from app.models.command_request_payloads import CheckPortRequestPayload
from app.models.http_context import G8eHttpContext
from app.models.investigations import EnrichedInvestigationContext
from app.models.settings import G8eeUserSettings
from app.models.tool_args import CheckPortArgs
from app.models.tool_results import ToolResult
from app.services.ai.tools._base import convert_args_to_payload

if TYPE_CHECKING:
    from app.services.ai.tool_service import AIToolService

logger = logging.getLogger(__name__)


def build() -> types.ToolDeclaration:
    return types.ToolDeclaration(
        name=OperatorToolName.CHECK_PORT,
        description=load_prompt(PromptFile.TOOLS_CHECK_PORT),
        parameters=schema_from_model(CheckPortArgs, required_override=["port"]),
    )


async def handle(
    svc: AIToolService,
    tool_args: dict[str, object],
    investigation: EnrichedInvestigationContext,
    g8e_context: G8eHttpContext,
    request_settings: G8eeUserSettings,
    execution_id: str,
) -> ToolResult:
    args = convert_args_to_payload(tool_args, CheckPortRequestPayload, execution_id, investigation)
    logger.info(
        "[CHECK_PORT_STATUS] Host: %s Port: %s Protocol: %s",
        args.host,
        args.port,
        args.protocol,
    )
    result = await svc.operator_command_service.execute_port_check(
        args=args,
        investigation=investigation,
        g8e_context=g8e_context,
    )
    logger.info("[CHECK_PORT_STATUS] Result: %s", result)
    return result
