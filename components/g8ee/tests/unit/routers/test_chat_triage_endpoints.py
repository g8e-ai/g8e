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

from unittest.mock import AsyncMock, MagicMock
import pytest
from fastapi import Request
from app.constants import AuthMethod, MessageSender
from app.constants.events import EventType
from app.models.triage_api import TriageAnswerRequest, TriageSkipRequest, TriageTimeoutRequest
from app.routers.chat_router import answer_triage_question, skip_triage_questions, timeout_triage_questions
from tests.fakes.factories import build_authenticated_user, create_investigation_data

pytestmark = [pytest.mark.unit]

@pytest.mark.asyncio(loop_scope="session")
class TestTriageEndpoints:
    """Test triage interaction endpoints."""

    async def test_answer_triage_question_persists_message(self):
        mock_request = MagicMock(spec=Request)
        mock_investigation_service = MagicMock()
        investigation_id = "inv-123"
        user_id = "user-456"

        investigation = create_investigation_data(investigation_id=investigation_id, user_id=user_id)
        mock_investigation_service.get_investigation = AsyncMock(return_value=investigation)
        mock_investigation_service.investigation_data_service.add_chat_message = AsyncMock(return_value=True)

        user_info = build_authenticated_user(
            uid=user_id,
            user_id=user_id,
            email="user@example.com",
            organization_id="org-789",
            web_session_id=investigation_id,
            auth_method=AuthMethod.TEST
        )
        payload = TriageAnswerRequest(investigation_id=investigation_id, question_index=1, answer=True)

        result = await answer_triage_question(
            request=payload,
            investigation_service=mock_investigation_service,
            user_info=user_info
        )

        assert result == {"success": True}
        mock_investigation_service.investigation_data_service.add_chat_message.assert_called_once()
        args, kwargs = mock_investigation_service.investigation_data_service.add_chat_message.call_args
        assert kwargs["sender"] == MessageSender.USER_CHAT
        assert kwargs["metadata"].event_type == EventType.AI_TRIAGE_CLARIFICATION_ANSWERED
        assert kwargs["metadata"].question_index == 1
        assert kwargs["metadata"].answer is True

    async def test_skip_triage_questions_persists_message(self):
        mock_investigation_service = MagicMock()
        investigation_id = "inv-123"
        user_id = "user-456"

        investigation = create_investigation_data(investigation_id=investigation_id, user_id=user_id)
        mock_investigation_service.get_investigation = AsyncMock(return_value=investigation)
        mock_investigation_service.investigation_data_service.add_chat_message = AsyncMock(return_value=True)

        user_info = build_authenticated_user(
            uid=user_id,
            user_id=user_id,
            email="user@example.com",
            organization_id="org-789",
            web_session_id=investigation_id,
            auth_method=AuthMethod.TEST
        )
        payload = TriageSkipRequest(investigation_id=investigation_id)

        result = await skip_triage_questions(
            request=payload,
            investigation_service=mock_investigation_service,
            user_info=user_info
        )

        assert result == {"success": True}
        mock_investigation_service.investigation_data_service.add_chat_message.assert_called_once()
        args, kwargs = mock_investigation_service.investigation_data_service.add_chat_message.call_args
        assert kwargs["metadata"].event_type == EventType.AI_TRIAGE_CLARIFICATION_SKIPPED

    async def test_timeout_triage_questions_persists_message(self):
        mock_investigation_service = MagicMock()
        investigation_id = "inv-123"
        user_id = "user-456"

        investigation = create_investigation_data(investigation_id=investigation_id, user_id=user_id)
        mock_investigation_service.get_investigation = AsyncMock(return_value=investigation)
        mock_investigation_service.investigation_data_service.add_chat_message = AsyncMock(return_value=True)

        user_info = build_authenticated_user(
            uid=user_id,
            user_id=user_id,
            email="user@example.com",
            organization_id="org-789",
            web_session_id=investigation_id,
            auth_method=AuthMethod.TEST
        )
        payload = TriageTimeoutRequest(investigation_id=investigation_id)

        result = await timeout_triage_questions(
            request=payload,
            investigation_service=mock_investigation_service,
            user_info=user_info
        )

        assert result == {"success": True}
        mock_investigation_service.investigation_data_service.add_chat_message.assert_called_once()
        args, kwargs = mock_investigation_service.investigation_data_service.add_chat_message.call_args
        assert kwargs["metadata"].event_type == EventType.AI_TRIAGE_CLARIFICATION_TIMEOUT
