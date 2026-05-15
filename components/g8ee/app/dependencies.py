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

import logging
from typing import cast

from fastapi import Depends, Request

from app.clients.kv_cache_client import KVCacheClient
from app.clients.pubsub_client import PubSubClient
from app.clients.blob_client import BlobClient
from app.models.settings import G8eePlatformSettings, G8eeUserSettings
from app.models.state import G8eeAppState
from app.services.service_factory import AllServices
from app.constants import (
    ComponentName,
    G8eHeaders,
)
from app.errors import (
    AuthenticationError,
    ConfigurationError,
    ServiceUnavailableError,
)
from app.models.auth import AuthenticatedUser
from app.models.health import HealthCheckResult
from app.models.http_context import G8eHttpContext
from app.services.cache.cache_aside import CacheAsideService
from app.security.auth import (
    is_infrastructure_health_check_ip,
    authenticate_proxy_or_internal,
)
from app.services.infra.health_service import HealthService

from .services.data.case_data_service import CaseDataService
from .services.investigation.investigation_service import InvestigationService
from .services.investigation.investigation_data_service import InvestigationDataService
from .services.investigation.memory_data_service import MemoryDataService
from .services.ai.memory_generation_service import MemoryGenerationService
from .services.ai.grounding.grounding_service import GroundingService
from .services.ai.grounding.web_search_provider import WebSearchProvider
from .services.ai.chat_pipeline import ChatPipelineService
from .services.ai.chat_task_manager import BackgroundTaskManager
from .services.data.attachment_store_service import AttachmentService
from .db.blob_service import BlobService
from .services.protocols import SettingsServiceProtocol
from .services.infra.event_service import EventService
from .services.infra.internal_http_client import InternalHttpClient
from .services.operator.approval_service import OperatorApprovalService
from .services.operator.command_service import OperatorCommandService
from app.services.operator.heartbeat_service import HeartbeatSnapshotService
from .services.operator.operator_data_service import OperatorDataService
from .services.operator.operator_lifecycle_service import OperatorLifecycleService
from .services.operator.operator_session_service import OperatorSessionService
from .services.operator.operator_auth_service import OperatorAuthService
from .services.operator.session_auth_listener import SessionAuthListener
from .services.auth.api_key_service import ApiKeyService
from .services.auth.certificate_service import CertificateService
from .services.infra.settings_service import SettingsService
logger = logging.getLogger(__name__)

__all__ = [
    "get_g8e_http_context",
    "get_g8ee_api_key_service",
    "get_g8ee_approval_service",
    "get_g8ee_attachment_service",
    "get_g8ee_blob_client",
    "get_g8ee_blob_service",
    "get_g8ee_cache_aside_service",
    "get_g8ee_case_data_service",
    "get_g8ee_certificate_service",
    "get_g8ee_chat_pipeline",
    "get_g8ee_chat_task_manager",
    "get_g8ee_client_http_client",
    "get_g8ee_current_active_user",
    "get_g8ee_event_service",
    "get_g8ee_grounding_service",
    "get_g8ee_investigation_data_service",
    "get_g8ee_investigation_service",
    "get_g8ee_kv_cache_client",
    "get_g8ee_memory_generation_service",
    "get_g8ee_memory_service",
    "get_g8ee_operator_auth_service",
    "get_g8ee_operator_cache",
    "get_g8ee_operator_command_service",
    "get_g8ee_operator_data_service",
    "get_g8ee_operator_lifecycle_service",
    "get_g8ee_operator_session_service",
    "get_g8ee_platform_settings",
    "get_g8ee_pubsub_client",
    "get_g8ee_session_auth_listener",
    "get_g8eeweb_search_provider",
    "health_check_dependencies",
    "is_infrastructure_health_check_ip",
    "require_proxy_auth",
]


async def get_g8ee_all_services(request: Request) -> AllServices:
    state = cast(G8eeAppState, request.app.state)
    if not hasattr(state, "services") or not state.services:
        logger.error("AllServices not found in app state")
        raise ServiceUnavailableError("Services not available")
    return state.services


async def get_g8ee_settings_service(request: Request) -> SettingsServiceProtocol:
    state = cast(G8eeAppState, request.app.state)
    service = state.services.settings_service
    if not service:
        logger.error("Settings service not found in app state")
        raise ServiceUnavailableError("Settings service not available")
    return service


