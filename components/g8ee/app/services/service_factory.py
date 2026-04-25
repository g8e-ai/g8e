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

from typing import TYPE_CHECKING, cast, Any, TypedDict, Optional

from app.services.ai.agent import g8eEngine
from app.services.ai.chat_pipeline import ChatPipelineService
from app.services.ai.chat_task_manager import BackgroundTaskManager
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
from app.services.ai.reputation_service import ReputationService
from app.services.data.agent_activity_data_service import AgentActivityDataService
from app.services.data.reputation_data_service import ReputationDataService
from app.services.data.stake_resolution_data_service import StakeResolutionDataService
from app.services.infra.http_service import HTTPService
from app.services.infra.internal_http_client import InternalHttpClient
from app.services.infra.g8ed_event_service import EventService
from app.services.protocols import (
    HTTPServiceProtocol,
    InvestigationServiceProtocol,
    InvestigationDataServiceProtocol,
    OperatorDataServiceProtocol,
    MemoryDataServiceProtocol,
    OperatorHeartbeatServiceProtocol,
    EventServiceProtocol,
    AIResponseAnalyzerProtocol,
    ToolExecutorProtocol,
    ApprovalServiceProtocol,
)
from app.services.operator.command_service import OperatorCommandService
from app.services.operator.operator_data_service import OperatorDataService
from app.services.data.case_data_service import CaseDataService
from app.services.operator.heartbeat_service import OperatorHeartbeatService
from app.models.settings import G8eePlatformSettings

if TYPE_CHECKING:
    from app.clients.pubsub_client import PubSubClient
    from app.services.investigation.investigation_service import InvestigationService
    from app.services.investigation.investigation_data_service import InvestigationDataService
    from app.services.operator.operator_data_service import OperatorDataService
    from app.services.investigation.memory_data_service import MemoryDataService
    from app.services.operator.heartbeat_service import OperatorHeartbeatService
    from app.services.ai.response_analyzer import AIResponseAnalyzer
    from app.services.operator.approval_service import OperatorApprovalService


class CoreServices(TypedDict):
    http_service: HTTPService | HTTPServiceProtocol
    internal_http_client: InternalHttpClient
    g8ed_event_service: EventService | EventServiceProtocol


class DataServices(TypedDict):
    investigation_data_service: InvestigationDataService | InvestigationDataServiceProtocol
    operator_data_service: OperatorDataService | OperatorDataServiceProtocol
    memory_data_service: MemoryDataService | MemoryDataServiceProtocol
    case_data_service: CaseDataService
    agent_activity_data_service: AgentActivityDataService
    reputation_data_service: ReputationDataService
    stake_resolution_data_service: StakeResolutionDataService


class DomainServices(TypedDict):
    investigation_service: InvestigationService | InvestigationServiceProtocol
    memory_generation_service: MemoryGenerationService
    reputation_service: ReputationService


class OperatorServices(TypedDict):
    heartbeat_service: OperatorHeartbeatService | OperatorHeartbeatServiceProtocol


class AllServices(CoreServices, DataServices, DomainServices, OperatorServices):
    cache_aside_service: CacheAsideService
    operator_cache_aside_service: CacheAsideService
    attachment_service: AttachmentService
    response_analyzer: AIResponseAnalyzer | AIResponseAnalyzerProtocol
    grounding_service: GroundingService
    web_search_provider: Optional[WebSearchProvider]
    approval_service: OperatorApprovalService | ApprovalServiceProtocol
    operator_command_service: OperatorCommandService
    tool_service: ToolExecutorProtocol
    tool_executor: ToolExecutorProtocol
    request_builder: AIRequestBuilder
    g8e_agent: g8eEngine
    chat_task_manager: BackgroundTaskManager
    chat_pipeline: ChatPipelineService
    memory_service: MemoryDataService | MemoryDataServiceProtocol


