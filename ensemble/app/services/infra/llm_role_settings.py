# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

"""Browser-facing view and update rules for LLM provider and role settings.

The console saves one endpoint and API key per provider, then assigns a
provider/model pair independently to primary, assistant, and lite roles.
API keys never leave g8ee: the view reports only whether one is stored.

Every provider, g8e included, stores the model the user chose for the role. The
Inference Operator is a worker and never decides it; the console lists the
models the Operator's Ollama provider serves from /settings/llm/models.
"""

from __future__ import annotations

from urllib.parse import urlsplit

from app.constants import (
    ANTHROPIC_DEFAULT_ENDPOINT,
    LLMProvider,
    LLAMACPP_DEFAULT_ENDPOINT,
    OLLAMA_DEFAULT_ENDPOINT,
    OPENAI_DEFAULT_ENDPOINT,
)
from app.errors import ValidationError
from app.models.internal_api import (
    FieldRequirement,
    LLMProviderOption,
    LLMProviderUpdate,
    LLMRole,
    LLMRoleSettingsResponse,
    LLMRoleSettingsUpdateRequest,
    LLMRoleView,
)
from app.models.settings import LLMSettings

# Providers a user may assign from the console, in display order:
# (label, endpoint, api_key, lists_models). Jev is a lite-only decision
# provider that requires Tribunal to be disabled, and fake is test-only; both
# stay configurable through platform settings, not here.
_PROVIDER_FIELDS: dict[LLMProvider, tuple[str, FieldRequirement, FieldRequirement, bool]] = {
    LLMProvider.OLLAMA: ("Ollama", "optional", "optional", True),
    LLMProvider.OPENAI: ("OpenAI-compatible", "optional", "required", True),
    LLMProvider.ANTHROPIC: ("Anthropic", "optional", "required", True),
    LLMProvider.GEMINI: ("Google Gemini", "none", "required", True),
    LLMProvider.LLAMACPP: ("llama.cpp", "optional", "optional", True),
    LLMProvider.G8E: ("g8e governed inference", "none", "none", True),
}


def _provider_default_endpoint(provider: LLMProvider) -> str | None:
    return {
        LLMProvider.OLLAMA: OLLAMA_DEFAULT_ENDPOINT,
        LLMProvider.OPENAI: OPENAI_DEFAULT_ENDPOINT,
        LLMProvider.ANTHROPIC: ANTHROPIC_DEFAULT_ENDPOINT,
        LLMProvider.LLAMACPP: LLAMACPP_DEFAULT_ENDPOINT,
    }.get(provider)


def provider_options(llm: LLMSettings) -> list[LLMProviderOption]:
    options: list[LLMProviderOption] = []
    for provider, (label, endpoint, api_key, lists_models) in _PROVIDER_FIELDS.items():
        endpoint_value, provider_key = provider_connection(llm, provider)
        options.append(
            LLMProviderOption(
                provider=provider,
                label=label,
                endpoint=endpoint,
                api_key=api_key,
                default_endpoint=_provider_default_endpoint(provider) if endpoint != "none" else None,
                configured_endpoint=endpoint_value,
                api_key_set=bool(provider_key),
                lists_models=lists_models,
            )
        )
    return options


_PROVIDER_CONNECTION_FIELDS: dict[LLMProvider, tuple[str | None, str | None]] = {
    LLMProvider.OLLAMA: ("ollama_endpoint", "ollama_api_key"),
    LLMProvider.OPENAI: ("openai_endpoint", "openai_api_key"),
    LLMProvider.ANTHROPIC: ("anthropic_endpoint", "anthropic_api_key"),
    LLMProvider.GEMINI: (None, "gemini_api_key"),
    LLMProvider.LLAMACPP: ("llamacpp_endpoint", "llamacpp_api_key"),
    LLMProvider.G8E: (None, None),
}


def provider_connection(
    llm: LLMSettings, provider: LLMProvider
) -> tuple[str | None, str | None]:
    fields = _PROVIDER_CONNECTION_FIELDS.get(provider)
    if fields is None:
        raise ValidationError(
            f"Provider '{provider.value}' cannot be selected here",
            field="provider",
            constraint="console_provider",
        )
    endpoint_field, key_field = fields
    endpoint = getattr(llm, endpoint_field) if endpoint_field else None
    api_key = getattr(llm, key_field) if key_field else None
    return endpoint, api_key


