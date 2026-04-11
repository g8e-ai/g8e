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

from app.constants import AuthMethod

from .base import Field, VSOBaseModel


class AuthenticatedUser(VSOBaseModel):
    """Authenticated user context returned by g8ee auth dependencies."""
    uid: str = Field(description="User identifier (primary key)")
    user_id: str = Field(description="User identifier (alias for uid)")
    email: str | None = Field(default=None, description="User email address")
    name: str | None = Field(default=None, description="User display name")
    organization_id: str | None = Field(default=None, description="Organization identifier")
    web_session_id: str | None = Field(default=None, description="Web session ID (internal auth only)")
    auth_method: AuthMethod = Field(description="Authentication method used")
