# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

from __future__ import annotations

"""
Internal API Router for g8ee

Cluster-internal HTTP endpoints for direct communication from other g8e components.
NOT exposed via Ingress - only accessible from pods within the Kubernetes cluster.

Note: g8eo Operator commands still use PubSub (external agent communication).
"""

import asyncio
import logging
import uuid
import secrets
from fastapi import APIRouter, Depends, Request, status
from app.models.http_context import G8eHttpContext, RequestContext

from app.models.settings import G8eeAppSettings, G8eeUserSettings
from app.constants import (
    DB_COLLECTION_MEMORIES,
    EventType,
    G8EE_COMPONENT,
    HistoryActor,
    InternalAPIPaths,
    OperatorStatus,
    Priority,
)
from app.constants.collections import (
    DB_COLLECTION_SETTINGS,
    USER_SETTINGS_DOC_PREFIX,
)
from app.errors import ResourceNotFoundError, ServiceUnavailableError
from app.models import CaseCreateRequest
from app.models.cases import (
    CaseCreatedPayload,
    CaseEventPayload,
    CaseGetRequest,
    CaseUpdateRequest,
    CaseDeleteRequest,
)
from app.models.cache import FieldFilter
from app.models.pubsub_messages import G8eMessage
from app.models.internal_api import (
    APIKeyGenerationRequest,
    APIKeyGenerationResponse,
    ApprovalRespondedResponse,
    CaseResponse,
    ChatMessageRequest,
    ChatStartedResponse,
    EvaluationTraceResponse,
    DirectCommandRequest,
    DirectCommandSentResponse,
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
    OperatorUpdateAPIKeyRequest,
    OperatorUpdateAPIKeyResponse,
    PendingApprovalsResponse,
    StopAIRequest,
    StopAIResponse,
    StopOperatorRequest,
    UserSettingsUpdateResponse,
    SettingsGetRequest,
    SettingsSyncRequest,
)
from app.models.triage_api import (
    TriageAnswerRequest,
    TriageSkipRequest,
    TriageTimeoutRequest,
)
from app.models.investigations import (
    ConversationMessageMetadata,
    InvestigationModel,
    InvestigationQueryRequest,
    InvestigationUpdateRequest,
    InvestigationGetRequest,
)

InvestigationUpdateRequest.model_rebuild()
InvestigationQueryRequest.model_rebuild()
InvestigationGetRequest.model_rebuild()
from app.models.events import SessionEvent
from app.models.operators import (
    HeartbeatSnapshot,
    OperatorDocument,
    OperatorStatusUpdatedPayload,
)
from app.clients.gateway_operator_client import GatewayOperatorClient
from app.errors import NetworkError
from app.services.operator.operator_data_service import OperatorDataService
from app.services.data.case_data_service import CaseDataService
from app.services.data.attachment_store_service import AttachmentService
from app.services.investigation.investigation_service import InvestigationService
from app.services.ai.chat_pipeline import ChatPipelineService
from app.services.ai.chat_task_manager import BackgroundTaskManager
from app.services.ai.title_generator import generate_case_title
from app.services.infra.event_service import EventService
from app.services.cache.cache_aside import CacheAsideService
from app.services.auth.api_key_service import APIKeyService
from app.services.auth.certificate_service import CertificateService
from app.services.infra.settings_service import SettingsService
from app.utils.timestamp import now
from app.constants.message_sender import MessageSender

_GATEWAY_OPERATOR_AUTHORITY_ERROR = (
    "Operator auth and session authority are Gateway-owned; use gateway enrollment "
    "and POST /api/v1/operators/reauth instead of g8ee local services."
)

from app.services.evaluation.trace_service import EvaluationTraceService, validated_trace_ids
from app.dependencies import (
    get_g8ee_app_settings,
    get_g8ee_approval_service,
    get_g8ee_attachment_service,
    get_g8ee_cache_aside_service,
    get_g8ee_case_data_service,
    get_g8ee_chat_pipeline,
    get_g8ee_chat_task_manager,
    get_g8ee_event_service,
    get_g8ee_investigation_service,
    get_g8ee_operator_command_service,
    get_g8ee_operator_data_service,
    get_g8ee_gateway_operator_client,
    get_g8ee_api_key_service,
    get_g8ee_certificate_service,
    get_g8ee_settings_service,
    get_g8ee_settings_service_write,
    get_g8ee_user_settings,
    get_request_context,
    require_authenticated_context,
)

logger = logging.getLogger(__name__)

router = APIRouter(tags=["internal"])
_background_tasks: set[asyncio.Task] = set()


def _status_payload_from_gateway_doc(
    operator_doc: dict[str, object], status: OperatorStatus
) -> OperatorStatusUpdatedPayload:
    snapshot_raw = operator_doc.get("latest_heartbeat_snapshot")
    snapshot: HeartbeatSnapshot | None = None
    hostname = operator_doc.get("current_hostname")
    if isinstance(snapshot_raw, dict):
        try:
            snapshot = HeartbeatSnapshot.model_validate(snapshot_raw)
            if snapshot.system_identity and snapshot.system_identity.hostname:
                hostname = snapshot.system_identity.hostname
        except Exception:
            snapshot = None
    return OperatorStatusUpdatedPayload(
        operator_id=str(operator_doc.get("id", "")),
        status=status,
        name=operator_doc.get("name") if isinstance(operator_doc.get("name"), str) else None,
        hostname=hostname if isinstance(hostname, str) else None,
        system_fingerprint=snapshot.system_fingerprint if snapshot else None,
        metrics=snapshot,
    )


async def _publish_gateway_operator_status_events(
    gateway_operator_client: GatewayOperatorClient,
    event_service: EventService,
    g8e_context: G8eHttpContext,
    operator_ids: list[str],
    status: OperatorStatus,
    event_type: EventType,
) -> None:
    if not operator_ids:
        return
    try:
        operators = await gateway_operator_client.list(user_id=g8e_context.user_id)
        by_id = {
            str(op.get("id")): op for op in operators if isinstance(op, dict) and op.get("id")
        }
        for operator_id in operator_ids:
            operator_doc = by_id.get(operator_id)
            if not operator_doc:
                continue
            await event_service.publish(
                SessionEvent.from_context(
                    context=g8e_context,
                    event_type=event_type,
                    payload=_status_payload_from_gateway_doc(operator_doc, status),
                )
            )
    except Exception as exc:
        logger.warning(
            "[INTERNAL-HTTP] Failed to publish operator status events from gateway docs: %s",
            exc,
        )


def _per_operator_errors(
    failed_operator_ids: list[str], message: str | None
) -> list[dict[str, str]]:
    error_message = message or "Gateway operator request failed"
    return [{"operator_id": operator_id, "error": error_message} for operator_id in failed_operator_ids]


