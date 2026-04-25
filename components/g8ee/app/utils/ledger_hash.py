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

import hashlib
import json
from datetime import datetime


def canonical_json(obj: dict) -> bytes:
    """Convert object to canonical JSON bytes (sorted keys, no whitespace, UTF-8).
    
    This ensures deterministic serialization for hash computation regardless
    of key order or formatting differences. Handles datetime objects by converting
    them to ISO format strings.
    
    Args:
        obj: Dictionary to serialize
        
    Returns:
        UTF-8 encoded JSON bytes with sorted keys and no whitespace
    """
    def json_serializer(obj):
        """Custom JSON serializer for datetime objects."""
        if isinstance(obj, datetime):
            return obj.isoformat()
        raise TypeError(f"Object of type {obj.__class__.__name__} is not JSON serializable")
    
    # Sort keys recursively, no whitespace, ensure_ascii=False for UTF-8
    return json.dumps(
        obj,
        sort_keys=True,
        separators=(",", ":"),
        ensure_ascii=False,
        default=json_serializer,
    ).encode("utf-8")


def compute_entry_hash(entry: dict, prev_hash: str | None) -> str:
    """Compute the hash for a ledger entry.
    
    The hash is computed from the entry content (excluding prev_hash and entry_hash
    fields themselves) concatenated with the previous entry's hash, forming a chain.
    
    Args:
        entry: Dictionary representing the ledger entry
        prev_hash: Hash of the previous entry in the chain (None for genesis)
        
    Returns:
        Hexadecimal SHA256 hash string (64 characters)
    """
    # Create a copy without the hash fields to avoid circular dependency
    entry_copy = {k: v for k, v in entry.items() if k not in ("prev_hash", "entry_hash")}
    
    hasher = hashlib.sha256()
    hasher.update(canonical_json(entry_copy))
    
    if prev_hash:
        hasher.update(prev_hash.encode("utf-8"))
    
    return hasher.hexdigest()


def genesis_hash(investigation_id: str, created_at: str) -> str:
    """Compute the genesis hash for a new investigation chain.
    
    The genesis hash is the starting point of the hash chain, derived from
    the investigation's identity and creation timestamp.
    
    Args:
        investigation_id: UUID of the investigation
        created_at: ISO 8601 timestamp string
        
    Returns:
        Hexadecimal SHA256 hash string (64 characters)
    """
    hasher = hashlib.sha256()
    hasher.update(f"{investigation_id}:{created_at}".encode("utf-8"))
    return hasher.hexdigest()


def verify_chain(
    entries: list[dict],
    investigation_id: str,
    created_at: str,
) -> tuple[bool, int | None]:
    """Verify the integrity of a hash chain.
    
    Checks that each entry's hash correctly chains to the previous entry,
    starting from the genesis hash.
    
    Args:
        entries: List of ledger entries in order
        investigation_id: Investigation UUID for genesis computation
        created_at: Investigation creation timestamp for genesis computation
        
    Returns:
        Tuple of (is_valid, first_bad_index):
        - is_valid: True if chain is valid, False otherwise
        - first_bad_index: Index of first invalid entry, or None if valid
    """
    if not entries:
        return True, None
    
    # Start with genesis hash
    expected_prev_hash = genesis_hash(investigation_id, created_at)
    
    for idx, entry in enumerate(entries):
        # Check if entry has required hash fields
        if "entry_hash" not in entry or "prev_hash" not in entry:
            return False, idx
        
        # Verify prev_hash matches expected
        if entry["prev_hash"] != expected_prev_hash:
            return False, idx
        
        # Verify entry_hash is correct
        computed_hash = compute_entry_hash(entry, entry["prev_hash"])
        if entry["entry_hash"] != computed_hash:
            return False, idx
        
        # Move to next entry
        expected_prev_hash = entry["entry_hash"]
    
    return True, None
