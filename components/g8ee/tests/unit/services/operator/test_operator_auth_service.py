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

"""Unit tests for OperatorAuthService."""

from unittest.mock import AsyncMock, MagicMock

import pytest

from app.constants import OperatorStatus, DB_COLLECTION_USERS
from app.services.auth.api_key_service import ApiKeyService
from app.services.auth.certificate_service import CertificateService
from app.services.operator.operator_auth_service import OperatorAuthService
from app.services.operator.operator_session_service import OperatorSessionService
from app.services.operator.operator_data_service import OperatorDataService
from app.services.cache.cache_aside import CacheAsideService
from app.models.operators import OperatorDocument

pytestmark = [pytest.mark.unit, pytest.mark.asyncio(loop_scope="session")]

class TestOperatorAuthService:
    @pytest.fixture
    def mock_api_key_service(self):
        return AsyncMock(spec=ApiKeyService)

    @pytest.fixture
    def mock_session_service(self):
        return AsyncMock(spec=OperatorSessionService)

    @pytest.fixture
    def mock_operator_data_service(self):
        return AsyncMock(spec=OperatorDataService)

    @pytest.fixture
    def mock_lifecycle_service(self):
        return AsyncMock()

    @pytest.fixture
    def mock_certificate_service(self):
        return AsyncMock(spec=CertificateService)

    @pytest.fixture
    def mock_cache(self):
        return AsyncMock(spec=CacheAsideService)

    @pytest.fixture
    def auth_service(
        self,
        mock_api_key_service,
        mock_session_service,
        mock_operator_data_service,
        mock_lifecycle_service,
        mock_certificate_service,
        mock_cache,
    ):
        return OperatorAuthService(
            api_key_service=mock_api_key_service,
            session_service=mock_session_service,
            operator_data_service=mock_operator_data_service,
            lifecycle_service=mock_lifecycle_service,
            certificate_service=mock_certificate_service,
            cache_aside=mock_cache,
        )

    async def test_authenticate_via_api_key_happy(self, auth_service, mock_api_key_service, mock_operator_data_service, mock_session_service, mock_lifecycle_service, mock_certificate_service, mock_cache):
        # Setup
        api_key = "g8e_test_key"
        operator_id = "op-123"
        user_id = "user-456"
        org_id = "org-789"
        
        mock_api_key_service.validate_key.return_value = (True, MagicMock(user_id=user_id, operator_id=operator_id, organization_id=org_id), None)
        
        operator_doc = MagicMock(spec=OperatorDocument)
        operator_doc.id = operator_id
        operator_doc.user_id = user_id
        operator_doc.status = OperatorStatus.AVAILABLE
        operator_doc.operator_type = "system"
        operator_doc.bound_web_session_id = "web-session"
        mock_operator_data_service.get_operator.return_value = operator_doc
        
        mock_cache.get_document_with_cache.return_value = {"id": user_id, "name": "test-user"}
        
        session_mock = MagicMock()
        session_mock.id = "session-123"
        mock_session_service.create_operator_session.return_value = session_mock
        
        mock_lifecycle_service.claim_operator_slot.return_value = True
        
        mock_certificate_service.generate_operator_certificate.return_value = {"cert": "CERT", "key": "KEY"}
        
        # Execute
        result = await auth_service.authenticate_operator(
            authorization_header=f"Bearer {api_key}",
            body={},
            request_context={}
        )
        
        # Assert
        assert result["success"] is True
        assert result["operator_session_id"] == "session-123"
        assert result["api_key"] == api_key
        assert result["operator_cert"] == "CERT"
        mock_lifecycle_service.claim_operator_slot.assert_called_once()
        mock_api_key_service.record_usage.assert_called_once_with(api_key)

    async def test_authenticate_via_api_key_missing_bearer(self, auth_service):
        result = await auth_service.authenticate_operator(
            authorization_header=None,
            body={},
            request_context={}
        )
        assert result["success"] is False
        assert result["error"] == "Missing API key"

    async def test_authenticate_via_api_key_invalid(self, auth_service, mock_api_key_service):
        mock_api_key_service.validate_key.return_value = (False, None, "Invalid key")
        result = await auth_service.authenticate_operator(
            authorization_header="Bearer bad-key",
            body={},
            request_context={}
        )
        assert result["success"] is False
        assert result["error"] == "Invalid key"

    async def test_authenticate_via_api_key_unknown_operator(self, auth_service, mock_api_key_service, mock_operator_data_service):
        mock_api_key_service.validate_key.return_value = (True, MagicMock(user_id="u", operator_id="missing"), None)
        mock_operator_data_service.get_operator.return_value = None
        
        result = await auth_service.authenticate_operator(
            authorization_header="Bearer key",
            body={},
            request_context={}
        )
        assert result["success"] is False
        assert result["error"] == "Operator not found"

    async def test_authenticate_via_api_key_user_mismatch(self, auth_service, mock_api_key_service, mock_operator_data_service):
        mock_api_key_service.validate_key.return_value = (True, MagicMock(user_id="user-a", operator_id="op-1"), None)
        mock_operator_data_service.get_operator.return_value = MagicMock(user_id="user-b")
        
        result = await auth_service.authenticate_operator(
            authorization_header="Bearer key",
            body={},
            request_context={}
        )
        assert result["success"] is False
        assert result["error"] == "Unauthorized"

    async def test_register_device_link_operator_happy(self, auth_service, mock_operator_data_service, mock_session_service, mock_lifecycle_service, mock_certificate_service, mock_cache):
        # Setup
        operator_id = "op-123"
        user_id = "user-456"
        api_key = "g8e_device_key"
        
        operator_doc = MagicMock(spec=OperatorDocument)
        operator_doc.id = operator_id
        operator_doc.user_id = user_id
        operator_doc.api_key = api_key
        operator_doc.status = OperatorStatus.AVAILABLE
        operator_doc.bound_web_session_id = "web-123"
        mock_operator_data_service.get_operator.return_value = operator_doc
        
        mock_cache.get_document_with_cache.return_value = {"id": user_id}
        
        session_mock = MagicMock()
        session_mock.id = "session-789"
        mock_session_service.create_operator_session.return_value = session_mock
        
        mock_lifecycle_service.claim_operator_slot.return_value = True
        mock_certificate_service.generate_operator_certificate.return_value = {"cert": "C", "key": "K"}
        
        # Execute
        result = await auth_service.register_device_link_operator(
            operator_id=operator_id,
            user_id=user_id,
            organization_id="org-1",
            operator_type="system",
            request_context={}
        )
        
        # Assert
        assert result["success"] is True
        assert result["api_key"] == api_key
        assert result["operator_session_id"] == "session-789"
        mock_lifecycle_service.claim_operator_slot.assert_called_once()

    async def test_register_device_link_operator_no_api_key_on_slot(self, auth_service, mock_operator_data_service):
        operator_doc = MagicMock(spec=OperatorDocument)
        operator_doc.user_id = "user-1"
        operator_doc.api_key = None
        mock_operator_data_service.get_operator.return_value = operator_doc
        
        result = await auth_service.register_device_link_operator(
            operator_id="op-1",
            user_id="user-1",
            organization_id=None,
            operator_type="system",
            request_context={}
        )
        assert result["success"] is False
        assert "missing api_key" in result["error"]
