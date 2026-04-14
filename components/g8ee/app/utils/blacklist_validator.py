# Copyright (c) 2026 Lateralus Labs, LLC.
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

"""g8e command blacklist validator."""

from __future__ import annotations

import json
import logging
import re
from dataclasses import dataclass
from pathlib import Path

from app.errors import ConfigurationError

logger = logging.getLogger(__name__)


@dataclass
class CommandBlacklistResult:
    """Result of validating a command against the blacklist."""

    is_allowed: bool
    reason: str = ""
    rule: str = ""


class CommandBlacklistValidator:
    """Validates commands against the strict g8e blacklist."""

    def __init__(self, blacklist_path: str) -> None:
        self._config: dict[str, list[dict[str, str]]] = {}
        self._compiled_patterns: list[tuple[re.Pattern[str], dict[str, str]]] = []

        resolved_path = Path(blacklist_path) if blacklist_path else self._default_path()
        if not resolved_path.is_file():
            raise ConfigurationError(f"Command blacklist not found at {resolved_path}")

        self._load(resolved_path)

    def _default_path(self) -> Path:
        return Path(__file__).parent.parent.parent / "config" / "blacklist.json"

    def _load(self, path: Path) -> None:
        logger.info("Loading command blacklist from %s", path)
        with path.open("r", encoding="utf-8") as handle:
            data = json.load(handle)

        required_sections = {
            "forbidden_commands",
            "forbidden_binaries",
            "forbidden_substrings",
            "forbidden_arguments",
            "forbidden_patterns",
        }
        missing = required_sections.difference(data.keys())
        if missing:
            raise ConfigurationError(f"Blacklist file missing required sections: {sorted(missing)}")

        self._config = {
            key: data.get(key, []) for key in required_sections
        }
        self._compiled_patterns = []
        for entry in self._config["forbidden_patterns"]:
            pattern = entry.get("value")
            if not pattern:
                raise ConfigurationError("Blacklist pattern entry missing value")
            try:
                compiled = re.compile(pattern)
            except re.error as exc:
                raise ConfigurationError(f"Invalid blacklist regex '{pattern}': {exc}", cause=exc) from exc
            self._compiled_patterns.append((compiled, entry))

        logger.info(
            "Command blacklist loaded: %d commands, %d substrings, %d patterns",
            len(self._config["forbidden_commands"]),
            len(self._config["forbidden_substrings"]),
            len(self._compiled_patterns),
        )

    def validate_command(self, command_string: str) -> CommandBlacklistResult:
        if not command_string or not command_string.strip():
            return CommandBlacklistResult(
                is_allowed=False,
                reason="Empty command",
                rule="empty_command",
            )

        command_string = command_string.strip()
        tokens = command_string.split()
        base_command = tokens[0]

        for entry in self._config["forbidden_commands"]:
            value = entry.get("value")
            if value and base_command == value:
                return CommandBlacklistResult(
                    is_allowed=False,
                    reason=entry.get("reason") or "",
                    rule=f"command:{value}",
                )

        for entry in self._config["forbidden_binaries"]:
            value = entry.get("value")
            if value and value in {base_command, command_string}:
                return CommandBlacklistResult(
                    is_allowed=False,
                    reason=entry.get("reason") or "",
                    rule=f"binary:{value}",
                )

        for entry in self._config["forbidden_substrings"]:
            value = entry.get("value")
            if value and value in command_string:
                return CommandBlacklistResult(
                    is_allowed=False,
                    reason=entry.get("reason") or "",
                    rule=f"substring:{value}",
                )

        forbidden_args = {entry.get("value") for entry in self._config["forbidden_arguments"] if entry.get("value")}
        if forbidden_args and any(arg in forbidden_args for arg in tokens[1:]):
            hit = next(arg for arg in tokens[1:] if arg in forbidden_args)
            entry = next(item for item in self._config["forbidden_arguments"] if item.get("value") == hit)
            return CommandBlacklistResult(
                is_allowed=False,
                reason=entry.get("reason") or "",
                rule=f"argument:{hit}",
            )

        for regex, entry in self._compiled_patterns:
            if regex.search(command_string):
                return CommandBlacklistResult(
                    is_allowed=False,
                    reason=entry.get("reason") or "",
                    rule=f"pattern:{entry.get('value')}",
                )

        return CommandBlacklistResult(is_allowed=True)

    def get_forbidden_commands(self) -> list[dict[str, str]]:
        """Get list of forbidden base commands with reasons."""
        return [{"command": e.get("value", ""), "reason": e.get("reason", "")} for e in self._config["forbidden_commands"] if e.get("value")]

    def get_forbidden_substrings(self) -> list[dict[str, str]]:
        """Get list of forbidden command substrings with reasons."""
        return [{"substring": e.get("value", ""), "reason": e.get("reason", "")} for e in self._config["forbidden_substrings"] if e.get("value")]

    def get_forbidden_patterns(self) -> list[dict[str, str]]:
        """Get list of forbidden regex patterns with reasons."""
        return [{"pattern": e.get("value", ""), "reason": e.get("reason", "")} for e in self._config["forbidden_patterns"] if e.get("value")]


_validator: CommandBlacklistValidator | None = None


def get_blacklist_validator(blacklist_path: str | None = None) -> CommandBlacklistValidator:
    global _validator
    if _validator is None:
        _validator = CommandBlacklistValidator(blacklist_path=blacklist_path or "")
    return _validator


def validate_command_against_blacklist(command: str) -> CommandBlacklistResult:
    validator = get_blacklist_validator()
    return validator.validate_command(command)
