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

from unittest.mock import AsyncMock, MagicMock
import pytest
from app.constants import EventType, RiskLevel
from app.models.settings import G8eeUserSettings, LLMSettings
from app.models.tool_results import CommandRiskAnalysis
from app.models.agents.tribunal import TribunalWardenBlockedError
from app.services.ai.tribunal.emitter import TribunalEmitter
from app.services.ai.tribunal.stages.warden import _run_warden_stage

@pytest.mark.asyncio
class TestRunWardenStage:
    async def test_returns_analysis_on_low_risk(self, mock_g8e_context, mock_operator_context):
        analyzer = MagicMock()
        analyzer.analyze_command_risk = AsyncMock(
            return_value=CommandRiskAnalysis(risk_level=RiskLevel.LOW)
        )
        emitter = TribunalEmitter(None, mock_g8e_context)
        settings = G8eeUserSettings(llm=LLMSettings())

        result = await _run_warden_stage(
            request="list files", guidelines="", vote_winner="ls -la",
            operator_context=mock_operator_context, emitter=emitter,
            settings=settings, investigation_id="inv-1",
            ai_response_analyzer=analyzer, investigation_state=MagicMock(),
        )

        assert result.risk_level == RiskLevel.LOW

    async def test_raises_blocked_error_on_high_risk(self, mock_g8e_context, mock_operator_context):
        analyzer = MagicMock()
        analyzer.analyze_command_risk = AsyncMock(
            return_value=CommandRiskAnalysis(risk_level=RiskLevel.HIGH)
        )
        analyzer.analyze_error_and_suggest_fix = AsyncMock(return_value=MagicMock(user_message="Risk!"))
        
        emitter = MagicMock(spec=TribunalEmitter)
        emitter.emit = AsyncMock()
        
        settings = G8eeUserSettings(llm=LLMSettings())
        investigation_state = MagicMock()
        investigation_state.warden_block_count = 0

        with pytest.raises(TribunalWardenBlockedError):
            await _run_warden_stage(
                request="danger", guidelines="", vote_winner="rm -rf /",
                operator_context=mock_operator_context, emitter=emitter,
                settings=settings, investigation_id="inv-1",
                ai_response_analyzer=analyzer, investigation_state=investigation_state,
            )

        assert investigation_state.warden_block_count == 1
        emitter.emit.assert_called_with(EventType.TRIBUNAL_SESSION_WARDEN_BLOCKED, pytest.any)
