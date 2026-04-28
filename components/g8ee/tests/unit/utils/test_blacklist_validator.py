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

"""Unit tests for ``CommandBlacklistValidator``.

Run with:
    /home/bob/g8e/g8e test g8ee -- tests/unit/utils/test_blacklist_validator.py
"""

import json
import tempfile

import pytest

from app.errors import ConfigurationError
from app.utils.blacklist_validator import (
    CommandBlacklistValidator,
    get_blacklist_validator,
)

pytestmark = [pytest.mark.unit]


def _write_json(data: dict) -> str:
    f = tempfile.NamedTemporaryFile(mode="w", suffix=".json", delete=False)
    json.dump(data, f)
    f.flush()
    f.close()
    return f.name


def _minimal_blacklist(**overrides) -> dict:
    base = {
        "forbidden_commands": [],
        "forbidden_binaries": [],
        "forbidden_substrings": [],
        "forbidden_arguments": [],
        "forbidden_patterns": [],
    }
    base.update(overrides)
    return base


class TestCommandBlacklistValidatorConfigurationErrors:
    def test_missing_file_raises_configuration_error(self):
        with pytest.raises(ConfigurationError, match="blacklist not found"):
            CommandBlacklistValidator(blacklist_path="/nonexistent/blacklist.json")

    def test_invalid_json_raises_configuration_error(self):
        f = tempfile.NamedTemporaryFile(mode="w", suffix=".json", delete=False)
        f.write("{not json")
        f.flush()
        f.close()
        with pytest.raises(ConfigurationError, match="Invalid JSON"):
            CommandBlacklistValidator(blacklist_path=f.name)

    def test_missing_required_sections_raises_configuration_error(self):
        path = _write_json({})
        with pytest.raises(ConfigurationError, match="missing required sections"):
            CommandBlacklistValidator(blacklist_path=path)


class TestCommandBlacklistValidatorBehavior:
    @pytest.fixture
    def validator(self):
        path = _write_json(
            _minimal_blacklist(
                forbidden_commands=[
                    {"value": "sudo", "reason": "privilege escalation"},
                ],
                forbidden_patterns=[
                    {"value": "(?i)rm\\s+(-[rf]|-fr)", "reason": "recursive delete"},
                ],
            )
        )
        return CommandBlacklistValidator(blacklist_path=path)

    def test_forbidden_command_is_blocked(self, validator):
        result = validator.validate_command("sudo ls")
        assert result.is_allowed is False
        assert "sudo" in result.rule

    def test_allowed_command_passes(self, validator):
        result = validator.validate_command("ls -la")
        assert result.is_allowed is True

    def test_forbidden_pattern_is_blocked(self, validator):
        result = validator.validate_command("rm -rf /tmp")
        assert result.is_allowed is False
        assert "pattern" in result.rule

    def test_empty_command_is_blocked(self, validator):
        result = validator.validate_command("")
        assert result.is_allowed is False
        assert result.rule == "empty_command"


class TestEnabledField:
    """Test the enabled field as a file-level kill switch."""

    def test_enabled_false_loads_empty_index(self):
        path = _write_json(
            _minimal_blacklist(
                enabled=False,
                forbidden_commands=[
                    {"value": "sudo", "reason": "should be ignored"},
                ],
            )
        )
        validator = CommandBlacklistValidator(blacklist_path=path)
        assert validator.get_forbidden_commands() == []
        result = validator.validate_command("sudo ls")
        assert result.is_allowed is True

    def test_enabled_true_loads_commands(self):
        path = _write_json(
            _minimal_blacklist(
                enabled=True,
                forbidden_commands=[
                    {"value": "sudo", "reason": "should be loaded"},
                ],
            )
        )
        validator = CommandBlacklistValidator(blacklist_path=path)
        assert len(validator.get_forbidden_commands()) == 1
        result = validator.validate_command("sudo ls")
        assert result.is_allowed is False

    def test_enabled_defaults_to_true(self):
        path = _write_json(
            _minimal_blacklist(
                forbidden_commands=[
                    {"value": "sudo", "reason": "should be loaded"},
                ],
            )
        )
        validator = CommandBlacklistValidator(blacklist_path=path)
        assert len(validator.get_forbidden_commands()) == 1
        result = validator.validate_command("sudo ls")
        assert result.is_allowed is False
