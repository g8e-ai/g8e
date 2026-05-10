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
from fastapi import Request
from app.models.http_context import G8eHttpContext
from app.constants.status import ComponentName
from app.constants.headers import G8eHeaders
from app.errors import AuthenticationError
from unittest.mock import MagicMock

pytestmark = pytest.mark.unit

def create_mock_request(path: str, headers: dict):
    request = MagicMock(spec=Request)
    request.url = MagicMock()
    request.url.path = path
    request.method = "POST"
    # FastAPI headers are case-insensitive, but Mock doesn't handle that automatically
    # unless we simulate it. G8eHttpContext.from_request uses .lower() on the keys.
    request.headers = {k.lower(): v for k, v in headers.items()}
    return request

@pytest.mark.asyncio
async def test_g8ed_bypass_security_risk_reproduction():
    """
    Verify that G8ED CANNOT bypass web_session_id/user_id on non-exempt paths.
    This test previously passed (vulnerability), now it should raise AuthenticationError.
    """
    # A path that SHOULD NOT be exempt (e.g., chat)
    vulnerable_path = "/api/internal/chat"
    
    headers = {
        G8eHeaders.SOURCE_COMPONENT: ComponentName.G8ED,
        G8eHeaders.CASE_ID: "some-case",
        G8eHeaders.INVESTIGATION_ID: "some-inv",
        G8eHeaders.SYSTEM_FINGERPRINT: "fp-test-123",
        # web_session_id and user_id are MISSING
    }
    
    request = create_mock_request(vulnerable_path, headers)
    
    # FIXED BEHAVIOR: This should now raise AuthenticationError
    with pytest.raises(AuthenticationError) as excinfo:
        await G8eHttpContext.from_request(request)
    
    assert "header is required for all internal requests" in str(excinfo.value)

@pytest.mark.asyncio
async def test_g8ed_allowed_on_exempt_path():
    """Verify that G8ED can still bypass on legitimate exempt paths."""
    exempt_path = "/api/internal/operators/authenticate"
    
    headers = {
        G8eHeaders.SOURCE_COMPONENT: ComponentName.G8ED,
        G8eHeaders.CASE_ID: "some-case",
        G8eHeaders.INVESTIGATION_ID: "some-inv",
        G8eHeaders.SYSTEM_FINGERPRINT: "fp-test-456",
    }
    
    request = create_mock_request(exempt_path, headers)
    
    context = await G8eHttpContext.from_request(request)
    assert context.source_component == ComponentName.G8ED
    assert context.web_session_id is None
    assert context.user_id is None
