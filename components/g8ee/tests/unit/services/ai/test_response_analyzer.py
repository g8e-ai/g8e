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
Unit tests for AIResponseAnalyzer.
"""

import pytest
from unittest.mock import patch

from app.constants import ErrorAnalysisCategory, RiskLevel
from app.llm.llm_types import Content, Part, GenerateContentResponse, Candidate
from app.services.ai.response_analyzer import AIResponseAnalyzer
from tests.fakes.fake_llm_provider import FakeLLMProvider
from app.models.settings import G8eeUserSettings, LLMSettings
from app.models.tool_results import (
    CommandRiskContext,
    ErrorAnalysisContext,
    FileOperationRiskContext,
)

pytestmark = [pytest.mark.unit]


def create_real_llm_response(text: str | None) -> GenerateContentResponse:
    """Create a real GenerateContentResponse object instead of a mock."""
    parts = [Part(text=text)] if text is not None else []
    return GenerateContentResponse(
        candidates=[
            Candidate(
                content=Content(role="model", parts=parts),
                finish_reason="STOP"
            )
        ]
    )


@pytest.fixture
def mock_settings():
    llm = LLMSettings()
    llm.assistant_model = "lite-model"
    return G8eeUserSettings(llm=llm)


@pytest.fixture
def fake_provider():
    return FakeLLMProvider()


@pytest.fixture
def analyzer():
    return AIResponseAnalyzer()


# ---------------------------------------------------------------------------
# analyze_command_risk Tests
# ---------------------------------------------------------------------------

@pytest.mark.asyncio
async def test_analyze_command_risk_success(analyzer, fake_provider, mock_settings):
    fake_provider.add_response('{"risk_level": "LOW", "reasoning": "Read-only command"}')
    
    with patch("app.services.ai.response_analyzer.get_llm_provider", return_value=fake_provider):
        result = await analyzer.analyze_command_risk(
            "ls -la",
            "Checking files",
            CommandRiskContext(),
            mock_settings
        )
    
    assert result.risk_level == RiskLevel.LOW


@pytest.mark.asyncio
async def test_analyze_command_risk_accepts_bare_enum_value(analyzer, fake_provider, mock_settings):
    """Small local models (e.g. gemma4:e2b) sometimes ignore the JSON-schema
    format hint and return a bare enum value like 'LOW' instead of
    {"risk_level": "LOW"}. For single-required-field schemas the response is
    unambiguous and must be coerced, not dropped to the HIGH fallback.
    """
    fake_provider.add_response("LOW")

    with patch("app.services.ai.response_analyzer.get_llm_provider", return_value=fake_provider):
        result = await analyzer.analyze_command_risk(
            "ls -la /tmp",
            "List /tmp",
            CommandRiskContext(),
            mock_settings,
        )

    assert result.risk_level == RiskLevel.LOW


@pytest.mark.asyncio
async def test_analyze_command_risk_accepts_fenced_json(analyzer, fake_provider, mock_settings):
    """Some models wrap JSON in ```json ... ``` fences. Strip and parse."""
    fake_provider.add_response('```json\n{"risk_level": "MEDIUM"}\n```')

    with patch("app.services.ai.response_analyzer.get_llm_provider", return_value=fake_provider):
        result = await analyzer.analyze_command_risk(
            "systemctl restart nginx",
            "Restart nginx",
            CommandRiskContext(),
            mock_settings,
        )

    assert result.risk_level == RiskLevel.MEDIUM


@pytest.mark.asyncio
async def test_analyze_command_risk_accepts_json_after_preamble(analyzer, fake_provider, mock_settings):
    """Models sometimes prefix prose before the JSON object. Extract and parse."""
    fake_provider.add_response('Here is my classification:\n{"risk_level": "HIGH"}\nThanks.')

    with patch("app.services.ai.response_analyzer.get_llm_provider", return_value=fake_provider):
        result = await analyzer.analyze_command_risk(
            "rm -rf /var/data",
            "Wipe",
            CommandRiskContext(),
            mock_settings,
        )

    assert result.risk_level == RiskLevel.HIGH


@pytest.mark.asyncio
async def test_analyze_command_risk_fallback_on_empty_response(analyzer, fake_provider, mock_settings):
    # Queue a response with no text parts
    response = GenerateContentResponse(candidates=[Candidate(content=Content(role="model", parts=[]), finish_reason="STOP")])
    fake_provider.responses.append(response)
    
    with patch("app.services.ai.response_analyzer.get_llm_provider", return_value=fake_provider):
        result = await analyzer.analyze_command_risk(
            "rm -rf /",
            "Destructive",
            CommandRiskContext(),
            mock_settings
        )
    
    assert result.risk_level == RiskLevel.HIGH


@pytest.mark.asyncio
async def test_analyze_command_risk_fallback_on_exception(analyzer, fake_provider, mock_settings):
    with patch("app.services.ai.response_analyzer.get_llm_provider", side_effect=Exception("LLM error")):
        result = await analyzer.analyze_command_risk(
            "ls",
            "List",
            CommandRiskContext(),
            mock_settings
        )
    
    assert result.risk_level == RiskLevel.HIGH


# ---------------------------------------------------------------------------
# analyze_error_and_suggest_fix Tests
# ---------------------------------------------------------------------------

@pytest.mark.asyncio
async def test_analyze_error_short_circuit_at_retry_limit(analyzer, mock_settings):
    context = ErrorAnalysisContext(retry_count=2)
    
    result = await analyzer.analyze_error_and_suggest_fix(
        command="npm install",
        exit_code=1,
        stdout="",
        stderr="network timeout",
        context=context,
        settings=mock_settings
    )
    
    assert result.can_auto_fix is False
    assert result.should_escalate is True
    assert "Retry limit reached" in result.reasoning


@pytest.mark.asyncio
async def test_analyze_error_success(analyzer, fake_provider, mock_settings):
    fake_provider.add_response('''{
            "error_category": "dependency",
            "root_cause": "Missing npm package",
            "can_auto_fix": true,
            "should_escalate": false,
            "reasoning": "Package is missing, can install",
            "suggested_fix": "npm install lodash",
            "user_message": "I'll install the missing package."
        }''')
    
    with patch("app.services.ai.response_analyzer.get_llm_provider", return_value=fake_provider):
        result = await analyzer.analyze_error_and_suggest_fix(
            command="node app.js",
            exit_code=1,
            stdout="",
            stderr="Error: Cannot find module 'lodash'",
            context=ErrorAnalysisContext(),
            settings=mock_settings
        )
    
    assert result.error_category == ErrorAnalysisCategory.DEPENDENCY
    assert result.can_auto_fix is True
    assert result.suggested_fix == "npm install lodash"


@pytest.mark.asyncio
async def test_analyze_error_enforces_escalation_at_retry_limit(analyzer, fake_provider, mock_settings):
    # LLM might incorrectly say it's fixable even at limit
    fake_provider.add_response('''{
            "error_category": "dependency",
            "root_cause": "Still missing",
            "can_auto_fix": true,
            "should_escalate": false,
            "reasoning": "Try again",
            "user_message": "Trying again"
        }''')
    
    with patch("app.services.ai.response_analyzer.get_llm_provider", return_value=fake_provider):
        result = await analyzer.analyze_error_and_suggest_fix(
            command="node app.js",
            exit_code=1,
            stdout="",
            stderr="Error",
            context=ErrorAnalysisContext(retry_count=2),
            settings=mock_settings
        )
    
    assert result.can_auto_fix is False
    assert result.should_escalate is True


@pytest.mark.asyncio
async def test_analyze_error_fallback_on_exception(analyzer, fake_provider, mock_settings):
    with patch("app.services.ai.response_analyzer.get_llm_provider", side_effect=Exception("LLM fail")):
        result = await analyzer.analyze_error_and_suggest_fix(
            command="ls",
            exit_code=1,
            stdout="",
            stderr="error",
            context=ErrorAnalysisContext(),
            settings=mock_settings
        )
    
    assert result.can_auto_fix is False
    assert result.should_escalate is True
    assert result.error_category == ErrorAnalysisCategory.UNKNOWN


# ---------------------------------------------------------------------------
# analyze_file_operation_risk Tests
# ---------------------------------------------------------------------------

@pytest.mark.asyncio
async def test_analyze_file_operation_risk_success(analyzer, fake_provider, mock_settings):
    fake_provider.add_response('''{
            "risk_level": "MEDIUM",
            "is_system_file": false,
            "safe_to_proceed": true,
            "blocking_issues": [],
            "approval_prompt": "Proceed with editing app.py?"
        }''')
    
    with patch("app.services.ai.response_analyzer.get_llm_provider", return_value=fake_provider):
        result = await analyzer.analyze_file_operation_risk(
            operation="edit",
            file_path="app.py",
            content="print('hello')",
            context=FileOperationRiskContext(),
            settings=mock_settings
        )
    
    assert result.risk_level == RiskLevel.MEDIUM
    assert result.is_system_file is False
    assert result.safe_to_proceed is True


@pytest.mark.asyncio
async def test_analyze_file_operation_risk_system_file_override(analyzer, fake_provider, mock_settings):
    # LLM might say it's not a system file, but code overrides based on prefix
    fake_provider.add_response('''{
            "risk_level": "HIGH",
            "is_system_file": false,
            "safe_to_proceed": true,
            "blocking_issues": [],
            "approval_prompt": "Proceed?"
        }''')
    
    with patch("app.services.ai.response_analyzer.get_llm_provider", return_value=fake_provider):
        result = await analyzer.analyze_file_operation_risk(
            operation="edit",
            file_path="/etc/passwd",
            content="",
            context=FileOperationRiskContext(),
            settings=mock_settings
        )
    
    assert result.is_system_file is True
    assert result.safe_to_proceed is False  # HIGH + system file = safe_to_proceed False


@pytest.mark.asyncio
async def test_analyze_file_operation_risk_fallback_on_exception(analyzer, fake_provider, mock_settings):
    with patch("app.services.ai.response_analyzer.get_llm_provider", side_effect=Exception("LLM fail")):
        result = await analyzer.analyze_file_operation_risk(
            operation="delete",
            file_path="important.db",
            content="",
            context=FileOperationRiskContext(),
            settings=mock_settings
        )
    
    assert result.risk_level == RiskLevel.HIGH
    assert result.safe_to_proceed is False
    assert "Risk analysis failed" in result.blocking_issues[0]
