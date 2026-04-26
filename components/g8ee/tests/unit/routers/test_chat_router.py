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

from app.constants import ChatSessionStatus, InvestigationStatus, AuthMethod
from app.constants.events import EventType
from app.errors import ResourceNotFoundError
from app.routers.chat_router import router
from tests.fakes.factories import (
    build_case_model,
    build_authenticated_user,
    create_conversation_message,
    create_investigation_data,
)

pytestmark = [pytest.mark.unit]


@pytest.mark.asyncio(loop_scope="session")
class TestGetChatSession:
    """Test GET /chat/sessions/{web_session_id} endpoint."""

    async def test_get_chat_session_returns_session_info(self):
        """Test get chat session returns session information."""
        from app.routers.chat_router import get_chat_session

        mock_request = MagicMock(spec=Request)
        mock_investigation_service = MagicMock()
        web_session_id = "session-123"
        user_id = "user-456"

        investigation = create_investigation_data(
            investigation_id=web_session_id,
            user_id=user_id,
            case_id="case-789",
        )
        mock_investigation_service.investigation_data_service.get_investigation = AsyncMock(return_value=investigation)

        result = await get_chat_session(
            web_session_id=web_session_id,
            request=mock_request,
            investigation_service=mock_investigation_service,
            user_info=build_authenticated_user(
                uid=user_id,
                user_id=user_id,
                email="user@example.com",
                organization_id="org-789",
                web_session_id=web_session_id,
                auth_method=AuthMethod.TEST
            ),
        )

        assert result.web_session_id == web_session_id
        assert result.status == ChatSessionStatus.ACTIVE
        assert result.created_at is not None

    async def test_get_chat_session_wrong_user_raises_not_found(self):
        """Test ResourceNotFoundError when session belongs to a different user."""
        from app.routers.chat_router import get_chat_session

        mock_request = MagicMock(spec=Request)
        mock_investigation_service = MagicMock()
        owner_id = "user-owner"
        other_id = "user-other"

        investigation = create_investigation_data(
            investigation_id="session-123",
            user_id=owner_id,
            case_id="case-789",
        )
        mock_investigation_service.investigation_data_service.get_investigation = AsyncMock(return_value=investigation)

        with pytest.raises(ResourceNotFoundError) as exc_info:
            await get_chat_session(
                web_session_id="session-123",
                request=mock_request,
                investigation_service=mock_investigation_service,
                user_info=build_authenticated_user(
                    uid=other_id,
                    user_id=other_id,
                    email="other@example.com",
                    organization_id="org-789",
                    web_session_id="session-123",
                    auth_method=AuthMethod.TEST
                ),
            )

        assert exc_info.value.get_http_status() == 404

    async def test_get_chat_session_not_found_raises(self):
        """Test ResourceNotFoundError when investigation does not exist."""
        from app.routers.chat_router import get_chat_session

        mock_request = MagicMock(spec=Request)
        mock_investigation_service = MagicMock()
        mock_investigation_service.investigation_data_service.get_investigation = AsyncMock(return_value=None)

        with pytest.raises(ResourceNotFoundError) as exc_info:
            await get_chat_session(
                web_session_id="session-missing",
                request=mock_request,
                investigation_service=mock_investigation_service,
                user_info=build_authenticated_user(
                    uid="user-456",
                    user_id="user-456",
                    email="user@example.com",
                    organization_id="org-789",
                    web_session_id="session-missing",
                    auth_method=AuthMethod.TEST
                ),
            )

        assert exc_info.value.get_http_status() == 404

    async def test_get_chat_session_inactive_when_closed(self):
        """Test session status is INACTIVE when investigation is CLOSED."""
        from app.routers.chat_router import get_chat_session

        mock_request = MagicMock(spec=Request)
        mock_investigation_service = MagicMock()
        user_id = "user-456"

        investigation = create_investigation_data(
            investigation_id="session-closed",
            user_id=user_id,
            case_id="case-789",
            status=InvestigationStatus.CLOSED,
        )
        mock_investigation_service.investigation_data_service.get_investigation = AsyncMock(return_value=investigation)

        result = await get_chat_session(
            web_session_id="session-closed",
            request=mock_request,
            investigation_service=mock_investigation_service,
            user_info=build_authenticated_user(
                uid=user_id,
                user_id=user_id,
                email="user@example.com",
                organization_id="org-789",
                web_session_id="session-closed",
                auth_method=AuthMethod.TEST
            ),
        )

        assert result.status == ChatSessionStatus.INACTIVE


