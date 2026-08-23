# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

from __future__ import annotations

import logging
import os
from typing import Any

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
from app.constants.env_vars import EnvVar
from app.constants.generated_paths import PathConstants, PortConstants
from app.constants.paths import PATHS
from app.models.base import ConfigDict, Field, G8eBaseModel, G8eIdentifiableModel, PrivateAttr, field_validator
from g8e.models.settings import (
    BatchExecutionSettings as _ProtocolBatchExecutionSettings,
    CommandValidationSettings as _ProtocolCommandValidationSettings,
    EvalJudgeSettings as _ProtocolEvalJudgeSettings,
    LLMSettings as _ProtocolLLMSettings,
    SearchSettings as _ProtocolSearchSettings,
    G8eeUserSettings as _ProtocolG8eeUserSettings,
)


logger = logging.getLogger(__name__)


class TLSConfig(G8eBaseModel):
    """TLS/mTLS configuration for operator client connections."""

    ca_cert_path: str | None = Field(None, description="Path to CA certificate bundle")
    client_cert_path: str | None = Field(None, description="Path to client certificate for mTLS")
    client_key_path: str | None = Field(None, description="Path to client private key for mTLS")


class AppSettingsDocument(G8eIdentifiableModel):
    """Platform-wide configuration document from operator 'platform_settings' collection."""

    model_config = ConfigDict(extra="forbid")

    settings: G8eeAppSettings


class UserSettingsDocument(G8eIdentifiableModel):
    """Per-user settings document from operator 'user_settings' collection."""

    model_config = ConfigDict(extra="forbid")

    user_id: str = Field(..., description="User identifier for these settings")
    settings: G8eeUserSettings


class AuthSettings(G8eBaseModel):
    """Authentication and security token configuration."""

    session_encryption_key: str | None = Field(None, repr=False)
    operator_session_id: str | None = Field(None)
    operator_api_key: str | None = Field(None, repr=False)
    internal_api_key: str | None = Field(None, repr=False)
    auditor_hmac_key: str | None = Field(
        None,
        repr=False,
        description=(
            "HMAC-SHA256 key used by the Tribunal auditor to sign reputation "
            "commitments (GDD §14.4 Artifact B). Generated and rotated by "
            "g8eo SecretManager; mirrored into g8ee via the bootstrap volume "
            "and tamper-verified against bootstrap_digest.json on load."
        ),
    )


class ComponentURLsSettings(G8eBaseModel):
    """Internal and external component URL configuration.

    The SSE event bridge (`/api/v1/sse/push`, `/api/v1/sse/events`,
    `/api/v1/sse/stream`) lives on the Governance Gateway's HTTPS surface.
    ``client_url`` is the base URL the internal HTTP client uses for SSE push,
    intent grant/revoke, and operator-link creation — all gateway endpoints.
    It defaults to the gateway's HTTPS port. Override with the
    ``G8E_GATEWAY_URL`` env var when the gateway is behind a different ingress
    or hostname (e.g. ``https://g8e.local:8443`` in the unified Docker stack).
    """

    g8ee_url: str = Field(
        default_factory=lambda: (
            os.environ.get(
                EnvVar.G8EE_URL,
                f"https://{PATHS.get('host', 'localhost')}:{PortConstants.G8E_PORT_G8EE_HTTPS}",
            )
            or f"https://{PATHS.get('host', 'localhost')}:{PortConstants.G8E_PORT_G8EE_HTTPS}"
        )
    )
    client_url: str = Field(
        default_factory=lambda: (
            os.environ.get(
                EnvVar.GATEWAY_URL,
                f"https://{PATHS.get('host', 'localhost')}:{PortConstants.PORT_OPERATOR_HTTPS}",
            )
            or f"https://{PATHS.get('host', 'localhost')}:{PortConstants.PORT_OPERATOR_HTTPS}"
        )
    )


