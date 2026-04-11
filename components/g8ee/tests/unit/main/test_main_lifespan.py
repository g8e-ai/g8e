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

"""
Tests for app.main.lifespan — the FastAPI startup/shutdown orchestrator.

main.py responsibilities:
    Phase 0: Bootstrap settings (SettingsService, initialize_g8e_service, setup_logging)
    Phase 1: Core g8es clients (DB, KV, PubSub, Blob) via _connect_clients
    Phase 2: Handler services (DBService, KVService, BlobService)
    Phase 3: CacheAsideService
    Phase 4: Platform settings from g8es
    Phase 5: ServiceFactory.create_all_services -> bind_to_app_state
    Phase 6: ServiceFactory.start_services
    Shutdown: ServiceFactory.stop_services -> close clients
"""

import pytest
from unittest.mock import AsyncMock, MagicMock, patch

from fastapi import FastAPI

from app.main import lifespan

pytestmark = [pytest.mark.unit]

_PATCHES = [
    "app.main.SettingsService",
    "app.main.initialize_g8e_service",
    "app.main.setup_logging",
    "app.main.set_settings",
    "app.main.DBClient",
    "app.main.KVCacheClient",
    "app.main.PubSubClient",
    "app.main.BlobClient",
    "app.main.DBService",
    "app.main.KVService",
    "app.main.BlobService",
    "app.main.CacheAsideService",
    "app.main.ServiceFactory",
]


def _build_mocks():
    """Start all patches and return (mocks_dict, patches_list)."""
    patches = [patch(p) for p in _PATCHES]
    mocks = {}
    for p in patches:
        mock_obj = p.start()
        name = p.attribute
        if name in ("DBClient", "KVCacheClient", "PubSubClient", "BlobClient"):
            mock_obj.return_value.connect = AsyncMock(return_value=True)
            mock_obj.return_value.close = AsyncMock()
        mocks[name] = mock_obj
    return mocks, patches


def _configure_settings(mocks):
    """Wire up SettingsService + initialize_g8e_service to return a usable mock settings."""
    settings = MagicMock()
    settings.ca_cert_path = "/tmp/ca.crt"
    settings.auth.internal_auth_token = "tok"
    settings.listen.default_ttl = 3600
    settings.port = 443

    mocks["initialize_g8e_service"].side_effect = AsyncMock(return_value=settings)

    settings_svc = mocks["SettingsService"].return_value
    settings_svc.get_local_settings.return_value = settings
    settings_svc.get_platform_settings = AsyncMock(return_value=settings)
    settings_svc._cache_aside = None

    return settings, settings_svc


def _configure_factory(mocks):
    """Set up ServiceFactory.create_all_services / bind / start / stop."""
    factory = mocks["ServiceFactory"]
    factory.create_all_services.return_value = {"some_service": MagicMock()}
    factory.bind_to_app_state = MagicMock()
    factory.start_services = AsyncMock()
    factory.stop_services = AsyncMock()
    return factory


@pytest.fixture
def mock_app():
    app = MagicMock(spec=FastAPI)
    app.state = MagicMock()
    return app


class TestLifespanStartup:

    async def test_connects_four_core_clients(self, mock_app):
        mocks, patches = _build_mocks()
        _configure_settings(mocks)
        _configure_factory(mocks)
        try:
            async with lifespan(mock_app):
                pass

            mocks["DBClient"].return_value.connect.assert_called_once()
            mocks["KVCacheClient"].return_value.connect.assert_called_once()
            mocks["PubSubClient"].return_value.connect.assert_called_once()
            mocks["BlobClient"].return_value.connect.assert_called_once()
        finally:
            for p in patches:
                p.stop()

    async def test_creates_handler_services(self, mock_app):
        mocks, patches = _build_mocks()
        _configure_settings(mocks)
        _configure_factory(mocks)
        try:
            async with lifespan(mock_app):
                pass

            mocks["DBService"].assert_called_once()
            mocks["KVService"].assert_called_once()
            mocks["BlobService"].assert_called_once()
            mocks["CacheAsideService"].assert_called_once()
        finally:
            for p in patches:
                p.stop()

    async def test_delegates_to_service_factory(self, mock_app):
        mocks, patches = _build_mocks()
        settings, _ = _configure_settings(mocks)
        factory = _configure_factory(mocks)
        try:
            async with lifespan(mock_app):
                pass

            factory.create_all_services.assert_called_once()
            call_kwargs = factory.create_all_services.call_args
            assert call_kwargs.kwargs.get("pubsub_client") is not None
            assert "heartbeat_client" not in (call_kwargs.kwargs or {})

            factory.bind_to_app_state.assert_called_once()
            factory.start_services.assert_called_once()
        finally:
            for p in patches:
                p.stop()

    async def test_loads_platform_settings(self, mock_app):
        mocks, patches = _build_mocks()
        _, settings_svc = _configure_settings(mocks)
        _configure_factory(mocks)
        try:
            async with lifespan(mock_app):
                pass

            settings_svc.get_platform_settings.assert_called_once()
            mocks["set_settings"].assert_called_once()
        finally:
            for p in patches:
                p.stop()

    async def test_bootstrap_logging_called(self, mock_app):
        mocks, patches = _build_mocks()
        _configure_settings(mocks)
        _configure_factory(mocks)
        try:
            async with lifespan(mock_app):
                pass

            mocks["initialize_g8e_service"].assert_called_once()
            mocks["setup_logging"].assert_called_once()
        finally:
            for p in patches:
                p.stop()


class TestLifespanShutdown:

    async def test_stop_services_and_close_clients(self, mock_app):
        mocks, patches = _build_mocks()
        _configure_settings(mocks)
        factory = _configure_factory(mocks)
        try:
            async with lifespan(mock_app):
                pass

            factory.stop_services.assert_called_once()

            mock_app.state.pubsub_client.close.assert_called_once()
            mock_app.state.kv_cache_client.close.assert_called_once()
            mock_app.state.blob_client.close.assert_called_once()
            mock_app.state.db_service.close.assert_called_once()
        finally:
            for p in patches:
                p.stop()

    async def test_shutdown_runs_even_on_startup_failure(self, mock_app):
        mocks, patches = _build_mocks()
        _configure_settings(mocks)
        factory = _configure_factory(mocks)
        factory.start_services.side_effect = RuntimeError("boom")
        try:
            with pytest.raises(RuntimeError, match="boom"):
                async with lifespan(mock_app):
                    pass

            factory.stop_services.assert_called_once()
        finally:
            for p in patches:
                p.stop()

    async def test_no_heartbeat_client_in_shutdown(self, mock_app):
        """heartbeat_client no longer exists -- verify it is not referenced."""
        mocks, patches = _build_mocks()
        _configure_settings(mocks)
        _configure_factory(mocks)
        try:
            async with lifespan(mock_app):
                pass

            assert not hasattr(mock_app.state, "heartbeat_client") or \
                not getattr(mock_app.state.heartbeat_client, "close", MagicMock()).called
        finally:
            for p in patches:
                p.stop()