@pytest.mark.asyncio(loop_scope="session")
class TestGetLatestChatSessionForCase:
    """Test GET /chat/cases/{case_id}/latest-session endpoint."""

    async def test_get_latest_session_with_investigations(self):
        """Test getting latest session when investigations exist."""
        from app.routers.chat_router import get_latest_chat_session_for_case

        mock_request = MagicMock(spec=Request)
        mock_case_service = MagicMock()
        mock_investigation_service = MagicMock()
        user_id = "user-456"

        case = build_case_model(case_id="case-789", user_id=user_id, title="Test Case")
        mock_case_service.get_case = AsyncMock(return_value=case)

        investigation = create_investigation_data(
            investigation_id="inv-123",
            case_id="case-789",
            user_id=user_id,
        )
        investigation.conversation_history = [
            create_conversation_message(sender=EventType.EVENT_SOURCE_USER_CHAT, content="Hello")
        ]
        mock_investigation_service.investigation_data_service.get_case_investigations = AsyncMock(return_value=[investigation])

        result = await get_latest_chat_session_for_case(
            case_id="case-789",
            request=mock_request,
            case_service=mock_case_service,
            investigation_service=mock_investigation_service,
            user_info=build_authenticated_user(
                uid=user_id,
                user_id=user_id,
                email="user@example.com",
                organization_id="org-789",
                web_session_id="session-123",
                auth_method=AuthMethod.TEST
            ),
        )

        assert result.success is True
        assert result.session is not None
        assert result.session.investigation_id == "inv-123"
        assert len(result.session.conversation_history) == 1

    async def test_get_latest_session_no_investigations(self):
        """Test getting latest session when no investigations exist."""
        from app.routers.chat_router import get_latest_chat_session_for_case

        mock_request = MagicMock(spec=Request)
        mock_case_service = MagicMock()
        mock_investigation_service = MagicMock()
        user_id = "user-456"

        case = build_case_model(case_id="case-empty", user_id=user_id, title="Empty Case")
        mock_case_service.get_case = AsyncMock(return_value=case)
        mock_investigation_service.investigation_data_service.get_case_investigations = AsyncMock(return_value=[])

        result = await get_latest_chat_session_for_case(
            case_id="case-empty",
            request=mock_request,
            case_service=mock_case_service,
            investigation_service=mock_investigation_service,
            user_info=build_authenticated_user(
                uid=user_id,
                user_id=user_id,
                email="user@example.com",
                organization_id="org-789",
                web_session_id="session-123",
                auth_method=AuthMethod.TEST
            ),
        )

        assert result.success is True
        assert result.session is None
        assert "No investigations" in result.message

    async def test_get_latest_session_finds_most_recent(self):
        """Test that endpoint returns the most recent investigation."""
        from app.routers.chat_router import get_latest_chat_session_for_case

        mock_request = MagicMock(spec=Request)
        mock_case_service = MagicMock()
        mock_investigation_service = MagicMock()
        user_id = "user-456"

        case = build_case_model(case_id="case-789", user_id=user_id, title="Test Case")
        mock_case_service.get_case = AsyncMock(return_value=case)

        inv_old = create_investigation_data(investigation_id="inv-old", case_id="case-789", user_id=user_id)
        inv_old.conversation_history = [create_conversation_message(sender=EventType.EVENT_SOURCE_USER_CHAT, content="Old")]
        inv_new = create_investigation_data(investigation_id="inv-new", case_id="case-789", user_id=user_id)
        inv_new.conversation_history = [create_conversation_message(sender=EventType.EVENT_SOURCE_USER_CHAT, content="New")]
        mock_investigation_service.investigation_data_service.get_case_investigations = AsyncMock(
            return_value=[inv_old, inv_new]
        )

        result = await get_latest_chat_session_for_case(
            case_id="case-789",
            request=mock_request,
            case_service=mock_case_service,
            investigation_service=mock_investigation_service,
            user_info=build_authenticated_user(
                uid=user_id,
                user_id=user_id,
                email="user@example.com",
                organization_id="org-789",
                web_session_id="session-123",
                auth_method=AuthMethod.TEST
            ),
        )

        assert result.session is not None
        assert result.session.investigation_id == "inv-new"

    async def test_get_latest_session_case_not_found(self):
        """Test error raised when case does not exist."""
        from app.routers.chat_router import get_latest_chat_session_for_case

        mock_request = MagicMock(spec=Request)
        mock_case_service = MagicMock()
        mock_investigation_service = MagicMock()
        user_id = "user-456"
        mock_case_service.get_case = AsyncMock(return_value=None)

        with pytest.raises(ResourceNotFoundError):
            await get_latest_chat_session_for_case(
                case_id="case-missing",
                request=mock_request,
                case_service=mock_case_service,
                investigation_service=mock_investigation_service,
                user_info=build_authenticated_user(
                    uid=user_id,
                    user_id=user_id,
                    email="user@example.com",
                    organization_id="org-789",
                    web_session_id="session-123",
                    auth_method=AuthMethod.TEST
                ),
            )

    async def test_get_latest_session_wrong_user_raises_not_found(self):
        """Test ResourceNotFoundError when case belongs to a different user."""
        from app.routers.chat_router import get_latest_chat_session_for_case

        mock_request = MagicMock(spec=Request)
        mock_case_service = MagicMock()
        mock_investigation_service = MagicMock()
        owner_id = "user-owner"
        other_id = "user-other"

        case = build_case_model(case_id="case-789", user_id=owner_id, title="Owned Case")
        mock_case_service.get_case = AsyncMock(return_value=case)

        with pytest.raises(ResourceNotFoundError) as exc_info:
            await get_latest_chat_session_for_case(
                case_id="case-789",
                request=mock_request,
                case_service=mock_case_service,
                investigation_service=mock_investigation_service,
                user_info=build_authenticated_user(
                    uid=other_id,
                    user_id=other_id,
                    email="other@example.com",
                    organization_id="org-789",
                    web_session_id="session-123",
                    auth_method=AuthMethod.TEST
                ),
            )

        assert exc_info.value.get_http_status() == 404


class TestChatRouterConfiguration:
    """Test chat router configuration."""

    def test_router_has_chat_endpoint(self):
        """Test router has /chat endpoint."""
        routes = [route.path for route in router.routes]
        assert any("chat" in path for path in routes)

    def test_router_has_session_endpoints(self):
        """Test router has session-related endpoints."""
        routes = [route.path for route in router.routes]
        assert any("sessions" in path for path in routes)

    def test_session_endpoints_are_get(self):
        """Test session endpoints accept GET method."""
        for route in router.routes:
            if "sessions" in route.path or "latest-session" in route.path:
                if hasattr(route, "methods"):
                    assert "GET" in route.methods
