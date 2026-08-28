# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

"""In-memory mock g8e gateway for integration tests.

Reconstructs the subset of the g8e Gateway that g8ee clients talk to, so
integration tests can run without a real Go gateway binary.

Implements:
  - HTTP endpoints: /api/v1/health, /api/v1/data/{collection}/{id},
    /api/v1/kv/{key}, /api/v1/blobs/{ns}/{id}
  - WebSocket pub/sub at /api/v1/pubsub/stream using the same protobuf wire
    protocol (PubSubMessage / PubSubEvent) as the real GatewayWebSocketHandler.

TLS:
  PubSubClient unconditionally forces wss://, so the mock gateway starts
  an aiohttp HTTPS server with a self-signed certificate.  The generated
  CA cert path is exposed via ``MockGateway.ca_cert_path`` so that
  DBClient, KVCacheClient, BlobClient, and PubSubClient can all connect
  with a matching TLSConfig.

Usage (pytest fixture)::

    @pytest_asyncio.fixture
    async def mock_gateway():
        gw = MockGateway()
        await gw.start()
        yield gw
        await gw.stop()

    async def test_kv(mock_gateway):
        settings = mock_gateway.gateway_settings
        tls = mock_gateway.tls_config
        kv = KVCacheClient(http_url=settings.http_url, tls_config=tls)
        await kv.connect()
        assert kv.is_healthy()
        await kv.close()
"""

from __future__ import annotations

import asyncio
import datetime
import fnmatch
import json
import logging
import os
import ssl
import tempfile
from typing import Any
from urllib.parse import quote, unquote

import aiohttp
from aiohttp import web

from app.constants import GatewayAPIPaths, PubSubAction, PubSubWireEventType
from app.models.settings import GatewaySettings, TLSConfig
from g8e.pubsub.v1.pubsub_pb2 import PubSubEvent, PubSubMessage

logger = logging.getLogger(__name__)


# ---------------------------------------------------------------------------
# TLS certificate generation
# ---------------------------------------------------------------------------


def _generate_self_signed_cert(tmpdir: str) -> tuple[str, str, str]:
    """Generate a self-signed CA + server cert in *tmpdir*.

    Returns (ca_cert_path, server_cert_path, server_key_path).
    """
    from cryptography import x509
    from cryptography.hazmat.primitives import hashes, serialization
    from cryptography.hazmat.primitives.asymmetric import ec
    from cryptography.x509.oid import NameOID

    now = datetime.datetime.now(datetime.timezone.utc)

    # --- CA ---
    ca_key = ec.generate_private_key(ec.SECP256R1())
    ca_subject = x509.Name([
        x509.NameAttribute(NameOID.COMMON_NAME, "g8e-test-mock-ca"),
    ])
    ca_cert = (
        x509.CertificateBuilder()
        .subject_name(ca_subject)
        .issuer_name(ca_subject)
        .public_key(ca_key.public_key())
        .serial_number(x509.random_serial_number())
        .not_valid_before(now - datetime.timedelta(days=1))
        .not_valid_after(now + datetime.timedelta(days=365))
        .add_extension(
            x509.BasicConstraints(ca=True, path_length=None),
            critical=True,
        )
        .sign(ca_key, hashes.SHA256())
    )

    ca_path = os.path.join(tmpdir, "ca.pem")
    with open(ca_path, "wb") as f:
        f.write(ca_cert.public_bytes(serialization.Encoding.PEM))

    # --- Server cert signed by CA ---
    server_key = ec.generate_private_key(ec.SECP256R1())
    server_subject = x509.Name([
        x509.NameAttribute(NameOID.COMMON_NAME, "localhost"),
    ])
    server_cert = (
        x509.CertificateBuilder()
        .subject_name(server_subject)
        .issuer_name(ca_subject)
        .public_key(server_key.public_key())
        .serial_number(x509.random_serial_number())
        .not_valid_before(now - datetime.timedelta(days=1))
        .not_valid_after(now + datetime.timedelta(days=365))
        .add_extension(
            x509.SubjectAlternativeName([
                x509.DNSName("localhost"),
                x509.IPAddress(__import__("ipaddress").ip_address("127.0.0.1")),
            ]),
            critical=False,
        )
        .sign(ca_key, hashes.SHA256())
    )

    cert_path = os.path.join(tmpdir, "server.pem")
    with open(cert_path, "wb") as f:
        f.write(server_cert.public_bytes(serialization.Encoding.PEM))

    key_path = os.path.join(tmpdir, "server.key")
    with open(key_path, "wb") as f:
        f.write(
            server_key.private_bytes(
                serialization.Encoding.PEM,
                serialization.PrivateFormat.TraditionalOpenSSL,
                serialization.NoEncryption(),
            )
        )

    return ca_path, cert_path, key_path


