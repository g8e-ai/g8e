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

"""Unit tests for OperatorLifecycleService."""

from unittest.mock import AsyncMock

import pytest

from app.constants import OperatorStatus, ComponentName, OperatorType
from app.errors import ValidationError
from app.models.operators import OperatorDocument
from app.services.operator.operator_lifecycle_service import OperatorLifecycleService
from app.services.operator.operator_data_service import OperatorDataService
from app.models.cache import CacheOperationResult
from app.clients.http_client import HTTPClient 
from app.utils.timestamp import now

pytestmark = [pytest.mark.unit, pytest.mark.asyncio(loop_scope="session")]

class TestOperatorLifecycleService:
    @pytest.fixture
    def mock_g8ed_http_client(self):
        return AsyncMock(spec=HTTPClient)

    @pytest.fixture
    def operator_data_service(self, mock_cache_aside_service, mock_g8ed_http_client):
        return OperatorDataService(mock_cache_aside_service, mock_g8ed_http_client)

    @pytest.fixture
    def lifecycle_service(self, operator_data_service):
        return OperatorLifecycleService(operator_data_service)

    @pytest.fixture
    def mock_cache(self, mock_cache_aside_service):
        return mock_cache_aside_service

    async def test_claim_operator_slot_success(self, lifecycle_service, operator_data_service, mock_cache):
        operator_id = "op-123"
        operator_session_id = "session-abc"
        system_info = {"hostname": "test-host", "system_fingerprint": "fp-123"}
        
        mock_cache.get_document_with_cache.return_value = {
            "id": operator_id,
            "user_id": "user-test",
            "status": OperatorStatus.AVAILABLE,
            "first_deployed": None,
        }
        mock_cache.update_document.return_value = CacheOperationResult(success=True)

        success = await lifecycle_service.claim_operator_slot(
            operator_id=operator_id,
            operator_session_id=operator_session_id,
            bound_web_session_id="web-123",
            system_info=system_info,
            operator_type=OperatorType.SYSTEM,
        )

        assert success is True
        assert mock_cache.update_document.call_count == 1

    async def test_claim_operator_slot_not_found_returns_false(self, lifecycle_service, mock_cache):
        mock_cache.get_document_with_cache.return_value = None

        success = await lifecycle_service.claim_operator_slot(
            operator_id="missing",
            operator_session_id="session-abc",
            bound_web_session_id="web-123",
            system_info={"hostname": "test"},
        )

        assert success is False

    async def test_terminate_operator_success(self, lifecycle_service, operator_data_service, mock_cache):
        operator_id = "op-terminate-test"
        mock_cache.get_document_with_cache.return_value = {
            "id": operator_id,
            "user_id": "user-test",
            "status": OperatorStatus.ACTIVE,
            "created_at": now().isoformat(),
        }
        mock_cache.update_document.return_value = CacheOperationResult(success=True)

        result = await lifecycle_service.terminate_operator(
            operator_id=operator_id,
            actor=ComponentName.G8ED,
            summary="Manual termination"
        )

        assert result.status == OperatorStatus.TERMINATED
        assert result.terminated_at is not None
        assert result.operator_session_id is None
        assert result.bound_web_session_id is None

        # Verify single update with status fields
        assert mock_cache.update_document.call_count == 1

        call_args = mock_cache.update_document.call_args
        update_data = call_args.kwargs["data"]
        assert "status" in update_data
        assert update_data["status"] == OperatorStatus.TERMINATED
        assert "terminated_at" in update_data
        assert "operator_session_id" in update_data
        assert update_data["operator_session_id"] is None
        assert "bound_web_session_id" in update_data
        assert update_data["bound_web_session_id"] is None

    async def test_terminate_operator_not_found_raises_validation_error(self, lifecycle_service, mock_cache):
        mock_cache.get_document_with_cache.return_value = None
        
        with pytest.raises(ValidationError, match="Operator missing not found"):
            await lifecycle_service.terminate_operator("missing")

    async def test_terminate_operator_cache_failure_raises_error(self, lifecycle_service, mock_cache):
        operator_id = "op-fail"
        mock_cache.get_document_with_cache.return_value = {
            "id": operator_id,
            "user_id": "user-test",
            "status": OperatorStatus.ACTIVE,
            "created_at": now().isoformat(),
        }
        # Update fails
        mock_cache.update_document.return_value = CacheOperationResult(success=False, error="cache down")

        with pytest.raises(ValidationError, match=f"Failed to terminate operator {operator_id}"):
            await lifecycle_service.terminate_operator(operator_id)

    async def test_update_operator_status_success(self, lifecycle_service, mock_cache):
        operator_id = "op-123"
        mock_cache.get_document_with_cache.return_value = {
            "id": operator_id,
            "user_id": "user-test",
            "status": OperatorStatus.BOUND,
        }
        mock_cache.update_document.return_value = CacheOperationResult(success=True)

        success = await lifecycle_service.update_operator_status(operator_id, OperatorStatus.ACTIVE)

        assert success is True
        assert mock_cache.update_document.call_count == 1

    async def test_update_operator_status_active_sets_heartbeat_if_missing(self, lifecycle_service, mock_cache):
        operator_id = "op-123"
        mock_cache.get_document_with_cache.return_value = {
            "id": operator_id,
            "user_id": "user-test",
            "status": OperatorStatus.BOUND,
            "last_heartbeat": None,
        }
        mock_cache.update_document.return_value = CacheOperationResult(success=True)

        await lifecycle_service.update_operator_status(operator_id, OperatorStatus.ACTIVE)

        # First call is status update
        args, kwargs = mock_cache.update_document.call_args_list[0]
        assert "last_heartbeat" in kwargs["data"]

    async def test_update_operator_status_not_found_returns_false(self, lifecycle_service, mock_cache):
        mock_cache.get_document_with_cache.return_value = None
        mock_cache.update_document.return_value = CacheOperationResult(success=False)
        
        success = await lifecycle_service.update_operator_status("missing", OperatorStatus.ACTIVE)
        assert success is False
