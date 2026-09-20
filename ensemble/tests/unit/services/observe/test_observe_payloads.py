# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

"""Unit tests for observe producer payload construction helpers.

Covers persona resolution, agent-state request building, investigation
run-state request building, status mapping, targetless skip, and the
task lifecycle unsupported invariant (task counts are truthful zeros
because no task document creation or lifecycle path is implemented in
the current ensemble).
"""

from __future__ import annotations

import pytest

from app.constants import InvestigationStatus, ReasoningAgent
from app.services.observe.payloads import (
    build_agent_state_request,
    build_investigation_run_state_request,
    map_investigation_status_to_run_lifecycle,
    resolve_chat_persona_id,
)

pytestmark = [pytest.mark.unit]


class TestResolveChatPersonaId:
    def test_sage_maps_to_sage_persona(self):
        assert resolve_chat_persona_id(ReasoningAgent.SAGE) == "sage"

    def test_dash_maps_to_dash_persona(self):
        assert resolve_chat_persona_id(ReasoningAgent.DASH) == "dash"

    def test_none_returns_none(self):
        assert resolve_chat_persona_id(None) is None


class TestBuildAgentStateRequest:
    def test_returns_typed_request_with_registry_metadata(self):
        req = build_agent_state_request(
            user_id="user-1",
            persona_id="sage",
            status="running",
            run_id="inv-1",
            web_session_id="web-1",
        )
        assert req is not None
        assert req.agent_id == "user-1:sage"
        assert req.display_name == "Sage"
        assert req.role == "reasoner"
        assert req.status == "running"
        assert req.run_id == "inv-1"

    def test_returns_none_for_targetless(self):
        req = build_agent_state_request(
            user_id="user-1",
            persona_id="sage",
            status="running",
            web_session_id=None,
            cli_session_id=None,
        )
        assert req is None

    def test_returns_none_for_unknown_persona(self):
        req = build_agent_state_request(
            user_id="user-1",
            persona_id="nonexistent",
            status="running",
            web_session_id="web-1",
        )
        assert req is None

    def test_cli_routing_works(self):
        req = build_agent_state_request(
            user_id="user-1",
            persona_id="sage",
            status="running",
            cli_session_id="cli-1",
        )
        assert req is not None
        assert req.cli_session_id == "cli-1"
        assert req.web_session_id is None

    def test_model_is_optional(self):
        req = build_agent_state_request(
            user_id="user-1",
            persona_id="sage",
            status="running",
            model=None,
            web_session_id="web-1",
        )
        assert req is not None
        assert req.model is None


class TestBuildInvestigationRunStateRequest:
    def test_returns_typed_request(self):
        req = build_investigation_run_state_request(
            run_id="inv-1",
            display_name="My Case",
            status="running",
            user_id="user-1",
            web_session_id="web-1",
        )
        assert req is not None
        assert req.run_id == "inv-1"
        assert req.run_kind == "investigation"
        assert req.display_name == "My Case"
        assert req.status == "running"

    def test_returns_none_for_targetless(self):
        req = build_investigation_run_state_request(
            run_id="inv-1",
            display_name="My Case",
            status="running",
            user_id="user-1",
        )
        assert req is None

    def test_task_counts_are_truthful_zeros_when_no_task_owner_implemented(self):
        """Task counts are truthful zeros because no task lifecycle owner is implemented.

        The protocol (``protocol/models/task.json``) designates the
        ensemble as the authority for task documents, but no ensemble
        code creates task documents, emits ``APP_TASK_*`` events, or
        defines a task model or service. The ``tasks`` collection is
        read by ``CaseDataService.get_case_tasks`` but nothing writes to
        it. Until a task lifecycle implementation lands, the producer
        payload carries truthful zero task counts. The Gateway computes
        ``tasks_in_queue`` from these projection fields
        (``total_tasks - completed_tasks``), not from SSE event
        subtraction; that behavior is covered by the Go gateway
        integration test
        ``TestObserveService_GetBootstrapSnapshot_PopulatedStateReturnsObservedFreshness``.
        """
        req = build_investigation_run_state_request(
            run_id="inv-1",
            display_name="My Case",
            status="running",
            user_id="user-1",
            web_session_id="web-1",
        )
        assert req is not None
        assert req.completed_tasks == 0
        assert req.total_tasks == 0
        assert req.active_task_id is None


class TestMapInvestigationStatusToRunLifecycle:
    @pytest.mark.parametrize(
        ("status", "expected"),
        [
            (InvestigationStatus.OPEN, "running"),
            (InvestigationStatus.ESCALATED, "running"),
            (InvestigationStatus.CLOSED, "completed"),
            (InvestigationStatus.RESOLVED, "completed"),
        ],
    )
    def test_status_mapping(self, status, expected):
        assert map_investigation_status_to_run_lifecycle(status) == expected
