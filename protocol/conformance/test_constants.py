"""Cross-language protocol constant conformance tests.

Validates that the JSON files in protocol/constants/ (the shared source of truth)
have the required metadata fields for both Go and Python code generation:

- Every entry in every constant file has a ``_go_const`` field (Go mirror name).
- Every entry in status.json has a ``_python_const`` field (Python enum member name).
- Values are unique within each category (no duplicate wire formats).
- Python-loaded constants match the raw JSON values exactly.
- Event values follow the ``g8e.v1.*`` namespace convention.
- Go constant names follow PascalCase naming conventions with consistent prefixes.
"""

from __future__ import annotations

import json
import re
from pathlib import Path
from typing import Any

import pytest

# ---------------------------------------------------------------------------
# Helpers
# ---------------------------------------------------------------------------

CONSTANTS_DIR = Path(__file__).parent.parent / "constants"


def _load_json(filename: str) -> dict[str, Any]:
    path = CONSTANTS_DIR / filename
    assert path.exists(), f"Protocol JSON file not found: {path}"
    with open(path, encoding="utf-8") as f:
        return json.load(f)


def _iter_entries(data: dict[str, Any], root_key: str) -> dict[str, dict[str, Any]]:
    """Extract the inner entries dict from a top-level wrapper key."""
    assert root_key in data, f"Missing top-level key '{root_key}' in JSON"
    entries = data[root_key]
    assert isinstance(entries, dict), f"'{root_key}' must be a dict"
    return entries


# ---------------------------------------------------------------------------
# File classification
# ---------------------------------------------------------------------------

# Files with a single-level wrapper key containing entries
WRAPPER_KEY_FILES = {
    "events.json": "events",
    "senders.json": "senders",
    "collections.json": "collections",
    "kv_keys.json": "kv_keys",
    "channels.json": "channels",
    "pubsub.json": "pubsub",
    "intents.json": "intents",
    "prompts.json": "prompts",
    "timestamp.json": "timestamp",
    "headers.json": "headers",
    "document_ids.json": "document_ids",
    "platform.json": "platform",
    "agents.json": "agents",
    "network.json": "network",
    "exit_codes.json": "exit_codes",
    "env_vars.json": "env_vars",
    "field_paths.json": "field_paths",
    "output.json": "output",
    "ports.json": "ports",
}

# status.json has a nested structure: status -> category -> entries
STATUS_FILE = "status.json"

# api_paths.json is a flat structure with no wrapper key
API_PATHS_FILE = "api_paths.json"

# Files where entries have _go_const (exit_codes uses a different schema)
FILES_WITH_GO_CONST = set(WRAPPER_KEY_FILES.keys()) - {"exit_codes.json"}

# Files where entries have a 'value' field (field_paths uses allowed_paths/forbidden_paths)
FILES_WITH_VALUE = set(WRAPPER_KEY_FILES.keys()) - {"field_paths.json"}

# auth.json has multiple wrapper keys, each containing entries with _go_const and value
AUTH_WRAPPER_KEYS = [
    "passkey_purposes",
    "webauthn_types",
    "webauthn_algorithms",
    "webauthn_attestation",
    "webauthn_requirements",
    "pki_leaf_types",
    "context_keys",
]

# Files where value uniqueness is enforced (events allow aliases)
FILES_WITH_UNIQUE_VALUES = set(WRAPPER_KEY_FILES.keys()) - {"events.json"}


# ---------------------------------------------------------------------------
# Tests: JSON structural integrity
# ---------------------------------------------------------------------------


