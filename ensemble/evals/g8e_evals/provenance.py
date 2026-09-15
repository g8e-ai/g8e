# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

"""Source provenance from an explicit reviewed inclusion manifest.

Source provenance is checksum-bound from an explicit inclusion list
rather than derived from repository history. The inclusion manifest is
a frozen, ``extra="forbid"`` typed model listing every expected source
file with its relative path, SHA-256, and byte length. A manifest hash
binds the entire manifest content.

The verifier checks the manifest against the on-disk source tree and
fails closed on any discrepancy: missing files, escaping files (on disk
but not in the manifest), duplicate paths, symlinks, path traversal,
checksum mismatch, byte-length mismatch, or manifest-hash mismatch.

This module performs read-only checks. It does not mutate, create, or
delete any files.
"""

from __future__ import annotations

import hashlib
from collections.abc import Sequence
from enum import StrEnum
from pathlib import Path, PurePosixPath

from pydantic import BaseModel, ConfigDict, Field


PROVENANCE_MANIFEST_SCHEMA_VERSION = "1.0.0"


class ProvenanceFailureCode(StrEnum):
    """Centralized stable failure codes for source provenance verification."""

    DUPLICATE_PATH = "duplicate_path"
    PATH_TRAVERSAL = "path_traversal"
    SYMLINK_DETECTED = "symlink_detected"
    MISSING_FILE = "missing_file"
    ESCAPING_FILE = "escaping_file"
    CHECKSUM_MISMATCH = "checksum_mismatch"
    BYTE_LENGTH_MISMATCH = "byte_length_mismatch"
    MANIFEST_HASH_MISMATCH = "manifest_hash_mismatch"


class SourceInclusionEntry(BaseModel):
    """A single source file entry in the inclusion manifest.

    The path is a relative POSIX path from the source root. The SHA-256
    and byte length bind the expected file content.
    """

    model_config = ConfigDict(extra="forbid", frozen=True)

    path: str = Field(min_length=1, description="Relative POSIX path from the source root.")
    sha256: str = Field(pattern=r"^[0-9a-f]{64}$", description="Expected SHA-256 of the file content.")
    byte_length: int = Field(ge=0, description="Expected file size in bytes.")


class SourceInclusionManifest(BaseModel):
    """Explicit reviewed inclusion manifest for source provenance.

    Lists every expected source file with its checksum. The manifest
    hash binds the entire manifest content so any tampering is detected.
    """

    model_config = ConfigDict(extra="forbid", frozen=True)

    schema_version: str = Field(description="Manifest schema version.")
    entries: list[SourceInclusionEntry] = Field(min_length=1, description="Explicit list of source files to include.")
    manifest_hash: str = Field(pattern=r"^[0-9a-f]{64}$", description="SHA-256 over the canonical manifest content.")
    reviewed_by: str = Field(default="", description="Reviewer identity.")
    review_timestamp: str = Field(default="", description="Review timestamp.")


class ProvenanceFailure(BaseModel):
    """A single typed provenance verification failure."""

    model_config = ConfigDict(extra="forbid", frozen=True)

    code: ProvenanceFailureCode
    path: str = Field(default="", description="File path related to the failure, if applicable.")
    message: str = Field(description="Human-readable failure detail.")


class ProvenanceVerificationResult(BaseModel):
    """Typed result of source provenance verification.

    ``ok`` is True only when every checked layer passes. ``checked_layers``
    is a sorted list of verification layer names. ``failures`` is a sorted
    list of typed failures.
    """

    model_config = ConfigDict(extra="forbid", frozen=True)

    ok: bool
    manifest_hash: str
    checked_layers: list[str]
    failures: list[ProvenanceFailure]


def compute_manifest_hash(entries: Sequence[SourceInclusionEntry]) -> str:
    """Compute a deterministic SHA-256 over the canonical manifest content.

    Entries are sorted by path so the hash is independent of input order.
    Each entry contributes its path, SHA-256, and byte length.
    """
    sorted_entries = sorted(entries, key=lambda e: e.path)
    hasher = hashlib.sha256()
    for entry in sorted_entries:
        hasher.update(entry.path.encode("utf-8"))
        hasher.update(b"\0")
        hasher.update(entry.sha256.encode("utf-8"))
        hasher.update(b"\0")
        hasher.update(str(entry.byte_length).encode("utf-8"))
        hasher.update(b"\0")
    return hasher.hexdigest()


def _has_traversal(path: str) -> bool:
    """Return True if the path contains traversal sequences or is absolute."""
    if not path:
        return False
    if path.startswith("/"):
        return True
    parts = PurePosixPath(path).parts
    return ".." in parts


