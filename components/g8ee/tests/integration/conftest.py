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

"""
Integration test fixtures and utilities.

This conftest provides shared fixtures specifically for integration tests,
extracting common patterns and ensuring consistent service construction
across all integration test files.

Key fixtures:
- all_services: Returns all g8ee services properly configured
- investigation_service: Returns the InvestigationService from all_services
- tool_service: Returns the AIToolService from all_services
- chat_pipeline: Returns the ChatPipelineService from all_services
- test_settings: Shared settings fixture from main conftest

All integration tests should use these fixtures to ensure consistency
and avoid code duplication.
"""

import asyncio

import pytest
import pytest_asyncio
from app.models.operators import PendingApproval
from app.services.service_factory import ServiceFactory
from app.utils.timestamp import now
from tests.integration.cleanup import IntegrationCleanupTracker


@pytest_asyncio.fixture(scope="function", loop_scope="session")
async def all_services(cache_aside_service, test_settings):
    """Fixture that returns all g8ee services properly configured.

    This is the recommended way to get services for integration tests.
    """
    services = ServiceFactory.create_all_services(test_settings, cache_aside_service)

    def _auto_approve_callback(approval_id: str, pending: PendingApproval):
        loop = asyncio.get_event_loop()
        loop.call_later(0.01, lambda: pending.resolve(
            approved=True,
            reason="Auto-approved by integration test runner",
            responded_at=now(),
        ))

    approval_service = services['approval_service']
    approval_service.set_on_approval_requested(_auto_approve_callback)

    yield services
    await ServiceFactory.stop_services(services)


@pytest.fixture(scope="function")
def investigation_service(all_services):
    """Returns the InvestigationService from all_services."""
    return all_services['investigation_service']


@pytest.fixture(scope="function")
def tool_service(all_services):
    """Returns the AIToolService from all_services."""
    return all_services['tool_service']


@pytest.fixture(scope="function")
def chat_pipeline(all_services):
    """Returns the ChatPipelineService from all_services."""
    return all_services['chat_pipeline']


@pytest_asyncio.fixture(scope="function", loop_scope="session")
async def cleanup(cache_aside_service):
    """Autouse-friendly cleanup tracker for integration tests.

    Track documents created during a test via ``cleanup.track_investigation(id)``
    etc. All tracked documents are deleted after the test, even on failure.
    """
    tracker = IntegrationCleanupTracker(cache_aside_service)
    yield tracker
    await tracker.cleanup()
