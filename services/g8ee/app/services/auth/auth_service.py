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

from __future__ import annotations

import logging
from typing import TYPE_CHECKING

from fastapi import Request

from app.constants import (
    CLI_SESSION_ID,
    HTTP_AUTHORIZATION_HEADER,
    HTTP_BEARER_PREFIX,
    PROXY_ORGANIZATION_ID_HEADER,
    PROXY_USER_EMAIL_HEADER,
    PROXY_USER_ID_HEADER,
    AuthMethod,
    ComponentName,
)
from app.errors import AuthenticationError
from app.models.auth import AuthenticatedUser
from app.models.http_context import G8eHttpContext, RequestContext

if TYPE_CHECKING:
    from app.models.settings import G8eeAppSettings
    from app.services.operator.operator_session_service import OperatorSessionService
    from app.services.operator.operator_data_service import OperatorDataService

logger = logging.getLogger(__name__)


class AuthService:
    """Unified authentication and context validation service."""

    def __init__(
        self,
        operator_session_service: OperatorSessionService,
        operator_data_service: OperatorDataService,
    ):
        self._operator_session_service = operator_session_service
        self._operator_data_service = operator_data_service

    async def authenticate_request(
        self,
        request: Request,
        settings: G8eeAppSettings,
    ) -> AuthenticatedUser:
        """Authenticate via proxy headers (browser) or Bearer operator session (CLI/mTLS)."""
        proxy_user_id = request.headers.get(PROXY_USER_ID_HEADER)
        proxy_user_email = request.headers.get(PROXY_USER_EMAIL_HEADER)
        proxy_org_id = request.headers.get(PROXY_ORGANIZATION_ID_HEADER)

        user = None

        if proxy_user_id and proxy_user_email:
            logger.debug(
                "[AuthService] Authenticated via proxy headers",
                extra={
                    "user_id": proxy_user_id,
                    "email": proxy_user_email,
                    "organization_id": proxy_org_id,
                },
            )
            user = AuthenticatedUser(
                uid=proxy_user_id,
                user_id=proxy_user_id,
                email=proxy_user_email,
                organization_id=proxy_org_id,
                auth_method=AuthMethod.PROXY,
            )
        else:
            auth_header = request.headers.get(HTTP_AUTHORIZATION_HEADER, "")
            if auth_header.startswith(HTTP_BEARER_PREFIX):
                bearer_token = auth_header[len(HTTP_BEARER_PREFIX) :]
                session = await self._operator_session_service.validate_operator_session(bearer_token)
                if session and session.user_id:
                    # Prefer cli_session_id from body-embedded context if available
                    g8e_context = getattr(request.state, "g8e_context", None)
                    cli_session_id = None
                    if g8e_context:
                        cli_session_id = g8e_context.cli_session_id

                    if not cli_session_id:
                        cli_session_id = request.headers.get(CLI_SESSION_ID)

                    logger.debug(
                        "[AuthService] Authenticated via operator session Bearer token",
                        extra={"user_id": session.user_id, "operator_session_id": bearer_token[:8]},
                    )
                    user = AuthenticatedUser(
                        uid=session.user_id,
                        user_id=session.user_id,
                        operator_session_id=bearer_token,
                        cli_session_id=cli_session_id,
                        auth_method=AuthMethod.OPERATOR_SESSION,
                    )

        if not user:
            raise AuthenticationError("Authentication required", component=ComponentName.G8EE)

        # [PIVOT] Validate session bindings for CLI sessions (Plan §4.6)
        # If a CLI session ID is provided, it must be bound to the authenticated
        # operator session. This prevents cross-session routing leaks.
        if user.cli_session_id:
            if not user.operator_session_id:
                raise AuthenticationError("CLI session requires operator session", component=ComponentName.G8EE)

            if not await self._operator_data_service.validate_cli_session_ownership(
                user.cli_session_id, user.operator_session_id
            ):
                raise AuthenticationError("CLI session ownership mismatch", component=ComponentName.G8EE)

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
            source_component=ComponentName.G8EE,  # Default for internal relay
        )
