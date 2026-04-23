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

"""
Unit tests for OperatorPortService (port_service.py).
"""

import pytest
import asyncio
from typing import cast

from app.constants import CommandErrorType, EventType, OperatorStatus, NetworkProtocol
from app.constants.status import ExecutionStatus
from app.errors import BusinessLogicError, ValidationError
from app.models.command_request_payloads import CheckPortRequestPayload
from app.models.operators import OperatorDocument, OperatorSystemInfo
from app.models.pubsub_messages import PortCheckResultPayload, G8eoResultEnvelope
from app.models.tool_results import CommandInternalResult
from app.services.operator.port_service import OperatorPortService
from tests.fakes.factories import (
    build_enriched_context,
    build_g8e_http_context,
)
from tests.fakes.fake_event_service import FakeEventService
from tests.fakes.fake_execution_service import FakeExecutionService
from tests.fakes.fake_pubsub_service import FakePubSubService

pytestmark = pytest.mark.unit

# ---------------------------------------------------------------------------
# Helpers
# ---------------------------------------------------------------------------

def _make_operator(
    operator_id: str = "op-1",
    operator_session_id: str = "session-1",
    hostname: str = "host-1",
) -> OperatorDocument:
    return OperatorDocument(
        id=operator_id,
        user_id="user-1",
        bound_web_session_id="ws-1",
        operator_session_id=operator_session_id,
        current_hostname=hostname,
        status=OperatorStatus.BOUND,
        system_info=OperatorSystemInfo(hostname=hostname, os="linux", architecture="x86_64"),
    )

def _make_service(
    *,
    pubsub_ready: bool = True,
    resolved_operator: OperatorDocument | None = None,
    resolve_error: Exception | None = None,
) -> tuple[OperatorPortService, FakePubSubService, FakeExecutionService]:
    pubsub = FakePubSubService()
    if pubsub_ready:
        pubsub._ready = True
    operator = resolved_operator or _make_operator()
    event_service = FakeEventService()
    execution = FakeExecutionService(
        resolved_operator=operator, 
        resolve_error=resolve_error, 
        g8ed_event_service=event_service,
        pubsub_service=pubsub
    )
    service = OperatorPortService(
        pubsub_service=pubsub,
        execution_service=execution,
    )
    return service, pubsub, execution

def _make_args(
    port: int = 443,
    host: str = "google.com",
    protocol: str = "tcp",
    target_operator: str | None = None,
    execution_id: str | None = None,
) -> CheckPortRequestPayload:
    return CheckPortRequestPayload(
        port=port,
        host=host,
        protocol=protocol,
        target_operator=target_operator,
        execution_id=execution_id or "test-exec-id",
    )

def _make_investigation(operators: list[OperatorDocument] | None = None):
    return build_enriched_context(operator_documents=operators or [_make_operator()])

def _make_context():
    return build_g8e_http_context()

def _make_success_envelope(
    execution_id: str = "test-exec-id",
    host: str = "google.com",
    port: int = 443,
    is_open: bool = True,
    latency_ms: float = 12.5,
) -> G8eoResultEnvelope:
    return G8eoResultEnvelope(
        event_type=EventType.OPERATOR_NETWORK_PORT_CHECK_COMPLETED,
        operator_id="op-1",
        operator_session_id="session-1",
        payload=PortCheckResultPayload(
            execution_id=execution_id,
            host=host,
            port=port,
            protocol="tcp",
            is_open=is_open,
            latency_ms=latency_ms,
        ),
    )

def _make_failed_envelope(error: str = "Connection refused", execution_id: str = "test-exec-id") -> G8eoResultEnvelope:
    return G8eoResultEnvelope(
        event_type=EventType.OPERATOR_NETWORK_PORT_CHECK_FAILED,
        operator_id="op-1",
        operator_session_id="session-1",
        payload=PortCheckResultPayload(
            execution_id=execution_id,
            host="google.com",
            port=443,
            protocol="tcp",
            is_open=False,
            error=error,
        ),
    )

# ---------------------------------------------------------------------------
# Happy path
# ---------------------------------------------------------------------------

