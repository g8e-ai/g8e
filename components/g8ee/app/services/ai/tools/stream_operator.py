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

"""``stream_operator`` tool — shotgun operator binary across the SSH fleet."""

from __future__ import annotations

import logging
from typing import TYPE_CHECKING

import app.llm.llm_types as types
from app.constants.prompts import PromptFile
from app.constants.status import CommandErrorType, OperatorToolName
from app.llm.llm_types import schema_from_model
from app.llm.prompts import load_prompt
from app.models.http_context import G8eHttpContext
from app.models.investigations import EnrichedInvestigationContext
from app.models.settings import G8eeUserSettings
from app.models.tool_args import StreamOperatorArgs
from app.models.tool_results import ToolResult # We'll need a concrete result type later

if TYPE_CHECKING:
    from app.services.ai.tool_service import AIToolService

logger = logging.getLogger(__name__)


def build() -> types.ToolDeclaration:
    return types.ToolDeclaration(
        name=OperatorToolName.STREAM_OPERATOR,
        description=load_prompt(PromptFile.TOOL_STREAM_OPERATOR),
        parameters=schema_from_model(StreamOperatorArgs),
    )


async def handle(
    svc: "AIToolService",
    tool_args: dict[str, object],
    investigation: EnrichedInvestigationContext,
    g8e_context: G8eHttpContext,
    request_settings: G8eeUserSettings,
    execution_id: str,
) -> ToolResult:
    """Execute the stream_operator tool via the OperatorStreamExecutor."""
    args = StreamOperatorArgs.model_validate(tool_args)
    logger.info("[STREAM_OPERATOR] Dispatching to executor: hosts=%d", len(args.hosts))
    
    return await svc.stream_executor.execute_stream(
        args=args,
        g8e_context=g8e_context,
        execution_id=execution_id,
    )
