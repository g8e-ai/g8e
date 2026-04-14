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
from app.db import DBClient
from app.models.settings import G8eePlatformSettings

@pytest.mark.unit
@pytest.mark.asyncio
class TestDBClientBootstrapAuth:
    async def test_init_prefers_explicit_token(self):
        # Test: Initialize DBClient with explicit token
        explicit_token = "explicit-token-456"
        client = DBClient(ca_cert_path="/mock/ca.crt", internal_auth_token=explicit_token)
        
        # Verify: Explicit token is used
        assert client._internal_auth_token == explicit_token
