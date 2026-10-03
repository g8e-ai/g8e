# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

"""List the models a provider endpoint serves, for the console's model picker.

Each provider's own listing API is called with the endpoint and key the role
would use. Failures surface as ExternalServiceError carrying only the HTTP
status or transport error class, never the upstream body.
"""

from __future__ import annotations

from typing import Any

import httpx

from app.constants import LLMProvider
from app.errors import ExternalServiceError, ValidationError

_TIMEOUT = httpx.Timeout(10.0)
_GEMINI_MODELS_URL = "https://generativelanguage.googleapis.com/v1beta/models"
_ANTHROPIC_VERSION = "2023-06-01"


def _openai_base(endpoint: str) -> str:
    base = endpoint.rstrip("/")
    return base if base.endswith("/v1") else f"{base}/v1"


def _request(
    provider: LLMProvider, endpoint: str | None, api_key: str | None
) -> tuple[str, dict[str, str], dict[str, str]]:
    """Return (url, headers, params) for provider's model-listing call."""
    bearer = {"Authorization": f"Bearer {api_key}"} if api_key else {}
    if provider is LLMProvider.GEMINI:
        if not api_key:
            raise ValidationError("Gemini needs an API key to list models", field="api_key")
        return _GEMINI_MODELS_URL, {"x-goog-api-key": api_key}, {"pageSize": "1000"}
    if not endpoint:
        raise ValidationError("An endpoint is required to list models", field="endpoint")
    if provider is LLMProvider.OLLAMA:
        return f"{endpoint.rstrip('/')}/api/tags", bearer, {}
    if provider in (LLMProvider.OPENAI, LLMProvider.LLAMACPP):
        return f"{_openai_base(endpoint)}/models", bearer, {}
    if provider is LLMProvider.ANTHROPIC:
        if not api_key:
            raise ValidationError("Anthropic needs an API key to list models", field="api_key")
        headers = {"x-api-key": api_key, "anthropic-version": _ANTHROPIC_VERSION}
        return f"{endpoint.rstrip('/')}/v1/models", headers, {"limit": "1000"}
    raise ValidationError(
        f"Provider '{provider.value}' does not support model listing",
        field="provider",
        constraint="lists_models",
    )


def parse_models(provider: LLMProvider, body: Any) -> list[str]:
    """Extract sorted, de-duplicated model names from a listing response body."""
    names: list[str] = []
    if isinstance(body, dict):
        if provider is LLMProvider.OLLAMA:
            names = [m.get("name") for m in body.get("models") or [] if isinstance(m, dict)]
        elif provider is LLMProvider.GEMINI:
            names = [
                str(m.get("name", "")).removeprefix("models/")
                for m in body.get("models") or []
                if isinstance(m, dict)
                and "generateContent" in (m.get("supportedGenerationMethods") or [])
            ]
        else:
            names = [m.get("id") for m in body.get("data") or [] if isinstance(m, dict)]
    return sorted({n for n in names if isinstance(n, str) and n})


async def list_models(
    provider: LLMProvider,
    endpoint: str | None,
    api_key: str | None,
    client: httpx.AsyncClient | None = None,
) -> list[str]:
    url, headers, params = _request(provider, endpoint, api_key)
    owned = client is None
    http = client or httpx.AsyncClient(timeout=_TIMEOUT, follow_redirects=False)
    try:
        response = await http.get(url, headers=headers, params=params)
    except httpx.HTTPError as exc:
        raise ExternalServiceError(
            f"Could not reach the {provider.value} endpoint ({type(exc).__name__})",
            service_name=provider.value,
        ) from exc
    finally:
        if owned:
            await http.aclose()
    if response.status_code >= 400:
        raise ExternalServiceError(
            f"The {provider.value} endpoint refused the model listing (HTTP {response.status_code})",
            service_name=provider.value,
        )
    try:
        body = response.json()
    except ValueError as exc:
        raise ExternalServiceError(
            f"The {provider.value} endpoint returned a non-JSON model listing",
            service_name=provider.value,
        ) from exc
    return parse_models(provider, body)
