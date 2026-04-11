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

import hmac
import logging
from fastapi import Request

from app.constants import (
    HTTP_FORWARDED_FOR_HEADER,
    HTTP_USER_AGENT_HEADER,
    INTERNAL_AUTH_HEADER,
    PROXY_ORGANIZATION_ID_HEADER,
    PROXY_USER_EMAIL_HEADER,
    PROXY_USER_ID_HEADER,
    AuthMethod,
    ComponentName,
    VSOHeaders,
)
from app.errors import AuthenticationError, AuthorizationError
from app.models.auth import AuthenticatedUser
from app.models.settings import VSEPlatformSettings

logger = logging.getLogger(__name__)


def verify_internal_auth_token(request: Request, settings: VSEPlatformSettings) -> bool:
    """Verify the internal authentication token from headers."""
    provided = request.headers.get(INTERNAL_AUTH_HEADER)
    expected = settings.auth.internal_auth_token
    if provided and expected:
        return hmac.compare_digest(provided, expected)
    return False


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

    if normalized_ip.startswith("10."):
        return True

    return False


async def validate_internal_origin(request: Request, settings: VSEPlatformSettings | None = None) -> bool:
    """Validate that the request originates from an internal source or health checker."""
    client_ip = request.client.host if request.client else None
    forwarded_for = request.headers.get(HTTP_FORWARDED_FOR_HEADER)
    user_agent = request.headers.get(HTTP_USER_AGENT_HEADER)

    normalized_ip = client_ip.replace("::ffff:", "") if client_ip and client_ip.startswith("::ffff:") else client_ip

    if not settings and hasattr(request.app.state, "settings"):
        settings = request.app.state.settings

    if settings and verify_internal_auth_token(request, settings):
        logger.info(
            "[AUTH] Internal endpoint access granted via auth token",
            extra={
                "endpoint": request.url.path,
                "ip": client_ip
            }
        )
        return True

    if is_infrastructure_health_check_ip(normalized_ip):
        logger.info(
            "[AUTH] Health check from GKE load balancer",
            extra={
                "endpoint": request.url.path,
                "ip": normalized_ip
            }
        )
        return True

    if normalized_ip == "127.0.0.1" and request.url.path.startswith("/health"):
        logger.info(
            "[AUTH] Health check from localhost (container internal)",
            extra={"endpoint": request.url.path}
        )
        return True

    logger.warning(
        "[AUTH] INTERNAL ENDPOINT ACCESS DENIED - missing or invalid auth token",
        extra={
            "endpoint": request.url.path,
            "method": request.method,
            "ip": client_ip,
            "normalized_ip": normalized_ip if normalized_ip != client_ip else None,
            "forwarded_for": forwarded_for,
            "has_auth_token": bool(request.headers.get(INTERNAL_AUTH_HEADER)),
            "has_expected_token": bool(settings and settings.auth.internal_auth_token),
            "user_agent": user_agent
        }
    )

    raise AuthorizationError(
        "Forbidden - Internal endpoint requires authentication",
        component=ComponentName.VSE,
    )


async def authenticate_proxy_or_internal(request: Request, settings: VSEPlatformSettings) -> AuthenticatedUser:
    """Authenticate the user via proxy headers or internal auth token."""
    proxy_user_id = request.headers.get(PROXY_USER_ID_HEADER)
    proxy_user_email = request.headers.get(PROXY_USER_EMAIL_HEADER)
    proxy_org_id = request.headers.get(PROXY_ORGANIZATION_ID_HEADER)

    if proxy_user_id and proxy_user_email:
        logger.info(
            "[VSE] Authenticated via proxy headers",
            extra={
                "user_id": proxy_user_id,
                "email": proxy_user_email,
                "organization_id": proxy_org_id
            }
        )
        return AuthenticatedUser(
            uid=proxy_user_id,
            user_id=proxy_user_id,
            email=proxy_user_email,
            organization_id=proxy_org_id,
            auth_method=AuthMethod.PROXY,
        )

    if verify_internal_auth_token(request, settings):
        vso_user_id = request.headers.get(VSOHeaders.USER_ID.lower())
        vso_session_id = request.headers.get(VSOHeaders.WEB_SESSION_ID.lower())
        vso_org_id = request.headers.get(VSOHeaders.ORGANIZATION_ID.lower())

        if vso_user_id:
            logger.info(
                "[VSE] Authenticated via internal auth token",
                extra={
                    "user_id": vso_user_id,
                    "web_session_id": vso_session_id[:12] + "..." if vso_session_id else None
                }
            )
            return AuthenticatedUser(
                uid=vso_user_id,
                user_id=vso_user_id,
                organization_id=vso_org_id,
                web_session_id=vso_session_id,
                auth_method=AuthMethod.INTERNAL,
            )

    raise AuthenticationError("Authentication required", component=ComponentName.VSE)
