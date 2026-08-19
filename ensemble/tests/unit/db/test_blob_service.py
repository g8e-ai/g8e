# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

from unittest.mock import AsyncMock, MagicMock

import pytest

from app.db.blob_service import BlobService

pytestmark = pytest.mark.unit


@pytest.fixture
def mock_client():
    client = MagicMock()
    client.put_blob = AsyncMock()
    client.get_blob = AsyncMock(return_value=b"blob-data")
    client.delete_blob = AsyncMock()
    client.delete_namespace = AsyncMock(return_value=1)
    client.close = AsyncMock()
    return client


@pytest.fixture
def blob_service(mock_client):
    return BlobService(client=mock_client)


class TestBlobService:
    @pytest.mark.asyncio
    async def test_put_blob(self, blob_service, mock_client):
        await blob_service.put_blob("ns", "id", b"data", "image/png")
        mock_client.put_blob.assert_called_once_with("ns", "id", b"data", "image/png")

    @pytest.mark.asyncio
    async def test_get_blob(self, blob_service, mock_client):
        result = await blob_service.get_blob("ns", "id")
        assert result == b"blob-data"
        mock_client.get_blob.assert_called_once_with("ns", "id")

    @pytest.mark.asyncio
    async def test_delete_blob(self, blob_service, mock_client):
        await blob_service.delete_blob("ns", "id")
        mock_client.delete_blob.assert_called_once_with("ns", "id")

    @pytest.mark.asyncio
    async def test_delete_namespace(self, blob_service, mock_client):
        count = await blob_service.delete_namespace("ns")
        assert count == 1
        mock_client.delete_namespace.assert_called_once_with("ns")

    @pytest.mark.asyncio
    async def test_close(self, blob_service, mock_client):
        await blob_service.close()
        mock_client.close.assert_called_once()

    def test_namespace_derivation(self, blob_service):
        # Should delegate to BlobClient.namespace
        ns = blob_service.namespace("inv-123")
        assert ns == "att:inv-123"

    def test_object_key_derivation(self, blob_service):
        # Should delegate to BlobClient.object_key
        key = blob_service.object_key("inv-123", "att-456")
        assert key == "att:inv-123/att-456"