class CommandValidationSettings(_ProtocolCommandValidationSettings):
    """Operator command safety and validation configuration.

    Three independent policies are surfaced here. They have distinct semantics
    and MUST NOT be conflated:

    - ``enable_whitelisting``: HARD ALLOW-LIST. When enabled, only whitelisted
      commands are permitted to run AT ALL. Any non-listed command is blocked at
      Doctrine (L1Doctrine) safety validation, regardless of human approval. This is a *generation*
      and *execution* constraint.

      Two mutually exclusive whitelist sources exist:

      1. JSON whitelist (config/whitelist.json): Provides rich per-command
         validation including safe_options and validation regexes for parameters.
         This is the default and recommended mode for production use.

      2. CSV whitelist (whitelisted_commands field): A simple comma-separated
         list of base commands. When non-empty, this REPLACES the JSON whitelist
         entirely and uses only basic character-level validation. The JSON
         whitelist's per-command safe_options and validation regexes are NOT
         applied in CSV mode. Use CSV mode for simple deployments that don't
         need fine-grained parameter validation.

      Only one whitelist source should be active at a time: either leave
      whitelisted_commands empty to use JSON mode, or populate it to use CSV mode.

    - ``enable_blacklisting``: HARD BLOCK-LIST. Commands matching blacklist
      entries are blocked at Doctrine (L1Doctrine) safety validation. **This is enabled by default**
      as a recommended boundary to ensure maximum safety and system integrity.

    - ``enable_auto_approve`` / ``auto_approved_commands``: SKIP-APPROVAL list.
      When enabled, commands whose base verb is listed bypass the human
      approval gate (rubber-stamped). **This is enabled by default** to work in
      harmony with the built-in reputation staking system, providing peak signal
      and operational efficiency for low-risk commands.

      A team of heterogeneous agent personas stake their reputation on every
      command alongside the built-in reputation engine. This multi-layered
      staking, combined with the auto-approve and blacklist boundaries,
      creates an ideal operating mode for peak efficiency without compromising
      safety.

      This does NOT permit blacklisted or forbidden commands, and does NOT
      widen the whitelist when whitelisting is enabled - the command must still
      pass all hard gates first.

      Two auto-approve sources are unioned at request time:

      1. JSON file (config/auto_approved.json): Platform-default base
         commands rubber-stamped as benign (e.g., uptime, df, free). Loaded
         by ``CommandAutoApprovedValidator``.
      2. CSV ``auto_approved_commands`` field: Per-user / per-request
         override that augments the JSON list with additional base commands.
    """

    enable_auto_approve: bool = Field(
        True,
        description="If true, commands listed in auto_approved_commands bypass human approval. Independent of whitelisting.",
    )
    auto_approved_commands: str = Field(
        "",
        description="Comma-separated list of base commands that skip human approval (e.g., uptime,df,free). Only used when enable_auto_approve is true. The human is rubber-stamping these as benign.",
    )

    @staticmethod
    def _validate_command_csv(v: str, field_label: str) -> str:
        """Reject whitespace and shell metacharacters in CSV commands."""
        if not v:
            return v

        parts = [p.strip() for p in v.split(",") if p.strip()]
        unsafe_chars = set(";|`$<>&\n\r\t")

        for part in parts:
            if any(c in unsafe_chars for c in part) or " " in part:
                raise ValueError(
                    f"Invalid {field_label} command '{part}'. "
                    "Commands cannot contain spaces or shell metacharacters (; | ` $ < > &). "
                    "Enter base commands only (e.g., 'uptime', 'df')."
                )

        return v

    @field_validator("whitelisted_commands", mode="after")
    @classmethod
    def _validate_whitelisted_commands(cls, v: str) -> str:
        return cls._validate_command_csv(v, "whitelisted")

    @field_validator("auto_approved_commands", mode="after")
    @classmethod
    def _validate_auto_approved_commands(cls, v: str) -> str:
        return cls._validate_command_csv(v, "auto-approved")


class SearchSettings(_ProtocolSearchSettings):
    """Unified search configuration (Vertex AI and Google Search)."""


