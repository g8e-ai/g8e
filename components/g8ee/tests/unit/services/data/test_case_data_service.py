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

"""Unit tests for CaseDataService."""

from unittest.mock import MagicMock, patch

import pytest

from app.constants import (
    CaseStatus,
    ComponentName,
    ErrorCode,
    EventType,
    Priority,
    Severity,
    TaskStatus,
)
from app.errors import (
    BusinessLogicError,
    DatabaseError,
    ResourceNotFoundError,
    ValidationError,
)
from app.models import (
    CaseCreateRequest,
    CaseEventPayload,
    CaseUpdateRequest,
)
from app.models.db_queries import CaseHistoryQuery
from app.services.data.case_data_service import CaseDataService

pytestmark = [pytest.mark.unit, pytest.mark.asyncio(loop_scope="session")]

async def mock_awaitable(result):
    return result

async def mock_awaitable_exception(exc):
    raise exc

class TestCaseDataService:
    @pytest.fixture
    def service(self, mock_settings, mock_cache_aside_service, mock_event_service):
        return CaseDataService(
            settings=mock_settings,
            cache=mock_cache_aside_service,
            event_service=mock_event_service
        )

    @pytest.fixture
    def mock_cache(self, mock_cache_aside_service):
        return mock_cache_aside_service

    @pytest.fixture
    def mock_event(self, mock_event_service):
        return mock_event_service

    # --- create_case tests ---

    async def test_create_case_success_with_generated_title(self, service, mock_cache):
        request = CaseCreateRequest(
            user_id="user-123",
            user_email="test@example.com",
            web_session_id="ws-123",
            initial_message="Hello world",
            priority=Priority.MEDIUM,
            severity=Severity.LOW,
            source="web"
        )
        generated_title = "Generated Title"
        mock_cache.create_document.return_value = None

        result = await service.create_case(request, generated_title)

        assert result.title == generated_title
        assert result.description == "Hello world"
        assert result.status == CaseStatus.NEW
        assert result.user_id == "user-123"
        assert result.web_session_id == "ws-123"
        mock_cache.create_document.assert_called_once()

    async def test_create_case_success_derived_title(self, service, mock_cache):
        request = CaseCreateRequest(
            user_id="user-123",
            web_session_id="ws-123",
            initial_message="A very long message " * 10, # "A very long message " is 20 chars, * 10 = 200 chars
            priority=Priority.MEDIUM,
            severity=Severity.LOW,
            source="web"
        )
        mock_cache.create_document.return_value = None

        result = await service.create_case(request, None)

        assert result.title.startswith("A very long message ")
        assert result.title.endswith("...")
        assert len(result.title) <= 103
        mock_cache.create_document.assert_called_once()

    async def test_create_case_success_untitled(self, service, mock_cache):
        request = CaseCreateRequest(
            user_id="user-123",
            web_session_id="ws-123",
            initial_message=" ", # initial_message is required and min_length=1
            priority=Priority.MEDIUM,
            severity=Severity.LOW,
            source="web"
        )
        mock_cache.create_document.return_value = None

        result = await service.create_case(request, None)

        assert result.title == "Untitled Case"
        assert result.description == " "
        mock_cache.create_document.assert_called_once()

    async def test_create_case_db_error(self, service, mock_cache):
        request = CaseCreateRequest(
            user_id="user-123",
            web_session_id="ws-123",
            initial_message="Hello"
        )
        mock_cache.create_document.side_effect = Exception("DB failure")

        with pytest.raises(DatabaseError, match="Failed to create case"):
            await service.create_case(request, "Title")

    async def test_create_case_g8e_error(self, service, mock_cache):
        from app.errors import G8eError
        from app.constants import ErrorCategory
        request = CaseCreateRequest(
            user_id="user-123",
            web_session_id="ws-123",
            initial_message="Hello"
        )
        # Line 128: except G8eError: raise
        mock_cache.create_document.side_effect = G8eError(message="G8e failure", code=ErrorCode.DB_WRITE_ERROR, category=ErrorCategory.DATABASE)

        with pytest.raises(G8eError, match="G8e failure"):
            await service.create_case(request, "Title")

    # --- get_case tests ---

    async def test_get_case_success(self, service, mock_cache):
        case_id = "case-123"
        mock_cache.get_document_with_cache.return_value = {
            "title": "Test Case",
            "description": "Test description",
            "user_id": "user-123",
            "status": CaseStatus.NEW,
            "priority": Priority.MEDIUM,
            "severity": Severity.LOW,
            "source": "web",
            "created_at": "2026-01-01T00:00:00Z",
            "updated_at": "2026-01-01T00:00:00Z",
        }

        result = await service.get_case(case_id)

        assert result.id == case_id
        assert result.title == "Test Case"
        mock_cache.get_document_with_cache.assert_called_once_with(
            collection=service.cases_collection,
            document_id=case_id
        )

    async def test_get_case_empty_id(self, service):
        with pytest.raises(ValidationError, match="Case ID is required"):
            await service.get_case("")

    async def test_get_case_not_found(self, service, mock_cache):
        mock_cache.get_document_with_cache.return_value = None
        with pytest.raises(ResourceNotFoundError, match="Case not found"):
            await service.get_case("missing")

    async def test_get_case_db_error(self, service, mock_cache):
        mock_cache.get_document_with_cache.side_effect = Exception("Fetch error")
        with pytest.raises(DatabaseError, match="Failed to retrieve case"):
            await service.get_case("case-123")

    # --- update_case tests ---

    async def test_update_case_success(self, service, mock_cache):
        case_id = "case-123"
        updates = CaseUpdateRequest(title="Updated Title")

        # Mock get_case (internal call)
        mock_cache.get_document_with_cache.return_value = {
            "title": "Old Title",
            "description": "Old description",
            "user_id": "user-123",
            "status": CaseStatus.NEW,
            "priority": Priority.MEDIUM,
            "severity": Severity.LOW,
            "source": "web",
            "created_at": "2026-01-01T00:00:00Z",
            "updated_at": "2026-01-01T00:00:00Z",
        }
        mock_cache.update_document.return_value = None

        result = await service.update_case(case_id, updates)

        assert result.title == "Updated Title"
        mock_cache.update_document.assert_called_once()

    async def test_update_case_no_updates(self, service):
        with pytest.raises(BusinessLogicError, match="No updates provided"):
            await service.update_case("case-123", CaseUpdateRequest())

    async def test_update_case_db_error(self, service, mock_cache):
        case_id = "case-123"
        mock_cache.get_document_with_cache.return_value = {
            "title": "Test Case",
            "description": "Test description",
            "user_id": "u1",
            "status": CaseStatus.NEW,
            "priority": Priority.MEDIUM,
            "severity": Severity.LOW,
            "source": "web",
            "created_at": "2026-01-01T00:00:00Z",
            "updated_at": "2026-01-01T00:00:00Z",
        }
        mock_cache.update_document.side_effect = Exception("Update fail")

        with pytest.raises(DatabaseError, match="Failed to update case"):
            await service.update_case(case_id, CaseUpdateRequest(title="New"))

    async def test_update_case_g8e_error(self, service, mock_cache):
        from app.errors import G8eError
        from app.constants import ErrorCategory
        case_id = "case-123"
        mock_cache.get_document_with_cache.return_value = {
            "title": "Test Case",
            "description": "Test description",
            "user_id": "u1",
            "status": CaseStatus.NEW,
            "priority": Priority.MEDIUM,
            "severity": Severity.LOW,
            "source": "web",
            "created_at": "2026-01-01T00:00:00Z",
            "updated_at": "2026-01-01T00:00:00Z",
        }
        # Line 226: except G8eError: raise
        mock_cache.update_document.side_effect = G8eError(message="G8e update failure", code=ErrorCode.DB_WRITE_ERROR, category=ErrorCategory.DATABASE)

        with pytest.raises(G8eError, match="G8e update failure"):
            await service.update_case(case_id, CaseUpdateRequest(title="New"))

    # --- delete_case tests ---

    async def test_delete_case_success(self, service, mock_cache):
        case_id = "case-123"
        mock_cache.get_document_with_cache.return_value = {
            "title": "Test Case",
            "description": "Test description",
            "user_id": "u1",
            "status": CaseStatus.NEW,
            "priority": Priority.MEDIUM,
            "severity": Severity.LOW,
            "source": "web",
            "created_at": "2026-01-01T00:00:00Z",
            "updated_at": "2026-01-01T00:00:00Z",
        }

        mock_del_result = MagicMock()
        mock_del_result.success = True
        # db.delete_document is called and its result is awaited
        mock_cache.db.delete_document = MagicMock(side_effect=lambda *args, **kwargs: mock_awaitable(mock_del_result))
        mock_cache.kv.delete = MagicMock(side_effect=lambda *args, **kwargs: mock_awaitable(None))
        mock_cache.invalidate_query_cache = MagicMock(side_effect=lambda *args, **kwargs: mock_awaitable(None))

        await service.delete_case(case_id)

        mock_cache.db.delete_document.assert_called_once()
        mock_cache.kv.delete.assert_called_once()

    async def test_delete_case_failure_result(self, service, mock_cache):
        case_id = "case-123"
        mock_cache.get_document_with_cache.return_value = {
            "title": "Test Case",
            "description": "Test description",
            "user_id": "u1",
            "status": CaseStatus.NEW,
            "priority": Priority.MEDIUM,
            "severity": Severity.LOW,
            "source": "web",
            "created_at": "2026-01-01T00:00:00Z",
            "updated_at": "2026-01-01T00:00:00Z",
        }

        mock_del_result = MagicMock()
        mock_del_result.success = False
        mock_del_result.error = "Delete error"
        mock_cache.db.delete_document = MagicMock(side_effect=lambda *args, **kwargs: mock_awaitable(mock_del_result))

        with pytest.raises(DatabaseError, match="Failed to delete case: Delete error"):
            await service.delete_case(case_id)

    async def test_delete_case_exception(self, service, mock_cache):
        case_id = "case-123"
        mock_cache.get_document_with_cache.return_value = {
            "title": "Test Case",
            "description": "Test description",
            "user_id": "u1",
            "status": CaseStatus.NEW,
            "priority": Priority.MEDIUM,
            "severity": Severity.LOW,
            "source": "web",
            "created_at": "2026-01-01T00:00:00Z",
            "updated_at": "2026-01-01T00:00:00Z",
        }
        mock_cache.db.delete_document = MagicMock(side_effect=lambda *args, **kwargs: mock_awaitable_exception(Exception("Fatal delete")))

        with pytest.raises(DatabaseError, match="Failed to delete case: Fatal delete"):
            await service.delete_case(case_id)

    # --- publish_case_update_sse tests ---

    async def test_publish_case_update_sse_success(self, service, mock_event):
        payload = CaseEventPayload(
            updated_at="2026-01-01T01:00:00Z",
            case_id="case-123",
            status=CaseStatus.NEW
        )
        await service.publish_case_update_sse(
            case_id="case-123",
            web_session_id="ws-123",
            payload=payload,
            user_id="user-123"
        )
        assert len(mock_event.published) == 1

    async def test_publish_case_update_sse_no_service(self, service):
        service.event_service = None
        # Should just log a warning and return
        await service.publish_case_update_sse("c1", "ws1", MagicMock(), "u1")

    async def test_publish_case_update_sse_error(self, service, mock_event):
        # We need to make the publish fail.
        # Since service.event_service is our FakeEventService from mock_event fixture,
        # we can patch its publish method.
        with patch.object(mock_event, "publish", side_effect=Exception("Publish fail")):
            await service.publish_case_update_sse(
                case_id="c1",
                web_session_id="ws1",
                payload=MagicMock(spec=CaseEventPayload),
                user_id="u1"
            )
            # Should catch and log warning, no exception raised

    # --- get_case_history tests ---

    async def test_get_case_history_success(self, service, mock_cache):
        query = CaseHistoryQuery(case_id="case-123", start_time="2026-01-01T00:00:00Z", limit=10)
        mock_cache.query_documents.return_value = [
            {
                "timestamp": "2026-05-06T10:33:31Z",
                "event_type": EventType.CASE_UPDATED,
                "source_component": ComponentName.G8EE,
                "summary": "Case updated",
                "details": {}
            }
        ]

        result = await service.get_case_history(query)
        assert len(result) == 1
        assert result[0].event_type == EventType.CASE_UPDATED
        mock_cache.query_documents.assert_called_once()

    async def test_get_case_history_filters(self, service, mock_cache):
        from app.models.db_queries import CaseHistoryQuery
        # Lines 326, 330, 333: query.start_time, query.end_time, query.event_type
        query = CaseHistoryQuery(
            case_id="case-123",
            start_time="2026-01-01T00:00:00Z",
            end_time="2026-01-02T00:00:00Z",
            event_type=EventType.CASE_UPDATED,
            limit=5
        )
        mock_cache.query_documents.return_value = []

        await service.get_case_history(query)

        # Verify filters were passed
        args, kwargs = mock_cache.query_documents.call_args
        filters = kwargs["field_filters"]
        assert len(filters) == 4 # case_id, start_time, end_time, event_type

    # --- get_case_tasks tests ---

    async def test_get_case_tasks_with_status(self, service, mock_cache):
        # Line 353: if task_status: ...
        mock_cache.query_documents.return_value = []
        await service.get_case_tasks("case-123", TaskStatus.COMPLETED)

        args, kwargs = mock_cache.query_documents.call_args
        filters = kwargs["field_filters"]
        assert len(filters) == 2 # case_id, status

    async def test_get_case_tasks_empty_id(self, service):
        with pytest.raises(ValidationError, match="Case ID is required"):
            await service.get_case_tasks("", TaskStatus.PENDING)
