# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

"""Unit tests for OperatorDataService."""

from unittest.mock import AsyncMock

import pytest

from app.clients.http_client import HTTPClient
from app.constants import OperatorStatus
from app.errors import ValidationError
from app.models.sessions import CliSessionDocument
from app.models.operators import OperatorDocument
from app.services.operator.operator_data_service import OperatorDataService
from app.services.protocols import OperatorDataServiceProtocol
pytestmark = [pytest.mark.unit, pytest.mark.asyncio(loop_scope="session")]


class TestOperatorDataService:
    @pytest.fixture
    def mock_client_http_client(self):
        return AsyncMock(spec=HTTPClient)

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

    async def test_satisfies_operator_data_service_protocol(self, service):
        assert isinstance(service, OperatorDataServiceProtocol)
        assert hasattr(service, "get_operator")
        assert hasattr(service, "query_operators")
        assert not hasattr(service, "create_operator")
        assert not hasattr(service, "update_operator_status")
