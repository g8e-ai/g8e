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

from pydantic import Field
from app.utils.timestamp import now
from .base import G8eBaseModel, UTCDatetime

class ApiKeyDocument(G8eBaseModel):
    """API key record stored in g8es document store."""
    user_id: str = Field(description="User ID who owns this key")
    organization_id: str | None = Field(default=None, description="Organization ID")
    operator_id: str | None = Field(default=None, description="Operator ID if tied to a specific operator")
    client_name: str = Field(description="Client name (e.g. 'operator', 'cli')")
    permissions: list[str] = Field(default_factory=list, description="List of granted permissions")
    status: str = Field(default="ACTIVE", description="Status of the key (ACTIVE, REVOKED)")
    created_at: UTCDatetime = Field(default_factory=now, description="When the key was created")
    last_used_at: UTCDatetime | None = Field(default=None, description="When the key was last used")
    expires_at: UTCDatetime | None = Field(default=None, description="When the key expires")
