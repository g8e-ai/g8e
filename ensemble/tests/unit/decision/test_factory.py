# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

"""Unit tests for decision provider factory."""

from __future__ import annotations

import pytest

from app.constants import JEV_DEFAULT_ENDPOINT, JEV_DEFAULT_MODEL, LLMProvider
from app.decision.factory import (
    clear_decision_provider_cache,
    get_decision_provider,
    reset_decision_provider_cache,
)
from app.decision.providers.jev import JevProvider
from app.errors import ConfigurationError
from app.models.settings import LLMSettings

pytestmark = pytest.mark.unit


@pytest.fixture(autouse=True)
def _reset_cache():
    reset_decision_provider_cache()
    yield
    reset_decision_provider_cache()


def _jev_settings(**overrides) -> LLMSettings:
    base = {
        "lite_provider": LLMProvider.JEV,
        "lite_model": JEV_DEFAULT_MODEL,
        "jev_api_key": "ts_test_key",
        "jev_endpoint": JEV_DEFAULT_ENDPOINT,
    }
    base.update(overrides)
    return LLMSettings(**base)


class TestDecisionProviderFactory:
    def test_get_decision_provider_returns_jev_when_lite_role_is_jev(self):
        provider = get_decision_provider(_jev_settings())
        assert isinstance(provider, JevProvider)

    def test_get_decision_provider_rejects_non_jev_lite_provider(self):
        settings = LLMSettings(lite_provider=LLMProvider.OLLAMA)
        with pytest.raises(ConfigurationError, match="lite_provider is not 'jev'"):
            get_decision_provider(settings)

    def test_get_decision_provider_caches_by_endpoint_and_api_key(self):
        first = get_decision_provider(_jev_settings())
        second = get_decision_provider(_jev_settings())
        assert first is second

        different_key = get_decision_provider(_jev_settings(jev_api_key="ts_other_key"))
        assert different_key is not first

    @pytest.mark.asyncio
    async def test_clear_decision_provider_cache_closes_cached_instances(self):
        provider = get_decision_provider(_jev_settings())
        provider._client = object()
        provider._owns_client = False

        closed = False

        async def _force_close():
            nonlocal closed
            closed = True

        provider.force_close = _force_close  # type: ignore[method-assign]

        await clear_decision_provider_cache()
        assert closed is True
