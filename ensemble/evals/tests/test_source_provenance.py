# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

"""Tests for source provenance from an explicit reviewed inclusion manifest.

Verifies that source provenance is checksum-bound from an explicit
inclusion list. Missing, escaping, duplicate, symlinked, or changed
source entries fail closed.
"""

from __future__ import annotations

import hashlib
from pathlib import Path

import pytest
from pydantic import ValidationError

from g8e_evals.provenance import (
    PROVENANCE_MANIFEST_SCHEMA_VERSION,
    ProvenanceFailure,
    ProvenanceFailureCode,
    ProvenanceVerificationResult,
    SourceInclusionEntry,
    SourceInclusionManifest,
    compute_manifest_hash,
    verify_source_provenance,
)


# ---------------------------------------------------------------------------
# Helpers
# ---------------------------------------------------------------------------

_VALID_SHA = "a" * 64


def _make_entry(
    path: str = "src/main.py",
    sha256: str = _VALID_SHA,
    byte_length: int = 100,
) -> dict:
    return {"path": path, "sha256": sha256, "byte_length": byte_length}


def _make_manifest_data(
    entries: list[dict] | None = None,
    manifest_hash: str | None = None,
    reviewed_by: str = "reviewer-1",
    review_timestamp: str = "2026-09-10T00:00:00Z",
) -> dict:
    if entries is None:
        entries = [_make_entry()]
    data = {
        "schema_version": PROVENANCE_MANIFEST_SCHEMA_VERSION,
        "entries": entries,
        "reviewed_by": reviewed_by,
        "review_timestamp": review_timestamp,
    }
    if manifest_hash is not None:
        data["manifest_hash"] = manifest_hash
    else:
        data["manifest_hash"] = compute_manifest_hash(entries)
    return data


def _write_source_tree(root: Path, files: dict[str, bytes]) -> None:
    """Write a set of files under root. ``files`` maps relative POSIX paths to content."""
    for rel, content in files.items():
        target = root / rel
        target.parent.mkdir(parents=True, exist_ok=True)
        target.write_bytes(content)


def _entry_for(rel: str, content: bytes) -> dict:
    return {
        "path": rel,
        "sha256": hashlib.sha256(content).hexdigest(),
        "byte_length": len(content),
    }


# ---------------------------------------------------------------------------
# Unit: model validation
# ---------------------------------------------------------------------------

@pytest.mark.unit
def test_source_inclusion_entry_rejects_unknown_field():
    with pytest.raises(ValidationError, match="extra"):
        SourceInclusionEntry.model_validate({**_make_entry(), "unexpected": "bad"})


@pytest.mark.unit
def test_source_inclusion_entry_rejects_malformed_sha256():
    with pytest.raises(ValidationError, match="string_pattern_mismatch"):
        SourceInclusionEntry.model_validate(_make_entry(sha256="not-a-hash"))


@pytest.mark.unit
def test_source_inclusion_entry_rejects_empty_path():
    with pytest.raises(ValidationError, match="at least 1 character"):
        SourceInclusionEntry.model_validate(_make_entry(path=""))


@pytest.mark.unit
def test_source_inclusion_entry_rejects_negative_byte_length():
    with pytest.raises(ValidationError, match="greater than or equal to 0"):
        SourceInclusionEntry.model_validate(_make_entry(byte_length=-1))


@pytest.mark.unit
def test_source_inclusion_manifest_rejects_unknown_field():
    data = _make_manifest_data()
    data["unexpected"] = "bad"
    with pytest.raises(ValidationError, match="extra"):
        SourceInclusionManifest.model_validate(data)


@pytest.mark.unit
def test_source_inclusion_manifest_rejects_empty_entries():
    data = _make_manifest_data(entries=[])
    with pytest.raises(ValidationError, match="at least 1 item"):
        SourceInclusionManifest.model_validate(data)


@pytest.mark.unit
def test_source_inclusion_manifest_rejects_malformed_manifest_hash():
    data = _make_manifest_data(manifest_hash="not-a-hash")
    with pytest.raises(ValidationError, match="string_pattern_mismatch"):
        SourceInclusionManifest.model_validate(data)


@pytest.mark.unit
def test_provenance_failure_model_is_frozen():
    fail = ProvenanceFailure(code=ProvenanceFailureCode.MISSING_FILE, path="x", message="m")
    with pytest.raises(ValidationError, match="frozen"):
        fail.code = ProvenanceFailureCode.DUPLICATE_PATH  # type: ignore[misc]


