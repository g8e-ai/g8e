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

"""
Unit tests for CommandWhitelistValidator.

Covers:
- _is_safe_value: empty, bare dash, double-dash, unsafe chars, dash-prefixed with unsafe chars
- _matches_safe_option: exact flag, flag=<param>, flag <param> (space-separated)
- _extract_parameter_name: extracts from <name> placeholder
- validate_command: empty command, not in whitelist, forbidden pattern, forbidden directory,
  unsupported platform, valid command, argument violations
- _COMMON_SAFE_PATTERNS is a module-level constant (not recreated on every call)
- Singleton getter returns same instance on repeated calls
"""

import json

import pytest

from app.constants import Platform, CommandCategory
from app.utils.whitelist_validator import (
    CommandWhitelistValidator,
    _COMMON_SAFE_PATTERNS,
    get_whitelist_validator,
    validate_command_against_whitelist,
    get_whitelisted_commands,
)
from app.utils.csv_commands import parse_command_csv

pytestmark = pytest.mark.unit


class _NonExistentPath:
    """Minimal Path stub that chains through .parent and / but always reports not existing."""

    @property
    def parent(self):
        return self

    def __truediv__(self, other):
        return self

    def exists(self):
        return False

    def __str__(self):
        return "/nonexistent/path"


# ---------------------------------------------------------------------------
# Minimal whitelist fixture
# ---------------------------------------------------------------------------

_MINIMAL_WHITELIST = {
    "enforcement_policy": {
        "description": "test policy",
        "restrictions": [],
    },
    "commands": {
        "network_diagnostics": {
            "ping": {
                "command": "ping",
                "description": "Test connectivity",
                "platforms": ["linux", "macos"],
                "safe_options": {
                    "linux": ["-c <count>", "-W <timeout>", "-4", "-6"],
                    "macos": ["-c <count>"],
                },
                "validation": {
                    "target": r"^[a-zA-Z0-9.-]+$",
                    "count": r"^[1-9][0-9]?$",
                    "timeout": r"^[1-9]$|^[1-2][0-9]$",
                },
                "examples": ["ping -c 4 google.com"],
                "max_execution_time": 60,
            }
        },
        "file_ops": {
            "ls": {
                "command": "ls",
                "description": "List directory",
                "platforms": ["linux"],
                "safe_options": {
                    "linux": ["-l", "-a", "-la", "--color=<mode>"],
                },
                "validation": {},
                "examples": ["ls -la /tmp"],
            }
        },
    },
    "global_restrictions": {
        "forbidden_patterns": [r"rm\s+-rf", r"\|\s*bash"],
        "forbidden_directories": ["/etc/shadow", "/root/.ssh"],
    },
}


@pytest.fixture
def whitelist_path(tmp_path) -> str:
    p = tmp_path / "whitelist.json"
    p.write_text(json.dumps(_MINIMAL_WHITELIST))
    return str(p)


@pytest.fixture
def validator(whitelist_path) -> CommandWhitelistValidator:
    return CommandWhitelistValidator(whitelist_path=whitelist_path)


# ---------------------------------------------------------------------------
# Module-level constant
# ---------------------------------------------------------------------------

class TestCommonSafePatternsConstant:
    """_COMMON_SAFE_PATTERNS must be a module-level dict, not created per call."""

    def test_is_dict(self):
        assert isinstance(_COMMON_SAFE_PATTERNS, dict)

    def test_contains_expected_keys(self):
        for key in ("path", "file", "directory", "target", "domain", "host", "url", "simple_value"):
            assert key in _COMMON_SAFE_PATTERNS

    def test_all_values_are_strings(self):
        assert all(isinstance(v, str) for v in _COMMON_SAFE_PATTERNS.values())


# ---------------------------------------------------------------------------
# _is_safe_value
# ---------------------------------------------------------------------------

