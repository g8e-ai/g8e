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
Contract tests for the constants/shared-JSON audit fixes.

Verifies:
- CACHE_PREFIX is not redefined locally in cache_aside.py
- Broadcast event models are typed Pydantic models, not raw dicts
- investigations.py model uses EventType enum, not raw strings
"""

import json

import pytest

from app.constants import (
    EventType,
    ExecutionStatus,
    InvestigationStatus,
)
from app.models.operators import (
    BatchCommandBroadcastEvent,
    CommandExecutingBroadcastEvent,
    CommandFailedBroadcastEvent,
)
from tests.fakes.factories import create_investigation_data

pytestmark = [pytest.mark.unit]

_SHARED_ROOT = "/app/shared/constants"


def _load_events():
    with open(_SHARED_ROOT + "/events.json") as f:
        return json.load(f)


def _load_kv_keys():
    with open(_SHARED_ROOT + "/kv_keys.json") as f:
        return json.load(f)


# =============================================================================
# CACHE_PREFIX — not redefined locally in cache_aside
# =============================================================================


class TestCacheVersionNotRedefinedLocally:
    def test_cache_aside_does_not_redefine_cache_prefix(self):
        import inspect

        import app.services.cache.cache_aside as cache_aside_module

        source = inspect.getsource(cache_aside_module)
        assert 'CACHE_PREFIX = "g8e"' not in source, (
            "cache_aside.py must not define CACHE_PREFIX locally — import it from constants"
        )

    def test_cache_aside_key_matches_constants_cache_prefix(self):
        from tests.fakes.builder import create_mock_cache_aside_service

        svc = create_mock_cache_aside_service()
        key = svc._make_key("operators", "op-abc")
        kv_keys = _load_kv_keys()
        expected_prefix = kv_keys["cache.prefix"]
        assert key.startswith(expected_prefix + ":"), (
            f"Key must start with shared JSON cache version '{expected_prefix}'"
        )


# =============================================================================
# InvestigationModel.update_status — uses enum, not raw string
# =============================================================================


class TestInvestigationModelUpdateStatusUsesEnum:
    def _make_investigation(self):
        return create_investigation_data(
            case_id="case-123",
            user_id="user-456",
            sentinel_mode=False,
        )

    def test_update_status_records_status_changed_event(self):
        inv = self._make_investigation()
        inv.update_status(InvestigationStatus.CLOSED, actor="g8ee", summary="Closing investigation")

        assert len(inv.history_trail) == 1
        entry = inv.history_trail[0]
        assert entry.event_type == EventType.INVESTIGATION_STATUS_UPDATED_CLOSED

    def test_update_status_event_type_is_enum_value(self):
        inv = self._make_investigation()
        inv.update_status(InvestigationStatus.CLOSED, actor="g8ee", summary="Done")

        entry = inv.history_trail[0]
        assert entry.event_type == "g8e.v1.app.investigation.status.updated.closed"

    def test_update_status_actor_is_recorded(self):
        from app.constants import ComponentName
        inv = self._make_investigation()
        inv.update_status(InvestigationStatus.CLOSED, actor=ComponentName.G8EE, summary="Done")

        assert inv.history_trail[0].actor == ComponentName.G8EE


# =============================================================================
# InvestigationService — add_history_entry event types are enums
# =============================================================================


class TestInvestigationServiceHistoryEventTypes:
    def _make_service(self):
        from app.services.investigation.investigation_data_service import InvestigationDataService
        from tests.fakes.builder import create_mock_cache_aside_service

        cache_aside_service = create_mock_cache_aside_service()
        return InvestigationDataService(cache=cache_aside_service)

    async def test_create_investigation_uses_investigation_created_enum(self):
        from unittest.mock import AsyncMock, patch

        from app.constants import Priority
        from app.models.investigations import InvestigationCreateRequest, InvestigationModel

        service = self._make_service()

        captured = []

        original_add = InvestigationModel.add_history_entry

        def capturing_add(self, event_type, **kwargs):
            captured.append(event_type)
            return original_add(self, event_type=event_type, **kwargs)

        request = InvestigationCreateRequest(
            case_id="case-001",
            case_title="Test Case",
            case_description="Test case description",
            user_id="user-001",
            priority=Priority.MEDIUM,
            sentinel_mode=False,
        )

        with patch.object(InvestigationModel, "add_history_entry", capturing_add):
            from app.models.cache import CacheOperationResult
            service.cache.create_document = AsyncMock(return_value=CacheOperationResult(success=True, document_id="inv-001"))

            await service.create_investigation(request)

        assert len(captured) >= 1
        assert captured[0] == EventType.INVESTIGATION_CREATED
        assert captured[0] != "investigation_created", (
            "Raw string 'investigation_created' must not be used — use EventType"
        )


# =============================================================================
# Broadcast event models — typed Pydantic models, not raw dicts
# =============================================================================


class TestBroadcastEventModelsAreTyped:
    def test_command_failed_broadcast_event_is_pydantic_model(self):
        from app.models.base import G8eBaseModel

        assert issubclass(CommandFailedBroadcastEvent, G8eBaseModel)

    def test_command_executing_broadcast_event_is_pydantic_model(self):
        from app.models.base import G8eBaseModel

        assert issubclass(CommandExecutingBroadcastEvent, G8eBaseModel)

    def test_batch_command_broadcast_event_is_pydantic_model(self):
        from app.models.base import G8eBaseModel

        assert issubclass(BatchCommandBroadcastEvent, G8eBaseModel)

    def test_command_failed_broadcast_event_construction(self):
        from app.constants import CommandErrorType

        event = CommandFailedBroadcastEvent(
            command="ls -la",
            execution_id="exec-123",
            case_id="case-abc",
            web_session_id="web-123",
            error="No operator available",
            error_type=CommandErrorType.NO_OPERATORS_AVAILABLE,
        )
        assert event.command == "ls -la"
        assert event.execution_id == "exec-123"
        assert event.error_type == CommandErrorType.NO_OPERATORS_AVAILABLE
        assert event.status == "failed"

    def test_command_executing_broadcast_event_construction(self):
        event = CommandExecutingBroadcastEvent(
            command="df -h",
            execution_id="exec-456",
            web_session_id="web-456",
            message="Command sent to operator, awaiting execution...",
            approval_id="appr-789",
        )
        assert event.command == "df -h"
        assert event.message == "Command sent to operator, awaiting execution..."
        assert event.approval_id == "appr-789"
        assert event.status == "executing"

    def test_batch_command_broadcast_event_construction(self):
        event = BatchCommandBroadcastEvent(
            command="uname -a",
            execution_id="exec-batch-1",
            status=ExecutionStatus.COMPLETED,
            output="Linux host 6.1.0",
            operators_used=3,
            successful_count=3,
            failed_count=0,
        )
        assert event.batch_execution is True
        assert event.operators_used == 3
        assert event.successful_count == 3

    def test_command_failed_broadcast_event_with_denial_status(self):
        event = CommandFailedBroadcastEvent(
            command="rm -rf /",
            execution_id="exec-deny",
            status="denied",
            error="User denied",
            denial_reason="Too dangerous",
            approval_id="appr-deny-123",
        )
        assert event.status == "denied"
        assert event.denial_reason == "Too dangerous"
        assert event.approval_id == "appr-deny-123"

    def test_command_failed_broadcast_event_with_feedback_status(self):
        event = CommandFailedBroadcastEvent(
            command="docker restart app",
            execution_id="exec-fb",
            status="feedback",
            error="User provided feedback",
            feedback_reason="Can you explain why?",
            approval_id="appr-fb-456",
        )
        assert event.status == "feedback"
        assert event.feedback_reason == "Can you explain why?"


# =============================================================================
# Broadcast event models use typed enum fields, not raw strings
# =============================================================================


class TestBroadcastEventTypedFields:
    """Verify broadcast event models use enum types, not raw strings."""

    def test_command_failed_error_type_is_command_error_type(self):
        from app.models.operators import CommandFailedBroadcastEvent

        hints = CommandFailedBroadcastEvent.model_fields
        field = hints["error_type"]
        annotation = str(field.annotation)
        assert "CommandErrorType" in annotation, (
            f"CommandFailedBroadcastEvent.error_type should be Optional[CommandErrorType], got: {annotation}"
        )

    def test_command_failed_error_type_accepts_enum_value(self):
        from app.constants import CommandErrorType
        from app.models.operators import CommandFailedBroadcastEvent

        event = CommandFailedBroadcastEvent(
            command="ls",
            error_type=CommandErrorType.VALIDATION_ERROR,
        )
        assert event.error_type == CommandErrorType.VALIDATION_ERROR

    def test_batch_command_status_is_execution_status(self):
        from app.models.operators import BatchCommandBroadcastEvent

        hints = BatchCommandBroadcastEvent.model_fields
        field = hints["status"]
        annotation = str(field.annotation)
        assert "ExecutionStatus" in annotation, (
            f"BatchCommandBroadcastEvent.status should be ExecutionStatus, got: {annotation}"
        )

    def test_batch_command_status_accepts_enum_value(self):
        from app.constants import ExecutionStatus
        from app.models.operators import BatchCommandBroadcastEvent

        event = BatchCommandBroadcastEvent(
            command="ls",
            execution_id="exec-1",
            status=ExecutionStatus.COMPLETED,
        )
        assert event.status == ExecutionStatus.COMPLETED


# =============================================================================
# CancelCommandResult and DirectCommandResult are typed models
# =============================================================================


class TestTypedCommandOperationResults:
    """Verify cancel_command and send_command_to_operator return typed models."""

    def test_cancel_command_result_is_g8e_base_model(self):
        from app.models.base import G8eBaseModel
        from app.models.operators import CancelCommandResult

        assert issubclass(CancelCommandResult, G8eBaseModel)

    def test_cancel_command_result_exported_from_models_init(self):
        from app.models import CancelCommandResult
        assert CancelCommandResult is not None

    def test_direct_command_result_is_g8e_base_model(self):
        from app.models.base import G8eBaseModel
        from app.models.operators import DirectCommandResult

        assert issubclass(DirectCommandResult, G8eBaseModel)

    def test_direct_command_result_exported_from_models_init(self):
        from app.models import DirectCommandResult
        assert DirectCommandResult is not None

    def test_cancel_command_result_fields(self):
        from app.models.operators import CancelCommandResult

        result = CancelCommandResult(execution_id="exec-1", status="cancel_requested", message="sent")
        assert result.execution_id == "exec-1"
        assert result.status == "cancel_requested"
        assert result.message == "sent"
        assert result.error is None

    def test_direct_command_result_fields(self):
        from app.models.operators import DirectCommandResult

        result = DirectCommandResult(execution_id="exec-1", status="executing", message="Command sent to operator")
        assert result.execution_id == "exec-1"
        assert result.status == "executing"
        assert result.error is None

    def test_cancel_command_returns_typed_model(self):
        """cancel_command return type annotation is CancelCommandResult."""
        from app.models.operators import CancelCommandResult
        from app.services.operator.execution_service import OperatorExecutionService

        hints = OperatorExecutionService.cancel_command.__annotations__
        assert hints.get("return") is CancelCommandResult, (
            f"cancel_command should return CancelCommandResult, got: {hints.get('return')}"
        )

    def test_send_command_to_operator_returns_typed_model(self):
        """send_command_to_operator return type annotation is DirectCommandResult."""
        from app.models.operators import DirectCommandResult
        from app.services.operator.execution_service import OperatorExecutionService

        hints = OperatorExecutionService.send_command_to_operator.__annotations__
        assert hints.get("return") is DirectCommandResult, (
            f"send_command_to_operator should return DirectCommandResult, got: {hints.get('return')}"
        )


# =============================================================================
# cloud_command_validator is a standalone module, not inlined in command_executor
# =============================================================================


class TestCloudCommandValidatorModule:
    """Verify cloud command helpers live in their own module."""

    def test_cloud_command_validator_module_exists(self):
        from app.services.operator import cloud_command_validator
        assert cloud_command_validator is not None

    def test_is_cloud_only_command_importable(self):
        from app.services.operator.cloud_command_validator import is_cloud_only_command
        assert callable(is_cloud_only_command)

    def test_is_cloud_operator_self_discovery_command_importable(self):
        from app.services.operator.cloud_command_validator import (
            is_cloud_operator_self_discovery_command,
        )
        assert callable(is_cloud_operator_self_discovery_command)

    def test_execution_service_does_not_define_patterns(self):
        """Verify CLOUD_ONLY_COMMAND_PATTERNS lives in cloud_command_validator, not execution_service."""
        import app.services.operator.execution_service as mod
        assert not hasattr(mod, "CLOUD_ONLY_COMMAND_PATTERNS"), (
            "CLOUD_ONLY_COMMAND_PATTERNS should be in cloud_command_validator, not execution_service"
        )
        assert not hasattr(mod, "CLOUD_OPERATOR_AUTO_APPROVED_PATTERNS"), (
            "CLOUD_OPERATOR_AUTO_APPROVED_PATTERNS should be in cloud_command_validator, not execution_service"
        )

    def test_execution_service_does_not_define_helpers(self):
        """Verify helper functions live in cloud_command_validator, not execution_service module scope."""
        import app.services.operator.execution_service as mod
        assert not hasattr(mod, "is_cloud_only_command"), (
            "is_cloud_only_command should be in cloud_command_validator, not execution_service module scope"
        )
        assert not hasattr(mod, "is_cloud_operator_self_discovery_command"), (
            "is_cloud_operator_self_discovery_command should be in cloud_command_validator, not execution_service module scope"
        )