async def get_g8ee_settings_service_write(request: Request) -> SettingsService:
    """Get SettingsService for write operations (e.g., updating platform settings)."""
    state = cast(G8eeAppState, request.app.state)
    service = state.services.settings_service
    if not service:
        logger.error("Settings service not found in app state")
        raise ServiceUnavailableError("Settings service not available")
    return service


async def get_g8ee_platform_settings(request: Request) -> G8eePlatformSettings:
    state = cast(G8eeAppState, request.app.state)
    if not hasattr(state, "settings"):
        logger.error("Settings not found in app state - g8ee initialization may have failed")
        raise ConfigurationError("Settings not available")

    return state.settings


async def get_g8ee_pubsub_client(request: Request) -> PubSubClient:
    state = cast(G8eeAppState, request.app.state)
    client = state.pubsub_client
    if not client:
        logger.error("PubSubClient not found in app state - g8ee initialization may have failed")
        raise ServiceUnavailableError("PubSubClient not available")

    return client


async def get_g8ee_kv_cache_client(request: Request) -> KVCacheClient:
    state = cast(G8eeAppState, request.app.state)
    client = state.kv_cache_client
    if not client:
        logger.error("KVCacheClient not found in app state - g8ee initialization may have failed")
        raise ServiceUnavailableError("KVCacheClient not available")

    return client


async def get_g8ee_blob_client(request: Request) -> BlobClient:
    state = cast(G8eeAppState, request.app.state)
    client = state.blob_client
    if not client:
        logger.error("BlobClient not found in app state - g8ee initialization may have failed")
        raise ServiceUnavailableError("BlobClient not available")

    return client


async def get_g8ee_cache_aside_service(request: Request) -> CacheAsideService:
    state = cast(G8eeAppState, request.app.state)
    service = state.services.cache_aside_service
    if not service:
        logger.error("Cache service not found in app state - g8ee initialization may have failed")
        raise ServiceUnavailableError("Cache service not available")

    return service


async def get_g8ee_case_data_service(request: Request) -> CaseDataService:
    state = cast(G8eeAppState, request.app.state)
    service = state.services.case_data_service
    if not service:
        logger.error("Case Data Service not found in app state - g8ee initialization may have failed")
        raise ServiceUnavailableError("Case Data Service not available")
    return service


async def get_g8ee_investigation_data_service(request: Request) -> InvestigationDataService:
    state = cast(G8eeAppState, request.app.state)
    service = state.services.investigation_data_service
    if not service:
        logger.error("Investigation Data Service not found in app state - g8ee initialization may have failed")
        raise ServiceUnavailableError("Investigation Data Service not available")

    return service


async def get_g8ee_investigation_service(request: Request) -> InvestigationService:
    state = cast(G8eeAppState, request.app.state)
    service = state.services.investigation_service
    if not service:
        logger.error("Investigation Domain Service not found in app state")
        raise ServiceUnavailableError("Investigation Domain Service not available")
    return service


async def get_g8ee_memory_service(request: Request) -> MemoryDataService:
    state = cast(G8eeAppState, request.app.state)
    service = state.services.memory_data_service
    if not service:
        logger.error("Memory Data service not found in app state - g8ee initialization may have failed")
        raise ServiceUnavailableError("Memory service not available")
    return service


async def get_g8ee_memory_generation_service(request: Request) -> MemoryGenerationService:
    state = cast(G8eeAppState, request.app.state)
    service = state.services.memory_generation_service
    if not service:
        logger.error("Memory generation service not found in app state - g8ee initialization may have failed")
        raise ServiceUnavailableError("Memory generation service not available")
    return service


async def get_g8ee_chat_pipeline(request: Request) -> ChatPipelineService:
    state = cast(G8eeAppState, request.app.state)
    service = state.services.chat_pipeline
    if not service:
        logger.error("Chat Pipeline not found in app state - g8ee initialization may have failed")
        raise ServiceUnavailableError("Chat Pipeline not available", component=ComponentName.G8EE)

    return service


async def get_g8ee_investigation_domain_service(request: Request) -> InvestigationService:
    return await get_g8ee_investigation_service(request)


async def get_g8ee_grounding_service(request: Request) -> GroundingService:
    state = cast(G8eeAppState, request.app.state)
    service = state.services.grounding_service
    if not service:
        logger.error("Grounding service not found in app state - g8ee initialization may have failed")
        raise ServiceUnavailableError("Grounding service not available", component=ComponentName.G8EE)

    return service


