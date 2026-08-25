# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

"""Cross-language fixture parity for ledger hashing.

These tests consume the protocol fixture file at
``protocol/test-fixtures/ledger-hash-fixtures.json`` and assert that the Python
implementation reproduces the recorded canonical-JSON encodings, entry hashes,
genesis hashes, and chain results. The same fixture file is consumed by the
client JS verifier test (``ledger-verify-fixtures.spec.js``) so any drift between
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
        / "protocol"
        / "test-fixtures"
        / "ledger-hash-fixtures.json"
    )
    if not fixtures_path.exists():
        return None
    with fixtures_path.open(encoding="utf-8") as f:
        return json.load(f)


FIXTURES = _load_fixtures()

if FIXTURES is None:
    pytest.skip("Protocol ledger hash fixtures not found — skipping", allow_module_level=True)


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
    is_valid, first_bad = verify_chain(tampered, chain["investigation_id"], chain["created_at"])
    assert is_valid is False
    assert first_bad == 1
