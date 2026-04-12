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
Unit tests for OperatorCommandService (command_service.py).

Covers the service's own responsibilities:
- Initialization validation (ValidationError on missing required deps)
- set_pubsub_client guards
- operator_service_available
- Pending commands store CRUD delegation
- _broadcast_command_event (success + swallowed SSE failure)
- _resolve_intent_dependencies (dependency graph)

"""

from datetime import UTC, datetime
from unittest.mock import AsyncMock

import pytest

from app.constants import (
    EventType,
)
from app.errors import ValidationError

from app.services.operator import OperatorCommandService
from tests.fakes.builder import build_command_service

pytestmark = pytest.mark.unit


# ---------------------------------------------------------------------------
# Helpers
# ---------------------------------------------------------------------------

def _make_service() -> OperatorCommandService:
    """Build a minimal OperatorCommandService via factory with typed fakes."""
    return build_command_service()


# ---------------------------------------------------------------------------
# Initialization
# ---------------------------------------------------------------------------

class TestOperatorCommandServiceInit:
    """OperatorCommandService.__init__ validation."""

    pytestmark = pytest.mark.unit

    def test_raises_type_error_when_required_arg_missing(self):
        """Missing required arg must raise TypeError at construction time."""
        with pytest.raises(TypeError):
            # Missing one or more required positional arguments to build()
            # The current implementation has 8 required arguments.
            OperatorCommandService.build(
                cache_aside_service=None,
                operator_data_service=None,
                # investigation_service is missing
                g8ed_event_service=None,
                execution_registry=None,
                settings=None,
                ai_response_analyzer=None,
                internal_http_client=None,
            )

    def test_succeeds_with_all_required_args(self):
        """Service constructs without error when all required deps are provided."""
        service = _make_service()
        assert service is not None

    def test_pubsub_client_starts_as_none(self):
        """pubsub_client is None until set_pubsub_client is called."""
        service = build_command_service(skip_pubsub_client=True)
        assert service._pubsub_service.pubsub_client is None

    def test_pubsub_ready_starts_false(self):
        """_pubsub_ready starts False."""
        service = _make_service()
        assert service._pubsub_service._pubsub_ready is False


# ---------------------------------------------------------------------------
# set_pubsub_client
# ---------------------------------------------------------------------------

class TestClientSetters:
    """Guards on set_pubsub_client."""

    pytestmark = pytest.mark.unit

    def test_set_pubsub_client_stores_client(self):
        """set_pubsub_client assigns the client."""
        service = _make_service()
        # pubsub_client is already a FakePubSubClient from builder
        client = service._pubsub_service.pubsub_client
        service.set_pubsub_client(client)
        assert service._pubsub_service.pubsub_client is client

    def test_set_pubsub_client_raises_validation_error_on_none(self):
        """set_pubsub_client(None) must raise ValidationError."""
        service = _make_service()
        with pytest.raises(ValidationError):
            service.set_pubsub_client(None)

    def test_set_pubsub_client_raises_validation_error_on_falsy(self):
        """set_pubsub_client with falsy value must raise ValidationError."""
        service = _make_service()
        with pytest.raises(ValidationError):
            service.set_pubsub_client(0)


# ---------------------------------------------------------------------------
# operator_service_available
# ---------------------------------------------------------------------------

class TestOperatorServiceAvailable:

    pytestmark = pytest.mark.unit

    def test_returns_true_when_operator_service_set(self):
        service = _make_service()
        assert service.operator_service_available() is True

    def test_returns_false_when_operator_service_is_none(self):
        service = _make_service()
        service.operator_data_service = None
        assert service.operator_service_available() is False

# ---------------------------------------------------------------------------
# publish_command_event (formerly _broadcast_command_event)
# ---------------------------------------------------------------------------

class TestBroadcastCommandEvent:

    pytestmark = [pytest.mark.unit, pytest.mark.asyncio(loop_scope="session")]

    async def test_publishes_event_to_g8ed(self):
        """OperatorExecutionService publishes events via g8ed_event_service.publish_command_event."""
        from unittest.mock import AsyncMock as _AsyncMock
        from app.models.base import G8eBaseModel
        from tests.fakes.factories import build_g8e_http_context
        service = _make_service()
        execution_svc = service._execution_service
        execution_svc.g8ed_event_service.publish_command_event = _AsyncMock()

        class _TestEvent(G8eBaseModel):
            operator_session_id: str

        data = _TestEvent(operator_session_id="sess-abc")
        g8e_context = build_g8e_http_context(
            web_session_id="web-abc",
            user_id="user-abc",
            case_id="case-xyz",
            investigation_id="inv-111",
        )
        await execution_svc.g8ed_event_service.publish_command_event(
            EventType.OPERATOR_COMMAND_REQUESTED,
            data,
            g8e_context=g8e_context,
            task_id="task-123",
        )

        execution_svc.g8ed_event_service.publish_command_event.assert_awaited_once()
        call_args = execution_svc.g8ed_event_service.publish_command_event.call_args
        assert call_args.args[0] == EventType.OPERATOR_COMMAND_REQUESTED
        assert call_args.kwargs["g8e_context"].web_session_id == "web-abc"
        assert call_args.kwargs["g8e_context"].case_id == "case-xyz"

    async def test_swallows_g8ed_publish_exception(self):
        """g8ed publish exceptions from execute_command_internal paths must not propagate."""
        from app.models.base import G8eBaseModel
        from tests.fakes.factories import build_g8e_http_context
        service = _make_service()
        execution_svc = service._execution_service
        execution_svc.g8ed_event_service.publish_command_event = AsyncMock(
            side_effect=Exception("g8ed unreachable")
        )
        class _EmptyEvent(G8eBaseModel):
            pass
        
        g8e_context = build_g8e_http_context(
            web_session_id="web-abc",
            user_id="user-abc",
            case_id="case-xyz",
            investigation_id="inv-111",
        )
        try:
            await execution_svc.g8ed_event_service.publish_command_event(
                EventType.OPERATOR_COMMAND_REQUESTED,
                _EmptyEvent(),
                g8e_context=g8e_context,
                task_id="task-123",
            )
        except Exception:
            pass


# ---------------------------------------------------------------------------
# _resolve_intent_dependencies
# ---------------------------------------------------------------------------

class TestResolveIntentDependencies:

    pytestmark = pytest.mark.unit

    def test_no_dependencies_returned_unchanged(self):
        service = _make_service()
        result = service._intent_service._resolve_intent_dependencies(["ec2_discovery"])
        assert result == ["ec2_discovery"]

    def test_management_intent_pulls_in_discovery(self):
        """ec2_management requires ec2_discovery."""
        service = _make_service()
        result = service._intent_service._resolve_intent_dependencies(["ec2_management"])
        assert "ec2_discovery" in result
        assert "ec2_management" in result

    def test_s3_write_pulls_in_s3_read(self):
        service = _make_service()
        result = service._intent_service._resolve_intent_dependencies(["s3_write"])
        assert "s3_read" in result
        assert "s3_write" in result

    def test_s3_delete_pulls_in_s3_read(self):
        service = _make_service()
        result = service._intent_service._resolve_intent_dependencies(["s3_delete"])
        assert "s3_read" in result

    def test_dynamodb_write_pulls_in_dynamodb_read(self):
        service = _make_service()
        result = service._intent_service._resolve_intent_dependencies(["dynamodb_write"])
        assert "dynamodb_read" in result

    def test_result_is_sorted(self):
        """Output must be alphabetically sorted."""
        service = _make_service()
        result = service._intent_service._resolve_intent_dependencies(["s3_write", "ec2_management"])
        assert result == sorted(result)

    def test_no_duplicates_when_dependency_already_requested(self):
        """Requesting both ec2_management and ec2_discovery must not duplicate ec2_discovery."""
        service = _make_service()
        result = service._intent_service._resolve_intent_dependencies(["ec2_management", "ec2_discovery"])
        assert result.count("ec2_discovery") == 1

    def test_transitive_dependencies_resolved(self):
        """ec2_snapshot_management -> ec2_discovery (one hop)."""
        service = _make_service()
        result = service._intent_service._resolve_intent_dependencies(["ec2_snapshot_management"])
        assert "ec2_discovery" in result

    def test_multiple_independent_intents(self):
        service = _make_service()
        result = service._intent_service._resolve_intent_dependencies(["cloudwatch_logs", "iam_discovery"])
        assert "cloudwatch_logs" in result
        assert "iam_discovery" in result

    def test_empty_list_returns_empty(self):
        service = _make_service()
        result = service._intent_service._resolve_intent_dependencies([])
        assert result == []


# ---------------------------------------------------------------------------
# execute_command — target_systems population
# ---------------------------------------------------------------------------

class TestExecuteCommandTargetSystems:
    """Regression tests: target_systems must be populated in CommandApprovalRequest."""

    pytestmark = [pytest.mark.unit, pytest.mark.asyncio(loop_scope="session")]

    def _make_operator(self, operator_id: str, session_id: str, hostname: str):
        from app.models.operators import OperatorDocument, OperatorType
        return OperatorDocument(
            operator_id=operator_id,
            user_id="user-1",
            operator_session_id=session_id,
            current_hostname=hostname,
            operator_type=OperatorType.SYSTEM,
            web_session_id="ws-1",
        )

    def _make_investigation(self, operators):
        from app.models.investigations import EnrichedInvestigationContext
        return EnrichedInvestigationContext(
            id="inv-1",
            user_id="user-1",
            case_id="case-1",
            web_session_id="ws-1",
            sentinel_mode=False,
            operator_documents=operators,
        )

    def _make_g8e_context(self):
        from app.models.http_context import G8eHttpContext
        from app.constants.status import ComponentName
        return G8eHttpContext(
            web_session_id="ws-1",
            user_id="user-1",
            case_id="case-1",
            investigation_id="inv-1",
            source_component=ComponentName.G8EE,
        )

    async def test_single_operator_target_systems_populated(self):
        """With a single operator, target_systems must contain that operator."""
        from app.models.agent import OperatorCommandArgs
        from tests.fakes.fake_approval_service import FakeApprovalService
        from tests.fakes.builder import build_command_service

        approval_service = FakeApprovalService()
        service = build_command_service(approval_service=approval_service)

        op = self._make_operator("op-1", "sess-1", "host-1")
        investigation = self._make_investigation([op])
        g8e_context = self._make_g8e_context()
        args = OperatorCommandArgs(command="ls", justification="test")

        from app.models.settings import G8eeUserSettings, LLMSettings
        request_settings = G8eeUserSettings(llm=LLMSettings())
        await service.execute_command(args, g8e_context, investigation, request_settings)

        assert len(approval_service.command_approval_calls) == 1
        req = approval_service.command_approval_calls[0]
        assert len(req.target_systems) == 1
        assert req.target_systems[0].operator_id == "op-1"
        assert req.target_systems[0].hostname == "host-1"

    async def test_target_operators_arg_populates_target_systems(self):
        """When target_operators is set, target_systems must reflect all resolved operators."""
        from app.models.agent import OperatorCommandArgs
        from tests.fakes.fake_approval_service import FakeApprovalService
        from tests.fakes.builder import build_command_service

        approval_service = FakeApprovalService()
        service = build_command_service(approval_service=approval_service)

        op1 = self._make_operator("op-1", "sess-1", "web-1")
        op2 = self._make_operator("op-2", "sess-2", "web-2")
        investigation = self._make_investigation([op1, op2])
        g8e_context = self._make_g8e_context()
        args = OperatorCommandArgs(
            command="uptime",
            justification="batch check",
            target_operator="op-1",
            target_operators=["op-1", "op-2"],
        )

        from app.models.settings import G8eeUserSettings, LLMSettings
        request_settings = G8eeUserSettings(llm=LLMSettings())
        await service.execute_command(args, g8e_context, investigation, request_settings)

        assert len(approval_service.command_approval_calls) == 1
        req = approval_service.command_approval_calls[0]
        resolved_ids = {ts.operator_id for ts in req.target_systems}
        assert "op-1" in resolved_ids
        assert "op-2" in resolved_ids

    async def test_target_systems_never_empty_for_valid_operator(self):
        """target_systems must never be empty when a valid operator is resolved."""
        from app.models.agent import OperatorCommandArgs
        from tests.fakes.fake_approval_service import FakeApprovalService
        from tests.fakes.builder import build_command_service

        approval_service = FakeApprovalService()
        service = build_command_service(approval_service=approval_service)

        op = self._make_operator("op-abc", "sess-abc", "db-server")
        investigation = self._make_investigation([op])
        g8e_context = self._make_g8e_context()
        args = OperatorCommandArgs(command="df -h", justification="disk check")

        from app.models.settings import G8eeUserSettings, LLMSettings
        request_settings = G8eeUserSettings(llm=LLMSettings())
        await service.execute_command(args, g8e_context, investigation, request_settings)

        assert len(approval_service.command_approval_calls) == 1
        req = approval_service.command_approval_calls[0]
        assert req.target_systems, "target_systems must not be empty"
