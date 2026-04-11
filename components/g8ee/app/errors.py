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

import traceback
from typing import Any

from app.constants import ErrorCategory, ErrorCode, ErrorSeverity
from app.models.errors import ErrorCauseDetail, ErrorDetail

class VSOError(Exception):
    def __init__(
        self,
        message: str,
        code: ErrorCode,
        category: ErrorCategory,
        severity: ErrorSeverity = ErrorSeverity.MEDIUM,
        source: str = "g8ee",
        details: dict[str, Any] | None = None,
        trace_id: str = "unknown",
        execution_id: str = "unknown",
        retry_suggested: bool = False,
        remediation_steps: list[str] | None = None,
        cause: Exception | None = None,
        component: str = "g8ee",
    ):
        details_dict = details or {}

        if cause and not isinstance(cause, ErrorCauseDetail):
            cause_detail = ErrorCauseDetail(
                cause_message=str(cause),
                cause_stack_trace=traceback.format_exception(type(cause), cause, cause.__traceback__)
            )
        else:
            cause_detail = cause

        self.error_detail = ErrorDetail(
            code=code,
            message=message,
            category=category,
            severity=severity,
            source=source,
            component=component,
            details=details_dict,
            trace_id=trace_id,
            execution_id=execution_id,
            retry_suggested=retry_suggested,
            remediation_steps=remediation_steps,
            cause=cause_detail,
        )
        self.cause = cause

        super().__init__(message)

    @property
    def code(self) -> ErrorCode:
        return self.error_detail.code

    @property
    def retry_suggested(self) -> bool:
        return self.error_detail.retry_suggested

    @property
    def category(self) -> ErrorCategory:
        return self.error_detail.category

    @property
    def severity(self) -> ErrorSeverity:
        return self.error_detail.severity

    @property
    def component(self) -> str | None:
        return self.error_detail.component

    def __str__(self) -> str:
        result = f"{self.error_detail.code}: {self.error_detail.message}"
        if self.cause:
            result += f" Caused by: {self.cause!s}"
        return result

    def get_http_status(self) -> int:
        category_to_status = {
            ErrorCategory.VALIDATION: 400,
            ErrorCategory.BUSINESS_LOGIC: 400,
            ErrorCategory.AUTH: 401,
            ErrorCategory.PERMISSION: 403,
            ErrorCategory.RESOURCE_NOT_FOUND: 404,
            ErrorCategory.CONFLICT: 409,
            ErrorCategory.RATE_LIMIT: 429,
            ErrorCategory.SERVICE_UNAVAILABLE: 503,
            ErrorCategory.EXTERNAL_SERVICE: 502,
            ErrorCategory.TIMEOUT: 504,
        }
        return category_to_status.get(self.error_detail.category, 500)


# Configuration Errors
class ConfigurationError(VSOError):
    def __init__(
        self,
        message: str,
        code: ErrorCode = ErrorCode.CONFIG_ERROR,
        cause: Exception | None = None,
        details: dict[str, Any] | None = None,
        component: str = "g8ee",
    ):
        super().__init__(
            message,
            code=code,
            category=ErrorCategory.CONFIGURATION,
            cause=cause,
            details=details,
            component=component,
        )

class MissingBootstrapSecretError(ConfigurationError):
    def __init__(self, secret_name: str):
        super().__init__(
            f"Missing required bootstrap secret: {secret_name}",
            code=ErrorCode.MISSING_ENV_VAR
        )


# Database Errors
class DatabaseError(VSOError):
    def __init__(
        self,
        message: str,
        code: ErrorCode = ErrorCode.DB_CONNECTION_ERROR,
        cause: Exception | None = None,
        details: dict[str, Any] | None = None,
        component: str = "g8ee",
    ):
        super().__init__(
            message,
            code=code,
            category=ErrorCategory.DATABASE,
            cause=cause,
            details=details,
            component=component,
        )

class DatabaseQueryError(DatabaseError):
    def __init__(self, message: str, query: str | None = None, cause: Exception | None = None):
        super().__init__(
            message,
            code=ErrorCode.DB_QUERY_ERROR,
            cause=cause,
            details={"query": query} if query else None
        )


class PubSubError(VSOError):
    def __init__(
        self,
        message: str,
        code: ErrorCode = ErrorCode.PUBSUB_CONNECTION_ERROR,
        cause: Exception | None = None,
        details: dict[str, Any] | None = None,
        component: str = "g8ee",
    ):
        super().__init__(
            message,
            code=code,
            category=ErrorCategory.PUBSUB,
            cause=cause,
            details=details,
            component=component,
        )


class StorageError(VSOError):
    def __init__(
        self,
        message: str,
        code: ErrorCode = ErrorCode.STORAGE_CONNECTION_ERROR,
        cause: Exception | None = None,
        details: dict[str, Any] | None = None,
        component: str = "g8ee",
    ):
        super().__init__(
            message,
            code=code,
            category=ErrorCategory.STORAGE,
            cause=cause,
            details=details,
            component=component,
        )


class ResourceNotFoundError(VSOError):
    def __init__(
        self,
        message: str,
        resource_type: str,
        resource_id: str,
        code: ErrorCode = ErrorCode.GENERIC_NOT_FOUND,
        cause: Exception | None = None,
        details: dict[str, Any] | None = None,
        component: str = "g8ee",
    ):
        self.resource_type = resource_type
        self.resource_id = resource_id
        full_details = details or {}
        full_details.update({
            "resource_type": resource_type,
            "resource_id": resource_id
        })
        super().__init__(
            message,
            code=code,
            category=ErrorCategory.RESOURCE_NOT_FOUND,
            cause=cause,
            details=full_details,
            component=component
        )

