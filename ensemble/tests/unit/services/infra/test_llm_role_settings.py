# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

from unittest.mock import AsyncMock, MagicMock

import pytest

from app.constants.collections import DB_COLLECTION_SETTINGS, USER_SETTINGS_DOC_PREFIX
from app.constants.config import LLMProvider
from app.errors import ValidationError
from app.models.http_context import RequestContext
from app.models.internal_api import LLMRoleSettingsUpdateRequest, LLMRoleUpdate
from app.models.settings import G8eeUserSettings, LLMSettings, UserSettingsDocument
from app.services.infra.llm_role_settings import (
    apply_role_updates,
    normalize_endpoint,
    settings_view,
    stored_connection,
)
from app.services.infra.settings_service import SettingsService

pytestmark = pytest.mark.unit

CTX = RequestContext(user_id="user_1", web_session_id="ws_1")


def _request(primary: LLMRoleUpdate, assistant=None, lite=None) -> LLMRoleSettingsUpdateRequest:
    return LLMRoleSettingsUpdateRequest(
        context=CTX,
        primary=primary,
        assistant=assistant or LLMRoleUpdate(),
        lite=lite or LLMRoleUpdate(),
    )


def _ollama(model="gemma4:e4b", endpoint="192.168.1.2:11434", api_key=None) -> LLMRoleUpdate:
    return LLMRoleUpdate(provider=LLMProvider.OLLAMA, model=model, endpoint=endpoint, api_key=api_key)


class TestNormalizeEndpoint:
    def test_bare_host_port_becomes_http(self):
        assert normalize_endpoint("192.168.1.2:11434", "e") == "http://192.168.1.2:11434"

    def test_trailing_slash_and_whitespace_trimmed(self):
        assert normalize_endpoint("  https://api.example.com/v1/ ", "e") == "https://api.example.com/v1"

    @pytest.mark.parametrize("value", [None, "", "   "])
    def test_empty_means_provider_default(self, value):
        assert normalize_endpoint(value, "e") is None

    @pytest.mark.parametrize("value", ["file:///etc/passwd", "ftp://host", "http://"])
    def test_rejects_non_http_urls(self, value):
        with pytest.raises(ValidationError):
            normalize_endpoint(value, "e")


class TestApplyRoleUpdates:
    def test_writes_role_specific_fields(self):
        llm = LLMSettings()
        apply_role_updates(
            llm,
            _request(
                _ollama(),
                assistant=_ollama(model="qwen3:4b"),
                lite=LLMRoleUpdate(provider=LLMProvider.G8E, model="gemma3:1b", endpoint="ignored"),
            ),
        )
        assert llm.primary_provider is LLMProvider.OLLAMA
        assert llm.primary_model == "gemma4:e4b"
        assert llm.primary_endpoint == "http://192.168.1.2:11434"
        assert llm.assistant_model == "qwen3:4b"
        assert llm.lite_provider is LLMProvider.G8E
        assert llm.lite_endpoint is None
        assert llm.resolve("primary") == (
            "ollama",
            None,
            "http://192.168.1.2:11434",
            "gemma4:e4b",
        )

    def test_unset_assistant_and_lite_fall_back_to_primary(self):
        llm = LLMSettings(assistant_provider=LLMProvider.OPENAI, assistant_model="gpt-x")
        apply_role_updates(llm, _request(_ollama()))
        assert llm.assistant_provider is None
        assert llm.assistant_model is None
        assert llm.resolve("assistant")[0] == "ollama"
        assert llm.resolve("lite")[3] == "gemma4:e4b"

    def test_primary_requires_provider(self):
        with pytest.raises(ValidationError):
            apply_role_updates(LLMSettings(), _request(LLMRoleUpdate()))

    def test_model_required_when_provider_set(self):
        with pytest.raises(ValidationError):
            apply_role_updates(LLMSettings(), _request(_ollama(model="  ")))

    @pytest.mark.parametrize("provider", [LLMProvider.JEV, LLMProvider.FAKE])
    def test_rejects_providers_outside_the_console_set(self, provider):
        with pytest.raises(ValidationError):
            apply_role_updates(
                LLMSettings(), _request(LLMRoleUpdate(provider=provider, model="m"))
            )

    def test_invalid_role_leaves_settings_untouched(self):
        llm = LLMSettings(primary_provider=LLMProvider.OLLAMA, primary_model="old")
        with pytest.raises(ValidationError):
            apply_role_updates(
                llm, _request(_ollama(model="new"), assistant=_ollama(endpoint="ftp://x"))
            )
        assert llm.primary_model == "old"

    def test_null_api_key_keeps_stored_key_for_same_provider(self):
        llm = LLMSettings(
            primary_provider=LLMProvider.OPENAI, primary_model="a", primary_api_key="sk-old"
        )
        apply_role_updates(
            llm, _request(LLMRoleUpdate(provider=LLMProvider.OPENAI, model="b"))
        )
        assert llm.primary_api_key == "sk-old"

    def test_provider_change_drops_stored_key(self):
        llm = LLMSettings(
            primary_provider=LLMProvider.OPENAI, primary_model="a", primary_api_key="sk-old"
        )
        apply_role_updates(llm, _request(_ollama()))
        assert llm.primary_api_key is None

    def test_empty_api_key_clears_and_value_replaces(self):
        llm = LLMSettings(
            primary_provider=LLMProvider.OLLAMA, primary_model="a", primary_api_key="old"
        )
        apply_role_updates(llm, _request(_ollama(api_key="")))
        assert llm.primary_api_key is None
        apply_role_updates(llm, _request(_ollama(api_key=" new ")))
        assert llm.primary_api_key == "new"