async def get_g8eeweb_search_provider(request: Request) -> WebSearchProvider | None:
    state = cast(G8eeAppState, request.app.state)
    return getattr(state.services, "web_search_provider", None)


async def get_g8ee_chat_task_manager(request: Request) -> BackgroundTaskManager:
    state = cast(G8eeAppState, request.app.state)
    service = state.services.chat_task_manager
    if not service:
        logger.error("Chat Task Manager not found in app state - g8ee initialization may have failed")
        raise ServiceUnavailableError("Chat Task Manager not available", component=ComponentName.G8EE)

    return service


async def get_g8ee_operator_cache(request: Request) -> CacheAsideService:
    state = cast(G8eeAppState, request.app.state)
    service = state.services.cache_aside_service
    if not service:
        logger.error("Operator cache service not found in app state - g8ee initialization may have failed")
        raise ServiceUnavailableError("Operator cache service not available")

    return service


async def get_g8ee_approval_service(request: Request) -> OperatorApprovalService:
    state = cast(G8eeAppState, request.app.state)
    service = state.services.approval_service
    if not service:
        logger.error("Operator Approval Service not found in app state - g8ee initialization may have failed")
        raise ServiceUnavailableError("Operator Approval Service not available")
    return cast(OperatorApprovalService, service)


async def get_g8ee_operator_command_service(request: Request) -> OperatorCommandService:
    state = cast(G8eeAppState, request.app.state)
    service = state.services.operator_command_service
    if not service:
        logger.error("Operator Command Service not found in app state - g8ee initialization may have failed")
        raise ServiceUnavailableError("Operator Command Service not available")

    return service


async def get_g8ee_heartbeat_service(request: Request) -> HeartbeatSnapshotService:
    state = cast(G8eeAppState, request.app.state)
    service = state.services.heartbeat_service
    if not service:
        logger.error("Heartbeat service not found in app state - g8ee initialization may have failed")
        raise ServiceUnavailableError("Heartbeat service not available")
    return cast(HeartbeatSnapshotService, service)


async def get_g8ee_operator_data_service(request: Request) -> OperatorDataService:
    state = cast(G8eeAppState, request.app.state)
    service = state.services.operator_data_service
    if not service:
        logger.error("Operator Data Service not found in app state - g8ee initialization may have failed")
        raise ServiceUnavailableError("Operator Data Service not available")
    return cast(OperatorDataService, service)


async def get_g8ee_operator_lifecycle_service(request: Request) -> OperatorLifecycleService:
    state = cast(G8eeAppState, request.app.state)
    service = state.services.operator_lifecycle_service
    if not service:
        logger.error("Operator Lifecycle Service not found in app state - g8ee initialization may have failed")
        raise ServiceUnavailableError("Operator Lifecycle Service not available")
    return cast(OperatorLifecycleService, service)


async def get_g8ee_operator_session_service(request: Request) -> OperatorSessionService:
    state = cast(G8eeAppState, request.app.state)
    service = state.services.operator_session_service
    if not service:
        logger.error("Operator Session Service not found in app state - g8ee initialization may have failed")
        raise ServiceUnavailableError("Operator Session Service not available")
    return service


async def get_g8ee_operator_auth_service(request: Request) -> OperatorAuthService:
    state = cast(G8eeAppState, request.app.state)
    service = state.services.operator_auth_service
    if not service:
        logger.error("Operator Auth Service not found in app state - g8ee initialization may have failed")
        raise ServiceUnavailableError("Operator Auth Service not available")
    return service


async def get_g8ee_session_auth_listener(request: Request) -> SessionAuthListener:
    state = cast(G8eeAppState, request.app.state)
    service = state.services.session_auth_listener
    if not service:
        logger.error("Session Auth Listener not found in app state - g8ee initialization may have failed")
        raise ServiceUnavailableError("Session Auth Listener not available")
    return service


async def get_g8ee_api_key_service(request: Request) -> ApiKeyService:
    state = cast(G8eeAppState, request.app.state)
    service = state.services.api_key_service
    if not service:
        logger.error("API Key Service not found in app state - g8ee initialization may have failed")
        raise ServiceUnavailableError("API Key Service not available")
    return service


