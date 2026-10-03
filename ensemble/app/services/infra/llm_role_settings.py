# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

"""Browser-facing view and update rules for per-role LLM selection.

The console assigns a provider, model, endpoint, and API key to each of the
primary, assistant, and lite roles. Those land in the role-specific
``LLMSettings`` fields, which ``LLMSettings.resolve()`` reads ahead of the
provider-level defaults. API keys never leave g8ee: the view reports only
whether one resolves for the role.

Every provider, g8e included, stores the model the user chose for the role. The
Inference Operator is a worker and never decides it; the console lists the
models the Operator's Ollama provider serves from /settings/llm/models.
"""

from __future__ import annotations

from typing import get_args
from urllib.parse import urlsplit

from app.constants import LLMProvider
from app.errors import ValidationError
from app.models.internal_api import (
    FieldRequirement,
    LLMProviderOption,
    LLMRole,
    LLMRoleSettingsResponse,
    LLMRoleSettingsUpdateRequest,
    LLMRoleUpdate,
    LLMRoleView,
)
from app.models.settings import LLMSettings

ROLES: tuple[LLMRole, ...] = get_args(LLMRole)

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


def _provider_default_endpoint(llm: LLMSettings, provider: LLMProvider) -> str | None:
    return {
        LLMProvider.OLLAMA: llm.ollama_endpoint,
        LLMProvider.OPENAI: llm.openai_endpoint,
        LLMProvider.ANTHROPIC: llm.anthropic_endpoint,
        LLMProvider.LLAMACPP: llm.llamacpp_endpoint,
    }.get(provider)


def provider_options(llm: LLMSettings) -> list[LLMProviderOption]:
    return [
        LLMProviderOption(
            provider=provider,
            label=label,
            endpoint=endpoint,
            api_key=api_key,
            default_endpoint=_provider_default_endpoint(llm, provider) if endpoint != "none" else None,
            lists_models=lists_models,
        )
        for provider, (label, endpoint, api_key, lists_models) in _PROVIDER_FIELDS.items()
    ]


def role_view(llm: LLMSettings, role: LLMRole) -> LLMRoleView:
    provider: LLMProvider | None = getattr(llm, f"{role}_provider")
    if provider is None:
        return LLMRoleView()
    _, api_key, _, _ = llm.resolve(role)
    return LLMRoleView(
        provider=provider,
        model=getattr(llm, f"{role}_model"),
        endpoint=getattr(llm, f"{role}_endpoint"),
        api_key_set=bool(api_key),
    )


def settings_view(llm: LLMSettings) -> LLMRoleSettingsResponse:
    return LLMRoleSettingsResponse(
        providers=provider_options(llm),
        primary=role_view(llm, "primary"),
        assistant=role_view(llm, "assistant"),
        lite=role_view(llm, "lite"),
    )


def stored_connection(
    llm: LLMSettings, role: LLMRole, provider: LLMProvider
) -> tuple[str | None, str | None]:
    """Return the (endpoint, api_key) role would use with provider.

    The role's own endpoint and key apply only while the role is stored with
    that same provider; otherwise the provider-level defaults apply.
    """
    if getattr(llm, f"{role}_provider") is provider:
        _, api_key, endpoint, _ = llm.resolve(role)
        return endpoint, api_key
    neutral = llm.model_copy(update={f"{role}_endpoint": None, f"{role}_api_key": None})
    _, api_key, endpoint, _ = neutral.resolve(role, provider_override=provider.value)
    return endpoint, api_key


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
    """Validate every role first, then write the role-specific fields onto llm.

    A role whose provider changes drops its stored key unless a new one is
    given, so a key entered for one provider is never sent to another.
    """
    updates: dict[LLMRole, LLMRoleUpdate] = {
        "primary": request.primary,
        "assistant": request.assistant,
        "lite": request.lite,
    }
    normalized: dict[LLMRole, tuple[LLMProvider | None, str | None, str | None]] = {}
    for role, update in updates.items():
        if update.provider is None:
            if role == "primary":
                raise ValidationError(
                    "The primary role needs a provider", field="primary.provider", constraint="required"
                )
            normalized[role] = (None, None, None)
            continue
        endpoint_rule = _check_provider(update.provider, f"{role}.provider")
        model = (update.model or "").strip()
        if not model:
            raise ValidationError(
                f"Choose a model for the {role} role", field=f"{role}.model", constraint="required"
            )
        endpoint = (
            None
            if endpoint_rule == "none"
            else normalize_endpoint(update.endpoint, f"{role}.endpoint")
        )
        normalized[role] = (update.provider, model, endpoint)

    for role, (provider, model, endpoint) in normalized.items():
        update = updates[role]
        provider_changed = getattr(llm, f"{role}_provider") != provider
        setattr(llm, f"{role}_provider", provider)
        setattr(llm, f"{role}_model", model)
        setattr(llm, f"{role}_endpoint", endpoint)
        if provider is None or (provider_changed and update.api_key is None):
            setattr(llm, f"{role}_api_key", None)
        elif update.api_key is not None:
            setattr(llm, f"{role}_api_key", update.api_key.strip() or None)
