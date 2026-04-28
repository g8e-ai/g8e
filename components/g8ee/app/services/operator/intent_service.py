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

"""Operator Intent Service

Intent permission grant and revocation via Two-Role IAM architecture.
"""

import logging

from app.services.protocols import (
    ApprovalServiceProtocol,
    EventServiceProtocol,
    ExecutionServiceProtocol,
    InvestigationServiceProtocol,
    G8edClientProtocol,
)
from app.constants.status import (
    AITaskId,
    CommandErrorType,
    ComponentName,
    ExecutionStatus,
    OperatorType,
)
from app.constants.events import (
    EventType,
)
from app.constants.intents import (
    CLOUD_INTENT_DEPENDENCIES,
    CLOUD_INTENT_VERIFICATION_ACTIONS,
    CloudIntent,
)
from app.models.tool_args import GrantIntentArgs, RevokeIntentArgs
from app.models.http_context import G8eHttpContext
from app.models.investigations import EnrichedInvestigationContext
from app.models.tool_results import FailedIntentResult, IamIntentResult, IntentPermissionResult
from app.models.command_request_payloads import CommandRequestPayload
from app.models.operators import IntentApprovalRequest, CommandExecutingBroadcastEvent, CommandResultBroadcastEvent, CloudSubtype
from app.models.pubsub_messages import G8eMessage
from app.services.operator.iam_command_builder import IamCommandBuilder
from app.utils.ids import (
    generate_iam_execution_id,
    generate_iam_revoke_intent_execution_id,
    generate_iam_verify_execution_id,
    generate_intent_execution_id,
)
from app.utils.timestamp import now

logger = logging.getLogger(__name__)


