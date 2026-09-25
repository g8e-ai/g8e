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
from app.llm.factory import get_llm_provider
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