@pytest.mark.unit
def test_provenance_verification_result_model_rejects_unknown_field():
    with pytest.raises(ValidationError, match="extra"):
        ProvenanceVerificationResult.model_validate({
            "ok": True,
            "manifest_hash": _VALID_SHA,
            "checked_layers": [],
            "failures": [],
            "unexpected": "bad",
        })


# ---------------------------------------------------------------------------
# Unit: manifest hash computation
# ---------------------------------------------------------------------------

@pytest.mark.unit
def test_compute_manifest_hash_is_deterministic():
    entries = [_make_entry("a.py"), _make_entry("b.py")]
    h1 = compute_manifest_hash(entries)
    h2 = compute_manifest_hash(entries)
    assert h1 == h2
    assert len(h1) == 64


@pytest.mark.unit
def test_compute_manifest_hash_changes_with_different_entries():
    entries_a = [_make_entry("a.py")]
    entries_b = [_make_entry("b.py")]
    assert compute_manifest_hash(entries_a) != compute_manifest_hash(entries_b)


@pytest.mark.unit
def test_compute_manifest_hash_independent_of_entry_order():
    entries_a = [_make_entry("a.py"), _make_entry("b.py")]
    entries_b = [_make_entry("b.py"), _make_entry("a.py")]
    assert compute_manifest_hash(entries_a) == compute_manifest_hash(entries_b)


# ---------------------------------------------------------------------------
# Unit: duplicate path detection (no filesystem)
# ---------------------------------------------------------------------------

@pytest.mark.unit
def test_verify_rejects_duplicate_paths_without_filesystem():
    entries = [_make_entry("src/a.py"), _make_entry("src/a.py")]
    manifest = SourceInclusionManifest.model_validate(_make_manifest_data(entries=entries))
    result = verify_source_provenance(manifest, source_root=Path("/nonexistent"))
    assert not result.ok
    codes = {f.code for f in result.failures}
    assert ProvenanceFailureCode.DUPLICATE_PATH in codes


# ---------------------------------------------------------------------------
# Unit: path traversal detection (no filesystem)
# ---------------------------------------------------------------------------

@pytest.mark.unit
def test_verify_rejects_traversal_path_without_filesystem():
    entries = [_make_entry("../../etc/passwd")]
    manifest = SourceInclusionManifest.model_validate(_make_manifest_data(entries=entries))
    result = verify_source_provenance(manifest, source_root=Path("/nonexistent"))
    assert not result.ok
    codes = {f.code for f in result.failures}
    assert ProvenanceFailureCode.PATH_TRAVERSAL in codes


@pytest.mark.unit
def test_verify_rejects_absolute_path_without_filesystem():
    entries = [_make_entry("/etc/passwd")]
    manifest = SourceInclusionManifest.model_validate(_make_manifest_data(entries=entries))
    result = verify_source_provenance(manifest, source_root=Path("/nonexistent"))
    assert not result.ok
    codes = {f.code for f in result.failures}
    assert ProvenanceFailureCode.PATH_TRAVERSAL in codes


# ---------------------------------------------------------------------------
# Unit: manifest hash mismatch (no filesystem)
# ---------------------------------------------------------------------------

@pytest.mark.unit
def test_verify_rejects_manifest_hash_mismatch():
    entries = [_make_entry("src/a.py")]
    wrong_hash = "b" * 64
    manifest = SourceInclusionManifest.model_validate(
        _make_manifest_data(entries=entries, manifest_hash=wrong_hash)
    )
    result = verify_source_provenance(manifest, source_root=Path("/nonexistent"))
    assert not result.ok
    codes = {f.code for f in result.failures}
    assert ProvenanceFailureCode.MANIFEST_HASH_MISMATCH in codes


# ---------------------------------------------------------------------------
# Integration: valid manifest passes
# ---------------------------------------------------------------------------

@pytest.mark.integration
def test_verify_accepts_valid_manifest(tmp_path: Path):
    files = {"src/main.py": b"# main\n", "src/utils.py": b"# utils\n"}
    _write_source_tree(tmp_path, files)
    entries = [_entry_for(rel, content) for rel, content in files.items()]
    manifest = SourceInclusionManifest.model_validate(_make_manifest_data(entries=entries))
    result = verify_source_provenance(manifest, source_root=tmp_path)
    assert result.ok, [f"{f.code}: {f.message}" for f in result.failures]
    assert result.failures == []


# ---------------------------------------------------------------------------
# Integration: missing file fails
# ---------------------------------------------------------------------------

