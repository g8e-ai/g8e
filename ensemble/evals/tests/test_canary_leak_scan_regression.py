# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

"""Tier 2 integration regression tests for the canary leak scan.

These tests verify the P1-2 fix for ``_scan_report_for_canary_leaks``: no
public report artifact may contain raw canary values (synthetic sensitive
values) or the per-run decryption key (raw bytes or hex encoding).  The
scan enumerates every file in the report tree recursively, reads its
bytes, and raises ``EvaluationRunError`` if any leak is found.

The tests use ``tmp_path`` to create isolated report directories with
planted leak files, then call ``_scan_report_for_canary_leaks`` directly
to verify fail-closed behavior.
"""

from __future__ import annotations

from pathlib import Path

import pytest

from g8e_evals.cli import EvaluationRunError, _scan_report_for_canary_leaks


pytestmark = pytest.mark.integration

_KEY = b"k" * 32
_CANARY_VALUES = {"secret-token-abc123", "expired-token-xyz789"}


def test_clean_report_does_not_raise(tmp_path: Path) -> None:
    """A report tree with no canary values or key bytes does not raise."""
    (tmp_path / "manifest.json").write_text('{"run_id": "clean-run"}')
    (tmp_path / "metrics.jsonl").write_text('{"metric_id": "test", "value": 1.0}\n')
    sub = tmp_path / "evidence"
    sub.mkdir()
    (sub / "abc123.json").write_text('{"observation": "safe"}')

    _scan_report_for_canary_leaks(tmp_path, _CANARY_VALUES, _KEY)


def test_raw_canary_value_in_report_file_raises(tmp_path: Path) -> None:
    """A raw canary value in any report file causes ``EvaluationRunError``."""
    (tmp_path / "manifest.json").write_text('{"run_id": "leaky-run"}')
    (tmp_path / "observations.jsonl").write_text(
        '{"token": "secret-token-abc123"}\n'
    )

    with pytest.raises(EvaluationRunError, match="raw canary"):
        _scan_report_for_canary_leaks(tmp_path, _CANARY_VALUES, _KEY)


def test_raw_canary_value_in_nested_file_raises(tmp_path: Path) -> None:
    """A raw canary value in a nested report file causes ``EvaluationRunError``."""
    nested = tmp_path / "evidence" / "deep" / "deeper"
    nested.mkdir(parents=True)
    (nested / "artifact.json").write_text(
        '{"value": "expired-token-xyz789"}'
    )

    with pytest.raises(EvaluationRunError, match="raw canary"):
        _scan_report_for_canary_leaks(tmp_path, _CANARY_VALUES, _KEY)


def test_per_run_key_bytes_in_report_file_raises(tmp_path: Path) -> None:
    """Raw per-run key bytes in any report file cause ``EvaluationRunError``."""
    (tmp_path / "manifest.json").write_text('{"run_id": "key-leak-run"}')
    (tmp_path / "config.json").write_bytes(b'{"key": ' + _KEY + b"}")

    with pytest.raises(EvaluationRunError, match="per-run key bytes"):
        _scan_report_for_canary_leaks(tmp_path, _CANARY_VALUES, _KEY)


def test_per_run_key_hex_in_report_file_raises(tmp_path: Path) -> None:
    """Hex-encoded per-run key in any report file causes ``EvaluationRunError``."""
    (tmp_path / "manifest.json").write_text('{"run_id": "hex-key-leak"}')
    key_hex = _KEY.hex()
    (tmp_path / "debug.log").write_text(f"debug: key={key_hex}\n")

    with pytest.raises(EvaluationRunError, match="per-run key hex"):
        _scan_report_for_canary_leaks(tmp_path, _CANARY_VALUES, _KEY)


def test_multiple_leaks_are_all_reported(tmp_path: Path) -> None:
    """Multiple leaks in different files are all reported in the error message."""
    (tmp_path / "file1.json").write_text('{"token": "secret-token-abc123"}')
    (tmp_path / "file2.json").write_text('{"token": "expired-token-xyz789"}')

    with pytest.raises(EvaluationRunError) as exc_info:
        _scan_report_for_canary_leaks(tmp_path, _CANARY_VALUES, _KEY)

    message = str(exc_info.value)
    assert "secret-token-abc123" in message
    assert "expired-token-xyz789" in message


def test_empty_canary_set_does_not_raise(tmp_path: Path) -> None:
    """An empty canary set is a no-op: the scan returns without checking."""
    (tmp_path / "manifest.json").write_text('{"run_id": "no-canaries"}')

    _scan_report_for_canary_leaks(tmp_path, set(), _KEY)


def test_canary_as_substring_still_detected(tmp_path: Path) -> None:
    """A canary value appearing as a substring within a larger string is detected."""
    (tmp_path / "report.md").write_text(
        "The token value was secret-token-abc123 and it leaked."
    )

    with pytest.raises(EvaluationRunError, match="raw canary"):
        _scan_report_for_canary_leaks(tmp_path, _CANARY_VALUES, _KEY)
