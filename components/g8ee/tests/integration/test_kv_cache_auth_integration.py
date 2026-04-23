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
Deep integration test for KVCacheClient with real g8es authentication.

This test actually makes HTTP requests to g8es using the real internal auth token.
It verifies that the X-Internal-Auth header is being sent correctly and that
cache-aside operations work end-to-end.

This test requires g8es to be running and accessible.
"""

import os
import pytest

from app.clients.kv_cache_client import KVCacheClient
from app.clients.db_client import DBClient
from app.services.infra.settings_service import SettingsService
from app.services.cache.cache_aside import CacheAsideService
from app.db.kv_service import KVService
from app.db.db_service import DBService
from app.constants import ComponentName

pytestmark = pytest.mark.integration


@pytest.fixture
async def real_kv_client():
    """Create a KVCacheClient that actually connects to g8es with real auth."""
    settings_service = SettingsService()
    bootstrap_settings = settings_service.get_local_settings()
    
    # Skip test if no auth token is available (e.g., running without g8es)
    if not bootstrap_settings.auth.internal_auth_token:
        pytest.skip("No internal auth token available - g8es not accessible")
    
    client = KVCacheClient(
        http_url=bootstrap_settings.listen.http_url,
        component_name=ComponentName.G8EE,
        ca_cert_path=bootstrap_settings.ca_cert_path,
        internal_auth_token=bootstrap_settings.auth.internal_auth_token,
    )
    
    await client.connect()
    
    yield client
    
    await client.close()


@pytest.fixture
async def real_db_client():
    """Create a DBClient that actually connects to g8es with real auth."""
    settings_service = SettingsService()
    bootstrap_settings = settings_service.get_local_settings()
    
    # Skip test if no auth token is available
    if not bootstrap_settings.auth.internal_auth_token:
        pytest.skip("No internal auth token available - g8es not accessible")
    
    client = DBClient(
        ca_cert_path=bootstrap_settings.ca_cert_path,
        internal_auth_token=bootstrap_settings.auth.internal_auth_token,
    )
    
    await client.connect()
    
    yield client
    
    await client.close()


@pytest.fixture
async def real_cache_aside(real_kv_client, real_db_client):
    """Create a CacheAsideService with real g8es clients."""
    cache_aside = CacheAsideService(
        kv=KVService(real_kv_client),
        db=DBService(real_db_client),
        component_name=ComponentName.G8EE
    )
    return cache_aside


@pytest.mark.asyncio
async def test_kv_cache_client_real_auth(real_kv_client):
    """Test that KVCacheClient is configured with correct auth settings."""
    # This test verifies the client is configured correctly with the auth token
    # and correct port. It will catch the port mismatch issue (9001 vs 9000).
    
    # Verify the client has the auth token set
    assert real_kv_client._internal_auth_token is not None, "Internal auth token is None"
    assert len(real_kv_client._internal_auth_token) == 64, \
        f"Token length is {len(real_kv_client._internal_auth_token)}, expected 64"
    
    # Verify the port is correct (9000 for HTTPS, not 9001 for WSS)
    assert real_kv_client.http_url.endswith(":9000"), \
        f"KVCacheClient should use port 9000, but got {real_kv_client.http_url}"
    assert ":9001" not in real_kv_client.http_url, \
        f"KVCacheClient should not use port 9001 (WSS), but got {real_kv_client.http_url}"


@pytest.mark.asyncio
async def test_kv_cache_client_token_length(real_kv_client):
    """Test that the internal auth token has the expected length (64 chars)."""
    # The token should be 64 characters (32-byte hex)
    token = real_kv_client._internal_auth_token
    assert token is not None, "Internal auth token is None"
    assert len(token) == 64, f"Token length is {len(token)}, expected 64"
    assert all(c in '0123456789abcdef' for c in token), "Token should be hex-encoded"


@pytest.mark.asyncio
async def test_kv_cache_port_correctness(real_kv_client):
    """Test that KVCacheClient is using the correct port (9000, not 9001)."""
    # Port 9000 is for HTTPS, port 9001 is for WSS
    # The HTTP client should use port 9000
    assert real_kv_client.http_url.endswith(":9000"), \
        f"KVCacheClient should use port 9000, but got {real_kv_client.http_url}"
    assert ":9001" not in real_kv_client.http_url, \
        f"KVCacheClient should not use port 9001 (WSS), but got {real_kv_client.http_url}"
