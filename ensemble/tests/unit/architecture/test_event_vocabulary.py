# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

"""W12: ensemble/app must not reintroduce banned event vocabulary or wire literals."""

from __future__ import annotations

import ast
import re
from pathlib import Path

import pytest

pytestmark = pytest.mark.unit

REPO_ROOT = Path(__file__).resolve().parents[4]
ENSEMBLE_APP = REPO_ROOT / "ensemble" / "app"

# MessageSender values are DB persistence identifiers, not SSE/governed events.
ALLOWED_G8E_V1_LITERAL_FILES = {
    ENSEMBLE_APP / "constants" / "message_sender.py",
}

SKIP_BANNED_VOCABULARY_FILES = {
    ENSEMBLE_APP / "constants" / "__init__.py",
}

BANNED_SUBSTRINGS = (
    "MapActionTypeToEventType",
    "MapEventTypeToResultActionType",
    "map_event_type_to_action_type",
    "action_type_mappings",
    "G8eActionType",
    "action_status",
    "thinking_action_type",
    "EXECUTE_STATUS_UPDATE",
    "INFERENCE_PROGRESS",
    "CommandIntent",
)

G8E_V1_LITERAL = re.compile(r'"g8e\.v1\.[^"]+"')
ACTION_TYPE_STRING_KW = re.compile(r'\baction_type\s*=\s*["\']')


def _python_files_under(path: Path) -> list[Path]:
    return sorted(path.rglob("*.py"))


def _read(path: Path) -> str:
    return path.read_text(encoding="utf-8")


class TestEventVocabularyArchitecture:
    def test_no_g8e_v1_wire_literals_in_ensemble_app(self):
        offenders: list[str] = []
        for path in _python_files_under(ENSEMBLE_APP):
            if path in ALLOWED_G8E_V1_LITERAL_FILES:
                continue
            for line_no, line in enumerate(_read(path).splitlines(), start=1):
                if G8E_V1_LITERAL.search(line):
                    offenders.append(f"{path.relative_to(REPO_ROOT)}:{line_no}: {line.strip()}")
        assert offenders == [], (
            "ensemble/app must use EventType/registry constants, not raw g8e.v1 wire literals: "
            f"{offenders}"
        )

    def test_no_banned_mapper_or_legacy_vocabulary_in_ensemble_app(self):
        offenders: list[str] = []
        for path in _python_files_under(ENSEMBLE_APP):
            if path in SKIP_BANNED_VOCABULARY_FILES:
                continue
            text = _read(path)
            for needle in BANNED_SUBSTRINGS:
                if needle in text:
                    offenders.append(f"{path.relative_to(REPO_ROOT)} matches {needle}")
        assert offenders == [], f"Banned vocabulary reappeared in ensemble/app: {offenders}"

    def test_no_action_type_string_keyword_args_in_ensemble_app(self):
        offenders: list[str] = []
        for path in _python_files_under(ENSEMBLE_APP):
            for line_no, line in enumerate(_read(path).splitlines(), start=1):
                if ACTION_TYPE_STRING_KW.search(line):
                    offenders.append(f"{path.relative_to(REPO_ROOT)}:{line_no}: {line.strip()}")
        assert offenders == [], (
            "ensemble/app must derive action_type from the registry, not string literals: "
            f"{offenders}"
        )

    def test_no_fail_open_event_map_get_defaults(self):
        offenders: list[str] = []
        for path in _python_files_under(ENSEMBLE_APP):
            tree = ast.parse(_read(path), filename=str(path))
            for node in ast.walk(tree):
                if not isinstance(node, ast.Call):
                    continue
                if not isinstance(node.func, ast.Attribute) or node.func.attr != "get":
                    continue
                if len(node.args) < 2:
                    continue
                key = node.args[0]
                default = node.args[1]
                if (
                    isinstance(key, ast.Constant)
                    and isinstance(default, ast.Constant)
                    and isinstance(key.value, str)
                    and key.value == default.value
                ):
                    offenders.append(
                        f"{path.relative_to(REPO_ROOT)}:{node.lineno}: .get({key.value!r}, {default.value!r})"
                    )
        assert offenders == [], f"Fail-open event map defaults are banned: {offenders}"
