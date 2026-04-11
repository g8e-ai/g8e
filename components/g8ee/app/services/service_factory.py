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

"""Service factory for consistent service construction across application and tests."""

from typing import TYPE_CHECKING, cast, Any

from app.services.ai.agent import g8eEngine
from app.services.ai.chat_pipeline import ChatPipelineService
from app.services.ai.chat_task_manager import ChatTaskManager
from app.services.ai.grounding import GroundingService, WebSearchProvider
from app.services.ai.memory_generation_service import MemoryGenerationService
from app.services.ai.request_builder import AIRequestBuilder
from app.services.ai.response_analyzer import AIResponseAnalyzer
from app.services.ai.tool_service import AIToolService
from app.services.cache.cache_aside import CacheAsideService
from app.services.data.attachment_store_service import AttachmentService
from app.services.investigation.investigation_service import InvestigationService
from app.services.investigation.investigation_data_service import InvestigationDataService
from app.services.investigation.memory_data_service import MemoryDataService
from app.services.operator.approval_service import OperatorApprovalService
from app.services.operator.command_service import OperatorCommandService
from app.services.operator.operator_data_service import OperatorDataService
from app.services.infra.g8ed_event_service import EventService
from app.services.data.case_data_service import CaseDataService
from app.services.operator.heartbeat_service import OperatorHeartbeatService
from app.services.operator.execution_registry import ExecutionRegistryService
from app.services.infra.http_service import HTTPService
from app.services.infra.settings_service import SettingsService
from app.services.mcp.gateway_service import MCPGatewayService
from app.models.settings import G8eePlatformSettings

if TYPE_CHECKING:
    from app.services.protocols import (
        InvestigationServiceProtocol,
        InvestigationDataServiceProtocol,
        OperatorDataServiceProtocol,
        MemoryDataServiceProtocol,
        HTTPServiceProtocol,
        OperatorHeartbeatServiceProtocol,
        ExecutionRegistryProtocol,
        EventServiceProtocol,
    )


