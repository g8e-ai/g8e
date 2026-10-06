# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

"""
LLM Provider Factory

One entry point:

  get_llm_provider(settings, is_assistant=False) - returns a cached LLMProvider instance
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

from __future__ import annotations

import importlib
import logging
from dataclasses import dataclass
from typing import Protocol, cast

from app.constants import LLMProvider
from app.models.settings import G8eeAppSettings, LLMSettings, SearchSettings

from .endpoints import normalize_ollama_host
from .provider import LLMProvider as LLMProviderBase

logger = logging.getLogger(__name__)


@dataclass
class _FactoryState:
    """Process-wide settings and clients injected at startup (or by tests)."""

    settings: G8eeAppSettings | None = None
    llm_settings: LLMSettings | None = None
    search_settings: SearchSettings | None = None
    internal_http_client: object | None = None


_state = _FactoryState()
_provider_cache: dict[str, LLMProviderBase] = {}

# Provider modules are resolved by name only when selected, so an unused
# provider SDK is never imported (and a broken one cannot break startup).
_PROVIDER_MODULES: dict[LLMProvider, tuple[str, str]] = {
    LLMProvider.OLLAMA: ("app.llm.providers.ollama", "OllamaProvider"),
    LLMProvider.OPENAI: ("app.llm.providers.open_ai", "OpenAIProvider"),
    LLMProvider.GEMINI: ("app.llm.providers.gemini", "GeminiProvider"),
    LLMProvider.ANTHROPIC: ("app.llm.providers.anthropic", "AnthropicProvider"),
    LLMProvider.LLAMACPP: ("app.llm.providers.llama_cpp", "LlamaCppProvider"),
    LLMProvider.FAKE: ("app.llm.providers.fake", "FakeProvider"),
    LLMProvider.G8E: ("app.llm.providers.g8e", "G8EProvider"),
}

# app.errors and app.decision.factory sit on the app.errors -> app.models ->
# app.llm import cycle, so they are resolved at call time rather than module load.
_ERRORS_MODULE = "app.errors"
_DECISION_FACTORY_MODULE = "app.decision.factory"


class _EndpointProviderClass(Protocol):
    def __call__(self, *, endpoint: str | None, api_key: str | None) -> LLMProviderBase: ...


class _ApiKeyProviderClass(Protocol):
    def __call__(self, *, api_key: str | None) -> LLMProviderBase: ...


class _GovernedProviderClass(Protocol):
    def __call__(self, *, internal_http_client: object) -> LLMProviderBase: ...


def _configuration_error(message: str) -> Exception:
    error_class = importlib.import_module(_ERRORS_MODULE).ConfigurationError
    return error_class(message)


def set_settings(settings: G8eeAppSettings) -> None:
    """Inject the platform G8eeAppSettings at startup."""
    _state.settings = settings


def get_settings() -> G8eeAppSettings | None:
    """Return the platform settings singleton."""
    return _state.settings


def set_llm_settings(settings: LLMSettings) -> None:
    """Inject LLM settings for testing. Production code uses G8eeUserSettings.llm."""
    _state.llm_settings = settings


def get_llm_settings() -> LLMSettings | None:
    """Return the LLM settings singleton (used in tests)."""
    return _state.llm_settings


def set_search_settings(settings: SearchSettings) -> None:
    """Inject search settings for testing. Production code uses G8eeUserSettings.search."""
    _state.search_settings = settings


def get_search_settings() -> SearchSettings | None:
    """Return the search settings singleton (used in tests)."""
    return _state.search_settings


def set_internal_http_client(client: object) -> None:
    """Inject the platform InternalHttpClient at startup.

    The G8E governed-dispatch provider uses this client to call the
    gateway's /api/v1/inference/dispatch endpoint over mTLS. The client
    is a singleton shared with the rest of the application; the factory
    does not own its lifecycle.
    """
    _state.internal_http_client = client


def get_internal_http_client() -> object | None:
    """Return the internal HTTP client singleton (used by the G8E provider)."""
    return _state.internal_http_client


def _get_provider_cache_key(
    settings: LLMSettings, is_assistant: bool = False, is_lite: bool = False
) -> str:
    """Generate a cache key for provider instances based on configuration."""
    role = "lite" if is_lite else "assistant" if is_assistant else "primary"
    provider, api_key, endpoint, _ = settings.resolve(role)

    provider_value = provider or "none"
    key_parts = [provider_value]

    if provider_value == LLMProvider.GEMINI.value:
        key_parts.append(api_key or "")
    elif provider_value in (LLMProvider.OPENAI.value, LLMProvider.ANTHROPIC.value):
        key_parts.append(endpoint or "")
        key_parts.append(api_key or "")
    elif provider_value in (LLMProvider.OLLAMA.value, LLMProvider.LLAMACPP.value):
        key_parts.append(normalize_ollama_host(endpoint or ""))
        key_parts.append(api_key or "")

    return "|".join(key_parts)


async def clear_provider_cache() -> None:
    """Close and clear all cached provider instances. Intended for shutdown/testing."""
    for provider in _provider_cache.values():
        try:
            await provider.force_close()
        except Exception as exc:
            logger.info("Error closing provider during cache clear: %s", exc)
    _provider_cache.clear()
    decision_factory = importlib.import_module(_DECISION_FACTORY_MODULE)
    await decision_factory.clear_decision_provider_cache()


def reset_settings() -> None:
    """Reset all settings singletons. Intended for use in tests only."""
    _state.settings = None
    _state.llm_settings = None
    _state.search_settings = None
    _state.internal_http_client = None


def get_generative_lite_provider(settings: LLMSettings) -> LLMProviderBase:
    """Return an LLM provider for generative lite workloads.

    When lite_provider is Jev, falls back to the assistant provider because Jev only
    supports structured decision evaluation. Triage and eval judge should use
    get_decision_provider() when lite_provider is jev.
    """
    if settings.lite_provider is LLMProvider.JEV:
        logger.debug("Lite provider is jev; using assistant provider for generative lite call")
        return get_llm_provider(settings, is_assistant=True)
    return get_llm_provider(settings, is_lite=True)


def get_llm_provider(
    settings: LLMSettings, is_assistant: bool = False, is_lite: bool = False
) -> LLMProviderBase:
    """Return a configured LLMProvider instance based on settings.

    SSL strategy:
      - Ollama endpoints may be internal (LAN, Docker
        network) and need the platform CA cert for TLS verification.
      - Gemini is always a public Google API - never needs the platform CA.
      - Anthropic / OpenAI cloud APIs are public - the provider decides
        based on the endpoint whether to use the platform CA or the public
        CA bundle (certifi).

    Provider instances are cached and reused to avoid repeated initialization.
    """
    cache_key = _get_provider_cache_key(settings, is_assistant, is_lite)
    if cache_key in _provider_cache:
        return _provider_cache[cache_key]

    role = "lite" if is_lite else "assistant" if is_assistant else "primary"
    provider_str, api_key, endpoint, _ = settings.resolve(role)

    if not provider_str:
        raise _configuration_error(f"No provider configured for role: {role}")

    provider = _construct_provider(LLMProvider(provider_str), api_key=api_key, endpoint=endpoint)
    provider.mark_cached_singleton()
    _provider_cache[cache_key] = provider
    return provider


def _construct_provider(
    provider_type: LLMProvider, *, api_key: str | None, endpoint: str | None
) -> LLMProviderBase:
    provider_class = get_llm_provider_class(provider_type)
    if provider_type == LLMProvider.G8E:
        if _state.internal_http_client is None:
            raise _configuration_error(
                "G8E provider requires the InternalHttpClient to be injected "
                "at startup via set_internal_http_client()"
            )
        governed_class = cast(_GovernedProviderClass, provider_class)
        return governed_class(internal_http_client=_state.internal_http_client)
    if provider_type == LLMProvider.GEMINI:
        api_key_class = cast(_ApiKeyProviderClass, provider_class)
        return api_key_class(api_key=api_key)
    endpoint_class = cast(_EndpointProviderClass, provider_class)
    return endpoint_class(endpoint=endpoint, api_key=api_key)


def get_llm_provider_class(provider_type: LLMProvider) -> type[LLMProviderBase]:
    """Load only the selected provider, including during config validation.

    Import failures propagate for the selected provider. We never silently
    switch providers or weaken validation when its SDK cannot load.
    """
    location = _PROVIDER_MODULES.get(provider_type)
    if location is not None:
        module_name, class_name = location
        provider_class = getattr(importlib.import_module(module_name), class_name)
        if not (isinstance(provider_class, type) and issubclass(provider_class, LLMProviderBase)):
            raise TypeError(f"{module_name}.{class_name} is not an LLMProvider subclass")
        return provider_class

    if provider_type == LLMProvider.JEV:
        raise _configuration_error(
            "Provider 'jev' does not support lite text generation; use jev only "
            "for triage/eval_judge or select a generative lite provider."
        )
    raise _configuration_error(f"Unsupported LLM provider: {provider_type}")
