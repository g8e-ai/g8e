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
