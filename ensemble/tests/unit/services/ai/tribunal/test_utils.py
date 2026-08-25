# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

import pytest
from app.constants import LLMProvider, ConsensusMember
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
    assert member_for_pass(0) == ConsensusMember.AXIOM
    assert member_for_pass(1) == ConsensusMember.CONCORD
    assert member_for_pass(2) == ConsensusMember.VARIANCE
    assert member_for_pass(3) == ConsensusMember.PRAGMA
    assert member_for_pass(4) == ConsensusMember.NEMESIS
    assert member_for_pass(5) == ConsensusMember.AXIOM
