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
from app.constants import LLMProvider, TribunalMember
from app.models.settings import LLMSettings
from app.models.agents.tribunal import TribunalModelNotConfiguredError
from app.services.ai.tribunal.utils import is_system_error, member_for_pass, resolve_model

class TestResolveModel:
    """resolve_model returns a concrete model string with proper fallback chain."""

    def test_returns_lite_model_when_set(self):
        llm = LLMSettings(lite_model="custom-lite")
        assert resolve_model(llm, tier="lite") == "custom-lite"

    def test_falls_back_to_primary_model_when_lite_is_none(self):
        llm = LLMSettings(primary_model="custom-primary")
        assert llm.lite_model is None
        assert resolve_model(llm, tier="lite") == "custom-primary"

    def test_raises_when_both_models_none(self):
        llm = LLMSettings(primary_provider=LLMProvider.OLLAMA)
        assert llm.lite_model is None
        assert llm.primary_model is None
        with pytest.raises(TribunalModelNotConfiguredError) as exc_info:
            resolve_model(llm, tier="lite")
        assert exc_info.value.provider == "ollama"

    def test_raises_for_openai_when_no_model_configured(self):
        llm = LLMSettings(primary_provider=LLMProvider.OPENAI)
        with pytest.raises(TribunalModelNotConfiguredError) as exc_info:
            resolve_model(llm, tier="lite")
        assert exc_info.value.provider == "openai"

    def test_raises_for_anthropic_when_no_model_configured(self):
        llm = LLMSettings(primary_provider=LLMProvider.ANTHROPIC)
        with pytest.raises(TribunalModelNotConfiguredError) as exc_info:
            resolve_model(llm, tier="lite")
        assert exc_info.value.provider == "anthropic"

    def test_raises_for_gemini_when_no_model_configured(self):
        llm = LLMSettings(primary_provider=LLMProvider.GEMINI)
        with pytest.raises(TribunalModelNotConfiguredError) as exc_info:
            resolve_model(llm, tier="lite")
        assert exc_info.value.provider == "gemini"

    def test_lite_takes_priority_over_primary(self):
        llm = LLMSettings(primary_model="primary", lite_model="lite")
        assert resolve_model(llm, tier="lite") == "lite"

class TestIsSystemError:
    """is_system_error classifies error messages into system vs. model errors."""

    def test_auth_errors(self):
        assert is_system_error("401 Unauthorized")
        assert is_system_error("403 Forbidden")
        assert is_system_error("Invalid API key provided")
        assert is_system_error("Authentication failed for endpoint")

    def test_network_errors(self):
        assert is_system_error("Connection refused")
        assert is_system_error("ConnectionError: cannot reach host")
        assert is_system_error("Timeout waiting for response")
        assert is_system_error("DNS name resolution failed")
        assert is_system_error("SSL certificate verify failed")
        assert is_system_error("ECONNREFUSED 127.0.0.1:11434")

    def test_config_errors(self):
        assert is_system_error("Unsupported LLM provider: foo")

    def test_model_errors_are_not_system(self):
        assert not is_system_error("Model returned empty response")
        assert not is_system_error("Invalid JSON in response")
        assert not is_system_error("Unexpected response format")
        assert not is_system_error("Content filter triggered")

    def test_empty_string_is_not_system(self):
        assert not is_system_error("")

def test_member_for_pass():
    assert member_for_pass(0) == TribunalMember.AXIOM
    assert member_for_pass(1) == TribunalMember.CONCORD
    assert member_for_pass(2) == TribunalMember.VARIANCE
    assert member_for_pass(3) == TribunalMember.PRAGMA
    assert member_for_pass(4) == TribunalMember.NEMESIS
    assert member_for_pass(5) == TribunalMember.AXIOM
