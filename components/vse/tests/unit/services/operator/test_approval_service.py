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

"""Unit tests for OperatorApprovalService."""

from unittest.mock import AsyncMock, MagicMock, patch

import pytest

from app.constants.events import EventType
from app.constants.intents import CloudIntent
from app.constants.status import FileOperation
from app.models.internal_api import OperatorApprovalResponse
from app.models.investigations import ApprovalMetadata, FileEditMetadata
from app.models.operators import (
    ApprovalResult,
    ApprovalType,
    CommandApprovalEvent,
    CommandApprovalRequest,
    CommandRiskAnalysis,
    FileEditApprovalEvent,
    FileEditApprovalRequest,
    FileOperationRiskAnalysis,
    IntentApprovalEvent,
    IntentApprovalRequest,
    PendingApproval,
    TargetSystem,
)
from app.models.events import SessionEvent
from app.models.http_context import VSOHttpContext
from app.models.tool_results import RiskLevel
from app.services.operator.approval_service import OperatorApprovalService
from app.services.protocols import (
    EventServiceProtocol,
    InvestigationDataServiceProtocol,
    OperatorDataServiceProtocol,
)
from app.utils.ids import generate_approval_id, generate_intent_approval_id


class TestOperatorApprovalServiceInit:
    """Test OperatorApprovalService initialization."""

    def test_constructor_with_dependencies(self):
        """Constructor accepts all required protocol dependencies."""
        vsod_event_service = MagicMock(spec=EventServiceProtocol)
        operator_data_service = MagicMock(spec=OperatorDataServiceProtocol)
        investigation_data_service = MagicMock(spec=InvestigationDataServiceProtocol)

        service = OperatorApprovalService(
            vsod_event_service=vsod_event_service,
            operator_data_service=operator_data_service,
            investigation_data_service=investigation_data_service,
        )

        assert service.vsod_event_service is vsod_event_service
        assert service.operator_data_service is operator_data_service
        assert service.investigation_data_service is investigation_data_service
        assert service.get_pending_approvals() == {}

    def test_constructor_with_callback(self):
        """Constructor accepts optional on_approval_requested callback."""
        vsod_event_service = MagicMock(spec=EventServiceProtocol)
        operator_data_service = MagicMock(spec=OperatorDataServiceProtocol)
        investigation_data_service = MagicMock(spec=InvestigationDataServiceProtocol)
        callback = MagicMock()

        service = OperatorApprovalService(
            vsod_event_service=vsod_event_service,
            operator_data_service=operator_data_service,
            investigation_data_service=investigation_data_service,
            on_approval_requested=callback,
        )

        assert service._on_approval_requested is callback

    def test_set_on_approval_requested(self):
        """set_on_approval_requested updates the callback."""
        vsod_event_service = MagicMock(spec=EventServiceProtocol)
        operator_data_service = MagicMock(spec=OperatorDataServiceProtocol)
        investigation_data_service = MagicMock(spec=InvestigationDataServiceProtocol)
        service = OperatorApprovalService(
            vsod_event_service=vsod_event_service,
            operator_data_service=operator_data_service,
            investigation_data_service=investigation_data_service,
        )

        callback = MagicMock()
        service.set_on_approval_requested(callback)

        assert service._on_approval_requested is callback


