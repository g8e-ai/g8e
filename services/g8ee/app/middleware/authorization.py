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

from fastapi import Request
from starlette.middleware.base import BaseHTTPMiddleware, RequestResponseEndpoint
from starlette.responses import Response

from app.constants import (
    ComponentName,
    InternalApiPaths,
)
from app.errors import AuthorizationError, ResourceNotFoundError, ServiceUnavailableError
from app.models.http_context import G8eHttpContext

logger = logging.getLogger(__name__)


class AuthorizationMiddleware(BaseHTTPMiddleware):

    EXEMPT_PATHS = {
        "/health",
        "/health/details",
        InternalApiPaths.G8EE_HEALTH,
        "/docs",
        "/openapi.json",
        "/redoc"
    }

    INTERNAL_PATHS = {
        "/investigations",
        "/chat",
    }

    async def dispatch(self, request: Request, call_next: RequestResponseEndpoint) -> Response:
        if request.url.path in self.EXEMPT_PATHS:
            return await call_next(request)

        # Ensure g8e_context is set by the G8eHttpContextMiddleware
        g8e_context: G8eHttpContext | None = getattr(request.state, "g8e_context", None)

        # If no context is present, we cannot perform authorization checks
        # This middleware should run AFTER G8eHttpContextMiddleware
        if not g8e_context:
            return await call_next(request)

        if self._is_user_scoped_path(request.url.path):
            query_params = dict(request.query_params)
            path_params = request.path_params if hasattr(request, "path_params") else {}

            path = request.url.path

            user_id_in_request = (
                query_params.get("user_id") or
                path_params.get("user_id")
            )

            if user_id_in_request and user_id_in_request != g8e_context.user_id:
                logger.error(
                    "AUTHORIZATION VIOLATION: User attempted to access another user's data",
                    extra={
                        "authenticated_user_id": g8e_context.user_id,
                        "requested_user_id": user_id_in_request,
                        "path": request.url.path,
                        "method": request.method,
                    }
                )
                raise AuthorizationError(
                    "Access denied: Cannot access other user's resources",
                    component=ComponentName.G8EE
                )

            # Extract investigation_id and case_id from path or query
            investigation_id = (
                query_params.get("investigation_id") or
                path_params.get("investigation_id")
            )
            if not investigation_id:
                # Try to extract from path: /investigations/{id}
                parts = path.split("/")
                if "investigations" in parts:
                    idx = parts.index("investigations")
                    if len(parts) > idx + 1:
                        investigation_id = parts[idx + 1]

            if investigation_id and investigation_id != "new-case":
                await self._validate_investigation_ownership(
                    request,
                    investigation_id,
                    g8e_context.user_id
                )

            case_id = (
                query_params.get("case_id") or
                path_params.get("case_id")
            )
            if not case_id:
                # Try to extract from path: /cases/{id}
                parts = path.split("/")
                if "cases" in parts:
                    idx = parts.index("cases")
                    if len(parts) > idx + 1:
                        case_id = parts[idx + 1]

            if case_id and case_id != "new-case":
                await self._validate_case_ownership(
                    request,
                    case_id,
                    g8e_context.user_id
                )

        return await call_next(request)

    def _is_user_scoped_path(self, path: str) -> bool:
        # Check if the path starts with any of our internal paths
        # InternalApiPaths.G8EE_INVESTIGATIONS is usually /api/internal/investigations
        return any(path.startswith(internal_path) for internal_path in self.INTERNAL_PATHS)

    def _extract_g8e_context(self, request: Request) -> G8eHttpContext | None:
        if hasattr(request.state, "g8e_context"):
            return request.state.g8e_context
        return None

    async def _validate_investigation_ownership(
        self,
        request: Request,
        investigation_id: str,
        authenticated_user_id: str
    ) -> None:
        services = getattr(request.app.state, "services", None)
        investigation_service = getattr(services, "investigation_service", None) if services else None
        if investigation_service is None:
            raise ServiceUnavailableError(
                "investigation_service not initialised",
                component=ComponentName.G8EE
            )

        investigation = await investigation_service.investigation_data_service.get_investigation(investigation_id)
        if not investigation or investigation.user_id != authenticated_user_id:
            logger.error(
                "AUTHORIZATION VIOLATION: Investigation ownership check failed",
                extra={
                    "authenticated_user_id": authenticated_user_id,
                    "investigation_id": investigation_id,
                    "investigation_user_id": investigation.user_id if investigation else None,
                    "path": request.url.path,
                    "method": request.method,
                    "source_ip": request.client.host if request.client else "None"
                }
            )
            raise AuthorizationError(
                "Investigation not found or access denied",
                component=ComponentName.G8EE
            )

    async def _validate_case_ownership(
        self,
        request: Request,
        case_id: str,
        authenticated_user_id: str
    ) -> None:
        services = getattr(request.app.state, "services", None)
        db_service = getattr(services, "db_service", None) if services else None
        if db_service is None:
            raise ServiceUnavailableError(
                "db_service not initialised",
                component=ComponentName.G8EE
            )

        try:
            case = await db_service.get_case(case_id)
        except ResourceNotFoundError as err:
            raise AuthorizationError("Case not found or access denied", component=ComponentName.G8EE) from err

        if case is None:
            raise AuthorizationError("Case not found or access denied", component=ComponentName.G8EE)

        case_user_id = case.user_id
        if case_user_id != authenticated_user_id:
            logger.error(
                "AUTHORIZATION VIOLATION: Case ownership check failed",
                extra={
                    "authenticated_user_id": authenticated_user_id,
                    "case_id": case_id,
                    "case_user_id": case_user_id,
                    "path": request.url.path,
                    "method": request.method,
                    "source_ip": request.client.host if request.client else "None"
                }
            )
            raise AuthorizationError(
                "Case not found or access denied",
                component=ComponentName.G8EE
            )
