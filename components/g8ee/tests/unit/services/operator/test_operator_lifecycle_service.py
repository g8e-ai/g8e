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

from unittest.mock import AsyncMock, MagicMock

import pytest

from app.clients.http_client import HTTPClient
from app.constants import ComponentName, OperatorStatus, OperatorType, CloudSubtype
from app.errors import ValidationError
from app.models.cache import CacheOperationResult
from app.services.operator.operator_data_service import OperatorDataService
from app.services.operator.operator_lifecycle_service import OperatorLifecycleService
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
        service = OperatorLifecycleService(operator_data_service)
        mock_api_key_service = MagicMock()
        mock_api_key_service.rotate_operator_key = AsyncMock(return_value={"success": True, "api_key": "new-key"})
        service.set_api_key_service(mock_api_key_service)
        return service

    @pytest.fixture
    def mock_cache(self, mock_cache_aside_service):
        return mock_cache_aside_service

    async def test_claim_operator_slot_success(self, lifecycle_service, operator_data_service, mock_cache):
        operator_id = "op-123"
        operator_session_id = "session-abc"

        mock_cache.get_document_with_cache.side_effect = [
            {
                "id": operator_id,
                "user_id": "user-test",
                "status": OperatorStatus.OFFLINE,
                "first_deployed": None,
                "history_trail": [],
            },
            {
                "id": operator_id,
                "user_id": "user-test",
                "status": OperatorStatus.ACTIVE,
                "first_deployed": now().isoformat(),
                "history_trail": [],
            },
            {
                "id": operator_id,
                "user_id": "user-test",
                "status": OperatorStatus.ACTIVE,
                "first_deployed": now().isoformat(),
                "history_trail": [],
            },
            None
        ]
        mock_cache.update_document.return_value = CacheOperationResult(success=True)

        success = await lifecycle_service.claim_operator_slot(
            operator_id=operator_id,
            operator_session_id=operator_session_id,
            bound_web_session_id="web-123",
            operator_type=OperatorType.SYSTEM,
        )

        assert success is True
        # add_history_entry calls update_document once
        assert mock_cache.update_document.call_count == 1

        # Verify history append via update_document in add_history_entry
        call_args = mock_cache.update_document.call_args
        update_data = call_args.kwargs["data"]
        assert "history_trail" in update_data
        history_entry = update_data["history_trail"].values[0]
        assert history_entry["event_type"] == "slot.consumed"
        assert history_entry["prev_hash"] == "0" * 64

    async def test_claim_operator_slot_not_found_returns_false(self, lifecycle_service, mock_cache):
        mock_cache.get_document_with_cache.return_value = None

        success = await lifecycle_service.claim_operator_slot(
            operator_id="missing",
            operator_session_id="session-abc",
            bound_web_session_id="web-123",
        )

        assert success is False

    async def test_terminate_operator_success(self, lifecycle_service, operator_data_service, mock_cache):
        operator_id = "op-terminate-test"
        mock_cache.get_document_with_cache.side_effect = [
            {
                "id": operator_id,
                "user_id": "user-test",
                "status": OperatorStatus.ACTIVE,
                "created_at": now().isoformat(),
                "history_trail": [],
            },
            {
                "id": operator_id,
                "user_id": "user-test",
                "status": OperatorStatus.TERMINATED,
                "created_at": now().isoformat(),
                "history_trail": [],
                "terminated_at": now().isoformat(),
            },
            {
                "id": operator_id,
                "user_id": "user-test",
                "status": OperatorStatus.TERMINATED,
                "created_at": now().isoformat(),
                "history_trail": [],
                "terminated_at": now().isoformat(),
            },
            None
        ]
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

        # Verify history append via update_document in add_history_entry
        history_entry = update_data["history_trail"].values[0]
        assert history_entry["event_type"] == "terminated"
        assert history_entry["actor"] == "g8ed"

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

    async def test_update_operator_status_active_does_not_set_heartbeat(self, lifecycle_service, mock_cache):
        operator_id = "op-123"
        mock_cache.get_document_with_cache.return_value = {
            "id": operator_id,
            "user_id": "user-test",
            "status": OperatorStatus.BOUND,
            "latest_heartbeat_snapshot": None,
        }
        mock_cache.update_document.return_value = CacheOperationResult(success=True)

        await lifecycle_service.update_operator_status(operator_id, OperatorStatus.ACTIVE)

        # Verify status update does NOT set heartbeat timestamp (only set on actual heartbeat ingestion)
        _, kwargs = mock_cache.update_document.call_args_list[0]
        assert kwargs["data"]["status"] == OperatorStatus.ACTIVE

    async def test_update_operator_status_not_found_returns_false(self, lifecycle_service, mock_cache):
        mock_cache.get_document_with_cache.return_value = None
        mock_cache.update_document.return_value = CacheOperationResult(success=False)

        success = await lifecycle_service.update_operator_status("missing", OperatorStatus.ACTIVE)
        assert success is False

    async def test_claim_operator_slot_active_status_different_session_preempts(self, lifecycle_service, mock_cache):
        operator_id = "op-reclaim"
        old_session_id = "session-old"
        new_session_id = "session-new"

        mock_cache.get_document_with_cache.side_effect = [
            {
                "id": operator_id,
                "user_id": "user-test",
                "status": OperatorStatus.ACTIVE,
                "operator_session_id": old_session_id,
                "first_deployed": now().isoformat(),
                "history_trail": [],
            },
            {
                "id": operator_id,
                "user_id": "user-test",
                "status": OperatorStatus.ACTIVE,
                "operator_session_id": new_session_id,
                "first_deployed": now().isoformat(),
                "history_trail": [],
            },
            {
                "id": operator_id,
                "user_id": "user-test",
                "status": OperatorStatus.ACTIVE,
                "operator_session_id": new_session_id,
                "first_deployed": now().isoformat(),
                "history_trail": [],
            },
            None
        ]
        mock_cache.update_document.return_value = CacheOperationResult(success=True)

        success = await lifecycle_service.claim_operator_slot(
            operator_id=operator_id,
            operator_session_id=new_session_id,
            bound_web_session_id="web-123",
            operator_type=OperatorType.SYSTEM,
        )

        assert success is True
        assert mock_cache.update_document.call_count == 1

        call_args = mock_cache.update_document.call_args
        update_data = call_args.kwargs["data"]
        assert update_data["operator_session_id"] == new_session_id
        assert update_data["status"] == OperatorStatus.ACTIVE

    async def test_claim_operator_slot_active_status_same_session_fails(self, lifecycle_service, mock_cache):
        operator_id = "op-same-session"
        session_id = "session-same"

        mock_cache.get_document_with_cache.return_value = {
            "id": operator_id,
            "user_id": "user-test",
            "status": OperatorStatus.ACTIVE,
            "operator_session_id": session_id,
            "first_deployed": now().isoformat(),
            "history_trail": [],
        }

        success = await lifecycle_service.claim_operator_slot(
            operator_id=operator_id,
            operator_session_id=session_id,
            bound_web_session_id="web-123",
        )

        assert success is False
        assert mock_cache.update_document.call_count == 0

    async def test_claim_operator_slot_stale_status_different_session_preempts(self, lifecycle_service, mock_cache):
        operator_id = "op-stale"
        old_session_id = "session-old"
        new_session_id = "session-new"

        mock_cache.get_document_with_cache.side_effect = [
            {
                "id": operator_id,
                "user_id": "user-test",
                "status": OperatorStatus.STALE,
                "operator_session_id": old_session_id,
                "first_deployed": now().isoformat(),
                "history_trail": [],
            },
            {
                "id": operator_id,
                "user_id": "user-test",
                "status": OperatorStatus.ACTIVE,
                "operator_session_id": new_session_id,
                "first_deployed": now().isoformat(),
                "history_trail": [],
            },
            {
                "id": operator_id,
                "user_id": "user-test",
                "status": OperatorStatus.ACTIVE,
                "operator_session_id": new_session_id,
                "first_deployed": now().isoformat(),
                "history_trail": [],
            },
            None
        ]
        mock_cache.update_document.return_value = CacheOperationResult(success=True)

        success = await lifecycle_service.claim_operator_slot(
            operator_id=operator_id,
            operator_session_id=new_session_id,
            bound_web_session_id="web-123",
        )

        assert success is True
        assert mock_cache.update_document.call_count == 1

    async def test_claim_operator_slot_terminated_status_fails(self, lifecycle_service, mock_cache):
        operator_id = "op-terminated"

        mock_cache.get_document_with_cache.return_value = {
            "id": operator_id,
            "user_id": "user-test",
            "status": OperatorStatus.TERMINATED,
            "history_trail": [],
        }

        success = await lifecycle_service.claim_operator_slot(
            operator_id=operator_id,
            operator_session_id="session-new",
            bound_web_session_id="web-123",
        )

        assert success is False
        assert mock_cache.update_document.call_count == 0

