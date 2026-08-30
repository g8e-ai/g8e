# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

"""Regression tests for InternalHttpClient.validate_operator_session.

The Gateway-authoritative session-validation path replaced g8ee's local
operator_sessions projection. g8ee now sends a typed
OperatorSessionValidationRequest to POST /api/v1/operators/validate through
the app-mTLS InternalHttpClient and constructs the authenticated user only
after the Gateway confirms the exact operator_session_id, cli_session_id, and
user_id tuple.

These tests cover the typed request shape, success parsing, non-2xx
rejection, malformed-response rejection, user-mismatch rejection, and the
exact Gateway API path. They mock the underlying HTTPClient.post so no network
or TLS machinery is required (Tier 1).
"""

from unittest.mock import AsyncMock, MagicMock

import pytest

from app.clients.http_client import AiohttpResponse
from app.constants.api_paths import GatewayAPIPaths
from app.models.auth import OperatorSessionValidationRequest, OperatorSessionValidationResponse
from app.services.infra.internal_http_client import InternalHttpClient


def _make_response(status: int, body: bytes) -> AiohttpResponse:
    return AiohttpResponse(status=status, body=body, headers={})


def _make_client() -> InternalHttpClient:
    """Build an InternalHttpClient without running the real HTTPClient constructor.

    The real constructor wires aiohttp TLS transports; Tier 1 tests stub the
    underlying ``_http`` so no transport is needed.
    """
    settings = MagicMock()
    settings.component_urls.client_url = "https://gateway:8443"
    settings.ca_cert_path = "/ca.pem"
    settings.client_cert_path = "/cert.pem"
    settings.client_key_path = "/key.pem"
    settings.auth.internal_api_key = ""

    # Bypass __init__ to avoid constructing a real HTTPClient with aiohttp.
    client = InternalHttpClient.__new__(InternalHttpClient)
    client.client_url = settings.component_urls.client_url
    client._settings = settings
    client._cached_cert_path = settings.client_cert_path
    client._cached_key_path = settings.client_key_path
    client._http = MagicMock()
    client._http.refresh_mtls_credentials = MagicMock()
    return client


@pytest.mark.asyncio
async def test_validate_operator_session_success_returns_typed_response():
    client = _make_client()
    client._http.post = AsyncMock(return_value=_make_response(
        200,
        b'{"valid": true, "operator_id": "op-1", "user_id": "user-1"}',
    ))

    result = await client.validate_operator_session("op-session", "cli-session", "user-1")

    assert result is not None
    assert result.valid is True
    assert result.operator_id == "op-1"
    assert result.user_id == "user-1"


@pytest.mark.asyncio
async def test_validate_operator_session_posts_typed_request_to_exact_path():
    client = _make_client()
    client._http.post = AsyncMock(return_value=_make_response(
        200,
        b'{"valid": true, "operator_id": "op-1", "user_id": "user-1"}',
    ))

    await client.validate_operator_session("op-session", "cli-session", "user-1")

    client._http.post.assert_awaited_once()
    call_args = client._http.post.call_args
    assert call_args.args[0] == GatewayAPIPaths.OPERATORS_VALIDATE
    sent = call_args.kwargs["json_data"]
    assert isinstance(sent, OperatorSessionValidationRequest)
    assert sent.operator_session_id == "op-session"
    assert sent.cli_session_id == "cli-session"
    assert sent.user_id == "user-1"


@pytest.mark.asyncio
async def test_validate_operator_session_non_2xx_returns_none():
    client = _make_client()
    client._http.post = AsyncMock(return_value=_make_response(401, b'{"error": "unauthorized"}'))

    result = await client.validate_operator_session("op-session", "cli-session", "user-1")

    assert result is None


@pytest.mark.asyncio
async def test_validate_operator_session_forbidden_returns_none():
    client = _make_client()
    client._http.post = AsyncMock(return_value=_make_response(403, b'{"error": "forbidden"}'))

    result = await client.validate_operator_session("op-session", "cli-session", "user-1")

    assert result is None


@pytest.mark.asyncio
async def test_validate_operator_session_malformed_response_returns_none():
    client = _make_client()
    client._http.post = AsyncMock(return_value=_make_response(200, b'not-json'))

    with pytest.raises(Exception):
        await client.validate_operator_session("op-session", "cli-session", "user-1")


@pytest.mark.asyncio
async def test_validate_operator_session_gateway_says_invalid_returns_none():
    client = _make_client()
    client._http.post = AsyncMock(return_value=_make_response(
        200,
        b'{"valid": false, "operator_id": "", "user_id": ""}',
    ))

    result = await client.validate_operator_session("op-session", "cli-session", "user-1")

    assert result is None


@pytest.mark.asyncio
async def test_validate_operator_session_user_mismatch_returns_none():
    client = _make_client()
    client._http.post = AsyncMock(return_value=_make_response(
        200,
        b'{"valid": true, "operator_id": "op-1", "user_id": "different-user"}',
    ))

    result = await client.validate_operator_session("op-session", "cli-session", "user-1")

    assert result is None
