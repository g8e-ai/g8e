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

"""Factory helpers for building services under test with typed fakes.

Use build_command_service() in any test that needs a full OperatorCommandService
wired with typed fakes. Use the individual fake constructors directly when
testing a sub-service in isolation.
"""

from app.models.settings import G8eePlatformSettings
from app.services.operator.command_service import OperatorCommandService
from app.services.operator.intent_service import OperatorIntentService
from app.services.operator.operator_data_service import OperatorDataService
from app.services.protocols import ExecutionServiceProtocol
from app.utils.whitelist_validator import CommandWhitelistValidator
from app.utils.blacklist_validator import CommandBlacklistValidator

from app.models.cache import CacheOperationResult
from .fake_ai_response_analyzer import FakeAIResponseAnalyzer
from .fake_approval_service import FakeApprovalService
from tests.fakes.fake_g8es_clients import FakeKVClient, FakeDBClient, FakePubSubClient
from .fake_db_service import FakeDBService
from .fake_event_service import FakeEventService
from .fake_execution_service import FakeExecutionService
from .fake_investigation_service import FakeInvestigationService
from .fake_g8ed_client import FakeG8edClient


def create_pure_mock_cache_aside():
    """Returns a MagicMock spec'd to CacheAsideService with AsyncMock methods.
    
    Use this for pure unit tests of services that depend on CacheAsideService
    where you only want to assert on the service-level interface calls.
    """
    from unittest.mock import MagicMock, AsyncMock
    from app.services.cache.cache_aside import CacheAsideService
    
    mock = MagicMock(spec=CacheAsideService)
    # CRUD operations
    mock.create_document = AsyncMock(return_value=CacheOperationResult(success=True))
    mock.get_document = AsyncMock(return_value=None)
    mock.get_document_with_cache = AsyncMock(return_value=None)
    mock.update_document = AsyncMock(return_value=CacheOperationResult(success=True))
    mock.delete_document = AsyncMock(return_value=CacheOperationResult(success=True))
    mock.query_documents = AsyncMock(return_value=[])
    mock.append_to_array = AsyncMock(return_value=CacheOperationResult(success=True))
    mock.batch_create_documents = AsyncMock(return_value=CacheOperationResult(success=True))
    
    # KV operations
    mock.kv_get = AsyncMock()
    mock.kv_set = AsyncMock()
    mock.kv_delete = AsyncMock()
    mock.kv_exists = AsyncMock()
    mock.kv_lrange = AsyncMock()
    mock.kv_rpush = AsyncMock()
    mock.kv_ltrim = AsyncMock()
    
    # Query cache
    mock.get_query_result = AsyncMock()
    mock.set_query_result = AsyncMock()
    mock.invalidate_query_cache = AsyncMock()
    
    # Misc
    mock.get_stats = AsyncMock()
    mock.clear_all = AsyncMock()
    mock.invalidate_collection = AsyncMock()
    
    return mock


def create_mock_cache_aside_service(kv_cache_client=None, db_client=None):
    """Wired CacheAsideService with fake KV/DB for tests."""
    from app.services.cache.cache_aside import CacheAsideService
    from app.db.db_service import DBService
    from app.db.kv_service import KVService
    from app.constants import ComponentName

    # Use provided clients or create new fakes
    kv_raw = kv_cache_client or FakeKVClient()
    db_raw = db_client or FakeDBClient()

    kv_svc = KVService(kv_raw)
    db_svc = DBService(db_raw)

    svc = CacheAsideService(
        kv=kv_svc,
        db=db_svc,
        component_name=ComponentName.G8EE,
    )
    # Attach raw clients for convenience in tests
    svc.kv_cache_client = kv_raw
    svc.db_client = db_raw
    return svc


def create_mock_tool_executor():
    """Build a mock ToolExecutor for tests."""
    from unittest.mock import MagicMock
    executor = MagicMock()
    executor.get_tools = MagicMock(return_value=[])
    executor.g8e_web_search_available = False
    return executor