class TestJSONStructuralIntegrity:
    """Verify all protocol JSON files are valid and have expected structure."""

    @pytest.mark.parametrize(
        "filename",
        list(WRAPPER_KEY_FILES.keys()) + [STATUS_FILE, API_PATHS_FILE, "auth.json"],
    )
    def test_json_file_loads_successfully(self, filename: str):
        data = _load_json(filename)
        assert isinstance(data, dict), f"{filename} must be a JSON object"

    @pytest.mark.parametrize("filename", list(WRAPPER_KEY_FILES.keys()))
    def test_json_file_has_wrapper_key(self, filename: str):
        data = _load_json(filename)
        wrapper_key = WRAPPER_KEY_FILES[filename]
        assert wrapper_key in data, f"{filename} missing top-level key '{wrapper_key}'"

    @pytest.mark.parametrize("filename", list(WRAPPER_KEY_FILES.keys()))
    def test_json_file_has_entries(self, filename: str):
        data = _load_json(filename)
        wrapper_key = WRAPPER_KEY_FILES[filename]
        entries = _iter_entries(data, wrapper_key)
        assert len(entries) > 0, f"{filename} has no entries under '{wrapper_key}'"

    def test_status_has_categories(self):
        data = _load_json(STATUS_FILE)
        assert "status" in data
        categories = data["status"]
        assert isinstance(categories, dict)
        assert len(categories) > 0, "status.json has no categories"

    def test_api_paths_has_entries(self):
        data = _load_json(API_PATHS_FILE)
        assert len(data) > 0, "api_paths.json is empty"


# ---------------------------------------------------------------------------
# Tests: _go_const field presence (Go cross-language parity)
# ---------------------------------------------------------------------------


class TestGoConstPresence:
    """Entries in applicable constant files must have a _go_const field."""

    @pytest.mark.parametrize("filename", sorted(FILES_WITH_GO_CONST))
    def test_all_entries_have_go_const(self, filename: str):
        data = _load_json(filename)
        wrapper_key = WRAPPER_KEY_FILES[filename]
        entries = _iter_entries(data, wrapper_key)

        missing = []
        for key, meta in entries.items():
            if isinstance(meta, dict):
                if "_go_const" not in meta:
                    missing.append(key)
        assert not missing, (
            f"{filename}: entries missing _go_const: {missing}"
        )

    def test_status_entries_have_go_const(self):
        data = _load_json(STATUS_FILE)
        status = _iter_entries(data, "status")

        missing = []
        for cat_name, cat_vals in status.items():
            if not isinstance(cat_vals, dict):
                continue
            for key, meta in cat_vals.items():
                if isinstance(meta, dict):
                    if "_go_const" not in meta:
                        missing.append(f"{cat_name}/{key}")
        assert not missing, (
            f"status.json: entries missing _go_const: {missing}"
        )


# ---------------------------------------------------------------------------
# Tests: _python_const field presence (Python cross-language parity)
# ---------------------------------------------------------------------------


# Files with _python_const fields and their wrapper keys
FILES_WITH_PYTHON_CONST = {
    "channels.json": ["channels"],
    "intents.json": ["intents"],
    "prompts.json": ["prompts"],
    "collections.json": ["collections"],
    "kv_keys.json": ["kv_keys", "session_types"],
    "agents.json": [
        "agents",
        "agent_binaries",
        "triage_complexity",
        "triage_confidence",
        "triage_intent",
        "triage_posture",
    ],
    "field_paths.json": ["field_paths"],
}


class TestPythonConstPresence:
    """Entries in constant files must have _python_const for Python enum generation."""

    def test_status_entries_have_python_const(self):
        data = _load_json(STATUS_FILE)
        status = _iter_entries(data, "status")

        missing = []
        for cat_name, cat_vals in status.items():
            if not isinstance(cat_vals, dict):
                continue
            for key, meta in cat_vals.items():
                if isinstance(meta, dict):
                    if "_python_const" not in meta or not meta["_python_const"]:
                        missing.append(f"{cat_name}/{key}")
        assert not missing, (
            f"status.json: entries missing _python_const: {missing}"
        )

    @pytest.mark.parametrize(
        "filename,wrapper_key",
        [
            (fn, wk)
            for fn, wks in FILES_WITH_PYTHON_CONST.items()
            for wk in wks
        ],
    )
    def test_entries_have_python_const(self, filename: str, wrapper_key: str):
        data = _load_json(filename)
        assert wrapper_key in data, (
            f"{filename}: missing top-level key '{wrapper_key}'"
        )
        entries = data[wrapper_key]
        assert isinstance(entries, dict), (
            f"{filename}/{wrapper_key}: must be a dict"
        )

        missing = []
        for key, meta in entries.items():
            if isinstance(meta, dict):
                if "_python_const" not in meta or not meta["_python_const"]:
                    missing.append(key)
        assert not missing, (
            f"{filename}/{wrapper_key}: entries missing _python_const: {missing}"
        )