class TestPortCheckSuccess:

    @pytest.mark.asyncio
    async def test_port_open(self, task_tracker):
        service, pubsub, execution = _make_service()
        investigation = _make_investigation()
        args = _make_args()

        execution._envelope = _make_success_envelope(is_open=True)

        result = await service.execute_port_check(args, investigation, _make_context())

        assert result.success is True
        assert result.is_open is True
        assert result.host == "google.com"
        assert result.port == 443
        assert result.latency_ms == 12.5
        assert result.error_type is None

    @pytest.mark.asyncio
    async def test_port_closed(self, task_tracker):
        service, pubsub, execution = _make_service()
        investigation = _make_investigation()
        args = _make_args()

        execution._envelope = _make_success_envelope(is_open=False, latency_ms=None)

        result = await service.execute_port_check(args, investigation, _make_context())

        assert result.success is True
        assert result.is_open is False
        assert result.latency_ms is None

    @pytest.mark.asyncio
    async def test_publishes_command_to_pubsub(self, task_tracker):
        service, pubsub, execution = _make_service()
        investigation = _make_investigation()
        args = _make_args(port=8080, host="redis-server")

        execution._envelope = _make_success_envelope(host="redis-server", port=8080)

        await service.execute_port_check(args, investigation, _make_context())

        # PortService calls execution_service.execute, which calls pubsub_service.publish_command
        assert len(pubsub.published_commands) == 1
        msg = pubsub.published_commands[0]
        assert msg.event_type == EventType.OPERATOR_NETWORK_PORT_CHECK_REQUESTED
        assert msg.operator_id == "op-1"
        assert msg.operator_session_id == "session-1"

    @pytest.mark.asyncio
    async def test_registers_operator_session_before_publish(self, task_tracker):
        service, pubsub, execution = _make_service()
        investigation = _make_investigation()
        args = _make_args()

        execution._envelope = _make_success_envelope()

        await service.execute_port_check(args, investigation, _make_context())

        assert ("op-1", "session-1") in pubsub.registered_sessions

# ---------------------------------------------------------------------------
# Port validation
# ---------------------------------------------------------------------------

class TestPortValidation:

    @pytest.mark.asyncio
    async def test_port_zero_rejected(self):
        service, *_ = _make_service()
        result = await service.execute_port_check(
            _make_args(port=0), _make_investigation(), _make_context(),
        )
        assert result.success is False
        assert result.error_type == CommandErrorType.VALIDATION_ERROR
        assert "Invalid port" in result.error

    @pytest.mark.asyncio
    async def test_port_negative_rejected(self):
        service, *_ = _make_service()
        result = await service.execute_port_check(
            _make_args(port=-1), _make_investigation(), _make_context(),
        )
        assert result.success is False
        assert result.error_type == CommandErrorType.VALIDATION_ERROR

    @pytest.mark.asyncio
    async def test_port_above_65535_rejected(self):
        service, *_ = _make_service()
        result = await service.execute_port_check(
            _make_args(port=65536), _make_investigation(), _make_context(),
        )
        assert result.success is False
        assert result.error_type == CommandErrorType.VALIDATION_ERROR

    @pytest.mark.asyncio
    async def test_port_1_accepted(self, task_tracker):
        service, _, execution = _make_service()

        execution._envelope = _make_success_envelope(port=1)

        result = await service.execute_port_check(
            _make_args(port=1), _make_investigation(), _make_context(),
        )
        assert result.success is True

    @pytest.mark.asyncio
    async def test_port_65535_accepted(self, task_tracker):
        service, _, execution = _make_service()

        execution._envelope = _make_success_envelope(port=65535)

        result = await service.execute_port_check(
            _make_args(port=65535), _make_investigation(), _make_context(),
        )
        assert result.success is True

# ---------------------------------------------------------------------------
# Operator resolution
# ---------------------------------------------------------------------------

