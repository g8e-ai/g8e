# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

from app.constants import AuthMethod

from .base import Field, G8eBaseModel


class OperatorSessionValidationRequest(G8eBaseModel):
    operator_session_id: str
    cli_session_id: str
    user_id: str


class OperatorSessionValidationResponse(G8eBaseModel):
    valid: bool
    operator_id: str
    user_id: str


class AuthenticatedUser(G8eBaseModel):
    """Authenticated user context returned by g8ee auth dependencies."""

    uid: str = Field(description="User identifier (primary key)")
    user_id: str = Field(description="User identifier (alias for uid)")
    email: str | None = Field(default=None, description="User email address")
    name: str | None = Field(default=None, description="User display name")
    organization_id: str | None = Field(default=None, description="Organization identifier")
    web_session_id: str | None = Field(default=None, description="Web session ID")
    cli_session_id: str | None = Field(default=None, description="CLI session ID")
    operator_session_id: str | None = Field(default=None, description="Operator session ID")
    auth_method: AuthMethod = Field(description="Authentication method used")
