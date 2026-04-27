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
Chat Router for g8ee

Provides chat endpoints for interactive customer support using g8ee's
LLM-based analysis system.
"""

import logging

from fastapi import APIRouter, Depends, Request

from app.constants import ChatSessionStatus, InvestigationStatus, MessageSender, EventType
from app.errors import ResourceNotFoundError
from app.models import InvestigationModel
from app.models.chat_api import ChatSessionDetailsResponse, ChatSessionResponse, LatestChatSessionResponse
from app.models.investigations import ConversationMessageMetadata
from app.models.auth import AuthenticatedUser
from app.models.triage_api import TriageAnswerRequest, TriageSkipRequest, TriageTimeoutRequest
from ..dependencies import get_g8ee_case_data_service, get_g8ee_investigation_service, require_proxy_auth
from ..services.investigation.investigation_service import InvestigationService
from ..services.investigation.investigation_data_service import InvestigationDataService
from ..services.data.case_data_service import CaseDataService

router = APIRouter()
logger = logging.getLogger(__name__)


def _is_chat_session_active(status: InvestigationStatus) -> bool:
    return status == InvestigationStatus.OPEN


@router.post("/chat/triage/answer")
async def answer_triage_question(
    request: TriageAnswerRequest,
    investigation_service: InvestigationService = Depends(get_g8ee_investigation_service),
    user_info: AuthenticatedUser = Depends(require_proxy_auth)
) -> dict[str, bool]:
    """
    Receive user answer to a triage clarifying question and store in ledger.
    """
    investigation = await investigation_service.get_investigation(request.investigation_id)
    if not investigation or investigation.user_id != user_info.uid:
        raise ResourceNotFoundError("Investigation not found", resource_id=request.investigation_id, resource_type="investigation", component="g8ee")

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


@router.post("/chat/triage/skip")
async def skip_triage_questions(
    request: TriageSkipRequest,
    investigation_service: InvestigationService = Depends(get_g8ee_investigation_service),
    user_info: AuthenticatedUser = Depends(require_proxy_auth)
) -> dict[str, bool]:
    """
    Record that user skipped the triage clarifying questions.
    """
    investigation = await investigation_service.get_investigation(request.investigation_id)
    if not investigation or investigation.user_id != user_info.uid:
        raise ResourceNotFoundError("Investigation not found", resource_id=request.investigation_id, resource_type="investigation", component="g8ee")

    await investigation_service.investigation_data_service.add_chat_message(
        investigation_id=request.investigation_id,
        sender=MessageSender.USER_CHAT,
        content="Skipped clarifying questions",
        metadata=ConversationMessageMetadata(
            event_type=EventType.AI_TRIAGE_CLARIFICATION_SKIPPED
        )
    )
    return {"success": True}


@router.post("/chat/triage/timeout")
async def timeout_triage_questions(
    request: TriageTimeoutRequest,
    investigation_service: InvestigationService = Depends(get_g8ee_investigation_service),
    user_info: AuthenticatedUser = Depends(require_proxy_auth)
) -> dict[str, bool]:
    """
    Record that triage clarifying questions timed out.
    """
    investigation = await investigation_service.get_investigation(request.investigation_id)
    if not investigation or investigation.user_id != user_info.uid:
        raise ResourceNotFoundError("Investigation not found", resource_id=request.investigation_id, resource_type="investigation", component="g8ee")

    await investigation_service.investigation_data_service.add_chat_message(
        investigation_id=request.investigation_id,
        sender=MessageSender.USER_CHAT,
        content="Clarifying questions timed out",
        metadata=ConversationMessageMetadata(
            event_type=EventType.AI_TRIAGE_CLARIFICATION_TIMEOUT
        )
    )
    return {"success": True}


@router.get("/chat/sessions/{web_session_id}")
async def get_chat_session(
    web_session_id: str,
    request: Request,
    investigation_service: InvestigationDataService = Depends(get_g8ee_investigation_service),
    user_info: AuthenticatedUser = Depends(require_proxy_auth)
) -> ChatSessionResponse:
    """
    Get chat session information.
    
    SECURITY: Validates that the authenticated user owns the session/investigation.
    
    Args:
        web_session_id: Chat session identifier (investigation_id)
        
    Returns:
        Chat session information including associated case context
    """
    authenticated_user_id = user_info.uid

    investigation = await investigation_service.investigation_data_service.get_investigation(web_session_id)
    if investigation:
        if investigation.user_id != authenticated_user_id:
            raise ResourceNotFoundError("Chat session not found", resource_type="chat_session", resource_id=web_session_id, component="g8ee")

        session_status = ChatSessionStatus.ACTIVE if _is_chat_session_active(investigation.status) else ChatSessionStatus.INACTIVE

        return ChatSessionResponse(
            web_session_id=web_session_id,
            created_at=investigation.created_at,
            status=session_status,
            case_id=investigation.case_id
        )

    raise ResourceNotFoundError("Chat session not found", resource_type="chat_session", resource_id=web_session_id, component="g8ee")


@router.get("/chat/cases/{case_id}/latest-session")
async def get_latest_chat_session_for_case(
    case_id: str,
    request: Request,
    case_service: CaseDataService = Depends(get_g8ee_case_data_service),
    investigation_service: InvestigationDataService = Depends(get_g8ee_investigation_service),
    user_info: AuthenticatedUser = Depends(require_proxy_auth)
) -> LatestChatSessionResponse:
    """
    Get the most recent chat session for a case, including conversation history.

    SECURITY: Validates that the authenticated user owns the case before returning data.

    Args:
        case_id: Case ID to get latest chat session for

    Returns:
        Latest chat session data with conversation history, or null if no session exists
    """
    authenticated_user_id = user_info.uid

    raw_case = await case_service.get_case(case_id)
    if raw_case is None:
        raise ResourceNotFoundError("Case not found", resource_type="case", resource_id=case_id, component="g8ee")
    case_user_id = raw_case.user_id
    if case_user_id != authenticated_user_id:
        raise ResourceNotFoundError("Case not found", resource_type="case", resource_id=case_id, component="g8ee")

    raw_investigations = await investigation_service.investigation_data_service.get_case_investigations(
        case_id=case_id,
        user_id=authenticated_user_id
    )

    latest_investigation: InvestigationModel | None = None
    for inv in raw_investigations:
        if not inv.conversation_history:
            continue
        if latest_investigation is None or (inv.updated_at or inv.created_at) > (latest_investigation.updated_at or latest_investigation.created_at):
            latest_investigation = inv

    if latest_investigation:
        logger.info(
            f"Found latest investigation for case {case_id}: {latest_investigation.id} with {len(latest_investigation.conversation_history)} messages",
            extra={"case_id": case_id, "investigation_id": latest_investigation.id, "message_count": len(latest_investigation.conversation_history), "user_id": authenticated_user_id}
        )

        session_details = ChatSessionDetailsResponse(
            id=latest_investigation.id,
            case_id=case_id,
            investigation_id=latest_investigation.id,
            conversation_history=latest_investigation.conversation_history,
            created_at=latest_investigation.created_at,
            updated_at=latest_investigation.updated_at,
            is_active=_is_chat_session_active(latest_investigation.status)
        )

        return LatestChatSessionResponse(
            success=True,
            session=session_details,
            message=f"Latest investigation found for case {case_id}"
        )

    logger.info(
        f"No investigations with conversation history found for case {case_id}",
        extra={"case_id": case_id, "user_id": authenticated_user_id}
    )

    return LatestChatSessionResponse(
        success=True,
        session=None,
        message=f"No investigations with conversation history found for case {case_id}"
    )

