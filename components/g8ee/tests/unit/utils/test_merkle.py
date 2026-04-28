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

"""Unit tests for the Merkle utility used by the Auditor commitment step.

These tests pin determinism: any party with the same `reputation_state`
snapshot must derive the same root. They are the foundation under
`auditor_service.commit_reputation` (Phase 2) and the peer-auditor replay
job (GDD §14.7).
"""

import hashlib

import pytest

from app.utils.merkle import (
    leaf_bytes,
    merkle_proof,
    merkle_root,
    scalar_to_canonical_str,
    verify_proof,
)

pytestmark = pytest.mark.unit


class TestScalarCanonicalisation:
    def test_fixed_width_decimal(self):
        assert scalar_to_canonical_str(0.5) == "0.500000000000"
        assert scalar_to_canonical_str(1.0) == "1.000000000000"
        assert scalar_to_canonical_str(0.0) == "0.000000000000"

    def test_floating_point_round_trip_is_stable(self):
        # A classic float-precision trap: 0.1 + 0.2 != 0.3 in binary.
        # Canonicalisation must collapse them to the same string so two
        # parties that arrive at "the same scalar" by different addition
        # orders still produce the same leaf.
        assert scalar_to_canonical_str(0.1 + 0.2) == scalar_to_canonical_str(0.3)


class TestLeafBytes:
    def test_leaf_is_sha256_of_canonical_string(self):
        expected = hashlib.sha256(b"axiom:0.500000000000").digest()
        assert leaf_bytes("axiom", 0.5) == expected

    def test_distinct_inputs_distinct_leaves(self):
        assert leaf_bytes("axiom", 0.5) != leaf_bytes("concord", 0.5)
        assert leaf_bytes("axiom", 0.5) != leaf_bytes("axiom", 0.6)


class TestMerkleRoot:
    def test_empty_tree_is_sha256_of_empty_string(self):
        # RFC 6962 convention: empty tree's commitment is sha256(b"").
        # We do not raise; an empty scoreboard still needs a well-defined
        # root so the genesis commitment is verifiable.
        assert merkle_root([]) == hashlib.sha256(b"").hexdigest()

    def test_single_leaf_root_equals_leaf_hex(self):
        leaf = leaf_bytes("axiom", 0.5)
        assert merkle_root([leaf]) == leaf.hex()

    def test_two_leaves_root_is_pair_hash(self):
        a = leaf_bytes("axiom", 0.5)
        b = leaf_bytes("concord", 0.5)
        expected = hashlib.sha256(a + b).hexdigest()
        assert merkle_root([a, b]) == expected

    def test_odd_leaf_duplicates_last(self):
        a = leaf_bytes("axiom", 0.5)
        b = leaf_bytes("concord", 0.5)
        c = leaf_bytes("variance", 0.5)
        # Tree: ((a,b), (c,c))
        ab = hashlib.sha256(a + b).digest()
        cc = hashlib.sha256(c + c).digest()
        expected = hashlib.sha256(ab + cc).hexdigest()
        assert merkle_root([a, b, c]) == expected

    def test_root_is_deterministic_across_calls(self):
        leaves = [leaf_bytes(name, 0.5) for name in ("axiom", "concord", "variance", "pragma", "nemesis")]
        assert merkle_root(leaves) == merkle_root(leaves)

    def test_reordering_changes_root(self):
        # Callers MUST sort leaves by agent_id before calling merkle_root;
        # this test pins that the function itself is order-sensitive so a
        # forgotten sort step fails loudly rather than silently producing
        # a different root than the verifier.
        a = leaf_bytes("axiom", 0.5)
        b = leaf_bytes("concord", 0.5)
        assert merkle_root([a, b]) != merkle_root([b, a])

    def test_scalar_change_changes_root(self):
        before = merkle_root([leaf_bytes("axiom", 0.5)])
        after = merkle_root([leaf_bytes("axiom", 0.6)])
        assert before != after


class TestMerkleProof:
    def test_empty_proof_raises(self):
        with pytest.raises(IndexError):
            merkle_proof([], 0)

    def test_out_of_range_raises(self):
        with pytest.raises(IndexError):
            merkle_proof([leaf_bytes("axiom", 0.5)], 1)
        with pytest.raises(IndexError):
            merkle_proof([leaf_bytes("axiom", 0.5)], -1)

    def test_single_leaf_proof_is_empty(self):
        # A single-leaf tree has no siblings; the leaf hex IS the root.
        leaf = leaf_bytes("axiom", 0.5)
        assert merkle_proof([leaf], 0) == []
        assert verify_proof(leaf, [], merkle_root([leaf]), 0)

    def test_proof_verifies_for_every_index(self):
        names = ("axiom", "concord", "variance", "pragma", "nemesis", "sage", "triage", "auditor")
        leaves = [leaf_bytes(n, 0.5) for n in names]
        root = merkle_root(leaves)
        for idx in range(len(leaves)):
            proof = merkle_proof(leaves, idx)
            assert verify_proof(leaves[idx], proof, root, idx), (
                f"Proof failed for index {idx}"
            )

    def test_proof_verifies_with_odd_leaf_count(self):
        names = ("axiom", "concord", "variance")
        leaves = [leaf_bytes(n, 0.5) for n in names]
        root = merkle_root(leaves)
        for idx in range(len(leaves)):
            proof = merkle_proof(leaves, idx)
            assert verify_proof(leaves[idx], proof, root, idx)

    def test_tampered_leaf_fails_verification(self):
        names = ("axiom", "concord", "variance", "pragma")
        leaves = [leaf_bytes(n, 0.5) for n in names]
        root = merkle_root(leaves)
        proof = merkle_proof(leaves, 1)
        tampered = leaf_bytes("concord", 0.6)
        assert not verify_proof(tampered, proof, root, 1)

    def test_tampered_proof_fails_verification(self):
        names = ("axiom", "concord", "variance", "pragma")
        leaves = [leaf_bytes(n, 0.5) for n in names]
        root = merkle_root(leaves)
        proof = merkle_proof(leaves, 1)
        # Flip a bit in the first sibling.
        bad = bytearray.fromhex(proof[0])
        bad[0] ^= 0x01
        proof[0] = bad.hex()
        assert not verify_proof(leaves[1], proof, root, 1)

    def test_wrong_index_fails_verification(self):
        names = ("axiom", "concord", "variance", "pragma")
        leaves = [leaf_bytes(n, 0.5) for n in names]
        root = merkle_root(leaves)
        proof = merkle_proof(leaves, 1)
        # Same proof + leaf, but a different index in the tree → wrong sibling order.
        assert not verify_proof(leaves[1], proof, root, 2)
