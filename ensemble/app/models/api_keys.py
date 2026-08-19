# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

from app.constants import APIKeyStatus
from app.utils.timestamp import now
from .base import G8eBaseModel, UTCDatetime, Field


class APIKeyDocument(G8eBaseModel):
    """API key record stored in operator document store."""

    user_id: str = Field(description="User ID who owns this key")
    organization_id: str | None = Field(default=None, description="Organization ID")
    operator_id: str | None = Field(
        default=None, description="Operator ID if tied to a specific operator"
    )
    client_name: str = Field(description="Client name (e.g. 'operator', 'cli')")
    permissions: list[str] = Field(default_factory=list, description="List of granted permissions")
    status: APIKeyStatus = Field(default=APIKeyStatus.ACTIVE, description="Status of the key")
    system_fingerprint: str | None = Field(
        default=None, description="System fingerprint established on first use"
    )
    created_at: UTCDatetime = Field(default_factory=now, description="When the key was created")
    last_used_at: UTCDatetime | None = Field(default=None, description="When the key was last used")
    expires_at: UTCDatetime | None = Field(default=None, description="When the key expires")
    revoked_at: UTCDatetime | None = Field(default=None, description="When the key was revoked")
