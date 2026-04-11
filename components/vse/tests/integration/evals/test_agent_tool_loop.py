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
Reliable AI Agent Tool Loop Evaluation Test Suite.

Tests the actual agent_tool_loop.py code with REAL services from ServiceFactory.
These tests exercise real AIToolService validation logic with real services,
but do not call any LLM (deterministic validation paths only).
"""

import pytest
import logging

from app.constants import OperatorStatus
from app.llm.llm_types import ToolCall
from app.models.settings import VSEUserSettings, LLMSettings
from tests.fakes.factories import (
    build_vso_http_context,
    build_enriched_investigation,
    build_bound_operator,
    build_operator_document,
)

logger = logging.getLogger(__name__)

pytestmark = [pytest.mark.integration, pytest.mark.slow]


@pytest.mark.asyncio
async def test_orchestrate_tool_execution_no_bound_operator(
    cache_aside_service,
    unique_investigation_id,
    unique_case_id,
    unique_web_session_id,
    all_services,
    tool_service,
):
    """
    Test that operator tools fail gracefully when no operator is bound.

    This tests the REAL validation logic in AIToolService.execute_tool_call.
    """
    investigation = build_enriched_investigation(
        investigation_id=unique_investigation_id,
        case_id=unique_case_id,
        operator_documents=[],
    )

    vso_context = build_vso_http_context(
        web_session_id=unique_web_session_id,
        user_id="user-test-001",
        bound_operators=[],
    )

    user_settings = VSEUserSettings(llm=LLMSettings())

    tool_call = ToolCall(
        name="run_commands_with_operator",
        args={"command": "ls /tmp", "justification": "List files"},
        id="tool-call-001",
    )

    # Call tool_service.execute_tool_call directly to test validation logic
    result = await tool_service.execute_tool_call(
        tool_name=tool_call.name,
        tool_args=tool_call.args,
        investigation=investigation,
        vso_context=vso_context,
        request_settings=user_settings,
    )

    assert result.success is False
    assert "No operators are currently BOUND" in result.error


@pytest.mark.asyncio
async def test_orchestrate_tool_execution_security_violation(
    cache_aside_service,
    unique_investigation_id,
    unique_case_id,
    unique_web_session_id,
    all_services,
    tool_service,
):
    """
    Test that forbidden command patterns are blocked.

    This tests the REAL security validation in AIToolService.execute_tool_call.
    """
    operator_doc = build_operator_document(
        operator_id="op-test-001",
        hostname="test-server-01",
        status=OperatorStatus.BOUND,
    )

    investigation = build_enriched_investigation(
        investigation_id=unique_investigation_id,
        case_id=unique_case_id,
        operator_documents=[operator_doc],
    )

    bound_op = build_bound_operator(
        operator_id="op-test-001",
        operator_session_id="sess-test-001",
        status=OperatorStatus.BOUND,
    )

    vso_context = build_vso_http_context(
        web_session_id=unique_web_session_id,
        user_id="user-test-001",
        bound_operators=[bound_op],
    )

    user_settings = VSEUserSettings(llm=LLMSettings())

    tool_call = ToolCall(
        name="run_commands_with_operator",
        args={"command": "sudo rm -rf /", "justification": "Clean system"},
        id="tool-call-001",
    )

    # Call tool_service.execute_tool_call directly to test validation logic
    result = await tool_service.execute_tool_call(
        tool_name=tool_call.name,
        tool_args=tool_call.args,
        investigation=investigation,
        vso_context=vso_context,
        request_settings=user_settings,
    )

    assert result.success is False
    assert "SECURITY VIOLATION" in result.error
    assert result.error_type == "security.violation"
