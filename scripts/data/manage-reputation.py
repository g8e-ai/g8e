#!/usr/bin/env python3
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
"""
g8e Reputation Management CLI

Usage:
    manage-reputation.py seed [--force]
    manage-reputation.py repair [--dry-run]
"""

import sys
import argparse
import hashlib
import hmac
import json
from datetime import datetime, UTC
from pathlib import Path
from typing import List, Dict

# Add current directory to path to import _lib
SCRIPT_DIR = Path(__file__).parent.absolute()
sys.path.insert(0, str(SCRIPT_DIR))

from _lib import (
    g8es_request,
    query_collection,
    print_banner,
    get_auditor_hmac_key,
)

# Constants mirrored from components/g8ee/app/models/reputation.py
GENESIS_PREV_ROOT = "0" * 64
COLLECTION_COMMITMENTS = "reputation_commitments"
COLLECTION_STATE = "reputation_state"
COLLECTION_RESOLUTIONS = "stake_resolutions"

# ---------------------------------------------------------------------------
# Merkle Logic (Mirrored from components/g8ee/app/utils/merkle.py)
# ---------------------------------------------------------------------------

def scalar_to_canonical_str(scalar: float) -> str:
    return f"{scalar:.12f}"

def leaf_bytes(agent_id: str, scalar: float) -> bytes:
    payload = f"{agent_id}:{scalar_to_canonical_str(scalar)}".encode("utf-8")
    return hashlib.sha256(payload).digest()

def _hash_pair(left: bytes, right: bytes) -> bytes:
    return hashlib.sha256(left + right).digest()

def _build_levels(leaves: List[bytes]) -> List[List[bytes]]:
    if not leaves:
        return [[]]
    levels = [list(leaves)]
    current = list(leaves)
    while len(current) > 1:
        next_level = []
        for i in range(0, len(current), 2):
            left = current[i]
            right = current[i + 1] if i + 1 < len(current) else current[i]
            next_level.append(_hash_pair(left, right))
        levels.append(next_level)
        current = next_level
    return levels

def compute_merkle_root(leaves: List[bytes]) -> str:
    if not leaves:
        return hashlib.sha256(b"").hexdigest()
    levels = _build_levels(leaves)
    return levels[-1][0].hex()

# ---------------------------------------------------------------------------
# CLI Commands
# ---------------------------------------------------------------------------

def run_seed(args):
    """Dispatch to seed-reputation-state.py."""
    import subprocess
    cmd = [sys.executable, str(SCRIPT_DIR / "seed-reputation-state.py")]
    if args.force:
        cmd.append("--force")
    
    return subprocess.run(cmd).returncode