class ServiceFactory:
    """Factory for creating g8ee services with consistent dependency injection."""
    
    @staticmethod
    def create_core_services(
        settings: G8eePlatformSettings, cache_aside_service: CacheAsideService
    ) -> dict[str, object]:
        """Create core services that other services depend on."""
        from app.services.infra.internal_http_client import InternalHttpClient
        from app.services.infra.http_service import HTTPService
        from app.services.infra.g8ed_event_service import EventService

        # Create HTTP service first to manage all HTTP clients
        http_service: "HTTPServiceProtocol" = HTTPService()

        internal_http_client = InternalHttpClient(settings)

        # Register g8ed HTTP client with the HTTP service
        http_service.set_http_client(internal_http_client.client, "g8ed")

        g8ed_event_service: "EventServiceProtocol" = EventService(
            internal_http_client=internal_http_client
        )

        return {
            'http_service': http_service,
            'internal_http_client': internal_http_client,
            'g8ed_event_service': g8ed_event_service,
        }
    
    @staticmethod
    def create_data_services(
        settings: G8eePlatformSettings,
        cache_aside_service: CacheAsideService,
        core_services: dict[str, object],
    ) -> dict[str, object]:
        """Create data services for CRUD operations."""
        investigation_data_service = InvestigationDataService(
            cache=cache_aside_service
        )

        operator_data_service = OperatorDataService(
            cache=cache_aside_service,
            internal_http_client=core_services['internal_http_client'],
        )

        memory_data_service = MemoryDataService(
            cache_aside_service=cache_aside_service
        )

        case_data_service = CaseDataService(
            settings=settings,
            cache=cache_aside_service,
            event_service=cast("EventServiceProtocol", core_services['g8ed_event_service']),
        )

        return {
            'investigation_data_service': investigation_data_service,
            'operator_data_service': operator_data_service,
            'memory_data_service': memory_data_service,
            'case_data_service': case_data_service,
        }
    
    @staticmethod
    def create_domain_services(data_services: dict[str, object]) -> dict[str, object]:
        """Create domain services that orchestrate business logic."""
        investigation_service = InvestigationService(
            investigation_data_service=cast("InvestigationDataServiceProtocol", data_services['investigation_data_service']),
            operator_data_service=cast("OperatorDataServiceProtocol", data_services['operator_data_service']),
            memory_data_service=cast("MemoryDataServiceProtocol", data_services['memory_data_service']),
        )

        memory_generation_service = MemoryGenerationService(
            memory_crud=cast("MemoryDataServiceProtocol", data_services['memory_data_service']),
        )

        return {
            'investigation_service': investigation_service,
            'memory_generation_service': memory_generation_service,
        }
    
    @staticmethod
    def create_operator_services(
        core_services: dict[str, object],
        data_services: dict[str, object],
    ) -> dict[str, object]:
        """Create operator-specific services."""
        heartbeat_service = OperatorHeartbeatService(
            operator_data_service=cast("OperatorDataServiceProtocol", data_services['operator_data_service']),
            event_service=cast("EventServiceProtocol", core_services['g8ed_event_service']),
        )

        execution_registry = ExecutionRegistryService()

        return {
            'heartbeat_service': heartbeat_service,
            'execution_registry': execution_registry,
        }
    
    @staticmethod
    def create_all_services(
        settings: G8eePlatformSettings,
        cache_aside_service: CacheAsideService,
        pubsub_client: object | None = None,
        blob_service: object | None = None,
    ) -> dict[str, object]:
        """Create all g8ee services in proper dependency order.

        When *pubsub_client* is supplied (production path), both the
        OperatorCommandService and HeartbeatService are wired to the
        shared PubSubClient and ready for ``start_services``.
        """
        core_services = ServiceFactory.create_core_services(settings, cache_aside_service)
        data_services = ServiceFactory.create_data_services(settings, cache_aside_service, core_services)
        domain_services = ServiceFactory.create_domain_services(data_services)
        operator_services = ServiceFactory.create_operator_services(core_services, data_services)

        attachment_service = AttachmentService(
            blob_service=blob_service,
            settings=settings,
        )
        response_analyzer = AIResponseAnalyzer()
        grounding_service = GroundingService()

        web_search_provider = None
        if settings.search.enabled:
            web_search_provider = WebSearchProvider(
                project_id=settings.search.project_id,
                engine_id=settings.search.engine_id,
                api_key=settings.search.api_key,
                location=settings.search.location,
            )

        from app.services.protocols import EventServiceProtocol

        approval_service = OperatorApprovalService(
            g8ed_event_service=cast(EventServiceProtocol, core_services['g8ed_event_service']),
            operator_data_service=cast("OperatorDataServiceProtocol", data_services['operator_data_service']),
            investigation_data_service=cast("InvestigationDataServiceProtocol", data_services['investigation_data_service']),
        )

        operator_command_service = OperatorCommandService.build(
            cache_aside_service=cache_aside_service,
            operator_data_service=cast("OperatorDataServiceProtocol", data_services['operator_data_service']),
            investigation_service=cast("InvestigationServiceProtocol", domain_services['investigation_service']),
            g8ed_event_service=cast(EventServiceProtocol, core_services['g8ed_event_service']),
            execution_registry=cast("ExecutionRegistryProtocol", operator_services['execution_registry']),
            settings=settings,
            ai_response_analyzer=response_analyzer,
            internal_http_client=core_services['internal_http_client'],
            approval_service=approval_service,
        )

        if pubsub_client is not None:
            operator_command_service.set_pubsub_client(pubsub_client)
            operator_services['heartbeat_service'].set_pubsub_client(pubsub_client)

        tool_executor = AIToolService(
            operator_command_service=operator_command_service,
            investigation_service=domain_services['investigation_service'],
            web_search_provider=web_search_provider,
        )

        mcp_gateway_service = MCPGatewayService(
            tool_service=tool_executor,
            investigation_service=cast("InvestigationServiceProtocol", domain_services['investigation_service']),
            operator_data_service=cast("OperatorDataServiceProtocol", data_services['operator_data_service']),
        )

        request_builder = AIRequestBuilder(
            tool_executor=tool_executor,
        )

        g8e_agent = g8eEngine(
            tool_executor=tool_executor,
            grounding_service=grounding_service,
        )

        chat_task_manager = ChatTaskManager()

        chat_pipeline = ChatPipelineService(
            investigation_data_service=cast("InvestigationDataServiceProtocol", data_services['investigation_data_service']),
            g8ed_event_service=cast("EventServiceProtocol", core_services['g8ed_event_service']),
            investigation_service=cast("InvestigationServiceProtocol", domain_services['investigation_service']),
            operator_command_service=operator_command_service,
            request_builder=request_builder,
            response_analyzer=response_analyzer,
            g8e_agent=g8e_agent,
            memory_service=cast("MemoryDataServiceProtocol", data_services['memory_data_service']),
            memory_generation_service=domain_services['memory_generation_service'],
            settings=settings,
        )

        all_services = {
            'cache_aside_service': cache_aside_service,
            'operator_cache_aside_service': cache_aside_service,
            'attachment_service': attachment_service,
            'response_analyzer': response_analyzer,
            'grounding_service': grounding_service,
            'web_search_provider': web_search_provider,
            'approval_service': approval_service,
            'operator_command_service': operator_command_service,
            'tool_service': tool_executor,
            'tool_executor': tool_executor,
            'mcp_gateway_service': mcp_gateway_service,
            'request_builder': request_builder,
            'g8e_agent': g8e_agent,
            'chat_task_manager': chat_task_manager,
            'chat_pipeline': chat_pipeline,
            **core_services,
            **data_services,
            **domain_services,
            **operator_services,
        }

        return all_services

    @staticmethod
    def bind_to_app_state(app: Any, services: dict[str, object]) -> None:
        """Assign every service in *services* to ``app.state``.

        Also creates the legacy alias ``memory_service`` that some dependency
        getters expect.
        """
        for name, svc in services.items():
            setattr(app.state, name, svc)

        if 'memory_data_service' in services:
            app.state.memory_service = services['memory_data_service']

    @staticmethod
    async def start_services(services: dict[str, object]) -> None:
        """Run lifecycle start hooks for services that require them."""
        operator_command_service = services.get('operator_command_service')
        if operator_command_service is not None:
            await cast("OperatorCommandService", operator_command_service).start_pubsub_listeners()

        http_service = services.get('http_service')
        if http_service is not None:
            await cast("HTTPServiceProtocol", http_service).start()

        heartbeat_service = services.get('heartbeat_service')
        if heartbeat_service is not None:
            await cast("OperatorHeartbeatServiceProtocol", heartbeat_service).start()

    @staticmethod
    async def stop_services(services: dict[str, object]) -> None:
        """Run lifecycle stop hooks (reverse order of start)."""
        import logging as _logging
        _logger = _logging.getLogger(__name__)

        heartbeat_service = services.get('heartbeat_service')
        if heartbeat_service is not None:
            try:
                await cast("OperatorHeartbeatServiceProtocol", heartbeat_service).stop()
            except Exception as exc:
                _logger.error("Error stopping heartbeat service: %s", exc)

        http_service = services.get('http_service')
        if http_service is not None:
            try:
                await cast("HTTPServiceProtocol", http_service).stop()
            except Exception as exc:
                _logger.error("Error stopping HTTP service: %s", exc)

        operator_command_service = services.get('operator_command_service')
        if operator_command_service is not None:
            try:
                await cast("OperatorCommandService", operator_command_service).stop_pubsub_listeners()
            except Exception as exc:
                _logger.error("Error stopping pubsub listeners: %s", exc)

