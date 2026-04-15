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

  get_llm_provider(settings, is_assistant=False) — returns a cached LLMProvider instance
      based on the given LLMSettings. The provider type is settings.primary_provider by
      default, or settings.assistant_provider when is_assistant=True. The model is passed
      per-call to generate_content_stream_primary / generate_content_assistant.

  Provider instances are cached and reused across calls to avoid repeated initialization.
  The ``async with`` pattern is still supported for compatibility, but is optional for
  cached providers (close() is a no-op for singletons). Use clear_provider_cache() on
  shutdown to clean up resources::

      async with get_llm_provider(settings.llm) as provider:
          stream = provider.generate_content_stream_primary(model=..., ...)

  Or simply (cached provider, no cleanup needed)::

      provider = get_llm_provider(settings.llm)
      stream = await provider.generate_content_stream_primary(model=..., ...)

All Gemini-specific logic lives in app.llm.providers.gemini.
"""

import logging

from app.models.settings import LLMSettings, G8eePlatformSettings, SearchSettings
from app.constants import LLMProvider

from .provider import LLMProvider as LLMProviderBase
from .providers.open_ai import OpenAIProvider
from .providers.gemini import GeminiProvider
from .providers.anthropic import AnthropicProvider

logger = logging.getLogger(__name__)

_settings: G8eePlatformSettings | None = None
_llm_settings: LLMSettings | None = None
_search_settings: SearchSettings | None = None
_provider_cache: dict[str, LLMProviderBase] = {}


def set_settings(settings: G8eePlatformSettings) -> None:
    """Inject the platform G8eePlatformSettings at startup."""
    global _settings
    _settings = settings


def get_settings() -> G8eePlatformSettings | None:
    """Return the platform settings singleton."""
    return _settings


def set_llm_settings(settings: LLMSettings) -> None:
    """Inject LLM settings for testing. Production code uses G8eeUserSettings.llm."""
    global _llm_settings
    _llm_settings = settings


def get_llm_settings() -> LLMSettings | None:
    """Return the LLM settings singleton (used in tests)."""
    return _llm_settings


def set_search_settings(settings: SearchSettings) -> None:
    """Inject search settings for testing. Production code uses G8eeUserSettings.search."""
    global _search_settings
    _search_settings = settings


def get_search_settings() -> SearchSettings | None:
    """Return the search settings singleton (used in tests)."""
    return _search_settings


def _get_provider_cache_key(settings: LLMSettings, is_assistant: bool) -> str:
    """Generate a cache key for provider instances based on configuration."""
    provider_type = settings.assistant_provider if is_assistant else settings.primary_provider
    # Handle both enum and string values (LLMSettings stores as string)
    provider_value = provider_type.value if hasattr(provider_type, 'value') else str(provider_type)
    key_parts = [provider_value]

    if provider_value == LLMProvider.GEMINI.value:
        key_parts.append(settings.gemini_api_key or "")
    elif provider_value == LLMProvider.OPENAI.value:
        key_parts.append(settings.openai_endpoint or "")
        key_parts.append(settings.openai_api_key or "")
    elif provider_value == LLMProvider.ANTHROPIC.value:
        key_parts.append(settings.anthropic_endpoint or "")
        key_parts.append(settings.anthropic_api_key or "")
    elif provider_value == LLMProvider.OLLAMA.value:
        key_parts.append(settings.ollama_endpoint or "")
        key_parts.append(settings.ollama_api_key or "")

    platform_settings = get_settings()
    if platform_settings and platform_settings.ca_cert_path:
        key_parts.append(platform_settings.ca_cert_path)

    return "|".join(key_parts)


async def clear_provider_cache() -> None:
    """Close and clear all cached provider instances. Intended for shutdown/testing."""
    global _provider_cache
    for provider in _provider_cache.values():
        try:
            await provider.force_close()
        except Exception as exc:
            logger.debug("Error closing provider during cache clear: %s", exc)
    _provider_cache.clear()


def reset_settings() -> None:
    """Reset all settings singletons. Intended for use in tests only."""
    global _settings, _llm_settings, _search_settings
    _settings = None
    _llm_settings = None
    _search_settings = None


def get_llm_provider(settings: LLMSettings, is_assistant: bool = False) -> LLMProviderBase:
    """Return a configured LLMProvider instance based on settings.

    SSL strategy:
      - Ollama endpoints may be internal (LAN, Docker
        network) and need the platform CA cert for TLS verification.
      - Gemini is always a public Google API — never needs the platform CA.
      - Anthropic / OpenAI cloud APIs are public — the provider decides
        based on the endpoint whether to use the platform CA or the public
        CA bundle (certifi).

    Provider instances are cached and reused to avoid repeated initialization.
    """
    from app.errors import ConfigurationError

    cache_key = _get_provider_cache_key(settings, is_assistant)
    if cache_key in _provider_cache:
        return _provider_cache[cache_key]

    provider_type = settings.assistant_provider if is_assistant else settings.primary_provider

    platform_settings = get_settings()
    ca_cert_path = platform_settings.ca_cert_path if platform_settings else None

    if provider_type == LLMProvider.OLLAMA:
        from .providers.ollama import OllamaProvider
        provider = OllamaProvider(
            endpoint=settings.ollama_endpoint,
            api_key=settings.ollama_api_key,
            ca_cert_path=ca_cert_path,
        )
    elif provider_type == LLMProvider.OPENAI:
        provider = OpenAIProvider(
            endpoint=settings.openai_endpoint,
            api_key=settings.openai_api_key,
            ca_cert_path=ca_cert_path,
        )
    elif provider_type == LLMProvider.GEMINI:
        provider = GeminiProvider(api_key=settings.gemini_api_key)
    elif provider_type == LLMProvider.ANTHROPIC:
        provider = AnthropicProvider(
            endpoint=settings.anthropic_endpoint,
            api_key=settings.anthropic_api_key,
            ca_cert_path=ca_cert_path,
        )
    else:
        raise ConfigurationError(f"Unsupported LLM provider: {provider_type}")

    provider._is_cached_singleton = True
    _provider_cache[cache_key] = provider
    return provider
