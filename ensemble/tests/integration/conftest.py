# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

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

import asyncio
import logging
from contextlib import contextmanager
from dataclasses import dataclass, field
from datetime import UTC, datetime
from importlib import import_module
from pathlib import Path
from unittest.mock import AsyncMock, MagicMock

import pytest
import pytest_asyncio

from app.utils.time_ids.timestamp import now

# Lazy imports for protocol-dependent modules to prevent pytest collection crashes
# when protocol JSON files are missing or malformed.
# These are imported inside functions/fixtures where they're actually needed.

logger = logging.getLogger(__name__)


def make_write_through_governance_client(cache_aside_service):
    """Governance client mock that persists governed writes to the fake DB client.

    Data services route creates/updates/deletes through governance envelopes;
    integration tests that read documents back need this write-through behavior.
    """
    governance_client = MagicMock()

    async def _submit_write_through(message):
        payload = message.payload
        if payload is not None and hasattr(payload, "case_id"):
            data = payload.model_dump(mode="json")
            data["id"] = message.id
            data["created_at"] = datetime.now(UTC).isoformat()
            await cache_aside_service.db_client.create_document(
                collection="investigations",
                document_id=message.id,
                data=data,
            )
        elif payload is not None and hasattr(payload, "collection") and hasattr(payload, "updates"):
            await cache_aside_service.db_client.create_document(
                collection=payload.collection,
                document_id=payload.document_id,
                data=payload.updates,
            )
        return {"status": "accepted"}

    async def _update_write_through(collection, document_id, updates, **kwargs):
        return await cache_aside_service.db_client.update_document(
            collection=collection,
            document_id=document_id,
            data=updates,
            merge=kwargs.get("merge", True),
        )

    governance_client.submit_envelope = AsyncMock(side_effect=_submit_write_through)
    governance_client.update_governed_doc = AsyncMock(side_effect=_update_write_through)
    governance_client.delete_governed_doc = AsyncMock(return_value={"status": "accepted"})
    return governance_client


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
    counts: dict = field(default_factory=dict)
    total: int = 0

    def record(self, approval_type) -> None:
        self.counts[approval_type] = self.counts.get(approval_type, 0) + 1
        self.total += 1

    def count(self, approval_type) -> int:
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

    def _callback(approval_id: str, pending) -> None:
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
    request_context = import_module("app.models.http_context").RequestContext
    operator_approval_response = import_module("app.models.internal_api").OperatorApprovalResponse

    response = operator_approval_response(
        context=request_context(
            web_session_id=operator_session_id or None,
            user_id="integration-test-user",
            operator_session_id=operator_session_id or None,
            operator_id=operator_id or None,
        ),
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
    db_client = import_module("app.clients.db_client").DBClient
    get_paths = import_module("app.constants.paths").get_paths
    get_search_settings = import_module("app.llm.factory").get_search_settings
    tls_config = import_module("app.models.settings").TLSConfig
    web_search_provider = import_module(
        "app.services.ai.grounding.web_search_provider"
    ).WebSearchProvider
    settings_service = import_module("app.services.infra.settings_service").SettingsService
    service_factory = import_module("app.services.service_factory").ServiceFactory

    # Check if CA certificate exists
    paths = get_paths()
    ca_cert_path = paths["infra"]["ca_cert_path"]
    if not await asyncio.to_thread(Path(ca_cert_path).exists):
        pytest.skip(f"CA certificate not found at {ca_cert_path}")

    # Check if operator is online AND SSL is working
    try:
        settings_service = settings_service()
        bootstrap_settings = settings_service.get_local_settings()
        tls_config = tls_config(
            ca_cert_path=bootstrap_settings.ca_cert_path,
            client_cert_path=bootstrap_settings.client_cert_path,
            client_key_path=bootstrap_settings.client_key_path,
        )
        db_client = db_client(tls_config=tls_config)
        await db_client.connect()
        await db_client.close()
    except Exception as e:
        pytest.skip(f"Operator SSL connection failed: {e}")

    # Check if web search settings are configured
    web_search_provider = None
    search_settings = get_search_settings()
    if search_settings and search_settings.enabled:
        web_search_provider = web_search_provider(
            project_id=search_settings.project_id,
            engine_id=search_settings.engine_id,
            api_key=search_settings.api_key,
            location=search_settings.location,
        )
        logger.info(
            "[INTEGRATION-FIXTURE] Injecting real WebSearchProvider from search settings: project_id=%s engine_id=%s",
            search_settings.project_id,
            search_settings.engine_id,
        )

    services = service_factory.create_all_services(
        test_settings,
        cache_aside_service,
        db_service=MagicMock(),
        kv_service=MagicMock(),
        blob_service=MagicMock(),
        governance_client=make_write_through_governance_client(cache_aside_service),
        web_search_provider=web_search_provider,
    )

    yield services

    await service_factory.stop_services(services)


@pytest.fixture
def investigation_service(all_services):
    """Returns the InvestigationService from all_services."""
    return all_services.investigation_service


@pytest.fixture
def tool_service(all_services):
    """Returns the AIToolService from all_services."""
    return all_services.tool_service


@pytest.fixture
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
    integration_cleanup_tracker = import_module(
        "tests.integration.cleanup"
    ).IntegrationCleanupTracker

    tracker = integration_cleanup_tracker(cache_aside_service)
    yield tracker

    await tracker.cleanup()


@pytest_asyncio.fixture(scope="function", loop_scope="session")
async def user_settings(cache_aside_service, test_settings):
    """Returns user settings for integration tests.

    Uses TEST_LLM settings when available (set via ./g8e test flags),
    otherwise loads user settings from operator.
    """
    get_llm_settings = import_module("app.llm.factory").get_llm_settings
    get_search_settings = import_module("app.llm.factory").get_search_settings
    g8ee_user_settings = import_module("app.models.settings").G8eeUserSettings
    llm_settings = import_module("app.models.settings").LLMSettings
    settings_service = import_module("app.services.infra.settings_service").SettingsService

    # Use TEST_LLM settings if available
    llm = get_llm_settings()
    search = get_search_settings()
    if llm:
        return g8ee_user_settings(llm=llm, search=search or test_settings.search)

    # Otherwise load from operator
    settings_service = settings_service(cache_aside_service=cache_aside_service)
    try:
        return await settings_service.get_user_settings("test-user-id")
    except Exception as e:
        logger.warning("Failed to load user settings from operator: %s", e)

    # Fallback to mock settings if operator is offline or connection fails
    return g8ee_user_settings(llm=llm or llm_settings(), search=search or test_settings.search)
