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

"""
Unit tests for G8elProvider.

Tests construction, service_name, and inheritance from LlamaCppProvider.
Note: Basic identity tests are in test_openai_compatibility.py
"""

import pytest
from unittest.mock import patch

from app.llm.providers.g8el import G8elProvider
from app.llm.providers.llama_cpp import LlamaCppProvider
from app.llm.providers.open_ai import OpenAIProvider

pytestmark = [pytest.mark.unit]


class TestG8elProviderIdentity:
    """Test G8elProvider identity and inheritance."""

    def test_service_name_property(self):
        with patch("app.llm.providers.open_ai.AsyncOpenAI"):
            provider = G8elProvider(endpoint="http://g8el:11444", api_key="test-key")
            assert provider.service_name == "g8el"

    def test_inherits_from_llamacpp_provider(self):
        with patch("app.llm.providers.open_ai.AsyncOpenAI"):
            provider = G8elProvider(endpoint="http://g8el:11444", api_key="test-key")
            assert isinstance(provider, LlamaCppProvider)

    def test_inherits_from_openai_provider(self):
        with patch("app.llm.providers.open_ai.AsyncOpenAI"):
            provider = G8elProvider(endpoint="http://g8el:11444", api_key="test-key")
            assert isinstance(provider, OpenAIProvider)
