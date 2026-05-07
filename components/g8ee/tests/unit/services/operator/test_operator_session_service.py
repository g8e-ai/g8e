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

"""Unit tests for OperatorSessionService."""

from unittest.mock import AsyncMock, MagicMock
import pytest
import uuid

from app.constants import (
    DB_COLLECTION_OPERATOR_SESSIONS,
    SessionType,
    SessionEndReason,
)
from app.models.sessions import OperatorSessionDocument
from app.services.cache.cache_aside import CacheAsideService
from app.services.operator.operator_session_service import OperatorSessionService
from app.utils.timestamp import now, add_seconds

pytestmark = [pytest.mark.unit, pytest.mark.asyncio(loop_scope="session")]

class TestOperatorSessionService:
    @pytest.fixture
    def mock_cache(self):
        return AsyncMock(spec=CacheAsideService)

    @pytest.fixture
    def session_service(self, mock_cache):
        return OperatorSessionService(cache_aside=mock_cache)

    async def test_create_operator_session_happy(self, session_service, mock_cache):
        # Setup
        session_data = {
            "user_id": "user-123",
            "organization_id": "org-456",
            "user_data": {"name": "Test User"},
            "api_key": "g8e_test_key",
            "operator_id": "op-789",
            "operator_status": "offline",
            "metadata": {"version": "1.0.0"}
        }
        request_context = {
            "ip": "1.2.3.4",
            "user_agent": "g8e-daemon/1.0",
            "login_method": "api_key"
        }

        mock_cache.create_document.return_value = MagicMock(success=True)

        # Execute
        session = await session_service.create_operator_session(
            session_data=session_data,
            request_context=request_context
        )

        # Assert
        assert session.id.startswith("ops_")
        assert session.user_id == "user-123"
        assert session.operator_id == "op-789"
        assert session.client_ip == "1.2.3.4"
        assert session.user_agent == "g8e-daemon/1.0"
        assert session.is_active is True
        
        mock_cache.create_document.assert_called_once_with(
            collection=DB_COLLECTION_OPERATOR_SESSIONS,
            document_id=session.id,
            data=session
        )

    async def test_create_operator_session_custom_ttl(self, session_service, mock_cache):
        # Setup
        session_data = {"user_id": "u1", "operator_id": "o1"}
        mock_cache.create_document.return_value = MagicMock(success=True)
        ttl = 300

        # Execute
        session = await session_service.create_operator_session(
            session_data=session_data,
            ttl_seconds=ttl
        )

        # Assert
        # absolute_expires_at and idle_expires_at should be roughly now + 300
        expected_expiry = add_seconds(session.created_at, ttl)
        assert session.absolute_expires_at == expected_expiry
        assert session.idle_expires_at == expected_expiry

    async def test_create_operator_session_failure(self, session_service, mock_cache):
        # Setup
        session_data = {"user_id": "u1", "operator_id": "o1"}
        mock_cache.create_document.return_value = MagicMock(success=False, error="DB Error")

        # Execute & Assert
        with pytest.raises(Exception) as exc:
            await session_service.create_operator_session(session_data=session_data)
        assert "Failed to persist operator session" in str(exc.value)

    async def test_validate_session_happy(self, session_service, mock_cache):
        # Setup
        session_id = "ops_123"
        ts = now()
        session_dict = {
            "id": session_id,
            "session_type": SessionType.OPERATOR,
            "user_id": "u1",
            "operator_id": "o1",
            "is_active": True,
            "created_at": ts.isoformat(),
            "absolute_expires_at": add_seconds(ts, 3600).isoformat(),
            "idle_expires_at": add_seconds(ts, 3600).isoformat(),
            "last_activity": ts.isoformat()
        }
        mock_cache.get_document_with_cache.return_value = session_dict

        # Execute
        session = await session_service.validate_session(session_id)

        # Assert
        assert session is not None
        assert session.id == session_id
        assert session.is_active is True

    async def test_validate_session_not_found(self, session_service, mock_cache):
        mock_cache.get_document_with_cache.return_value = None
        assert await session_service.validate_session("missing") is None

    async def test_validate_session_empty_id(self, session_service):
        assert await session_service.validate_session("") is None

    async def test_validate_session_inactive(self, session_service, mock_cache):
        # Setup
        session_id = "ops_123"
        session_dict = {
            "id": session_id,
            "session_type": SessionType.OPERATOR,
            "user_id": "u1",
            "operator_id": "o1",
            "is_active": False,
            "absolute_expires_at": add_seconds(now(), 3600).isoformat(),
            "idle_expires_at": add_seconds(now(), 3600).isoformat()
        }
        mock_cache.get_document_with_cache.return_value = session_dict

        # Execute & Assert
        assert await session_service.validate_session(session_id) is None

    async def test_validate_session_absolute_timeout(self, session_service, mock_cache):
        # Setup
        session_id = "ops_123"
        ts = now()
        # Expired in the past
        session_dict = {
            "id": session_id,
            "session_type": SessionType.OPERATOR,
            "user_id": "u1",
            "operator_id": "o1",
            "is_active": True,
            "absolute_expires_at": add_seconds(ts, -60).isoformat(),
            "idle_expires_at": add_seconds(ts, 3600).isoformat()
        }
        mock_cache.get_document_with_cache.return_value = session_dict
        mock_cache.delete_document.return_value = MagicMock(success=True)

        # Execute
        session = await session_service.validate_session(session_id)

        # Assert
        assert session is None
        mock_cache.delete_document.assert_called_once_with(
            collection=DB_COLLECTION_OPERATOR_SESSIONS,
            document_id=session_id
        )

    async def test_validate_session_idle_timeout(self, session_service, mock_cache):
        # Setup
        session_id = "ops_123"
        ts = now()
        # Idle timeout in the past
        session_dict = {
            "id": session_id,
            "session_type": SessionType.OPERATOR,
            "user_id": "u1",
            "operator_id": "o1",
            "is_active": True,
            "absolute_expires_at": add_seconds(ts, 3600).isoformat(),
            "idle_expires_at": add_seconds(ts, -60).isoformat()
        }
        mock_cache.get_document_with_cache.return_value = session_dict
        mock_cache.delete_document.return_value = MagicMock(success=True)

        # Execute
        session = await session_service.validate_session(session_id)

        # Assert
        assert session is None
        mock_cache.delete_document.assert_called_once_with(
            collection=DB_COLLECTION_OPERATOR_SESSIONS,
            document_id=session_id
        )

    async def test_refresh_session_happy(self, session_service, mock_cache):
        # Setup
        session_id = "ops_123"
        mock_cache.update_document.return_value = MagicMock(success=True)
        
        # We need a session object to pass or it will call validate_session
        session = MagicMock(spec=OperatorSessionDocument)

        # Execute
        success = await session_service.refresh_session(session_id, session=session)

        # Assert
        assert success is True
        mock_cache.update_document.assert_called_once()
        call_args = mock_cache.update_document.call_args[1]
        assert call_args["collection"] == DB_COLLECTION_OPERATOR_SESSIONS
        assert call_args["document_id"] == session_id
        assert "last_activity" in call_args["data"]
        assert "idle_expires_at" in call_args["data"]

    async def test_refresh_session_via_validation(self, session_service, mock_cache):
        # Setup
        session_id = "ops_123"
        ts = now()
        session_dict = {
            "id": session_id,
            "session_type": SessionType.OPERATOR,
            "user_id": "u1",
            "operator_id": "o1",
            "is_active": True,
            "absolute_expires_at": add_seconds(ts, 3600).isoformat(),
            "idle_expires_at": add_seconds(ts, 3600).isoformat()
        }
        mock_cache.get_document_with_cache.return_value = session_dict
        mock_cache.update_document.return_value = MagicMock(success=True)

        # Execute
        success = await session_service.refresh_session(session_id)

        # Assert
        assert success is True
        mock_cache.get_document_with_cache.assert_called_once()
        mock_cache.update_document.assert_called_once()

    async def test_refresh_session_invalid_session(self, session_service, mock_cache):
        # Setup
        session_id = "ops_123"
        mock_cache.get_document_with_cache.return_value = None

        # Execute
        success = await session_service.refresh_session(session_id)

        # Assert
        assert success is False

    async def test_end_session_happy(self, session_service, mock_cache):
        # Setup
        session_id = "ops_123"
        mock_cache.delete_document.return_value = MagicMock(success=True)

        # Execute
        success = await session_service.end_session(session_id, reason=SessionEndReason.LOGOUT)

        # Assert
        assert success is True
        mock_cache.delete_document.assert_called_once_with(
            collection=DB_COLLECTION_OPERATOR_SESSIONS,
            document_id=session_id
        )

    async def test_end_session_failure(self, session_service, mock_cache):
        # Setup
        session_id = "ops_123"
        mock_cache.delete_document.return_value = MagicMock(success=False)

        # Execute
        success = await session_service.end_session(session_id)

        # Assert
        assert success is False
