# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.

from unittest.mock import AsyncMock, MagicMock

import pytest

from app.constants.collections import DB_COLLECTION_SETTINGS, USER_SETTINGS_DOC_PREFIX
from app.constants.config import LLMProvider
from app.errors import ValidationError
from app.models.http_context import RequestContext
from app.models.internal_api import (
    LLMProviderUpdate,
    LLMRoleSettingsUpdateRequest,
    LLMRoleUpdate,
)
from app.models.settings import G8eeUserSettings, LLMSettings, UserSettingsDocument
from app.services.infra.llm_role_settings import (
    apply_provider_updates,
    apply_role_updates,
    normalize_endpoint,
    provider_connection,
    settings_view,
)
from app.services.infra.settings_service import SettingsService

pytestmark = pytest.mark.unit

CTX = RequestContext(user_id="user_1", web_session_id="ws_1")


def _request(**updates) -> LLMRoleSettingsUpdateRequest:
    return LLMRoleSettingsUpdateRequest(context=CTX, **updates)


class TestNormalizeEndpoint:
    def test_bare_host_port_becomes_http(self):
        assert normalize_endpoint("192.168.1.2:11434", "e") == "http://192.168.1.2:11434"

    def test_trailing_slash_and_whitespace_trimmed(self):
        assert (
            normalize_endpoint("  https://api.example.com/v1/ ", "e")
            == "https://api.example.com/v1"
        )

    @pytest.mark.parametrize("value", [None, "", "   "])
    def test_empty_means_provider_default(self, value):
        assert normalize_endpoint(value, "e") is None

    @pytest.mark.parametrize("value", ["file:///etc/passwd", "ftp://host", "http://"])
    def test_rejects_non_http_urls(self, value):
        with pytest.raises(ValidationError):
            normalize_endpoint(value, "e")


class TestApplyRoleUpdates:
    def test_default_roles_resolve_through_governed_inference(self):
        llm = LLMSettings(primary_model="chosen-model")
        for role in ("primary", "assistant", "lite"):
            assert llm.resolve(role) == ("g8e", None, None, "chosen-model")
        assert llm.assistant_provider is None
        assert llm.lite_provider is None

    def test_writes_only_role_provider_and_model(self):
        llm = LLMSettings(
            ollama_endpoint="http://ollama:11434",
            ollama_api_key="ollama-key",
            openai_endpoint="https://openai-proxy.example/v1",
            openai_api_key="openai-key",
        )
        apply_role_updates(
            llm,
            _request(
                primary=LLMRoleUpdate(provider=LLMProvider.OLLAMA, model="gemma4:e4b"),
                assistant=LLMRoleUpdate(provider=LLMProvider.OPENAI, model="gpt-5-mini"),
                lite=LLMRoleUpdate(provider=LLMProvider.G8E, model="qwen3:1.7b"),
            ),
        )

        assert llm.primary_provider is LLMProvider.OLLAMA
        assert llm.primary_model == "gemma4:e4b"
        assert llm.assistant_provider is LLMProvider.OPENAI
        assert llm.assistant_model == "gpt-5-mini"
        assert llm.lite_provider is LLMProvider.G8E
        assert llm.lite_model == "qwen3:1.7b"
        assert llm.resolve("primary") == (
            "ollama",
            "ollama-key",
            "http://ollama:11434",
            "gemma4:e4b",
        )
        assert llm.resolve("assistant") == (
            "openai",
            "openai-key",
            "https://openai-proxy.example/v1",
            "gpt-5-mini",
        )

    def test_partial_update_leaves_other_roles_unchanged(self):
        llm = LLMSettings(
            primary_provider=LLMProvider.OLLAMA,
            primary_model="primary",
            assistant_provider=LLMProvider.OPENAI,
            assistant_model="assistant",
        )
        apply_role_updates(
            llm,
            _request(primary=LLMRoleUpdate(provider=LLMProvider.G8E, model="replacement")),
        )
        assert llm.primary_provider is LLMProvider.G8E
        assert llm.assistant_provider is LLMProvider.OPENAI
        assert llm.assistant_model == "assistant"

    def test_unset_optional_role_falls_back_to_primary(self):
        llm = LLMSettings(
            primary_provider=LLMProvider.OLLAMA,
            primary_model="gemma4:e4b",
            assistant_provider=LLMProvider.OPENAI,
            assistant_model="gpt-x",
        )
        apply_role_updates(llm, _request(assistant=LLMRoleUpdate()))
        assert llm.assistant_provider is None
        assert llm.assistant_model is None
        assert llm.resolve("assistant")[0] == "ollama"

    def test_primary_requires_provider(self):
        with pytest.raises(ValidationError):
            apply_role_updates(LLMSettings(), _request(primary=LLMRoleUpdate()))

    def test_model_required_when_provider_set(self):
        with pytest.raises(ValidationError):
            apply_role_updates(
                LLMSettings(),
                _request(primary=LLMRoleUpdate(provider=LLMProvider.OLLAMA, model="  ")),
            )

    @pytest.mark.parametrize("provider", [LLMProvider.JEV, LLMProvider.FAKE])
    def test_rejects_providers_outside_the_console_set(self, provider):
        with pytest.raises(ValidationError):
            apply_role_updates(
                LLMSettings(),
                _request(primary=LLMRoleUpdate(provider=provider, model="m")),
            )

    def test_invalid_role_leaves_settings_untouched(self):
        llm = LLMSettings(primary_provider=LLMProvider.OLLAMA, primary_model="old")
        with pytest.raises(ValidationError):
            apply_role_updates(
                llm,
                _request(
                    primary=LLMRoleUpdate(provider=LLMProvider.OLLAMA, model="new"),
                    assistant=LLMRoleUpdate(provider=LLMProvider.FAKE, model="bad"),
                ),
            )
        assert llm.primary_model == "old"


