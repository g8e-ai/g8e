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

"""Integration test: JSON-listed verbs bypass the human approval gate.

Unit tests cover ``CommandAutoApprovedValidator`` in isolation, and other
unit tests cover the CSV ``auto_approved_commands`` override path. This
suite proves the JSON-only path end-to-end through ``OperatorCommandService.
execute_command``: with an empty CSV override, a verb listed in the JSON
file alone must

  - bypass the human approval prompt (``request_command_approval`` not
    invoked),
  - suppress the ``OPERATOR_COMMAND_APPROVAL_PREPARING`` UI event,
  - still drive a real downstream execution dispatch.

A symmetric negative test (empty JSON, CSV-listed verb) regression-protects
the JSON+CSV union logic in ``CommandAutoApprovedValidator.is_auto_approved``.
"""

from __future__ import annotations

import json
import tempfile
from pathlib import Path

import pytest

from app.constants.events import EventType
from app.constants.status import ComponentName
from app.models.agent import ExecutorCommandArgs
from app.models.http_context import G8eHttpContext
from app.models.investigations import EnrichedInvestigationContext
from app.models.operators import OperatorDocument, OperatorType
from app.models.settings import (
    CommandValidationSettings,
    G8eeUserSettings,
    LLMSettings,
)
from app.utils.auto_approved_validator import CommandAutoApprovedValidator
from tests.fakes.builder import build_command_service
from tests.fakes.fake_ai_response_analyzer import FakeAIResponseAnalyzer
from tests.fakes.fake_approval_service import FakeApprovalService
from tests.fakes.fake_event_service import FakeEventService
from tests.fakes.fake_execution_service import FakeExecutionService

pytestmark = [pytest.mark.integration, pytest.mark.asyncio(loop_scope="session")]


# ---------------------------------------------------------------------------
# Helpers
# ---------------------------------------------------------------------------

def _make_operator() -> OperatorDocument:
    return OperatorDocument(
        id="op-1",
        user_id="user-1",
        operator_session_id="sess-1",
        current_hostname="host-1",
        operator_type=OperatorType.SYSTEM,
        bound_web_session_id="ws-1",
    )


def _make_investigation() -> EnrichedInvestigationContext:
    return EnrichedInvestigationContext(
        id="inv-1",
        user_id="user-1",
        case_id="case-1",
        web_session_id="ws-1",
        sentinel_mode=False,
        operator_documents=[_make_operator()],
    )


def _make_g8e_context() -> G8eHttpContext:
    return G8eHttpContext(
        web_session_id="ws-1",
        user_id="user-1",
        case_id="case-1",
        investigation_id="inv-1",
        source_component=ComponentName.G8EE,
    )


def _write_auto_approved_json(tmp_path: Path, base_commands: list[str]) -> Path:
    """Write a temporary auto_approved.json with the given base commands."""
    payload = {
        "enabled": True,
        "version": 1,
        "description": "test fixture",
        "auto_approved_commands": [
            {"value": cmd, "reason": f"test fixture for {cmd}"}
            for cmd in base_commands
        ],
    }
    path = tmp_path / "auto_approved.json"
    path.write_text(json.dumps(payload), encoding="utf-8")
    return path


def _build_service(auto_approved_path: Path) -> tuple[
    object,  # OperatorCommandService
    FakeApprovalService,
    FakeEventService,
]:
    approval_service = FakeApprovalService()
    event_service = FakeEventService()
    execution_service = FakeExecutionService(
        g8ed_event_service=event_service,
        ai_response_analyzer=FakeAIResponseAnalyzer(),
    )
    validator = CommandAutoApprovedValidator(
        auto_approved_path=str(auto_approved_path)
    )
    service = build_command_service(
        approval_service=approval_service,
        execution_service=execution_service,
        auto_approved_validator=validator,
    )
    return service, approval_service, event_service


def _approval_preparing_events(event_service: FakeEventService) -> list[dict]:
    return [
        rec
        for rec in event_service.command_events
        if rec["event_type"] == EventType.OPERATOR_COMMAND_APPROVAL_PREPARING
    ]


# ---------------------------------------------------------------------------
# Tests
# ---------------------------------------------------------------------------

class TestAutoApproveJsonIntegration:
    """End-to-end: JSON-listed verbs bypass the approval gate without CSV help."""

    async def test_json_listed_verb_bypasses_approval_with_empty_csv(self, tmp_path):
        """Empty CSV + JSON-listed ``uptime`` must skip approval and PREPARING event."""
        json_path = _write_auto_approved_json(tmp_path, ["uptime"])
        service, approval_service, event_service = _build_service(json_path)

        request_settings = G8eeUserSettings(
            llm=LLMSettings(),
            command_validation=CommandValidationSettings(
                enable_auto_approve=True,
                auto_approved_commands="",  # CSV intentionally empty
            ),
        )

        result = await service.execute_command(
            ExecutorCommandArgs(command="uptime", request="health check"),
            _make_g8e_context(),
            _make_investigation(),
            request_settings,
        )

        assert approval_service.command_approval_calls == [], (
            "JSON-listed verb must not trigger human approval"
        )
        assert _approval_preparing_events(event_service) == [], (
            "OPERATOR_COMMAND_APPROVAL_PREPARING must be suppressed for "
            "auto-approved commands so the UI does not flash an approval card"
        )
        assert result.command_executed == "uptime"
        # The fake execution service still drives the downstream dispatch,
        # so a successful result is expected.
        assert result.success is True

    async def test_empty_json_with_csv_override_still_bypasses(self, tmp_path):
        """Empty JSON + CSV-listed verb must still bypass (regression for union)."""
        json_path = _write_auto_approved_json(tmp_path, [])
        service, approval_service, event_service = _build_service(json_path)

        request_settings = G8eeUserSettings(
            llm=LLMSettings(),
            command_validation=CommandValidationSettings(
                enable_auto_approve=True,
                auto_approved_commands="uptime",
            ),
        )

        result = await service.execute_command(
            ExecutorCommandArgs(command="uptime", request="health check"),
            _make_g8e_context(),
            _make_investigation(),
            request_settings,
        )

        assert approval_service.command_approval_calls == [], (
            "CSV-listed verb must still bypass approval when JSON is empty"
        )
        assert _approval_preparing_events(event_service) == []
        assert result.command_executed == "uptime"

    async def test_unlisted_verb_with_json_present_still_requires_approval(
        self, tmp_path
    ):
        """A verb absent from BOTH JSON and CSV must still require approval."""
        json_path = _write_auto_approved_json(tmp_path, ["uptime"])
        service, approval_service, event_service = _build_service(json_path)

        request_settings = G8eeUserSettings(
            llm=LLMSettings(),
            command_validation=CommandValidationSettings(
                enable_auto_approve=True,
                auto_approved_commands="",
            ),
        )

        await service.execute_command(
            ExecutorCommandArgs(command="cat /etc/hosts", request="inspect"),
            _make_g8e_context(),
            _make_investigation(),
            request_settings,
        )

        assert len(approval_service.command_approval_calls) == 1, (
            "Verb missing from JSON and CSV must still go through human approval"
        )
        assert len(_approval_preparing_events(event_service)) == 1
