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

from __future__ import annotations

import asyncio
import logging
from typing import Any

from pydantic import BaseModel

from app.constants.prompts import AgentMode
from app.constants.settings import MCP_TOOL_CALL_TIMEOUT_SECONDS
from app.constants.status import OperatorStatus
from app.models.http_context import G8eHttpContext
from app.models.investigations import EnrichedInvestigationContext
from app.models.settings import G8eeUserSettings
from app.services.ai.tool_service import AIToolService
from app.services.investigation.investigation_service import InvestigationService
from app.models.operators import OperatorDocument
from app.services.operator.operator_data_service import OperatorDataService
from app.utils.version import get_version

logger = logging.getLogger(__name__)

MCP_SERVER_INFO: dict[str, Any] = {
    "protocolVersion": "2025-03-26",
    "serverInfo": {
        "name": "g8e",
        "version": get_version(),
    },
    "capabilities": {
        "tools": {},
    },
}


class MCPGatewayService:
    """Translates MCP JSON-RPC tool calls into internal g8e tool execution.

    This service sits between the g8ed HTTP gateway and the existing
    AIToolService pipeline. It converts MCP tool declarations and call
    results between MCP wire format and the internal ToolResult types.
    """

    def __init__(
        self,
        tool_service: AIToolService,
        investigation_service: InvestigationService,
        operator_data_service: OperatorDataService,
    ):
        self._tool_service = tool_service
        self._investigation_service = investigation_service
        self._operator_data_service = operator_data_service
        logger.info("MCPGatewayService initialized")

    def list_tools(self, agent_mode: AgentMode, model_to_use: str | None = None) -> list[dict[str, Any]]:
        """Return tool declarations formatted as MCP tools/list response items.

        Each item has: { name, description, inputSchema }.
        """
        tool_groups = self._tool_service.get_tools(agent_mode, model_to_use)
        mcp_tools: list[dict[str, Any]] = []
        for group in tool_groups:
            for decl in group.tools:
                input_schema: dict[str, Any] = (
                    decl.parameters if isinstance(decl.parameters, dict) else {}
                )
                mcp_tools.append({
                    "name": decl.name,
                    "description": decl.description,
                    "inputSchema": input_schema,
                })
        return mcp_tools

    async def call_tool(
        self,
        tool_name: str,
        arguments: dict[str, Any],
        g8e_context: G8eHttpContext,
        user_settings: G8eeUserSettings | None = None,
        sentinel_mode: bool = True,
    ) -> dict[str, Any]:
        """Execute an MCP tool call through the existing governance pipeline.

        Returns an MCP CallToolResult dict: { content: [...], isError: bool }.
        """
        investigation = await self._build_investigation_context(g8e_context, sentinel_mode)

        context_token = self._tool_service.start_invocation_context(
            g8e_context=g8e_context,
        )
        try:
            # MCP callers may not provide user settings; use defaults if missing
            # G8eeUserSettings requires llm field; LLMSettings defaults to OLLAMA provider.
            from app.models.settings import LLMSettings
            default_settings = G8eeUserSettings(llm=LLMSettings())
            
            result = await asyncio.wait_for(
                self._tool_service.execute_tool_call(
                    tool_name=tool_name,
                    tool_args=arguments,
                    investigation=investigation,
                    g8e_context=g8e_context,
                    request_settings=user_settings or default_settings,
                ),
                timeout=MCP_TOOL_CALL_TIMEOUT_SECONDS,
            )
        except (asyncio.TimeoutError, TimeoutError):
            logger.warning(
                "[MCP] Tool call timed out after %ds (likely waiting for human approval)",
                MCP_TOOL_CALL_TIMEOUT_SECONDS,
                extra={"tool_name": tool_name},
            )
            return {
                "content": [{
                    "type": "text",
                    "text": f"Tool call timed out after {MCP_TOOL_CALL_TIMEOUT_SECONDS}s. "
                            "This operation requires human approval in the g8e dashboard. "
                            "The approval request is still pending and will be processed "
                            "when the user responds.",
                }],
                "isError": True,
            }
        finally:
            self._tool_service.reset_invocation_context(context_token)

        return self._tool_result_to_mcp(result)

    async def _build_investigation_context(
        self,
        g8e_context: G8eHttpContext,
        sentinel_mode: bool = True,
    ) -> EnrichedInvestigationContext:
        """Build a synthetic EnrichedInvestigationContext from G8eHttpContext.

        External MCP callers may not have an active investigation. We build a
        minimal context with operator documents resolved from bound_operators,
        reusing the same operator-data lookup path as InvestigationService.
        """
        operator_docs: list[OperatorDocument] = []
        for bound_op in (g8e_context.bound_operators or []):
            if bound_op.status != OperatorStatus.BOUND:
                continue
            try:
                doc = await self._operator_data_service.get_operator(bound_op.operator_id)
                if doc:
                    operator_docs.append(doc)
            except Exception as e:
                logger.error(
                    "MCP gateway: failed to fetch operator document",
                    extra={"operator_id": bound_op.operator_id, "error": str(e)},
                )

        return EnrichedInvestigationContext(
            id=g8e_context.execution_id or "mcp-gateway",
            case_id=g8e_context.case_id or "mcp-gateway",
            user_id=g8e_context.user_id,
            organization_id=g8e_context.organization_id,
            web_session_id=g8e_context.web_session_id,
            sentinel_mode=sentinel_mode,
            operator_documents=operator_docs,
            bound_operators=g8e_context.bound_operators or [],
        )

    @staticmethod
    def _tool_result_to_mcp(result: Any) -> dict[str, Any]:
        """Convert an internal ToolResult into MCP CallToolResult format."""
        is_error = False
        text_content = ""

        if hasattr(result, "success"):
            is_error = not result.success

        if hasattr(result, "error") and result.error:
            text_content = str(result.error)
            is_error = True
        elif hasattr(result, "output") and result.output:
            text_content = str(result.output)
        elif isinstance(result, BaseModel):
            text_content = str(result.model_dump(mode="json"))
        else:
            text_content = str(result)

        return {
            "content": [{"type": "text", "text": text_content}],
            "isError": is_error,
        }
