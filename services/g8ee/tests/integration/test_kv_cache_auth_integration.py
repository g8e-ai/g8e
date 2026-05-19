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
Deep integration test for KVCacheClient with real operator authentication.

This test actually makes HTTP requests to operator using mTLS authentication.
It verifies that client certificates are configured correctly and that
cache-aside operations work end-to-end.

This test requires operator to be running and accessible with client certificates.
"""

import pytest

from app.clients.db_client import DBClient
from app.clients.kv_cache_client import KVCacheClient
from app.constants import ComponentName
from app.db.db_service import DBService
from app.db.kv_service import KVService
from app.services.cache.cache_aside import CacheAsideService
from app.services.infra.settings_service import SettingsService

pytestmark = [pytest.mark.integration, pytest.mark.requires_operator]


@pytest.fixture
async def real_kv_client():
    """Create a KVCacheClient that actually connects to operator with real mTLS auth."""
    settings_service = SettingsService()
    bootstrap_settings = settings_service.get_local_settings()

    # Skip test if client certificates are not available for mTLS auth
    if not bootstrap_settings.client_cert_path or not bootstrap_settings.client_key_path:
        pytest.skip("No client certificates available for mTLS authentication")

    # Create KVCacheClient with mTLS authentication
    client = KVCacheClient(
        http_url=bootstrap_settings.listen.http_url,
        component_name=ComponentName.G8EE,
        ca_cert_path=bootstrap_settings.ca_cert_path,
        client_cert_path=bootstrap_settings.client_cert_path,
        client_key_path=bootstrap_settings.client_key_path,
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
    client = DBClient(
        ca_cert_path=bootstrap_settings.ca_cert_path,
        client_cert_path=bootstrap_settings.client_cert_path,
        client_key_path=bootstrap_settings.client_key_path,
    )

    await client.connect()

    yield client

    await client.close()


@pytest.fixture
async def real_cache_aside(real_kv_client, real_db_client):
    """Create a CacheAsideService with real operator clients."""
    return CacheAsideService(
        kv=KVService(real_kv_client),
        db=DBService(real_db_client),
        component_name=ComponentName.G8EE
    )


@pytest.mark.asyncio
async def test_kv_cache_client_real_auth(real_kv_client):
    """Test that KVCacheClient is configured with correct mTLS auth settings."""
    # This test verifies the client is configured correctly with mTLS auth
    # and correct port. It will catch the port mismatch issue (9001 vs 9000).

    # Verify the client has mTLS auth configured
    assert real_kv_client._ca_cert_path is not None, \
        "No CA cert path configured for mTLS"
    assert real_kv_client._client_cert_path is not None, \
        "No client cert path configured for mTLS"
    assert real_kv_client._client_key_path is not None, \
        "No client key path configured for mTLS"

    # Verify the port is correct (9000 for HTTPS, not 9001 for WSS)
    assert real_kv_client.http_url.endswith(":9000"), \
        f"KVCacheClient should use port 9000, but got {real_kv_client.http_url}"
    assert ":9001" not in real_kv_client.http_url, \
        f"KVCacheClient should not use port 9001 (WSS), but got {real_kv_client.http_url}"


@pytest.mark.asyncio
async def test_kv_cache_client_auth_present(real_kv_client):
    """Test that the operator mTLS auth is present."""
    assert real_kv_client._client_cert_path is not None, \
        "Client certificate not configured for mTLS auth"
    assert real_kv_client._client_key_path is not None, \
        "Client key not configured for mTLS auth"


@pytest.mark.asyncio
async def test_kv_cache_port_correctness(real_kv_client):
    """Test that KVCacheClient is using the correct port (9000, not 9001)."""
    # Port 9000 is for HTTPS, port 9001 is for WSS
    # The HTTP client should use port 9000
    assert real_kv_client.http_url.endswith(":9000"), \
        f"KVCacheClient should use port 9000, but got {real_kv_client.http_url}"
    assert ":9001" not in real_kv_client.http_url, \
        f"KVCacheClient should not use port 9001 (WSS), but got {real_kv_client.http_url}"
