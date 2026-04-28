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

"""g8e command auto-approved list validator.

Auto-approve is the SKIP-APPROVAL gate (distinct from whitelist/blacklist):
when ``CommandValidationSettings.enable_auto_approve`` is true, base commands
listed here bypass the human approval prompt. The human has rubber-stamped
these as benign. Auto-approve does NOT widen the whitelist and does NOT
override the blacklist; commands must still pass all hard L1 safety gates
(forbidden patterns, blacklist, whitelist if enabled) before auto-approve
can apply.
"""

from __future__ import annotations

import logging
from dataclasses import dataclass
from pathlib import Path

from app.errors import ConfigurationError
from app.utils.config_loader import load_json_config

logger = logging.getLogger(__name__)


@dataclass
class CommandAutoApprovedResult:
    """Result of checking a command against the auto-approved list."""

    is_auto_approved: bool
    reason: str = ""
    rule: str = ""


class CommandAutoApprovedValidator:
    """Validates whether a base command may bypass human approval."""

    def __init__(self, auto_approved_path: str) -> None:
        self._entries: list[dict[str, str]] = []
        self._index: dict[str, dict[str, str]] = {}

        resolved_path = (
            Path(auto_approved_path) if auto_approved_path else self._default_path()
        )
        self._load(resolved_path)

    def _default_path(self) -> Path:
        return Path(__file__).parent.parent.parent / "config" / "auto_approved.json"

    def _load(self, path: Path) -> None:
        logger.info("Loading command auto-approved list from %s", path)
        data = load_json_config(path, config_name="auto-approved list")

        enabled = data.get("enabled", True)
        if not enabled:
            logger.warning(
                "Command auto-approved list is disabled via 'enabled: false' in %s; "
                "loading empty index (no commands will be auto-approved at the JSON level)",
                path,
            )
            self._entries = []
            self._index = {}
            return

        if "auto_approved_commands" not in data:
            raise ConfigurationError(
                "Auto-approved list missing required 'auto_approved_commands' section"
            )

        entries = data.get("auto_approved_commands") or []
        if not isinstance(entries, list):
            raise ConfigurationError(
                "Auto-approved list 'auto_approved_commands' must be a list"
            )

        normalized: list[dict[str, str]] = []
        index: dict[str, dict[str, str]] = {}
        for entry in entries:
            if not isinstance(entry, dict):
                raise ConfigurationError(
                    "Auto-approved entry must be an object with 'value' and optional 'reason'"
                )
            value = entry.get("value")
            if not value or not isinstance(value, str):
                raise ConfigurationError(
                    "Auto-approved entry missing required 'value' string"
                )
            if any(c.isspace() for c in value):
                raise ConfigurationError(
                    f"Auto-approved 'value' must be a base command without whitespace, got '{value}'"
                )
            normalized_entry = {
                "value": value,
                "reason": str(entry.get("reason") or ""),
            }
            normalized.append(normalized_entry)
            # Last write wins for duplicates; log to surface config issues.
            if value in index:
                logger.warning(
                    "Duplicate auto-approved entry for base command '%s'; overriding earlier entry",
                    value,
                )
            index[value] = normalized_entry

        self._entries = normalized
        self._index = index

        logger.info(
            "Command auto-approved list loaded: %d base commands",
            len(self._entries),
        )

    def is_auto_approved(
        self,
        command_string: str,
        extra_commands: list[str] | None = None,
    ) -> CommandAutoApprovedResult:
        """Return whether the base command is auto-approved.

        Args:
            command_string: Full command string (only the first whitespace-
                delimited token is considered; arguments are ignored because
                auto-approve operates on the base command).
            extra_commands: Optional per-user CSV override. Treated as an
                additional auto-approved set (rubber-stamped at request time).
                Reasons sourced from JSON file when the same base command
                exists in both; otherwise a generic reason is returned.

        Returns:
            CommandAutoApprovedResult with ``is_auto_approved`` flag.
        """
        if not command_string or not command_string.strip():
            return CommandAutoApprovedResult(
                is_auto_approved=False,
                reason="Empty command",
                rule="empty_command",
            )

        base_command = command_string.strip().split()[0]

        json_entry = self._index.get(base_command)
        if json_entry is not None:
            return CommandAutoApprovedResult(
                is_auto_approved=True,
                reason=json_entry.get("reason", ""),
                rule=f"json:{base_command}",
            )

        if extra_commands and base_command in extra_commands:
            return CommandAutoApprovedResult(
                is_auto_approved=True,
                reason="User-configured per-request auto-approve override",
                rule=f"override:{base_command}",
            )

        return CommandAutoApprovedResult(is_auto_approved=False)

    def get_auto_approved_commands(self) -> list[dict[str, str]]:
        """Return all JSON-configured auto-approved base commands with reasons."""
        return [
            {"command": entry["value"], "reason": entry.get("reason", "")}
            for entry in self._entries
        ]

    def get_auto_approved_command_names(self) -> list[str]:
        """Return the JSON-configured auto-approved base command names."""
        return [entry["value"] for entry in self._entries]


_validator: CommandAutoApprovedValidator | None = None


def register_auto_approved_validator(validator: CommandAutoApprovedValidator) -> None:
    """Explicitly register the global auto-approved validator instance."""
    global _validator
    _validator = validator


def get_auto_approved_validator(
    auto_approved_path: str | None = None,
) -> CommandAutoApprovedValidator:
    """Return the process-wide ``CommandAutoApprovedValidator`` singleton.

    If no validator has been explicitly registered, one will be created from
    the default path (or the provided path). This backward-compatibility mode
    is deprecated; new code should use register_auto_approved_validator().
    """
    global _validator
    if _validator is None:
        logger.warning(
            "get_auto_approved_validator() called without explicit registration; "
            "creating validator implicitly. Use register_auto_approved_validator() for explicit DI."
        )
        _validator = CommandAutoApprovedValidator(
            auto_approved_path=auto_approved_path or ""
        )
    return _validator


def is_command_auto_approved(
    command: str, extra_commands: list[str] | None = None
) -> CommandAutoApprovedResult:
    """Module-level convenience wrapper around the singleton validator."""
    return get_auto_approved_validator().is_auto_approved(command, extra_commands)