async def get_g8ee_certificate_service(request: Request) -> CertificateService:
    state = cast(G8eeAppState, request.app.state)
    service = state.services.certificate_service
    if not service:
        logger.error("Certificate Service not found in app state - g8ee initialization may have failed")
        raise ServiceUnavailableError("Certificate Service not available")
    return service


async def get_g8ee_blob_service(request: Request) -> BlobService:
    state = cast(G8eeAppState, request.app.state)
    service = state.services.blob_service
    if not service:
        logger.error("Blob Service not found in app state - g8ee initialization may have failed")
        raise ServiceUnavailableError("Blob Service not available")
    return service


async def get_g8ee_attachment_service(request: Request) -> AttachmentService:
    state = cast(G8eeAppState, request.app.state)
    service = state.services.attachment_service
    if not service:
        logger.error("Attachment Service not found in app state - g8ee initialization may have failed")
        raise ServiceUnavailableError("Attachment Service not available")

    return service


async def get_g8ee_client_http_client(request: Request) -> InternalHttpClient:
    state = cast(G8eeAppState, request.app.state)
    client = state.internal_http_client
    if not client:
        logger.error("internal HTTP client not found in app state - g8ee initialization may have failed")
        raise ServiceUnavailableError("internal HTTP client not available")

    return client


async def get_g8ee_event_service(request: Request) -> EventService:
    state = cast(G8eeAppState, request.app.state)
    service = state.services.event_service
    if not service:
        logger.error("event service not found in app state - g8ee initialization may have failed")
        raise ServiceUnavailableError("event service not available")

    return cast(EventService, service)


async def get_g8e_http_context(request: Request) -> G8eHttpContext:
    return await G8eHttpContext.from_request(request)


async def get_g8ee_user_settings(
    request: Request,
    settings_service: SettingsServiceProtocol = Depends(get_g8ee_settings_service),
) -> G8eeUserSettings:
    """Load per-request G8eeUserSettings following Platform Settings < User Settings.

    Extracts user_id from the G8eHeaders.USER_ID header and overlays user-specific
    settings on top of the platform settings loaded at startup.
    """
    user_id = request.headers.get(G8eHeaders.USER_ID.lower())
    if not user_id:
        # We need to return G8eeUserSettings, so we'll get it via the service
        # which will handle the merging logic.
        return await settings_service.get_user_settings("default") # This will likely return merged platform data
    return await settings_service.get_user_settings(user_id)


async def get_g8ee_current_active_user(request: Request) -> AuthenticatedUser:
    user = getattr(request.state, "user", None)
    if not user:
        logger.error("No authenticated user found in request state")
        raise AuthenticationError("Authentication required")

    return user


async def require_proxy_auth(
    request: Request,
    settings: G8eePlatformSettings = Depends(get_g8ee_platform_settings)
) -> AuthenticatedUser:
    return await authenticate_proxy_or_internal(request, settings)


async def health_check_dependencies(request: Request) -> HealthCheckResult:
    return await HealthService.check_dependencies(request)


__all__ = [
    "get_g8e_http_context",
    "get_g8ee_api_key_service",
    "get_g8ee_approval_service",
    "get_g8ee_attachment_service",
    "get_g8ee_blob_client",
    "get_g8ee_blob_service",
    "get_g8ee_cache_aside_service",
    "get_g8ee_case_data_service",
    "get_g8ee_certificate_service",
    "get_g8ee_chat_pipeline",
    "get_g8ee_chat_task_manager",
    "get_g8ee_client_http_client",
    "get_g8ee_current_active_user",
    "get_g8ee_event_service",
    "get_g8ee_grounding_service",
    "get_g8ee_investigation_data_service",
    "get_g8ee_investigation_service",
    "get_g8ee_kv_cache_client",
    "get_g8ee_memory_generation_service",
    "get_g8ee_memory_service",
    "get_g8ee_operator_auth_service",
    "get_g8ee_operator_cache",
    "get_g8ee_operator_command_service",
    "get_g8ee_operator_data_service",
    "get_g8ee_operator_lifecycle_service",
    "get_g8ee_operator_session_service",
    "get_g8ee_platform_settings",
    "get_g8ee_pubsub_client",
    "get_g8ee_session_auth_listener",
    "get_g8eeweb_search_provider",
    "health_check_dependencies",
    "is_infrastructure_health_check_ip",
    "require_proxy_auth",
]
