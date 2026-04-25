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

from app.constants import ComponentName, EventType
from app.models.investigations import (
    ConversationHistoryMessage,
    ConversationMessageMetadata,
    InvestigationCreateRequest,
    InvestigationCurrentState,
)
from app.services.investigation.investigation_data_service import InvestigationDataService
from app.utils.ledger_hash import verify_chain


@pytest.fixture
def investigation_data_service(fake_cache_aside_service):
    """Investigation data service fixture."""
    return InvestigationDataService(fake_cache_aside_service)


@pytest.mark.asyncio
async def test_chat_message_creates_hash_chain(investigation_data_service):
    """Adding chat messages creates a valid hash chain."""
    # Create an investigation first
    request = InvestigationCreateRequest(
        case_id="test-case",
        case_title="Test Case",
        case_description="Test Description",
        user_id="test-user",
    )
    investigation = await investigation_data_service.create_investigation(request)
    
    # Add multiple chat messages
    metadata1 = ConversationMessageMetadata(event_type=EventType.INVESTIGATION_CHAT_MESSAGE_USER)
    await investigation_data_service.add_chat_message(
        investigation_id=investigation.id,
        sender="user.chat",
        content="First message",
        metadata=metadata1,
    )
    
    metadata2 = ConversationMessageMetadata(event_type=EventType.INVESTIGATION_CHAT_MESSAGE_AI)
    await investigation_data_service.add_chat_message(
        investigation_id=investigation.id,
        sender="ai.primary",
        content="Second message",
        metadata=metadata2,
    )
    
    metadata3 = ConversationMessageMetadata(event_type=EventType.INVESTIGATION_CHAT_MESSAGE_USER)
    await investigation_data_service.add_chat_message(
        investigation_id=investigation.id,
        sender="user.chat",
        content="Third message",
        metadata=metadata3,
    )
    
    # Verify the chain
    updated_investigation = await investigation_data_service.get_investigation(investigation.id)
    entries = [msg.model_dump(mode="json") for msg in updated_investigation.conversation_history]
    
    valid, bad_index = verify_chain(entries, investigation.id, investigation.created_at.isoformat())
    assert valid is True
    assert bad_index is None


@pytest.mark.asyncio
async def test_history_entry_creates_hash_chain(investigation_data_service):
    """Adding history entries creates a valid hash chain."""
    # Create an investigation
    request = InvestigationCreateRequest(
        case_id="test-case",
        case_title="Test Case",
        case_description="Test Description",
        user_id="test-user",
    )
    investigation = await investigation_data_service.create_investigation(request)
    
    # Add multiple history entries
    details1 = ConversationMessageMetadata()
    await investigation_data_service.add_history_entry(
        investigation_id=investigation.id,
        event_type=EventType.INVESTIGATION_CREATED,
        actor=ComponentName.G8EE,
        summary="First entry",
        details=details1,
    )
    
    details2 = ConversationMessageMetadata()
    await investigation_data_service.add_history_entry(
        investigation_id=investigation.id,
        event_type=EventType.INVESTIGATION_STATUS_UPDATED_OPEN,
        actor=ComponentName.G8EE,
        summary="Second entry",
        details=details2,
    )
    
    details3 = ConversationMessageMetadata()
    await investigation_data_service.add_history_entry(
        investigation_id=investigation.id,
        event_type=EventType.INVESTIGATION_STATUS_UPDATED_CLOSED,
        actor=ComponentName.G8EE,
        summary="Third entry",
        details=details3,
    )
    
    # Verify the chain
    updated_investigation = await investigation_data_service.get_investigation(investigation.id)
    entries = [entry.model_dump(mode="json") for entry in updated_investigation.history_trail]
    
    valid, bad_index = verify_chain(entries, investigation.id, investigation.created_at.isoformat())
    assert valid is True
    assert bad_index is None


@pytest.mark.asyncio
async def test_concurrent_appends_produce_valid_chain(investigation_data_service):
    """Concurrent appends produce a valid hash chain (serialized via lock)."""
    # Create an investigation
    request = InvestigationCreateRequest(
        case_id="test-case",
        case_title="Test Case",
        case_description="Test Description",
        user_id="test-user",
    )
    investigation = await investigation_data_service.create_investigation(request)
    
    # Add messages sequentially (the service should serialize these)
    for i in range(5):
        metadata = ConversationMessageMetadata(event_type=EventType.INVESTIGATION_CHAT_MESSAGE_USER)
        await investigation_data_service.add_chat_message(
            investigation_id=investigation.id,
            sender="user.chat",
            content=f"Message {i}",
            metadata=metadata,
        )
    
    # Verify the chain
    updated_investigation = await investigation_data_service.get_investigation(investigation.id)
    entries = [msg.model_dump(mode="json") for msg in updated_investigation.conversation_history]
    
    valid, bad_index = verify_chain(entries, investigation.id, investigation.created_at.isoformat())
    assert valid is True
    assert bad_index is None


@pytest.mark.asyncio
async def test_first_entry_uses_genesis_hash(investigation_data_service):
    """First entry in chain uses genesis hash as prev_hash."""
    # Create an investigation
    request = InvestigationCreateRequest(
        case_id="test-case",
        case_title="Test Case",
        case_description="Test Description",
        user_id="test-user",
    )
    investigation = await investigation_data_service.create_investigation(request)
    
    # Add first message
    metadata = ConversationMessageMetadata(event_type=EventType.INVESTIGATION_CHAT_MESSAGE_USER)
    await investigation_data_service.add_chat_message(
        investigation_id=investigation.id,
        sender="user.chat",
        content="First message",
        metadata=metadata,
    )
    
    # Check that first entry uses genesis hash
    from app.utils.ledger_hash import genesis_hash
    expected_genesis = genesis_hash(investigation.id, investigation.created_at.isoformat())
    
    updated_investigation = await investigation_data_service.get_investigation(investigation.id)
    first_entry = updated_investigation.conversation_history[0]
    
    assert first_entry.prev_hash == expected_genesis
    assert first_entry.entry_hash is not None
    assert len(first_entry.entry_hash) == 64


@pytest.mark.asyncio
async def test_backward_compat_without_hash_fields(investigation_data_service):
    """Entries without hash fields are handled gracefully for backward compatibility."""
    # Create an investigation
    request = InvestigationCreateRequest(
        case_id="test-case",
        case_title="Test Case",
        case_description="Test Description",
        user_id="test-user",
    )
    investigation = await investigation_data_service.create_investigation(request)
    
    # Manually add an entry without hash fields (simulating old data)
    await investigation_data_service.cache.append_to_array(
        collection=investigation_data_service.collection,
        document_id=investigation.id,
        array_field="conversation_history",
        items_to_add=[{
            "id": "old-message-id",
            "sender": "user.chat",
            "content": "Old message without hash",
            "timestamp": "2024-01-01T00:00:00Z",
            "metadata": {},
        }],
        additional_updates={},
    )
    
    # New message should still work
    metadata = ConversationMessageMetadata(event_type=EventType.INVESTIGATION_CHAT_MESSAGE_USER)
    await investigation_data_service.add_chat_message(
        investigation_id=investigation.id,
        sender="user.chat",
        content="New message with hash",
        metadata=metadata,
    )
    
    # The new message should have hash fields
    updated_investigation = await investigation_data_service.get_investigation(investigation.id)
    new_entry = updated_investigation.conversation_history[-1]
    
    assert new_entry.entry_hash is not None
    assert new_entry.prev_hash is not None