@pytest.mark.integration
def test_verify_rejects_missing_file(tmp_path: Path):
    files = {"src/main.py": b"# main\n"}
    _write_source_tree(tmp_path, files)
    entries = [
        _entry_for("src/main.py", b"# main\n"),
        _entry_for("src/missing.py", b"# missing\n"),
    ]
    manifest = SourceInclusionManifest.model_validate(_make_manifest_data(entries=entries))
    result = verify_source_provenance(manifest, source_root=tmp_path)
    assert not result.ok
    codes = {f.code for f in result.failures}
    assert ProvenanceFailureCode.MISSING_FILE in codes
    missing_failures = [f for f in result.failures if f.code == ProvenanceFailureCode.MISSING_FILE]
    assert any("src/missing.py" in f.path for f in missing_failures)


# ---------------------------------------------------------------------------
# Integration: escaping file fails (file on disk not in manifest)
# ---------------------------------------------------------------------------

@pytest.mark.integration
def test_verify_rejects_escaping_file(tmp_path: Path):
    files = {"src/main.py": b"# main\n", "src/extra.py": b"# extra\n"}
    _write_source_tree(tmp_path, files)
    entries = [_entry_for("src/main.py", b"# main\n")]
    manifest = SourceInclusionManifest.model_validate(_make_manifest_data(entries=entries))
    result = verify_source_provenance(manifest, source_root=tmp_path)
    assert not result.ok
    codes = {f.code for f in result.failures}
    assert ProvenanceFailureCode.ESCAPING_FILE in codes
    escaping_failures = [f for f in result.failures if f.code == ProvenanceFailureCode.ESCAPING_FILE]
    assert any("src/extra.py" in f.path for f in escaping_failures)


# ---------------------------------------------------------------------------
# Integration: symlinked file fails
# ---------------------------------------------------------------------------

@pytest.mark.integration
def test_verify_rejects_symlinked_file_on_disk(tmp_path: Path):
    real_file = tmp_path / "real.py"
    real_file.write_bytes(b"# real\n")
    link = tmp_path / "link.py"
    link.symlink_to(real_file)
    entries = [
        _entry_for("link.py", b"# real\n"),
    ]
    manifest = SourceInclusionManifest.model_validate(_make_manifest_data(entries=entries))
    result = verify_source_provenance(manifest, source_root=tmp_path)
    assert not result.ok
    codes = {f.code for f in result.failures}
    assert ProvenanceFailureCode.SYMLINK_DETECTED in codes


@pytest.mark.integration
def test_verify_rejects_symlinked_file_in_subdirectory(tmp_path: Path):
    sub = tmp_path / "src"
    sub.mkdir()
    real_file = tmp_path / "real.py"
    real_file.write_bytes(b"# real\n")
    link = sub / "link.py"
    link.symlink_to(real_file)
    entries = [_entry_for("src/link.py", b"# real\n")]
    manifest = SourceInclusionManifest.model_validate(_make_manifest_data(entries=entries))
    result = verify_source_provenance(manifest, source_root=tmp_path)
    assert not result.ok
    codes = {f.code for f in result.failures}
    assert ProvenanceFailureCode.SYMLINK_DETECTED in codes


# ---------------------------------------------------------------------------
# Integration: changed file (checksum mismatch) fails
# ---------------------------------------------------------------------------

@pytest.mark.integration
def test_verify_rejects_changed_file_checksum_mismatch(tmp_path: Path):
    files = {"src/main.py": b"# changed content\n"}
    _write_source_tree(tmp_path, files)
    entries = [_entry_for("src/main.py", b"# original content\n")]
    manifest = SourceInclusionManifest.model_validate(_make_manifest_data(entries=entries))
    result = verify_source_provenance(manifest, source_root=tmp_path)
    assert not result.ok
    codes = {f.code for f in result.failures}
    assert ProvenanceFailureCode.CHECKSUM_MISMATCH in codes


# ---------------------------------------------------------------------------
# Integration: byte length mismatch fails
# ---------------------------------------------------------------------------

@pytest.mark.integration
def test_verify_rejects_byte_length_mismatch(tmp_path: Path):
    content = b"# main\n"
    _write_source_tree(tmp_path, {"src/main.py": content})
    entries = [{
        "path": "src/main.py",
        "sha256": hashlib.sha256(content).hexdigest(),
        "byte_length": 999,
    }]
    manifest = SourceInclusionManifest.model_validate(_make_manifest_data(entries=entries))
    result = verify_source_provenance(manifest, source_root=tmp_path)
    assert not result.ok
    codes = {f.code for f in result.failures}
    assert ProvenanceFailureCode.BYTE_LENGTH_MISMATCH in codes


# ---------------------------------------------------------------------------
# Integration: multiple failures are all reported
# ---------------------------------------------------------------------------

