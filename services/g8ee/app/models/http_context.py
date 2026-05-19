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


from app.constants import (
    ComponentName,
    OperatorStatus,
)
from app.utils.ids import generate_execution_id
from app.utils.timestamp import now

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

    @classmethod
    def from_app_context(cls, context: "G8eHttpContext") -> "RequestContext":
        """Create a RequestContext from a G8eHttpContext."""
        return cls(
            web_session_id=context.web_session_id,
            cli_session_id=context.cli_session_id,
            user_id=context.user_id,
            organization_id=context.organization_id,
            case_id=context.case_id,
            investigation_id=context.investigation_id,
            task_id=context.task_id,
            bound_operators=context.bound_operators,
            execution_id=context.execution_id,
            source_component=context.source_component,
            system_fingerprint=context.system_fingerprint,
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
        """Ensure strict separation of session types and validate required context.
        
        Rules:
        1. For CLIENT source:
           - MUST have either web_session_id OR cli_session_id (mutually exclusive).
           - MUST NOT have both.
           - MUST have user_id.
        2. For non-CLIENT source:
           - MUST have either web_session_id, cli_session_id, or user_id (not anonymous).
        3. For operator auth relay (exempt paths):
           - Can have null sessions if source is CLIENT.
        4. If operators are bound:
           - Every bound operator MUST have an operator_session_id.
        """
        if self.is_operator_auth_relay and self.source_component == ComponentName.CLIENT:
            # Exempt paths (bootstrap) allowed to have no session for CLIENT
            return self

        # 1. Mutual Exclusivity and Presence for CLIENT source
        if self.source_component == ComponentName.CLIENT:
            if self.web_session_id and self.cli_session_id:
                raise ValueError("Context cannot have both web_session_id and cli_session_id")

            if not self.web_session_id and not self.cli_session_id:
                raise ValueError("Context must have either web_session_id or cli_session_id for CLIENT source")

            if not self.user_id:
                raise ValueError("user_id is required for CLIENT source")
        # 2. Minimum identity requirement for non-CLIENT sources
        elif not self.web_session_id and not self.cli_session_id and not self.user_id:
            raise ValueError("web_session_id, cli_session_id or user_id are required")

        # 3. Operator Session Validation
        if self.bound_operators:
            for op in self.bound_operators:
                if op.status == OperatorStatus.BOUND and not op.operator_session_id:
                    raise ValueError(f"Operator {op.operator_id} is BOUND but missing operator_session_id")

        return self

    def has_bound_operator(self) -> bool:
        """Returns True if at least one operator has status bound."""
        return any(op.status == OperatorStatus.BOUND for op in self.bound_operators)

    def validate_against_user(self, user: Any):
        """Verify that session IDs in context match the authenticated user's sessions.
        
        Args:
            user: AuthenticatedUser object (from headers).
        """
        # 1. User ID must match
        if self.user_id and self.user_id != user.uid:
             from app.errors import AuthenticationError
             raise AuthenticationError(f"User ID mismatch: context={self.user_id}, auth={user.uid}")

        # 2. Web session validation
        if self.web_session_id:
            if not user.web_session_id:
                 from app.errors import AuthenticationError
                 raise AuthenticationError("Web session context provided but not authenticated as a web session")
            if self.web_session_id != user.web_session_id:
                 from app.errors import AuthenticationError
                 raise AuthenticationError(f"Web session ID mismatch: context={self.web_session_id}, auth={user.web_session_id}")

        # 3. CLI session validation
        if self.cli_session_id:
            if not user.cli_session_id:
                 from app.errors import AuthenticationError
                 raise AuthenticationError("CLI session context provided but not authenticated as a CLI session")
            if self.cli_session_id != user.cli_session_id:
                 from app.errors import AuthenticationError
                 raise AuthenticationError(f"CLI session ID mismatch: context={self.cli_session_id}, auth={user.cli_session_id}")

        # 4. Operator session check for bound operators
        if self.bound_operators and user.operator_session_id:
            for op in self.bound_operators:
                if op.operator_session_id and op.operator_session_id != user.operator_session_id:
                    from app.errors import AuthenticationError
                    raise AuthenticationError(f"Operator session ID mismatch for operator {op.operator_id}")

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

