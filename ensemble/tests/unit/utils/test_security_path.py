# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

from __future__ import annotations

from pathlib import Path

import pytest

from app.utils.security import (
    resolve_safe_path_segments,
    validate_safe_filename,
    validate_safe_path,
)


def test_validate_safe_filename_accepts_simple_segment() -> None:
    assert validate_safe_filename("assignment-1", label="assignment_id") == "assignment-1"


@pytest.mark.parametrize(
    ("value", "label"),
    [
        ("", "assignment_id"),
        (".", "assignment_id"),
        ("..", "assignment_id"),
        ("../etc", "assignment_id"),
        ("foo/bar", "assignment_id"),
        ("foo\\bar", "assignment_id"),
    ],
)
def test_validate_safe_filename_rejects_unsafe_segments(value: str, label: str) -> None:
    with pytest.raises(ValueError, match=f"invalid {label}"):
        validate_safe_filename(value, label=label)


def test_validate_safe_path_resolves_under_root(tmp_path: Path) -> None:
    resolved = validate_safe_path("assignment-1/attempt-1.json", tmp_path)
    assert resolved == (tmp_path / "assignment-1" / "attempt-1.json").resolve()


def test_validate_safe_path_rejects_traversal(tmp_path: Path) -> None:
    with pytest.raises(ValueError, match="Path traversal detected"):
        validate_safe_path("../outside.json", tmp_path)


def test_resolve_safe_path_segments_resolves_under_root(tmp_path: Path) -> None:
    resolved = resolve_safe_path_segments(
        tmp_path,
        "assignment-1",
        "attempt-1.json",
        segment_labels=("assignment_id", "evaluation_attempt_id"),
    )
    assert resolved == (tmp_path / "assignment-1" / "attempt-1.json").resolve()


def test_resolve_safe_path_segments_rejects_unsafe_assignment_id(tmp_path: Path) -> None:
    with pytest.raises(ValueError, match="invalid assignment_id"):
        resolve_safe_path_segments(
            tmp_path,
            "../etc",
            "attempt-1.json",
            segment_labels=("assignment_id", "evaluation_attempt_id"),
        )


def test_resolve_safe_path_segments_rejects_unsafe_attempt_id(tmp_path: Path) -> None:
    with pytest.raises(ValueError, match="invalid evaluation_attempt_id"):
        resolve_safe_path_segments(
            tmp_path,
            "assignment-1",
            "../secret.json",
            segment_labels=("assignment_id", "evaluation_attempt_id"),
        )
