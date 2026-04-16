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
import logging

import pytest
import pytest_asyncio
from app.services.service_factory import ServiceFactory
from app.utils.timestamp import now
from tests.integration.cleanup import IntegrationCleanupTracker

logger = logging.getLogger(__name__)


async def auto_approve_pending(approval_service) -> None:
    """Simple helper to approve all pending approvals.

    Used in integration tests with fake operators to prevent infinite loops
    when commands are dispatched.
    """
    pending = approval_service.get_pending_approvals()
    for approval_id, pending_approval in pending.items():
        pending_approval.resolve(
            approved=True,
            reason="Auto-approved by integration test runner",
            responded_at=now(),
        )
        logger.info("[AUTO-APPROVE] Approved %s", approval_id)


async def approve_via_http(
    approval_id: str,
    approval_service,
    approved: bool = True,
    reason: str = "",
    operator_session_id: str = "",
    operator_id: str = "",
) -> dict:
    """Simple helper to send approval HTTP call to g8ee internal API.

    Used in integration tests to approve pending approvals via HTTP instead
    of directly resolving the pending approval object.

    Returns a simple dict with the approval result.
    """
    from app.models.internal_api import OperatorApprovalResponse

    response = OperatorApprovalResponse(
        approval_id=approval_id,
        approved=approved,
        reason=reason,
        operator_session_id=operator_session_id,
        operator_id=operator_id,
    )

    await approval_service.handle_approval_response(response)

    result = {
        "approved": approved,
        "feedback": False,
        "approval_id": approval_id,
    }

    logger.info(
        "[APPROVAL-HTTP] Simulated approval via HTTP: approval_id=%s, approved=%s",
        approval_id,
        approved,
    )

    return result


@pytest_asyncio.fixture(scope="function", loop_scope="session")
async def all_services(cache_aside_service, test_settings):
    """Fixture that returns all g8ee services properly configured.

    This is the recommended way to get services for integration tests.
    Use auto_approve_pending helper to approve pending approvals during tests.
    """
    services = ServiceFactory.create_all_services(test_settings, cache_aside_service)

    yield services

    # Await all background tasks before stopping services to prevent race conditions
    chat_task_manager = services.get('chat_task_manager')
    if chat_task_manager is not None:
        await chat_task_manager.wait_all(timeout=5.0)

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
async def cleanup(cache_aside_service, all_services):
    """Autouse-friendly cleanup tracker for integration tests.

    Track documents created during a test via ``cleanup.track_investigation(id)``
    etc. All tracked documents are deleted after the test, even on failure.
    
    Awaits all background tasks before document deletion to prevent race conditions.
    """
    tracker = IntegrationCleanupTracker(cache_aside_service)
    yield tracker
    
    # Await all background tasks before document deletion
    chat_task_manager = all_services.get('chat_task_manager')
    if chat_task_manager is not None:
        await chat_task_manager.wait_all(timeout=5.0)
    
    await tracker.cleanup()
