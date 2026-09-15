# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

"""Deterministic canonicalization of the bundle manifest and checksum root.

Canonical bytes are produced by sorting artifacts by path, serializing
with sorted keys and compact separators, and excluding self-hash fields.
The manifest hash and checksum root hash are SHA-256 digests over the
canonical bytes with the self-hash field set to empty string.
"""

from __future__ import annotations

import hashlib
import json

from pydantic import JsonValue

from g8e_evals.bundle.manifest import BundleManifest, ChecksumRoot
from g8e_evals.bundle.validation import BundlePathError, validate_bundle_path


def _safe_sort_key(path: str) -> str:
    """Sort key for artifact paths that does not crash on invalid paths.

    Falls back to the raw path when ``validate_bundle_path`` raises so that
    canonicalization never crashes on a manifest with invalid paths. Invalid
    paths are reported by the verifier in layer 1; canonicalization only
    needs a stable sort key.
    """
    try:
        return validate_bundle_path(path)
    except BundlePathError:
        return path


def _canonical_json(model_dict: dict[str, JsonValue]) -> bytes:
    return json.dumps(
        model_dict,
        allow_nan=False,
        ensure_ascii=False,
        separators=(",", ":"),
        sort_keys=True,
    ).encode()


def _sorted_artifact_dicts(manifest: BundleManifest) -> list[dict[str, JsonValue]]:
    artifacts = [a.model_dump(mode="json", by_alias=True) for a in manifest.artifacts]
    return sorted(artifacts, key=lambda a: _safe_sort_key(a["path"]))


def _sorted_external_refs(manifest: BundleManifest) -> list[dict[str, JsonValue]]:
    refs = [r.model_dump(mode="json", by_alias=True) for r in manifest.external_references]
    return sorted(refs, key=lambda r: r["reference_id"])


def canonical_manifest_bytes(manifest: BundleManifest) -> bytes:
    """Canonical bytes of the manifest for hashing and roundtrip.

    Artifacts are sorted by path. External references are sorted by
    reference ID. The ``manifest_content_sha256`` field is set to empty
    string so the hash is independent of the hash field itself.
    """
    manifest_dict = manifest.model_dump(mode="json", by_alias=True)
    manifest_dict["artifacts"] = _sorted_artifact_dicts(manifest)
    manifest_dict["external_references"] = _sorted_external_refs(manifest)
    manifest_dict["manifest_content_sha256"] = ""
    return _canonical_json(manifest_dict)


def compute_manifest_hash(manifest: BundleManifest) -> str:
    """SHA-256 over the canonical manifest bytes."""
    return hashlib.sha256(canonical_manifest_bytes(manifest)).hexdigest()


def _sorted_checksum_entries(root: ChecksumRoot) -> list[dict[str, JsonValue]]:
    entries = [e.model_dump(mode="json", by_alias=True) for e in root.entries]
    return sorted(entries, key=lambda e: _safe_sort_key(e["path"]))


def canonical_checksum_root_bytes(root: ChecksumRoot) -> bytes:
    """Canonical bytes of the checksum root for hashing and roundtrip.

    Entries are sorted by path. The ``checksum_root_sha256`` field is set
    to empty string so the hash is independent of the hash field itself.
    """
    root_dict = root.model_dump(mode="json", by_alias=True)
    root_dict["entries"] = _sorted_checksum_entries(root)
    root_dict["checksum_root_sha256"] = ""
    return _canonical_json(root_dict)


def compute_checksum_root_hash(root: ChecksumRoot) -> str:
    """SHA-256 over the canonical checksum root bytes."""
    return hashlib.sha256(canonical_checksum_root_bytes(root)).hexdigest()