class TestHandleApprovalResponse:
    """Test handle_approval_response method."""

    async def test_handle_approval_response_success(self):
        """Successfully processes an approval response."""
        vsod_event_service = MagicMock(spec=EventServiceProtocol)
        operator_data_service = MagicMock(spec=OperatorDataServiceProtocol)
        investigation_data_service = MagicMock(spec=InvestigationDataServiceProtocol)
        service = OperatorApprovalService(
            vsod_event_service=vsod_event_service,
            operator_data_service=operator_data_service,
            investigation_data_service=investigation_data_service,
        )

        approval_id = generate_approval_id()
        pending = PendingApproval(
            approval_id=approval_id,
            approval_type=ApprovalType.INTENT,
            intent_name=CloudIntent.EC2_MANAGEMENT,
            requested_at="2022-01-01 12:00:00",
            case_id="case-1",
            investigation_id="inv-1",
            user_id="user-1",
            operator_id="op-1",
            operator_session_id="session-1",
        )
        service._pending_approvals[approval_id] = pending

        response = OperatorApprovalResponse(
            approval_id=approval_id,
            approved=True,
            reason="Approved",
            operator_session_id="session-1",
            operator_id="op-1",
        )

        await service.handle_approval_response(response)

        assert pending.approved is True
        assert pending.reason == "Approved"
        assert pending.operator_session_id == "session-1"
        assert pending.operator_id == "op-1"

    async def test_handle_approval_response_unknown_approval_id(self):
        """Logs warning for unknown approval_id and returns early."""
        vsod_event_service = MagicMock(spec=EventServiceProtocol)
        operator_data_service = MagicMock(spec=OperatorDataServiceProtocol)
        investigation_data_service = MagicMock(spec=InvestigationDataServiceProtocol)
        service = OperatorApprovalService(
            vsod_event_service=vsod_event_service,
            operator_data_service=operator_data_service,
            investigation_data_service=investigation_data_service,
        )

        response = OperatorApprovalResponse(
            approval_id="unknown-id",
            approved=True,
            reason="Approved",
        )

        await service.handle_approval_response(response)

        assert service.get_pending_approvals() == {}

    async def test_handle_approval_response_missing_approval_id(self):
        """Raises ValidationError when approval_id is empty string."""
        vsod_event_service = MagicMock(spec=EventServiceProtocol)
        operator_data_service = MagicMock(spec=OperatorDataServiceProtocol)
        investigation_data_service = MagicMock(spec=InvestigationDataServiceProtocol)
        service = OperatorApprovalService(
            vsod_event_service=vsod_event_service,
            operator_data_service=operator_data_service,
            investigation_data_service=investigation_data_service,
        )

        response = OperatorApprovalResponse(
            approval_id="",
            approved=True,
            reason="Approved",
        )

        with pytest.raises(Exception) as exc_info:
            await service.handle_approval_response(response)

        assert "approval_id must be provided" in str(exc_info.value)