@pytest.mark.integration
def test_verify_reports_multiple_failures(tmp_path: Path):
    files = {"src/present.py": b"# present\n", "src/extra.py": b"# extra\n"}
    _write_source_tree(tmp_path, files)
    entries = [
        _entry_for("src/present.py", b"# wrong content\n"),
        _entry_for("src/missing.py", b"# missing\n"),
    ]
    manifest = SourceInclusionManifest.model_validate(_make_manifest_data(entries=entries))
    result = verify_source_provenance(manifest, source_root=tmp_path)
    assert not result.ok
    codes = {f.code for f in result.failures}
    assert ProvenanceFailureCode.CHECKSUM_MISMATCH in codes
    assert ProvenanceFailureCode.MISSING_FILE in codes
    assert ProvenanceFailureCode.ESCAPING_FILE in codes


# ---------------------------------------------------------------------------
# Integration: nested directories
# ---------------------------------------------------------------------------

@pytest.mark.integration
def test_verify_accepts_nested_directories(tmp_path: Path):
    files = {
        "src/main.py": b"# main\n",
        "src/deep/nested/mod.py": b"# mod\n",
        "tests/test_main.py": b"# test\n",
    }
    _write_source_tree(tmp_path, files)
    entries = [_entry_for(rel, content) for rel, content in files.items()]
    manifest = SourceInclusionManifest.model_validate(_make_manifest_data(entries=entries))
    result = verify_source_provenance(manifest, source_root=tmp_path)
    assert result.ok, [f"{f.code}: {f.message}" for f in result.failures]


# ---------------------------------------------------------------------------
# Integration: empty source root with non-empty manifest fails
# ---------------------------------------------------------------------------

@pytest.mark.integration
def test_verify_rejects_empty_source_root_with_manifest_entries(tmp_path: Path):
    entries = [_entry_for("src/main.py", b"# main\n")]
    manifest = SourceInclusionManifest.model_validate(_make_manifest_data(entries=entries))
    result = verify_source_provenance(manifest, source_root=tmp_path)
    assert not result.ok
    codes = {f.code for f in result.failures}
    assert ProvenanceFailureCode.MISSING_FILE in codes


# ---------------------------------------------------------------------------
# Integration: manifest hash is verified against computed hash
# ---------------------------------------------------------------------------

@pytest.mark.integration
def test_verify_accepts_correct_manifest_hash(tmp_path: Path):
    files = {"src/main.py": b"# main\n"}
    _write_source_tree(tmp_path, files)
    entries = [_entry_for("src/main.py", b"# main\n")]
    correct_hash = compute_manifest_hash(entries)
    manifest = SourceInclusionManifest.model_validate(
        _make_manifest_data(entries=entries, manifest_hash=correct_hash)
    )
    result = verify_source_provenance(manifest, source_root=tmp_path)
    assert result.ok, [f"{f.code}: {f.message}" for f in result.failures]


# ---------------------------------------------------------------------------
# Integration: result carries manifest hash and checked layers
# ---------------------------------------------------------------------------

@pytest.mark.integration
def test_verify_result_carries_manifest_hash_and_checked_layers(tmp_path: Path):
    files = {"src/main.py": b"# main\n"}
    _write_source_tree(tmp_path, files)
    entries = [_entry_for("src/main.py", b"# main\n")]
    manifest = SourceInclusionManifest.model_validate(_make_manifest_data(entries=entries))
    result = verify_source_provenance(manifest, source_root=tmp_path)
    assert result.ok
    assert result.manifest_hash == manifest.manifest_hash
    assert len(result.checked_layers) > 0
    assert "manifest_hash" in result.checked_layers
    assert "duplicate_paths" in result.checked_layers
    assert "path_traversal" in result.checked_layers
    assert "symlink_scan" in result.checked_layers
    assert "missing_files" in result.checked_layers
    assert "escaping_files" in result.checked_layers
    assert "checksum_verification" in result.checked_layers


# ---------------------------------------------------------------------------
# Integration: traversal path in manifest fails even if file exists
# ---------------------------------------------------------------------------

@pytest.mark.integration
def test_verify_rejects_traversal_path_with_existing_file(tmp_path: Path):
    outside = tmp_path / "outside.txt"
    outside.write_bytes(b"outside\n")
    entries = [_entry_for("../outside.txt", b"outside\n")]
    manifest = SourceInclusionManifest.model_validate(_make_manifest_data(entries=entries))
    result = verify_source_provenance(manifest, source_root=tmp_path)
    assert not result.ok
    codes = {f.code for f in result.failures}
    assert ProvenanceFailureCode.PATH_TRAVERSAL in codes
