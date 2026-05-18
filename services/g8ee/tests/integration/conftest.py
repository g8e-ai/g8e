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

This conftest provides protocol fixtures specifically for integration tests,
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

import logging
from contextlib import contextmanager
from dataclasses import dataclass, field
from unittest.mock import MagicMock

import pytest
import pytest_asyncio

from app.models.operators import ApprovalType, PendingApproval
from app.models.settings import G8eeUserSettings
from app.services.service_factory import ServiceFactory
from app.utils.timestamp import now
from tests.integration.cleanup import IntegrationCleanupTracker

logger = logging.getLogger(__name__)


async def auto_approve_pending(approval_service) -> None:
    """Simple helper to approve all pending approvals.

    Used in integration tests with fake operators to prevent infinite loops
    when commands are dispatched. This is a *post-hoc* helper: it drains
    pending approvals only after the code path returns. For tests that can
    block mid-run on ``PendingApproval.wait()`` (e.g. the benchmark agent
    hitting ``AGENT_MAX_TOOL_TURNS`` and requesting an ``AGENT_CONTINUE``
    approval), use ``auto_approve_inline_callback`` instead.
    """
    pending = approval_service.get_pending_approvals()
    for approval_id, pending_approval in pending.items():
        pending_approval.resolve(
            approved=True,
            reason="Auto-approved by integration test runner",
            responded_at=now(),
        )
        logger.info("[AUTO-APPROVE] Approved %s", approval_id)


@dataclass
class ApprovalCallbackTracker:
    """Per-type counters for auto-approved approvals.

    Populated by ``auto_approve_inline_callback`` whenever the approval
    service registers a pending approval. Lets tests assert that specific
    approval flows (in particular ``AGENT_CONTINUE``) were exercised.
    """
    approved: bool = True
    reason: str = "Auto-approved by integration test runner"
    counts: dict[ApprovalType, int] = field(default_factory=dict)
    total: int = 0

    def record(self, approval_type: ApprovalType) -> None:
        self.counts[approval_type] = self.counts.get(approval_type, 0) + 1
        self.total += 1

    def count(self, approval_type: ApprovalType) -> int:
        return self.counts.get(approval_type, 0)


@contextmanager
def auto_approve_inline_callback(
    approval_service,
    *,
    approved: bool = True,
    reason: str = "Auto-approved by integration test runner",
):
    """Register an inline callback that resolves approvals as they are created.

    Unlike ``auto_approve_pending`` which runs post-hoc, this callback fires
    synchronously from ``OperatorApprovalService._register_pending`` for every
    approval type (``COMMAND``, ``FILE_EDIT``, ``INTENT``, ``AGENT_CONTINUE``).
    That is required for long-running eval flows where ``chat_pipeline.run_chat``
    itself blocks on ``PendingApproval.wait()`` mid-invocation -- most notably
    the benchmark suite, whose multi-step scenarios can hit
    ``AGENT_MAX_TOOL_TURNS`` and emit an ``AGENT_CONTINUE`` approval request
    that must be answered before the agent loop can finish.

    Yields an ``ApprovalCallbackTracker`` so tests can assert which approval
    types fired (e.g. ``tracker.count(ApprovalType.AGENT_CONTINUE) >= 1``).
    Restores the previous callback on exit.
    """
    tracker = ApprovalCallbackTracker(approved=approved, reason=reason)
    previous = getattr(approval_service, "_on_approval_requested", None)

    def _callback(approval_id: str, pending: PendingApproval) -> None:
        tracker.record(pending.approval_type)
        pending.resolve(
            approved=tracker.approved,
            reason=tracker.reason,
            responded_at=now(),
        )
        logger.info(
            "[AUTO-APPROVE] Inline-resolved %s (type=%s approved=%s)",
            approval_id,
            pending.approval_type.value,
            tracker.approved,
        )

    approval_service.set_on_approval_requested(_callback)
    try:
        yield tracker
    finally:
        approval_service.set_on_approval_requested(previous)


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

    Injects a real WebSearchProvider if search settings are configured,
    ensuring the g8e_web_search tool is registered for eval scenarios that expect it.
    """
    import socket
    import os
    from app.llm.factory import get_search_settings
    from app.services.ai.grounding.web_search_provider import WebSearchProvider
    from app.constants.paths import PATHS
    from app.clients.db_client import DBClient
    from app.services.infra.settings_service import SettingsService

    # Check if CA certificate exists
    ca_cert_path = PATHS["infra"]["ca_cert_path"]
    if not os.path.exists(ca_cert_path):
        pytest.skip(f"CA certificate not found at {ca_cert_path}")

    # Check if operator is online AND SSL is working
    try:
        settings_service = SettingsService()
        bootstrap_settings = settings_service.get_local_settings()
        db_client = DBClient(
            ca_cert_path=bootstrap_settings.ca_cert_path,
            client_cert_path=bootstrap_settings.client_cert_path,
            client_key_path=bootstrap_settings.client_key_path,
        )
        await db_client.connect()
        await db_client.close()
    except Exception as e:
        pytest.skip(f"Operator SSL connection failed: {e}")

    # Check if web search settings are configured
    web_search_provider = None
    search_settings = get_search_settings()
    if search_settings and search_settings.enabled:
        web_search_provider = WebSearchProvider(
            project_id=search_settings.project_id,
            engine_id=search_settings.engine_id,
            api_key=search_settings.api_key,
            location=search_settings.location,
        )
        logger.info(
            "[INTEGRATION-FIXTURE] Injecting real WebSearchProvider from search settings: project_id=%s engine_id=%s",
            search_settings.project_id, search_settings.engine_id
        )

    services = ServiceFactory.create_all_services(
        test_settings,
        cache_aside_service,
        db_service=MagicMock(),
        kv_service=MagicMock(),
        blob_service=MagicMock(),
        web_search_provider=web_search_provider,
    )

    yield services

    await ServiceFactory.stop_services(services)


@pytest.fixture(scope="function")
def investigation_service(all_services):
    """Returns the InvestigationService from all_services."""
    return all_services.investigation_service


@pytest.fixture(scope="function")
def tool_service(all_services):
    """Returns the AIToolService from all_services."""
    return all_services.tool_service


@pytest.fixture(scope="function")
def chat_pipeline(all_services):
    """Returns the ChatPipelineService from all_services."""
    return all_services.chat_pipeline


@pytest_asyncio.fixture(scope="function", loop_scope="session")
async def cleanup(cache_aside_service, all_services):
    """Autouse-friendly cleanup tracker for integration tests.

    Track documents created during a test via ``cleanup.track_investigation(id)``
    etc. All tracked documents are deleted after the test, even on failure.
    
    Awaits all background tasks before document deletion to prevent race conditions.
    """
    tracker = IntegrationCleanupTracker(cache_aside_service)
    yield tracker

    await tracker.cleanup()


@pytest_asyncio.fixture(scope="function", loop_scope="session")
async def user_settings(cache_aside_service, test_settings):
    """Returns user settings for integration tests.
    
    Uses TEST_LLM settings when available (set via ./g8e test flags),
    otherwise loads user settings from operator.
    """
    from app.llm.factory import get_llm_settings, get_search_settings
    from app.services.infra.settings_service import SettingsService

    # Use TEST_LLM settings if available
    llm = get_llm_settings()
    search = get_search_settings()
    if llm:
        return G8eeUserSettings(llm=llm, search=search or test_settings.search)

    # Otherwise load from operator
    settings_service = SettingsService(cache_aside_service=cache_aside_service)
    return await settings_service.get_user_settings("test-user-id")
