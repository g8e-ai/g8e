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

"""
Integration tests: Internal Router Path Registration

These tests verify that FastAPI routes are registered with the correct absolute paths.
This addresses the "off-by-prefix" risk by ensuring the router's route paths match
the InternalApiPaths constants exactly.

Tests exercise actual FastAPI routing logic via TestClient, not just handler functions.
"""

import pytest
from unittest.mock import MagicMock, AsyncMock
from fastapi.testclient import TestClient
from app.main import app
from app.constants import InternalApiPaths


@pytest.mark.integration
class TestInternalRouterPathRegistration:
    """Verify internal router routes are registered with correct absolute paths."""

    @pytest.fixture
    def client(self):
        """FastAPI TestClient for making HTTP requests with dependency overrides."""
        from app.dependencies import (
            get_g8ee_operator_lifecycle_service,
            get_g8ee_operator_auth_service,
            get_g8ee_heartbeat_service,
            get_g8ee_operator_data_service,
            get_g8ee_session_auth_listener,
            get_g8ee_certificate_service,
            get_g8ee_investigation_service,
            get_g8ee_approval_service,
            get_g8e_http_context,
            get_g8ee_case_data_service,
        )
        
        # Override dependencies to avoid ServiceUnavailableError
        # Use AsyncMock for services with async methods
        operator_lifecycle_mock = AsyncMock()
        operator_lifecycle_mock.operator_data_service = AsyncMock()
        operator_lifecycle_mock.operator_data_service.get_operator.return_value = MagicMock(id="test-op", user_id="test-user")
        operator_lifecycle_mock.operator_data_service.cache = MagicMock()
        operator_lifecycle_mock.operator_data_service.cache.update_document.return_value = True
        # Configure async methods to return actual values (not coroutines)
        operator_lifecycle_mock.activate_g8ep_operator.return_value = None
        operator_lifecycle_mock.relaunch_g8ep_operator.return_value = {"success": True, "operator_id": "test-op"}
        operator_lifecycle_mock.terminate_operator.return_value = None
        operator_lifecycle_mock.claim_operator_slot.return_value = True
        app.dependency_overrides[get_g8ee_operator_lifecycle_service] = lambda: operator_lifecycle_mock
        
        app.dependency_overrides[get_g8ee_operator_auth_service] = lambda: AsyncMock()
        app.dependency_overrides[get_g8ee_heartbeat_service] = lambda: AsyncMock()
        
        operator_data_mock = AsyncMock()
        operator_data_mock.get_operator.return_value = MagicMock(id="test-op", user_id="test-user")
        operator_data_mock.register_device_link.return_value = True
        operator_data_mock.send_command_to_operator.return_value = None
        operator_data_mock.send_direct_exec_audit_event.return_value = None
        app.dependency_overrides[get_g8ee_operator_data_service] = lambda: operator_data_mock
        
        app.dependency_overrides[get_g8ee_session_auth_listener] = lambda: AsyncMock()
        app.dependency_overrides[get_g8ee_certificate_service] = lambda: AsyncMock()
        
        investigation_mock = AsyncMock()
        investigation_mock.investigation_data_service = AsyncMock()
        investigation_mock.investigation_data_service.query_investigations.return_value = []
        investigation_mock.investigation_data_service.get_investigation.return_value = MagicMock(id="test-inv")
        investigation_mock.investigation_data_service.add_chat_message.return_value = None
        app.dependency_overrides[get_g8ee_investigation_service] = lambda: investigation_mock
        
        approval_mock = AsyncMock()
        approval_mock.get_pending_approvals.return_value = {}
        approval_mock.handle_approval_response.return_value = None
        app.dependency_overrides[get_g8ee_approval_service] = lambda: approval_mock
        
        case_mock = AsyncMock()
        case_mock.get_case.return_value = MagicMock(id="test-case", user_id="test-user")
        case_mock.update_case.return_value = MagicMock(id="test-case", user_id="test-user")
        case_mock.publish_case_update_sse.return_value = None
        case_mock.delete_case.return_value = None
        app.dependency_overrides[get_g8ee_case_data_service] = lambda: case_mock
        
        # For G8eHttpContext, we need to bypass the header validation since
        # these tests only care about path registration, not auth
        from app.models.http_context import G8eHttpContext
        from app.constants import ComponentName
        app.dependency_overrides[get_g8e_http_context] = lambda: G8eHttpContext(
            web_session_id="test-session",
            user_id="test-user",
            case_id="test-case",
            investigation_id="test-inv",
            source_component=ComponentName.G8ED,
        )
        
        with TestClient(app) as test_client:
            yield test_client
        
        # Clean up overrides
        app.dependency_overrides.clear()

    def test_health_check_path(self, client):
        """Health check endpoint should be accessible at absolute path."""
        response = client.get(InternalApiPaths.G8EE_HEALTH)
        assert response.status_code == 200
        data = response.json()
        assert data["service"] == "g8ee-internal-api"
        assert data["status"] == "healthy"

    def test_chat_path_absolute(self, client):
        """Chat endpoint should be accessible at absolute InternalApiPaths.G8EE_CHAT path."""
        # This will fail with 422 or similar due to missing auth/headers,
        # but we're testing the path resolution, not the handler logic
        response = client.post(
            InternalApiPaths.G8EE_CHAT,
            json={"message": "test"},
            headers={
                "X-G8E-Source-Component": "g8ed",
                "X-G8E-User-Id": "test-user",
                "X-G8E-Web-Session-Id": "test-session",
                "X-Internal-Auth": "test-token",
            }
        )
        # We expect 4xx error due to missing dependencies, not 404
        assert response.status_code != 404

    def test_chat_stop_path_absolute(self, client):
        """Chat stop endpoint should be accessible at absolute path."""
        response = client.post(
            InternalApiPaths.G8EE_CHAT_STOP,
            json={"investigation_id": "test-inv", "reason": "test"},
            headers={
                "X-G8E-Source-Component": "g8ed",
                "X-G8E-User-Id": "test-user",
                "X-Internal-Auth": "test-token",
            }
        )
        # Expect 4xx error due to missing dependencies, not 404
        assert response.status_code != 404

    def test_investigations_path_absolute(self, client):
        """Investigations endpoint should be accessible at absolute path."""
        response = client.get(
            InternalApiPaths.G8EE_INVESTIGATIONS,
            headers={
                "X-G8E-Source-Component": "g8ed",
                "X-G8E-User-Id": "test-user",
                "X-Internal-Auth": "test-token",
            }
        )
        # Expect 4xx error due to missing dependencies, not 404
        assert response.status_code != 404

    def test_case_path_absolute(self, client):
        """Case endpoint should be accessible at absolute path."""
        response = client.get(
            InternalApiPaths.G8EE_CASE.replace("{case_id}", "test-case"),
            headers={
                "X-G8E-Source-Component": "g8ed",
                "X-G8E-User-Id": "test-user",
                "X-Internal-Auth": "test-token",
            }
        )
        # Expect 4xx error due to missing dependencies, not 404
        assert response.status_code != 404

    def test_operator_approval_respond_path_absolute(self, client):
        """Operator approval respond endpoint should be accessible at absolute path."""
        response = client.post(
            InternalApiPaths.G8EE_OPERATOR_APPROVAL_RESPOND,
            json={"approval_id": "test", "approved": True},
            headers={
                "X-G8E-Source-Component": "g8ed",
                "X-G8E-User-Id": "test-user",
                "X-Internal-Auth": "test-token",
            }
        )
        # Expect 4xx error due to missing dependencies, not 404
        assert response.status_code != 404

    def test_operator_direct_command_path_absolute(self, client):
        """Operator direct command endpoint should be accessible at absolute path."""
        response = client.post(
            InternalApiPaths.G8EE_OPERATOR_DIRECT_COMMAND,
            json={"command": "ls", "execution_id": "test-exec"},
            headers={
                "X-G8E-Source-Component": "g8ed",
                "X-G8E-User-Id": "test-user",
                "X-Internal-Auth": "test-token",
            }
        )
        # Expect 4xx error due to missing dependencies, not 404
        assert response.status_code != 404

    def test_operators_terminate_path_absolute(self, client):
        """Operators terminate endpoint should be accessible at absolute path."""
        response = client.post(
            InternalApiPaths.G8EE_OPERATORS_TERMINATE,
            json={"operator_id": "test-op"},
            headers={
                "X-G8E-Source-Component": "g8ed",
                "X-G8E-User-Id": "test-user",
                "X-Internal-Auth": "test-token",
            }
        )
        # Expect 4xx error due to missing dependencies, not 404
        assert response.status_code != 404

    def test_operators_create_slot_path_absolute(self, client):
        """Operators create slot endpoint should be accessible at absolute path."""
        response = client.post(
            InternalApiPaths.G8EE_OPERATORS_CREATE_SLOT,
            json={
                "user_id": "test-user",
                "organization_id": "test-org",
                "slot_number": 1,
                "operator_type": "cloud",
                "cloud_subtype": "aws",
                "name_prefix": "test",
                "is_g8e_node": False,
            },
            headers={
                "X-G8E-Source-Component": "g8ed",
                "X-G8E-User-Id": "test-user",
                "X-Internal-Auth": "test-token",
            }
        )
        # Expect 4xx error due to missing dependencies, not 404
        assert response.status_code != 404

    def test_operators_bind_path_absolute(self, client):
        """Operators bind endpoint should be accessible at absolute path."""
        response = client.post(
            InternalApiPaths.G8EE_OPERATORS_BIND,
            json={"operator_ids": ["test-op"], "web_session_id": "test-session", "user_id": "test-user"},
            headers={
                "X-G8E-Source-Component": "g8ed",
                "X-G8E-User-Id": "test-user",
                "X-Internal-Auth": "test-token",
            }
        )
        # Expect 4xx error due to missing dependencies, not 404
        assert response.status_code != 404

    def test_operators_unbind_path_absolute(self, client):
        """Operators unbind endpoint should be accessible at absolute path."""
        response = client.post(
            InternalApiPaths.G8EE_OPERATORS_UNBIND,
            json={"operator_ids": ["test-op"], "web_session_id": "test-session", "user_id": "test-user"},
            headers={
                "X-G8E-Source-Component": "g8ed",
                "X-G8E-User-Id": "test-user",
                "X-Internal-Auth": "test-token",
            }
        )
        # Expect 4xx error due to missing dependencies, not 404
        assert response.status_code != 404

    def test_auth_generate_key_path_absolute(self, client):
        """Auth generate key endpoint should be accessible at absolute path."""
        response = client.post(
            InternalApiPaths.G8EE_AUTH_GENERATE_KEY,
            json={"operator_id": "test-op"},
            headers={
                "X-G8E-Source-Component": "g8ed",
                "X-G8E-User-Id": "test-user",
                "X-Internal-Auth": "test-token",
            }
        )
        # Expect 4xx error due to missing dependencies, not 404
        assert response.status_code != 404

    def test_chat_triage_answer_path_absolute(self, client):
        """Chat triage answer endpoint should be accessible at absolute path."""
        response = client.post(
            InternalApiPaths.G8EE_CHAT_TRIAGE_ANSWER,
            json={"investigation_id": "test-inv", "question_index": 0, "answer": True},
            headers={
                "X-G8E-Source-Component": "g8ed",
                "X-G8E-User-Id": "test-user",
                "X-Internal-Auth": "test-token",
            }
        )
        # Expect 4xx error due to missing dependencies, not 404
        assert response.status_code != 404

    def test_chat_triage_skip_path_absolute(self, client):
        """Chat triage skip endpoint should be accessible at absolute path."""
        response = client.post(
            InternalApiPaths.G8EE_CHAT_TRIAGE_SKIP,
            json={"investigation_id": "test-inv"},
            headers={
                "X-G8E-Source-Component": "g8ed",
                "X-G8E-User-Id": "test-user",
                "X-Internal-Auth": "test-token",
            }
        )
        # Expect 4xx error due to missing dependencies, not 404
        assert response.status_code != 404

    def test_chat_triage_timeout_path_absolute(self, client):
        """Chat triage timeout endpoint should be accessible at absolute path."""
        response = client.post(
            InternalApiPaths.G8EE_CHAT_TRIAGE_TIMEOUT,
            json={"investigation_id": "test-inv"},
            headers={
                "X-G8E-Source-Component": "g8ed",
                "X-G8E-User-Id": "test-user",
                "X-Internal-Auth": "test-token",
            }
        )
        # Expect 4xx error due to missing dependencies, not 404
        assert response.status_code != 404

    def test_operators_g8ep_activate_path_absolute(self, client):
        """Operators g8ep activate endpoint should be accessible at absolute path."""
        response = client.post(
            InternalApiPaths.G8EE_OPERATORS_G8EP_ACTIVATE,
            json={"user_id": "test-user"},
            headers={
                "X-G8E-Source-Component": "g8ed",
                "X-G8E-User-Id": "test-user",
                "X-Internal-Auth": "test-token",
            }
        )
        # Expect 4xx error due to missing dependencies, not 404
        assert response.status_code != 404

    def test_operators_g8ep_relaunch_path_absolute(self, client):
        """Operators g8ep relaunch endpoint should be accessible at absolute path."""
        response = client.post(
            InternalApiPaths.G8EE_OPERATORS_G8EP_RELAUNCH,
            json={"user_id": "test-user"},
            headers={
                "X-G8E-Source-Component": "g8ed",
                "X-G8E-User-Id": "test-user",
                "X-Internal-Auth": "test-token",
            }
        )
        # Expect 4xx error due to missing dependencies, not 404
        assert response.status_code != 404

    def test_operators_claim_slot_path_absolute(self, client):
        """Operators claim slot endpoint should be accessible at absolute path."""
        response = client.post(
            InternalApiPaths.G8EE_OPERATORS_CLAIM_SLOT,
            json={
                "operator_id": "test-op",
                "operator_session_id": "test-session",
                "bound_web_session_id": "test-web-session",
                "system_info": {"hostname": "test", "system_fingerprint": "fp"},
                "operator_type": "CLOUD",
            },
            headers={
                "X-G8E-Source-Component": "g8ed",
                "X-G8E-User-Id": "test-user",
                "X-Internal-Auth": "test-token",
            }
        )
        # Expect 4xx error due to missing dependencies, not 404
        assert response.status_code != 404

    def test_operators_device_link_register_path_absolute(self, client):
        """Operators device link register endpoint should be accessible at absolute path."""
        response = client.post(
            InternalApiPaths.G8EE_OPERATORS_DEVICE_LINK_REGISTER,
            json={"operator_id": "test-op", "user_id": "test-user", "organization_id": "test-org"},
            headers={
                "X-G8E-Source-Component": "g8ed",
                "X-G8E-User-Id": "test-user",
                "X-Internal-Auth": "test-token",
            }
        )
        # Expect 4xx error due to missing dependencies, not 404
        assert response.status_code != 404

    def test_operators_register_session_path_absolute(self, client):
        """Operators register session endpoint should be accessible at absolute path."""
        response = client.post(
            InternalApiPaths.G8EE_OPERATORS_REGISTER_SESSION,
            json={"operator_id": "test-op", "operator_session_id": "test-session"},
            headers={
                "X-G8E-Source-Component": "g8ed",
                "X-G8E-User-Id": "test-user",
                "X-Internal-Auth": "test-token",
            }
        )
        # Expect 4xx error due to missing dependencies, not 404
        assert response.status_code != 404

    def test_operators_deregister_session_path_absolute(self, client):
        """Operators deregister session endpoint should be accessible at absolute path."""
        response = client.post(
            InternalApiPaths.G8EE_OPERATORS_DEREGISTER_SESSION,
            json={"operator_id": "test-op", "operator_session_id": "test-session"},
            headers={
                "X-G8E-Source-Component": "g8ed",
                "X-G8E-User-Id": "test-user",
                "X-Internal-Auth": "test-token",
            }
        )
        # Expect 4xx error due to missing dependencies, not 404
        assert response.status_code != 404

    def test_operators_stop_path_absolute(self, client):
        """Operators stop endpoint should be accessible at absolute path."""
        response = client.post(
            InternalApiPaths.G8EE_OPERATORS_STOP,
            json={"operator_id": "test-op"},
            headers={
                "X-G8E-Source-Component": "g8ed",
                "X-G8E-User-Id": "test-user",
                "X-Internal-Auth": "test-token",
            }
        )
        # Expect 4xx error due to missing dependencies, not 404
        assert response.status_code != 404

    def test_operators_listen_session_auth_path_absolute(self, client):
        """Operators listen session auth endpoint should be accessible at absolute path."""
        response = client.post(
            InternalApiPaths.G8EE_OPERATORS_LISTEN_SESSION_AUTH,
            json={"operator_session_id": "test-session", "operator_id": "test-op", "user_id": "test-user", "organization_id": "test-org"},
            headers={
                "X-G8E-Source-Component": "g8ed",
                "X-G8E-User-Id": "test-user",
                "X-Internal-Auth": "test-token",
            }
        )
        # Expect 4xx error due to missing dependencies, not 404
        assert response.status_code != 404

    def test_operators_update_api_key_path_absolute(self, client):
        """Operators update api key endpoint should be accessible at absolute path."""
        response = client.post(
            InternalApiPaths.G8EE_OPERATORS_UPDATE_API_KEY,
            json={"operator_id": "test-op", "api_key": "new-key"},
            headers={
                "X-G8E-Source-Component": "g8ed",
                "X-G8E-User-Id": "test-user",
                "X-Internal-Auth": "test-token",
            }
        )
        # Expect 4xx error due to missing dependencies, not 404
        assert response.status_code != 404

    def test_auth_revoke_cert_path_absolute(self, client):
        """Auth revoke cert endpoint should be accessible at absolute path."""
        response = client.post(
            InternalApiPaths.G8EE_AUTH_REVOKE_CERT,
            json={"operator_id": "test-op", "serial": "test-serial", "reason": "test"},
            headers={
                "X-G8E-Source-Component": "g8ed",
                "X-G8E-User-Id": "test-user",
                "X-Internal-Auth": "test-token",
            }
        )
        # Expect 4xx error due to missing dependencies, not 404
        assert response.status_code != 404

    def test_investigation_path_absolute(self, client):
        """Investigation endpoint should be accessible at absolute path."""
        response = client.get(
            InternalApiPaths.G8EE_INVESTIGATION.replace("{investigation_id}", "test-inv"),
            headers={
                "X-G8E-Source-Component": "g8ed",
                "X-G8E-User-Id": "test-user",
                "X-Internal-Auth": "test-token",
            }
        )
        # Expect 4xx error due to missing dependencies, not 404
        assert response.status_code != 404

    def test_operator_approval_pending_path_absolute(self, client):
        """Operator approval pending endpoint should be accessible at absolute path."""
        response = client.get(
            InternalApiPaths.G8EE_OPERATOR_APPROVAL_PENDING,
            headers={
                "X-G8E-Source-Component": "g8ed",
                "X-G8E-User-Id": "test-user",
                "X-Internal-Auth": "test-token",
            }
        )
        # Expect 4xx error due to missing dependencies, not 404
        assert response.status_code != 404
