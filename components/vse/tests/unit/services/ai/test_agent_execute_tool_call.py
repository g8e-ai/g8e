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
    ./scripts/testing/run_tests.sh vse -- tests/unit/services/ai/test_agent_orchestrate_tool_execution.py
"""

from unittest.mock import AsyncMock, MagicMock, patch

import pytest

import app.services.ai.agent_tool_loop as agent_tool_loop_module
from app.constants import NEW_CASE_ID, CommandErrorType, OperatorStatus, OperatorToolName, OperatorType
from app.models.agents.tribunal import TribunalSystemError
from app.llm.llm_types import ToolCall
from app.models.agent import StreamChunkData
from app.models.operators import OperatorDocument, OperatorSystemInfo
from app.services.ai.agent_tool_loop import ToolCallResult
from app.models.tool_results import CommandExecutionResult
from app.models.settings import LLMSettings, VSEUserSettings
from app.services.ai.agent_tool_loop import orchestrate_tool_execution
from app.services.ai.tool_service import AIToolService
from tests.fakes.factories import (
    build_vso_http_context,
    build_enriched_context as create_enriched_context,
    build_bound_operator,
)

pytestmark = [pytest.mark.unit, pytest.mark.asyncio(loop_scope="session")]


# =============================================================================
# HELPERS
# =============================================================================

def _make_tool_executor() -> MagicMock:
    executor = MagicMock(spec=AIToolService)
    executor.web_search_provider = None
    
    # Mock OperatorCommandService with required _settings
    mock_exec_svc = MagicMock()
    mock_exec_svc._settings = MagicMock()
    executor.operator_command_service = mock_exec_svc
    
    return executor


def _mock_executor_success(executor: MagicMock, output: str = "ok") -> None:
    result = CommandExecutionResult(success=True, output=output, exit_code=0, command_executed="mock")
    executor.execute_tool_call = AsyncMock(return_value=result)


def _mock_executor_failure(executor: MagicMock, error_type: CommandErrorType = CommandErrorType.EXECUTION_ERROR) -> None:
    result = CommandExecutionResult(success=False, error="failed", error_type=error_type)
    executor.execute_tool_call = AsyncMock(return_value=result)


def _noop_generate_command(original_command: str, **_kwargs):
    from app.services.ai.command_generator import CommandGenerationOutcome, CommandGenerationResult
    return CommandGenerationResult(
        original_command=original_command,
        final_command=original_command,
        outcome=CommandGenerationOutcome.FALLBACK,
    )


def _refining_generate_command(original_command: str, refined: str, **_kwargs):
    from app.services.ai.command_generator import CommandGenerationOutcome, CommandGenerationResult
    return CommandGenerationResult(
        original_command=original_command,
        final_command=refined,
        outcome=CommandGenerationOutcome.CONSENSUS,
    )


NON_OPERATOR_FUNCTION = "some_unknown_tool_that_is_not_registered"

REQUEST_SETTINGS = VSEUserSettings(llm=LLMSettings())


INVESTIGATION = create_enriched_context(
    investigation_id="inv-exec-test",
    case_id="case-exec-test",
    user_id="user-exec-test",
    operator_documents=[
        OperatorDocument(
            operator_id="op-1",
            operator_session_id="session-op-1",
            status=OperatorStatus.AVAILABLE,
            operator_type=OperatorType.SYSTEM,
            system_info=OperatorSystemInfo(
                hostname="op-1-host",
                os="linux",
                architecture="amd64",
                cpu_count=2,
                memory_mb=4096,
            ),
            user_id="user-exec-test",
            web_session_id="web-001",
        )
    ],
)

VSO_CONTEXT = build_vso_http_context(
    user_id="user-exec-test",
    case_id="case-exec-test",
    investigation_id="inv-exec-test",
    web_session_id="web-001",
)


# =============================================================================
# TEST: execution_id generation
# =============================================================================

class TestExecutionIdGeneration:

    async def test_operator_tool_getsexecution_id(self):
        executor = _make_tool_executor()
        _mock_executor_success(executor)

        with patch.object(agent_tool_loop_module, "generate_command", new=AsyncMock(side_effect=_noop_generate_command)):
            result = await orchestrate_tool_execution(
                ToolCall(name=OperatorToolName.RUN_COMMANDS, args={"command": "ls"}),
                tool_executor=executor,
                investigation=INVESTIGATION,
                vso_context=VSO_CONTEXT,
                vsod_event_service=AsyncMock(),
                request_settings=REQUEST_SETTINGS,
            )

        assert result.call_info.execution_id is not None

    async def testexecution_id_unique_per_call(self):
        executor = _make_tool_executor()
        _mock_executor_success(executor)

        ids = []
        with patch.object(agent_tool_loop_module, "generate_command", new=AsyncMock(side_effect=_noop_generate_command)):
            for _ in range(3):
                result = await orchestrate_tool_execution(
                    ToolCall(name=OperatorToolName.RUN_COMMANDS, args={"command": "whoami"}),
                    tool_executor=executor,
                    investigation=INVESTIGATION,
                    vso_context=VSO_CONTEXT,
                    web_session_id="web-001",
                    investigation_id="inv-001",
                    vsod_event_service=AsyncMock(),
                    request_settings=REQUEST_SETTINGS,
                )
                ids.append(result.call_info.execution_id)

        assert len(set(ids)) == len(ids), "execution_ids must be unique across calls"

    async def test_non_operator_tool_has_noexecution_id(self):
        executor = _make_tool_executor()
        result = CommandExecutionResult(success=True, output="results")
        executor.execute_tool_call = AsyncMock(return_value=result)

        result = await orchestrate_tool_execution(
            ToolCall(name=NON_OPERATOR_FUNCTION, args={}),
            tool_executor=executor,
            investigation=INVESTIGATION,
            vso_context=VSO_CONTEXT,
            vsod_event_service=AsyncMock(),
            request_settings=REQUEST_SETTINGS,
        )

        assert result.call_info.execution_id is None

    async def testexecution_id_format_matches_expected_pattern(self):
        """execution_id must be 'exec_<12hex>_<timestamp_int>'."""
        import re
        executor = _make_tool_executor()
        _mock_executor_success(executor)

        with patch.object(agent_tool_loop_module, "generate_command", new=AsyncMock(side_effect=_noop_generate_command)):
            result = await orchestrate_tool_execution(
                ToolCall(name=OperatorToolName.RUN_COMMANDS, args={"command": "id"}),
                tool_executor=executor,
                investigation=INVESTIGATION,
                vso_context=VSO_CONTEXT,
                vsod_event_service=AsyncMock(),
                request_settings=REQUEST_SETTINGS,
            )

        pattern = r"^exec_[0-9a-f]{12}_\d+$"
        assert re.match(pattern, result.call_info.execution_id), (
            f"execution_id '{result.call_info.execution_id}' does not match expected format"
        )


# =============================================================================
# TEST: operator function detection
# =============================================================================

class TestOperatorToolDetection:

    async def test_run_commands_detected_as_operator_tool(self):
        executor = _make_tool_executor()
        _mock_executor_success(executor)

        with patch.object(agent_tool_loop_module, "generate_command", new=AsyncMock(side_effect=_noop_generate_command)):
            result = await orchestrate_tool_execution(
                ToolCall(name=OperatorToolName.RUN_COMMANDS, args={"command": "ls"}),
                tool_executor=executor,
                investigation=INVESTIGATION,
                vso_context=VSO_CONTEXT,
                vsod_event_service=AsyncMock(),
                request_settings=REQUEST_SETTINGS,
            )

        assert result.call_info.is_operator_tool is True

    async def test_file_read_detected_as_operator_tool(self):
        executor = _make_tool_executor()
        _mock_executor_success(executor)

        result = await orchestrate_tool_execution(
            ToolCall(name=OperatorToolName.FILE_READ, args={"file_path": "/etc/hosts"}),
            tool_executor=executor,
            investigation=INVESTIGATION,
            vso_context=VSO_CONTEXT,
            vsod_event_service=AsyncMock(),
            request_settings=REQUEST_SETTINGS,
        )

        assert result.call_info.is_operator_tool is True

    async def test_search_web_is_operator_tool(self):
        """search_web is OperatorToolName.G8E_SEARCH_WEB and must be detected as such."""
        executor = _make_tool_executor()
        result = CommandExecutionResult(success=True, output="results")
        executor.execute_tool_call = AsyncMock(return_value=result)

        result = await orchestrate_tool_execution(
            ToolCall(name=OperatorToolName.G8E_SEARCH_WEB, args={"query": "test"}),
            tool_executor=executor,
            investigation=INVESTIGATION,
            vso_context=VSO_CONTEXT,
            vsod_event_service=AsyncMock(),
            request_settings=REQUEST_SETTINGS,
        )

        assert result.call_info.is_operator_tool is True

    async def test_unregistered_tool_not_operator_tool(self):
        executor = _make_tool_executor()
        result = CommandExecutionResult(success=True, output="ok")
        executor.execute_tool_call = AsyncMock(return_value=result)

        result = await orchestrate_tool_execution(
            ToolCall(name=NON_OPERATOR_FUNCTION, args={}),
            tool_executor=executor,
            investigation=INVESTIGATION,
            vso_context=VSO_CONTEXT,
            vsod_event_service=AsyncMock(),
            request_settings=REQUEST_SETTINGS,
        )

        assert result.call_info.is_operator_tool is False


# =============================================================================
# TEST: internal field injection into tool_args_with_id
# =============================================================================

class TestToolArgsInjection:

    async def testexecution_id_injected_for_operator_tool(self):
        executor = _make_tool_executor()
        _mock_executor_success(executor)
        captured_args = {}

        async def capture_call(tool_name, tool_args_with_id, investigation, vso_context, **kwargs):
            nonlocal captured_args
            captured_args = tool_args_with_id
            return CommandExecutionResult(success=True, output="ok")

        executor.execute_tool_call = AsyncMock(side_effect=capture_call)

        with patch.object(agent_tool_loop_module, "generate_command", new=AsyncMock(side_effect=_noop_generate_command)):
            await orchestrate_tool_execution(
                ToolCall(name=OperatorToolName.RUN_COMMANDS, args={"command": "df -h"}),
                tool_executor=executor,
                investigation=INVESTIGATION,
                vso_context=VSO_CONTEXT,
                vsod_event_service=AsyncMock(),
                request_settings=REQUEST_SETTINGS,
            )

        assert "execution_id" in captured_args
        assert captured_args["execution_id"].startswith("exec_")

    async def test_web_session_id_injected_for_operator_tool(self):
        executor = _make_tool_executor()
        captured_args = {}

        async def capture_call(tool_name, tool_args_with_id, investigation, vso_context, **kwargs):
            nonlocal captured_args
            captured_args = tool_args_with_id
            return CommandExecutionResult(success=True, output="ok")

        executor.execute_tool_call = AsyncMock(side_effect=capture_call)

        with patch.object(agent_tool_loop_module, "generate_command", new=AsyncMock(side_effect=_noop_generate_command)):
            await orchestrate_tool_execution(
                ToolCall(name=OperatorToolName.RUN_COMMANDS, args={"command": "uptime"}),
                tool_executor=executor,
                investigation=INVESTIGATION,
                vso_context=VSO_CONTEXT,
                vsod_event_service=AsyncMock(),
                request_settings=REQUEST_SETTINGS,
            )

        assert captured_args["_web_session_id"] == "web-001"

    async def test_internal_fields_not_injected_for_non_operator_tool(self):
        executor = _make_tool_executor()
        captured_args = {}

        async def capture_call(tool_name, tool_args_with_id, investigation, vso_context, **kwargs):
            nonlocal captured_args
            captured_args = tool_args_with_id
            return CommandExecutionResult(success=True, output="ok")

        executor.execute_tool_call = AsyncMock(side_effect=capture_call)

        await orchestrate_tool_execution(
            ToolCall(name=NON_OPERATOR_FUNCTION, args={"query": "linux memory usage"}),
            tool_executor=executor,
            investigation=INVESTIGATION,
            vso_context=VSO_CONTEXT,
            vsod_event_service=AsyncMock(),
            request_settings=REQUEST_SETTINGS,
        )

        assert "execution_id" not in captured_args

    async def test_original_args_preserved_alongside_injected_fields(self):
        executor = _make_tool_executor()
        captured_args = {}

        async def capture_call(tool_name, tool_args_with_id, investigation, vso_context, **kwargs):
            nonlocal captured_args
            captured_args = tool_args_with_id
            return CommandExecutionResult(success=True, output="ok")

        executor.execute_tool_call = AsyncMock(side_effect=capture_call)

        with patch.object(agent_tool_loop_module, "generate_command", new=AsyncMock(side_effect=_noop_generate_command)):
            await orchestrate_tool_execution(
                ToolCall(
                    name=OperatorToolName.RUN_COMMANDS,
                    args={"command": "echo hello", "justification": "checking output"},
                ),
                tool_executor=executor,
                investigation=INVESTIGATION,
                vso_context=VSO_CONTEXT,
                vsod_event_service=AsyncMock(),
                request_settings=REQUEST_SETTINGS,
            )

        assert captured_args["command"] == "echo hello"
        assert captured_args["justification"] == "checking output"
        assert "execution_id" in captured_args
        assert "_web_session_id" in captured_args


# =============================================================================
# TEST: ToolCallResult structure
# =============================================================================

class TestToolCallResultStructure:

    async def test_result_is_tool_call_result_model(self):
        executor = _make_tool_executor()
        _mock_executor_success(executor)

        with patch.object(agent_tool_loop_module, "generate_command", new=AsyncMock(side_effect=_noop_generate_command)):
            result = await orchestrate_tool_execution(
                ToolCall(name=OperatorToolName.RUN_COMMANDS, args={"command": "ls"}),
                tool_executor=executor,
                investigation=INVESTIGATION,
                vso_context=VSO_CONTEXT,
                vsod_event_service=AsyncMock(),
                request_settings=REQUEST_SETTINGS,
            )

        assert isinstance(result, ToolCallResult)

    async def test_call_info_is_stream_from_model_chunk_data(self):
        executor = _make_tool_executor()
        _mock_executor_success(executor)

        with patch.object(agent_tool_loop_module, "generate_command", new=AsyncMock(side_effect=_noop_generate_command)):
            result = await orchestrate_tool_execution(
                ToolCall(name=OperatorToolName.RUN_COMMANDS, args={"command": "ls"}),
                tool_executor=executor,
                investigation=INVESTIGATION,
                vso_context=VSO_CONTEXT,
                vsod_event_service=AsyncMock(),
                request_settings=REQUEST_SETTINGS,
            )

        assert isinstance(result.call_info, StreamChunkData)
        assert isinstance(result.result_info, StreamChunkData)

    async def test_tool_name_propagated_to_result(self):
        executor = _make_tool_executor()
        _mock_executor_success(executor)

        with patch.object(agent_tool_loop_module, "generate_command", new=AsyncMock(side_effect=_noop_generate_command)):
            result = await orchestrate_tool_execution(
                ToolCall(name=OperatorToolName.RUN_COMMANDS, args={"command": "pwd"}),
                tool_executor=executor,
                investigation=INVESTIGATION,
                vso_context=VSO_CONTEXT,
                vsod_event_service=AsyncMock(),
                request_settings=REQUEST_SETTINGS,
            )

        assert result.tool_name == OperatorToolName.RUN_COMMANDS
        assert result.call_info.tool_name == OperatorToolName.RUN_COMMANDS

    async def test_result_carries_raw_handler_result(self):
        executor = _make_tool_executor()
        raw = CommandExecutionResult(success=True, output="hello from cmd", exit_code=0, command_executed="echo hello")
        executor.execute_tool_call = AsyncMock(return_value=raw)

        with patch.object(agent_tool_loop_module, "generate_command", new=AsyncMock(side_effect=_noop_generate_command)):
            result = await orchestrate_tool_execution(
                ToolCall(name=OperatorToolName.RUN_COMMANDS, args={"command": "echo hello"}),
                tool_executor=executor,
                investigation=INVESTIGATION,
                vso_context=VSO_CONTEXT,
                vsod_event_service=AsyncMock(),
                request_settings=REQUEST_SETTINGS,
            )

        assert result.result is raw
        assert result.result.output == "hello from cmd"

    async def test_result_info_success_reflects_handler_success(self):
        executor = _make_tool_executor()
        _mock_executor_success(executor)

        with patch.object(agent_tool_loop_module, "generate_command", new=AsyncMock(side_effect=_noop_generate_command)):
            result = await orchestrate_tool_execution(
                ToolCall(name=OperatorToolName.RUN_COMMANDS, args={"command": "ls"}),
                tool_executor=executor,
                investigation=INVESTIGATION,
                vso_context=VSO_CONTEXT,
                vsod_event_service=AsyncMock(),
                request_settings=REQUEST_SETTINGS,
            )

        assert result.result_info.success is True
        assert result.result_info.error_type is None

    async def test_result_info_error_type_set_on_failure(self):
        executor = _make_tool_executor()
        _mock_executor_failure(executor, error_type=CommandErrorType.EXECUTION_FAILED)

        with patch.object(agent_tool_loop_module, "generate_command", new=AsyncMock(side_effect=_noop_generate_command)):
            result = await orchestrate_tool_execution(
                ToolCall(name=OperatorToolName.RUN_COMMANDS, args={"command": "bad"}),
                tool_executor=executor,
                investigation=INVESTIGATION,
                vso_context=VSO_CONTEXT,
                vsod_event_service=AsyncMock(),
                request_settings=REQUEST_SETTINGS,
            )

        assert result.result_info.success is False
        assert result.result_info.error_type == CommandErrorType.EXECUTION_FAILED

    async def testexecution_id_consistent_in_call_info_and_result_info(self):
        executor = _make_tool_executor()
        _mock_executor_success(executor)

        with patch.object(agent_tool_loop_module, "generate_command", new=AsyncMock(side_effect=_noop_generate_command)):
            result = await orchestrate_tool_execution(
                ToolCall(name=OperatorToolName.RUN_COMMANDS, args={"command": "id"}),
                tool_executor=executor,
                investigation=INVESTIGATION,
                vso_context=VSO_CONTEXT,
                vsod_event_service=AsyncMock(),
                request_settings=REQUEST_SETTINGS,
            )

        assert result.call_info.execution_id == result.result_info.execution_id


# =============================================================================
# TEST: tool_name extraction
# =============================================================================

class TestToolNameExtraction:

    async def test_name_extracted_from_name_attribute(self):
        executor = _make_tool_executor()
        result = CommandExecutionResult(success=True, output="ok")
        executor.execute_tool_call = AsyncMock(return_value=result)

        result = await orchestrate_tool_execution(
            ToolCall(name=NON_OPERATOR_FUNCTION, args={}),
            tool_executor=executor,
            investigation=INVESTIGATION,
            vso_context=VSO_CONTEXT,
            vsod_event_service=AsyncMock(),
            request_settings=REQUEST_SETTINGS,
        )

        assert result.tool_name == NON_OPERATOR_FUNCTION

    async def test_name_none_uses_empty_string(self):
        """When .name is None, tool_name is an empty string."""
        executor = _make_tool_executor()
        result = CommandExecutionResult(success=True, output="ok")
        executor.execute_tool_call = AsyncMock(return_value=result)

        class NoNameToolCall:
            name = None
            args = {}

        result = await orchestrate_tool_execution(
            NoNameToolCall(),
            tool_executor=executor,
            investigation=INVESTIGATION,
            vso_context=VSO_CONTEXT,
            vsod_event_service=AsyncMock(),
            request_settings=REQUEST_SETTINGS,
        )

        assert result.tool_name == ""


# =============================================================================
# TEST: Tribunal command refinement
# =============================================================================

class TestTribunalRefinement:

    async def test_generate_command_called_for_run_commands(self):
        executor = _make_tool_executor()
        _mock_executor_success(executor)

        mock_generate = AsyncMock(side_effect=_noop_generate_command)
        with patch.object(agent_tool_loop_module, "generate_command", new=mock_generate):
            await orchestrate_tool_execution(
                ToolCall(
                    name=OperatorToolName.RUN_COMMANDS,
                    args={"command": "ls -la", "justification": "list files"},
                ),
                tool_executor=executor,
                investigation=INVESTIGATION,
                vso_context=VSO_CONTEXT,
                vsod_event_service=AsyncMock(),
                request_settings=REQUEST_SETTINGS,
            )

        mock_generate.assert_awaited_once()
        call_kwargs = mock_generate.call_args.kwargs
        assert call_kwargs["original_command"] == "ls -la"
        assert call_kwargs["intent"] == "list files"

    async def test_generate_command_not_called_for_file_read(self):
        executor = _make_tool_executor()
        _mock_executor_success(executor)

        mock_generate = AsyncMock(side_effect=_noop_generate_command)
        with patch.object(agent_tool_loop_module, "generate_command", new=mock_generate):
            await orchestrate_tool_execution(
                ToolCall(
                    name=OperatorToolName.FILE_READ,
                    args={"file_path": "/etc/hosts"},
                ),
                tool_executor=executor,
                investigation=INVESTIGATION,
                vso_context=VSO_CONTEXT,
                vsod_event_service=AsyncMock(),
                request_settings=REQUEST_SETTINGS,
            )

        mock_generate.assert_not_awaited()

    async def test_refined_command_replaces_original_in_handler_call(self):
        executor = _make_tool_executor()
        captured_args = {}

        async def capture_call(tool_name, tool_args_with_id, investigation, vso_context, **kwargs):
            nonlocal captured_args
            captured_args = tool_args_with_id
            return CommandExecutionResult(success=True, output="ok")

        executor.execute_tool_call = capture_call

        async def refining_generate(original_command, **kwargs):
            return _refining_generate_command(original_command, refined="df -h --output=source,size,avail")

        with patch.object(agent_tool_loop_module, "generate_command", new=refining_generate):
            await orchestrate_tool_execution(
                ToolCall(
                    name=OperatorToolName.RUN_COMMANDS,
                    args={"command": "df -h", "justification": "check disk"},
                ),
                tool_executor=executor,
                investigation=INVESTIGATION,
                vso_context=VSO_CONTEXT,
                vsod_event_service=AsyncMock(),
                request_settings=REQUEST_SETTINGS,
            )

        assert captured_args["command"] == "df -h --output=source,size,avail"

    async def test_unchanged_command_not_replaced(self):
        executor = _make_tool_executor()
        captured_args = {}

        async def capture_call(tool_name, tool_args_with_id, investigation, vso_context, **kwargs):
            nonlocal captured_args
            captured_args = tool_args_with_id
            return CommandExecutionResult(success=True, output="ok")

        executor.execute_tool_call = capture_call

        with patch.object(agent_tool_loop_module, "generate_command", new=AsyncMock(side_effect=_noop_generate_command)):
            await orchestrate_tool_execution(
                ToolCall(
                    name=OperatorToolName.RUN_COMMANDS,
                    args={"command": "uptime", "justification": "check uptime"},
                ),
                tool_executor=executor,
                investigation=INVESTIGATION,
                vso_context=VSO_CONTEXT,
                vsod_event_service=AsyncMock(),
                request_settings=REQUEST_SETTINGS,
            )

        assert captured_args["command"] == "uptime"

    async def test_generate_command_receives_operator_os_context(self):
        """os_name and shell must come from the investigation's operator context."""
        from app.constants import OperatorType
        from app.models.operators import OperatorDocument, OperatorSystemInfo

        op = OperatorDocument(
            operator_id="op-linux",
            operator_session_id="session-op-linux",
            operator_type=OperatorType.SYSTEM,
            status=OperatorStatus.BOUND,
            system_info=OperatorSystemInfo(os="ubuntu", hostname="srv-01"),
        )
        investigation = create_enriched_context(
            investigation_id="inv-os-test",
            case_id="case-os-test",
            user_id="user-os-test",
            operator_documents=[op],
        )

        executor = _make_tool_executor()
        _mock_executor_success(executor)

        mock_generate = AsyncMock(side_effect=_noop_generate_command)
        with patch.object(agent_tool_loop_module, "generate_command", new=mock_generate):
            await orchestrate_tool_execution(
                ToolCall(
                    name=OperatorToolName.RUN_COMMANDS,
                    args={"command": "ls", "justification": "list files"},
                ),
                tool_executor=executor,
                investigation=investigation,
                vso_context=build_vso_http_context(
                    user_id="user-os-test",
                    case_id="case-os-test",
                    investigation_id="inv-os-test",
                ),
                vsod_event_service=AsyncMock(),
                request_settings=REQUEST_SETTINGS,
            )

        call_kwargs = mock_generate.call_args.kwargs
        assert call_kwargs["os_name"] == "ubuntu"

    async def test_generate_command_skipped_when_command_arg_missing(self):
        """Tribunal refinement must be skipped when there is no 'command' key in args."""
        executor = _make_tool_executor()
        _mock_executor_success(executor)

        mock_generate = AsyncMock(side_effect=_noop_generate_command)
        with patch.object(agent_tool_loop_module, "generate_command", new=mock_generate):
            await orchestrate_tool_execution(
                ToolCall(
                    name=OperatorToolName.RUN_COMMANDS,
                    args={},
                ),
                tool_executor=executor,
                investigation=INVESTIGATION,
                vso_context=VSO_CONTEXT,
                vsod_event_service=AsyncMock(),
                request_settings=REQUEST_SETTINGS,
            )

        mock_generate.assert_not_awaited()