class TestPythonConstNamingConventions:
    """_python_const values must be valid SCREAMING_SNAKE_CASE identifiers."""

    SCREAMING_SNAKE_RE = re.compile(r"^[A-Z][A-Z0-9_]*$")

    def test_status_python_consts_are_screaming_snake_case(self):
        data = _load_json(STATUS_FILE)
        status = _iter_entries(data, "status")

        violations = []
        for cat_name, cat_vals in status.items():
            if not isinstance(cat_vals, dict):
                continue
            for key, meta in cat_vals.items():
                if isinstance(meta, dict) and "_python_const" in meta:
                    pc = meta["_python_const"]
                    if not self.SCREAMING_SNAKE_RE.match(pc):
                        violations.append(f"status.json/{cat_name}/{key}: '{pc}'")
        assert not violations, (
            f"_python_const values not SCREAMING_SNAKE_CASE: {violations}"
        )

    @pytest.mark.parametrize(
        "filename,wrapper_key",
        [
            (fn, wk)
            for fn, wks in FILES_WITH_PYTHON_CONST.items()
            for wk in wks
        ],
    )
    def test_python_consts_are_screaming_snake_case(
        self, filename: str, wrapper_key: str
    ):
        data = _load_json(filename)
        entries = data[wrapper_key]

        violations = []
        for key, meta in entries.items():
            if isinstance(meta, dict) and "_python_const" in meta:
                pc = meta["_python_const"]
                if not self.SCREAMING_SNAKE_RE.match(pc):
                    violations.append(f"{filename}/{wrapper_key}/{key}: '{pc}'")
        assert not violations, (
            f"_python_const values not SCREAMING_SNAKE_CASE: {violations}"
        )


# ---------------------------------------------------------------------------
# Tests: Value uniqueness
# ---------------------------------------------------------------------------


class TestValueUniqueness:
    """Values within each category must be unique (no duplicate wire formats)."""

    @pytest.mark.parametrize("filename", sorted(FILES_WITH_UNIQUE_VALUES))
    def test_values_unique_within_file(self, filename: str):
        data = _load_json(filename)
        wrapper_key = WRAPPER_KEY_FILES[filename]
        entries = _iter_entries(data, wrapper_key)

        seen: dict[str, str] = {}
        duplicates = []
        for key, meta in entries.items():
            if isinstance(meta, dict) and "value" in meta:
                val = str(meta["value"])
                if val in seen:
                    duplicates.append(f"{key} duplicates {seen[val]} (value={val})")
                else:
                    seen[val] = key
        assert not duplicates, (
            f"{filename}: duplicate values found: {duplicates}"
        )

    # Categories with intentional duplicate values (alias entries)
    _SKIP_UNIQUENESS_CATEGORIES = {"ai_task_id"}

    def test_status_values_unique_within_category(self):
        data = _load_json(STATUS_FILE)
        status = _iter_entries(data, "status")

        for cat_name, cat_vals in status.items():
            if not isinstance(cat_vals, dict):
                continue
            if cat_name in self._SKIP_UNIQUENESS_CATEGORIES:
                continue
            seen: dict[str, str] = {}
            duplicates = []
            for key, meta in cat_vals.items():
                if isinstance(meta, dict) and "value" in meta:
                    val = str(meta["value"])
                    if val in seen:
                        duplicates.append(f"{key} duplicates {seen[val]} (value={val})")
                    else:
                        seen[val] = key
            assert not duplicates, (
                f"status.json/{cat_name}: duplicate values: {duplicates}"
            )


# ---------------------------------------------------------------------------
# Tests: Value field presence and non-emptiness
# ---------------------------------------------------------------------------