def role_view(llm: LLMSettings, role: LLMRole) -> LLMRoleView:
    provider: LLMProvider | None = getattr(llm, f"{role}_provider")
    if provider is None:
        return LLMRoleView()
    return LLMRoleView(
        provider=provider,
        model=getattr(llm, f"{role}_model"),
    )


def settings_view(llm: LLMSettings) -> LLMRoleSettingsResponse:
    return LLMRoleSettingsResponse(
        providers=provider_options(llm),
        primary=role_view(llm, "primary"),
        assistant=role_view(llm, "assistant"),
        lite=role_view(llm, "lite"),
    )


def normalize_endpoint(endpoint: str | None, field: str) -> str | None:
    """Return an http(s) URL without a trailing slash, or None for "provider default".

    A bare host:port (the common way an Ollama or llama.cpp server is written)
    is taken as http.
    """
    value = (endpoint or "").strip()
    if not value:
        return None
    if "://" not in value:
        value = f"http://{value}"
    parts = urlsplit(value)
    if parts.scheme not in ("http", "https") or not parts.hostname:
        raise ValidationError(
            "Endpoint must be an http or https URL", field=field, constraint="http_url"
        )
    return value.rstrip("/")


def _check_provider(provider: LLMProvider, field: str) -> FieldRequirement:
    """Return the endpoint requirement of a console provider."""
    fields = _PROVIDER_FIELDS.get(provider)
    if fields is None:
        raise ValidationError(
            f"Provider '{provider.value}' cannot be selected here",
            field=field,
            constraint="console_provider",
        )
    return fields[1]


def apply_role_updates(llm: LLMSettings, request: LLMRoleSettingsUpdateRequest) -> None:
    """Validate role selections, then replace each selected provider/model pair."""
    updates = {
        role: update
        for role, update in (
            ("primary", request.primary),
            ("assistant", request.assistant),
            ("lite", request.lite),
        )
        if update is not None
    }
    normalized: dict[LLMRole, tuple[LLMProvider | None, str | None]] = {}
    for role, update in updates.items():
        if update.provider is None:
            if role == "primary":
                raise ValidationError(
                    "The primary role needs a provider", field="primary.provider", constraint="required"
                )
            normalized[role] = (None, None)
            continue
        _check_provider(update.provider, f"{role}.provider")
        model = (update.model or "").strip()
        if not model:
            raise ValidationError(
                f"Choose a model for the {role} role", field=f"{role}.model", constraint="required"
            )
        normalized[role] = (update.provider, model)

    for role, (provider, model) in normalized.items():
        setattr(llm, f"{role}_provider", provider)
        setattr(llm, f"{role}_model", model)


def apply_provider_updates(llm: LLMSettings, updates: list[LLMProviderUpdate]) -> None:
    """Validate then save caller-owned provider endpoints and credentials."""
    normalized: list[tuple[LLMProviderUpdate, str | None, bool]] = []
    for update in updates:
        _check_provider(update.provider, f"providers.{update.provider.value}")
        endpoint_rule = _PROVIDER_FIELDS[update.provider][1]
        has_endpoint = "endpoint" in update.model_fields_set
        if endpoint_rule == "none":
            endpoint = None
        elif has_endpoint:
            endpoint = normalize_endpoint(
                update.endpoint, f"providers.{update.provider.value}.endpoint"
            ) or _provider_default_endpoint(update.provider)
        else:
            endpoint_field, _ = _PROVIDER_CONNECTION_FIELDS[update.provider]
            endpoint = getattr(llm, endpoint_field) if endpoint_field else None
        normalized.append((update, endpoint, has_endpoint))

    for update, endpoint, has_endpoint in normalized:
        endpoint_field, key_field = _PROVIDER_CONNECTION_FIELDS[update.provider]
        if endpoint_field and has_endpoint:
            setattr(llm, endpoint_field, endpoint)
        if key_field and update.api_key is not None:
            setattr(llm, key_field, update.api_key.strip() or None)
