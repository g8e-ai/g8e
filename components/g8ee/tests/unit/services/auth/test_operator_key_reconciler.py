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

from app.services.auth.operator_key_reconciler import reconcile_g8ep_operator_key


@pytest.fixture
def mock_api_key_service():
    service = AsyncMock()
    service.validate_key = AsyncMock()
    return service


@pytest.fixture
def mock_settings_service():
    service = AsyncMock()
    service.get_stored_g8ep_operator_api_key = AsyncMock()
    service.clear_g8ep_operator_api_key = AsyncMock()
    return service


class TestOperatorKeyReconciler:
    """Test operator key reconciler bootstrap-window self-healing."""

    async def test_reconcile_no_stored_key(self, mock_api_key_service, mock_settings_service):
        """Test reconciler when no mirrored key exists."""
        mock_settings_service.get_stored_g8ep_operator_api_key.return_value = None
        
        await reconcile_g8ep_operator_key(mock_api_key_service, mock_settings_service)
        
        mock_settings_service.get_stored_g8ep_operator_api_key.assert_called_once()
        mock_api_key_service.validate_key.assert_not_called()
        mock_settings_service.clear_g8ep_operator_api_key.assert_not_called()

    async def test_reconcile_valid_key(self, mock_api_key_service, mock_settings_service):
        """Test reconciler when mirrored key is valid."""
        mock_settings_service.get_stored_g8ep_operator_api_key.return_value = "g8e_valid_key_12345"
        mock_api_key_service.validate_key.return_value = (True, None, None)
        
        await reconcile_g8ep_operator_key(mock_api_key_service, mock_settings_service)
        
        mock_api_key_service.validate_key.assert_called_once_with("g8e_valid_key_12345")
        mock_settings_service.clear_g8ep_operator_api_key.assert_not_called()

    async def test_reconcile_invalid_key_clears_mirror(self, mock_api_key_service, mock_settings_service):
        """Test reconciler clears mirror when key is invalid."""
        mock_settings_service.get_stored_g8ep_operator_api_key.return_value = "g8e_invalid_key_12345"
        mock_api_key_service.validate_key.return_value = (False, None, "REVOKED")
        
        await reconcile_g8ep_operator_key(mock_api_key_service, mock_settings_service)
        
        mock_api_key_service.validate_key.assert_called_once_with("g8e_invalid_key_12345")
        mock_settings_service.clear_g8ep_operator_api_key.assert_called_once_with(expected="g8e_invalid_key_12345")

    async def test_reconcile_read_failure_skips(self, mock_api_key_service, mock_settings_service):
        """Test reconciler skips when platform_settings read fails."""
        mock_settings_service.get_stored_g8ep_operator_api_key.side_effect = Exception("Read failed")
        
        await reconcile_g8ep_operator_key(mock_api_key_service, mock_settings_service)
        
        mock_api_key_service.validate_key.assert_not_called()
        mock_settings_service.clear_g8ep_operator_api_key.assert_not_called()

    async def test_reconcile_validation_failure_skips_clear(self, mock_api_key_service, mock_settings_service):
        """Test reconciler skips clear when validation lookup fails (avoids false-negative)."""
        mock_settings_service.get_stored_g8ep_operator_api_key.return_value = "g8e_key_12345"
        mock_api_key_service.validate_key.side_effect = Exception("Validation failed")
        
        await reconcile_g8ep_operator_key(mock_api_key_service, mock_settings_service)
        
        mock_settings_service.clear_g8ep_operator_api_key.assert_not_called()

    async def test_reconcile_clear_failure_logged(self, mock_api_key_service, mock_settings_service):
        """Test reconciler logs error when clear operation fails."""
        mock_settings_service.get_stored_g8ep_operator_api_key.return_value = "g8e_invalid_key_12345"
        mock_api_key_service.validate_key.return_value = (False, None, "REVOKED")
        mock_settings_service.clear_g8ep_operator_api_key.side_effect = Exception("Clear failed")
        
        await reconcile_g8ep_operator_key(mock_api_key_service, mock_settings_service)
        
        mock_settings_service.clear_g8ep_operator_api_key.assert_called_once()

    async def test_reconcile_expired_key_clears_mirror(self, mock_api_key_service, mock_settings_service):
        """Test reconciler clears mirror when key is expired."""
        mock_settings_service.get_stored_g8ep_operator_api_key.return_value = "g8e_expired_key_12345"
        mock_api_key_service.validate_key.return_value = (False, None, "API key has expired")
        
        await reconcile_g8ep_operator_key(mock_api_key_service, mock_settings_service)
        
        mock_settings_service.clear_g8ep_operator_api_key.assert_called_once_with(expected="g8e_expired_key_12345")

    async def test_reconcile_revoked_key_clears_mirror(self, mock_api_key_service, mock_settings_service):
        """Test reconciler clears mirror when key is revoked."""
        mock_settings_service.get_stored_g8ep_operator_api_key.return_value = "g8e_revoked_key_12345"
        mock_api_key_service.validate_key.return_value = (False, None, "API key is REVOKED")
        
        await reconcile_g8ep_operator_key(mock_api_key_service, mock_settings_service)
        
        mock_settings_service.clear_g8ep_operator_api_key.assert_called_once_with(expected="g8e_revoked_key_12345")
