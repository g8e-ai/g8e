# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

"""Regression tests for the local artifact emitter P1-2 fixes.

Two defects were fixed in ``LocalArtifactEmitter``:

1. ``scan_artifact`` now counts every marker occurrence (using
   ``str.count``) rather than incrementing once per sensitive type, so
   repeated leaks are fully counted in ``sensitive_occurrences``.

2. ``_validate_artifact_class`` rejects empty strings, path separators
   (``/``, ``\\``), traversal sequences (``..``), and null bytes in
   ``artifact_class``, preventing path traversal outside the collection
   root.  Both ``emit_artifact`` and ``artifact_path`` validate the
   artifact class.

The validation tests are Tier 1 (pure function, no filesystem).  The
scanning and emission tests are Tier 2 (real filesystem via
``tmp_path``).
"""

from __future__ import annotations

from pathlib import Path

import pytest

from g8e_evals.benchmarks.privacy.artifact_emitter import (
    LocalArtifactEmitter,
    _validate_artifact_class,
)
from g8e_evals.schema import SensitiveArtifactContentType


# ---------------------------------------------------------------------------
# Tier 1: _validate_artifact_class path traversal rejection (pure function)
# ---------------------------------------------------------------------------


@pytest.mark.unit
def test_validate_artifact_class_rejects_empty_string() -> None:
    """An empty artifact class is rejected."""
    with pytest.raises(ValueError, match="must not be empty"):
        _validate_artifact_class("")


@pytest.mark.unit
def test_validate_artifact_class_rejects_forward_slash() -> None:
    """A forward slash in the artifact class is rejected."""
    with pytest.raises(ValueError, match="path separators"):
        _validate_artifact_class("leak/../../etc/passwd")


@pytest.mark.unit
def test_validate_artifact_class_rejects_backslash() -> None:
    """A backslash in the artifact class is rejected."""
    with pytest.raises(ValueError, match="path separators"):
        _validate_artifact_class("leak\\..\\..\\windows")


@pytest.mark.unit
def test_validate_artifact_class_rejects_dot_dot() -> None:
    """A bare ``..`` traversal sequence is rejected."""
    with pytest.raises(ValueError, match="traversal sequences"):
        _validate_artifact_class("..")


@pytest.mark.unit
def test_validate_artifact_class_rejects_embedded_dot_dot() -> None:
    """An embedded ``..`` traversal sequence is rejected."""
    with pytest.raises(ValueError, match="traversal sequences"):
        _validate_artifact_class("safe..traversal")


@pytest.mark.unit
def test_validate_artifact_class_rejects_null_byte() -> None:
    """A null byte in the artifact class is rejected."""
    with pytest.raises(ValueError, match="null bytes"):
        _validate_artifact_class("safe\x00evil")


@pytest.mark.unit
def test_validate_artifact_class_accepts_safe_name() -> None:
    """A safe single-element artifact class is accepted and returned unchanged."""
    assert _validate_artifact_class("public-export") == "public-export"


# ---------------------------------------------------------------------------
# Tier 2: scan_artifact repeated marker counting (real filesystem)
# ---------------------------------------------------------------------------


@pytest.mark.integration
def test_scan_artifact_counts_repeated_marker_occurrences(tmp_path: Path) -> None:
    """Repeated marker occurrences are fully counted, not just one per type.

    The artifact content contains the same sensitive marker three times.
    ``scan_artifact`` returns ``sensitive_occurrences=3``, not 1.
    """
    emitter = LocalArtifactEmitter(tmp_path)
    artifact_path = emitter.emit_artifact(
        "repeated-leak",
        '{"public": "safe"}',
        leak_types=[SensitiveArtifactContentType.RAW_CANARY],
    )

    # The emit_artifact method adds one marker per leak type.  To test
    # repeated occurrences, we write an artifact with multiple copies of
    # the same marker directly.
    marker = "RAW_CANARY:"
    content = (
        '{"content": "'
        + marker + 'first", '
        + marker + 'second", '
        + marker + 'third"}'
    )
    artifact_path.write_text(content)

    present, sha, byte_length, occurrences, types_found = emitter.scan_artifact(artifact_path)
    assert present is True
    assert sha != ""
    assert byte_length > 0
    assert occurrences == 3
    assert SensitiveArtifactContentType.RAW_CANARY in types_found


