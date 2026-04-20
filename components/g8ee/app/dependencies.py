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

from fastapi import Depends, Request

from app.clients.kv_cache_client import KVCacheClient
from app.clients.pubsub_client import PubSubClient
from app.clients.blob_client import BlobClient
from app.models.settings import G8eePlatformSettings, G8eeUserSettings
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
    validate_internal_origin,
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
from .services.protocols import ExecutionRegistryProtocol, SettingsServiceProtocol
from .services.infra.g8ed_event_service import EventService
from .services.infra.internal_http_client import InternalHttpClient
from .services.operator.approval_service import OperatorApprovalService
from .services.operator.command_service import OperatorCommandService
from .services.operator.heartbeat_service import OperatorHeartbeatService
from .services.mcp.gateway_service import MCPGatewayService

logger = logging.getLogger(__name__)


def _verify_internal_auth_token(request: Request, settings: G8eePlatformSettings) -> bool:
    from app.security.auth import verify_internal_auth_token
    return verify_internal_auth_token(request, settings)


async def require_internal_origin(request: Request) -> bool:
    return await validate_internal_origin(request)


async def get_g8ee_settings_service(request: Request) -> SettingsServiceProtocol:
    service = getattr(request.app.state, "settings_service", None)
    if not service:
        logger.error("Settings service not found in app state")
        raise ServiceUnavailableError("Settings service not available")
    return service


async def get_g8ee_platform_settings(request: Request) -> G8eePlatformSettings:
    if not hasattr(request.app.state, "settings"):
        logger.error("Settings not found in app state - g8ee initialization may have failed")
        raise ConfigurationError("Settings not available")

    return request.app.state.settings


async def get_g8ee_pubsub_client(request: Request) -> PubSubClient:
    client = getattr(request.app.state, "pubsub_client", None)
    if not client:
        logger.error("PubSubClient not found in app state - g8ee initialization may have failed")
        raise ServiceUnavailableError("PubSubClient not available")

    return client


async def get_g8ee_kv_cache_client(request: Request) -> KVCacheClient:
    client = getattr(request.app.state, "kv_cache_client", None)
    if not client:
        logger.error("KVCacheClient not found in app state - g8ee initialization may have failed")
        raise ServiceUnavailableError("KVCacheClient not available")

    return client


async def get_g8ee_blob_client(request: Request) -> BlobClient:
    client = getattr(request.app.state, "blob_client", None)
    if not client:
        logger.error("BlobClient not found in app state - g8ee initialization may have failed")
        raise ServiceUnavailableError("BlobClient not available")

    return client


async def get_g8ee_cache_aside_service(request: Request) -> CacheAsideService:
    service = getattr(request.app.state, "cache_aside_service", None)
    if not service:
        logger.error("Cache service not found in app state - g8ee initialization may have failed")
        raise ServiceUnavailableError("Cache service not available")

    return service


async def get_g8ee_case_data_service(request: Request) -> CaseDataService:
    service = getattr(request.app.state, "case_data_service", None)
    if not service:
        logger.error("Case Data Service not found in app state - g8ee initialization may have failed")
        raise ServiceUnavailableError("Case Data Service not available")
    return service


async def get_g8ee_investigation_data_service(request: Request) -> InvestigationDataService:
    service = getattr(request.app.state, "investigation_data_service", None)
    if not service:
        logger.error("Investigation Data Service not found in app state - g8ee initialization may have failed")
        raise ServiceUnavailableError("Investigation Data Service not available")

    return service


async def get_g8ee_investigation_service(request: Request) -> InvestigationService:
    service = getattr(request.app.state, "investigation_service", None)
    if not service:
        logger.error("Investigation Domain Service not found in app state")
        raise ServiceUnavailableError("Investigation Domain Service not available")
    return service


async def get_g8ee_execution_registry(request: Request) -> ExecutionRegistryProtocol:
    service = getattr(request.app.state, "execution_registry", None)
    if not service:
        logger.error("Execution Registry Service not found in app state")
        raise ServiceUnavailableError("Execution Registry Service not available")
    return service


async def get_g8ee_memory_service(request: Request) -> MemoryDataService:
    service = getattr(request.app.state, "memory_service", None)
    if not service:
        logger.error("Memory Data service not found in app state - g8ee initialization may have failed")
        raise ServiceUnavailableError("Memory service not available")
    return service


async def get_g8ee_memory_generation_service(request: Request) -> MemoryGenerationService:
    service = getattr(request.app.state, "memory_generation_service", None)
    if not service:
        logger.error("Memory generation service not found in app state - g8ee initialization may have failed")
        raise ServiceUnavailableError("Memory generation service not available")
    return service


async def get_g8ee_chat_pipeline(request: Request) -> ChatPipelineService:
    service = getattr(request.app.state, "chat_pipeline", None)
    if not service:
        logger.error("Chat Pipeline not found in app state - g8ee initialization may have failed")
        raise ServiceUnavailableError("Chat Pipeline not available", component=ComponentName.G8EE)

    return service