# ---------------------------------------------------------------------------
# In-memory stores
# ---------------------------------------------------------------------------


class _KVStore:
    """Dict-backed KV store matching the operator /kv API surface."""

    def __init__(self) -> None:
        self._store: dict[str, str] = {}

    async def get(self, key: str) -> str | None:
        return self._store.get(key)

    async def set(self, key: str, value: str, ttl: int = 0) -> None:
        self._store[key] = value

    async def delete(self, key: str) -> bool:
        return self._store.pop(key, None) is not None

    async def expire(self, key: str, ttl: int) -> bool:
        return key in self._store

    async def ttl(self, key: str) -> int:
        if key not in self._store:
            return -2
        return -1

    async def keys(self, pattern: str = "*") -> list[str]:
        return [k for k in self._store if fnmatch.fnmatch(k, pattern)]

    async def delete_pattern(self, pattern: str) -> int:
        keys = [k for k in self._store if fnmatch.fnmatch(k, pattern)]
        for k in keys:
            del self._store[k]
        return len(keys)


class _DocStore:
    """Dict-backed document store matching the operator /db API surface."""

    def __init__(self) -> None:
        self._collections: dict[str, dict[str, dict[str, Any]]] = {}

    def _col(self, collection: str) -> dict[str, dict[str, Any]]:
        return self._collections.setdefault(collection, {})

    async def get(self, collection: str, doc_id: str) -> dict[str, Any] | None:
        return self._col(collection).get(doc_id)

    async def put(self, collection: str, doc_id: str, data: dict[str, Any]) -> None:
        self._col(collection)[doc_id] = dict(data)

    async def patch(self, collection: str, doc_id: str, data: dict[str, Any]) -> None:
        col = self._col(collection)
        existing = col.get(doc_id, {})
        existing.update(data)
        col[doc_id] = existing

    async def delete(self, collection: str, doc_id: str) -> bool:
        return self._col(collection).pop(doc_id, None) is not None

    async def query(
        self,
        collection: str,
        filters: list[dict[str, Any]] | None = None,
        order_by: str | None = None,
        limit: int = 100,
    ) -> list[dict[str, Any]]:
        docs = list(self._col(collection).values())
        for f in filters or []:
            field = f.get("field")
            op = f.get("op", "==")
            value = f.get("value")
            if op == "==":
                docs = [d for d in docs if d.get(field) == value]
        if order_by:
            parts = order_by.split()
            field = parts[0]
            reverse = len(parts) > 1 and parts[1].upper() == "DESC"
            docs.sort(key=lambda d: str(d.get(field, "")), reverse=reverse)
        if limit:
            docs = docs[:limit]
        return docs


class _BlobStore:
    """Dict-backed blob store matching the operator /blob API surface."""

    def __init__(self) -> None:
        self._store: dict[str, dict[str, bytes]] = {}

    async def put(self, namespace: str, blob_id: str, data: bytes) -> None:
        self._store.setdefault(namespace, {})[blob_id] = data

    async def get(self, namespace: str, blob_id: str) -> bytes | None:
        return self._store.get(namespace, {}).get(blob_id)

    async def delete(self, namespace: str, blob_id: str) -> bool:
        return self._store.get(namespace, {}).pop(blob_id, None) is not None

    async def delete_namespace(self, namespace: str) -> int:
        ns = self._store.pop(namespace, {})
        return len(ns)


