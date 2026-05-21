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
    LogLevel,
)
from app.constants.collections import (
    DB_COLLECTION_SETTINGS,
    PLATFORM_SETTINGS_DOC,
    USER_SETTINGS_DOC_PREFIX,
)
from app.constants.env_vars import EnvVar
from app.constants.generated_paths import PathConstants, PortConstants
from app.constants.paths import PATHS
from app.errors import ConfigurationError
from app.models.settings import (
    AuthSettings,
    LLMSettings,
    G8eePlatformSettings,
    G8eeUserSettings,
    PlatformSettingsDocument,
    UserSettingsDocument,
    SearchSettings,
)
from app.models.base import G8eBaseModel

from app.services.infra.bootstrap_service import BootstrapService, BootstrapServiceProtocol

if TYPE_CHECKING:
    from app.services.cache.cache_aside import CacheAsideService
    from app.models.internal_api import RequestOverrides


@runtime_checkable
class SettingsServiceProtocol(Protocol):
    """Protocol for SettingsService ensuring read-only access to platform and user settings."""

    async def get_platform_settings(self) -> G8eePlatformSettings:
        """Retrieve platform-level settings from operator with cache-aside."""
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


    def get_local_settings(self) -> G8eePlatformSettings:
        """Load settings using canonical defaults plus secrets sourced from the
        bootstrap service (operator volume)."""
        settings = G8eePlatformSettings(
            host="0.0.0.0",
            port=PortConstants.G8E_PORT_G8EE_HTTPS,
            log_level=LogLevel.INFO,
            enable_logging=True,
            docker_gid="988",
            session_ttl=3600,
            absolute_session_timeout=86400,
            docs_dir=PathConstants.PATH_DOCS_DIR,
            app_url=f"http://{PATHS.get('host', 'localhost')}:{PortConstants.G8E_PORT_G8EE_HTTPS}",
            allowed_origins="*",
            passkey_rp_name="g8e",
            passkey_rp_id="g8e",
            passkey_origin=f"http://{PATHS.get('host', 'localhost')}:{PortConstants.G8E_PORT_G8EE_HTTPS}",
        )

        # Load secrets from bootstrap service
        session_key = self._bootstrap.load_session_encryption_key()
        if session_key:
            self._bootstrap.verify_against_manifest("session_encryption_key", session_key)
            settings.auth.session_encryption_key = session_key
        else:
            self._logger.info("Session encryption key not available from bootstrap service")

        auditor_hmac_key = self._bootstrap.load_auditor_hmac_key()
        if auditor_hmac_key:
            self._bootstrap.verify_against_manifest("auditor_hmac_key", auditor_hmac_key)
            settings.auth.auditor_hmac_key = auditor_hmac_key
        else:
            self._logger.info("Auditor HMAC key not available from bootstrap service")

        return settings

    def overlay_platform_data(self, settings: G8eePlatformSettings, platform_settings: G8eePlatformSettings) -> G8eePlatformSettings:
        """Overlay platform DB settings onto local bootstrap settings.

        Model-driven by design: each nested settings model is overlaid as a
        whole object, except for AuthSettings which merges. This ensures
        any new fields or nested models added to G8eePlatformSettings
        automatically flow through without manual code updates here.

        Auth is the only sub-model that merges instead of being replaced,
        because bootstrap-loaded secrets (verified against the on-disk
        SecretManager digest) must take precedence over whatever the
        platform document carries; the DB only fills gaps when the
        bootstrap volume hasn't surfaced a value yet.
        """
        for field_name in type(settings).model_fields:
            if field_name.startswith("_"):
                continue

            local_value = getattr(settings, field_name)
            platform_value = getattr(platform_settings, field_name)

            # We only overlay nested models that are G8eBaseModels.
            if not isinstance(local_value, G8eBaseModel):
                continue

            if isinstance(local_value, AuthSettings):
                # Auth: bootstrap value wins when present; platform DB fills gaps.
                for auth_field in type(local_value).model_fields:
                    p_val = getattr(platform_value, auth_field, None)
                    if p_val and not getattr(local_value, auth_field, None):
                        setattr(local_value, auth_field, p_val)
            else:
                # Whole-object overlay for all other nested models.
                setattr(settings, field_name, platform_value)

        return settings

    def _build_llm_settings(self, user_settings: G8eeUserSettings) -> LLMSettings:
        """Build LLMSettings from G8eeUserSettings.

        LLM provider configuration is user-specific only.
        """
        return user_settings.llm

    async def get_platform_settings(self) -> G8eePlatformSettings:
        """Load platform settings from operator via CacheAsideService."""
        if not self._cache_aside:
            return self.get_local_settings()

        doc_dict = await self._cache_aside.get_document_with_cache(
            collection=DB_COLLECTION_SETTINGS,
            document_id=PLATFORM_SETTINGS_DOC,
        )

        if not doc_dict:
            raise ConfigurationError(
                "g8ee cannot start: platform_settings document missing in operator",
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
        user_doc_dict = await self._cache_aside.get_document_with_cache(
            collection=DB_COLLECTION_SETTINGS,
            document_id=user_doc_id,
        )

        if not user_doc_dict:
            self._logger.info(
                "No user settings document for user %s; using empty defaults so request overrides can complete validation",
                user_id,
            )
            platform_settings = await self.get_platform_settings()
            return G8eeUserSettings(
                llm=platform_settings.llm,
                search=platform_settings.search,
                eval_judge=platform_settings.eval_judge,
                command_validation=platform_settings.command_validation,
                batch_execution=platform_settings.batch_execution,
            )

        user_doc = UserSettingsDocument.model_validate(user_doc_dict)
        data = user_doc.settings

        return G8eeUserSettings(
            llm=self._build_llm_settings(data),
            search=self._build_search_settings(data),
            eval_judge=data.eval_judge,
            command_validation=data.command_validation,
            batch_execution=data.batch_execution,
        )

    async def update_user_settings(self, user_id: str, new_settings: G8eeUserSettings) -> None:
        """Update user settings in the database and invalidate the local cache."""
        if not self._cache_aside:
             raise ConfigurationError("CacheAsideService required for writing settings")
             
        user_doc_id = f"{USER_SETTINGS_DOC_PREFIX}{user_id}"
        
        doc = UserSettingsDocument(
            id=user_doc_id,
            user_id=user_id,
            settings=new_settings
        )
        
        await self._cache_aside.update_document(
            collection=DB_COLLECTION_SETTINGS,
            document_id=user_doc_id,
            data=doc.model_dump(mode="json"),
            merge=False
        )

    async def sync_settings_overrides(
        self,
        user_id: str,
        user_settings: G8eeUserSettings,
        overrides: RequestOverrides
    ) -> bool:
        """Extract LLM and Search overrides from a request and update user settings if needed."""
        # Check if any overrides are provided
        llm_overrides = any([
            overrides.llm_primary_model,
            overrides.llm_assistant_model,
            overrides.llm_lite_model,
            overrides.llm_primary_provider,
            overrides.llm_assistant_provider,
            overrides.llm_lite_provider,
            overrides.llm_primary_api_key,
            overrides.llm_primary_endpoint,
            overrides.llm_assistant_api_key,
            overrides.llm_assistant_endpoint,
            overrides.llm_lite_api_key,
            overrides.llm_lite_endpoint,
        ])
        
        search_overrides = any([
            overrides.web_search_project,
            overrides.web_search_app,
            overrides.web_search_api_key,
        ])
        
        if not llm_overrides and not search_overrides:
            return False

        self._logger.info("[SettingsService] Storing request config overrides into user settings for user %s", user_id)
        
        # Update LLM settings
        if overrides.llm_primary_model:
            user_settings.llm.primary_model = overrides.llm_primary_model
        if overrides.llm_assistant_model:
            user_settings.llm.assistant_model = overrides.llm_assistant_model
        if overrides.llm_lite_model:
            user_settings.llm.lite_model = overrides.llm_lite_model
            
        if overrides.llm_primary_provider:
            user_settings.llm.primary_provider = overrides.llm_primary_provider
        if overrides.llm_assistant_provider:
            user_settings.llm.assistant_provider = overrides.llm_assistant_provider
        if overrides.llm_lite_provider:
            user_settings.llm.lite_provider = overrides.llm_lite_provider
            
        # Provider-specific keys/endpoints
        for role, key, endpoint, prov in [
            ("primary", overrides.llm_primary_api_key, overrides.llm_primary_endpoint, overrides.llm_primary_provider or user_settings.llm.primary_provider),
            ("assistant", overrides.llm_assistant_api_key, overrides.llm_assistant_endpoint, overrides.llm_assistant_provider or user_settings.llm.assistant_provider),
            ("lite", overrides.llm_lite_api_key, overrides.llm_lite_endpoint, overrides.llm_lite_provider or user_settings.llm.lite_provider)
        ]:
            if not prov:
                continue
            
            # Using strings for provider comparison as they might be strings or enums
            p_str = prov.value if hasattr(prov, 'value') else str(prov)
            
            if p_str == "openai":
                if key: user_settings.llm.openai_api_key = key
                if endpoint: user_settings.llm.openai_endpoint = endpoint
            elif p_str == "anthropic":
                if key: user_settings.llm.anthropic_api_key = key
                if endpoint: user_settings.llm.anthropic_endpoint = endpoint
            elif p_str == "gemini":
                if key: user_settings.llm.gemini_api_key = key
            elif p_str == "ollama":
                if key: user_settings.llm.ollama_api_key = key
                if endpoint: user_settings.llm.ollama_endpoint = endpoint
            elif p_str == "llamacpp":
                if key: user_settings.llm.llamacpp_api_key = key
                if endpoint: user_settings.llm.llamacpp_endpoint = endpoint

        # Update Search settings
        if search_overrides:
            if overrides.web_search_project:
                user_settings.search.project_id = overrides.web_search_project
            if overrides.web_search_app:
                user_settings.search.engine_id = overrides.web_search_app
            if overrides.web_search_api_key:
                user_settings.search.api_key = overrides.web_search_api_key
            # If any search overrides provided, ensure search is enabled in user settings
            user_settings.search.enabled = True

        await self.update_user_settings(user_id, user_settings)
        return True

    def _build_search_settings(self, settings: G8eePlatformSettings | G8eeUserSettings) -> SearchSettings:
        """Build SearchSettings from platform or user settings."""
        return settings.search

    def get_bootstrap_service(self) -> BootstrapServiceProtocol:
        """Get the bootstrap service dependency."""
        return self._bootstrap

