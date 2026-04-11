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

from app.models.settings import VSEPlatformSettings
from app.services.operator.command_service import OperatorCommandService
from app.services.operator.intent_service import OperatorIntentService
from app.services.operator.operator_data_service import OperatorDataService
from app.services.investigation.investigation_data_service import InvestigationDataService
from app.services.investigation.memory_data_service import MemoryDataService

from app.models.cache import CacheOperationResult
from .fake_ai_response_analyzer import FakeAIResponseAnalyzer
from .fake_approval_service import FakeApprovalService
from tests.fakes.fake_vsodb_clients import FakeKVClient, FakeDBClient, FakePubSubClient
from .fake_execution_registry import FakeExecutionRegistry
from .fake_db_service import FakeDBService
from .fake_event_service import FakeEventService
from .fake_execution_service import FakeExecutionService
from .fake_investigation_service import FakeInvestigationService
from .fake_vsod_client import FakeVSODClient
from .fake_pubsub_service import FakePubSubService
from .fake_operator_cache import FakeOperatorCache


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
        component_name=ComponentName.VSE,
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
    vsod_client: FakeVSODClient | None = None,
    investigation_service: FakeInvestigationService | None = None,
    execution_registry: FakeExecutionRegistry | None = None,
    pubsub_client: FakePubSubClient | None = None,
    settings: VSEPlatformSettings | None = None,
    approval_service: FakeApprovalService | None = None,
    skip_pubsub_client: bool = False,
) -> OperatorCommandService:
    """Build an OperatorCommandService with typed fakes for all dependencies.

    All parameters are optional — provide only the fakes you need to configure
    or assert on. Omitted deps default to a fresh fake with sensible defaults.
    """
    cache_aside_service = create_mock_cache_aside_service()
    internal_http_client = vsod_client or FakeVSODClient()
    
    # Ensure all required fakes are present
    event_service = event_service or FakeEventService()
    execution_registry = execution_registry or FakeExecutionRegistry()
    ai_response_analyzer = ai_response_analyzer or FakeAIResponseAnalyzer()
    investigation_service = investigation_service or FakeInvestigationService()
    settings = settings or VSEPlatformSettings(port=443)
    
    operator_data_service = OperatorDataService(cache=cache_aside_service, internal_http_client=internal_http_client)
    
    approval_service = approval_service or FakeApprovalService()

    svc = OperatorCommandService.build(
        cache_aside_service=cache_aside_service,
        operator_data_service=operator_data_service,
        vsod_event_service=event_service,
        execution_registry=execution_registry,
        settings=settings,
        ai_response_analyzer=ai_response_analyzer,
        internal_http_client=internal_http_client,
        investigation_service=investigation_service,
        approval_service=approval_service,
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
    vsod_client: FakeVSODClient | None = None,
) -> OperatorIntentService:
    """Build an OperatorIntentService with typed fakes for all dependencies."""
    return OperatorIntentService(
        approval_service=approval_service or FakeApprovalService(),
        execution_service=execution_service or FakeExecutionService(),
        vsod_event_service=event_service or FakeEventService(),
        investigation_service=investigation_service or FakeInvestigationService(),
        vsod_client=vsod_client or FakeVSODClient(),
    )
