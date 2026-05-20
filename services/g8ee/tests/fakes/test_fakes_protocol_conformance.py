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

from app.services.protocols import (
    AIResponseAnalyzerProtocol,
    ApprovalServiceProtocol,
    DocumentServiceProtocol,
    EventServiceProtocol,
    ExecutionServiceProtocol,
    G8eClientProtocol,
    HTTPServiceProtocol,
    InvestigationServiceProtocol,
    LFAAServiceProtocol,
    MemoryDataServiceProtocol,
    OperatorDataServiceProtocol,
    PubSubServiceProtocol,
)
from tests.fakes.factories import build_production_operator_document
from tests.fakes.fake_ai_response_analyzer import FakeAIResponseAnalyzer
from tests.fakes.fake_approval_service import FakeApprovalService
from tests.fakes.fake_db_service import FakeDBService
from tests.fakes.fake_event_service import FakeEventService
from tests.fakes.fake_execution_service import FakeExecutionService
from tests.fakes.fake_operator_clients import FakeG8eClient
from tests.fakes.fake_http_service import FakeHTTPService
from tests.fakes.fake_investigation_service import FakeInvestigationService
from tests.fakes.fake_lfaa_service import FakeLFAAService
from tests.fakes.fake_memory_data_service import FakeMemoryDataService
from tests.fakes.fake_operator_cache import FakeOperatorCache
from tests.fakes.fake_pubsub_service import FakePubSubService


def test_fakes_implement_protocols():
    """Verify that all Fakes structurally implement their designated protocols.

    Using isinstance with @runtime_checkable enforces that all protocol methods
    are implemented by the fake.
    """
    fakes_and_protocols = [
        (FakeEventService(), EventServiceProtocol),
        (FakeOperatorCache(), OperatorDataServiceProtocol),
        (FakeHTTPService(), HTTPServiceProtocol),
        (FakeLFAAService(), LFAAServiceProtocol),
        (FakePubSubService(), PubSubServiceProtocol),
        (FakeG8eClient(), G8eClientProtocol),
        (FakeApprovalService(), ApprovalServiceProtocol),
        (FakeInvestigationService(), InvestigationServiceProtocol),
        (FakeMemoryDataService(), MemoryDataServiceProtocol),
        (FakeDBService(), DocumentServiceProtocol),
        (FakeAIResponseAnalyzer(), AIResponseAnalyzerProtocol),
        (FakeExecutionService(resolved_operator=build_production_operator_document()), ExecutionServiceProtocol)
    ]

    for fake_instance, protocol in fakes_and_protocols:
        assert isinstance(fake_instance, protocol), f"{type(fake_instance).__name__} is missing methods defined in {protocol.__name__}"
