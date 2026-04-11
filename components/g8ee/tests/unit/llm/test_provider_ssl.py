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
SSL verification strategy tests for LLM providers.

The contract is simple — httpx natively accepts:
  True   -> use the certifi CA bundle (default for external cloud APIs)
  False  -> disable verification (plain HTTP)
  str    -> path to a CA cert file (internal HTTPS endpoints)

Providers MUST NOT construct ssl.SSLContext objects. They pass one of the
three values above to httpx.AsyncClient(verify=...) and let httpx handle
the rest.
"""

from unittest.mock import patch, MagicMock

import pytest


INTERNAL_CA = "/vsodb/ca.crt"

pytestmark = [pytest.mark.unit]


class TestOpenAICompatibleProviderSSL:
    """SSL verification strategy for OpenAI-compatible provider."""

    def _make_provider(self, endpoint, api_key="test-key", ca_cert_path=None):
        with patch("app.llm.providers.openai_compatible.AsyncOpenAI"):
            from app.llm.providers.openai_compatible import OpenAICompatibleProvider
            return OpenAICompatibleProvider(
                endpoint=endpoint,
                api_key=api_key,
                ca_cert_path=ca_cert_path,
            )

    def test_external_endpoint_uses_default_verification(self):
        with patch("app.llm.providers.openai_compatible.httpx.AsyncClient") as mock_client:
            self._make_provider("https://api.openai.com/v1", ca_cert_path=INTERNAL_CA)
            assert mock_client.call_count == 2
            for call in mock_client.call_args_list:
                assert call.kwargs["verify"] is True

    def test_internal_localhost_uses_platform_ca(self):
        with patch("app.llm.providers.openai_compatible.httpx.AsyncClient") as mock_client:
            self._make_provider("https://localhost:11434/v1", ca_cert_path=INTERNAL_CA)
            assert mock_client.call_count == 2
            for call in mock_client.call_args_list:
                assert call.kwargs["verify"] == INTERNAL_CA

    def test_internal_ip_uses_platform_ca(self):
        with patch("app.llm.providers.openai_compatible.httpx.AsyncClient") as mock_client:
            self._make_provider("https://192.168.1.50:11434/v1", ca_cert_path=INTERNAL_CA)
            assert mock_client.call_count == 2
            for call in mock_client.call_args_list:
                assert call.kwargs["verify"] == INTERNAL_CA

    def test_internal_http_disables_ssl(self):
        with patch("app.llm.providers.openai_compatible.httpx.AsyncClient") as mock_client:
            self._make_provider("http://10.0.0.1:11434/v1", ca_cert_path=INTERNAL_CA)
            assert mock_client.call_count == 2
            for call in mock_client.call_args_list:
                assert call.kwargs["verify"] is False

    def test_internal_without_ca_falls_back_to_true(self):
        with patch("app.llm.providers.openai_compatible.httpx.AsyncClient") as mock_client:
            self._make_provider("https://localhost:11434/v1", ca_cert_path=None)
            assert mock_client.call_count == 2
            for call in mock_client.call_args_list:
                assert call.kwargs["verify"] is True

    def test_is_internal_endpoint_patterns(self):
        from app.llm.utils import is_internal_endpoint
        assert is_internal_endpoint("https://localhost:11434/v1") is True
        assert is_internal_endpoint("https://127.0.0.1:11434/v1") is True
        assert is_internal_endpoint("https://vsod:3000/api") is True
        assert is_internal_endpoint("https://g8eo:9000/api") is True
        assert is_internal_endpoint("https://my-server.local:8080") is True
        assert is_internal_endpoint("https://api.openai.com/v1") is False
        assert is_internal_endpoint("https://generativelanguage.googleapis.com") is False
        assert is_internal_endpoint("") is False


class TestAnthropicProviderSSL:
    """SSL verification strategy for Anthropic provider."""

    def _make_provider(self, endpoint=None, api_key="test-key", ca_cert_path=None):
        with patch("app.llm.providers.anthropic.anthropic.AsyncAnthropic"):
            from app.llm.providers.anthropic import AnthropicProvider
            return AnthropicProvider(
                endpoint=endpoint,
                api_key=api_key,
                ca_cert_path=ca_cert_path,
            )

    def test_cloud_endpoint_uses_default_verification(self):
        with patch("app.llm.providers.anthropic.httpx.AsyncClient") as mock_client:
            self._make_provider(endpoint=None, ca_cert_path=INTERNAL_CA)
            assert mock_client.call_args.kwargs["verify"] is True

    def test_explicit_cloud_endpoint_uses_default_verification(self):
        with patch("app.llm.providers.anthropic.httpx.AsyncClient") as mock_client:
            self._make_provider(endpoint="https://api.anthropic.com", ca_cert_path=INTERNAL_CA)
            assert mock_client.call_args.kwargs["verify"] is True

    def test_internal_endpoint_uses_platform_ca(self):
        with patch("app.llm.providers.anthropic.httpx.AsyncClient") as mock_client:
            self._make_provider(endpoint="https://10.0.0.5:8080/v1", ca_cert_path=INTERNAL_CA)
            assert mock_client.call_args.kwargs["verify"] == INTERNAL_CA

    def test_internal_http_disables_ssl(self):
        with patch("app.llm.providers.anthropic.httpx.AsyncClient") as mock_client:
            self._make_provider(endpoint="http://10.0.0.5:8080/v1", ca_cert_path=INTERNAL_CA)
            assert mock_client.call_args.kwargs["verify"] is False

    def test_internal_without_ca_falls_back_to_true(self):
        with patch("app.llm.providers.anthropic.httpx.AsyncClient") as mock_client:
            self._make_provider(endpoint="https://localhost:8080/v1", ca_cert_path=None)
            assert mock_client.call_args.kwargs["verify"] is True

    def test_is_internal_endpoint_patterns(self):
        from app.llm.providers.anthropic import AnthropicProvider
        assert AnthropicProvider._is_internal_endpoint(None) is False
        assert AnthropicProvider._is_internal_endpoint("") is False
        assert AnthropicProvider._is_internal_endpoint("https://localhost:8080") is True
        assert AnthropicProvider._is_internal_endpoint("https://172.16.0.1:8080") is True
        assert AnthropicProvider._is_internal_endpoint("https://192.168.1.1:8080") is True
        assert AnthropicProvider._is_internal_endpoint("https://api.anthropic.com") is False


class TestGeminiProviderSSL:
    """Gemini is SaaS-only — no ca_cert_path, no env-var juggling."""

    def test_constructor_does_not_accept_ca_cert_path(self):
        from app.llm.providers.gemini import GeminiProvider
        import inspect
        sig = inspect.signature(GeminiProvider.__init__)
        params = list(sig.parameters.keys())
        assert "ca_cert_path" not in params

    def test_get_client_creates_genai_client(self):
        import asyncio
        from app.llm.providers.gemini import GeminiProvider

        provider = GeminiProvider(api_key="test-key")
        mock_client = MagicMock()

        with patch("google.genai.Client", return_value=mock_client) as mock_ctor:
            loop = asyncio.new_event_loop()
            try:
                provider._loop = None
                provider._client = None

                async def run():
                    return provider._get_client()
                result = loop.run_until_complete(run())
            finally:
                loop.close()

            mock_ctor.assert_called_once_with(api_key="test-key")
            assert result is mock_client

    def test_get_client_is_lazy_singleton(self):
        import asyncio
        from app.llm.providers.gemini import GeminiProvider

        provider = GeminiProvider(api_key="test-key")
        mock_client = MagicMock()

        with patch("google.genai.Client", return_value=mock_client) as mock_ctor:
            loop = asyncio.new_event_loop()
            try:
                provider._loop = None
                provider._client = None

                async def run():
                    first = provider._get_client()
                    second = provider._get_client()
                    return first, second
                first, second = loop.run_until_complete(run())
            finally:
                loop.close()

            mock_ctor.assert_called_once()
            assert first is second


class TestFactorySSL:
    """Factory passes ca_cert_path only to providers that may use internal endpoints."""

    def test_gemini_gets_no_ca_cert_path(self):
        from app.llm.factory import get_llm_provider, set_settings, reset_settings
        from app.models.settings import LLMSettings, G8eePlatformSettings
        from app.constants import LLMProvider

        llm_settings = LLMSettings(provider=LLMProvider.GEMINI, gemini_api_key="test")
        mock_settings = MagicMock(spec=G8eePlatformSettings)
        mock_settings.ca_cert_path = INTERNAL_CA
        set_settings(mock_settings)

        try:
            with patch("app.llm.factory.GeminiProvider") as mock_gemini:
                get_llm_provider(llm_settings)
                mock_gemini.assert_called_once_with(api_key="test")
        finally:
            reset_settings()

    def test_ollama_gets_ca_cert_path(self):
        from app.llm.factory import get_llm_provider, set_settings, reset_settings
        from app.models.settings import LLMSettings, G8eePlatformSettings
        from app.constants import LLMProvider

        llm_settings = LLMSettings(
            provider=LLMProvider.OLLAMA,
            ollama_endpoint="https://localhost:11434/v1",
            ollama_api_key="test"
        )
        mock_settings = MagicMock(spec=G8eePlatformSettings)
        mock_settings.ca_cert_path = INTERNAL_CA
        set_settings(mock_settings)

        try:
            with patch("app.llm.factory.OpenAICompatibleProvider") as mock_oai:
                get_llm_provider(llm_settings)
                assert mock_oai.call_args.kwargs["ca_cert_path"] == INTERNAL_CA
        finally:
            reset_settings()

    def test_anthropic_gets_ca_cert_path(self):
        from app.llm.factory import get_llm_provider, set_settings, reset_settings
        from app.models.settings import LLMSettings, G8eePlatformSettings
        from app.constants import LLMProvider

        llm_settings = LLMSettings(provider=LLMProvider.ANTHROPIC, anthropic_api_key="test")
        mock_settings = MagicMock(spec=G8eePlatformSettings)
        mock_settings.ca_cert_path = INTERNAL_CA
        set_settings(mock_settings)

        try:
            with patch("app.llm.factory.AnthropicProvider") as mock_anth:
                get_llm_provider(llm_settings)
                assert mock_anth.call_args.kwargs["ca_cert_path"] == INTERNAL_CA
        finally:
            reset_settings()

    def test_openai_gets_ca_cert_path(self):
        from app.llm.factory import get_llm_provider, set_settings, reset_settings
        from app.models.settings import LLMSettings, G8eePlatformSettings
        from app.constants import LLMProvider

        llm_settings = LLMSettings(
            provider=LLMProvider.OPENAI,
            openai_api_key="test",
            openai_endpoint="https://api.openai.com/v1",
        )
        mock_settings = MagicMock(spec=G8eePlatformSettings)
        mock_settings.ca_cert_path = INTERNAL_CA
        set_settings(mock_settings)

        try:
            with patch("app.llm.factory.OpenAICompatibleProvider") as mock_oai:
                get_llm_provider(llm_settings)
                assert mock_oai.call_args.kwargs["ca_cert_path"] == INTERNAL_CA
        finally:
            reset_settings()
