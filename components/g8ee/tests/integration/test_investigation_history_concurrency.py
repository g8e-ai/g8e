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

import asyncio
import pytest
from unittest.mock import AsyncMock

from app.constants import EventType, ComponentName
from app.models.investigations import InvestigationModel, InvestigationCreateRequest, ConversationMessageMetadata
from app.services.investigation.investigation_data_service import InvestigationDataService
from app.utils.ledger_hash import verify_chain

@pytest.fixture
def mock_cache_aside_service():
    return AsyncMock()

@pytest.fixture
def service(mock_cache_aside_service):
    return InvestigationDataService(mock_cache_aside_service)

@pytest.mark.asyncio
async def test_concurrent_chat_appends_preserve_chain_under_load(service, mock_cache_aside_service):
    """
    REPRODUCER: Spin 20 coroutines via asyncio.gather calling add_chat_message
    against the same investigation.
    """
    investigation_id = "inv-concurrent-test"
    # Initial investigation state
    initial_inv = InvestigationModel(
        id=investigation_id,
        case_id="case-1",
        user_id="user-1",
        sentinel_mode=True
    )
    
    created_at = initial_inv.created_at.isoformat()
    
    # Use a shared object to represent the "database" state.
    shared_db_state = [initial_inv.model_dump(mode="json")]
    
    async def mock_get_document(collection, document_id):
        await asyncio.sleep(0.01)
        # Return a copy to simulate a fresh read from the "database"
        return shared_db_state[0].copy()

    async def mock_update_document(collection, document_id, data, merge=True):
        await asyncio.sleep(0.01)
        if "conversation_history" in data:
            new_state = shared_db_state[0].copy()
            new_state["conversation_history"] = data["conversation_history"]
            shared_db_state[0] = new_state
        return AsyncMock(success=True)

    async def mock_append_to_array(collection, document_id, array_field, items_to_add, additional_updates=None):
        await asyncio.sleep(0.01)
        if array_field == "conversation_history":
            # This mimics what happens if we don't have a lock
            new_state = shared_db_state[0].copy()
            current_history = new_state.get(array_field, [])
            new_state[array_field] = current_history + items_to_add
            shared_db_state[0] = new_state
        return AsyncMock(success=True)

    mock_cache_aside_service.get_document_with_cache.side_effect = mock_get_document
    mock_cache_aside_service.update_document.side_effect = mock_update_document
    mock_cache_aside_service.append_to_array.side_effect = mock_append_to_array

    # Launch 20 concurrent appends
    num_concurrent = 20
    tasks = []
    for i in range(num_concurrent):
        tasks.append(service.add_chat_message(
            investigation_id=investigation_id,
            sender="user.chat",
            content=f"Concurrent message {i}",
            metadata=ConversationMessageMetadata(event_type=EventType.INVESTIGATION_CHAT_MESSAGE_USER)
        ))

    await asyncio.gather(*tasks)

    history = shared_db_state[0].get("conversation_history", [])
    print(f"Final conversation history length: {len(history)}")
    
    assert len(history) == num_concurrent, f"Expected {num_concurrent} entries, but got {len(history)}. Race condition detected!"
    
    is_valid, bad_idx = verify_chain(
        entries=history,
        investigation_id=investigation_id,
        created_at=created_at
    )
    assert is_valid is True, f"Chain integrity check failed at index {bad_idx}"
