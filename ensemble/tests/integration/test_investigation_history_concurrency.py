# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

import asyncio
from unittest.mock import AsyncMock

import pytest

from app.constants import EventType
from app.models.investigations import (
    ConversationMessageMetadata,
    InvestigationModel,
)
from app.services.investigation.investigation_data_service import InvestigationDataService
from app.utils.ledger_hash import verify_chain


@pytest.fixture
def mock_cache_aside_service():
    return AsyncMock()


@pytest.fixture
def mock_governance_client():
    return AsyncMock()


@pytest.fixture
def service(mock_cache_aside_service, mock_governance_client):
    return InvestigationDataService(mock_cache_aside_service, mock_governance_client)


@pytest.mark.asyncio
async def test_concurrent_chat_appends_preserve_chain_under_load(
    service, mock_cache_aside_service, mock_governance_client
):
    """
    REPRODUCER: Spin 20 coroutines via asyncio.gather calling add_chat_message
    against the same investigation.
    """
    investigation_id = "inv-concurrent-test"
    # Initial investigation state
    initial_inv = InvestigationModel(
        id=investigation_id, case_id="case-1", user_id="user-1", sentinel_mode=True
    )

    created_at = initial_inv.created_at.isoformat()

    # Use a protocol object to represent the "database" state.
    protocol_db_state = [initial_inv.model_dump(mode="json")]

    async def mock_get_document(collection, document_id):
        await asyncio.sleep(0.01)
        # Return a copy to simulate a fresh read from the "database"
        return protocol_db_state[0].copy()

    async def mock_update_governed_doc(**kwargs):
        # add_chat_message routes writes through the governance envelope as a
        # DOCUMENT_UPDATE with merge=true; mirror the persisted state here so
        # the next read observes the appended message.
        await asyncio.sleep(0.01)
        updates = kwargs.get("updates", {})
        if "conversation_history" in updates:
            new_state = protocol_db_state[0].copy()
            new_state["conversation_history"] = updates["conversation_history"]
            protocol_db_state[0] = new_state
        return AsyncMock(success=True)

    mock_cache_aside_service.get_document_with_cache.side_effect = mock_get_document
    mock_governance_client.update_governed_doc.side_effect = mock_update_governed_doc

    # Launch 20 concurrent appends
    num_concurrent = 20
    tasks = []
    for i in range(num_concurrent):
        tasks.append(
            service.add_chat_message(
                investigation_id=investigation_id,
                sender="user.chat",
                content=f"Concurrent message {i}",
                metadata=ConversationMessageMetadata(
                    event_type=EventType.APP_INVESTIGATION_CHAT_MESSAGE_USER
                ),
            )
        )

    await asyncio.gather(*tasks)

    history = protocol_db_state[0].get("conversation_history", [])

    assert len(history) == num_concurrent, (
        f"Expected {num_concurrent} entries, but got {len(history)}. Race condition detected!"
    )

    is_valid, bad_idx = verify_chain(
        entries=history, investigation_id=investigation_id, created_at=created_at
    )
    assert is_valid is True, f"Chain integrity check failed at index {bad_idx}"
