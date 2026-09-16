# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software
# is released under the Apache License, Version 2.0.

"""Unit tests for InternalHttpClient.dispatch_inference.

These tests verify the protocol-owned protojson wire contract: the request
serializes via ``MessageToDict(preserving_proto_field_name=True)`` (proto
field names, enum names as strings) and the response parses via
``ParseDict`` into the generated ``InferenceDispatchResponse`` message.
They do not touch the network; the underlying HTTPClient is stubbed.
"""

from __future__ import annotations

from unittest.mock import AsyncMock, MagicMock

import pytest
from google.protobuf import json_format

from app.constants import G8EE_COMPONENT
from app.errors import NetworkError
from app.models.internal_api import (
    InferenceDispatchRequest,
    InferenceDispatchResponse,
)
from app.services.infra.internal_http_client import InternalHttpClient
from g8e.operator.v1.operator_pb2 import (
    EXECUTION_STATUS_COMPLETED,
    INFERENCE_MESSAGE_ROLE_USER,
    MODEL_ROLE_ASSISTANT,
    RECEIPT_FAILURE_CODE_GOVERNANCE_REJECTED,
)

pytestmark = pytest.mark.unit


def _make_client() -> InternalHttpClient:
    """Build an InternalHttpClient with a stubbed HTTPClient and settings."""
    settings = MagicMock()
    settings.component_urls.client_url = "https://client.local"
    settings.ca_cert_path = None
    settings.client_cert_path = None
    settings.client_key_path = None
    settings.auth.internal_api_key = None
    return InternalHttpClient(settings)


def _dispatch_request() -> InferenceDispatchRequest:
    request = InferenceDispatchRequest(
        role=MODEL_ROLE_ASSISTANT,
        model="gemma3:4b",
        max_tokens=128,
        case_id="case-1",
        investigation_id="inv-1",
        task_id="task-1",
        web_session_id="web-1",
        target_operator_session_id="sess-inf-1",
        provider_attempt_id="provider-attempt-1",
    )
    message = request.messages.add(role=INFERENCE_MESSAGE_ROLE_USER)
    message.parts.add(text="summarize this")
    return request


@pytest.mark.asyncio
async def test_dispatch_inference_posts_protojson_request_and_parses_proto_response():
    client = _make_client()
    response = MagicMock()
    response.is_success = True
    response.json.return_value = {
        "transaction_id": "tx-inference-001",
        "result": {
            "parts": [{"text": "generated output"}],
            "prompt_tokens": 7,
            "completion_tokens": 11,
            "total_tokens": 18,
            "finish_reason": "stop",
            "model": "gemma3:4b",
            "result_digest": "ab" * 32,
        },
        "receipt": {
            "transaction_id": "tx-inference-001",
            "status": "EXECUTION_STATUS_COMPLETED",
            "result_summary": "ab" * 32,
            "signer_key_id": "warden-key",
        },
    }
    client._http.post = AsyncMock(return_value=response)

    result = await client.dispatch_inference(_dispatch_request())

    assert isinstance(result, InferenceDispatchResponse)
    assert result.transaction_id == "tx-inference-001"
    assert result.HasField("result")
    assert result.result.parts[0].text == "generated output"
    assert result.result.total_tokens == 18
    assert result.HasField("receipt")
    assert result.receipt.status == EXECUTION_STATUS_COMPLETED

    client._http.post.assert_awaited_once()
    path = client._http.post.await_args.args[0]
    assert path == "/api/v1/inference/dispatch"
    sent = client._http.post.await_args.kwargs["json_data"]
    # The wire body is the canonical protojson mapping: proto field names,
    # enum names as strings — the same shape the Go controller decodes with
    # strict protojson.
    assert sent == {
        "role": "MODEL_ROLE_ASSISTANT",
        "model": "gemma3:4b",
        "max_tokens": 128,
        "target_operator_session_id": "sess-inf-1",
        "case_id": "case-1",
        "investigation_id": "inv-1",
        "task_id": "task-1",
        "web_session_id": "web-1",
        "provider_attempt_id": "provider-attempt-1",
        "messages": [
            {
                "role": "INFERENCE_MESSAGE_ROLE_USER",
                "parts": [{"text": "summarize this"}],
            }
        ],
    }


@pytest.mark.asyncio
async def test_dispatch_inference_response_round_trips_through_protojson():
    """The parsed response re-serializes to the same protojson the gateway emitted."""
    client = _make_client()
    expected = InferenceDispatchResponse(
        transaction_id="tx-inference-002",
    )
    expected.receipt.transaction_id = "tx-inference-002"
    expected.receipt.status = EXECUTION_STATUS_COMPLETED
    expected.receipt.failure_code = RECEIPT_FAILURE_CODE_GOVERNANCE_REJECTED
    wire = json_format.MessageToDict(expected, preserving_proto_field_name=True)

    response = MagicMock()
    response.is_success = True
    response.json.return_value = wire
    client._http.post = AsyncMock(return_value=response)

    result = await client.dispatch_inference(_dispatch_request())

    assert result.receipt.failure_code == RECEIPT_FAILURE_CODE_GOVERNANCE_REJECTED
    assert not result.HasField("result")


@pytest.mark.asyncio
async def test_dispatch_inference_raises_network_error_on_non_2xx_with_status_preserved():
    client = _make_client()
    response = MagicMock()
    response.is_success = False
    response.status_code = 403
    response.text = '{"error":"inference model override denied"}'
    client._http.post = AsyncMock(return_value=response)

    with pytest.raises(NetworkError) as exc_info:
        await client.dispatch_inference(_dispatch_request())

    assert exc_info.value.error_detail.details["status_code"] == 403
    assert exc_info.value.error_detail.details["role"] == MODEL_ROLE_ASSISTANT


@pytest.mark.asyncio
async def test_dispatch_inference_preserves_network_error_details_from_http_client():
    client = _make_client()
    raised = NetworkError(
        "HTTP request failed with status 403",
        details={
            "status_code": 403,
            "response": {"error": "inference model override denied"},
        },
    )
    client._http.post = AsyncMock(side_effect=raised)

    with pytest.raises(NetworkError) as exc_info:
        await client.dispatch_inference(_dispatch_request())

    assert exc_info.value is raised
    assert exc_info.value.error_detail.details["status_code"] == 403
    assert exc_info.value.error_detail.details["response"] == {
        "error": "inference model override denied"
    }


@pytest.mark.asyncio
async def test_dispatch_inference_wraps_transport_exception_in_network_error():
    client = _make_client()
    client._http.post = AsyncMock(side_effect=ConnectionError("reset"))

    with pytest.raises(NetworkError) as exc_info:
        await client.dispatch_inference(_dispatch_request())

    assert exc_info.value.component == G8EE_COMPONENT


@pytest.mark.asyncio
async def test_dispatch_inference_ensures_mtls_before_request():
    client = _make_client()
    response = MagicMock()
    response.is_success = True
    response.json.return_value = {"transaction_id": "tx-1"}
    client._http.post = AsyncMock(return_value=response)
    client._ensure_mtls = MagicMock()

    await client.dispatch_inference(_dispatch_request())

    client._ensure_mtls.assert_called_once()
