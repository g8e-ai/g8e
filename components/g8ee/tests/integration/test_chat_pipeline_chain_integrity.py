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

import pytest

from app.constants import EventType
from app.models.investigations import (
    ConversationMessageMetadata,
    InvestigationCreateRequest,
)
from app.services.investigation.investigation_data_service import InvestigationDataService
from app.utils.ledger_hash import verify_chain


@pytest.mark.asyncio
async def test_full_chat_turn_produces_valid_chain(fake_cache_aside_service):
    """A full chat turn produces a valid hash chain."""
    investigation_data_service = InvestigationDataService(fake_cache_aside_service)
    
    # Create investigation
    request = InvestigationCreateRequest(
        case_id="test-case",
        case_title="Test Case",
        case_description="Test Description",
        user_id="test-user",
    )
    investigation = await investigation_data_service.create_investigation(request)
    
    # Simulate a chat turn: user message + AI response + system notification
    user_metadata = ConversationMessageMetadata(event_type=EventType.INVESTIGATION_CHAT_MESSAGE_USER)
    await investigation_data_service.add_chat_message(
        investigation_id=investigation.id,
        sender="user.chat",
        content="Help me debug this issue",
        metadata=user_metadata,
    )
    
    ai_metadata = ConversationMessageMetadata(
        event_type=EventType.INVESTIGATION_CHAT_MESSAGE_AI,
        model="gpt-4",
        tokens=100,
    )
    await investigation_data_service.add_chat_message(
        investigation_id=investigation.id,
        sender="ai.primary",
        content="I'll help you debug. Let me check the logs.",
        metadata=ai_metadata,
    )
    
    system_metadata = ConversationMessageMetadata(event_type=EventType.INVESTIGATION_STATUS_UPDATED_OPEN)
    await investigation_data_service.add_chat_message(
        investigation_id=investigation.id,
        sender="system",
        content="Investigation status updated",
        metadata=system_metadata,
    )
    
    # Verify the chain
    updated_investigation = await investigation_data_service.get_investigation(investigation.id)
    entries = [msg.model_dump(mode="json") for msg in updated_investigation.conversation_history]
    
    valid, bad_index = verify_chain(entries, investigation.id, investigation.created_at.isoformat())
    assert valid is True, f"Chain validation failed at index {bad_index}"
    assert bad_index is None


@pytest.mark.asyncio
async def test_mixed_history_and_chat_chains(fake_cache_aside_service):
    """conversation_history maintains valid chain when history_trail is also updated."""
    investigation_data_service = InvestigationDataService(fake_cache_aside_service)
    
    # Create investigation
    request = InvestigationCreateRequest(
        case_id="test-case",
        case_title="Test Case",
        case_description="Test Description",
        user_id="test-user",
    )
    investigation = await investigation_data_service.create_investigation(request)
    
    # Add chat message
    chat_metadata = ConversationMessageMetadata(event_type=EventType.INVESTIGATION_CHAT_MESSAGE_USER)
    await investigation_data_service.add_chat_message(
        investigation_id=investigation.id,
        sender="user.chat",
        content="User message",
        metadata=chat_metadata,
    )
    
    # Add history entry
    from app.constants import ComponentName
    history_details = ConversationMessageMetadata()
    await investigation_data_service.add_history_entry(
        investigation_id=investigation.id,
        event_type=EventType.INVESTIGATION_CREATED,
        actor=ComponentName.G8EE,
        summary="History entry",
        details=history_details,
    )
    
    # Add another chat message
    await investigation_data_service.add_chat_message(
        investigation_id=investigation.id,
        sender="ai.primary",
        content="AI response",
        metadata=ConversationMessageMetadata(event_type=EventType.INVESTIGATION_CHAT_MESSAGE_AI),
    )
    
    # Verify conversation_history chain (primary ledger)
    updated_investigation = await investigation_data_service.get_investigation(investigation.id)
    
    chat_entries = [msg.model_dump(mode="json") for msg in updated_investigation.conversation_history]
    chat_valid, chat_bad_index = verify_chain(chat_entries, investigation.id, investigation.created_at.isoformat())
    assert chat_valid is True, f"Chat chain validation failed at index {chat_bad_index}"

    # Verify history_trail chain (independent ledger). Includes the genesis
    # INVESTIGATION_CREATED entry written during create_investigation.
    history_entries = [e.model_dump(mode="json") for e in updated_investigation.history_trail]
    assert len(history_entries) >= 2, "history_trail should contain creation entry plus added entry"
    history_valid, history_bad_index = verify_chain(history_entries, investigation.id, investigation.created_at.isoformat())
    assert history_valid is True, f"History trail chain validation failed at index {history_bad_index}"


@pytest.mark.asyncio
async def test_chain_persists_across_retrieval(fake_cache_aside_service):
    """Hash chain persists correctly across investigation retrieval."""
    investigation_data_service = InvestigationDataService(fake_cache_aside_service)
    
    # Create investigation and add messages
    request = InvestigationCreateRequest(
        case_id="test-case",
        case_title="Test Case",
        case_description="Test Description",
        user_id="test-user",
    )
    investigation = await investigation_data_service.create_investigation(request)
    
    for i in range(3):
        await investigation_data_service.add_chat_message(
            investigation_id=investigation.id,
            sender="user.chat",
            content=f"Message {i}",
            metadata=ConversationMessageMetadata(event_type=EventType.INVESTIGATION_CHAT_MESSAGE_USER),
        )
    
    # Retrieve and verify chain
    retrieved = await investigation_data_service.get_investigation(investigation.id)
    entries = [msg.model_dump(mode="json") for msg in retrieved.conversation_history]
    
    valid, bad_index = verify_chain(entries, investigation.id, investigation.created_at.isoformat())
    assert valid is True, f"Chain validation failed at index {bad_index}"
    
    # Retrieve again to ensure persistence
    retrieved_again = await investigation_data_service.get_investigation(investigation.id)
    entries_again = [msg.model_dump(mode="json") for msg in retrieved_again.conversation_history]
    
    valid_again, bad_index_again = verify_chain(entries_again, investigation.id, investigation.created_at.isoformat())
    assert valid_again is True, f"Chain validation failed on second retrieval at index {bad_index_again}"
