# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

import os
from unittest.mock import AsyncMock, MagicMock

import pytest

from app.constants.collections import (
    DB_COLLECTION_SETTINGS,
    PLATFORM_SETTINGS_DOC,
    USER_SETTINGS_DOC_PREFIX,
)
from app.constants.config import LLMProvider
from app.constants.env_vars import EnvVar
from app.models.settings import (
    G8eeAppSettings,
    G8eeUserSettings,
    LLMSettings,
    AppSettingsDocument,
    UserSettingsDocument,
)
from app.services.infra.settings_service import SettingsService


@pytest.mark.asyncio
class TestSettingsService:
    @pytest.fixture(autouse=True)
    def _clean_llm_env(self, monkeypatch):
        """Remove LLM env vars before each test so .env leakage from
        app.main import does not affect settings service tests."""
        for attr in dir(EnvVar):
            val = getattr(EnvVar, attr)
            if isinstance(val, str) and val.startswith("G8E_LLM_"):
                monkeypatch.delenv(val, raising=False)

    async def test_get_user_settings_success(self):
        """Test retrieving user settings when the document exists."""
        cache_mock = MagicMock()
        cache_mock.get_document_with_cache = AsyncMock()

        user_id = "user_123"
        user_doc_id = f"{USER_SETTINGS_DOC_PREFIX}{user_id}"

        # Mock user document
        user_settings = G8eeUserSettings(
            llm=LLMSettings(
                primary_provider=LLMProvider.OPENAI,
                primary_model="gpt-4",
                openai_api_key="sk-user-key",
            )
        )
        user_doc = UserSettingsDocument(user_id=user_id, settings=user_settings)
        cache_mock.get_document_with_cache.side_effect = lambda collection, document_id: (
            user_doc.model_dump() if document_id == user_doc_id else None
        )

        service = SettingsService(cache_aside_service=cache_mock)
        settings = await service.get_user_settings(user_id)

        assert settings.llm.primary_provider == LLMProvider.OPENAI
        assert settings.llm.primary_model == "gpt-4"
        assert settings.llm.openai_api_key == "sk-user-key"

        # Verify cache calls
        cache_mock.get_document_with_cache.assert_any_call(
            collection=DB_COLLECTION_SETTINGS, document_id=user_doc_id
        )

    async def test_get_user_settings_missing_returns_empty_defaults(self):
        """Missing user settings document yields empty defaults so request-scoped
        LLM overrides (CLI/BYO) can populate validate_llm_config. Hard failure on
        absent credentials is the responsibility of validate_llm_config, not this
        dependency-injected loader."""
        cache_mock = MagicMock()
        cache_mock.get_document_with_cache = AsyncMock()

        user_id = "user_456"
        user_doc_id = f"{USER_SETTINGS_DOC_PREFIX}{user_id}"

        platform_doc = AppSettingsDocument(settings=G8eeAppSettings()).model_dump()

        cache_mock.get_document_with_cache.side_effect = lambda collection, document_id: (
            None if document_id == user_doc_id else platform_doc
        )

        service = SettingsService(cache_aside_service=cache_mock)
        settings = await service.get_user_settings(user_id)

        assert isinstance(settings, G8eeUserSettings)
        assert settings.llm.primary_provider is None
        assert settings.llm.primary_model is None
        assert settings.llm.primary_api_key is None

        cache_mock.get_document_with_cache.assert_any_call(
            collection=DB_COLLECTION_SETTINGS, document_id=user_doc_id
        )
        cache_mock.get_document_with_cache.assert_any_call(
            collection=DB_COLLECTION_SETTINGS, document_id=PLATFORM_SETTINGS_DOC
        )
        assert cache_mock.get_document_with_cache.call_count == 2

    async def test_llm_settings_no_overrides(self):
        """Test that llm_max_tokens is None if not provided in user settings."""
        cache_mock = MagicMock()
        cache_mock.get_document_with_cache = AsyncMock()

        user_id = "user_temp"
        user_doc_id = f"{USER_SETTINGS_DOC_PREFIX}{user_id}"

        user_settings = G8eeUserSettings(
            llm=LLMSettings(
                primary_provider=LLMProvider.OLLAMA,
                primary_model="gemma3:27b",
            )
        )
        user_doc = UserSettingsDocument(user_id=user_id, settings=user_settings)

        cache_mock.get_document_with_cache.side_effect = lambda collection, document_id: (
            user_doc.model_dump() if document_id == user_doc_id else None
        )

        service = SettingsService(cache_aside_service=cache_mock)
        settings = await service.get_user_settings(user_id)

        assert settings.llm.llm_max_tokens is None

    async def test_llm_settings_with_overrides(self):
        """Test that llm_max_tokens ARE set if provided in user settings."""
        cache_mock = MagicMock()
        cache_mock.get_document_with_cache = AsyncMock()

        user_id = "user_override"
        user_doc_id = f"{USER_SETTINGS_DOC_PREFIX}{user_id}"

        user_settings = G8eeUserSettings(
            llm=LLMSettings(
                primary_provider=LLMProvider.OLLAMA, primary_model="gemma3:27b", llm_max_tokens=2048
            )
        )
        user_doc = UserSettingsDocument(user_id=user_id, settings=user_settings)

        cache_mock.get_document_with_cache.side_effect = lambda collection, document_id: (
            user_doc.model_dump() if document_id == user_doc_id else None
        )

        service = SettingsService(cache_aside_service=cache_mock)
        settings = await service.get_user_settings(user_id)

        assert settings.llm.llm_max_tokens == 2048

    async def test_command_gen_defaults_preserved_when_db_has_no_values(self):
        """Regression: llm_command_gen_passes=None caused TypeError in max(1, None).

        When the DB has no command_gen settings, LLMSettings defaults must survive.
        """
        cache_mock = MagicMock()
        cache_mock.get_document_with_cache = AsyncMock()

        user_id = "user_cmdgen"
        user_doc_id = f"{USER_SETTINGS_DOC_PREFIX}{user_id}"

        user_settings = G8eeUserSettings(
            llm=LLMSettings(
                primary_provider=LLMProvider.OLLAMA,
                primary_model="gemma3:27b",
            )
        )
        user_doc = UserSettingsDocument(user_id=user_id, settings=user_settings)

        cache_mock.get_document_with_cache.side_effect = lambda collection, document_id: (
            user_doc.model_dump() if document_id == user_doc_id else None
        )

        service = SettingsService(cache_aside_service=cache_mock)
        settings = await service.get_user_settings(user_id)

        assert settings.llm.llm_command_gen_passes == 5
        assert settings.llm.llm_command_gen_enabled is True
        assert settings.llm.llm_command_gen_auditor is True

    async def test_command_gen_overrides_applied_when_db_has_values(self):
        """Explicit DB values for command_gen fields override the defaults."""
        cache_mock = MagicMock()
        cache_mock.get_document_with_cache = AsyncMock()

        user_id = "user_cmdgen_override"
        user_doc_id = f"{USER_SETTINGS_DOC_PREFIX}{user_id}"

        user_settings = G8eeUserSettings(
            llm=LLMSettings(
                primary_provider=LLMProvider.OLLAMA,
                primary_model="gemma3:27b",
                llm_command_gen_passes=5,
                llm_command_gen_enabled=False,
                llm_command_gen_auditor=False,
            )
        )
        user_doc = UserSettingsDocument(user_id=user_id, settings=user_settings)

        cache_mock.get_document_with_cache.side_effect = lambda collection, document_id: (
            user_doc.model_dump() if document_id == user_doc_id else None
        )

        service = SettingsService(cache_aside_service=cache_mock)
        settings = await service.get_user_settings(user_id)

        assert settings.llm.llm_command_gen_passes == 5
        assert settings.llm.llm_command_gen_enabled is False
        assert settings.llm.llm_command_gen_auditor is False

    async def test_user_settings_command_gen_defaults_preserved(self):
        """Regression: user settings with no command_gen values must preserve defaults."""
        cache_mock = MagicMock()
        cache_mock.get_document_with_cache = AsyncMock()

        user_id = "user_789"
        user_doc_id = f"{USER_SETTINGS_DOC_PREFIX}{user_id}"

        user_settings = G8eeUserSettings(
            llm=LLMSettings(
                primary_provider=LLMProvider.GEMINI,
                primary_model="gemini-2.5-pro",
                gemini_api_key="test-key",
            )
        )
        user_doc = UserSettingsDocument(user_id=user_id, settings=user_settings)
        cache_mock.get_document_with_cache.side_effect = lambda collection, document_id: (
            user_doc.model_dump() if document_id == user_doc_id else None
        )

        service = SettingsService(cache_aside_service=cache_mock)
        settings = await service.get_user_settings(user_id)

        assert settings.llm.llm_command_gen_passes == 5
        assert settings.llm.llm_command_gen_enabled is True
        assert settings.llm.llm_command_gen_auditor is True

    async def test_llm_settings_provider_preserved(self):
        """Test that valid provider is preserved in user settings (explicitly set, not a default)."""
        cache_mock = MagicMock()
        cache_mock.get_document_with_cache = AsyncMock()

        user_id = "user_provider"
        user_doc_id = f"{USER_SETTINGS_DOC_PREFIX}{user_id}"

        user_settings = G8eeUserSettings(
            llm=LLMSettings(primary_provider=LLMProvider.OLLAMA, primary_model="gemma3:27b")
        )
        user_doc = UserSettingsDocument(user_id=user_id, settings=user_settings)

        cache_mock.get_document_with_cache.side_effect = lambda collection, document_id: (
            user_doc.model_dump() if document_id == user_doc_id else None
        )

        service = SettingsService(cache_aside_service=cache_mock)
        settings = await service.get_user_settings(user_id)

        assert settings.llm.primary_provider == LLMProvider.OLLAMA


