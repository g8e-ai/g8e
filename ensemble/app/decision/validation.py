# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

"""Jev lite-role coexistence validation."""

from __future__ import annotations

import logging

from app.constants import LLMProvider
from app.models.settings import LLMSettings

# Call sites that use get_generative_lite_provider() for text generation.
# Triage and eval judge use DecisionProvider when lite_provider is jev.
GENERATIVE_LITE_CALL_SITES: tuple[str, ...] = (
    "case title generation",
    "memory extraction",
    "marshal response analysis",
)


def validate_jev_lite_coexistence(llm: LLMSettings) -> list[str]:
    """Return configuration errors when Jev on the lite role conflicts with enabled features."""
    if llm.lite_provider is not LLMProvider.JEV:
        return []

    errors: list[str] = []
    if llm.llm_command_gen_enabled:
        errors.append(
            "Lite provider 'jev' does not support Tribunal command generation. "
            "Disable G8E_LLM_COMMAND_GEN_ENABLED or select a generative lite provider."
        )
    return errors


def jev_generative_lite_limitations_message() -> str:
    """Summarize how generative lite features coexist when Jev is on the lite role."""
    sites = ", ".join(GENERATIVE_LITE_CALL_SITES)
    return (
        "Lite provider 'jev' supports triage and semantic eval judge via System One. "
        f"Generative lite features ({sites}) use the assistant provider when lite is jev."
    )


def log_jev_generative_lite_warning(logger: logging.Logger, llm: LLMSettings) -> None:
    """Emit a startup warning when Jev is configured on the lite role."""
    if llm.lite_provider is not LLMProvider.JEV:
        return
    logger.warning(jev_generative_lite_limitations_message())
