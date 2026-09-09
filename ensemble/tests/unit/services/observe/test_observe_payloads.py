# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

"""Unit tests for observe producer payload construction helpers.

Covers persona resolution, agent-state request building, investigation
run-state request building, status mapping, targetless skip, and the
task lifecycle unsupported invariant (task counts are truthful zeros).
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

    def test_task_counts_are_truthful_zeros(self):
        """Task lifecycle is unsupported: task counts are always zero."""
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

    def test_dashboard_does_not_derive_queue_depth_from_sse_events(self):
        """The run-state request never carries SSE-derived task counts.

        This test proves the dashboard cannot derive queue depth from
        SSE event subtraction because the producer payload always
        contains truthful zero task counts when no authoritative task
        owner exists.
        """
        req = build_investigation_run_state_request(
            run_id="inv-1",
            display_name="My Case",
            status="running",
            user_id="user-1",
            web_session_id="web-1",
        )
        assert req is not None
        assert req.total_tasks == 0
        assert req.completed_tasks == 0
        # No SSE event count or subtraction artifact in the payload.
        dumped = req.model_dump(mode="json")
        assert "event_count" not in dumped
        assert "sse_count" not in dumped


class TestMapInvestigationStatusToRunLifecycle:
    @pytest.mark.parametrize(
        "status,expected",
        [
            (InvestigationStatus.OPEN, "running"),
            (InvestigationStatus.ESCALATED, "running"),
            (InvestigationStatus.CLOSED, "completed"),
            (InvestigationStatus.RESOLVED, "completed"),
        ],
    )
    def test_status_mapping(self, status, expected):
        assert map_investigation_status_to_run_lifecycle(status) == expected
