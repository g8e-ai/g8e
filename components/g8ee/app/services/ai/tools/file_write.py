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

"""``file_write_on_operator`` tool — overwrite an existing file."""

from __future__ import annotations

import logging
from collections.abc import Callable
from typing import TYPE_CHECKING

import app.llm.llm_types as types
from app.constants.prompts import PromptFile
from app.constants.status import FileOperation, OperatorToolName
from app.llm.llm_types import schema_from_model
from app.llm.prompts import load_prompt
from app.models.command_request_payloads import FileEditRequestPayload
from app.models.http_context import G8eHttpContext
from app.models.investigations import EnrichedInvestigationContext
from app.models.settings import G8eeUserSettings
from app.models.tool_args import FileWriteArgs
from app.models.tool_results import ToolResult

if TYPE_CHECKING:
    from app.services.ai.tool_service import AIToolService

logger = logging.getLogger(__name__)


def build() -> tuple[types.ToolDeclaration, Callable[..., ToolResult]]:
    def file_write_on_operator(args: FileEditRequestPayload) -> ToolResult:
        raise NotImplementedError(
            "file_write_on_operator should be called via execute_tool_call"
        )

    declaration = types.ToolDeclaration(
        name=OperatorToolName.FILE_WRITE,
        description=load_prompt(PromptFile.TOOL_FILE_WRITE),
        parameters=schema_from_model(FileWriteArgs),
    )
    return declaration, file_write_on_operator


async def handle(
    svc: "AIToolService",
    tool_args: dict[str, object],
    investigation: EnrichedInvestigationContext,
    g8e_context: G8eHttpContext,
    request_settings: G8eeUserSettings,
    execution_id: str,
) -> ToolResult:
    args = FileEditRequestPayload.model_validate({
        **tool_args,
        "execution_id": execution_id,
        "operation": FileOperation.WRITE,
    })
    logger.info("[FILE_WRITE] File path: %s", args.file_path)
    result = await svc.operator_command_service.execute_file_edit(
        args=args, g8e_context=g8e_context, investigation=investigation,
    )
    logger.info("[FILE_WRITE] Result: %s", result)
    return result
