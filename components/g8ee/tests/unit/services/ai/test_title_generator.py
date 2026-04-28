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
Unit tests for the Title Generator service.
"""

import pytest
from unittest.mock import AsyncMock, patch

from app.llm.llm_types import Content, Part, GenerateContentResponse, Candidate
from app.services.ai.title_generator import generate_case_title, _create_fallback_title
from app.models.agents.title_generator import CaseTitleResult

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
    from app.models.settings import G8eeUserSettings, LLMSettings
    llm = LLMSettings()
    llm.lite_provider = "ollama"
    llm.lite_model = "lite-model"
    return G8eeUserSettings(llm=llm)


@pytest.fixture
def mock_provider():
    provider = AsyncMock()
    provider.__aenter__ = AsyncMock(return_value=provider)
    provider.__aexit__ = AsyncMock(return_value=False)
    provider.generate_content_lite = AsyncMock()
    with patch("app.services.ai.title_generator.get_llm_provider", return_value=provider):
        yield provider


# ---------------------------------------------------------------------------
# generate_case_title Tests
# ---------------------------------------------------------------------------

@pytest.mark.asyncio
async def test_generate_title_returns_default_for_empty_description(mock_settings):
    """Test that empty or whitespace descriptions return a default title with fallback=True."""
    result = await generate_case_title(None, max_length=80, settings=mock_settings)
    assert isinstance(result, CaseTitleResult)
    assert result.generated_title == "New Technical Support Case"
    assert result.fallback is True

    result = await generate_case_title("", max_length=80, settings=mock_settings)
    assert isinstance(result, CaseTitleResult)
    assert result.generated_title == "New Technical Support Case"
    assert result.fallback is True

    result = await generate_case_title("   ", max_length=80, settings=mock_settings)
    assert isinstance(result, CaseTitleResult)
    assert result.generated_title == "New Technical Support Case"
    assert result.fallback is True


@pytest.mark.asyncio
async def test_generate_title_success(mock_provider, mock_settings):
    """Test successful title generation using LLM."""
    mock_provider.generate_content_lite.return_value = create_real_llm_response("Fixed DNS Resolution Issue")

    result = await generate_case_title(
        description="I am having trouble resolving DNS on my local machine.",
        max_length=80,
        settings=mock_settings
    )

    assert isinstance(result, CaseTitleResult)
    assert result.generated_title == "Fixed DNS Resolution Issue"
    assert result.fallback is False


@pytest.mark.asyncio
async def test_generate_title_removes_quotes(mock_provider, mock_settings):
    """Test that LLM-generated quotes are stripped."""
    mock_provider.generate_content_lite.return_value = create_real_llm_response('"Database Connection Error"')
    result = await generate_case_title("db is down", max_length=80, settings=mock_settings)
    assert isinstance(result, CaseTitleResult)
    assert result.generated_title == "Database Connection Error"
    assert result.fallback is False

    mock_provider.generate_content_lite.return_value = create_real_llm_response("'Service Restart Required'")
    result = await generate_case_title("restart svc", max_length=80, settings=mock_settings)
    assert isinstance(result, CaseTitleResult)
    assert result.generated_title == "Service Restart Required"
    assert result.fallback is False


@pytest.mark.asyncio
async def test_generate_title_truncates_long_titles(mock_provider, mock_settings):
    """Test truncation of titles exceeding max_length."""
    long_title = "This is an extremely long title that definitely exceeds the default character limit of eighty characters"
    mock_provider.generate_content_lite.return_value = create_real_llm_response(long_title)

    result = await generate_case_title("help", max_length=20, settings=mock_settings)

    assert isinstance(result, CaseTitleResult)
    assert len(result.generated_title) <= 20
    assert result.generated_title == "This is an extrem..."
    assert result.fallback is False


@pytest.mark.asyncio
async def test_generate_title_uses_fallback_on_empty_llm_response(mock_provider, mock_settings):
    """Test fallback when LLM returns no content."""
    mock_provider.generate_content_lite.return_value = create_real_llm_response("")

    description = "Nginx service is failing to start on port 80"
    result = await generate_case_title(description, max_length=80, settings=mock_settings)

    assert isinstance(result, CaseTitleResult)
    assert result.generated_title == "Nginx service is failing to start on port 80"
    assert result.fallback is True


@pytest.mark.asyncio
async def test_generate_title_uses_fallback_on_short_llm_response(mock_provider, mock_settings):
    """Test fallback when LLM returns a title that is too short (< 5 chars)."""
    mock_provider.generate_content_lite.return_value = create_real_llm_response("Fix")

    description = "Memory leak in the worker process"
    result = await generate_case_title(description, max_length=80, settings=mock_settings)

    assert isinstance(result, CaseTitleResult)
    assert result.generated_title == "Memory leak in the worker process"
    assert result.fallback is True


@pytest.mark.asyncio
async def test_generate_title_uses_fallback_on_exception(mock_provider, mock_settings):
    """Test fallback when an exception occurs during generation."""
    mock_provider.generate_content_lite.side_effect = Exception("LLM connection failed")

    description = "Permission denied when accessing /var/log/syslog"
    result = await generate_case_title(description, max_length=80, settings=mock_settings)

    assert isinstance(result, CaseTitleResult)
    assert result.generated_title == "Permission denied when accessing /var/log/syslog"
    assert result.fallback is True


# ---------------------------------------------------------------------------
# _create_fallback_title Tests
# ---------------------------------------------------------------------------

def test_fallback_title_returns_default_for_empty():
    assert _create_fallback_title(None, 80) == "New Technical Support Case"
    assert _create_fallback_title("", 80) == "New Technical Support Case"


def test_fallback_title_extracts_first_line():
    desc = "First line of context\nSecond line should be ignored"
    assert _create_fallback_title(desc, 80) == "First line of context"


def test_fallback_title_removes_common_prefixes():
    assert _create_fallback_title("Hi, I need help with nginx", 80) == "Nginx"
    assert _create_fallback_title("Hello, can you help me with the database", 80) == "The database"
    assert _create_fallback_title("i need help with SSH access", 80) == "SSH access"


def test_fallback_title_capitalizes_first_letter():
    assert _create_fallback_title("my service is down", 80) == "My service is down"


def test_fallback_title_truncates():
    desc = "This is a very long line that should be truncated when it exceeds the maximum length allowed for titles"
    title = _create_fallback_title(desc, 30)
    assert len(title) <= 30
    assert title == "This is a very long line th..."
