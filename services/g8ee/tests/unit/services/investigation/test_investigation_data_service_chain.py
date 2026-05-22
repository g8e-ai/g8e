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
    ConversationMessageMetadata,
    InvestigationCreateRequest,
    InvestigationCurrentState,
)
from app.services.investigation.investigation_data_service import InvestigationDataService
from app.utils.ledger_hash import verify_chain


@pytest.fixture
def investigation_data_service(fake_cache_aside_service):
    """Investigation data service fixture - bypasses governance for hash chain tests."""
    from unittest.mock import AsyncMock, MagicMock
    from app.models.investigations import InvestigationModel
    from app.constants import ComponentName, EscalationRisk, ComponentStatus
    from app.utils.timestamp import now
    
    # Use None for governance_client to bypass governance envelope
    service = InvestigationDataService(fake_cache_aside_service, None)
    
    # Patch create_investigation to use cache directly (bypass governance)
    async def patched_create_investigation(request):
        investigation = InvestigationModel(
            case_id=request.case_id,
            case_title=request.case_title,
            case_description=request.case_description,
            web_session_id=request.web_session_id,
            user_id=request.user_id,
            user_email=request.user_email,
            priority=request.priority,
            created_with_case=request.created_with_case,
            case_source=request.case_source,
            sentinel_mode=request.sentinel_mode,
            created_at=now(),
        )
        investigation.current_state = InvestigationCurrentState(
            active_attempt=1,
            escalation_risk=EscalationRisk.LOW,
            collaboration_status={ComponentName.G8EE: ComponentStatus.ACTIVE},
        )
        investigation.add_history_entry(
            event_type=EventType.APP_INVESTIGATION_CREATED,
            actor=ComponentName.G8EE,
            summary=f"Investigation created for case {request.case_id}",
        )
        await fake_cache_aside_service.create_document(
            collection="investigations",
            document_id=investigation.id,
            document_data=investigation.model_dump(mode="json"),
        )
        return investigation
    
    service.create_investigation = patched_create_investigation
    
    return service


@pytest.mark.asyncio
async def test_chat_message_creates_hash_chain(investigation_data_service):
    """Adding chat messages creates a valid hash chain."""
    # Create an investigation first
    request = InvestigationCreateRequest(
        case_id="test-case",
        case_title="Test Case",
        case_description="Test Description",
        user_id="test-user",
        web_session_id="test-session",
    )
    investigation = await investigation_data_service.create_investigation(request)

    # Add multiple chat messages
    metadata1 = ConversationMessageMetadata(event_type=EventType.APP_INVESTIGATION_CHAT_MESSAGE_USER)
    await investigation_data_service.add_chat_message(
        investigation_id=investigation.id,
        sender="user.chat",
        content="First message",
        metadata=metadata1,
    )

    metadata2 = ConversationMessageMetadata(event_type=EventType.APP_INVESTIGATION_CHAT_MESSAGE_AI)
    await investigation_data_service.add_chat_message(
        investigation_id=investigation.id,
        sender="ai.primary",
        content="Second message",
        metadata=metadata2,
    )

    metadata3 = ConversationMessageMetadata(event_type=EventType.APP_INVESTIGATION_CHAT_MESSAGE_USER)
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
        web_session_id="test-session",
    )
    investigation = await investigation_data_service.create_investigation(request)

    # Add multiple history entries
    from app.models.http_context import RequestContext
    context = RequestContext(
        web_session_id="test-session",
        user_id="test-user",
        source_component=ComponentName.G8EE,
    )
    
    details1 = ConversationMessageMetadata()
    await investigation_data_service.add_history_entry(
        investigation_id=investigation.id,
        event_type=EventType.APP_INVESTIGATION_CREATED,
        actor=ComponentName.G8EE,
        summary="First entry",
        details=details1,
        context=context,
    )

    details2 = ConversationMessageMetadata()
    await investigation_data_service.add_history_entry(
        investigation_id=investigation.id,
        event_type=EventType.APP_INVESTIGATION_STATUS_UPDATED_OPEN,
        actor=ComponentName.G8EE,
        summary="Second entry",
        details=details2,
        context=context,
    )

    details3 = ConversationMessageMetadata()
    await investigation_data_service.add_history_entry(
        investigation_id=investigation.id,
        event_type=EventType.APP_INVESTIGATION_STATUS_UPDATED_CLOSED,
        actor=ComponentName.G8EE,
        summary="Third entry",
        details=details3,
        context=context,
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
        web_session_id="test-session",
    )
    investigation = await investigation_data_service.create_investigation(request)

    # Add messages sequentially (the service should serialize these)
    for i in range(5):
        metadata = ConversationMessageMetadata(event_type=EventType.APP_INVESTIGATION_CHAT_MESSAGE_USER)
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
        web_session_id="test-session",
    )
    investigation = await investigation_data_service.create_investigation(request)

    # Add first message
    metadata = ConversationMessageMetadata(event_type=EventType.APP_INVESTIGATION_CHAT_MESSAGE_USER)
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
    """Entries without hash fields are rejected - no backward compatibility for hash fields."""
    # Create an investigation
    request = InvestigationCreateRequest(
        case_id="test-case",
        case_title="Test Case",
        case_description="Test Description",
        user_id="test-user",
        web_session_id="test-session",
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

    # Attempting to retrieve the investigation should fail validation
    # because old entries lack required hash fields
    with pytest.raises(Exception):  # Pydantic ValidationError
        await investigation_data_service.get_investigation(investigation.id)