class TestValueFieldPresence:
    """Every entry in applicable files must have a non-empty value field."""

    @pytest.mark.parametrize("filename", sorted(FILES_WITH_VALUE))
    def test_all_entries_have_value(self, filename: str):
        data = _load_json(filename)
        wrapper_key = WRAPPER_KEY_FILES[filename]
        entries = _iter_entries(data, wrapper_key)

        missing = []
        for key, meta in entries.items():
            if isinstance(meta, dict):
                if "value" not in meta:
                    missing.append(key)
                elif meta["value"] is None or (isinstance(meta["value"], str) and not meta["value"]):
                    missing.append(key)
        assert not missing, (
            f"{filename}: entries missing or empty 'value': {missing}"
        )

    def test_status_entries_have_value(self):
        data = _load_json(STATUS_FILE)
        status = _iter_entries(data, "status")

        missing = []
        for cat_name, cat_vals in status.items():
            if not isinstance(cat_vals, dict):
                continue
            for key, meta in cat_vals.items():
                if not key:
                    continue  # Skip empty-key entries (e.g. history_actor/"")
                if isinstance(meta, dict):
                    if "value" not in meta:
                        missing.append(f"{cat_name}/{key}")
                    elif meta["value"] is None or (isinstance(meta["value"], str) and not meta["value"]):
                        missing.append(f"{cat_name}/{key}")
        assert not missing, (
            f"status.json: entries missing or empty 'value': {missing}"
        )


# ---------------------------------------------------------------------------
# Tests: Event namespace convention
# ---------------------------------------------------------------------------


class TestEventNamespaceConvention:
    """Event values must follow the g8e.v1.* namespace convention."""

    def test_event_values_start_with_g8e_v1(self):
        data = _load_json("events.json")
        events = _iter_entries(data, "events")

        violations = []
        for key, meta in events.items():
            if isinstance(meta, dict) and "value" in meta:
                val = meta["value"]
                if not val.startswith("g8e.v1."):
                    violations.append(f"{key}: {val}")
        assert not violations, (
            f"Event values not following g8e.v1.* namespace: {violations}"
        )

    def test_event_values_are_dotted(self):
        data = _load_json("events.json")
        events = _iter_entries(data, "events")

        for key, meta in events.items():
            if isinstance(meta, dict) and "value" in meta:
                assert "." in meta["value"], (
                    f"Event '{key}' value '{meta['value']}' should contain dots"
                )


# ---------------------------------------------------------------------------
# Tests: Go constant naming conventions
# ---------------------------------------------------------------------------


class TestGoConstNamingConventions:
    """Go constant names must follow PascalCase with consistent prefixes."""

    def test_event_go_consts_prefixed_with_event(self):
        data = _load_json("events.json")
        events = _iter_entries(data, "events")

        violations = []
        for key, meta in events.items():
            if isinstance(meta, dict) and "_go_const" in meta:
                go_const = meta["_go_const"]
                if not go_const.startswith("Event"):
                    violations.append(f"{key}: {go_const}")
        assert not violations, (
            f"Event _go_const values not prefixed with 'Event': {violations}"
        )

    def test_header_go_consts_prefixed_with_header(self):
        data = _load_json("headers.json")
        headers = _iter_entries(data, "headers")

        violations = []
        for key, meta in headers.items():
            if isinstance(meta, dict) and "_go_const" in meta:
                go_const = meta["_go_const"]
                if not go_const.startswith("Header"):
                    violations.append(f"{key}: {go_const}")
        assert not violations, (
            f"Header _go_const values not prefixed with 'Header': {violations}"
        )

    def test_collection_go_consts_prefixed_with_collection(self):
        data = _load_json("collections.json")
        collections = _iter_entries(data, "collections")

        violations = []
        for key, meta in collections.items():
            if isinstance(meta, dict) and "_go_const" in meta:
                go_const = meta["_go_const"]
                if not go_const.startswith("Collection"):
                    violations.append(f"{key}: {go_const}")
        assert not violations, (
            f"Collection _go_const values not prefixed with 'Collection': {violations}"
        )

    def test_go_consts_are_pascalcase(self):
        """All _go_const values must be valid PascalCase identifiers.

        Some naming conventions use a lowercase prefix (e.g. ``envG8EClientCert``
        for environment variable constants). These are allowed.
        """
        pascal_re = re.compile(r"^[A-Z][A-Za-z0-9.]*$")
        # Allow lowercase-prefixed names like envG8EClientCert
        relaxed_re = re.compile(r"^[a-z]+[A-Z][A-Za-z0-9.]*$")

        for filename in sorted(FILES_WITH_GO_CONST):
            data = _load_json(filename)
            wrapper_key = WRAPPER_KEY_FILES[filename]
            entries = _iter_entries(data, wrapper_key)
            for key, meta in entries.items():
                if isinstance(meta, dict) and "_go_const" in meta:
                    go_const = meta["_go_const"]
                    if not go_const:
                        continue  # Empty _go_const is allowed (intentionally unmapped)
                    # Allow dot notation for nested constants (e.g. "Ports.OperatorHttp")
                    base_name = go_const.split(".")[-1]
                    assert pascal_re.match(base_name) or relaxed_re.match(base_name), (
                        f"{filename}/{key}: _go_const '{go_const}' is not PascalCase"
                    )

    def test_status_go_consts_are_pascalcase(self):
        pascal_re = re.compile(r"^[A-Z][A-Za-z0-9.]*$")
        data = _load_json(STATUS_FILE)
        status = _iter_entries(data, "status")

        for cat_name, cat_vals in status.items():
            if not isinstance(cat_vals, dict):
                continue
            for key, meta in cat_vals.items():
                if isinstance(meta, dict) and "_go_const" in meta:
                    go_const = meta["_go_const"]
                    if not go_const:
                        continue
                    assert pascal_re.match(go_const), (
                        f"status.json/{cat_name}/{key}: _go_const '{go_const}' is not PascalCase"
                    )


