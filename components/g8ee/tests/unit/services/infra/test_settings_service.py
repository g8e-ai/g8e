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
    PlatformSettingsData,
    UserSettingsData
)

@pytest.mark.asyncio
class TestSettingsService:
    async def test_get_user_settings_success(self):
        """Test retrieving user settings when the document exists."""
        cache_mock = MagicMock()
        cache_mock.get_document = AsyncMock()
        
        user_id = "user_123"
        user_doc_id = f"{USER_SETTINGS_DOC_PREFIX}{user_id}"
        
        # Mock user document
        user_data = UserSettingsData(
            llm_provider=LLMProvider.OPENAI,
            llm_model="gpt-4",
            openai_api_key="sk-user-key"
        )
        user_doc = UserSettingsDocument(
            settings=user_data,
            created_at="2026-04-04T00:00:00Z",
            updated_at="2026-04-04T00:00:00Z"
        )
        cache_mock.get_document.side_effect = lambda collection, document_id: (
            user_doc.model_dump() if document_id == user_doc_id else None
        )
        
        service = SettingsService(cache_aside_service=cache_mock)
        settings = await service.get_user_settings(user_id)
        
        assert settings.llm.provider == LLMProvider.OPENAI
        assert settings.llm.primary_model == "gpt-4"
        assert settings.llm.openai_api_key == "sk-user-key"
        
        # Verify cache calls
        cache_mock.get_document.assert_any_call(
            collection=DB_COLLECTION_SETTINGS,
            document_id=user_doc_id
        )

    async def test_get_user_settings_fallback_to_platform(self):
        """Test fallback to platform settings when user settings document is missing."""
        cache_mock = MagicMock()
        cache_mock.get_document = AsyncMock()
        
        user_id = "user_456"
        user_doc_id = f"{USER_SETTINGS_DOC_PREFIX}{user_id}"
        
        # Mock platform document
        platform_data = PlatformSettingsData(
            llm_provider=LLMProvider.ANTHROPIC,
            llm_model="claude-3",
            anthropic_api_key="sk-platform-key"
        )
        platform_doc = PlatformSettingsDocument(
            settings=platform_data,
            created_at="2026-04-04T00:00:00Z",
            updated_at="2026-04-04T00:00:00Z"
        )
        
        # Return None for user doc, valid for platform doc
        def get_doc_mock(collection, document_id):
            if document_id == user_doc_id:
                return None
            if document_id == PLATFORM_SETTINGS_DOC:
                return platform_doc.model_dump()
            return None
            
        cache_mock.get_document.side_effect = get_doc_mock
        
        service = SettingsService(cache_aside_service=cache_mock)
        settings = await service.get_user_settings(user_id)
        
        assert settings.llm.provider == LLMProvider.ANTHROPIC
        assert settings.llm.primary_model == "claude-3"
        assert settings.llm.anthropic_api_key == "sk-platform-key"
        
        # Verify both lookups happened
        cache_mock.get_document.assert_any_call(
            collection=DB_COLLECTION_SETTINGS,
            document_id=user_doc_id
        )
        cache_mock.get_document.assert_any_call(
            collection=DB_COLLECTION_SETTINGS,
            document_id=PLATFORM_SETTINGS_DOC
        )

    async def test_llm_settings_no_overrides(self):
        """Test that llm_temperature and llm_max_tokens are None if not provided."""
        cache_mock = MagicMock()
        cache_mock.get_document = AsyncMock()
        
        # Mock platform document with NO temperature or max_tokens
        platform_data = PlatformSettingsData(
            llm_provider=LLMProvider.OLLAMA,
            llm_model="gemma3:27b",
            # llm_temperature and llm_max_tokens left as None
        )
        platform_doc = PlatformSettingsDocument(
            settings=platform_data,
            created_at="2026-04-04T00:00:00Z",
            updated_at="2026-04-04T00:00:00Z"
        )
        
        cache_mock.get_document.return_value = platform_doc.model_dump()
        
        service = SettingsService(cache_aside_service=cache_mock)
        settings = await service.get_platform_settings()
        
        # These MUST be None to allow agent-specific unique values
        assert settings.llm.llm_temperature is None
        assert settings.llm.llm_max_tokens is None

    async def test_llm_settings_with_overrides(self):
        """Test that llm_max_tokens ARE set if provided."""
        cache_mock = MagicMock()
        cache_mock.get_document = AsyncMock()
        
        # Mock platform document WITH max_tokens
        platform_data = PlatformSettingsData(
            llm_provider=LLMProvider.OLLAMA,
            llm_model="gemma3:27b",
            llm_max_tokens=2048
        )
        platform_doc = PlatformSettingsDocument(
            settings=platform_data,
            created_at="2026-04-04T00:00:00Z",
            updated_at="2026-04-04T00:00:00Z"
        )
        
        cache_mock.get_document.return_value = platform_doc.model_dump()
        
        service = SettingsService(cache_aside_service=cache_mock)
        settings = await service.get_platform_settings()
        
        assert settings.llm.llm_max_tokens == 2048

    async def test_command_gen_defaults_preserved_when_db_has_no_values(self):
        """Regression: llm_command_gen_passes=None caused TypeError in max(1, None).

        When the DB has no command_gen settings, LLMSettings defaults must survive.
        """
        cache_mock = MagicMock()
        cache_mock.get_document = AsyncMock()

        platform_data = PlatformSettingsData(
            llm_provider=LLMProvider.OLLAMA,
            llm_model="gemma3:27b",
        )
        platform_doc = PlatformSettingsDocument(
            settings=platform_data,
            created_at="2026-04-04T00:00:00Z",
            updated_at="2026-04-04T00:00:00Z",
        )
        cache_mock.get_document.return_value = platform_doc.model_dump()

        service = SettingsService(cache_aside_service=cache_mock)
        settings = await service.get_platform_settings()

        assert settings.llm.llm_command_gen_passes == 3
        assert settings.llm.llm_command_gen_enabled is True
        assert settings.llm.llm_command_gen_verifier is True

    async def test_command_gen_overrides_applied_when_db_has_values(self):
        """Explicit DB values for command_gen fields override the defaults."""
        cache_mock = MagicMock()
        cache_mock.get_document = AsyncMock()

        platform_data = PlatformSettingsData(
            llm_provider=LLMProvider.OLLAMA,
            llm_model="gemma3:27b",
            llm_command_gen_passes=5,
            llm_command_gen_enabled=False,
            llm_command_gen_verifier=False,
        )
        platform_doc = PlatformSettingsDocument(
            settings=platform_data,
            created_at="2026-04-04T00:00:00Z",
            updated_at="2026-04-04T00:00:00Z",
        )
        cache_mock.get_document.return_value = platform_doc.model_dump()

        service = SettingsService(cache_aside_service=cache_mock)
        settings = await service.get_platform_settings()

        assert settings.llm.llm_command_gen_passes == 5
        assert settings.llm.llm_command_gen_enabled is False
        assert settings.llm.llm_command_gen_verifier is False

    async def test_user_settings_command_gen_defaults_preserved(self):
        """Regression: user settings with no command_gen values must preserve defaults."""
        cache_mock = MagicMock()
        cache_mock.get_document = AsyncMock()

        user_id = "user_789"
        user_doc_id = f"{USER_SETTINGS_DOC_PREFIX}{user_id}"

        user_data = UserSettingsData(
            llm_provider=LLMProvider.GEMINI,
            llm_model="gemini-2.5-pro",
            gemini_api_key="test-key",
        )
        user_doc = UserSettingsDocument(
            settings=user_data,
            created_at="2026-04-04T00:00:00Z",
            updated_at="2026-04-04T00:00:00Z",
        )
        cache_mock.get_document.side_effect = lambda collection, document_id: (
            user_doc.model_dump() if document_id == user_doc_id else None
        )

        service = SettingsService(cache_aside_service=cache_mock)
        settings = await service.get_user_settings(user_id)

        assert settings.llm.llm_command_gen_passes == 3
        assert settings.llm.llm_command_gen_enabled is True
        assert settings.llm.llm_command_gen_verifier is True

    async def test_llm_settings_undefined_provider_skips(self):
        """Test that 'undefined' provider results in None in LLMSettings."""
        cache_mock = MagicMock()
        cache_mock.get_document = AsyncMock()
        
        # Mock platform document with 'undefined' provider
        platform_data = PlatformSettingsData(
            llm_provider="undefined",
            llm_model="gemma3:27b"
        )
        platform_doc = PlatformSettingsDocument(
            settings=platform_data,
            created_at="2026-04-04T00:00:00Z",
            updated_at="2026-04-04T00:00:00Z"
        )
        
        cache_mock.get_document.return_value = platform_doc.model_dump()
        
        service = SettingsService(cache_aside_service=cache_mock)
        settings = await service.get_platform_settings()
        
        # Should NOT be set if provider is 'undefined'
        assert settings.llm.provider == LLMProvider.OLLAMA # Default from G8eePlatformSettings
