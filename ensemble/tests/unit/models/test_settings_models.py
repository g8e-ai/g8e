# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

import pytest
from pydantic import ValidationError as PydanticValidationError

from app.constants.config import LLMProvider
from app.models.settings import BatchExecutionSettings, LLMSettings, G8eeAppSettings

pytestmark = [pytest.mark.unit]


class TestBatchExecutionSettingsBounds:
    def test_default_max_concurrency(self):
        be = BatchExecutionSettings()
        assert be.max_concurrency == 10
        assert be.fail_fast is False

    def test_accepts_valid_concurrency(self):
        be = BatchExecutionSettings(max_concurrency=32)
        assert be.max_concurrency == 32

    def test_rejects_zero_or_negative_concurrency(self):
        with pytest.raises(PydanticValidationError):
            BatchExecutionSettings(max_concurrency=0)
        with pytest.raises(PydanticValidationError):
            BatchExecutionSettings(max_concurrency=-1)

    def test_rejects_absurdly_large_concurrency(self):
        # Guards against a misconfiguration that would fan out to every operator at once.
        with pytest.raises(PydanticValidationError):
            BatchExecutionSettings(max_concurrency=10000)


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


class TestLLMSettingsResolvedLiteModel:
    def test_returns_lite_model_when_set(self):
        llm = LLMSettings(lite_model="gemma3:1b")
        assert llm.resolved_lite_model == "gemma3:1b"

    def test_returns_none_when_nothing_configured(self):
        llm = LLMSettings()
        assert llm.resolved_lite_model is None

    def test_returns_none_for_empty_lite_model(self):
        llm = LLMSettings(lite_model="")
        assert llm.resolved_lite_model is None

    def test_falls_back_to_assistant_model(self):
        llm = LLMSettings(assistant_model="gemma3:4b")
        assert llm.resolved_lite_model == "gemma3:4b"

    def test_falls_back_to_primary_model_when_assistant_unset(self):
        llm = LLMSettings(primary_model="gemma3:27b")
        assert llm.resolved_lite_model == "gemma3:27b"

    def test_falls_back_to_primary_model_when_assistant_empty(self):
        llm = LLMSettings(primary_model="gemma3:27b", assistant_model="")
        assert llm.resolved_lite_model == "gemma3:27b"

    def test_lite_model_takes_precedence_over_assistant_and_primary(self):
        llm = LLMSettings(
            lite_model="gemma3:1b", assistant_model="gemma3:4b", primary_model="gemma3:27b"
        )
        assert llm.resolved_lite_model == "gemma3:1b"

    def test_assistant_model_takes_precedence_over_primary(self):
        llm = LLMSettings(assistant_model="gemma3:4b", primary_model="gemma3:27b")
        assert llm.resolved_lite_model == "gemma3:4b"

    def test_falls_back_to_lite_provider_default(self):
        llm = LLMSettings(lite_provider=LLMProvider.OLLAMA, ollama_model="llama3:8b")
        assert llm.resolved_lite_model == "llama3:8b"

    def test_lite_model_takes_precedence_over_provider_default(self):
        llm = LLMSettings(lite_provider=LLMProvider.OLLAMA, lite_model="custom:1b", ollama_model="llama3:8b")
        assert llm.resolved_lite_model == "custom:1b"

    def test_primary_provider_default_used_when_only_primary_configured(self):
        llm = LLMSettings(primary_provider=LLMProvider.OLLAMA, ollama_model="llama3:8b")
        assert llm.resolved_lite_model == "llama3:8b"


class TestLLMSettingsResolveLiteFallback:
    def test_resolve_lite_falls_back_to_primary_provider(self):
        llm = LLMSettings(primary_provider=LLMProvider.FAKE, primary_model="fake")
        provider, api_key, endpoint, model = llm.resolve("lite")
        assert provider == LLMProvider.FAKE.value
        assert model == "fake"

    def test_resolve_lite_returns_none_when_nothing_configured(self):
        llm = LLMSettings()
        provider, api_key, endpoint, model = llm.resolve("lite")
        assert provider is None
        assert model is None

    def test_resolve_lite_uses_lite_provider_when_configured(self):
        llm = LLMSettings(
            lite_provider=LLMProvider.OLLAMA, lite_model="llama3:8b", primary_provider=LLMProvider.FAKE
        )
        provider, api_key, endpoint, model = llm.resolve("lite")
        assert provider == LLMProvider.OLLAMA.value
        assert model == "llama3:8b"

    def test_resolve_lite_uses_assistant_provider_when_configured(self):
        llm = LLMSettings(
            assistant_provider=LLMProvider.OLLAMA, assistant_model="llama3:8b",
            primary_provider=LLMProvider.FAKE, primary_model="fake",
        )
        provider, api_key, endpoint, model = llm.resolve("lite")
        assert provider == LLMProvider.OLLAMA.value
        assert model == "llama3:8b"

    def test_resolve_lite_override_takes_precedence_over_primary_fallback(self):
        llm = LLMSettings(primary_provider=LLMProvider.FAKE, primary_model="fake")
        provider, api_key, endpoint, model = llm.resolve(
            "lite", provider_override=LLMProvider.OLLAMA.value, model_override="llama3:8b"
        )
        assert provider == LLMProvider.OLLAMA.value
        assert model == "llama3:8b"

    def test_resolve_lite_model_override_honored_with_primary_provider_fallback(self):
        llm = LLMSettings(primary_provider=LLMProvider.FAKE, primary_model="fake")
        provider, api_key, endpoint, model = llm.resolve(
            "lite", model_override="custom-lite:1b"
        )
        assert provider == LLMProvider.FAKE.value
        assert model == "custom-lite:1b"

    def test_resolve_lite_uses_fallback_model_when_no_model_override(self):
        llm = LLMSettings(primary_provider=LLMProvider.FAKE, primary_model="fake")
        provider, api_key, endpoint, model = llm.resolve("lite")
        assert provider == LLMProvider.FAKE.value
        assert model == "fake"

    def test_resolve_lite_falls_back_to_primary_with_api_key_and_endpoint(self):
        llm = LLMSettings(
            primary_provider=LLMProvider.OLLAMA,
            primary_model="llama3:8b",
            primary_api_key="secret-key",
            primary_endpoint="http://ollama:11434",
        )
        provider, api_key, endpoint, model = llm.resolve("lite")
        assert provider == LLMProvider.OLLAMA.value
        assert model == "llama3:8b"
        assert api_key == "secret-key"
        assert endpoint == "http://ollama:11434"


class TestG8eeAppSettingsMTLSPaths:
    def test_default_paths_are_none_or_string(self):
        settings = G8eeAppSettings()
        # Should not raise AttributeError
        assert settings.client_cert_path is None or isinstance(settings.client_cert_path, str)
        assert settings.client_key_path is None or isinstance(settings.client_key_path, str)

    def test_private_field_overrides(self):
        settings = G8eeAppSettings()
        settings._client_cert_path = "/tmp/cert.pem"
        settings._client_key_path = "/tmp/key.pem"
        assert settings.client_cert_path == "/tmp/cert.pem"
        assert settings.client_key_path == "/tmp/key.pem"
