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

"""``query_investigation_context`` tool — read-only inspection of investigation state."""

from __future__ import annotations

import logging
from typing import TYPE_CHECKING, Any

import app.llm.llm_types as types
from app.constants.prompts import PromptFile
from app.constants.status import CommandErrorType, OperatorToolName
from app.llm.llm_types import schema_from_model
from app.llm.prompts import load_prompt
from app.models.http_context import G8eHttpContext
from app.models.investigations import EnrichedInvestigationContext
from app.models.settings import G8eeUserSettings
from app.models.tool_args import QueryInvestigationContextArgs
from app.models.tool_results import InvestigationContextResult, ToolResult

if TYPE_CHECKING:
    from app.services.ai.tool_service import AIToolService
    from app.services.investigation.investigation_service import InvestigationService

logger = logging.getLogger(__name__)


def build() -> types.ToolDeclaration:
    declaration = types.ToolDeclaration(
        name=OperatorToolName.QUERY_INVESTIGATION_CONTEXT,
        description=load_prompt(PromptFile.TOOL_QUERY_INVESTIGATION_CONTEXT),
        parameters=schema_from_model(QueryInvestigationContextArgs),
    )
    return declaration


async def _get_investigation_or_error(
    investigation_service: "InvestigationService",
    investigation_id: str,
    data_type: str,
) -> tuple[dict[str, Any] | None, InvestigationContextResult | None]:
    inv = await investigation_service.investigation_data_service.get_investigation(investigation_id)
    if inv:
        return inv.model_dump(), None
    return None, InvestigationContextResult(
        success=False,
        error=f"Investigation not found: {investigation_id}",
        error_type=CommandErrorType.VALIDATION_ERROR,
        data_type=data_type,
        investigation_id=investigation_id,
    )


async def handle(
    svc: "AIToolService",
    tool_args: dict[str, object],
    investigation: EnrichedInvestigationContext,
    g8e_context: G8eHttpContext,
    request_settings: G8eeUserSettings,
    execution_id: str,
) -> ToolResult:
    args = QueryInvestigationContextArgs.model_validate(tool_args)
    logger.info(
        "[QUERY_INVESTIGATION_CONTEXT] data_type=%s limit=%s",
        args.data_type, args.limit,
    )

    if not investigation or not investigation.id:
        logger.error("[QUERY_INVESTIGATION_CONTEXT] No investigation ID available")
        return InvestigationContextResult(
            success=False,
            error="No investigation ID available",
            error_type=CommandErrorType.VALIDATION_ERROR,
            data_type=args.data_type,
        )

    investigation_id = investigation.id
    investigation_service = svc.investigation_service
    data: dict[str, Any] | list[dict[str, Any]] | str | None = None
    item_count: int | None = None

    try:
        if args.data_type == "conversation_history":
            messages = await investigation_service.investigation_data_service.get_chat_messages(investigation_id)
            if args.limit:
                messages = messages[-args.limit:] if args.limit > 0 else messages
            data = [msg.model_dump() for msg in messages]
            item_count = len(messages)

        elif args.data_type == "investigation_status":
            data, error_res = await _get_investigation_or_error(
                investigation_service, investigation_id, args.data_type
            )
            if error_res:
                return error_res
            item_count = 1

        elif args.data_type == "history_trail":
            data, error_res = await _get_investigation_or_error(
                investigation_service, investigation_id, args.data_type
            )
            if error_res:
                return error_res
            item_count = 1

        elif args.data_type == "operator_actions":
            data = await investigation_service.investigation_data_service.get_operator_actions_for_ai_context(
                investigation_id
            )
            item_count = 1

        else:
            return InvestigationContextResult(
                success=False,
                error=(
                    f"Invalid data_type: {args.data_type}. Valid values: "
                    "conversation_history, investigation_status, history_trail, "
                    "operator_actions"
                ),
                error_type=CommandErrorType.VALIDATION_ERROR,
                data_type=args.data_type,
                investigation_id=investigation_id,
            )

        logger.info(
            "[QUERY_INVESTIGATION_CONTEXT] success=True item_count=%s", item_count
        )
        return InvestigationContextResult(
            success=True,
            data_type=args.data_type,
            data=data,
            item_count=item_count,
            investigation_id=investigation_id,
        )

    except Exception as e:
        logger.error("[QUERY_INVESTIGATION_CONTEXT] Failed: %s", e, exc_info=True)
        return InvestigationContextResult(
            success=False,
            error=f"Investigation context query failed: {e}. Retry or check investigation ID.",
            error_type=CommandErrorType.EXECUTION_ERROR,
            data_type=args.data_type,
            investigation_id=investigation_id,
        )
