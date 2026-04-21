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
Unit tests for orchestrate_tool_execution (agent_tool_loop).

Tests:
- execution_id generation for operator functions (format, uniqueness)
- execution_id is None for non-operator functions
- is_operator_tool detection via OperatorToolName membership
- execution_id and _web_session_id injected into tool_args_with_id for operator functions
- internal fields NOT injected for non-operator functions (e.g. search_web)
- ToolCallResult structure — tool_name, call_info, result_info typed models
- call_info.is_operator_tool reflects correct detection
- Tribunal refinement: generate_command called for run_commands_with_operator
- Tribunal refinement: refined command replaces original in tool_args
- Tribunal refinement: unchanged command does not alter tool_args
- Tribunal refinement: skipped for non-command operator functions
- Tribunal system error: TribunalSystemError halts execution and returns failed ToolCallResult
- tool_name extracted from .name attribute
- tool_name extracted from .tool_name fallback attribute
- ToolCallResult.result carries through the raw handler result
- Target operator resolution: by operator_id, hostname, index, fallback
- No target_operator defaults to first operator

Run with:
    ./g8e test g8ee -- tests/unit/services/ai/test_agent_orchestrate_tool_execution.py
"""

from unittest.mock import AsyncMock, MagicMock, patch

import pytest

import app.services.ai.agent_tool_loop as agent_tool_loop_module
from app.constants import CommandErrorType, OperatorStatus, OperatorToolName, OperatorType
from app.models.agents.tribunal import TribunalSystemError
from app.llm.llm_types import ToolCall
from app.models.agent import StreamChunkData
from app.models.operators import OperatorDocument, OperatorSystemInfo
from app.services.ai.agent_tool_loop import ToolCallResult
from app.models.tool_results import CommandExecutionResult
from app.models.settings import LLMSettings, G8eeUserSettings
from app.services.ai.agent_tool_loop import orchestrate_tool_execution
from app.services.ai.tool_service import AIToolService
from tests.fakes.factories import (
    build_g8e_http_context,
    build_enriched_context,
    build_bound_operator,
)

pytestmark = [pytest.mark.unit, pytest.mark.asyncio(loop_scope="session")]


# =============================================================================
# FIXTURES
# =============================================================================

@pytest.fixture
def mock_tool_executor():
    """Mock AIToolService executor for unit tests."""
    executor = MagicMock(spec=AIToolService)
    executor.web_search_provider = None
    
    # Mock OperatorCommandService with required _settings
    mock_exec_svc = MagicMock()
    mock_exec_svc._settings = MagicMock()
    executor.operator_command_service = mock_exec_svc
    
    # Mock user settings and validators for command constraints
    from app.models.settings import CommandValidationSettings
    mock_user_settings = MagicMock()
    mock_user_settings.command_validation = CommandValidationSettings()
    executor._user_settings = mock_user_settings
    
    from app.utils.validators import get_whitelist_validator, get_blacklist_validator
    executor._whitelist_validator = get_whitelist_validator()
    executor._blacklist_validator = get_blacklist_validator()
    
    return executor


@pytest.fixture
def request_settings():
    """Sample request settings for testing."""
    return G8eeUserSettings(llm=LLMSettings())


@pytest.fixture
def sample_investigation(unique_investigation_id, unique_case_id, unique_user_id, unique_operator_id, unique_session_id, unique_web_session_id):
    """Sample investigation for testing with unique IDs."""
    return build_enriched_context(
        investigation_id=unique_investigation_id,
        case_id=unique_case_id,
        user_id=unique_user_id,
        operator_documents=[
            OperatorDocument(
                id=unique_operator_id,
                operator_session_id=unique_session_id,
                status=OperatorStatus.AVAILABLE,
                operator_type=OperatorType.SYSTEM,
                system_info=OperatorSystemInfo(
                    hostname="op-1-host",
                    os="linux",
                    architecture="amd64",
                    cpu_count=2,
                    memory_mb=4096,
                ),
                user_id=unique_user_id,
                bound_web_session_id=unique_web_session_id,
            )
        ],
    )


@pytest.fixture
def sample_g8e_context(unique_user_id, unique_case_id, unique_investigation_id, unique_web_session_id, unique_operator_id, unique_session_id):
    """Sample g8e HTTP context for testing with unique IDs."""
    bound_operator = build_bound_operator(
        operator_id=unique_operator_id,
        operator_session_id=unique_session_id,
    )
    return build_g8e_http_context(
        user_id=unique_user_id,
        case_id=unique_case_id,
        investigation_id=unique_investigation_id,
        web_session_id=unique_web_session_id,
        bound_operators=[bound_operator],
    )


# =============================================================================
# HELPER FUNCTIONS
# =============================================================================

def _mock_executor_success(executor: MagicMock, output: str = "ok") -> None:
    result = CommandExecutionResult(success=True, output=output, exit_code=0, command_executed="mock")
    executor.execute_tool_call = AsyncMock(return_value=result)


def _mock_executor_failure(executor: MagicMock, error_type: CommandErrorType = CommandErrorType.EXECUTION_ERROR) -> None:
    result = CommandExecutionResult(success=False, error="failed", error_type=error_type)
    executor.execute_tool_call = AsyncMock(return_value=result)


def _noop_generate_command(request: str, **_kwargs):
    """Mock Tribunal that passes the request through verbatim as the final command.

    Used by tests that need Tribunal to run but do not care about refinement.
    In production the Tribunal would never return the raw natural-language
    request as a shell command; tests only care that the pipeline flows.
    """
    from app.services.ai.command_generator import CommandGenerationOutcome, CommandGenerationResult
    return CommandGenerationResult(
        request=request,
        final_command=request,
        outcome=CommandGenerationOutcome.CONSENSUS,
    )


def _refining_generate_command(request: str, refined: str, **_kwargs):
    """Mock Tribunal that produces a refined command distinct from the request."""
    from app.services.ai.command_generator import CommandGenerationOutcome, CommandGenerationResult
    return CommandGenerationResult(
        request=request,
        final_command=refined,
        outcome=CommandGenerationOutcome.CONSENSUS,
    )


NON_OPERATOR_FUNCTION = "some_unknown_tool_that_is_not_registered"


# =============================================================================
# TEST: execution_id generation
# =============================================================================

class TestExecutionIdGeneration:

    async def test_operator_tool_getsexecution_id(self, mock_tool_executor, sample_investigation, sample_g8e_context, request_settings, mock_event_service):
        _mock_executor_success(mock_tool_executor)

        with patch.object(agent_tool_loop_module, "generate_command", new=AsyncMock(side_effect=_noop_generate_command)):
            result = await orchestrate_tool_execution(
                ToolCall(name=OperatorToolName.RUN_COMMANDS, args={"request": "list files"}),
                tool_executor=mock_tool_executor,
                investigation=sample_investigation,
                g8e_context=sample_g8e_context,
                g8ed_event_service=mock_event_service,
                request_settings=request_settings,
            )

        assert result.call_info.execution_id is not None

    async def test_execution_id_unique_per_call(self, mock_tool_executor, sample_investigation, sample_g8e_context, request_settings, mock_event_service):
        _mock_executor_success(mock_tool_executor)

        ids = []
        with patch.object(agent_tool_loop_module, "generate_command", new=AsyncMock(side_effect=_noop_generate_command)):
            for _ in range(3):
                result = await orchestrate_tool_execution(
                    ToolCall(name=OperatorToolName.RUN_COMMANDS, args={"request": "show current user"}),
                    tool_executor=mock_tool_executor,
                    investigation=sample_investigation,
                    g8e_context=sample_g8e_context,
                    g8ed_event_service=mock_event_service,
                    request_settings=request_settings,
                )
                ids.append(result.call_info.execution_id)

        assert len(set(ids)) == len(ids), "execution_ids must be unique across calls"

    async def test_non_operator_tool_has_execution_id(self, mock_tool_executor, sample_investigation, sample_g8e_context, request_settings, mock_event_service):
        result = CommandExecutionResult(success=True, output="results")
        mock_tool_executor.execute_tool_call = AsyncMock(return_value=result)

        result = await orchestrate_tool_execution(
            ToolCall(name=NON_OPERATOR_FUNCTION, args={}),
            tool_executor=mock_tool_executor,
            investigation=sample_investigation,
            g8e_context=sample_g8e_context,
            g8ed_event_service=mock_event_service,
            request_settings=request_settings,
        )

        assert result.call_info.execution_id is not None
        import re
        pattern = r"^cmd_[0-9a-f]{12}_\d+$"
        assert re.match(pattern, result.call_info.execution_id), (
            f"execution_id '{result.call_info.execution_id}' does not match expected format"
        )

    async def test_execution_id_format_matches_expected_pattern(self, mock_tool_executor, sample_investigation, sample_g8e_context, request_settings, mock_event_service):
        """execution_id must be 'cmd_<12hex>_<timestamp_int>'."""
        import re
        _mock_executor_success(mock_tool_executor)

        with patch.object(agent_tool_loop_module, "generate_command", new=AsyncMock(side_effect=_noop_generate_command)):
            result = await orchestrate_tool_execution(
                ToolCall(name=OperatorToolName.RUN_COMMANDS, args={"request": "show user id"}),
                tool_executor=mock_tool_executor,
                investigation=sample_investigation,
                g8e_context=sample_g8e_context,
                g8ed_event_service=mock_event_service,
                request_settings=request_settings,
            )

        pattern = r"^cmd_[0-9a-f]{12}_\d+$"
        assert re.match(pattern, result.call_info.execution_id), (
            f"execution_id '{result.call_info.execution_id}' does not match expected format"
        )


# =============================================================================
# TEST: operator function detection
# =============================================================================

class TestOperatorToolDetection:

    async def test_run_commands_detected_as_operator_tool(self, mock_tool_executor, sample_investigation, sample_g8e_context, request_settings, mock_event_service):
        _mock_executor_success(mock_tool_executor)

        with patch.object(agent_tool_loop_module, "generate_command", new=AsyncMock(side_effect=_noop_generate_command)):
            result = await orchestrate_tool_execution(
                ToolCall(name=OperatorToolName.RUN_COMMANDS, args={"request": "list files"}),
                tool_executor=mock_tool_executor,
                investigation=sample_investigation,
                g8e_context=sample_g8e_context,
                g8ed_event_service=mock_event_service,
                request_settings=request_settings,
            )

        assert result.call_info.is_operator_tool is True

    async def test_file_read_detected_as_operator_tool(self, mock_tool_executor, sample_investigation, sample_g8e_context, request_settings, mock_event_service):
        _mock_executor_success(mock_tool_executor)

        result = await orchestrate_tool_execution(
            ToolCall(name=OperatorToolName.FILE_READ, args={"file_path": "/etc/hosts"}),
            tool_executor=mock_tool_executor,
            investigation=sample_investigation,
            g8e_context=sample_g8e_context,
            g8ed_event_service=mock_event_service,
            request_settings=request_settings,
        )

        assert result.call_info.is_operator_tool is True

    async def test_search_web_is_not_operator_tool(self, mock_tool_executor, sample_investigation, sample_g8e_context, request_settings, mock_event_service):
        """search_web does not require an operator."""
        result = CommandExecutionResult(success=True, output="results")
        mock_tool_executor.execute_tool_call = AsyncMock(return_value=result)

        result = await orchestrate_tool_execution(
            ToolCall(name=OperatorToolName.G8E_SEARCH_WEB, args={"query": "test"}),
            tool_executor=mock_tool_executor,
            investigation=sample_investigation,
            g8e_context=sample_g8e_context,
            g8ed_event_service=mock_event_service,
            request_settings=request_settings,
        )

        assert result.call_info.is_operator_tool is False

    async def test_unregistered_tool_not_operator_tool(self, mock_tool_executor, sample_investigation, sample_g8e_context, request_settings, mock_event_service):
        result = CommandExecutionResult(success=True, output="ok")
        mock_tool_executor.execute_tool_call = AsyncMock(return_value=result)

        result = await orchestrate_tool_execution(
            ToolCall(name=NON_OPERATOR_FUNCTION, args={}),
            tool_executor=mock_tool_executor,
            investigation=sample_investigation,
            g8e_context=sample_g8e_context,
            g8ed_event_service=mock_event_service,
            request_settings=request_settings,
        )

        assert result.call_info.is_operator_tool is False


# =============================================================================
# TEST: internal field injection into tool_args_with_id
# =============================================================================

class TestToolArgsInjection:

    async def test_execution_id_passed_as_kwarg_for_operator_tool(self, mock_tool_executor, sample_investigation, sample_g8e_context, request_settings, mock_event_service):
        """The canonical per-tool execution_id is threaded to the executor via the
        ``execution_id`` keyword — not stuffed into the LLM-facing tool_args dict.
        This keeps the args validated by the typed Pydantic model clean while still
        giving the executor (and every downstream operator service) an authoritative
        id to use for the execution registry and UI lifecycle events.
        """
        _mock_executor_success(mock_tool_executor)
        captured_kwargs = {}
        captured_args = {}

        async def capture_call(tool_name, tool_args, investigation, g8e_context, **kwargs):
            nonlocal captured_args, captured_kwargs
            captured_args = tool_args
            captured_kwargs = kwargs
            return CommandExecutionResult(success=True, output="ok")

        mock_tool_executor.execute_tool_call = AsyncMock(side_effect=capture_call)

        with patch.object(agent_tool_loop_module, "generate_command", new=AsyncMock(side_effect=_noop_generate_command)):
            await orchestrate_tool_execution(
                ToolCall(name=OperatorToolName.RUN_COMMANDS, args={"request": "show disk usage"}),
                tool_executor=mock_tool_executor,
                investigation=sample_investigation,
                g8e_context=sample_g8e_context,
                g8ed_event_service=mock_event_service,
                request_settings=request_settings,
            )

        # execution_id is delivered as a typed kwarg, not silently stuffed into args.
        assert captured_kwargs.get("execution_id") is not None
        assert captured_kwargs["execution_id"].startswith("cmd_")
        # LLM-facing args dict never carries internal routing fields.
        assert "execution_id" not in captured_args
        assert "_web_session_id" not in captured_args

    async def test_execution_id_kwarg_none_for_non_operator_tool(self, mock_tool_executor, sample_investigation, sample_g8e_context, request_settings, mock_event_service):
        captured_kwargs = {}
        captured_args = {}

        async def capture_call(tool_name, tool_args, investigation, g8e_context, **kwargs):
            nonlocal captured_args, captured_kwargs
            captured_args = tool_args
            captured_kwargs = kwargs
            return CommandExecutionResult(success=True, output="ok")

        mock_tool_executor.execute_tool_call = AsyncMock(side_effect=capture_call)

        await orchestrate_tool_execution(
            ToolCall(name=NON_OPERATOR_FUNCTION, args={"query": "linux memory usage"}),
            tool_executor=mock_tool_executor,
            investigation=sample_investigation,
            g8e_context=sample_g8e_context,
            g8ed_event_service=mock_event_service,
            request_settings=request_settings,
        )

        # Non-operator tools do get an execution_id for tracking.
        assert captured_kwargs.get("execution_id") is not None
        assert "execution_id" not in captured_args

    async def test_original_args_preserved_without_hidden_injection(self, mock_tool_executor, sample_investigation, sample_g8e_context, request_settings, mock_event_service):
        """The Tribunal adds ``command`` to the executor args; the caller's original
        ``request``/``guidelines`` are preserved. Internal routing fields
        (``execution_id``, ``_web_session_id``) MUST NOT be silently injected —
        they are passed as typed parameters instead."""
        captured_args = {}

        async def capture_call(tool_name, tool_args, investigation, g8e_context, **kwargs):
            nonlocal captured_args
            captured_args = tool_args
            return CommandExecutionResult(success=True, output="ok")

        mock_tool_executor.execute_tool_call = AsyncMock(side_effect=capture_call)

        with patch.object(agent_tool_loop_module, "generate_command", new=AsyncMock(side_effect=_noop_generate_command)):
            await orchestrate_tool_execution(
                ToolCall(
                    name=OperatorToolName.RUN_COMMANDS,
                    args={"request": "echo hello", "guidelines": "prefer minimal output"},
                ),
                tool_executor=mock_tool_executor,
                investigation=sample_investigation,
                g8e_context=sample_g8e_context,
                g8ed_event_service=mock_event_service,
                request_settings=request_settings,
            )

        assert captured_args["request"] == "echo hello"
        assert captured_args["guidelines"] == "prefer minimal output"
        assert "command" in captured_args
        assert "execution_id" not in captured_args
        assert "_web_session_id" not in captured_args


# =============================================================================
# TEST: ToolCallResult structure
# =============================================================================

class TestToolCallResultStructure:

    async def test_result_is_tool_call_result_model(self, mock_tool_executor, sample_investigation, sample_g8e_context, request_settings, mock_event_service):
        _mock_executor_success(mock_tool_executor)

        with patch.object(agent_tool_loop_module, "generate_command", new=AsyncMock(side_effect=_noop_generate_command)):
            result = await orchestrate_tool_execution(
                ToolCall(name=OperatorToolName.RUN_COMMANDS, args={"request": "list files"}),
                tool_executor=mock_tool_executor,
                investigation=sample_investigation,
                g8e_context=sample_g8e_context,
                g8ed_event_service=mock_event_service,
                request_settings=request_settings,
            )

        assert isinstance(result, ToolCallResult)

    async def test_call_info_is_stream_from_model_chunk_data(self, mock_tool_executor, sample_investigation, sample_g8e_context, request_settings, mock_event_service):
        _mock_executor_success(mock_tool_executor)

        with patch.object(agent_tool_loop_module, "generate_command", new=AsyncMock(side_effect=_noop_generate_command)):
            result = await orchestrate_tool_execution(
                ToolCall(name=OperatorToolName.RUN_COMMANDS, args={"request": "list files"}),
                tool_executor=mock_tool_executor,
                investigation=sample_investigation,
                g8e_context=sample_g8e_context,
                g8ed_event_service=mock_event_service,
                request_settings=request_settings,
            )

        assert isinstance(result.call_info, StreamChunkData)
        assert isinstance(result.result_info, StreamChunkData)

    async def test_tool_name_propagated_to_result(self, mock_tool_executor, sample_investigation, sample_g8e_context, request_settings, mock_event_service):
        _mock_executor_success(mock_tool_executor)

        with patch.object(agent_tool_loop_module, "generate_command", new=AsyncMock(side_effect=_noop_generate_command)):
            result = await orchestrate_tool_execution(
                ToolCall(name=OperatorToolName.RUN_COMMANDS, args={"request": "show working directory"}),
                tool_executor=mock_tool_executor,
                investigation=sample_investigation,
                g8e_context=sample_g8e_context,
                g8ed_event_service=mock_event_service,
                request_settings=request_settings,
            )

        assert result.tool_name == OperatorToolName.RUN_COMMANDS
        assert result.call_info.tool_name == OperatorToolName.RUN_COMMANDS

    async def test_result_carries_raw_handler_result(self, mock_tool_executor, sample_investigation, sample_g8e_context, request_settings, mock_event_service):
        raw = CommandExecutionResult(success=True, output="hello from cmd", exit_code=0, command_executed="echo hello")
        mock_tool_executor.execute_tool_call = AsyncMock(return_value=raw)

        with patch.object(agent_tool_loop_module, "generate_command", new=AsyncMock(side_effect=_noop_generate_command)):
            result = await orchestrate_tool_execution(
                ToolCall(name=OperatorToolName.RUN_COMMANDS, args={"request": "echo hello"}),
                tool_executor=mock_tool_executor,
                investigation=sample_investigation,
                g8e_context=sample_g8e_context,
                g8ed_event_service=mock_event_service,
                request_settings=request_settings,
            )

        assert result.result is raw
        assert result.result.output == "hello from cmd"

    async def test_result_info_success_reflects_handler_success(self, mock_tool_executor, sample_investigation, sample_g8e_context, request_settings, mock_event_service):
        _mock_executor_success(mock_tool_executor)

        with patch.object(agent_tool_loop_module, "generate_command", new=AsyncMock(side_effect=_noop_generate_command)):
            result = await orchestrate_tool_execution(
                ToolCall(name=OperatorToolName.RUN_COMMANDS, args={"request": "list files"}),
                tool_executor=mock_tool_executor,
                investigation=sample_investigation,
                g8e_context=sample_g8e_context,
                g8ed_event_service=mock_event_service,
                request_settings=request_settings,
            )

        assert result.result_info.success is True
        assert result.result_info.error_type is None

    async def test_result_info_error_type_set_on_failure(self, mock_tool_executor, sample_investigation, sample_g8e_context, request_settings, mock_event_service):
        _mock_executor_failure(mock_tool_executor, error_type=CommandErrorType.EXECUTION_FAILED)

        with patch.object(agent_tool_loop_module, "generate_command", new=AsyncMock(side_effect=_noop_generate_command)):
            result = await orchestrate_tool_execution(
                ToolCall(name=OperatorToolName.RUN_COMMANDS, args={"request": "run a bad command"}),
                tool_executor=mock_tool_executor,
                investigation=sample_investigation,
                g8e_context=sample_g8e_context,
                g8ed_event_service=mock_event_service,
                request_settings=request_settings,
            )

        assert result.result_info.success is False
        assert result.result_info.error_type == CommandErrorType.EXECUTION_FAILED

    async def test_execution_id_consistent_in_call_info_and_result_info(self, mock_tool_executor, sample_investigation, sample_g8e_context, request_settings, mock_event_service):
        _mock_executor_success(mock_tool_executor)

        with patch.object(agent_tool_loop_module, "generate_command", new=AsyncMock(side_effect=_noop_generate_command)):
            result = await orchestrate_tool_execution(
                ToolCall(name=OperatorToolName.RUN_COMMANDS, args={"request": "show user id"}),
                tool_executor=mock_tool_executor,
                investigation=sample_investigation,
                g8e_context=sample_g8e_context,
                g8ed_event_service=mock_event_service,
                request_settings=request_settings,
            )

        assert result.call_info.execution_id == result.result_info.execution_id


    async def test_event_id_invariant_through_execution_threading(self, mock_tool_executor, sample_investigation, sample_g8e_context, request_settings):
        """Regression: event IDs must remain invariant through execution_id threading.
        
        This test verifies that when execution_id is threaded through the tool
        execution pipeline (Report 1), the event IDs generated for call_info
        and result_info events remain consistent and don't change mid-stream.
        """
        _mock_executor_success(mock_tool_executor)

        mock_event_service = AsyncMock()
        captured_events = []

        def _capture_event(event):
            captured_events.append(event)
            return "success"

        mock_event_service.publish = _capture_event
        mock_event_service.publish_investigation_event = _capture_event

        with patch.object(agent_tool_loop_module, "generate_command", new=AsyncMock(side_effect=_noop_generate_command)):
            result = await orchestrate_tool_execution(
                ToolCall(name=OperatorToolName.RUN_COMMANDS, args={"request": "echo test"}),
                tool_executor=mock_tool_executor,
                investigation=sample_investigation,
                g8e_context=sample_g8e_context,
                g8ed_event_service=mock_event_service,
                request_settings=request_settings,
            )

        # Verify execution_id is consistent across call_info and result_info
        assert result.call_info.execution_id == result.result_info.execution_id
        
        # Verify execution_id was used in event emission (if events were captured)
        # The exact event structure depends on the SSE event format, but the key
        # invariant is that the same execution_id appears in related events
        if captured_events:
            execution_ids_in_events = []
            for event in captured_events:
                if hasattr(event, 'execution_id'):
                    execution_ids_in_events.append(event.execution_id)
            
            # All events for this tool call should share the same execution_id
            if execution_ids_in_events:
                assert len(set(execution_ids_in_events)) == 1, (
                    f"Events for single tool call have different execution_ids: {execution_ids_in_events}"
                )
                assert execution_ids_in_events[0] == result.call_info.execution_id


# =============================================================================
# TEST: tribunal_result on ToolCallResult
# =============================================================================

class TestTribunalResultSurfaced:

    async def test_tribunal_result_present_on_noop(self, mock_tool_executor, sample_investigation, sample_g8e_context, request_settings, mock_event_service):
        _mock_executor_success(mock_tool_executor)

        with patch.object(agent_tool_loop_module, "generate_command", new=AsyncMock(side_effect=_noop_generate_command)):
            result = await orchestrate_tool_execution(
                ToolCall(name=OperatorToolName.RUN_COMMANDS, args={"request": "list files"}),
                tool_executor=mock_tool_executor,
                investigation=sample_investigation,
                g8e_context=sample_g8e_context,
                g8ed_event_service=mock_event_service,
                request_settings=request_settings,
            )

        assert result.tribunal_result is not None
        assert result.tribunal_result.request == "list files"
        assert result.tribunal_result.final_command == "list files"

    async def test_tribunal_result_carries_refined_command(self, mock_tool_executor, sample_investigation, sample_g8e_context, request_settings, mock_event_service):
        _mock_executor_success(mock_tool_executor)

        with patch.object(agent_tool_loop_module, "generate_command", new=AsyncMock(side_effect=lambda request, **kwargs: _refining_generate_command(request, refined="ls -lhR"))):
            result = await orchestrate_tool_execution(
                ToolCall(name=OperatorToolName.RUN_COMMANDS, args={"request": "list files recursively"}),
                tool_executor=mock_tool_executor,
                investigation=sample_investigation,
                g8e_context=sample_g8e_context,
                g8ed_event_service=mock_event_service,
                request_settings=request_settings,
            )

        assert result.tribunal_result is not None
        assert result.tribunal_result.request == "list files recursively"
        assert result.tribunal_result.final_command == "ls -lhR"
        from app.constants import CommandGenerationOutcome
        assert result.tribunal_result.outcome == CommandGenerationOutcome.CONSENSUS

    async def test_tribunal_result_none_for_non_command_tool(self, mock_tool_executor, sample_investigation, sample_g8e_context, request_settings, mock_event_service):
        result_raw = CommandExecutionResult(success=True, output="ok")
        mock_tool_executor.execute_tool_call = AsyncMock(return_value=result_raw)

        result = await orchestrate_tool_execution(
            ToolCall(name=OperatorToolName.FILE_READ, args={"file_path": "/etc/hosts"}),
            tool_executor=mock_tool_executor,
            investigation=sample_investigation,
            g8e_context=sample_g8e_context,
            g8ed_event_service=mock_event_service,
            request_settings=request_settings,
        )

        assert result.tribunal_result is None

    async def test_tribunal_result_none_for_non_operator_tool(self, mock_tool_executor, sample_investigation, sample_g8e_context, request_settings, mock_event_service):
        result_raw = CommandExecutionResult(success=True, output="ok")
        mock_tool_executor.execute_tool_call = AsyncMock(return_value=result_raw)

        result = await orchestrate_tool_execution(
            ToolCall(name=NON_OPERATOR_FUNCTION, args={}),
            tool_executor=mock_tool_executor,
            investigation=sample_investigation,
            g8e_context=sample_g8e_context,
            g8ed_event_service=mock_event_service,
            request_settings=request_settings,
        )

        assert result.tribunal_result is None

    async def test_tribunal_result_none_on_system_error(self, mock_tool_executor, sample_investigation, sample_g8e_context, request_settings, mock_event_service):
        _mock_executor_success(mock_tool_executor)

        exc = TribunalSystemError(
            pass_errors=["401 Unauthorized"],
            request="list files",
        )
        with patch.object(agent_tool_loop_module, "generate_command", new=AsyncMock(side_effect=exc)):
            result = await orchestrate_tool_execution(
                ToolCall(name=OperatorToolName.RUN_COMMANDS, args={"request": "list files"}),
                tool_executor=mock_tool_executor,
                investigation=sample_investigation,
                g8e_context=sample_g8e_context,
                g8ed_event_service=mock_event_service,
                request_settings=request_settings,
            )

        assert result.tribunal_result is None

    async def test_tribunal_result_none_when_request_missing(self, mock_tool_executor, sample_investigation, sample_g8e_context, request_settings, mock_event_service):
        """Empty args => empty request => Tribunal is skipped and tribunal_result is None."""
        _mock_executor_success(mock_tool_executor)

        with patch.object(agent_tool_loop_module, "generate_command", new=AsyncMock(side_effect=_noop_generate_command)):
            result = await orchestrate_tool_execution(
                ToolCall(name=OperatorToolName.RUN_COMMANDS, args={}),
                tool_executor=mock_tool_executor,
                investigation=sample_investigation,
                g8e_context=sample_g8e_context,
                g8ed_event_service=mock_event_service,
                request_settings=request_settings,
            )

        assert result.tribunal_result is None


# =============================================================================
# TEST: tool_name extraction
# =============================================================================

class TestToolNameExtraction:

    async def test_name_extracted_from_name_attribute(self, mock_tool_executor, sample_investigation, sample_g8e_context, request_settings, mock_event_service):
        result = CommandExecutionResult(success=True, output="ok")
        mock_tool_executor.execute_tool_call = AsyncMock(return_value=result)

        result = await orchestrate_tool_execution(
            ToolCall(name=NON_OPERATOR_FUNCTION, args={}),
            tool_executor=mock_tool_executor,
            investigation=sample_investigation,
            g8e_context=sample_g8e_context,
            g8ed_event_service=mock_event_service,
            request_settings=request_settings,
        )

        assert result.tool_name == NON_OPERATOR_FUNCTION

    async def test_name_none_uses_empty_string(self, mock_tool_executor, sample_investigation, sample_g8e_context, request_settings, mock_event_service):
        """When .name is None, tool_name is an empty string."""
        result = CommandExecutionResult(success=True, output="ok")
        mock_tool_executor.execute_tool_call = AsyncMock(return_value=result)

        class NoNameToolCall:
            name = None
            args = {}

        result = await orchestrate_tool_execution(
            NoNameToolCall(),
            tool_executor=mock_tool_executor,
            investigation=sample_investigation,
            g8e_context=sample_g8e_context,
            g8ed_event_service=mock_event_service,
            request_settings=request_settings,
        )

        assert result.tool_name == ""


# =============================================================================
# TEST: Tribunal command refinement
# =============================================================================

class TestTribunalRefinement:

    async def test_generate_command_called_for_run_commands(self, mock_tool_executor, sample_investigation, sample_g8e_context, request_settings, mock_event_service):
        _mock_executor_success(mock_tool_executor)

        mock_generate = AsyncMock(side_effect=_noop_generate_command)
        with patch.object(agent_tool_loop_module, "generate_command", new=mock_generate):
            await orchestrate_tool_execution(
                ToolCall(
                    name=OperatorToolName.RUN_COMMANDS,
                    args={"request": "list files in long format", "guidelines": "favour -la flags"},
                ),
                tool_executor=mock_tool_executor,
                investigation=sample_investigation,
                g8e_context=sample_g8e_context,
                g8ed_event_service=mock_event_service,
                request_settings=request_settings,
            )

        mock_generate.assert_awaited_once()
        call_kwargs = mock_generate.call_args.kwargs
        assert call_kwargs["request"] == "list files in long format"
        assert call_kwargs["guidelines"] == "favour -la flags"

    async def test_generate_command_not_called_for_file_read(self, mock_tool_executor, sample_investigation, sample_g8e_context, request_settings, mock_event_service):
        _mock_executor_success(mock_tool_executor)

        mock_generate = AsyncMock(side_effect=_noop_generate_command)
        with patch.object(agent_tool_loop_module, "generate_command", new=mock_generate):
            await orchestrate_tool_execution(
                ToolCall(
                    name=OperatorToolName.FILE_READ,
                    args={"file_path": "/etc/hosts"},
                ),
                tool_executor=mock_tool_executor,
                investigation=sample_investigation,
                g8e_context=sample_g8e_context,
                g8ed_event_service=mock_event_service,
                request_settings=request_settings,
            )

        mock_generate.assert_not_awaited()

    async def test_refined_command_replaces_original_in_handler_call(self, mock_tool_executor, sample_investigation, sample_g8e_context, request_settings, mock_event_service):
        captured_args = {}

        async def capture_call(tool_name, tool_args_with_id, investigation, g8e_context, **kwargs):
            nonlocal captured_args
            captured_args = tool_args_with_id
            return CommandExecutionResult(success=True, output="ok")

        mock_tool_executor.execute_tool_call = capture_call

        with patch.object(agent_tool_loop_module, "generate_command", new=AsyncMock(side_effect=lambda request, **kwargs: _refining_generate_command(request, refined="df -h --output=source,size,avail"))):
            await orchestrate_tool_execution(
                ToolCall(
                    name=OperatorToolName.RUN_COMMANDS,
                    args={"request": "check disk usage", "guidelines": "human-readable"},
                ),
                tool_executor=mock_tool_executor,
                investigation=sample_investigation,
                g8e_context=sample_g8e_context,
                g8ed_event_service=mock_event_service,
                request_settings=request_settings,
            )

        # The Tribunal-produced final_command is what the executor sees, not
        # the caller's natural-language request.
        assert captured_args["command"] == "df -h --output=source,size,avail"
        assert captured_args["request"] == "check disk usage"
        assert captured_args["guidelines"] == "human-readable"

    async def test_noop_tribunal_passes_request_as_command(self, mock_tool_executor, sample_investigation, sample_g8e_context, request_settings, mock_event_service):
        """The noop mock Tribunal returns request verbatim as final_command.

        Verifies that whatever the Tribunal produces is what the executor sees,
        even when the noop mock does not refine.
        """
        captured_args = {}

        async def capture_call(tool_name, tool_args_with_id, investigation, g8e_context, **kwargs):
            nonlocal captured_args
            captured_args = tool_args_with_id
            return CommandExecutionResult(success=True, output="ok")

        mock_tool_executor.execute_tool_call = capture_call

        with patch.object(agent_tool_loop_module, "generate_command", new=AsyncMock(side_effect=_noop_generate_command)):
            await orchestrate_tool_execution(
                ToolCall(
                    name=OperatorToolName.RUN_COMMANDS,
                    args={"request": "uptime", "guidelines": "show uptime"},
                ),
                tool_executor=mock_tool_executor,
                investigation=sample_investigation,
                g8e_context=sample_g8e_context,
                g8ed_event_service=mock_event_service,
                request_settings=request_settings,
            )

        assert captured_args["command"] == "uptime"

    async def test_generate_command_receives_operator_os_context(self, mock_tool_executor, unique_user_id, unique_investigation_id, unique_case_id, unique_operator_id, unique_session_id, unique_web_session_id, request_settings, mock_event_service):
        """os_name and shell must come from the investigation's operator context."""
        op = OperatorDocument(
            id=unique_operator_id,
            user_id=unique_user_id,
            operator_session_id=unique_session_id,
            operator_type=OperatorType.SYSTEM,
            status=OperatorStatus.BOUND,
            system_info=OperatorSystemInfo(os="ubuntu", hostname="srv-01"),
        )
        investigation = build_enriched_context(
            investigation_id=unique_investigation_id,
            case_id=unique_case_id,
            user_id=unique_user_id,
            operator_documents=[op],
        )

        _mock_executor_success(mock_tool_executor)

        mock_generate = AsyncMock(side_effect=_noop_generate_command)
        with patch.object(agent_tool_loop_module, "generate_command", new=mock_generate):
            await orchestrate_tool_execution(
                ToolCall(
                    name=OperatorToolName.RUN_COMMANDS,
                    args={"request": "list files", "guidelines": "detailed view"},
                ),
                tool_executor=mock_tool_executor,
                investigation=investigation,
                g8e_context=build_g8e_http_context(
                    user_id=unique_user_id,
                    case_id=unique_case_id,
                    investigation_id=unique_investigation_id,
                    web_session_id=unique_web_session_id,
                ),
                g8ed_event_service=mock_event_service,
                request_settings=request_settings,
            )

        call_kwargs = mock_generate.call_args.kwargs
        op_ctx = call_kwargs["operator_context"]
        assert op_ctx is not None
        assert op_ctx.os == "ubuntu"
        assert op_ctx.operator_id == unique_operator_id

    async def test_generate_command_skipped_when_request_missing(self, mock_tool_executor, sample_investigation, sample_g8e_context, request_settings, mock_event_service):
        """Tribunal refinement must be skipped when there is no 'request' key in args.

        Sage is the sole writer of ``request``; an empty request means Sage
        had nothing to ask, so there is nothing for the Tribunal to produce.
        """
        _mock_executor_success(mock_tool_executor)

        mock_generate = AsyncMock(side_effect=_noop_generate_command)
        with patch.object(agent_tool_loop_module, "generate_command", new=mock_generate):
            await orchestrate_tool_execution(
                ToolCall(
                    name=OperatorToolName.RUN_COMMANDS,
                    args={},
                ),
                tool_executor=mock_tool_executor,
                investigation=sample_investigation,
                g8e_context=sample_g8e_context,
                g8ed_event_service=mock_event_service,
                request_settings=request_settings,
            )

        mock_generate.assert_not_awaited()


