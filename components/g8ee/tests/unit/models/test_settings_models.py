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

import pytest
from app.models.settings import LLMSettings

pytestmark = [pytest.mark.unit]


class TestLLMSettingsResolvedAssistantModel:

    def test_returns_model_when_set(self):
        llm = LLMSettings(assistant_model="gemma3:4b")
        assert llm.resolved_assistant_model == "gemma3:4b"

    def test_returns_none_when_not_set(self):
        llm = LLMSettings()
        assert llm.resolved_assistant_model is None

    def test_returns_none_for_empty_string(self):
        llm = LLMSettings(assistant_model="")
        assert llm.resolved_assistant_model is None

    def test_does_not_fallback_to_primary_model(self):
        llm = LLMSettings(primary_model="gemma3:27b")
        assert llm.resolved_assistant_model is None

    def test_independent_of_primary_model(self):
        llm = LLMSettings(primary_model="gemma3:27b", assistant_model="gemma3:4b")
        assert llm.resolved_assistant_model == "gemma3:4b"
