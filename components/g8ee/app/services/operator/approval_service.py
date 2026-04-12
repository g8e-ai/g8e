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

"""Operator Approval Service

Owns the complete approval lifecycle for all three approval types:
  - Command approval
  - File edit approval
  - Intent (IAM) approval

_pending_approvals is instance state — not a module-level global — making
this service injectable, testable, and horizontally scalable (with a
persistent backing store, pod-restart safe).
"""

import logging
from collections.abc import Callable


from app.services.protocols import (
    EventServiceProtocol,
    InvestigationDataServiceProtocol,
    OperatorDataServiceProtocol,
)
from app.constants.status import (
    FileOperation,
)
from app.constants.events import (
    EventType,
)
from app.constants.intents import (
    CLOUD_INTENT_QUESTIONS,
    CloudIntent,
)
from app.constants.settings import (
    ApprovalErrorType,
)
from app.models.internal_api import OperatorApprovalResponse
from app.models.investigations import ApprovalMetadata, FileEditMetadata, ConversationMessageMetadata
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
from app.models.http_context import G8eHttpContext
from app.utils.ids import generate_approval_id, generate_intent_approval_id
from app.utils.timestamp import now

logger = logging.getLogger(__name__)


class OperatorApprovalService:
    """Manages approval request/wait/resolve lifecycle for operator operations."""

    def __init__(
        self,
        g8ed_event_service: EventServiceProtocol,
        operator_data_service: OperatorDataServiceProtocol,
        investigation_data_service: InvestigationDataServiceProtocol,
        on_approval_requested: Callable[[str, PendingApproval], None] | None = None,
    ) -> None:
        self.g8ed_event_service = g8ed_event_service
        self.operator_data_service = operator_data_service
        self.investigation_data_service = investigation_data_service
        self._on_approval_requested = on_approval_requested
        self._pending_approvals: dict[str, PendingApproval] = {}

    def set_on_approval_requested(self, callback: Callable[[str, PendingApproval], None] | None) -> None:
        self._on_approval_requested = callback

    def get_pending_approvals(self) -> dict[str, PendingApproval]:
        return self._pending_approvals

    async def handle_approval_response(self, response: OperatorApprovalResponse) -> None:
        from app.errors import ExternalServiceError, ValidationError

        if not response.approval_id:
            raise ValidationError("approval_id must be provided", component="g8ee")

        if response.approval_id not in self._pending_approvals:
            logger.warning(
                "[APPROVAL-HTTP] Received response for unknown approval_id: %s",
                response.approval_id,
                extra={"approval_id": response.approval_id, "approved": response.approved},
            )
            return

        try:
            pending = self._pending_approvals[response.approval_id]
            pending.resolve(
                approved=response.approved,
                reason=response.reason,
                responded_at=now(),
                operator_session_id=response.operator_session_id,
                operator_id=response.operator_id,
            )
        except Exception as e:
            raise ExternalServiceError(
                "[APPROVAL-HTTP] Failed to process approval response for %s" % response.approval_id,
                service_name="approval_service",
                component="g8ee",
            ) from e

        logger.info(
            "[APPROVAL-HTTP] Approval response processed: approval_id=%s, approved=%s",
            response.approval_id,
            response.approved,
            extra={
                "approval_id": response.approval_id,
                "approved": response.approved,
                "reason": response.reason,
                "has_operator_session_id": bool(response.operator_session_id),
            },
        )

    def mark_pending_approvals_as_feedback(
        self,
        investigation_id: str,
        user_message: str,
        user_id: str,
    ) -> int:
        marked_count = 0

        logger.info(
            "[APPROVAL-CANCEL] Checking for pending approvals to mark as feedback",
            extra={
                "investigation_id": investigation_id,
                "user_id": user_id,
                "total_pending_approvals": len(self._pending_approvals),
                "message_preview": user_message[:50] + "..." if len(user_message) > 50 else user_message,
            },
        )

        for approval_id, pending in list(self._pending_approvals.items()):
            if pending.investigation_id != investigation_id:
                continue
            if user_id and pending.user_id != user_id:
                continue
            if pending.response_received:
                continue

            pending.resolve(
                approved=False,
                reason=f"User provided additional context: {user_message[:100]}...",
                responded_at=now(),
                feedback=True,
            )
            marked_count += 1

            logger.info(
                "[APPROVAL] Marked %s as feedback due to new user message",
                approval_id,
                extra={
                    "approval_id": approval_id,
                    "investigation_id": investigation_id,
                    "user_id": user_id,
                },
            )

        if marked_count > 0:
            logger.info(
                "[APPROVAL] Marked %d pending approval(s) as feedback for investigation %s",
                marked_count,
                investigation_id,
            )
        else:
            logger.info(
                "[APPROVAL-CANCEL] No pending approvals found for investigation %s",
                investigation_id,
            )

        return marked_count

    def _register_pending(self, approval_id: str, pending: PendingApproval) -> None:
        self._pending_approvals[approval_id] = pending
        if self._on_approval_requested is not None:
            try:
                self._on_approval_requested(approval_id, pending)
            except Exception as e:
                logger.error("[APPROVAL] on_approval_requested callback failed: %s", e)

    async def _audit(
        self,
        *,
        operator_id: str | None,
        event_type: EventType,
        metadata: ConversationMessageMetadata,
        g8e_context: G8eHttpContext,
        log_tag: str,
    ) -> None:
        """Record an approval lifecycle event to both operator activity_log and conversation_history."""
        if operator_id:
            try:
                await self.operator_data_service.add_operator_approval(
                    operator_id=operator_id,
                    event_type=event_type,
                    metadata=metadata,
                )
                logger.info("[%s] Recorded in operator activity_log", log_tag)
            except Exception as e:
                logger.error("[AUDIT-FAILURE] %s operator: %s", log_tag, e, exc_info=True)

        try:
            await self.investigation_data_service.add_approval_record(
                investigation_id=g8e_context.investigation_id,
                event_type=event_type,
                metadata=metadata,
            )
            logger.info("[%s] Recorded in conversation_history", log_tag)
        except Exception as e:
            logger.warning("[AUDIT-FAILURE] %s investigation: %s", log_tag, e)

    async def request_command_approval(self, request: CommandApprovalRequest) -> ApprovalResult:
        """Request operator approval for a command execution."""
        return await self._request_command_approval(
            command=request.command,
            justification=request.justification,
            g8e_context=request.g8e_context,
            timeout_seconds=request.timeout_seconds,
            user_id=request.g8e_context.user_id,
            execution_id=request.execution_id,
            operator_session_id=request.operator_session_id,
            operator_id=request.operator_id,
            risk_analysis=request.risk_analysis,
            target_systems=request.target_systems,
            task_id=request.task_id,
        )

    async def request_file_edit_approval(self, request: FileEditApprovalRequest) -> ApprovalResult:
        """Request operator approval for a file edit operation."""
        return await self._request_file_edit_approval(
            file_path=request.file_path,
            operation=request.operation,
            justification=request.justification,
            g8e_context=request.g8e_context,
            timeout_seconds=request.timeout_seconds,
            user_id=request.g8e_context.user_id,
            execution_id=request.execution_id,
            operator_session_id=request.operator_session_id,
            operator_id=request.operator_id,
            risk_analysis=request.risk_analysis,
        )

    async def request_intent_approval(self, request: IntentApprovalRequest) -> ApprovalResult:
        """Request operator approval for an intent (IAM) permission grant."""
        return await self._grant_intent_permission(
            intent_name=request.intent_name,
            justification=request.justification,
            g8e_context=request.g8e_context,
            timeout_seconds=request.timeout_seconds,
            user_id=request.g8e_context.user_id,
            execution_id=request.execution_id,
            operator_session_id=request.operator_session_id,
            operator_id=request.operator_id,
            all_intents=request.all_intents,
            operation_context=request.operation_context or "",
        )

    async def _request_command_approval(
        self,
        command: str,
        justification: str,
        g8e_context: G8eHttpContext,
        timeout_seconds: int,
        user_id: str,
        execution_id: str,
        operator_session_id: str,
        operator_id: str,
        risk_analysis: CommandRiskAnalysis | None,
        target_systems: list[TargetSystem],
        task_id: str | None,
    ) -> ApprovalResult:
        approval_id = generate_approval_id()
        try:

            logger.info("[APPROVAL] Requesting approval: %s", command)
            logger.info("[APPROVAL] approval_id=%s execution_id=%s", approval_id, execution_id)
            logger.info(
                "[APPROVAL] case_id=%s investigation_id=%s user_id=%s web_session_id=%s operator_id=%s",
                g8e_context.case_id, g8e_context.investigation_id, user_id,
                g8e_context.web_session_id, operator_id,
            )
            if risk_analysis:
                logger.info("[APPROVAL] risk_level=%s", risk_analysis.risk_level)

            approval_event = CommandApprovalEvent(
                approval_id=approval_id,
                execution_id=execution_id,
                command=command,
                justification=justification,
                timeout_seconds=timeout_seconds,
                user_id=user_id,
                task_id=task_id,
                risk_analysis=risk_analysis,
                target_systems=target_systems or [],
            )

            if approval_event.is_batch_execution:
                logger.info("[APPROVAL] Batch execution: %d systems", len(approval_event.target_systems))
                for ts in approval_event.target_systems:
                    logger.info("[APPROVAL]   - %s (%s)", ts.hostname, ts.operator_type)

            try:
                await self.g8ed_event_service.publish(
                    SessionEvent(
                        event_type=EventType.OPERATOR_COMMAND_APPROVAL_REQUESTED,
                        payload=approval_event,
                        web_session_id=g8e_context.web_session_id,
                        user_id=g8e_context.user_id,
                        case_id=g8e_context.case_id,
                        investigation_id=g8e_context.investigation_id,
                        task_id=task_id,
                    )
                )
                logger.info("[APPROVAL] Published to g8ed")
            except Exception as publish_error:
                error_msg = f"Failed to publish approval request to g8ed: {publish_error}"
                logger.error("[APPROVAL-PUBLISH-FAILURE] %s", error_msg, exc_info=True)
                return ApprovalResult(
                    approved=False,
                    reason=error_msg,
                    error=True,
                    error_type=ApprovalErrorType.APPROVAL_PUBLISH_FAILURE,
                    approval_id=approval_id,
                )

            await self._audit(
                operator_id=operator_id,
                event_type=EventType.OPERATOR_COMMAND_APPROVAL_REQUESTED,
                metadata=ApprovalMetadata(
                    execution_id=execution_id,
                    approval_id=approval_id,
                    command=command,
                    justification=justification,
                    requested_at=approval_event.requested_at,
                    is_batch_execution=approval_event.is_batch_execution,
                ),
                g8e_context=g8e_context,
                log_tag="APPROVAL",
            )

            pending = PendingApproval(
                approval_id=approval_id,
                approval_type=ApprovalType.COMMAND,
                command=command,
                requested_at=now(),
                case_id=g8e_context.case_id,
                investigation_id=g8e_context.investigation_id,
                user_id=user_id,
                operator_id=operator_id,
                operator_session_id=operator_session_id,
            )
            self._register_pending(approval_id, pending)
            logger.info("[APPROVAL] Stored pending (key=%s)", approval_id)

            logger.info("[APPROVAL] Awaiting user response")
            await pending.wait()
            self._pending_approvals.pop(approval_id, None)
            logger.info("[APPROVAL] Response received")

            if pending.operator_session_id and pending.operator_session_id != operator_session_id:
                logger.info(
                    "[APPROVAL] Updating operator_session_id: %s... -> %s...",
                    operator_session_id[:12] if operator_session_id else "null",
                    pending.operator_session_id[:12] if pending.operator_session_id else "null",
                )
                operator_session_id = pending.operator_session_id

            if pending.operator_id and pending.operator_id != operator_id:
                logger.info("[APPROVAL] Updating operator_id: %s", pending.operator_id)
                operator_id = pending.operator_id

            if pending.feedback:
                logger.info("[APPROVAL] User provided feedback: %s", pending.reason or "User sent a message")
                await self._audit(
                    operator_id=operator_id,
                    event_type=EventType.OPERATOR_COMMAND_APPROVAL_REJECTED,
                    metadata=ApprovalMetadata(
                        execution_id=execution_id,
                        approval_id=approval_id,
                        command=command,
                        feedback_reason=pending.reason,
                        responded_at=pending.responded_at or now(),
                    ),
                    g8e_context=g8e_context,
                    log_tag="APPROVAL",
                )
                return ApprovalResult(
                    approved=False,
                    feedback=True,
                    reason=pending.reason or "User provided additional context via chat message",
                    approval_id=approval_id,
                )

            logger.info("[APPROVAL] User response: approved=%s", pending.approved)

            await self._audit(
                operator_id=operator_id,
                event_type=EventType.OPERATOR_COMMAND_APPROVAL_GRANTED if pending.approved else EventType.OPERATOR_COMMAND_APPROVAL_REJECTED,
                metadata=ApprovalMetadata(
                    execution_id=execution_id,
                    approval_id=approval_id,
                    command=command,
                    approved=pending.approved,
                    reason=pending.reason,
                    responded_at=now(),
                ),
                g8e_context=g8e_context,
                log_tag="APPROVAL",
            )

            return ApprovalResult(
                approved=pending.approved or False,
                reason=pending.reason,
                approval_id=approval_id,
                operator_session_id=operator_session_id,
                operator_id=operator_id,
            )

        except Exception as e:
            logger.error(
                "[APPROVAL-EXCEPTION] Failed to request command approval: %s", e, exc_info=True
            )
            logger.error(
                "[APPROVAL-EXCEPTION] command=%s approval_id=%s case_id=%s investigation_id=%s user_id=%s web_session_id=%s operator_id=%s",
                command,
                approval_id,
                g8e_context.case_id,
                g8e_context.investigation_id,
                user_id,
                g8e_context.web_session_id,
                operator_id,
            )
            return ApprovalResult(
                approved=False,
                reason=f"Approval request failed: {e}",
                error=True,
                error_type=ApprovalErrorType.APPROVAL_EXCEPTION,
                approval_id=approval_id,
            )

    async def _request_file_edit_approval(
        self,
        file_path: str,
        operation: FileOperation,
        justification: str,
        g8e_context: G8eHttpContext,
        timeout_seconds: int,
        user_id: str,
        execution_id: str,
        operator_session_id: str,
        operator_id: str,
        risk_analysis: FileOperationRiskAnalysis | None,
    ) -> ApprovalResult:
        approval_id = generate_approval_id()
        try:

            logger.info("[FILE_EDIT_APPROVAL] Requesting approval: %s", file_path)
            logger.info("[FILE_EDIT_APPROVAL] approval_id=%s execution_id=%s", approval_id, execution_id)
            if risk_analysis:
                logger.info("[FILE_EDIT_APPROVAL] risk_level=%s", risk_analysis.risk_level)

            approval_event = FileEditApprovalEvent(
                approval_id=approval_id,
                execution_id=execution_id,
                file_path=file_path,
                operation=operation,
                justification=justification,
                timeout_seconds=timeout_seconds,
                user_id=user_id,
                risk_analysis=risk_analysis,
            )

            try:
                await self.g8ed_event_service.publish(
                    SessionEvent(
                        event_type=EventType.OPERATOR_FILE_EDIT_APPROVAL_REQUESTED,
                        payload=approval_event,
                        web_session_id=g8e_context.web_session_id,
                        user_id=g8e_context.user_id,
                        case_id=g8e_context.case_id,
                        investigation_id=g8e_context.investigation_id,
                    )
                )
                logger.info("[FILE_EDIT_APPROVAL] Published to g8ed")
            except Exception as publish_error:
                error_msg = f"Failed to publish file edit approval request to g8ed: {publish_error}"
                logger.error("[FILE_EDIT_APPROVAL-PUBLISH-FAILURE] %s", error_msg, exc_info=True)
                return ApprovalResult(
                    approved=False,
                    reason=error_msg,
                    error=True,
                    error_type=ApprovalErrorType.APPROVAL_PUBLISH_FAILURE,
                    approval_id=approval_id,
                )

            await self._audit(
                operator_id=operator_id,
                event_type=EventType.OPERATOR_FILE_EDIT_APPROVAL_REQUESTED,
                metadata=FileEditMetadata(
                    execution_id=execution_id,
                    approval_id=approval_id,
                    file_path=file_path,
                    operation=operation,
                    requested_at=approval_event.requested_at,
                ),
                g8e_context=g8e_context,
                log_tag="FILE_EDIT_APPROVAL",
            )

            pending = PendingApproval(
                approval_id=approval_id,
                approval_type=ApprovalType.FILE_EDIT,
                file_path=file_path,
                operation=operation,
                requested_at=now(),
                case_id=g8e_context.case_id,
                investigation_id=g8e_context.investigation_id,
                user_id=user_id,
                operator_id=operator_id,
                operator_session_id=operator_session_id,
            )
            self._register_pending(approval_id, pending)

            logger.info("[FILE_EDIT_APPROVAL] Awaiting user response")
            await pending.wait()
            self._pending_approvals.pop(approval_id, None)

            if pending.feedback:
                logger.info("[FILE_EDIT_APPROVAL] User provided feedback: %s", pending.reason or "User sent a message")
                await self._audit(
                    operator_id=operator_id,
                    event_type=EventType.OPERATOR_FILE_EDIT_APPROVAL_REJECTED,
                    metadata=FileEditMetadata(
                        execution_id=execution_id,
                        approval_id=approval_id,
                        file_path=file_path,
                        responded_at=pending.responded_at or now(),
                    ),
                    g8e_context=g8e_context,
                    log_tag="FILE_EDIT_APPROVAL",
                )
                return ApprovalResult(
                    approved=False,
                    feedback=True,
                    reason=pending.reason or "User provided additional context via chat message",
                    approval_id=approval_id,
                )

            logger.info("[FILE_EDIT_APPROVAL] User response: approved=%s", pending.approved)

            await self._audit(
                operator_id=operator_id,
                event_type=EventType.OPERATOR_FILE_EDIT_APPROVAL_GRANTED if pending.approved else EventType.OPERATOR_FILE_EDIT_APPROVAL_REJECTED,
                metadata=FileEditMetadata(
                    execution_id=execution_id,
                    approval_id=approval_id,
                    file_path=file_path,
                    operation=operation,
                    responded_at=now(),
                ),
                g8e_context=g8e_context,
                log_tag="FILE_EDIT_APPROVAL",
            )

            return ApprovalResult(
                approved=pending.approved or False,
                reason=pending.reason,
                approval_id=approval_id,
            )

        except Exception as e:
            logger.error(
                "[FILE_EDIT_APPROVAL-EXCEPTION] Failed to request file edit approval: %s",
                e,
                exc_info=True,
            )
            logger.error(
                "[FILE_EDIT_APPROVAL-EXCEPTION] file_path=%s approval_id=%s case_id=%s investigation_id=%s operator_id=%s",
                file_path,
                approval_id,
                g8e_context.case_id,
                g8e_context.investigation_id,
                operator_id,
            )
            return ApprovalResult(
                approved=False,
                reason=f"File edit approval request failed: {e}",
                error=True,
                error_type=ApprovalErrorType.APPROVAL_EXCEPTION,
                approval_id=approval_id,
            )

    async def _grant_intent_permission(
        self,
        intent_name: str | CloudIntent,
        justification: str,
        g8e_context: G8eHttpContext,
        timeout_seconds: int,
        user_id: str,
        execution_id: str,
        operator_session_id: str,
        operator_id: str,
        all_intents: list[str | CloudIntent],
        operation_context: str,
    ) -> ApprovalResult:
        normalized_str = intent_name.replace("-", "_").lower()

        try:
            intent: CloudIntent = CloudIntent(normalized_str)
        except ValueError:
            error_msg = f"Invalid intent '{intent_name}'. Valid intents: {', '.join(sorted(CloudIntent))}"
            logger.error("[INTENT_APPROVAL] %s", error_msg)
            return ApprovalResult(
                approved=False,
                reason=error_msg,
                error=True,
                error_type=ApprovalErrorType.INVALID_INTENT,
            )

        approval_id = generate_intent_approval_id()
        try:

            logger.info("[INTENT_APPROVAL] Requesting approval: %s", intent)
            logger.info("[INTENT_APPROVAL] approval_id=%s execution_id=%s operator_id=%s", approval_id, execution_id, operator_id)

            intent_question: str = CLOUD_INTENT_QUESTIONS.get(intent, f"Should I have {intent} permission?")

            parsed_all_intents: list[CloudIntent] = []
            if all_intents:
                for raw in all_intents:
                    raw_str = raw.replace("-", "_").lower()
                    try:
                        parsed_all_intents.append(CloudIntent(raw_str))
                    except ValueError:
                        parsed_all_intents.append(intent)
            else:
                parsed_all_intents = [intent]

            approval_event = IntentApprovalEvent(
                approval_id=approval_id,
                execution_id=execution_id,
                intent_name=intent,
                all_intents=parsed_all_intents,
                operation_context=operation_context,
                intent_question=intent_question,
                justification=justification,
                timeout_seconds=timeout_seconds,
                user_id=user_id,
                operator_id=operator_id,
            )

            try:
                await self.g8ed_event_service.publish(
                    SessionEvent(
                        event_type=EventType.OPERATOR_INTENT_APPROVAL_REQUESTED,
                        payload=approval_event,
                        web_session_id=g8e_context.web_session_id,
                        user_id=g8e_context.user_id,
                        case_id=g8e_context.case_id,
                        investigation_id=g8e_context.investigation_id,
                    )
                )
                logger.info("[INTENT_APPROVAL] Published to g8ed")
            except Exception as publish_error:
                error_msg = f"Failed to publish intent approval request to g8ed: {publish_error}"
                logger.error("[INTENT_APPROVAL] %s", error_msg, exc_info=True)
                return ApprovalResult(
                    approved=False,
                    reason=error_msg,
                    error=True,
                    error_type=ApprovalErrorType.APPROVAL_PUBLISH_FAILURE,
                    approval_id=approval_id,
                )

            await self._audit(
                operator_id=operator_id,
                event_type=EventType.OPERATOR_INTENT_APPROVAL_REQUESTED,
                metadata=ApprovalMetadata(
                    execution_id=execution_id,
                    approval_id=approval_id,
                    intent_name=intent,
                    intent_question=intent_question,
                    justification=justification,
                    requested_at=approval_event.requested_at,
                ),
                g8e_context=g8e_context,
                log_tag="INTENT_APPROVAL",
            )

            pending = PendingApproval(
                approval_id=approval_id,
                approval_type=ApprovalType.INTENT,
                intent_name=intent,
                requested_at=now(),
                case_id=g8e_context.case_id,
                investigation_id=g8e_context.investigation_id,
                user_id=user_id,
                operator_id=operator_id,
                operator_session_id=operator_session_id,
            )
            self._register_pending(approval_id, pending)
            logger.info("[INTENT_APPROVAL] Stored pending (key=%s)", approval_id)

            logger.info("[INTENT_APPROVAL] Awaiting user response")
            await pending.wait()
            self._pending_approvals.pop(approval_id, None)
            logger.info("[INTENT_APPROVAL] Response received")

            if pending.feedback:
                logger.info("[INTENT_APPROVAL] User provided feedback")
                await self._audit(
                    operator_id=operator_id,
                    event_type=EventType.OPERATOR_INTENT_APPROVAL_REJECTED,
                    metadata=ApprovalMetadata(
                        execution_id=execution_id,
                        approval_id=approval_id,
                        intent_name=intent,
                        feedback_reason=pending.reason,
                        responded_at=pending.responded_at or now(),
                    ),
                    g8e_context=g8e_context,
                    log_tag="INTENT_APPROVAL",
                )
                return ApprovalResult(
                    approved=False,
                    feedback=True,
                    reason=pending.reason or "User provided additional context",
                    approval_id=approval_id,
                )

            logger.info("[INTENT_APPROVAL] User response: approved=%s", pending.approved)

            await self._audit(
                operator_id=operator_id,
                event_type=EventType.OPERATOR_INTENT_APPROVAL_GRANTED if pending.approved else EventType.OPERATOR_INTENT_APPROVAL_REJECTED,
                metadata=ApprovalMetadata(
                    execution_id=execution_id,
                    approval_id=approval_id,
                    intent_name=intent,
                    approved=pending.approved,
                    reason=pending.reason,
                    responded_at=now(),
                ),
                g8e_context=g8e_context,
                log_tag="INTENT_APPROVAL",
            )

            return ApprovalResult(
                approved=pending.approved or False,
                intent_name=intent,
                reason=pending.reason or ("User approved intent grant" if pending.approved else "User denied intent grant"),
                approval_id=approval_id,
                operator_session_id=pending.operator_session_id or operator_session_id,
                operator_id=pending.operator_id or operator_id,
            )

        except Exception as e:
            logger.error("[INTENT_APPROVAL] Failed to request intent permission: %s", e, exc_info=True)
            return ApprovalResult(
                approved=False,
                reason=f"Intent approval request failed: {e}",
                error=True,
                error_type=ApprovalErrorType.INTENT_APPROVAL_EXCEPTION,
                approval_id=approval_id,
            )
