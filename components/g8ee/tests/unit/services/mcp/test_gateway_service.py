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

"""
Unit tests for MCPGatewayService.

Tests:
- list_tools returns correct MCP format from tool declarations
- list_tools with no bound operators returns only web search (if configured)
- call_tool dispatches through AIToolService and wraps result
- call_tool with no bound operator propagates the error from AIToolService
- call_tool with unknown tool returns error content
- call_tool times out gracefully when tool execution exceeds MCP_TOOL_CALL_TIMEOUT_SECONDS
- call_tool resets invocation context even on timeout
- _build_investigation_context resolves operator docs from bound_operators
- _tool_result_to_mcp converts success/error results correctly
"""

import asyncio
from unittest.mock import AsyncMock, MagicMock, patch

import pytest

from app.constants.prompts import AgentMode
from app.constants.settings import MCP_TOOL_CALL_TIMEOUT_SECONDS
from app.constants.status import OperatorStatus, OperatorToolName
from app.models.tool_results import CommandExecutionResult
from app.services.ai.tool_service import AIToolService
from app.services.investigation.investigation_service import InvestigationService
from app.services.mcp.gateway_service import MCPGatewayService
from app.services.operator.operator_data_service import OperatorDataService
from tests.fakes.factories import (
    build_bound_operator,
    build_operator_document,
    build_vso_http_context,
)

pytestmark = pytest.mark.unit


def _make_gateway() -> tuple[MCPGatewayService, MagicMock, MagicMock, MagicMock]:
    tool_service = MagicMock(spec=AIToolService)
    investigation_service = MagicMock(spec=InvestigationService)
    operator_data_service = MagicMock(spec=OperatorDataService)

    gateway = MCPGatewayService(
        tool_service=tool_service,
        investigation_service=investigation_service,
        operator_data_service=operator_data_service,
    )
    return gateway, tool_service, investigation_service, operator_data_service


# =============================================================================
# list_tools
# =============================================================================

class TestListTools:
    def test_returns_mcp_format(self):
        gateway, tool_service, _, _ = _make_gateway()

        import app.llm.llm_types as types
        decl = types.ToolDeclaration(
            name="run_commands_with_operator",
            description="Run a command",
            parameters={"type": "object", "properties": {"command": {"type": "string"}}},
        )
        tool_service.get_tools.return_value = [
            types.ToolGroup(tools=[decl])
        ]

        result = gateway.list_tools(AgentMode.OPERATOR_BOUND)

        assert len(result) == 1
        assert result[0]["name"] == "run_commands_with_operator"
        assert result[0]["description"] == "Run a command"
        assert "inputSchema" in result[0]
        assert result[0]["inputSchema"]["type"] == "object"

    def test_empty_when_no_tools(self):
        gateway, tool_service, _, _ = _make_gateway()
        tool_service.get_tools.return_value = []

        result = gateway.list_tools(AgentMode.OPERATOR_NOT_BOUND)

        assert result == []

    def test_multiple_tools_flattened(self):
        gateway, tool_service, _, _ = _make_gateway()

        import app.llm.llm_types as types
        decl_a = types.ToolDeclaration(name="tool_a", description="A", parameters={})
        decl_b = types.ToolDeclaration(name="tool_b", description="B", parameters={})
        tool_service.get_tools.return_value = [
            types.ToolGroup(tools=[decl_a, decl_b])
        ]

        result = gateway.list_tools(AgentMode.OPERATOR_BOUND)

        assert len(result) == 2
        names = {t["name"] for t in result}
        assert names == {"tool_a", "tool_b"}


# =============================================================================
# call_tool
# =============================================================================

