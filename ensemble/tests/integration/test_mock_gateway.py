# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

"""Smoke tests for the in-memory mock g8e gateway.

Verifies that the real g8ee clients (DBClient, KVCacheClient, BlobClient,
PubSubClient) can connect to and interact with MockGateway.
"""

import asyncio
import pytest
import pytest_asyncio

from app.clients.blob_client import BlobClient
from app.clients.db_client import DBClient
from app.clients.kv_cache_client import KVCacheClient
from app.clients.pubsub_client import PubSubClient
from app.constants import G8EE_COMPONENT

pytestmark = [pytest.mark.integration, pytest.mark.asyncio(loop_scope="session")]


class TestMockGatewayHealth:
    async def test_kv_connect(self, mock_gateway):
        kv = KVCacheClient(
            http_url=mock_gateway.gateway_settings.http_url,
            tls_config=mock_gateway.tls_config,
        )
        try:
            ok = await kv.connect()
            assert ok
            assert kv.is_healthy()
        finally:
            await kv.close()

    async def test_db_connect(self, mock_gateway):
        db = DBClient(
            gateway_settings=mock_gateway.gateway_settings,
            tls_config=mock_gateway.tls_config,
        )
        try:
            ok = await db.connect()
            assert ok
        finally:
            await db.close()

    async def test_blob_connect(self, mock_gateway):
        blob = BlobClient(
            gateway_settings=mock_gateway.gateway_settings,
            tls_config=mock_gateway.tls_config,
        )
        try:
            ok = await blob.connect()
            assert ok
        finally:
            await blob.close()

    async def test_pubsub_connect(self, mock_gateway):
        pubsub = PubSubClient(
            pubsub_url=mock_gateway.gateway_settings.pubsub_url,
            component_name=G8EE_COMPONENT,
            tls_config=mock_gateway.tls_config,
        )
        try:
            ok = await pubsub.connect()
            assert ok
        finally:
            await pubsub.close()


class TestMockGatewayKV:
    async def test_set_and_get(self, mock_gateway):
        kv = KVCacheClient(
            http_url=mock_gateway.gateway_settings.http_url,
            tls_config=mock_gateway.tls_config,
        )
        try:
            await kv.connect()
            await kv.set("test_key", "test_value")
            val = await kv.get("test_key")
            assert val == "test_value"
        finally:
            await kv.close()

    async def test_delete(self, mock_gateway):
        kv = KVCacheClient(
            http_url=mock_gateway.gateway_settings.http_url,
            tls_config=mock_gateway.tls_config,
        )
        try:
            await kv.connect()
            await kv.set("del_key", "del_value")
            count = await kv.delete("del_key")
            assert count == 1
            val = await kv.get("del_key")
            assert val is None
        finally:
            await kv.close()

    async def test_keys_pattern(self, mock_gateway):
        kv = KVCacheClient(
            http_url=mock_gateway.gateway_settings.http_url,
            tls_config=mock_gateway.tls_config,
        )
        try:
            await kv.connect()
            await kv.set("prefix:1", "a")
            await kv.set("prefix:2", "b")
            await kv.set("other:1", "c")
            keys = await kv.keys("prefix:*")
            assert sorted(keys) == ["prefix:1", "prefix:2"]
        finally:
            await kv.close()

    async def test_set_json_and_get_json(self, mock_gateway):
        kv = KVCacheClient(
            http_url=mock_gateway.gateway_settings.http_url,
            tls_config=mock_gateway.tls_config,
        )
        try:
            await kv.connect()
            await kv.set_json("json_key", {"name": "test", "count": 42})
            data = await kv.get_json("json_key")
            assert data == {"name": "test", "count": 42}
        finally:
            await kv.close()


