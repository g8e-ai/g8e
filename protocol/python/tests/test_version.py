"""Tests for g8e version consistency between __init__.py and pyproject.toml."""

import re
from pathlib import Path

import g8e


def _read_pyproject_version() -> str:
    pyproject = Path(__file__).parent.parent / "pyproject.toml"
    text = pyproject.read_text(encoding="utf-8")
    match = re.search(r'^version\s*=\s*"([^"]+)"', text, re.MULTILINE)
    assert match, "Could not find version in pyproject.toml"
    return match.group(1)


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
