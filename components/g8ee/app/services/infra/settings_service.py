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

from __future__ import annotations

import logging
from typing import TYPE_CHECKING, Protocol, runtime_checkable

from app.constants import (
    ErrorCode,
)
from app.constants.collections import (
    DB_COLLECTION_SETTINGS,
    PLATFORM_SETTINGS_DOC,
    USER_SETTINGS_DOC_PREFIX,
)
from app.constants.settings import (
    LLMProvider,
)
from app.errors import ConfigurationError
from app.models.settings import (
    LLMSettings,
    G8eePlatformSettings,
    G8eeUserSettings,
    PlatformSettingsData,
    UserSettingsData,
    PlatformSettingsDocument,
    UserSettingsDocument,
    SearchSettings,
)

from app.services.infra.bootstrap_service import BootstrapService, BootstrapServiceProtocol

if TYPE_CHECKING:
    from app.services.cache.cache_aside import CacheAsideService


@runtime_checkable
class SettingsServiceProtocol(Protocol):
    """Protocol for SettingsService ensuring read-only access to platform and user settings."""

    async def get_platform_settings(self) -> G8eePlatformSettings:
        """Retrieve platform-level settings from g8es with cache-aside."""
        ...

    async def get_user_settings(self, user_id: str) -> G8eeUserSettings:
        """Retrieve settings for a specific user, overlaid on platform settings."""
        ...

    def get_local_settings(self) -> G8eePlatformSettings:
        """Retrieve local bootstrap settings (bootstrap)."""
        ...

    def get_bootstrap_service(self) -> BootstrapServiceProtocol:
        """Get the bootstrap service dependency."""
        ...


