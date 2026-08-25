# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

"""
Session models for g8e system.
"""

from __future__ import annotations

from typing import Any

from app.constants import SessionType
from app.models.base import Field
from app.utils.timestamp import now

from .base import G8eBaseModel, UTCDatetime


class SessionDocument(G8eBaseModel):
    """Base session document model."""

    id: str = Field(description="Unique session identifier")
    session_type: SessionType = Field(description="Type of session (WEB, OPERATOR, CLI)")
    user_id: str = Field(description="User ID who owns this session")
    organization_id: str | None = Field(default=None, description="Organization ID")
    user_data: dict[str, Any] | None = Field(default=None, description="User profile data")
    api_key: str | None = Field(default=None, description="Encrypted API key if applicable")
    client_ip: str | None = Field(default=None, description="Client IP address")
    user_agent: str | None = Field(default=None, description="Client user agent")
    login_method: str | None = Field(default=None, description="Method used to login")
    created_at: UTCDatetime = Field(default_factory=now, description="When session was created")
    absolute_expires_at: UTCDatetime = Field(description="Absolute expiration timestamp")
    idle_expires_at: UTCDatetime = Field(description="Idle expiration timestamp")
    last_activity: UTCDatetime = Field(default_factory=now, description="Last activity timestamp")
    last_ip: str | None = Field(default=None, description="Last known IP address")
    ip_changes: int = Field(default=0, description="Count of IP changes detected")
    suspicious_activity: bool = Field(default=False, description="Flag for suspicious activity")
    is_active: bool = Field(default=True, description="Whether session is active")
    operator_status: str | None = Field(default=None, description="Operator status if applicable")
    metadata: dict[str, Any] | None = Field(default=None, description="Additional session metadata")


class WebSessionDocument(SessionDocument):
    """Web browser session document."""

    session_type: SessionType = Field(default=SessionType.WEB)
    operator_ids: list[str] = Field(default_factory=list, description="Bound operator IDs")


class OperatorSessionDocument(SessionDocument):
    """Operator process session document."""

    session_type: SessionType = Field(default=SessionType.OPERATOR)
    operator_id: str = Field(description="Operator identifier (required for operator sessions)")


class CliSessionDocument(SessionDocument):
    """CLI tool session document."""

    session_type: SessionType = Field(default=SessionType.CLI)
    operator_session_id: str | None = Field(
        default=None, description="Operator session ID this CLI session is bound to"
    )
