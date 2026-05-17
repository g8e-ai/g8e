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

from typing import Any
from .base import G8eBaseModel, Field, model_validator
from ..constants import ComponentName

class BoundOperator(G8eBaseModel):
    """Represents a bound operator in the protocol context."""
    operator_id: str = Field(..., description="Unique operator identifier")
    operator_session_id: str | None = Field(default=None, description="Operator session identifier")
    bound_web_session_id: str | None = Field(default=None, description="Web session ID this operator is bound to")
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