class TestMarkPendingApprovalsAsFeedback:
    """Test mark_pending_approvals_as_feedback method."""

    def test_mark_pending_approvals_as_feedback_filters_by_investigation(self):
        """Marks approvals as feedback only for matching investigation."""
        vsod_event_service = MagicMock(spec=EventServiceProtocol)
        operator_data_service = MagicMock(spec=OperatorDataServiceProtocol)
        investigation_data_service = MagicMock(spec=InvestigationDataServiceProtocol)
        service = OperatorApprovalService(
            vsod_event_service=vsod_event_service,
            operator_data_service=operator_data_service,
            investigation_data_service=investigation_data_service,
        )

        pending1 = PendingApproval(
            approval_id="app-1",
            approval_type=ApprovalType.COMMAND,
            command="cmd1",
            requested_at=MagicMock(),
            case_id="case-1",
            investigation_id="inv-1",
            user_id="user-1",
            operator_id="op-1",
            operator_session_id="session-1",
        )
        pending2 = PendingApproval(
            approval_id="app-2",
            approval_type=ApprovalType.COMMAND,
            command="cmd2",
            requested_at=MagicMock(),
            case_id="case-1",
            investigation_id="inv-2",
            user_id="user-1",
            operator_id="op-1",
            operator_session_id="session-1",
        )
        service._pending_approvals["app-1"] = pending1
        service._pending_approvals["app-2"] = pending2

        count = service.mark_pending_approvals_as_feedback(
            investigation_id="inv-1",
            user_message="additional context",
            user_id="user-1",
        )

        assert count == 1
        assert pending1.feedback is True
        assert pending2.feedback is False

    def test_mark_pending_approvals_as_feedback_filters_by_user_id(self):
        """Marks approvals as feedback only for matching user."""
        vsod_event_service = MagicMock(spec=EventServiceProtocol)
        operator_data_service = MagicMock(spec=OperatorDataServiceProtocol)
        investigation_data_service = MagicMock(spec=InvestigationDataServiceProtocol)
        service = OperatorApprovalService(
            vsod_event_service=vsod_event_service,
            operator_data_service=operator_data_service,
            investigation_data_service=investigation_data_service,
        )

        pending1 = PendingApproval(
            approval_id="app-1",
            approval_type=ApprovalType.COMMAND,
            command="cmd1",
            requested_at=MagicMock(),
            case_id="case-1",
            investigation_id="inv-1",
            user_id="user-1",
            operator_id="op-1",
            operator_session_id="session-1",
        )
        pending2 = PendingApproval(
            approval_id="app-2",
            approval_type=ApprovalType.COMMAND,
            command="cmd2",
            requested_at=MagicMock(),
            case_id="case-1",
            investigation_id="inv-1",
            user_id="user-2",
            operator_id="op-1",
            operator_session_id="session-1",
        )
        service._pending_approvals["app-1"] = pending1
        service._pending_approvals["app-2"] = pending2

        count = service.mark_pending_approvals_as_feedback(
            investigation_id="inv-1",
            user_message="additional context",
            user_id="user-1",
        )

        assert count == 1
        assert pending1.feedback is True
        assert pending2.feedback is False

    def test_mark_pending_approvals_as_feedback_skips_responded(self):
        """Skips approvals that already have a response."""
        vsod_event_service = MagicMock(spec=EventServiceProtocol)
        operator_data_service = MagicMock(spec=OperatorDataServiceProtocol)
        investigation_data_service = MagicMock(spec=InvestigationDataServiceProtocol)
        service = OperatorApprovalService(
            vsod_event_service=vsod_event_service,
            operator_data_service=operator_data_service,
            investigation_data_service=investigation_data_service,
        )

        pending = PendingApproval(
            approval_id="app-1",
            approval_type=ApprovalType.COMMAND,
            command="cmd1",
            requested_at=MagicMock(),
            case_id="case-1",
            investigation_id="inv-1",
            user_id="user-1",
            operator_id="op-1",
            operator_session_id="session-1",
        )
        pending.resolve(approved=True, reason="Done", responded_at=MagicMock())
        service._pending_approvals["app-1"] = pending

        count = service.mark_pending_approvals_as_feedback(
            investigation_id="inv-1",
            user_message="additional context",
            user_id="user-1",
        )

        assert count == 0

    def test_mark_pending_approvals_as_feedback_no_matching(self):
        """Returns zero when no matching approvals found."""
        vsod_event_service = MagicMock(spec=EventServiceProtocol)
        operator_data_service = MagicMock(spec=OperatorDataServiceProtocol)
        investigation_data_service = MagicMock(spec=InvestigationDataServiceProtocol)
        service = OperatorApprovalService(
            vsod_event_service=vsod_event_service,
            operator_data_service=operator_data_service,
            investigation_data_service=investigation_data_service,
        )

        count = service.mark_pending_approvals_as_feedback(
            investigation_id="inv-1",
            user_message="additional context",
            user_id="user-1",
        )

        assert count == 0


