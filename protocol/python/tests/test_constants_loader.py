"""Tests for the fail-closed protocol constants loader (E.1, E.8).

Covers:
- ``_get_protocol_dir`` treats an empty ``G8E_PROTOCOL_DIR`` as unset so a
  stray ``G8E_PROTOCOL_DIR=`` line in ``.env`` cannot shadow the bundled
  ``_data/`` bundle (the E.1 root cause).
- ``_get_protocol_dir`` has no dev-mode fallbacks — only the explicit env var
  and the bundled ``_data/`` path are consulted (E.8).
- ``_load_protocol_json`` raises :class:`ProtocolConstantsError` when a
  required constant file is missing or empty, rather than returning ``{}``
  and letting downstream code raise an opaque ``KeyError``.

These tests call ``_get_protocol_dir()`` and ``_load_protocol_json()``
directly with patched env vars / module attributes. No ``importlib.reload``
is used because reload creates a new ``ProtocolConstantsError`` class object,
which breaks ``pytest.raises(ProtocolConstantsError)`` identity checks.
"""

import json

import pytest

from g8e import constants as constants_module
from g8e.constants import ProtocolConstantsError


def _clear_protocol_env(monkeypatch: pytest.MonkeyPatch):
    """Remove the G8E_PROTOCOL_DIR env var so the bundled _data/ path wins."""
    monkeypatch.delenv("G8E_PROTOCOL_DIR", raising=False)


class TestGetProtocolDirEmptyEnvVar:
    """An empty G8E_PROTOCOL_DIR must be treated as unset (E.1 root cause)."""

    def test_empty_protocol_dir_falls_back_to_bundled_data(self, monkeypatch):
        _clear_protocol_env(monkeypatch)
        monkeypatch.setenv("G8E_PROTOCOL_DIR", "")
        resolved = constants_module._get_protocol_dir()
        # The bundled _data/ path inside the installed package must win over
        # the empty env var (not Path("constants") which was the E.1 bug).
        bundled = constants_module.Path(constants_module.__file__).parent / "_data"
        assert resolved == bundled

    def test_whitespace_only_protocol_dir_falls_back_to_bundled_data(self, monkeypatch):
        _clear_protocol_env(monkeypatch)
        monkeypatch.setenv("G8E_PROTOCOL_DIR", "   ")
        resolved = constants_module._get_protocol_dir()
        bundled = constants_module.Path(constants_module.__file__).parent / "_data"
        assert resolved == bundled

    def test_nonempty_protocol_dir_uses_env_var(self, monkeypatch, tmp_path):
        _clear_protocol_env(monkeypatch)
        custom = tmp_path / "custom-protocol"
        monkeypatch.setenv("G8E_PROTOCOL_DIR", str(custom))
        resolved = constants_module._get_protocol_dir()
        assert resolved == custom / "constants"


class TestGetProtocolDirNoDevModeFallbacks:
    """_get_protocol_dir must not probe source-tree or container paths (E.8).

    The old four-level fallback chain (env var, _data/, source-tree relative,
    /app/protocol/constants) could silently resolve to a stale or empty path
    in a container. The dev-mode fallbacks were removed entirely — only the
    explicit env var and the bundled _data/ are consulted.
    """

    def test_unset_env_var_always_returns_bundled_data(self, monkeypatch):
        _clear_protocol_env(monkeypatch)
        resolved = constants_module._get_protocol_dir()
        bundled = constants_module.Path(constants_module.__file__).parent / "_data"
        assert resolved == bundled

    def test_no_source_tree_fallback_when_bundled_data_absent(self, monkeypatch, tmp_path):
        """When the bundled _data/ is absent and no env var is set, the loader
        returns the (nonexistent) bundled path rather than probing the source
        tree. _load_protocol_json then fails closed with ProtocolConstantsError."""
        _clear_protocol_env(monkeypatch)
        # Point __file__ at a temp location with no _data/ sibling.
        fake_pkg = tmp_path / "protocol" / "python" / "g8e"
        fake_pkg.mkdir(parents=True)
        (fake_pkg / "constants.py").write_text("")
        # A source-tree constants/ dir exists at the old fallback location —
        # the loader must NOT resolve to it.
        source_constants = tmp_path / "protocol" / "constants"
        source_constants.mkdir(parents=True)
        (source_constants / "status.json").write_text(json.dumps({"status": {}}))
        monkeypatch.setattr(constants_module, "__file__", str(fake_pkg / "constants.py"))
        resolved = constants_module._get_protocol_dir()
        # The resolved path is the nonexistent bundled _data/, not the source tree.
        assert resolved == fake_pkg / "_data"
        assert resolved != source_constants


