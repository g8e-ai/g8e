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

"""Unit tests for ``CommandAutoApprovedValidator``.

Run with:
    /home/bob/g8e/g8e test g8ee -- tests/unit/utils/test_auto_approved_validator.py
"""

import json
import tempfile

import pytest

from app.errors import ConfigurationError
from app.utils.auto_approved_validator import (
    CommandAutoApprovedValidator,
    get_auto_approved_validator,
)

pytestmark = [pytest.mark.unit]


def _write_json(data: dict) -> str:
    f = tempfile.NamedTemporaryFile(mode="w", suffix=".json", delete=False)
    json.dump(data, f)
    f.flush()
    f.close()
    return f.name


def _minimal_auto_approved(**overrides) -> dict:
    base = {"auto_approved_commands": []}
    base.update(overrides)
    return base


class TestCommandAutoApprovedValidatorConfigurationErrors:
    def test_missing_file_raises_configuration_error(self):
        with pytest.raises(ConfigurationError, match="auto-approved list not found"):
            CommandAutoApprovedValidator(auto_approved_path="/nonexistent/auto.json")

    def test_invalid_json_raises_configuration_error(self):
        f = tempfile.NamedTemporaryFile(mode="w", suffix=".json", delete=False)
        f.write("{not json")
        f.flush()
        f.close()
        with pytest.raises(ConfigurationError, match="Invalid JSON"):
            CommandAutoApprovedValidator(auto_approved_path=f.name)

    def test_missing_section_raises_configuration_error(self):
        path = _write_json({})
        with pytest.raises(ConfigurationError, match="auto_approved_commands"):
            CommandAutoApprovedValidator(auto_approved_path=path)

    def test_section_must_be_list(self):
        path = _write_json({"auto_approved_commands": {"value": "uptime"}})
        with pytest.raises(ConfigurationError, match="must be a list"):
            CommandAutoApprovedValidator(auto_approved_path=path)

    def test_entry_must_have_value(self):
        path = _write_json({"auto_approved_commands": [{"reason": "no value"}]})
        with pytest.raises(ConfigurationError, match="missing required 'value'"):
            CommandAutoApprovedValidator(auto_approved_path=path)

    def test_entry_value_must_be_base_command(self):
        path = _write_json(
            {"auto_approved_commands": [{"value": "df -h", "reason": "with args"}]}
        )
        with pytest.raises(ConfigurationError, match="without whitespace"):
            CommandAutoApprovedValidator(auto_approved_path=path)


class TestCommandAutoApprovedValidatorBehavior:
    @pytest.fixture
    def validator(self):
        path = _write_json(
            _minimal_auto_approved(
                auto_approved_commands=[
                    {"value": "uptime", "reason": "benign uptime"},
                    {"value": "df", "reason": "benign disk free"},
                ]
            )
        )
        return CommandAutoApprovedValidator(auto_approved_path=path)

    def test_listed_base_command_is_auto_approved(self, validator):
        result = validator.is_auto_approved("uptime")
        assert result.is_auto_approved is True
        assert result.reason == "benign uptime"
        assert result.rule == "json:uptime"

    def test_listed_base_command_with_args_is_auto_approved(self, validator):
        result = validator.is_auto_approved("df -h /var")
        assert result.is_auto_approved is True
        assert result.rule == "json:df"

    def test_unlisted_base_command_is_not_auto_approved(self, validator):
        result = validator.is_auto_approved("rm -rf /")
        assert result.is_auto_approved is False

    def test_empty_command_not_auto_approved(self, validator):
        result = validator.is_auto_approved("")
        assert result.is_auto_approved is False
        assert result.rule == "empty_command"

    def test_whitespace_only_command_not_auto_approved(self, validator):
        result = validator.is_auto_approved("   ")
        assert result.is_auto_approved is False

    def test_csv_override_augments_json_list(self, validator):
        result = validator.is_auto_approved("free -m", extra_commands=["free"])
        assert result.is_auto_approved is True
        assert result.rule == "override:free"

    def test_csv_override_does_not_widen_for_unlisted(self, validator):
        result = validator.is_auto_approved("rm -rf /", extra_commands=["free"])
        assert result.is_auto_approved is False

    def test_json_takes_precedence_over_override(self, validator):
        result = validator.is_auto_approved("df -h", extra_commands=["df"])
        assert result.rule == "json:df"
        assert result.reason == "benign disk free"

    def test_get_auto_approved_command_names(self, validator):
        assert validator.get_auto_approved_command_names() == ["uptime", "df"]

    def test_get_auto_approved_commands_returns_full_records(self, validator):
        records = validator.get_auto_approved_commands()
        assert {"command": "uptime", "reason": "benign uptime"} in records
        assert {"command": "df", "reason": "benign disk free"} in records


class TestDefaultJsonFileLoads:
    """The shipped config/auto_approved.json must be parseable."""

    def test_default_singleton_loads(self):
        # Reset singleton to force a fresh load from the default path.
        import app.utils.auto_approved_validator as module

        module._validator = None
        validator = get_auto_approved_validator()
        # Default config ships with sensible benign defaults.
        assert "uptime" in validator.get_auto_approved_command_names()