# ---------------------------------------------------------------------------
# Tests: Python constant loading matches JSON
# ---------------------------------------------------------------------------


class TestPythonConstantsMatchJSON:
    """Verify that Python-loaded constants match the raw JSON values."""

    def test_python_events_match_json(self):
        from g8e.constants import EVENTS

        json_events = _load_json("events.json")
        json_data = _iter_entries(json_events, "events")

        assert len(EVENTS["events"]) == len(json_data), (
            f"Python EVENTS count ({len(EVENTS['events'])}) != JSON count ({len(json_data)})"
        )
        for key, meta in json_data.items():
            assert key in EVENTS["events"], f"Event '{key}' missing from Python EVENTS"
            assert EVENTS["events"][key]["value"] == meta["value"], (
                f"Event '{key}' value mismatch: Python={EVENTS['events'][key]['value']} vs JSON={meta['value']}"
            )

    def test_python_status_match_json(self):
        from g8e.constants import STATUS

        json_status = _load_json("status.json")
        json_data = _iter_entries(json_status, "status")

        assert len(STATUS["status"]) == len(json_data), (
            f"Python STATUS category count ({len(STATUS['status'])}) != JSON count ({len(json_data)})"
        )
        for cat_name, cat_vals in json_data.items():
            assert cat_name in STATUS["status"], (
                f"Status category '{cat_name}' missing from Python STATUS"
            )
            assert len(STATUS["status"][cat_name]) == len(cat_vals), (
                f"Status category '{cat_name}': Python count ({len(STATUS['status'][cat_name])}) != JSON count ({len(cat_vals)})"
            )

    def test_python_headers_match_json(self):
        from g8e.constants import HEADERS

        json_headers = _load_json("headers.json")
        json_data = _iter_entries(json_headers, "headers")

        assert len(HEADERS["headers"]) == len(json_data), (
            f"Python HEADERS count ({len(HEADERS['headers'])}) != JSON count ({len(json_data)})"
        )
        for key, meta in json_data.items():
            assert key in HEADERS["headers"], f"Header '{key}' missing from Python HEADERS"
            assert HEADERS["headers"][key]["value"] == meta["value"], (
                f"Header '{key}' value mismatch: Python={HEADERS['headers'][key]['value']} vs JSON={meta['value']}"
            )

    def test_python_collections_match_json(self):
        from g8e.constants import COLLECTIONS

        json_collections = _load_json("collections.json")
        json_data = _iter_entries(json_collections, "collections")

        assert len(COLLECTIONS["collections"]) == len(json_data), (
            f"Python COLLECTIONS count ({len(COLLECTIONS['collections'])}) != JSON count ({len(json_data)})"
        )
        for key, meta in json_data.items():
            assert key in COLLECTIONS["collections"], (
                f"Collection '{key}' missing from Python COLLECTIONS"
            )
            assert COLLECTIONS["collections"][key]["value"] == meta["value"], (
                f"Collection '{key}' value mismatch"
            )


