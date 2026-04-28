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
Bootstrap script to seed the initial reputation state for the Tribunal ensemble.

This script ensures that the `reputation_state` collection contains a row for
each recognized agent persona (axiom, concord, variance, pragma, nemesis, sage,
triage, auditor). If a row already exists for an agent, it is skipped (idempotent).

Usage:
    python3 seed-reputation-state.py [--force]
"""

import sys
from datetime import datetime, UTC
from pathlib import Path
from typing import List

# Add current directory to path to import _lib
sys.path.insert(0, str(Path(__file__).parent.absolute()))

from _lib import (
    g8es_request,
    get_document,
    print_banner,
)

AGENTS = [
    "axiom",
    "concord",
    "variance",
    "pragma",
    "nemesis",
    "sage",
    "triage",
    "auditor",
]

COLLECTION = "reputation_state"
BOOTSTRAP_SCALAR = 0.5


def seed_reputation(force: bool = False) -> int:
    print_banner("seed-reputation-state.py", f"{'--force' if force else ''}")
    
    success_count = 0
    skipped_count = 0
    error_count = 0

    now = datetime.now(UTC).isoformat().replace("+00:00", "Z")

    for agent_id in AGENTS:
        print(f"  Checking {agent_id}...", end="", flush=True)
        
        existing = None
        if not force:
            try:
                existing = get_document(COLLECTION, agent_id)
            except Exception as e:
                # If collection doesn't exist yet, we might get an error.
                # Treat as missing.
                pass

        if existing and not force:
            print(" [SKIPPED] already exists")
            skipped_count += 1
            continue

        state_doc = {
            "id": agent_id,
            "agent_id": agent_id,
            "scalar": BOOTSTRAP_SCALAR,
            "updated_at": now,
            "unbonding_until": None,
            "last_slash_tier": None,
        }

        try:
            # POST /db/<collection>/<id> to create or replace
            g8es_request("POST", f"/db/{COLLECTION}/{agent_id}", state_doc)
            print(" [OK] seeded")
            success_count += 1
        except Exception as e:
            print(f" [FAILED] {e}")
            error_count += 1

    print("\nSummary:")
    print(f"  Seeded:  {success_count}")
    print(f"  Skipped: {skipped_count}")
    print(f"  Errors:  {error_count}")
    
    return 1 if error_count > 0 else 0


if __name__ == "__main__":
    force_flag = "--force" in sys.argv
    sys.exit(seed_reputation(force=force_flag))
