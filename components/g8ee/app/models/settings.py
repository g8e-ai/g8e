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
    LLAMACPP_DEFAULT_ENDPOINT,
    LLMProvider,
    LogLevel,
)
from app.constants.paths import PATHS
from pydantic import field_validator
from app.models.base import ConfigDict, Field, G8eBaseModel, G8eIdentifiableModel

if TYPE_CHECKING:
    pass

logger = logging.getLogger(__name__)

class PlatformSettingsDocument(G8eIdentifiableModel):
    """Platform-wide configuration document from g8es 'platform_settings' collection."""

    model_config = ConfigDict(extra="forbid")

    settings: G8eePlatformSettings

class UserSettingsDocument(G8eIdentifiableModel):
    """Per-user settings document from g8es 'user_settings' collection."""

    model_config = ConfigDict(extra="forbid")

    user_id: str = Field(..., description="User identifier for these settings")
    settings: G8eeUserSettings

class AuthSettings(G8eBaseModel):
    """Authentication and security token configuration."""
    internal_auth_token: str | None = Field(None)
    session_encryption_key: str | None = Field(None)
    g8e_api_key: str | None = Field(None)
    auditor_hmac_key: str | None = Field(
        None,
        description=(
            "HMAC-SHA256 key used by the Tribunal auditor to sign reputation "
            "commitments (GDD §14.4 Artifact B). Generated and rotated by "
            "g8eo SecretManager; mirrored into g8ee via the bootstrap volume "
            "and tamper-verified against bootstrap_digest.json on load."
        ),
    )

class ComponentURLsSettings(G8eBaseModel):
    """Internal and external component URL configuration."""
    g8ee_url: str = Field("https://g8ee")
    g8ed_url: str = Field("https://g8ed")

class CommandValidationSettings(G8eBaseModel):
    """Operator command safety and validation configuration."""
    enable_whitelisting: bool = Field(False)
    enable_blacklisting: bool = Field(False)
    max_batch_concurrency: int = Field(
        10,
        ge=1,
        le=64,
        description="Maximum number of operators a single batched command may dispatch to concurrently.",
    )
    batch_fail_fast: bool = Field(
        False,
        description="If true, remaining per-operator executions are cancelled after the first failure in a batch.",
    )

class SearchSettings(G8eBaseModel):
    """Unified search configuration (Vertex AI and Google Search)."""
    enabled: bool = Field(False)
    project_id: str | None = Field(None)
    engine_id: str | None = Field(None)
    location: str = Field("global")
    api_key: str | None = Field(None)

class DatabaseSettings(G8eBaseModel):
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

class ListenSettings(G8eBaseModel):
    """g8es (Operator --listen mode) configuration."""
    http_url: str = Field("https://g8es:9000")
    pubsub_url: str = Field("wss://g8es:9001")
    blob_url: str = Field("https://g8es:9000")
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

class EvalJudgeSettings(G8eBaseModel):
    """Evaluation judge configuration for grading agent performance."""
    model_config = ConfigDict(
        populate_by_name=True,
        extra="ignore",
        coerce_numbers_from_str=True,
    )

    model: str | None = Field(None, alias="eval_judge_model")
    max_output_tokens: int = Field(4096, alias="eval_judge_max_tokens")

