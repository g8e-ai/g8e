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

"""``list_ssh_inventory`` tool — read-only enumeration of the SSH fleet."""

from __future__ import annotations

import logging
from typing import TYPE_CHECKING

import app.llm.llm_types as types
from app.constants.prompts import PromptFile
from app.constants.status import CommandErrorType, OperatorToolName
from app.errors import ConfigurationError
from app.llm.llm_types import schema_from_model
from app.llm.prompts import load_prompt
from app.models.http_context import G8eHttpContext
from app.models.investigations import EnrichedInvestigationContext
from app.models.settings import G8eeUserSettings
from app.models.tool_args import SshInventoryArgs
from app.models.tool_results import SshInventoryToolResult, ToolResult

if TYPE_CHECKING:
    from app.services.ai.tool_service import AIToolService

logger = logging.getLogger(__name__)


def build() -> types.ToolDeclaration:
    return types.ToolDeclaration(
        name=OperatorToolName.SSH_INVENTORY,
        description=load_prompt(PromptFile.TOOL_SSH_INVENTORY),
        parameters=schema_from_model(SshInventoryArgs),
    )


async def handle(
    svc: "AIToolService",
    tool_args: dict[str, object],
    investigation: EnrichedInvestigationContext,
    g8e_context: G8eHttpContext,
    request_settings: G8eeUserSettings,
    execution_id: str,
) -> ToolResult:
    args = SshInventoryArgs.model_validate(tool_args)
    logger.info("[SSH_INVENTORY] justification=%s", args.justification)

    try:
        inventory = svc.ssh_inventory_service.load()
    except ConfigurationError as exc:
        logger.warning("[SSH_INVENTORY] %s", exc)
        return SshInventoryToolResult(
            success=False,
            error=str(exc),
            error_type=CommandErrorType.CONFIGURATION_ERROR,
            source_path=svc.ssh_inventory_service.source_path,
        )

    logger.info(
        "[SSH_INVENTORY] source_path=%s host_count=%d",
        inventory.source_path, len(inventory.hosts),
    )
    return SshInventoryToolResult(
        success=True,
        source_path=inventory.source_path,
        hosts=inventory.hosts,
        total_count=len(inventory.hosts),
    )
