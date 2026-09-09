# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

"""Path rooting, manifest validation, and orphan/missing-file detection.

Every path in a bundle manifest is rooted beneath the bundle root.
Absolute paths, traversal, backslashes, null bytes, duplicate normalized
paths, symlinks, unknown artifact types, and unlisted files are rejected.
"""

from __future__ import annotations

import hashlib
from pathlib import Path, PurePosixPath

from g8e_evals.bundle.manifest import BundleManifest


class BundlePathError(ValueError):
    """A bundle path violates the rooting or safety requirements."""


class BundleManifestValidationError(ValueError):
    """A bundle manifest violates the validation requirements."""


def validate_bundle_path(path: str) -> str:
    """Validate and normalize a bundle path.

    Returns the normalized relative path. Raises ``BundlePathError`` for
    absolute paths, traversal, backslashes, null bytes, and empty paths.
    """
    if not path:
        raise BundlePathError("bundle path must not be empty")

    if "\x00" in path:
        raise BundlePathError("bundle path must not contain null bytes")

    if "\\" in path:
        raise BundlePathError("bundle path must not contain backslashes")

    if path.startswith("/"):
        raise BundlePathError(f"bundle path must not be absolute: {path}")

    normalized = str(PurePosixPath(path))

    if normalized.startswith("..") or "/../" in f"/{normalized}/" or normalized == "..":
        raise BundlePathError(f"bundle path must not contain traversal: {path}")

    parts = PurePosixPath(normalized).parts
    if ".." in parts:
        raise BundlePathError(f"bundle path must not contain traversal: {path}")

    return normalized


def validate_bundle_manifest(manifest: BundleManifest) -> None:
    """Validate the manifest: paths, duplicates, restricted encryption.

    Raises ``BundleManifestValidationError`` for absolute paths, traversal,
    duplicate normalized paths, restricted artifacts without encryption,
    and empty artifact lists.
    """
    if not manifest.artifacts:
        raise BundleManifestValidationError("bundle manifest must contain at least one artifact")

    seen: set[str] = set()
    for entry in manifest.artifacts:
        try:
            normalized = validate_bundle_path(entry.path)
        except BundlePathError as exc:
            raise BundleManifestValidationError(str(exc)) from exc

        if normalized in seen:
            raise BundleManifestValidationError(f"duplicate bundle path: {entry.path}")
        seen.add(normalized)

        if entry.privacy_class == "restricted" and entry.encryption is None:
            raise BundleManifestValidationError(
                f"restricted artifact must carry encryption metadata: {entry.path}",
            )


def validate_no_orphans(manifest: BundleManifest, bundle_root: Path) -> None:
    """Reject files in the bundle directory not listed in the manifest.

    Walks the bundle directory and checks that every regular file is listed
    in the manifest. Symlinks are rejected. Raises
    ``BundleManifestValidationError`` for orphans or symlinks.
    """
    listed = {validate_bundle_path(e.path) for e in manifest.artifacts}

    for file_path in sorted(bundle_root.rglob("*")):
        if file_path.is_dir():
            continue
        if file_path.is_symlink():
            raise BundleManifestValidationError(f"symlink rejected in bundle: {file_path}")
        rel = str(file_path.relative_to(bundle_root))
        rel = validate_bundle_path(rel)
        if rel not in listed:
            raise BundleManifestValidationError(f"orphan file not listed in manifest: {rel}")


def validate_no_missing_files(manifest: BundleManifest, bundle_root: Path) -> None:
    """Reject files listed in the manifest that do not exist or hash-mismatch.

    Checks that every file listed in the manifest exists, is a regular file
    (not a symlink), and its SHA-256 matches the manifest entry. Raises
    ``BundleManifestValidationError`` for missing files, symlinks, or hash
    mismatches.
    """
    for entry in manifest.artifacts:
        normalized = validate_bundle_path(entry.path)
        file_path = bundle_root / normalized
        if not file_path.exists():
            raise BundleManifestValidationError(f"missing file listed in manifest: {normalized}")
        if file_path.is_symlink():
            raise BundleManifestValidationError(f"symlink rejected in bundle: {normalized}")
        if not file_path.is_file():
            raise BundleManifestValidationError(f"manifest path is not a regular file: {normalized}")
        content = file_path.read_bytes()
        actual_hash = hashlib.sha256(content).hexdigest()
        if actual_hash != entry.sha256:
            raise BundleManifestValidationError(
                f"hash mismatch for {normalized}: manifest={entry.sha256} actual={actual_hash}",
            )
