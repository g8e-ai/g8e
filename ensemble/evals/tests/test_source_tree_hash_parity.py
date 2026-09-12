# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

"""Cross-language parity test for the canonical source-tree state hash.

The Go implementation (``internal/buildinfo.ComputeSourceTreeHash``) and
this Python reference must produce byte-identical digests for the same
tree so the ``G8E_EVALS_SOURCE_TREE_STATE_HASH`` contract means the same
thing on both sides of the bridge. Both suites pin the same golden
constant over the same fixture; the fixture layout deliberately exercises
path-component ordering (``a/`` entries sort before ``a.b`` and ``a.txt``,
which differs from byte-wise string ordering).
"""

from __future__ import annotations

from pathlib import Path

import pytest

from g8e_evals.preflight import compute_source_tree_state_hash

pytestmark = pytest.mark.unit

# The same constant is asserted in internal/buildinfo/treehash_test.go.
_GOLDEN_FIXTURE_DIGEST = "36ee160f6da7ad1edd2ffebe9086a825c7361dacbb77238f33d432964b0e634f"

_FIXTURE: dict[str, str] = {
    "a.txt": "alpha",
    "a/b.txt": "beta",
    "a/c.txt": "chi",
    "a.b/d.txt": "delta",
    ".hidden": "hidden",
    "dir/sub/c.txt": "gamma",
}


def _write_parity_fixture(root: Path) -> None:
    for rel, content in _FIXTURE.items():
        path = root / rel
        path.parent.mkdir(parents=True, exist_ok=True)
        path.write_bytes(content.encode())


def test_compute_source_tree_state_hash_matches_go_reference_digest(tmp_path: Path):
    _write_parity_fixture(tmp_path)
    assert compute_source_tree_state_hash(tmp_path) == _GOLDEN_FIXTURE_DIGEST
