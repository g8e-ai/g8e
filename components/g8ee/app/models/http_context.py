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
from typing import Any, TYPE_CHECKING

from fastapi import Request

from app.constants import (
    NEW_CASE_ID,
    ComponentName,
    OperatorStatus,
    G8eHeaders,
)
from app.utils.ids import generate_execution_id
from app.utils.timestamp import now

from .base import Field, G8eBaseModel, UTCDatetime, field_validator

if TYPE_CHECKING:
    pass


class BoundOperator(G8eBaseModel):
    """Represents a bound operator in the HTTP context.
    
    Canonical wire shape: shared/models/wire/bound_operator_context.json
    """
    operator_id: str = Field(..., description="Unique operator identifier")
    operator_session_id: str | None = Field(default=None, description="Operator session identifier")
    bound_web_session_id: str | None = Field(default=None, description="Web session ID this operator is bound to")
    status: OperatorStatus | None = Field(default=None, description="Operator status")


class G8eHttpContext(G8eBaseModel):
    """Standard context object for all internal HTTP requests."""

    web_session_id: str = Field(
        description="Web user session ID - used for routing SSE events to browser"
    )
    user_id: str = Field(
        description="User identifier - owner of the session and data"
    )
    organization_id: str | None = Field(
        default=None,
        description="Organization identifier - for multi-tenant data isolation"
    )
    case_id: str = Field(
        description="Current case ID being worked on"
    )
    investigation_id: str = Field(
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
    new_case: bool = Field(
        default=False,
        description="True when g8ed signals that no prior case exists and g8ee must create one inline"
    )
    source_component: ComponentName = Field(
        description="Component that created this context"
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
        return v if v is not None else []

    def has_bound_operator(self) -> bool:
        """Returns True if at least one operator has status bound."""
        return any(op.status == OperatorStatus.BOUND for op in self.bound_operators)

    @classmethod
    async def from_request(cls, request: Request) -> "G8eHttpContext":
        """Extract and validate G8eHttpContext from FastAPI Request headers."""
        from app.errors import AuthenticationError
        from app.logging import get_logger
        logger = get_logger(__name__)

        logger.info(
            "[G8eHTTP-CONTEXT] Starting header validation",
            extra={"endpoint": request.url.path, "method": request.method}
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
            logger.error(
                "SECURITY VIOLATION: Missing required %s header",
                G8eHeaders.WEB_SESSION_ID,
                extra={
                    "endpoint": request.url.path,
                    "source_component": raw_source_component,
                }
            )
            raise AuthenticationError(
                f"{G8eHeaders.WEB_SESSION_ID} header is required for all internal requests",
                component=ComponentName.G8EE,
            )

        if not user_id:
            logger.error(
                "SECURITY VIOLATION: Missing required %s header",
                G8eHeaders.USER_ID,
                extra={
                    "endpoint": request.url.path,
                    "web_session_id": web_session_id[:12] + "...",
                    "source_component": raw_source_component,
                }
            )
            raise AuthenticationError(
                f"{G8eHeaders.USER_ID} header is required for all internal requests",
                component=ComponentName.G8EE,
            )

        if not raw_source_component:
            logger.error(
                "SECURITY VIOLATION: Missing required %s header",
                G8eHeaders.SOURCE_COMPONENT,
                extra={"endpoint": request.url.path}
            )
            raise AuthenticationError(
                f"{G8eHeaders.SOURCE_COMPONENT} header is required for all internal requests",
                component=ComponentName.G8EE,
            )

        try:
            source_component = ComponentName(raw_source_component)
        except ValueError:
            logger.error(
                "SECURITY VIOLATION: Invalid %s header value",
                G8eHeaders.SOURCE_COMPONENT,
                extra={"endpoint": request.url.path, "value": raw_source_component}
            )
            raise AuthenticationError(
                f"{G8eHeaders.SOURCE_COMPONENT} header contains an unrecognised component name",
                component=ComponentName.G8EE,
            )

        new_case = request.headers.get(G8eHeaders.NEW_CASE.lower(), "").lower() == "true"

        case_id = request.headers.get(G8eHeaders.CASE_ID.lower())
        logger.info(
            "[G8eHTTP-CONTEXT] Extracted case_id",
            extra={
                "endpoint": request.url.path,
                "has_case_id": bool(case_id),
                "case_id": case_id[:20] + "..." if case_id else None,
                "new_case": new_case,
            }
        )
        if not case_id and not new_case:
            logger.error(
                "SECURITY VIOLATION: Missing required %s header",
                G8eHeaders.CASE_ID,
                extra={"endpoint": request.url.path}
            )
            raise AuthenticationError(
                f"{G8eHeaders.CASE_ID} header is required for all internal requests",
                component=ComponentName.G8EE,
            )
        elif new_case and not case_id:
            case_id = NEW_CASE_ID

        investigation_id = request.headers.get(G8eHeaders.INVESTIGATION_ID.lower())
        logger.info(
            "[G8eHTTP-CONTEXT] Extracted investigation_id",
            extra={
                "endpoint": request.url.path,
                "has_investigation_id": bool(investigation_id),
                "investigation_id": investigation_id[:20] + "..." if investigation_id else None,
                "new_case": new_case,
            }
        )
        if not investigation_id and not new_case:
            logger.error(
                "SECURITY VIOLATION: Missing required %s header",
                G8eHeaders.INVESTIGATION_ID,
                extra={"endpoint": request.url.path}
            )
            raise AuthenticationError(
                f"{G8eHeaders.INVESTIGATION_ID} header is required for all internal requests",
                component=ComponentName.G8EE,
            )
        elif new_case and not investigation_id:
            investigation_id = NEW_CASE_ID

        context_kwargs: dict[str, Any] = {
            "web_session_id": web_session_id,
            "user_id": user_id,
            "organization_id": request.headers.get(G8eHeaders.ORGANIZATION_ID.lower()),
            "case_id": case_id,
            "investigation_id": investigation_id,
            "new_case": new_case,
            "task_id": request.headers.get(G8eHeaders.TASK_ID.lower()),
            "bound_operators": request.headers.get(G8eHeaders.BOUND_OPERATORS.lower(), "[]"),
            "source_component": source_component,
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
