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
import logging
import re
from pathlib import Path
from typing import Any

from app.constants import Platform, CommandCategory
from app.errors import ConfigurationError
from app.models.whitelist import CommandValidationResult, WhitelistedCommand
from app.utils.config_loader import load_json_config

logger = logging.getLogger(__name__)

_COMMON_SAFE_PATTERNS: dict[str, str] = {
    "path": r"^[/a-zA-Z0-9._-]+$",
    "file": r"^[/a-zA-Z0-9._-]+$",
    "directory": r"^[/a-zA-Z0-9._-]+$",
    "target": r"^[a-zA-Z0-9.-]+$",
    "domain": r"^[a-zA-Z0-9.-]+$",
    "host": r"^[a-zA-Z0-9.-]+$",
    "url": r"^https?://[a-zA-Z0-9.-]+(:[0-9]+)?(/.*)?$",
    "simple_value": r"^[a-zA-Z0-9._-]+$",
}


class CommandWhitelistValidator:
    """Validates commands against the L1-L3 technical support whitelist."""

    def __init__(self, whitelist_path: str):
        """Initialize validator with whitelist configuration."""
        self.whitelist_data: dict[str, Any] = {}
        self.commands_by_category: dict[str, dict[str, Any]] = {}
        self.all_commands: set[str] = set()
        self.forbidden_patterns: list[str] = []
        self.forbidden_directories: list[str] = []

        if whitelist_path:
            self.load_whitelist(whitelist_path)
        else:
            default_path = Path(__file__).parent.parent.parent / "config" / "whitelist.json"
            if default_path.exists():
                self.load_whitelist(str(default_path))
            else:
                logger.error("Command whitelist not found - AI command execution disabled for security")
                raise ConfigurationError(f"Required whitelist configuration not found at {default_path}")

    def load_whitelist(self, whitelist_path: str) -> None:
        """Load whitelist configuration from JSON file."""
        logger.info("Loading command whitelist from %s", whitelist_path)
        self.whitelist_data = load_json_config(Path(whitelist_path), config_name="whitelist configuration")

        enabled = self.whitelist_data.get("enabled", True)
        if not enabled:
            logger.warning(
                "Command whitelist is disabled via 'enabled: false' in %s; "
                "loading empty index (no commands will be allowed at the JSON level)",
                whitelist_path,
            )
            self.commands_by_category = {}
            self.all_commands = set()
            self.forbidden_patterns = []
            self.forbidden_directories = []
            return

        self._parse_whitelist_data()

        enforcement_policy = self.whitelist_data.get("enforcement_policy")
        if not enforcement_policy:
            raise ConfigurationError("Whitelist missing required enforcement_policy section")

        logger.info("Loaded command whitelist with %d commands from %s", len(self.all_commands), whitelist_path)

    def _parse_whitelist_data(self) -> None:
        """Parse loaded whitelist data into internal structures."""
        commands_section = self.whitelist_data.get("commands", {})

        for category_name, category_data in commands_section.items():
            self.commands_by_category[category_name] = {}

            for command_name, command_config in category_data.items():
                self.commands_by_category[category_name][command_name] = command_config
                self.all_commands.add(command_config.get("command", command_name))

        global_restrictions = self.whitelist_data.get("global_restrictions", {})
        self.forbidden_patterns = global_restrictions.get("forbidden_patterns", [])
        self.forbidden_directories = global_restrictions.get("forbidden_directories", [])

        logger.info("Parsed whitelist: %d categories, %d forbidden patterns", len(self.commands_by_category), len(self.forbidden_patterns))

    def validate_command(
        self,
        command_string: str,
        platform: Platform = Platform.LINUX,
        allowed_commands_override: list[str] | None = None,
    ) -> CommandValidationResult:
        """
        Validate a command string against the whitelist.

        Args:
            command_string: Full command to validate (e.g., "ping -c 4 google.com")
            platform: Target platform
            allowed_commands_override: Optional list of base commands that fully
                replaces the JSON whitelist when non-empty. When provided, the
                command is validated as: (1) global forbidden patterns/directories,
                (2) base command must be in the override set, (3) every argument
                must pass ``_is_safe_value``. The JSON whitelist's per-command
                ``safe_options`` and ``validation`` patterns are NOT consulted in
                this mode. An empty/None override falls back to the JSON whitelist.

        Returns:
            CommandValidationResult with validation details
        """
        if not command_string or not command_string.strip():
            logger.info("Empty command string provided")
            return CommandValidationResult(
                is_valid=False,
                command="",
                reason="Empty command"
            )

        command_string = command_string.strip()
        parts = command_string.split()
        base_command = parts[0]
        command_args = parts[1:] if len(parts) > 1 else []

        logger.info("Validating command: %s with args: %s", base_command, command_args)

        for pattern in self.forbidden_patterns:
            try:
                if re.search(pattern, command_string, re.IGNORECASE):
                    logger.info("Command blocked by forbidden pattern: %s", pattern)
                    return CommandValidationResult(
                        is_valid=False,
                        command=base_command,
                        reason=f"Contains forbidden pattern: {pattern}",
                        violations=[f"forbidden_pattern: {pattern}"]
                    )
            except re.error as e:
                logger.error("Invalid forbidden pattern regex '%s': %s", pattern, e)
                continue

        for forbidden_dir in self.forbidden_directories:
            if forbidden_dir in command_string:
                logger.info("Command blocked by forbidden directory: %s", forbidden_dir)
                return CommandValidationResult(
                    is_valid=False,
                    command=base_command,
                    reason=f"Accesses forbidden directory: {forbidden_dir}",
                    violations=[f"forbidden_directory: {forbidden_dir}"]
                )

        # CSV override mode: if user supplied an explicit allow-list, replace the
        # JSON whitelist entirely (per design choice). Per-command arg metadata is
        # not available, so we fall back to the conservative `_is_safe_value`
        # check on every argument.
        if allowed_commands_override:
            if base_command not in allowed_commands_override:
                logger.info(
                    "Command '%s' not in user-configured whitelist (CSV mode)", base_command
                )
                return CommandValidationResult(
                    is_valid=False,
                    command=base_command,
                    reason=f"Command '{base_command}' not in whitelist",
                )
            violations: list[str] = []
            for arg in command_args:
                if not self._is_safe_value(arg):
                    violations.append(f"Argument '{arg}' contains unsafe characters or format")
            if violations:
                return CommandValidationResult(
                    is_valid=False,
                    command=base_command,
                    reason=f"Invalid arguments: {'; '.join(violations)}",
                    violations=violations,
                )
            return CommandValidationResult(
                is_valid=True,
                command=base_command,
                category=CommandCategory.CSV_WHITELIST,
                platform=platform,
            )

        command_config = self._find_command_config(base_command)
        if not command_config:
            logger.info("Command '%s' not found in whitelist", base_command)
            return CommandValidationResult(
                is_valid=False,
                command=base_command,
                reason=f"Command '{base_command}' not in whitelist"
            )

        category_name, config = command_config
        logger.info("Found command '%s' in category '%s'", base_command, category_name)

        category_enum = self._parse_category(category_name)

        supported_platforms = config.get("platforms", [])
        if platform not in supported_platforms:
            logger.info("Command '%s' not supported on %s platform", base_command, platform)
            return CommandValidationResult(
                is_valid=False,
                command=base_command,
                category=category_enum,
                platform=platform,
                reason=f"Command not supported on {platform} platform"
            )

        validation_result = self._validate_command_arguments(
            base_command, command_args, config, platform
        )

        if not validation_result.is_valid:
            logger.info("Command arguments validation failed: %s", validation_result.reason)
            return validation_result

        logger.info("Command '%s' validated successfully", command_string)
        return CommandValidationResult(
            is_valid=True,
            command=base_command,
            category=category_enum,
            platform=platform,
            max_execution_time=config.get("max_execution_time"),
            safe_options_used=validation_result.safe_options_used
        )

    def _find_command_config(self, command: str) -> tuple[str, dict[str, Any]] | None:
        """Find command configuration in whitelist."""
        for category_name, commands in self.commands_by_category.items():
            for exec_name, config in commands.items():
                if config.get("command", exec_name) == command:
                    return category_name, config
        return None

    def _parse_category(self, category_name: str) -> CommandCategory | None:
        """Parse category string to CommandCategory enum."""
        category_map = {
            "csv_whitelist": CommandCategory.CSV_WHITELIST,
            "network_diagnostics": CommandCategory.NETWORK_DIAGNOSTICS,
            "system_diagnostics": CommandCategory.SYSTEM_DIAGNOSTICS,
        }
        return category_map.get(category_name)

    def _validate_command_arguments(
        self,
        command: str,
        args: list[str],
        config: dict[str, Any],
        platform: Platform
    ) -> CommandValidationResult:
        """Validate command arguments against safe options and validation patterns."""
        violations = []
        safe_options_used = []

        safe_options = config.get("safe_options", {})
        if isinstance(safe_options, dict):
            platform_options = safe_options.get(platform, [])
        else:
            platform_options = safe_options

        validation_patterns = config.get("validation", {})

        i = 0
        while i < len(args):
            arg = args[i]
            option_matched = False

            for safe_option in platform_options:
                if self._matches_safe_option(arg, args[i:], safe_option):
                    safe_options_used.append(safe_option)
                    option_matched = True

                    if "<" in safe_option and ">" in safe_option:
                        param_name = self._extract_parameter_name(safe_option)
                        param_value = None

                        if "=" in arg:
                            _, param_value = arg.split("=", 1)
                        elif i + 1 < len(args):
                            param_value = args[i + 1]
                            i += 1

                        if param_value is not None:
                            if param_name and param_name in validation_patterns:
                                pattern = validation_patterns[param_name]
                                if not re.match(pattern, param_value):
                                    violations.append(f"Parameter {param_name}='{param_value}' doesn't match pattern {pattern}")
                            elif not self._is_safe_value(param_value):
                                violations.append(f"Parameter value '{param_value}' contains unsafe characters")
                    break

            if not option_matched:
                pattern_matched = False
                for pattern_name, pattern in validation_patterns.items():
                    try:
                        if re.match(pattern, arg):
                            pattern_matched = True
                            break
                    except re.error:
                        logger.error("Invalid regex pattern '%s' for %s", pattern, pattern_name)
                        continue

                if not pattern_matched:
                    for pattern_name, pattern in _COMMON_SAFE_PATTERNS.items():
                        try:
                            if re.match(pattern, arg):
                                pattern_matched = True
                                break
                        except re.error:
                            continue

                if not pattern_matched and not self._is_safe_value(arg):
                    violations.append(f"Argument '{arg}' contains unsafe characters or format")

            i += 1

        if violations:
            return CommandValidationResult(
                is_valid=False,
                command=command,
                reason=f"Invalid arguments: {'; '.join(violations)}",
                violations=violations
            )

        return CommandValidationResult(
            is_valid=True,
            command=command,
            safe_options_used=safe_options_used
        )

    def _is_safe_value(self, value: str) -> bool:
        """Check if a value contains only safe characters."""
        if not value:
            return False

        if value == "-" or value == "--":
            return False

        unsafe_chars = [";", "&", "`", "$", "(", ")", "{", "}", "<", ">", "\\", "\n", "\r", "\t"]
        for char in unsafe_chars:
            if char in value:
                return False

        return True

    def _matches_safe_option(self, arg: str, remaining_args: list[str], safe_option: str) -> bool:
        """Check if argument matches a safe option pattern."""
        if "<" not in safe_option:
            return arg == safe_option

        if "=" in safe_option and "=" in arg:
            option_part, _ = safe_option.split("=", 1)
            arg_part, _ = arg.split("=", 1)
            return option_part.strip() == arg_part.strip()
        option_part = safe_option.split("<")[0].strip()
        return arg == option_part

    def _extract_parameter_name(self, safe_option: str) -> str | None:
        """Extract parameter name from safe option (e.g., "-c <count>" -> "count", "--max-depth=<depth>" -> "depth")."""
        match = re.search(r"<(\w+)>", safe_option)
        return match.group(1) if match else None

    def get_command_metadata(self, command: str, platform: Platform = Platform.LINUX) -> WhitelistedCommand | None:
        """Get full metadata for a command, including safe options and validation."""
        command_config = self._find_command_config(command)
        if not command_config:
            return None
        
        category_name, config = command_config
        category_enum = self._parse_category(category_name)
        
        safe_options = config.get("safe_options", {})
        if isinstance(safe_options, dict):
            platform_options = safe_options.get(platform, [])
        else:
            platform_options = safe_options
            
        return WhitelistedCommand(
            command=config.get("command", command),
            category=category_enum,
            description=config.get("description"),
            safe_options=platform_options,
            validation=config.get("validation", {}),
            examples=config.get("examples", []),
            max_execution_time=config.get("max_execution_time")
        )

    def get_available_commands_with_metadata(self, platform: Platform = Platform.LINUX) -> list[WhitelistedCommand]:
        """Get list of all available commands for a platform with their metadata."""
        available = []
        for category_name, commands in self.commands_by_category.items():
            for exec_name, config in commands.items():
                if platform in config.get("platforms", []):
                    metadata = self.get_command_metadata(config.get("command", exec_name), platform)
                    if metadata:
                        available.append(metadata)
        return sorted(available, key=lambda x: x.command)

    def get_available_commands(self, platform: Platform = Platform.LINUX) -> list[str]:
        """Get list of all available commands for a platform."""
        available = []
        for category_name, commands in self.commands_by_category.items():
            for exec_name, config in commands.items():
                if platform in config.get("platforms", []):
                    available.append(config.get("command", exec_name))
        return sorted(available)

    def get_command_examples(self, command: str) -> list[str]:
        """Get example usage for a command."""
        command_config = self._find_command_config(command)
        if command_config:
            _, config = command_config
            return config.get("examples", [])
        return []

    def get_command_description(self, command: str) -> str | None:
        """Get description for a command."""
        command_config = self._find_command_config(command)
        if command_config:
            _, config = command_config
            return config.get("description")
        return None


