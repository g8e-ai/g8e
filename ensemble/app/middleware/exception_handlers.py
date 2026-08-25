# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

import logging
from fastapi import FastAPI, Request
from fastapi.responses import JSONResponse

from app.errors import G8eError
from app.models.errors import ErrorBody, ErrorResponse

logger = logging.getLogger(__name__)


def setup_exception_handlers(app: FastAPI) -> None:
    """Register custom exception handlers for the FastAPI application."""

    @app.exception_handler(G8eError)
    async def g8e_error_handler(request: Request, exc: G8eError):
        """Handle custom G8eError exceptions and return structured JSON."""
        status_code = exc.get_http_status()
        error_detail = exc.error_detail

        # Extract trace/execution IDs if available in context
        g8e_context = getattr(request.state, "g8e_context", None)
        trace_id = "unknown"
        execution_id = "unknown"

        if g8e_context:
            execution_id = g8e_context.execution_id
            # trace_id is currently synonymous with execution_id in this substrate
            trace_id = execution_id

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
            },
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

        g8e_context = getattr(request.state, "g8e_context", None)
        trace_id = "unknown"
        execution_id = "unknown"

        if g8e_context:
            execution_id = g8e_context.execution_id
            trace_id = execution_id

        # Return a generic 500 error in production-safe format
        from app.constants import ErrorCode, ErrorCategory, ErrorSeverity

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
