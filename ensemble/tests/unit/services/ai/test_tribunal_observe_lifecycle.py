# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

"""Unit tests for observe producer Tribunal agent-state projections
emitted by ``generate_command`` during the consensus lifecycle.

Asserts that the Tribunal persona enters running at consensus start,
completed on final success, failed on terminal failure, and that
first-round no-consensus followed by round two is not terminal. Also
asserts that no raw request or candidate command appears in producer
payloads.
"""

from __future__ import annotations

from unittest.mock import AsyncMock, MagicMock, patch

import pytest

from app.constants import G8EE_COMPONENT, LLMProvider
from app.models.agents.tribunal import (
    TribunalConsensusFailedError,
    TribunalDisabledError,
)
from app.models.http_context import G8eHttpContext
from app.models.settings import G8eeUserSettings, LLMSettings
from app.services.ai.generator import generate_command
from tests.fakes.fake_event_service import FakeEventService
from tests.unit.services.ai.tribunal.conftest import (
    make_tribunal_generation_request,
)

pytestmark = [pytest.mark.unit, pytest.mark.asyncio]


def _make_context() -> G8eHttpContext:
    return G8eHttpContext(
        web_session_id="ws-trib-1",
        user_id="user-trib-1",
        case_id="case-trib-1",
        investigation_id="inv-trib-1",
        source_component=G8EE_COMPONENT,
    )


def _make_disabled_settings() -> G8eeUserSettings:
    return G8eeUserSettings(llm=LLMSettings(llm_command_gen_enabled=False))


def _make_enabled_settings(passes: int = 3) -> G8eeUserSettings:
    return G8eeUserSettings(
        llm=LLMSettings(
            llm_command_gen_enabled=True,
            primary_provider=LLMProvider.OLLAMA,
            lite_provider=LLMProvider.OLLAMA,
            lite_model="gemma3:1b",
            primary_model="test-primary",
            llm_command_gen_passes=passes,
            llm_command_gen_auditor=False,
        )
    )


def _make_mock_provider(generation_text: str, passes: int = 3):
    """Build a mock provider returning the same command for all passes."""
    call_count = 0

    async def _side_effect(**kwargs):
        nonlocal call_count
        call_count += 1
        resp = MagicMock()
        resp.text = generation_text
        return resp

    mock_provider = MagicMock()
    mock_provider.generate_content_lite = AsyncMock(side_effect=_side_effect)
    mock_provider.__aenter__ = AsyncMock(return_value=mock_provider)
    mock_provider.__aexit__ = AsyncMock(return_value=False)
    return mock_provider


def _make_mock_provider_divergent(commands: list[str]):
    """Build a mock provider returning different commands per pass (no consensus).

    Each call cycles through the commands list so that every pass produces
    a different command, preventing consensus in both rounds.
    """
    call_count = 0

    async def _side_effect(**kwargs):
        nonlocal call_count
        idx = call_count % len(commands)
        call_count += 1
        resp = MagicMock()
        resp.text = commands[idx]
        return resp

    mock_provider = MagicMock()
    mock_provider.generate_content_lite = AsyncMock(side_effect=_side_effect)
    mock_provider.__aenter__ = AsyncMock(return_value=mock_provider)
    mock_provider.__aexit__ = AsyncMock(return_value=False)
    return mock_provider


def _agent_state_statuses(event_svc: FakeEventService) -> list[str]:
    return [req.status for req in event_svc.agent_state_requests]


async def test_disabled_tribunal_emits_offline_agent_state():
    event_svc = FakeEventService()
    settings = _make_disabled_settings()

    with pytest.raises(TribunalDisabledError):
        await generate_command(
            make_tribunal_generation_request(
                request="list files",
                event_service=event_svc,
                g8e_context=_make_context(),
                settings=settings,
            )
        )

    statuses = _agent_state_statuses(event_svc)
    assert "offline" in statuses


async def test_successful_consensus_emits_running_then_completed():
    event_svc = FakeEventService()
    settings = _make_enabled_settings()

    mock_provider = _make_mock_provider("ls -la")

    with patch(
        "app.services.ai.generator.get_llm_provider",
        return_value=mock_provider,
    ):
        await generate_command(
            make_tribunal_generation_request(
                request="list files",
                event_service=event_svc,
                g8e_context=_make_context(),
                settings=settings,
            )
        )

    statuses = _agent_state_statuses(event_svc)
    assert statuses[0] == "running"
    assert statuses[-1] == "completed"


async def test_consensus_failure_emits_failed_agent_state():
    event_svc = FakeEventService()
    settings = _make_enabled_settings(passes=5)

    mock_provider = _make_mock_provider_divergent(["cmd-a", "cmd-b", "cmd-c", "cmd-d", "cmd-e"])

    with patch(
        "app.services.ai.generator.get_llm_provider",
        return_value=mock_provider,
    ):
        with pytest.raises(TribunalConsensusFailedError):
            await generate_command(
                make_tribunal_generation_request(
                    request="list files",
                    event_service=event_svc,
                    g8e_context=_make_context(),
                    settings=settings,
                )
            )

    statuses = _agent_state_statuses(event_svc)
    assert "failed" in statuses


async def test_producer_payload_contains_no_raw_request_or_candidate_command():
    event_svc = FakeEventService()
    settings = _make_enabled_settings()

    mock_provider = _make_mock_provider("ls -la")

    with patch(
        "app.services.ai.generator.get_llm_provider",
        return_value=mock_provider,
    ):
        await generate_command(
            make_tribunal_generation_request(
                request="list files",
                event_service=event_svc,
                g8e_context=_make_context(),
                settings=settings,
            )
        )

    for req in event_svc.agent_state_requests:
        dumped = req.model_dump(mode="json")
        for value in dumped.values():
            if isinstance(value, str):
                assert "list files" not in value, (
                    "Raw request must not appear in producer payload"
                )
                assert "ls -la" not in value, (
                    "Candidate command must not appear in producer payload"
                )


async def test_producer_failure_does_not_abort_consensus():
    event_svc = FakeEventService()
    settings = _make_enabled_settings()

    async def _failing_push(request):
        raise RuntimeError("producer unavailable")

    event_svc.publish_agent_state = _failing_push

    mock_provider = _make_mock_provider("ls -la")

    with patch(
        "app.services.ai.generator.get_llm_provider",
        return_value=mock_provider,
    ):
        result = await generate_command(
            make_tribunal_generation_request(
                request="list files",
                event_service=event_svc,
                g8e_context=_make_context(),
                settings=settings,
            )
        )

    assert result is not None


async def test_tribunal_agent_id_uses_tribunal_persona():
    event_svc = FakeEventService()
    settings = _make_enabled_settings()

    mock_provider = _make_mock_provider("ls -la")

    with patch(
        "app.services.ai.generator.get_llm_provider",
        return_value=mock_provider,
    ):
        await generate_command(
            make_tribunal_generation_request(
                request="list files",
                event_service=event_svc,
                g8e_context=_make_context(),
                settings=settings,
            )
        )

    assert len(event_svc.agent_state_requests) > 0
    req = event_svc.agent_state_requests[0]
    assert req.agent_id == "user-trib-1:tribunal"
    assert req.display_name == "Tribunal"
    assert req.role == "arbitrator"
