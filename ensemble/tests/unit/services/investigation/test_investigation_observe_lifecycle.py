# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

"""Unit tests for observe producer investigation run-state projections
emitted by ``InvestigationService`` after governed persistence.

Asserts persist-before-projection on creation, correct terminal mappings
on status update, non-blocking producer failure, and that the display
name is derived from the disclosure-safe case title only.
"""

from __future__ import annotations

from unittest.mock import AsyncMock

import pytest

from app.constants import (
    G8EE_COMPONENT,
    InvestigationStatus,
    Priority,
    Severity,
)
from app.models.http_context import RequestContext
from app.models.investigations import (
    InvestigationCreateRequest,
    InvestigationModel,
    InvestigationUpdateRequest,
)
from app.services.investigation.investigation_service import InvestigationService
from tests.fakes.fake_event_service import FakeEventService

pytestmark = [pytest.mark.unit, pytest.mark.asyncio]


def _make_investigation(
    investigation_id: str = "inv-obs-1",
    case_title: str = "Test Case",
    web_session_id: str | None = "web-obs-1",
    user_id: str = "user-obs-1",
    status: InvestigationStatus = InvestigationStatus.OPEN,
) -> InvestigationModel:
    return InvestigationModel(
        id=investigation_id,
        case_id="case-obs-1",
        case_title=case_title,
        user_id=user_id,
        status=status,
        priority=Priority.MEDIUM,
        severity=Severity.MEDIUM,
        sentinel_mode=True,
        web_session_id=web_session_id,
    )


def _make_create_request(
    case_id: str = "case-obs-1",
    case_title: str = "Test Case",
    user_id: str = "user-obs-1",
    web_session_id: str | None = "web-obs-1",
) -> InvestigationCreateRequest:
    return InvestigationCreateRequest(
        case_id=case_id,
        case_title=case_title,
        case_description="desc",
        user_id=user_id,
        web_session_id=web_session_id,
    )


def _make_update_request(
    status: InvestigationStatus | None = None,
    web_session_id: str = "web-obs-1",
    user_id: str = "user-obs-1",
) -> InvestigationUpdateRequest:
    return InvestigationUpdateRequest(
        context=RequestContext(
            web_session_id=web_session_id,
            user_id=user_id,
            case_id="case-obs-1",
            investigation_id="inv-obs-1",
            source_component=G8EE_COMPONENT,
        ),
        status=status,
    )


async def test_create_investigation_pushes_queued_run_projection():
    event_svc = FakeEventService()
    data_svc = AsyncMock()
    investigation = _make_investigation()
    data_svc.create_investigation.return_value = investigation

    service = InvestigationService(
        investigation_data_service=data_svc,
        operator_data_service=AsyncMock(),
        memory_data_service=AsyncMock(),
        event_service=event_svc,
    )

    result = await service.create_investigation(_make_create_request())

    assert result is not None
    assert len(event_svc.run_state_requests) == 1
    req = event_svc.run_state_requests[0]
    assert req.run_id == "inv-obs-1"
    assert req.status == "queued"
    assert req.run_kind == "investigation"
    assert req.display_name == "Test Case"


async def test_create_investigation_skips_projection_when_creation_fails():
    event_svc = FakeEventService()
    data_svc = AsyncMock()
    data_svc.create_investigation.side_effect = RuntimeError("governance rejected")

    service = InvestigationService(
        investigation_data_service=data_svc,
        operator_data_service=AsyncMock(),
        memory_data_service=AsyncMock(),
        event_service=event_svc,
    )

    with pytest.raises(RuntimeError):
        await service.create_investigation(_make_create_request())

    assert len(event_svc.run_state_requests) == 0


async def test_update_status_to_closed_pushes_completed_run_projection():
    event_svc = FakeEventService()
    investigation = _make_investigation(status=InvestigationStatus.OPEN)
    data_svc = AsyncMock()
    data_svc.get_investigation.return_value = investigation

    service = InvestigationService(
        investigation_data_service=data_svc,
        operator_data_service=AsyncMock(),
        memory_data_service=AsyncMock(),
        event_service=event_svc,
    )

    await service.update_investigation(
        "inv-obs-1",
        _make_update_request(status=InvestigationStatus.CLOSED),
    )

    assert len(event_svc.run_state_requests) == 1
    req = event_svc.run_state_requests[0]
    assert req.status == "completed"


