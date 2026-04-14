# Copyright (c) 2026 Lateralus Labs, LLC.
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

import aiohttp
import pytest
from unittest.mock import AsyncMock, MagicMock, patch

from app.clients.blob_client import BlobClient
from app.errors import DatabaseError, NetworkError

pytestmark = pytest.mark.unit

@pytest.fixture
def mock_listen_settings():
    mock = MagicMock()
    mock.blob_url = "https://g8es:9000"
    return mock

@pytest.fixture
def blob_client(mock_listen_settings):
    with patch("app.services.infra.settings_service.SettingsService") as mock_svc_cls:
        mock_svc = mock_svc_cls.return_value
        mock_svc.get_local_settings.return_value.auth.internal_auth_token = "test-token"
        
        client = BlobClient(
            ca_cert_path="/path/to/ca.crt",
            internal_auth_token="test-token",
            listen_settings=mock_listen_settings
        )
        return client

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
            mock_session.get.assert_called_once_with("https://g8es:9000/health")

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
                "https://g8es:9000/blob/ns/id",
                data=b"data",
                headers={"Content-Type": "image/png"}
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
            mock_session.delete.assert_called_once_with("https://g8es:9000/blob/ns/id")

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
            mock_session.delete.assert_called_once_with("https://g8es:9000/blob/ns")