async def _generate_and_update_title(
    message: str,
    case_id: str,
    investigation_id: str,
    context: RequestContext,
    user_settings: G8eeUserSettings,
    case_service: CaseDataService,
    investigation_service: InvestigationService,
):
    try:
        case_result = await generate_case_title(message, settings=user_settings)
        ai_title = case_result.generated_title

        context = context.model_copy(
            update={"case_id": case_id, "investigation_id": investigation_id}
        )

        updated_case = await case_service.update_case(
            case_id, CaseUpdateRequest(context=context, title=ai_title)
        )
        await investigation_service.update_investigation(
            investigation_id, InvestigationUpdateRequest(context=context, case_title=ai_title)
        )
        if context.web_session_id:
            await case_service.publish_case_update_sse(
                case_id=case_id,
                web_session_id=context.web_session_id,
                payload=CaseEventPayload(
                    updated_at=updated_case.updated_at,
                    title=ai_title,
                ),
                user_id=context.user_id,
            )
    except Exception as e:
        logger.error(
            "[INTERNAL-HTTP] Failed to generate case title in background task",
            extra={"case_id": case_id, "error": str(e)},
            exc_info=True,
        )


@router.post(InternalAPIPaths.G8EE_CHAT, response_model=ChatStartedResponse)
async def internal_chat(
    request: ChatMessageRequest,
    app_settings: G8eeAppSettings = Depends(get_g8ee_app_settings),
    user_settings: G8eeUserSettings = Depends(get_g8ee_user_settings),
    chat_pipeline: ChatPipelineService = Depends(get_g8ee_chat_pipeline),
    chat_task_manager: BackgroundTaskManager = Depends(get_g8ee_chat_task_manager),
    case_service: CaseDataService = Depends(get_g8ee_case_data_service),
    investigation_service: InvestigationService = Depends(get_g8ee_investigation_service),
    attachment_service: AttachmentService = Depends(get_g8ee_attachment_service),
    event_service: EventService = Depends(get_g8ee_event_service),
    g8e_context: G8eHttpContext = Depends(require_authenticated_context),
    settings_service: SettingsService = Depends(get_g8ee_settings_service_write),
):
    """
    Non-streaming chat endpoint - default path for browser sessions.

    Creates case + investigation inline when case_id is absent, then fires
    run_chat as a background task. The AI response and all tool events are
    delivered to the browser via the existing SSE connection; this endpoint
    returns immediately with case/investigation IDs so the browser can update
    its state without waiting for the LLM.

    Context is extracted from request body (RequestContext) instead of headers,
    eliminating the fragile header-as-state pattern.
    """
    if request.evaluation_context is not None:
        g8e_context = g8e_context.model_copy(
            update={"evaluation_context": request.evaluation_context}
        )
        chat_pipeline.evaluation_trace_service.begin(g8e_context)

    # Fail-fast if no LLM models are configured
    chat_pipeline.validate_llm_config(
        user_settings=user_settings,
        primary_model_override=request.llm_primary_model,
        assistant_model_override=request.llm_assistant_model,
        lite_model_override=request.llm_lite_model,
        primary_provider_override=request.llm_primary_provider,
        assistant_provider_override=request.llm_assistant_provider,
        lite_provider_override=request.llm_lite_provider,
        primary_api_key_override=request.llm_primary_api_key,
        primary_endpoint_override=request.llm_primary_endpoint,
        assistant_api_key_override=request.llm_assistant_api_key,
        assistant_endpoint_override=request.llm_assistant_endpoint,
        lite_api_key_override=request.llm_lite_api_key,
        lite_endpoint_override=request.llm_lite_endpoint,
    )

    resource_creation = request.resource_creation
    create_new_case = resource_creation.create_case if resource_creation else False

    # Validate investigation_id exists before proceeding, UNLESS we are creating a new case
    if not create_new_case:
        if not g8e_context.investigation_id:
            logger.error(
                "[INTERNAL-HTTP] Cannot start chat - investigation_id is missing",
                extra={
                    "case_id": g8e_context.case_id,
                    "web_session_id": (g8e_context.web_session_id[:8] + "...")
                    if g8e_context.web_session_id
                    else None,
                },
            )
            return ChatStartedResponse(
                success=False,
                case_id=g8e_context.case_id or "",
                investigation_id=g8e_context.investigation_id or "",
            )

    logger.info(
        "[INTERNAL-HTTP] Non-streaming chat request received",
        extra={
            "case_id": g8e_context.case_id,
            "investigation_id": g8e_context.investigation_id,
            "create_new_case": create_new_case,
            "web_session_id": (g8e_context.web_session_id[:8] + "...")
            if g8e_context.web_session_id
            else None,
            "message_length": len(request.message),
        },
    )

    if create_new_case:
        case_create_data = CaseCreateRequest(
            initial_message=request.message,
            attachments=request.attachments or [],
            sentinel_mode=request.sentinel_mode,
            user_id=g8e_context.user_id,
            web_session_id=g8e_context.web_session_id,
            organization_id=g8e_context.organization_id,
            operator_id=g8e_context.operator_id,
            operator_session_id=g8e_context.operator_session_id,
        )
        case = await case_service.create_case(case_create_data, generated_title=None)

        from app.models.investigations import InvestigationCreateRequest

        investigation_request = InvestigationCreateRequest(
            case_id=case.id,
            case_title=case.title,
            case_description=case.description,
            web_session_id=g8e_context.web_session_id,
            priority=Priority(case.priority) if isinstance(case.priority, str) else case.priority,
            user_email=case.user_email,
            user_id=case.user_id,
            operator_id=g8e_context.operator_id,
            operator_session_id=g8e_context.operator_session_id,
            sentinel_mode=request.sentinel_mode,
            created_with_case=True,
            case_source=case.source,
        )
        investigation = await investigation_service.create_investigation(investigation_request)

        g8e_context = g8e_context.model_copy(
            update={
                "case_id": case.id,
                "investigation_id": investigation.id,
            }
        )

        from app.models.events import SessionEvent

        # Publish CASE_CREATED event immediately after inline creation.
        try:
            await event_service.publish(
                SessionEvent.from_context(
                    context=g8e_context,
                    event_type=EventType.APP_CASE_CREATED,
                    payload=CaseCreatedPayload(title=case.title),
                )
            )
        except Exception as sse_err:
            logger.warning(
                "[INTERNAL-HTTP] Failed to publish g8e.v1.app.case.created SSE - case created successfully, continuing",
                extra={"case_id": g8e_context.case_id, "error": str(sse_err)},
            )

        if request.message.strip():
            task = asyncio.create_task(
                _generate_and_update_title(
                    message=request.message,
                    case_id=g8e_context.case_id,
                    investigation_id=g8e_context.investigation_id,
                    context=RequestContext.from_app_context(g8e_context),
                    user_settings=user_settings,
                    case_service=case_service,
                    investigation_service=investigation_service,
                )
            )
            # Track background task for cleanup
            task_id = f"title_generation_{g8e_context.investigation_id}"
            _t = asyncio.create_task(
                chat_task_manager.track(task_id, task, auto_cancel_previous=False)
            )
            _background_tasks.add(_t)
            _t.add_done_callback(_background_tasks.discard)

        logger.info(
            "[INTERNAL-HTTP] New conversation created inline",
            extra={
                "case_id": g8e_context.case_id,
                "investigation_id": g8e_context.investigation_id,
            },
        )

    resolved_attachments = []
    if request.attachments:
        try:
            raw_attachments = await attachment_service.get_attachments_by_metadata(
                request.attachments
            )
            resolved_attachments = await attachment_service.process_attachments(raw_attachments)
        except Exception as att_err:
            logger.error("[INTERNAL-HTTP] Failed to retrieve attachments: %s", att_err)

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
            llm_lite_model=request.llm_lite_model,
            llm_primary_api_key=request.llm_primary_api_key,
            llm_primary_endpoint=request.llm_primary_endpoint,
            llm_assistant_api_key=request.llm_assistant_api_key,
            llm_assistant_endpoint=request.llm_assistant_endpoint,
            llm_lite_api_key=request.llm_lite_api_key,
            llm_lite_endpoint=request.llm_lite_endpoint,
            _task_manager=chat_task_manager,
            user_settings=user_settings,
        )
    )
    # Track the task - run_chat will also track it internally, but we track it here
    # to ensure it's in the registry before we return
    _t = asyncio.create_task(
        chat_task_manager.track(g8e_context.investigation_id, chat_task, auto_cancel_previous=False)
    )
    _background_tasks.add(_t)
    _t.add_done_callback(_background_tasks.discard)

    return ChatStartedResponse(
        success=True,
        case_id=g8e_context.case_id,
        investigation_id=g8e_context.investigation_id,
    )


