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

"""Unit tests for AgentActivityDataService."""

from unittest.mock import AsyncMock, MagicMock

import pytest

from app.constants import AgentMode, TriageComplexityClassification, TriageConfidence
from app.errors import ValidationError, DatabaseError
from app.models.agent_activity import AgentActivityMetadata
from app.models.tool_results import TokenUsage
from app.services.data.agent_activity_data_service import AgentActivityDataService

pytestmark = [pytest.mark.unit, pytest.mark.asyncio(loop_scope="session")]


class TestAgentActivityDataService:
    @pytest.fixture
    def service(self, mock_cache_aside_service):
        return AgentActivityDataService(mock_cache_aside_service)

    @pytest.fixture
    def mock_cache(self, mock_cache_aside_service):
        return mock_cache_aside_service

    async def test_record_activity_success(self, service, mock_cache):
        metadata = AgentActivityMetadata(
            user_id="user-123",
            investigation_id="inv-123",
            case_id="case-123",
            agent_mode=AgentMode.OPERATOR_BOUND,
            model_name="gemini-3.1-pro-preview",
            provider="gemini",
            token_usage=TokenUsage(input_tokens=100, output_tokens=50, total_tokens=150),
            finish_reason="stop",
        )

        mock_cache.create_document.return_value = None

        result = await service.record_activity(metadata)

        assert result is not None
        assert result.id is not None
        assert result.user_id == "user-123"
        assert result.investigation_id == "inv-123"
        mock_cache.create_document.assert_called_once()

    async def test_record_activity_generates_id(self, service, mock_cache):
        metadata = AgentActivityMetadata(
            user_id="user-123",
            investigation_id="inv-123",
        )

        mock_cache.create_document.return_value = None

        result = await service.record_activity(metadata)

        assert result.id is not None
        assert len(result.id) > 0

    async def test_get_activity_success(self, service, mock_cache):
        activity_id = "activity-123"
        mock_cache.get_document_with_cache.return_value = {
            "id": activity_id,
            "user_id": "user-123",
            "investigation_id": "inv-123",
            "agent_mode": AgentMode.OPERATOR_BOUND,
            "model_name": "gemini-3.1-pro-preview",
        }

        result = await service.get_activity(activity_id)

        assert result is not None
        assert isinstance(result, AgentActivityMetadata)
        assert result.id == activity_id
        assert result.user_id == "user-123"
        mock_cache.get_document_with_cache.assert_called_once_with(collection=service.collection, document_id=activity_id)

    async def test_get_activity_not_found(self, service, mock_cache):
        mock_cache.get_document_with_cache.return_value = None
        result = await service.get_activity("nonexistent")
        assert result is None

    async def test_get_activity_empty_id_raises_error(self, service):
        with pytest.raises(ValidationError, match="Activity ID is required"):
            await service.get_activity("")

    async def test_query_activities_with_filters(self, service, mock_cache):
        mock_cache.query_documents.return_value = [
            {
                "id": "activity-1",
                "user_id": "user-123",
                "investigation_id": "inv-123",
                "model_name": "gemini-3.1-pro-preview",
            },
            {
                "id": "activity-2",
                "user_id": "user-123",
                "investigation_id": "inv-456",
                "model_name": "gemini-3.1-pro-preview",
            },
        ]

        results = await service.query_activities(user_id="user-123")

        assert len(results) == 2
        assert all(isinstance(r, AgentActivityMetadata) for r in results)
        mock_cache.query_documents.assert_called_once()

    async def test_query_activities_with_model_filter(self, service, mock_cache):
        mock_cache.query_documents.return_value = [
            {
                "id": "activity-1",
                "user_id": "user-123",
                "model_name": "gemini-3.1-pro-preview",
            },
        ]

        results = await service.query_activities(model_name="gemini-3.1-pro-preview")

        assert len(results) == 1
        assert results[0].model_name == "gemini-3.1-pro-preview"

    async def test_delete_activity_success(self, service, mock_cache):
        activity_id = "activity-123"
        mock_result = MagicMock()
        mock_result.success = True
        mock_cache.delete_document.return_value = mock_result

        await service.delete_activity(activity_id)

        mock_cache.delete_document.assert_called_once_with(collection=service.collection, document_id=activity_id)

    async def test_delete_activity_empty_id_raises_error(self, service):
        with pytest.raises(ValidationError, match="Activity ID is required"):
            await service.delete_activity("")

    async def test_query_activities_handles_db_error(self, service, mock_cache):
        mock_cache.query_documents.side_effect = Exception("DB error")

        with pytest.raises(DatabaseError, match="Failed to query agent activity metadata"):
            await service.query_activities(user_id="user-123")

    async def test_record_activity_handles_db_error(self, service, mock_cache):
        metadata = AgentActivityMetadata(user_id="user-123")
        mock_cache.create_document.side_effect = Exception("DB error")

        with pytest.raises(DatabaseError, match="Failed to record agent activity metadata"):
            await service.record_activity(metadata)