# =============================================================================
# TEST: TribunalSystemError halts execution
# =============================================================================

class TestTribunalSystemErrorHaltsExecution:

    async def test_system_error_returns_failed_tool_call_result(self):
        """TribunalSystemError must produce a failed ToolCallResult, not propagate."""
        executor = _make_tool_executor()
        _mock_executor_success(executor)

        exc = TribunalSystemError(
            pass_errors=["401 Unauthorized", "Connection refused"],
            original_command="rm -rf /",
        )
        with patch.object(agent_tool_loop_module, "generate_command", new=AsyncMock(side_effect=exc)):
            result = await orchestrate_tool_execution(
                ToolCall(name=OperatorToolName.RUN_COMMANDS, args={"command": "rm -rf /"}),
                tool_executor=executor,
                investigation=INVESTIGATION,
                vso_context=VSO_CONTEXT,
                vsod_event_service=AsyncMock(),
                request_settings=REQUEST_SETTINGS,
            )

        assert isinstance(result, ToolCallResult)
        assert result.result_info.success is False
        assert result.result.success is False
        assert "401 Unauthorized" in result.result.error
        assert "Connection refused" in result.result.error
        assert result.result.error_type == CommandErrorType.EXECUTION_ERROR

    async def test_system_error_prevents_tool_executor_call(self):
        """When TribunalSystemError fires, the underlying executor must NOT be called."""
        executor = _make_tool_executor()
        executor.execute_tool_call = AsyncMock()

        exc = TribunalSystemError(
            pass_errors=["401 Unauthorized"],
            original_command="ls",
        )
        with patch.object(agent_tool_loop_module, "generate_command", new=AsyncMock(side_effect=exc)):
            await orchestrate_tool_execution(
                ToolCall(name=OperatorToolName.RUN_COMMANDS, args={"command": "ls"}),
                tool_executor=executor,
                investigation=INVESTIGATION,
                vso_context=VSO_CONTEXT,
                vsod_event_service=AsyncMock(),
                request_settings=REQUEST_SETTINGS,
            )

        executor.execute_tool_call.assert_not_awaited()