# =============================================================================
# TEST: TribunalSystemError halts execution
# =============================================================================

class TestTribunalSystemErrorHaltsExecution:

    async def test_system_error_returns_failed_tool_call_result(self, mock_tool_executor, sample_investigation, sample_g8e_context, request_settings, mock_event_service):
        """TribunalSystemError must produce a failed ToolCallResult, not propagate."""
        _mock_executor_success(mock_tool_executor)

        exc = TribunalSystemError(
            pass_errors=["401 Unauthorized", "Connection refused"],
            request="delete the filesystem root",
        )
        with patch.object(agent_tool_loop_module, "generate_command", new=AsyncMock(side_effect=exc)):
            result = await orchestrate_tool_execution(
                ToolCall(name=OperatorToolName.RUN_COMMANDS, args={"request": "delete the filesystem root"}),
                tool_executor=mock_tool_executor,
                investigation=sample_investigation,
                g8e_context=sample_g8e_context,
                g8ed_event_service=mock_event_service,
                request_settings=request_settings,
            )

        assert isinstance(result, ToolCallResult)
        assert result.result_info.success is False
        assert result.result.success is False
        assert "401 Unauthorized" in result.result.error
        assert "Connection refused" in result.result.error
        assert result.result.error_type == CommandErrorType.EXECUTION_ERROR

    async def test_system_error_prevents_tool_executor_call(self, mock_tool_executor, sample_investigation, sample_g8e_context, request_settings, mock_event_service):
        """When TribunalSystemError fires, the underlying executor must NOT be called."""
        mock_tool_executor.execute_tool_call = AsyncMock()

        exc = TribunalSystemError(
            pass_errors=["401 Unauthorized"],
            request="list files",
        )
        with patch.object(agent_tool_loop_module, "generate_command", new=AsyncMock(side_effect=exc)):
            await orchestrate_tool_execution(
                ToolCall(name=OperatorToolName.RUN_COMMANDS, args={"request": "list files"}),
                tool_executor=mock_tool_executor,
                investigation=sample_investigation,
                g8e_context=sample_g8e_context,
                g8ed_event_service=mock_event_service,
                request_settings=request_settings,
            )

        mock_tool_executor.execute_tool_call.assert_not_awaited()




