# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

"""Integration tests for supported CLI authentication commands.

Verifies the supported CLI authentication entrypoints (``./g8e auth enroll user``,
``./g8e auth refresh``, and ``./g8e auth context``) behave as expected when invoked
against empty or isolated runtime environments.
"""
from __future__ import annotations

import subprocess
from pathlib import Path

import pytest

pytestmark = pytest.mark.integration

REPO_ROOT = Path(__file__).resolve().parent.parent.parent.parent
G8E_BIN = REPO_ROOT / "g8e"


def test_auth_context_in_empty_runtime_fails_closed(tmp_path: Path):
    """./g8e auth context in a directory with no runtime credentials must fail closed."""
    proc = subprocess.run(
        [str(G8E_BIN), "auth", "context", "--project-root", str(tmp_path)],
        capture_output=True,
        text=True,
        check=False,
    )
    assert proc.returncode != 0
    assert "not authenticated" in proc.stderr.lower() or "error" in proc.stderr.lower()


def test_auth_help_describes_supported_commands():
    """./g8e auth --help must advertise enroll, refresh, and context commands."""
    proc = subprocess.run(
        [str(G8E_BIN), "auth", "--help"],
        capture_output=True,
        text=True,
        check=True,
    )
    assert "enroll" in proc.stdout
    assert "refresh" in proc.stdout
    assert "context" in proc.stdout
    assert "login" not in proc.stdout

