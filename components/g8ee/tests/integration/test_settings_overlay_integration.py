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
from app.models.settings import G8eePlatformSettings
from app.constants.settings import LLMProvider

@pytest.mark.asyncio
class TestG8eeSettingsOverlayIntegration:
    """Deep integration tests for g8ee settings loading and overlay logic."""

    @pytest.fixture
    def cache_service(self):
        return AsyncMock()

    @pytest.fixture
    def settings_service(self, cache_service):
        return SettingsService(cache_aside_service=cache_service)

    async def test_get_platform_settings_loads_from_g8es(self, settings_service, cache_service):
        """Verify platform settings are loaded from the correct g8es collection/ID.
        
        G8eePlatformSettings no longer carries LLM config. LLM settings are
        accessed via get_user_settings() which falls back to platform data
        when no user document exists.
        """
        platform_data = {
            "id": "platform-doc-id",
            "settings": {
                "port": 443,
                "host": "0.0.0.0",
                "log_level": "INFO",
                "enable_logging": True,
                "database": {
                    "db_path": "/g8e/g8ee.db",
                    "poll_interval_active_seconds": 0.5,
                    "poll_interval_idle_seconds": 1.0,
                    "tasks_collection": "tasks",
                    "cases_collection": "cases",
                    "users_collection": "users",
                    "investigations_collection": "investigations",
                    "platform_settings_collection": "settings",
                    "user_settings_collection": "settings",
                    "api_keys_collection": "api_keys",
                    "memories_collection": "memories",
                    "web_sessions_collection": "web_sessions",
                    "operator_sessions_collection": "operator_sessions",
                    "orgs_collection": "orgs",
                    "operators_collection": "operators"
                },
                "listen": {
                    "http_url": "https://g8es:9000",
                    "pubsub_url": "wss://g8es:9001",
                    "blob_url": "https://g8es:9000",
                    "default_ttl": 3600
                },
                "auth": {
                    "internal_auth_token": "test-token",
                    "session_encryption_key": None,
                    "g8e_api_key": None
                },
                "component_urls": {
                    "g8ee_url": "https://g8ee",
                    "g8ed_url": "https://g8ed"
                },
                "docker_gid": "988",
                "session_ttl": 28800,
                "absolute_session_timeout": 86400,
                "docs_dir": "/g8e/docs",
                "supervisor_port": 443,
                "app_url": "https://localhost",
                "allowed_origins": "",
                "passkey_rp_name": "g8e.local",
                "passkey_rp_id": "localhost",
                "passkey_origin": "https://localhost",
                "command_validation": {
                    "enable_whitelisting": False,
                    "enable_blacklisting": False
                },
                "search": {
                    "enabled": False,
                    "project_id": None,
                    "engine_id": None,
                    "location": "global",
                    "api_key": None
                },
                "eval_judge": {
                    "model": None,
                    "max_output_tokens": 4096
                }
            },
            "created_at": "2026-01-01T00:00:00Z",
            "updated_at": "2026-01-01T00:00:00Z"
        }
        cache_service.get_document_with_cache.return_value = platform_data

        settings = await settings_service.get_platform_settings()

        assert isinstance(settings, G8eePlatformSettings)
        cache_service.get_document_with_cache.assert_called_once_with(
            collection=DB_COLLECTION_SETTINGS,
            document_id=PLATFORM_SETTINGS_DOC
        )

    async def test_get_user_settings_overlays_user_data(self, settings_service, cache_service):
        """Verify user settings overlay platform defaults correctly."""
        user_id = "test-user-123"
        user_doc_id = f"{USER_SETTINGS_DOC_PREFIX}{user_id}"
        
        platform_data = {
            "id": "platform-doc-id",
            "settings": {
                "port": 443,
                "host": "0.0.0.0",
                "log_level": "INFO",
                "enable_logging": True,
                "database": {
                    "db_path": "/g8e/g8ee.db",
                    "poll_interval_active_seconds": 0.5,
                    "poll_interval_idle_seconds": 1.0,
                    "tasks_collection": "tasks",
                    "cases_collection": "cases",
                    "users_collection": "users",
                    "investigations_collection": "investigations",
                    "platform_settings_collection": "settings",
                    "user_settings_collection": "settings",
                    "api_keys_collection": "api_keys",
                    "memories_collection": "memories",
                    "web_sessions_collection": "web_sessions",
                    "operator_sessions_collection": "operator_sessions",
                    "orgs_collection": "orgs",
                    "operators_collection": "operators"
                },
                "listen": {
                    "http_url": "https://g8es:9000",
                    "pubsub_url": "wss://g8es:9001",
                    "blob_url": "https://g8es:9000",
                    "default_ttl": 3600
                },
                "auth": {
                    "internal_auth_token": None,
                    "session_encryption_key": None,
                    "g8e_api_key": None
                },
                "component_urls": {
                    "g8ee_url": "https://g8ee",
                    "g8ed_url": "https://g8ed"
                },
                "docker_gid": "988",
                "session_ttl": 28800,
                "absolute_session_timeout": 86400,
                "docs_dir": "/g8e/docs",
                "supervisor_port": 443,
                "app_url": "https://localhost",
                "allowed_origins": "",
                "passkey_rp_name": "g8e.local",
                "passkey_rp_id": "localhost",
                "passkey_origin": "https://localhost",
                "command_validation": {
                    "enable_whitelisting": False,
                    "enable_blacklisting": False
                },
                "search": {
                    "enabled": False,
                    "project_id": None,
                    "engine_id": None,
                    "location": "global",
                    "api_key": None
                },
                "eval_judge": {
                    "model": None,
                    "max_output_tokens": 4096
                }
            },
            "created_at": "2026-01-01T00:00:00Z",
            "updated_at": "2026-01-01T00:00:00Z"
        }
        
        user_data = {
            "id": "user-doc-id",
            "user_id": user_id,
            "settings": {
                "llm": {
                    "llm_primary_provider": "openai",
                    "llm_model": "gpt-4o",
                    "openai_api_key": "user-key"
                },
                "search": {
                    "enabled": False,
                    "project_id": None,
                    "engine_id": None,
                    "location": "global",
                    "api_key": None
                },
                "eval_judge": {
                    "model": None,
                    "max_output_tokens": 4096
                }
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

        cache_service.get_document_with_cache.side_effect = get_doc_mock

        # g8ee SettingsService.get_user_settings currently only returns the UserSettings part
        # overlaid on schema defaults, but we want to ensure it uses the user document if present.
        user_settings = await settings_service.get_user_settings(user_id)
        
        assert user_settings.llm.primary_provider == "openai"
        assert user_settings.llm.primary_model == "gpt-4o"
        assert user_settings.llm.openai_api_key == "user-key"

    async def test_overlay_carries_auditor_hmac_key_from_platform_settings(self, cache_service):
        """Overlay must propagate ``auditor_hmac_key`` from the platform DB
        document onto local bootstrap settings when the bootstrap volume
        has not surfaced one (e.g. on a g8ee process that started before
        the SecretManager ran). The auditor commit step in GDD §14.4
        relies on this key being present in the platform settings object
        the AI pipeline reads from."""
        hmac_key = "f" * 64
        platform_data = {
            "id": "platform-doc-id",
            "settings": {
                "port": 443,
                "host": "0.0.0.0",
                "log_level": "INFO",
                "enable_logging": True,
                "auth": {
                    "internal_auth_token": None,
                    "session_encryption_key": None,
                    "auditor_hmac_key": hmac_key,
                    "g8e_api_key": None,
                },
            },
            "created_at": "2026-01-01T00:00:00Z",
            "updated_at": "2026-01-01T00:00:00Z",
        }
        cache_service.get_document_with_cache.return_value = platform_data

        # Stub bootstrap so it reports no on-disk secrets; this isolates the
        # platform-DB-overlay path described in the docstring.
        bootstrap = MagicMock()
        bootstrap.load_internal_auth_token.return_value = None
        bootstrap.load_session_encryption_key.return_value = None
        bootstrap.load_auditor_hmac_key.return_value = None
        bootstrap.load_ca_cert_path.return_value = None
        settings_service = SettingsService(
            cache_aside_service=cache_service,
            bootstrap_service=bootstrap,
        )

        settings = await settings_service.get_platform_settings()

        assert settings.auth.auditor_hmac_key == hmac_key

    async def test_get_user_settings_falls_back_to_empty_llm_when_missing(self, settings_service, cache_service):
        """Verify user settings return empty LLMSettings with None provider when UserSettingsDocument is missing."""
        user_id = "new-user"
        user_doc_id = f"{USER_SETTINGS_DOC_PREFIX}{user_id}"
        
        platform_data = {
            "id": "platform-doc-id",
            "settings": {
                "port": 443,
                "host": "0.0.0.0",
                "log_level": "INFO",
                "enable_logging": True,
                "database": {
                    "db_path": "/g8e/g8ee.db",
                    "poll_interval_active_seconds": 0.5,
                    "poll_interval_idle_seconds": 1.0,
                    "tasks_collection": "tasks",
                    "cases_collection": "cases",
                    "users_collection": "users",
                    "investigations_collection": "investigations",
                    "platform_settings_collection": "settings",
                    "user_settings_collection": "settings",
                    "api_keys_collection": "api_keys",
                    "memories_collection": "memories",
                    "web_sessions_collection": "web_sessions",
                    "operator_sessions_collection": "operator_sessions",
                    "orgs_collection": "orgs",
                    "operators_collection": "operators"
                },
                "listen": {
                    "http_url": "https://g8es:9000",
                    "pubsub_url": "wss://g8es:9001",
                    "blob_url": "https://g8es:9000",
                    "default_ttl": 3600
                },
                "auth": {
                    "internal_auth_token": None,
                    "session_encryption_key": None,
                    "g8e_api_key": None
                },
                "component_urls": {
                    "g8ee_url": "https://g8ee",
                    "g8ed_url": "https://g8ed"
                },
                "docker_gid": "988",
                "session_ttl": 28800,
                "absolute_session_timeout": 86400,
                "docs_dir": "/g8e/docs",
                "supervisor_port": 443,
                "app_url": "https://localhost",
                "allowed_origins": "",
                "passkey_rp_name": "g8e.local",
                "passkey_rp_id": "localhost",
                "passkey_origin": "https://localhost",
                "command_validation": {
                    "enable_whitelisting": False,
                    "enable_blacklisting": False
                },
                "search": {
                    "enabled": False,
                    "project_id": None,
                    "engine_id": None,
                    "location": "global",
                    "api_key": None
                },
                "eval_judge": {
                    "model": None,
                    "max_output_tokens": 4096
                }
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

        cache_service.get_document_with_cache.side_effect = get_doc_mock

        user_settings = await settings_service.get_user_settings(user_id)

        # LLM settings are user-specific only; missing user doc returns empty LLMSettings with None provider
        assert user_settings.llm.primary_provider is None
        assert user_settings.llm.primary_model is None
        assert user_settings.llm.gemini_api_key is None

    async def test_overlay_auth_merge_is_model_driven(self, cache_service):
        """Every ``AuthSettings`` field present in the platform DB document
        and absent from local bootstrap settings must overlay onto the merged
        object — without ``overlay_platform_data`` having to hand-list it.

        This locks in the structural fix for a recurring class of bugs where
        adding a new auth token (e.g. ``auditor_hmac_key``) silently failed
        to flow through the platform-bootstrap overlay because the merge
        enumerated fields by hand. The test iterates ``AuthSettings.model_fields``
        directly so any newly added auth field is automatically covered.
        """
        from app.models.settings import AuthSettings

        auth_field_names = list(AuthSettings.model_fields.keys())
        # Synthesise a unique non-empty value per field so we can assert each
        # one independently flowed through.
        synthetic_values = {name: f"platform-{name}-value" for name in auth_field_names}

        platform_data = {
            "id": "platform-doc-id",
            "settings": {
                "port": 443,
                "host": "0.0.0.0",
                "log_level": "INFO",
                "enable_logging": True,
                "auth": synthetic_values,
            },
            "created_at": "2026-01-01T00:00:00Z",
            "updated_at": "2026-01-01T00:00:00Z",
        }
        cache_service.get_document_with_cache.return_value = platform_data

        # Stub bootstrap so no on-disk secrets exist; the platform DB doc is
        # the only source of auth values, exercising the overlay path.
        bootstrap = MagicMock()
        bootstrap.load_internal_auth_token.return_value = None
        bootstrap.load_session_encryption_key.return_value = None
        bootstrap.load_auditor_hmac_key.return_value = None
        bootstrap.load_ca_cert_path.return_value = None
        settings_service = SettingsService(
            cache_aside_service=cache_service,
            bootstrap_service=bootstrap,
        )

        settings = await settings_service.get_platform_settings()

        for name, expected in synthetic_values.items():
            assert getattr(settings.auth, name) == expected, (
                f"AuthSettings field {name!r} was not propagated by "
                f"overlay_platform_data; the auth merge has likely regressed "
                f"to hand-listed fields."
            )

    async def test_overlay_auth_bootstrap_value_wins_over_platform(self, cache_service):
        """When the bootstrap volume already provided an auth secret, the
        platform DB value must NOT clobber it. This guards the precedence
        contract that the model-driven auth merge has to preserve."""
        bootstrap_token = "bootstrap-token-value"
        platform_data = {
            "id": "platform-doc-id",
            "settings": {
                "port": 443,
                "host": "0.0.0.0",
                "log_level": "INFO",
                "enable_logging": True,
                "auth": {
                    "internal_auth_token": "platform-token-should-be-ignored",
                    "session_encryption_key": None,
                },
            },
            "created_at": "2026-01-01T00:00:00Z",
            "updated_at": "2026-01-01T00:00:00Z",
        }
        cache_service.get_document_with_cache.return_value = platform_data

        bootstrap = MagicMock()
        bootstrap.load_internal_auth_token.return_value = bootstrap_token
        bootstrap.load_session_encryption_key.return_value = None
        bootstrap.load_auditor_hmac_key.return_value = None
        bootstrap.load_ca_cert_path.return_value = None
        settings_service = SettingsService(
            cache_aside_service=cache_service,
            bootstrap_service=bootstrap,
        )

        settings = await settings_service.get_platform_settings()

        assert settings.auth.internal_auth_token == bootstrap_token

    async def test_overlay_propagates_auto_approve_from_platform(self, cache_service):
        """Platform-level auto_approve settings must flow through overlay_platform_data.

        Regression for a gap where overlay_platform_data only copied
        enable_whitelisting and enable_blacklisting, silently dropping
        whitelisted_commands, enable_auto_approve, auto_approved_commands,
        max_batch_concurrency, and batch_fail_fast.
        """
        platform_data = {
            "id": "platform-doc-id",
            "settings": {
                "port": 443,
                "host": "0.0.0.0",
                "log_level": "INFO",
                "enable_logging": True,
                "auth": {
                    "internal_auth_token": None,
                    "session_encryption_key": None,
                    "g8e_api_key": None,
                },
                "command_validation": {
                    "enable_whitelisting": True,
                    "whitelisted_commands": "uptime,df,free",
                    "enable_blacklisting": True,
                    "enable_auto_approve": True,
                    "auto_approved_commands": "uptime,df",
                    "max_batch_concurrency": 5,
                    "batch_fail_fast": True,
                },
            },
            "created_at": "2026-01-01T00:00:00Z",
            "updated_at": "2026-01-01T00:00:00Z",
        }
        cache_service.get_document_with_cache.return_value = platform_data

        bootstrap = MagicMock()
        bootstrap.load_internal_auth_token.return_value = None
        bootstrap.load_session_encryption_key.return_value = None
        bootstrap.load_auditor_hmac_key.return_value = None
        bootstrap.load_ca_cert_path.return_value = None
        settings_service = SettingsService(
            cache_aside_service=cache_service,
            bootstrap_service=bootstrap,
        )

        settings = await settings_service.get_platform_settings()

        cv = settings.command_validation
        assert cv.enable_whitelisting is True
        assert cv.whitelisted_commands == "uptime,df,free"
        assert cv.enable_blacklisting is True
        assert cv.enable_auto_approve is True
        assert cv.auto_approved_commands == "uptime,df"
        assert cv.max_batch_concurrency == 5
        assert cv.batch_fail_fast is True

    async def test_user_overlay_auto_approve_overrides_platform(self, settings_service, cache_service):
        """User-level command_validation must override platform-level values.

        Verifies that a per-user document with enable_auto_approve=False wins
        even when the platform default has it enabled (and vice versa).
        """
        user_id = "user-with-overrides"
        user_doc_id = f"{USER_SETTINGS_DOC_PREFIX}{user_id}"

        platform_data = {
            "id": "platform-doc-id",
            "settings": {
                "port": 443,
                "host": "0.0.0.0",
                "log_level": "INFO",
                "enable_logging": True,
                "auth": {
                    "internal_auth_token": None,
                    "session_encryption_key": None,
                    "g8e_api_key": None,
                },
                "command_validation": {
                    "enable_auto_approve": True,
                    "auto_approved_commands": "uptime,df,free",
                },
            },
            "created_at": "2026-01-01T00:00:00Z",
            "updated_at": "2026-01-01T00:00:00Z",
        }
        user_data = {
            "id": "user-doc-id",
            "user_id": user_id,
            "settings": {
                "llm": {},
                "command_validation": {
                    "enable_auto_approve": False,
                    "auto_approved_commands": "",
                },
            },
            "created_at": "2026-01-01T00:00:00Z",
            "updated_at": "2026-01-01T00:00:00Z",
        }

        def get_doc_mock(collection, document_id):
            if document_id == user_doc_id:
                return user_data
            if document_id == PLATFORM_SETTINGS_DOC:
                return platform_data
            return None

        cache_service.get_document_with_cache.side_effect = get_doc_mock

        user_settings = await settings_service.get_user_settings(user_id)

        assert user_settings.command_validation.enable_auto_approve is False
        assert user_settings.command_validation.auto_approved_commands == ""

    async def test_user_overlay_inherits_auto_approve_when_user_doc_missing(
        self, settings_service, cache_service
    ):
        """When the user document is missing, command_validation falls back to platform defaults."""
        user_id = "new-user-no-doc"
        user_doc_id = f"{USER_SETTINGS_DOC_PREFIX}{user_id}"

        platform_data = {
            "id": "platform-doc-id",
            "settings": {
                "port": 443,
                "host": "0.0.0.0",
                "log_level": "INFO",
                "enable_logging": True,
                "auth": {
                    "internal_auth_token": None,
                    "session_encryption_key": None,
                    "g8e_api_key": None,
                },
                "command_validation": {
                    "enable_auto_approve": True,
                    "auto_approved_commands": "uptime,df",
                },
            },
            "created_at": "2026-01-01T00:00:00Z",
            "updated_at": "2026-01-01T00:00:00Z",
        }

        def get_doc_mock(collection, document_id):
            if document_id == user_doc_id:
                return None
            if document_id == PLATFORM_SETTINGS_DOC:
                return platform_data
            return None

        cache_service.get_document_with_cache.side_effect = get_doc_mock

        user_settings = await settings_service.get_user_settings(user_id)

        assert user_settings.command_validation.enable_auto_approve is True
        assert user_settings.command_validation.auto_approved_commands == "uptime,df"
