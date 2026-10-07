# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

from unittest.mock import AsyncMock, MagicMock

import pytest

from app.constants import OperatorStatus
from app.models.command_request_payloads import FileEditRequestPayload
from app.models.http_context import BoundOperator, G8eHttpContext
from app.services.ai.tools._base import convert_args_to_payload
from app.services.investigation.investigation_service import InvestigationService
from tests.fakes.factories import build_enriched_context

pytestmark = [pytest.mark.unit, pytest.mark.asyncio(loop_scope="session")]


def _service() -> InvestigationService:
    operator_data = MagicMock()
    operator_data.get_operator_by_session = AsyncMock(return_value=None)
    operator_data.get_operator = AsyncMock(return_value=None)
    return InvestigationService(
        investigation_data_service=MagicMock(),
        operator_data_service=operator_data,
        memory_data_service=MagicMock(),
    )


def _context(*operators: BoundOperator) -> G8eHttpContext:
    return G8eHttpContext(
        user_id="user-1",
        case_id="case-1",
        investigation_id="inv-1",
        web_session_id="web-1",
        bound_operators=list(operators),
    )


async def test_enrichment_mirrors_bound_operators_so_target_defaults_to_all():
    bound = BoundOperator(
        operator_id="op-1", operator_session_id="sess-1", status=OperatorStatus.BOUND
    )
    investigation = await _service().get_enriched_investigation_context(
        build_enriched_context(), "user-1", _context(bound)
    )

    assert [op.operator_id for op in investigation.bound_operators] == ["op-1"]

    payload = convert_args_to_payload(
        {"file_path": "/tmp/x"},
        FileEditRequestPayload,
        "exec-1",
        investigation,
        operation="read",
    )
    assert payload.target_operators == ["all"]


async def test_enrichment_excludes_operators_that_are_not_bound():
    bound = BoundOperator(
        operator_id="op-1", operator_session_id="sess-1", status=OperatorStatus.BOUND
    )
    stale = BoundOperator(
        operator_id="op-2", operator_session_id="sess-2", status=OperatorStatus.AVAILABLE
    )
    investigation = await _service().get_enriched_investigation_context(
        build_enriched_context(), "user-1", _context(bound, stale)
    )

    assert [op.operator_id for op in investigation.bound_operators] == ["op-1"]
