# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

"""Verification of the Gateway's signed browser-proxy identity stamp.

The browser only talks to the Gateway. When the Gateway forwards a request to
g8ee it signs who the user is, together with the exact request (method,
target, body) and a short-lived nonce. g8ee believes proxy identity headers
only when that signature verifies against the Gateway's published key, so
nothing about g8ee's network position (loopback, a container network, a shared
volume) is part of the trust decision.

The canonical encoding is pinned by the shared vector file
``protocol/conformance/browser_proxy_stamp_vectors.json`` and by
``internal/services/gateway/browser_proxy_stamp.go``.
"""

from __future__ import annotations

import asyncio
import hashlib
import logging
import time
from collections.abc import Awaitable, Callable
from dataclasses import dataclass

from fastapi import Request
from nacl.exceptions import BadSignatureError
from nacl.signing import VerifyKey

from app.constants import (
    BROWSER_PROXY_STAMP_DOMAIN,
    BROWSER_PROXY_STAMP_MAX_SKEW_SECONDS,
    G8EE_COMPONENT,
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
from app.errors import AuthenticationError
from app.models.auth import ProxySigningKeyResponse

logger = logging.getLogger(__name__)

KeySource = Callable[[], Awaitable[ProxySigningKeyResponse]]

# Smallest gap between two fetches of the Gateway's signing key. A request
# naming an unknown key ID may trigger a refetch (the Gateway rotated), but a
# caller spraying random key IDs cannot make g8ee hammer the Gateway.
KEY_REFRESH_MIN_INTERVAL_SECONDS = 10.0
MAX_REMEMBERED_NONCES = 10_000

_REJECTED = "Proxy identity requires a valid Gateway signature"


@dataclass(frozen=True)
class BrowserProxyStamp:
    """The values the Gateway signs for one proxied request."""

    method: str
    request_uri: str
    body_sha256: str
    user_id: str
    user_email: str
    web_session_id: str
    organization_id: str
    cli_session_id: str
    issued_at_unix: int
    nonce: str

    def canonical_bytes(self) -> bytes:
        fields = (
            BROWSER_PROXY_STAMP_DOMAIN,
            self.method,
            self.request_uri,
            self.body_sha256,
            self.user_id,
            self.user_email,
            self.web_session_id,
            self.organization_id,
            self.cli_session_id,
            str(self.issued_at_unix),
            self.nonce,
        )
        if any("\n" in f or "\r" in f for f in fields):
            raise AuthenticationError(_REJECTED, component=G8EE_COMPONENT)
        return "\n".join(fields).encode("utf-8")


def _request_uri(request: Request) -> str:
    raw_path = request.scope.get("raw_path")
    path = raw_path.decode("latin-1") if raw_path else request.url.path
    query = request.scope.get("query_string", b"").decode("latin-1")
    return f"{path}?{query}" if query else path


class ProxyStampVerifier:
    """Verifies stamps against the Gateway's published Ed25519 key."""

    def __init__(
        self,
        key_source: KeySource,
        *,
        clock: Callable[[], float] = time.time,
        max_nonces: int = MAX_REMEMBERED_NONCES,
    ):
        self._key_source = key_source
        self._clock = clock
        self._max_nonces = max_nonces
        self._key_id: str | None = None
        self._verify_key: VerifyKey | None = None
        self._last_fetch: float | None = None
        self._nonces: dict[str, float] = {}
        # Serializes key fetches. Browsers fan out several proxied requests on
        # console load; without this, the first starts a fetch and the rest hit
        # the refetch throttle and reject a valid stamp as "unknown key id".
        self._fetch_lock = asyncio.Lock()

    async def verify(self, request: Request) -> None:
        """Raise AuthenticationError unless the request carries a valid stamp.

        The nonce is recorded only after the signature verifies, so a forged
        request cannot burn a legitimate request's nonce.
        """
        try:
            await self._verify(request)
        except AuthenticationError:
            raise
        except Exception as exc:
            logger.warning("[ProxyStamp] verification error: %s", exc)
            raise AuthenticationError(_REJECTED, component=G8EE_COMPONENT) from exc

    async def _verify(self, request: Request) -> None:
        headers = request.headers
        signature_hex = headers.get(X_PROXY_SIGNATURE)
        key_id = headers.get(X_PROXY_KEY_ID)
        issued_raw = headers.get(X_PROXY_ISSUED_AT)
        nonce = headers.get(X_PROXY_NONCE)
        if not (signature_hex and key_id and issued_raw and nonce):
            self._reject("missing stamp headers")

        try:
            issued_at = int(issued_raw)
            signature = bytes.fromhex(signature_hex)
        except ValueError:
            self._reject("malformed stamp headers")

        now = self._clock()
        if abs(now - issued_at) > BROWSER_PROXY_STAMP_MAX_SKEW_SECONDS:
            self._reject("stamp outside the allowed clock window")

        stamp = BrowserProxyStamp(
            method=request.method,
            request_uri=_request_uri(request),
            body_sha256=hashlib.sha256(await request.body()).hexdigest(),
            user_id=headers.get(X_PROXY_USER_ID, ""),
            user_email=headers.get(X_PROXY_USER_EMAIL, ""),
            web_session_id=headers.get(X_PROXY_WEB_SESSION_ID, ""),
            organization_id=headers.get(X_PROXY_ORGANIZATION_ID, ""),
            cli_session_id=headers.get(X_PROXY_CLI_SESSION_ID, ""),
            issued_at_unix=issued_at,
            nonce=nonce,
        )

        verify_key = await self._key_for(key_id)
        try:
            verify_key.verify(stamp.canonical_bytes(), signature)
        except BadSignatureError:
            self._reject("signature does not match the request")

        self._remember(nonce, now)

    async def _key_for(self, key_id: str) -> VerifyKey:
        if self._verify_key is not None and self._key_id == key_id:
            return self._verify_key

        async with self._fetch_lock:
            # Re-check: a fetch that finished while we waited may already
            # have published this key ID.
            if self._verify_key is not None and self._key_id == key_id:
                return self._verify_key

            current_time = self._clock()
            can_fetch = (
                self._last_fetch is None
                or current_time - self._last_fetch >= KEY_REFRESH_MIN_INTERVAL_SECONDS
            )
            if not can_fetch:
                self._reject("unknown key id")

            self._last_fetch = current_time
            published = await self._key_source()
            if published.algorithm != "ed25519":
                self._reject("gateway published a non-ed25519 key")
            self._verify_key = VerifyKey(bytes.fromhex(published.public_key))
            self._key_id = published.key_id
            if self._key_id != key_id:
                self._reject("unknown key id")
            return self._verify_key

    def _remember(self, nonce: str, now: float) -> None:
        window = BROWSER_PROXY_STAMP_MAX_SKEW_SECONDS * 2
        self._nonces = {n: seen for n, seen in self._nonces.items() if now - seen <= window}
        if nonce in self._nonces:
            self._reject("nonce already used")
        if len(self._nonces) >= self._max_nonces:
            self._reject("nonce cache full")
        self._nonces[nonce] = now

    @staticmethod
    def _reject(reason: str):
        logger.warning("[ProxyStamp] rejected: %s", reason)
        raise AuthenticationError(_REJECTED, component=G8EE_COMPONENT)
