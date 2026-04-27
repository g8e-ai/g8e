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
    OPENAI_DEFAULT_ENDPOINT,
    OLLAMA_DEFAULT_ENDPOINT,
    ANTHROPIC_DEFAULT_ENDPOINT,
    LogLevel,
)
from app.constants.collections import (
    DB_COLLECTION_SETTINGS,
    PLATFORM_SETTINGS_DOC,
    USER_SETTINGS_DOC_PREFIX,
)
from app.errors import ConfigurationError
from app.models.settings import (
    LLMSettings,
    G8eePlatformSettings,
    G8eeUserSettings,
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

    async def update_g8ep_operator_api_key(self, api_key: str) -> None:
        """Update the g8ep operator API key in platform settings."""
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
        settings = G8eePlatformSettings(
            host="0.0.0.0",
            port=443,
            log_level=LogLevel.INFO,
            enable_logging=True,
            docker_gid="988",
            session_ttl=3600,
            absolute_session_timeout=86400,
            docs_dir="/docs",
            supervisor_port=9001,
            app_url="http://localhost:443",
            allowed_origins="*",
            passkey_rp_name="g8e",
            passkey_rp_id="g8e",
            passkey_origin="http://localhost:443",
        )
        
        # Load secrets from bootstrap service
        internal_token = self._bootstrap.load_internal_auth_token()
        if internal_token:
            # Tamper-evidence: confirm the on-disk secret matches the SHA-256
            # digest g8eo SecretManager recorded in bootstrap_digest.json at
            # write time. Divergence (partial write, corruption, manual edit)
            # must abort startup cleanly rather than let g8ee authenticate
            # with a drifted secret and surface an opaque 401 later.
            self._bootstrap.verify_against_manifest("internal_auth_token", internal_token)
            settings.auth.internal_auth_token = internal_token
        else:
            self._logger.info("Internal auth token not available from bootstrap service")

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
        whole object, and the auth merge iterates ``AuthSettings`` fields
        rather than hand-listing them. Adding a new field on any of these
        nested models therefore flows through automatically — hand-listing
        fields here is the bug class that previously dropped new fields
        (e.g. command_validation auto-approve, auth auditor_hmac_key) on
        the platform-bootstrap path.

        Auth is the only sub-model that merges instead of being replaced,
        because bootstrap-loaded secrets (verified against the on-disk
        SecretManager digest) must take precedence over whatever the
        platform document carries; the DB only fills gaps when the
        bootstrap volume hasn't surfaced a value yet.
        """
        # Whole-object overlay for nested models where the platform DB
        # document is authoritative. Any new field added inside these
        # models flows through with no change here.
        settings.command_validation = platform_settings.command_validation
        settings.search = platform_settings.search
        settings.reputation = platform_settings.reputation

        # Auth: bootstrap value wins when present; platform DB fills gaps.
        # Iterating AuthSettings.model_fields makes this structural — newly
        # added auth tokens (e.g. future signing keys) overlay automatically
        # without revisiting this method.
        for field_name in type(settings.auth).model_fields:
            platform_value = getattr(platform_settings.auth, field_name, None)
            if not platform_value:
                continue
            if not getattr(settings.auth, field_name, None):
                setattr(settings.auth, field_name, platform_value)

        return settings

    def _build_llm_settings(self, user_settings: G8eeUserSettings) -> LLMSettings:
        """Build LLMSettings from G8eeUserSettings.
        
        LLM provider configuration is user-specific only.
        """
        return user_settings.llm

    async def get_platform_settings(self) -> G8eePlatformSettings:
        """Load platform settings from g8es via CacheAsideService."""
        if not self._cache_aside:
            return self.get_local_settings()

        doc_dict = await self._cache_aside.get_document_with_cache(
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
        user_doc_dict = await self._cache_aside.get_document_with_cache(
            collection=DB_COLLECTION_SETTINGS,
            document_id=user_doc_id,
        )

        if not user_doc_dict:
            # LLM settings are user-specific only. Return empty LLMSettings if user doc missing.
            # Search settings can fall back to platform defaults.
            platform_doc_dict = await self._cache_aside.get_document_with_cache(
                collection=DB_COLLECTION_SETTINGS,
                document_id=PLATFORM_SETTINGS_DOC,
            )
            platform_doc = PlatformSettingsDocument.model_validate(platform_doc_dict)
            
            return G8eeUserSettings(
                llm=LLMSettings(),
                search=self._build_search_settings(platform_doc.settings),
                eval_judge=platform_doc.settings.eval_judge,
                command_validation=platform_doc.settings.command_validation,
            )

        user_doc = UserSettingsDocument.model_validate(user_doc_dict)
        data = user_doc.settings

        return G8eeUserSettings(
            llm=self._build_llm_settings(data),
            search=self._build_search_settings(data),
            eval_judge=data.eval_judge,
            command_validation=data.command_validation,
        )

    def _build_search_settings(self, settings: G8eePlatformSettings | G8eeUserSettings) -> SearchSettings:
        """Build SearchSettings from platform or user settings."""
        return settings.search

    def get_bootstrap_service(self) -> BootstrapServiceProtocol:
        """Get the bootstrap service dependency."""
        return self._bootstrap

    async def update_g8ep_operator_api_key(self, api_key: str) -> None:
        """Update the g8ep operator API key in platform settings.

        This persists the API key to the platform_settings document so g8ep's
        fetch-key-and-run.sh can retrieve it. Authority: g8ee.
        """
        if not self._cache_aside:
            raise ConfigurationError("CacheAsideService required for updating platform settings")

        await self._cache_aside.update_document(
            collection=DB_COLLECTION_SETTINGS,
            document_id=PLATFORM_SETTINGS_DOC,
            update_data={"settings": {"g8ep_operator_api_key": api_key}}
        )

        self._logger.info(
            "g8ep operator API key updated in platform settings",
            extra={"api_key_prefix": api_key[:8] + "..."}
        )
