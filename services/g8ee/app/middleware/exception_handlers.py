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

import logging
from fastapi import FastAPI, Request
from fastapi.responses import JSONResponse

from ..errors import G8eError
from ..models.errors import ErrorBody, ErrorResponse
from ..constants import G8eHeaders

logger = logging.getLogger(__name__)

def setup_exception_handlers(app: FastAPI) -> None:
    """Register custom exception handlers for the FastAPI application."""

    @app.exception_handler(G8eError)
    async def g8e_error_handler(request: Request, exc: G8eError):
        """Handle custom G8eError exceptions and return structured JSON."""
        status_code = exc.get_http_status()
        error_detail = exc.error_detail

        # Extract trace/execution IDs if available in context/headers
        trace_id = request.headers.get("X-G8E-Trace-ID", "unknown")
        execution_id = request.headers.get(G8eHeaders.EXECUTION_ID, "unknown")

        error_body = ErrorBody(
            code=error_detail.code,
            message=error_detail.message,
            category=error_detail.category,
            severity=error_detail.severity,
            timestamp=error_detail.timestamp,
            component=error_detail.component,
            details=error_detail.details,
            cause_message=error_detail.cause.cause_message if error_detail.cause else None,
            cause_stack_trace=error_detail.cause.cause_stack_trace if error_detail.cause else None,
        )

        response_envelope = ErrorResponse(
            error=error_body,
            trace_id=trace_id,
            execution_id=execution_id,
        )

        logger.error(
            "[EXCEPTION-HANDLER] G8eError caught: code=%s status=%d message=%s",
            error_detail.code.value,
            status_code,
            error_detail.message,
            extra={
                "trace_id": trace_id,
                "execution_id": execution_id,
                "category": error_detail.category.value,
            }
        )

        return JSONResponse(
            status_code=status_code,
            content=response_envelope.model_dump(mode="json"),
        )

    @app.exception_handler(Exception)
    async def universal_exception_handler(request: Request, exc: Exception):
        """Catch-all for unhandled exceptions to prevent leaking internals."""
        # Log the full exception for internal debugging
        logger.exception("[EXCEPTION-HANDLER] Unhandled exception caught: %s", exc)

        trace_id = request.headers.get("X-G8E-Trace-ID", "unknown")
        execution_id = request.headers.get("X-G8E-Execution-ID", "unknown")

        # Return a generic 500 error in production-safe format
        from ..constants import ErrorCode, ErrorCategory, ErrorSeverity
        error_body = ErrorBody(
            code=ErrorCode.UNEXPECTED_ERROR,
            message="An unexpected internal error occurred",
            category=ErrorCategory.INTERNAL,
            severity=ErrorSeverity.CRITICAL,
            component="g8ee",
        )

        response_envelope = ErrorResponse(
            error=error_body,
            trace_id=trace_id,
            execution_id=execution_id,
        )

        return JSONResponse(
            status_code=500,
            content=response_envelope.model_dump(mode="json"),
        )