class TestOperatorResolution:

    @pytest.mark.asyncio
    async def test_no_operator_documents_returns_error(self):
        service, *_ = _make_service(
            resolve_error=BusinessLogicError("No operators bound to this session", component="g8ee"),
        )
        investigation = build_enriched_context(operator_documents=[])
        result = await service.execute_port_check(
            _make_args(), investigation, _make_context(),
        )
        assert result.success is False
        assert result.error_type == CommandErrorType.OPERATOR_RESOLUTION_ERROR
        assert "No operators" in result.error

    @pytest.mark.asyncio
    async def test_validation_error_from_resolve_returns_error(self):
        service, *_ = _make_service(
            resolve_error=ValidationError("Could not resolve target_operator 'bad'", component="g8ee"),
        )
        investigation = _make_investigation()
        result = await service.execute_port_check(
            _make_args(target_operator="bad"), investigation, _make_context(),
        )
        assert result.success is False
        assert result.error_type == CommandErrorType.OPERATOR_RESOLUTION_ERROR

    @pytest.mark.asyncio
    async def test_operator_session_missing_returns_error(self):
        op_no_session = _make_operator(operator_session_id=None)
        service, *_ = _make_service(resolved_operator=op_no_session)
        investigation = _make_investigation(operators=[op_no_session])
        result = await service.execute_port_check(
            _make_args(), investigation, _make_context(),
        )
        assert result.success is False
        assert result.error_type == CommandErrorType.NO_OPERATORS_AVAILABLE
        assert "session not found" in result.error

    @pytest.mark.asyncio
    async def test_resolve_called_with_correct_args(self, task_tracker):
        op = _make_operator()
        service, _, execution = _make_service(resolved_operator=op)
        investigation = _make_investigation(operators=[op])

        execution._envelope = _make_success_envelope()

        await service.execute_port_check(
            _make_args(target_operator="op-1"), investigation, _make_context(),
        )

        assert len(execution.resolve_calls) == 1
        call = execution.resolve_calls[0]
        assert call["operator_documents"] == [op]
        assert call["target_operator"] == "op-1"

    @pytest.mark.asyncio
    async def test_resolve_called_with_none_target_for_single_operator(self, task_tracker):
        op = _make_operator()
        service, _, execution = _make_service(resolved_operator=op)
        investigation = _make_investigation(operators=[op])

        execution._envelope = _make_success_envelope()

        await service.execute_port_check(
            _make_args(target_operator=None), investigation, _make_context(),
        )

        assert execution.resolve_calls[0]["target_operator"] is None

# ---------------------------------------------------------------------------
# Pubsub not ready
# ---------------------------------------------------------------------------

class TestPubsubNotReady:

    @pytest.mark.asyncio
    async def test_returns_error_when_pubsub_not_ready(self):
        service, *_ = _make_service(pubsub_ready=False)
        result = await service.execute_port_check(
            _make_args(), _make_investigation(), _make_context(),
        )
        assert result.success is False
        assert result.error_type == CommandErrorType.PUBSUB_SUBSCRIPTION_NOT_READY
        assert "not ready" in result.error

# ---------------------------------------------------------------------------
# Timeout
# ---------------------------------------------------------------------------

class TestTimeout:

    @pytest.mark.asyncio
    async def test_timeout_returns_error(self):
        service, pubsub, execution = _make_service()
        investigation = _make_investigation()
        args = _make_args()

        # Simulate timeout by having execution return no envelope
        execution._envelope = None
        
        async def _mock_execute(*args, **kwargs):
            from app.models.tool_results import CommandInternalResult
            from app.constants.status import ExecutionStatus, CommandErrorType
            return CommandInternalResult(
                execution_id="test",
                status=ExecutionStatus.TIMEOUT,
                error="timed out",
                error_type=CommandErrorType.COMMAND_TIMEOUT
            ), None
            
        execution.execute = _mock_execute

        result = await service.execute_port_check(args, investigation, _make_context())

        assert result.success is False
        assert result.error_type == CommandErrorType.OPERATION_TIMEOUT
        assert "timed out" in result.error

# ---------------------------------------------------------------------------
# g8eo result handling
# ---------------------------------------------------------------------------

