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
    InvestigationHistoryEntry,
)


def test_conversation_history_message_with_hash_fields():
    """ConversationHistoryMessage accepts and serializes prev_hash and entry_hash."""
    message = ConversationHistoryMessage(
        sender="user.chat",
        content="Test message",
        prev_hash="a" * 64,
        entry_hash="b" * 64,
    )
    
    assert message.prev_hash == "a" * 64
    assert message.entry_hash == "b" * 64
    
    # Test serialization
    dumped = message.model_dump(mode="json")
    assert "prev_hash" in dumped
    assert "entry_hash" in dumped
    assert dumped["prev_hash"] == "a" * 64
    assert dumped["entry_hash"] == "b" * 64


def test_conversation_history_message_without_hash_fields():
    """ConversationHistoryMessage requires prev_hash and entry_hash (no backward compat in ephemeral architecture)."""
    import pytest
    with pytest.raises(ValueError, match="prev_hash|entry_hash"):
        ConversationHistoryMessage(
            sender="user.chat",
            content="Test message",
        )


def test_conversation_history_message_round_trip_with_hashes():
    """ConversationHistoryMessage round-trips correctly with hash fields."""
    original = ConversationHistoryMessage(
        sender="ai.primary",
        content="AI response",
        metadata=ConversationMessageMetadata(event_type=EventType.INVESTIGATION_CHAT_MESSAGE_AI),
        prev_hash="c" * 64,
        entry_hash="d" * 64,
    )
    
    dumped = original.model_dump(mode="json")
    loaded = ConversationHistoryMessage.model_validate(dumped)
    
    assert loaded.sender == original.sender
    assert loaded.content == original.content
    assert loaded.prev_hash == original.prev_hash
    assert loaded.entry_hash == original.entry_hash
    assert loaded.metadata.event_type == original.metadata.event_type


def test_conversation_history_message_round_trip_without_hashes():
    """ConversationHistoryMessage requires hash fields - old data without hashes fails validation."""
    import pytest
    original_data = {
        "id": "test-id",
        "sender": "user.chat",
        "content": "Old message",
        "timestamp": "2024-01-01T00:00:00Z",
        "metadata": {},
    }
    
    with pytest.raises(ValueError, match="prev_hash|entry_hash"):
        ConversationHistoryMessage.model_validate(original_data)


def test_investigation_history_entry_with_hash_fields():
    """InvestigationHistoryEntry accepts and serializes prev_hash and entry_hash."""
    entry = InvestigationHistoryEntry(
        attempt_number=1,
        event_type=EventType.INVESTIGATION_CREATED,
        actor=ComponentName.G8EE,
        summary="Test entry",
        prev_hash="e" * 64,
        entry_hash="f" * 64,
    )
    
    assert entry.prev_hash == "e" * 64
    assert entry.entry_hash == "f" * 64
    
    # Test serialization
    dumped = entry.model_dump(mode="json")
    assert "prev_hash" in dumped
    assert "entry_hash" in dumped
    assert dumped["prev_hash"] == "e" * 64
    assert dumped["entry_hash"] == "f" * 64


def test_investigation_history_entry_without_hash_fields():
    """InvestigationHistoryEntry requires prev_hash and entry_hash (no backward compat in ephemeral architecture)."""
    import pytest
    with pytest.raises(ValueError, match="prev_hash|entry_hash"):
        InvestigationHistoryEntry(
            attempt_number=1,
            event_type=EventType.INVESTIGATION_CREATED,
            actor=ComponentName.G8EE,
            summary="Test entry",
        )


def test_investigation_history_entry_round_trip_with_hashes():
    """InvestigationHistoryEntry round-trips correctly with hash fields."""
    original = InvestigationHistoryEntry(
        attempt_number=2,
        event_type=EventType.INVESTIGATION_CREATED,
        actor=ComponentName.G8EE,
        summary="Test entry",
        details=ConversationMessageMetadata(),
        prev_hash="g" * 64,
        entry_hash="h" * 64,
    )
    
    dumped = original.model_dump(mode="json")
    loaded = InvestigationHistoryEntry.model_validate(dumped)
    
    assert loaded.attempt_number == original.attempt_number
    assert loaded.event_type == original.event_type
    assert loaded.actor == original.actor
    assert loaded.summary == original.summary
    assert loaded.prev_hash == original.prev_hash
    assert loaded.entry_hash == original.entry_hash


def test_investigation_history_entry_round_trip_without_hashes():
    """InvestigationHistoryEntry requires hash fields - old data without hashes fails validation."""
    import pytest
    original_data = {
        "attempt_number": 1,
        "timestamp": "2024-01-01T00:00:00Z",
        "event_type": EventType.INVESTIGATION_CREATED.value,
        "actor": ComponentName.G8EE.value,
        "summary": "Old entry",
        "details": {},
    }
    
    with pytest.raises(ValueError, match="prev_hash|entry_hash"):
        InvestigationHistoryEntry.model_validate(original_data)


def test_hash_field_validation_length():
    """Hash fields accept 64-character hex strings."""
    valid_hash = "a" * 64
    
    message = ConversationHistoryMessage(
        sender="user.chat",
        content="Test",
        prev_hash=valid_hash,
        entry_hash=valid_hash,
    )
    
    assert len(message.prev_hash) == 64
    assert len(message.entry_hash) == 64


def test_hash_field_accepts_none():
    """Hash fields do not accept None (no backward compat in ephemeral architecture)."""
    import pytest
    with pytest.raises(ValueError):
        ConversationHistoryMessage(
            sender="user.chat",
            content="Test",
            prev_hash=None,
            entry_hash=None,
        )