def build_command_service(
    *,
    event_service: FakeEventService | None = None,
    db_service: FakeDBService | None = None,
    ai_response_analyzer: FakeAIResponseAnalyzer | None = None,
    g8ed_client: FakeG8edClient | None = None,
    investigation_service: FakeInvestigationService | None = None,
    pubsub_client: FakePubSubClient | None = None,
    settings: G8eePlatformSettings | None = None,
    approval_service: FakeApprovalService | None = None,
    skip_pubsub_client: bool = False,
    whitelist_validator: CommandWhitelistValidator | None = None,
    blacklist_validator: CommandBlacklistValidator | None = None,
    execution_service: ExecutionServiceProtocol | None = None,
) -> OperatorCommandService:
    """Build an OperatorCommandService with typed fakes for all dependencies.

    All parameters are optional — provide only the fakes you need to configure
    or assert on. Omitted deps default to a fresh fake with sensible defaults.
    """
    cache_aside_service = create_mock_cache_aside_service()
    internal_http_client = g8ed_client or FakeG8edClient()

    # Ensure all required fakes are present
    event_service = event_service or FakeEventService()
    ai_response_analyzer = ai_response_analyzer or FakeAIResponseAnalyzer()
    investigation_service = investigation_service or FakeInvestigationService()
    settings = settings or G8eePlatformSettings(port=443)

    operator_data_service = OperatorDataService(cache=cache_aside_service, internal_http_client=internal_http_client)

    approval_service = approval_service or FakeApprovalService()

    # Build sub-services manually (mirroring OperatorCommandService.build)
    from app.services.operator.pubsub_service import OperatorPubSubService
    from app.services.operator.lfaa_service import OperatorLFAAService
    from app.services.operator.execution_service import OperatorExecutionService
    from app.services.operator.filesystem_service import OperatorFilesystemService
    from app.services.operator.port_service import OperatorPortService
    from app.services.operator.file_service import OperatorFileService
    from app.services.operator.intent_service import OperatorIntentService

    pubsub_service = OperatorPubSubService()

    lfaa_service = OperatorLFAAService(
        pubsub_service=pubsub_service,
    )

    if execution_service is None:
        execution_service = OperatorExecutionService(
            pubsub_service=pubsub_service,
            approval_service=approval_service,
            g8ed_event_service=event_service,
            settings=settings,
            ai_response_analyzer=ai_response_analyzer,
            operator_data_service=operator_data_service,
            investigation_service=investigation_service,
        )

    filesystem_service = OperatorFilesystemService(
        pubsub_service=pubsub_service,
        execution_service=execution_service,
        investigation_service=investigation_service,
    )

    port_service = OperatorPortService(
        pubsub_service=pubsub_service,
        execution_service=execution_service,
    )

    file_service = OperatorFileService(
        pubsub_service=pubsub_service,
        approval_service=approval_service,
        g8ed_event_service=event_service,
        execution_service=execution_service,
        ai_response_analyzer=ai_response_analyzer,
        investigation_service=investigation_service,
    )

    intent_service = OperatorIntentService(
        approval_service=approval_service,
        execution_service=execution_service,
        g8ed_event_service=event_service,
        investigation_service=investigation_service,
        g8ed_client=internal_http_client,
    )

    svc = OperatorCommandService(
        pubsub_service=pubsub_service,
        approval_service=approval_service,
        execution_service=execution_service,
        filesystem_service=filesystem_service,
        port_service=port_service,
        file_service=file_service,
        intent_service=intent_service,
        lfaa_service=lfaa_service,
        cache_aside_service=cache_aside_service,
        operator_data_service=operator_data_service,
        investigation_service=investigation_service,
        settings=settings,
        whitelist_validator=whitelist_validator,
        blacklist_validator=blacklist_validator,
    )
    if not skip_pubsub_client:
        svc.set_pubsub_client(pubsub_client or FakePubSubClient())

    svc._store = {}
    return svc


def build_intent_service(
    *,
    approval_service: FakeApprovalService | None = None,
    execution_service: FakeExecutionService | None = None,
    event_service: FakeEventService | None = None,
    investigation_service: FakeInvestigationService | None = None,
    g8ed_client: FakeG8edClient | None = None,
) -> OperatorIntentService:
    """Build an OperatorIntentService with typed fakes for all dependencies."""
    return OperatorIntentService(
        approval_service=approval_service or FakeApprovalService(),
        execution_service=execution_service or FakeExecutionService(),
        g8ed_event_service=event_service or FakeEventService(),
        investigation_service=investigation_service or FakeInvestigationService(),
        g8ed_client=g8ed_client or FakeG8edClient(),
    )