class SettingsService:
    """Service for managing g8ee settings with bootstrap loading and cache-aside logic."""

    def __init__(self, cache_aside_service: CacheAsideService | None = None, bootstrap_service: BootstrapService | None = None) -> None:
        self._cache_aside = cache_aside_service
        self._bootstrap = bootstrap_service or BootstrapService()
        self._logger = logging.getLogger(__name__)

    def _get_env(self, env_key: str, default: str | None = None) -> str | None:
        """DEPRECATED: Configuration is now loaded from g8es or bootstrap volumes.
        """
        return default

    def get_local_settings(self) -> G8eePlatformSettings:
        """Load settings using canonical defaults and bootstrap service.
        
        This replaces legacy configuration with platform defaults and
        secure bootstrap service for secrets from g8es volume.
        """
        settings = G8eePlatformSettings(port=443)  # type: ignore[arg-type]
        
        # Load secrets from bootstrap service
        internal_token = self._bootstrap.load_internal_auth_token()
        if internal_token:
            settings.auth.internal_auth_token = internal_token
        else:
            self._logger.info("Internal auth token not available from bootstrap service")
                
        session_key = self._bootstrap.load_session_encryption_key()
        if session_key:
            settings.auth.session_encryption_key = session_key
        else:
            self._logger.info("Session encryption key not available from bootstrap service")

        return settings

    def overlay_platform_data(self, settings: G8eePlatformSettings, data: PlatformSettingsData) -> G8eePlatformSettings:
        """Overlay PlatformSettingsData onto a G8eePlatformSettings instance."""
        
        # Command Validation
        if data.enable_command_whitelisting is not None:
            settings.command_validation.enable_whitelisting = bool(data.enable_command_whitelisting)
        if data.enable_command_blacklisting is not None:
            settings.command_validation.enable_blacklisting = bool(data.enable_command_blacklisting)

        # Search (Merged Vertex/Google)
        settings.search = self._build_search_settings(data)

        # Auth
        if data.internal_auth_token and not settings.auth.internal_auth_token:
            settings.auth.internal_auth_token = data.internal_auth_token
        if data.session_encryption_key and not settings.auth.session_encryption_key:
            settings.auth.session_encryption_key = data.session_encryption_key
        if data.g8e_api_key:
            settings.auth.g8e_api_key = data.g8e_api_key
            
        # Do NOT provide platform-level defaults for temperature/max_tokens if not explicitly set.
        # Our agents use unique temperatures and we should not override them with a platform default
        # unless the user has explicitly configured one in g8es.
        
        return settings

    def _build_llm_settings(self, data: UserSettingsData | PlatformSettingsData) -> LLMSettings:
        """Build LLMSettings from flat data without fallbacks.
        
        Accepts both UserSettingsData and PlatformSettingsData:
        - UserSettingsData: normal path for per-user settings
        - PlatformSettingsData: fallback path in get_user_settings() when user doc is missing
        The PlatformSettingsData branch handles endpoint fields which are platform-level only.
        """
        kwargs = {}

        if data.llm_provider:
            try:
                kwargs["provider"] = LLMProvider(data.llm_provider)
            except ValueError:
                self._logger.warning(f"Invalid LLM provider in settings: {data.llm_provider}. Falling back to default.")

        llm = LLMSettings(**kwargs)
        
        # Models
        llm.primary_model = data.llm_model
        llm.assistant_model = data.llm_assistant_model

        # Keys
        llm.openai_api_key = data.openai_api_key
        llm.ollama_api_key = data.ollama_api_key
        llm.gemini_api_key = data.gemini_api_key
        llm.anthropic_api_key = data.anthropic_api_key

        # Endpoints (Platform only)
        if isinstance(data, PlatformSettingsData):
            llm.openai_endpoint = data.openai_endpoint
            llm.ollama_endpoint = data.ollama_endpoint
            llm.anthropic_endpoint = data.anthropic_endpoint

        # Shared - ONLY set if present in data, otherwise remain None to allow agent-specific defaults
        if data.llm_temperature is not None:
            llm.llm_temperature = data.llm_temperature
        if data.llm_max_tokens is not None:
            llm.llm_max_tokens = data.llm_max_tokens
        
        if data.llm_command_gen_enabled is not None:
            llm.llm_command_gen_enabled = bool(data.llm_command_gen_enabled)
        if data.llm_command_gen_verifier is not None:
            llm.llm_command_gen_verifier = bool(data.llm_command_gen_verifier)
        if data.llm_command_gen_passes is not None:
            llm.llm_command_gen_passes = data.llm_command_gen_passes
        if data.llm_command_gen_temp is not None:
            llm.llm_command_gen_temp = data.llm_command_gen_temp
        
        return llm

    async def get_platform_settings(self) -> G8eePlatformSettings:
        """Load platform settings from g8es via CacheAsideService."""
        if not self._cache_aside:
            return self.get_local_settings()

        doc_dict = await self._cache_aside.get_document(
            collection=DB_COLLECTION_SETTINGS,
            document_id=PLATFORM_SETTINGS_DOC,
        )

        if not doc_dict:
            raise ConfigurationError(
                "g8ee cannot start: platform_settings document missing in g8es",
                code=ErrorCode.DB_QUERY_ERROR
            )

        doc = PlatformSettingsDocument.model_validate(doc_dict)
        
        settings = self.get_local_settings()
        return self.overlay_platform_data(settings, doc.settings)

    async def get_user_settings(self, user_id: str) -> G8eeUserSettings:
        """Load per-request settings for a specific user."""
        if not self._cache_aside:
             raise ConfigurationError("CacheAsideService required for user settings")

        user_doc_id = f"{USER_SETTINGS_DOC_PREFIX}{user_id}"
        user_doc_dict = await self._cache_aside.get_document(
            collection=DB_COLLECTION_SETTINGS,
            document_id=user_doc_id,
        )

        if not user_doc_dict:
            # Fallback to platform LLM settings if user settings missing? 
            # User request said explicit G8eeUserSettings, so we must return one.
            # We'll need platform data to populate the LLM settings if user hasn't set them.
            platform_doc_dict = await self._cache_aside.get_document(
                collection=DB_COLLECTION_SETTINGS,
                document_id=PLATFORM_SETTINGS_DOC,
            )
            platform_doc = PlatformSettingsDocument.model_validate(platform_doc_dict)
            data = platform_doc.settings
        else:
            user_doc = UserSettingsDocument.model_validate(user_doc_dict)
            data = user_doc.settings

        return G8eeUserSettings(
            llm=self._build_llm_settings(data),
            search=self._build_search_settings(data)
        )

    def _build_search_settings(self, data: UserSettingsData | PlatformSettingsData) -> SearchSettings:
        """Build SearchSettings from flat data."""
        return SearchSettings(
            enabled=bool(data.vertex_search_enabled or data.google_search_enabled),
            project_id=data.vertex_search_project_id,
            engine_id=data.vertex_search_engine_id or data.google_search_engine_id,
            location=data.vertex_search_location or "global",
            api_key=data.vertex_search_api_key or data.google_search_api_key
        )

    def get_bootstrap_service(self) -> BootstrapServiceProtocol:
        """Get the bootstrap service dependency."""
        return self._bootstrap