async def get_g8ee_investigation_domain_service(request: Request) -> InvestigationService:
    return await get_g8ee_investigation_service(request)


async def get_g8ee_grounding_service(request: Request) -> GroundingService:
    service = getattr(request.app.state, "grounding_service", None)
    if not service:
        logger.error("Grounding service not found in app state - g8ee initialization may have failed")
        raise ServiceUnavailableError("Grounding service not available", component=ComponentName.G8EE)

    return service


async def get_g8eeweb_search_provider(request: Request) -> WebSearchProvider | None:
    return getattr(request.app.state, "web_search_provider", None)


async def get_g8ee_chat_task_manager(request: Request) -> BackgroundTaskManager:
    service = getattr(request.app.state, "chat_task_manager", None)
    if not service:
        logger.error("Chat Task Manager not found in app state - g8ee initialization may have failed")
        raise ServiceUnavailableError("Chat Task Manager not available", component=ComponentName.G8EE)

    return service


async def get_g8ee_operator_cache(request: Request) -> CacheAsideService:
    service = getattr(request.app.state, "operator_cache_aside_service", None)
    if not service:
        logger.error("Operator cache service not found in app state - g8ee initialization may have failed")
        raise ServiceUnavailableError("Operator cache service not available")

    return service


async def get_g8ee_approval_service(request: Request) -> OperatorApprovalService:
    service = getattr(request.app.state, "approval_service", None)
    if not service:
        logger.error("Operator Approval Service not found in app state - g8ee initialization may have failed")
        raise ServiceUnavailableError("Operator Approval Service not available")
    return service


async def get_g8ee_operator_command_service(request: Request) -> OperatorCommandService:
    service = getattr(request.app.state, "operator_command_service", None)
    if not service:
        logger.error("Operator Command Service not found in app state - g8ee initialization may have failed")
        raise ServiceUnavailableError("Operator Command Service not available")

    return service


async def get_g8ee_heartbeat_service(request: Request) -> OperatorHeartbeatService:
    service = getattr(request.app.state, "heartbeat_service", None)
    if not service:
        logger.error("Operator Heartbeat Service not found in app state - g8ee initialization may have failed")
        raise ServiceUnavailableError("Operator Heartbeat Service not available")
    return service


async def get_g8ee_blob_service(request: Request) -> BlobService:
    service = getattr(request.app.state, "blob_service", None)
    if not service:
        logger.error("Blob Service not found in app state - g8ee initialization may have failed")
        raise ServiceUnavailableError("Blob Service not available")
    return service


async def get_g8ee_mcp_gateway_service(request: Request) -> MCPGatewayService:
    service = getattr(request.app.state, "mcp_gateway_service", None)
    if not service:
        logger.error("MCP Gateway Service not found in app state - g8ee initialization may have failed")
        raise ServiceUnavailableError("MCP Gateway Service not available")
    return service


async def get_g8ee_attachment_service(request: Request) -> AttachmentService:
    service = getattr(request.app.state, "attachment_service", None)
    if not service:
        logger.error("Attachment Service not found in app state - g8ee initialization may have failed")
        raise ServiceUnavailableError("Attachment Service not available")

    return service


async def get_g8ee_g8ed_http_client(request: Request) -> InternalHttpClient:
    client = getattr(request.app.state, "internal_http_client", None)
    if not client:
        logger.error("g8ed HTTP client not found in app state - g8ee initialization may have failed")
        raise ServiceUnavailableError("g8ed HTTP client not available")

    return client


async def get_g8ee_event_service(request: Request) -> EventService:
    service = getattr(request.app.state, "g8ed_event_service", None)
    if not service:
        logger.error("g8ed event service not found in app state - g8ee initialization may have failed")
        raise ServiceUnavailableError("g8ed event service not available")

    return service


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
    "get_g8ee_platform_settings",
    "get_g8ee_pubsub_client",
    "get_g8ee_kv_cache_client",
    "get_g8ee_blob_client",
    "get_g8ee_blob_service",
    "get_g8ee_cache_aside_service",
    "get_g8ee_investigation_data_service",
    "get_g8ee_investigation_service",
    "get_g8ee_case_data_service",
    "get_g8ee_memory_service",
    "get_g8ee_memory_generation_service",
    "get_g8ee_chat_pipeline",
    "get_g8ee_chat_task_manager",
    "get_g8ee_operator_cache",
    "get_g8ee_approval_service",
    "get_g8ee_operator_command_service",
    "get_g8ee_attachment_service",
    "get_g8ee_g8ed_http_client",
    "get_g8ee_event_service",
    "get_g8e_http_context",
    "get_g8ee_current_active_user",
    "require_proxy_auth",
    "require_internal_origin",
    "is_infrastructure_health_check_ip",
    "health_check_dependencies",
    "get_g8ee_grounding_service",
    "get_g8eeweb_search_provider",
    "get_g8ee_mcp_gateway_service",
]
