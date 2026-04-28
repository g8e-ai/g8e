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
Internal API Router for g8ee

Cluster-internal HTTP endpoints for direct communication from other g8e components.
NOT exposed via Ingress - only accessible from pods within the Kubernetes cluster.

Note: g8eo Operator commands still use PubSub (external agent communication).
"""

import asyncio
import logging
from fastapi import APIRouter, Depends, Request, status

from app.models.settings import G8eePlatformSettings, G8eeUserSettings
from app.constants import (
    ComponentName,
    DB_COLLECTION_MEMORIES,
    EventType,
    OperatorHistoryEventType,
    OperatorStatus,
    Priority,
    InternalApiPaths,
)
from app.errors import ResourceNotFoundError, ServiceUnavailableError
from app.models import CaseCreateRequest, CaseEventPayload, CaseUpdateRequest
from app.models.cases import CaseCreatedPayload
from app.models.cache import FieldFilter
from app.models.internal_api import (
    ApiKeyGenerationRequest,
    ApiKeyGenerationResponse,
    ApprovalRespondedResponse,
    CaseResponse,
    ChatMessageRequest,
    ChatStartedResponse,
    DirectCommandRequest,
    DirectCommandSentResponse,
    G8epOperatorActivationRequest,
    G8epOperatorActivationResponse,
    G8epOperatorRelaunchRequest,
    G8epOperatorRelaunchResponse,
    OperatorApprovalResponse,
    InternalOperatorAuthCall,
    OperatorAuthenticateResponse,
    OperatorDeviceLinkRegisterRequest,
    OperatorDeviceLinkRegisterResponse,
    OperatorBindRequest,
    OperatorBindResponse,
    OperatorCertificateRevokeRequest,
    OperatorCertificateRevokeResponse,
    OperatorListenSessionAuthRequest,
    OperatorSessionRegisteredResponse,
    OperatorSessionRegistrationRequest,
    OperatorSessionRefreshRequest,
    OperatorSessionRefreshResponse,
    OperatorSessionValidateRequest,
    OperatorSessionValidateResponse,
    OperatorSlotClaimRequest,
    OperatorSlotClaimResponse,
    OperatorSlotCreationRequest,
    OperatorSlotCreationResponse,
    OperatorStoppedResponse,
    OperatorTerminateRequest,
    OperatorTerminateResponse,
    OperatorUnbindRequest,
    OperatorUnbindResponse,
    OperatorUpdateApiKeyRequest,
    OperatorUpdateApiKeyResponse,
    PendingApprovalsResponse,
    StopAIRequest,
    StopAIResponse,
    StopOperatorRequest,
)
from app.models.triage_api import (
    TriageAnswerRequest,
    TriageSkipRequest,
    TriageTimeoutRequest,
)
from app.models.investigations import (
    InvestigationCreateRequest,
    InvestigationModel,
    InvestigationQueryRequest,
    InvestigationUpdateRequest,
)
from app.models.events import SessionEvent
from app.models.http_context import G8eHttpContext
from app.services.operator.session_auth_listener import SessionAuthListener
from app.services.operator.operator_data_service import OperatorDataService
from app.services.operator.heartbeat_service import HeartbeatSnapshotService
from app.services.operator.approval_service import OperatorApprovalService
from app.services.operator.command_service import OperatorCommandService
from app.services.operator.operator_session_service import OperatorSessionService
from app.services.operator.operator_auth_service import OperatorAuthService
from app.services.data.case_data_service import CaseDataService
from app.services.data.attachment_store_service import AttachmentService
from app.services.investigation.investigation_service import InvestigationService
from app.services.ai.chat_pipeline import ChatPipelineService
from app.services.ai.chat_task_manager import BackgroundTaskManager
from app.services.ai.title_generator import generate_case_title
from app.services.infra.g8ed_event_service import EventService
from app.services.cache.cache_aside import CacheAsideService
from app.services.auth.api_key_service import ApiKeyService
from app.services.auth.certificate_service import CertificateService
from app.services.infra.settings_service import SettingsService
from app.utils.timestamp import now_iso

from ..dependencies import (
    get_g8ee_platform_settings,
    get_g8ee_approval_service,
    get_g8ee_attachment_service,
    get_g8ee_cache_aside_service,
    get_g8ee_case_data_service,
    get_g8ee_chat_pipeline,
    get_g8ee_chat_task_manager,
    get_g8ee_event_service,
    get_g8ee_heartbeat_service,
    get_g8ee_investigation_service,
    get_g8ee_operator_command_service,
    get_g8ee_operator_data_service,
    get_g8ee_operator_lifecycle_service,
    get_g8ee_operator_session_service,
    get_g8ee_operator_auth_service,
    get_g8ee_session_auth_listener,
    get_g8ee_api_key_service,
    get_g8ee_certificate_service,
    get_g8ee_settings_service_write,
    get_g8e_http_context,
    get_g8ee_user_settings,
)

logger = logging.getLogger(__name__)

router = APIRouter(tags=["internal"])




async def _generate_and_update_title(
    message: str,
    case_id: str,
    investigation_id: str,
    web_session_id: str,
    user_id: str,
    user_settings: G8eeUserSettings,
    case_service: CaseDataService,
    investigation_service: InvestigationService
):
    try:
        case_result = await generate_case_title(message, settings=user_settings)
        ai_title = case_result.generated_title
        updated_case = await case_service.update_case(case_id, CaseUpdateRequest(title=ai_title))
        await investigation_service.update_investigation(
            investigation_id,
            InvestigationUpdateRequest(case_title=ai_title)
        )
        await case_service.publish_case_update_sse(
            case_id=case_id,
            web_session_id=web_session_id,
            payload=CaseEventPayload(
                updated_at=updated_case.updated_at,
                title=ai_title,
            ),
            user_id=user_id,
        )
    except Exception as e:
        logger.error(
            "[INTERNAL-HTTP] Failed to generate case title in background task",
            extra={"case_id": case_id, "error": str(e)},
            exc_info=True
        )


@router.post(InternalApiPaths.G8EE_CHAT, response_model=ChatStartedResponse)
async def internal_chat(
    request: ChatMessageRequest,
    platform_settings: G8eePlatformSettings = Depends(get_g8ee_platform_settings),
    user_settings: G8eeUserSettings = Depends(get_g8ee_user_settings),
    chat_pipeline: ChatPipelineService = Depends(get_g8ee_chat_pipeline),
    chat_task_manager: BackgroundTaskManager = Depends(get_g8ee_chat_task_manager),
    case_service: CaseDataService = Depends(get_g8ee_case_data_service),
    investigation_service: InvestigationService = Depends(get_g8ee_investigation_service),
    attachment_service: AttachmentService = Depends(get_g8ee_attachment_service),
    event_service: EventService = Depends(get_g8ee_event_service),
    g8e_context: G8eHttpContext = Depends(get_g8e_http_context)
):
    """
    Non-streaming chat endpoint — default path for browser sessions.

    Creates case + investigation inline when case_id is absent, then fires
    run_chat as a background task. The AI response and all tool events are
    delivered to the browser via the existing SSE connection; this endpoint
    returns immediately with case/investigation IDs so the browser can update
    its state without waiting for the LLM.
    """
    logger.info(
        "[INTERNAL-HTTP] Non-streaming chat request received",
        extra={
            "case_id": g8e_context.case_id,
            "investigation_id": g8e_context.investigation_id,
            "new_case": g8e_context.new_case,
            "web_session_id": g8e_context.web_session_id[:8] + "...",
            "message_length": len(request.message),
        }
    )

    if g8e_context.new_case:
        case_create_data = CaseCreateRequest(
            initial_message=request.message,
            attachments=request.attachments or [],
            sentinel_mode=request.sentinel_mode,
            user_id=g8e_context.user_id,
            web_session_id=g8e_context.web_session_id,
            organization_id=g8e_context.organization_id,
        )
        case = await case_service.create_case(case_create_data, generated_title=None)

        investigation_request = InvestigationCreateRequest(
            case_id=case.id,
            case_title=case.title,
            case_description=case.description,
            web_session_id=g8e_context.web_session_id,
            priority=Priority(case.priority) if isinstance(case.priority, str) else case.priority,
            user_email=case.user_email,
            user_id=case.user_id,
            sentinel_mode=request.sentinel_mode,
            created_with_case=True,
            case_source=case.source,
        )
        investigation = await investigation_service.create_investigation(investigation_request)

        g8e_context = g8e_context.model_copy(update={
            "case_id": case.id,
            "investigation_id": investigation.id,
        })

        # Publish CASE_CREATED event immediately after inline creation.
        # This MUST happen BEFORE title generation and its CASE_UPDATED event,
        # so the frontend receives creation before update.
        try:
            await event_service.publish(
                SessionEvent(
                    event_type=EventType.CASE_CREATED,
                    payload=CaseCreatedPayload(title=case.title),
                    web_session_id=g8e_context.web_session_id,
                    user_id=g8e_context.user_id,
                    case_id=g8e_context.case_id,
                    investigation_id=g8e_context.investigation_id,
                )
            )
        except Exception as sse_err:
            logger.warning(
                "[INTERNAL-HTTP] Failed to publish g8e.v1.app.case.created SSE — case created successfully, continuing",
                extra={"case_id": g8e_context.case_id, "error": str(sse_err)},
            )

        if request.message.strip():
            task = asyncio.create_task(
                _generate_and_update_title(
                    message=request.message,
                    case_id=g8e_context.case_id,
                    investigation_id=g8e_context.investigation_id,
                    web_session_id=g8e_context.web_session_id,
                    user_id=g8e_context.user_id,
                    user_settings=user_settings,
                    case_service=case_service,
                    investigation_service=investigation_service,
                )
            )
            # Track background task for cleanup
            task_id = f"title_generation_{g8e_context.investigation_id}"
            asyncio.create_task(chat_task_manager.track(task_id, task, auto_cancel_previous=False))

        logger.info(
            "[INTERNAL-HTTP] New conversation created inline",
            extra={"case_id": g8e_context.case_id, "investigation_id": g8e_context.investigation_id}
        )

    resolved_attachments = []
    if request.attachments:
        try:
            raw_attachments = await attachment_service.get_attachments_by_metadata(
                request.attachments
            )
            resolved_attachments = await attachment_service.process_attachments(raw_attachments)
        except Exception as att_err:
            logger.error(f"[INTERNAL-HTTP] Failed to retrieve attachments: {att_err}")

    # Validate investigation_id exists before proceeding (allow NEW_CASE_ID for new cases)
    if not g8e_context.investigation_id or g8e_context.investigation_id == "unknown":
        logger.error(
            "[INTERNAL-HTTP] Cannot start chat - investigation_id is missing or unknown",
            extra={"case_id": g8e_context.case_id, "web_session_id": g8e_context.web_session_id[:8] + "..."}
        )
        return ChatStartedResponse(
            success=False,
            case_id=g8e_context.case_id or "",
            investigation_id=g8e_context.investigation_id or "",
        )

    chat_task = asyncio.create_task(
        chat_pipeline.run_chat(
            message=request.message,
            g8e_context=g8e_context,
            attachments=resolved_attachments,
            sentinel_mode=request.sentinel_mode,
            llm_primary_provider=request.llm_primary_provider,
            llm_assistant_provider=request.llm_assistant_provider,
            llm_lite_provider=request.llm_lite_provider,
            llm_primary_model=request.llm_primary_model,
            llm_assistant_model=request.llm_assistant_model,
            llm_lite_model=request.llm_lite_model or user_settings.llm.resolved_lite_model,
            _task_manager=chat_task_manager,
            user_settings=user_settings,
        )
    )
    # Track the task - run_chat will also track it internally, but we track it here
    # to ensure it's in the registry before we return
    asyncio.create_task(chat_task_manager.track(g8e_context.investigation_id, chat_task, auto_cancel_previous=False))

    return ChatStartedResponse(
        success=True,
        case_id=g8e_context.case_id,
        investigation_id=g8e_context.investigation_id,
    )


@router.post(InternalApiPaths.G8EE_CHAT_TRIAGE_ANSWER)
async def internal_triage_answer(
    request: TriageAnswerRequest,
    investigation_service: InvestigationService = Depends(get_g8ee_investigation_service),
    g8e_context: G8eHttpContext = Depends(get_g8e_http_context),
):
    """
    Receive user answer to a triage clarifying question - internal cluster use only.
    """
    from app.models.investigations import ConversationMessageMetadata
    from app.models.investigation_status import MessageSender

    logger.info(
        "[INTERNAL-HTTP] Triage answer received",
        extra={
            "investigation_id": request.investigation_id,
            "question_index": request.question_index,
            "answer": request.answer,
            "user_id": g8e_context.user_id,
        }
    )

    # Store answer as user.chat message with structured metadata
    await investigation_service.investigation_data_service.add_chat_message(
        investigation_id=request.investigation_id,
        sender=MessageSender.USER_CHAT,
        content=f"Answered clarifying question {request.question_index}: {'Yes' if request.answer else 'No'}",
        metadata=ConversationMessageMetadata(
            event_type=EventType.AI_TRIAGE_CLARIFICATION_ANSWERED,
            question_index=request.question_index,
            answer=request.answer
        )
    )
    return {"success": True}


@router.post(InternalApiPaths.G8EE_CHAT_TRIAGE_SKIP)
async def internal_triage_skip(
    request: TriageSkipRequest,
    investigation_service: InvestigationService = Depends(get_g8ee_investigation_service),
    g8e_context: G8eHttpContext = Depends(get_g8e_http_context),
):
    """
    Skip triage clarifying questions - internal cluster use only.
    """
    from app.models.investigations import ConversationMessageMetadata
    from app.models.investigation_status import MessageSender

    logger.info(
        "[INTERNAL-HTTP] Triage skip received",
        extra={
            "investigation_id": request.investigation_id,
            "user_id": g8e_context.user_id,
        }
    )

    await investigation_service.investigation_data_service.add_chat_message(
        investigation_id=request.investigation_id,
        sender=MessageSender.USER_CHAT,
        content="Skipped clarifying questions",
        metadata=ConversationMessageMetadata(
            event_type=EventType.AI_TRIAGE_CLARIFICATION_SKIPPED
        )
    )
    return {"success": True}


@router.post(InternalApiPaths.G8EE_CHAT_TRIAGE_TIMEOUT)
async def internal_triage_timeout(
    request: TriageTimeoutRequest,
    investigation_service: InvestigationService = Depends(get_g8ee_investigation_service),
    g8e_context: G8eHttpContext = Depends(get_g8e_http_context),
):
    """
    Record triage clarifying questions timeout - internal cluster use only.
    """
    from app.models.investigations import ConversationMessageMetadata
    from app.models.investigation_status import MessageSender

    logger.info(
        "[INTERNAL-HTTP] Triage timeout received",
        extra={
            "investigation_id": request.investigation_id,
            "user_id": g8e_context.user_id,
        }
    )

    await investigation_service.investigation_data_service.add_chat_message(
        investigation_id=request.investigation_id,
        sender=MessageSender.USER_CHAT,
        content="Clarifying questions timed out",
        metadata=ConversationMessageMetadata(
            event_type=EventType.AI_TRIAGE_CLARIFICATION_TIMEOUT
        )
    )
    return {"success": True}


@router.post(InternalApiPaths.G8EE_CHAT_STOP, response_model=StopAIResponse)
async def stop_ai_processing(
    request: StopAIRequest,
    g8e_context: G8eHttpContext = Depends(get_g8e_http_context),
    chat_task_manager: BackgroundTaskManager = Depends(get_g8ee_chat_task_manager),
    chat_pipeline: ChatPipelineService = Depends(get_g8ee_chat_pipeline),
):
    """
    Stop active AI processing for an investigation - internal cluster use only.

    Called by g8ed when user clicks the stop button in the UI.
    Gracefully cancels the asyncio task processing the AI response.
    """
    investigation_id = request.investigation_id
    reason = request.reason
    web_session_id = request.web_session_id

    logger.info(
        "[INTERNAL-HTTP] Stop AI processing request",
        extra={
            "investigation_id": investigation_id,
            "reason": reason,
            "user_id": g8e_context.user_id
        }
    )

    cancelled = await chat_task_manager.cancel(
        task_id=investigation_id,
        reason=reason,
        web_session_id=web_session_id,
        user_id=g8e_context.user_id,
        case_id=g8e_context.case_id,
        g8ed_event_service=chat_pipeline.g8ed_event_service,
    )

    if cancelled:
        logger.info(
            "[INTERNAL-HTTP] AI processing stopped successfully",
            extra={"investigation_id": investigation_id}
        )
        return StopAIResponse(
            success=True,
            investigation_id=investigation_id,
            was_active=True,
        )
    logger.info(
        "[INTERNAL-HTTP] No active AI processing to stop",
        extra={"investigation_id": investigation_id}
    )
    return StopAIResponse(
        success=True,
        investigation_id=investigation_id,
        was_active=False,
    )


@router.post(InternalApiPaths.G8EE_OPERATOR_APPROVAL_RESPOND, response_model=ApprovalRespondedResponse)
async def operator_approval_respond(
    request: OperatorApprovalResponse,
    g8e_context: G8eHttpContext = Depends(get_g8e_http_context),
    approval_service: OperatorApprovalService = Depends(get_g8ee_approval_service),
):
    """
    Handle Operator command approval response from g8ed.
    Called directly by g8ed via HTTP when user approves/denies a command.
    """
    bound_op = g8e_context.bound_operators[0] if g8e_context.bound_operators else None
    request.operator_session_id = bound_op.operator_session_id or "" if bound_op else ""
    request.operator_id = bound_op.operator_id if bound_op else ""

    logger.info(
        "[INTERNAL-HTTP] Received approval response from g8ed",
        extra={
            "approval_id": request.approval_id,
            "approved": request.approved,
            "case_id": g8e_context.case_id,
            "investigation_id": g8e_context.investigation_id,
            "web_session_id": g8e_context.web_session_id[:12] + "..." if g8e_context.web_session_id else None,
            "bound_operators_count": len(g8e_context.bound_operators),
            "operator_id": request.operator_id,
            "user_id": g8e_context.user_id,
        }
    )

    await approval_service.handle_approval_response(request)

    logger.info(
        "[INTERNAL-HTTP] Approval response processed successfully",
        extra={
            "approval_id": request.approval_id,
            "approved": request.approved,
        }
    )

    return ApprovalRespondedResponse(
        success=True,
        approval_id=request.approval_id,
        approved=request.approved,
    )


@router.get(InternalApiPaths.G8EE_OPERATOR_APPROVAL_PENDING, response_model=PendingApprovalsResponse)
async def get_pending_approvals(
    approval_service: OperatorApprovalService = Depends(get_g8ee_approval_service),
):
    """
    Get all pending approvals currently waiting for user response.
    Returns a dictionary of approval_id -> PendingApproval.
    """
    pending_approvals = approval_service.get_pending_approvals()

    logger.info(
        "[INTERNAL-HTTP] Retrieved pending approvals",
        extra={"count": len(pending_approvals)}
    )

    return PendingApprovalsResponse(pending_approvals=pending_approvals)


@router.post(InternalApiPaths.G8EE_OPERATORS_G8EP_ACTIVATE, response_model=G8epOperatorActivationResponse)
async def activate_g8ep_operator(
    request: G8epOperatorActivationRequest,
    operator_lifecycle_service: "OperatorLifecycleService" = Depends(get_g8ee_operator_lifecycle_service),
):
    """
    Activate the g8ep operator for a user.
    
    Called by g8ed after login/registration.
    Authority: g8ee (process owner for g8ep operator).
    """
    try:
        await operator_lifecycle_service.activate_g8ep_operator(request.user_id)
        return G8epOperatorActivationResponse(success=True)
    except Exception as e:
        logger.error(
            "[INTERNAL-HTTP] Failed to activate g8ep operator",
            extra={"error": str(e), "user_id": request.user_id}
        )
        return G8epOperatorActivationResponse(success=False, error=str(e))


@router.post(InternalApiPaths.G8EE_OPERATORS_G8EP_RELAUNCH, response_model=G8epOperatorRelaunchResponse)
async def relaunch_g8ep_operator(
    request: G8epOperatorRelaunchRequest,
    operator_lifecycle_service: "OperatorLifecycleService" = Depends(get_g8ee_operator_lifecycle_service),
):
    """
    Relaunch the g8ep operator for a user.
    
    Called by g8ed during reauth/relaunch.
    Authority: g8ee (process owner for g8ep operator).
    """
    try:
        result = await operator_lifecycle_service.relaunch_g8ep_operator(request.user_id)
        return G8epOperatorRelaunchResponse(
            success=result.get("success", False),
            operator_id=result.get("operator_id"),
            error=result.get("error"),
        )
    except Exception as e:
        logger.error(
            "[INTERNAL-HTTP] Failed to relaunch g8ep operator",
            extra={"error": str(e), "user_id": request.user_id}
        )
        return G8epOperatorRelaunchResponse(success=False, error=str(e))


@router.post(InternalApiPaths.G8EE_OPERATOR_DIRECT_COMMAND, response_model=DirectCommandSentResponse)
async def execute_direct_command(
    request: DirectCommandRequest,
    g8e_context: G8eHttpContext = Depends(get_g8e_http_context),
    operator_data_service: OperatorCommandService = Depends(get_g8ee_operator_command_service),
):
    logger.info(
        "[INTERNAL-HTTP] Direct command request received",
        extra={
            "command": request.command[:100] if len(request.command) > 100 else request.command,
            "execution_id": g8e_context.execution_id,
            "web_session_id": g8e_context.web_session_id[:12] + "...",
            "source": g8e_context.source_component,
            "has_case_id": g8e_context.case_id is not None,
            "has_investigation_id": g8e_context.investigation_id is not None,
        }
    )

    await operator_data_service.send_command_to_operator(
        command_payload=request,
        g8e_context=g8e_context,
    )

    await operator_data_service.send_direct_exec_audit_event(
        command=request.command,
        execution_id=g8e_context.execution_id,
        g8e_context=g8e_context,
    )

    logger.info(
        "[INTERNAL-HTTP] Direct command sent to operator",
        extra={"execution_id": g8e_context.execution_id}
    )

    return DirectCommandSentResponse(
        success=True,
        execution_id=g8e_context.execution_id,
    )


@router.get(InternalApiPaths.G8EE_CASE, response_model=CaseResponse)
async def get_case(
    case_id: str,
    case_service: CaseDataService = Depends(get_g8ee_case_data_service),
    g8e_context: G8eHttpContext = Depends(get_g8e_http_context)
):
    """Get a case by ID - internal cluster use only."""
    case = await case_service.get_case(case_id)
    return CaseResponse(success=True, case=case)


@router.patch(InternalApiPaths.G8EE_CASE, response_model=CaseResponse)
async def update_case(
    case_id: str,
    updates: CaseUpdateRequest,
    case_service: CaseDataService = Depends(get_g8ee_case_data_service),
    g8e_context: G8eHttpContext = Depends(get_g8e_http_context)
):
    """Update a case - internal cluster use only."""
    case = await case_service.update_case(case_id, updates)
    if g8e_context.web_session_id:
        await case_service.publish_case_update_sse(
            case_id=case_id,
            web_session_id=g8e_context.web_session_id,
            payload=CaseEventPayload(
                updated_at=case.updated_at,
                title=case.title,
                status=case.status,
                priority=case.priority,
                severity=case.severity,
            ),
            user_id=g8e_context.user_id,
        )
    return CaseResponse(success=True, case=case)


@router.delete(InternalApiPaths.G8EE_CASE, status_code=status.HTTP_204_NO_CONTENT)
async def delete_case(
    case_id: str,
    case_service: CaseDataService = Depends(get_g8ee_case_data_service),
    investigation_service: InvestigationService = Depends(get_g8ee_investigation_service),
    cache_aside_service: CacheAsideService = Depends(get_g8ee_cache_aside_service),
    g8e_context: G8eHttpContext = Depends(get_g8e_http_context)
):
    """
    Delete a case and all related data - internal cluster use only.
    
    Deletes:
    - Case document
    - All investigations with this case_id
    - All memories with this case_id
    """
    logger.info(
        "[INTERNAL-HTTP] Deleting case and related data",
        extra={"case_id": case_id}
    )

    try:
        case = await case_service.get_case(case_id)
        user_id = case.user_id
    except ResourceNotFoundError:
        logger.info(
            "[INTERNAL-HTTP] Case already deleted (idempotent)",
            extra={"case_id": case_id}
        )
        return

    # Delete all investigations for this case - SCOPED BY USER for security
    investigations = await investigation_service.investigation_data_service.get_case_investigations(
        case_id=case_id,
        user_id=user_id
    )
    for investigation in investigations:
        logger.info(
            "[INTERNAL-HTTP] Deleting investigation",
            extra={"investigation_id": investigation.id, "case_id": case_id}
        )
        await investigation_service.investigation_data_service.delete_investigation(investigation.id)

    logger.info(
        "[INTERNAL-HTTP] Deleted investigations",
        extra={"case_id": case_id, "count": len(investigations)}
    )

    # Delete all memories for this case (scoped to user for tenant isolation)
    memory_docs = await cache_aside_service.query_documents(
        collection=DB_COLLECTION_MEMORIES,
        field_filters=[
            FieldFilter(field="user_id", op="==", value=user_id).model_dump(mode="json"),
            FieldFilter(field="case_id", op="==", value=case_id).model_dump(mode="json"),
        ],
    )

    if memory_docs:
        for memory_doc in memory_docs:
            memory_id = memory_doc.get("investigation_id")
            if memory_id:
                await cache_aside_service.delete_document(
                    collection=DB_COLLECTION_MEMORIES,
                    document_id=memory_id
                )
                logger.info(
                    "[INTERNAL-HTTP] Deleted memory",
                    extra={"memory_id": memory_id, "case_id": case_id}
                )

    if memory_docs:
        logger.info(
            "[INTERNAL-HTTP] Deleted memories",
            extra={"case_id": case_id, "count": len(memory_docs)}
        )

    # Finally delete the case
    await case_service.delete_case(case_id)

    logger.info(
        "[INTERNAL-HTTP] Case and all related data deleted successfully",
        extra={"case_id": case_id}
    )



@router.post(InternalApiPaths.G8EE_OPERATORS_TERMINATE, response_model=OperatorTerminateResponse)
async def terminate_operator(
    request: OperatorTerminateRequest,
    operator_lifecycle_service: "OperatorLifecycleService" = Depends(get_g8ee_operator_lifecycle_service),
    g8e_context: G8eHttpContext = Depends(get_g8e_http_context),
):
    """
    Terminate an operator slot.
    
    Atomically marks operator status TERMINATED and appends a TERMINATED audit
    history entry under a single per-operator lock so concurrent writes cannot
    interleave a partial termination.

    Called by g8ed during API key refresh and manual termination.
    SECURITY: Internal only - g8ed component.
    """
    operator = await operator_lifecycle_service.operator_data_service.get_operator(request.operator_id)
    if not operator:
        return OperatorTerminateResponse(success=False, error="Operator not found")

    await operator_lifecycle_service.terminate_operator(
        operator_id=request.operator_id,
        actor=ComponentName.G8ED,
        summary="Operator terminated via g8ed relay",
    )

    logger.info(
        "[INTERNAL-HTTP] Operator terminated",
        extra={"operator_id": request.operator_id, "user_id": operator.user_id}
    )

    return OperatorTerminateResponse(success=True)


@router.post(InternalApiPaths.G8EE_OPERATORS_LISTEN_SESSION_AUTH)
async def listen_session_auth(
    request: OperatorListenSessionAuthRequest,
    session_auth_listener: SessionAuthListener = Depends(get_g8ee_session_auth_listener),
):
    """
    Start a session auth listener on PubSub.
    """
    try:
        await session_auth_listener.listen(
            operator_session_id=request.operator_session_id,
            operator_id=request.operator_id,
            user_id=request.user_id,
            organization_id=request.organization_id
        )
        return {"success": True}
    except Exception as e:
        logger.error(f"[INTERNAL-HTTP] Failed to start session auth listener: {e}")
        return {"success": False, "error": "Failed to start session auth listener"}


@router.post(InternalApiPaths.G8EE_OPERATORS_CREATE_SLOT, response_model=OperatorSlotCreationResponse)
async def create_operator_slot(
    request: OperatorSlotCreationRequest,
    operator_data_service: "OperatorDataService" = Depends(get_g8ee_operator_data_service),
    settings_service: SettingsService = Depends(get_g8ee_settings_service_write),
    api_key_service: ApiKeyService = Depends(get_g8ee_api_key_service),
    g8e_context: G8eHttpContext = Depends(get_g8e_http_context),
):
    """
    Create an operator slot.

    Called by g8ed during user initialization and device link creation.
    g8ee handles the actual write to the operator document to enforce the
    architectural boundary: after auth, g8ed has no business writing to operators.
    SECURITY: Internal only - g8ed component.
    """
    from app.models.operators import OperatorDocument
    from app.utils.timestamp import now
    import uuid
    import secrets

    try:
        operator_id = str(uuid.uuid4())

        # Generate API key (authority: g8ee for operator bootstrap)
        operator_suffix = operator_id.split('-')[-1][:8]
        random_token = secrets.token_hex(32)
        api_key = f"g8e_{operator_suffix}_{random_token}"

        # Create operator document
        operator_doc = OperatorDocument(
            id=operator_id,
            user_id=request.user_id,
            organization_id=request.organization_id,
            name=f"{request.name_prefix}-{request.slot_number}",
            slot_number=request.slot_number,
            operator_type=request.operator_type,
            cloud_subtype=request.cloud_subtype,
            is_g8ep=request.is_g8e_node,
            status=OperatorStatus.AVAILABLE,
            api_key=api_key,
            created_at=now(),
            updated_at=now(),
        )

        await operator_data_service.create_operator(operator_doc)

        # Issue API key to api_keys collection (canonical) and, if this is a
        # g8ep operator, mirror to platform_settings in a single coordinated
        # call. The coordinator rolls back the api_keys entry if the mirror
        # write fails so we never leave authoritative-without-mirror state.
        key_issued = await api_key_service.issue_operator_key(
            api_key=api_key,
            user_id=request.user_id,
            organization_id=request.organization_id,
            operator_id=operator_id,
            is_g8ep=request.is_g8e_node,
            settings_service=settings_service,
            client_name="operator",
            permissions=["OPERATOR_BOOTSTRAP", "OPERATOR_HEARTBEAT", "OPERATOR_DOWNLOAD"],
        )

        if not key_issued:
            logger.error(
                "[INTERNAL-HTTP] Failed to issue API key to api_keys collection",
                extra={"operator_id": operator_id, "user_id": request.user_id}
            )
            return OperatorSlotCreationResponse(
                success=False,
                operator_id=None,
                error="Failed to issue API key",
            )

        if request.is_g8e_node:
            logger.info(
                "[INTERNAL-HTTP] g8ep operator API key persisted to platform_settings",
                extra={"operator_id": operator_id, "user_id": request.user_id}
            )

        logger.info(
            "[INTERNAL-HTTP] Operator slot created",
            extra={
                "operator_id": operator_id,
                "user_id": request.user_id,
                "slot_number": request.slot_number,
                "is_g8ep": request.is_g8e_node,
            }
        )

        return OperatorSlotCreationResponse(
            success=True,
            operator_id=operator_id,
            api_key=api_key,
        )

    except Exception as e:
        logger.error(
            "[INTERNAL-HTTP] Failed to create operator slot",
            extra={"error": str(e), "user_id": request.user_id}
        )
        return OperatorSlotCreationResponse(
            success=False,
            operator_id=None,
            error=str(e),
        )


@router.post(InternalApiPaths.G8EE_OPERATORS_UPDATE_API_KEY, response_model=OperatorUpdateApiKeyResponse)
async def update_operator_api_key(
    request: OperatorUpdateApiKeyRequest,
    operator_data_service: "OperatorDataService" = Depends(get_g8ee_operator_data_service),
    settings_service: SettingsService = Depends(get_g8ee_settings_service_write),
    api_key_service: ApiKeyService = Depends(get_g8ee_api_key_service),
    g8e_context: G8eHttpContext = Depends(get_g8e_http_context),
):
    """
    Update an operator's API key.

    Called by g8ed during initialization to issue API keys for existing slots
    that were created without keys during setup.
    g8ee handles the actual write to the operator document to enforce the
    architectural boundary: after auth, g8ed has no business writing to operators.
    SECURITY: Internal only - g8ed component.
    """
    from app.models.operators import OperatorDocument
    from app.utils.timestamp import now

    try:
        operator = await operator_data_service.get_operator(request.operator_id)
        if not operator:
            logger.error(
                "[INTERNAL-HTTP] Operator not found for API key update",
                extra={"operator_id": request.operator_id}
            )
            return OperatorUpdateApiKeyResponse(success=False, error="Operator not found")

        # Rotate the API key in the canonical store (and the platform_settings
        # mirror if g8ep) BEFORE updating the operator doc. Failure here means
        # the operator doc is left untouched and the old key remains
        # authoritative — no phantom keys, no split-brain.
        rotated = await api_key_service.rotate_operator_key(
            old_api_key=operator.api_key,
            new_api_key=request.api_key,
            user_id=operator.user_id,
            organization_id=operator.organization_id,
            operator_id=operator.id,
            is_g8ep=operator.is_g8ep,
            settings_service=settings_service,
            permissions=["OPERATOR_BOOTSTRAP", "OPERATOR_HEARTBEAT", "OPERATOR_DOWNLOAD"],
        )
        if not rotated:
            logger.error(
                "[INTERNAL-HTTP] Failed to rotate operator API key",
                extra={"operator_id": request.operator_id}
            )
            return OperatorUpdateApiKeyResponse(success=False, error="Failed to rotate API key")

        updated_operator = operator.model_copy(update={
            "api_key": request.api_key,
            "updated_at": now(),
        })

        await operator_data_service.update_operator(updated_operator)

        if operator.is_g8ep:
            logger.info(
                "[INTERNAL-HTTP] g8ep operator API key updated in platform_settings",
                extra={"operator_id": request.operator_id}
            )

        logger.info(
            "[INTERNAL-HTTP] Operator API key updated",
            extra={"operator_id": request.operator_id}
        )

        return OperatorUpdateApiKeyResponse(success=True)

    except Exception as e:
        logger.error(
            "[INTERNAL-HTTP] Failed to update operator API key",
            extra={"error": str(e), "operator_id": request.operator_id}
        )
        return OperatorUpdateApiKeyResponse(success=False, error=str(e))


@router.post(InternalApiPaths.G8EE_AUTH_GENERATE_KEY, response_model=ApiKeyGenerationResponse)
async def generate_api_key(
    request: ApiKeyGenerationRequest,
    api_key_service: ApiKeyService = Depends(get_g8ee_api_key_service),
):
    """Generate a new API key.
    
    Authority: g8ee.
    SECURITY: Internal only - g8ed component.
    """
    try:
        api_key = api_key_service.generate_raw_key(prefix=request.prefix)
        return ApiKeyGenerationResponse(
            success=True,
            api_key=api_key
        )
    except Exception as e:
        logger.error(f"[INTERNAL-HTTP] Failed to generate API key: {str(e)}")
        return ApiKeyGenerationResponse(
            success=False,
            error=str(e)
        )


@router.post(InternalApiPaths.G8EE_AUTH_REVOKE_CERT, response_model=OperatorCertificateRevokeResponse)
async def revoke_operator_certificate(
    request: OperatorCertificateRevokeRequest,
    certificate_service: CertificateService = Depends(get_g8ee_certificate_service),
):
    """Revoke an operator certificate.
    
    Authority: g8ee.
    SECURITY: Internal only - g8ed component.
    """
    try:
        success = await certificate_service.revoke_certificate(
            serial=request.serial,
            reason=request.reason,
            operator_id=request.operator_id
        )
        return OperatorCertificateRevokeResponse(success=success)
    except Exception as e:
        logger.error(f"[INTERNAL-HTTP] Failed to revoke certificate: {str(e)}")
        return OperatorCertificateRevokeResponse(success=False, error=str(e))


@router.post(InternalApiPaths.G8EE_OPERATORS_CLAIM_SLOT, response_model=OperatorSlotClaimResponse)
async def claim_operator_slot(
    request: OperatorSlotClaimRequest,
    operator_lifecycle_service: "OperatorLifecycleService" = Depends(get_g8ee_operator_lifecycle_service),
    g8e_context: G8eHttpContext = Depends(get_g8e_http_context),
):
    """
    Claim an operator slot for device registration.

    Called by g8ed during device registration.
    g8ee handles the actual write to the operator document to enforce the
    architectural boundary: after auth, g8ed has no business writing to operators.
    SECURITY: Internal only - g8ed component.
    """
    from app.models.operators import OperatorDocument
    from app.utils.timestamp import now

    try:
        success = await operator_lifecycle_service.claim_operator_slot(
            operator_id=request.operator_id,
            operator_session_id=request.operator_session_id,
            bound_web_session_id=request.bound_web_session_id,
            system_info=request.system_info,
            operator_type=request.operator_type,
        )

        if not success:
            return OperatorSlotClaimResponse(
                success=False,
                error="Failed to claim operator slot",
            )

        logger.info(
            "[INTERNAL-HTTP] Operator slot claimed",
            extra={
                "operator_id": request.operator_id,
                "operator_session_id": request.operator_session_id[:12] + "...",
            }
        )

        return OperatorSlotClaimResponse(success=True)

    except Exception as e:
        logger.error(
            "[INTERNAL-HTTP] Failed to claim operator slot",
            extra={"error": str(e), "operator_id": request.operator_id}
        )
        return OperatorSlotClaimResponse(
            success=False,
            error=str(e),
        )


@router.post(InternalApiPaths.G8EE_OPERATORS_BIND, response_model=OperatorBindResponse)
async def bind_operators(
    request: OperatorBindRequest,
    operator_data_service: "OperatorDataService" = Depends(get_g8ee_operator_data_service),
    g8e_context: G8eHttpContext = Depends(get_g8e_http_context),
):
    """
    Bind operators to a web session.

    Called by g8ed during operator bind operations.
    g8ee handles the actual write to the operator document to enforce the
    architectural boundary: after auth, g8ed has no business writing to operators.
    SECURITY: Internal only - g8ed component.
    """
    from app.utils.timestamp import now

    bound = []
    failed = []
    errors = []

    for operator_id in request.operator_ids:
        try:
            operator = await operator_data_service.get_operator(operator_id)
            if not operator:
                failed.append(operator_id)
                errors.append({"operator_id": operator_id, "error": "Operator not found"})
                continue

            if operator.user_id != request.user_id:
                failed.append(operator_id)
                errors.append({"operator_id": operator_id, "error": "Unauthorized"})
                continue

            # Update operator status to BOUND
            update_data = {
                "status": OperatorStatus.BOUND,
                "bound_web_session_id": request.web_session_id,
                "updated_at": now(),
            }

            result = await operator_data_service.cache.update_document(
                collection=operator_data_service.collection,
                document_id=operator_id,
                data=update_data,
                merge=True,
            )

            if result.success:
                bound.append(operator_id)
            else:
                failed.append(operator_id)
                errors.append({"operator_id": operator_id, "error": result.error or "Failed to update operator"})

        except Exception as e:
            failed.append(operator_id)
            errors.append({"operator_id": operator_id, "error": str(e)})

    logger.info(
        "[INTERNAL-HTTP] Operators bound",
        extra={
            "bound_count": len(bound),
            "failed_count": len(failed),
            "user_id": request.user_id,
        }
    )

    return OperatorBindResponse(
        success=len(bound) > 0,
        bound_count=len(bound),
        failed_count=len(failed),
        bound_operator_ids=bound,
        failed_operator_ids=failed,
        errors=errors,
    )


@router.post(InternalApiPaths.G8EE_OPERATORS_UNBIND, response_model=OperatorUnbindResponse)
async def unbind_operators(
    request: OperatorUnbindRequest,
    operator_data_service: "OperatorDataService" = Depends(get_g8ee_operator_data_service),
    g8e_context: G8eHttpContext = Depends(get_g8e_http_context),
):
    """
    Unbind operators from a web session.

    Called by g8ed during operator unbind operations.
    g8ee handles the actual write to the operator document to enforce the
    architectural boundary: after auth, g8ed has no business writing to operators.
    SECURITY: Internal only - g8ed component.
    """
    from app.utils.timestamp import now

    unbound = []
    failed = []
    errors = []

    for operator_id in request.operator_ids:
        try:
            operator = await operator_data_service.get_operator(operator_id)
            if not operator:
                failed.append(operator_id)
                errors.append({"operator_id": operator_id, "error": "Operator not found"})
                continue

            if operator.user_id != request.user_id:
                failed.append(operator_id)
                errors.append({"operator_id": operator_id, "error": "Unauthorized"})
                continue

            # Update operator status to ACTIVE
            update_data = {
                "status": OperatorStatus.ACTIVE,
                "bound_web_session_id": None,
                "updated_at": now(),
            }

            result = await operator_data_service.cache.update_document(
                collection=operator_data_service.collection,
                document_id=operator_id,
                data=update_data,
                merge=True,
            )

            if result.success:
                unbound.append(operator_id)
            else:
                failed.append(operator_id)
                errors.append({"operator_id": operator_id, "error": result.error or "Failed to update operator"})

        except Exception as e:
            failed.append(operator_id)
            errors.append({"operator_id": operator_id, "error": str(e)})

    logger.info(
        "[INTERNAL-HTTP] Operators unbound",
        extra={
            "unbound_count": len(unbound),
            "failed_count": len(failed),
            "user_id": request.user_id,
        }
    )

    return OperatorUnbindResponse(
        success=len(unbound) > 0,
        unbound_count=len(unbound),
        failed_count=len(failed),
        unbound_operator_ids=unbound,
        failed_operator_ids=failed,
        errors=errors,
    )


@router.post(InternalApiPaths.G8EE_OPERATORS_AUTHENTICATE, response_model=OperatorAuthenticateResponse)
async def authenticate_operator(
    request: InternalOperatorAuthCall,
    operator_auth_service: OperatorAuthService = Depends(get_g8ee_operator_auth_service),
    http_request: Request = None,
):
    """
    Authenticate an operator via API key (Bearer).
    Called by g8ed (as proxy) or directly via internal API.
    """
    request_context = {
        "ip": http_request.client.host if http_request else None,
        "user_agent": http_request.headers.get("user-agent") if http_request else None,
    }

    result = await operator_auth_service.authenticate_operator(
        authorization_header=request.authorization,
        body=request.model_dump(),
        request_context=request_context
    )

    if result.get("success"):
        return OperatorAuthenticateResponse(**result)
    else:
        return OperatorAuthenticateResponse(
            success=False,
            error=result.get("error")
        )


@router.post(InternalApiPaths.G8EE_OPERATORS_DEVICE_LINK_REGISTER, response_model=OperatorDeviceLinkRegisterResponse)
async def register_device_link_operator(
    request: OperatorDeviceLinkRegisterRequest,
    operator_auth_service: OperatorAuthService = Depends(get_g8ee_operator_auth_service),
    http_request: Request = None,
) -> OperatorDeviceLinkRegisterResponse:
    """Bootstrap an operator after device-link consumption (g8ed-internal)."""
    result = await operator_auth_service.register_device_link_operator(
        operator_id=request.operator_id,
        user_id=request.user_id,
        organization_id=request.organization_id,
        operator_type=request.operator_type,
        system_info=request.system_info,
        request_context={
            "ip": http_request.client.host if http_request.client else None,
            "user_agent": http_request.headers.get("user-agent"),
        },
    )
    if result.get("success"):
        return OperatorDeviceLinkRegisterResponse(**result)
    return OperatorDeviceLinkRegisterResponse(success=False, error=result.get("error"))


@router.post(InternalApiPaths.G8EE_OPERATORS_VALIDATE_SESSION, response_model=OperatorSessionValidateResponse)
async def validate_operator_session(
    request: OperatorSessionValidateRequest,
    session_service: OperatorSessionService = Depends(get_g8ee_operator_session_service),
):
    """
    Validate an operator session.
    """
    try:
        session = await session_service.validate_session(request.operator_session_id)
        if session:
            return OperatorSessionValidateResponse(
                success=True,
                valid=True,
                user_id=session.user_id,
                operator_id=session.operator_id,
                session_type=session.session_type.value
            )
        else:
            return OperatorSessionValidateResponse(success=True, valid=False)
    except Exception as e:
        logger.error(f"[INTERNAL-HTTP] Session validation failed: {e}")
        return OperatorSessionValidateResponse(success=False, valid=False, error=str(e))


@router.post(InternalApiPaths.G8EE_OPERATORS_REFRESH_SESSION, response_model=OperatorSessionRefreshResponse)
async def refresh_operator_session(
    request: OperatorSessionRefreshRequest,
    session_service: OperatorSessionService = Depends(get_g8ee_operator_session_service),
):
    """
    Refresh an operator session.
    """
    try:
        success = await session_service.refresh_session(request.operator_session_id)
        if success:
            session = await session_service.validate_session(request.operator_session_id)
            return OperatorSessionRefreshResponse(
                success=True,
                operator_id=session.operator_id if session else None,
                session={
                    "id": request.operator_session_id,
                    "expires_at": session.absolute_expires_at if session else None,
                }
            )
        else:
            return OperatorSessionRefreshResponse(success=False, error="Session not found or expired")
    except Exception as e:
        logger.error(f"[INTERNAL-HTTP] Session refresh failed: {e}")
        return OperatorSessionRefreshResponse(success=False, error=str(e))


@router.post(InternalApiPaths.G8EE_OPERATORS_REGISTER_SESSION, response_model=OperatorSessionRegisteredResponse)
async def register_operator_session(
    request: OperatorSessionRegistrationRequest,
    heartbeat_service: HeartbeatSnapshotService = Depends(get_g8ee_heartbeat_service),
    g8e_context: G8eHttpContext = Depends(get_g8e_http_context),
):
    """
    Subscribe g8ee to the heartbeat pub/sub channel for an operator session.

    Called by g8ed immediately after operator authentication succeeds so g8ee
    is listening before the first heartbeat arrives.
    SECURITY: Internal only - g8ed component.
    """
    await heartbeat_service.register_operator_session(
        operator_id=request.operator_id,
        operator_session_id=request.operator_session_id,
    )

    logger.info(
        "[INTERNAL-HTTP] Operator session registered for heartbeat subscription",
        extra={
            "operator_id": request.operator_id,
            "operator_session_id": request.operator_session_id[:12] + "...",
        },
    )

    return OperatorSessionRegisteredResponse(
        success=True,
        operator_id=request.operator_id,
        operator_session_id=request.operator_session_id,
    )


@router.post(InternalApiPaths.G8EE_OPERATORS_DEREGISTER_SESSION, response_model=OperatorSessionRegisteredResponse)
async def deregister_operator_session(
    request: OperatorSessionRegistrationRequest,
    heartbeat_service: HeartbeatSnapshotService = Depends(get_g8ee_heartbeat_service),
    g8e_context: G8eHttpContext = Depends(get_g8e_http_context),
):
    """
    Unsubscribe g8ee from the heartbeat pub/sub channel for an operator session.

    Called by g8ed when an operator goes offline, is stopped, or is terminated.
    SECURITY: Internal only - g8ed component.
    """
    await heartbeat_service.deregister_operator_session(
        operator_id=request.operator_id,
        operator_session_id=request.operator_session_id,
    )

    logger.info(
        "[INTERNAL-HTTP] Operator session deregistered from heartbeat subscription",
        extra={
            "operator_id": request.operator_id,
            "operator_session_id": request.operator_session_id[:12] + "...",
        },
    )

    return OperatorSessionRegisteredResponse(
        success=True,
        operator_id=request.operator_id,
        operator_session_id=request.operator_session_id,
    )


@router.post(InternalApiPaths.G8EE_OPERATORS_STOP, response_model=OperatorStoppedResponse)
async def stop_operator(
    request: StopOperatorRequest,
    g8e_context: G8eHttpContext = Depends(get_g8e_http_context),
    operator_command_service: OperatorCommandService = Depends(get_g8ee_operator_command_service)
):
    """
    Stop an Operator by sending shutdown command via g8es pub/sub.
    The Operator will receive the shutdown command and exit.
    """
    operator_id = request.operator_id
    operator_session_id = request.operator_session_id
    user_id = request.user_id

    logger.info(
        "[OPERATOR-STOP] Publishing shutdown command to operator",
        extra={
            "operator_id": operator_id,
            "operator_session_id": operator_session_id,
            "user_id": user_id,
            "web_session_id": g8e_context.web_session_id[:12] + "..." if g8e_context.web_session_id else None,
            "validated_user": g8e_context.user_id
        }
    )

    if operator_command_service.pubsub_client is None:
        raise ServiceUnavailableError("pub/sub client not initialized", component="g8ee")

    command_data = {
        "event_type": EventType.OPERATOR_SHUTDOWN_REQUESTED,
        "operator_id": operator_id,
        "user_id": user_id,
        "timestamp": now_iso(),
    }

    subscribers = await operator_command_service.pubsub_client.publish_command(
        operator_id=operator_id,
        operator_session_id=operator_session_id,
        command_data=command_data
    )

    logger.info(
        "[OPERATOR-STOP] Shutdown command published successfully",
        extra={
            "operator_id": operator_id,
            "subscribers": subscribers
        }
    )

    return OperatorStoppedResponse(
        success=True,
        operator_id=operator_id,
        subscribers=subscribers,
    )


@router.get(InternalApiPaths.G8EE_INVESTIGATIONS)
async def query_investigations(
    request: Request,
    investigation_service: InvestigationService = Depends(get_g8ee_investigation_service),
    g8e_context: G8eHttpContext = Depends(get_g8e_http_context)
):
    logger.info(
        "[INTERNAL-HTTP] Investigation query via G8eHttpContext",
        extra={"user_id": g8e_context.user_id, "source": g8e_context.source_component}
    )

    query_request = InvestigationQueryRequest(
        case_id=request.query_params.get("case_id"),
        task_id=request.query_params.get("task_id"),
        web_session_id=request.query_params.get("web_session_id"),
        user_id=g8e_context.user_id,
        status=request.query_params.get("status"),
        priority=request.query_params.get("priority"),
        limit=int(request.query_params.get("limit", 20)),
        order_by=request.query_params.get("order_by", "created_at"),
        order_direction=request.query_params.get("order_direction", "desc"),
    )

    investigations = await investigation_service.investigation_data_service.query_investigations(query_request)
    return investigations


@router.get(InternalApiPaths.G8EE_INVESTIGATION, response_model=InvestigationModel)
async def get_investigation(
    investigation_id: str,
    investigation_service: InvestigationService = Depends(get_g8ee_investigation_service),
    g8e_context: G8eHttpContext = Depends(get_g8e_http_context)
):
    """Get investigation by ID - internal cluster use only.
    
    SECURITY: Validates that the authenticated user owns the investigation.
    """
    logger.info(
        "[INTERNAL-HTTP] Get investigation via G8eHttpContext",
        extra={"user_id": g8e_context.user_id, "investigation_id": investigation_id}
    )

    investigation = await investigation_service.investigation_data_service.get_investigation(investigation_id)
    if not investigation:
        raise ResourceNotFoundError(
            f"Investigation {investigation_id} not found",
            resource_type="investigation",
            resource_id=investigation_id,
            component="g8ee"
        )

    if investigation.user_id != g8e_context.user_id:
        logger.warning(
            "AUTHORIZATION VIOLATION: User attempted to access another user's investigation",
            extra={
                "authenticated_user_id": g8e_context.user_id,
                "investigation_owner": investigation.user_id,
                "investigation_id": investigation_id
            }
        )
        raise ResourceNotFoundError(
            f"Investigation {investigation_id} not found",
            resource_type="investigation",
            resource_id=investigation_id,
            component="g8ee"
        )

    return investigation


@router.get(InternalApiPaths.G8EE_HEALTH)
async def health_check():
    """Health check for internal API"""
    return {
        "service": "g8ee-internal-api",
        "status": "healthy",
        "endpoints": [
            InternalApiPaths.G8EE_CHAT,
            InternalApiPaths.G8EE_CHAT_STOP,
            InternalApiPaths.G8EE_OPERATOR_APPROVAL_RESPOND,
            InternalApiPaths.G8EE_OPERATOR_DIRECT_COMMAND,
            InternalApiPaths.G8EE_CASES,
            InternalApiPaths.G8EE_CASE,
            InternalApiPaths.G8EE_INVESTIGATIONS,
            InternalApiPaths.G8EE_INVESTIGATION,
            InternalApiPaths.G8EE_CHAT_TRIAGE_ANSWER,
            InternalApiPaths.G8EE_CHAT_TRIAGE_SKIP,
            InternalApiPaths.G8EE_CHAT_TRIAGE_TIMEOUT,
        ]
    }