@router.post(InternalAPIPaths.G8EE_CHAT_TRIAGE_ANSWER)
async def internal_triage_answer(
    request: TriageAnswerRequest,
    investigation_service: InvestigationService = Depends(get_g8ee_investigation_service),
    chat_pipeline: ChatPipelineService = Depends(get_g8ee_chat_pipeline),
    chat_task_manager: BackgroundTaskManager = Depends(get_g8ee_chat_task_manager),
    settings_service: SettingsService = Depends(get_g8ee_settings_service),
    g8e_context: G8eHttpContext = Depends(require_authenticated_context),
):
    """
    Receive user answer to a triage clarifying question - internal cluster use only.

    Context is extracted from request body (RequestContext) instead of headers,
    eliminating the fragile header-as-state pattern.
    """
    # Fetch user settings manually using user_id from context to eliminate header dependency
    user_settings = await settings_service.get_user_settings(g8e_context.user_id)

    # Fail-fast if no LLM models are configured
    chat_pipeline.validate_llm_config(
        user_settings=user_settings,
        primary_model_override=None,
        assistant_model_override=None,
        lite_model_override=None,
    )

    logger.info(
        "[INTERNAL-HTTP] Triage answer received",
        extra={
            "investigation_id": g8e_context.investigation_id,
            "question_index": request.question_index,
            "answer": request.answer,
            "user_id": g8e_context.user_id,
        },
    )

    investigation = await investigation_service.get_investigation(g8e_context.investigation_id)
    if not investigation:
        raise ResourceNotFoundError(
            "Investigation not found",
            resource_id=g8e_context.investigation_id,
            resource_type="investigation",
            component="g8ee",
        )

    # Store answer as user.chat message with structured metadata
    answer_text = f"Answered clarifying question {request.question_index}: {'Yes' if request.answer else 'No'}"
    await investigation_service.investigation_data_service.add_chat_message(
        investigation_id=g8e_context.investigation_id,
        sender=MessageSender.USER_CHAT,
        content=answer_text,
        metadata=ConversationMessageMetadata(
            event_type=EventType.AI_TRIAGE_CLARIFICATION_ANSWERED,
            question_index=request.question_index,
            answer=request.answer,
        ),
    )

    # Trigger AI response by calling run_chat.
    # ChatPipeline.run_chat internally tracks the task via ChatTaskManager,
    # which will cancel any existing active task for this investigation_id.
    await chat_pipeline.run_chat(
        message=answer_text,
        g8e_context=g8e_context,
        attachments=[],
        sentinel_mode=investigation.sentinel_mode,
        llm_primary_provider=None,
        llm_assistant_provider=None,
        llm_lite_provider=None,
        llm_primary_model=user_settings.llm.primary_model,
        llm_assistant_model=user_settings.llm.resolved_assistant_model,
        llm_lite_model=None,
        _task_manager=chat_task_manager,
        user_settings=user_settings,
    )

    return {"success": True}


@router.post(InternalAPIPaths.G8EE_CHAT_TRIAGE_SKIP)
async def internal_triage_skip(
    request: TriageSkipRequest,
    investigation_service: InvestigationService = Depends(get_g8ee_investigation_service),
    chat_pipeline: ChatPipelineService = Depends(get_g8ee_chat_pipeline),
    chat_task_manager: BackgroundTaskManager = Depends(get_g8ee_chat_task_manager),
    settings_service: SettingsService = Depends(get_g8ee_settings_service),
    g8e_context: G8eHttpContext = Depends(require_authenticated_context),
):
    """
    Skip triage clarifying questions - internal cluster use only.

    Context is extracted from request body (RequestContext) instead of headers,
    eliminating the fragile header-as-state pattern.
    """
    # Fetch user settings manually using user_id from context to eliminate header dependency
    user_settings = await settings_service.get_user_settings(g8e_context.user_id)

    # Fail-fast if no LLM models are configured
    chat_pipeline.validate_llm_config(
        user_settings=user_settings,
        primary_model_override=None,
        assistant_model_override=None,
        lite_model_override=None,
    )

    logger.info(
        "[INTERNAL-HTTP] Triage skip received",
        extra={
            "investigation_id": g8e_context.investigation_id,
            "user_id": g8e_context.user_id,
        },
    )

    investigation = await investigation_service.get_investigation(g8e_context.investigation_id)
    if not investigation:
        raise ResourceNotFoundError(
            "Investigation not found",
            resource_id=g8e_context.investigation_id,
            resource_type="investigation",
            component="g8ee",
        )

    skip_text = "Skipped clarifying questions"
    await investigation_service.investigation_data_service.add_chat_message(
        investigation_id=g8e_context.investigation_id,
        sender=MessageSender.USER_CHAT,
        content=skip_text,
        metadata=ConversationMessageMetadata(event_type=EventType.AI_TRIAGE_CLARIFICATION_SKIPPED),
    )

    # Trigger AI response by calling run_chat.
    # ChatPipeline.run_chat internally tracks the task via ChatTaskManager,
    # which will cancel any existing active task for this investigation_id.
    await chat_pipeline.run_chat(
        message=skip_text,
        g8e_context=g8e_context,
        attachments=[],
        sentinel_mode=investigation.sentinel_mode,
        llm_primary_provider=None,
        llm_assistant_provider=None,
        llm_lite_provider=None,
        llm_primary_model=user_settings.llm.primary_model,
        llm_assistant_model=user_settings.llm.resolved_assistant_model,
        llm_lite_model=None,
        _task_manager=chat_task_manager,
        user_settings=user_settings,
    )

    return {"success": True}