class LLMSettings(G8eBaseModel):
    """LLM provider configuration.

    Enum fields (primary_provider, assistant_provider, lite_provider) stay
    as ``LLMProvider`` enum instances inside the application boundary — the
    G8eBaseModel contract. Wire/DB serialization runs through
    ``flatten_for_*`` which uses ``mode="json"`` and emits string values.
    """
    model_config = ConfigDict(
        populate_by_name=True,
        extra="ignore",
        coerce_numbers_from_str=True,
    )

    primary_provider: LLMProvider = Field(default=None, alias="llm_primary_provider")
    assistant_provider: LLMProvider = Field(default=None, alias="llm_assistant_provider")
    lite_provider: LLMProvider = Field(default=None, alias="llm_lite_provider")

    primary_model: str | None = Field(default=None, alias="llm_model")
    assistant_model: str | None = Field(default=None, alias="llm_assistant_model")
    lite_model: str | None = Field(default=None, alias="llm_lite_model")

    openai_endpoint: str | None = Field(default=OPENAI_DEFAULT_ENDPOINT)
    openai_api_key: str | None = Field(default=None)

    ollama_endpoint: str | None = Field(default=None)
    ollama_api_key: str | None = Field(default=None)

    gemini_api_key: str | None = Field(default=None)

    anthropic_endpoint: str | None = Field(default=ANTHROPIC_DEFAULT_ENDPOINT)
    anthropic_api_key: str | None = Field(default=None)
    ollama_assistant_model: str | None = Field(default=None)

    llamacpp_endpoint: str | None = Field(default=LLAMACPP_DEFAULT_ENDPOINT)
    llamacpp_api_key: str | None = Field(default=None)
    llamacpp_assistant_model: str | None = Field(default=None)

    llm_max_tokens: int | None = Field(default=None)
    llm_command_gen_enabled: bool = Field(default=True)
    llm_command_gen_auditor: bool = Field(default=True)
    llm_command_gen_passes: int = Field(default=5)

    @property
    def resolved_assistant_model(self) -> str | None:
        """Return the configured assistant model, or None if not set."""
        return self.assistant_model or None

    @property
    def resolved_lite_model(self) -> str | None:
        """Return the configured lite model, or assistant_model as fallback if lite is not set."""
        return self.lite_model or self.assistant_model or None

    @property
    def primary_endpoint(self) -> str | None:
        """Return the active primary provider endpoint."""
        endpoints = {
            LLMProvider.OPENAI: self.openai_endpoint,
            LLMProvider.ANTHROPIC: self.anthropic_endpoint,
            LLMProvider.OLLAMA: self.ollama_endpoint,
            LLMProvider.GEMINI: None,
            LLMProvider.LLAMACPP: self.llamacpp_endpoint,
        }
        return endpoints.get(self.primary_provider)

    @property
    def assistant_endpoint(self) -> str | None:
        """Return the active assistant provider endpoint."""
        endpoints = {
            LLMProvider.OPENAI: self.openai_endpoint,
            LLMProvider.ANTHROPIC: self.anthropic_endpoint,
            LLMProvider.OLLAMA: self.ollama_endpoint,
            LLMProvider.GEMINI: None,
            LLMProvider.LLAMACPP: self.llamacpp_endpoint,
        }
        return endpoints.get(self.assistant_provider)

class ReputationSettings(G8eBaseModel):
    """Phase 3 reputation-resolution configuration (GDD §14.5, §15 Phase 3).

    Reputation resolution is always enabled in the ephemeral architecture.
    The per-tool-call reputation hook runs after every Tribunal-backed
    `run_commands_with_operator` invocation via `orchestrate_tool_execution`.
    """

    ema_half_life: int = Field(
        default=50,
        ge=1,
        description=(
            "EMA half-life in resolutions; alpha = 1 / half_life. GDD §14.10 "
            "suggests 50 as the start point."
        ),
    )


class G8eePlatformSettings(G8eBaseModel):
    """Platform-level deployment configuration."""
    port: int = Field(443)
    host: str = Field("0.0.0.0")
    log_level: LogLevel = Field(LogLevel.INFO)
    enable_logging: bool = Field(True)

    database: DatabaseSettings = Field(default_factory=DatabaseSettings)
    listen: ListenSettings = Field(default_factory=ListenSettings)
    auth: AuthSettings = Field(default_factory=AuthSettings)
    component_urls: ComponentURLsSettings = Field(default_factory=ComponentURLsSettings)

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
    eval_judge: EvalJudgeSettings = Field(default_factory=EvalJudgeSettings)
    reputation: ReputationSettings = Field(default_factory=ReputationSettings)

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
    async def from_db(cls, settings_service: Any) -> "G8eePlatformSettings":
        """Load platform settings from DB: Defaults < Env < Platform."""
        return await settings_service.get_platform_settings()


class G8eeUserSettings(G8eBaseModel):
    """Per-user settings, overlaid on platform settings."""
    llm: LLMSettings
    search: SearchSettings = Field(default_factory=SearchSettings)
    eval_judge: EvalJudgeSettings = Field(default_factory=EvalJudgeSettings)
    command_validation: CommandValidationSettings = Field(default_factory=CommandValidationSettings)

    @classmethod
    async def from_db(cls, settings_service: Any, user_id: str) -> "G8eeUserSettings":
        """Load user settings from DB."""
        return await settings_service.get_user_settings(user_id)