class TestRequestCommandApproval:
    """Test request_command_approval method."""

    @patch("app.services.operator.approval_service.generate_approval_id")
    @patch("app.services.operator.approval_service.PendingApproval")
    async def test_request_command_approval_granted(self, mock_pending_approval_class, mock_generate_approval_id):
        """Successfully requests and receives command approval granted."""
        vsod_event_service = AsyncMock(spec=EventServiceProtocol)
        operator_data_service = AsyncMock(spec=OperatorDataServiceProtocol)
        investigation_data_service = AsyncMock(spec=InvestigationDataServiceProtocol)
        callback = MagicMock()
        service = OperatorApprovalService(
            vsod_event_service=vsod_event_service,
            operator_data_service=operator_data_service,
            investigation_data_service=investigation_data_service,
            on_approval_requested=callback,
        )

        approval_id = "test-approval-id"
        mock_generate_approval_id.return_value = approval_id

        vso_context = VSOHttpContext(
            case_id="case-1",
            investigation_id="inv-1",
            web_session_id="session-1",
            user_id="user-1",
            source_component="vse",
        )
        request = CommandApprovalRequest(
            vso_context=vso_context,
            timeout_seconds=30,
            justification="test justification",
            execution_id="exec-1",
            operator_session_id="session-1",
            operator_id="op-1",
            command="ls -la",
            risk_analysis=None,
            target_systems=[],
            task_id="task-1",
        )

        # Mock PendingApproval to return a pre-resolved approval with wait() mocked
        mock_pending = MagicMock()
        mock_pending.wait = AsyncMock()
        mock_pending.resolve = MagicMock()
        mock_pending.approved = True
        mock_pending.reason = "Approved"
        mock_pending.operator_id = "op-1"
        mock_pending.operator_session_id = "session-1"
        mock_pending.responded_at = MagicMock()
        mock_pending.feedback = False
        mock_pending_approval_class.return_value = mock_pending

        result = await service.request_command_approval(request)

        assert result.approved is True
        assert result.approval_id == approval_id
        assert vsod_event_service.publish.called
        assert callback.called

    @patch("app.services.operator.approval_service.PendingApproval")
    async def test_request_command_approval_rejected(self, mock_pending_approval_class):
        """Successfully requests and receives command approval rejected."""
        vsod_event_service = AsyncMock(spec=EventServiceProtocol)
        operator_data_service = AsyncMock(spec=OperatorDataServiceProtocol)
        investigation_data_service = AsyncMock(spec=InvestigationDataServiceProtocol)
        service = OperatorApprovalService(
            vsod_event_service=vsod_event_service,
            operator_data_service=operator_data_service,
            investigation_data_service=investigation_data_service,
        )

        approval_id = generate_approval_id()
        vso_context = VSOHttpContext(
            case_id="case-1",
            investigation_id="inv-1",
            web_session_id="session-1",
            user_id="user-1",
            source_component="vse",
        )
        request = CommandApprovalRequest(
            vso_context=vso_context,
            timeout_seconds=30,
            justification="test justification",
            execution_id="exec-1",
            operator_session_id="session-1",
            operator_id="op-1",
            command="ls -la",
            risk_analysis=None,
            target_systems=[],
            task_id="task-1",
        )

        # Mock PendingApproval to return a pre-resolved approval with wait() mocked
        mock_pending = MagicMock()
        mock_pending.wait = AsyncMock()
        mock_pending.resolve = MagicMock()
        mock_pending.approved = False
        mock_pending.reason = "Denied"
        mock_pending.operator_id = "op-1"
        mock_pending.operator_session_id = "session-1"
        mock_pending.responded_at = MagicMock()
        mock_pending.feedback = False
        mock_pending_approval_class.return_value = mock_pending

        result = await service.request_command_approval(request)

        assert result.approved is False
        assert result.reason == "Denied"

    @patch("app.services.operator.approval_service.PendingApproval")
    async def test_request_command_approval_with_risk_analysis(self, mock_pending_approval_class):
        """Command approval with risk analysis logs risk level."""
        vsod_event_service = AsyncMock(spec=EventServiceProtocol)
        operator_data_service = AsyncMock(spec=OperatorDataServiceProtocol)
        investigation_data_service = AsyncMock(spec=InvestigationDataServiceProtocol)
        service = OperatorApprovalService(
            vsod_event_service=vsod_event_service,
            operator_data_service=operator_data_service,
            investigation_data_service=investigation_data_service,
        )

        approval_id = generate_approval_id()
        vso_context = VSOHttpContext(
            case_id="case-1",
            investigation_id="inv-1",
            web_session_id="session-1",
            user_id="user-1",
            source_component="vse",
        )
        risk_analysis = CommandRiskAnalysis(risk_level=RiskLevel.HIGH)
        request = CommandApprovalRequest(
            vso_context=vso_context,
            timeout_seconds=30,
            justification="test justification",
            execution_id="exec-1",
            operator_session_id="session-1",
            operator_id="op-1",
            command="rm -rf /",
            risk_analysis=risk_analysis,
            target_systems=[],
            task_id="task-1",
        )

        # Mock PendingApproval to return a pre-resolved approval with wait() mocked
        mock_pending = MagicMock()
        mock_pending.wait = AsyncMock()
        mock_pending.resolve = MagicMock()
        mock_pending.approved = True
        mock_pending.reason = "Approved"
        mock_pending.operator_id = "op-1"
        mock_pending.operator_session_id = "session-1"
        mock_pending.responded_at = MagicMock()
        mock_pending.feedback = False
        mock_pending_approval_class.return_value = mock_pending

        result = await service.request_command_approval(request)

        assert result.approved is True


