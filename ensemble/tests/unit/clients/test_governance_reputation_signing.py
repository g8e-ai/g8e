# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

from unittest.mock import AsyncMock, MagicMock

import pytest

from app.clients.governance_client import GovernanceClient
from app.constants import GatewayAPIPaths
from app.errors import NetworkError, ValidationError
from app.models.reputation import ReputationSignRequest
from app.models.settings import GatewaySettings, TLSConfig

pytestmark = pytest.mark.unit


@pytest.mark.asyncio
async def test_reputation_signing_uses_mtls_and_typed_gateway_response():
    client = GovernanceClient(
        tls_config=TLSConfig(client_cert_path="app.pem", client_key_path="app.key"),
        gateway_settings=GatewaySettings(http_url="https://gateway.test"),
    )
    request = ReputationSignRequest(
        merkle_root="a" * 64, prev_root="0" * 64, tribunal_command_id="verdict-1"
    )
    response = MagicMock()
    response.status = 200
    response.json = AsyncMock(return_value={"signature": "b" * 64})
    session = MagicMock()
    session.post.return_value.__aenter__ = AsyncMock(return_value=response)
    session.post.return_value.__aexit__ = AsyncMock(return_value=False)
    client._get_http_session = AsyncMock(return_value=session)
    signed = await client.sign_reputation_commitment(request)
    assert signed.signature == "b" * 64
    session.post.assert_called_once_with(
        "https://gateway.test" + GatewayAPIPaths.GATEWAY_REPUTATION_SIGN,
        json=request.model_dump(mode="json"),
    )

    response.status = 503
    with pytest.raises(NetworkError):
        await client.sign_reputation_commitment(request)


@pytest.mark.asyncio
async def test_reputation_signing_refuses_missing_app_credentials():
    client = GovernanceClient(gateway_settings=GatewaySettings(http_url="https://gateway.test"))
    request = ReputationSignRequest(
        merkle_root="a" * 64, prev_root="0" * 64, tribunal_command_id="verdict-1"
    )
    with pytest.raises(ValidationError):
        await client.sign_reputation_commitment(request)
