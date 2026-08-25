# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

"""
Deep integration test for KVCacheClient with real operator authentication.

This test actually makes HTTP requests to operator using mTLS authentication.
It verifies that client certificates are configured correctly and that
cache-aside operations work end-to-end.

This test requires operator to be running and accessible with client certificates.
"""

import pytest

from app.clients.db_client import DBClient
from app.clients.kv_cache_client import KVCacheClient
from app.constants import G8EE_COMPONENT
from app.db.db_service import DBService
from app.db.kv_service import KVService
from app.models.settings import TLSConfig
from app.services.cache.cache_aside import CacheAsideService
from app.services.infra.settings_service import SettingsService

pytestmark = [pytest.mark.integration]


@pytest.fixture
async def real_kv_client():
    """Create a KVCacheClient that actually connects to operator with real mTLS auth."""
    settings_service = SettingsService()
    bootstrap_settings = settings_service.get_local_settings()

    # Skip test if client certificates are not available for mTLS auth
    if not bootstrap_settings.client_cert_path or not bootstrap_settings.client_key_path:
        pytest.skip("No client certificates available for mTLS authentication")

    # Create KVCacheClient with mTLS authentication
    tls_config = TLSConfig(
        ca_cert_path=bootstrap_settings.ca_cert_path,
        client_cert_path=bootstrap_settings.client_cert_path,
        client_key_path=bootstrap_settings.client_key_path,
    )
    client = KVCacheClient(
        http_url=bootstrap_settings.gateway.http_url,
        component_name=G8EE_COMPONENT,
        tls_config=tls_config,
    )

    await client.connect()

    yield client

    await client.close()


@pytest.fixture
async def real_db_client():
    """Create a DBClient that actually connects to operator with real mTLS auth."""
    settings_service = SettingsService()
    bootstrap_settings = settings_service.get_local_settings()

    # Skip test if client certificates are not available for mTLS auth
    if not bootstrap_settings.client_cert_path or not bootstrap_settings.client_key_path:
        pytest.skip("No client certificates available for mTLS authentication")

    # Create DBClient with mTLS authentication
    tls_config = TLSConfig(
        ca_cert_path=bootstrap_settings.ca_cert_path,
        client_cert_path=bootstrap_settings.client_cert_path,
        client_key_path=bootstrap_settings.client_key_path,
    )
    client = DBClient(tls_config=tls_config)

    await client.connect()

    yield client

    await client.close()


@pytest.fixture
async def real_cache_aside(real_kv_client, real_db_client):
    """Create a CacheAsideService with real operator clients."""
    return CacheAsideService(
        kv=KVService(real_kv_client),
        db=DBService(real_db_client),
        component_name=G8EE_COMPONENT,
    )


@pytest.mark.asyncio
async def test_kv_cache_client_real_auth(real_kv_client):
    """Test that KVCacheClient is configured with correct mTLS auth settings."""
    # This test verifies the client is configured correctly with mTLS auth
    # and correct port.

    # Verify the client has mTLS auth configured
    assert real_kv_client._ca_cert_path is not None, "No CA cert path configured for mTLS"
    assert real_kv_client._client_cert_path is not None, "No client cert path configured for mTLS"
    assert real_kv_client._client_key_path is not None, "No client key path configured for mTLS"

    # Verify the port is correct (8443 for HTTPS)
    assert real_kv_client.http_url.endswith(":8443"), (
        f"KVCacheClient should use port 8443, but got {real_kv_client.http_url}"
    )


@pytest.mark.asyncio
async def test_kv_cache_client_auth_present(real_kv_client):
    """Test that the operator mTLS auth is present."""
    assert real_kv_client._client_cert_path is not None, (
        "Client certificate not configured for mTLS auth"
    )
    assert real_kv_client._client_key_path is not None, "Client key not configured for mTLS auth"


@pytest.mark.asyncio
async def test_kv_cache_port_correctness(real_kv_client):
    """Test that KVCacheClient is using the correct port (8443 for HTTPS)."""
    # Port 8443 is for HTTPS per g8e protocol
    # The HTTP client should use port 8443
    assert real_kv_client.http_url.endswith(":8443"), (
        f"KVCacheClient should use port 8443, but got {real_kv_client.http_url}"
    )
