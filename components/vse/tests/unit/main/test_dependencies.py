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
    HTTP_FORWARDED_FOR_HEADER,
    HTTP_USER_AGENT_HEADER,
    INTERNAL_AUTH_HEADER,
    PROXY_ORGANIZATION_ID_HEADER,
    PROXY_USER_EMAIL_HEADER,
    PROXY_USER_ID_HEADER,
    AuthMethod,
    ComponentName,
    HealthStatus,
    VSOHeaders,
)
from tests.fakes.headers import TEST_VSO_HEADERS
from app.models.settings import VSEPlatformSettings
from app.dependencies import (
    get_vse_platform_settings,
    get_vse_attachment_service,
    get_vse_cache_aside_service,
    get_vse_case_data_service,
    get_vse_chat_pipeline,
    get_vse_chat_task_manager,
    get_vse_current_active_user,
    get_vse_pubsub_client,
    get_vse_kv_cache_client,
    get_vse_investigation_service,
    get_vse_operator_cache,
    get_vse_operator_command_service,
    get_vso_http_context,
    health_check_dependencies,
    require_internal_origin,
    require_proxy_auth,
)
from app.errors import (
    AuthenticationError,
    AuthorizationError,
    ConfigurationError,
    ServiceUnavailableError,
)
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


class TestGetVseAppSettings:
    async def test_returns_settings_from_app_state(self, mock_request):
        # We need a real VSEPlatformSettings object for this test to be meaningful
        settings = VSEPlatformSettings(port=443)
        mock_request.app.state.settings = settings
        result = await get_vse_platform_settings(mock_request)
        assert result.port == settings.port
        assert isinstance(result, VSEPlatformSettings)

    async def test_missing_raises_configuration_error(self, mock_request):
        # In a real app, if the attribute is missing, it's a configuration failure
        if hasattr(mock_request.app.state, "settings"):
            delattr(mock_request.app.state, "settings")
        
        with pytest.raises(ConfigurationError, match="Settings not available"):
            await get_vse_platform_settings(mock_request)


class TestGetVsePubSubClient:
    async def test_returns_client_from_app_state(self, mock_request):
        mock_client = MagicMock()
        mock_request.app.state.pubsub_client = mock_client
        result = await get_vse_pubsub_client(mock_request)
        assert result == mock_client

    async def test_missing_raises_service_unavailable(self, mock_request):
        mock_request.app.state.pubsub_client = None
        with pytest.raises(ServiceUnavailableError, match="PubSubClient not available"):
            await get_vse_pubsub_client(mock_request)

class TestGetVseKVClient:
    async def test_returns_client_from_app_state(self, mock_request):
        mock_client = MagicMock()
        mock_request.app.state.kv_cache_client = mock_client
        result = await get_vse_kv_cache_client(mock_request)
        assert result == mock_client

    async def test_missing_raises_service_unavailable(self, mock_request):
        mock_request.app.state.kv_cache_client = None
        with pytest.raises(ServiceUnavailableError, match="KVCacheClient not available"):
            await get_vse_kv_cache_client(mock_request)

class TestGetVseCacheService:
    async def test_returns_service_from_app_state(self, mock_request):
        mock_cache = MagicMock()
        mock_request.app.state.cache_aside_service = mock_cache
        result = await get_vse_cache_aside_service(mock_request)
        assert result == mock_cache

    async def test_missing_raises_service_unavailable(self, mock_request):
        mock_request.app.state.cache_aside_service = None
        with pytest.raises(ServiceUnavailableError, match="Cache service not available"):
            await get_vse_cache_aside_service(mock_request)


class TestGetCaseDataService:
    async def test_returns_service_from_app_state(self, mock_request):
        mock_service = MagicMock()
        mock_request.app.state.case_data_service = mock_service
        result = await get_vse_case_data_service(mock_request)
        assert result == mock_service

    async def test_missing_raises_service_unavailable(self, mock_request):
        mock_request.app.state.case_data_service = None
        with pytest.raises(ServiceUnavailableError, match="Case Data Service not available"):
            await get_vse_case_data_service(mock_request)


