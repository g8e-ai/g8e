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

import json
from typing import Any

from fastapi import Request

from app.constants import (
    ComponentName,
    OperatorStatus,
    G8eHeaders,
    InternalApiPaths,
)
from app.utils.ids import generate_execution_id
from app.utils.timestamp import now
from app.logging import get_logger

from .base import Field, G8eBaseModel, UTCDatetime, field_validator, model_validator


class BoundOperator(G8eBaseModel):
    """Represents a bound operator in the HTTP context.
    
    Internal g8ee-client contract for bound operator context.
    """
    operator_id: str = Field(..., description="Unique operator identifier")
    operator_session_id: str | None = Field(default=None, description="Operator session identifier")
    bound_web_session_id: str | None = Field(default=None, description="Web session ID this operator is bound to")
    status: OperatorStatus | None = Field(default=None, description="Operator status")


class RequestContext(G8eBaseModel):
    """Request context embedded in request bodies instead of headers.
    
    This eliminates the fragile header-as-state pattern that forced every client
    (CLI, BYO, tests, evals) to re-implement the same header assembly.
    Context is now passed in the request body as a typed field.
    """
    web_session_id: str | None = Field(
        default=None,
        description="Web user session ID - used for routing SSE events to browser (null for operator auth)"
    )
    cli_session_id: str | None = Field(
        default=None,
        description="CLI session ID - used for routing SSE events to CLI clients"
    )
    user_id: str | None = Field(
        default=None,
        description="User identifier - owner of the session and data (null for operator auth)"
    )
    organization_id: str | None = Field(
        default=None,
        description="Organization identifier - for multi-tenant data isolation"
    )
    case_id: str | None = Field(
        default=None,
        description="Current case ID being worked on"
    )
    investigation_id: str | None = Field(
        default=None,
        description="Current investigation ID (AI chat session)"
    )
    task_id: str | None = Field(
        default=None,
        description="Current task ID"
    )
    bound_operators: list[BoundOperator] = Field(
        default_factory=list,
        description="List of all bound operators with their IDs, session IDs, and metadata."
    )
    execution_id: str | None = Field(
        default=None,
        description="Unique execution identifier for tracking"
    )
    source_component: ComponentName = Field(
        description="Component that created this context"
    )
    system_fingerprint: str | None = Field(
        default=None,
        description="System fingerprint of the caller"
    )