class DatabaseSettings(G8eBaseModel):
    """SQLite coordination store configuration."""

    db_path: str = Field(PATHS["infra"]["db_path"])
    poll_interval_active_seconds: float = Field(0.5)
    poll_interval_idle_seconds: float = Field(1.0)

    tasks_collection: str = Field(DB_COLLECTION_TASKS)
    cases_collection: str = Field(DB_COLLECTION_CASES)
    users_collection: str = Field(DB_COLLECTION_USERS)
    investigations_collection: str = Field(DB_COLLECTION_INVESTIGATIONS)
    app_settings_collection: str = Field(DB_COLLECTION_SETTINGS)
    user_settings_collection: str = Field(DB_COLLECTION_SETTINGS)
    api_keys_collection: str = Field(DB_COLLECTION_API_KEYS)
    memories_collection: str = Field(DB_COLLECTION_MEMORIES)
    web_sessions_collection: str = Field(DB_COLLECTION_WEB_SESSIONS)
    operator_sessions_collection: str = Field(DB_COLLECTION_OPERATOR_SESSIONS)
    orgs_collection: str = Field(DB_COLLECTION_ORGANIZATIONS)
    operators_collection: str = Field(DB_COLLECTION_OPERATORS)


class GatewaySettings(G8eBaseModel):
    """operator (Operator Gateway mode) configuration."""

    http_url: str = Field(
        default_factory=lambda: (
            os.environ.get(
                EnvVar.OPERATOR_URL,
                f"https://{PATHS.get('host', 'localhost')}:{PATHS['ports']['operator_https']}",
            )
            or f"https://{PATHS.get('host', 'localhost')}:{PATHS['ports']['operator_https']}"
        )
    )
    pubsub_url: str = Field(
        default_factory=lambda: (
            os.environ.get(
                EnvVar.OPERATOR_PUBSUB_URL,
                f"wss://{PATHS.get('host', 'localhost')}:{PATHS['ports']['operator_https']}",
            )
            or f"wss://{PATHS.get('host', 'localhost')}:{PATHS['ports']['operator_https']}"
        )
    )
    blob_url: str = Field(
        default_factory=lambda: (
            os.environ.get(
                EnvVar.OPERATOR_URL,
                f"https://{PATHS.get('host', 'localhost')}:{PATHS['ports']['operator_https']}",
            )
            or f"https://{PATHS.get('host', 'localhost')}:{PATHS['ports']['operator_https']}"
        )
    )
    default_ttl: int = Field(CACHE_TTL_DEFAULT)
    enable_cache_read: bool = Field(False)

    @field_validator("http_url", "pubsub_url", "blob_url", mode="after")
    @classmethod
    def _strip_slashes(cls, v: str) -> str:
        return v.rstrip("/")

    @classmethod
    def from_bootstrap(cls, settings_service: Any) -> GatewaySettings:
        """Load GatewaySettings from bootstrap (volume-based secrets)."""
        settings = settings_service.get_local_settings()
        return settings.gateway


class EvalJudgeSettings(_ProtocolEvalJudgeSettings):
    """Evaluation judge configuration for grading agent performance."""

    model_config = ConfigDict(coerce_numbers_from_str=True)


