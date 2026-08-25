# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

from unittest.mock import patch

import pytest

from app.llm.providers.llama_cpp import LlamaCppProvider
from app.llm.providers.open_ai import OpenAIProvider


@pytest.mark.unit
class TestOpenAICompatibility:
    def test_openai_provider_identity(self):
        with patch("app.llm.providers.open_ai.AsyncOpenAI"):
            provider = OpenAIProvider(endpoint="http://test", api_key="test")
            assert provider.service_name == "openai"

    def test_llamacpp_provider_identity(self):
        with patch("app.llm.providers.open_ai.AsyncOpenAI"):
            provider = LlamaCppProvider(endpoint="http://test", api_key="test")
            assert provider.service_name == "llamacpp"
            assert isinstance(provider, OpenAIProvider)
