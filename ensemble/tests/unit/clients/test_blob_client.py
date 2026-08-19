# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

from unittest.mock import AsyncMock, MagicMock, patch

import pytest

from app.clients.blob_client import BlobClient
from app.constants import GatewayAPIPaths
from app.errors import NetworkError
from app.models.settings import TLSConfig

pytestmark = pytest.mark.unit


@pytest.fixture
def mock_listen_settings():
    mock = MagicMock()
    mock.blob_url = "https://localhost:8443"
    return mock


@pytest.fixture
def blob_client(mock_listen_settings):
    with patch("app.services.infra.settings_service.SettingsService") as mock_svc_cls:
        mock_svc = mock_svc_cls.return_value
        mock_settings = MagicMock()
        mock_settings.operator_session_id = "test-session"
        mock_settings.ca_cert_path = None
        mock_settings.client_cert_path = None
        mock_settings.client_key_path = None
        mock_svc.get_local_settings.return_value = mock_settings

        tls_config = TLSConfig(ca_cert_path="/path/to/ca.crt")
        return BlobClient(
            tls_config=tls_config,
            operator_session_id="test-session",
            gateway_settings=mock_listen_settings,
        )


@pytest.mark.asyncio
class TestBlobClient:
    async def test_connect_success(self, blob_client):
        mock_resp = AsyncMock()
        mock_resp.status = 200
        mock_resp.__aenter__.return_value = mock_resp

        mock_session = MagicMock()
        mock_session.get.return_value = mock_resp

        with patch.object(blob_client, "_get_http_session", return_value=mock_session):
            assert await blob_client.connect() is True
            mock_session.get.assert_called_once_with(
                f"https://localhost:8443{GatewayAPIPaths.HEALTH}"
            )

    async def test_connect_failure(self, blob_client):
        mock_resp = AsyncMock()
        mock_resp.status = 500
        mock_resp.__aenter__.return_value = mock_resp

        mock_session = MagicMock()
        mock_session.get.return_value = mock_resp

        with patch.object(blob_client, "_get_http_session", return_value=mock_session):
            assert await blob_client.connect() is False

    async def test_put_blob_success(self, blob_client):
        mock_resp = AsyncMock()
        mock_resp.status = 200
        mock_resp.__aenter__.return_value = mock_resp

        mock_session = MagicMock()
        mock_session.put.return_value = mock_resp

        with patch.object(blob_client, "_get_http_session", return_value=mock_session):
            await blob_client.put_blob("ns", "id", b"data", "image/png")
            mock_session.put.assert_called_once_with(
                f"https://localhost:8443{GatewayAPIPaths.DATA_BLOBS_PREFIX}ns/id",
                data=b"data",
                headers={"Content-Type": "image/png"},
            )

    async def test_put_blob_network_error(self, blob_client):
        mock_resp = AsyncMock()
        mock_resp.status = 400
        mock_resp.text.return_value = "Bad Request"
        mock_resp.__aenter__.return_value = mock_resp

        mock_session = MagicMock()
        mock_session.put.return_value = mock_resp

        with patch.object(blob_client, "_get_http_session", return_value=mock_session):
            with pytest.raises(NetworkError) as excinfo:
                await blob_client.put_blob("ns", "id", b"data", "image/png")
            assert "400" in str(excinfo.value)

    async def test_get_blob_success(self, blob_client):
        mock_resp = AsyncMock()
        mock_resp.status = 200
        mock_resp.read.return_value = b"blob-data"
        mock_resp.__aenter__.return_value = mock_resp

        mock_session = MagicMock()
        mock_session.get.return_value = mock_resp

        with patch.object(blob_client, "_get_http_session", return_value=mock_session):
            result = await blob_client.get_blob("ns", "id")
            assert result == b"blob-data"

    async def test_get_blob_404(self, blob_client):
        mock_resp = AsyncMock()
        mock_resp.status = 404
        mock_resp.__aenter__.return_value = mock_resp

        mock_session = MagicMock()
        mock_session.get.return_value = mock_resp

        with patch.object(blob_client, "_get_http_session", return_value=mock_session):
            result = await blob_client.get_blob("ns", "id")
            assert result is None

    async def test_delete_blob_success(self, blob_client):
        mock_resp = AsyncMock()
        mock_resp.status = 204
        mock_resp.__aenter__.return_value = mock_resp

        mock_session = MagicMock()
        mock_session.delete.return_value = mock_resp

        with patch.object(blob_client, "_get_http_session", return_value=mock_session):
            await blob_client.delete_blob("ns", "id")
            mock_session.delete.assert_called_once_with(
                f"https://localhost:8443{GatewayAPIPaths.DATA_BLOBS_PREFIX}ns/id"
            )

    async def test_delete_namespace_success(self, blob_client):
        mock_resp = AsyncMock()
        mock_resp.status = 200
        mock_resp.text.return_value = '{"deleted": 5}'
        mock_resp.__aenter__.return_value = mock_resp

        mock_session = MagicMock()
        mock_session.delete.return_value = mock_resp

        with patch.object(blob_client, "_get_http_session", return_value=mock_session):
            count = await blob_client.delete_namespace("ns")
            assert count == 5
            mock_session.delete.assert_called_once_with(
                f"https://localhost:8443{GatewayAPIPaths.DATA_BLOBS_PREFIX}ns"
            )
