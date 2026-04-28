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

import pytest
from unittest.mock import AsyncMock, MagicMock

from app.services.auth.api_key_service import ApiKeyService, API_KEY_STATUS_ACTIVE, API_KEY_STATUS_REVOKED


@pytest.fixture
def mock_cache_aside():
    cache = AsyncMock()
    cache.create_document = AsyncMock()
    cache.get_document_with_cache = AsyncMock()
    cache.update_document = AsyncMock()
    return cache


@pytest.fixture
def api_key_service(mock_cache_aside):
    return ApiKeyService(mock_cache_aside)


class TestApiKeyService:
    """Test ApiKeyService coordinator methods (issue, rotate, revoke)."""

    async def test_issue_key_success(self, api_key_service, mock_cache_aside):
        """Test issuing a new API key successfully."""
        mock_cache_aside.create_document.return_value = MagicMock(success=True)
        
        result = await api_key_service.issue_key(
            api_key="g8e_test_key_12345",
            user_id="user-123",
            organization_id="org-456",
            operator_id="op-789",
            client_name="operator",
            permissions=["command:execute"],
            status=API_KEY_STATUS_ACTIVE,
        )
        
        assert result is True
        mock_cache_aside.create_document.assert_called_once()
        call_args = mock_cache_aside.create_document.call_args
        assert call_args[1]["collection"] == "api_keys"
        assert call_args[1]["data"]["user_id"] == "user-123"
        assert call_args[1]["data"]["operator_id"] == "op-789"
        assert call_args[1]["data"]["status"] == API_KEY_STATUS_ACTIVE

    async def test_issue_key_failure(self, api_key_service, mock_cache_aside):
        """Test issuing a key when storage fails."""
        mock_cache_aside.create_document.return_value = MagicMock(success=False, error="Storage error")
        
        result = await api_key_service.issue_key(
            api_key="g8e_test_key_12345",
            user_id="user-123",
        )
        
        assert result is False

    async def test_revoke_key_success(self, api_key_service, mock_cache_aside):
        """Test revoking an API key successfully."""
        mock_cache_aside.get_document_with_cache.return_value = {"status": API_KEY_STATUS_ACTIVE}
        mock_cache_aside.update_document.return_value = MagicMock(success=True)
        
        result = await api_key_service.revoke_key("g8e_test_key_12345")
        
        assert result is True
        mock_cache_aside.update_document.assert_called_once()
        call_args = mock_cache_aside.update_document.call_args
        assert call_args[1]["data"]["status"] == API_KEY_STATUS_REVOKED

    async def test_revoke_key_not_found(self, api_key_service, mock_cache_aside):
        """Test revoking a key that doesn't exist (idempotent)."""
        mock_cache_aside.get_document_with_cache.return_value = None
        
        result = await api_key_service.revoke_key("g8e_test_key_12345")
        
        assert result is True
        mock_cache_aside.update_document.assert_not_called()

    async def test_revoke_key_storage_failure(self, api_key_service, mock_cache_aside):
        """Test revoking a key when storage fails."""
        mock_cache_aside.get_document_with_cache.return_value = {"status": API_KEY_STATUS_ACTIVE}
        mock_cache_aside.update_document.side_effect = Exception("Storage error")
        
        result = await api_key_service.revoke_key("g8e_test_key_12345")
        
        assert result is False

    async def test_issue_operator_key_success(self, api_key_service, mock_cache_aside):
        """Test issuing an operator key with g8ep mirror."""
        mock_cache_aside.create_document.return_value = MagicMock(success=True)
        mock_settings_service = AsyncMock()
        mock_settings_service.update_g8ep_operator_api_key = AsyncMock()
        
        result = await api_key_service.issue_operator_key(
            api_key="g8e_op_key_12345",
            user_id="user-123",
            organization_id="org-456",
            operator_id="op-789",
            is_g8ep=True,
            settings_service=mock_settings_service,
        )
        
        assert result is True
        mock_settings_service.update_g8ep_operator_api_key.assert_called_once_with("g8e_op_key_12345")

    async def test_issue_operator_key_mirror_failure_rollback(self, api_key_service, mock_cache_aside):
        """Test that mirror failure rolls back the canonical record."""
        mock_cache_aside.create_document.return_value = MagicMock(success=True)
        mock_settings_service = AsyncMock()
        mock_settings_service.update_g8ep_operator_api_key = AsyncMock(side_effect=Exception("Mirror failed"))
        mock_cache_aside.update_document = AsyncMock()
        
        result = await api_key_service.issue_operator_key(
            api_key="g8e_op_key_12345",
            user_id="user-123",
            operator_id="op-789",
            is_g8ep=True,
            settings_service=mock_settings_service,
        )
        
        assert result is False
        mock_cache_aside.update_document.assert_called_once()
        call_args = mock_cache_aside.update_document.call_args
        assert call_args[1]["data"]["status"] == API_KEY_STATUS_REVOKED

    async def test_rotate_operator_key_success(self, api_key_service, mock_cache_aside):
        """Test rotating an operator key."""
        mock_cache_aside.create_document.return_value = MagicMock(success=True)
        mock_settings_service = AsyncMock()
        mock_settings_service.update_g8ep_operator_api_key = AsyncMock()
        
        result = await api_key_service.rotate_operator_key(
            old_api_key="g8e_old_key_12345",
            new_api_key="g8e_new_key_67890",
            user_id="user-123",
            operator_id="op-789",
            is_g8ep=True,
            settings_service=mock_settings_service,
        )
        
        assert result is True

    async def test_rotate_operator_key_new_key_issue_failure(self, api_key_service, mock_cache_aside):
        """Test rotate fails when new key issuance fails."""
        mock_cache_aside.create_document.return_value = MagicMock(success=False)
        mock_settings_service = AsyncMock()
        
        result = await api_key_service.rotate_operator_key(
            old_api_key="g8e_old_key_12345",
            new_api_key="g8e_new_key_67890",
            user_id="user-123",
            operator_id="op-789",
            is_g8ep=True,
            settings_service=mock_settings_service,
        )
        
        assert result is False

    async def test_revoke_operator_key_success(self, api_key_service, mock_cache_aside):
        """Test revoking an operator key with mirror clear."""
        mock_cache_aside.get_document_with_cache.return_value = {"status": API_KEY_STATUS_ACTIVE}
        mock_cache_aside.update_document.return_value = MagicMock(success=True)
        mock_settings_service = AsyncMock()
        mock_settings_service.clear_g8ep_operator_api_key = AsyncMock()
        
        result = await api_key_service.revoke_operator_key(
            api_key="g8e_op_key_12345",
            is_g8ep=True,
            settings_service=mock_settings_service,
        )
        
        assert result is True
        mock_settings_service.clear_g8ep_operator_api_key.assert_called_once()

    async def test_revoke_operator_key_mirror_clear_failure(self, api_key_service, mock_cache_aside):
        """Test that mirror clear failure is logged but not fatal."""
        mock_cache_aside.get_document_with_cache.return_value = {"status": API_KEY_STATUS_ACTIVE}
        mock_cache_aside.update_document.return_value = MagicMock(success=True)
        mock_settings_service = AsyncMock()
        mock_settings_service.clear_g8ep_operator_api_key = AsyncMock(side_effect=Exception("Clear failed"))
        
        result = await api_key_service.revoke_operator_key(
            api_key="g8e_op_key_12345",
            is_g8ep=True,
            settings_service=mock_settings_service,
        )
        
        assert result is True

    async def test_validate_key_success(self, api_key_service, mock_cache_aside):
        """Test validating a valid API key."""
        mock_cache_aside.get_document_with_cache.return_value = {
            "status": API_KEY_STATUS_ACTIVE,
            "expires_at": None,
        }
        
        valid, doc, error = await api_key_service.validate_key("g8e_test_key_12345")
        
        assert valid is True
        assert error is None
        assert doc is not None

    async def test_validate_key_missing(self, api_key_service, mock_cache_aside):
        """Test validating a missing API key."""
        mock_cache_aside.get_document_with_cache.return_value = None
        
        valid, doc, error = await api_key_service.validate_key("g8e_test_key_12345")
        
        assert valid is False
        assert error == "API key not found"
        assert doc is None

    async def test_validate_key_revoked(self, api_key_service, mock_cache_aside):
        """Test validating a revoked API key."""
        mock_cache_aside.get_document_with_cache.return_value = {
            "status": API_KEY_STATUS_REVOKED,
            "expires_at": None,
        }
        
        valid, doc, error = await api_key_service.validate_key("g8e_test_key_12345")
        
        assert valid is False
        assert "REVOKED" in error
        assert doc is not None

    async def test_validate_key_expired(self, api_key_service, mock_cache_aside):
        """Test validating an expired API key."""
        from app.utils.timestamp import now
        from datetime import timedelta
        
        expired_time = now() - timedelta(days=1)
        mock_cache_aside.get_document_with_cache.return_value = {
            "status": API_KEY_STATUS_ACTIVE,
            "expires_at": expired_time,
        }
        
        valid, doc, error = await api_key_service.validate_key("g8e_test_key_12345")
        
        assert valid is False
        assert "expired" in error.lower()
        assert doc is not None

    def test_make_doc_id(self, api_key_service):
        """Test generating deterministic document ID from API key."""
        doc_id = api_key_service.make_doc_id("g8e_test_key_12345")
        
        assert len(doc_id) == 32
        assert isinstance(doc_id, str)

    def test_generate_raw_key(self, api_key_service):
        """Test generating a new raw API key."""
        key = api_key_service.generate_raw_key()
        
        assert key.startswith("g8e_")
        assert len(key) > 10
