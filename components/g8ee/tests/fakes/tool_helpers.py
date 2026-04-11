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

"""Consolidated tool executor helpers for g8ee tests."""

from unittest.mock import MagicMock
from .fake_web_search_provider import FakeWebSearchProvider
from app.services.ai.tool_service import AIToolService
from app.services.protocols import MemoryDataServiceProtocol



def create_tool_service_fake(investigation_service=None, web_search_provider=None, with_run_commands_result=None, auto_approve=True, event_service=None):
    """Return an AIToolService with all external dependencies wired.
    
    Uses build_command_service to ensure we have a real OperatorCommandService
    with awaitable methods on its sub-services.
    """
    from .builder import build_command_service
    from app.services.investigation.investigation_service import InvestigationService
    from app.models.operators import PendingApproval
    from app.utils.timestamp import now
    
    # Build a real wired OperatorCommandService using our fakes
    operator_command_service = build_command_service(
        investigation_service=investigation_service,
        event_service=event_service
    )

    if auto_approve:
        def _auto_approve_callback(approval_id: str, pending: PendingApproval):
            import asyncio
            # Schedule the resolution to happen almost immediately but in the next loop tick
            # to simulate an external response while we are waiting.
            loop = asyncio.get_event_loop()
            loop.call_later(0.01, lambda: pending.resolve(
                approved=True,
                reason="Auto-approved by test runner",
                responded_at=now()
            ))
        
        operator_command_service._approval_service.set_on_approval_requested(_auto_approve_callback)

    from tests.fakes.fake_investigation_service import FakeInvestigationService
    memory_data_service = MagicMock(spec=MemoryDataServiceProtocol)

    # We use a real InvestigationService wired to the same fakes if possible
    # This acts as our domain-layer service
    investigation_service_domain = InvestigationService(
        investigation_data_service=operator_command_service.investigation_service,
        operator_data_service=operator_command_service.operator_data_service,
        memory_data_service=memory_data_service,
    )
    
    # Default investigation_service to a fake if not provided
    if investigation_service is None:
        investigation_service = operator_command_service.investigation_service

    return AIToolService(
        operator_command_service=operator_command_service,
        investigation_service=investigation_service_domain,
        web_search_provider=web_search_provider,
    )


def create_tool_service_with_search_fake(investigation_service=None):
    """Return an AIToolService with a fake WebSearchProvider configured."""
    from app.models.tool_results import SearchWebResult
    provider = FakeWebSearchProvider(search_result=SearchWebResult(
        success=True, query="test query", results=[]
    ))
    return create_tool_service_fake(investigation_service=investigation_service, web_search_provider=provider)
