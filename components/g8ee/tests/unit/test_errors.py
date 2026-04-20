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

from datetime import datetime

import pytest

from app.constants import ErrorCategory, ErrorCode, ErrorSeverity
from app.errors import (
    AuthenticationError,
    AuthorizationError,
    BusinessLogicError,
    ConfigurationError,
    DatabaseError,
    ExternalServiceError,
    NetworkError,
    PubSubError,
    RateLimitError,
    ResourceConflictError,
    ResourceNotFoundError,
    ServiceUnavailableError,
    StorageError,
    ValidationError,
    G8eError,
    G8eTimeoutError,
)
from app.models.errors import ErrorBody, ErrorCauseDetail, ErrorDetail, ErrorResponse

pytestmark = [pytest.mark.unit]


# ---------------------------------------------------------------------------
# Fixtures
# ---------------------------------------------------------------------------

@pytest.fixture
def basic_error() -> G8eError:
    return G8eError("basic error message", code=ErrorCode.GENERIC_ERROR, category=ErrorCategory.INTERNAL, source="test")


@pytest.fixture
def error_with_cause() -> G8eError:
    cause = ValueError("root cause")
    return G8eError("wrapping message", code=ErrorCode.GENERIC_ERROR, category=ErrorCategory.INTERNAL, cause=cause, source="test")


@pytest.fixture
def error_with_all_fields() -> G8eError:
    return G8eError(
        message="full error",
        code=ErrorCode.DB_WRITE_ERROR,
        category=ErrorCategory.DATABASE,
        severity=ErrorSeverity.HIGH,
        source="test.module",
        details={"key": "value"},
        trace_id="trace-abc",
        execution_id="req-xyz",
        retry_suggested=True,
        remediation_steps=["check db", "retry"],
        component="test-component",
    )


@pytest.fixture
def validation_error() -> ValidationError:
    return ValidationError("invalid input", field="email", constraint="must be valid email")


@pytest.fixture
def not_found_error() -> ResourceNotFoundError:
    return ResourceNotFoundError("investigation not found", resource_type="investigation", resource_id="inv-123")


# ---------------------------------------------------------------------------
# ErrorCauseDetail model
# ---------------------------------------------------------------------------

class TestErrorCauseDetail:
    def test_construction(self):
        detail = ErrorCauseDetail(
            cause_message="something went wrong",
            cause_stack_trace=["line 1", "line 2"],
        )
        assert detail.cause_message == "something went wrong"
        assert detail.cause_stack_trace == ["line 1", "line 2"]

    def test_wire_dump_includes_all_fields(self):
        detail = ErrorCauseDetail(
            cause_message="msg",
            cause_stack_trace=["frame"],
        )
        wire = detail.model_dump(mode="json")
        assert wire["cause_message"] == "msg"
        assert wire["cause_stack_trace"] == ["frame"]


# ---------------------------------------------------------------------------
# ErrorDetail model
# ---------------------------------------------------------------------------

class TestErrorDetail:
    def test_defaults(self):
        detail = ErrorDetail(
            code=ErrorCode.GENERIC_ERROR,
            message="test",
            category=ErrorCategory.INTERNAL,
            source="unknown",
        )
        assert detail.severity == ErrorSeverity.MEDIUM
        assert detail.source == "unknown"
        assert detail.component is None
        assert detail.trace_id is None
        assert detail.execution_id is None
        assert detail.details == {}
        assert detail.retry_suggested is False
        assert detail.remediation_steps == []
        assert detail.cause is None
        assert isinstance(detail.timestamp, datetime)

    def test_with_cause(self):
        cause = ErrorCauseDetail(cause_message="root", cause_stack_trace=["frame"])
        detail = ErrorDetail(
            code=ErrorCode.DB_WRITE_ERROR,
            message="write failed",
            category=ErrorCategory.DATABASE,
            cause=cause,
            source="test",
        )
        assert detail.cause is not None
        assert detail.cause.cause_message == "root"

    def test_wire_dump_omits_none(self):
        detail = ErrorDetail(
            code=ErrorCode.GENERIC_ERROR,
            message="test",
            category=ErrorCategory.INTERNAL,
            source="test",
        )
        wire = detail.model_dump(mode="json")
        assert "component" not in wire
        assert "trace_id" not in wire
        assert "execution_id" not in wire
        assert "cause" not in wire

    def test_wire_dump_includes_set_fields(self):
        detail = ErrorDetail(
            code=ErrorCode.VALIDATION_ERROR,
            message="bad input",
            category=ErrorCategory.VALIDATION,
            component="api",
            trace_id="trace-1",
            source="test",
        )
        wire = detail.model_dump(mode="json")
        assert wire["component"] == "api"
        assert wire["trace_id"] == "trace-1"