class TestRequestFileEditApproval:
    """Test request_file_edit_approval method."""

    @patch("app.services.operator.approval_service.generate_approval_id")
    @patch("app.services.operator.approval_service.PendingApproval")
    async def test_request_file_edit_approval_granted(self, mock_pending_approval_class, mock_generate_approval_id):
        """Successfully requests and receives file edit approval granted."""
        vsod_event_service = AsyncMock(spec=EventServiceProtocol)
        operator_data_service = AsyncMock(spec=OperatorDataServiceProtocol)
        investigation_data_service = AsyncMock(spec=InvestigationDataServiceProtocol)
        service = OperatorApprovalService(
            vsod_event_service=vsod_event_service,
            operator_data_service=operator_data_service,
            investigation_data_service=investigation_data_service,
        )

        approval_id = "test-approval-id"
        mock_generate_approval_id.return_value = approval_id
        vso_context = VSOHttpContext(
            case_id="case-1",
            investigation_id="inv-1",
            web_session_id="session-1",
            user_id="user-1",
            source_component="vse",
        )
        request = FileEditApprovalRequest(
            vso_context=vso_context,
            timeout_seconds=30,
            justification="test justification",
            execution_id="exec-1",
            operator_session_id="session-1",
            operator_id="op-1",
            file_path="/etc/config.conf",
            operation=FileOperation.WRITE,
            risk_analysis=None,
        )

        # Mock PendingApproval to return a pre-resolved approval with wait() mocked
        mock_pending = MagicMock()
        mock_pending.wait = AsyncMock()
        mock_pending.resolve = MagicMock()
        mock_pending.approved = True
        mock_pending.reason = "Approved"
        mock_pending.operator_id = "op-1"
        mock_pending.operator_session_id = "session-1"
        mock_pending.responded_at = MagicMock()
        mock_pending.feedback = False
        mock_pending_approval_class.return_value = mock_pending

        result = await service.request_file_edit_approval(request)

        assert result.approved is True
        assert result.approval_id == approval_id


