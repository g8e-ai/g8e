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

from app.constants import TribunalMember
from app.models.settings import LLMSettings
from app.models.agents.tribunal import TribunalModelNotConfiguredError

def _is_system_error(error_message: str) -> bool:
    """Classify an error message as a system error vs. a model error."""
    error_lower = error_message.lower()
    if "safety validation failed" in error_lower:
        return False
    system_indicators = [
        "401", "403", "unauthorized", "forbidden",
        "authentication", "api key",
        "connection refused", "connectionerror", "timeout",
        "dns", "ssl", "econnrefused",
        "unsupported llm provider",
    ]
    return any(indicator in error_lower for indicator in system_indicators)

def _member_for_pass(pass_index: int) -> TribunalMember:
    """Map a pass index to a Tribunal member."""
    members = [
        TribunalMember.AXIOM,
        TribunalMember.CONCORD,
        TribunalMember.VARIANCE,
        TribunalMember.PRAGMA,
        TribunalMember.NEMESIS,
    ]
    return members[pass_index % len(members)]

def _resolve_model(llm_settings: LLMSettings, tier: str = "assistant", request: str = "") -> str:
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

    provider = llm_settings.primary_provider or llm_settings.assistant_provider or llm_settings.lite_provider
    raise TribunalModelNotConfiguredError(
        provider=provider.value if provider else "unknown",
        request=request,
    )
