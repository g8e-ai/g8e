# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

import pytest
from unittest.mock import AsyncMock, MagicMock
from fastapi import Request

from app.constants import (
    AUTHORIZATION,
    CLI_SESSION_ID,
    AuthMethod,
    OperatorStatus,
    X_PROXY_CLI_SESSION_ID,
    X_PROXY_ORGANIZATION_ID,
    X_PROXY_USER_EMAIL,
    X_PROXY_USER_ID,
    X_PROXY_WEB_SESSION_ID,
)
from app.errors import AuthenticationError
from app.models.auth import AuthenticatedUser, OperatorSessionValidationResponse
from app.models.http_context import BoundOperator, G8eHttpContext, RequestContext
from app.services.auth.auth_service import AuthService


@pytest.fixture
def mock_internal_http_client():
    return AsyncMock()


@pytest.fixture
def auth_service(mock_internal_http_client):
    return AuthService(internal_http_client=mock_internal_http_client)


class TestAuthServiceProxyAuthentication:
    @pytest.mark.asyncio
    async def test_proxy_auth_extracts_user_and_cli_session_id(self, auth_service):
        request = MagicMock(spec=Request)
        request.headers = {
            X_PROXY_USER_ID: "user-123",
            X_PROXY_USER_EMAIL: "user-123@g8e.local",
            X_PROXY_ORGANIZATION_ID: "org-456",
            X_PROXY_CLI_SESSION_ID: "cli-session-789",
        }
        request.state = MagicMock()
        request.state.g8e_context = None

        settings = MagicMock()
        user = await auth_service.authenticate_request(request, settings)

        assert user.uid == "user-123"
        assert user.user_id == "user-123"
        assert user.email == "user-123@g8e.local"
        assert user.organization_id == "org-456"
        assert user.cli_session_id == "cli-session-789"
        assert user.web_session_id is None
        assert user.auth_method == AuthMethod.PROXY

    @pytest.mark.asyncio
    async def test_proxy_auth_extracts_web_session_id(self, auth_service):
        request = MagicMock(spec=Request)
        request.headers = {
            X_PROXY_USER_ID: "user-123",
            X_PROXY_USER_EMAIL: "user-123@g8e.local",
            X_PROXY_WEB_SESSION_ID: "web-session-abc",
        }
        request.state = MagicMock()
        request.state.g8e_context = None

        settings = MagicMock()
        user = await auth_service.authenticate_request(request, settings)

        assert user.uid == "user-123"
        assert user.cli_session_id is None
        assert user.web_session_id == "web-session-abc"
        assert user.auth_method == AuthMethod.PROXY

    @pytest.mark.asyncio
    async def test_proxy_auth_missing_email_fails(self, auth_service):
        request = MagicMock(spec=Request)
        request.headers = {
            X_PROXY_USER_ID: "user-123",
        }
        request.state = MagicMock()

        settings = MagicMock()
        with pytest.raises(AuthenticationError):
            await auth_service.authenticate_request(request, settings)


class TestAuthServiceOperatorSessionAuthentication:
    @pytest.mark.asyncio
    async def test_missing_local_projection_uses_authoritative_gateway_binding(
        self, auth_service, mock_internal_http_client
    ):
        request = MagicMock(spec=Request)
        request.headers = {
            AUTHORIZATION: "Bearer operator-session-123",
            CLI_SESSION_ID: "cli-session-789",
        }
        request.state = MagicMock()
        request.state.g8e_context = G8eHttpContext(
            user_id="user-123",
            cli_session_id="cli-session-789",
            source_component="CLIENT",
        )
        mock_internal_http_client.validate_operator_session.return_value = (
            OperatorSessionValidationResponse(
                valid=True,
                operator_id="operator-456",
                user_id="user-123",
            )
        )

        user = await auth_service.authenticate_request(request, MagicMock())

        assert user.user_id == "user-123"
        assert user.operator_session_id == "operator-session-123"
        assert user.cli_session_id == "cli-session-789"
        mock_internal_http_client.validate_operator_session.assert_awaited_once_with(
            "operator-session-123", "cli-session-789", "user-123"
        )

    @pytest.mark.asyncio
    async def test_mismatched_authoritative_binding_is_rejected(
        self, auth_service, mock_internal_http_client
    ):
        request = MagicMock(spec=Request)
        request.headers = {
            AUTHORIZATION: "Bearer operator-session-123",
            CLI_SESSION_ID: "cli-session-789",
        }
        request.state = MagicMock()
        request.state.g8e_context = G8eHttpContext(
            user_id="user-123",
            cli_session_id="cli-session-789",
            source_component="CLIENT",
        )
        mock_internal_http_client.validate_operator_session.return_value = None

        with pytest.raises(AuthenticationError):
            await auth_service.authenticate_request(request, MagicMock())


class TestAuthServiceGetValidatedContext:
    @pytest.mark.asyncio
    async def test_get_validated_context_returns_body_context_with_bound_operators(
        self, auth_service
    ):
        request = MagicMock(spec=Request)
        bound_op = BoundOperator(
            operator_id="op-1",
            operator_session_id="op-sess-1",
            status=OperatorStatus.BOUND,
        )
        context = G8eHttpContext(
            user_id="user-123",
            cli_session_id="cli-session-789",
            source_component="CLIENT",
            bound_operators=[bound_op],
        )
        request.state = MagicMock()
        request.state.g8e_context = context

        user = AuthenticatedUser(
            uid="user-123",
            user_id="user-123",
            email="user-123@g8e.local",
            cli_session_id="cli-session-789",
            auth_method=AuthMethod.PROXY,
        )

        validated = await auth_service.get_validated_context(request, user)
        assert validated.user_id == "user-123"
        assert validated.cli_session_id == "cli-session-789"
        assert len(validated.bound_operators) == 1
        assert validated.bound_operators[0].operator_id == "op-1"
        assert validated.has_bound_operator() is True

    @pytest.mark.asyncio
    async def test_get_validated_context_fallback_derives_from_authenticated_user(
        self, auth_service
    ):
        request = MagicMock(spec=Request)
        request.state = MagicMock()
        request.state.g8e_context = None

        user = AuthenticatedUser(
            uid="user-123",
            user_id="user-123",
            email="user-123@g8e.local",
            cli_session_id="cli-session-789",
            auth_method=AuthMethod.PROXY,
        )

        validated = await auth_service.get_validated_context(request, user)
        assert validated.user_id == "user-123"
        assert validated.cli_session_id == "cli-session-789"
        assert validated.web_session_id is None
