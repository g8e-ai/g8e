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

from app.clients.http_client import HTTPClient
from app.constants import EventType, OperatorStatus
from app.models.investigations import ConversationMessageMetadata
from app.errors import ExternalServiceError, ValidationError
from app.models.cache import CacheOperationResult
from app.models.sessions import CliSessionDocument
from app.models.operators import (
    CommandResultRecord,
    OperatorDocument,
)
from app.services.operator.operator_data_service import OperatorDataService
from app.services.protocols import OperatorDataServiceProtocol
from tests.fakes.factories import build_operator_heartbeat

pytestmark = [pytest.mark.unit, pytest.mark.asyncio(loop_scope="session")]

class TestOperatorDataService:
    @pytest.fixture
    def mock_client_http_client(self):
        return AsyncMock(spec=HTTPClient )

    @pytest.fixture
    def service(self, mock_cache_aside_service, mock_client_http_client):
        return OperatorDataService(mock_cache_aside_service, mock_client_http_client)

    @pytest.fixture
    def mock_cache(self, mock_cache_aside_service):
        return mock_cache_aside_service

    async def test_get_operator_success(self, service, mock_cache):
        operator_id = "op-123"
        mock_cache.get_document_with_cache.return_value = {
            "id": operator_id,
            "user_id": "user-test",
            "status": OperatorStatus.ACTIVE,
            "bound_web_session_id": None,
        }

        result = await service.get_operator(operator_id)

        assert result is not None
        assert isinstance(result, OperatorDocument)
        assert result.id == operator_id
        assert result.status == OperatorStatus.ACTIVE
        mock_cache.get_document_with_cache.assert_called_once_with(service.collection, operator_id)

    async def test_get_operator_not_found(self, service, mock_cache):
        mock_cache.get_document_with_cache.return_value = None
        result = await service.get_operator("nonexistent")
        assert result is None

    async def test_get_operator_empty_id_raises_error(self, service):
        with pytest.raises(ValidationError, match="operator_id is required"):
            await service.get_operator("")

    async def test_get_cli_session_success(self, service, mock_cache):
        cli_session_id = "cli-123"
        mock_cache.get_document_with_cache.return_value = {
            "id": cli_session_id,
            "session_type": "cli",
            "user_id": "user-test",
            "operator_session_id": "op-sess-123",
            "absolute_expires_at": "2026-05-17T00:00:00Z",
            "idle_expires_at": "2026-05-17T00:00:00Z",
        }

        result = await service.get_cli_session(cli_session_id)

        assert result is not None
        assert isinstance(result, CliSessionDocument)
        assert result.id == cli_session_id
        assert result.operator_session_id == "op-sess-123"
        mock_cache.get_document_with_cache.assert_called_once()

    async def test_validate_cli_session_ownership_success(self, service, mock_cache):
        cli_session_id = "cli-123"
        operator_session_id = "op-sess-123"
        mock_cache.get_document_with_cache.return_value = {
            "id": cli_session_id,
            "session_type": "cli",
            "user_id": "user-test",
            "operator_session_id": operator_session_id,
            "absolute_expires_at": "2026-05-17T00:00:00Z",
            "idle_expires_at": "2026-05-17T00:00:00Z",
        }

        is_owned = await service.validate_cli_session_ownership(cli_session_id, operator_session_id)
        assert is_owned is True

    async def test_validate_cli_session_ownership_mismatch(self, service, mock_cache):
        cli_session_id = "cli-123"
        operator_session_id = "op-sess-wrong"
        mock_cache.get_document_with_cache.return_value = {
            "id": cli_session_id,
            "session_type": "cli",
            "user_id": "user-test",
            "operator_session_id": "op-sess-correct",
            "absolute_expires_at": "2026-05-17T00:00:00Z",
            "idle_expires_at": "2026-05-17T00:00:00Z",
        }

        is_owned = await service.validate_cli_session_ownership(cli_session_id, operator_session_id)
        assert is_owned is False

    async def test_update_operator_heartbeat_success(self, service, mock_cache):
        operator_id = "op-123"
        hb = build_operator_heartbeat()
        mock_cache.update_document.return_value = CacheOperationResult(success=True)

        success = await service.update_operator_heartbeat(operator_id, hb, investigation_id="inv-123", case_id="case-123")

        assert success is True
        mock_cache.update_document.assert_called_once()
        _, kwargs = mock_cache.update_document.call_args
        assert kwargs["document_id"] == operator_id
        assert "current_hostname" in kwargs["data"]
        assert "heartbeat_history" in kwargs["data"]

    async def test_update_operator_heartbeat_failure_raises_external_service_error(self, service, mock_cache):
        operator_id = "op-123"
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

    async def test_update_operator_status_success(self, service, mock_cache):
        operator_id = "op-123"
        mock_cache.update_document.return_value = CacheOperationResult(success=True)

        success = await service.update_operator_status(operator_id, OperatorStatus.STALE)

        assert success is True
        mock_cache.update_document.assert_called_once()
        _, kwargs = mock_cache.update_document.call_args
        assert kwargs["document_id"] == operator_id
        assert kwargs["data"]["status"] == OperatorStatus.STALE
        assert "updated_at" in kwargs["data"]
        assert kwargs["merge"] is True

    async def test_update_operator_status_empty_id_raises_validation_error(self, service):
        with pytest.raises(ValidationError, match="operator_id is required"):
            await service.update_operator_status("", OperatorStatus.OFFLINE)

    async def test_update_operator_status_failure_raises_external_service_error(self, service, mock_cache):
        operator_id = "op-123"
        mock_cache.update_document.return_value = CacheOperationResult(success=False, error="db down")

        with pytest.raises(
            ExternalServiceError,
            match="Failed to update Operator op-123 status: db down",
        ):
            await service.update_operator_status(operator_id, OperatorStatus.STALE)

    async def test_satisfies_operator_data_service_protocol(self, service):
        """Regression: ``HeartbeatStaleMonitorService`` is typed against
        ``OperatorDataServiceProtocol`` and calls ``update_operator_status``.
        A protocol-conformance check would have caught the gap where the
        protocol declared a method the implementation lacked (or vice versa).
        """
        assert isinstance(service, OperatorDataServiceProtocol)
        assert hasattr(service, "update_operator_status")