class TestGetInvestigationService:
    async def test_returns_service_from_app_state(self, mock_request):
        mock_service = MagicMock()
        mock_request.app.state.investigation_service = mock_service
        result = await get_vse_investigation_service(mock_request)
        assert result == mock_service

    async def test_missing_raises_service_unavailable(self, mock_request):
        mock_request.app.state.investigation_service = None
        with pytest.raises(ServiceUnavailableError, match="Investigation Domain Service not available"):
            await get_vse_investigation_service(mock_request)


class TestGetVseChatPipeline:
    async def test_returns_service_from_app_state(self, mock_request):
        mock_service = MagicMock()
        mock_request.app.state.chat_pipeline = mock_service
        result = await get_vse_chat_pipeline(mock_request)
        assert result == mock_service

    async def test_missing_raises_service_unavailable(self, mock_request):
        mock_request.app.state.chat_pipeline = None
        with pytest.raises(ServiceUnavailableError, match="Chat Pipeline not available"):
            await get_vse_chat_pipeline(mock_request)

    async def test_none_raises_service_unavailable(self, mock_request):
        mock_request.app.state.chat_pipeline = None
        with pytest.raises(ServiceUnavailableError, match="Chat Pipeline not available"):
            await get_vse_chat_pipeline(mock_request)


class TestGetVseChatTaskManager:
    async def test_returns_service_from_app_state(self, mock_request):
        mock_service = MagicMock()
        mock_request.app.state.chat_task_manager = mock_service
        result = await get_vse_chat_task_manager(mock_request)
        assert result == mock_service

    async def test_missing_raises_service_unavailable(self, mock_request):
        mock_request.app.state.chat_task_manager = None
        with pytest.raises(ServiceUnavailableError, match="Chat Task Manager not available"):
            await get_vse_chat_task_manager(mock_request)

    async def test_none_raises_service_unavailable(self, mock_request):
        mock_request.app.state.chat_task_manager = None
        with pytest.raises(ServiceUnavailableError, match="Chat Task Manager not available"):
            await get_vse_chat_task_manager(mock_request)


class TestGetOperatorCache:
    async def test_returns_service_from_app_state(self, mock_request):
        mock_service = MagicMock()
        mock_request.app.state.operator_cache_aside_service = mock_service
        result = await get_vse_operator_cache(mock_request)
        assert result == mock_service

    async def test_missing_raises_service_unavailable(self, mock_request):
        mock_request.app.state.operator_cache_aside_service = None
        with pytest.raises(ServiceUnavailableError, match="Operator cache service not available"):
            await get_vse_operator_cache(mock_request)

    async def test_none_raises_service_unavailable(self, mock_request):
        mock_request.app.state.operator_cache_aside_service = None
        with pytest.raises(ServiceUnavailableError, match="Operator cache service not available"):
            await get_vse_operator_cache(mock_request)


class TestGetVseVsaOperatorService:
    async def test_returns_service_from_app_state(self, mock_request):
        mock_service = MagicMock()
        mock_request.app.state.operator_command_service = mock_service
        result = await get_vse_operator_command_service(mock_request)
        assert result == mock_service

    async def test_missing_raises_service_unavailable(self, mock_request):
        mock_request.app.state.operator_command_service = None
        with pytest.raises(ServiceUnavailableError, match="Operator Command Service not available"):
            await get_vse_operator_command_service(mock_request)

    async def test_none_raises_service_unavailable(self, mock_request):
        mock_request.app.state.operator_command_service = None
        with pytest.raises(ServiceUnavailableError, match="Operator Command Service not available"):
            await get_vse_operator_command_service(mock_request)


class TestGetVseAttachmentService:
    async def test_returns_service_from_app_state(self, mock_request):
        mock_service = MagicMock()
        mock_request.app.state.attachment_service = mock_service
        result = await get_vse_attachment_service(mock_request)
        assert result == mock_service

    async def test_missing_raises_service_unavailable(self, mock_request):
        mock_request.app.state.attachment_service = None
        with pytest.raises(ServiceUnavailableError, match="Attachment Service not available"):
            await get_vse_attachment_service(mock_request)

    async def test_none_raises_service_unavailable(self, mock_request):
        mock_request.app.state.attachment_service = None
        with pytest.raises(ServiceUnavailableError, match="Attachment Service not available"):
            await get_vse_attachment_service(mock_request)