@router.post(InternalAPIPaths.G8EE_CHAT_TRIAGE_TIMEOUT)
async def internal_triage_timeout(
    request: TriageTimeoutRequest,
    investigation_service: InvestigationService = Depends(get_g8ee_investigation_service),
    g8e_context: G8eHttpContext = Depends(require_authenticated_context),
):
    """
    Record triage clarifying questions timeout - internal cluster use only.

    Context is extracted from request body (RequestContext) instead of headers,
    eliminating the fragile header-as-state pattern.
    """
    logger.info(
        "[INTERNAL-HTTP] Triage timeout received",
        extra={
            "investigation_id": g8e_context.investigation_id,
            "user_id": g8e_context.user_id,
        },
    )

    await investigation_service.investigation_data_service.add_chat_message(
        investigation_id=g8e_context.investigation_id,
        sender=MessageSender.USER_CHAT,
        content="Clarifying questions timed out",
        metadata=ConversationMessageMetadata(event_type=EventType.AI_TRIAGE_CLARIFICATION_TIMEOUT),
    )
    return {"success": True}


@router.post(InternalAPIPaths.G8EE_CHAT_STOP, response_model=StopAIResponse)
async def stop_ai_processing(
    request: StopAIRequest,
    chat_task_manager: BackgroundTaskManager = Depends(get_g8ee_chat_task_manager),
    chat_pipeline: ChatPipelineService = Depends(get_g8ee_chat_pipeline),
    g8e_context: G8eHttpContext = Depends(require_authenticated_context),
):
    """
    Stop active AI processing for an investigation - internal cluster use only.

    Called by client when user clicks the stop button in the UI.
    Gracefully cancels the asyncio task processing the AI response.

    Context is extracted from request body (RequestContext) instead of headers,
    eliminating the fragile header-as-state pattern.
    """
    investigation_id = g8e_context.investigation_id
    reason = request.reason
    web_session_id = g8e_context.web_session_id

    logger.info(
        "[INTERNAL-HTTP] Stop AI processing request",
        extra={
            "investigation_id": investigation_id,
            "reason": reason,
            "user_id": g8e_context.user_id,
        },
    )

    cancelled = await chat_task_manager.cancel(
        task_id=investigation_id,
        reason=reason,
        web_session_id=web_session_id,
        user_id=g8e_context.user_id,
        case_id=g8e_context.case_id,
        event_service=chat_pipeline.event_service,
    )

    if cancelled:
        logger.info(
            "[INTERNAL-HTTP] AI processing stopped successfully",
            extra={"investigation_id": investigation_id},
        )
        return StopAIResponse(
            success=True,
            investigation_id=investigation_id,
            was_active=True,
        )
    logger.info(
        "[INTERNAL-HTTP] No active AI processing to stop",
        extra={"investigation_id": investigation_id},
    )
    return StopAIResponse(
        success=True,
        investigation_id=investigation_id,
        was_active=False,
    )


@router.post(
    InternalAPIPaths.G8EE_OPERATOR_APPROVAL_RESPOND, response_model=ApprovalRespondedResponse
)
async def operator_approval_respond(
    request: OperatorApprovalResponse,
    approval_service: OperatorApprovalService = Depends(get_g8ee_approval_service),
    g8e_context: G8eHttpContext = Depends(require_authenticated_context),
):
    """
    Handle Operator command approval response from client.
    Called directly by client via HTTP when user approves/denies a command.

    Context is extracted from request body (RequestContext) instead of headers,
    eliminating the fragile header-as-state pattern.
    """
    bound_op = g8e_context.bound_operators[0] if g8e_context.bound_operators else None
    request.operator_session_id = bound_op.operator_session_id or "" if bound_op else ""
    request.operator_id = bound_op.operator_id if bound_op else ""

    logger.info(
        "[INTERNAL-HTTP] Received approval response from client",
        extra={
            "approval_id": request.approval_id,
            "approved": request.approved,
            "case_id": g8e_context.case_id,
            "investigation_id": g8e_context.investigation_id,
            "web_session_id": g8e_context.web_session_id[:12] + "..."
            if g8e_context.web_session_id
            else None,
            "bound_operators_count": len(g8e_context.bound_operators),
            "operator_id": request.operator_id,
            "user_id": g8e_context.user_id,
        },
    )

    await approval_service.handle_approval_response(request)

    logger.info(
        "[INTERNAL-HTTP] Approval response processed successfully",
        extra={
            "approval_id": request.approval_id,
            "approved": request.approved,
        },
    )

    return ApprovalRespondedResponse(
        success=True,
        approval_id=request.approval_id,
        approved=request.approved,
    )


@router.get(
    InternalAPIPaths.G8EE_OPERATOR_APPROVAL_PENDING, response_model=PendingApprovalsResponse
)
async def get_pending_approvals(
    approval_service: OperatorApprovalService = Depends(get_g8ee_approval_service),
):
    """
    Get all pending approvals currently waiting for user response.
    Returns a dictionary of approval_id -> PendingApproval.
    """
    pending_approvals = approval_service.get_pending_approvals()

    logger.info(
        "[INTERNAL-HTTP] Retrieved pending approvals", extra={"count": len(pending_approvals)}
    )

    return PendingApprovalsResponse(pending_approvals=pending_approvals)


@router.post(
    InternalAPIPaths.G8EE_OPERATOR_DIRECT_COMMAND, response_model=DirectCommandSentResponse
)
async def execute_direct_command(
    request: DirectCommandRequest,
    operator_data_service: OperatorCommandService = Depends(get_g8ee_operator_command_service),
    g8e_context: G8eHttpContext = Depends(require_authenticated_context),
):
    """
    Execute direct command on operator.

    Context is extracted from request body (RequestContext) instead of headers,
    eliminating the fragile header-as-state pattern.
    """
    logger.info(
        "[INTERNAL-HTTP] Direct command request received",
        extra={
            "command": request.command[:100] if len(request.command) > 100 else request.command,
            "execution_id": g8e_context.execution_id,
            "web_session_id": (g8e_context.web_session_id[:12] + "...")
            if g8e_context.web_session_id
            else None,
            "source": g8e_context.source_component,
            "has_case_id": g8e_context.case_id is not None,
            "has_investigation_id": g8e_context.investigation_id is not None,
        },
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
        extra={"execution_id": g8e_context.execution_id},
    )

    return DirectCommandSentResponse(
        success=True,
        execution_id=g8e_context.execution_id,
    )


@router.post(InternalAPIPaths.G8EE_CASE + "/get", response_model=CaseResponse)
async def get_case(
    case_id: str,
    request: CaseGetRequest,
    case_service: CaseDataService = Depends(get_g8ee_case_data_service),
    g8e_context: G8eHttpContext = Depends(require_authenticated_context),
):
    """Get a case by ID - internal cluster use only."""
    case = await case_service.get_case(case_id)
    return CaseResponse(success=True, case=case)


@router.patch(InternalAPIPaths.G8EE_CASE, response_model=CaseResponse)
async def update_case(
    case_id: str,
    request: CaseUpdateRequest,
    case_service: CaseDataService = Depends(get_g8ee_case_data_service),
    g8e_context: G8eHttpContext = Depends(require_authenticated_context),
):
    """Update a case - internal cluster use only."""
    case = await case_service.update_case(case_id, request)
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


