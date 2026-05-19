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

import logging
from typing import TYPE_CHECKING
from fastapi import Request

from app.constants import (
    HTTP_AUTHORIZATION_HEADER,
    HTTP_BEARER_PREFIX,
    PROXY_ORGANIZATION_ID_HEADER,
    PROXY_USER_EMAIL_HEADER,
    PROXY_USER_ID_HEADER,
    AuthMethod,
    ComponentName,
    G8eHeaders,
)
from app.errors import AuthenticationError
from app.models.auth import AuthenticatedUser
from app.models.settings import G8eePlatformSettings

if TYPE_CHECKING:
    from app.services.operator.operator_session_service import OperatorSessionService

logger = logging.getLogger(__name__)


def is_infrastructure_health_check_ip(ip: str) -> bool:
    """Check if the given IP belongs to known infrastructure health checkers."""
    if not ip:
        return False

    normalized_ip = ip.replace("::ffff:", "") if ip.startswith("::ffff:") else ip

    if normalized_ip.startswith("35.191."):
        return True

    if normalized_ip.startswith("130.211."):
        parts = normalized_ip.split(".")
        if len(parts) == 4:
            try:
                third_octet = int(parts[2])
                if 0 <= third_octet <= 3:
                    return True
            except ValueError:
                pass

    return bool(normalized_ip.startswith("10."))


async def authenticate_proxy_or_internal(
    request: Request,
    settings: G8eePlatformSettings,
    operator_session_service: "OperatorSessionService | None" = None,
) -> AuthenticatedUser:
    """Authenticate via proxy headers (browser) or Bearer operator session (CLI/mTLS)."""
    proxy_user_id = request.headers.get(PROXY_USER_ID_HEADER)
    proxy_user_email = request.headers.get(PROXY_USER_EMAIL_HEADER)
    proxy_org_id = request.headers.get(PROXY_ORGANIZATION_ID_HEADER)

    if proxy_user_id and proxy_user_email:
        logger.info(
            "[g8ee] Authenticated via proxy headers",
            extra={
                "user_id": proxy_user_id,
                "email": proxy_user_email,
                "organization_id": proxy_org_id,
            }
        )
        return AuthenticatedUser(
            uid=proxy_user_id,
            user_id=proxy_user_id,
            email=proxy_user_email,
            organization_id=proxy_org_id,
            auth_method=AuthMethod.PROXY,
        )

    auth_header = request.headers.get(HTTP_AUTHORIZATION_HEADER, "")
    if auth_header.startswith(HTTP_BEARER_PREFIX) and operator_session_service is not None:
        bearer_token = auth_header[len(HTTP_BEARER_PREFIX):]
        session = await operator_session_service.validate_operator_session(bearer_token)
        if session and session.user_id:
            cli_session_id = request.headers.get(G8eHeaders.CLI_SESSION_ID)
            logger.info(
                "[g8ee] Authenticated via operator session Bearer token",
                extra={"user_id": session.user_id, "operator_session_id": bearer_token[:8]},
            )
            return AuthenticatedUser(
                uid=session.user_id,
                user_id=session.user_id,
                operator_session_id=bearer_token,
                cli_session_id=cli_session_id,
                auth_method=AuthMethod.OPERATOR_SESSION,
            )

    raise AuthenticationError("Authentication required", component=ComponentName.G8EE)
