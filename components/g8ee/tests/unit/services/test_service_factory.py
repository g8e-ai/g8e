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
Smoke test for ServiceFactory.create_all_services.

This test exercises real construction to catch production startup bugs that
would be hidden by mocking create_all_services in test_main_lifespan.py.
"""

import os
import tempfile
from unittest.mock import MagicMock, patch

import pytest

from app.models.settings import G8eePlatformSettings
from app.services.cache.cache_aside import CacheAsideService
from app.services.service_factory import ServiceFactory

pytestmark = [pytest.mark.unit]


@pytest.fixture
def mock_settings():
    """Create a minimal G8eePlatformSettings for smoke testing."""
    settings = G8eePlatformSettings()
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
        with tempfile.NamedTemporaryFile(mode='w', delete=False) as f:
            f.write("")
            ssh_config_path = f.name

        try:
            os.environ["G8E_SSH_CONFIG_PATH"] = ssh_config_path

            services = ServiceFactory.create_all_services(
                settings=mock_settings,
                cache_aside_service=mock_cache_aside,
                pubsub_client=None,
                blob_service=None,
                web_search_provider=None,
            )

            assert services is not None
            assert "tool_service" in services
            assert "tool_executor" in services
            assert "investigation_service" in services
            assert "ssh_inventory_service" in services
            assert services["tool_service"] is services["tool_executor"]

        finally:
            os.unlink(ssh_config_path)
            if "G8E_SSH_CONFIG_PATH" in os.environ:
                del os.environ["G8E_SSH_CONFIG_PATH"]

    def test_create_all_services_with_web_search_provider(self, mock_settings, mock_cache_aside):
        """Test create_all_services with web search provider injected."""
        with tempfile.NamedTemporaryFile(mode='w', delete=False) as f:
            f.write("")
            ssh_config_path = f.name

        try:
            os.environ["G8E_SSH_CONFIG_PATH"] = ssh_config_path

            web_search_provider = MagicMock()

            services = ServiceFactory.create_all_services(
                settings=mock_settings,
                cache_aside_service=mock_cache_aside,
                pubsub_client=None,
                blob_service=None,
                web_search_provider=web_search_provider,
            )

            assert services is not None
            assert services["web_search_provider"] is web_search_provider

        finally:
            os.unlink(ssh_config_path)
            if "G8E_SSH_CONFIG_PATH" in os.environ:
                del os.environ["G8E_SSH_CONFIG_PATH"]
