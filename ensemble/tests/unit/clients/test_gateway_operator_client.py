# Copyright (c) 2026 Lateralus Labs, LLC.

from unittest.mock import AsyncMock, MagicMock

import pytest

from app.clients.gateway_operator_client import GatewayOperatorClient
from app.models.http_context import G8eHttpContext

pytestmark = [pytest.mark.unit]


def _response(payload: object, *, success: bool = True) -> MagicMock:
    response = MagicMock()
    response.is_success = success
    response.status_code = 200 if success else 403
    response.text = "denied" if not success else ""
    response.json.return_value = payload
    return response


@pytest.fixture
def http_client() -> MagicMock:
    return MagicMock()


@pytest.fixture
def client(http_client: MagicMock) -> GatewayOperatorClient:
    return GatewayOperatorClient(http_client)


async def test_list_reads_gateway_operator_documents(client, http_client):
    http_client.get = AsyncMock(return_value=_response({"operators": [{"id": "op-1"}]}))

    result = await client.list(user_id="user-1")

    assert result == [{"id": "op-1"}]
    http_client.get.assert_awaited_once()
    assert http_client.get.await_args.kwargs["params"] == {"user_id": "user-1"}


async def test_dispatch_sends_base64_typed_payload(client, http_client):
    http_client.post = AsyncMock(return_value=_response({"transaction_id": "tx-1"}))
    context = G8eHttpContext(user_id="user-1", operator_session_id="sess-1")

    result = await client.dispatch(
        context=context,
        operator_session_id="sess-1",
        action_type="COMMAND_REQUESTED",
        payload=b"typed-payload",
    )

    assert result == {"transaction_id": "tx-1"}
    body = http_client.post.await_args.kwargs["json_data"]
    assert body["target_operator_session_id"] == "sess-1"
    assert body["payload"] == "dHlwZWQtcGF5bG9hZA=="
    assert body["context"]["user_id"] == "user-1"


async def test_gateway_failure_is_not_reinterpreted_as_local_operator_state(
    client, http_client
):
    http_client.post = AsyncMock(return_value=_response({}, success=False))

    with pytest.raises(Exception, match="Gateway failed to stop operator"):
        await client.stop(
            context=G8eHttpContext(user_id="user-1"),
            operator_id="op-1",
        )
