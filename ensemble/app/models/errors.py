# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

from typing import Any

from app.constants import ErrorCategory, ErrorCode, ErrorSeverity
from app.models.base import Field, G8eBaseModel, UTCDatetime, field_validator
from app.utils.timestamp import now


class ErrorCauseDetail(G8eBaseModel):
    """Structured cause information captured when a G8eError wraps another exception."""

    cause_message: str
    cause_stack_trace: list[str]


class ErrorDetail(G8eBaseModel):
    """Internal error detail - attached to every G8eError instance."""

    code: ErrorCode
    message: str
    category: ErrorCategory
    severity: ErrorSeverity = ErrorSeverity.MEDIUM
    timestamp: UTCDatetime = Field(default_factory=now)
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
    """HTTP response body for a single error - included inside ErrorResponse."""

    code: ErrorCode
    message: str
    category: ErrorCategory
    severity: ErrorSeverity
    timestamp: UTCDatetime = Field(default_factory=now)
    component: str | None = None
    details: dict[str, object] | None = None
    cause_message: str | None = None
    cause_stack_trace: list[str] | None = None


class ErrorResponse(G8eBaseModel):
    """Top-level HTTP error response envelope returned by setup_exception_handlers."""

    error: ErrorBody
    trace_id: str | None = None
    execution_id: str | None = None