class TestLoadProtocolJsonFailClosed:
    """_load_protocol_json must raise on missing/empty/malformed files (E.1).

    These tests patch ``_PROTOCOL_CONSTANTS_DIR`` directly (no reload) so the
    module-level constant dicts are not disturbed. The function is tested in
    isolation against a temp directory with a single bad file.
    """

    def test_missing_file_raises(self, monkeypatch, tmp_path):
        empty_dir = tmp_path / "empty-protocol"
        empty_dir.mkdir(parents=True)
        monkeypatch.setattr(constants_module, "_PROTOCOL_CONSTANTS_DIR", empty_dir)
        with pytest.raises(ProtocolConstantsError, match="not found"):
            constants_module._load_protocol_json("nonexistent.json")

    def test_empty_file_raises(self, monkeypatch, tmp_path):
        bad_dir = tmp_path / "empty-file-protocol"
        bad_dir.mkdir(parents=True)
        (bad_dir / "status.json").write_text("")
        monkeypatch.setattr(constants_module, "_PROTOCOL_CONSTANTS_DIR", bad_dir)
        with pytest.raises(ProtocolConstantsError, match="malformed JSON"):
            constants_module._load_protocol_json("status.json")

    def test_empty_json_object_raises(self, monkeypatch, tmp_path):
        bad_dir = tmp_path / "empty-object-protocol"
        bad_dir.mkdir(parents=True)
        (bad_dir / "status.json").write_text("{}")
        monkeypatch.setattr(constants_module, "_PROTOCOL_CONSTANTS_DIR", bad_dir)
        with pytest.raises(ProtocolConstantsError, match="empty"):
            constants_module._load_protocol_json("status.json")

    def test_malformed_json_raises(self, monkeypatch, tmp_path):
        bad_dir = tmp_path / "malformed-protocol"
        bad_dir.mkdir(parents=True)
        (bad_dir / "status.json").write_text("{not valid json")
        monkeypatch.setattr(constants_module, "_PROTOCOL_CONSTANTS_DIR", bad_dir)
        with pytest.raises(ProtocolConstantsError, match="malformed JSON"):
            constants_module._load_protocol_json("status.json")

    def test_valid_file_returns_data(self, monkeypatch, tmp_path):
        good_dir = tmp_path / "good-protocol"
        good_dir.mkdir(parents=True)
        payload = {"status": {"action_status": {"foo": {"value": "bar", "_python_const": "FOO"}}}}
        (good_dir / "status.json").write_text(json.dumps(payload))
        monkeypatch.setattr(constants_module, "_PROTOCOL_CONSTANTS_DIR", good_dir)
        data = constants_module._load_protocol_json("status.json")
        assert data == payload


class TestStatusBundleIntegrity:
    """STATUS['status'] must be non-empty with expected categories (E.1 guard)."""

    def test_status_has_expected_categories(self):
        from g8e.constants import STATUS
        cats = STATUS["status"]
        expected = {"action_status", "action_type"}
        assert expected.issubset(cats.keys()), (
            f"STATUS['status'] missing expected categories {expected - cats.keys()}"
        )