async def test_update_status_to_resolved_pushes_completed_run_projection():
    event_svc = FakeEventService()
    investigation = _make_investigation(status=InvestigationStatus.OPEN)
    data_svc = AsyncMock()
    data_svc.get_investigation.return_value = investigation

    service = InvestigationService(
        investigation_data_service=data_svc,
        operator_data_service=AsyncMock(),
        memory_data_service=AsyncMock(),
        event_service=event_svc,
    )

    await service.update_investigation(
        "inv-obs-1",
        _make_update_request(status=InvestigationStatus.RESOLVED),
    )

    assert len(event_svc.run_state_requests) == 1
    assert event_svc.run_state_requests[0].status == "completed"


async def test_update_status_to_open_pushes_running_run_projection():
    event_svc = FakeEventService()
    investigation = _make_investigation(status=InvestigationStatus.CLOSED)
    data_svc = AsyncMock()
    data_svc.get_investigation.return_value = investigation

    service = InvestigationService(
        investigation_data_service=data_svc,
        operator_data_service=AsyncMock(),
        memory_data_service=AsyncMock(),
        event_service=event_svc,
    )

    await service.update_investigation(
        "inv-obs-1",
        _make_update_request(status=InvestigationStatus.OPEN),
    )

    assert len(event_svc.run_state_requests) == 1
    assert event_svc.run_state_requests[0].status == "running"


async def test_update_without_status_change_skips_projection():
    event_svc = FakeEventService()
    investigation = _make_investigation(status=InvestigationStatus.OPEN)
    data_svc = AsyncMock()
    data_svc.get_investigation.return_value = investigation

    service = InvestigationService(
        investigation_data_service=data_svc,
        operator_data_service=AsyncMock(),
        memory_data_service=AsyncMock(),
        event_service=event_svc,
    )

    # Update priority only, no status change.
    await service.update_investigation(
        "inv-obs-1",
        _make_update_request(status=None),
    )

    assert len(event_svc.run_state_requests) == 0


async def test_producer_failure_does_not_abort_update():
    event_svc = FakeEventService()

    async def _failing_push(request):
        raise RuntimeError("producer unavailable")

    event_svc.publish_run_state = _failing_push

    investigation = _make_investigation(status=InvestigationStatus.OPEN)
    data_svc = AsyncMock()
    data_svc.get_investigation.return_value = investigation

    service = InvestigationService(
        investigation_data_service=data_svc,
        operator_data_service=AsyncMock(),
        memory_data_service=AsyncMock(),
        event_service=event_svc,
    )

    # Should not raise despite producer failure.
    result = await service.update_investigation(
        "inv-obs-1",
        _make_update_request(status=InvestigationStatus.CLOSED),
    )

    assert result is not None


async def test_no_event_service_skips_projection():
    data_svc = AsyncMock()
    investigation = _make_investigation()
    data_svc.create_investigation.return_value = investigation

    service = InvestigationService(
        investigation_data_service=data_svc,
        operator_data_service=AsyncMock(),
        memory_data_service=AsyncMock(),
        event_service=None,
    )

    result = await service.create_investigation(_make_create_request())

    assert result is not None
    # No event service means no projection push; should not raise.


async def test_targetless_investigation_skips_projection():
    event_svc = FakeEventService()
    data_svc = AsyncMock()
    investigation = _make_investigation(web_session_id=None)
    data_svc.create_investigation.return_value = investigation

    service = InvestigationService(
        investigation_data_service=data_svc,
        operator_data_service=AsyncMock(),
        memory_data_service=AsyncMock(),
        event_service=event_svc,
    )

    await service.create_investigation(
        _make_create_request(web_session_id=None),
    )

    assert len(event_svc.run_state_requests) == 0


async def test_task_counts_are_truthful_zeros():
    event_svc = FakeEventService()
    data_svc = AsyncMock()
    investigation = _make_investigation()
    data_svc.create_investigation.return_value = investigation

    service = InvestigationService(
        investigation_data_service=data_svc,
        operator_data_service=AsyncMock(),
        memory_data_service=AsyncMock(),
        event_service=event_svc,
    )

    await service.create_investigation(_make_create_request())

    req = event_svc.run_state_requests[0]
    assert req.completed_tasks == 0
    assert req.total_tasks == 0
    assert req.active_task_id is None
