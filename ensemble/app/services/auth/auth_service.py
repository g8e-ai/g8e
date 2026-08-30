# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

from __future__ import annotations

import logging
from typing import TYPE_CHECKING

from fastapi import Request

from app.constants import (
    AUTHORIZATION,
    AuthMethod,
    CLI_SESSION_ID,
    G8EE_COMPONENT,
    WEB_SESSION_ID,
    X_PROXY_CLI_SESSION_ID,
    X_PROXY_ORGANIZATION_ID,
    X_PROXY_USER_EMAIL,
    X_PROXY_USER_ID,
    X_PROXY_WEB_SESSION_ID,
)
from app.errors import AuthenticationError
from app.models.auth import AuthenticatedUser
from app.models.http_context import G8eHttpContext

if TYPE_CHECKING:
    from app.models.settings import G8eeAppSettings
    from app.services.infra.internal_http_client import InternalHttpClient

logger = logging.getLogger(__name__)


class AuthService:
    """Unified authentication and context validation service."""

    def __init__(self, internal_http_client: InternalHttpClient):
        self._internal_http_client = internal_http_client

    async def authenticate_request(
        self,
        request: Request,
        settings: G8eeAppSettings,
    ) -> AuthenticatedUser:
        """Authenticate via proxy headers (browser) or Bearer operator session (CLI/mTLS)."""
        proxy_user_id = request.headers.get(X_PROXY_USER_ID)
        proxy_user_email = request.headers.get(X_PROXY_USER_EMAIL)
        proxy_org_id = request.headers.get(X_PROXY_ORGANIZATION_ID)

        user = None

        if proxy_user_id and proxy_user_email:
            proxy_cli_session_id = request.headers.get(X_PROXY_CLI_SESSION_ID) or request.headers.get(CLI_SESSION_ID)
            proxy_web_session_id = request.headers.get(X_PROXY_WEB_SESSION_ID) or request.headers.get(WEB_SESSION_ID)
            g8e_context = getattr(request.state, "g8e_context", None)
            if g8e_context:
                if not proxy_cli_session_id and g8e_context.cli_session_id:
                    proxy_cli_session_id = g8e_context.cli_session_id
                if not proxy_web_session_id and g8e_context.web_session_id:
                    proxy_web_session_id = g8e_context.web_session_id

            logger.debug(
                "[AuthService] Authenticated via proxy headers",
                extra={
                    "user_id": proxy_user_id,
                    "email": proxy_user_email,
                    "organization_id": proxy_org_id,
                    "cli_session_id": proxy_cli_session_id,
                    "web_session_id": proxy_web_session_id,
                },
            )
            user = AuthenticatedUser(
                uid=proxy_user_id,
                user_id=proxy_user_id,
                email=proxy_user_email,
                organization_id=proxy_org_id,
                cli_session_id=proxy_cli_session_id,
                web_session_id=proxy_web_session_id,
                auth_method=AuthMethod.PROXY,
            )
        else:
            auth_header = request.headers.get(AUTHORIZATION, "")
            if auth_header.startswith("Bearer "):
                bearer_token = auth_header[len("Bearer ") :]
                g8e_context = getattr(request.state, "g8e_context", None)
                cli_session_id = (
                    g8e_context.cli_session_id if g8e_context else None
                ) or request.headers.get(CLI_SESSION_ID)
                user_id = g8e_context.user_id if g8e_context else None
                if cli_session_id and user_id:
                    binding = await self._internal_http_client.validate_operator_session(
                        bearer_token, cli_session_id, user_id
                    )
                    if binding:
                        logger.debug(
                            "[AuthService] Authenticated via authoritative operator session binding",
                            extra={"user_id": binding.user_id, "operator_session_id": bearer_token[:8]},
                        )
                        user = AuthenticatedUser(
                            uid=binding.user_id,
                            user_id=binding.user_id,
                            operator_id=binding.operator_id,
                            operator_session_id=bearer_token,
                            cli_session_id=cli_session_id,
                            auth_method=AuthMethod.OPERATOR_SESSION,
                        )

        if not user:
            raise AuthenticationError("Authentication required", component=G8EE_COMPONENT)

        return user

    async def get_validated_context(
        self,
        request: Request,
        user: AuthenticatedUser,
        is_exempt_path: bool = False,
    ) -> G8eHttpContext:
        """Unified context validation: extracts from body and checks against auth user.

        If no context is found in the body, a default context is derived from the
        authenticated user.
        """
        # Check if middleware already extracted context
        g8e_context = getattr(request.state, "g8e_context", None)
        if g8e_context:
            g8e_context.validate_against_user(user)
            return g8e_context

        # No context in body, return context derived from authenticated user
        return G8eHttpContext(
            user_id=user.uid,
            web_session_id=user.web_session_id,
            cli_session_id=user.cli_session_id,
            source_component=G8EE_COMPONENT,  # Default for internal relay
        )
