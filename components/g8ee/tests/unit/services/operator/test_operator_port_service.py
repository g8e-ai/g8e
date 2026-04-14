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

Covers:
- Successful port check (open port, closed port)
- Port validation (invalid port numbers)
- Operator resolution: single operator, multi-operator with target, no operators
- Operator session missing
- Pubsub not ready
- Timeout handling
- Failed result from g8eo
- Unexpected result payload
- Generic exception handling
"""

import asyncio

import pytest

from app.constants import CommandErrorType, EventType, NetworkProtocol, OperatorStatus
from app.errors import BusinessLogicError, ValidationError
from app.models.command_payloads import CheckPortArgs
from app.models.operators import OperatorDocument, OperatorSystemInfo
from app.models.pubsub_messages import PortCheckResultPayload, G8eoResultEnvelope
from app.models.tool_results import PortCheckToolResult
from app.services.operator.port_service import OperatorPortService
from tests.fakes.factories import (
    build_enriched_context,
    build_operator_document,
    build_g8e_http_context,
)
from tests.fakes.fake_event_service import FakeEventService
from tests.fakes.fake_execution_registry import FakeExecutionRegistry
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
        operator_id=operator_id,
        user_id="user-1",
        web_session_id="ws-1",
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
) -> tuple[OperatorPortService, FakePubSubService, FakeExecutionRegistry, FakeExecutionService]:
    pubsub = FakePubSubService()
    if pubsub_ready:
        pubsub._ready = True
    registry = FakeExecutionRegistry()
    operator = resolved_operator or _make_operator()
    event_service = FakeEventService()
    execution = FakeExecutionService(resolved_operator=operator, resolve_error=resolve_error, g8ed_event_service=event_service)
    service = OperatorPortService(
        pubsub_service=pubsub,
        execution_registry=registry,
        execution_service=execution,
    )
    return service, pubsub, registry, execution


def _make_args(
    port: int = 443,
    host: str = "google.com",
    protocol: str = "tcp",
    target_operator: str | None = None,
) -> CheckPortArgs:
    return CheckPortArgs(
        port=port,
        host=host,
        protocol=protocol,
        target_operator=target_operator,
    )


def _make_investigation(operators: list[OperatorDocument] | None = None):
    return build_enriched_context(operator_documents=operators or [_make_operator()])


def _make_context():
    return build_g8e_http_context()


def _make_success_envelope(
    execution_id: str = "test",
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


def _make_failed_envelope(error: str = "Connection refused") -> G8eoResultEnvelope:
    return G8eoResultEnvelope(
        event_type=EventType.OPERATOR_NETWORK_PORT_CHECK_FAILED,
        operator_id="op-1",
        operator_session_id="session-1",
        payload=PortCheckResultPayload(
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
        service, pubsub, registry, _ = _make_service()
        investigation = _make_investigation()
        args = _make_args()

        async def _simulate():
            await asyncio.sleep(0.01)
            for eid in list(registry._events):
                registry.complete(eid, _make_success_envelope(is_open=True))

        task = task_tracker.track(asyncio.create_task(_simulate()))
        result = await service.execute_port_check(args, investigation, _make_context())
        await task

        assert result.success is True
        assert result.is_open is True
        assert result.host == "google.com"
        assert result.port == 443
        assert result.latency_ms == 12.5
        assert result.error_type is None

    @pytest.mark.asyncio
    async def test_port_closed(self, task_tracker):
        service, pubsub, registry, _ = _make_service()
        investigation = _make_investigation()
        args = _make_args()

        async def _simulate():
            await asyncio.sleep(0.01)
            for eid in list(registry._events):
                registry.complete(eid, _make_success_envelope(is_open=False, latency_ms=None))

        task = task_tracker.track(asyncio.create_task(_simulate()))
        result = await service.execute_port_check(args, investigation, _make_context())
        await task

        assert result.success is True
        assert result.is_open is False
        assert result.latency_ms is None

    @pytest.mark.asyncio
    async def test_publishes_command_to_pubsub(self, task_tracker):
        service, pubsub, registry, _ = _make_service()
        investigation = _make_investigation()
        args = _make_args(port=8080, host="redis-server")

        async def _simulate():
            await asyncio.sleep(0.01)
            for eid in list(registry._events):
                registry.complete(eid, _make_success_envelope(host="redis-server", port=8080))

        task = task_tracker.track(asyncio.create_task(_simulate()))
        await service.execute_port_check(args, investigation, _make_context())
        await task

        assert len(pubsub.published_commands) == 1
        msg = pubsub.published_commands[0]
        assert msg.event_type == EventType.OPERATOR_MCP_TOOLS_CALL
        assert msg.operator_id == "op-1"
        assert msg.operator_session_id == "session-1"

    @pytest.mark.asyncio
    async def test_registers_operator_session_before_publish(self, task_tracker):
        service, pubsub, registry, _ = _make_service()
        investigation = _make_investigation()
        args = _make_args()

        async def _simulate():
            await asyncio.sleep(0.01)
            for eid in list(registry._events):
                registry.complete(eid, _make_success_envelope())

        task = task_tracker.track(asyncio.create_task(_simulate()))
        await service.execute_port_check(args, investigation, _make_context())
        await task

        assert ("op-1", "session-1") in pubsub.registered_sessions

    @pytest.mark.asyncio
    async def test_allocates_and_releases_execution_id(self, task_tracker):
        service, pubsub, registry, _ = _make_service()
        investigation = _make_investigation()
        args = _make_args()

        async def _simulate():
            await asyncio.sleep(0.01)
            for eid in list(registry._events):
                registry.complete(eid, _make_success_envelope())

        task = task_tracker.track(asyncio.create_task(_simulate()))
        await service.execute_port_check(args, investigation, _make_context())
        await task

        assert len(registry.allocate_calls) == 1
        assert len(registry.release_calls) == 1
        assert registry.allocate_calls[0] == registry.release_calls[0]


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
        service, _, registry, _ = _make_service()

        async def _simulate():
            await asyncio.sleep(0.01)
            for eid in list(registry._events):
                registry.complete(eid, _make_success_envelope(port=1))

        task = task_tracker.track(asyncio.create_task(_simulate()))
        result = await service.execute_port_check(
            _make_args(port=1), _make_investigation(), _make_context(),
        )
        await task
        assert result.success is True

    @pytest.mark.asyncio
    async def test_port_65535_accepted(self, task_tracker):
        service, _, registry, _ = _make_service()

        async def _simulate():
            await asyncio.sleep(0.01)
            for eid in list(registry._events):
                registry.complete(eid, _make_success_envelope(port=65535))

        task = task_tracker.track(asyncio.create_task(_simulate()))
        result = await service.execute_port_check(
            _make_args(port=65535), _make_investigation(), _make_context(),
        )
        await task
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
        service, _, registry, execution = _make_service(resolved_operator=op)
        investigation = _make_investigation(operators=[op])

        async def _simulate():
            await asyncio.sleep(0.01)
            for eid in list(registry._events):
                registry.complete(eid, _make_success_envelope())

        task = task_tracker.track(asyncio.create_task(_simulate()))
        await service.execute_port_check(
            _make_args(target_operator="op-1"), investigation, _make_context(),
        )
        await task

        assert len(execution.resolve_calls) == 1
        call = execution.resolve_calls[0]
        assert call["operator_documents"] == [op]
        assert call["target_operator"] == "op-1"

    @pytest.mark.asyncio
    async def test_resolve_called_with_none_target_for_single_operator(self, task_tracker):
        op = _make_operator()
        service, _, registry, execution = _make_service(resolved_operator=op)
        investigation = _make_investigation(operators=[op])

        async def _simulate():
            await asyncio.sleep(0.01)
            for eid in list(registry._events):
                registry.complete(eid, _make_success_envelope())

        task = task_tracker.track(asyncio.create_task(_simulate()))
        await service.execute_port_check(
            _make_args(target_operator=None), investigation, _make_context(),
        )
        await task

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
    async def test_timeout_returns_error_and_releases(self):
        service, pubsub, registry, _ = _make_service()
        investigation = _make_investigation()
        args = _make_args()

        from unittest.mock import patch
        with patch("app.services.operator.port_service.OPERATOR_COMMAND_WAIT_TIMEOUT_SECONDS", 0.01):
            result = await service.execute_port_check(args, investigation, _make_context())

        assert result.success is False
        assert result.error_type == CommandErrorType.OPERATION_TIMEOUT
        assert "timed out" in result.error
        assert len(registry.release_calls) == 1


# ---------------------------------------------------------------------------
# g8eo result handling
# ---------------------------------------------------------------------------

class TestG8eoResultHandling:

    @pytest.mark.asyncio
    async def test_failed_event_type_returns_port_check_failed(self, task_tracker):
        service, pubsub, registry, _ = _make_service()
        investigation = _make_investigation()

        async def _simulate():
            await asyncio.sleep(0.01)
            for eid in list(registry._events):
                registry.complete(eid, _make_failed_envelope("Connection refused"))

        task = task_tracker.track(asyncio.create_task(_simulate()))
        result = await service.execute_port_check(_make_args(), investigation, _make_context())
        await task

        assert result.success is False
        assert result.error_type == CommandErrorType.PORT_CHECK_FAILED
        assert "Connection refused" in result.error

    @pytest.mark.asyncio
    async def test_failed_event_with_no_error_msg_uses_default(self, task_tracker):
        service, pubsub, registry, _ = _make_service()
        investigation = _make_investigation()

        envelope = G8eoResultEnvelope(
            event_type=EventType.OPERATOR_NETWORK_PORT_CHECK_FAILED,
            operator_id="op-1",
            operator_session_id="session-1",
            payload=PortCheckResultPayload(is_open=False, error=None),
        )

        async def _simulate():
            await asyncio.sleep(0.01)
            for eid in list(registry._events):
                registry.complete(eid, envelope)

        task = task_tracker.track(asyncio.create_task(_simulate()))
        result = await service.execute_port_check(_make_args(), investigation, _make_context())
        await task

        assert result.success is False
        assert result.error == "Port check failed"

    @pytest.mark.asyncio
    async def test_unexpected_payload_type_returns_error(self, task_tracker):
        service, pubsub, registry, _ = _make_service()
        investigation = _make_investigation()

        async def _simulate():
            await asyncio.sleep(0.01)
            for eid in list(registry._events):
                registry.complete(eid, "not_an_envelope")

        task = task_tracker.track(asyncio.create_task(_simulate()))
        result = await service.execute_port_check(_make_args(), investigation, _make_context())
        await task

        assert result.success is False
        assert result.error_type == CommandErrorType.EXECUTION_ERROR
        assert "Unexpected" in result.error

    @pytest.mark.asyncio
    async def test_envelope_with_wrong_payload_type_returns_error(self, task_tracker):
        service, pubsub, registry, _ = _make_service()
        investigation = _make_investigation()

        envelope = G8eoResultEnvelope(
            event_type=EventType.OPERATOR_NETWORK_PORT_CHECK_COMPLETED,
            operator_id="op-1",
            operator_session_id="session-1",
            payload=None,
        )

        async def _simulate():
            await asyncio.sleep(0.01)
            for eid in list(registry._events):
                registry.complete(eid, envelope)

        task = task_tracker.track(asyncio.create_task(_simulate()))
        result = await service.execute_port_check(_make_args(), investigation, _make_context())
        await task

        assert result.success is False
        assert result.error_type == CommandErrorType.EXECUTION_ERROR


# ---------------------------------------------------------------------------
# Generic exception handling
# ---------------------------------------------------------------------------

class TestExceptionHandling:

    @pytest.mark.asyncio
    async def test_unexpected_exception_returns_execution_error(self):
        service, pubsub, registry, _ = _make_service()
        investigation = _make_investigation()

        original_allocate = registry.allocate
        def _explode(eid):
            original_allocate(eid)
            raise RuntimeError("boom")

        registry.allocate = _explode
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
        service, _, registry, _ = _make_service()
        investigation = _make_investigation()

        async def _simulate():
            await asyncio.sleep(0.01)
            for eid in list(registry._events):
                envelope = _make_success_envelope()
                envelope.payload.protocol = "udp"
                registry.complete(eid, envelope)

        task = task_tracker.track(asyncio.create_task(_simulate()))
        result = await service.execute_port_check(
            _make_args(protocol="udp"), investigation, _make_context(),
        )
        await task

        assert result.success is True
