# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

"""Unit tests for LLM factory Jev guardrails."""

from __future__ import annotations

import pytest

from app.constants import LLMProvider
from app.errors import ConfigurationError
from app.llm.factory import get_generative_lite_provider, get_llm_provider
from app.llm.providers.fake import FakeProvider
from app.models.settings import LLMSettings

pytestmark = pytest.mark.unit


class TestLlmFactoryJevGuardrails:
    def test_get_llm_provider_rejects_jev_for_lite_text_generation(self):
        settings = LLMSettings(
            lite_provider=LLMProvider.JEV,
            lite_model="jev-latest",
            jev_api_key="ts_test_key",
        )

        with pytest.raises(ConfigurationError, match="does not support lite text generation"):
            get_llm_provider(settings, is_lite=True)

    def test_get_generative_lite_provider_falls_back_to_assistant_when_lite_is_jev(self):
        settings = LLMSettings(
            lite_provider=LLMProvider.JEV,
            lite_model="jev-latest",
            jev_api_key="ts_test_key",
            assistant_provider=LLMProvider.FAKE,
            assistant_model="fake-assistant",
        )

        provider = get_generative_lite_provider(settings)

        assert isinstance(provider, FakeProvider)

    def test_get_generative_lite_provider_uses_lite_role_for_generative_providers(self):
        settings = LLMSettings(
            lite_provider=LLMProvider.FAKE,
            lite_model="fake-lite",
            assistant_provider=LLMProvider.FAKE,
            assistant_model="fake-assistant",
        )

        provider = get_generative_lite_provider(settings)

        assert isinstance(provider, FakeProvider)
