# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

"""Shared builders for the signed browser-proxy stamp tests: a stand-in for the
Gateway's key endpoint, a controllable clock, and a signer that produces the
headers and request the Gateway would forward."""

import asyncio
import hashlib
import json
from pathlib import Path

from fastapi import Request
from nacl.signing import SigningKey

from app.constants import (
    X_PROXY_CLI_SESSION_ID,
    X_PROXY_ISSUED_AT,
    X_PROXY_KEY_ID,
    X_PROXY_NONCE,
    X_PROXY_ORGANIZATION_ID,
    X_PROXY_SIGNATURE,
    X_PROXY_USER_EMAIL,
    X_PROXY_USER_ID,
    X_PROXY_WEB_SESSION_ID,
)
from app.models.auth import ProxySigningKeyResponse
from app.services.auth.proxy_stamp import BrowserProxyStamp, ProxyStampVerifier

VECTORS = json.loads(
    (Path(__file__).parents[5] / "protocol" / "conformance" / "browser_proxy_stamp_vectors.json").read_text()
)
NOW = 1_790_000_000.0
KEY_ID = "gateway-key-1"
BODY = b'{"message":"hi"}'
PATH = "/api/v1/chat/send"
QUERY = "stream=1"


def make_key(seed_byte: int = 1) -> SigningKey:
    return SigningKey(bytes([seed_byte]) * 32)


def key_response(key: SigningKey, key_id: str = KEY_ID) -> ProxySigningKeyResponse:
    return ProxySigningKeyResponse(key_id=key_id, public_key=bytes(key.verify_key).hex(), algorithm="ed25519")


class KeySource:
    """Stands in for the Gateway's proxy-signing-key endpoint.

    ``delay`` suspends each fetch so concurrent verifications overlap it, the
    way browser requests do against a cold g8ee.
    """

    def __init__(self, *responses: ProxySigningKeyResponse | Exception, delay: float = 0.0):
        self.responses = list(responses)
        self.calls = 0
        self.delay = delay

    async def __call__(self) -> ProxySigningKeyResponse:
        self.calls += 1
        item = self.responses[min(self.calls - 1, len(self.responses) - 1)]
        if self.delay:
            await asyncio.sleep(self.delay)
        if isinstance(item, Exception):
            raise item
        return item


class Clock:
    def __init__(self, now: float = NOW):
        self.now = now

    def __call__(self) -> float:
        return self.now


def stamped_headers(
    key: SigningKey,
    *,
    method: str = "POST",
    path: str = PATH,
    query: str = QUERY,
    body: bytes = BODY,
    user_id: str = "user-1",
    organization_id: str = "",
    cli_session_id: str = "",
    nonce: str = "n" * 32,
    issued_at: int = int(NOW),
    key_id: str = KEY_ID,
) -> dict[str, str]:
    request_uri = f"{path}?{query}" if query else path
    stamp = BrowserProxyStamp(
        method=method,
        request_uri=request_uri,
        body_sha256=hashlib.sha256(body).hexdigest(),
        user_id=user_id,
        user_email=f"{user_id}@g8e.local",
        web_session_id="web-1",
        organization_id=organization_id,
        cli_session_id=cli_session_id,
        issued_at_unix=issued_at,
        nonce=nonce,
    )
    headers = {
        X_PROXY_USER_ID: user_id,
        X_PROXY_USER_EMAIL: f"{user_id}@g8e.local",
        X_PROXY_WEB_SESSION_ID: "web-1",
        X_PROXY_KEY_ID: key_id,
        X_PROXY_ISSUED_AT: str(issued_at),
        X_PROXY_NONCE: nonce,
        X_PROXY_SIGNATURE: key.sign(stamp.canonical_bytes()).signature.hex(),
    }
    if organization_id:
        headers[X_PROXY_ORGANIZATION_ID] = organization_id
    if cli_session_id:
        headers[X_PROXY_CLI_SESSION_ID] = cli_session_id
    return headers


def make_request(
    headers: dict[str, str],
    *,
    method: str = "POST",
    path: str = PATH,
    query: str = QUERY,
    body: bytes = BODY,
) -> Request:
    scope = {
        "type": "http",
        "method": method,
        "path": path,
        "raw_path": path.encode(),
        "query_string": query.encode(),
        "headers": [(k.lower().encode(), v.encode()) for k, v in headers.items()],
    }
    sent = False

    async def receive():
        nonlocal sent
        if sent:
            return {"type": "http.disconnect"}
        sent = True
        return {"type": "http.request", "body": body, "more_body": False}

    return Request(scope, receive)


def verifier(key_source: KeySource, clock: Clock | None = None, **kwargs) -> ProxyStampVerifier:
    return ProxyStampVerifier(key_source, clock=clock or Clock(), **kwargs)