class OperatorIntentService:
    """Intent permission grant and revocation for Cloud Operators (AWS Two-Role IAM)."""

    def __init__(
        self,
        approval_service: ApprovalServiceProtocol,
        execution_service: ExecutionServiceProtocol,
        g8ed_event_service: EventServiceProtocol,
        investigation_service: InvestigationServiceProtocol,
        g8ed_client: G8edClientProtocol,
    ) -> None:
        self._approval_service = approval_service
        self._execution_service = execution_service
        self._g8ed_event_service = g8ed_event_service
        self._investigation_service = investigation_service
        self._g8ed_client = g8ed_client
        self._iam_builder = IamCommandBuilder()

    @property
    def approval_service(self) -> ApprovalServiceProtocol:
        return self._approval_service

    @property
    def execution_service(self) -> ExecutionServiceProtocol:
        return self._execution_service

    @property
    def g8ed_event_service(self) -> EventServiceProtocol:
        return self._g8ed_event_service

    @property
    def investigation_service(self) -> InvestigationServiceProtocol:
        return self._investigation_service

    @property
    def g8ed_client(self) -> G8edClientProtocol:
        return self._g8ed_client

    def _resolve_intent_dependencies(self, requested_intents: list[str]) -> list[str]:
        all_intents = set(requested_intents)
        changed = True
        while changed:
            changed = False
            for intent in list(all_intents):
                for dep in CLOUD_INTENT_DEPENDENCIES.get(intent, []):
                    if dep not in all_intents:
                        all_intents.add(dep)
                        changed = True
        return sorted(list(all_intents))

    def _get_verification_action_for_intent(self, intent: str) -> str | None:
        return CLOUD_INTENT_VERIFICATION_ACTIONS.get(intent)

    def _build_iam_attach_command(self, intent: str) -> str:
        return self._iam_builder.build_attach_command(intent)

    def _build_iam_detach_command(self, intent: str) -> str:
        return self._iam_builder.build_detach_command(intent)

    def _build_iam_verify_command(self, intent: str, verification_action: str) -> str:
        return self._iam_builder.build_verify_command(intent, verification_action)

    async def execute_intent_permission_request(
        self,
        *,
        args: GrantIntentArgs,
        g8e_context: G8eHttpContext,
        investigation: EnrichedInvestigationContext,
    ) -> IntentPermissionResult:
        intent_names_raw = args.intent_name.strip()
        operation_context = (args.operation_context or "").strip()
        justification = args.justification.strip()

        requested_intents = [i.strip().replace("-", "_").lower()
                             for i in intent_names_raw.split(",") if i.strip()]

        if not requested_intents:
            return IntentPermissionResult(success=False, error="At least one intent name is required", error_type=CommandErrorType.VALIDATION_ERROR)

        if not justification:
            return IntentPermissionResult(success=False, error="Justification is required", error_type=CommandErrorType.VALIDATION_ERROR)

        all_intents = self._resolve_intent_dependencies(requested_intents)

        invalid_intents = [i for i in all_intents if i not in CloudIntent._value2member_map_]
        if invalid_intents:
            return IntentPermissionResult(
                success=False,
                error=f"Invalid intents: {', '.join(invalid_intents)}. Valid intents: {', '.join(sorted(CloudIntent._value2member_map_))}",
                error_type=CommandErrorType.INVALID_INTENT,
                invalid_intents=invalid_intents,
                requested_intents=requested_intents,
            )

        op_doc = (investigation.operator_documents[0] if investigation and investigation.operator_documents else None)
        if not op_doc or op_doc.operator_type != OperatorType.CLOUD:
            return IntentPermissionResult(success=False, error="Intent permissions require a Cloud Operator", error_type=CommandErrorType.CLOUD_OPERATOR_REQUIRED)

        if op_doc.cloud_subtype == CloudSubtype.G8E_POD:
            return IntentPermissionResult(
                success=False,
                error="g8ep operators have direct system access and do not support IAM intent grants. Use run_commands_with_operator directly.",
                error_type=CommandErrorType.VALIDATION_ERROR
            )

        operator_id = op_doc.id
        operator_session_id = op_doc.operator_session_id
        execution_id = generate_intent_execution_id()

        approval_result = await self.approval_service.request_intent_approval(
            IntentApprovalRequest(
                g8e_context=g8e_context,
                timeout_seconds=600,
                justification=justification,
                execution_id=execution_id,
                operator_session_id=operator_session_id or "unknown",
                operator_id=operator_id,
                intent_name=all_intents[0],
                all_intents=all_intents,
                operation_context=operation_context,
            )
        )

        # Notify start
        await self.g8ed_event_service.publish_command_event(
            EventType.OPERATOR_INTENT_APPROVAL_REQUESTED,
            CommandExecutingBroadcastEvent(
                command=f"intent_grant {', '.join(requested_intents)}",
                execution_id=execution_id,
                operator_session_id=operator_session_id,
                approval_id=None, # Approval ID not generated yet or managed by service
            ),
            g8e_context,
            task_id=AITaskId.INTENT_GRANT,
        )

        if not approval_result.approved:
            # Notify failure
            await self.g8ed_event_service.publish_command_event(
                EventType.OPERATOR_INTENT_DENIED,
                CommandResultBroadcastEvent(
                    execution_id=execution_id,
                    command=f"intent_grant {', '.join(requested_intents)}",
                    status=ExecutionStatus.DENIED,
                    error=approval_result.reason or "User denied",
                    operator_id=operator_id,
                    operator_session_id=operator_session_id,
                    approval_id=approval_result.approval_id,
                ),
                g8e_context,
                task_id=AITaskId.INTENT_GRANT,
            )

            return IntentPermissionResult(
                success=False,
                approved=False,
                feedback=approval_result.feedback,
                error=f"Intent permission denied: {approval_result.reason}",
                error_type=CommandErrorType.USER_FEEDBACK if approval_result.feedback else CommandErrorType.USER_DENIED,
                intent_name=all_intents[0],
                all_intents=all_intents,
            )

        final_session_id = approval_result.operator_session_id or operator_session_id
        final_op_id = approval_result.operator_id or operator_id

        # Notify completion
        await self.g8ed_event_service.publish_command_event(
            EventType.OPERATOR_INTENT_GRANTED,
            CommandResultBroadcastEvent(
                execution_id=execution_id,
                command=f"intent_grant {', '.join(requested_intents)}",
                status=ExecutionStatus.COMPLETED,
                output=f"Intent permission granted: {', '.join(all_intents)}",
                operator_id=final_op_id,
                operator_session_id=final_session_id,
                approval_id=approval_result.approval_id,
            ),
            g8e_context,
            task_id=AITaskId.INTENT_GRANT,
        )

        iam_results: list[IamIntentResult] = []
        failed_intents: list[FailedIntentResult] = []

        for intent in all_intents:
            attach_cmd = self._build_iam_attach_command(intent)
            exec_id = generate_iam_execution_id(intent)
            msg = G8eMessage(
                id=exec_id,
                source_component=ComponentName.G8EE,
                event_type=EventType.OPERATOR_COMMAND_REQUESTED,
                case_id=g8e_context.case_id,
                task_id=AITaskId.COMMAND,
                investigation_id=g8e_context.investigation_id,
                web_session_id=g8e_context.web_session_id,
                operator_session_id=final_session_id,
                operator_id=final_op_id,
                payload=CommandRequestPayload(
                    command=attach_cmd,
                    execution_id=exec_id,
                    justification="IAM Policy Update",
                ),
            )
            
            iam_result, _envelope = await self.execution_service.execute(msg, g8e_context)
            iam_results.append(IamIntentResult(intent=intent, result=iam_result))

            if iam_result and iam_result.status != ExecutionStatus.COMPLETED:
                failed_intents.append(FailedIntentResult(intent=intent, error=(iam_result.error if iam_result else None) or "Unknown error"))
            else:
                v_action = self._get_verification_action_for_intent(intent)
                if v_action:
                    v_cmd = self._build_iam_verify_command(intent, v_action)
                    v_exec_id = generate_iam_verify_execution_id(intent)
                    v_msg = G8eMessage(
                        id=v_exec_id,
                        source_component=ComponentName.G8EE,
                        event_type=EventType.OPERATOR_COMMAND_REQUESTED,
                        case_id=g8e_context.case_id,
                        task_id=AITaskId.COMMAND,
                        investigation_id=g8e_context.investigation_id,
                        web_session_id=g8e_context.web_session_id,
                        operator_session_id=final_session_id,
                        operator_id=final_op_id,
                        payload=CommandRequestPayload(
                            command=v_cmd,
                            execution_id=v_exec_id,
                            justification="IAM Verification",
                        ),
                    )
                    await self.execution_service.execute(v_msg, g8e_context)

        if failed_intents:
            return IntentPermissionResult(success=False, approved=True, error="Partial IAM failure", failed_intents=failed_intents, iam_results=iam_results)

        return IntentPermissionResult(
            success=True, approved=True, intent_name=all_intents[0], all_intents=all_intents, 
            message=f"Permission granted for {', '.join(all_intents)}", iam_results=iam_results, timestamp=now()
        )

    async def execute_intent_revocation(
        self,
        *,
        args: RevokeIntentArgs,
        g8e_context: G8eHttpContext,
        investigation: EnrichedInvestigationContext,
    ) -> IntentPermissionResult:
        intent_names_raw = args.intent_name.strip()
        requested_intents = [i.strip().replace("-", "_").lower()
                             for i in intent_names_raw.split(",") if i.strip()]

        if not requested_intents:
            return IntentPermissionResult(
                success=False,
                error="At least one intent name is required",
                error_type=CommandErrorType.VALIDATION_ERROR,
            )

        justification = args.justification.strip()
        if not justification:
            return IntentPermissionResult(
                success=False,
                error="Justification is required",
                error_type=CommandErrorType.VALIDATION_ERROR,
            )

        op_doc = (investigation.operator_documents[0] if investigation and investigation.operator_documents else None)
        if not op_doc or not op_doc.operator_session_id:
            return IntentPermissionResult(success=False, error="Operator offline", error_type=CommandErrorType.NO_OPERATORS_AVAILABLE)

        execution_id = generate_intent_execution_id()

        # Notify start
        await self.g8ed_event_service.publish_command_event(
            EventType.OPERATOR_COMMAND_STARTED,
            CommandExecutingBroadcastEvent(
                command=f"intent_revoke {', '.join(requested_intents)}",
                execution_id=execution_id,
                operator_session_id=op_doc.operator_session_id,
            ),
            g8e_context,
            task_id=AITaskId.INTENT_REVOKE,
        )

        iam_results: list[IamIntentResult] = []
        for intent in requested_intents:
            detach_cmd = self._build_iam_detach_command(intent)
            exec_id = generate_iam_revoke_intent_execution_id(intent)
            msg = G8eMessage(
                id=exec_id,
                source_component=ComponentName.G8EE,
                event_type=EventType.OPERATOR_COMMAND_REQUESTED,
                case_id=g8e_context.case_id,
                task_id=AITaskId.COMMAND,
                investigation_id=g8e_context.investigation_id,
                web_session_id=g8e_context.web_session_id,
                operator_session_id=op_doc.operator_session_id,
                operator_id=op_doc.id,
                payload=CommandRequestPayload(
                    command=detach_cmd,
                    execution_id=exec_id,
                    justification="IAM Revoke",
                ),
            )
            res, _envelope = await self.execution_service.execute(msg, g8e_context)
            iam_results.append(IamIntentResult(intent=intent, result=res))
            if res and res.status == ExecutionStatus.COMPLETED and self.g8ed_client:
                await self.g8ed_client.revoke_intent(op_doc.id, intent, g8e_context)

        # Notify completion
        await self.g8ed_event_service.publish_command_event(
            EventType.OPERATOR_INTENT_REVOKED,
            CommandResultBroadcastEvent(
                execution_id=execution_id,
                command=f"intent_revoke {', '.join(requested_intents)}",
                status=ExecutionStatus.COMPLETED,
                output=f"Intent permission revoked: {', '.join(requested_intents)}",
                operator_id=op_doc.id,
                operator_session_id=op_doc.operator_session_id,
            ),
            g8e_context,
            task_id=AITaskId.INTENT_REVOKE,
        )

        return IntentPermissionResult(success=True, revoked_intents=requested_intents, iam_results=iam_results, timestamp=now())

