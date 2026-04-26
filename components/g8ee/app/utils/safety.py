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

import logging
from typing import Optional

from app.constants import FORBIDDEN_COMMAND_PATTERNS, DEFAULT_OS_NAME
from app.constants.status import Platform, CommandErrorType
from app.models.agent import OperatorContext
from app.utils.validators import get_blacklist_validator, get_whitelist_validator
from app.utils.whitelist_validator import CommandWhitelistValidator
from app.utils.blacklist_validator import CommandBlacklistValidator

logger = logging.getLogger(__name__)


class SafetyValidationResult:
    """Structured result from command safety validation.
    
    Replaces raw (bool, str) tuple with typed fields for better error type resolution.
    """
    def __init__(
        self,
        is_safe: bool,
        error_message: str | None = None,
        error_type: CommandErrorType | None = None,
    ) -> None:
        self.is_safe = is_safe
        self.error_message = error_message
        self.error_type = error_type
    
    @property
    def safe(self) -> bool:
        """Alias for is_safe for backward compatibility."""
        return self.is_safe


def map_os_string_to_platform(os_name: str | None) -> Platform:
    """Convert OS string to Platform enum.
    
    Centralized mapping to eliminate repetitive if/elif blocks across the codebase.
    
    Args:
        os_name: OS string (e.g., "linux", "windows", "darwin", "macos")
        
    Returns:
        Platform enum value, defaults to LINUX if unknown
    """
    if not os_name:
        return Platform.LINUX
    
    os_lower = os_name.lower()
    if os_lower == "windows":
        return Platform.WINDOWS
    elif os_lower == "darwin" or os_lower == "macos":
        return Platform.DARWIN
    else:
        return Platform.LINUX

def validate_command_safety(
    command: str,
    whitelisting_enabled: bool,
    blacklisting_enabled: bool,
    operator_context: OperatorContext | None,
    whitelisted_commands_override: list[str] | None = None,
    whitelist_validator: CommandWhitelistValidator | None = None,
    blacklist_validator: CommandBlacklistValidator | None = None,
) -> SafetyValidationResult:
    """Validate a command against technical (L1) safety rules.

    This is the technical bedrock that all agents must respect.
    Checks:
    1. Forbidden patterns (sudo, etc.)
    2. Blacklist (if enabled)
    3. Whitelist (if enabled)

    Args:
        command: The command string to validate.
        whitelisting_enabled: Whether whitelist enforcement is active.
        blacklisting_enabled: Whether blacklist enforcement is active.
        operator_context: Optional operator context (used for platform mapping).
        whitelisted_commands_override: Optional user-supplied list of base
            commands that fully replaces the JSON whitelist when non-empty.
            Empty/None falls back to the JSON whitelist.
        whitelist_validator: Optional injected whitelist validator instance.
        blacklist_validator: Optional injected blacklist validator instance.

    Returns SafetyValidationResult with is_safe, error_message, and error_type.
    """
    if not command:
        return SafetyValidationResult(
            is_safe=False,
            error_message="Empty command",
            error_type=CommandErrorType.VALIDATION_ERROR,
        )

    # 1. Check global forbidden patterns (sudo, su, etc.)
    # These are hardcoded safety invariants.
    command_lower = command.lower()
    for pattern in FORBIDDEN_COMMAND_PATTERNS:
        if pattern in command_lower:
            logger.info("Command blocked by global forbidden pattern: %s", pattern)
            return SafetyValidationResult(
                is_safe=False,
                error_message=f"Command contains forbidden pattern: {pattern}",
                error_type=CommandErrorType.SECURITY_VIOLATION,
            )

    # 2. Check blacklist
    # The blacklist blocks specific dangerous commands/substrings.
    if blacklisting_enabled:
        validator = blacklist_validator or get_blacklist_validator()
        blacklist_result = validator.validate_command(command)
        if not blacklist_result.is_allowed:
            logger.info("Command blocked by blacklist: %s", blacklist_result.reason)
            return SafetyValidationResult(
                is_safe=False,
                error_message=f"Command blocked by blacklist: {blacklist_result.reason or 'forbidden entry'}",
                error_type=CommandErrorType.BLACKLIST_VIOLATION,
            )

    # 3. Check whitelist
    # The whitelist is the strict list of permitted commands and arguments.
    if whitelisting_enabled:
        os_name = operator_context.os if operator_context else DEFAULT_OS_NAME
        platform = map_os_string_to_platform(os_name)

        validator = whitelist_validator or get_whitelist_validator()
        whitelist_result = validator.validate_command(
            command, platform, allowed_commands_override=whitelisted_commands_override
        )
        if not whitelist_result.is_valid:
            logger.info("Command blocked by whitelist: %s", whitelist_result.reason)
            return SafetyValidationResult(
                is_safe=False,
                error_message=f"Command not whitelisted: {whitelist_result.reason}",
                error_type=CommandErrorType.WHITELIST_VIOLATION,
            )

    return SafetyValidationResult(is_safe=True, error_message=None, error_type=None)
