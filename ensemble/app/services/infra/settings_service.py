# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

from __future__ import annotations

import logging
import os
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
from app.models.base import G8eBaseModel
from app.models.internal_api import LLMRoleSettingsResponse, LLMRoleSettingsUpdateRequest
from app.models.settings import (
    AppSettingsDocument,
    AuthSettings,
    G8eeAppSettings,
    G8eeUserSettings,
    LLMSettings,
    SearchSettings,
    UserSettingsDocument,
)
from app.services.infra.llm_role_settings import (
    apply_provider_updates,
    apply_role_updates,
    settings_view,
)

if TYPE_CHECKING:
    from app.services.cache.cache_aside import CacheAsideService


@runtime_checkable
class SettingsServiceProtocol(Protocol):
    """Protocol for SettingsService ensuring read-only access to platform and user settings."""

    async def get_app_settings(self) -> G8eeAppSettings:
        """Retrieve platform-level settings from operator with cache-aside."""
        ...

    async def get_user_settings(self, user_id: str) -> G8eeUserSettings:
        """Retrieve settings for a specific user, overlaid on platform settings."""
        ...

    def get_local_settings(self) -> G8eeAppSettings:
        """Retrieve local bootstrap settings (bootstrap)."""
        ...


class SettingsService:
    """Service for managing g8ee settings with bootstrap loading and cache-aside logic."""

    def __init__(
        self,
        cache_aside_service: CacheAsideService | None = None,
    ) -> None:
        self._cache_aside = cache_aside_service
        self._logger = logging.getLogger(__name__)

    def attach_cache_aside(self, cache_aside_service: CacheAsideService) -> None:
        """Attach the cache-aside service once it is constructed after bootstrap."""
        self._cache_aside = cache_aside_service

    def get_local_settings(self) -> G8eeAppSettings:
        """Load canonical defaults and local LLM credentials."""
        settings = G8eeAppSettings(
            host="0.0.0.0",
            port=PortConstants.G8E_PORT_G8EE_HTTPS,
            log_level=LogLevel.INFO,
            enable_logging=True,
            session_ttl=3600,
            absolute_session_timeout=86400,
            docs_dir=PathConstants.PATH_DOCS_DIR,
            app_url=f"http://{PATHS.get('host', 'localhost')}:{PortConstants.G8E_PORT_G8EE_HTTPS}",
            allowed_origins="*",
            passkey_rp_name="g8e",
            passkey_rp_id="g8e",
            passkey_origin=f"http://{PATHS.get('host', 'localhost')}:{PortConstants.G8E_PORT_G8EE_HTTPS}",
        )

        # Apply LLM credential and endpoint bootstrap defaults (lowest
        # priority). Only secrets (API keys) and user-specific endpoints come
        # from the environment (INV-ENV-04); provider and model selection are
        # caller/platform settings only. Provider connection priority here is:
        # platform DB settings > env-var bootstrap defaults.
        self._apply_llm_env_defaults(settings.llm)

        return settings

    def _apply_llm_env_defaults(self, llm: LLMSettings) -> None:
        """Populate LLMSettings API keys and endpoints from environment variables.

        This is the lowest-priority bootstrap source. Each field is set only
        when the env var is present and non-empty; unset env vars leave the
        field at its model default (None). The platform DB overlay
        (overlay_platform_data) takes precedence over these values. Provider
        and model role selections are never read
        from the environment.
        """
        env = os.environ.get

        # Provider-specific endpoint/api-key defaults
        if env(EnvVar.LLM_OPENAI_API_KEY):
            llm.openai_api_key = env(EnvVar.LLM_OPENAI_API_KEY)
        if env(EnvVar.LLM_OPENAI_ENDPOINT):
            llm.openai_endpoint = env(EnvVar.LLM_OPENAI_ENDPOINT)
        if env(EnvVar.LLM_OLLAMA_API_KEY):
            llm.ollama_api_key = env(EnvVar.LLM_OLLAMA_API_KEY)
        if env(EnvVar.LLM_OLLAMA_ENDPOINT):
            llm.ollama_endpoint = env(EnvVar.LLM_OLLAMA_ENDPOINT)
        if env(EnvVar.LLM_ANTHROPIC_API_KEY):
            llm.anthropic_api_key = env(EnvVar.LLM_ANTHROPIC_API_KEY)
        if env(EnvVar.LLM_ANTHROPIC_ENDPOINT):
            llm.anthropic_endpoint = env(EnvVar.LLM_ANTHROPIC_ENDPOINT)
        if env(EnvVar.LLM_GEMINI_API_KEY):
            llm.gemini_api_key = env(EnvVar.LLM_GEMINI_API_KEY)
        if env(EnvVar.LLM_LLAMACPP_API_KEY):
            llm.llamacpp_api_key = env(EnvVar.LLM_LLAMACPP_API_KEY)
        if env(EnvVar.LLM_LLAMACPP_ENDPOINT):
            llm.llamacpp_endpoint = env(EnvVar.LLM_LLAMACPP_ENDPOINT)
        if env(EnvVar.LLM_JEV_MODEL):
            llm.jev_model = env(EnvVar.LLM_JEV_MODEL)

    def overlay_platform_data(
        self, settings: G8eeAppSettings, app_settings: G8eeAppSettings
    ) -> G8eeAppSettings:
        """Overlay platform DB settings onto local bootstrap settings.

        Model-driven by design: each nested settings model is overlaid as a
        whole object, except for AuthSettings and LLMSettings which merge.
        This ensures any new fields or nested models added to G8eeAppSettings
        automatically flow through without manual code updates here.

        Auth merges with bootstrap-wins semantics: bootstrap-loaded secrets
        (verified against the on-disk SecretManager digest) take precedence
        over whatever the platform document carries; the DB only fills gaps
        when the bootstrap volume hasn't surfaced a value yet.

        LLM merges with platform-DB-wins semantics: platform DB values take
        precedence when present, and env-var bootstrap defaults (lowest
        priority, already applied in get_local_settings) fill gaps the
        platform DB leaves unset. Priority order: platform DB settings >
        env-var defaults.
        """
        for field_name in type(settings).model_fields:
            if field_name.startswith("_"):
                continue

            local_value = getattr(settings, field_name)
            platform_value = getattr(app_settings, field_name)

            # We only overlay nested models that are G8eBaseModels.
            if not isinstance(local_value, G8eBaseModel):
                continue

            if isinstance(local_value, AuthSettings):
                # Auth: bootstrap value wins when present; platform DB fills gaps.
                for sub_field in type(local_value).model_fields:
                    p_val = getattr(platform_value, sub_field, None)
                    if p_val and not getattr(local_value, sub_field, None):
                        setattr(local_value, sub_field, p_val)
            elif isinstance(local_value, LLMSettings):
                # LLM: platform DB wins when explicitly set; env-var defaults
                # (already applied in get_local_settings) fill gaps. LLMSettings
                # has non-None defaults for endpoint fields (e.g.
                # ollama_endpoint defaults to OLLAMA_DEFAULT_ENDPOINT), so a
                # platform DB document that omits LLM still carries those
                # defaults. Only override the local/env value when the
                # platform value differs from the model default — a platform
                # value equal to the default means the DB didn't set it.
                # Priority: platform DB > env-var defaults.
                llm_defaults = LLMSettings()
                for sub_field in type(local_value).model_fields:
                    p_val = getattr(platform_value, sub_field, None)
                    default_val = getattr(llm_defaults, sub_field, None)
                    if p_val is not None and p_val != default_val:
                        setattr(local_value, sub_field, p_val)
            else:
                # Whole-object overlay for all other nested models.
                setattr(settings, field_name, platform_value)

        return settings

    def _build_llm_settings(self, user_settings: G8eeUserSettings) -> LLMSettings:
        """Build LLMSettings from G8eeUserSettings.

        LLM provider configuration is user-specific only.
        """
        return user_settings.llm

    async def get_app_settings(self) -> G8eeAppSettings:
        """Load platform settings from operator via CacheAsideService."""
        if not self._cache_aside:
            return self.get_local_settings()

        doc_dict = await self._cache_aside.get_document_with_cache(
            collection=DB_COLLECTION_SETTINGS,
            document_id=PLATFORM_SETTINGS_DOC,
        )

        if not doc_dict:
            raise ConfigurationError(
                "g8ee cannot start: app_settings document missing in operator",
                code=ErrorCode.DB_QUERY_ERROR,
            )

        doc = AppSettingsDocument.model_validate(doc_dict)

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
                "No user settings document for user %s; using governed inference defaults with caller-selected models",
                user_id,
            )
            app_settings = await self.get_app_settings()
            return G8eeUserSettings(
                llm=app_settings.llm,
                search=app_settings.search,
                eval_judge=app_settings.eval_judge,
                command_validation=app_settings.command_validation,
                batch_execution=app_settings.batch_execution,
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

        doc = UserSettingsDocument(id=user_doc_id, user_id=user_id, settings=new_settings)

        await self._cache_aside.update_document(
            collection=DB_COLLECTION_SETTINGS,
            document_id=user_doc_id,
            data=doc.model_dump(mode="json"),
            merge=False,
        )
        # The next request must read what was just written, including when KV
        # cache reads are enabled.
        await self._cache_aside.invalidate_document(DB_COLLECTION_SETTINGS, user_doc_id)

    async def get_llm_role_settings(self, user_id: str) -> LLMRoleSettingsResponse:
        """Return the caller's provider connections and per-role selections with keys masked."""
        user_settings = await self.get_user_settings(user_id)
        return settings_view(user_settings.llm)

    async def update_llm_role_settings(
        self, user_id: str, request: LLMRoleSettingsUpdateRequest
    ) -> LLMRoleSettingsResponse:
        """Persist caller-owned provider connections and/or role selections."""
        user_settings = await self.get_user_settings(user_id)
        if request.providers is not None:
            apply_provider_updates(user_settings.llm, request.providers)
        if any((request.primary, request.assistant, request.lite)):
            apply_role_updates(user_settings.llm, request)
        await self.update_user_settings(user_id, user_settings)
        self._logger.info(
            "[SettingsService] Updated inference settings for user %s (primary=%s/%s)",
            user_id,
            user_settings.llm.primary_provider,
            user_settings.llm.primary_model,
        )
        return settings_view(user_settings.llm)

    def _build_search_settings(
        self, settings: G8eeAppSettings | G8eeUserSettings
    ) -> SearchSettings:
        """Build SearchSettings from platform or user settings."""
        return settings.search
