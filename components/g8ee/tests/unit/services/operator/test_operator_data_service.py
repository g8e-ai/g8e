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

"""Unit tests for OperatorDataService."""

from unittest.mock import AsyncMock

import pytest

from app.constants import OperatorStatus
from app.errors import ExternalServiceError, ValidationError
from app.models.operators import (
    CommandResultRecord,
    OperatorDocument,
)
from app.services.operator.operator_data_service import OperatorDataService
from app.models.cache import CacheOperationResult
from app.clients.http_client import HTTPClient 
from app.utils.timestamp import now

pytestmark = [pytest.mark.unit, pytest.mark.asyncio(loop_scope="session")]

class TestOperatorDataService:
    @pytest.fixture
    def mock_g8ed_http_client(self):
        return AsyncMock(spec=HTTPClient )

    @pytest.fixture
    def service(self, mock_cache_aside_service, mock_g8ed_http_client):
        from app.models.cache import CacheOperationResult
        return OperatorDataService(mock_cache_aside_service, mock_g8ed_http_client)

    @pytest.fixture
    def mock_cache(self, mock_cache_aside_service):
        return mock_cache_aside_service

    async def test_get_operator_success(self, service, mock_cache):
        operator_id = "op-123"
        mock_cache.get_document.return_value = {
            "operator_id": operator_id,
            "status": OperatorStatus.ACTIVE,
            "system_info": {"hostname": "test-host"}
        }

        result = await service.get_operator(operator_id)

        assert result is not None
        assert isinstance(result, OperatorDocument)
        assert result.operator_id == operator_id
        assert result.status == OperatorStatus.ACTIVE
        mock_cache.get_document.assert_called_once_with(service.collection, operator_id)

    async def test_get_operator_not_found(self, service, mock_cache):
        mock_cache.get_document.return_value
        result = await service.get_operator("nonexistent")
        assert result is None

    async def test_get_operator_empty_id_raises_error(self, service):
        with pytest.raises(ValidationError, match="operator_id is required"):
            await service.get_operator("")

    async def test_update_operator_status(self, service, mock_cache):
        operator_id = "op-123"
        mock_cache.update_document.return_value = CacheOperationResult(success=True)
        mock_cache.get_document.return_value

        success = await service.update_operator_status(operator_id, OperatorStatus.ACTIVE)

        assert success is True
        mock_cache.update_document.assert_called_once()
        _, kwargs = mock_cache.update_document.call_args
        assert kwargs["document_id"] == operator_id
        assert kwargs["data"]["status"] == OperatorStatus.ACTIVE

    async def test_update_operator_status_active_does_not_overwrite_existing_heartbeat(self, service, mock_cache):
        operator_id = "op-hb-existing"
        existing_hb = now()
        mock_cache.get_document.return_value = {
            "operator_id": operator_id,
            "status": OperatorStatus.BOUND,
            "last_heartbeat": existing_hb
        }
        mock_cache.update_document.return_value = CacheOperationResult(success=True)

        await service.update_operator_status(operator_id, OperatorStatus.ACTIVE)

        _, kwargs = mock_cache.update_document.call_args
        assert "last_heartbeat"not in kwargs["data"]

    async def test_update_operator_status_not_found_returns_false(self, service, mock_cache):
        mock_cache.get_document.return_value
        mock_cache.update_document.return_value = CacheOperationResult(success=False)
        
        success = await service.update_operator_status("missing", OperatorStatus.ACTIVE)
        assert success is False

    async def test_update_operator_heartbeat_success(self, service, mock_cache):
        operator_id = "op-123"
        from tests.fakes.factories import build_operator_heartbeat
        hb = build_operator_heartbeat()
        mock_cache.update_document.return_value = CacheOperationResult(success=True)

        success = await service.update_operator_heartbeat(operator_id, hb, investigation_id="inv-123", case_id="case-123")

        assert success is True
        mock_cache.update_document.assert_called_once()
        _, kwargs = mock_cache.update_document.call_args
        assert kwargs["document_id"] == operator_id
        assert "system_info" in kwargs["data"]
        assert "heartbeat_history" in kwargs["data"]

    async def test_update_operator_heartbeat_failure_raises_external_service_error(self, service, mock_cache):
        operator_id = "op-123"
        from tests.fakes.factories import build_operator_heartbeat
        hb = build_operator_heartbeat()
        mock_cache.update_document.return_value = CacheOperationResult(success=False, error="db error")

        with pytest.raises(ExternalServiceError, match="Failed to update Operator op-123 heartbeat: db error"):
            await service.update_operator_heartbeat(operator_id, hb, investigation_id="inv-123", case_id="case-123")

    async def test_append_command_result(self, service, mock_cache):
        operator_id = "op-123"
        command_result = CommandResultRecord(
            execution_id="exec-1",
            command="ls",
            status="completed"
        )
        mock_cache.update_document.return_value = CacheOperationResult(success=True)

        success = await service.append_command_result(operator_id, command_result)

        assert success is True
        mock_cache.update_document.assert_called_once()
        _, kwargs = mock_cache.update_document.call_args
        assert "command_results_history" in kwargs["data"]

    async def test_add_operator_activity(self, service, mock_cache):
        operator_id = "op-123"
        from app.constants import EventType
        from app.models.investigations import ConversationMessageMetadata
        mock_cache.append_to_array.return_value = CacheOperationResult(success=True)

        success = await service.add_operator_activity(
            operator_id=operator_id,
            sender=EventType.OPERATOR_COMMAND_REQUESTED,
            content="test activity",
            metadata=ConversationMessageMetadata()
        )

        assert success is True
        mock_cache.append_to_array.assert_called_once()
        _, kwargs = mock_cache.append_to_array.call_args
        assert kwargs["document_id"] == operator_id
        assert kwargs["array_field"] == "activity_log"