class TestLLMEnvVarBootstrapDefaults:
    """D.6: LLM env-var bootstrap defaults.

    A fresh deployment can serve chat via LLM env vars alone, without
    platform DB configuration or per-request overrides. Priority order:
    platform DB settings > per-request overrides > env-var defaults.
    """

    @pytest.fixture(autouse=True)
    def _clean_llm_env(self, monkeypatch):
        """Remove all LLM env vars before each test so prior tests do not leak."""
        for attr in dir(EnvVar):
            val = getattr(EnvVar, attr)
            if isinstance(val, str) and val.startswith("G8E_LLM_"):
                monkeypatch.delenv(val, raising=False)

    def _make_service(self) -> SettingsService:
        bootstrap = MagicMock()
        bootstrap.load_session_encryption_key.return_value = None
        bootstrap.load_auditor_hmac_key.return_value = None
        return SettingsService(bootstrap_service=bootstrap)

    def test_env_vars_populate_local_settings(self, monkeypatch):
        """Env vars set primary provider, model, endpoint, and api_key on local settings."""
        monkeypatch.setenv(EnvVar.LLM_PRIMARY_PROVIDER, "ollama")
        monkeypatch.setenv(EnvVar.LLM_PRIMARY_MODEL, "gemma4:12b")
        monkeypatch.setenv(EnvVar.LLM_PRIMARY_ENDPOINT, "http://192.168.1.2:11434")
        monkeypatch.setenv(EnvVar.LLM_PRIMARY_API_KEY, "env-key")

        service = self._make_service()
        settings = service.get_local_settings()

        assert settings.llm.primary_provider == LLMProvider.OLLAMA
        assert settings.llm.primary_model == "gemma4:12b"
        assert settings.llm.primary_endpoint == "http://192.168.1.2:11434"
        assert settings.llm.primary_api_key == "env-key"

    def test_no_env_vars_leaves_defaults(self):
        """With no LLM env vars set, local settings retain model defaults (None)."""
        service = self._make_service()
        settings = service.get_local_settings()

        assert settings.llm.primary_provider is None
        assert settings.llm.primary_model is None
        assert settings.llm.primary_endpoint is None
        assert settings.llm.primary_api_key is None

    def test_provider_specific_endpoint_env_vars(self, monkeypatch):
        """Provider-specific endpoint/api-key env vars populate the matching fields."""
        monkeypatch.setenv(EnvVar.LLM_OLLAMA_ENDPOINT, "http://10.0.0.5:11434")
        monkeypatch.setenv(EnvVar.LLM_OPENAI_API_KEY, "sk-env-openai")

        service = self._make_service()
        settings = service.get_local_settings()

        assert settings.llm.ollama_endpoint == "http://10.0.0.5:11434"
        assert settings.llm.openai_api_key == "sk-env-openai"

    def test_platform_db_overrides_env_defaults(self, monkeypatch):
        """Platform DB values take precedence over env-var defaults (priority order)."""
        monkeypatch.setenv(EnvVar.LLM_PRIMARY_PROVIDER, "ollama")
        monkeypatch.setenv(EnvVar.LLM_PRIMARY_MODEL, "gemma4:12b")

        service = self._make_service()
        local = service.get_local_settings()
        assert local.llm.primary_provider == LLMProvider.OLLAMA

        # Platform DB carries a different provider and model.
        platform = G8eeAppSettings(
            llm=LLMSettings(
                primary_provider=LLMProvider.OPENAI,
                primary_model="gpt-4o",
                openai_api_key="platform-key",
            )
        )

        merged = service.overlay_platform_data(local, platform)

        assert merged.llm.primary_provider == LLMProvider.OPENAI
        assert merged.llm.primary_model == "gpt-4o"
        assert merged.llm.openai_api_key == "platform-key"

    def test_env_defaults_preserved_when_platform_db_empty(self, monkeypatch):
        """Env-var defaults are preserved when the platform DB has no value for a field."""
        monkeypatch.setenv(EnvVar.LLM_PRIMARY_PROVIDER, "ollama")
        monkeypatch.setenv(EnvVar.LLM_PRIMARY_MODEL, "gemma4:12b")
        monkeypatch.setenv(EnvVar.LLM_OLLAMA_ENDPOINT, "http://192.168.1.2:11434")

        service = self._make_service()
        local = service.get_local_settings()
        assert local.llm.primary_provider == LLMProvider.OLLAMA

        # Platform DB has no LLM values (all None).
        platform = G8eeAppSettings()

        merged = service.overlay_platform_data(local, platform)

        assert merged.llm.primary_provider == LLMProvider.OLLAMA
        assert merged.llm.primary_model == "gemma4:12b"
        assert merged.llm.ollama_endpoint == "http://192.168.1.2:11434"

    def test_platform_db_fills_gaps_not_in_env(self, monkeypatch):
        """Platform DB fills fields that env vars did not set (merge semantics)."""
        monkeypatch.setenv(EnvVar.LLM_PRIMARY_PROVIDER, "ollama")
        # No model or api_key in env.

        service = self._make_service()
        local = service.get_local_settings()
        assert local.llm.primary_provider == LLMProvider.OLLAMA
        assert local.llm.primary_model is None

        platform = G8eeAppSettings(
            llm=LLMSettings(
                primary_model="gemma4:12b",
                ollama_api_key="platform-ollama-key",
            )
        )

        merged = service.overlay_platform_data(local, platform)

        # Env provider wins (local set), platform model fills the gap.
        assert merged.llm.primary_provider == LLMProvider.OLLAMA
        assert merged.llm.primary_model == "gemma4:12b"
        assert merged.llm.ollama_api_key == "platform-ollama-key"