class TestGetVseCurrentActiveUser:
    async def test_returns_authenticated_user_from_request_state(self, mock_request):
        mock_request.state.user = build_authenticated_user(
            uid="user-123",
            user_id="user-123",
            email="test@example.com",
            organization_id="org-123",
            web_session_id="session-123",
            auth_method=AuthMethod.PROXY,
        )
        result = await get_vse_current_active_user(mock_request)
        assert result.uid == "user-123"
        assert result.email == "test@example.com"
        assert result.auth_method == AuthMethod.PROXY

    async def test_missing_raises_authentication_error(self, mock_request):
        mock_request.state.user = None
        with pytest.raises(AuthenticationError, match="Authentication required"):
            await get_vse_current_active_user(mock_request)

    async def test_http_status_is_401(self, mock_request):
        mock_request.state.user = None
        with pytest.raises(AuthenticationError) as exc_info:
            await get_vse_current_active_user(mock_request)
        assert exc_info.value.get_http_status() == 401


class TestGetVsoHttpContext:
    async def test_extracts_full_context_from_headers(self, mock_request):
        mock_request.headers = {
            **TEST_VSO_HEADERS,
            VSOHeaders.WEB_SESSION_ID.lower(): "session-123",
            VSOHeaders.USER_ID.lower(): "user-456",
            VSOHeaders.ORGANIZATION_ID.lower(): "org-789",
            VSOHeaders.CASE_ID.lower(): "case-111",
            VSOHeaders.INVESTIGATION_ID.lower(): "inv-222",
            VSOHeaders.BOUND_OPERATORS.lower(): '[{"operator_id": "op-333", "operator_session_id": "sess-333", "status": "bound"}]',
            VSOHeaders.SOURCE_COMPONENT.lower(): "vsod",
        }
        context = await get_vso_http_context(mock_request)
        assert context.web_session_id == "session-123"
        assert context.user_id == "user-456"
        assert context.organization_id == "org-789"
        assert context.case_id == "case-111"
        assert context.investigation_id == "inv-222"
        assert len(context.bound_operators) == 1
        assert context.bound_operators[0].operator_id == "op-333"
        assert context.source_component == ComponentName.VSOD

    async def test_missing_session_id_raises_authentication_error(self, mock_request):
        mock_request.headers = {VSOHeaders.USER_ID.lower(): "user-456", VSOHeaders.SOURCE_COMPONENT.lower(): "vsod"}
        with pytest.raises(AuthenticationError) as exc_info:
            await get_vso_http_context(mock_request)
        assert exc_info.value.get_http_status() == 401

    async def test_missing_user_id_raises_authentication_error(self, mock_request):
        mock_request.headers = {VSOHeaders.WEB_SESSION_ID.lower(): "session-123", VSOHeaders.SOURCE_COMPONENT.lower(): "vsod"}
        with pytest.raises(AuthenticationError) as exc_info:
            await get_vso_http_context(mock_request)
        assert exc_info.value.get_http_status() == 401

    async def test_missing_source_component_raises_authentication_error(self, mock_request):
        mock_request.headers = {
            VSOHeaders.WEB_SESSION_ID.lower(): "session-123",
            VSOHeaders.USER_ID.lower(): "user-456",
        }
        with pytest.raises(AuthenticationError) as exc_info:
            await get_vso_http_context(mock_request)
        assert exc_info.value.get_http_status() == 401

    async def test_invalid_source_component_raises_authentication_error(self, mock_request):
        mock_request.headers = {
            VSOHeaders.WEB_SESSION_ID.lower(): "session-123",
            VSOHeaders.USER_ID.lower(): "user-456",
            VSOHeaders.SOURCE_COMPONENT.lower(): "unknown",
        }
        with pytest.raises(AuthenticationError) as exc_info:
            await get_vso_http_context(mock_request)
        assert exc_info.value.get_http_status() == 401

    async def test_minimal_required_headers_succeeds(self, mock_request):
        mock_request.headers = {
            VSOHeaders.WEB_SESSION_ID.lower(): "session-abc",
            VSOHeaders.USER_ID.lower(): "user-xyz",
            VSOHeaders.SOURCE_COMPONENT.lower(): "vsod",
            VSOHeaders.CASE_ID.lower(): "case-min-001",
            VSOHeaders.INVESTIGATION_ID.lower(): "inv-min-001",
        }
        context = await get_vso_http_context(mock_request)
        assert context.web_session_id == "session-abc"
        assert context.user_id == "user-xyz"
        assert context.organization_id is None
        assert context.case_id == "case-min-001"
        assert context.bound_operators == []

    async def test_source_component_is_enum(self, mock_request):
        mock_request.headers = {
            VSOHeaders.WEB_SESSION_ID.lower(): "session-abc",
            VSOHeaders.USER_ID.lower(): "user-xyz",
            VSOHeaders.SOURCE_COMPONENT.lower(): "vse",
            VSOHeaders.CASE_ID.lower(): "case-src-001",
            VSOHeaders.INVESTIGATION_ID.lower(): "inv-src-001",
        }
        context = await get_vso_http_context(mock_request)
        assert context.source_component == ComponentName.VSE

    async def test_request_id_defaults_when_header_absent(self, mock_request):
        mock_request.headers = {
            VSOHeaders.WEB_SESSION_ID.lower(): "session-abc",
            VSOHeaders.USER_ID.lower(): "user-xyz",
            VSOHeaders.SOURCE_COMPONENT.lower(): "vsod",
            VSOHeaders.CASE_ID.lower(): "case-req-001",
            VSOHeaders.INVESTIGATION_ID.lower(): "inv-req-001",
        }
        context = await get_vso_http_context(mock_request)
        assert context.execution_id is not None
        assert context.execution_id.startswith("exec")

    async def test_request_id_uses_header_when_present(self, mock_request):
        mock_request.headers = {
            VSOHeaders.WEB_SESSION_ID.lower(): "session-abc",
            VSOHeaders.USER_ID.lower(): "user-xyz",
            VSOHeaders.SOURCE_COMPONENT.lower(): "vsod",
            VSOHeaders.CASE_ID.lower(): "case-rid-001",
            VSOHeaders.INVESTIGATION_ID.lower(): "inv-rid-001",
            VSOHeaders.EXECUTION_ID.lower(): "exec_explicit_id",
        }
        context = await get_vso_http_context(mock_request)
        assert context.execution_id == "exec_explicit_id"


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
        settings.auth.internal_auth_token = settings_token
        request.app.state.settings = settings
    else:
        del request.app.state.settings
    return request


