# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

"""
Smoke test for ServiceFactory.create_all_services.

This test exercises real construction to catch production startup bugs that
would be hidden by mocking create_all_services in test_main_lifespan.py.
"""

import os
import tempfile
from pathlib import Path
from unittest.mock import MagicMock

import pytest

from app.models.settings import G8eeAppSettings
from app.services.auth.certificate_data_service import CertificateDataService
from app.services.cache.cache_aside import CacheAsideService
from app.services.service_factory import ServiceFactory

pytestmark = [pytest.mark.unit]


@pytest.fixture
def mock_settings():
    """Create a minimal G8eeAppSettings for smoke testing."""
    settings = G8eeAppSettings()
    settings.search.enabled = False
    return settings


@pytest.fixture
def mock_cache_aside():
    """Create a minimal CacheAsideService mock."""
    cache = MagicMock(spec=CacheAsideService)
    cache.get = MagicMock(return_value=None)
    cache.set = MagicMock()
    cache.delete = MagicMock()
    return cache


class TestServiceFactorySmoke:
    """Smoke test for ServiceFactory.create_all_services real construction."""

    def test_create_all_services_real_construction(self, mock_settings, mock_cache_aside):
        """Exercise real ServiceFactory.create_all_services to catch signature mismatches.

        This test validates that the actual create_all_services signature matches
        what production code expects, catching bugs like missing parameters or
        incorrect field access that would be hidden by mocking.
        """
        with tempfile.NamedTemporaryFile(mode="w", delete=False) as f:
            f.write("")
            ssh_config_path = f.name

        try:
            os.environ["G8E_SSH_CONFIG_PATH"] = ssh_config_path

            services = ServiceFactory.create_all_services(
                settings=mock_settings,
                cache_aside_service=mock_cache_aside,
                db_service=MagicMock(),
                kv_service=MagicMock(),
                blob_service=None,
                pubsub_client=None,
                web_search_provider=None,
                governance_client=MagicMock(),
            )

            assert services is not None
            assert hasattr(services, "tool_service")
            assert hasattr(services, "investigation_service")
            assert hasattr(services, "ssh_inventory_service")
            assert isinstance(services.certificate_service.data_service, CertificateDataService)

        finally:
            Path(ssh_config_path).unlink()
            if "G8E_SSH_CONFIG_PATH" in os.environ:
                del os.environ["G8E_SSH_CONFIG_PATH"]

    def test_create_all_services_with_web_search_provider(self, mock_settings, mock_cache_aside):
        """Test create_all_services with web search provider injected."""
        with tempfile.NamedTemporaryFile(mode="w", delete=False) as f:
            f.write("")
            ssh_config_path = f.name

        try:
            os.environ["G8E_SSH_CONFIG_PATH"] = ssh_config_path

            web_search_provider = MagicMock()

            services = ServiceFactory.create_all_services(
                settings=mock_settings,
                cache_aside_service=mock_cache_aside,
                db_service=MagicMock(),
                kv_service=MagicMock(),
                blob_service=None,
                pubsub_client=None,
                web_search_provider=web_search_provider,
                governance_client=MagicMock(),
            )

            assert services is not None
            assert services.web_search_provider is web_search_provider

        finally:
            Path(ssh_config_path).unlink()
            if "G8E_SSH_CONFIG_PATH" in os.environ:
                del os.environ["G8E_SSH_CONFIG_PATH"]