@router.post(InternalAPIPaths.G8EE_CASE + "/delete", status_code=status.HTTP_204_NO_CONTENT)
async def delete_case(
    case_id: str,
    request: CaseDeleteRequest,
    case_service: CaseDataService = Depends(get_g8ee_case_data_service),
    investigation_service: InvestigationService = Depends(get_g8ee_investigation_service),
    cache_aside_service: CacheAsideService = Depends(get_g8ee_cache_aside_service),
    g8e_context: G8eHttpContext = Depends(require_authenticated_context),
    request_context: RequestContext = Depends(get_request_context),
):
    """
    Delete a case and all related data - internal cluster use only.

    Deletes:
    - Case document
    - All investigations with this case_id
    - All memories with this case_id
    """
    try:
        case = await case_service.get_case(case_id)
        case_user_id = case.user_id
    except ResourceNotFoundError:
        logger.info("[INTERNAL-HTTP] Case already deleted (idempotent)", extra={"case_id": case_id})
        return

    logger.info("[INTERNAL-HTTP] Deleting case and related data", extra={"case_id": case_id})

    # Delete all investigations for this case - SCOPED BY USER for security
    investigations = await investigation_service.investigation_data_service.get_case_investigations(
        case_id=case_id,
        user_id=case_user_id,
        context=request_context,
    )
    for investigation in investigations:
        logger.info(
            "[INTERNAL-HTTP] Deleting investigation",
            extra={"investigation_id": investigation.id, "case_id": case_id},
        )
        await investigation_service.investigation_data_service.delete_investigation(
            investigation.id, context=request_context
        )

    logger.info(
        "[INTERNAL-HTTP] Deleted investigations",
        extra={"case_id": case_id, "count": len(investigations)},
    )

    # Delete all memories for this case (scoped to user for tenant isolation)
    memory_docs = await cache_aside_service.query_documents(
        collection=DB_COLLECTION_MEMORIES,
        field_filters=[
            FieldFilter(field="user_id", op="==", value=case_user_id).model_dump(mode="json"),
            FieldFilter(field="case_id", op="==", value=case_id).model_dump(mode="json"),
        ],
    )

    if memory_docs:
        for memory_doc in memory_docs:
            memory_id = memory_doc.get("investigation_id")
            if memory_id:
                await cache_aside_service.delete_document(
                    collection=DB_COLLECTION_MEMORIES, document_id=memory_id
                )
                logger.info(
                    "[INTERNAL-HTTP] Deleted memory",
                    extra={"memory_id": memory_id, "case_id": case_id},
                )

    if memory_docs:
        logger.info(
            "[INTERNAL-HTTP] Deleted memories",
            extra={"case_id": case_id, "count": len(memory_docs)},
        )

    # Finally delete the case
    await case_service.delete_case(case_id, context=request_context)

    logger.info(
        "[INTERNAL-HTTP] Case and all related data deleted successfully", extra={"case_id": case_id}
    )


@router.post(InternalAPIPaths.G8EE_OPERATORS_TERMINATE, response_model=OperatorTerminateResponse)
async def terminate_operator(
    request: OperatorTerminateRequest,
    gateway_operator_client: GatewayOperatorClient = Depends(get_g8ee_gateway_operator_client),
    g8e_context: G8eHttpContext = Depends(require_authenticated_context),
):
    """
    Terminate an operator slot via the Gateway-owned lifecycle API.

    Called by client during API key refresh and manual termination.
    SECURITY: Internal only - client component.
    """
    try:
        result = await gateway_operator_client.terminate(
            context=g8e_context,
            operator_id=request.operator_id,
        )
    except NetworkError as exc:
        logger.error(
            "[INTERNAL-HTTP] Gateway terminate operator failed",
            extra={"operator_id": request.operator_id, "error": str(exc)},
        )
        return OperatorTerminateResponse(success=False, error=str(exc))

    logger.info(
        "[INTERNAL-HTTP] Operator terminated via gateway",
        extra={"operator_id": request.operator_id, "user_id": g8e_context.user_id},
    )
    return OperatorTerminateResponse(success=bool(result.get("success", True)))


@router.post(InternalAPIPaths.G8EE_OPERATORS_GATEWAY_SESSION_AUTH)
async def listen_session_auth(
    request: OperatorListenSessionAuthRequest,
    g8e_context: G8eHttpContext = Depends(require_authenticated_context),
):
    """Removed: operators bootstrap via Gateway POST /api/v1/operators/reauth."""
    logger.warning(
        "[INTERNAL-HTTP] Rejected legacy session auth gateway request",
        extra={"operator_id": request.operator_id, "user_id": request.user_id},
    )
    return {"success": False, "error": _GATEWAY_OPERATOR_AUTHORITY_ERROR}


@router.post(
    InternalAPIPaths.G8EE_OPERATORS_CREATE_SLOT, response_model=OperatorSlotCreationResponse
)
async def create_operator_slot(
    request: OperatorSlotCreationRequest,
    operator_data_service: OperatorDataService = Depends(get_g8ee_operator_data_service),
    settings_service: SettingsService = Depends(get_g8ee_settings_service_write),
    api_key_service: APIKeyService = Depends(get_g8ee_api_key_service),
    g8e_context: G8eHttpContext = Depends(require_authenticated_context),
):
    """
    Create an operator slot.

    Called by client during user initialization and device link creation.
    g8ee handles the actual write to the operator document to enforce the
    architectural boundary: after auth, client has no business writing to operators.
    SECURITY: Internal only - client component.

    Context is extracted from request body (RequestContext) instead of headers,
    eliminating the fragile header-as-state pattern.
    """
    try:
        operator_id = str(uuid.uuid4())

        # Generate API key (authority: g8ee for operator bootstrap)
        operator_suffix = operator_id.rsplit("-", maxsplit=1)[-1][:8]
        random_token = secrets.token_hex(32)
        api_key = f"g8e_{operator_suffix}_{random_token}"

        # Create operator document
        operator_doc = OperatorDocument(
            id=operator_id,
            user_id=g8e_context.user_id,
            organization_id=g8e_context.organization_id,
            name=f"{request.name_prefix}-{request.slot_number}",
            slot_number=request.slot_number,
            operator_type=request.operator_type,
            status=OperatorStatus.OFFLINE,
            api_key=api_key,
            created_at=now(),
            updated_at=now(),
        )

        await operator_data_service.create_operator(operator_doc)

        # Issue API key to api_keys collection (canonical)
        key_issued = await api_key_service.issue_operator_key(
            api_key=api_key,
            user_id=g8e_context.user_id,
            organization_id=g8e_context.organization_id,
            operator_id=operator_id,
            settings_service=settings_service,
            client_name="operator",
            permissions=["OPERATOR_BOOTSTRAP", "OPERATOR_HEARTBEAT", "OPERATOR_DOWNLOAD"],
        )

        if not key_issued:
            logger.error(
                "[INTERNAL-HTTP] Failed to issue API key to api_keys collection",
                extra={"operator_id": operator_id, "user_id": g8e_context.user_id},
            )
            return OperatorSlotCreationResponse(
                success=False,
                operator_id=None,
                error="Failed to issue API key",
            )

        logger.info(
            "[INTERNAL-HTTP] Operator slot created",
            extra={
                "operator_id": operator_id,
                "user_id": g8e_context.user_id,
                "slot_number": request.slot_number,
            },
        )

        return OperatorSlotCreationResponse(
            success=True,
            operator_id=operator_id,
            api_key=api_key,
        )

    except Exception as e:
        logger.error(
            "[INTERNAL-HTTP] Failed to create operator slot",
            extra={"error": str(e), "user_id": g8e_context.user_id},
        )
        return OperatorSlotCreationResponse(
            success=False,
            operator_id=None,
            error=str(e),
        )


