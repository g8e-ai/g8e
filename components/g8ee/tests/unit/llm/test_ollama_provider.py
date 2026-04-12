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
Unit tests for OllamaProvider.

Tests SSL verification strategy, close behavior, and construction.
"""

from unittest.mock import patch, MagicMock, AsyncMock

import pytest

from app.llm.providers.ollama import OllamaProvider


PATCH_TARGET = "app.llm.providers.ollama.AsyncClient"
INTERNAL_CA = "/g8es/ca.crt"

pytestmark = [pytest.mark.unit]


class TestOllamaProviderSSL:
    """SSL verification strategy for Ollama provider."""

    def test_external_endpoint_uses_default_verification(self):
        with patch(PATCH_TARGET) as mock_client:
            OllamaProvider(
                endpoint="https://api.ollama.ai",
                api_key="test-key",
                ca_cert_path=INTERNAL_CA,
            )
            mock_client.assert_called_once()
            assert mock_client.call_args.kwargs["verify"] is True

    def test_internal_localhost_uses_platform_ca(self):
        with patch(PATCH_TARGET) as mock_client:
            OllamaProvider(
                endpoint="https://localhost:11434",
                api_key="test-key",
                ca_cert_path=INTERNAL_CA,
            )
            mock_client.assert_called_once()
            assert mock_client.call_args.kwargs["verify"] == INTERNAL_CA

    def test_internal_ip_uses_platform_ca(self):
        with patch(PATCH_TARGET) as mock_client:
            OllamaProvider(
                endpoint="https://192.168.1.50:11434",
                api_key="test-key",
                ca_cert_path=INTERNAL_CA,
            )
            mock_client.assert_called_once()
            assert mock_client.call_args.kwargs["verify"] == INTERNAL_CA

    def test_internal_http_disables_ssl(self):
        with patch(PATCH_TARGET) as mock_client:
            OllamaProvider(
                endpoint="http://10.0.0.1:11434",
                api_key="test-key",
                ca_cert_path=INTERNAL_CA,
            )
            mock_client.assert_called_once()
            assert mock_client.call_args.kwargs["verify"] is False

    def test_internal_without_ca_falls_back_to_true(self):
        with patch(PATCH_TARGET) as mock_client:
            OllamaProvider(
                endpoint="https://localhost:11434",
                api_key="test-key",
                ca_cert_path=None,
            )
            mock_client.assert_called_once()
            assert mock_client.call_args.kwargs["verify"] is True

    def test_endpoint_with_v1_suffix_strips_v1(self):
        with patch(PATCH_TARGET) as mock_client:
            OllamaProvider(
                endpoint="https://localhost:11434/v1",
                api_key="test-key",
                ca_cert_path=INTERNAL_CA,
            )
            mock_client.assert_called_once()
            # /v1 should be stripped from the host passed to AsyncClient
            assert mock_client.call_args.kwargs["host"] == "https://localhost:11434"


class TestOllamaProviderClose:
    """Test that OllamaProvider properly closes its httpx client."""

    @pytest.mark.asyncio
    async def test_close_calls_aclose_on_client(self):
        mock_inner_client = MagicMock()
        mock_inner_client.aclose = AsyncMock()
        mock_client = MagicMock()
        mock_client._client = mock_inner_client

        with patch(PATCH_TARGET, return_value=mock_client):
            provider = OllamaProvider(
                endpoint="https://localhost:11434",
                api_key="test-key",
                ca_cert_path=None,
            )
            await provider.close()
            mock_inner_client.aclose.assert_called_once()


class TestOllamaProviderConstruction:
    """Test OllamaProvider construction and initialization."""

    def test_constructor_creates_async_client(self):
        mock_client = MagicMock()

        with patch(PATCH_TARGET, return_value=mock_client) as mock_ctor:
            provider = OllamaProvider(
                endpoint="https://localhost:11434",
                api_key="test-key",
                ca_cert_path=INTERNAL_CA,
            )

            mock_ctor.assert_called_once()
            assert provider._client is mock_client

    def test_constructor_strips_trailing_slash(self):
        with patch(PATCH_TARGET):
            provider = OllamaProvider(
                endpoint="https://localhost:11434/",
                api_key="test-key",
                ca_cert_path=None,
            )
            assert provider._original_endpoint == "https://localhost:11434"

    def test_constructor_strips_v1_suffix(self):
        with patch(PATCH_TARGET) as mock_client:
            OllamaProvider(
                endpoint="https://localhost:11434/v1",
                api_key="test-key",
                ca_cert_path=None,
            )
            # /v1 should be stripped when creating the AsyncClient host
            assert mock_client.call_args.kwargs["host"] == "https://localhost:11434"

    @pytest.mark.asyncio
    async def test_context_manager_support(self):
        """Test that OllamaProvider supports async context manager."""
        mock_inner_client = MagicMock()
        mock_inner_client.aclose = AsyncMock()
        mock_client = MagicMock()
        mock_client._client = mock_inner_client

        with patch(PATCH_TARGET, return_value=mock_client):
            provider = OllamaProvider(
                endpoint="https://localhost:11434",
                api_key="test-key",
                ca_cert_path=None,
            )
            async with provider:
                assert provider is not None
            mock_inner_client.aclose.assert_called_once()
