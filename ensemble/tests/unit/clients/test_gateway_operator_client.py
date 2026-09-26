# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

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
def internal_http_client() -> MagicMock:
    client = MagicMock()
    client.client = MagicMock()
    client._ensure_mtls = MagicMock()
    return client


@pytest.fixture
def gateway_client(internal_http_client: MagicMock) -> GatewayOperatorClient:
    return GatewayOperatorClient(internal_http_client)


async def test_list_reads_gateway_operator_documents(gateway_client, internal_http_client):
    internal_http_client.client.get = AsyncMock(return_value=_response({"operators": [{"id": "op-1"}]}))

    result = await gateway_client.list(user_id="user-1")

    assert result == [{"id": "op-1"}]
    internal_http_client.client.get.assert_awaited_once()
    assert internal_http_client.client.get.await_args.kwargs["params"] == {"user_id": "user-1"}
    internal_http_client._ensure_mtls.assert_called()


async def test_dispatch_sends_base64_typed_payload(gateway_client, internal_http_client):
    internal_http_client.client.post = AsyncMock(return_value=_response({"transaction_id": "tx-1"}))
    context = G8eHttpContext(
        user_id="user-1",
        operator_session_id="sess-1",
        cli_session_id="cli-1",
        case_id="case-1",
    )

    result = await gateway_client.dispatch(
        context=context,
        operator_session_id="sess-1",
        event_type="g8e.v1.operator.command.requested",
        payload=b"typed-payload",
    )

    assert result == {"transaction_id": "tx-1"}
    body = internal_http_client.client.post.await_args.kwargs["json_data"]
    assert body["target_operator_session_id"] == "sess-1"
    assert body["event_type"] == "g8e.v1.operator.command.requested"
    assert body["payload"] == "dHlwZWQtcGF5bG9hZA=="
    assert body["cli_session_id"] == "cli-1"
    assert body["case_id"] == "case-1"


async def test_gateway_failure_is_not_reinterpreted_as_local_operator_state(
    gateway_client, internal_http_client
):
    internal_http_client.client.post = AsyncMock(return_value=_response({}, success=False))

    with pytest.raises(Exception, match="Gateway failed to stop operator"):
        await gateway_client.stop(
            context=G8eHttpContext(user_id="user-1"),
            operator_session_id="sess-1",
        )


async def test_bind_sends_canonical_gateway_body(gateway_client, internal_http_client):
    internal_http_client.client.post = AsyncMock(return_value=_response({"success": True, "bound_count": 1}))
    context = G8eHttpContext(user_id="user-1", web_session_id="web-1")

    result = await gateway_client.bind(context=context, operator_ids=["op-1"])

    assert result["bound_count"] == 1
    body = internal_http_client.client.post.await_args.kwargs["json_data"]
    assert body == {
        "operator_ids": ["op-1"],
        "user_id": "user-1",
        "web_session_id": "web-1",
    }


async def test_validate_session_calls_gateway(gateway_client, internal_http_client):
    internal_http_client.client.post = AsyncMock(return_value=_response({"valid": True, "operator_id": "op-1"}))
    context = G8eHttpContext(user_id="user-1")

    result = await gateway_client.validate_session(
        context=context,
        operator_session_id="sess-1",
        cli_session_id="cli-1",
    )

    assert result["valid"] is True
    body = internal_http_client.client.post.await_args.kwargs["json_data"]
    assert body == {
        "operator_session_id": "sess-1",
        "cli_session_id": "cli-1",
        "user_id": "user-1",
    }
