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

"""``fetch_file_diff`` tool — return a diff between two operator-side file versions."""

from __future__ import annotations

import logging
from collections.abc import Callable
from typing import TYPE_CHECKING

import app.llm.llm_types as types
from app.constants.prompts import PromptFile
from app.constants.status import OperatorToolName
from app.llm.llm_types import schema_from_model
from app.llm.prompts import load_prompt
from app.models.command_request_payloads import FetchFileDiffRequestPayload
from app.models.http_context import G8eHttpContext
from app.models.investigations import EnrichedInvestigationContext
from app.models.settings import G8eeUserSettings
from app.models.tool_args import FetchFileDiffArgs
from app.models.tool_results import ToolResult
from app.services.ai.tools._base import convert_args_to_payload

if TYPE_CHECKING:
    from app.services.ai.tool_service import AIToolService

logger = logging.getLogger(__name__)


def build() -> tuple[types.ToolDeclaration, Callable[..., ToolResult]]:
    def fetch_file_diff(args: FetchFileDiffArgs) -> ToolResult:
        raise NotImplementedError(
            "fetch_file_diff should be called via execute_tool_call"
        )

    declaration = types.ToolDeclaration(
        name=OperatorToolName.FETCH_FILE_DIFF,
        description=load_prompt(PromptFile.TOOL_FETCH_FILE_DIFF),
        parameters=schema_from_model(FetchFileDiffArgs),
    )
    return declaration, fetch_file_diff


async def handle(
    svc: "AIToolService",
    tool_args: dict[str, object],
    investigation: EnrichedInvestigationContext,
    g8e_context: G8eHttpContext,
    request_settings: G8eeUserSettings,
    execution_id: str,
) -> ToolResult:
    args = convert_args_to_payload(tool_args, FetchFileDiffRequestPayload, execution_id)
    logger.info("[FETCH_FILE_DIFF] File path: %s", args.file_path)
    result = await svc.operator_command_service.execute_fetch_file_diff(
        args=args, investigation=investigation, g8e_context=g8e_context,
    )
    logger.info("[FETCH_FILE_DIFF] Result: %s", result)
    return result
