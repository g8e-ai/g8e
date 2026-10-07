# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

"""InternalHttpClient.fetch_proxy_signing_key: g8ee learns the Gateway's proxy
signing key over its app-mTLS client, never from a file or a shared volume."""

from unittest.mock import AsyncMock, MagicMock

import pytest

from app.clients.http_client import AiohttpResponse
from app.constants.api_paths import GatewayAPIPaths
from app.errors import NetworkError
from app.models.auth import ProxySigningKeyResponse
from app.services.infra.internal_http_client import InternalHttpClient


def _client() -> InternalHttpClient:
    client = InternalHttpClient.__new__(InternalHttpClient)
    client._settings = MagicMock(client_cert_path="/cert.pem", client_key_path="/key.pem")
    client._cached_cert_path = "/cert.pem"
    client._cached_key_path = "/key.pem"
    client._http = MagicMock()
    return client


@pytest.mark.asyncio
async def test_fetch_proxy_signing_key_returns_typed_key_from_exact_path():
    client = _client()
    client._http.get = AsyncMock(
        return_value=AiohttpResponse(
            status=200,
            body=b'{"key_id": "k1", "public_key": "ab12", "algorithm": "ed25519"}',
            headers={},
        )
    )

    key = await client.fetch_proxy_signing_key()

    assert key == ProxySigningKeyResponse(key_id="k1", public_key="ab12", algorithm="ed25519")
    assert client._http.get.call_args.args[0] == GatewayAPIPaths.GATEWAY_PROXY_SIGNING_KEY
    assert GatewayAPIPaths.GATEWAY_PROXY_SIGNING_KEY == "/api/v1/gateway/proxy-signing-key"


@pytest.mark.asyncio
@pytest.mark.parametrize("status", [401, 403, 503])
async def test_fetch_proxy_signing_key_fails_on_non_2xx(status):
    client = _client()
    client._http.get = AsyncMock(
        return_value=AiohttpResponse(status=status, body=b"{}", headers={})
    )

    with pytest.raises(NetworkError):
        await client.fetch_proxy_signing_key()
