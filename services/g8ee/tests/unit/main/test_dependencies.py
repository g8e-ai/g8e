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

from unittest.mock import MagicMock

import pytest
from fastapi import Request

from app.constants import (
    PROXY_ORGANIZATION_ID_HEADER,
    PROXY_USER_EMAIL_HEADER,
    PROXY_USER_ID_HEADER,
    AuthMethod,
    ComponentName,
    G8eHeaders,
    HealthStatus,
)
from app.dependencies import (
    get_g8ee_attachment_service,
    get_g8ee_cache_aside_service,
    get_g8ee_case_data_service,
    get_g8ee_chat_pipeline,
    get_g8ee_chat_task_manager,
    get_g8ee_current_active_user,
    get_g8ee_investigation_service,
    get_g8ee_kv_cache_client,
    get_g8ee_operator_cache,
    get_g8ee_operator_command_service,
    get_g8ee_platform_settings,
    get_g8ee_pubsub_client,
    health_check_dependencies,
    require_proxy_auth,
)
from app.errors import (
    AuthenticationError,
    ConfigurationError,
    ServiceUnavailableError,
)
from app.models.settings import G8eePlatformSettings
from tests.fakes.factories import build_authenticated_user
from tests.fakes.headers import TEST_G8E_HEADERS

pytestmark = [pytest.mark.unit, pytest.mark.asyncio(loop_scope="session")]


@pytest.fixture
def mock_request():
    request = MagicMock(spec=Request)
    request.app = MagicMock()
    request.app.state = MagicMock()
    request.state = MagicMock()
    request.headers = {}
    request.url = MagicMock()
    request.url.path = "/test"
    request.method = "GET"
    request.client = MagicMock()
    request.client.host = "127.0.0.1"
    return request


class TestGetG8eeAppSettings:
    async def test_returns_settings_from_app_state(self, mock_request):
        # We need a real G8eePlatformSettings object for this test to be meaningful
        settings = G8eePlatformSettings(port=8443)
        mock_request.app.state.settings = settings
        result = await get_g8ee_platform_settings(mock_request)
        assert result.port == settings.port
        assert isinstance(result, G8eePlatformSettings)

    async def test_missing_raises_configuration_error(self, mock_request):
        # In a real app, if the attribute is missing, it's a configuration failure
        if hasattr(mock_request.app.state, "settings"):
            delattr(mock_request.app.state, "settings")

        with pytest.raises(ConfigurationError, match="Settings not available"):
            await get_g8ee_platform_settings(mock_request)


class TestGetG8eePubSubClient:
    async def test_returns_client_from_app_state(self, mock_request):
        mock_client = MagicMock()
        mock_request.app.state.pubsub_client = mock_client
        result = await get_g8ee_pubsub_client(mock_request)
        assert result == mock_client

    async def test_missing_raises_service_unavailable(self, mock_request):
        mock_request.app.state.pubsub_client = None
        with pytest.raises(ServiceUnavailableError, match="PubSubClient not available"):
            await get_g8ee_pubsub_client(mock_request)

class TestGetG8eeKVClient:
    async def test_returns_client_from_app_state(self, mock_request):
        mock_client = MagicMock()
        mock_request.app.state.kv_cache_client = mock_client
        result = await get_g8ee_kv_cache_client(mock_request)
        assert result == mock_client

    async def test_missing_raises_service_unavailable(self, mock_request):
        mock_request.app.state.kv_cache_client = None
        with pytest.raises(ServiceUnavailableError, match="KVCacheClient not available"):
            await get_g8ee_kv_cache_client(mock_request)

class TestGetG8eeCacheService:
    async def test_returns_service_from_app_state(self, mock_request):
        mock_cache = MagicMock()
        mock_request.app.state.services.cache_aside_service = mock_cache
        result = await get_g8ee_cache_aside_service(mock_request)
        assert result == mock_cache

    async def test_missing_raises_service_unavailable(self, mock_request):
        mock_request.app.state.services.cache_aside_service = None
        with pytest.raises(ServiceUnavailableError, match="Cache service not available"):
            await get_g8ee_cache_aside_service(mock_request)


class TestGetCaseDataService:
    async def test_returns_service_from_app_state(self, mock_request):
        mock_service = MagicMock()
        mock_request.app.state.services.case_data_service = mock_service
        result = await get_g8ee_case_data_service(mock_request)
        assert result == mock_service

    async def test_missing_raises_service_unavailable(self, mock_request):
        mock_request.app.state.services.case_data_service = None
        with pytest.raises(ServiceUnavailableError, match="Case Data Service not available"):
            await get_g8ee_case_data_service(mock_request)


class TestGetInvestigationService:
    async def test_returns_service_from_app_state(self, mock_request):
        mock_service = MagicMock()
        mock_request.app.state.services.investigation_service = mock_service
        result = await get_g8ee_investigation_service(mock_request)
        assert result == mock_service

    async def test_missing_raises_service_unavailable(self, mock_request):
        mock_request.app.state.services.investigation_service = None
        with pytest.raises(ServiceUnavailableError, match="Investigation Domain Service not available"):
            await get_g8ee_investigation_service(mock_request)