@router.post(
    InternalAPIPaths.G8EE_OPERATORS_UPDATE_API_KEY, response_model=OperatorUpdateAPIKeyResponse
)
async def update_operator_api_key(
    request: OperatorUpdateAPIKeyRequest,
    operator_data_service: OperatorDataService = Depends(get_g8ee_operator_data_service),
    settings_service: SettingsService = Depends(get_g8ee_settings_service_write),
    api_key_service: APIKeyService = Depends(get_g8ee_api_key_service),
    g8e_context: G8eHttpContext = Depends(require_authenticated_context),
):
    """
    Update an operator's API key.

    Called by client during initialization to issue API keys for existing slots
    that were created without keys during setup.
    g8ee handles the actual write to the operator document to enforce the
    architectural boundary: after auth, client has no business writing to operators.
    SECURITY: Internal only - client component.

    Context is extracted from request body (RequestContext) instead of headers,
    eliminating the fragile header-as-state pattern.
    """
    try:
        operator = await operator_data_service.get_operator(request.operator_id)
        if not operator:
            logger.error(
                "[INTERNAL-HTTP] Operator not found for API key update",
                extra={"operator_id": request.operator_id},
            )
            return OperatorUpdateAPIKeyResponse(success=False, error="Operator not found")

        # Rotate the API key in the canonical store BEFORE updating the operator doc.
        # Failure here means the operator doc is left untouched and the old key remains
        # authoritative - no phantom keys, no split-brain.
        rotated = await api_key_service.rotate_operator_key(
            old_api_key=operator.api_key,
            new_api_key=request.api_key,
            user_id=g8e_context.user_id,
            organization_id=g8e_context.organization_id,
            operator_id=operator.id,
            settings_service=settings_service,
            permissions=["OPERATOR_BOOTSTRAP", "OPERATOR_HEARTBEAT", "OPERATOR_DOWNLOAD"],
        )
        if not rotated:
            logger.error(
                "[INTERNAL-HTTP] Failed to rotate operator API key",
                extra={"operator_id": request.operator_id},
            )
            return OperatorUpdateAPIKeyResponse(success=False, error="Failed to rotate API key")

        updated_operator = operator.model_copy(
            update={
                "api_key": request.api_key,
                "updated_at": now(),
            }
        )

        await operator_data_service.update_operator(updated_operator)

        logger.info(
            "[INTERNAL-HTTP] Operator API key updated", extra={"operator_id": request.operator_id}
        )

        return OperatorUpdateAPIKeyResponse(success=True)

    except Exception as e:
        logger.error(
            "[INTERNAL-HTTP] Failed to update operator API key",
            extra={"error": str(e), "operator_id": request.operator_id},
        )
        return OperatorUpdateAPIKeyResponse(success=False, error=str(e))


@router.post(InternalAPIPaths.G8EE_AUTH_GENERATE_KEY, response_model=APIKeyGenerationResponse)
async def generate_api_key(
    request: APIKeyGenerationRequest,
    api_key_service: APIKeyService = Depends(get_g8ee_api_key_service),
    g8e_context: G8eHttpContext = Depends(require_authenticated_context),
):
    """Generate a new API key.

    Authority: g8ee.
    SECURITY: Internal only - client component.
    """
    try:
        api_key = api_key_service.generate_raw_key(prefix=request.prefix)
        return APIKeyGenerationResponse(success=True, api_key=api_key)
    except Exception as e:
        logger.error("[INTERNAL-HTTP] Failed to generate API key: %s", e)
        return APIKeyGenerationResponse(success=False, error=str(e))


@router.post(
    InternalAPIPaths.G8EE_AUTH_REVOKE_CERT, response_model=OperatorCertificateRevokeResponse
)
async def revoke_operator_certificate(
    request: OperatorCertificateRevokeRequest,
    certificate_service: CertificateService = Depends(get_g8ee_certificate_service),
    g8e_context: G8eHttpContext = Depends(require_authenticated_context),
):
    """Revoke an operator certificate.

    Authority: g8ee.
    SECURITY: Internal only - client component.
    """
    try:
        success = await certificate_service.revoke_certificate(
            serial=request.serial, reason=request.reason, operator_id=request.operator_id
        )
        return OperatorCertificateRevokeResponse(success=success)
    except Exception as e:
        logger.error("[INTERNAL-HTTP] Failed to revoke certificate: %s", e)
        return OperatorCertificateRevokeResponse(success=False, error=str(e))


@router.post(InternalAPIPaths.G8EE_OPERATORS_CLAIM_SLOT, response_model=OperatorSlotClaimResponse)
async def claim_operator_slot(
    request: OperatorSlotClaimRequest,
    g8e_context: G8eHttpContext = Depends(require_authenticated_context),
):
    """Removed: slot claims are Gateway-owned during enrollment/reauth."""
    logger.warning(
        "[INTERNAL-HTTP] Rejected legacy operator slot claim",
        extra={"operator_id": request.operator_id},
    )
    return OperatorSlotClaimResponse(success=False, error=_GATEWAY_OPERATOR_AUTHORITY_ERROR)


@router.post(InternalAPIPaths.G8EE_OPERATORS_BIND, response_model=OperatorBindResponse)
async def bind_operators(
    request: OperatorBindRequest,
    gateway_operator_client: GatewayOperatorClient = Depends(get_g8ee_gateway_operator_client),
    event_service: EventService = Depends(get_g8ee_event_service),
    g8e_context: G8eHttpContext = Depends(require_authenticated_context),
):
    """
    Bind operators to a web session via the Gateway-owned binding API.

    Called by client during operator bind operations.
    SECURITY: Internal only - client component.
    """
    try:
        result = await gateway_operator_client.bind(
            context=g8e_context,
            operator_ids=request.operator_ids,
        )
    except NetworkError as exc:
        logger.error("[INTERNAL-HTTP] Gateway bind operators failed: %s", exc)
        return OperatorBindResponse(
            success=False,
            failed_count=len(request.operator_ids),
            failed_operator_ids=request.operator_ids,
            errors=[{"error": str(exc)}],
        )

    bound_ids = result.get("bound_operator_ids") or []
    failed_ids = result.get("failed_operator_ids") or []
    if not isinstance(bound_ids, list):
        bound_ids = []
    if not isinstance(failed_ids, list):
        failed_ids = []

    await _publish_gateway_operator_status_events(
        gateway_operator_client,
        event_service,
        g8e_context,
        bound_ids,
        OperatorStatus.BOUND,
        EventType.OPERATOR_STATUS_UPDATED_BOUND,
    )

    logger.info(
        "[INTERNAL-HTTP] Operators bound via gateway",
        extra={
            "bound_count": len(bound_ids),
            "failed_count": len(failed_ids),
            "user_id": g8e_context.user_id,
        },
    )

    return OperatorBindResponse(
        success=bool(result.get("success")),
        bound_count=int(result.get("bound_count", len(bound_ids))),
        failed_count=int(result.get("failed_count", len(failed_ids))),
        bound_operator_ids=bound_ids,
        failed_operator_ids=failed_ids,
        errors=_per_operator_errors(failed_ids, result.get("error")),
    )