class TestRequestIntentApproval:
    """Test request_intent_approval method."""

    @patch("app.services.operator.approval_service.generate_intent_approval_id")
    @patch("app.services.operator.approval_service.PendingApproval")
    async def test_request_intent_approval_granted(self, mock_pending_approval_class, mock_generate_intent_approval_id):
        """Successfully requests and receives intent approval granted."""
        vsod_event_service = AsyncMock(spec=EventServiceProtocol)
        operator_data_service = AsyncMock(spec=OperatorDataServiceProtocol)
        investigation_data_service = AsyncMock(spec=InvestigationDataServiceProtocol)
        service = OperatorApprovalService(
            vsod_event_service=vsod_event_service,
            operator_data_service=operator_data_service,
            investigation_data_service=investigation_data_service,
        )

        approval_id = "test-intent-approval-id"
        mock_generate_intent_approval_id.return_value = approval_id
        vso_context = VSOHttpContext(
            case_id="case-1",
            investigation_id="inv-1",
            web_session_id="session-1",
            user_id="user-1",
            source_component="vse",
        )
        request = IntentApprovalRequest(
            vso_context=vso_context,
            timeout_seconds=30,
            justification="test justification",
            execution_id="exec-1",
            operator_session_id="session-1",
            operator_id="op-1",
            intent_name=CloudIntent.EC2_MANAGEMENT,
            all_intents=[CloudIntent.EC2_MANAGEMENT],
            operation_context="test context",
        )

        # Mock PendingApproval to return a pre-resolved approval with wait() mocked
        mock_pending = MagicMock()
        mock_pending.wait = AsyncMock()
        mock_pending.resolve = MagicMock()
        mock_pending.approved = True
        mock_pending.reason = "Approved"
        mock_pending.operator_id = "op-1"
        mock_pending.operator_session_id = "session-1"
        mock_pending.responded_at = MagicMock()
        mock_pending.feedback = False
        mock_pending_approval_class.return_value = mock_pending

        result = await service.request_intent_approval(request)

        assert result.approved is True
        assert result.approval_id == approval_id

    async def test_request_intent_approval_invalid_intent(self):
        """Returns error result for invalid intent name."""
        vsod_event_service = AsyncMock(spec=EventServiceProtocol)
        operator_data_service = AsyncMock(spec=OperatorDataServiceProtocol)
        investigation_data_service = AsyncMock(spec=InvestigationDataServiceProtocol)
        service = OperatorApprovalService(
            vsod_event_service=vsod_event_service,
            operator_data_service=operator_data_service,
            investigation_data_service=investigation_data_service,
        )

        vso_context = VSOHttpContext(
            case_id="case-1",
            investigation_id="inv-1",
            web_session_id="session-1",
            user_id="user-1",
            source_component="vse",
        )
        request = IntentApprovalRequest(
            vso_context=vso_context,
            timeout_seconds=30,
            justification="test justification",
            execution_id="exec-1",
            operator_session_id="session-1",
            operator_id="op-1",
            intent_name="invalid-intent",
            all_intents=[],
            operation_context="test context",
        )

        result = await service.request_intent_approval(request)

        assert result.approved is False
        assert result.error is True
        assert "Invalid intent" in result.reason


class TestRegisterPending:
    """Test _register_pending method."""

    def test_register_pending_with_callback(self):
        """Registers pending approval and calls callback if set."""
        vsod_event_service = MagicMock(spec=EventServiceProtocol)
        operator_data_service = MagicMock(spec=OperatorDataServiceProtocol)
        investigation_data_service = MagicMock(spec=InvestigationDataServiceProtocol)
        callback = MagicMock()
        service = OperatorApprovalService(
            vsod_event_service=vsod_event_service,
            operator_data_service=operator_data_service,
            investigation_data_service=investigation_data_service,
            on_approval_requested=callback,
        )

        pending = PendingApproval(
            approval_id="app-1",
            approval_type=ApprovalType.COMMAND,
            command="cmd1",
            requested_at=MagicMock(),
            case_id="case-1",
            investigation_id="inv-1",
            user_id="user-1",
            operator_id="op-1",
            operator_session_id="session-1",
        )

        service._register_pending("app-1", pending)

        assert service.get_pending_approvals()["app-1"] is pending
        callback.assert_called_once_with("app-1", pending)

    def test_register_pending_without_callback(self):
        """Registers pending approval without calling callback when not set."""
        vsod_event_service = MagicMock(spec=EventServiceProtocol)
        operator_data_service = MagicMock(spec=OperatorDataServiceProtocol)
        investigation_data_service = MagicMock(spec=InvestigationDataServiceProtocol)
        service = OperatorApprovalService(
            vsod_event_service=vsod_event_service,
            operator_data_service=operator_data_service,
            investigation_data_service=investigation_data_service,
        )

        pending = PendingApproval(
            approval_id="app-1",
            approval_type=ApprovalType.COMMAND,
            command="cmd1",
            requested_at=MagicMock(),
            case_id="case-1",
            investigation_id="inv-1",
            user_id="user-1",
            operator_id="op-1",
            operator_session_id="session-1",
        )

        service._register_pending("app-1", pending)

        assert service.get_pending_approvals()["app-1"] is pending

    def test_register_pending_callback_exception_logged(self):
        """Logs error when callback raises exception."""
        vsod_event_service = MagicMock(spec=EventServiceProtocol)
        operator_data_service = MagicMock(spec=OperatorDataServiceProtocol)
        investigation_data_service = MagicMock(spec=InvestigationDataServiceProtocol)
        callback = MagicMock(side_effect=RuntimeError("Callback failed"))
        service = OperatorApprovalService(
            vsod_event_service=vsod_event_service,
            operator_data_service=operator_data_service,
            investigation_data_service=investigation_data_service,
            on_approval_requested=callback,
        )

        pending = PendingApproval(
            approval_id="app-1",
            approval_type=ApprovalType.COMMAND,
            command="cmd1",
            requested_at=MagicMock(),
            case_id="case-1",
            investigation_id="inv-1",
            user_id="user-1",
            operator_id="op-1",
            operator_session_id="session-1",
        )

        service._register_pending("app-1", pending)

        assert service.get_pending_approvals()["app-1"] is pending


