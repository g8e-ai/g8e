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

from unittest.mock import MagicMock, AsyncMock

import pytest
from fastapi import Request

from app.constants import (
    X_PROXY_ORGANIZATION_ID,
    X_PROXY_USER_EMAIL,
    X_PROXY_USER_ID,
    AuthMethod,
    ComponentName,
    HealthStatus,
)
from app.dependencies import (
    get_g8ee_attachment_service,
    get_g8ee_auth_service,
    get_g8ee_cache_aside_service,
    get_g8ee_case_data_service,
    get_g8ee_chat_pipeline,
    get_g8ee_chat_task_manager,
    get_g8ee_current_active_user,
    get_g8ee_investigation_service,
    get_g8ee_kv_cache_client,
    get_g8ee_operator_cache,
    get_g8ee_operator_command_service,
    get_g8ee_app_settings,
    get_g8ee_pubsub_client,
    health_check_dependencies,
    require_authenticated_context,
    require_authenticated_user,
)
from app.constants.generated_paths import PathConstants, PortConstants
from app.errors import (
    AuthenticationError,
    ConfigurationError,
    ServiceUnavailableError,
)
from app.models.settings import G8eeAppSettings
from tests.fakes.factories import build_authenticated_user

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
        # We need a real G8eeAppSettings object for this test to be meaningful
        settings = G8eeAppSettings(port=PortConstants.G8E_PORT_G8EE_HTTPS)
        mock_request.app.state.settings = settings
        result = await get_g8ee_app_settings(mock_request)
        assert result.port == settings.port
        assert isinstance(result, G8eeAppSettings)

    async def test_missing_raises_configuration_error(self, mock_request):
        # In a real app, if the attribute is missing, it's a configuration failure
        if hasattr(mock_request.app.state, "settings"):
            delattr(mock_request.app.state, "settings")

        with pytest.raises(ConfigurationError, match="Settings not available"):
            await get_g8ee_app_settings(mock_request)


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


class TestRequireAuthenticatedUser:
    async def test_proxy_headers_return_authenticated_user(self, mock_request, mock_settings):
        mock_auth_service = MagicMock()
        mock_auth_service.authenticate_request = AsyncMock(return_value=build_authenticated_user(
            uid="user-abc",
            user_id="user-abc",
            email="user@example.com",
            organization_id="org-xyz",
            web_session_id="session-abc",
            auth_method=AuthMethod.PROXY,
        ))
        result = await require_authenticated_user(mock_request, mock_settings, mock_auth_service)
        assert result.uid == "user-abc"
        assert result.email == "user@example.com"
        assert result.organization_id == "org-xyz"
        assert result.auth_method == AuthMethod.PROXY

    async def test_no_auth_raises_authentication_error(self, mock_request, mock_settings):
        mock_auth_service = MagicMock()
        mock_auth_service.authenticate_request = AsyncMock(side_effect=AuthenticationError("Authentication required"))
        with pytest.raises(AuthenticationError, match="Authentication required"):
            await require_authenticated_user(mock_request, mock_settings, mock_auth_service)

    async def test_authentication_error_http_status_is_401(self, mock_request, mock_settings):
        mock_auth_service = MagicMock()
        mock_auth_service.authenticate_request = AsyncMock(side_effect=AuthenticationError("Authentication required"))
        with pytest.raises(AuthenticationError) as exc_info:
            await require_authenticated_user(mock_request, mock_settings, mock_auth_service)
        assert exc_info.value.get_http_status() == 401


class TestRequireAuthenticatedContext:
    async def test_returns_context_from_auth_service(self, mock_request):
        mock_user = build_authenticated_user(
            uid="user-123",
            user_id="user-123",
            email="user@example.com",
            organization_id="org-xyz",
            web_session_id="session-abc",
            auth_method=AuthMethod.PROXY,
        )
        mock_auth_service = MagicMock()
        from app.models.http_context import G8eHttpContext
        mock_context = G8eHttpContext(user_id="user-123", source_component=ComponentName.G8EE)
        mock_auth_service.get_validated_context = AsyncMock(return_value=mock_context)

        result = await require_authenticated_context(mock_request, mock_user, mock_auth_service)
        assert result == mock_context
        mock_auth_service.get_validated_context.assert_called_once_with(mock_request, mock_user, is_exempt_path=False)


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
