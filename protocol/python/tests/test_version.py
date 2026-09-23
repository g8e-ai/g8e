# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

"""Tests for g8e version consistency across package and protocol documentation."""

import re
from pathlib import Path

import g8e


PROTOCOL_DOCUMENTS = ("a2a.md", "constants.md", "mcp.md", "spec.md")


def _read_pyproject_version() -> str:
    pyproject = Path(__file__).parent.parent / "pyproject.toml"
    text = pyproject.read_text(encoding="utf-8")
    match = re.search(r'^version\s*=\s*"([^"]+)"', text, re.MULTILINE)
    assert match, "Could not find version in pyproject.toml"
    return match.group(1)


def _read_repository_version() -> str:
    version_file = Path(__file__).parents[3] / "VERSION"
    return version_file.read_text(encoding="utf-8").strip().removeprefix("v")


class TestVersionConsistency:
    """Verify version is consistent across all sources."""

    def test_init_version_is_string(self):
        assert isinstance(g8e.__version__, str)

    def test_init_version_non_empty(self):
        assert len(g8e.__version__) > 0

    def test_init_version_matches_pyproject(self):
        pyproject_version = _read_pyproject_version()
        assert g8e.__version__ == pyproject_version, (
            f"__init__.py version ({g8e.__version__}) != "
            f"pyproject.toml version ({pyproject_version})"
        )

    def test_version_is_semver(self):
        version = g8e.__version__
        # Basic semver pattern: X.Y.Z with optional pre-release suffix
        assert re.match(r"^\d+\.\d+\.\d+", version), (
            f"Version '{version}' does not look like semver"
        )

    def test_protocol_documentation_matches_repository_version(self):
        expected_version = _read_repository_version()
        protocol_docs = Path(__file__).parents[2] / "docs"
        for name in PROTOCOL_DOCUMENTS:
            path = protocol_docs / name
            text = path.read_text(encoding="utf-8")
            match = re.search(r"^Version: v([^\n]+)$", text, re.MULTILINE)
            assert match, f"Could not find a version in {path}"
            assert match.group(1) == expected_version, (
                f"{path} version ({match.group(1)}) != "
                f"repository version ({expected_version})"
            )
