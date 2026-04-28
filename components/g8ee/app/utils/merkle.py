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

"""Deterministic Merkle tree over (agent_id, scalar) reputation leaves.

Used by the Auditor's verdict step to commit the reputation scoreboard at
verdict time (GDD §14.4 Artifact B). Determinism is load-bearing: any
party with a snapshot of `reputation_state` must be able to recompute the
root and verify the auditor's HMAC signature without re-executing the
verdict path.

Tree shape: balanced binary, last leaf duplicated to keep pairs at every
level (Bitcoin-style). Hash function: SHA-256. Leaf encoding:
``sha256(f"{agent_id}:{scalar_str}".encode("utf-8"))``. Internal nodes:
``sha256(left || right)`` over raw bytes (not hex). The returned root is
hex (64 chars).
"""

from __future__ import annotations

import hashlib
from typing import Sequence


__all__ = [
    "scalar_to_canonical_str",
    "leaf_bytes",
    "merkle_root",
    "merkle_proof",
    "verify_proof",
]


def scalar_to_canonical_str(scalar: float) -> str:
    """Render a reputation scalar as a stable decimal string.

    Uses a fixed 12-decimal representation so floating-point round-trip
    differences (e.g. ``0.1 + 0.2``) cannot produce divergent leaf bytes
    across re-derivations. Trailing zeros are *kept* — fixed-width keeps
    the canonicalisation trivially obvious from the wire bytes.
    """
    return f"{scalar:.12f}"


def leaf_bytes(agent_id: str, scalar: float) -> bytes:
    """Hash a (agent_id, scalar) pair into a 32-byte leaf digest.

    Encoding: ``sha256(f"{agent_id}:{canonical_scalar}".encode("utf-8"))``.
    """
    payload = f"{agent_id}:{scalar_to_canonical_str(scalar)}".encode("utf-8")
    return hashlib.sha256(payload).digest()


def _hash_pair(left: bytes, right: bytes) -> bytes:
    return hashlib.sha256(left + right).digest()


def _build_levels(leaves: Sequence[bytes]) -> list[list[bytes]]:
    """Build every level of the Merkle tree, leaves up to root.

    Pairs at each level are formed left-to-right; a trailing odd node is
    duplicated so its sibling exists. Returns a list whose first entry is
    the leaf level and whose final entry is a single-element list holding
    the root.
    """
    if not leaves:
        return [[]]

    levels: list[list[bytes]] = [list(leaves)]
    current = list(leaves)
    while len(current) > 1:
        next_level: list[bytes] = []
        for i in range(0, len(current), 2):
            left = current[i]
            right = current[i + 1] if i + 1 < len(current) else current[i]
            next_level.append(_hash_pair(left, right))
        levels.append(next_level)
        current = next_level
    return levels


def merkle_root(leaves: Sequence[bytes]) -> str:
    """Return the Merkle root over `leaves` as a 64-char hex string.

    Empty input yields ``sha256(b"")`` so a "no leaves" snapshot still has
    a well-defined, recomputable commitment value (matches the convention
    used by RFC 6962 for empty trees).
    """
    if not leaves:
        return hashlib.sha256(b"").hexdigest()
    levels = _build_levels(leaves)
    return levels[-1][0].hex()


def merkle_proof(leaves: Sequence[bytes], index: int) -> list[str]:
    """Return the inclusion proof for `leaves[index]` as a list of hex siblings.

    Each proof entry is the sibling node hashed alongside the running
    candidate at that level, ordered leaves-up. Use `verify_proof` to check.
    """
    if not leaves:
        raise IndexError("Cannot build a Merkle proof over an empty leaf set")
    if index < 0 or index >= len(leaves):
        raise IndexError(f"Leaf index {index} out of range for {len(leaves)} leaves")

    levels = _build_levels(leaves)
    proof: list[str] = []
    idx = index
    # Walk every level except the root level itself.
    for level in levels[:-1]:
        sibling_idx = idx ^ 1
        if sibling_idx >= len(level):
            # Odd tail: sibling is the duplicate of the current node.
            sibling_idx = idx
        proof.append(level[sibling_idx].hex())
        idx //= 2
    return proof


def verify_proof(leaf: bytes, proof: list[str], root: str, index: int) -> bool:
    """Verify a Merkle inclusion proof against a known root.

    `leaf` must be the leaf bytes (from `leaf_bytes`); `proof` is the list
    returned by `merkle_proof`; `root` is the 64-char hex root; `index`
    is the leaf's position in the original sequence.
    """
    current = leaf
    idx = index
    for sibling_hex in proof:
        sibling = bytes.fromhex(sibling_hex)
        if idx % 2 == 0:
            current = _hash_pair(current, sibling)
        else:
            current = _hash_pair(sibling, current)
        idx //= 2
    return current.hex() == root