# ---------------------------------------------------------------------------
# ErrorBody and ErrorResponse models
# ---------------------------------------------------------------------------

class TestErrorBodyAndResponse:
    def test_error_body_construction(self):
        body = ErrorBody(
            code=ErrorCode.GENERIC_ERROR,
            message="test",
            category=ErrorCategory.INTERNAL,
            severity=ErrorSeverity.MEDIUM,
        )
        assert body.code == ErrorCode.GENERIC_ERROR
        assert body.message == "test"
        assert body.component is None
        assert body.details is None
        assert body.cause_message is None
        assert body.cause_stack_trace is None

    def test_error_response_flatten_for_wire(self):
        body = ErrorBody(
            code=ErrorCode.GENERIC_ERROR,
            message="test",
            category=ErrorCategory.INTERNAL,
            severity=ErrorSeverity.MEDIUM,
        )
        response = ErrorResponse(error=body, trace_id="trace-1")
        wire = response.model_dump(mode="json")
        assert "error" in wire
        assert wire["trace_id"] == "trace-1"
        assert "execution_id" not in wire

    def test_error_response_omits_none_fields(self):
        body = ErrorBody(
            code=ErrorCode.GENERIC_ERROR,
            message="test",
            category=ErrorCategory.INTERNAL,
            severity=ErrorSeverity.MEDIUM,
        )
        wire = ErrorResponse(error=body).model_dump(mode="json")
        assert "trace_id" not in wire
        assert "execution_id" not in wire


# ---------------------------------------------------------------------------
# G8eError base class
# ---------------------------------------------------------------------------

class TestG8eError:
    def test_basic_construction(self, basic_error):
        assert basic_error.error_detail.code == ErrorCode.GENERIC_ERROR
        assert basic_error.error_detail.message == "basic error message"
        assert basic_error.error_detail.category == ErrorCategory.INTERNAL
        assert basic_error.error_detail.severity == ErrorSeverity.MEDIUM
        assert basic_error.cause is None

    def test_str_without_cause(self, basic_error):
        result = str(basic_error)
        assert "G8E-1000" in result
        assert "basic error message" in result
        assert "Caused by" not in result

    def test_str_with_cause(self, error_with_cause):
        result = str(error_with_cause)
        assert "Caused by" in result
        assert "root cause" in result

    def test_properties(self, error_with_all_fields):
        assert error_with_all_fields.code == ErrorCode.DB_WRITE_ERROR
        assert error_with_all_fields.category == ErrorCategory.DATABASE
        assert error_with_all_fields.severity == ErrorSeverity.HIGH
        assert error_with_all_fields.component == "test-component"
        assert error_with_all_fields.retry_suggested is True

    def test_cause_captured_in_error_detail(self, error_with_cause):
        assert error_with_cause.error_detail.cause is not None
        # G8eError.cause stores the raw Exception, but ErrorDetail.cause is a model with cause_message
        assert error_with_cause.error_detail.cause.cause_message == "root cause"
        assert isinstance(error_with_cause.error_detail.cause.cause_stack_trace, list)
        assert len(error_with_cause.error_detail.cause.cause_stack_trace) > 0

    def test_cause_without_traceback(self):
        cause = Exception("no traceback")
        error = G8eError("wrapped", code=ErrorCode.GENERIC_ERROR, category=ErrorCategory.INTERNAL, cause=cause, source="test")
        assert error.error_detail.cause is not None
        assert error.error_detail.cause.cause_message == "no traceback"

    def test_is_exception_subclass(self, basic_error):
        assert isinstance(basic_error, Exception)

    def test_can_be_raised_and_caught(self, basic_error):
        with pytest.raises(G8eError):
            raise basic_error


