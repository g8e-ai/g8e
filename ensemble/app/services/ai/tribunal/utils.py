# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

from app.constants import ConsensusMember
from app.models.settings import LLMSettings
from app.models.agents.tribunal import TribunalModelNotConfiguredError


def is_system_error(error_message: str) -> bool:
    """Classify an error message as a system error vs. a model error."""
    error_lower = error_message.lower()
    if "safety validation failed" in error_lower:
        return False
    system_indicators = [
        "401",
        "403",
        "unauthorized",
        "forbidden",
        "authentication",
        "api key",
        "connection refused",
        "connectionerror",
        "timeout",
        "dns",
        "ssl",
        "econnrefused",
        "unsupported llm provider",
    ]
    return any(indicator in error_lower for indicator in system_indicators)


def member_for_pass(pass_index: int) -> ConsensusMember:
    """Map a pass index to a Tribunal member."""
    members = [
        ConsensusMember.AXIOM,
        ConsensusMember.CONCORD,
        ConsensusMember.VARIANCE,
        ConsensusMember.PRAGMA,
        ConsensusMember.NEMESIS,
    ]
    return members[pass_index % len(members)]


def resolve_model(llm_settings: LLMSettings, tier: str = "assistant", request: str = "") -> str:
    """Resolve the concrete model string from settings based on tier."""
    if tier == "lite":
        resolved = llm_settings.resolved_lite_model
        if resolved:
            return resolved

    if tier == "assistant" and llm_settings.assistant_model:
        return llm_settings.assistant_model

    if tier == "primary" and llm_settings.primary_model:
        return llm_settings.primary_model

    # Fallback chain: lite -> assistant -> primary
    resolved = llm_settings.resolved_lite_model
    if resolved:
        return resolved
    if llm_settings.assistant_model:
        return llm_settings.assistant_model
    if llm_settings.primary_model:
        return llm_settings.primary_model

    provider = (
        llm_settings.primary_provider
        or llm_settings.assistant_provider
        or llm_settings.lite_provider
    )
    raise TribunalModelNotConfiguredError(
        provider=provider.value if provider else "unknown",
        request=request,
    )