@pytest.mark.integration
def test_scan_artifact_counts_multiple_types_with_repeats(tmp_path: Path) -> None:
    """Multiple sensitive types with repeated occurrences are all counted.

    The artifact contains two ``RAW_CANARY:`` markers and one
    ``CREDENTIAL:`` marker.  ``scan_artifact`` returns
    ``sensitive_occurrences=3`` and both types in ``types_found``.
    """
    emitter = LocalArtifactEmitter(tmp_path)
    content = (
        '{"content": "'
        "RAW_CANARY:first RAW_CANARY:second CREDENTIAL:third"
        '"}'
    )
    artifact_path = tmp_path / "multi-type.json"
    artifact_path.write_text(content)

    present, _sha, _byte_length, occurrences, types_found = emitter.scan_artifact(artifact_path)
    assert present is True
    assert occurrences == 3
    assert SensitiveArtifactContentType.RAW_CANARY in types_found
    assert SensitiveArtifactContentType.CREDENTIAL in types_found


@pytest.mark.integration
def test_scan_artifact_zero_occurrences_for_clean_artifact(tmp_path: Path) -> None:
    """A clean artifact with no sensitive markers returns zero occurrences."""
    emitter = LocalArtifactEmitter(tmp_path)
    artifact_path = emitter.emit_artifact(
        "clean-export",
        '{"public": "sha256:abc123"}',
    )

    present, _sha, _byte_length, occurrences, types_found = emitter.scan_artifact(artifact_path)
    assert present is True
    assert occurrences == 0
    assert types_found == []


@pytest.mark.integration
def test_scan_artifact_missing_file_returns_absent(tmp_path: Path) -> None:
    """A missing artifact file returns ``present=False`` with zeroed fields."""
    emitter = LocalArtifactEmitter(tmp_path)
    missing = tmp_path / "does-not-exist.json"

    present, sha, byte_length, occurrences, types_found = emitter.scan_artifact(missing)
    assert present is False
    assert sha == ""
    assert byte_length == 0
    assert occurrences == 0
    assert types_found == []


# ---------------------------------------------------------------------------
# Tier 2: emit_artifact and artifact_path path traversal rejection
# ---------------------------------------------------------------------------


@pytest.mark.integration
def test_emit_artifact_rejects_path_traversal_via_artifact_class(
    tmp_path: Path,
) -> None:
    """``emit_artifact`` rejects a path-traversal artifact class."""
    emitter = LocalArtifactEmitter(tmp_path)
    with pytest.raises(ValueError, match="traversal sequences"):
        emitter.emit_artifact("..", '{"content": "evil"}')


@pytest.mark.integration
def test_emit_artifact_rejects_path_separator_in_artifact_class(
    tmp_path: Path,
) -> None:
    """``emit_artifact`` rejects a path separator in the artifact class."""
    emitter = LocalArtifactEmitter(tmp_path)
    with pytest.raises(ValueError, match="path separators"):
        emitter.emit_artifact("sub/dir", '{"content": "evil"}')


@pytest.mark.integration
def test_artifact_path_rejects_path_traversal(tmp_path: Path) -> None:
    """``artifact_path`` rejects a path-traversal artifact class."""
    emitter = LocalArtifactEmitter(tmp_path)
    with pytest.raises(ValueError, match="traversal sequences"):
        emitter.artifact_path("..")


@pytest.mark.integration
def test_artifact_path_rejects_empty_class(tmp_path: Path) -> None:
    """``artifact_path`` rejects an empty artifact class."""
    emitter = LocalArtifactEmitter(tmp_path)
    with pytest.raises(ValueError, match="must not be empty"):
        emitter.artifact_path("")


@pytest.mark.integration
def test_emit_artifact_writes_within_collection_root(tmp_path: Path) -> None:
    """A safe artifact class writes within the collection root directory."""
    emitter = LocalArtifactEmitter(tmp_path)
    path = emitter.emit_artifact("safe-export", '{"public": "ok"}')
    assert path == tmp_path / "safe-export.json"
    assert path.is_file()
    # The file must be within the base directory, not outside it.
    assert tmp_path in path.resolve().parents or path.resolve() == tmp_path.resolve()
