# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

"""Decision provider factory."""

from __future__ import annotations

import hashlib
import logging

from app.constants import JEV_DEFAULT_ENDPOINT, JEV_DEFAULT_MODEL, LLMProvider
from app.decision.provider import DecisionProvider
from app.decision.providers.jev import JevProvider
from app.errors import ConfigurationError
from app.models.settings import LLMSettings

logger = logging.getLogger(__name__)

_provider_cache: dict[str, DecisionProvider] = {}


def _api_key_fingerprint(api_key: str | None) -> str:
    if not api_key:
        return ""
    return hashlib.sha256(api_key.encode()).hexdigest()[:16]


def _get_provider_cache_key(settings: LLMSettings) -> str:
    _, api_key, endpoint, _ = settings.resolve("lite")
    endpoint = endpoint or JEV_DEFAULT_ENDPOINT
    return f"jev|{endpoint}|{_api_key_fingerprint(api_key)}"


def get_decision_provider(settings: LLMSettings) -> DecisionProvider:
    """Return a cached DecisionProvider when lite role resolves to Jev."""
    if settings.lite_provider is not LLMProvider.JEV:
        raise ConfigurationError(
            "Decision provider requested but lite_provider is not 'jev'. "
            "Configure G8E_LLM_LITE_PROVIDER=jev to use System One."
        )

    cache_key = _get_provider_cache_key(settings)
    if cache_key in _provider_cache:
        return _provider_cache[cache_key]

    _, api_key, endpoint, _ = settings.resolve("lite")
    model = settings.resolved_lite_model or JEV_DEFAULT_MODEL

    provider = JevProvider(
        api_key=api_key,
        endpoint=endpoint or JEV_DEFAULT_ENDPOINT,
        default_model=model,
    )
    provider._is_cached_singleton = True
    _provider_cache[cache_key] = provider
    return provider


async def clear_decision_provider_cache() -> None:
    """Close and clear all cached decision provider instances."""
    for provider in _provider_cache.values():
        try:
            await provider.force_close()
        except Exception as exc:
            logger.info("Error closing decision provider during cache clear: %s", exc)
    _provider_cache.clear()


def reset_decision_provider_cache() -> None:
    """Reset decision provider cache without closing. Intended for tests only."""
    _provider_cache.clear()
