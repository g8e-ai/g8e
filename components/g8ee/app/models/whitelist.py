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

from app.constants import Platform

from .base import G8eBaseModel


class CommandValidationResult(G8eBaseModel):
    """Result of command validation against whitelist."""
    is_valid: bool
    command: str
    category: str | None = None
    platform: Platform | None = None
    reason: str | None = None
    max_execution_time: int | None = None
    safe_options_used: list[str] = Field(default_factory=list)
    violations: list[str] = Field(default_factory=list)