class ServiceFactory:
    """Factory for creating g8ee services with consistent dependency injection."""
    
    @staticmethod
    def create_core_services(
        settings: G8eePlatformSettings, cache_aside_service: CacheAsideService
    ) -> CoreServices:
        """Create core services that other services depend on."""
        from app.services.infra.internal_http_client import InternalHttpClient
        from app.services.infra.http_service import HTTPService
        from app.services.infra.g8ed_event_service import EventService

        # Create HTTP service first to manage all HTTP clients
        http_service: HTTPService = HTTPService()

        internal_http_client = InternalHttpClient(settings)

        # Register g8ed HTTP client with the HTTP service
        http_service.set_http_client(internal_http_client.client, "g8ed")

        g8ed_event_service: EventService = EventService(
            internal_http_client=internal_http_client
        )

        return CoreServices(
            http_service=http_service,
            internal_http_client=internal_http_client,
            g8ed_event_service=g8ed_event_service,
        )
    
    @staticmethod
    def create_data_services(
        settings: G8eePlatformSettings,
        cache_aside_service: CacheAsideService,
        core_services: CoreServices,
    ) -> DataServices:
        """Create data services for CRUD operations."""
        investigation_data_service = InvestigationDataService(
            cache=cache_aside_service
        )

        operator_data_service = OperatorDataService(
            cache=cache_aside_service,
            internal_http_client=core_services['internal_http_client'],  # type: ignore[arg-type]
        )

        # Inject operator_data_service into internal_http_client to resolve circular dependency
        core_services['internal_http_client'].set_operator_data_service(operator_data_service)

        memory_data_service = MemoryDataService(
            cache_aside_service=cache_aside_service
        )

        case_data_service = CaseDataService(
            settings=settings,
            cache=cache_aside_service,
            event_service=cast(EventService, core_services['g8ed_event_service']),
        )

        agent_activity_data_service = AgentActivityDataService(
            cache=cache_aside_service
        )

        reputation_data_service = ReputationDataService(
            cache=cache_aside_service
        )

        stake_resolution_data_service = StakeResolutionDataService(
            cache=cache_aside_service
        )

        return DataServices(
            investigation_data_service=investigation_data_service,
            operator_data_service=operator_data_service,
            memory_data_service=memory_data_service,
            case_data_service=case_data_service,
            agent_activity_data_service=agent_activity_data_service,
            reputation_data_service=reputation_data_service,
            stake_resolution_data_service=stake_resolution_data_service,
        )
    
    @staticmethod
    def create_domain_services(data_services: DataServices) -> DomainServices:
        """Create domain services that orchestrate business logic."""
        investigation_service = InvestigationService(
            investigation_data_service=data_services['investigation_data_service'],
            operator_data_service=data_services['operator_data_service'],
            memory_data_service=data_services['memory_data_service'],
        )

        memory_generation_service = MemoryGenerationService(
            memory_crud=data_services['memory_data_service'],
        )

        reputation_service = ReputationService(
            reputation_data_service=data_services['reputation_data_service'],
            stake_resolution_data_service=data_services['stake_resolution_data_service'],
        )

        return DomainServices(
            investigation_service=investigation_service,
            memory_generation_service=memory_generation_service,
            reputation_service=reputation_service,
        )
    
    @staticmethod
    def create_operator_services(
        core_services: CoreServices,
        data_services: DataServices,
    ) -> OperatorServices:
        """Create operator-specific services."""
        heartbeat_service = OperatorHeartbeatService(
            operator_data_service=data_services['operator_data_service'],
            event_service=core_services['g8ed_event_service'],
        )

        return OperatorServices(
            heartbeat_service=heartbeat_service,
        )
    
    @staticmethod
    def create_all_services(
        settings: G8eePlatformSettings,
        cache_aside_service: CacheAsideService,
        pubsub_client: object | None = None,
        blob_service: object | None = None,
        web_search_provider: WebSearchProvider | None = None,
    ) -> AllServices:
        """Create all g8ee services in proper dependency order.

        When *pubsub_client* is supplied (production path), both the
        OperatorCommandService and HeartbeatService are wired to the
        shared PubSubClient and ready for ``start_services``.

        *web_search_provider* allows tests to inject a provider without
        requiring platform settings to have search configured.
        """
        core_services = ServiceFactory.create_core_services(settings, cache_aside_service)
        data_services = ServiceFactory.create_data_services(settings, cache_aside_service, core_services)
        domain_services = ServiceFactory.create_domain_services(data_services)
        operator_services = ServiceFactory.create_operator_services(core_services, data_services)

        attachment_service = AttachmentService(
            blob_service=blob_service,  # type: ignore[arg-type]
            settings=settings,
        )
        response_analyzer = AIResponseAnalyzer()
        grounding_service = GroundingService()

        # Use injected provider if provided, otherwise create from platform settings
        if web_search_provider is None and settings.search.enabled:
            web_search_provider = WebSearchProvider(
                project_id=settings.search.project_id,
                engine_id=settings.search.engine_id,
                api_key=settings.search.api_key,
                location=settings.search.location,
            )

        approval_service = OperatorApprovalService(
            g8ed_event_service=core_services['g8ed_event_service'],
            operator_data_service=data_services['operator_data_service'],
            investigation_data_service=data_services['investigation_data_service'],
        )

        operator_command_service = OperatorCommandService.build(
            cache_aside_service=cache_aside_service,
            operator_data_service=data_services['operator_data_service'],  # type: ignore[arg-type]
            investigation_service=domain_services['investigation_service'],  # type: ignore[arg-type]
            g8ed_event_service=core_services['g8ed_event_service'],  # type: ignore[arg-type]
            settings=settings,
            ai_response_analyzer=response_analyzer,  # type: ignore[arg-type]
            internal_http_client=core_services['internal_http_client'],
            approval_service=approval_service,  # type: ignore[arg-type]
        )

        if pubsub_client is not None:
            operator_command_service.set_pubsub_client(cast("PubSubClient", pubsub_client))
            operator_services['heartbeat_service'].set_pubsub_client(cast("PubSubClient", pubsub_client))

        tool_executor = AIToolService(
            operator_command_service=operator_command_service,
            investigation_service=cast(InvestigationService, domain_services['investigation_service']),
            web_search_provider=web_search_provider,
            platform_settings=settings,
            reputation_data_service=data_services['reputation_data_service'],
        )

        request_builder = AIRequestBuilder(
            tool_executor=tool_executor,
        )

        g8e_agent = g8eEngine(
            tool_executor=tool_executor,
            grounding_service=grounding_service,
            approval_service=approval_service,
        )

        chat_task_manager = BackgroundTaskManager()

        chat_pipeline = ChatPipelineService(
            g8ed_event_service=core_services['g8ed_event_service'],  # type: ignore[arg-type]
            investigation_service=domain_services['investigation_service'],  # type: ignore[arg-type]
            request_builder=request_builder,
            g8e_agent=g8e_agent,
            memory_service=data_services['memory_data_service'],  # type: ignore[arg-type]
            memory_generation_service=domain_services['memory_generation_service'],
            agent_activity_data_service=data_services['agent_activity_data_service'],
        )

        all_services = AllServices(
            cache_aside_service=cache_aside_service,
            operator_cache_aside_service=cache_aside_service,
            attachment_service=attachment_service,
            response_analyzer=response_analyzer,
            grounding_service=grounding_service,
            web_search_provider=web_search_provider,
            approval_service=approval_service,
            operator_command_service=operator_command_service,
            tool_service=tool_executor,
            tool_executor=tool_executor,
            request_builder=request_builder,
            g8e_agent=g8e_agent,
            chat_task_manager=chat_task_manager,
            chat_pipeline=chat_pipeline,
            memory_service=data_services['memory_data_service'],
            **core_services,
            **data_services,
            **domain_services,
            **operator_services,
        )

        return all_services

    @staticmethod
    def bind_to_app_state(app: Any, services: AllServices) -> None:
        """Assign every service in *services* to ``app.state``.

        Also creates the legacy alias ``memory_service`` that some dependency
        getters expect.
        """
        for name, svc in services.items():
            setattr(app.state, name, svc)

        if 'memory_data_service' in services:
            app.state.memory_service = services['memory_data_service']

    @staticmethod
    async def start_services(services: AllServices) -> None:
        """Run lifecycle start hooks for services that require them."""
        await services['operator_command_service'].start_pubsub_listeners()
        await services['http_service'].start()
        await services['heartbeat_service'].start()

    @staticmethod
    async def stop_services(services: AllServices) -> None:
        """Run lifecycle stop hooks (reverse order of start)."""
        import logging as _logging
        _logger = _logging.getLogger(__name__)

        # First, await all background tasks to ensure they complete before cleanup
        try:
            _logger.info("Awaiting background task completion before service shutdown")
            await services['chat_task_manager'].wait_all(timeout=5.0)
        except TimeoutError:
            _logger.warning("Background tasks did not complete within 5s timeout, proceeding with shutdown")
        except Exception as exc:
            _logger.error("Error awaiting background tasks: %s", exc)

        try:
            await services['heartbeat_service'].stop()
        except Exception as exc:
            _logger.error("Error stopping heartbeat service: %s", exc)

        try:
            await services['http_service'].stop()
        except Exception as exc:
            _logger.error("Error stopping HTTP service: %s", exc)

        try:
            await services['operator_command_service'].stop_pubsub_listeners()
        except Exception as exc:
            _logger.error("Error stopping pubsub listeners: %s", exc)