# ---------------------------------------------------------------------------
# HTTP status mapping
# ---------------------------------------------------------------------------

class TestGetHttpStatus:
    @pytest.mark.parametrize("category,expected_status", [
        (ErrorCategory.VALIDATION, 400),
        (ErrorCategory.BUSINESS_LOGIC, 400),
        (ErrorCategory.AUTH, 401),
        (ErrorCategory.PERMISSION, 403),
        (ErrorCategory.RESOURCE_NOT_FOUND, 404),
        (ErrorCategory.CONFLICT, 409),
        (ErrorCategory.RATE_LIMIT, 429),
        (ErrorCategory.SERVICE_UNAVAILABLE, 503),
        (ErrorCategory.EXTERNAL_SERVICE, 502),
        (ErrorCategory.TIMEOUT, 504),
        (ErrorCategory.INTERNAL, 500),
        (ErrorCategory.DATABASE, 500),
        (ErrorCategory.NETWORK, 500),
        (ErrorCategory.PUBSUB, 500),
        (ErrorCategory.STORAGE, 500),
        (ErrorCategory.CONFIGURATION, 500),
    ])
    def test_status_mapping(self, category, expected_status):
        error = G8eError("msg", code=ErrorCode.GENERIC_ERROR, category=category)
        assert error.get_http_status() == expected_status


# ---------------------------------------------------------------------------
# Error subclasses — defaults and specialised fields
# ---------------------------------------------------------------------------

