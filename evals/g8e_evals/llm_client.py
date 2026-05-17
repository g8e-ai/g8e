"""Reuse the g8ee LLM provider stack (`app.llm.*`) for evaluation calls.

The bench is launched against the g8ee virtualenv with PYTHONPATH set to
``services/g8ee:protocol`` (see ``scripts/cmd/evals.sh``), so the
``app.*`` namespace is importable here.

The ``provider:model`` identifier passed to the bench is parsed into an
``LLMSettings`` instance; credentials and endpoints are pulled from
environment variables — no platform settings store is required for a
standalone bench.
"""

from __future__ import annotations

import os
from typing import Optional, TYPE_CHECKING
if TYPE_CHECKING:
    from g8e_evals.harness import SUTConfig

_PROVIDER_ENV: dict[str, dict[str, str]] = {
    "openai":    {"api_key": "OPENAI_API_KEY",    "endpoint": "OPENAI_ENDPOINT"},
    "anthropic": {"api_key": "ANTHROPIC_API_KEY", "endpoint": "ANTHROPIC_ENDPOINT"},
    "gemini":    {"api_key": "GEMINI_API_KEY"},
    "ollama":    {"api_key": "OLLAMA_API_KEY",    "endpoint": "OLLAMA_HOST"},
    "llamacpp":  {"api_key": "LLAMACPP_API_KEY",  "endpoint": "LLAMACPP_ENDPOINT"},
}


def _parse_model(model_provider: str) -> tuple[str, str]:
    if ":" not in model_provider:
        raise ValueError(
            f"--model must be in 'provider:name' form, got {model_provider!r}. "
            f"Examples: openai:gpt-4o, anthropic:claude-3-5-sonnet-latest, gemini:gemini-2.0-flash"
        )
    provider, name = model_provider.split(":", 1)
    provider = provider.strip().lower()
    name = name.strip()
    if not provider or not name:
        raise ValueError(f"--model {model_provider!r} is missing provider or name")
    if provider not in _PROVIDER_ENV:
        raise ValueError(
            f"Unsupported provider {provider!r}. Supported: {sorted(_PROVIDER_ENV)}"
        )
    return provider, name


def _build_settings(config: SUTConfig):
    from app.constants import LLMProvider
    from app.models.settings import LLMSettings

    settings = LLMSettings()
    
    # Credentials mapping helper
    def _apply_creds(p: str, key: Optional[str], url: Optional[str]):
        env = _PROVIDER_ENV[p]
        if key is None:
            api_key_env = env.get("api_key")
            key = os.environ.get(api_key_env) if api_key_env else None
            if p == "gemini" and not key:
                key = os.environ.get("GOOGLE_API_KEY")

        if url is None:
            endpoint_env = env.get("endpoint")
            url = os.environ.get(endpoint_env) if endpoint_env else None

        if p == "openai":
            settings.openai_api_key = key or settings.openai_api_key or ""
            if url: settings.openai_endpoint = url
        elif p == "anthropic":
            settings.anthropic_api_key = key or settings.anthropic_api_key or ""
            if url: settings.anthropic_endpoint = url
        elif p == "gemini":
            settings.gemini_api_key = key or settings.gemini_api_key or ""
        elif p == "ollama":
            settings.ollama_api_key = key or settings.ollama_api_key or ""
            if url: settings.ollama_endpoint = url
        elif p == "llamacpp":
            settings.llamacpp_api_key = key or settings.llamacpp_api_key or ""
            if url: settings.llamacpp_endpoint = url
        return key, url

    # 1. Primary
    provider_str = config.primary.provider
    model_name = config.primary.model
    
    if not provider_str and model_name and ":" in model_name:
        provider_str, model_name = _parse_model(model_name)
    
    provider_str = provider_str or "openai"
    model_name = model_name or "gpt-4"
    
    enum_provider = LLMProvider(provider_str)
    settings.primary_provider = enum_provider
    settings.primary_model = model_name
    
    # Resolve and set primary credentials
    pk, pu = _apply_creds(provider_str, config.primary.api_key, config.primary.endpoint)
    settings.primary_api_key = pk
    settings.primary_endpoint = pu

    # 2. Assistant
    a_provider_str = config.assistant.provider
    a_model_name = config.assistant.model
    
    if not a_provider_str and a_model_name and ":" in a_model_name:
        a_provider_str, a_model_name = _parse_model(a_model_name)
    
    a_provider_str = a_provider_str or provider_str
    a_model_name = a_model_name or model_name
    
    settings.assistant_provider = LLMProvider(a_provider_str)
    settings.assistant_model = a_model_name
    
    # Resolve and set assistant credentials
    ak, au = _apply_creds(a_provider_str, config.assistant.api_key, config.assistant.endpoint)
    settings.assistant_api_key = ak
    settings.assistant_endpoint = au

    # 3. Lite
    l_provider_str = config.lite.provider
    l_model_name = config.lite.model
    
    if not l_provider_str and l_model_name and ":" in l_model_name:
        l_provider_str, l_model_name = _parse_model(l_model_name)
        
    l_provider_str = l_provider_str or a_provider_str
    l_model_name = l_model_name or a_model_name
    
    settings.lite_provider = LLMProvider(l_provider_str)
    settings.lite_model = l_model_name
    
    # Resolve and set lite credentials
    lk, lu = _apply_creds(l_provider_str, config.lite.api_key, config.lite.endpoint)
    settings.lite_api_key = lk
    settings.lite_endpoint = lu

    # Validation
    if settings.primary_provider in ("openai", "anthropic", "gemini") and not settings.get_primary_api_key():
        api_key_env = _PROVIDER_ENV[settings.primary_provider.value].get("api_key", "API_KEY")
        raise RuntimeError(
            f"{settings.primary_provider.value} requires {api_key_env}"
            + (" or GOOGLE_API_KEY" if settings.primary_provider == LLMProvider.GEMINI else "")
            + " to be set in the environment or passed via --llm-api-key"
        )

    return settings


async def call_llm(
    model_provider: str, 
    prompt: str, 
    config: "SUTConfig"
) -> str:
    """Run a single non-streaming primary completion and return text."""
    settings = _build_settings(config)

    from app.llm.factory import get_llm_provider
    from app.llm.llm_dataclasses import Content, Part
    from app.llm.llm_types import PrimaryLLMSettings

    llm = get_llm_provider(settings)
    contents = [Content(role="user", parts=[Part.from_text(prompt)])]
    primary_settings = PrimaryLLMSettings(system_instructions="")
    response = await llm.generate_content_primary(
        model=settings.primary_model,
        contents=contents,
        primary_llm_settings=primary_settings,
    )
    text = response.text or ""
    return text
