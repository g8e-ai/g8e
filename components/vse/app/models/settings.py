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
from enum import Enum
from typing import TYPE_CHECKING, Any

from app.constants import (
    CACHE_TTL_DEFAULT,
    DB_COLLECTION_API_KEYS,
    DB_COLLECTION_CASES,
    DB_COLLECTION_SETTINGS,
    DB_COLLECTION_INVESTIGATIONS,
    DB_COLLECTION_MEMORIES,
    DB_COLLECTION_OPERATORS,
    DB_COLLECTION_OPERATOR_SESSIONS,
    DB_COLLECTION_ORGANIZATIONS,
    DB_COLLECTION_TASKS,
    DB_COLLECTION_WEB_SESSIONS,
    DB_COLLECTION_USERS,
    OPENAI_DEFAULT_ENDPOINT,
    OLLAMA_DEFAULT_ENDPOINT,
    ANTHROPIC_DEFAULT_ENDPOINT,
    OLLAMA_DEFAULT_MODEL,
    LLMProvider,
    LogLevel,
)
from app.constants.paths import PATHS
from pydantic import model_validator, field_validator
from app.models.base import ConfigDict, Field, VSOBaseModel

if TYPE_CHECKING:
    from app.services.cache.cache_aside import CacheAsideService

logger = logging.getLogger(__name__)

class PlatformSettingsData(VSOBaseModel):
    """Internal flat map of platform settings as stored in VSODB.
    
    Keys must match shared/models/platform_settings.json.
    """
    model_config = ConfigDict(extra="ignore")

    internal_auth_token: str | None = None
    session_encryption_key: str | None = None
    
    # Platform-wide defaults/overrides
    llm_provider: str | None = None
    llm_model: str | None = None
    llm_assistant_model: str | None = None
    
    openai_endpoint: str | None = None
    openai_api_key: str | None = None
    
    ollama_endpoint: str | None = None
    ollama_api_key: str | None = None
    
    gemini_api_key: str | None = None
    
    anthropic_endpoint: str | None = None
    anthropic_api_key: str | None = None
    
    llm_temperature: float | None = None
    llm_max_tokens: int | None = None
    llm_command_gen_enabled: bool = True
    llm_command_gen_verifier: bool = True
    llm_command_gen_passes: int | None = None
    llm_command_gen_temp: float | None = None
    
    google_search_enabled: bool | str = False
    google_search_api_key: str | None = None
    google_search_engine_id: str | None = None
    
    vertex_search_enabled: bool | str = False
    vertex_search_project_id: str | None = None
    vertex_search_engine_id: str | None = None
    vertex_search_location: str = "global"
    vertex_search_api_key: str | None = None
    
    enable_command_whitelisting: bool | str = False
    enable_command_blacklisting: bool | str = False
    g8e_api_key: str | None = None
    
    passkey_rp_name: str = "g8e.local"
    passkey_rp_id: str = "localhost"
    passkey_origin: str = "https://localhost"
    app_url: str = "https://localhost"
    allowed_origins: str = ""

    # Cluster Port Configuration
    https_port: int = 443
    http_port: int = 80
    vsodb_http_port: int = 9000
    vsodb_wss_port: int = 9001
    supervisor_port: int = 443

    @model_validator(mode="before")
    @classmethod
    def _coerce_booleans(cls, data: Any) -> Any:
        if not isinstance(data, dict):
            return data
        
        bool_fields = {
            "llm_command_gen_enabled",
            "llm_command_gen_verifier",
            "vertex_search_enabled", 
            "enable_command_whitelisting", 
            "enable_command_blacklisting",
            "google_search_enabled"
        }
        
        for field in bool_fields:
            if field in data:
                val = data[field]
                if isinstance(val, str):
                    data[field] = val.lower() not in ("false", "0", "")
        return data


