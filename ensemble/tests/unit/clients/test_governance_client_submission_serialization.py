import asyncio
import json
from unittest.mock import AsyncMock, patch

import pytest

from app.clients.governance_client import GovernanceClient
from app.constants import EventType
from app.constants.config import G8EE_COMPONENT
from app.models.command_request_payloads import DocumentUpdateRequestPayload
from app.models.pubsub_messages import G8eMessage

pytestmark = pytest.mark.unit


def _message(document_id: str) -> G8eMessage:
    return G8eMessage(
        id=document_id,
        source_component=G8EE_COMPONENT,
        event_type=EventType.APP_CASE_CREATED,
        case_id="test-case-id",
        user_id="test-user-id",
        operator_id="test-operator-id",
        operator_session_id="test-operator-session-id",
        payload=DocumentUpdateRequestPayload(
            collection="cases",
            document_id=document_id,
            updates={"field": "value"},
            merge=False,
        ),
    )


@pytest.mark.asyncio
async def test_submit_envelope_serializes_state_root_fetch_build_and_post_per_client():
    client = GovernanceClient(tls_config=None)
    first_post_started = asyncio.Event()
    release_first_post = asyncio.Event()
    second_submission_started = asyncio.Event()
    second_fetch_started = asyncio.Event()
    submitted_roots: list[str] = []
    fetch_count = 0

    async def fetch_state_root() -> str:
        nonlocal fetch_count
        fetch_count += 1
        if fetch_count == 2:
            second_fetch_started.set()
        return f"state-root-{fetch_count}"

    class FakeResponse:
        status = 200

        def __init__(self, post_number: int) -> None:
            self._post_number = post_number

        async def __aenter__(self):
            if self._post_number == 1:
                first_post_started.set()
                await release_first_post.wait()
            return self

        async def __aexit__(self, *args):
            return None

        async def text(self) -> str:
            return '{"status": "COMPLETED"}'

    class FakeSession:
        def post(self, _url: str, data: str):
            submitted_roots.append(json.loads(data)["state_merkle_root"])
            return FakeResponse(len(submitted_roots))

    async def submit_second() -> dict:
        second_submission_started.set()
        return await client.submit_envelope(_message("second-document"))

    with (
        patch.object(client, "fetch_state_root", new=fetch_state_root),
        patch.object(client, "_get_http_session", new=AsyncMock(return_value=FakeSession())),
    ):
        first = asyncio.create_task(client.submit_envelope(_message("first-document")))
        await first_post_started.wait()
        second = asyncio.create_task(submit_second())
        await second_submission_started.wait()

        assert not second_fetch_started.is_set()

        release_first_post.set()
        await asyncio.gather(first, second)

    assert fetch_count == 2
    assert submitted_roots == ["state-root-1", "state-root-2"]


@pytest.mark.asyncio
async def test_submit_envelope_refetches_state_root_after_state_mismatch():
    client = GovernanceClient(tls_config=None)
    submitted_roots: list[str] = []
    fetch_count = 0

    async def fetch_state_root() -> str:
        nonlocal fetch_count
        fetch_count += 1
        return f"state-root-{fetch_count}"

    class FakeResponse:
        def __init__(self, status: int, body: str) -> None:
            self.status = status
            self._body = body

        async def __aenter__(self):
            return self

        async def __aexit__(self, *args):
            return None

        async def text(self) -> str:
            return self._body

    class FakeSession:
        def post(self, _url: str, data: str):
            submitted_roots.append(json.loads(data)["state_merkle_root"])
            if len(submitted_roots) == 1:
                return FakeResponse(403, "TX_STATE_MISMATCH")
            return FakeResponse(200, '{"status": "COMPLETED"}')

    with (
        patch.object(client, "fetch_state_root", new=fetch_state_root),
        patch.object(client, "_get_http_session", new=AsyncMock(return_value=FakeSession())),
    ):
        receipt = await client.submit_envelope(_message("test-document"))

    assert receipt == {"status": "COMPLETED"}
    assert fetch_count == 2
    assert submitted_roots == ["state-root-1", "state-root-2"]