class TestGetG8eeChatPipeline:
    async def test_returns_service_from_app_state(self, mock_request):
        mock_service = MagicMock()
        mock_request.app.state.services.chat_pipeline = mock_service
        result = await get_g8ee_chat_pipeline(mock_request)
        assert result == mock_service

    async def test_missing_raises_service_unavailable(self, mock_request):
        mock_request.app.state.services.chat_pipeline = None
        with pytest.raises(ServiceUnavailableError, match="Chat Pipeline not available"):
            await get_g8ee_chat_pipeline(mock_request)

    async def test_none_raises_service_unavailable(self, mock_request):
        mock_request.app.state.services.chat_pipeline = None
        with pytest.raises(ServiceUnavailableError, match="Chat Pipeline not available"):
            await get_g8ee_chat_pipeline(mock_request)


class TestGetG8eeChatTaskManager:
    async def test_returns_service_from_app_state(self, mock_request):
        mock_service = MagicMock()
        mock_request.app.state.services.chat_task_manager = mock_service
        result = await get_g8ee_chat_task_manager(mock_request)
        assert result == mock_service

    async def test_missing_raises_service_unavailable(self, mock_request):
        mock_request.app.state.services.chat_task_manager = None
        with pytest.raises(ServiceUnavailableError, match="Chat Task Manager not available"):
            await get_g8ee_chat_task_manager(mock_request)

    async def test_none_raises_service_unavailable(self, mock_request):
        mock_request.app.state.services.chat_task_manager = None
        with pytest.raises(ServiceUnavailableError, match="Chat Task Manager not available"):
            await get_g8ee_chat_task_manager(mock_request)


class TestGetOperatorCache:
    async def test_returns_service_from_app_state(self, mock_request):
        mock_service = MagicMock()
        mock_request.app.state.services.cache_aside_service = mock_service
        result = await get_g8ee_operator_cache(mock_request)
        assert result == mock_service

    async def test_missing_raises_service_unavailable(self, mock_request):
        mock_request.app.state.services.cache_aside_service = None
        with pytest.raises(ServiceUnavailableError, match="Operator cache service not available"):
            await get_g8ee_operator_cache(mock_request)

    async def test_none_raises_service_unavailable(self, mock_request):
        mock_request.app.state.services.cache_aside_service = None
        with pytest.raises(ServiceUnavailableError, match="Operator cache service not available"):
            await get_g8ee_operator_cache(mock_request)


class TestGetOperatorCommandService:
    async def test_returns_service_from_app_state(self, mock_request):
        mock_service = MagicMock()
        mock_request.app.state.services.operator_command_service = mock_service
        result = await get_g8ee_operator_command_service(mock_request)
        assert result == mock_service

    async def test_missing_raises_service_unavailable(self, mock_request):
        mock_request.app.state.services.operator_command_service = None
        with pytest.raises(ServiceUnavailableError, match="Operator Command Service not available"):
            await get_g8ee_operator_command_service(mock_request)

    async def test_none_raises_service_unavailable(self, mock_request):
        mock_request.app.state.services.operator_command_service = None
        with pytest.raises(ServiceUnavailableError, match="Operator Command Service not available"):
            await get_g8ee_operator_command_service(mock_request)


class TestGetG8eeAttachmentService:
    async def test_returns_service_from_app_state(self, mock_request):
        mock_service = MagicMock()
        mock_request.app.state.services.attachment_service = mock_service
        result = await get_g8ee_attachment_service(mock_request)
        assert result == mock_service

    async def test_missing_raises_service_unavailable(self, mock_request):
        mock_request.app.state.services.attachment_service = None
        with pytest.raises(ServiceUnavailableError, match="Attachment Service not available"):
            await get_g8ee_attachment_service(mock_request)

    async def test_none_raises_service_unavailable(self, mock_request):
        mock_request.app.state.services.attachment_service = None
        with pytest.raises(ServiceUnavailableError, match="Attachment Service not available"):
            await get_g8ee_attachment_service(mock_request)


class TestGetG8eeCurrentActiveUser:
    async def test_returns_authenticated_user_from_request_state(self, mock_request):
        mock_request.state.user = build_authenticated_user(
            uid="user-123",
            user_id="user-123",
            email="test@example.com",
            organization_id="org-123",
            web_session_id="session-123",
            auth_method=AuthMethod.PROXY,
        )
        result = await get_g8ee_current_active_user(mock_request)
        assert result.uid == "user-123"
        assert result.email == "test@example.com"
        assert result.auth_method == AuthMethod.PROXY

    async def test_missing_raises_authentication_error(self, mock_request):
        mock_request.state.user = None
        with pytest.raises(AuthenticationError, match="Authentication required"):
            await get_g8ee_current_active_user(mock_request)

    async def test_http_status_is_401(self, mock_request):
        mock_request.state.user = None
        with pytest.raises(AuthenticationError) as exc_info:
            await get_g8ee_current_active_user(mock_request)
        assert exc_info.value.get_http_status() == 401