# =============================================================================
# TEST: target_operator resolution in Tribunal
# =============================================================================

class TestTargetOperatorResolution:

    def setup_method(self):
        """Setup multi-operator investigation for target operator tests."""
        self.multi_op_investigation = build_enriched_context(
            investigation_id="inv-multi-test",
            case_id="case-multi-test", 
            user_id="user-multi-test",
            operator_documents=[
                OperatorDocument(
                    id="op-linux",
                    operator_session_id="session-linux",
                    status=OperatorStatus.AVAILABLE,
                    operator_type=OperatorType.SYSTEM,
                    system_info=OperatorSystemInfo(
                        hostname="linux-host",
                        os="linux",
                        architecture="amd64",
                        shell="bash",
                        working_directory="/home/g8e",
                    ),
                    user_id="user-multi-test",
                    bound_web_session_id="web-001",
                ),
                OperatorDocument(
                    id="op-ubuntu",
                    operator_session_id="session-ubuntu", 
                    status=OperatorStatus.AVAILABLE,
                    operator_type=OperatorType.SYSTEM,
                    current_hostname="ubuntu-host",  # Use current_hostname for resolution
                    system_info=OperatorSystemInfo(
                        hostname="ubuntu-host",
                        os="ubuntu",
                        architecture="amd64",
                        shell="bash",
                    ),
                    user_id="user-multi-test",
                    bound_web_session_id="web-001",
                ),
            ],
        )

        self.multi_op_g8e_context = build_g8e_http_context(
            user_id="user-multi-test",
            case_id="case-multi-test",
            investigation_id="inv-multi-test",
            web_session_id="web-001",
            bound_operators=[
                # Use factory to build real BoundOperator models
                build_bound_operator(operator_id="op-linux", operator_session_id="session-linux"),
                build_bound_operator(operator_id="op-ubuntu", operator_session_id="session-ubuntu"),
            ],
        )

    async def test_target_operator_by_id_linux(self, mock_tool_executor, request_settings, mock_event_service):
        """Test Tribunal uses Linux operator context when targeting by operator_id."""
        _mock_executor_success(mock_tool_executor)

        mock_generate = AsyncMock(side_effect=_noop_generate_command)
        with patch.object(agent_tool_loop_module, "generate_command", new=mock_generate):
            await orchestrate_tool_execution(
                ToolCall(
                    name=OperatorToolName.RUN_COMMANDS,
                    args={"request": "list files", "target_operator": "op-linux"},
                ),
                tool_executor=mock_tool_executor,
                investigation=self.multi_op_investigation,
                g8e_context=self.multi_op_g8e_context,
                g8ed_event_service=mock_event_service,
                request_settings=request_settings,
            )

        # Verify Tribunal received Linux operator's OS context
        call_kwargs = mock_generate.call_args.kwargs
        op_ctx = call_kwargs["operator_context"]
        assert op_ctx is not None
        assert op_ctx.operator_id == "op-linux"
        assert op_ctx.os == "linux"

    async def test_target_operator_by_id_ubuntu(self, mock_tool_executor, request_settings, mock_event_service):
        """Test Tribunal uses Ubuntu operator context when targeting by operator_id."""
        _mock_executor_success(mock_tool_executor)

        mock_generate = AsyncMock(side_effect=_noop_generate_command)
        with patch.object(agent_tool_loop_module, "generate_command", new=mock_generate):
            await orchestrate_tool_execution(
                ToolCall(
                    name=OperatorToolName.RUN_COMMANDS,
                    args={"request": "list files", "target_operator": "op-ubuntu"},
                ),
                tool_executor=mock_tool_executor,
                investigation=self.multi_op_investigation,
                g8e_context=self.multi_op_g8e_context,
                g8ed_event_service=mock_event_service,
                request_settings=request_settings,
            )

        # Verify Tribunal received Ubuntu operator's OS context
        call_kwargs = mock_generate.call_args.kwargs
        op_ctx = call_kwargs["operator_context"]
        assert op_ctx is not None
        assert op_ctx.operator_id == "op-ubuntu"
        assert op_ctx.os == "ubuntu"

    async def test_target_operator_by_hostname(self, mock_tool_executor, request_settings, mock_event_service):
        """Test Tribunal resolves operator by hostname."""
        _mock_executor_success(mock_tool_executor)

        mock_generate = AsyncMock(side_effect=_noop_generate_command)
        with patch.object(agent_tool_loop_module, "generate_command", new=mock_generate):
            await orchestrate_tool_execution(
                ToolCall(
                    name=OperatorToolName.RUN_COMMANDS,
                    args={"request": "list files", "target_operator": "ubuntu-host"},
                ),
                tool_executor=mock_tool_executor,
                investigation=self.multi_op_investigation,
                g8e_context=self.multi_op_g8e_context,
                g8ed_event_service=mock_event_service,
                request_settings=request_settings,
            )

        # Should resolve to ubuntu operator by hostname
        call_kwargs = mock_generate.call_args.kwargs
        op_ctx = call_kwargs["operator_context"]
        assert op_ctx is not None
        assert op_ctx.operator_id == "op-ubuntu"
        assert op_ctx.os == "ubuntu"

    async def test_target_operator_by_index(self, mock_tool_executor, request_settings, mock_event_service):
        """Test Tribunal resolves operator by index."""
        _mock_executor_success(mock_tool_executor)

        mock_generate = AsyncMock(side_effect=_noop_generate_command)
        with patch.object(agent_tool_loop_module, "generate_command", new=mock_generate):
            await orchestrate_tool_execution(
                ToolCall(
                    name=OperatorToolName.RUN_COMMANDS,
                    args={"request": "list files", "target_operator": "1"},
                ),
                tool_executor=mock_tool_executor,
                investigation=self.multi_op_investigation,
                g8e_context=self.multi_op_g8e_context,
                g8ed_event_service=mock_event_service,
                request_settings=request_settings,
            )

        # Index 1 should resolve to second operator (ubuntu)
        call_kwargs = mock_generate.call_args.kwargs
        op_ctx = call_kwargs["operator_context"]
        assert op_ctx is not None
        assert op_ctx.operator_id == "op-ubuntu"
        assert op_ctx.os == "ubuntu"

    async def test_target_operator_fallback_to_first(self, mock_tool_executor, request_settings, mock_event_service):
        """Test Tribunal falls back to first operator when target not found."""
        _mock_executor_success(mock_tool_executor)

        mock_generate = AsyncMock(side_effect=_noop_generate_command)
        with patch.object(agent_tool_loop_module, "generate_command", new=mock_generate):
            await orchestrate_tool_execution(
                ToolCall(
                    name=OperatorToolName.RUN_COMMANDS,
                    args={"request": "list files", "target_operator": "nonexistent-operator"},
                ),
                tool_executor=mock_tool_executor,
                investigation=self.multi_op_investigation,
                g8e_context=self.multi_op_g8e_context,
                g8ed_event_service=mock_event_service,
                request_settings=request_settings,
            )

        # Should fall back to first operator (linux)
        call_kwargs = mock_generate.call_args.kwargs
        op_ctx = call_kwargs["operator_context"]
        assert op_ctx is not None
        assert op_ctx.operator_id == "op-linux"
        assert op_ctx.os == "linux"

    async def test_no_target_operator_uses_first(self, mock_tool_executor, request_settings, mock_event_service):
        """Test that no target_operator defaults to first operator (backward compatibility)."""
        _mock_executor_success(mock_tool_executor)

        mock_generate = AsyncMock(side_effect=_noop_generate_command)
        with patch.object(agent_tool_loop_module, "generate_command", new=mock_generate):
            await orchestrate_tool_execution(
                ToolCall(
                    name=OperatorToolName.RUN_COMMANDS,
                    args={"request": "list files"},  # No target_operator
                ),
                tool_executor=mock_tool_executor,
                investigation=self.multi_op_investigation,
                g8e_context=self.multi_op_g8e_context,
                g8ed_event_service=mock_event_service,
                request_settings=request_settings,
            )

        # Should use first operator (linux) by default
        call_kwargs = mock_generate.call_args.kwargs
        op_ctx = call_kwargs["operator_context"]
        assert op_ctx is not None
        assert op_ctx.operator_id == "op-linux"
        assert op_ctx.os == "linux"
