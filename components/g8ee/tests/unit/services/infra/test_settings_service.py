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

import pytest
from unittest.mock import AsyncMock, MagicMock
from app.services.infra.settings_service import SettingsService
from app.constants.collections import (
    PLATFORM_SETTINGS_DOC,
    USER_SETTINGS_DOC_PREFIX,
    DB_COLLECTION_SETTINGS,
)
from app.constants.settings import LLMProvider
from app.models.settings import (
    PlatformSettingsDocument,
    UserSettingsDocument,
    G8eePlatformSettings,
    G8eeUserSettings,
    LLMSettings,
)

@pytest.mark.asyncio
class TestSettingsService:
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
                openai_api_key="sk-user-key"
            )
        )
        user_doc = UserSettingsDocument(
            user_id=user_id,
            settings=user_settings
        )
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
            collection=DB_COLLECTION_SETTINGS,
            document_id=user_doc_id
        )

    async def test_get_user_settings_missing_returns_empty_llm(self):
        """Test that missing user settings returns empty LLMSettings (no platform fallback for LLM keys)."""
        cache_mock = MagicMock()
        cache_mock.get_document_with_cache = AsyncMock()
        
        user_id = "user_456"
        user_doc_id = f"{USER_SETTINGS_DOC_PREFIX}{user_id}"
        
        # Mock platform document (without LLM keys since they are user-specific only)
        platform_settings = G8eePlatformSettings(
            port=8080
        )
        platform_doc = PlatformSettingsDocument(
            settings=platform_settings
        )
        
        # Return None for user doc, valid for platform doc
        def get_doc_mock(collection, document_id):
            if document_id == user_doc_id:
                return None
            if document_id == PLATFORM_SETTINGS_DOC:
                return platform_doc.model_dump()
            return None
            
        cache_mock.get_document_with_cache.side_effect = get_doc_mock
        
        service = SettingsService(cache_aside_service=cache_mock)
        settings = await service.get_user_settings(user_id)

        # LLM settings should be empty (no platform fallback)
        assert settings.llm.primary_provider is None
        assert settings.llm.openai_api_key is None
        assert settings.llm.anthropic_api_key is None
        assert settings.llm.gemini_api_key is None
        assert settings.llm.ollama_api_key is None
        
        # Verify both lookups happened
        cache_mock.get_document_with_cache.assert_any_call(
            collection=DB_COLLECTION_SETTINGS,
            document_id=user_doc_id
        )
        cache_mock.get_document_with_cache.assert_any_call(
            collection=DB_COLLECTION_SETTINGS,
            document_id=PLATFORM_SETTINGS_DOC
        )

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
        user_doc = UserSettingsDocument(
            user_id=user_id,
            settings=user_settings
        )

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
                primary_provider=LLMProvider.OLLAMA,
                primary_model="gemma3:27b",
                llm_max_tokens=2048
            )
        )
        user_doc = UserSettingsDocument(
            user_id=user_id,
            settings=user_settings
        )

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
        user_doc = UserSettingsDocument(
            user_id=user_id,
            settings=user_settings
        )

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
        user_doc = UserSettingsDocument(
            user_id=user_id,
            settings=user_settings
        )

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
        user_doc = UserSettingsDocument(
            user_id=user_id,
            settings=user_settings
        )
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
            llm=LLMSettings(
                primary_provider=LLMProvider.OLLAMA,
                primary_model="gemma3:27b"
            )
        )
        user_doc = UserSettingsDocument(
            user_id=user_id,
            settings=user_settings
        )

        cache_mock.get_document_with_cache.side_effect = lambda collection, document_id: (
            user_doc.model_dump() if document_id == user_doc_id else None
        )

        service = SettingsService(cache_aside_service=cache_mock)
        settings = await service.get_user_settings(user_id)

        assert settings.llm.primary_provider == LLMProvider.OLLAMA

    async def test_update_g8ep_operator_api_key_success(self):
        """Test updating the g8ep operator API key in platform settings."""
        cache_mock = MagicMock()
        cache_mock.update_document = AsyncMock()

        api_key = "g8e_test_key_12345678_abcdef1234567890abcdef1234567890abcdef1234567890abcdef1234567890"

        service = SettingsService(cache_aside_service=cache_mock)
        await service.update_g8ep_operator_api_key(api_key)

        # Verify cache_aside.update_document was called with correct parameters
        cache_mock.update_document.assert_called_once_with(
            collection=DB_COLLECTION_SETTINGS,
            document_id=PLATFORM_SETTINGS_DOC,
            update_data={"settings": {"g8ep_operator_api_key": api_key}}
        )

    async def test_update_g8ep_operator_api_key_without_cache_raises_error(self):
        """Test that updating without CacheAsideService raises ConfigurationError."""
        from app.errors import ConfigurationError

        service = SettingsService(cache_aside_service=None)
        api_key = "g8e_test_key_12345678_abcdef1234567890abcdef1234567890abcdef1234567890abcdef1234567890"

        with pytest.raises(ConfigurationError, match="CacheAsideService required"):
            await service.update_g8ep_operator_api_key(api_key)
