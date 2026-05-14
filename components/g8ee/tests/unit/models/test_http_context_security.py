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
async def test_client_rejected_on_non_exempt_path_without_web_session_id():
    """CLIENT requests to non-exempt paths without web_session_id are rejected."""
    non_exempt_path = "/api/internal/cases"
    
    headers = {
        G8eHeaders.SOURCE_COMPONENT: ComponentName.CLIENT,
        G8eHeaders.CASE_ID: "some-case",
        G8eHeaders.INVESTIGATION_ID: "some-inv",
        G8eHeaders.SYSTEM_FINGERPRINT: "fp-test-123",
    }
    
    request = create_mock_request(non_exempt_path, headers)
    
    with pytest.raises(AuthenticationError) as excinfo:
        await G8eHttpContext.from_request(request)
    
    assert "websession-id" in str(excinfo.value).lower()

@pytest.mark.asyncio
async def test_client_rejected_on_non_exempt_path_without_user_id():
    """CLIENT requests to non-exempt paths without user_id are rejected."""
    non_exempt_path = "/api/internal/cases"
    
    headers = {
        G8eHeaders.SOURCE_COMPONENT: ComponentName.CLIENT,
        G8eHeaders.WEB_SESSION_ID: "sess-123",
        G8eHeaders.CASE_ID: "some-case",
        G8eHeaders.INVESTIGATION_ID: "some-inv",
        G8eHeaders.SYSTEM_FINGERPRINT: "fp-test-123",
    }
    
    request = create_mock_request(non_exempt_path, headers)
    
    with pytest.raises(AuthenticationError) as excinfo:
        await G8eHttpContext.from_request(request)
    
    assert "user-id" in str(excinfo.value).lower()

@pytest.mark.asyncio
async def test_client_allowed_on_exempt_path():
    """Verify that CLIENT can still bypass on legitimate exempt paths."""
    exempt_path = "/api/internal/operators/authenticate"
    
    headers = {
        G8eHeaders.SOURCE_COMPONENT: ComponentName.CLIENT,
        G8eHeaders.CASE_ID: "some-case",
        G8eHeaders.INVESTIGATION_ID: "some-inv",
        G8eHeaders.SYSTEM_FINGERPRINT: "fp-test-456",
    }
    
    request = create_mock_request(exempt_path, headers)
    
    context = await G8eHttpContext.from_request(request)
    assert context.source_component == ComponentName.CLIENT
    assert context.web_session_id is None
    assert context.user_id is None