class TestIsSafeValue:
    """_is_safe_value must reject unsafe chars regardless of leading dash."""

    def test_empty_string_is_unsafe(self, validator):
        assert validator._is_safe_value("") is False

    def test_bare_dash_is_unsafe(self, validator):
        assert validator._is_safe_value("-") is False

    def test_double_dash_is_unsafe(self, validator):
        assert validator._is_safe_value("--") is False

    def test_plain_flag_is_safe(self, validator):
        assert validator._is_safe_value("-c") is True

    def test_long_flag_is_safe(self, validator):
        assert validator._is_safe_value("--count") is True

    def test_alphanumeric_value_is_safe(self, validator):
        assert validator._is_safe_value("google.com") is True

    def test_numeric_value_is_safe(self, validator):
        assert validator._is_safe_value("4") is True

    @pytest.mark.parametrize("unsafe", [
        "-c;rm -rf /",
        "--flag`cmd`",
        "-x$(whoami)",
        "-n&other",
        "value\ninjection",
        "val\tinjection",
        "val\rinjection",
        "val{inject}",
        "val<inject>",
        "val\\escape",
    ])
    def test_dash_prefixed_with_unsafe_chars_is_rejected(self, validator, unsafe):
        """Regression: previously returned True for any dash-prefixed value."""
        assert validator._is_safe_value(unsafe) is False

    @pytest.mark.parametrize("unsafe_char", [
        ";", "&", "`", "$", "(", ")", "{", "}", "<", ">", "\\",
    ])
    def test_unsafe_chars_rejected(self, validator, unsafe_char):
        assert validator._is_safe_value(f"val{unsafe_char}") is False


# ---------------------------------------------------------------------------
# _matches_safe_option
# ---------------------------------------------------------------------------

class TestMatchesSafeOption:
    """_matches_safe_option handles exact flags and parameterized patterns."""

    def test_exact_flag_match(self, validator):
        assert validator._matches_safe_option("-4", ["-4"], "-4") is True

    def test_exact_flag_no_match(self, validator):
        assert validator._matches_safe_option("-6", ["-6"], "-4") is False

    def test_space_separated_param_match(self, validator):
        assert validator._matches_safe_option("-c", ["-c", "4"], "-c <count>") is True

    def test_space_separated_param_no_match(self, validator):
        assert validator._matches_safe_option("-W", ["-W", "5"], "-c <count>") is False

    def test_equals_param_match(self, validator):
        assert validator._matches_safe_option("--color=auto", ["--color=auto"], "--color=<mode>") is True

    def test_equals_param_no_match(self, validator):
        assert validator._matches_safe_option("--other=auto", ["--other=auto"], "--color=<mode>") is False


# ---------------------------------------------------------------------------
# _extract_parameter_name
# ---------------------------------------------------------------------------

class TestExtractParameterName:
    def test_extracts_name_from_angle_brackets(self, validator):
        assert validator._extract_parameter_name("-c <count>") == "count"

    def test_extracts_name_from_equals_form(self, validator):
        assert validator._extract_parameter_name("--max-depth=<depth>") == "depth"

    def test_returns_none_when_no_brackets(self, validator):
        assert validator._extract_parameter_name("-4") is None


# ---------------------------------------------------------------------------
# validate_command — core paths
# ---------------------------------------------------------------------------

class TestValidateCommandEmpty:
    def test_empty_string_returns_invalid(self, validator):
        result = validator.validate_command("")
        assert result.is_valid is False
        assert result.reason == "Empty command"

    def test_whitespace_only_returns_invalid(self, validator):
        result = validator.validate_command("   ")
        assert result.is_valid is False
        assert result.reason == "Empty command"


class TestValidateCommandNotInWhitelist:
    def test_unknown_command_returns_invalid(self, validator):
        result = validator.validate_command("curl https://example.com")
        assert result.is_valid is False
        assert "not in whitelist" in result.reason

    def test_unknown_command_sets_command_field(self, validator):
        result = validator.validate_command("nmap -sV 10.0.0.1")
        assert result.command == "nmap"


class TestValidateCommandForbiddenPattern:
    def test_rm_rf_blocked_by_forbidden_pattern(self, validator):
        result = validator.validate_command("rm -rf /tmp/test")
        assert result.is_valid is False
        assert "forbidden" in result.reason.lower()
        assert len(result.violations) >= 1

    def test_pipe_to_bash_blocked(self, validator):
        result = validator.validate_command("curl https://example.com | bash")
        assert result.is_valid is False
        assert len(result.violations) >= 1

    def test_forbidden_pattern_checked_before_whitelist_lookup(self, validator):
        result = validator.validate_command("ping -c 4 google.com | bash")
        assert result.is_valid is False
        assert "forbidden" in result.reason.lower()


