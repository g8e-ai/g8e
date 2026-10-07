# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

from unittest.mock import AsyncMock, MagicMock

import pytest
from fastapi import Request
from proxy_stamp_support import (
    KeySource,
    key_response,
    make_key,
    make_request,
    stamped_headers,
    verifier,
)

from app.constants import (
    AUTHORIZATION,
    CLI_SESSION_ID,
    WEB_SESSION_ID,
    X_PROXY_CLI_SESSION_ID,
    X_PROXY_USER_EMAIL,
    X_PROXY_USER_ID,
    AuthMethod,
    OperatorStatus,
)
from app.errors import AuthenticationError
from app.models.auth import AuthenticatedUser, OperatorSessionValidationResponse
from app.models.http_context import BoundOperator, G8eHttpContext
from app.services.auth.auth_service import AuthService


@pytest.fixture
def mock_internal_http_client():
    return AsyncMock()


@pytest.fixture
def auth_service(mock_internal_http_client):
    return AuthService(internal_http_client=mock_internal_http_client)


class TestAuthServiceProxyAuthentication:
    @pytest.fixture
    def gateway_key(self):
        return make_key()

    @pytest.fixture
    def stamped_auth_service(self, mock_internal_http_client, gateway_key):
        return AuthService(
            internal_http_client=mock_internal_http_client,
            proxy_stamp_verifier=verifier(KeySource(key_response(gateway_key))),
        )

    @pytest.mark.asyncio
    async def test_signed_proxy_auth_extracts_user_and_cli_session_id(
        self, stamped_auth_service, gateway_key
    ):
        request = make_request(
            stamped_headers(
                gateway_key,
                user_id="user-123",
                organization_id="org-456",
                cli_session_id="cli-session-789",
            )
        )

        user = await stamped_auth_service.authenticate_request(request)

        assert user.uid == "user-123"
        assert user.user_id == "user-123"
        assert user.email == "user-123@g8e.local"
        assert user.organization_id == "org-456"
        assert user.cli_session_id == "cli-session-789"
        assert user.web_session_id == "web-1"
        assert user.auth_method == AuthMethod.PROXY

    @pytest.mark.asyncio
    async def test_signed_proxy_auth_ignores_unsigned_session_headers(
        self, stamped_auth_service, gateway_key
    ):
        headers = stamped_headers(gateway_key)
        headers[WEB_SESSION_ID] = "web-forged"
        headers[CLI_SESSION_ID] = "cli-forged"

        user = await stamped_auth_service.authenticate_request(make_request(headers))

        assert user.web_session_id == "web-1"
        assert user.cli_session_id is None

    @pytest.mark.asyncio
    async def test_proxy_headers_without_a_signature_are_rejected(self, stamped_auth_service):
        request = make_request(
            {X_PROXY_USER_ID: "user-123", X_PROXY_USER_EMAIL: "user-123@g8e.local"}
        )

        with pytest.raises(AuthenticationError, match="Gateway signature"):
            await stamped_auth_service.authenticate_request(request)

    @pytest.mark.asyncio
    async def test_static_marker_header_no_longer_authenticates(self, stamped_auth_service):
        request = make_request(
            {
                "X-G8E-Gateway-Browser-Proxy": "1",
                X_PROXY_USER_ID: "user-123",
                X_PROXY_USER_EMAIL: "user-123@g8e.local",
            }
        )

        with pytest.raises(AuthenticationError, match="Gateway signature"):
            await stamped_auth_service.authenticate_request(request)

    @pytest.mark.asyncio
    async def test_tampered_body_is_rejected(self, stamped_auth_service, gateway_key):
        request = make_request(stamped_headers(gateway_key), body=b'{"message":"different"}')

        with pytest.raises(AuthenticationError, match="Gateway signature"):
            await stamped_auth_service.authenticate_request(request)

    @pytest.mark.asyncio
    async def test_replayed_stamp_is_rejected_on_a_second_request(
        self, stamped_auth_service, gateway_key
    ):
        headers = stamped_headers(gateway_key)
        await stamped_auth_service.authenticate_request(make_request(headers))

        with pytest.raises(AuthenticationError, match="Gateway signature"):
            await stamped_auth_service.authenticate_request(make_request(headers))

    @pytest.mark.asyncio
    async def test_one_request_can_be_authenticated_by_several_dependencies(
        self, stamped_auth_service, gateway_key
    ):
        request = make_request(stamped_headers(gateway_key))

        first = await stamped_auth_service.authenticate_request(request)
        second = await stamped_auth_service.authenticate_request(request)

        assert first.user_id == second.user_id == "user-1"

    @pytest.mark.asyncio
    async def test_without_a_verifier_proxy_identity_is_rejected(self, auth_service, gateway_key):
        with pytest.raises(AuthenticationError, match="Gateway signature"):
            await auth_service.authenticate_request(make_request(stamped_headers(gateway_key)))

    @pytest.mark.asyncio
    async def test_proxy_auth_missing_email_fails(self, stamped_auth_service):
        request = make_request({X_PROXY_USER_ID: "user-123"})

        with pytest.raises(AuthenticationError):
            await stamped_auth_service.authenticate_request(request)


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

        user = await auth_service.authenticate_request(request)

        assert user.user_id == "user-123"
        assert user.operator_id == "operator-456"
        assert user.operator_session_id == "operator-session-123"
        assert user.cli_session_id == "cli-session-789"
        mock_internal_http_client.validate_operator_session.assert_awaited_once_with(
            "operator-session-123", "cli-session-789", "user-123"
        )

    @pytest.mark.asyncio
    async def test_bearer_auth_takes_precedence_over_proxy_headers(
        self, auth_service, mock_internal_http_client
    ):
        request = MagicMock(spec=Request)
        request.headers = {
            AUTHORIZATION: "Bearer operator-session-123",
            X_PROXY_USER_ID: "user-123",
            X_PROXY_USER_EMAIL: "user-123@g8e.local",
            X_PROXY_CLI_SESSION_ID: "cli-session-789",
        }
        request.state = MagicMock()
        request.state.g8e_context = None
        mock_internal_http_client.validate_operator_session.return_value = (
            OperatorSessionValidationResponse(
                valid=True,
                operator_id="operator-456",
                user_id="user-123",
            )
        )

        user = await auth_service.authenticate_request(request)

        assert user.auth_method == AuthMethod.OPERATOR_SESSION
        assert user.operator_session_id == "operator-session-123"

    @pytest.mark.asyncio
    async def test_bearer_auth_uses_proxy_headers_when_body_context_absent(
        self, auth_service, mock_internal_http_client
    ):
        request = MagicMock(spec=Request)
        request.headers = {
            AUTHORIZATION: "Bearer operator-session-123",
            X_PROXY_USER_ID: "user-123",
            X_PROXY_CLI_SESSION_ID: "cli-session-789",
        }
        request.state = MagicMock()
        request.state.g8e_context = None
        mock_internal_http_client.validate_operator_session.return_value = (
            OperatorSessionValidationResponse(
                valid=True,
                operator_id="operator-456",
                user_id="user-123",
            )
        )

        user = await auth_service.authenticate_request(request)

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
            await auth_service.authenticate_request(request)


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
    async def test_get_validated_context_rejects_operator_id_outside_authoritative_binding(
        self, auth_service
    ):
        request = MagicMock(spec=Request)
        request.state = MagicMock()
        request.state.g8e_context = G8eHttpContext(
            user_id="user-123",
            cli_session_id="cli-session-789",
            source_component="CLIENT",
            bound_operators=[
                BoundOperator(
                    operator_id="operator-other",
                    operator_session_id="operator-session-123",
                    status=OperatorStatus.BOUND,
                )
            ],
        )
        user = AuthenticatedUser(
            uid="user-123",
            user_id="user-123",
            cli_session_id="cli-session-789",
            operator_id="operator-456",
            operator_session_id="operator-session-123",
            auth_method=AuthMethod.OPERATOR_SESSION,
        )

        with pytest.raises(AuthenticationError):
            await auth_service.get_validated_context(request, user)

    @pytest.mark.asyncio
    async def test_get_validated_context_preserves_distinct_authority_and_execution_target(
        self, auth_service
    ):
        request = MagicMock(spec=Request)
        request.state = MagicMock()
        request.state.g8e_context = G8eHttpContext(
            user_id="user-123",
            cli_session_id="cli-session-789",
            source_component="CLIENT",
            operator_id="embedded-operator",
            operator_session_id="embedded-session-123",
            bound_operators=[
                BoundOperator(
                    operator_id="remote-operator",
                    operator_session_id="remote-session-456",
                    status=OperatorStatus.BOUND,
                )
            ],
        )
        user = AuthenticatedUser(
            uid="user-123",
            user_id="user-123",
            cli_session_id="cli-session-789",
            operator_id="embedded-operator",
            operator_session_id="embedded-session-123",
            auth_method=AuthMethod.OPERATOR_SESSION,
        )

        validated = await auth_service.get_validated_context(request, user)

        assert validated.operator_id == "embedded-operator"
        assert validated.operator_session_id == "embedded-session-123"
        assert validated.bound_operators[0].operator_id == "remote-operator"
        assert validated.bound_operators[0].operator_session_id == "remote-session-456"

    @pytest.mark.asyncio
    async def test_get_validated_context_rejects_routing_operator_outside_authoritative_binding(
        self, auth_service
    ):
        request = MagicMock(spec=Request)
        request.state = MagicMock()
        request.state.g8e_context = G8eHttpContext(
            user_id="user-123",
            cli_session_id="cli-session-789",
            source_component="CLIENT",
            operator_id="operator-other",
            operator_session_id="operator-session-123",
        )
        user = AuthenticatedUser(
            uid="user-123",
            user_id="user-123",
            cli_session_id="cli-session-789",
            operator_id="operator-456",
            operator_session_id="operator-session-123",
            auth_method=AuthMethod.OPERATOR_SESSION,
        )

        with pytest.raises(AuthenticationError):
            await auth_service.get_validated_context(request, user)

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