# ---------------------------------------------------------------------------
# WebSocket pub/sub broker
# ---------------------------------------------------------------------------


class _PubSubBroker:
    """In-process pub/sub broker matching the g8e GatewayWebSocketHandler wire protocol."""

    def __init__(self) -> None:
        self._channel_subs: dict[str, set[web.WebSocketResponse]] = {}
        self._pattern_subs: dict[str, set[web.WebSocketResponse]] = {}
        self._lock = asyncio.Lock()

    async def subscribe(self, channel: str, ws: web.WebSocketResponse) -> None:
        async with self._lock:
            self._channel_subs.setdefault(channel, set()).add(ws)

    async def psubscribe(self, pattern: str, ws: web.WebSocketResponse) -> None:
        async with self._lock:
            self._pattern_subs.setdefault(pattern, set()).add(ws)

    async def unsubscribe(self, channel: str, ws: web.WebSocketResponse) -> None:
        async with self._lock:
            subs = self._channel_subs.get(channel)
            if subs:
                subs.discard(ws)
                if not subs:
                    del self._channel_subs[channel]

    async def punsubscribe(self, pattern: str, ws: web.WebSocketResponse) -> None:
        async with self._lock:
            subs = self._pattern_subs.get(pattern)
            if subs:
                subs.discard(ws)
                if not subs:
                    del self._pattern_subs[pattern]

    async def publish(self, channel: str, data: bytes) -> int:
        count = 0

        # Direct channel subscribers
        async with self._lock:
            channel_subs = list(self._channel_subs.get(channel, set()))

        for ws in channel_subs:
            event = PubSubEvent(
                type=PubSubWireEventType.MESSAGE,
                channel=channel,
                data=data,
            )
            try:
                await ws.send_bytes(event.SerializeToString())
                count += 1
            except Exception:
                logger.debug("[MOCK-GATEWAY] Failed to send to channel subscriber")

        # Pattern subscribers
        async with self._lock:
            patterns = list(self._pattern_subs.items())

        for pattern, subs in patterns:
            if fnmatch.fnmatch(channel, pattern):
                for ws in list(subs):
                    event = PubSubEvent(
                        type=PubSubWireEventType.PMESSAGE,
                        channel=channel,
                        pattern=pattern,
                        data=data,
                    )
                    try:
                        await ws.send_bytes(event.SerializeToString())
                        count += 1
                    except Exception:
                        logger.debug("[MOCK-GATEWAY] Failed to send to pattern subscriber")

        return count

    async def send_ack(self, ws: web.WebSocketResponse, target: str) -> None:
        event = PubSubEvent(
            type=PubSubWireEventType.SUBSCRIBED,
            channel=target,
        )
        await ws.send_bytes(event.SerializeToString())

    async def remove_ws(self, ws: web.WebSocketResponse) -> None:
        async with self._lock:
            for channel, subs in list(self._channel_subs.items()):
                subs.discard(ws)
                if not subs:
                    del self._channel_subs[channel]
            for pattern, subs in list(self._pattern_subs.items()):
                subs.discard(ws)
                if not subs:
                    del self._pattern_subs[pattern]


# ---------------------------------------------------------------------------
# MockGateway
# ---------------------------------------------------------------------------