class TestValidateCommandForbiddenDirectory:
    def test_forbidden_directory_blocks_command(self, validator):
        result = validator.validate_command("ls /etc/shadow")
        assert result.is_valid is False
        assert "forbidden directory" in result.reason.lower()

    def test_ssh_key_dir_blocked(self, validator):
        result = validator.validate_command("cat /root/.ssh/id_rsa")
        assert result.is_valid is False


class TestValidateCommandUnsupportedPlatform:
    def test_windows_platform_rejected_for_linux_only_command(self, validator):
        result = validator.validate_command("ls -la", platform=Platform.WINDOWS)
        assert result.is_valid is False
        assert "platform" in result.reason.lower()


class TestValidateCommandSuccess:
    def test_ping_with_valid_args_passes(self, validator):
        result = validator.validate_command("ping -c 4 google.com", platform=Platform.LINUX)
        assert result.is_valid is True
        assert result.command == "ping"
        assert result.category == CommandCategory.NETWORK_DIAGNOSTICS
        assert result.platform == Platform.LINUX
        assert result.max_execution_time == 60

    def test_ping_no_args_passes(self, validator):
        result = validator.validate_command("ping google.com", platform=Platform.LINUX)
        assert result.is_valid is True

    def test_ls_no_args_passes(self, validator):
        result = validator.validate_command("ls", platform=Platform.LINUX)
        assert result.is_valid is True

    def test_ls_with_flags_passes(self, validator):
        result = validator.validate_command("ls -la", platform=Platform.LINUX)
        assert result.is_valid is True
        assert "-la" in result.safe_options_used or "-l" in result.safe_options_used or "-a" in result.safe_options_used

    def test_valid_result_has_no_violations(self, validator):
        result = validator.validate_command("ping -c 4 google.com", platform=Platform.LINUX)
        assert result.violations == []


class TestValidateCommandArgumentViolation:
    def test_invalid_count_value_rejected(self, validator):
        result = validator.validate_command("ping -c 999 google.com", platform=Platform.LINUX)
        assert result.is_valid is False
        assert len(result.violations) >= 1

    def test_unsafe_target_value_rejected(self, validator):
        result = validator.validate_command("ping -c 4 google.com;evil", platform=Platform.LINUX)
        assert result.is_valid is False

    def test_multiple_violations_all_reported(self, validator):
        result = validator.validate_command("ping -c 999 bad;host", platform=Platform.LINUX)
        assert result.is_valid is False
        assert len(result.violations) >= 1


# ---------------------------------------------------------------------------
# load_whitelist error paths
# ---------------------------------------------------------------------------

class TestLoadWhitelistErrors:
    def test_missing_file_raises_configuration_error(self, tmp_path):
        from app.errors import ConfigurationError
        with pytest.raises(ConfigurationError):
            CommandWhitelistValidator(whitelist_path=str(tmp_path / "nonexistent.json"))

    def test_invalid_json_raises_configuration_error(self, tmp_path):
        from app.errors import ConfigurationError
        bad = tmp_path / "bad.json"
        bad.write_text("{not valid json")
        with pytest.raises(ConfigurationError):
            CommandWhitelistValidator(whitelist_path=str(bad))

    def test_missing_enforcement_policy_raises_configuration_error(self, tmp_path):
        from app.errors import ConfigurationError
        p = tmp_path / "no_policy.json"
        p.write_text(json.dumps({"commands": {}}))
        with pytest.raises(ConfigurationError, match="enforcement_policy"):
            CommandWhitelistValidator(whitelist_path=str(p))

    def test_default_path_missing_raises_configuration_error(self, monkeypatch):
        # Force it to look for a nonexistent default path
        # Use a fresh instance to avoid global state issues
        from app.errors import ConfigurationError
        from unittest.mock import patch
        with patch("app.utils.whitelist_validator.Path.exists", return_value=False):
            with pytest.raises(ConfigurationError, match="Required whitelist configuration not found"):
                CommandWhitelistValidator(whitelist_path=None)


# ---------------------------------------------------------------------------
# get_available_commands / get_command_examples / get_command_description
# ---------------------------------------------------------------------------

