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

import pytest
from unittest.mock import AsyncMock, MagicMock, patch

from app.constants import (
    CommandGenerationOutcome,
    LLMProvider,
    EventType,
)
from app.models.settings import LLMSettings, G8eeUserSettings
from app.services.ai.generator import (
    generate_command,
)
from tests.unit.services.ai.test_command_generator import (
    _make_mock_provider,
    _make_mock_operator_context,
    _REPUTATION_KWARGS,
)

@pytest.mark.asyncio
async def test_generate_command_round_2_triggered():
    """Test that Round 2 is triggered when consensus is low and enabled."""
    llm = LLMSettings(
        primary_provider=LLMProvider.OLLAMA,
        lite_provider=LLMProvider.OLLAMA,
        lite_model="gemma3:1b",
        llm_command_gen_passes=3,
        llm_command_gen_rounds=2,
    )
    settings = G8eeUserSettings(llm=llm)

    call_count = 0

    async def mock_generate_content_lite(**kwargs):
        nonlocal call_count
        call_count += 1
        mock_response = MagicMock()
        if call_count <= 3:  # Round 1: 3 different commands (no consensus)
            mock_response.text = f"cmd_{call_count}"
        elif call_count <= 6:  # Round 2: consensus on cmd_1
            mock_response.text = "cmd_1"
        else:  # Auditor
            mock_response.text = '{"status": "ok"}'
        return mock_response

    mock_provider = _make_mock_provider(generate_content_lite_side_effect=mock_generate_content_lite)

    with patch("app.services.ai.generator.get_llm_provider", return_value=mock_provider):
        mock_event_service = MagicMock()
        mock_event_service.publish = AsyncMock()
        
        result = await generate_command(
            request="test request",
            guidelines="",
            operator_context=_make_mock_operator_context(),
            g8ed_event_service=mock_event_service,
            web_session_id="ws-1",
            user_id="user-1",
            case_id="case-1",
            investigation_id="inv-1",
            settings=settings,
            **_REPUTATION_KWARGS,
        )

        assert result.final_command == "cmd_1"
        assert result.outcome == CommandGenerationOutcome.VERIFIED
        assert result.round_2_candidates is not None
        assert len(result.round_2_candidates) == 3
        assert result.round_2_vote_breakdown is not None
        assert result.round_2_vote_breakdown.winner == "cmd_1"

        # Verify Round 2 events were emitted
        emitted_event_types = [call[0][0].event_type for call in mock_event_service.publish.call_args_list]
        assert EventType.TRIBUNAL_VOTING_ROUND_2_STARTED in emitted_event_types
        assert EventType.TRIBUNAL_VOTING_ROUND_2_CONSENSUS_REACHED in emitted_event_types
