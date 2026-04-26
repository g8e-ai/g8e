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
from app.models.operators import OperatorDocument, OperatorHistoryEntry
from app.constants import ComponentName, OperatorHistoryEventType
from app.utils.ledger_hash import verify_chain, genesis_hash

def test_operator_history_chain_integrity():
    """Verify that OperatorDocument.add_history_entry produces a valid cryptographic chain."""
    operator = OperatorDocument(
        id="op-123",
        user_id="user-123",
        name="test-operator"
    )
    
    # 1. Add first entry (Genesis -> Entry 1)
    operator.add_history_entry(
        event_type=OperatorHistoryEventType.CREATED,
        summary="Operator created",
        actor=ComponentName.G8EE
    )
    
    assert len(operator.history_trail) == 1
    entry1 = operator.history_trail[0]
    expected_genesis = genesis_hash(operator.id, operator.created_at.isoformat())
    assert entry1.prev_hash == expected_genesis
    assert entry1.entry_hash is not None
    
    # 2. Add second entry (Entry 1 -> Entry 2)
    operator.add_history_entry(
        event_type=OperatorHistoryEventType.AUTHENTICATED,
        summary="Operator authenticated",
        actor=ComponentName.G8EE
    )
    
    assert len(operator.history_trail) == 2
    entry2 = operator.history_trail[1]
    assert entry2.prev_hash == entry1.entry_hash
    
    # 3. Verify the whole chain using the utility
    entries_as_dicts = [e.model_dump(mode="json") for e in operator.history_trail]
    is_valid, bad_idx = verify_chain(
        entries=entries_as_dicts,
        investigation_id=operator.id,
        created_at=operator.created_at.isoformat()
    )
    
    assert is_valid is True
    assert bad_idx is None

def test_operator_history_chain_mutation_fails():
    """Verify that mutating an entry in the middle of the chain invalidates it."""
    operator = OperatorDocument(
        id="op-123",
        user_id="user-123",
        name="test-operator"
    )
    
    operator.add_history_entry(OperatorHistoryEventType.CREATED, "Entry 1")
    operator.add_history_entry(OperatorHistoryEventType.AUTHENTICATED, "Entry 2")
    operator.add_history_entry(OperatorHistoryEventType.STATUS_CHANGED, "Entry 3")
    
    entries_as_dicts = [e.model_dump(mode="json") for e in operator.history_trail]
    
    # Mutate Entry 2 summary
    entries_as_dicts[1]["summary"] = "MUTATED"
    
    is_valid, bad_idx = verify_chain(
        entries=entries_as_dicts,
        investigation_id=operator.id,
        created_at=operator.created_at.isoformat()
    )
    
    assert is_valid is False
    assert bad_idx == 1 # First bad entry is the mutated one

def test_operator_history_chain_reorder_fails():
    """Verify that reordering entries invalidates the chain."""
    operator = OperatorDocument(
        id="op-123",
        user_id="user-123",
        name="test-operator"
    )
    
    operator.add_history_entry(OperatorHistoryEventType.CREATED, "Entry 1")
    operator.add_history_entry(OperatorHistoryEventType.AUTHENTICATED, "Entry 2")
    
    entries_as_dicts = [e.model_dump(mode="json") for e in operator.history_trail]
    
    # Swap entries
    entries_as_dicts[0], entries_as_dicts[1] = entries_as_dicts[1], entries_as_dicts[0]
    
    is_valid, bad_idx = verify_chain(
        entries=entries_as_dicts,
        investigation_id=operator.id,
        created_at=operator.created_at.isoformat()
    )
    
    assert is_valid is False
    assert bad_idx == 0 # First entry fails because its prev_hash won't match genesis

def test_operator_history_chain_prev_hash_mutation_fails():
    """Verify that mutating a prev_hash invalidates the chain."""
    operator = OperatorDocument(
        id="op-123",
        user_id="user-123",
        name="test-operator"
    )
    
    operator.add_history_entry(OperatorHistoryEventType.CREATED, "Entry 1")
    operator.add_history_entry(OperatorHistoryEventType.AUTHENTICATED, "Entry 2")
    
    entries_as_dicts = [e.model_dump(mode="json") for e in operator.history_trail]
    
    # Mutate Entry 2 prev_hash
    entries_as_dicts[1]["prev_hash"] = "0" * 64
    
    is_valid, bad_idx = verify_chain(
        entries=entries_as_dicts,
        investigation_id=operator.id,
        created_at=operator.created_at.isoformat()
    )
    
    assert is_valid is False
    assert bad_idx == 1