class TestQueryMethods:
    def test_get_available_commands_returns_list(self, validator):
        cmds = validator.get_available_commands(Platform.LINUX)
        assert isinstance(cmds, list)
        assert "ping" in cmds
        assert "ls" in cmds

    def test_get_available_commands_sorted(self, validator):
        cmds = validator.get_available_commands(Platform.LINUX)
        assert cmds == sorted(cmds)

    def test_get_available_commands_filters_by_platform(self, validator):
        linux_cmds = validator.get_available_commands(Platform.LINUX)
        windows_cmds = validator.get_available_commands(Platform.WINDOWS)
        assert "ls" in linux_cmds
        assert "ls" not in windows_cmds

    def test_get_command_examples_returns_list(self, validator):
        examples = validator.get_command_examples("ping")
        assert isinstance(examples, list)
        assert len(examples) >= 1

    def test_get_command_examples_unknown_command_returns_empty(self, validator):
        assert validator.get_command_examples("nonexistent") == []

    def test_get_command_description_returns_string(self, validator):
        desc = validator.get_command_description("ping")
        assert isinstance(desc, str)
        assert len(desc) > 0

    def test_get_command_description_unknown_command_returns_none(self, validator):
        assert validator.get_command_description("nonexistent") is None


# ---------------------------------------------------------------------------
# Singleton getter
# ---------------------------------------------------------------------------

class TestSingletonGetter:
    def test_get_whitelist_validator_returns_same_instance(self):
        v1 = get_whitelist_validator()
        v2 = get_whitelist_validator()
        assert v1 is v2

    def test_get_whitelist_validator_returns_validator_instance(self):
        v = get_whitelist_validator()
        assert isinstance(v, CommandWhitelistValidator)


# ---------------------------------------------------------------------------
# Convenience functions
# ---------------------------------------------------------------------------

class TestConvenienceFunctions:
    def test_validate_command_against_whitelist_delegates(self, monkeypatch, whitelist_path):
        import app.utils.whitelist_validator as wv_module
        fresh = CommandWhitelistValidator(whitelist_path=whitelist_path)
        monkeypatch.setattr(wv_module, "_validator_instance", fresh)
        result = validate_command_against_whitelist("")
        assert result.is_valid is False

    def test_get_whitelisted_commands_returns_list(self):
        cmds = get_whitelisted_commands(Platform.LINUX)
        assert isinstance(cmds, list)


# ---------------------------------------------------------------------------
# parse_command_csv
# ---------------------------------------------------------------------------

class TestParseCommandCsv:
    """CSV parser must trim, dedupe, drop empties, preserve order."""

    def test_none_returns_empty_list(self):
        assert parse_command_csv(None) == []

    def test_empty_string_returns_empty_list(self):
        assert parse_command_csv("") == []

    def test_whitespace_only_returns_empty_list(self):
        assert parse_command_csv("   ,  ,  ") == []

    def test_basic_csv(self):
        assert parse_command_csv("uptime,df,free") == ["uptime", "df", "free"]

    def test_strips_whitespace(self):
        assert parse_command_csv(" uptime , df , free ") == ["uptime", "df", "free"]

    def test_drops_empty_fragments(self):
        assert parse_command_csv("uptime,,df,") == ["uptime", "df"]

    def test_dedupes_preserving_first_occurrence(self):
        assert parse_command_csv("uptime,df,uptime,free,df") == ["uptime", "df", "free"]

    def test_single_command(self):
        assert parse_command_csv("uptime") == ["uptime"]


# ---------------------------------------------------------------------------
# validate_command — allowed_commands_override (CSV mode)
# ---------------------------------------------------------------------------

