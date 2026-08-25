# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.


import pytest
from unittest.mock import AsyncMock, MagicMock
from app.services.ai.chat_pipeline import ChatPipelineService
from app.models.agent import AgentStreamState, AgentInputs
from app.models.agents.triage import TriageResult
from app.constants import (
    TriageComplexityClassification,
    TriageConfidence,
    TriageIntentClassification,
    TriageRequestPosture,
    EventType,
)
from app.models.http_context import G8eHttpContext
from tests.fakes.fake_event_service import FakeEventService


@pytest.mark.asyncio
async def test_interrogation_questions_published():
    # Setup
    svc = ChatPipelineService.__new__(ChatPipelineService)
    svc.event_service = FakeEventService()
    svc.investigation_service = MagicMock()
    svc.investigation_service.persist_ai_message = AsyncMock(return_value=True)

    g8e_context = G8eHttpContext(
        investigation_id="inv-123",
        case_id="case-456",
        user_id="user-789",
        web_session_id="sess-000",
    )

    triage_result = TriageResult(
        complexity=TriageComplexityClassification.COMPLEX,
        complexity_confidence=TriageConfidence.HIGH,
        intent=TriageIntentClassification.ACTION,
        intent_confidence=TriageConfidence.HIGH,
        intent_summary="test",
        request_posture=TriageRequestPosture.NORMAL,
        posture_confidence=TriageConfidence.HIGH,
    )

    inputs = MagicMock(spec=AgentInputs)
    inputs.triage_result = triage_result
    inputs.message_sender = "ai_primary"
    inputs.investigation = MagicMock()
    inputs.conversation_history = []

    state = AgentStreamState()
    state.response_text = """I need more info.
<interrogation>
1. What is the error?
2. When did it start?
3. Have you tried restarting?
</interrogation>"""

    user_settings = MagicMock()

    # Execute
    await svc._persist_ai_response(
        g8e_context=g8e_context, inputs=inputs, state=state, user_settings=user_settings
    )

    # Verify
    events = svc.event_service.published
    interrogation_events = [
        e for e in events if e.event_type == EventType.AI_TRIAGE_CLARIFICATION_QUESTIONS
    ]

    assert len(interrogation_events) == 1
    event = interrogation_events[0]
    assert event.payload.questions == [
        "What is the error?",
        "When did it start?",
        "Have you tried restarting?",
    ]
    assert event.investigation_id == "inv-123"
    assert event.web_session_id == "sess-000"


@pytest.mark.asyncio
async def test_interrogation_questions_not_published_when_missing():
    # Setup
    svc = ChatPipelineService.__new__(ChatPipelineService)
    svc.event_service = FakeEventService()
    svc.investigation_service = MagicMock()
    svc.investigation_service.persist_ai_message = AsyncMock(return_value=True)

    g8e_context = MagicMock()
    inputs = MagicMock(spec=AgentInputs)
    inputs.triage_result = None
    inputs.message_sender = "ai_primary"
    inputs.investigation = None

    state = AgentStreamState()
    state.response_text = "Just a normal response without questions."

    user_settings = MagicMock()

    # Execute
    await svc._persist_ai_response(
        g8e_context=g8e_context, inputs=inputs, state=state, user_settings=user_settings
    )

    # Verify
    events = svc.event_service.published
    interrogation_events = [
        e for e in events if e.event_type == EventType.AI_TRIAGE_CLARIFICATION_QUESTIONS
    ]
    assert len(interrogation_events) == 0