_validator_instance: CommandWhitelistValidator | None = None


def register_whitelist_validator(validator: CommandWhitelistValidator) -> None:
    """Explicitly register the global whitelist validator instance."""
    global _validator_instance
    _validator_instance = validator


def get_whitelist_validator(whitelist_path: str | None = None) -> CommandWhitelistValidator:
    """Get global whitelist validator instance.

    If no validator has been explicitly registered, one will be created from
    the default path (or the provided path). This backward-compatibility mode
    is deprecated; new code should use register_whitelist_validator().
    """
    global _validator_instance
    if _validator_instance is None:
        logger.warning(
            "get_whitelist_validator() called without explicit registration; "
            "creating validator implicitly. Use register_whitelist_validator() for explicit DI."
        )
        _validator_instance = CommandWhitelistValidator(whitelist_path=whitelist_path or "")
    return _validator_instance


def validate_command_against_whitelist(
    command: str,
    platform: Platform = Platform.LINUX,
    allowed_commands_override: list[str] | None = None,
) -> CommandValidationResult:
    """Validate a command against the whitelist (convenience function).

    See ``CommandWhitelistValidator.validate_command`` for the semantics of
    ``allowed_commands_override``.
    """
    validator = get_whitelist_validator()
    return validator.validate_command(command, platform, allowed_commands_override)


def parse_whitelisted_commands_csv(csv: str | None) -> list[str]:
    """Parse a comma-separated whitelist string into an ordered, deduplicated list.

    **Deprecated:** Use `app.utils.csv_commands.parse_command_csv` instead.
    This function is kept for backward compatibility and will be removed in a future release.
    """
    import warnings
    from app.utils.csv_commands import parse_command_csv

    warnings.warn(
        "parse_whitelisted_commands_csv is deprecated; use app.utils.csv_commands.parse_command_csv instead",
        DeprecationWarning,
        stacklevel=2,
    )
    return parse_command_csv(csv)


def get_whitelisted_commands(platform: Platform = Platform.LINUX) -> list[str]:
    """Get all whitelisted commands for a platform (convenience function)."""
    validator = get_whitelist_validator()
    return validator.get_available_commands(platform)