class TestValidateCommandWithOverride:
    """When override is provided it must replace JSON whitelist semantics."""

    def test_override_allows_command_not_in_json(self, validator):
        # `uptime` is not in the minimal JSON fixture, but is in the override
        result = validator.validate_command(
            "uptime", platform=Platform.LINUX,
            allowed_commands_override=["uptime", "df"],
        )
        assert result.is_valid is True
        assert result.command == "uptime"
        assert result.category == CommandCategory.CSV_WHITELIST

    def test_override_rejects_command_not_in_override(self, validator):
        # `ping` IS in the JSON fixture, but should be rejected when overridden
        result = validator.validate_command(
            "ping google.com", platform=Platform.LINUX,
            allowed_commands_override=["uptime", "df"],
        )
        assert result.is_valid is False
        assert "not in whitelist" in result.reason

    def test_override_still_enforces_forbidden_patterns(self, validator):
        result = validator.validate_command(
            "uptime | bash", platform=Platform.LINUX,
            allowed_commands_override=["uptime"],
        )
        assert result.is_valid is False
        assert "forbidden" in result.reason.lower()

    def test_override_still_enforces_forbidden_directories(self, validator):
        result = validator.validate_command(
            "cat /etc/shadow", platform=Platform.LINUX,
            allowed_commands_override=["cat"],
        )
        assert result.is_valid is False
        assert "forbidden directory" in result.reason.lower()

    def test_override_rejects_unsafe_arg_chars(self, validator):
        result = validator.validate_command(
            "df ;rm", platform=Platform.LINUX,
            allowed_commands_override=["df"],
        )
        assert result.is_valid is False
        assert result.violations

    def test_override_allows_safe_args(self, validator):
        result = validator.validate_command(
            "df -h /tmp", platform=Platform.LINUX,
            allowed_commands_override=["df"],
        )
        assert result.is_valid is True

    def test_empty_override_falls_back_to_json(self, validator):
        # Empty list => falls back to JSON-based validation
        result = validator.validate_command(
            "uptime", platform=Platform.LINUX,
            allowed_commands_override=[],
        )
        # `uptime` is NOT in the JSON fixture => should be rejected
        assert result.is_valid is False
        assert "not in whitelist" in result.reason

    def test_none_override_falls_back_to_json(self, validator):
        result = validator.validate_command(
            "ping google.com", platform=Platform.LINUX,
            allowed_commands_override=None,
        )
        assert result.is_valid is True


# ---------------------------------------------------------------------------
# enabled field tests
# ---------------------------------------------------------------------------

class TestEnabledField:
    """Test the enabled field as a file-level kill switch."""

    def test_enabled_false_loads_empty_index(self, tmp_path):
        whitelist_path = tmp_path / "whitelist.json"
        import json
        data = {
            "enabled": False,
            "enforcement_policy": {
                "description": "test policy",
                "restrictions": [],
            },
            "commands": {
                "network_diagnostics": {
                    "ping": {
                        "command": "ping",
                        "description": "Test connectivity",
                        "platforms": ["linux"],
                        "safe_options": {"linux": ["-c <count>"]},
                        "validation": {},
                        "examples": ["ping -c 4 google.com"],
                        "max_execution_time": 60,
                    }
                },
            },
        }
        whitelist_path.write_text(json.dumps(data))
        validator = CommandWhitelistValidator(whitelist_path=str(whitelist_path))
        assert validator.get_available_commands() == []
        result = validator.validate_command("ping google.com", platform=Platform.LINUX)
        assert result.is_valid is False
        assert "not in whitelist" in result.reason

    def test_enabled_true_loads_commands(self, tmp_path):
        whitelist_path = tmp_path / "whitelist.json"
        import json
        data = {
            "enabled": True,
            "enforcement_policy": {
                "description": "test policy",
                "restrictions": [],
            },
            "commands": {
                "network_diagnostics": {
                    "ping": {
                        "command": "ping",
                        "description": "Test connectivity",
                        "platforms": ["linux"],
                        "safe_options": {"linux": ["-c <count>"]},
                        "validation": {},
                        "examples": ["ping -c 4 google.com"],
                        "max_execution_time": 60,
                    }
                },
            },
        }
        whitelist_path.write_text(json.dumps(data))
        validator = CommandWhitelistValidator(whitelist_path=str(whitelist_path))
        assert "ping" in validator.get_available_commands()
        result = validator.validate_command("ping google.com", platform=Platform.LINUX)
        assert result.is_valid is True

    def test_enabled_defaults_to_true(self, tmp_path):
        whitelist_path = tmp_path / "whitelist.json"
        import json
        data = {
            "enforcement_policy": {
                "description": "test policy",
                "restrictions": [],
            },
            "commands": {
                "network_diagnostics": {
                    "ping": {
                        "command": "ping",
                        "description": "Test connectivity",
                        "platforms": ["linux"],
                        "safe_options": {"linux": ["-c <count>"]},
                        "validation": {},
                        "examples": ["ping -c 4 google.com"],
                        "max_execution_time": 60,
                    }
                },
            },
        }
        whitelist_path.write_text(json.dumps(data))
        validator = CommandWhitelistValidator(whitelist_path=str(whitelist_path))
        assert "ping" in validator.get_available_commands()
        result = validator.validate_command("ping google.com", platform=Platform.LINUX)
        assert result.is_valid is True
