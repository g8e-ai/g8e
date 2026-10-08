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

from unittest.mock import MagicMock

import pytest

from app.constants import LogLevel
from app.db.blob_service import BlobService
from app.db.document_service import DocumentService
from app.models.settings import G8eeAppSettings
from app.services.auth.certificate_data_service import CertificateDataService
from app.services.service_factory import ServiceFactory

pytestmark = [pytest.mark.unit]


@pytest.fixture
def mock_settings():
    """Create a minimal G8eeAppSettings for smoke testing."""
    settings = G8eeAppSettings(
        port=8443,
        host="0.0.0.0",
        log_level=LogLevel.INFO,
        enable_logging=True,
        session_ttl=28800,
        absolute_session_timeout=86400,
        docs_dir="docs",
    )
    settings.search.enabled = False
    return settings


@pytest.fixture
def mock_document_service():
    """Create a minimal DocumentService mock."""
    cache = MagicMock(spec=DocumentService)
    cache.get = MagicMock(return_value=None)
    cache.set = MagicMock()
    cache.delete = MagicMock()
    return cache


class TestServiceFactorySmoke:
    """Smoke test for ServiceFactory.create_all_services real construction."""

    def test_create_all_services_real_construction(self, mock_settings, mock_document_service):
        """Exercise real ServiceFactory.create_all_services to catch signature mismatches.

        This test validates that the actual create_all_services signature matches
        what production code expects, catching bugs like missing parameters or
        incorrect field access that would be hidden by mocking.
        """
        services = ServiceFactory.create_all_services(
            settings=mock_settings,
            document_service=mock_document_service,
            blob_service=MagicMock(spec=BlobService),
            web_search_provider=None,
            governance_client=MagicMock(),
        )

        assert services is not None
        assert hasattr(services, "tool_service")
        assert hasattr(services, "investigation_service")
        assert hasattr(services, "ssh_inventory_service")
        assert isinstance(services.certificate_service.data_service, CertificateDataService)

    def test_create_all_services_with_web_search_provider(self, mock_settings, mock_document_service):
        """Test create_all_services with web search provider injected."""
        web_search_provider = MagicMock()

        services = ServiceFactory.create_all_services(
            settings=mock_settings,
            document_service=mock_document_service,
            blob_service=MagicMock(spec=BlobService),
            web_search_provider=web_search_provider,
            governance_client=MagicMock(),
        )

        assert services is not None
        assert services.web_search_provider is web_search_provider