def verify_source_provenance(
    manifest: SourceInclusionManifest,
    source_root: Path,
) -> ProvenanceVerificationResult:
    """Verify source provenance from an explicit inclusion manifest.

    Checks the manifest against the on-disk source tree in ordered layers:

    1. **manifest_hash**: The declared manifest hash matches the computed hash.
    2. **duplicate_paths**: No path appears more than once in the manifest.
    3. **path_traversal**: No manifest entry contains traversal or absolute paths.
    4. **symlink_scan**: No symlinks exist in the source tree.
    5. **missing_files**: Every manifest entry exists on disk as a regular file.
    6. **escaping_files**: Every regular file on disk is listed in the manifest.
    7. **checksum_verification**: Every file's SHA-256 and byte length match the manifest.

    All layers run regardless of earlier failures so the result reports
    every discrepancy. Returns a typed ``ProvenanceVerificationResult``.
    """
    failures: list[ProvenanceFailure] = []
    checked_layers: list[str] = []

    # Layer 1: manifest hash
    checked_layers.append("manifest_hash")
    computed_hash = compute_manifest_hash(manifest.entries)
    if computed_hash != manifest.manifest_hash:
        failures.append(ProvenanceFailure(
            code=ProvenanceFailureCode.MANIFEST_HASH_MISMATCH,
            message=(
                f"declared manifest hash {manifest.manifest_hash} does not match "
                f"computed hash {computed_hash}"
            ),
        ))

    # Layer 2: duplicate paths
    checked_layers.append("duplicate_paths")
    seen: set[str] = set()
    for entry in manifest.entries:
        if entry.path in seen:
            failures.append(ProvenanceFailure(
                code=ProvenanceFailureCode.DUPLICATE_PATH,
                path=entry.path,
                message=f"duplicate path in manifest: {entry.path}",
            ))
        seen.add(entry.path)

    # Layer 3: path traversal
    checked_layers.append("path_traversal")
    for entry in manifest.entries:
        if _has_traversal(entry.path):
            failures.append(ProvenanceFailure(
                code=ProvenanceFailureCode.PATH_TRAVERSAL,
                path=entry.path,
                message=f"path traversal or absolute path in manifest: {entry.path}",
            ))

    # Layers 4-7 require filesystem access. If source_root doesn't exist,
    # every manifest entry is missing and no escaping files can be found.
    manifest_paths = {e.path for e in manifest.entries}

    # Layer 4: symlink scan
    checked_layers.append("symlink_scan")
    disk_files: set[str] = set()
    if source_root.is_dir():
        for path in sorted(source_root.rglob("*")):
            if path.is_symlink():
                rel = path.relative_to(source_root).as_posix()
                failures.append(ProvenanceFailure(
                    code=ProvenanceFailureCode.SYMLINK_DETECTED,
                    path=rel,
                    message=f"symlink detected in source tree: {rel}",
                ))
            elif path.is_file():
                rel = path.relative_to(source_root).as_posix()
                disk_files.add(rel)

    # Layer 5: missing files
    checked_layers.append("missing_files")
    for entry in manifest.entries:
        if _has_traversal(entry.path):
            continue
        disk_path = source_root / entry.path
        if not disk_path.exists() or not disk_path.is_file():
            failures.append(ProvenanceFailure(
                code=ProvenanceFailureCode.MISSING_FILE,
                path=entry.path,
                message=f"manifest entry not found on disk: {entry.path}",
            ))

    # Layer 6: escaping files
    checked_layers.append("escaping_files")
    for rel in sorted(disk_files):
        if rel not in manifest_paths:
            failures.append(ProvenanceFailure(
                code=ProvenanceFailureCode.ESCAPING_FILE,
                path=rel,
                message=f"file on disk not in manifest: {rel}",
            ))

    # Layer 7: checksum verification
    checked_layers.append("checksum_verification")
    for entry in manifest.entries:
        if _has_traversal(entry.path):
            continue
        disk_path = source_root / entry.path
        if not disk_path.is_file():
            continue
        content = disk_path.read_bytes()
        actual_sha = hashlib.sha256(content).hexdigest()
        if actual_sha != entry.sha256:
            failures.append(ProvenanceFailure(
                code=ProvenanceFailureCode.CHECKSUM_MISMATCH,
                path=entry.path,
                message=(
                    f"checksum mismatch for {entry.path}: "
                    f"expected {entry.sha256}, got {actual_sha}"
                ),
            ))
        if len(content) != entry.byte_length:
            failures.append(ProvenanceFailure(
                code=ProvenanceFailureCode.BYTE_LENGTH_MISMATCH,
                path=entry.path,
                message=(
                    f"byte length mismatch for {entry.path}: "
                    f"expected {entry.byte_length}, got {len(content)}"
                ),
            ))

    failures.sort(key=lambda f: (f.code.value, f.path))
    checked_layers.sort()
    return ProvenanceVerificationResult(
        ok=len(failures) == 0,
        manifest_hash=manifest.manifest_hash,
        checked_layers=checked_layers,
        failures=failures,
    )


__all__ = [
    "PROVENANCE_MANIFEST_SCHEMA_VERSION",
    "ProvenanceFailure",
    "ProvenanceFailureCode",
    "ProvenanceVerificationResult",
    "SourceInclusionEntry",
    "SourceInclusionManifest",
    "compute_manifest_hash",
    "verify_source_provenance",
]
