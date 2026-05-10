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
from app.constants import EventType, ComponentName
from app.models.agents.tribunal import (
    TribunalSessionGenerationFailedPayload,
    TribunalPassCompletedPayload,
    TribunalMember,
)
from app.services.ai.tribunal.emitter import TribunalEmitter

@pytest.mark.asyncio
class TestTribunalEmitter:
    """TribunalEmitter distinguishes terminal from progress events and handles publish failures appropriately."""

    async def test_terminal_event_publish_failure_raises(self, mock_g8e_context):
        """Terminal event publish failures are re-raised to ensure caller is aware of the failure."""
        mock_event_service = MagicMock()
        mock_event_service.publish = AsyncMock(side_effect=RuntimeError("broker down"))

        emitter = TribunalEmitter(event_service=mock_event_service, g8e_context=mock_g8e_context)

        with pytest.raises(RuntimeError, match="broker down"):
            await emitter.emit(
                EventType.TRIBUNAL_SESSION_GENERATION_FAILED,
                TribunalSessionGenerationFailedPayload(request="test", pass_errors=["error"]),
            )

    async def test_progress_event_publish_failure_swallowed(self, mock_g8e_context):
        """Progress event publish failures are logged but not re-raised."""
        mock_event_service = MagicMock()
        mock_event_service.publish = AsyncMock(side_effect=RuntimeError("broker down"))

        emitter = TribunalEmitter(event_service=mock_event_service, g8e_context=mock_g8e_context)

        await emitter.emit(
            EventType.TRIBUNAL_VOTING_PASS_COMPLETED,
            TribunalPassCompletedPayload(
                pass_index=0, member=TribunalMember.AXIOM, candidate="ls", success=True
            ),
        )

        mock_event_service.publish.assert_called_once()