class LLMSettings(_ProtocolLLMSettings):
    """LLM provider configuration.

    Enum fields (primary_provider, assistant_provider, lite_provider) stay
    as ``LLMProvider`` enum instances inside the application boundary - the
    G8eBaseModel contract. Wire/DB serialization runs through
    ``flatten_for_*`` which uses ``mode="json"`` and emits string values.
    """

    model_config = ConfigDict(coerce_numbers_from_str=True)

    primary_provider: LLMProvider = Field(default=None, alias="llm_primary_provider")
    assistant_provider: LLMProvider = Field(default=None, alias="llm_assistant_provider")
    lite_provider: LLMProvider = Field(default=None, alias="llm_lite_provider")

    openai_model: str | None = Field(default=None)
    openai_endpoint: str | None = Field(default=OPENAI_DEFAULT_ENDPOINT)
    openai_api_key: str | None = Field(default=None, repr=False)

    ollama_model: str | None = Field(default=None)
    ollama_endpoint: str | None = Field(default=OLLAMA_DEFAULT_ENDPOINT)
    ollama_api_key: str | None = Field(default=None, repr=False)

    gemini_model: str | None = Field(default=None)
    gemini_api_key: str | None = Field(default=None, repr=False)

    anthropic_model: str | None = Field(default=None)
    anthropic_endpoint: str | None = Field(default=ANTHROPIC_DEFAULT_ENDPOINT)
    anthropic_api_key: str | None = Field(default=None, repr=False)
    ollama_assistant_model: str | None = Field(default=None)

    llamacpp_model: str | None = Field(default=None)
    llamacpp_endpoint: str | None = Field(default=LLAMACPP_DEFAULT_ENDPOINT)
    llamacpp_api_key: str | None = Field(default=None, repr=False)
    llamacpp_assistant_model: str | None = Field(default=None)

    llm_max_tokens: int | None = Field(default=None)
    llm_command_gen_enabled: bool = Field(default=True)
    llm_command_gen_auditor: bool = Field(default=True)
    llm_command_gen_passes: int = Field(default=5)
    llm_parallel_tool_calls: bool = Field(default=True)

    @property
    def resolved_primary_model(self) -> str | None:
        """Return the configured primary model, or provider default if not set."""
        if self.primary_model:
            return self.primary_model

        if self.primary_provider:
            provider_models = {
                LLMProvider.OPENAI: self.openai_model,
                LLMProvider.ANTHROPIC: self.anthropic_model,
                LLMProvider.GEMINI: self.gemini_model,
                LLMProvider.OLLAMA: self.ollama_model,
                LLMProvider.LLAMACPP: self.llamacpp_model,
            }
            return provider_models.get(self.primary_provider)
        return None

    @property
    def resolved_assistant_model(self) -> str | None:
        """Return the configured assistant model, or provider default if not set."""
        if self.assistant_model:
            return self.assistant_model

        if self.assistant_provider:
            provider_models = {
                LLMProvider.OPENAI: self.openai_model,
                LLMProvider.ANTHROPIC: self.anthropic_model,
                LLMProvider.GEMINI: self.gemini_model,
                LLMProvider.OLLAMA: self.ollama_model,
                LLMProvider.LLAMACPP: self.llamacpp_model,
            }
            return provider_models.get(self.assistant_provider)
        return None

    @property
    def resolved_lite_model(self) -> str | None:
        """Return the configured lite model, or assistant_model/provider default as fallback."""
        if self.lite_model:
            return self.lite_model

        # Try provider default for lite role if lite_provider is set
        if self.lite_provider:
            provider_models = {
                LLMProvider.OPENAI: self.openai_model,
                LLMProvider.ANTHROPIC: self.anthropic_model,
                LLMProvider.GEMINI: self.gemini_model,
                LLMProvider.OLLAMA: self.ollama_model,
                LLMProvider.LLAMACPP: self.llamacpp_model,
            }
            provider_default = provider_models.get(self.lite_provider)
            if provider_default:
                return provider_default

        # Fall back to assistant model (which already falls back to provider default)
        return self.resolved_assistant_model

    def resolve(
        self,
        role: str,
        provider_override: str | None = None,
        api_key_override: str | None = None,
        endpoint_override: str | None = None,
        model_override: str | None = None,
    ) -> tuple[str | None, str | None, str | None, str | None]:
        """Resolve provider, API key, endpoint, and model for a given role.

        Args:
            role: One of 'primary', 'assistant', or 'lite'.
            provider_override: Optional provider string to override the stored provider.
            api_key_override: Optional API key to override the resolved key.
            endpoint_override: Optional endpoint to override the resolved endpoint.
            model_override: Optional model name to override the resolved model.

        Returns:
            Tuple of (provider, api_key, endpoint, model). Provider is the string value of the LLMProvider enum.
        """
        role_to_attrs = {
            "primary": (
                self.primary_provider,
                self.primary_api_key,
                self.primary_endpoint,
                self.primary_model,
            ),
            "assistant": (
                self.assistant_provider,
                self.assistant_api_key,
                self.assistant_endpoint,
                self.assistant_model,
            ),
            "lite": (self.lite_provider, self.lite_api_key, self.lite_endpoint, self.lite_model),
        }

        if role not in role_to_attrs:
            raise ValueError(f"Invalid role: {role}. Must be one of: primary, assistant, lite")

        stored_provider, stored_role_key, stored_role_endpoint, stored_role_model = role_to_attrs[
            role
        ]

        effective_provider = provider_override or (
            stored_provider.value if stored_provider else None
        )
        api_key = api_key_override or stored_role_key
        endpoint = endpoint_override or stored_role_endpoint
        model = model_override or stored_role_model

        if effective_provider:
            provider_defaults = {
                LLMProvider.OPENAI.value: (
                    self.openai_api_key,
                    self.openai_endpoint,
                    self.openai_model,
                ),
                LLMProvider.ANTHROPIC.value: (
                    self.anthropic_api_key,
                    self.anthropic_endpoint,
                    self.anthropic_model,
                ),
                LLMProvider.GEMINI.value: (self.gemini_api_key, None, self.gemini_model),
                LLMProvider.OLLAMA.value: (
                    self.ollama_api_key,
                    self.ollama_endpoint,
                    self.ollama_model,
                ),
                LLMProvider.LLAMACPP.value: (
                    self.llamacpp_api_key,
                    self.llamacpp_endpoint,
                    self.llamacpp_model,
                ),
            }

            p_key, p_endpoint, p_model = provider_defaults.get(
                effective_provider, (None, None, None)
            )

            if not api_key:
                api_key = p_key
            if not endpoint:
                endpoint = p_endpoint
            if not model:
                model = p_model

        return effective_provider, api_key, endpoint, model

    @property
    def primary_endpoint_resolved(self) -> str | None:
        """Return the active primary provider endpoint, role-specific first."""
        _, _, endpoint = self.resolve("primary")
        return endpoint

    @property
    def assistant_endpoint_resolved(self) -> str | None:
        """Return the active assistant provider endpoint, role-specific first."""
        _, _, endpoint = self.resolve("assistant")
        return endpoint

    @property
    def lite_endpoint_resolved(self) -> str | None:
        """Return the active lite provider endpoint, role-specific first."""
        _, _, endpoint = self.resolve("lite")
        return endpoint

    def get_primary_api_key(self) -> str | None:
        """Return the active primary provider API key, role-specific first."""
        _, api_key, _ = self.resolve("primary")
        return api_key

    def get_assistant_api_key(self) -> str | None:
        """Return the active assistant provider API key, role-specific first."""
        _, api_key, _ = self.resolve("assistant")
        return api_key

    def get_lite_api_key(self) -> str | None:
        """Return the active lite provider API key, role-specific first."""
        _, api_key, _ = self.resolve("lite")
        return api_key