class UserSettingsData(VSOBaseModel):
    """Internal flat map of user settings as stored in VSODB.
    
    Keys must match shared/models/user_settings.json.
    """
    model_config = ConfigDict(extra="ignore")

    llm_provider: str | None = None
    llm_model: str | None = None
    llm_assistant_model: str | None = None
    
    openai_api_key: str | None = None
    ollama_api_key: str | None = None
    gemini_api_key: str | None = None
    anthropic_api_key: str | None = None

    llm_temperature: float | None = None
    llm_max_tokens: int | None = None
    
    llm_command_gen_enabled: bool | str | None = None
    llm_command_gen_verifier: bool | str | None = None
    llm_command_gen_passes: int | None = None
    llm_command_gen_temp: float | None = None
    
    google_search_enabled: bool | str | None = None
    google_search_api_key: str | None = None
    google_search_engine_id: str | None = None
    
    vertex_search_enabled: bool | str | None = None
    vertex_search_project_id: str | None = None
    vertex_search_engine_id: str | None = None
    vertex_search_location: str | None = None
    vertex_search_api_key: str | None = None

    enable_command_whitelisting: bool | str | None = None
    enable_command_blacklisting: bool | str | None = None
    g8e_api_key: str | None = None

    @model_validator(mode="before")
    @classmethod
    def _coerce_booleans(cls, data: Any) -> Any:
        if not isinstance(data, dict):
            return data
        
        bool_fields = {
            "llm_command_gen_enabled", 
            "llm_command_gen_verifier", 
            "vertex_search_enabled",
            "google_search_enabled",
            "enable_command_whitelisting",
            "enable_command_blacklisting"
        }
        
        for field in bool_fields:
            if field in data:
                val = data[field]
                if isinstance(val, str):
                    data[field] = val.lower() not in ("false", "0", "")
        return data


class PlatformSettingsDocument(VSOBaseModel):
    """Platform-wide configuration document from VSODB 'platform_settings' collection."""
    settings: PlatformSettingsData
    created_at: str
    updated_at: str


class UserSettingsDocument(VSOBaseModel):
    """Per-user settings document from VSODB 'user_settings' collection."""
    settings: UserSettingsData
    created_at: str
    updated_at: str

class AuthSettings(VSOBaseModel):
    """Authentication and security token configuration."""
    internal_auth_token: str | None = Field(None)
    session_encryption_key: str | None = Field(None)
    g8e_api_key: str | None = Field(None)

class ServiceURLsSettings(VSOBaseModel):
    """Internal and external service URL configuration."""
    vse_url: str = Field("https://vse")
    vsod_url: str = Field("https://vsod")

class CommandValidationSettings(VSOBaseModel):
    """Operator command safety and validation configuration."""
    enable_whitelisting: bool = Field(False)
    enable_blacklisting: bool = Field(False)

class SearchSettings(VSOBaseModel):
    """Unified search configuration (Vertex AI and Google Search)."""
    enabled: bool = Field(False)
    project_id: str | None = Field(None)
    engine_id: str | None = Field(None)
    location: str = Field("global")
    api_key: str | None = Field(None)

class DatabaseSettings(VSOBaseModel):
    """SQLite coordination store configuration."""
    db_path: str = Field(PATHS["infra"]["db_path"])
    poll_interval_active_seconds: float = Field(0.5)
    poll_interval_idle_seconds: float = Field(1.0)

    tasks_collection: str = Field(DB_COLLECTION_TASKS)
    cases_collection: str = Field(DB_COLLECTION_CASES)
    users_collection: str = Field(DB_COLLECTION_USERS)
    investigations_collection: str = Field(DB_COLLECTION_INVESTIGATIONS)
    platform_settings_collection: str = Field(DB_COLLECTION_SETTINGS)
    user_settings_collection: str = Field(DB_COLLECTION_SETTINGS)
    api_keys_collection: str = Field(DB_COLLECTION_API_KEYS)
    memories_collection: str = Field(DB_COLLECTION_MEMORIES)
    web_sessions_collection: str = Field(DB_COLLECTION_WEB_SESSIONS)
    operator_sessions_collection: str = Field(DB_COLLECTION_OPERATOR_SESSIONS)
    orgs_collection: str = Field(DB_COLLECTION_ORGANIZATIONS)
    operators_collection: str = Field(DB_COLLECTION_OPERATORS)

