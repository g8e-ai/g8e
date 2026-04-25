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
Integration test for the reputation resolution pipeline (GDD Phase 3).

Verifies that:
1. Post-execution hook in agent_tool_loop.py correctly identifies Tribunal commands.
2. Reputation resolution is gated by REPUTATION_RESOLUTION_ENABLED.
3. Detached tasks are scheduled and execute resolution.
4. SSE events (STATE_UPDATED, SLASH_TIERN) are emitted on resolution.
"""

import pytest
import asyncio
from unittest.mock import AsyncMock, MagicMock, patch
from datetime import datetime, UTC

from app.constants import (
    CommandErrorType,
    CommandGenerationOutcome,
    ComponentName,
    EventType,
    ExecutionStatus,
    OperatorToolName,
)
from app.models.agents.tribunal import CommandGenerationResult
from app.models.reputation import ReputationState, StakeResolution, SlashTier
from app.models.http_context import G8eHttpContext
from app.models.investigations import EnrichedInvestigationContext
from app.models.tool_results import CommandExecutionResult, FileEditResult
from app.services.ai.agent_tool_loop import orchestrate_tool_execution
from app.llm.llm_types import ToolCall
from tests.fakes.agent_helpers import (
    make_agent_run_args,
    make_g8ed_event_service,
)

pytestmark = [pytest.mark.integration, pytest.mark.asyncio(loop_scope="session")]

class TestReputationPipelineIntegration:
    """Verify end-to-end reputation resolution flow from tool loop to SSE."""

    @pytest.fixture
    def mock_tool_executor(self):
        executor = MagicMock()
        executor.reputation_resolution_enabled = True
        executor.reputation_service = AsyncMock()
        executor.chat_task_manager = MagicMock()
        
        # Capture the detached task for manual execution/awaiting
        detached_tasks = []
        def track_detached(coro, name=None):
            task = asyncio.create_task(coro)
            detached_tasks.append(task)
            return task
        
        executor.chat_task_manager.track_detached.side_effect = track_detached
        executor._detached_tasks = detached_tasks
        
        return executor

    async def test_successful_tribunal_command_triggers_resolution(self, mock_tool_executor):
        """A successful Tribunal command should trigger reputation resolution."""
        inputs, _ = make_agent_run_args()
        event_svc = make_g8ed_event_service()
        
        # Setup context
        g8e_context = G8eHttpContext(
            case_id="rep-test-case",
            investigation_id="rep-test-inv",
            web_session_id="rep-test-sess",
            user_id="rep-test-user",
            source_component=ComponentName.G8EE
        )
        investigation = EnrichedInvestigationContext(
            id="rep-test-inv",
            case_id="rep-test-case",
            user_id="rep-test-user",
            sentinel_mode=False
        )
        
        # Setup tool call
        tool_call = ToolCall(
            id="call_123",
            name=OperatorToolName.RUN_COMMANDS,
            args={"request": "ls"}
        )
        
        # Setup results
        gen_result = CommandGenerationResult(
            correlation_id="tribunal_123",
            outcome=CommandGenerationOutcome.VERIFIED,
            final_command="ls",
            audit_reason="OK",
            request="ls"
        )
        exec_result = CommandExecutionResult(
            success=True,
            execution_id="exec_123",
            output="file1\nfile2",
            exit_code=0,
            execution_status=ExecutionStatus.COMPLETED
        )
        
        mock_tool_executor.execute_tool_call = AsyncMock(return_value=exec_result)
        
        # Mock reputation resolution return
        resolution = StakeResolution(
            id="tribunal_123:axiom",
            investigation_id="rep-test-inv",
            tribunal_command_id="tribunal_123",
            agent_id="axiom",
            outcome_score=1.0,
            rationale="OK",
            scalar_before=0.5,
            scalar_after=0.51,
            half_life=50,
            created_at=datetime.now(UTC)
        )
        mock_tool_executor.reputation_service.resolve_stakes.return_value = MagicMock(resolutions=[resolution])

        # Execute loop with Tribunal mock
        with patch("app.services.ai.agent_tool_loop.TribunalInvoker.run", new_callable=AsyncMock) as mock_tribunal:
            mock_tribunal.return_value = (MagicMock(), gen_result)
            
            await orchestrate_tool_execution(
                tool_call=tool_call,
                tool_executor=mock_tool_executor,
                investigation=investigation,
                g8e_context=g8e_context,
                g8ed_event_service=event_svc,
                request_settings=inputs.request_settings
            )

        # Verify detached task was scheduled
        assert len(mock_tool_executor._detached_tasks) == 1
        await asyncio.wait(mock_tool_executor._detached_tasks)

        # Verify resolve_stakes was called correctly
        mock_tool_executor.reputation_service.resolve_stakes.assert_called_once_with(
            tribunal_command_id="tribunal_123",
            investigation_id="rep-test-inv",
            gen_result=gen_result,
            execution_result=exec_result,
            warden_risk=None
        )

        # Verify SSE events
        published = [e.event_type for e in event_svc._published_events]
        assert EventType.REPUTATION_STATE_UPDATED in published

    async def test_failed_tribunal_command_triggers_tier2_slash(self, mock_tool_executor):
        """A failed Tribunal command (Tier 2) should trigger resolution with slashing."""
        inputs, _ = make_agent_run_args()
        event_svc = make_g8ed_event_service()
        
        g8e_context = G8eHttpContext(
            case_id="slash-case",
            investigation_id="slash-inv",
            web_session_id="slash-sess",
            user_id="slash-user",
            source_component=ComponentName.G8EE
        )
        investigation = EnrichedInvestigationContext(
            id="slash-inv",
            case_id="slash-case",
            user_id="slash-user",
            sentinel_mode=False
        )
        tool_call = ToolCall(id="call_456", name=OperatorToolName.RUN_COMMANDS, args={"request": "rm"})
        
        gen_result = CommandGenerationResult(
            correlation_id="tribunal_456",
            outcome=CommandGenerationOutcome.VERIFIED,
            final_command="rm",
            audit_reason="OK",
            request="rm"
        )
        exec_result = CommandExecutionResult(
            success=False,
            execution_id="exec_456",
            error="Command failed",
            exit_code=1,
            execution_status=ExecutionStatus.FAILED
        )
        
        mock_tool_executor.execute_tool_call = AsyncMock(return_value=exec_result)
        
        resolution = StakeResolution(
            id="tribunal_456:axiom",
            investigation_id="slash-inv",
            tribunal_command_id="tribunal_456",
            agent_id="axiom",
            outcome_score=0.0,
            rationale="EXEC_FAILED",
            slash_tier=SlashTier.TIER_2,
            scalar_before=0.5,
            scalar_after=0.4,
            half_life=50,
            created_at=datetime.now(UTC)
        )
        mock_tool_executor.reputation_service.resolve_stakes.return_value = MagicMock(resolutions=[resolution])

        with patch("app.services.ai.agent_tool_loop.TribunalInvoker.run", new_callable=AsyncMock) as mock_tribunal:
            mock_tribunal.return_value = (MagicMock(), gen_result)
            await orchestrate_tool_execution(
                tool_call=tool_call,
                tool_executor=mock_tool_executor,
                investigation=investigation,
                g8e_context=g8e_context,
                g8ed_event_service=event_svc,
                request_settings=inputs.request_settings
            )

        await asyncio.wait(mock_tool_executor._detached_tasks)

        # Verify events including slashing
        published = [e.event_type for e in event_svc._published_events]
        assert EventType.REPUTATION_STATE_UPDATED in published
        assert EventType.REPUTATION_SLASH_TIER2 in published

    async def test_resolution_disabled_skips_hook(self, mock_tool_executor):
        """When REPUTATION_RESOLUTION_ENABLED is False, no resolution should occur."""
        mock_tool_executor.reputation_resolution_enabled = False
        inputs, _ = make_agent_run_args()
        event_svc = make_g8ed_event_service()
        
        tool_call = ToolCall(id="call_789", name=OperatorToolName.RUN_COMMANDS, args={"request": "ls"})
        gen_result = CommandGenerationResult(
            correlation_id="tribunal_789",
            outcome=CommandGenerationOutcome.VERIFIED,
            final_command="ls",
            audit_reason="OK",
            request="ls"
        )
        exec_result = CommandExecutionResult(success=True)
        
        mock_tool_executor.execute_tool_call = AsyncMock(return_value=exec_result)

        with patch("app.services.ai.agent_tool_loop.TribunalInvoker.run", new_callable=AsyncMock) as mock_tribunal:
            mock_tribunal.return_value = (MagicMock(), gen_result)
            await orchestrate_tool_execution(
                tool_call=tool_call,
                tool_executor=mock_tool_executor,
                investigation=EnrichedInvestigationContext(
                    id="inv",
                    case_id="case",
                    user_id="user",
                    sentinel_mode=False
                ),
                g8e_context=G8eHttpContext(
                    web_session_id="sess",
                    user_id="user",
                    case_id="case",
                    investigation_id="inv",
                    source_component=ComponentName.G8EE
                ),
                g8ed_event_service=event_svc,
                request_settings=inputs.request_settings
            )

        # Verify no tasks were scheduled
        assert len(mock_tool_executor._detached_tasks) == 0
        mock_tool_executor.reputation_service.resolve_stakes.assert_not_called()

    async def test_non_tribunal_tool_skips_resolution(self, mock_tool_executor):
        """Non-Tribunal tools (e.g. file_read) should never trigger reputation resolution."""
        inputs, _ = make_agent_run_args()
        event_svc = make_g8ed_event_service()
        
        # file_read is an operator tool but not RUN_COMMANDS (Tribunal)
        tool_call = ToolCall(id="call_abc", name=OperatorToolName.FILE_READ, args={"file_path": "/tmp/test"})
        mock_tool_executor.execute_tool_call = AsyncMock(return_value=FileEditResult(
            success=True,
            execution_id="exec_abc"
        ))

        await orchestrate_tool_execution(
            tool_call=tool_call,
            tool_executor=mock_tool_executor,
            investigation=EnrichedInvestigationContext(
                id="inv",
                case_id="case",
                user_id="user",
                sentinel_mode=False
            ),
            g8e_context=G8eHttpContext(
                web_session_id="sess",
                user_id="user",
                case_id="case",
                investigation_id="inv",
                source_component=ComponentName.G8EE
            ),
            g8ed_event_service=event_svc,
            request_settings=inputs.request_settings
        )

        assert len(mock_tool_executor._detached_tasks) == 0
        mock_tool_executor.reputation_service.resolve_stakes.assert_not_called()