# ---------------------------------------------------------------------------
# Tests: Status mutation flags
# ---------------------------------------------------------------------------


class TestStatusMutationFlags:
    """Verify _mutation flags are present and boolean where applicable."""

    def test_mutation_flags_are_boolean(self):
        data = _load_json(STATUS_FILE)
        status = _iter_entries(data, "status")

        action_types = status.get("action_type", {})
        for key, meta in action_types.items():
            if isinstance(meta, dict) and "_mutation" in meta:
                assert isinstance(meta["_mutation"], bool), (
                    f"action_type/{key}: _mutation must be boolean, got {type(meta['_mutation'])}"
                )


# ---------------------------------------------------------------------------
# Tests: auth.json multi-wrapper structure
# ---------------------------------------------------------------------------


class TestAuthJsonConstants:
    """auth.json has multiple wrapper keys, each with entries containing _go_const and value."""

    @pytest.mark.parametrize("wrapper_key", AUTH_WRAPPER_KEYS)
    def test_auth_wrapper_key_exists(self, wrapper_key: str):
        data = _load_json("auth.json")
        assert wrapper_key in data, f"auth.json missing top-level key '{wrapper_key}'"
        entries = data[wrapper_key]
        assert isinstance(entries, dict), f"auth.json/{wrapper_key} must be a dict"
        assert len(entries) > 0, f"auth.json/{wrapper_key} has no entries"

    @pytest.mark.parametrize("wrapper_key", AUTH_WRAPPER_KEYS)
    def test_auth_entries_have_go_const(self, wrapper_key: str):
        data = _load_json("auth.json")
        entries = data[wrapper_key]

        missing = []
        for key, meta in entries.items():
            if isinstance(meta, dict) and "_go_const" not in meta:
                missing.append(key)
        assert not missing, (
            f"auth.json/{wrapper_key}: entries missing _go_const: {missing}"
        )

    @pytest.mark.parametrize("wrapper_key", AUTH_WRAPPER_KEYS)
    def test_auth_entries_have_value(self, wrapper_key: str):
        data = _load_json("auth.json")
        entries = data[wrapper_key]

        missing = []
        for key, meta in entries.items():
            if isinstance(meta, dict):
                if "value" not in meta:
                    missing.append(key)
                elif meta["value"] is None or (isinstance(meta["value"], str) and not meta["value"]):
                    missing.append(key)
        assert not missing, (
            f"auth.json/{wrapper_key}: entries missing or empty 'value': {missing}"
        )

    # webauthn_requirements: resident_key_required and user_verification_required
    # intentionally share the value "required" (different concepts, same wire value)
    _SKIP_AUTH_UNIQUENESS = {"webauthn_requirements"}

    @pytest.mark.parametrize("wrapper_key", AUTH_WRAPPER_KEYS)
    def test_auth_values_unique_within_wrapper(self, wrapper_key: str):
        if wrapper_key in self._SKIP_AUTH_UNIQUENESS:
            return

        data = _load_json("auth.json")
        entries = data[wrapper_key]

        seen: dict[str, str] = {}
        duplicates = []
        for key, meta in entries.items():
            if isinstance(meta, dict) and "value" in meta:
                val = str(meta["value"])
                if val in seen:
                    duplicates.append(f"{key} duplicates {seen[val]} (value={val})")
                else:
                    seen[val] = key
        assert not duplicates, (
            f"auth.json/{wrapper_key}: duplicate values: {duplicates}"
        )

    def test_auth_go_consts_are_pascalcase(self):
        pascal_re = re.compile(r"^[A-Z][A-Za-z0-9.]*$")
        relaxed_re = re.compile(r"^[a-z]+[A-Z][A-Za-z0-9.]*$")

        data = _load_json("auth.json")
        for wrapper_key in AUTH_WRAPPER_KEYS:
            entries = data[wrapper_key]
            for key, meta in entries.items():
                if isinstance(meta, dict) and "_go_const" in meta:
                    go_const = meta["_go_const"]
                    if not go_const:
                        continue
                    base_name = go_const.split(".")[-1]
                    assert pascal_re.match(base_name) or relaxed_re.match(base_name), (
                        f"auth.json/{wrapper_key}/{key}: _go_const '{go_const}' is not PascalCase"
                    )