class ListenSettings(VSOBaseModel):
    """VSODB (Operator --listen mode) configuration."""
    http_url: str = Field("https://vsodb:9000")
    pubsub_url: str = Field("wss://vsodb:9001")
    blob_url: str = Field("https://vsodb:9000")
    default_ttl: int = Field(CACHE_TTL_DEFAULT)

    @field_validator("http_url", "pubsub_url", "blob_url", mode="after")
    @classmethod
    def _strip_slashes(cls, v: str) -> str:
        return v.rstrip("/")

    @classmethod
    def from_bootstrap(cls, settings_service: Any) -> "ListenSettings":
        """Load ListenSettings from bootstrap (volume-based secrets)."""
        settings = settings_service.get_local_settings()
        return settings.listen

class LLMSettings(VSOBaseModel):
    """LLM provider configuration."""
    model_config = ConfigDict(
        populate_by_name=True,
        use_enum_values=True,
        extra="ignore",
        coerce_numbers_from_str=True,
    )

    provider: LLMProvider = Field(default=LLMProvider.OLLAMA)
    
    primary_model: str | None = Field(None, alias="llm_model")
    assistant_model: str | None = Field(None, alias="llm_assistant_model")
    
    openai_endpoint: str | None = Field(None)
    openai_api_key: str | None = Field(None)

    ollama_endpoint: str | None = Field(None)
    ollama_api_key: str | None = Field(None)

    gemini_api_key: str | None = Field(None)

    anthropic_endpoint: str | None = Field(None)
    anthropic_api_key: str | None = Field(None)
    ollama_assistant_model: str | None = Field(None)

    llm_temperature: float | None = Field(None)
    llm_max_tokens: int | None = Field(None)
    llm_command_gen_enabled: bool = Field(True)
    llm_command_gen_verifier: bool = Field(True)
    llm_command_gen_passes: int = Field(3)
    llm_command_gen_temp: float | None = Field(None)

    @property
    def endpoint(self) -> str | None:
        """Return the active provider endpoint."""
        endpoints = {
            LLMProvider.OPENAI: self.openai_endpoint,
            LLMProvider.ANTHROPIC: self.anthropic_endpoint,
            LLMProvider.OLLAMA: self.ollama_endpoint,
            LLMProvider.GEMINI: None,
        }
        return endpoints.get(self.provider)

class VSEPlatformSettings(VSOBaseModel):
    """Platform-level deployment configuration."""
    port: int
    host: str = Field("0.0.0.0")
    log_level: LogLevel = Field(LogLevel.INFO)
    enable_logging: bool = Field(True)

    database: DatabaseSettings = Field(default_factory=DatabaseSettings)
    listen: ListenSettings = Field(default_factory=ListenSettings)
    auth: AuthSettings = Field(default_factory=AuthSettings)
    service_urls: ServiceURLsSettings = Field(default_factory=ServiceURLsSettings)

    docker_gid: str = Field("988")
    session_ttl: int = Field(28800)
    absolute_session_timeout: int = Field(86400)
    docs_dir: str = Field(PATHS["infra"]["docs_dir"])
    supervisor_port: int = Field(443)

    app_url: str = Field("https://localhost")
    allowed_origins: str = Field("")
    passkey_rp_name: str = Field("g8e.local")
    passkey_rp_id: str = Field("localhost")
    passkey_origin: str = Field("https://localhost")

    command_validation: CommandValidationSettings = Field(default_factory=CommandValidationSettings)
    search: SearchSettings = Field(default_factory=SearchSettings)
    llm: LLMSettings = Field(default_factory=lambda: LLMSettings(provider=LLMProvider.OLLAMA))

    @property
    def ca_cert_path(self) -> str | None:
        """First valid CA path for internal services."""
        ca_path = PATHS["infra"]["ca_cert_path"]
        try:
            with open(ca_path):
                return ca_path
        except (OSError, IOError):
            return None

    @classmethod
    async def from_db(cls, settings_service: Any) -> "VSEPlatformSettings":
        """Load platform settings from DB: Defaults < Env < Platform."""
        return await settings_service.get_platform_settings()


class VSEUserSettings(VSOBaseModel):
    """Per-user settings, overlaid on platform settings."""
    llm: LLMSettings
    search: SearchSettings = Field(default_factory=SearchSettings)

    @classmethod
    async def from_db(cls, settings_service: Any, user_id: str) -> "VSEUserSettings":
        """Load user settings from DB."""
        return await settings_service.get_user_settings(user_id)
