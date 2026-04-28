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

"""Generate cross-language ledger-hash test fixtures.

The Python implementation is the source of truth. This script writes a JSON
fixture file that both the Python (``test_ledger_hash_fixtures.py``) and the
JS (``ledger-verify-fixtures.spec.js``) suites consume to assert byte-exact
agreement between implementations.

Run after any change to ``components/g8ee/app/utils/ledger_hash.py``::

    python3 scripts/testing/gen_ledger_hash_fixtures.py

The generated file is committed at::

    shared/test-fixtures/ledger-hash-fixtures.json
"""

from __future__ import annotations

import json
import sys
from pathlib import Path

REPO_ROOT = Path(__file__).resolve().parent.parent.parent
sys.path.insert(0, str(REPO_ROOT / "components" / "g8ee"))

from app.utils.ledger_hash import (  # noqa: E402
    canonical_json,
    compute_entry_hash,
    genesis_hash,
)


CASES = [
    {"name": "simple_flat", "obj": {"a": 1, "b": "two", "c": True}},
    {"name": "nested_object", "obj": {"z": {"b": 2, "a": 1}, "a": [3, 1, 2]}},
    {"name": "unicode_content", "obj": {"msg": "héllo wörld", "sender": "user.chat"}},
    {
        "name": "deep_nesting",
        "obj": {"l1": {"l2": {"l3": {"l4": {"k": "v", "a": 1}}}}, "top": 0},
    },
    {
        "name": "realistic_history_entry",
        "obj": {
            "sender": "ai.primary",
            "content": "Investigating port 443 on host.",
            "timestamp": "2024-01-01T12:34:56Z",
            "metadata": {
                "event_type": "g8e.v1.ai.primary.message.created",
                "tokens": 42,
            },
            "tool_calls": [
                {"tool": "port_check", "args": {"host": "a", "port": 443}}
            ],
        },
    },
    {"name": "array_of_primitives", "obj": {"list": [1, "two", True, None, 3.14]}},
    {"name": "empty_object", "obj": {}},
]

GENESIS_INPUTS = [
    ("inv-001", "2024-01-01T00:00:00Z"),
    ("inv-002", "2025-06-15T13:45:30+00:00"),
    ("00000000-0000-0000-0000-000000000000", "1970-01-01T00:00:00Z"),
]


def build_fixtures() -> dict:
    out: dict = {"canonical_json": [], "entry_hash": [], "genesis_hash": []}

    for c in CASES:
        out["canonical_json"].append(
            {
                "name": c["name"],
                "input": c["obj"],
                "expected_utf8": canonical_json(c["obj"]).decode("utf-8"),
            }
        )

    prev = "a" * 64
    for c in CASES:
        out["entry_hash"].append(
            {
                "name": c["name"],
                "entry": c["obj"],
                "prev_hash": prev,
                "expected_hash": compute_entry_hash(c["obj"], prev),
            }
        )

    for inv_id, ts in GENESIS_INPUTS:
        out["genesis_hash"].append(
            {
                "investigation_id": inv_id,
                "created_at": ts,
                "expected_hash": genesis_hash(inv_id, ts),
            }
        )

    chain_inv_id = "chain-test-inv"
    chain_created = "2024-03-15T10:00:00Z"
    prev_h = genesis_hash(chain_inv_id, chain_created)
    chain_entries: list[dict] = []
    for i, content in enumerate(["first", "second with nested", "third héllo"]):
        entry = {
            "id": f"msg-{i}",
            "sender": "user.chat" if i % 2 == 0 else "ai.primary",
            "content": content,
            "timestamp": f"2024-03-15T10:0{i}:00Z",
            "metadata": {"idx": i, "sub": {"k": i * 2}},
            "prev_hash": prev_h,
        }
        h = compute_entry_hash(entry, prev_h)
        entry["entry_hash"] = h
        chain_entries.append(entry)
        prev_h = h
    out["chain"] = {
        "investigation_id": chain_inv_id,
        "created_at": chain_created,
        "entries": chain_entries,
    }

    return out


def main() -> int:
    fixtures = build_fixtures()
    target = REPO_ROOT / "shared" / "test-fixtures" / "ledger-hash-fixtures.json"
    with open(target, "w", encoding="utf-8") as f:
        json.dump(fixtures, f, indent=2, sort_keys=True, ensure_ascii=False)
        f.write("\n")
    print(f"wrote {target.relative_to(REPO_ROOT)}")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
