# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.


from app.constants import Platform, CommandCategory

from .base import G8eBaseModel, Field


class WhitelistedCommand(G8eBaseModel):
    """Metadata for a whitelisted command, including constraints and examples.

    Internal g8ee whitelist contract.
    """

    command: str
    category: CommandCategory | None = None
    description: str | None = None
    safe_options: list[str] = Field(default_factory=list)
    validation: dict[str, str] = Field(default_factory=dict)
    examples: list[str] = Field(default_factory=list)
    max_execution_time: int | None = None


class CommandValidationResult(G8eBaseModel):
    """Result of command validation against whitelist."""

    is_valid: bool
    command: str
    category: CommandCategory | None = None
    platform: Platform | None = None
    reason: str | None = None
    max_execution_time: int | None = None
    safe_options_used: list[str] = Field(default_factory=list)
    violations: list[str] = Field(default_factory=list)
