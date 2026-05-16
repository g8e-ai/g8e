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