@router.post(InternalAPIPaths.G8EE_OPERATORS_UNBIND, response_model=OperatorUnbindResponse)
async def unbind_operators(
    request: OperatorUnbindRequest,
    gateway_operator_client: GatewayOperatorClient = Depends(get_g8ee_gateway_operator_client),
    event_service: EventService = Depends(get_g8ee_event_service),
    g8e_context: G8eHttpContext = Depends(require_authenticated_context),
):
    """
    Unbind operators from a web session via the Gateway-owned binding API.

    Called by client during operator unbind operations.
    SECURITY: Internal only - client component.
    """
    try:
        result = await gateway_operator_client.unbind(
            context=g8e_context,
            operator_ids=request.operator_ids,
        )
    except NetworkError as exc:
        logger.error("[INTERNAL-HTTP] Gateway unbind operators failed: %s", exc)
        return OperatorUnbindResponse(
            success=False,
            failed_count=len(request.operator_ids),
            failed_operator_ids=request.operator_ids,
            errors=[{"error": str(exc)}],
        )

    unbound_ids = result.get("unbound_operator_ids") or []
    failed_ids = result.get("failed_operator_ids") or []
    if not isinstance(unbound_ids, list):
        unbound_ids = []
    if not isinstance(failed_ids, list):
        failed_ids = []

    await _publish_gateway_operator_status_events(
        gateway_operator_client,
        event_service,
        g8e_context,
        unbound_ids,
        OperatorStatus.ACTIVE,
        EventType.OPERATOR_STATUS_UPDATED_ACTIVE,
    )

    logger.info(
        "[INTERNAL-HTTP] Operators unbound via gateway",
        extra={
            "unbound_count": len(unbound_ids),
            "failed_count": len(failed_ids),
            "user_id": g8e_context.user_id,
        },
    )

    return OperatorUnbindResponse(
        success=bool(result.get("success")),
        unbound_count=int(result.get("unbound_count", len(unbound_ids))),
        failed_count=int(result.get("failed_count", len(failed_ids))),
        unbound_operator_ids=unbound_ids,
        failed_operator_ids=failed_ids,
        errors=_per_operator_errors(failed_ids, result.get("error")),
    )


@router.post(
    InternalAPIPaths.G8EE_OPERATORS_AUTHENTICATE, response_model=OperatorAuthenticateResponse
)
async def authenticate_operator(
    request: InternalOperatorAuthCall,
    g8e_context: G8eHttpContext = Depends(require_authenticated_context),
):
    """Removed: operator authentication is Gateway-owned via enrollment/reauth."""
    logger.warning("[INTERNAL-HTTP] Rejected legacy operator authenticate request")
    return OperatorAuthenticateResponse(success=False, error=_GATEWAY_OPERATOR_AUTHORITY_ERROR)


@router.post(
    InternalAPIPaths.G8EE_OPERATORS_DEVICE_LINK_REGISTER,
    response_model=OperatorDeviceLinkRegisterResponse,
)
async def register_device_link_operator(
    request: OperatorDeviceLinkRegisterRequest,
    g8e_context: G8eHttpContext = Depends(require_authenticated_context),
) -> OperatorDeviceLinkRegisterResponse:
    """Removed: device-link operator bootstrap is Gateway-owned."""
    logger.warning(
        "[INTERNAL-HTTP] Rejected legacy device-link operator registration",
        extra={"operator_id": request.operator_id},
    )
    return OperatorDeviceLinkRegisterResponse(success=False, error=_GATEWAY_OPERATOR_AUTHORITY_ERROR)


@router.post(
    InternalAPIPaths.G8EE_OPERATORS_VALIDATE_SESSION, response_model=OperatorSessionValidateResponse
)
async def validate_operator_session(
    request: OperatorSessionValidateRequest,
    gateway_operator_client: GatewayOperatorClient = Depends(get_g8ee_gateway_operator_client),
    g8e_context: G8eHttpContext = Depends(require_authenticated_context),
):
    """Validate an operator session via the Gateway-owned binding API."""
    try:
        result = await gateway_operator_client.validate_session(
            context=g8e_context,
            operator_session_id=request.operator_session_id,
            cli_session_id=g8e_context.cli_session_id or "",
        )
    except NetworkError as exc:
        logger.error("[INTERNAL-HTTP] Gateway session validation failed: %s", exc)
        return OperatorSessionValidateResponse(success=False, valid=False, error=str(exc))

    return OperatorSessionValidateResponse(
        success=True,
        valid=bool(result.get("valid")),
        user_id=result.get("user_id"),
        operator_id=result.get("operator_id"),
    )


@router.post(
    InternalAPIPaths.G8EE_OPERATORS_REFRESH_SESSION, response_model=OperatorSessionRefreshResponse
)
async def refresh_operator_session(
    request: OperatorSessionRefreshRequest,
    g8e_context: G8eHttpContext = Depends(require_authenticated_context),
):
    """Removed: operator session refresh is Gateway-owned."""
    logger.warning(
        "[INTERNAL-HTTP] Rejected legacy operator session refresh",
        extra={"operator_session_id": request.operator_session_id[:12] + "..."},
    )
    return OperatorSessionRefreshResponse(success=False, error=_GATEWAY_OPERATOR_AUTHORITY_ERROR)


@router.post(InternalAPIPaths.G8EE_OPERATORS_STOP, response_model=OperatorStoppedResponse)
async def stop_operator(
    request: StopOperatorRequest,
    gateway_operator_client: GatewayOperatorClient = Depends(get_g8ee_gateway_operator_client),
    g8e_context: G8eHttpContext = Depends(require_authenticated_context),
):
    """
    Stop a remote operator via the Gateway-owned dispatch API.

    Context is extracted from request body (RequestContext) instead of headers,
    eliminating the fragile header-as-state pattern.
    """
    logger.info(
        "[OPERATOR-STOP] Requesting gateway stop for operator",
        extra={
            "operator_id": request.operator_id,
            "operator_session_id": request.operator_session_id,
            "user_id": g8e_context.user_id,
            "web_session_id": g8e_context.web_session_id[:12] + "..."
            if g8e_context.web_session_id
            else None,
        },
    )

    try:
        result = await gateway_operator_client.stop(
            context=g8e_context,
            operator_session_id=request.operator_session_id,
        )
    except NetworkError as exc:
        logger.error("[OPERATOR-STOP] Gateway stop failed: %s", exc)
        raise ServiceUnavailableError(str(exc), component="g8ee") from exc

    logger.info(
        "[OPERATOR-STOP] Gateway stop dispatched successfully",
        extra={
            "operator_id": result.get("operator_id", request.operator_id),
            "transaction_id": result.get("transaction_id"),
        },
    )

    return OperatorStoppedResponse(
        success=bool(result.get("success", True)),
        operator_id=str(result.get("operator_id", request.operator_id)),
        subscribers=1 if result.get("transaction_id") else 0,
    )


