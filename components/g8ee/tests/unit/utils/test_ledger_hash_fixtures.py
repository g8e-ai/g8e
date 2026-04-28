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

"""Cross-language fixture parity for ledger hashing.

These tests consume the shared fixture file at
``shared/test-fixtures/ledger-hash-fixtures.json`` and assert that the Python
implementation reproduces the recorded canonical-JSON encodings, entry hashes,
genesis hashes, and chain results. The same fixture file is consumed by the
g8ed JS verifier test (``ledger-verify-fixtures.spec.js``) so any drift between
the two implementations is caught immediately.
"""

import json
from pathlib import Path

import pytest

from app.utils.ledger_hash import (
    canonical_json,
    compute_entry_hash,
    genesis_hash,
    verify_chain,
)


def _load_fixtures():
    fixtures_path = (
        Path(__file__).resolve().parent.parent.parent.parent.parent.parent
        / "shared"
        / "test-fixtures"
        / "ledger-hash-fixtures.json"
    )
    with open(fixtures_path, encoding="utf-8") as f:
        return json.load(f)


FIXTURES = _load_fixtures()


@pytest.mark.parametrize("case", FIXTURES["canonical_json"], ids=lambda c: c["name"])
def test_canonical_json_matches_fixture(case):
    """Python canonical_json output must match the recorded UTF-8 string."""
    actual = canonical_json(case["input"]).decode("utf-8")
    assert actual == case["expected_utf8"]


@pytest.mark.parametrize("case", FIXTURES["entry_hash"], ids=lambda c: c["name"])
def test_compute_entry_hash_matches_fixture(case):
    """Python compute_entry_hash output must match the recorded hash."""
    actual = compute_entry_hash(case["entry"], case["prev_hash"])
    assert actual == case["expected_hash"]


@pytest.mark.parametrize(
    "case",
    FIXTURES["genesis_hash"],
    ids=lambda c: f"{c['investigation_id']}@{c['created_at']}",
)
def test_genesis_hash_matches_fixture(case):
    """Python genesis_hash output must match the recorded hash."""
    actual = genesis_hash(case["investigation_id"], case["created_at"])
    assert actual == case["expected_hash"]


def test_chain_fixture_verifies():
    """The recorded multi-entry chain must verify cleanly."""
    chain = FIXTURES["chain"]
    is_valid, first_bad = verify_chain(
        chain["entries"], chain["investigation_id"], chain["created_at"]
    )
    assert is_valid is True
    assert first_bad is None


def test_chain_fixture_tampering_detected():
    """Tampering with any chain entry must be caught."""
    chain = FIXTURES["chain"]
    tampered = [dict(e) for e in chain["entries"]]
    # Mutate the middle entry's content; entry_hash will no longer match.
    tampered[1] = dict(tampered[1])
    tampered[1]["content"] = "tampered"
    is_valid, first_bad = verify_chain(
        tampered, chain["investigation_id"], chain["created_at"]
    )
    assert is_valid is False
    assert first_bad == 1
