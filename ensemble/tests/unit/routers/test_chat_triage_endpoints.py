# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

from unittest.mock import AsyncMock, MagicMock

import pytest
from fastapi import Request

from app.constants import MessageSender, ComponentName
from app.constants.generated_status import EventType
from app.models.http_context import RequestContext, G8eHttpContext
from app.models.triage_api import TriageAnswerRequest, TriageSkipRequest, TriageTimeoutRequest
from app.routers.chat_router import (
    answer_triage_question,
    skip_triage_questions,
    timeout_triage_questions,
)
from tests.fakes.factories import create_investigation_data

pytestmark = [pytest.mark.unit]


@pytest.mark.asyncio(loop_scope="session")
class TestTriageEndpoints:
    """Test triage interaction endpoints."""

    async def test_answer_triage_question_persists_message(self):
        MagicMock(spec=Request)
        mock_investigation_service = MagicMock()
        mock_chat_pipeline = MagicMock()
        mock_chat_pipeline.run_chat = AsyncMock()
        mock_chat_task_manager = MagicMock()
        MagicMock()

        investigation_id = "inv-123"
        user_id = "user-456"

        investigation = create_investigation_data(
            investigation_id=investigation_id, user_id=user_id
        )
        mock_investigation_service.get_investigation = AsyncMock(return_value=investigation)
        mock_investigation_service.investigation_data_service.add_chat_message = AsyncMock(
            return_value=True
        )

        g8e_context = G8eHttpContext(
            user_id=user_id,
            web_session_id=investigation_id,
            organization_id="org-789",
            source_component=ComponentName.CLIENT,
        )
        payload = TriageAnswerRequest(
            investigation_id=investigation_id,
            question_index=1,
            answer=True,
            context=RequestContext(
                investigation_id=investigation_id,
                case_id="case-123",
                web_session_id=investigation_id,
                user_id=user_id,
                source_component=ComponentName.CLIENT,
            ),
        )

        result = await answer_triage_question(
            request=payload,
            investigation_service=mock_investigation_service,
            chat_pipeline=mock_chat_pipeline,
            chat_task_manager=mock_chat_task_manager,
            settings_service=AsyncMock(),
            g8e_context=g8e_context,
        )

        assert result == {"success": True}
        mock_investigation_service.investigation_data_service.add_chat_message.assert_called_once()
        mock_chat_pipeline.run_chat.assert_called_once()
        _args, kwargs = (
            mock_investigation_service.investigation_data_service.add_chat_message.call_args
        )
        assert kwargs["sender"] == MessageSender.USER_CHAT
        assert kwargs["metadata"].event_type == EventType.AI_TRIAGE_CLARIFICATION_ANSWERED
        assert kwargs["metadata"].question_index == 1
        assert kwargs["metadata"].answer is True

    async def test_skip_triage_questions_persists_message(self):
        mock_investigation_service = MagicMock()
        mock_chat_pipeline = MagicMock()
        mock_chat_pipeline.run_chat = AsyncMock()
        mock_chat_task_manager = MagicMock()
        MagicMock()

        investigation_id = "inv-123"
        user_id = "user-456"

        investigation = create_investigation_data(
            investigation_id=investigation_id, user_id=user_id
        )
        mock_investigation_service.get_investigation = AsyncMock(return_value=investigation)
        mock_investigation_service.investigation_data_service.add_chat_message = AsyncMock(
            return_value=True
        )

        g8e_context = G8eHttpContext(
            user_id=user_id,
            web_session_id=investigation_id,
            organization_id="org-789",
            source_component=ComponentName.CLIENT,
        )
        payload = TriageSkipRequest(
            investigation_id=investigation_id,
            context=RequestContext(
                investigation_id=investigation_id,
                case_id="case-123",
                web_session_id=investigation_id,
                user_id=user_id,
                source_component=ComponentName.CLIENT,
            ),
        )

        result = await skip_triage_questions(
            request=payload,
            investigation_service=mock_investigation_service,
            chat_pipeline=mock_chat_pipeline,
            chat_task_manager=mock_chat_task_manager,
            settings_service=AsyncMock(),
            g8e_context=g8e_context,
        )

        assert result == {"success": True}
        mock_investigation_service.investigation_data_service.add_chat_message.assert_called_once()
        mock_chat_pipeline.run_chat.assert_called_once()
        _args, kwargs = (
            mock_investigation_service.investigation_data_service.add_chat_message.call_args
        )
        assert kwargs["metadata"].event_type == EventType.AI_TRIAGE_CLARIFICATION_SKIPPED

    async def test_timeout_triage_questions_persists_message(self):
        mock_investigation_service = MagicMock()
        investigation_id = "inv-123"
        user_id = "user-456"

        investigation = create_investigation_data(
            investigation_id=investigation_id, user_id=user_id
        )
        mock_investigation_service.get_investigation = AsyncMock(return_value=investigation)
        mock_investigation_service.investigation_data_service.add_chat_message = AsyncMock(
            return_value=True
        )

        g8e_context = G8eHttpContext(
            user_id=user_id,
            web_session_id=investigation_id,
            organization_id="org-789",
            source_component=ComponentName.CLIENT,
            investigation_id=investigation_id,
        )
        payload = TriageTimeoutRequest(
            investigation_id=investigation_id,
            context=RequestContext(
                investigation_id=investigation_id,
                case_id="case-123",
                source_component=ComponentName.CLIENT,
            ),
        )

        result = await timeout_triage_questions(
            request=payload,
            investigation_service=mock_investigation_service,
            g8e_context=g8e_context,
        )

        assert result == {"success": True}
        mock_investigation_service.investigation_data_service.add_chat_message.assert_called_once()
        _args, kwargs = (
            mock_investigation_service.investigation_data_service.add_chat_message.call_args
        )
        assert kwargs["metadata"].event_type == EventType.AI_TRIAGE_CLARIFICATION_TIMEOUT
