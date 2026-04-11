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
from unittest.mock import AsyncMock, patch
from app.services.infra.settings_service import SettingsService
from app.constants.collections import (
    PLATFORM_SETTINGS_DOC,
    USER_SETTINGS_DOC_PREFIX,
    DB_COLLECTION_SETTINGS,
)
from app.models.settings import VSEPlatformSettings, PlatformSettingsData, PlatformSettingsDocument, UserSettingsData, UserSettingsDocument
from app.constants.settings import GEMINI_3_1_PRO_PREVIEW

@pytest.mark.asyncio
class TestVSESettingsOverlayIntegration:
    """Deep integration tests for VSE settings loading and overlay logic."""

    @pytest.fixture
    def cache_service(self):
        return AsyncMock()

    @pytest.fixture
    def settings_service(self, cache_service):
        return SettingsService(cache_aside_service=cache_service)

    async def test_get_platform_settings_loads_from_vsodb(self, settings_service, cache_service):
        """Verify platform settings are loaded from the correct VSODB collection/ID."""
        platform_data = {
            "settings": {
                "llm_provider": "gemini",
                "llm_model": GEMINI_3_1_PRO_PREVIEW,
                "gemini_api_key": "platform-key"
            },
            "created_at": "2026-01-01T00:00:00Z",
            "updated_at": "2026-01-01T00:00:00Z"
        }
        cache_service.get_document.return_value = platform_data

        settings = await settings_service.get_platform_settings()
        
        assert settings.llm.provider == "gemini"
        assert settings.llm.primary_model == GEMINI_3_1_PRO_PREVIEW
        assert settings.llm.gemini_api_key == "platform-key"
        
        cache_service.get_document.assert_called_once_with(
            collection=DB_COLLECTION_SETTINGS,
            document_id=PLATFORM_SETTINGS_DOC
        )

    async def test_get_user_settings_overlays_user_data(self, settings_service, cache_service):
        """Verify user settings overlay platform defaults correctly."""
        user_id = "test-user-123"
        user_doc_id = f"{USER_SETTINGS_DOC_PREFIX}{user_id}"
        
        platform_data = {
            "settings": {
                "llm_provider": "gemini",
                "llm_model": GEMINI_3_1_PRO_PREVIEW
            },
            "created_at": "2026-01-01T00:00:00Z",
            "updated_at": "2026-01-01T00:00:00Z"
        }
        
        user_data = {
            "settings": {
                "llm_provider": "openai",
                "llm_model": "gpt-4o",
                "openai_api_key": "user-key"
            },
            "created_at": "2026-01-01T00:00:00Z",
            "updated_at": "2026-01-01T00:00:00Z"
        }

        def get_doc_mock(collection, document_id):
            if document_id == user_doc_id:
                return user_data
            if document_id == PLATFORM_SETTINGS_DOC:
                return platform_data
            return None

        cache_service.get_document.side_effect = get_doc_mock

        # VSE SettingsService.get_user_settings currently only returns the UserSettings part
        # overlaid on schema defaults, but we want to ensure it uses the user document if present.
        user_settings = await settings_service.get_user_settings(user_id)
        
        assert user_settings.llm.provider == "openai"
        assert user_settings.llm.primary_model == "gpt-4o"
        assert user_settings.llm.openai_api_key == "user-key"

    async def test_get_user_settings_falls_back_to_platform_when_missing(self, settings_service, cache_service):
        """Verify user settings fall back to platform defaults when UserSettingsDocument is missing."""
        user_id = "new-user"
        user_doc_id = f"{USER_SETTINGS_DOC_PREFIX}{user_id}"
        
        platform_data = {
            "settings": {
                "llm_provider": "gemini",
                "llm_model": GEMINI_3_1_PRO_PREVIEW,
                "gemini_api_key": "platform-key"
            },
            "created_at": "2026-01-01T00:00:00Z",
            "updated_at": "2026-01-01T00:00:00Z"
        }

        def get_doc_mock(collection, document_id):
            if document_id == user_doc_id:
                return None
            if document_id == PLATFORM_SETTINGS_DOC:
                return platform_data
            return None

        cache_service.get_document.side_effect = get_doc_mock

        user_settings = await settings_service.get_user_settings(user_id)
        
        assert user_settings.llm.provider == "gemini"
        assert user_settings.llm.primary_model == GEMINI_3_1_PRO_PREVIEW
        assert user_settings.llm.gemini_api_key == "platform-key"
