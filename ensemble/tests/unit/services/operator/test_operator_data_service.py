# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

"""Unit tests for OperatorDataService."""

from unittest.mock import AsyncMock, MagicMock

import pytest

from app.constants import OperatorStatus
from app.errors import ValidationError
from app.models.sessions import CliSessionDocument
from app.models.operators import OperatorDocument
from app.services.operator.operator_data_service import OperatorDataService
from app.services.protocols import OperatorDataServiceProtocol

pytestmark = [pytest.mark.unit, pytest.mark.asyncio(loop_scope="session")]


class TestOperatorDataService:
    @pytest.fixture
    def mock_gateway_client(self):
        return MagicMock()

    @pytest.fixture
    def service(self, mock_cache_aside_service, mock_gateway_client):
        return OperatorDataService(mock_cache_aside_service, mock_gateway_client)

    @pytest.fixture
    def mock_cache(self, mock_cache_aside_service):
        return mock_cache_aside_service

    async def test_get_operator_success(self, service, mock_gateway_client):
        operator_id = "op-123"
        mock_gateway_client.list = AsyncMock(
            return_value=[
                {
                    "id": operator_id,
                    "user_id": "user-test",
                    "status": OperatorStatus.ACTIVE,
                    "bound_web_session_id": None,
                }
            ]
        )

        result = await service.get_operator(operator_id, user_id="user-test")

        assert result is not None
        assert isinstance(result, OperatorDocument)
        assert result.id == operator_id
        assert result.status == OperatorStatus.ACTIVE
        mock_gateway_client.list.assert_awaited_once_with(user_id="user-test")

    async def test_get_operator_by_session_success(self, service, mock_gateway_client):
        mock_gateway_client.get_by_session = AsyncMock(
            return_value={
                "id": "op-123",
                "user_id": "user-test",
                "status": OperatorStatus.ACTIVE,
                "operator_session_id": "sess-123",
            }
        )

        result = await service.get_operator_by_session("sess-123")

        assert result is not None
        assert result.id == "op-123"
        assert result.operator_session_id == "sess-123"

    async def test_get_operator_not_found(self, service, mock_gateway_client):
        mock_gateway_client.list = AsyncMock(return_value=[])
        result = await service.get_operator("nonexistent", user_id="user-test")
        assert result is None

    async def test_get_operator_empty_id_raises_error(self, service):
        with pytest.raises(ValidationError, match="operator_id is required"):
            await service.get_operator("", user_id="user-test")

    async def test_get_operator_requires_user_id(self, service):
        with pytest.raises(ValidationError, match="user_id is required"):
            await service.get_operator("op-123")

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

    async def test_query_operators_lists_from_gateway(self, service, mock_gateway_client):
        mock_gateway_client.list = AsyncMock(
            return_value=[
                {"id": "op-1", "user_id": "user-test", "status": OperatorStatus.ACTIVE},
                {"id": "op-2", "user_id": "user-test", "status": OperatorStatus.BOUND},
            ]
        )

        result = await service.query_operators(user_id="user-test")

        assert len(result) == 2
        assert result[0].id == "op-1"
        assert result[1].id == "op-2"

    async def test_satisfies_operator_data_service_protocol(self, service):
        assert isinstance(service, OperatorDataServiceProtocol)
        assert hasattr(service, "get_operator")
        assert hasattr(service, "get_operator_by_session")
        assert hasattr(service, "query_operators")
        assert not hasattr(service, "create_operator")
        assert not hasattr(service, "update_operator_status")