class TestSettingsView:
    def test_masks_keys_and_reports_provider_level_key(self):
        llm = LLMSettings(
            primary_provider=LLMProvider.ANTHROPIC,
            primary_model="claude",
            anthropic_api_key="sk-provider",
        )
        view = settings_view(llm)
        dumped = view.model_dump_json()
        assert "sk-provider" not in dumped
        assert view.primary.api_key_set is True
        assert view.assistant.provider is None
        assert view.assistant.api_key_set is False

    def test_lists_only_console_providers_with_defaults(self):
        llm = LLMSettings(ollama_endpoint="http://ollama.lan:11434")
        options = {o.provider: o for o in settings_view(llm).providers}
        assert set(options) == {
            LLMProvider.OLLAMA,
            LLMProvider.OPENAI,
            LLMProvider.ANTHROPIC,
            LLMProvider.GEMINI,
            LLMProvider.LLAMACPP,
            LLMProvider.G8E,
        }
        assert options[LLMProvider.OLLAMA].default_endpoint == "http://ollama.lan:11434"
        assert options[LLMProvider.G8E].lists_models is False
        assert options[LLMProvider.GEMINI].endpoint == "none"


class TestStoredConnection:
    def test_uses_role_values_for_the_stored_provider(self):
        llm = LLMSettings(
            primary_provider=LLMProvider.OLLAMA,
            primary_endpoint="http://role:11434",
            primary_api_key="role-key",
        )
        assert stored_connection(llm, "primary", LLMProvider.OLLAMA) == (
            "http://role:11434",
            "role-key",
        )

    def test_ignores_role_values_for_another_provider(self):
        llm = LLMSettings(
            primary_provider=LLMProvider.OLLAMA,
            primary_endpoint="http://role:11434",
            primary_api_key="role-key",
            openai_api_key="sk-provider",
        )
        endpoint, key = stored_connection(llm, "primary", LLMProvider.OPENAI)
        assert endpoint == llm.openai_endpoint
        assert key == "sk-provider"


@pytest.mark.asyncio
class TestSettingsServiceLLMRoles:
    async def test_update_persists_and_invalidates_cache(self):
        cache = MagicMock()
        cache.get_document_with_cache = AsyncMock(return_value=None)
        cache.update_document = AsyncMock()
        cache.invalidate_document = AsyncMock(return_value=True)
        service = SettingsService(cache_aside_service=cache)
        service.get_user_settings = AsyncMock(return_value=G8eeUserSettings(llm=LLMSettings()))

        view = await service.update_llm_role_settings("user_1", _request(_ollama(api_key="k")))

        doc_id = f"{USER_SETTINGS_DOC_PREFIX}user_1"
        kwargs = cache.update_document.await_args.kwargs
        assert kwargs["collection"] == DB_COLLECTION_SETTINGS
        assert kwargs["document_id"] == doc_id
        stored = UserSettingsDocument.model_validate(kwargs["data"]).settings.llm
        assert stored.primary_provider is LLMProvider.OLLAMA
        assert stored.primary_model == "gemma4:e4b"
        assert stored.primary_api_key == "k"
        cache.invalidate_document.assert_awaited_once_with(DB_COLLECTION_SETTINGS, doc_id)
        assert view.primary.model == "gemma4:e4b"
        assert view.primary.api_key_set is True

    async def test_saved_document_round_trips_through_get_user_settings(self):
        stored: dict = {}

        async def update_document(collection, document_id, data, merge):
            stored[document_id] = data

        cache = MagicMock()
        cache.update_document = AsyncMock(side_effect=update_document)
        cache.invalidate_document = AsyncMock(return_value=True)
        cache.get_document_with_cache = AsyncMock(
            side_effect=lambda collection, document_id: stored.get(document_id)
        )
        service = SettingsService(cache_aside_service=cache)
        await service.update_user_settings("user_1", G8eeUserSettings(llm=LLMSettings()))

        await service.update_llm_role_settings(
            "user_1",
            _request(_ollama(), assistant=_ollama(model="qwen3:4b")),
        )
        settings = await service.get_user_settings("user_1")

        assert settings.llm.primary_model == "gemma4:e4b"
        assert settings.llm.resolved_assistant_model == "qwen3:4b"
        assert settings.llm.resolve("lite")[0] == "ollama"