class ResourceConflictError(VSOError):
    def __init__(self, message: str, resource_type: str | None = None, resource_id: str | None = None):
        super().__init__(
            message,
            code=ErrorCode.RESOURCE_CONFLICT,
            category=ErrorCategory.CONFLICT,
            details={"resource_type": resource_type, "resource_id": resource_id}
        )


# Auth Errors
class AuthenticationError(VSOError):
    def __init__(
        self,
        message: str = "Authentication failed",
        code: ErrorCode = ErrorCode.AUTH_ERROR,
        details: dict[str, Any] | None = None,
        component: str = "g8ee",
    ):
        super().__init__(
            message,
            code=code,
            category=ErrorCategory.AUTH,
            severity=ErrorSeverity.HIGH,
            details=details,
            component=component,
        )

class TokenExpiredError(AuthenticationError):
    def __init__(self):
        super().__init__("Token has expired", code=ErrorCode.TOKEN_EXPIRED)

class AuthorizationError(VSOError):
    def __init__(self, message: str = "Insufficient permissions", component: str = "g8ee"):
        super().__init__(
            message,
            code=ErrorCode.INSUFFICIENT_PERMISSIONS,
            category=ErrorCategory.PERMISSION,
            severity=ErrorSeverity.HIGH,
            component=component
        )


# Validation Errors
class ValidationError(VSOError):
    def __init__(
        self,
        message: str,
        field: str | None = None,
        constraint: str | None = None,
        details: dict[str, Any] | None = None,
        component: str = "g8ee",
    ):
        full_details = details or {}
        if field:
            full_details["field"] = field
        if constraint:
            full_details["constraint"] = constraint

        super().__init__(
            message,
            code=ErrorCode.VALIDATION_ERROR,
            category=ErrorCategory.VALIDATION,
            details=full_details,
            component=component,
        )

class MissingRequiredFieldError(ValidationError):
    def __init__(self, field: str):
        super().__init__(f"Missing required field: {field}", field=field, constraint="required")


# Network & External Service Errors
class NetworkError(VSOError):
    def __init__(
        self,
        message: str,
        code: ErrorCode = ErrorCode.API_CONNECTION_ERROR,
        severity: ErrorSeverity = ErrorSeverity.MEDIUM,
        cause: Exception | None = None,
        details: dict[str, Any] | None = None,
        retry_suggested: bool = False,
        component: str = "g8ee",
    ):
        super().__init__(
            message,
            code=code,
            category=ErrorCategory.NETWORK,
            severity=severity,
            cause=cause,
            details=details,
            retry_suggested=retry_suggested,
            component=component,
        )

class RateLimitError(VSOError):
    def __init__(self, message: str = "Rate limit exceeded", component: str = "g8ee"):
        super().__init__(
            message,
            code=ErrorCode.API_RATE_LIMIT_ERROR,
            category=ErrorCategory.RATE_LIMIT,
            retry_suggested=True,
            component=component
        )

class VSOTimeoutError(VSOError):
    def __init__(self, message: str = "Operation timed out", component: str = "g8ee"):
        super().__init__(
            message,
            code=ErrorCode.API_TIMEOUT_ERROR,
            category=ErrorCategory.TIMEOUT,
            retry_suggested=True,
            component=component
        )

class ExternalServiceError(VSOError):
    def __init__(
        self,
        message: str,
        service_name: str = "unknown",
        code: ErrorCode = ErrorCode.EXTERNAL_SERVICE_ERROR,
        cause: Exception | None = None,
        details: dict[str, Any] | None = None,
        component: str = "g8ee",
    ):
        self.service_name = service_name
        full_details = details or {}
        full_details["service_name"] = service_name
        super().__init__(
            message,
            code=code,
            category=ErrorCategory.EXTERNAL_SERVICE,
            cause=cause,
            details=full_details,
            component=component
        )
    
    @property
    def message(self) -> str:
        return self.error_detail.message


# Business Logic Errors
class BusinessLogicError(VSOError):
    def __init__(
        self,
        message: str,
        code: ErrorCode = ErrorCode.BUSINESS_LOGIC_ERROR,
        component: str = "g8ee",
        details: dict[str, Any] | None = None,
    ):
        super().__init__(
            message,
            code=code,
            category=ErrorCategory.BUSINESS_LOGIC,
            component=component,
            details=details,
        )

# Infrastructure Errors
class ServiceUnavailableError(VSOError):
    def __init__(self, message: str = "Service unavailable", component: str = "g8ee"):
        super().__init__(
            message,
            code=ErrorCode.SERVICE_UNAVAILABLE_ERROR,
            category=ErrorCategory.SERVICE_UNAVAILABLE,
            retry_suggested=True,
            component=component
        )


# Catch-all
class InternalUnexpectedError(VSOError):
    def __init__(self, message: str = "An unexpected internal error occurred", cause: Exception | None = None):
        super().__init__(
            message,
            code=ErrorCode.UNEXPECTED_ERROR,
            category=ErrorCategory.INTERNAL,
            severity=ErrorSeverity.CRITICAL,
            cause=cause
        )