class MockGateway:
    """In-memory mock g8e gateway for integration tests.

    Starts an aiohttp HTTPS server with a self-signed certificate on a random
    port.  Implements the HTTP endpoints (health, db, kv, blob) and the
    WebSocket pub/sub protocol (/ws/pubsub) that g8ee clients expect.

    Attributes:
        ca_cert_path: Path to the generated CA certificate (for client TLSConfig).
        gateway_settings: GatewaySettings with http_url, pubsub_url, blob_url
            pointing at the mock server.
        tls_config: TLSConfig with ca_cert_path set (no client cert required).
        kv: Direct access to the in-memory KV store (for test assertions).
        db: Direct access to the in-memory document store.
        blob: Direct access to the in-memory blob store.
    """

    def __init__(self) -> None:
        self._tmpdir = tempfile.mkdtemp(prefix="g8e-mock-gw-")
        ca_path, cert_path, key_path = _generate_self_signed_cert(self._tmpdir)
        self._ca_cert_path = ca_path
        self._cert_path = cert_path
        self._key_path = key_path

        self.kv = _KVStore()
        self.db = _DocStore()
        self.blob = _BlobStore()
        self._broker = _PubSubBroker()

        self._runner: web.AppRunner | None = None
        self._site: web.TCPSite | None = None
        self._port: int = 0

    @property
    def ca_cert_path(self) -> str:
        return self._ca_cert_path

    @property
    def tls_config(self) -> TLSConfig:
        return TLSConfig(ca_cert_path=self._ca_cert_path)

    @property
    def gateway_settings(self) -> GatewaySettings:
        base = f"https://localhost:{self._port}"
        return GatewaySettings(
            http_url=base,
            pubsub_url=f"wss://localhost:{self._port}",
            blob_url=base,
        )

    @property
    def port(self) -> int:
        return self._port

    @property
    def base_url(self) -> str:
        return f"https://localhost:{self._port}"

    # ------------------------------------------------------------------
    # Lifecycle
    # ------------------------------------------------------------------

    async def start(self) -> None:
        app = web.Application()
        self._register_routes(app)
        self._runner = web.AppRunner(app)
        await self._runner.setup()

        ssl_ctx = ssl.SSLContext(ssl.PROTOCOL_TLS_SERVER)
        ssl_ctx.load_cert_chain(self._cert_path, self._key_path)

        self._site = web.TCPSite(
            self._runner,
            "127.0.0.1",
            0,
            ssl_context=ssl_ctx,
        )
        await self._site.start()

        # Discover the actual port
        sockets = self._site._server.sockets
        self._port = sockets[0].getsockname()[1]
        logger.info("[MOCK-GATEWAY] Listening on https://localhost:%d", self._port)

    async def stop(self) -> None:
        if self._runner:
            await self._runner.cleanup()
            self._runner = None
            self._site = None
        # Clean up temp dir
        import shutil

        shutil.rmtree(self._tmpdir, ignore_errors=True)

    # ------------------------------------------------------------------
    # Route registration
    # ------------------------------------------------------------------

    def _register_routes(self, app: web.Application) -> None:
        app.router.add_get(GatewayAPIPaths.HEALTH, self._handle_health)
        app.router.add_get("/health/live", self._handle_health)

        # DB endpoints (GatewayAPIPaths.DATA_DB = "/api/v1/data/")
        app.router.add_get(GatewayAPIPaths.DATA_DB + "{collection}/{doc_id}", self._handle_db_get)
        app.router.add_put(GatewayAPIPaths.DATA_DB + "{collection}/{doc_id}", self._handle_db_put)
        app.router.add_patch(GatewayAPIPaths.DATA_DB + "{collection}/{doc_id}", self._handle_db_patch)
        app.router.add_delete(GatewayAPIPaths.DATA_DB + "{collection}/{doc_id}", self._handle_db_delete)
        app.router.add_post(GatewayAPIPaths.DATA_DB + "{collection}/_query", self._handle_db_query)

        # KV endpoints (GatewayAPIPaths.KV_PREFIX = "/api/v1/kv/")
        app.router.add_get(GatewayAPIPaths.KV_PREFIX + "{key}", self._handle_kv_get)
        app.router.add_put(GatewayAPIPaths.KV_PREFIX + "{key}", self._handle_kv_set)
        app.router.add_delete(GatewayAPIPaths.KV_PREFIX + "{key}", self._handle_kv_delete)
        app.router.add_put(GatewayAPIPaths.KV_PREFIX + "{key}/_expire", self._handle_kv_expire)
        app.router.add_get(GatewayAPIPaths.KV_PREFIX + "{key}/_ttl", self._handle_kv_ttl)
        app.router.add_post(GatewayAPIPaths.KV_PREFIX + "_keys", self._handle_kv_keys)
        app.router.add_post(GatewayAPIPaths.KV_PREFIX + "_delete_pattern", self._handle_kv_delete_pattern)

        # Blob endpoints (GatewayAPIPaths.DATA_BLOBS_PREFIX = "/api/v1/blobs/")
        app.router.add_put(GatewayAPIPaths.DATA_BLOBS_PREFIX + "{namespace}/{blob_id}", self._handle_blob_put)
        app.router.add_get(GatewayAPIPaths.DATA_BLOBS_PREFIX + "{namespace}/{blob_id}", self._handle_blob_get)
        app.router.add_delete(GatewayAPIPaths.DATA_BLOBS_PREFIX + "{namespace}/{blob_id}", self._handle_blob_delete)
        app.router.add_delete(GatewayAPIPaths.DATA_BLOBS_PREFIX + "{namespace}", self._handle_blob_delete_ns)

        # WebSocket pub/sub (GatewayAPIPaths.PUBSUB_STREAM = "/api/v1/pubsub/stream")
        app.router.add_get(GatewayAPIPaths.PUBSUB_STREAM, self._handle_ws_pubsub)

    # ------------------------------------------------------------------
    # HTTP handlers - Health
    # ------------------------------------------------------------------

    async def _handle_health(self, request: web.Request) -> web.Response:
        return web.json_response({"status": "ok"})

    # ------------------------------------------------------------------
    # HTTP handlers - DB
    # ------------------------------------------------------------------

    async def _handle_db_get(self, request: web.Request) -> web.Response:
        collection = unquote(request.match_info["collection"])
        doc_id = unquote(request.match_info["doc_id"])
        doc = await self.db.get(collection, doc_id)
        if doc is None:
            return web.json_response({}, status=404)
        return web.json_response(doc)

    async def _handle_db_put(self, request: web.Request) -> web.Response:
        collection = unquote(request.match_info["collection"])
        doc_id = unquote(request.match_info["doc_id"])
        data = await request.json()
        await self.db.put(collection, doc_id, data)
        return web.json_response({"success": True}, status=200)

    async def _handle_db_patch(self, request: web.Request) -> web.Response:
        collection = unquote(request.match_info["collection"])
        doc_id = unquote(request.match_info["doc_id"])
        data = await request.json()
        await self.db.patch(collection, doc_id, data)
        return web.json_response({"success": True}, status=200)

    async def _handle_db_delete(self, request: web.Request) -> web.Response:
        collection = unquote(request.match_info["collection"])
        doc_id = unquote(request.match_info["doc_id"])
        await self.db.delete(collection, doc_id)
        return web.json_response({"success": True}, status=200)

    async def _handle_db_query(self, request: web.Request) -> web.Response:
        collection = unquote(request.match_info["collection"])
        body = await request.json()
        docs = await self.db.query(
            collection,
            filters=body.get("filters"),
            order_by=body.get("order_by"),
            limit=body.get("limit", 100),
        )
        return web.json_response(docs)

    # ------------------------------------------------------------------
    # HTTP handlers - KV
    # ------------------------------------------------------------------

    async def _handle_kv_get(self, request: web.Request) -> web.Response:
        key = unquote(request.match_info["key"])
        value = await self.kv.get(key)
        if value is None:
            return web.json_response({"error": "not found"}, status=404)
        return web.json_response({"value": value})

    async def _handle_kv_set(self, request: web.Request) -> web.Response:
        key = unquote(request.match_info["key"])
        body = await request.json()
        await self.kv.set(key, body.get("value", ""), body.get("ttl", 0))
        return web.json_response({"success": True})

    async def _handle_kv_delete(self, request: web.Request) -> web.Response:
        key = unquote(request.match_info["key"])
        await self.kv.delete(key)
        return web.json_response({"success": True})

    async def _handle_kv_expire(self, request: web.Request) -> web.Response:
        key = unquote(request.match_info["key"])
        body = await request.json()
        ok = await self.kv.expire(key, body.get("ttl", 0))
        return web.json_response({"success": ok})

    async def _handle_kv_ttl(self, request: web.Request) -> web.Response:
        key = unquote(request.match_info["key"])
        ttl = await self.kv.ttl(key)
        return web.json_response({"ttl": ttl})

    async def _handle_kv_keys(self, request: web.Request) -> web.Response:
        body = await request.json()
        keys = await self.kv.keys(body.get("pattern", "*"))
        return web.json_response({"keys": keys})

    async def _handle_kv_delete_pattern(self, request: web.Request) -> web.Response:
        body = await request.json()
        count = await self.kv.delete_pattern(body.get("pattern", "*"))
        return web.json_response({"deleted": count})

    # ------------------------------------------------------------------
    # HTTP handlers - Blob
    # ------------------------------------------------------------------

    async def _handle_blob_put(self, request: web.Request) -> web.Response:
        namespace = unquote(request.match_info["namespace"])
        blob_id = unquote(request.match_info["blob_id"])
        data = await request.read()
        await self.blob.put(namespace, blob_id, data)
        return web.json_response({"success": True}, status=201)

    async def _handle_blob_get(self, request: web.Request) -> web.StreamResponse:
        namespace = unquote(request.match_info["namespace"])
        blob_id = unquote(request.match_info["blob_id"])
        data = await self.blob.get(namespace, blob_id)
        if data is None:
            return web.json_response({"error": "not found"}, status=404)
        resp = web.StreamResponse(status=200)
        resp.content_type = "application/octet-stream"
        resp.content_length = len(data)
        await resp.prepare(request)
        await resp.write(data)
        return resp

    async def _handle_blob_delete(self, request: web.Request) -> web.Response:
        namespace = unquote(request.match_info["namespace"])
        blob_id = unquote(request.match_info["blob_id"])
        await self.blob.delete(namespace, blob_id)
        return web.json_response({"success": True})

    async def _handle_blob_delete_ns(self, request: web.Request) -> web.Response:
        namespace = unquote(request.match_info["namespace"])
        count = await self.blob.delete_namespace(namespace)
        return web.json_response({"deleted": count})

    # ------------------------------------------------------------------
    # WebSocket handler - Pub/Sub
    # ------------------------------------------------------------------

    async def _handle_ws_pubsub(self, request: web.Request) -> web.WebSocketResponse:
        ws = web.WebSocketResponse(max_msg_size=0)
        await ws.prepare(request)

        try:
            async for msg in ws:
                if msg.type == aiohttp.WSMsgType.BINARY:
                    try:
                        pubsub_msg = PubSubMessage()
                        pubsub_msg.ParseFromString(msg.data)
                    except Exception:
                        logger.warning("[MOCK-GATEWAY] Failed to parse PubSubMessage")
                        continue

                    action = pubsub_msg.action
                    channel = pubsub_msg.channel
                    data = pubsub_msg.data

                    if action == PubSubAction.SUBSCRIBE:
                        await self._broker.subscribe(channel, ws)
                        await self._broker.send_ack(ws, channel)

                    elif action == PubSubAction.PSUBSCRIBE:
                        await self._broker.psubscribe(channel, ws)
                        await self._broker.send_ack(ws, channel)

                    elif action == PubSubAction.UNSUBSCRIBE:
                        await self._broker.unsubscribe(channel, ws)
                        # Also try pattern unsubscribe
                        await self._broker.punsubscribe(channel, ws)

                    elif action == PubSubAction.PUBLISH:
                        await self._broker.publish(channel, data)

                elif msg.type in (aiohttp.WSMsgType.CLOSED, aiohttp.WSMsgType.ERROR):
                    break
        finally:
            await self._broker.remove_ws(ws)

        return ws