class TestMockGatewayDB:
    async def test_create_and_get_document(self, mock_gateway):
        db = DBClient(
            gateway_settings=mock_gateway.gateway_settings,
            tls_config=mock_gateway.tls_config,
        )
        try:
            await db.connect()
            await db.create_document("test_col", "doc1", {"name": "test", "value": 123})
            result = await db.get_document("test_col", "doc1")
            assert result.success
            assert result.data["name"] == "test"
            assert result.data["value"] == 123
        finally:
            await db.close()

    async def test_update_document_merge(self, mock_gateway):
        db = DBClient(
            gateway_settings=mock_gateway.gateway_settings,
            tls_config=mock_gateway.tls_config,
        )
        try:
            await db.connect()
            await db.create_document("test_col", "doc2", {"a": 1, "b": 2})
            await db.update_document("test_col", "doc2", {"b": 3, "c": 4})
            result = await db.get_document("test_col", "doc2")
            assert result.data["a"] == 1
            assert result.data["b"] == 3
            assert result.data["c"] == 4
        finally:
            await db.close()

    async def test_delete_document(self, mock_gateway):
        db = DBClient(
            gateway_settings=mock_gateway.gateway_settings,
            tls_config=mock_gateway.tls_config,
        )
        try:
            await db.connect()
            await db.create_document("test_col", "doc3", {"x": 1})
            await db.delete_document("test_col", "doc3")
            result = await db.get_document("test_col", "doc3")
            assert result.data is None
        finally:
            await db.close()

    async def test_query_collection(self, mock_gateway):
        db = DBClient(
            gateway_settings=mock_gateway.gateway_settings,
            tls_config=mock_gateway.tls_config,
        )
        try:
            await db.connect()
            await db.create_document("query_col", "d1", {"status": "active", "n": 1})
            await db.create_document("query_col", "d2", {"status": "active", "n": 2})
            await db.create_document("query_col", "d3", {"status": "inactive", "n": 3})
            result = await db.query_collection(
                "query_col",
                field_filters=[{"field": "status", "op": "==", "value": "active"}],
                order_by={},
                limit=0,
                select_fields=[],
            )
            assert len(result.data) == 2
        finally:
            await db.close()


class TestMockGatewayBlob:
    async def test_put_and_get_blob(self, mock_gateway):
        blob = BlobClient(
            gateway_settings=mock_gateway.gateway_settings,
            tls_config=mock_gateway.tls_config,
        )
        try:
            await blob.connect()
            data = b"\x89PNG fake image data"
            await blob.put_blob("ns1", "blob1", data, "image/png")
            result = await blob.get_blob("ns1", "blob1")
            assert result == data
        finally:
            await blob.close()

    async def test_delete_blob(self, mock_gateway):
        blob = BlobClient(
            gateway_settings=mock_gateway.gateway_settings,
            tls_config=mock_gateway.tls_config,
        )
        try:
            await blob.connect()
            await blob.put_blob("ns1", "blob2", b"data", "application/octet-stream")
            await blob.delete_blob("ns1", "blob2")
            result = await blob.get_blob("ns1", "blob2")
            assert result is None
        finally:
            await blob.close()

    async def test_delete_namespace(self, mock_gateway):
        blob = BlobClient(
            gateway_settings=mock_gateway.gateway_settings,
            tls_config=mock_gateway.tls_config,
        )
        try:
            await blob.connect()
            await blob.put_blob("ns2", "b1", b"x", "application/octet-stream")
            await blob.put_blob("ns2", "b2", b"y", "application/octet-stream")
            count = await blob.delete_namespace("ns2")
            assert count == 2
        finally:
            await blob.close()


class TestMockGatewayPubSub:
    async def test_subscribe_and_publish(self, mock_gateway):
        pubsub = PubSubClient(
            pubsub_url=mock_gateway.gateway_settings.pubsub_url,
            component_name=G8EE_COMPONENT,
            tls_config=mock_gateway.tls_config,
        )
        try:
            await pubsub.connect()

            received: list[str] = []
            done = asyncio.Event()

            async def handler(channel: str, data):
                received.append(data.decode() if isinstance(data, bytes) else str(data))
                done.set()

            pubsub.on_channel_message("test_channel", handler)
            await pubsub.subscribe("test_channel")

            await asyncio.sleep(0.1)
            await pubsub.publish("test_channel", b"hello world")

            await asyncio.wait_for(done.wait(), timeout=5.0)
            assert "hello world" in received
        finally:
            await pubsub.close()

    async def test_psubscribe_pattern(self, mock_gateway):
        pubsub = PubSubClient(
            pubsub_url=mock_gateway.gateway_settings.pubsub_url,
            component_name=G8EE_COMPONENT,
            tls_config=mock_gateway.tls_config,
        )
        try:
            await pubsub.connect()

            received: list[tuple[str, str]] = []
            done = asyncio.Event()

            async def handler(pattern: str, channel: str, data):
                received.append((channel, data.decode() if isinstance(data, bytes) else str(data)))
                done.set()

            pubsub.on_pmessage("events:*", handler)
            await pubsub.psubscribe("events:*")

            await asyncio.sleep(0.1)
            await pubsub.publish("events:foo", b"event data")

            await asyncio.wait_for(done.wait(), timeout=5.0)
            assert len(received) == 1
            assert received[0][0] == "events:foo"
            assert received[0][1] == "event data"
        finally:
            await pubsub.close()
