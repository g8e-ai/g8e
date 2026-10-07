# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

"""The stamp verifies end to end through the real ASGI stack.

G8eHttpContextMiddleware reads the request body before the route runs, so the
verifier must still see the exact bytes the Gateway signed.
"""

from unittest.mock import AsyncMock

import pytest
from fastapi import Depends, FastAPI, Request
from fastapi.responses import JSONResponse
from fastapi.testclient import TestClient
from proxy_stamp_support import KeySource, key_response, make_key, stamped_headers, verifier

from app.errors import AuthenticationError
from app.middleware.http_context import G8eHttpContextMiddleware
from app.models.auth import AuthenticatedUser
from app.services.auth.auth_service import AuthService

BODY = b'{"message":"hi","context":{"user_id":"user-1","web_session_id":"web-1","source_component":"CLIENT"}}'


@pytest.fixture
def stack():
    key = make_key()
    service = AuthService(AsyncMock(), proxy_stamp_verifier=verifier(KeySource(key_response(key))))
    app = FastAPI()
    app.add_middleware(G8eHttpContextMiddleware)

    @app.exception_handler(AuthenticationError)
    async def _auth_error(_request, _exc):
        return JSONResponse({"error": "unauthenticated"}, status_code=401)

    async def authenticated(request: Request) -> AuthenticatedUser:
        return await service.authenticate_request(request)

    @app.post("/api/v1/chat/send")
    async def send(user: AuthenticatedUser = Depends(authenticated)):
        return {"user_id": user.user_id, "web_session_id": user.web_session_id}

    return TestClient(app), key


def test_signed_post_with_json_body_authenticates(stack):
    client, key = stack
    headers = stamped_headers(key, body=BODY, query="")

    response = client.post("/api/v1/chat/send", content=BODY, headers=headers)

    assert response.status_code == 200
    assert response.json() == {"user_id": "user-1", "web_session_id": "web-1"}


def test_signed_post_with_query_string_authenticates(stack):
    client, key = stack
    headers = stamped_headers(key, body=BODY, query="stream=1")

    assert (
        client.post("/api/v1/chat/send?stream=1", content=BODY, headers=headers).status_code == 200
    )


def test_body_swapped_after_signing_is_rejected(stack):
    client, key = stack
    headers = stamped_headers(key, body=BODY, query="")

    response = client.post("/api/v1/chat/send", content=BODY.replace(b"hi", b"ho"), headers=headers)

    assert response.status_code == 401


def test_forged_identity_headers_without_a_signature_are_rejected(stack):
    client, _ = stack

    response = client.post(
        "/api/v1/chat/send",
        content=BODY,
        headers={
            "X-Proxy-User-Id": "admin",
            "X-Proxy-User-Email": "admin@g8e.local",
            "X-G8E-Gateway-Browser-Proxy": "1",
        },
    )

    assert response.status_code == 401