class TestAudit:
    """Test _audit method."""

    async def test_audit_with_operator_id(self):
        """Records audit to both operator activity_log and conversation_history."""
        vsod_event_service = MagicMock(spec=EventServiceProtocol)
        operator_data_service = AsyncMock(spec=OperatorDataServiceProtocol)
        investigation_data_service = AsyncMock(spec=InvestigationDataServiceProtocol)
        service = OperatorApprovalService(
            vsod_event_service=vsod_event_service,
            operator_data_service=operator_data_service,
            investigation_data_service=investigation_data_service,
        )

        vso_context = VSOHttpContext(
            case_id="case-1",
            investigation_id="inv-1",
            web_session_id="session-1",
            user_id="user-1",
            source_component="vse",
        )
        metadata = ApprovalMetadata(
            execution_id="exec-1",
            approval_id="app-1",
            command="ls -la",
            justification="test",
        )

        await service._audit(
            operator_id="op-1",
            event_type=EventType.OPERATOR_COMMAND_APPROVAL_REQUESTED,
            metadata=metadata,
            vso_context=vso_context,
            log_tag="APPROVAL",
        )

        operator_data_service.add_operator_approval.assert_called_once()
        investigation_data_service.add_approval_record.assert_called_once()

    async def test_audit_without_operator_id(self):
        """Records audit only to conversation_history when operator_id is None."""
        vsod_event_service = MagicMock(spec=EventServiceProtocol)
        operator_data_service = AsyncMock(spec=OperatorDataServiceProtocol)
        investigation_data_service = AsyncMock(spec=InvestigationDataServiceProtocol)
        service = OperatorApprovalService(
            vsod_event_service=vsod_event_service,
            operator_data_service=operator_data_service,
            investigation_data_service=investigation_data_service,
        )

        vso_context = VSOHttpContext(
            case_id="case-1",
            investigation_id="inv-1",
            web_session_id="session-1",
            user_id="user-1",
            source_component="vse",
        )
        metadata = ApprovalMetadata(
            execution_id="exec-1",
            approval_id="app-1",
            command="ls -la",
            justification="test",
        )

        await service._audit(
            operator_id=None,
            event_type=EventType.OPERATOR_COMMAND_APPROVAL_REQUESTED,
            metadata=metadata,
            vso_context=vso_context,
            log_tag="APPROVAL",
        )

        operator_data_service.add_operator_approval.assert_not_called()
        investigation_data_service.add_approval_record.assert_called_once()