@router.post(InternalAPIPaths.G8EE_INVESTIGATIONS + "/query")
async def query_investigations(
    request: InvestigationQueryRequest,
    investigation_service: InvestigationService = Depends(get_g8ee_investigation_service),
    g8e_context: G8eHttpContext = Depends(require_authenticated_context),
):
    """Query investigations - internal cluster use only."""
    logger.info(
        "[INTERNAL-HTTP] Investigation query via RequestContext",
        extra={"user_id": g8e_context.user_id, "source": g8e_context.source_component},
    )

    # Use the request object directly since it already contains the filters
    return await investigation_service.investigation_data_service.query_investigations(request)


@router.post(InternalAPIPaths.G8EE_INVESTIGATION + "/get", response_model=InvestigationModel)
async def get_investigation(
    investigation_id: str,
    request: InvestigationGetRequest,
    investigation_service: InvestigationService = Depends(get_g8ee_investigation_service),
    g8e_context: G8eHttpContext = Depends(require_authenticated_context),
):
    """Get investigation by ID - internal cluster use only.

    SECURITY: Validates that the authenticated user owns the investigation.
    """
    logger.info(
        "[INTERNAL-HTTP] Get investigation via RequestContext",
        extra={"user_id": g8e_context.user_id, "investigation_id": investigation_id},
    )

    investigation = await investigation_service.investigation_data_service.get_investigation(
        investigation_id
    )
    if not investigation:
        raise ResourceNotFoundError(
            f"Investigation {investigation_id} not found",
            resource_type="investigation",
            resource_id=investigation_id,
            component="g8ee",
        )

    if investigation.user_id != g8e_context.user_id:
        logger.warning(
            "AUTHORIZATION VIOLATION: User attempted to access another user's investigation",
            extra={
                "authenticated_user_id": g8e_context.user_id,
                "investigation_owner": investigation.user_id,
                "investigation_id": investigation_id,
            },
        )
        raise ResourceNotFoundError(
            f"Investigation {investigation_id} not found",
            resource_type="investigation",
            resource_id=investigation_id,
            component="g8ee",
        )

    return investigation


@router.get(
    InternalAPIPaths.G8EE_EVALUATION_TRACE,
    response_model=EvaluationTraceResponse,
)
async def get_evaluation_trace(
    trace_ids: tuple[str, str] = Depends(validated_trace_ids),
    _: G8eHttpContext = Depends(require_authenticated_context),
):
    """Authenticated read-only lookup for a persisted evaluation assignment trace."""
    assignment_id, evaluation_attempt_id = trace_ids
    trace_service = EvaluationTraceService()
    try:
        trace = trace_service.load(assignment_id, evaluation_attempt_id)
    except FileNotFoundError:
        raise ResourceNotFoundError(
            f"Evaluation trace not found for assignment {assignment_id}",
            resource_type="evaluation_trace",
            resource_id=f"{assignment_id}/{evaluation_attempt_id}",
            component="g8ee",
        )
    return EvaluationTraceResponse(trace=trace.model_dump(mode="json"))


@router.get(InternalAPIPaths.G8EE_HEALTH)
async def health_check():
    """Health check for internal API"""
    return {
        "service": "g8ee-internal-api",
        "status": "healthy",
        "endpoints": [
            InternalAPIPaths.G8EE_CHAT,
            InternalAPIPaths.G8EE_CHAT_STOP,
            InternalAPIPaths.G8EE_OPERATOR_APPROVAL_RESPOND,
            InternalAPIPaths.G8EE_OPERATOR_DIRECT_COMMAND,
            InternalAPIPaths.G8EE_CASES,
            InternalAPIPaths.G8EE_CASE,
            InternalAPIPaths.G8EE_INVESTIGATIONS,
            InternalAPIPaths.G8EE_INVESTIGATION,
            InternalAPIPaths.G8EE_CHAT_TRIAGE_ANSWER,
            InternalAPIPaths.G8EE_CHAT_TRIAGE_SKIP,
            InternalAPIPaths.G8EE_CHAT_TRIAGE_TIMEOUT,
            InternalAPIPaths.G8EE_SETTINGS_USER,
        ],
    }


@router.post(InternalAPIPaths.G8EE_SETTINGS_USER + "/get", response_model=G8eeUserSettings)
async def get_user_settings(
    request: SettingsGetRequest,
    settings_service: SettingsService = Depends(get_g8ee_settings_service_write),
    g8e_context: G8eHttpContext = Depends(require_authenticated_context),
):
    """
    Get user settings - internal cluster use only.
    """
    user_id = g8e_context.user_id
    logger.info("[INTERNAL-HTTP] Retrieving user settings", extra={"user_id": user_id})
    return await settings_service.get_user_settings(user_id)


@router.post(InternalAPIPaths.G8EE_SETTINGS_SYNC, response_model=UserSettingsUpdateResponse)
async def settings_sync(
    request: SettingsSyncRequest,
    settings_service: SettingsService = Depends(get_g8ee_settings_service_write),
):
    """
    Persist LLM and Search settings overrides into user settings.

    This replaces the legacy "sync-on-chat" behavior to avoid side-effects
    during inference and ensure settings are committed before chat starts.
    """
    user_id = request.context.user_id
    user_settings = await settings_service.get_user_settings(user_id)

    success = await settings_service.sync_settings_overrides(user_id, user_settings, request)
    return UserSettingsUpdateResponse(success=success)


@router.patch(InternalAPIPaths.G8EE_SETTINGS_USER, response_model=UserSettingsUpdateResponse)
async def sync_user_settings(
    request: dict,
    cache_aside: CacheAsideService = Depends(get_g8ee_cache_aside_service),
):
    """
    Sync user settings from client - internal cluster use only.

    Invalidates the local cache for the user's settings so subsequent
    requests will fetch the fresh settings from operator.
    """
    user_id = request.get("user_id")
    if not user_id:
        return UserSettingsUpdateResponse(
            success=False, error="user_id is required in request body"
        )

    logger.info(
        "[INTERNAL-HTTP] Syncing user settings (cache invalidation)", extra={"user_id": user_id}
    )

    try:
        user_doc_id = f"{USER_SETTINGS_DOC_PREFIX}{user_id}"
        await cache_aside.invalidate_local_cache(
            collection=DB_COLLECTION_SETTINGS, document_id=user_doc_id
        )
        return UserSettingsUpdateResponse(success=True)
    except Exception as e:
        logger.error(
            "[INTERNAL-HTTP] Failed to invalidate settings cache",
            extra={"error": str(e), "user_id": user_id},
        )
        return UserSettingsUpdateResponse(success=False, error=str(e))
