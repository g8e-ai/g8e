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

import json
import re
import tempfile

import pytest

from app.constants import Platform
from app.errors import ConfigurationError
from app.utils.blacklist_validator import CommandBlacklistValidator
from app.utils.whitelist_validator import CommandWhitelistValidator

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


def _minimal_whitelist(**overrides) -> dict:
    base = {
        "commands": {},
        "enforcement_policy": {"mode": "strict"},
    }
    base.update(overrides)
    return base


# =============================================================================
# CommandBlacklistValidator — configuration error paths
# =============================================================================


class TestCommandBlacklistValidatorConfigurationErrors:
    def test_missing_file_raises_configuration_error(self):
        with pytest.raises(ConfigurationError, match="Command blacklist not found"):
            CommandBlacklistValidator(blacklist_path="/nonexistent/path/blacklist.json")

    @pytest.mark.parametrize("missing_section", [
        "forbidden_commands",
        "forbidden_binaries",
        "forbidden_substrings",
        "forbidden_arguments",
        "forbidden_patterns",
    ])
    def test_missing_required_section_raises_configuration_error(self, missing_section):
        data = _minimal_blacklist()
        del data[missing_section]
        path = _write_json(data)

        with pytest.raises(ConfigurationError, match="missing required sections"):
            CommandBlacklistValidator(blacklist_path=path)

    def test_pattern_entry_missing_value_field_raises_configuration_error(self):
        data = _minimal_blacklist(
            forbidden_patterns=[{"reason": "no value field"}],
        )
        path = _write_json(data)

        with pytest.raises(ConfigurationError, match="missing value"):
            CommandBlacklistValidator(blacklist_path=path)

    def test_invalid_regex_pattern_raises_configuration_error(self):
        data = _minimal_blacklist(
            forbidden_patterns=[{"value": "[unclosed(group"}],
        )
        path = _write_json(data)

        with pytest.raises(ConfigurationError, match="Invalid blacklist regex"):
            CommandBlacklistValidator(blacklist_path=path)

    def test_invalid_regex_error_includes_pattern_in_message(self):
        pattern = "[unclosed(group"
        data = _minimal_blacklist(
            forbidden_patterns=[{"value": pattern}],
        )
        path = _write_json(data)

        with pytest.raises(ConfigurationError, match=re.escape(pattern)):
            CommandBlacklistValidator(blacklist_path=path)


# =============================================================================
# CommandBlacklistValidator — load and validation logic
# =============================================================================


class TestCommandBlacklistValidatorBehavior:
    @pytest.fixture
    def validator_with_rules(self):
        data = {
            "forbidden_commands": [{"value": "rm", "reason": "dangerous delete"}],
            "forbidden_binaries": [{"value": "nc", "reason": "netcat shell risk"}],
            "forbidden_substrings": [{"value": "--no-preserve-root", "reason": "destructive flag"}],
            "forbidden_arguments": [],
            "forbidden_patterns": [{"value": r";\s*rm\s+-rf", "reason": "shell injection"}],
        }
        path = _write_json(data)
        return CommandBlacklistValidator(blacklist_path=path)

    def test_forbidden_command_is_blocked(self, validator_with_rules):
        result = validator_with_rules.validate_command("rm -rf /")
        assert result.is_allowed is False

    def test_forbidden_command_result_includes_reason(self, validator_with_rules):
        result = validator_with_rules.validate_command("rm -rf /")
        assert result.reason == "dangerous delete"

    def test_forbidden_binary_is_blocked(self, validator_with_rules):
        result = validator_with_rules.validate_command("nc -lvp 4444")
        assert result.is_allowed is False

    def test_forbidden_substring_is_blocked(self, validator_with_rules):
        result = validator_with_rules.validate_command("rm --no-preserve-root /")
        assert result.is_allowed is False

    def test_forbidden_pattern_is_blocked(self, validator_with_rules):
        result = validator_with_rules.validate_command("echo hello; rm -rf /tmp")
        assert result.is_allowed is False

    @pytest.mark.parametrize("safe_command", [
        "ls -la",
        "df -h",
        "ps aux",
        "systemctl status nginx",
        "cat /etc/hosts",
        "uptime",
    ])
    def test_safe_commands_are_allowed(self, validator_with_rules, safe_command):
        assert validator_with_rules.validate_command(safe_command).is_allowed is True

    def test_allowed_result_has_no_rule(self, validator_with_rules):
        result = validator_with_rules.validate_command("ls -la")
        assert result.rule == ""

    def test_empty_blacklist_allows_all_commands(self):
        path = _write_json(_minimal_blacklist())
        validator = CommandBlacklistValidator(blacklist_path=path)
        assert validator.validate_command("rm -rf /").is_allowed is True


# =============================================================================
# CommandWhitelistValidator — configuration error paths
# =============================================================================


class TestCommandWhitelistValidatorConfigurationErrors:
    def test_explicit_nonexistent_path_raises_configuration_error(self):
        with pytest.raises(ConfigurationError, match="Required whitelist configuration not found"):
            CommandWhitelistValidator(whitelist_path="/nonexistent/path/whitelist.json")

    def test_missing_enforcement_policy_raises_configuration_error(self):
        data = {"commands": {}}
        path = _write_json(data)

        with pytest.raises(ConfigurationError, match="enforcement_policy"):
            CommandWhitelistValidator(whitelist_path=path)


# =============================================================================
# CommandWhitelistValidator — load and validation logic
# =============================================================================


class TestCommandWhitelistValidatorBehavior:
    @pytest.fixture
    def validator(self):
        data = _minimal_whitelist(
            commands={
                "filesystem": {
                    "ls": {"description": "list directory", "platforms": ["linux"]},
                    "df": {"description": "disk usage", "platforms": ["linux"]},
                },
            },
        )
        path = _write_json(data)
        return CommandWhitelistValidator(whitelist_path=path)

    def test_whitelisted_command_is_allowed(self, validator):
        result = validator.validate_command("ls -la", platform=Platform.LINUX)
        assert result.is_valid is True

    def test_non_whitelisted_command_is_blocked(self, validator):
        result = validator.validate_command("totally_unknown_binary --flag", platform=Platform.LINUX)
        assert result.is_valid is False

    def test_loaded_commands_are_populated(self, validator):
        assert len(validator.all_commands) > 0
        assert "ls" in validator.all_commands

    def test_whitelist_loads_all_categories(self, validator):
        assert "filesystem" in validator.commands_by_category
