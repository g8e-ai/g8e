# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

"""Unit tests for AgentActivityDataService."""

from unittest.mock import AsyncMock, MagicMock

import pytest

from app.constants import AgentMode
from app.errors import DatabaseError, ValidationError
from app.models.agent_activity import AgentActivityMetadata
from app.models.command_request_payloads import DocumentUpdateRequestPayload
from app.models.http_context import RequestContext
from app.models.tool_results import TokenUsage
from app.services.data.agent_activity_data_service import AgentActivityDataService

pytestmark = [pytest.mark.unit, pytest.mark.asyncio(loop_scope="session")]


class TestAgentActivityDataService:
    @pytest.fixture
    def service(self, mock_cache_aside_service, mock_governance_client):
        return AgentActivityDataService(mock_cache_aside_service, mock_governance_client)

    @pytest.fixture
    def mock_cache(self, mock_cache_aside_service):
        return mock_cache_aside_service

    @pytest.fixture
    def context(self):
        return RequestContext(web_session_id="web-session-123", user_id="user-123")

    async def test_record_activity_submits_governed_document_update(
        self, mock_cache_aside_service
    ):
        governance_client = AsyncMock()
        service = AgentActivityDataService(mock_cache_aside_service, governance_client)
        metadata = AgentActivityMetadata(
            id="activity-123",
            user_id="user-123",
            investigation_id="inv-123",
            case_id="case-123",
        )
        context = RequestContext(
            web_session_id="web-session-123",
            user_id="user-123",
            case_id="case-123",
            investigation_id="inv-123",
            task_id="chat",
            operator_id="operator-123",
            operator_session_id="operator-session-123",
        )

        result = await service.record_activity(metadata, context)

        assert result == metadata
        governance_client.submit_envelope.assert_awaited_once()
        message = governance_client.submit_envelope.await_args.args[0]
        assert message.event_type == "g8e.v1.app.agent.activity.recorded"
        assert message.case_id == context.case_id
        assert message.investigation_id == context.investigation_id
        assert message.task_id == context.task_id
        assert message.web_session_id == context.web_session_id
        assert message.user_id == context.user_id
        assert message.operator_id == context.operator_id
        assert message.operator_session_id == context.operator_session_id
        assert isinstance(message.payload, DocumentUpdateRequestPayload)
        assert message.payload.collection == service.collection
        assert message.payload.document_id == metadata.id
        assert message.payload.updates == metadata.model_dump(mode="json")
        assert message.payload.merge is False
        mock_cache_aside_service.create_document.assert_not_called()

    async def test_record_activity_success(self, service, mock_governance_client, context):
        metadata = AgentActivityMetadata(
            user_id="user-123",
            investigation_id="inv-123",
            case_id="case-123",
            agent_mode=AgentMode.G8E_BOUND,
            model_name="gemini-3.1-pro-preview",
            provider="gemini",
            token_usage=TokenUsage(input_tokens=100, output_tokens=50, total_tokens=150),
            finish_reason="stop",
        )

        result = await service.record_activity(metadata, context)

        assert result is not None
        assert result.id is not None
        assert result.user_id == "user-123"
        assert result.investigation_id == "inv-123"
        mock_governance_client.submit_envelope.assert_awaited_once()

    async def test_record_activity_generates_id(self, service, context):
        metadata = AgentActivityMetadata(
            user_id="user-123",
            investigation_id="inv-123",
        )

        result = await service.record_activity(metadata, context)

        assert result.id is not None
        assert len(result.id) > 0

    async def test_get_activity_success(self, service, mock_cache):
        activity_id = "activity-123"
        mock_cache.get_document_with_cache.return_value = {
            "id": activity_id,
            "user_id": "user-123",
            "investigation_id": "inv-123",
            "agent_mode": AgentMode.G8E_BOUND,
            "model_name": "gemini-3.1-pro-preview",
        }

        result = await service.get_activity(activity_id)

        assert result is not None
        assert isinstance(result, AgentActivityMetadata)
        assert result.id == activity_id
        assert result.user_id == "user-123"
        mock_cache.get_document_with_cache.assert_called_once_with(
            collection=service.collection, document_id=activity_id
        )

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

        mock_cache.delete_document.assert_called_once_with(
            collection=service.collection, document_id=activity_id
        )

    async def test_delete_activity_empty_id_raises_error(self, service):
        with pytest.raises(ValidationError, match="Activity ID is required"):
            await service.delete_activity("")

    async def test_query_activities_handles_db_error(self, service, mock_cache):
        mock_cache.query_documents.side_effect = Exception("DB error")

        with pytest.raises(DatabaseError, match="Failed to query agent activity metadata"):
            await service.query_activities(user_id="user-123")

    async def test_record_activity_handles_governance_error(
        self, service, mock_governance_client, context
    ):
        metadata = AgentActivityMetadata(user_id="user-123")
        mock_governance_client.submit_envelope.side_effect = Exception("governance error")

        with pytest.raises(DatabaseError, match="Failed to record agent activity metadata"):
            await service.record_activity(metadata, context)
