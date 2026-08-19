# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

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
        (
            FakeExecutionService(resolved_operator=build_production_operator_document()),
            ExecutionServiceProtocol,
        ),
    ]

    for fake_instance, protocol in fakes_and_protocols:
        assert isinstance(fake_instance, protocol), (
            f"{type(fake_instance).__name__} is missing methods defined in {protocol.__name__}"
        )
