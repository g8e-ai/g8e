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
    DB_COLLECTION_MEMORIES,
    EventType,
    Priority,
    InternalApiPaths,
    API_PATHS,
)
from app.errors import ResourceNotFoundError, ServiceUnavailableError
from app.models import CaseCreateRequest, CaseEventPayload, CaseUpdateRequest
from app.models.cases import CaseCreatedPayload
from app.models.cache import FieldFilter
from app.models.internal_api import (
    ApprovalRespondedResponse,
    CaseResponse,
    ChatMessageRequest,
    ChatStartedResponse,
    DirectCommandRequest,
    DirectCommandSentResponse,
    MCPToolCallRequest,
    MCPToolCallResponse,
    MCPToolListResponse,
    OperatorApprovalResponse,
    OperatorSessionRegisteredResponse,
    OperatorSessionRegistrationRequest,
    OperatorStoppedResponse,
    PendingApprovalsResponse,
    StopAIRequest,
    StopAIResponse,
    StopOperatorRequest,
)
from app.models.investigations import (
    InvestigationCreateRequest,
    InvestigationModel,
    InvestigationQueryRequest,
    InvestigationUpdateRequest,
)
from app.models.events import SessionEvent
from app.utils.timestamp import now_iso

from ..dependencies import (
    get_g8ee_platform_settings,
    get_g8ee_approval_service,
    get_g8ee_attachment_service,
    get_g8ee_cache_aside_service,
    get_g8ee_case_data_service,
    get_g8ee_chat_pipeline,
    get_g8ee_chat_task_manager,
    get_g8ee_mcp_gateway_service,
    get_g8ee_event_service,
    get_g8ee_heartbeat_service,
    get_g8ee_investigation_service,
    get_g8ee_operator_command_service,
    get_g8e_http_context,
    get_g8ee_user_settings,
)
from ..services.ai.chat_pipeline import ChatPipelineService
from ..services.ai.chat_task_manager import ChatTaskManager
from ..services.cache.cache_aside import CacheAsideService
from ..services.data.attachment_store_service import AttachmentService
from ..services.data.case_data_service import CaseDataService
from ..services.investigation.investigation_service import InvestigationService
from ..services.infra.g8ed_event_service import EventService
from ..models.http_context import G8eHttpContext
from ..services.operator.approval_service import OperatorApprovalService
from ..services.operator.command_service import OperatorCommandService
from ..services.operator.heartbeat_service import OperatorHeartbeatService
from ..services.mcp.gateway_service import MCPGatewayService
from ..services.ai.title_generator import generate_case_title

logger = logging.getLogger(__name__)

router = APIRouter(prefix=InternalApiPaths.PREFIX, tags=["internal"])




@router.post(API_PATHS["g8ee"]["chat"], response_model=ChatStartedResponse)
async def internal_chat(
    request: ChatMessageRequest,
    platform_settings: G8eePlatformSettings = Depends(get_g8ee_platform_settings),
    user_settings: G8eeUserSettings = Depends(get_g8ee_user_settings),
    chat_pipeline: ChatPipelineService = Depends(get_g8ee_chat_pipeline),
    chat_task_manager: ChatTaskManager = Depends(get_g8ee_chat_task_manager),
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

        if request.message.strip():
            case_result = await generate_case_title(request.message, settings=user_settings)
            ai_title = case_result.generated_title
            updated_case = await case_service.update_case(g8e_context.case_id, CaseUpdateRequest(title=ai_title))
            await investigation_service.update_investigation(
                g8e_context.investigation_id,
                InvestigationUpdateRequest(case_title=ai_title)
            )
            await case_service.publish_case_update_sse(
                case_id=g8e_context.case_id,
                web_session_id=g8e_context.web_session_id,
                payload=CaseEventPayload(
                    updated_at=updated_case.updated_at,
                    title=ai_title,
                ),
            )

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
            case_id=g8e_context.case_id,
            investigation_id=g8e_context.investigation_id,
        )

    asyncio.create_task(
        chat_pipeline.run_chat(
            message=request.message,
            g8e_context=g8e_context,
            attachments=resolved_attachments,
            sentinel_mode=request.sentinel_mode,
            llm_primary_model=request.llm_primary_model,
            llm_assistant_model=request.llm_assistant_model,
            _task_manager=chat_task_manager,
            user_settings=user_settings,
        )
    )

    return ChatStartedResponse(
        success=True,
        case_id=g8e_context.case_id,
        investigation_id=g8e_context.investigation_id,
    )