@pytest.mark.asyncio(loop_scope="session")
class TestCallTool:
    async def test_success_returns_mcp_content(self):
        gateway, tool_service, _, operator_data_service = _make_gateway()

        operator_doc = build_operator_document(operator_id="op-1", user_id="u-1")
        operator_data_service.get_operator = AsyncMock(return_value=operator_doc)

        tool_service.start_invocation_context.return_value = "token"
        tool_service.reset_invocation_context = MagicMock()
        tool_service.execute_tool = AsyncMock(return_value=CommandExecutionResult(
            success=True,
            output="hello world",
        ))

        ctx = build_vso_http_context(
            bound_operators=[build_bound_operator(operator_id="op-1")]
        )

        result = await gateway.call_tool(
            tool_name=OperatorToolName.RUN_COMMANDS,
            arguments={"command": "echo hello"},
            vso_context=ctx,
        )

        assert result["isError"] is False
        assert result["content"][0]["type"] == "text"
        assert "hello world" in result["content"][0]["text"]
        tool_service.execute_tool.assert_awaited_once()

    async def test_error_result_sets_is_error(self):
        gateway, tool_service, _, operator_data_service = _make_gateway()

        operator_data_service.get_operator = AsyncMock(return_value=None)
        tool_service.start_invocation_context.return_value = "token"
        tool_service.reset_invocation_context = MagicMock()
        tool_service.execute_tool = AsyncMock(return_value=CommandExecutionResult(
            success=False,
            error="No operators available",
        ))

        ctx = build_vso_http_context()

        result = await gateway.call_tool(
            tool_name=OperatorToolName.RUN_COMMANDS,
            arguments={"command": "ls"},
            vso_context=ctx,
        )

        assert result["isError"] is True
        assert "No operators available" in result["content"][0]["text"]

    async def test_timeout_returns_graceful_error(self):
        gateway, tool_service, _, operator_data_service = _make_gateway()

        operator_data_service.get_operator = AsyncMock(return_value=None)
        tool_service.start_invocation_context.return_value = "token"
        tool_service.reset_invocation_context = MagicMock()

        async def _hang_forever(**kwargs):
            await asyncio.sleep(999)

        tool_service.execute_tool = AsyncMock(side_effect=_hang_forever)

        ctx = build_vso_http_context()

        with patch("app.services.mcp.gateway_service.MCP_TOOL_CALL_TIMEOUT_SECONDS", 0.05):
            result = await gateway.call_tool(
                tool_name=OperatorToolName.RUN_COMMANDS,
                arguments={"command": "ls"},
                vso_context=ctx,
            )

        assert result["isError"] is True
        assert "timed out" in result["content"][0]["text"]
        assert "human approval" in result["content"][0]["text"]
        assert "still pending" in result["content"][0]["text"]

    async def test_timeout_still_resets_invocation_context(self):
        gateway, tool_service, _, operator_data_service = _make_gateway()

        operator_data_service.get_operator = AsyncMock(return_value=None)
        tool_service.start_invocation_context.return_value = "token"
        tool_service.reset_invocation_context = MagicMock()

        async def _hang_forever(**kwargs):
            await asyncio.sleep(999)

        tool_service.execute_tool = AsyncMock(side_effect=_hang_forever)

        ctx = build_vso_http_context()

        with patch("app.services.mcp.gateway_service.MCP_TOOL_CALL_TIMEOUT_SECONDS", 0.05):
            await gateway.call_tool(
                tool_name="some_tool",
                arguments={},
                vso_context=ctx,
            )

        tool_service.reset_invocation_context.assert_called_once_with("token")

    async def test_invocation_context_always_reset(self):
        gateway, tool_service, _, operator_data_service = _make_gateway()

        operator_data_service.get_operator = AsyncMock(return_value=None)
        tool_service.start_invocation_context.return_value = "token"
        tool_service.reset_invocation_context = MagicMock()
        tool_service.execute_tool = AsyncMock(side_effect=RuntimeError("boom"))

        ctx = build_vso_http_context()

        with pytest.raises(RuntimeError, match="boom"):
            await gateway.call_tool(
                tool_name="bad_tool",
                arguments={},
                vso_context=ctx,
            )

        tool_service.reset_invocation_context.assert_called_once_with("token")


# =============================================================================
# _build_investigation_context
# =============================================================================

@pytest.mark.asyncio(loop_scope="session")
class TestBuildInvestigationContext:
    async def test_resolves_operator_documents(self):
        gateway, _, _, operator_data_service = _make_gateway()

        op_doc = build_operator_document(operator_id="op-1", user_id="u-1")
        operator_data_service.get_operator = AsyncMock(return_value=op_doc)

        ctx = build_vso_http_context(
            bound_operators=[build_bound_operator(operator_id="op-1")]
        )

        investigation = await gateway._build_investigation_context(ctx)

        assert len(investigation.operator_documents) == 1
        assert investigation.operator_documents[0].operator_id == "op-1"
        assert investigation.user_id == ctx.user_id

    async def test_skips_non_bound_operators(self):
        gateway, _, _, operator_data_service = _make_gateway()

        operator_data_service.get_operator = AsyncMock()

        ctx = build_vso_http_context(
            bound_operators=[
                build_bound_operator(operator_id="op-1", status=OperatorStatus.AVAILABLE),
            ]
        )

        investigation = await gateway._build_investigation_context(ctx)

        assert len(investigation.operator_documents) == 0
        operator_data_service.get_operator.assert_not_awaited()

    async def test_handles_missing_operator_doc(self):
        gateway, _, _, operator_data_service = _make_gateway()

        operator_data_service.get_operator = AsyncMock(return_value=None)

        ctx = build_vso_http_context(
            bound_operators=[build_bound_operator(operator_id="op-missing")]
        )

        investigation = await gateway._build_investigation_context(ctx)

        assert len(investigation.operator_documents) == 0

    async def test_sentinel_mode_default_true(self):
        gateway, _, _, operator_data_service = _make_gateway()

        operator_data_service.get_operator = AsyncMock(return_value=None)

        ctx = build_vso_http_context()

        investigation = await gateway._build_investigation_context(ctx)

        assert investigation.sentinel_mode is True

    async def test_sentinel_mode_false_when_explicit(self):
        gateway, _, _, operator_data_service = _make_gateway()

        operator_data_service.get_operator = AsyncMock(return_value=None)

        ctx = build_vso_http_context()

        investigation = await gateway._build_investigation_context(ctx, sentinel_mode=False)

        assert investigation.sentinel_mode is False


# =============================================================================
# _tool_result_to_mcp
# =============================================================================

class TestToolResultToMCP:
    def test_success_result(self):
        result = CommandExecutionResult(success=True, output="ok")
        mcp = MCPGatewayService._tool_result_to_mcp(result)

        assert mcp["isError"] is False
        assert mcp["content"][0]["text"] == "ok"

    def test_error_result(self):
        result = CommandExecutionResult(success=False, error="denied")
        mcp = MCPGatewayService._tool_result_to_mcp(result)

        assert mcp["isError"] is True
        assert mcp["content"][0]["text"] == "denied"

    def test_fallback_serialization(self):
        result = CommandExecutionResult(success=True)
        mcp = MCPGatewayService._tool_result_to_mcp(result)

        assert mcp["isError"] is False
        assert mcp["content"][0]["type"] == "text"
        assert len(mcp["content"][0]["text"]) > 0