class TestG8eoResultHandling:

    @pytest.mark.asyncio
    async def test_failed_event_type_returns_port_check_failed(self, task_tracker):
        service, pubsub, execution = _make_service()
        investigation = _make_investigation()

        execution._envelope = _make_failed_envelope("Connection refused")

        result = await service.execute_port_check(_make_args(), investigation, _make_context())

        assert result.success is False
        assert result.error_type == CommandErrorType.PORT_CHECK_FAILED
        assert "Connection refused" in result.error

    @pytest.mark.asyncio
    async def test_failed_event_with_no_error_msg_uses_default(self, task_tracker):
        service, pubsub, execution = _make_service()
        investigation = _make_investigation()

        execution._envelope = G8eoResultEnvelope(
            event_type=EventType.OPERATOR_NETWORK_PORT_CHECK_FAILED,
            operator_id="op-1",
            operator_session_id="session-1",
            payload=PortCheckResultPayload(execution_id="exec-test", is_open=False, error=None),
        )

        result = await service.execute_port_check(_make_args(), investigation, _make_context())

        assert result.success is False
        assert result.error == "Port check failed"

    @pytest.mark.asyncio
    async def test_unexpected_payload_type_returns_error(self, task_tracker):
        service, pubsub, execution = _make_service()
        investigation = _make_investigation()

        execution._envelope = "not_an_envelope"

        result = await service.execute_port_check(_make_args(), investigation, _make_context())

        assert result.success is False
        assert result.error_type == CommandErrorType.EXECUTION_ERROR
        assert "Unexpected" in result.error

    @pytest.mark.asyncio
    async def test_envelope_with_wrong_payload_type_returns_error(self, task_tracker):
        service, pubsub, execution = _make_service()
        investigation = _make_investigation()

        from app.models.pubsub_messages import ExecutionResultsPayload
        execution._envelope = G8eoResultEnvelope(
            event_type=EventType.OPERATOR_NETWORK_PORT_CHECK_COMPLETED,
            operator_id="op-1",
            operator_session_id="session-1",
            payload=ExecutionResultsPayload(
                execution_id="test-exec-id",
                status=ExecutionStatus.COMPLETED,
            ),
        )

        result = await service.execute_port_check(_make_args(), investigation, _make_context())

        assert result.success is False
        assert result.error_type == CommandErrorType.EXECUTION_ERROR

# ---------------------------------------------------------------------------
# Generic exception handling
# ---------------------------------------------------------------------------

class TestExceptionHandling:

    @pytest.mark.asyncio
    async def test_unexpected_exception_returns_execution_error(self):
        service, pubsub, execution = _make_service()
        investigation = _make_investigation()

        async def _explode(*args, **kwargs):
            raise RuntimeError("boom")

        execution.execute = _explode
        result = await service.execute_port_check(_make_args(), investigation, _make_context())

        assert result.success is False
        assert result.error_type == CommandErrorType.EXECUTION_ERROR
        assert "boom" in result.error

    @pytest.mark.asyncio
    async def test_resolve_validation_error_returned_as_result(self):
        service, *_ = _make_service(
            resolve_error=ValidationError("bad target", component="g8ee"),
        )
        result = await service.execute_port_check(
            _make_args(), _make_investigation(), _make_context(),
        )
        assert result.success is False
        assert result.error_type == CommandErrorType.OPERATOR_RESOLUTION_ERROR
        assert "bad target" in result.error

    @pytest.mark.asyncio
    async def test_resolve_business_logic_error_returned_as_result(self):
        service, *_ = _make_service(
            resolve_error=BusinessLogicError("no operators", component="g8ee"),
        )
        result = await service.execute_port_check(
            _make_args(), _make_investigation(), _make_context(),
        )
        assert result.success is False
        assert result.error_type == CommandErrorType.OPERATOR_RESOLUTION_ERROR

    @pytest.mark.asyncio
    async def test_invalid_protocol_returns_execution_error(self):
        service, *_ = _make_service()
        result = await service.execute_port_check(
            _make_args(protocol="invalid_proto"),
            _make_investigation(),
            _make_context(),
        )
        assert result.success is False
        assert result.error_type == CommandErrorType.EXECUTION_ERROR

# ---------------------------------------------------------------------------
# Protocol
# ---------------------------------------------------------------------------

class TestProtocol:

    @pytest.mark.asyncio
    async def test_udp_protocol_accepted(self, task_tracker):
        service, _, execution = _make_service()
        investigation = _make_investigation()

        envelope = _make_success_envelope()
        envelope.payload.protocol = "udp"
        execution._envelope = envelope

        result = await service.execute_port_check(
            _make_args(protocol="udp"), investigation, _make_context(),
        )

        assert result.success is True
