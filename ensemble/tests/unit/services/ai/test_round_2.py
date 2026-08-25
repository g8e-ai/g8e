# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

from unittest.mock import AsyncMock, MagicMock, patch

import pytest

from app.constants import (
    CommandGenerationOutcome,
    ComponentName,
    EventType,
    G8EE_COMPONENT,
    LLMProvider,
)
from app.models.http_context import G8eHttpContext
from app.models.settings import G8eeUserSettings, LLMSettings
from app.services.ai.generator import (
    generate_command,
)
from app.models.tribunal_commands import TribunalGenerationRequest
from tests.unit.services.ai.tribunal.conftest import (
    make_tribunal_generation_request,
    _make_mock_provider,
    _make_mock_operator_context,
)


@pytest.mark.asyncio
async def test_generate_command_round_2_triggered():
    """Test that Round 2 is triggered when consensus is low and enabled."""
    llm = LLMSettings(
        primary_provider=LLMProvider.OLLAMA,
        lite_provider=LLMProvider.OLLAMA,
        lite_model="gemma3:1b",
        llm_command_gen_passes=3,
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

    mock_provider = _make_mock_provider(
        generate_content_lite_side_effect=mock_generate_content_lite
    )

    with patch("app.services.ai.generator.get_llm_provider", return_value=mock_provider):
        mock_event_service = MagicMock()
        mock_event_service.publish = AsyncMock()

        result = await generate_command(
            make_tribunal_generation_request(
                request="test request",
                guidelines="",
                event_service=mock_event_service,
                g8e_context=G8eHttpContext(
                    web_session_id="ws-1",
                    user_id="user-1",
                    case_id="case-1",
                    investigation_id="inv-1",
                    source_component=G8EE_COMPONENT,
                ),
                settings=settings,
            )
        )

        assert result.final_command == "cmd_1"
        assert result.outcome == CommandGenerationOutcome.VERIFIED
        assert result.round_2_candidates is not None
        assert len(result.round_2_candidates) == 3
        assert result.round_2_vote_breakdown is not None
        assert result.round_2_vote_breakdown.winner == "cmd_1"

        # Verify Round 2 events were emitted
        emitted_event_types = [
            call[0][0].event_type for call in mock_event_service.publish.call_args_list
        ]
        assert EventType.AI_CONSENSUS_VOTING_ROUND_2_STARTED in emitted_event_types
        assert EventType.AI_CONSENSUS_VOTING_ROUND_2_CONSENSUS_REACHED in emitted_event_types

        # Regression Test: Ensure TRIBUNAL_VOTING_CONSENSUS_NOT_REACHED is emitted instead of FAILED
        assert EventType.AI_CONSENSUS_VOTING_CONSENSUS_NOT_REACHED in emitted_event_types
        assert EventType.AI_CONSENSUS_VOTING_CONSENSUS_FAILED not in emitted_event_types
