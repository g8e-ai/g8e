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

import json

import pytest

from app.utils.ledger_hash import (
    canonical_json,
    compute_entry_hash,
    genesis_hash,
    verify_chain,
)


def test_canonical_json_stability():
    """canonical_json produces deterministic output regardless of key order."""
    obj1 = {"z": 1, "a": 2, "m": 3}
    obj2 = {"a": 2, "z": 1, "m": 3}
    obj3 = {"m": 3, "a": 2, "z": 1}
    
    result1 = canonical_json(obj1)
    result2 = canonical_json(obj2)
    result3 = canonical_json(obj3)
    
    assert result1 == result2 == result3
    
    # Verify it's sorted keys, no whitespace
    decoded = json.loads(result1)
    assert list(decoded.keys()) == ["a", "m", "z"]


def test_canonical_json_no_whitespace():
    """canonical_json produces compact JSON with no whitespace."""
    obj = {"a": 1, "b": {"c": 2, "d": [3, 4]}}
    result = canonical_json(obj)
    
    # No spaces, no newlines
    assert b" " not in result
    assert b"\n" not in result
    assert result == b'{"a":1,"b":{"c":2,"d":[3,4]}}'


def test_canonical_json_utf8():
    """canonical_json produces UTF-8 bytes."""
    obj = {"test": "value"}
    result = canonical_json(obj)
    assert isinstance(result, bytes)
    assert result.decode('utf-8') == '{"test":"value"}'


def test_genesis_hash_deterministic():
    """genesis_hash produces deterministic hash from investigation_id and created_at."""
    investigation_id = "test-investigation-123"
    created_at = "2024-01-01T00:00:00Z"
    
    hash1 = genesis_hash(investigation_id, created_at)
    hash2 = genesis_hash(investigation_id, created_at)
    
    assert hash1 == hash2
    assert len(hash1) == 64  # SHA256 hex string
    assert all(c in "0123456789abcdef" for c in hash1)


def test_genesis_hash_different_inputs():
    """genesis_hash produces different hashes for different inputs."""
    hash1 = genesis_hash("inv-1", "2024-01-01T00:00:00Z")
    hash2 = genesis_hash("inv-2", "2024-01-01T00:00:00Z")
    hash3 = genesis_hash("inv-1", "2024-01-02T00:00:00Z")
    
    assert hash1 != hash2
    assert hash1 != hash3
    assert hash2 != hash3


def test_compute_entry_hash():
    """compute_entry_hash produces hash from entry and prev_hash."""
    entry = {"sender": "user.chat", "content": "test", "timestamp": "2024-01-01T00:00:00Z"}
    prev_hash = "a" * 64
    
    hash1 = compute_entry_hash(entry, prev_hash)
    hash2 = compute_entry_hash(entry, prev_hash)
    
    assert hash1 == hash2
    assert len(hash1) == 64
    assert hash1 != prev_hash  # Hash should include entry content


def test_compute_entry_hash_without_prev():
    """compute_entry_hash works with None prev_hash (genesis case)."""
    entry = {"sender": "user.chat", "content": "test", "timestamp": "2024-01-01T00:00:00Z"}
    
    hash1 = compute_entry_hash(entry, None)
    hash2 = compute_entry_hash(entry, None)
    
    assert hash1 == hash2
    assert len(hash1) == 64


def test_compute_entry_hash_excludes_hash_fields():
    """compute_entry_hash excludes prev_hash and entry_hash from computation."""
    entry = {
        "sender": "user.chat",
        "content": "test",
        "timestamp": "2024-01-01T00:00:00Z",
        "prev_hash": "a" * 64,
        "entry_hash": "b" * 64,
    }
    prev_hash = "c" * 64
    
    hash1 = compute_entry_hash(entry, prev_hash)
    hash2 = compute_entry_hash(entry, prev_hash)
    
    # Changing the hash fields shouldn't affect the result
    entry["prev_hash"] = "d" * 64
    entry["entry_hash"] = "e" * 64
    hash3 = compute_entry_hash(entry, prev_hash)
    
    assert hash1 == hash2 == hash3


