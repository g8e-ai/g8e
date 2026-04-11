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
LLM Provider Factory

One entry point:

  get_llm_provider(settings) — constructs an LLMProvider from the given
      LLMSettings. The provider type is always settings.provider; the model
      is passed per-call to generate_content_stream_primary /
      generate_content_assistant. Callers MUST use ``async with`` to
      ensure the provider is closed::

          async with get_llm_provider(settings.llm) as provider:
              stream = provider.generate_content_stream_primary(model=..., ...)

All Gemini-specific logic lives in app.llm.providers.gemini.
"""

import logging

from app.models.settings import LLMSettings, G8eePlatformSettings
from app.constants import LLMProvider

from .provider import LLMProvider as LLMProviderBase
from .providers.openai_compatible import OpenAICompatibleProvider
from .providers.gemini import GeminiProvider
from .providers.anthropic import AnthropicProvider

logger = logging.getLogger(__name__)

_settings: G8eePlatformSettings | None = None


def set_settings(settings: G8eePlatformSettings) -> None:
    """Inject the platform G8eePlatformSettings at startup."""
    global _settings
    _settings = settings


def get_settings() -> G8eePlatformSettings | None:
    """Return the platform settings singleton."""
    return _settings


def reset_settings() -> None:
    """Reset the settings singleton. Intended for use in tests only."""
    global _settings
    _settings = None


def get_llm_provider(settings: LLMSettings) -> LLMProviderBase:
    """Return a configured LLMProvider instance based on settings.

    SSL strategy:
      - Ollama / OpenAI-compatible endpoints may be internal (LAN, Docker
        network) and need the platform CA cert for TLS verification.
      - Gemini is always a public Google API — never needs the platform CA.
      - Anthropic / OpenAI cloud APIs are public — the provider decides
        based on the endpoint whether to use the platform CA or the public
        CA bundle (certifi).
    """
    from app.errors import ConfigurationError

    provider_type = settings.provider

    platform_settings = get_settings()
    ca_cert_path = platform_settings.ca_cert_path if platform_settings else None

    if provider_type == LLMProvider.OLLAMA:
        return OpenAICompatibleProvider(
            endpoint=settings.ollama_endpoint,
            api_key=settings.ollama_api_key,
            ca_cert_path=ca_cert_path,
        )
    elif provider_type == LLMProvider.OPENAI:
        return OpenAICompatibleProvider(
            endpoint=settings.openai_endpoint,
            api_key=settings.openai_api_key,
            ca_cert_path=ca_cert_path,
        )
    elif provider_type == LLMProvider.GEMINI:
        return GeminiProvider(api_key=settings.gemini_api_key)
    elif provider_type == LLMProvider.ANTHROPIC:
        return AnthropicProvider(
            endpoint=settings.anthropic_endpoint,
            api_key=settings.anthropic_api_key,
            ca_cert_path=ca_cert_path,
        )

    raise ConfigurationError(f"Unsupported LLM provider: {provider_type}")
