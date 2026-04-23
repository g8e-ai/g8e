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
Unit tests for TribunalInvoker in agent_tool_loop.py.

Tests:
- TribunalInvoker._fetch_command_constraints returns (False, False, [], []) when both disabled
- TribunalInvoker._fetch_command_constraints returns sorted whitelist when whitelisting enabled
- TribunalInvoker.run raises TribunalError when Tribunal fails
- TribunalInvoker.run preserves target_operators, expected_output_lines, timeout_seconds (regression test)
- TribunalInvoker.run correctly propagates operator context defaults when empty operator_documents

Run with:
    /home/bob/g8e/g8e test g8ee -- tests/unit/services/ai/test_agent_tool_loop.py
"""

from unittest.mock import AsyncMock, MagicMock
import pytest

from app.models.agent import ExecutorCommandArgs, OperatorContext, SageOperatorRequest
from app.models.http_context import G8eHttpContext
from app.models.investigations import EnrichedInvestigationContext
from app.models.settings import CommandValidationSettings, G8eeUserSettings
from app.services.ai.agent_tool_loop import TribunalInvoker
from app.services.ai.generator import (
    CommandGenerationResult,
    TribunalDisabledError,
)
from app.services.ai.tool_service import AIToolService
from app.utils.whitelist_validator import CommandWhitelistValidator
from app.utils.blacklist_validator import CommandBlacklistValidator

pytestmark = [pytest.mark.unit]


@pytest.fixture
def mock_tool_executor():
    """Mock AIToolService with settings."""
    tool_executor = MagicMock(spec=AIToolService)
    tool_executor.user_settings = MagicMock(spec=G8eeUserSettings)
    tool_executor.user_settings.operator_context = MagicMock(spec=OperatorContext)
    tool_executor.user_settings.operator_context.os = "linux"
    tool_executor.whitelist_validator = MagicMock(spec=CommandWhitelistValidator)
    tool_executor.blacklist_validator = MagicMock(spec=CommandBlacklistValidator)
    return tool_executor


@pytest.fixture
def mock_sage_request():
    """Mock SageOperatorRequest."""
    return SageOperatorRequest(
        request="List all files in /tmp",
        guidelines="Use ls with detailed output",
        target_operator="op-001",
        target_operators=["op-001", "op-002"],
        expected_output_lines=20,
        timeout_seconds=600,
    )


@pytest.fixture
def mock_investigation():
    """Mock EnrichedInvestigationContext."""
    investigation = MagicMock(spec=EnrichedInvestigationContext)
    investigation.id = "inv-001"
    investigation.operator_documents = []
    return investigation


@pytest.fixture
def mock_g8e_context():
    """Mock G8eHttpContext."""
    context = MagicMock(spec=G8eHttpContext)
    context.web_session_id = "ws-001"
    context.user_id = "user-001"
    context.case_id = "case-001"
    return context


@pytest.fixture
def mock_request_settings():
    """Mock G8eeUserSettings."""
    settings = MagicMock(spec=G8eeUserSettings)
    settings.operator_context = MagicMock(spec=OperatorContext)
    settings.operator_context.os = "linux"
    return settings


@pytest.fixture
def mock_event_service():
    """Mock EventService."""
    return AsyncMock()


class TestTribunalInvokerFetchCommandConstraints:
    """Tests for TribunalInvoker._fetch_command_constraints."""

    def test_returns_disabled_when_both_flags_false(self, mock_tool_executor):
        """Test returns (False, False, [], []) when both whitelisting and blacklisting disabled."""
        mock_tool_executor.user_settings.command_validation = CommandValidationSettings(
            enable_whitelisting=False,
            enable_blacklisting=False,
        )
        mock_tool_executor.whitelist_validator.all_commands = []
        mock_tool_executor.whitelist_validator.get_available_commands_with_metadata = MagicMock(return_value=[])
        mock_tool_executor.blacklist_validator.get_forbidden_commands.return_value = []

        result = TribunalInvoker._fetch_command_constraints(mock_tool_executor)

        assert result == (False, False, [], [])

    def test_returns_sorted_whitelist_when_enabled(self, mock_tool_executor):
        """Test returns sorted whitelist when whitelisting enabled."""
        mock_tool_executor.user_settings.command_validation = CommandValidationSettings(
            enable_whitelisting=True,
            enable_blacklisting=False,
        )
        mock_tool_executor.whitelist_validator.all_commands = ["cat", "ls", "ping"]
        mock_tool_executor.whitelist_validator.get_available_commands_with_metadata = MagicMock(return_value=[
            {"command": "cat"},
            {"command": "ls"},
            {"command": "ping"},
        ])
        mock_tool_executor.blacklist_validator.get_forbidden_commands.return_value = []

        result = TribunalInvoker._fetch_command_constraints(mock_tool_executor)

        whitelisting_enabled, blacklisting_enabled, whitelisted, blacklisted = result
        assert whitelisting_enabled is True
        assert blacklisting_enabled is False
        assert whitelisted == [{"command": "cat"}, {"command": "ls"}, {"command": "ping"}]
        assert blacklisted == []

    def test_returns_blacklist_when_enabled(self, mock_tool_executor):
        """Test returns blacklist when blacklisting enabled."""
        mock_tool_executor.user_settings.command_validation = CommandValidationSettings(
            enable_whitelisting=False,
            enable_blacklisting=True,
        )
        mock_tool_executor.whitelist_validator.all_commands = []
        mock_tool_executor.whitelist_validator.get_available_commands_with_metadata = MagicMock(return_value=[])
        mock_tool_executor.blacklist_validator.get_forbidden_commands.return_value = [
            {"command": "rm", "reason": "Destructive"},
            {"command": "dd", "reason": "Disk destruction"},
        ]

        result = TribunalInvoker._fetch_command_constraints(mock_tool_executor)

        whitelisting_enabled, blacklisting_enabled, whitelisted, blacklisted = result
        assert whitelisting_enabled is False
        assert blacklisting_enabled is True
        assert whitelisted == []
        assert len(blacklisted) == 2
        assert blacklisted[0]["command"] == "rm"


class TestTribunalInvokerRun:
    """Tests for TribunalInvoker.run."""

    @pytest.mark.asyncio
    async def test_raises_tribunal_error_when_tribunal_fails(
        self,
        mock_sage_request,
        mock_investigation,
        mock_g8e_context,
        mock_event_service,
        mock_request_settings,
        mock_tool_executor,
    ):
        """Test raises TribunalError when Tribunal fails and surfaces it unchanged."""
        from unittest.mock import patch

        mock_tool_executor.user_settings.command_validation = CommandValidationSettings(
            enable_whitelisting=False,
            enable_blacklisting=False,
        )
        mock_tool_executor.whitelist_validator.all_commands = []
        mock_tool_executor.whitelist_validator.get_available_commands_with_metadata = MagicMock(return_value=[])
        mock_tool_executor.blacklist_validator.get_forbidden_commands.return_value = []

        with patch(
            "app.services.ai.agent_tool_loop.generate_command",
            side_effect=TribunalDisabledError("Command generation disabled"),
        ):
            with pytest.raises(TribunalDisabledError) as exc_info:
                await TribunalInvoker.run(
                    sage_request=mock_sage_request,
                    investigation=mock_investigation,
                    g8e_context=mock_g8e_context,
                    g8ed_event_service=mock_event_service,
                    request_settings=mock_request_settings,
                    tool_executor=mock_tool_executor,
                )

            assert "Tribunal is disabled" in str(exc_info.value)

    @pytest.mark.asyncio
    async def test_preserves_target_operators_expected_output_timeout(
        self,
        mock_sage_request,
        mock_investigation,
        mock_g8e_context,
        mock_event_service,
        mock_request_settings,
        mock_tool_executor,
    ):
        """Test preserves target_operators, expected_output_lines, timeout_seconds (regression test)."""
        from unittest.mock import patch

        mock_tool_executor.user_settings.command_validation = CommandValidationSettings(
            enable_whitelisting=False,
            enable_blacklisting=False,
        )
        mock_tool_executor.whitelist_validator.all_commands = []
        mock_tool_executor.whitelist_validator.get_available_commands_with_metadata = MagicMock(return_value=[])
        mock_tool_executor.blacklist_validator.get_forbidden_commands.return_value = []

        mock_gen_result = CommandGenerationResult(
            request="List all files in /tmp",
            guidelines="Use ls with detailed output",
            final_command="ls -la /tmp",
            outcome="verified",
            vote_score=1.0,
            auditor_passed=True,
            auditor_revision=None,
            candidates=[],
        )

        with patch(
            "app.services.ai.agent_tool_loop.generate_command",
            return_value=mock_gen_result,
        ):
            executor_args, gen_result = await TribunalInvoker.run(
                sage_request=mock_sage_request,
                investigation=mock_investigation,
                g8e_context=mock_g8e_context,
                g8ed_event_service=mock_event_service,
                request_settings=mock_request_settings,
                tool_executor=mock_tool_executor,
            )

            assert executor_args.target_operators == ["op-001", "op-002"]
            assert executor_args.expected_output_lines == 20
            assert executor_args.timeout_seconds == 600
            assert gen_result == mock_gen_result

    @pytest.mark.asyncio
    async def test_propagates_operator_context_defaults_when_empty(
        self,
        mock_sage_request,
        mock_investigation,
        mock_g8e_context,
        mock_event_service,
        mock_request_settings,
        mock_tool_executor,
    ):
        """Test correctly propagates operator context defaults when investigation.operator_documents is empty."""
        from unittest.mock import patch

        mock_tool_executor.user_settings.command_validation = CommandValidationSettings(
            enable_whitelisting=False,
            enable_blacklisting=False,
        )
        mock_tool_executor.whitelist_validator.all_commands = []
        mock_tool_executor.whitelist_validator.get_available_commands_with_metadata = MagicMock(return_value=[])
        mock_tool_executor.blacklist_validator.get_forbidden_commands.return_value = []

        mock_investigation.operator_documents = []

        mock_gen_result = CommandGenerationResult(
            request="List all files in /tmp",
            guidelines="Use ls with detailed output",
            final_command="ls -la /tmp",
            outcome="verified",
            vote_score=1.0,
            auditor_passed=True,
            auditor_revision=None,
            candidates=[],
        )

        with patch(
            "app.services.ai.agent_tool_loop.generate_command",
            return_value=mock_gen_result,
        ):
            executor_args, gen_result = await TribunalInvoker.run(
                sage_request=mock_sage_request,
                investigation=mock_investigation,
                g8e_context=mock_g8e_context,
                g8ed_event_service=mock_event_service,
                request_settings=mock_request_settings,
                tool_executor=mock_tool_executor,
            )

            assert executor_args.command == "ls -la /tmp"
            assert executor_args.request == "List all files in /tmp"
            assert executor_args.guidelines == "Use ls with detailed output"

    @pytest.mark.asyncio
    async def test_executor_command_args_round_trip_preserves_fields(
        self,
        mock_sage_request,
        mock_investigation,
        mock_g8e_context,
        mock_event_service,
        mock_request_settings,
        mock_tool_executor,
    ):
        """Contract test: SageOperatorRequest → TribunalInvoker.run() → ExecutorCommandArgs preserves target_operators, expected_output_lines, timeout_seconds."""
        from unittest.mock import patch

        mock_tool_executor.user_settings.command_validation = CommandValidationSettings(
            enable_whitelisting=False,
            enable_blacklisting=False,
        )
        mock_tool_executor.whitelist_validator.all_commands = []
        mock_tool_executor.whitelist_validator.get_available_commands_with_metadata = MagicMock(return_value=[])
        mock_tool_executor.blacklist_validator.get_forbidden_commands.return_value = []

        mock_gen_result = CommandGenerationResult(
            request="List all files in /tmp",
            guidelines="Use ls with detailed output",
            final_command="ls -la /tmp",
            outcome="verified",
            vote_score=1.0,
            auditor_passed=True,
            auditor_revision=None,
            candidates=[],
        )

        with patch(
            "app.services.ai.agent_tool_loop.generate_command",
            return_value=mock_gen_result,
        ):
            executor_args, gen_result = await TribunalInvoker.run(
                sage_request=mock_sage_request,
                investigation=mock_investigation,
                g8e_context=mock_g8e_context,
                g8ed_event_service=mock_event_service,
                request_settings=mock_request_settings,
                tool_executor=mock_tool_executor,
            )

            # Verify round-trip of critical fields
            assert executor_args.target_operators == mock_sage_request.target_operators
            assert executor_args.expected_output_lines == mock_sage_request.expected_output_lines
            assert executor_args.timeout_seconds == mock_sage_request.timeout_seconds

            # Verify the fields survive serialization/deserialization
            serialized = executor_args.model_dump(by_alias=True)
            assert serialized["target_operators"] == mock_sage_request.target_operators
            assert serialized["expected_output_lines"] == mock_sage_request.expected_output_lines
            assert serialized["timeout_seconds"] == mock_sage_request.timeout_seconds

            # Reconstruct from serialized data and verify fields still intact
            reconstructed = ExecutorCommandArgs(**serialized)
            assert reconstructed.target_operators == mock_sage_request.target_operators
            assert reconstructed.expected_output_lines == mock_sage_request.expected_output_lines
            assert reconstructed.timeout_seconds == mock_sage_request.timeout_seconds
