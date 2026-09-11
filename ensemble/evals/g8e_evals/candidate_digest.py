# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

"""Deterministic candidate-tree digest for v5 publication promotion.

The candidate-tree digest is the content-addressed identity of an entire
v5 candidate directory. It is computed over the sorted file inventory of
the candidate directory: one entry per regular file, keyed by the relative
POSIX path, paired with the SHA-256 of that file's bytes. The digest is the
SHA-256 over the canonical JSON encoding of the sorted inventory.

The digest is independent of the ``ModelCampaignRef.content_hash`` (which
covers only the model-campaign reference JSON) so promotion can approve an
exact tree identity that encompasses every artifact in the candidate,
including projection rows, summaries, event/resource files, and any
disclosure outputs. A changed, added, or removed file changes the digest;
the same candidate tree always produces the same digest.

Symlinks and non-regular files are rejected so a candidate cannot hide
content behind a link. Empty directories do not contribute to the digest;
only files do. The relative path is normalized to POSIX form so the digest
is stable across operating systems.
"""

from __future__ import annotations

import hashlib
import json
from pathlib import Path


class CandidateDigestError(ValueError):
    """Raised when the candidate tree digest cannot be computed."""


def _sha256_bytes(data: bytes) -> str:
    return hashlib.sha256(data).hexdigest()


def _inventory_candidate(candidate_dir: Path) -> list[tuple[str, str]]:
    """Build the sorted (relative_path, file_content_sha256) inventory.

    Enumerates every regular file under ``candidate_dir`` recursively,
    rejects symlinks and non-regular files, and returns a list of
    (relative_posix_path, sha256_hex) tuples sorted by relative path.
    """
    inventory: list[tuple[str, str]] = []
    for path in sorted(candidate_dir.rglob("*")):
        if path.is_symlink():
            rel = path.relative_to(candidate_dir).as_posix()
            raise CandidateDigestError(f"symlink rejected in candidate: {rel}")
        if not path.is_file():
            continue
        rel = path.relative_to(candidate_dir).as_posix()
        content_hash = _sha256_bytes(path.read_bytes())
        inventory.append((rel, content_hash))
    inventory.sort(key=lambda entry: entry[0])
    return inventory


def compute_candidate_tree_digest(candidate_dir: Path) -> str:
    """Compute the deterministic candidate-tree digest.

    Returns the SHA-256 over the canonical JSON encoding of the sorted
    (relative_path, file_content_sha256) inventory of the candidate
    directory. The digest is deterministic: the same candidate tree always
    produces the same digest. Any added, removed, or modified file changes
    the digest.

    Raises ``CandidateDigestError`` when the candidate directory contains a
    symlink or when the directory does not exist.
    """
    if not candidate_dir.exists():
        raise CandidateDigestError(f"candidate directory does not exist: {candidate_dir}")
    if not candidate_dir.is_dir():
        raise CandidateDigestError(f"candidate path is not a directory: {candidate_dir}")
    inventory = _inventory_candidate(candidate_dir)
    payload = json.dumps(
        {"files": [{"path": p, "sha256": h} for p, h in inventory]},
        allow_nan=False,
        ensure_ascii=False,
        separators=(",", ":"),
        sort_keys=True,
    )
    return hashlib.sha256(payload.encode()).hexdigest()


__all__ = [
    "CandidateDigestError",
    "compute_candidate_tree_digest",
]
