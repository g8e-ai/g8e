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

"""Typed fake for ApprovalServiceProtocol."""

from app.models.internal_api import OperatorApprovalResponse
from app.models.operators import (
    AgentContinueApprovalRequest,
    ApprovalResult,
    CommandApprovalRequest,
    FileEditApprovalRequest,
    IntentApprovalRequest,
    PendingApproval,
)
from app.services.protocols import ApprovalServiceProtocol


class FakeApprovalService:
    """Typed fake implementing ApprovalServiceProtocol.

    Returns approved=True by default. Approved/denied state is configurable.
    Records all calls for assertion in tests.
    """

    def __init__(self, *, approved: bool = True, approval_id: str = "fake-approval-id") -> None:
        self._approved = approved
        self._approval_id = approval_id
        self.command_approval_calls: list[CommandApprovalRequest] = []
        self.file_edit_approval_calls: list[FileEditApprovalRequest] = []
        self.intent_approval_calls: list[IntentApprovalRequest] = []
        self.agent_continue_approval_calls: list[AgentContinueApprovalRequest] = []
        self.approval_responses: list[dict] = []
        self._pending_approvals: dict[str, PendingApproval] = {}
        self._on_approval_requested = None

    @property
    def operator_data_service(self):
        return None

    @property
    def investigation_data_service(self):
        return None

    def set_on_approval_requested(self, callback) -> None:
        self._on_approval_requested = callback

    async def request_stream_approval(self, request) -> ApprovalResult:
        return ApprovalResult(approved=self._approved, approval_id=self._approval_id)

    async def request_command_approval(self, request: CommandApprovalRequest) -> ApprovalResult:
        self.command_approval_calls.append(request)
        return ApprovalResult(approved=self._approved, approval_id=self._approval_id)

    async def request_file_edit_approval(self, request: FileEditApprovalRequest) -> ApprovalResult:
        self.file_edit_approval_calls.append(request)
        return ApprovalResult(approved=self._approved, approval_id=self._approval_id)

    async def request_intent_approval(self, request: IntentApprovalRequest) -> ApprovalResult:
        self.intent_approval_calls.append(request)
        return ApprovalResult(approved=self._approved, approval_id=self._approval_id)

    async def request_agent_continue_approval(self, request: AgentContinueApprovalRequest) -> ApprovalResult:
        self.agent_continue_approval_calls.append(request)
        return ApprovalResult(approved=self._approved, approval_id=self._approval_id)

    async def handle_approval_response(self, response: OperatorApprovalResponse) -> None:
        self.approval_responses.append({
            "approval_id": response.approval_id,
            "approved": response.approved,
            "reason": response.reason,
            "operator_session_id": response.operator_session_id,
            "operator_id": response.operator_id,
        })

    def get_pending_approvals(self) -> dict[str, PendingApproval]:
        return self._pending_approvals

    def mark_pending_approvals_as_feedback(self, investigation_id: str, user_message: str, user_id: str) -> int:
        return 0


_: ApprovalServiceProtocol = FakeApprovalService()