class TestRequireInternalOrigin:
    async def test_valid_internal_auth_token_grants_access(self):
        token = "super-secret-token"
        request = _make_internal_request(
            client_ip="192.168.1.1",
            path="/api/internal/test",
            headers={INTERNAL_AUTH_HEADER: token, HTTP_FORWARDED_FOR_HEADER: None, HTTP_USER_AGENT_HEADER: "test"},
            settings_token=token,
        )
        result = await require_internal_origin(request)
        assert result is True

    async def test_gke_health_check_ip_grants_access(self):
        request = _make_internal_request(
            client_ip="35.191.5.10",
            path="/health",
            headers={HTTP_FORWARDED_FOR_HEADER: None, HTTP_USER_AGENT_HEADER: "GoogleHC/1.0"},
        )
        result = await require_internal_origin(request)
        assert result is True

    async def test_localhost_health_path_grants_access(self):
        request = _make_internal_request(
            client_ip="127.0.0.1",
            path="/health/live",
            headers={HTTP_FORWARDED_FOR_HEADER: None, HTTP_USER_AGENT_HEADER: "curl"},
        )
        result = await require_internal_origin(request)
        assert result is True

    async def test_localhost_non_health_path_raises_authorization_error(self):
        request = _make_internal_request(
            client_ip="127.0.0.1",
            path="/api/internal/sensitive",
            headers={HTTP_FORWARDED_FOR_HEADER: None, HTTP_USER_AGENT_HEADER: "curl"},
        )
        with pytest.raises(AuthorizationError) as exc_info:
            await require_internal_origin(request)
        assert exc_info.value.get_http_status() == 403

    async def test_unknown_ip_no_token_raises_authorization_error(self):
        request = _make_internal_request(
            client_ip="203.0.113.50",
            path="/api/internal/test",
            headers={HTTP_FORWARDED_FOR_HEADER: None, HTTP_USER_AGENT_HEADER: "attacker"},
        )
        with pytest.raises(AuthorizationError) as exc_info:
            await require_internal_origin(request)
        assert exc_info.value.get_http_status() == 403

    async def test_wrong_token_raises_authorization_error(self):
        request = _make_internal_request(
            client_ip="203.0.113.50",
            path="/api/internal/test",
            headers={INTERNAL_AUTH_HEADER: "wrong-token", HTTP_FORWARDED_FOR_HEADER: None, HTTP_USER_AGENT_HEADER: "test"},
            settings_token="correct-token",
        )
        with pytest.raises(AuthorizationError):
            await require_internal_origin(request)

    async def test_ipv6_mapped_ipv4_health_check_ip_grants_access(self):
        request = _make_internal_request(
            client_ip="::ffff:10.0.0.5",
            path="/health",
            headers={HTTP_FORWARDED_FOR_HEADER: None, HTTP_USER_AGENT_HEADER: "healthcheck"},
        )
        result = await require_internal_origin(request)
        assert result is True


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

    async def test_internal_token_with_vso_headers_return_authenticated_user(self, mock_request):
        token = "internal-secret"
        settings = MagicMock()
        settings.auth.internal_auth_token = token
        mock_request.headers = {
            **TEST_VSO_HEADERS,
            INTERNAL_AUTH_HEADER: token,
            VSOHeaders.USER_ID.lower(): "internal-user",
            VSOHeaders.WEB_SESSION_ID.lower(): "sess-abc",
            VSOHeaders.ORGANIZATION_ID.lower(): "org-internal",
        }
        result = await require_proxy_auth(mock_request, settings)
        assert result.uid == "internal-user"
        assert result.web_session_id == "sess-abc"
        assert result.auth_method == AuthMethod.INTERNAL

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
        request.app.state.cache_aside_service = MagicMock()
        request.app.state.case_data_service = MagicMock()
        request.app.state.investigation_service = MagicMock()
        request.app.state.memory_service = MagicMock()
        request.app.state.chat_pipeline = MagicMock()
        request.app.state.attachment_service = MagicMock()
        return request

    async def test_all_healthy_returns_healthy_result(self, healthy_request):
        # Set up all needed attributes in the mock request
        healthy_request.app.state.investigation_data_service = MagicMock()
        
        health = await health_check_dependencies(healthy_request)

        assert health.component == ComponentName.VSE
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
        healthy_request.app.state.cache_aside_service = None
        healthy_request.app.state.investigation_service = None

        health = await health_check_dependencies(healthy_request)

        assert health.overall_status == HealthStatus.UNHEALTHY
        assert health.dependencies["cache_aside_service"].status == HealthStatus.UNHEALTHY
        assert health.dependencies["investigation_service"].status == HealthStatus.UNHEALTHY
        assert health.unhealthy_dependencies is not None
        assert "cache_aside_service" in health.unhealthy_dependencies
        assert "investigation_service" in health.unhealthy_dependencies

    async def test_llm_provider_always_healthy(self, healthy_request):
        health = await health_check_dependencies(healthy_request)

        assert health.dependencies["llm_provider"].status == HealthStatus.HEALTHY

    async def test_timestamp_is_present(self, healthy_request):
        health = await health_check_dependencies(healthy_request)

        assert health.timestamp is not None