def _make_internal_request(client_ip, path, headers=None, settings_token=None):
    request = MagicMock(spec=Request)
    request.client = MagicMock()
    request.client.host = client_ip
    request.url = MagicMock()
    request.url.path = path
    request.method = "GET"
    request.headers = headers or {}
    request.app = MagicMock()
    if settings_token is not None:
        settings = MagicMock()
        request.app.state.settings = settings
    else:
        del request.app.state.settings
    return request


class TestRequireProxyAuth:
    async def test_proxy_headers_return_authenticated_user(self, mock_request, mock_settings):
        mock_request.app.state.settings = mock_settings
        mock_request.headers = {
            PROXY_USER_ID_HEADER: "user-abc",
            PROXY_USER_EMAIL_HEADER: "user@example.com",
            PROXY_ORGANIZATION_ID_HEADER: "org-xyz",
        }
        result = await require_proxy_auth(mock_request, mock_settings)
        assert result.uid == "user-abc"
        assert result.email == "user@example.com"
        assert result.organization_id == "org-xyz"
        assert result.auth_method == AuthMethod.PROXY

    async def test_g8e_headers_without_proxy_identity_raise_authentication_error(self, mock_request):
        settings = MagicMock()
        mock_request.headers = {
            **TEST_G8E_HEADERS,
            "x-g8e-user-id": "internal-user",
            "x-g8e-websession-id": "sess-abc",
        }
        with pytest.raises(AuthenticationError, match="Authentication required"):
            await require_proxy_auth(mock_request, settings)

    async def test_no_auth_raises_authentication_error(self, mock_request, mock_settings):
        mock_request.headers = {}
        with pytest.raises(AuthenticationError, match="Authentication required"):
            await require_proxy_auth(mock_request, mock_settings)

    async def test_authentication_error_http_status_is_401(self, mock_request, mock_settings):
        mock_request.headers = {}
        with pytest.raises(AuthenticationError) as exc_info:
            await require_proxy_auth(mock_request, mock_settings)
        assert exc_info.value.get_http_status() == 401

    async def test_proxy_user_id_without_email_raises_authentication_error(self, mock_request, mock_settings):
        mock_request.headers = {PROXY_USER_ID_HEADER: "user-abc"}
        with pytest.raises(AuthenticationError):
            await require_proxy_auth(mock_request, mock_settings)


class TestHealthCheckDependencies:
    @pytest.fixture
    def healthy_request(self, mock_settings):
        request = MagicMock(spec=Request)
        request.app = MagicMock()
        request.app.state.settings = mock_settings
        request.app.state.pubsub_client = MagicMock()
        request.app.state.services.cache_aside_service = MagicMock()
        request.app.state.services.case_data_service = MagicMock()
        request.app.state.services.investigation_service = MagicMock()
        request.app.state.services.memory_data_service = MagicMock()
        request.app.state.services.chat_pipeline = MagicMock()
        request.app.state.services.attachment_service = MagicMock()
        return request

    async def test_all_healthy_returns_healthy_result(self, healthy_request):
        # Set up all needed attributes in the mock request
        healthy_request.app.state.services.investigation_data_service = MagicMock()

        health = await health_check_dependencies(healthy_request)

        assert health.component == ComponentName.G8EE
        assert health.overall_status == HealthStatus.HEALTHY
        assert health.dependencies["settings"].status == HealthStatus.HEALTHY
        assert health.dependencies["cache_aside_service"].status == HealthStatus.HEALTHY
        assert health.dependencies["investigation_data_service"].status == HealthStatus.HEALTHY
        assert health.dependencies["investigation_service"].status == HealthStatus.HEALTHY
        assert health.dependencies["memory_service"].status == HealthStatus.HEALTHY
        assert health.dependencies["chat_pipeline"].status == HealthStatus.HEALTHY
        assert health.dependencies["attachment_service"].status == HealthStatus.HEALTHY
        assert health.unhealthy_dependencies is None

    async def test_missing_services_reported_as_unhealthy(self, healthy_request):
        healthy_request.app.state.services.cache_aside_service = None
        healthy_request.app.state.services.investigation_service = None

        health = await health_check_dependencies(healthy_request)

        assert health.overall_status == HealthStatus.UNHEALTHY
        assert health.dependencies["cache_aside_service"].status == HealthStatus.UNHEALTHY
        assert health.dependencies["investigation_service"].status == HealthStatus.UNHEALTHY
        assert health.unhealthy_dependencies is not None
        assert "cache_aside_service" in health.unhealthy_dependencies
        assert "investigation_service" in health.unhealthy_dependencies

    async def test_timestamp_is_present(self, healthy_request):
        health = await health_check_dependencies(healthy_request)

        assert health.timestamp is not None