class TestErrorSubclasses:
    def test_configuration_error_defaults(self):
        e = ConfigurationError("bad config")
        assert e.code == ErrorCode.CONFIG_ERROR
        assert e.category == ErrorCategory.CONFIGURATION

    def test_database_error_defaults(self):
        e = DatabaseError("db failed")
        assert e.code == ErrorCode.DB_CONNECTION_ERROR
        assert e.category == ErrorCategory.DATABASE

    def test_resource_not_found_defaults(self, not_found_error):
        assert not_found_error.code == ErrorCode.GENERIC_NOT_FOUND
        assert not_found_error.category == ErrorCategory.RESOURCE_NOT_FOUND
        assert not_found_error.get_http_status() == 404

    def test_resource_not_found_resource_fields(self, not_found_error):
        assert not_found_error.error_detail.details["resource_type"] == "investigation"
        assert not_found_error.error_detail.details["resource_id"] == "inv-123"

    def test_resource_not_found_without_resource_fields(self):
        with pytest.raises(TypeError):
            ResourceNotFoundError("not found")

    def test_resource_not_found_with_fields(self):
        e = ResourceNotFoundError("not found", resource_type="user", resource_id="123")
        assert e.category == ErrorCategory.RESOURCE_NOT_FOUND
        assert e.error_detail.details["resource_type"] == "user"
        assert e.error_detail.details["resource_id"] == "123"

    def test_authentication_error_defaults(self):
        e = AuthenticationError("bad token")
        assert e.code == ErrorCode.AUTH_ERROR
        assert e.category == ErrorCategory.AUTH
        assert e.get_http_status() == 401

    def test_authorization_error_defaults(self):
        e = AuthorizationError("forbidden")
        assert e.code == ErrorCode.INSUFFICIENT_PERMISSIONS
        assert e.category == ErrorCategory.PERMISSION
        assert e.get_http_status() == 403

    def test_validation_error_defaults(self, validation_error):
        assert validation_error.code == ErrorCode.VALIDATION_ERROR
        assert validation_error.category == ErrorCategory.VALIDATION
        assert validation_error.get_http_status() == 400

    def test_validation_error_field_and_constraint(self, validation_error):
        assert validation_error.error_detail.details["field"] == "email"
        assert validation_error.error_detail.details["constraint"] == "must be valid email"

    def test_validation_error_without_optional_fields(self):
        e = ValidationError("generic validation failure")
        assert "field" not in e.error_detail.details
        assert "constraint" not in e.error_detail.details

    def test_pubsub_error_defaults(self):
        e = PubSubError("publish failed")
        assert e.code == ErrorCode.PUBSUB_CONNECTION_ERROR
        assert e.category == ErrorCategory.PUBSUB

    def test_storage_error_defaults(self):
        e = StorageError("upload failed")
        assert e.code == ErrorCode.STORAGE_CONNECTION_ERROR
        assert e.category == ErrorCategory.STORAGE

    def test_network_error_defaults(self):
        e = NetworkError("connection refused")
        assert e.code == ErrorCode.API_CONNECTION_ERROR
        assert e.category == ErrorCategory.NETWORK

    def test_rate_limit_error_defaults(self):
        e = RateLimitError("too many requests")
        assert e.code == ErrorCode.API_RATE_LIMIT_ERROR
        assert e.category == ErrorCategory.RATE_LIMIT
        assert e.retry_suggested is True
        assert e.get_http_status() == 429

    def test_business_logic_error_defaults(self):
        e = BusinessLogicError("precondition failed")
        assert e.code == ErrorCode.BUSINESS_LOGIC_ERROR
        assert e.category == ErrorCategory.BUSINESS_LOGIC
        assert e.get_http_status() == 400

    def test_resource_conflict_error_defaults(self):
        e = ResourceConflictError("already exists")
        assert e.code == ErrorCode.RESOURCE_CONFLICT
        assert e.category == ErrorCategory.CONFLICT
        assert e.get_http_status() == 409

    def test_service_unavailable_error_defaults(self):
        e = ServiceUnavailableError("service not ready")
        assert e.code == ErrorCode.SERVICE_UNAVAILABLE_ERROR
        assert e.category == ErrorCategory.SERVICE_UNAVAILABLE
        assert e.retry_suggested is True
        assert e.get_http_status() == 503

    def test_external_service_error_defaults(self):
        e = ExternalServiceError("third party failed")
        assert e.code == ErrorCode.EXTERNAL_SERVICE_ERROR
        assert e.category == ErrorCategory.EXTERNAL_SERVICE
        assert e.get_http_status() == 502
        assert e.error_detail.details["service_name"] == "unknown"

    def test_g8e_timeout_error_defaults(self):
        e = G8eTimeoutError("timed out")
        assert e.code == ErrorCode.API_TIMEOUT_ERROR
        assert e.category == ErrorCategory.TIMEOUT
        assert e.retry_suggested is True
        assert e.get_http_status() == 504

    def test_business_logic_error_defaults(self):
        e = BusinessLogicError("operation failed")
        assert e.code == ErrorCode.BUSINESS_LOGIC_ERROR
        assert e.category == ErrorCategory.BUSINESS_LOGIC

    def test_subclass_code_override(self):
        e = DatabaseError("query failed", code=ErrorCode.DB_QUERY_ERROR)
        assert e.code == ErrorCode.DB_QUERY_ERROR
        assert e.category == ErrorCategory.DATABASE

    def test_all_subclasses_are_g8e_error(self):
        subclasses: list[G8eError] = [
            ConfigurationError("x"),
            DatabaseError("x"),
            ResourceNotFoundError("x", "type", "id"),
            AuthenticationError("x"),
            AuthorizationError("x"),
            ValidationError("x"),
            PubSubError("x"),
            StorageError("x"),
            NetworkError("x"),
            RateLimitError("x"),
            BusinessLogicError("x"),
            ResourceConflictError("x"),
            ServiceUnavailableError("x"),
            ExternalServiceError("x"),
            G8eTimeoutError("x"),
        ]
        for e in subclasses:
            assert isinstance(e, G8eError)
            assert isinstance(e, Exception)