def test_verify_chain_valid_chain():
    """verify_chain returns True for a valid hash chain."""
    investigation_id = "test-inv"
    created_at = "2024-01-01T00:00:00Z"
    
    prev_hash = genesis_hash(investigation_id, created_at)
    entries = []
    
    for i in range(3):
        entry = {
            "id": f"msg-{i}",
            "sender": "user.chat",
            "content": f"message {i}",
            "timestamp": f"2024-01-01T00:0{i}:00Z",
            "prev_hash": prev_hash,
        }
        entry_hash = compute_entry_hash(entry, prev_hash)
        entry["entry_hash"] = entry_hash
        entries.append(entry)
        prev_hash = entry_hash
    
    valid, bad_index = verify_chain(entries, investigation_id, created_at)
    assert valid is True
    assert bad_index is None


def test_verify_chain_tampered_entry():
    """verify_chain detects tampered entry."""
    investigation_id = "test-inv"
    created_at = "2024-01-01T00:00:00Z"
    
    prev_hash = genesis_hash(investigation_id, created_at)
    entries = []
    
    for i in range(3):
        entry = {
            "id": f"msg-{i}",
            "sender": "user.chat",
            "content": f"message {i}",
            "timestamp": f"2024-01-01T00:0{i}:00Z",
            "prev_hash": prev_hash,
        }
        entry_hash = compute_entry_hash(entry, prev_hash)
        entry["entry_hash"] = entry_hash
        entries.append(entry)
        prev_hash = entry_hash
    
    # Tamper with middle entry
    entries[1]["content"] = "tampered"
    
    valid, bad_index = verify_chain(entries, investigation_id, created_at)
    assert valid is False
    assert bad_index == 1


def test_verify_chain_broken_link():
    """verify_chain detects broken hash chain link."""
    investigation_id = "test-inv"
    created_at = "2024-01-01T00:00:00Z"
    
    prev_hash = genesis_hash(investigation_id, created_at)
    entries = []
    
    for i in range(3):
        entry = {
            "id": f"msg-{i}",
            "sender": "user.chat",
            "content": f"message {i}",
            "timestamp": f"2024-01-01T00:0{i}:00Z",
            "prev_hash": prev_hash,
        }
        entry_hash = compute_entry_hash(entry, prev_hash)
        entry["entry_hash"] = entry_hash
        entries.append(entry)
        prev_hash = entry_hash
    
    # Break the link
    entries[1]["prev_hash"] = "wrong" * 64
    
    valid, bad_index = verify_chain(entries, investigation_id, created_at)
    assert valid is False
    assert bad_index == 1


def test_verify_chain_empty():
    """verify_chain handles empty chain."""
    valid, bad_index = verify_chain([], "test-inv", "2024-01-01T00:00:00Z")
    assert valid is True
    assert bad_index is None


def test_verify_chain_single_entry():
    """verify_chain handles single entry chain."""
    investigation_id = "test-inv"
    created_at = "2024-01-01T00:00:00Z"
    
    prev_hash = genesis_hash(investigation_id, created_at)
    entry = {
        "id": "msg-0",
        "sender": "user.chat",
        "content": "message 0",
        "timestamp": "2024-01-01T00:00:00Z",
        "prev_hash": prev_hash,
    }
    entry_hash = compute_entry_hash(entry, prev_hash)
    entry["entry_hash"] = entry_hash
    
    valid, bad_index = verify_chain([entry], investigation_id, created_at)
    assert valid is True
    assert bad_index is None


def test_verify_chain_missing_hash_fields():
    """verify_chain handles entries without hash fields (backward compat)."""
    investigation_id = "test-inv"
    created_at = "2024-01-01T00:00:00Z"
    
    # Entry without hash fields (old format)
    entry = {
        "id": "msg-0",
        "sender": "user.chat",
        "content": "message 0",
        "timestamp": "2024-01-01T00:00:00Z",
    }
    
    # Should fail verification since hash fields are required for chain integrity
    valid, bad_index = verify_chain([entry], investigation_id, created_at)
    assert valid is False