class BatchExecutionSettings(_ProtocolBatchExecutionSettings):
    """Batch execution configuration for operator tools.

    These settings control how batched operations across multiple operators
    are executed. They apply to all operator tools that support batch execution
    (commands, port checks, filesystem operations, file operations).
    """


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


class G8eeAppSettings(G8eBaseModel):
    """Platform-level deployment configuration."""

    port: int = Field(PortConstants.G8E_PORT_G8EE_HTTPS)
    host: str = Field("0.0.0.0")
    log_level: LogLevel = Field(LogLevel.INFO)
    enable_logging: bool = Field(True)

    database: DatabaseSettings = Field(default_factory=DatabaseSettings)
    gateway: GatewaySettings = Field(default_factory=GatewaySettings)
    auth: AuthSettings = Field(default_factory=AuthSettings)
    component_urls: ComponentURLsSettings = Field(default_factory=ComponentURLsSettings)

    docker_gid: str = Field("988")
    session_ttl: int = Field(28800)
    absolute_session_timeout: int = Field(86400)
    docs_dir: str = Field(PATHS["infra"]["docs_dir"])

    app_url: str = Field(
        default_factory=lambda: (
            os.environ.get(
                EnvVar.OPERATOR_URL,
                f"https://{PATHS.get('host', 'localhost')}:{PortConstants.PORT_OPERATOR_HTTPS}",
            )
            or f"https://{PATHS.get('host', 'localhost')}:{PortConstants.PORT_OPERATOR_HTTPS}"
        )
    )
    allowed_origins: str = Field(
        default_factory=lambda: os.environ.get(EnvVar.ALLOWED_ORIGINS, "") or ""
    )
    passkey_rp_name: str = Field(
        default_factory=lambda: (
            os.environ.get(EnvVar.PASSKEY_RP_NAME, PATHS.get("host", "localhost"))
            or PATHS.get("host", "localhost")
        )
    )
    passkey_rp_id: str = Field(
        default_factory=lambda: (
            os.environ.get(EnvVar.PASSKEY_RP_ID, PATHS.get("host", "localhost"))
            or PATHS.get("host", "localhost")
        )
    )
    passkey_origin: str = Field(
        default_factory=lambda: (
            os.environ.get(
                EnvVar.PASSKEY_ORIGIN,
                f"https://{PATHS.get('host', 'localhost')}:{PortConstants.PORT_OPERATOR_HTTPS}",
            )
            or f"https://{PATHS.get('host', 'localhost')}:{PortConstants.PORT_OPERATOR_HTTPS}"
        )
    )

    llm: LLMSettings = Field(default_factory=LLMSettings)
    command_validation: CommandValidationSettings = Field(default_factory=CommandValidationSettings)
    search: SearchSettings = Field(default_factory=SearchSettings)
    eval_judge: EvalJudgeSettings = Field(default_factory=EvalJudgeSettings)
    reputation: ReputationSettings = Field(default_factory=ReputationSettings)
    batch_execution: BatchExecutionSettings = Field(default_factory=BatchExecutionSettings)

    # Private fields for overrides (GDD §18.2)
    _ca_cert_path: str | None = PrivateAttr(default=None)
    _client_cert_path: str | None = PrivateAttr(default=None)
    _client_key_path: str | None = PrivateAttr(default=None)

    @property
    def ca_cert_path(self) -> str | None:
        """First valid CA path for internal services."""
        if self._ca_cert_path is not None:
            return self._ca_cert_path
        ca_path = PATHS["infra"]["ca_cert_path"]
        try:
            with open(ca_path):
                return ca_path
        except OSError:
            return None

    @property
    def client_cert_path(self) -> str | None:
        """Client certificate path for mTLS."""
        if self._client_cert_path is not None:
            return self._client_cert_path
        from app.constants.paths import get_app_cert_paths

        cert_path, _ = get_app_cert_paths()
        if not cert_path:
            return None
        try:
            with open(cert_path):
                return cert_path
        except OSError:
            return None

    @property
    def client_key_path(self) -> str | None:
        """Client private key path for mTLS."""
        if self._client_key_path is not None:
            return self._client_key_path
        from app.constants.paths import get_app_cert_paths

        _, key_path = get_app_cert_paths()
        if not key_path:
            return None
        try:
            with open(key_path):
                return key_path
        except OSError:
            return None

    @classmethod
    async def from_db(cls, settings_service: Any) -> G8eeAppSettings:
        """Load platform settings from DB: Defaults < Env < Platform."""
        return await settings_service.get_app_settings()


class G8eeUserSettings(_ProtocolG8eeUserSettings):
    """Per-user settings, overlaid on platform settings."""

    llm: LLMSettings = Field(default_factory=LLMSettings)
    command_validation: CommandValidationSettings = Field(default_factory=CommandValidationSettings)

    @classmethod
    async def from_db(cls, settings_service: Any, user_id: str) -> G8eeUserSettings:
        """Load user settings from DB."""
        return await settings_service.get_user_settings(user_id)
