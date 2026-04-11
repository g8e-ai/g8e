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

"""Models for HTTP client and service state."""

from typing import Optional
from app.models.base import VSOBaseModel

class HTTPClientStatus(VSOBaseModel):
    """Status information for an individual HTTP client."""
    service_name: str
    base_url: str
    is_session_closed: bool
    circuit_breaker_count: int

class HTTPServiceStatus(VSOBaseModel):
    """Complete status information for the HTTP service."""
    is_ready: bool
    active_clients: dict[str, HTTPClientStatus]