@router.post(API_PATHS["g8ee"]["chat_stop"], response_model=StopAIResponse)
async def stop_ai_processing(
    request: StopAIRequest,
    g8e_context: G8eHttpContext = Depends(get_g8e_http_context),
    chat_task_manager: ChatTaskManager = Depends(get_g8ee_chat_task_manager),
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
        investigation_id=investigation_id,
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


@router.post(API_PATHS["g8ee"]["operator_approval_respond"], response_model=ApprovalRespondedResponse)
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


@router.get(API_PATHS["g8ee"]["operator_approval_pending"], response_model=PendingApprovalsResponse)
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


@router.post(API_PATHS["g8ee"]["operator_direct_command"], response_model=DirectCommandSentResponse)
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


@router.get(API_PATHS["g8ee"]["case"], response_model=CaseResponse)
async def get_case(
    case_id: str,
    case_service: CaseDataService = Depends(get_g8ee_case_data_service),
    g8e_context: G8eHttpContext = Depends(get_g8e_http_context)
):
    """Get a case by ID - internal cluster use only."""
    case = await case_service.get_case(case_id)
    return CaseResponse(success=True, case=case)


@router.patch(API_PATHS["g8ee"]["case"], response_model=CaseResponse)
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
        )
    return CaseResponse(success=True, case=case)


@router.delete(API_PATHS["g8ee"]["case"], status_code=status.HTTP_204_NO_CONTENT)
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
    investigations = await investigation_service.get_case_investigations(
        case_id=case_id,
        user_id=user_id
    )
    for investigation in investigations:
        logger.info(
            "[INTERNAL-HTTP] Deleting investigation",
            extra={"investigation_id": investigation.id, "case_id": case_id}
        )
        await investigation_service.delete_investigation(investigation.id)

    logger.info(
        "[INTERNAL-HTTP] Deleted investigations",
        extra={"case_id": case_id, "count": len(investigations)}
    )

    # Delete all memories for this case (scoped to user for tenant isolation)
    memory_docs = await cache_aside_service.query_documents(
        collection=DB_COLLECTION_MEMORIES,
        field_filters=[
            FieldFilter(field="user_id", op="==", value=user_id).flatten_for_wire(),
            FieldFilter(field="case_id", op="==", value=case_id).flatten_for_wire(),
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



@router.post(API_PATHS["g8ee"]["operators_register_session"], response_model=OperatorSessionRegisteredResponse)
async def register_operator_session(
    request: OperatorSessionRegistrationRequest,
    heartbeat_service: OperatorHeartbeatService = Depends(get_g8ee_heartbeat_service),
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


@router.post(API_PATHS["g8ee"]["operators_deregister_session"], response_model=OperatorSessionRegisteredResponse)
async def deregister_operator_session(
    request: OperatorSessionRegistrationRequest,
    heartbeat_service: OperatorHeartbeatService = Depends(get_g8ee_heartbeat_service),
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


@router.post(API_PATHS["g8ee"]["operators_stop"], response_model=OperatorStoppedResponse)
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


@router.get(API_PATHS["g8ee"]["investigations"])
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

    investigations = await investigation_service.query_investigations(query_request)
    return investigations


@router.get(API_PATHS["g8ee"]["investigation"], response_model=InvestigationModel)
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

    investigation = await investigation_service.get_investigation(investigation_id)
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


@router.post(API_PATHS["g8ee"]["mcp_tools_list"], response_model=MCPToolListResponse)
async def mcp_tools_list(
    g8e_context: G8eHttpContext = Depends(get_g8e_http_context),
    mcp_service: MCPGatewayService = Depends(get_g8ee_mcp_gateway_service),
    investigation_service: InvestigationService = Depends(get_g8ee_investigation_service),
):
    agent_mode = AgentMode.OPERATOR_BOUND if any(op.status == OperatorStatus.BOUND for op in g8e_context.bound_operators) else AgentMode.OPERATOR_NOT_BOUND

    logger.info(
        "[INTERNAL-HTTP] MCP tools/list request",
        extra={
            "agent_mode": agent_mode.value,
            "bound_operators": len(g8e_context.bound_operators) if g8e_context.bound_operators else 0,
        }
    )

    tools = mcp_service.list_tools(agent_mode)
    return MCPToolListResponse(tools=tools)


@router.post(API_PATHS["g8ee"]["mcp_tools_call"], response_model=MCPToolCallResponse)
async def mcp_tools_call(
    request: MCPToolCallRequest,
    g8e_context: G8eHttpContext = Depends(get_g8e_http_context),
    mcp_service: MCPGatewayService = Depends(get_g8ee_mcp_gateway_service),
    user_settings: G8eeUserSettings = Depends(get_g8ee_user_settings),
):
    logger.info(
        "[INTERNAL-HTTP] MCP tools/call request",
        extra={
            "tool_name": request.tool_name,
            "request_id": request.request_id,
            "bound_operators": len(g8e_context.bound_operators) if g8e_context.bound_operators else 0,
        }
    )

    try:
        result = await mcp_service.call_tool(
            tool_name=request.tool_name,
            arguments=request.arguments,
            g8e_context=g8e_context,
            user_settings=user_settings,
            sentinel_mode=request.sentinel_mode,
        )
        return MCPToolCallResponse(id=request.request_id, result=result)
    except Exception as e:
        logger.error("[INTERNAL-HTTP] MCP tools/call failed: %s", e, exc_info=True)
        return MCPToolCallResponse(
            id=request.request_id,
            error={"code": -32603, "message": str(e)},
        )


@router.get(API_PATHS["g8ee"]["health"])
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
            InternalApiPaths.G8EE_MCP_TOOLS_LIST,
            InternalApiPaths.G8EE_MCP_TOOLS_CALL,
        ]
    }
