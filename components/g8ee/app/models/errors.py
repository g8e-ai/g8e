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
from datetime import datetime

from pydantic import field_validator
from app.constants import ErrorCategory, ErrorCode, ErrorSeverity
from app.models.base import Field, G8eBaseModel
from app.utils.timestamp import now


class ErrorCauseDetail(G8eBaseModel):
    """Structured cause information captured when a G8eError wraps another exception."""

    cause_message: str
    cause_stack_trace: list[str]


class ErrorDetail(G8eBaseModel):
    """Internal error detail — attached to every G8eError instance."""

    code: ErrorCode
    message: str
    category: ErrorCategory
    severity: ErrorSeverity = ErrorSeverity.MEDIUM
    timestamp: datetime = Field(default_factory=now)
    source: str 
    component: str | None = None
    trace_id: str | None = None
    execution_id: str | None = None
    details: dict[str, object] = Field(default_factory=dict)
    retry_suggested: bool = False
    remediation_steps: list[str] = Field(default_factory=list)
    cause: Any | None = None

    @field_validator("remediation_steps", mode="before")
    @classmethod
    def ensure_list(cls, v: Any) -> list[str]:
        if v is None:
            return []
        return v


class ErrorBody(G8eBaseModel):
    """HTTP response body for a single error — included inside ErrorResponse."""

    code: ErrorCode
    message: str
    category: ErrorCategory
    severity: ErrorSeverity
    timestamp: datetime = Field(default_factory=now)
    component: str | None = None
    details: dict[str, object] | None = None
    cause_message: str | None = None
    cause_stack_trace: list[str] | None = None


class ErrorResponse(G8eBaseModel):
    """Top-level HTTP error response envelope returned by setup_exception_handlers."""

    error: ErrorBody
    trace_id: str | None = None
    execution_id: str | None = None
