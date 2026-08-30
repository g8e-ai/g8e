from unittest.mock import AsyncMock

import pytest

from app.constants import EventType
from app.models.events import SessionEvent
from app.models.http_context import G8eHttpContext
from app.models.reputation import StakeResolutionPayload
from app.services.infra.event_service import EventService

pytestmark = pytest.mark.unit


@pytest.mark.asyncio
async def test_publish_reputation_event_preserves_typed_payload_and_request_routing():
    http_client = AsyncMock()
    service = EventService(http_client)
    context = G8eHttpContext(
        cli_session_id="cli-session-id",
        user_id="user-id",
        case_id="case-id",
        investigation_id="investigation-id",
        task_id="task-id",
    )
    payload = StakeResolutionPayload(
        agent_id="axiom",
        investigation_id="investigation-id",
        tribunal_command_id="tribunal-command-id",
        scalar_before=0.5,
        scalar_after=0.51,
        outcome_score=1.0,
        rationale="consensus outcome held",
    )

    await service.publish_reputation_event(
        EventType.OPERATOR_REPUTATION_STATE_UPDATED,
        payload,
        context,
    )

    event = http_client.push_sse_event.await_args.args[0]
    assert isinstance(event, SessionEvent)
    assert event.event_type == EventType.OPERATOR_REPUTATION_STATE_UPDATED
    assert event.payload == payload
    assert event.cli_session_id == "cli-session-id"
    assert event.user_id == "user-id"
    assert event.case_id == "case-id"
    assert event.investigation_id == "investigation-id"
    assert event.task_id == "task-id"
