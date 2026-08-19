# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

"""``revoke_intent_permission`` tool - revoke a previously granted AWS intent."""

from __future__ import annotations

import logging
from typing import TYPE_CHECKING

import app.llm.llm_types as types
from app.constants.prompts import PromptFile
from app.constants.generated_status import OperatorToolName
from app.llm.llm_types import schema_from_model
from app.llm.prompts import load_prompt
from app.models.http_context import G8eHttpContext
from app.models.investigations import EnrichedInvestigationContext
from app.models.settings import G8eeUserSettings
from app.models.tool_args import RevokeIntentArgs
from app.models.tool_results import ToolResult

if TYPE_CHECKING:
    from app.services.ai.tool_service import AIToolService

logger = logging.getLogger(__name__)


def build() -> types.ToolDeclaration:
    return types.ToolDeclaration(
        name=OperatorToolName.REVOKE_INTENT,
        description=load_prompt(PromptFile.TOOLS_REVOKE_INTENT),
        parameters=schema_from_model(RevokeIntentArgs),
    )


async def handle(
    svc: AIToolService,
    tool_args: dict[str, object],
    investigation: EnrichedInvestigationContext,
    g8e_context: G8eHttpContext,
    request_settings: G8eeUserSettings,
    execution_id: str,
) -> ToolResult:
    args = RevokeIntentArgs.model_validate(tool_args)
    logger.info("[REVOKE_INTENT] Intent: %s", args.intent_name)
    result = await svc.operator_command_service.execute_intent_revocation(
        args=args,
        g8e_context=g8e_context,
        investigation=investigation,
    )
    logger.info("[REVOKE_INTENT] success=%s", result.success)
    return result
