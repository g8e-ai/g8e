
import pytest
import asyncio
from unittest.mock import AsyncMock, MagicMock, patch
from app.constants import (
    CommandGenerationOutcome,
    LLMProvider,
    TribunalMember,
    ComponentName,
    EventType,
    VerifierReason,
)
from app.llm.llm_types import Role
from app.models.agents.tribunal import VoteBreakdown, TribunalVerifierFailedError
from app.services.ai.command_generator import _run_verifier, TribunalEmitter
from app.models.http_context import G8eHttpContext
from app.utils.agent_persona_loader import get_agent_persona

def _make_mock_g8e_context() -> G8eHttpContext:
    return G8eHttpContext(
        web_session_id="test-session-id",
        user_id="test-user-id",
        case_id="test-case-id",
        investigation_id="test-investigation-id",
        source_component=ComponentName.G8EE,
    )

@pytest.mark.asyncio
async def test_verifier_repro_json_failure():
    # Mock a response that is truncated or has prose prefix that might confuse parsing
    # as seen in the logs: raw_text='Here is the JSON requested:\n```json'
    mock_response = MagicMock()
    mock_response.text = 'Here is the JSON requested:\n```json'

    mock_provider = MagicMock()
    mock_provider.generate_content_lite = AsyncMock(return_value=mock_response)
    
    # Mock emitter to avoid network calls
    emitter = MagicMock(spec=TribunalEmitter)
    emitter.emit = AsyncMock()

    vote_breakdown = VoteBreakdown(
        candidates_by_member={"axiom": "ls -la"},
        candidates_by_command={"ls -la": ["axiom"]},
        winner="ls -la",
        winner_supporters=["axiom"],
        dissenters_by_command={},
        consensus_strength=1.0,
    )

    with pytest.raises(TribunalVerifierFailedError) as exc_info:
        await _run_verifier(
            provider=mock_provider,
            model="test-model",
            request="list files",
            guidelines="",
            mode="unanimous",
            vote_winner="ls -la",
            vote_breakdown=vote_breakdown,
            tied_candidates=None,
            operator_context=MagicMock(),
            emitter=emitter,
            command_constraints_message="No constraints",
            verifier_persona=get_agent_persona("auditor"),
        )
    
    assert "no_valid_revision" in str(exc_info.value)
    assert "Failed to parse verifier response" in str(exc_info.value)

if __name__ == "__main__":
    asyncio.run(test_verifier_repro_json_failure())
