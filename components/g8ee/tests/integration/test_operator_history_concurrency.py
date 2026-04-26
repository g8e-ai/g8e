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

from app.constants.status import ComponentName, OperatorHistoryEventType, OperatorStatus
from app.models.operators import OperatorDocument
from app.services.operator.operator_data_service import OperatorDataService
from app.models.cache import CacheOperationResult
from app.utils.ledger_hash import verify_chain

@pytest.fixture
def mock_cache_aside_service():
    return AsyncMock()

@pytest.fixture
def mock_http_client():
    return AsyncMock()

@pytest.fixture
def service(mock_cache_aside_service, mock_http_client):
    return OperatorDataService(mock_cache_aside_service, mock_http_client)

@pytest.mark.asyncio
async def test_concurrent_appends_preserve_chain_under_load(service, mock_cache_aside_service):
    """
    REPRODUCER: Spin 20 coroutines via asyncio.gather calling add_history_entry 
    against the same operator.
    
    Without a lock, this test should reliably produce a corrupted or incomplete chain
    because of the Read-Modify-Write race.
    """
    operator_id = "op-concurrent-test"
    # Initial operator state
    initial_operator = OperatorDocument(
        id=operator_id,
        user_id="user-1",
        status=OperatorStatus.AVAILABLE
    )
    
    created_at = initial_operator.created_at.isoformat()
    
    # Use a list to store the state. Non-lock version will read and replace the whole list.
    # This simulates concurrent updates where the last write wins.
    shared_db_state = [initial_operator.model_dump(mode="json")]
    
    async def mock_get_document(collection, doc_id):
        # Simulate network delay to increase race window
        await asyncio.sleep(0.01)
        # Return a copy to simulate a fresh read from the "database"
        return shared_db_state[0].copy()

    async def mock_update_document(collection, document_id, data, merge=True):
        # Simulate network delay
        await asyncio.sleep(0.01)
        if "history_trail" in data:
            # Atomic update of the shared state - simulated by overwriting the list entry.
            # Without a lock, two tasks will both read state X, both update it to X+A and X+B,
            # and the last one to write will win.
            new_state = shared_db_state[0].copy()
            new_state["history_trail"] = data["history_trail"]
            shared_db_state[0] = new_state
        return CacheOperationResult(success=True)

    mock_cache_aside_service.get_document_with_cache.side_effect = mock_get_document
    mock_cache_aside_service.update_document.side_effect = mock_update_document

    # Launch 20 concurrent appends
    num_concurrent = 20
    tasks = []
    for i in range(num_concurrent):
        tasks.append(service.add_history_entry(
            operator_id=operator_id,
            event_type=OperatorHistoryEventType.STATUS_CHANGED,
            summary=f"Concurrent entry {i}",
            actor=ComponentName.G8EE
        ))

    await asyncio.gather(*tasks)

    # Verify the results
    history = shared_db_state[0].get("history_trail", [])
    
    # WITHOUT THE LOCK, this will likely be < num_concurrent
    print(f"Final history length: {len(history)}")
    
    # We expect all entries to be present if serialized correctly
    assert len(history) == num_concurrent, f"Expected {num_concurrent} entries, but got {len(history)}. Race condition detected!"
    
    # Verify the chain integrity
    is_valid, bad_idx = verify_chain(
        entries=history,
        investigation_id=operator_id,
        created_at=created_at
    )
    assert is_valid is True, f"Chain integrity check failed at index {bad_idx}"
