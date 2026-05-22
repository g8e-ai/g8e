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

from unittest.mock import AsyncMock, MagicMock
import pytest
from app.constants import ComponentName
from app.models.agent import OperatorContext
from app.models.http_context import G8eHttpContext
from app.models.reputation import ReputationCommitment
from app.models.settings import G8eeUserSettings, LLMSettings
from app.models.tribunal_commands import TribunalGenerationRequest
from app.models.whitelist import WhitelistedCommand

def _make_mock_reputation_service() -> MagicMock:
    svc = MagicMock()
    svc.list_states = AsyncMock(return_value=[])
    svc.get_latest_commitment = AsyncMock(return_value=None)

    async def _create_commitment(commitment: ReputationCommitment) -> ReputationCommitment:
        return commitment

    svc.create_commitment = AsyncMock(side_effect=_create_commitment)
    return svc

@pytest.fixture
def mock_reputation_service():
    return _make_mock_reputation_service()

@pytest.fixture
def mock_operator_context():
    return OperatorContext(
        operator_id="test-operator",
        os="linux",
        shell="bash",
        username="testuser",
        uid=1000,
        working_directory="/home/testuser",
        hostname="testhost",
        architecture="x86_64",
    )

@pytest.fixture
def mock_g8e_context():
    return G8eHttpContext(
        web_session_id="test-session-id",
        user_id="test-user-id",
        case_id="test-case-id",
        investigation_id="test-investigation-id",
        source_component=ComponentName.G8EE,
    )

def _make_mock_provider(generate_content_lite_side_effect=None, generate_content_lite_return=None):
    mock_provider = MagicMock()
    if generate_content_lite_side_effect is not None:
        mock_provider.generate_content_lite = AsyncMock(side_effect=generate_content_lite_side_effect)
    elif generate_content_lite_return is not None:
        mock_provider.generate_content_lite = AsyncMock(return_value=generate_content_lite_return)
    mock_provider.__aenter__ = AsyncMock(return_value=mock_provider)
    mock_provider.__aexit__ = AsyncMock(return_value=False)
    return mock_provider

@pytest.fixture
def make_mock_provider():
    return _make_mock_provider


_MOCK_USER_SETTINGS = G8eeUserSettings(
    llm=LLMSettings(
        assistant_model="test-assistant",
        primary_model="test-primary",
        lite_model="test-lite"
    )
)


def make_tribunal_generation_request(
    request: str,
    guidelines: str = "",
    operator_context: OperatorContext | None = None,
    event_service: EventServiceProtocol | None = None,
    g8e_context: G8eHttpContext | None = None,
    settings: G8eeUserSettings | None = None,
    reputation_data_service: ReputationDataService | None = None,
    auditor_hmac_key: str = "test-hmac-key",
    ai_response_analyzer: AIResponseAnalyzerProtocol | None = None,
    investigation_state: str = "",
    investigation_context: str = "",
    whitelisting_enabled: bool = False,
    blacklisting_enabled: bool = False,
    whitelisted_commands: list[WhitelistedCommand] | None = None,
    blacklisted_commands: list[str] | None = None,
) -> TribunalGenerationRequest:
    """Centralized helper for TribunalGenerationRequest with sensible defaults.

    Reduces boilerplate across test files. Tests can override specific fields
    by passing custom parameters.
    """
    if operator_context is None:
        operator_context = _make_mock_operator_context()
    if g8e_context is None:
        g8e_context = _make_mock_g8e_context()
    if settings is None:
        settings = _MOCK_USER_SETTINGS
    if whitelisted_commands is None:
        whitelisted_commands = []
    if blacklisted_commands is None:
        blacklisted_commands = []
    if reputation_data_service is None:
        reputation_data_service = _make_mock_reputation_service()
    return TribunalGenerationRequest(
        request=request,
        guidelines=guidelines,
        operator_context=operator_context,
        event_service=event_service,
        g8e_context=g8e_context,
        settings=settings,
        reputation_data_service=reputation_data_service,
        auditor_hmac_key=auditor_hmac_key,
        ai_response_analyzer=ai_response_analyzer,
        investigation_state=investigation_state,
        investigation_context=investigation_context,
        whitelisting_enabled=whitelisting_enabled,
        blacklisting_enabled=blacklisting_enabled,
        whitelisted_commands=whitelisted_commands,
        blacklisted_commands=blacklisted_commands,
    )


def _make_mock_operator_context() -> OperatorContext:
    return OperatorContext(
        operator_id="test-operator",
        os="linux",
        shell="bash",
        username="testuser",
        uid=1000,
        working_directory="/home/testuser",
        hostname="testhost",
        architecture="x86_64",
    )


def _make_mock_g8e_context() -> G8eHttpContext:
    return G8eHttpContext(
        web_session_id="test-session-id",
        user_id="test-user-id",
        case_id="test-case-id",
        investigation_id="test-investigation-id",
        source_component=ComponentName.G8EE,
    )


@pytest.fixture
def tribunal_generation_request():
    """Fixture that returns the make_tribunal_generation_request helper.

    Allows tests to use the helper as a fixture or import it directly.
    """
    return make_tribunal_generation_request
