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
Unit tests for command constraints (whitelist/blacklist) functionality.

Tests:
- _handle_get_command_constraints with both flags disabled
- _handle_get_command_constraints with whitelisting only
- _handle_get_command_constraints with blacklisting only
- _handle_get_command_constraints with both enabled
- execute_command rejection when command violates whitelist
- execute_command rejection when command violates blacklist
- CommandConstraintsResult model serialization
- get_tools returns GET_COMMAND_CONSTRAINTS declaration in all modes
- CommandBlacklistValidator public getter methods
- OperatorCommandService enforcement integration

Run with:
    /home/bob/g8e/g8e test g8ee -- tests/unit/services/ai/test_command_constraints.py
"""

from unittest.mock import AsyncMock, MagicMock
import logging
import pytest

from app.constants import AgentMode, OperatorToolName
from app.models.agent import OperatorContext
from app.models.http_context import G8eHttpContext
from app.models.investigations import EnrichedInvestigationContext
from app.models.settings import CommandValidationSettings, G8eeUserSettings
from app.models.tool_results import CommandConstraintsResult
from app.models.whitelist import CommandValidationResult, WhitelistedCommand
from app.services.ai.tool_service import AIToolService
from app.utils.whitelist_validator import CommandWhitelistValidator
from app.utils.blacklist_validator import CommandBlacklistValidator, CommandBlacklistResult

pytestmark = [pytest.mark.unit]


# =============================================================================
# FIXTURES
# =============================================================================

@pytest.fixture
def mock_user_settings_disabled():
    """User settings with both validations disabled."""
    settings = MagicMock(spec=G8eeUserSettings)
    settings.command_validation = CommandValidationSettings(
        enable_whitelisting=False,
        enable_blacklisting=False,
    )
    settings.operator_context = MagicMock(spec=OperatorContext)
    settings.operator_context.os = "linux"
    return settings


@pytest.fixture
def mock_user_settings_whitelist_only():
    """User settings with whitelisting enabled only."""
    settings = MagicMock(spec=G8eeUserSettings)
    settings.command_validation = CommandValidationSettings(
        enable_whitelisting=True,
        enable_blacklisting=False,
    )
    settings.operator_context = MagicMock(spec=OperatorContext)
    settings.operator_context.os = "linux"
    return settings


@pytest.fixture
def mock_user_settings_blacklist_only():
    """User settings with blacklisting enabled only."""
    settings = MagicMock(spec=G8eeUserSettings)
    settings.command_validation = CommandValidationSettings(
        enable_whitelisting=False,
        enable_blacklisting=True,
    )
    settings.operator_context = MagicMock(spec=OperatorContext)
    settings.operator_context.os = "linux"
    return settings


@pytest.fixture
def mock_user_settings_both():
    """User settings with both validations enabled."""
    settings = MagicMock(spec=G8eeUserSettings)
    settings.command_validation = CommandValidationSettings(
        enable_whitelisting=True,
        enable_blacklisting=True,
    )
    settings.operator_context = MagicMock(spec=OperatorContext)
    settings.operator_context.os = "linux"
    return settings


@pytest.fixture
def mock_whitelist_validator():
    """Mock whitelist validator with sample data."""
    validator = MagicMock(spec=CommandWhitelistValidator)
    validator.all_commands = {"ping", "ls", "cat"}
    validator.forbidden_patterns = [r"rm -rf", r"format"]
    validator.forbidden_directories = ["/etc", "/boot"]
    validator.validate_command = MagicMock(return_value=CommandValidationResult(
        is_valid=True,
        command="ping",
    ))
    validator.get_available_commands_with_metadata = MagicMock(return_value=[
        WhitelistedCommand(command="cat"),
        WhitelistedCommand(command="ls"),
        WhitelistedCommand(command="ping"),
    ])
    return validator


@pytest.fixture
def mock_blacklist_validator():
    """Mock blacklist validator with sample data."""
    validator = MagicMock(spec=CommandBlacklistValidator)
    validator.get_forbidden_commands = MagicMock(return_value=[
        {"command": "rm", "reason": "Destructive command"},
        {"command": "dd", "reason": "Disk destruction"},
    ])
    validator.get_forbidden_substrings = MagicMock(return_value=[
        {"substring": "format", "reason": "Disk formatting"},
    ])
    validator.get_forbidden_patterns = MagicMock(return_value=[
        {"pattern": r"rm\s+-rf\s+/", "reason": "Recursive delete"},
    ])
    validator.validate_command = MagicMock(return_value=CommandBlacklistResult(
        is_allowed=True,
    ))
    return validator


@pytest.fixture
def mock_g8e_context():
    """Mock G8eHttpContext."""
    return MagicMock(spec=G8eHttpContext)


@pytest.fixture
def mock_investigation():
    """Mock EnrichedInvestigationContext."""
    investigation = MagicMock(spec=EnrichedInvestigationContext)
    investigation.operator_documents = []
    return investigation


@pytest.fixture
def mock_request_settings():
    """Mock G8eeUserSettings."""
    settings = MagicMock(spec=G8eeUserSettings)
    settings.operator_context = MagicMock(spec=OperatorContext)
    settings.operator_context.os = "linux"
    return settings


@pytest.fixture
def mock_operator_command_service():
    """Mock OperatorCommandService."""
    return AsyncMock()


@pytest.fixture
def mock_investigation_service():
    """Mock InvestigationService."""
    return AsyncMock()


# =============================================================================
# TESTS: _handle_get_command_constraints
# =============================================================================

@pytest.mark.asyncio(loop_scope="session")
async def test_handle_get_command_constraints_both_disabled(
    mock_user_settings_disabled,
    mock_operator_command_service,
    mock_investigation_service,
    mock_g8e_context,
    mock_investigation,
    mock_request_settings,
    mock_whitelist_validator,
    mock_blacklist_validator,
):
    """Test handler returns empty constraint data when both validations disabled."""
    tool_service = AIToolService(
        operator_command_service=mock_operator_command_service,
        investigation_service=mock_investigation_service,
        reputation_data_service=AsyncMock(),
        reputation_service=AsyncMock(),
        stake_resolution_data_service=AsyncMock(),
        chat_task_manager=MagicMock(),
        web_search_provider=None,
        platform_settings=None,
        user_settings=mock_user_settings_disabled,
        whitelist_validator=mock_whitelist_validator,
        blacklist_validator=mock_blacklist_validator,
    )

    result = await tool_service._handle_get_command_constraints(
        tool_args={},
        investigation=mock_investigation,
        g8e_context=mock_g8e_context,
        request_settings=mock_request_settings,
        execution_id=None,
    )

    assert isinstance(result, CommandConstraintsResult)
    assert result.success is True
    assert result.whitelisting_enabled is False
    assert result.blacklisting_enabled is False
    assert result.whitelisted_commands == []
    assert result.blacklisted_commands == []
    assert result.blacklisted_substrings == []
    assert result.blacklisted_patterns == []
    assert "No command constraints are currently enforced" in result.message


@pytest.mark.asyncio(loop_scope="session")
async def test_handle_get_command_constraints_whitelist_only(
    mock_user_settings_whitelist_only,
    mock_operator_command_service,
    mock_investigation_service,
    mock_g8e_context,
    mock_investigation,
    mock_request_settings,
    mock_whitelist_validator,
    mock_blacklist_validator,
):
    """Test handler returns whitelist data when whitelisting enabled."""
    tool_service = AIToolService(
        operator_command_service=mock_operator_command_service,
        investigation_service=mock_investigation_service,
        reputation_data_service=AsyncMock(),
        reputation_service=AsyncMock(),
        stake_resolution_data_service=AsyncMock(),
        chat_task_manager=MagicMock(),
        web_search_provider=None,
        platform_settings=None,
        user_settings=mock_user_settings_whitelist_only,
        whitelist_validator=mock_whitelist_validator,
        blacklist_validator=mock_blacklist_validator,
    )

    result = await tool_service._handle_get_command_constraints(
        tool_args={},
        investigation=mock_investigation,
        g8e_context=mock_g8e_context,
        request_settings=mock_request_settings,
        execution_id=None,
    )

    assert isinstance(result, CommandConstraintsResult)
    assert result.success is True
    assert result.whitelisting_enabled is True
    assert result.blacklisting_enabled is False
    assert sorted([cmd.command for cmd in result.whitelisted_commands]) == ["cat", "ls", "ping"]
    assert result.global_forbidden_patterns == [r"rm -rf", r"format"]
    assert result.global_forbidden_directories == ["/etc", "/boot"]
    assert result.blacklisted_commands == []
    assert "Whitelisting ENABLED" in result.message


@pytest.mark.asyncio(loop_scope="session")
async def test_handle_get_command_constraints_blacklist_only(
    mock_user_settings_blacklist_only,
    mock_operator_command_service,
    mock_investigation_service,
    mock_g8e_context,
    mock_investigation,
    mock_request_settings,
    mock_whitelist_validator,
    mock_blacklist_validator,
):
    """Test handler returns blacklist data when blacklisting enabled."""
    tool_service = AIToolService(
        operator_command_service=mock_operator_command_service,
        investigation_service=mock_investigation_service,
        reputation_data_service=AsyncMock(),
        reputation_service=AsyncMock(),
        stake_resolution_data_service=AsyncMock(),
        chat_task_manager=MagicMock(),
        web_search_provider=None,
        platform_settings=None,
        user_settings=mock_user_settings_blacklist_only,
        whitelist_validator=mock_whitelist_validator,
        blacklist_validator=mock_blacklist_validator,
    )

    result = await tool_service._handle_get_command_constraints(
        tool_args={},
        investigation=mock_investigation,
        g8e_context=mock_g8e_context,
        request_settings=mock_request_settings,
        execution_id=None,
    )

    assert isinstance(result, CommandConstraintsResult)
    assert result.success is True
    assert result.whitelisting_enabled is False
    assert result.blacklisting_enabled is True
    assert result.whitelisted_commands == []
    assert len(result.blacklisted_commands) == 2
    assert result.blacklisted_commands[0]["command"] == "rm"
    assert result.blacklisted_commands[0]["reason"] == "Destructive command"
    assert len(result.blacklisted_substrings) == 1
    assert result.blacklisted_substrings[0]["substring"] == "format"
    assert len(result.blacklisted_patterns) == 1
    assert result.blacklisted_patterns[0]["pattern"] == r"rm\s+-rf\s+/"
    assert "Blacklisting ENABLED" in result.message


@pytest.mark.asyncio(loop_scope="session")
async def test_handle_get_command_constraints_both_enabled(
    mock_user_settings_both,
    mock_operator_command_service,
    mock_investigation_service,
    mock_g8e_context,
    mock_investigation,
    mock_request_settings,
    mock_whitelist_validator,
    mock_blacklist_validator,
):
    """Test handler returns both whitelist and blacklist data when both enabled."""
    from unittest.mock import AsyncMock, MagicMock
    tool_service = AIToolService(
        operator_command_service=mock_operator_command_service,
        investigation_service=mock_investigation_service,
        reputation_data_service=AsyncMock(),
        reputation_service=AsyncMock(),
        stake_resolution_data_service=AsyncMock(),
        chat_task_manager=MagicMock(),
        web_search_provider=None,
        platform_settings=None,
        user_settings=mock_user_settings_both,
        whitelist_validator=mock_whitelist_validator,
        blacklist_validator=mock_blacklist_validator,
    )

    result = await tool_service._handle_get_command_constraints(
        tool_args={},
        investigation=mock_investigation,
        g8e_context=mock_g8e_context,
        request_settings=mock_request_settings,
        execution_id=None,
    )

    assert isinstance(result, CommandConstraintsResult)
    assert result.success is True
    assert result.whitelisting_enabled is True
    assert result.blacklisting_enabled is True
    assert sorted([cmd.command for cmd in result.whitelisted_commands]) == ["cat", "ls", "ping"]
    assert len(result.blacklisted_commands) == 2
    assert "Whitelisting ENABLED" in result.message
    assert "Blacklisting ENABLED" in result.message


@pytest.mark.asyncio(loop_scope="session")
async def test_handle_get_command_constraints_csv_override(
    mock_operator_command_service,
    mock_investigation_service,
    mock_g8e_context,
    mock_investigation,
    mock_whitelist_validator,
    mock_blacklist_validator,
):
    """Test handler returns CSV override commands when whitelisted_commands CSV is set."""
    from unittest.mock import AsyncMock, MagicMock
    
    # Create settings with CSV override
    settings = MagicMock(spec=G8eeUserSettings)
    settings.command_validation = CommandValidationSettings(
        enable_whitelisting=True,
        enable_blacklisting=False,
        whitelisted_commands="uptime,df,free",
    )
    
    tool_service = AIToolService(
        operator_command_service=mock_operator_command_service,
        investigation_service=mock_investigation_service,
        reputation_data_service=AsyncMock(),
        reputation_service=AsyncMock(),
        stake_resolution_data_service=AsyncMock(),
        chat_task_manager=MagicMock(),
        web_search_provider=None,
        platform_settings=None,
        user_settings=settings,
        whitelist_validator=mock_whitelist_validator,
        blacklist_validator=mock_blacklist_validator,
    )

    result = await tool_service._handle_get_command_constraints(
        tool_args={},
        investigation=mock_investigation,
        g8e_context=mock_g8e_context,
        request_settings=settings,
        execution_id=None,
    )

    assert isinstance(result, CommandConstraintsResult)
    assert result.success is True
    assert result.whitelisting_enabled is True
    assert result.blacklisting_enabled is False
    # Should return CSV commands, not JSON validator commands
    assert len(result.whitelisted_commands) == 3
    assert result.whitelisted_commands == [
        WhitelistedCommand(command="uptime"),
        WhitelistedCommand(command="df"),
        WhitelistedCommand(command="free"),
    ]
    assert result.blacklisted_commands == []
    assert "Whitelisting ENABLED" in result.message

# =============================================================================
# TESTS: CommandBlacklistValidator Public Methods
# =============================================================================

def test_blacklist_validator_get_forbidden_commands():
    """Test CommandBlacklistValidator.get_forbidden_commands returns structured data."""
    validator = CommandBlacklistValidator.__new__(CommandBlacklistValidator)
    validator._config = {
        "forbidden_commands": [
            {"value": "rm", "reason": "Destructive"},
            {"value": "dd", "reason": "Disk destruction"},
        ],
        "forbidden_binaries": [],
        "forbidden_substrings": [],
        "forbidden_arguments": [],
        "forbidden_patterns": [],
    }
    
    result = validator.get_forbidden_commands()
    
    assert len(result) == 2
    assert result[0] == {"command": "rm", "reason": "Destructive"}
    assert result[1] == {"command": "dd", "reason": "Disk destruction"}


def test_blacklist_validator_get_forbidden_substrings():
    """Test CommandBlacklistValidator.get_forbidden_substrings returns structured data."""
    validator = CommandBlacklistValidator.__new__(CommandBlacklistValidator)
    validator._config = {
        "forbidden_commands": [],
        "forbidden_binaries": [],
        "forbidden_substrings": [
            {"value": "format", "reason": "Disk formatting"},
        ],
        "forbidden_arguments": [],
        "forbidden_patterns": [],
    }
    
    result = validator.get_forbidden_substrings()
    
    assert len(result) == 1
    assert result[0] == {"substring": "format", "reason": "Disk formatting"}


def test_blacklist_validator_get_forbidden_patterns():
    """Test CommandBlacklistValidator.get_forbidden_patterns returns structured data."""
    validator = CommandBlacklistValidator.__new__(CommandBlacklistValidator)
    validator._config = {
        "forbidden_commands": [],
        "forbidden_binaries": [],
        "forbidden_substrings": [],
        "forbidden_arguments": [],
        "forbidden_patterns": [
            {"value": r"rm\s+-rf\s+/", "reason": "Recursive delete"},
        ],
    }
    
    result = validator.get_forbidden_patterns()
    
    assert len(result) == 1
    assert result[0] == {"pattern": r"rm\s+-rf\s+/", "reason": "Recursive delete"}


# =============================================================================
# TESTS: CommandConstraintsResult Serialization
# =============================================================================

@pytest.mark.unit
def test_command_constraints_result_serialization():
    """Test CommandConstraintsResult serializes correctly."""
    result = CommandConstraintsResult(
        success=True,
        whitelisting_enabled=True,
        blacklisting_enabled=False,
        whitelisted_commands=[
            WhitelistedCommand(command="ping"),
            WhitelistedCommand(command="ls"),
        ],
        blacklisted_commands=[],
        blacklisted_substrings=[],
        blacklisted_patterns=[],
        global_forbidden_patterns=[r"rm -rf"],
        global_forbidden_directories=["/etc"],
        message="Whitelisting ENABLED",
    )
    
    data = result.model_dump()
    
    assert data["success"] is True
    assert data["whitelisting_enabled"] is True
    assert data["blacklisting_enabled"] is False
    assert data["whitelisted_commands"][0]["command"] == "ping"
    assert data["whitelisted_commands"][1]["command"] == "ls"
    assert data["message"] == "Whitelisting ENABLED"


@pytest.mark.unit
def test_command_constraints_result_deserialization():
    """Test CommandConstraintsResult deserializes correctly."""
    data = {
        "success": True,
        "whitelisting_enabled": True,
        "blacklisting_enabled": False,
        "whitelisted_commands": [
            {"command": "ping"},
            {"command": "ls"},
        ],
        "blacklisted_commands": [],
        "blacklisted_substrings": [],
        "blacklisted_patterns": [],
        "global_forbidden_patterns": [r"rm -rf"],
        "global_forbidden_directories": ["/etc"],
        "message": "Whitelisting ENABLED",
    }
    
    result = CommandConstraintsResult(**data)
    
    assert result.success is True
    assert result.whitelisting_enabled is True
    assert result.blacklisting_enabled is False
    assert result.whitelisted_commands[0].command == "ping"
    assert result.whitelisted_commands[1].command == "ls"
    assert result.message == "Whitelisting ENABLED"


# =============================================================================
# TESTS: get_tools Returns GET_COMMAND_CONSTRAINTS
# =============================================================================

async def test_get_tools_includes_get_command_constraints(
    mock_user_settings_both,
    mock_whitelist_validator,
    mock_blacklist_validator,
):
    """Test get_tools includes GET_COMMAND_CONSTRAINTS declaration in all modes."""
    tool_service = AIToolService(
        operator_command_service=MagicMock(),
        investigation_service=MagicMock(),
        reputation_data_service=AsyncMock(),
        reputation_service=AsyncMock(),
        stake_resolution_data_service=AsyncMock(),
        chat_task_manager=MagicMock(),
        web_search_provider=None,
        platform_settings=None,
        user_settings=mock_user_settings_both,
        whitelist_validator=mock_whitelist_validator,
        blacklist_validator=mock_blacklist_validator,
    )
    
    # Test in operator_bound mode
    tools_operator = tool_service.get_tools(agent_mode=AgentMode.OPERATOR_BOUND, model_to_use=None)
    tool_names_operator = [tool.name for group in tools_operator for tool in group.tools]
    assert OperatorToolName.GET_COMMAND_CONSTRAINTS in tool_names_operator
    
    # Test in cloud_operator_bound mode
    tools_cloud = tool_service.get_tools(agent_mode=AgentMode.CLOUD_OPERATOR_BOUND, model_to_use=None)
    tool_names_cloud = [tool.name for group in tools_cloud for tool in group.tools]
    assert OperatorToolName.GET_COMMAND_CONSTRAINTS in tool_names_cloud
    
    # Test in unbound mode
    tools_unbound = tool_service.get_tools(agent_mode=AgentMode.OPERATOR_NOT_BOUND, model_to_use=None)
    tool_names_unbound = [tool.name for group in tools_unbound for tool in group.tools]
    assert OperatorToolName.GET_COMMAND_CONSTRAINTS in tool_names_unbound
