"""Cross-language transaction hash parity tests.

Validates that Python's ``compute_transaction_hash`` produces identical
SHA-256 digests to Go's ``GenerateMessageID`` for the same envelope fields.
Test vectors are loaded from the shared ``hash_vectors.json`` file, which is
the single source of truth for expected hash outputs.
"""

from __future__ import annotations

import json
from pathlib import Path
from typing import Any

import pytest

from g8e.models.governance import compute_transaction_hash

VECTORS_PATH = Path(__file__).parent / "hash_vectors.json"


def _load_vectors() -> list[dict[str, Any]]:
    assert VECTORS_PATH.exists(), f"Hash vectors file not found: {VECTORS_PATH}"
    with open(VECTORS_PATH, encoding="utf-8") as f:
        data = json.load(f)
    vectors = data["vectors"]
    assert len(vectors) > 0, "hash_vectors.json contains no vectors"
    return vectors


def _vector_ids() -> list[str]:
    return [v["name"] for v in _load_vectors()]


@pytest.mark.parametrize("vector", _load_vectors(), ids=_vector_ids())
def test_hash_parity_vector(vector: dict[str, Any]) -> None:
    """Each vector must produce the expected SHA-256 hash."""
    actual = compute_transaction_hash(
        action_type=vector["action_type"],
        target_resource=vector["target_resource"],
        payload=vector["payload_b64"],
        state_merkle_root=vector["state_merkle_root"],
        nonce=vector["nonce"],
        expires_at=vector["expires_at"],
        intent_data=vector["intent_data"],
        requestor_user_id=vector.get("requestor_user_id"),
        acting_app_id=vector.get("acting_app_id"),
    )
    assert actual == vector["expected_hash"], (
        f"Hash mismatch for vector {vector['name']!r}:\n"
        f"  expected: {vector['expected_hash']}\n"
        f"  got:      {actual}"
    )


def test_hash_parity_timestamp_normalization() -> None:
    """Timestamps with and without fractional seconds must produce the same hash."""
    vectors = {v["name"]: v for v in _load_vectors()}
    no_frac = vectors["timestamp_no_fractional"]
    with_frac = vectors["timestamp_with_fractional"]

    hash_no_frac = compute_transaction_hash(
        action_type=no_frac["action_type"],
        target_resource=no_frac["target_resource"],
        payload=no_frac["payload_b64"],
        state_merkle_root=no_frac["state_merkle_root"],
        nonce=no_frac["nonce"],
        expires_at=no_frac["expires_at"],
        intent_data=no_frac["intent_data"],
    )
    hash_with_frac = compute_transaction_hash(
        action_type=with_frac["action_type"],
        target_resource=with_frac["target_resource"],
        payload=with_frac["payload_b64"],
        state_merkle_root=with_frac["state_merkle_root"],
        nonce=with_frac["nonce"],
        expires_at=with_frac["expires_at"],
        intent_data=with_frac["intent_data"],
    )
    assert hash_no_frac == hash_with_frac, (
        f"Timestamp normalization failed: {hash_no_frac} != {hash_with_frac}"
    )


def test_hash_parity_optional_fields_none_vs_empty() -> None:
    """None and empty string for optional fields must produce the same hash (both omitted)."""
    base = dict(
        action_type="EXECUTE_BASH",
        target_resource="localhost",
        payload="dGVzdA==",
        state_merkle_root="root-opt",
        nonce="nonce-opt",
        expires_at="2026-07-04T12:00:00Z",
        intent_data={"command": "ls"},
    )
    hash_none = compute_transaction_hash(**base)
    hash_empty = compute_transaction_hash(
        **base, requestor_user_id="", acting_app_id="",
    )
    assert hash_none == hash_empty


def test_hash_parity_deterministic() -> None:
    """Same inputs must always produce the same hash."""
    vectors = _load_vectors()
    v = vectors[0]
    kwargs = dict(
        action_type=v["action_type"],
        target_resource=v["target_resource"],
        payload=v["payload_b64"],
        state_merkle_root=v["state_merkle_root"],
        nonce=v["nonce"],
        expires_at=v["expires_at"],
        intent_data=v["intent_data"],
        requestor_user_id=v.get("requestor_user_id"),
        acting_app_id=v.get("acting_app_id"),
    )
    h1 = compute_transaction_hash(**kwargs)
    h2 = compute_transaction_hash(**kwargs)
    assert h1 == h2
    assert len(h1) == 64