class G8eHttpContext(G8eBaseModel):
    """Standard context object for all internal HTTP requests."""

    web_session_id: str | None = Field(
        default=None,
        description="Web user session ID - used for routing SSE events to browser (null for operator auth)"
    )
    cli_session_id: str | None = Field(
        default=None,
        description="CLI session ID - used for routing SSE events to CLI clients"
    )
    user_id: str | None = Field(
        default=None,
        description="User identifier - owner of the session and data (null for operator auth)"
    )
    organization_id: str | None = Field(
        default=None,
        description="Organization identifier - for multi-tenant data isolation"
    )
    case_id: str | None = Field(
        default=None,
        description="Current case ID being worked on"
    )
    investigation_id: str | None = Field(
        default=None,
        description="Current investigation ID (AI chat session)"
    )
    task_id: str | None = Field(
        default=None,
        description="Current task ID"
    )
    bound_operators: list[BoundOperator] = Field(
        default_factory=list,
        description="List of all bound operators with their IDs, session IDs, and metadata."
    )
    execution_id: str = Field(
        default_factory=generate_execution_id,
        description="Unique execution identifier for tracking"
    )
    timestamp: UTCDatetime = Field(
        default_factory=now,
        description="Timestamp of context creation"
    )
    source_component: ComponentName = Field(
        description="Component that created this context"
    )
    system_fingerprint: str | None = Field(
        default=None,
        description="System fingerprint of the caller"
    )
    is_operator_auth_relay: bool = Field(
        default=False,
        description="INTERNAL ONLY: Set to true by from_request for exempted operator auth paths"
    )

    @field_validator("bound_operators", mode="before")
    @classmethod
    def parse_bound_operators(cls, v):
        """Parse bound_operators from JSON string (from HTTP header) or pass through list."""
        if isinstance(v, str):
            try:
                return json.loads(v)
            except (json.JSONDecodeError, TypeError) as exc:
                raise ValueError(
                    f"bound_operators header contains malformed JSON: {exc}"
                ) from exc
        return v

    @model_validator(mode="after")
    def validate_session_or_operator_auth(self):
        """Ensure either a session context (web_session_id or cli_session_id + user_id) or operator auth (null values with CLIENT source + exempt path)."""
        # If no session ID and no user_id, this must be operator auth relay from client on an exempt path
        if not self.web_session_id and not self.cli_session_id and not self.user_id:
            if not self.is_operator_auth_relay or self.source_component != ComponentName.CLIENT:
                raise ValueError(
                    "web_session_id, cli_session_id or user_id are required unless source_component is CLIENT and path is exempted (operator auth relay)"
                )
        return self

    def has_bound_operator(self) -> bool:
        """Returns True if at least one operator has status bound."""
        return any(op.status == OperatorStatus.BOUND for op in self.bound_operators)

    @classmethod
    def from_request_context(cls, request_context: RequestContext, is_exempt_path: bool = False) -> "G8eHttpContext":
        """Create G8eHttpContext from RequestContext (extracted from request body).
        
        This is the new preferred method for context extraction, eliminating
        the fragile header-as-state pattern.
        """
        return cls(
            web_session_id=request_context.web_session_id,
            cli_session_id=request_context.cli_session_id,
            user_id=request_context.user_id,
            organization_id=request_context.organization_id,
            case_id=request_context.case_id,
            investigation_id=request_context.investigation_id,
            task_id=request_context.task_id,
            bound_operators=request_context.bound_operators,
            execution_id=request_context.execution_id or generate_execution_id(),
            source_component=request_context.source_component,
            system_fingerprint=request_context.system_fingerprint,
            is_operator_auth_relay=is_exempt_path,
        )

    @classmethod
    async def from_request(cls, request: Request) -> "G8eHttpContext":
        """Extract and validate G8eHttpContext from FastAPI Request headers."""
        logger = get_logger(__name__)

        # Authoritative list of paths that CLIENT is allowed to call without web_session_id/user_id.
        # These are strictly limited to operator authentication and session management.
        # Chat endpoints now REQUIRE a web session ID to enforce human presence (including for evals).
        exempt_paths = {
            InternalApiPaths.G8EE_OPERATORS_REGISTER_SESSION,
            InternalApiPaths.G8EE_OPERATORS_DEREGISTER_SESSION,
            InternalApiPaths.G8EE_OPERATORS_AUTHENTICATE,
            InternalApiPaths.G8EE_OPERATORS_VALIDATE_SESSION,
            InternalApiPaths.G8EE_OPERATORS_REFRESH_SESSION,
            InternalApiPaths.G8EE_OPERATORS_DEVICE_LINK_REGISTER,
            InternalApiPaths.G8EE_OPERATORS_LISTEN_SESSION_AUTH,
        }
        is_exempt_path = request.url.path in exempt_paths

        logger.info(
            "[G8eHTTP-CONTEXT] Starting header validation",
            extra={"endpoint": request.url.path, "method": request.method, "is_exempt_path": is_exempt_path}
        )

        web_session_id = request.headers.get(G8eHeaders.WEB_SESSION_ID.lower())
        user_id = request.headers.get(G8eHeaders.USER_ID.lower())
        raw_source_component = request.headers.get(G8eHeaders.SOURCE_COMPONENT.lower())

        logger.info(
            "[G8eHTTP-CONTEXT] Extracted basic headers",
            extra={
                "endpoint": request.url.path,
                "has_web_session_id": bool(web_session_id),
                "has_user_id": bool(user_id),
                "has_source_component": bool(raw_source_component),
            }
        )

        if not web_session_id:
            if raw_source_component != ComponentName.CLIENT or not is_exempt_path:
                logger.error(
                    "SECURITY VIOLATION: Missing required %s header",
                    G8eHeaders.WEB_SESSION_ID,
                    extra={
                        "endpoint": request.url.path,
                        "source_component": raw_source_component,
                        "is_exempt_path": is_exempt_path,
                    }
                )
                from app.errors import AuthenticationError
                raise AuthenticationError(
                    f"{G8eHeaders.WEB_SESSION_ID} header is required for all internal requests",
                    component=ComponentName.G8EE,
                )
            logger.info(
                "[G8eHTTP-CONTEXT] web_session_id is null; allowed for CLIENT source on exempt path",
                extra={"endpoint": request.url.path}
            )

        if not user_id:
            if raw_source_component != ComponentName.CLIENT or not is_exempt_path:
                logger.error(
                    "SECURITY VIOLATION: Missing required %s header",
                    G8eHeaders.USER_ID,
                    extra={
                        "endpoint": request.url.path,
                        "web_session_id": web_session_id[:12] + "..." if web_session_id else None,
                        "source_component": raw_source_component,
                        "is_exempt_path": is_exempt_path,
                    }
                )
                from app.errors import AuthenticationError
                raise AuthenticationError(
                    f"{G8eHeaders.USER_ID} header is required for all internal requests",
                    component=ComponentName.G8EE,
                )
            logger.info(
                "[G8eHTTP-CONTEXT] user_id is null; allowed for CLIENT source on exempt path",
                extra={"endpoint": request.url.path}
            )

        if not raw_source_component:
            logger.error(
                "SECURITY VIOLATION: Missing required %s header",
                G8eHeaders.SOURCE_COMPONENT,
                extra={"endpoint": request.url.path}
            )
            from app.errors import AuthenticationError
            raise AuthenticationError(
                f"{G8eHeaders.SOURCE_COMPONENT} header is required for all internal requests",
                component=ComponentName.G8EE,
            )

        try:
            source_component = ComponentName(raw_source_component)
        except ValueError as err:
            logger.error(
                "SECURITY VIOLATION: Invalid %s header value",
                G8eHeaders.SOURCE_COMPONENT,
                extra={"endpoint": request.url.path, "value": raw_source_component}
            )
            from app.errors import AuthenticationError
            raise AuthenticationError(
                f"{G8eHeaders.SOURCE_COMPONENT} header contains an unrecognised component name",
                component=ComponentName.G8EE,
            ) from err

        case_id = request.headers.get(G8eHeaders.CASE_ID.lower())
        logger.info(
            "[G8eHTTP-CONTEXT] Extracted case_id",
            extra={
                "endpoint": request.url.path,
                "has_case_id": bool(case_id),
                "case_id": case_id[:20] + "..." if case_id else None,
            }
        )
        if not case_id:
            logger.error(
                "SECURITY VIOLATION: Missing required %s header",
                G8eHeaders.CASE_ID,
                extra={"endpoint": request.url.path}
            )
            from app.errors import AuthenticationError
            raise AuthenticationError(
                f"{G8eHeaders.CASE_ID} header is required for all internal requests",
                component=ComponentName.G8EE,
            )
        investigation_id = request.headers.get(G8eHeaders.INVESTIGATION_ID.lower())
        logger.info(
            "[G8eHTTP-CONTEXT] Extracted investigation_id",
            extra={
                "endpoint": request.url.path,
                "has_investigation_id": bool(investigation_id),
                "investigation_id": investigation_id[:20] + "..." if investigation_id else None,
            }
        )
        if not investigation_id:
            logger.error(
                "SECURITY VIOLATION: Missing required %s header",
                G8eHeaders.INVESTIGATION_ID,
                extra={"endpoint": request.url.path}
            )
            from app.errors import AuthenticationError
            raise AuthenticationError(
                f"{G8eHeaders.INVESTIGATION_ID} header is required for all internal requests",
                component=ComponentName.G8EE,
            )
        context_kwargs: dict[str, Any] = {
            "web_session_id": web_session_id,
            "user_id": user_id,
            "organization_id": request.headers.get(G8eHeaders.ORGANIZATION_ID.lower()),
            "case_id": case_id,
            "investigation_id": investigation_id,
            "task_id": request.headers.get(G8eHeaders.TASK_ID.lower()),
            "system_fingerprint": request.headers.get(G8eHeaders.SYSTEM_FINGERPRINT.lower()),
            "bound_operators": request.headers.get(G8eHeaders.BOUND_OPERATORS.lower(), "[]"),
            "source_component": source_component,
            "is_operator_auth_relay": is_exempt_path,
        }
        execution_id = request.headers.get(G8eHeaders.EXECUTION_ID.lower())
        if execution_id:
            context_kwargs["execution_id"] = execution_id

        context = cls(**context_kwargs)

        logger.info(
            "G8eHttpContext validated and extracted",
            extra={"web_session_id": web_session_id, "case_id": case_id, "investigation_id": investigation_id}
        )

        return context
