# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

from .base import G8eBaseModel, Field, model_validator
from ..constants import ComponentName

class BoundOperator(G8eBaseModel):
    """Represents a bound Operator in the protocol context."""
    operator_id: str = Field(..., description="Unique Operator identifier")
    operator_session_id: str | None = Field(default=None, description="Operator session identifier")
    bound_web_session_id: str | None = Field(default=None, description="Web session ID this Operator is bound to")
    status: str | None = Field(default=None, description="Operator status")

class RequestContext(G8eBaseModel):
    """Request context embedded in request bodies instead of headers.
    
    Stabilized protocol version of the RequestContext model.
    """
    web_session_id: str | None = Field(
        default=None,
        description="Web user session ID"
    )
    cli_session_id: str | None = Field(
        default=None,
        description="CLI session ID"
    )
    user_id: str | None = Field(
        default=None,
        description="User identifier"
    )
    organization_id: str | None = Field(
        default=None,
        description="Organization identifier"
    )
    case_id: str | None = Field(
        default=None,
        description="Current case ID"
    )
    investigation_id: str | None = Field(
        default=None,
        description="Current investigation ID"
    )
    task_id: str | None = Field(
        default=None,
        description="Current task ID"
    )
    bound_operators: list[BoundOperator] = Field(
        default_factory=list,
        description="List of all bound operators"
    )
    execution_id: str | None = Field(
        default=None,
        description="Unique execution identifier"
    )
    source_component: str = Field(
        description="Component that created this context"
    )
    system_fingerprint: str | None = Field(
        default=None,
        description="System fingerprint of the caller"
    )

    @model_validator(mode="after")
    def validate_session_identity(self):
        """Basic validation of session identity."""
        if self.source_component == ComponentName.CLIENT:
            if self.web_session_id and self.cli_session_id:
                raise ValueError("Context cannot have both web_session_id and cli_session_id")
            
            if not self.web_session_id and not self.cli_session_id:
                raise ValueError("Context must have either web_session_id or cli_session_id for CLIENT source")
            if not self.user_id:
                raise ValueError("user_id is required for CLIENT source")
        return self