def run_repair(args):
    """Verify and repair the reputation commitment chain."""
    print_banner("reputation repair", f"{'--dry-run' if args.dry_run else ''}")
    
    hmac_key = get_auditor_hmac_key()
    if not hmac_key:
        print("Error: AUDITOR_HMAC_KEY not found in bootstrap volume or environment.")
        return 1

    # 1. Fetch all commitments sorted by creation time
    print("Fetching commitments...")
    commitments = query_collection(COLLECTION_COMMITMENTS)
    commitments.sort(key=lambda x: x.get("created_at", ""))
    
    if not commitments:
        print("No commitments found. Use 'seed' to initialize the state.")
        return 0

    print(f"Found {len(commitments)} commitments.")
    
    # 2. Verify the chain
    broken_indices = []
    prev_root = GENESIS_PREV_ROOT
    
    for i, comm in enumerate(commitments):
        c_id = comm.get("id")
        c_prev = comm.get("prev_root")
        c_root = comm.get("merkle_root")
        c_sig = comm.get("signature")
        c_cmd = comm.get("tribunal_command_id")
        
        # Verify prev_root link
        if c_prev != prev_root:
            print(f"  [BROKEN LINK] at index {i} (id={c_id}): expected prev_root={prev_root}, got {c_prev}")
            broken_indices.append(i)
        
        # Verify signature
        expected_sig = hmac.new(
            hmac_key.encode("utf-8"),
            (c_root + c_prev + c_cmd).encode("utf-8"),
            hashlib.sha256,
        ).hexdigest()
        
        if c_sig != expected_sig:
            print(f"  [INVALID SIG] at index {i} (id={c_id}): signature mismatch")
            if i not in broken_indices:
                broken_indices.append(i)
                
        prev_root = c_root

    if not broken_indices:
        print("\nChain is healthy. All links and signatures verified.")
        return 0

    print(f"\nFound {len(broken_indices)} broken commitments.")
    if args.dry_run:
        print("Dry run: no repairs attempted.")
        return 1

    # Repairing the chain requires re-signing everything from the first break
    repair_start_index = broken_indices[0]
    print(f"Repairing chain from index {repair_start_index}...")

    # Fetch current reputation state to compute correct Merkle roots
    print("Fetching current reputation state...")
    state_rows = query_collection(COLLECTION_STATE)
    
    # Build leaves from current state (note: this uses current state, not historical)
    # This is a limitation - proper historical repair would require state snapshots
    state_map = {row["agent_id"]: row["scalar"] for row in state_rows}
    
    # Sort by agent_id for deterministic leaf ordering
    sorted_agents = sorted(state_map.keys())
    leaves = [leaf_bytes(agent_id, state_map[agent_id]) for agent_id in sorted_agents]
    current_root = compute_merkle_root(leaves)
    
    print(f"Computed current Merkle root from {len(leaves)} agent states: {current_root[:16]}...")
    
    # Re-chain from the repair start point
    # We need to find the last good commitment before the break
    last_good_index = repair_start_index - 1
    if last_good_index < 0:
        prev_root = GENESIS_PREV_ROOT
    else:
        prev_root = commitments[last_good_index]["merkle_root"]
    
    repaired_count = 0
    
    for i in range(repair_start_index, len(commitments)):
        comm = commitments[i]
        c_id = comm.get("id")
        c_cmd = comm.get("tribunal_command_id")
        
        print(f"  Repairing commitment {i} (id={c_id})...")
        
        # Update prev_root to point to the previous commitment's root
        new_prev_root = prev_root
        
        # For simplicity, we use the current root for all repaired commitments
        # In a full implementation, we'd need historical state snapshots
        new_root = current_root
        
        # Re-sign with the correct prev_root
        new_sig = hmac.new(
            hmac_key.encode("utf-8"),
            (new_root + new_prev_root + c_cmd).encode("utf-8"),
            hashlib.sha256,
        ).hexdigest()
        
        # Prepare the update payload
        update_payload = {
            "prev_root": new_prev_root,
            "merkle_root": new_root,
            "signature": new_sig
        }
        
        # Send update to g8es
        update_result = g8es_request(
            "PATCH",
            f"/api/internal/data/{COLLECTION_COMMITMENTS}/{c_id}",
            body=update_payload
        )
        
        if update_result.get("success"):
            print(f"    ✓ Repaired: prev_root={new_prev_root[:16]}..., signature updated")
            repaired_count += 1
        else:
            print(f"    ✗ Failed to update: {update_result.get('error', 'unknown error')}")
            return 1
        
        # Chain to the next commitment
        prev_root = new_root
    
    print(f"\nRepair complete. {repaired_count} commitments re-chained and re-signed.")
    return 0

def run(argv: list[str]) -> int:
    parser = argparse.ArgumentParser(prog="manage-g8es.py reputation")
    subparsers = parser.add_subparsers(dest="command", help="Reputation command")

    seed_parser = subparsers.add_parser("seed", help="Seed initial reputation state")
    seed_parser.add_argument("--force", action="store_true", help="Force overwrite existing state")

    repair_parser = subparsers.add_parser("repair", help="Repair reputation commitment chain gaps")
    repair_parser.add_argument("--dry-run", action="store_true", help="Show gaps without repairing")

    args = parser.parse_args(argv)

    if args.command == "seed":
        return run_seed(args)
    elif args.command == "repair":
        return run_repair(args)
    else:
        parser.print_help()
        return 1

if __name__ == "__main__":
    sys.exit(run(sys.argv[1:]))