# =============================================================================
# TEST: target_operator resolution in Tribunal
# =============================================================================

class TestTargetOperatorResolution:

    def setup_method(self):
        """Setup multi-operator investigation for target operator tests."""
        self.multi_op_investigation = create_enriched_context(
            investigation_id="inv-multi-test",
            case_id="case-multi-test", 
            user_id="user-multi-test",
            operator_documents=[
                OperatorDocument(
                    operator_id="op-linux",
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
                    web_session_id="web-001",
                ),
                OperatorDocument(
                    operator_id="op-ubuntu",
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
                    web_session_id="web-001",
                ),
            ],
        )

        self.multi_op_vso_context = build_vso_http_context(
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

    async def test_target_operator_by_id_linux(self):
        """Test Tribunal uses Linux operator context when targeting by operator_id."""
        executor = _make_tool_executor()
        _mock_executor_success(executor)

        mock_generate = AsyncMock(side_effect=_noop_generate_command)
        with patch.object(agent_tool_loop_module, "generate_command", new=mock_generate):
            await orchestrate_tool_execution(
                ToolCall(
                    name=OperatorToolName.RUN_COMMANDS,
                    args={"command": "ls", "target_operator": "op-linux"},
                ),
                tool_executor=executor,
                investigation=self.multi_op_investigation,
                vso_context=self.multi_op_vso_context,
                vsod_event_service=AsyncMock(),
                request_settings=REQUEST_SETTINGS,
            )

        # Verify Tribunal received Linux operator's OS context
        call_kwargs = mock_generate.call_args.kwargs
        assert call_kwargs["os_name"] == "linux"
        assert call_kwargs["shell"] == "bash"
        assert call_kwargs["working_directory"] == "/"

    async def test_target_operator_by_id_ubuntu(self):
        """Test Tribunal uses Ubuntu operator context when targeting by operator_id."""
        executor = _make_tool_executor()
        _mock_executor_success(executor)

        mock_generate = AsyncMock(side_effect=_noop_generate_command)
        with patch.object(agent_tool_loop_module, "generate_command", new=mock_generate):
            await orchestrate_tool_execution(
                ToolCall(
                    name=OperatorToolName.RUN_COMMANDS,
                    args={"command": "ls", "target_operator": "op-ubuntu"},
                ),
                tool_executor=executor,
                investigation=self.multi_op_investigation,
                vso_context=self.multi_op_vso_context,
                vsod_event_service=AsyncMock(),
                request_settings=REQUEST_SETTINGS,
            )

        # Verify Tribunal received Ubuntu operator's OS context
        call_kwargs = mock_generate.call_args.kwargs
        assert call_kwargs["os_name"] == "ubuntu"
        assert call_kwargs["shell"] == "bash"
        assert call_kwargs["working_directory"] == "/"

    async def test_target_operator_by_hostname(self):
        """Test Tribunal resolves operator by hostname."""
        executor = _make_tool_executor()
        _mock_executor_success(executor)

        mock_generate = AsyncMock(side_effect=_noop_generate_command)
        with patch.object(agent_tool_loop_module, "generate_command", new=mock_generate):
            await orchestrate_tool_execution(
                ToolCall(
                    name=OperatorToolName.RUN_COMMANDS,
                    args={"command": "ls", "target_operator": "ubuntu-host"},
                ),
                tool_executor=executor,
                investigation=self.multi_op_investigation,
                vso_context=self.multi_op_vso_context,
                vsod_event_service=AsyncMock(),
                request_settings=REQUEST_SETTINGS,
            )

        # Should resolve to ubuntu operator by hostname
        call_kwargs = mock_generate.call_args.kwargs
        assert call_kwargs["os_name"] == "ubuntu"
        assert call_kwargs["working_directory"] == "/"

    async def test_target_operator_by_index(self):
        """Test Tribunal resolves operator by index."""
        executor = _make_tool_executor()
        _mock_executor_success(executor)

        mock_generate = AsyncMock(side_effect=_noop_generate_command)
        with patch.object(agent_tool_loop_module, "generate_command", new=mock_generate):
            await orchestrate_tool_execution(
                ToolCall(
                    name=OperatorToolName.RUN_COMMANDS,
                    args={"command": "ls", "target_operator": "1"},
                ),
                tool_executor=executor,
                investigation=self.multi_op_investigation,
                vso_context=self.multi_op_vso_context,
                vsod_event_service=AsyncMock(),
                request_settings=REQUEST_SETTINGS,
            )

        # Index 1 should resolve to second operator (ubuntu)
        call_kwargs = mock_generate.call_args.kwargs
        assert call_kwargs["os_name"] == "ubuntu"
        assert call_kwargs["working_directory"] == "/"

    async def test_target_operator_fallback_to_first(self):
        """Test Tribunal falls back to first operator when target not found."""
        executor = _make_tool_executor()
        _mock_executor_success(executor)

        mock_generate = AsyncMock(side_effect=_noop_generate_command)
        with patch.object(agent_tool_loop_module, "generate_command", new=mock_generate):
            await orchestrate_tool_execution(
                ToolCall(
                    name=OperatorToolName.RUN_COMMANDS,
                    args={"command": "ls", "target_operator": "nonexistent-operator"},
                ),
                tool_executor=executor,
                investigation=self.multi_op_investigation,
                vso_context=self.multi_op_vso_context,
                vsod_event_service=AsyncMock(),
                request_settings=REQUEST_SETTINGS,
            )

        # Should fall back to first operator (linux)
        call_kwargs = mock_generate.call_args.kwargs
        assert call_kwargs["os_name"] == "linux"
        assert call_kwargs["working_directory"] == "/"

    async def test_no_target_operator_uses_first(self):
        """Test that no target_operator defaults to first operator (backward compatibility)."""
        executor = _make_tool_executor()
        _mock_executor_success(executor)

        mock_generate = AsyncMock(side_effect=_noop_generate_command)
        with patch.object(agent_tool_loop_module, "generate_command", new=mock_generate):
            await orchestrate_tool_execution(
                ToolCall(
                    name=OperatorToolName.RUN_COMMANDS,
                    args={"command": "ls"},  # No target_operator
                ),
                tool_executor=executor,
                investigation=self.multi_op_investigation,
                vso_context=self.multi_op_vso_context,
                vsod_event_service=AsyncMock(),
                request_settings=REQUEST_SETTINGS,
            )

        # Should use first operator (linux) by default
        call_kwargs = mock_generate.call_args.kwargs
        assert call_kwargs["os_name"] == "linux"
        assert call_kwargs["working_directory"] == "/"