class TestProviderConnections:
    def test_updates_connections_independently_of_roles(self):
        llm = LLMSettings(primary_provider=LLMProvider.OPENAI, primary_model="gpt-5")
        apply_provider_updates(
            llm,
            [
                LLMProviderUpdate(
                    provider=LLMProvider.OPENAI,
                    endpoint="https://proxy.example/v1/",
                    api_key=" sk-new ",
                ),
                LLMProviderUpdate(provider=LLMProvider.OLLAMA, endpoint="ollama.lan:11434"),
            ],
        )
        assert provider_connection(llm, LLMProvider.OPENAI) == (
            "https://proxy.example/v1",
            "sk-new",
        )
        assert provider_connection(llm, LLMProvider.OLLAMA)[0] == "http://ollama.lan:11434"
        assert llm.primary_provider is LLMProvider.OPENAI
        assert llm.primary_model == "gpt-5"

    def test_omitted_key_keeps_it_and_empty_key_clears_it(self):
        llm = LLMSettings(openai_api_key="stored")
        apply_provider_updates(
            llm, [LLMProviderUpdate(provider=LLMProvider.OPENAI, endpoint="https://proxy.example")]
        )
        assert llm.openai_api_key == "stored"
        apply_provider_updates(llm, [LLMProviderUpdate(provider=LLMProvider.OPENAI, api_key="")])
        assert llm.openai_api_key is None

    def test_invalid_batch_leaves_connections_untouched(self):
        llm = LLMSettings(openai_api_key="old")
        with pytest.raises(ValidationError):
            apply_provider_updates(
                llm,
                [
                    LLMProviderUpdate(provider=LLMProvider.OPENAI, api_key="new"),
                    LLMProviderUpdate(provider=LLMProvider.OLLAMA, endpoint="ftp://bad"),
                ],
            )
        assert llm.openai_api_key == "old"


class TestSettingsView:
    def test_masks_provider_keys_and_keeps_roles_connection_free(self):
        llm = LLMSettings(
            primary_provider=LLMProvider.ANTHROPIC,
            primary_model="claude",
            anthropic_api_key="sk-provider",
        )
        view = settings_view(llm)
        dumped = view.model_dump_json()
        anthropic = next(
            option for option in view.providers if option.provider is LLMProvider.ANTHROPIC
        )
        assert "sk-provider" not in dumped
        assert anthropic.api_key_set is True
        assert view.primary.model == "claude"
        assert set(type(view.primary).model_fields) == {"provider", "model"}

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
        assert options[LLMProvider.OLLAMA].configured_endpoint == "http://ollama.lan:11434"
        assert options[LLMProvider.G8E].lists_models is True
        assert options[LLMProvider.GEMINI].endpoint == "none"


@pytest.mark.asyncio
class TestSettingsServiceLLMRoles:
    async def test_update_persists_provider_connection_and_role_selection(self):
        cache = MagicMock()
        cache.update_document = AsyncMock()
        cache.invalidate_document = AsyncMock(return_value=True)
        service = SettingsService(cache_aside_service=cache)
        service.get_user_settings = AsyncMock(return_value=G8eeUserSettings(llm=LLMSettings()))
        request = _request(
            primary=LLMRoleUpdate(provider=LLMProvider.OLLAMA, model="gemma4:e4b"),
            providers=[
                LLMProviderUpdate(
                    provider=LLMProvider.OLLAMA,
                    endpoint="192.168.1.2:11434",
                    api_key="k",
                )
            ],
        )

        view = await service.update_llm_role_settings("user_1", request)

        doc_id = f"{USER_SETTINGS_DOC_PREFIX}user_1"
        kwargs = cache.update_document.await_args.kwargs
        assert kwargs["collection"] == DB_COLLECTION_SETTINGS
        assert kwargs["document_id"] == doc_id
        stored = UserSettingsDocument.model_validate(kwargs["data"]).settings.llm
        assert stored.primary_provider is LLMProvider.OLLAMA
        assert stored.primary_model == "gemma4:e4b"
        assert stored.ollama_endpoint == "http://192.168.1.2:11434"
        assert stored.ollama_api_key == "k"
        cache.invalidate_document.assert_awaited_once_with(DB_COLLECTION_SETTINGS, doc_id)
        ollama = next(option for option in view.providers if option.provider is LLMProvider.OLLAMA)
        assert ollama.api_key_set is True

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
            _request(
                primary=LLMRoleUpdate(provider=LLMProvider.OLLAMA, model="gemma4:e4b"),
                assistant=LLMRoleUpdate(provider=LLMProvider.OLLAMA, model="qwen3:4b"),
            ),
        )

        settings = await service.get_user_settings("user_1")

        assert settings.llm.primary_model == "gemma4:e4b"
        assert settings.llm.resolved_assistant_model == "qwen3:4b"
        assert settings.llm.resolve("lite")[0] == "ollama"
