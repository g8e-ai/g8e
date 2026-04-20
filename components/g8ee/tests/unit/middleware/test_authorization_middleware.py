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

from unittest.mock import AsyncMock, MagicMock

import pytest
from fastapi import Request, Response

from app.errors import AuthorizationError, ServiceUnavailableError
from app.middleware.authorization import AuthorizationMiddleware
from tests.fakes.factories import (
    build_case_model,
    build_g8e_http_context,
    create_investigation_data,
)

pytestmark = [pytest.mark.unit, pytest.mark.asyncio(loop_scope="session")]


class TestAuthorizationMiddleware:

    @pytest.fixture
    def middleware(self):
        app = MagicMock()
        return AuthorizationMiddleware(app)

    @pytest.fixture
    def mock_request(self):
        request = MagicMock(spec=Request)
        request.url = MagicMock()
        request.state = MagicMock()
        request.client = MagicMock()
        request.client.host = "127.0.0.1"
        request.method = "GET"
        request.path_params = {}
        return request

    @pytest.fixture
    def mock_call_next(self):
        async def call_next(request):
            return Response(content="OK", status_code=200)
        return call_next

    @pytest.mark.parametrize("path", [
        "/health",
        "/health/details",
        "/api/internal/health",
        "/docs",
        "/openapi.json",
        "/redoc",
    ])
    async def test_exempt_paths_bypass_auth(self, middleware, mock_request, mock_call_next, path):
        mock_request.url.path = path
        response = await middleware.dispatch(mock_request, mock_call_next)
        assert response.status_code == 200

    async def test_user_id_mismatch_rejected(self, middleware, mock_request, mock_call_next):
        mock_request.url.path = "/investigations"
        mock_request.query_params = {"user_id": "attacker-999"}
        mock_request.state.g8e_context = build_g8e_http_context(
            web_session_id="session-123",
            user_id="user-456",
            organization_id="org-789",
        )

        with pytest.raises(AuthorizationError) as exc_info:
            await middleware.dispatch(mock_request, mock_call_next)

        assert exc_info.value.get_http_status() == 403
        assert "Cannot access other user's resources" in str(exc_info.value)

    async def test_user_id_match_allowed(self, middleware, mock_request, mock_call_next):
        mock_request.url.path = "/investigations"
        mock_request.query_params = {"user_id": "user-456"}
        mock_request.state.g8e_context = build_g8e_http_context(
            web_session_id="session-123",
            user_id="user-456",
            organization_id="org-789",
        )

        response = await middleware.dispatch(mock_request, mock_call_next)
        assert response.status_code == 200

    async def test_user_id_in_path_params_validated(self, middleware, mock_request, mock_call_next):
        mock_request.url.path = "/investigations/user/attacker-999"
        mock_request.query_params = {}
        mock_request.path_params = {"user_id": "attacker-999"}
        mock_request.state.g8e_context = build_g8e_http_context(
            web_session_id="session-123",
            user_id="user-456",
            organization_id="org-789",
        )

        with pytest.raises(AuthorizationError) as exc_info:
            await middleware.dispatch(mock_request, mock_call_next)

        assert exc_info.value.get_http_status() == 403
        assert "Cannot access other user's resources" in str(exc_info.value)

    async def test_no_user_id_in_request_allowed(self, middleware, mock_request, mock_call_next):
        mock_request.url.path = "/investigations"
        mock_request.query_params = {}
        mock_request.state.g8e_context = build_g8e_http_context(
            web_session_id="session-123",
            user_id="user-456",
            organization_id="org-789",
        )

        response = await middleware.dispatch(mock_request, mock_call_next)
        assert response.status_code == 200

    async def test_is_user_scoped_path_investigations(self, middleware):
        assert middleware._is_user_scoped_path("/investigations/123") is True

    async def test_is_user_scoped_path_chat(self, middleware):
        assert middleware._is_user_scoped_path("/chat/789") is True

    async def test_is_user_scoped_path_non_scoped(self, middleware):
        assert middleware._is_user_scoped_path("/api/public/info") is False

    async def test_is_user_scoped_path_exact_prefix_match(self, middleware):
        assert middleware._is_user_scoped_path("/investigations") is True
        assert middleware._is_user_scoped_path("/chat") is True

    async def test_extract_g8e_context_present(self, middleware, mock_request):
        g8e_context = build_g8e_http_context(
            web_session_id="session-123",
            user_id="user-456",
            organization_id="org-789",
        )
        mock_request.state.g8e_context = g8e_context
        assert middleware._extract_g8e_context(mock_request) == g8e_context

    async def test_extract_g8e_context_missing(self, middleware, mock_request):
        delattr(mock_request.state, "g8e_context")
        assert middleware._extract_g8e_context(mock_request) is None

    async def test_request_without_g8e_context_allowed(self, middleware, mock_request, mock_call_next):
        mock_request.url.path = "/investigations"
        mock_request.query_params = {}
        delattr(mock_request.state, "g8e_context")

        response = await middleware.dispatch(mock_request, mock_call_next)
        assert response.status_code == 200

    async def test_investigation_ownership_match_allowed(self, middleware, mock_request, mock_call_next):
        mock_request.url.path = "/investigations/inv-123"
        mock_request.query_params = {"investigation_id": "inv-123"}
        mock_request.state.g8e_context = build_g8e_http_context(
            web_session_id="session-123",
            user_id="user-456",
            organization_id="org-789",
        )
        investigation = create_investigation_data(
            investigation_id="inv-123",
            case_id="case-789",
            user_id="user-456",
            sentinel_mode=False,
        )
        mock_inv_service = MagicMock()
        mock_inv_service.get_investigation = AsyncMock(return_value=investigation)
        mock_request.app.state.investigation_service = mock_inv_service

        response = await middleware.dispatch(mock_request, mock_call_next)
        assert response.status_code == 200

    async def test_investigation_ownership_match_via_path_param(self, middleware, mock_request, mock_call_next):
        mock_request.url.path = "/investigations/inv-123"
        mock_request.query_params = {}
        mock_request.path_params = {"investigation_id": "inv-123"}
        mock_request.state.g8e_context = build_g8e_http_context(
            web_session_id="session-123",
            user_id="user-456",
            organization_id="org-789",
        )
        investigation = create_investigation_data(
            investigation_id="inv-123",
            case_id="case-789",
            user_id="user-456",
            sentinel_mode=False,
        )
        mock_inv_service = MagicMock()
        mock_inv_service.get_investigation = AsyncMock(return_value=investigation)
        mock_request.app.state.investigation_service = mock_inv_service

        response = await middleware.dispatch(mock_request, mock_call_next)
        assert response.status_code == 200

    async def test_investigation_ownership_mismatch_rejected(self, middleware, mock_request, mock_call_next):
        mock_request.url.path = "/investigations/inv-123"
        mock_request.query_params = {"investigation_id": "inv-123"}
        mock_request.state.g8e_context = build_g8e_http_context(
            web_session_id="session-123",
            user_id="user-456",
            organization_id="org-789",
        )
        investigation = create_investigation_data(
            investigation_id="inv-123",
            case_id="case-789",
            user_id="other-user",
            sentinel_mode=False,
        )
        mock_inv_service = MagicMock()
        mock_inv_service.get_investigation = AsyncMock(return_value=investigation)
        mock_request.app.state.investigation_service = mock_inv_service

        with pytest.raises(AuthorizationError) as exc_info:
            await middleware.dispatch(mock_request, mock_call_next)

        assert exc_info.value.get_http_status() == 403
        assert "Investigation not found or access denied" in str(exc_info.value)

    async def test_investigation_not_found_rejected(self, middleware, mock_request, mock_call_next):
        mock_request.url.path = "/investigations/inv-missing"
        mock_request.query_params = {"investigation_id": "inv-missing"}
        mock_request.state.g8e_context = build_g8e_http_context(
            web_session_id="session-123",
            user_id="user-456",
            organization_id="org-789",
        )
        mock_inv_service = MagicMock()
        mock_inv_service.get_investigation = AsyncMock(return_value=None)
        mock_request.app.state.investigation_service = mock_inv_service

        with pytest.raises(AuthorizationError) as exc_info:
            await middleware.dispatch(mock_request, mock_call_next)

        assert exc_info.value.get_http_status() == 403
        assert "Investigation not found or access denied" in str(exc_info.value)

    async def test_investigation_service_unavailable_raises(self, middleware, mock_request, mock_call_next):
        mock_request.url.path = "/investigations/inv-123"
        mock_request.query_params = {"investigation_id": "inv-123"}
        mock_request.state.g8e_context = build_g8e_http_context(
            web_session_id="session-123",
            user_id="user-456",
            organization_id="org-789",
        )
        # Mock investigation_service but make it None to trigger ServiceUnavailableError
        mock_request.app.state.investigation_service = None

        with pytest.raises(ServiceUnavailableError) as exc_info:
            await middleware.dispatch(mock_request, mock_call_next)

        assert exc_info.value.get_http_status() == 503

    async def test_case_ownership_match_allowed(self, middleware, mock_request, mock_call_next):
        mock_request.url.path = "/investigations"
        mock_request.query_params = {"case_id": "case-789"}
        mock_request.state.g8e_context = build_g8e_http_context(
            web_session_id="session-123",
            user_id="user-456",
            organization_id="org-789",
        )
        case = build_case_model(case_id="case-789", title="Test case", user_id="user-456")
        mock_db = MagicMock()
        mock_db.get_case = AsyncMock(return_value=case)
        mock_request.app.state.db_service = mock_db

        response = await middleware.dispatch(mock_request, mock_call_next)
        assert response.status_code == 200

    async def test_case_ownership_match_via_path_param(self, middleware, mock_request, mock_call_next):
        mock_request.url.path = "/investigations/case-789/details"
        mock_request.query_params = {}
        mock_request.path_params = {"case_id": "case-789"}
        mock_request.state.g8e_context = build_g8e_http_context(
            web_session_id="session-123",
            user_id="user-456",
            organization_id="org-789",
        )
        case = build_case_model(case_id="case-789", title="Test case", user_id="user-456")
        mock_db = MagicMock()
        mock_db.get_case = AsyncMock(return_value=case)
        mock_request.app.state.db_service = mock_db

        response = await middleware.dispatch(mock_request, mock_call_next)
        assert response.status_code == 200

    async def test_case_ownership_mismatch_rejected(self, middleware, mock_request, mock_call_next):
        mock_request.url.path = "/investigations"
        mock_request.query_params = {"case_id": "case-789"}
        mock_request.state.g8e_context = build_g8e_http_context(
            web_session_id="session-123",
            user_id="user-456",
            organization_id="org-789",
        )
        case = build_case_model(case_id="case-789", title="Test case", user_id="other-user")
        mock_db = MagicMock()
        mock_db.get_case = AsyncMock(return_value=case)
        mock_request.app.state.db_service = mock_db

        with pytest.raises(AuthorizationError) as exc_info:
            await middleware.dispatch(mock_request, mock_call_next)

        assert exc_info.value.get_http_status() == 403
        assert "Case not found or access denied" in str(exc_info.value)

    async def test_case_not_found_rejected(self, middleware, mock_request, mock_call_next):
        mock_request.url.path = "/investigations"
        mock_request.query_params = {"case_id": "case-missing"}
        mock_request.state.g8e_context = build_g8e_http_context(
            web_session_id="session-123",
            user_id="user-456",
            organization_id="org-789",
        )
        mock_db = MagicMock()
        mock_db.get_case = AsyncMock(return_value=None)
        mock_request.app.state.db_service = mock_db

        with pytest.raises(AuthorizationError) as exc_info:
            await middleware.dispatch(mock_request, mock_call_next)

        assert exc_info.value.get_http_status() == 403
        assert "Case not found or access denied" in str(exc_info.value)

    async def test_case_db_service_unavailable_raises(self, middleware, mock_request, mock_call_next):
        mock_request.url.path = "/investigations"
        mock_request.query_params = {"case_id": "case-789"}
        mock_request.state.g8e_context = build_g8e_http_context(
            web_session_id="session-123",
            user_id="user-456",
            organization_id="org-789",
        )
        # Mock db_service but make it None to trigger ServiceUnavailableError
        mock_request.app.state.db_service = None

        with pytest.raises(ServiceUnavailableError) as exc_info:
            await middleware.dispatch(mock_request, mock_call_next)

        assert exc_info.value.get_http_status() == 503

    async def test_null_client_does_not_crash_on_user_id_violation(self, middleware, mock_request, mock_call_next):
        mock_request.url.path = "/investigations"
        mock_request.query_params = {"user_id": "attacker-999"}
        mock_request.client
        mock_request.state.g8e_context = build_g8e_http_context(
            web_session_id="session-123",
            user_id="user-456",
            organization_id="org-789",
        )

        with pytest.raises(AuthorizationError) as exc_info:
            await middleware.dispatch(mock_request, mock_call_next)

        assert exc_info.value.get_http_status() == 403

    async def test_null_client_does_not_crash_on_investigation_violation(self, middleware, mock_request, mock_call_next):
        mock_request.url.path = "/investigations/inv-123"
        mock_request.query_params = {"investigation_id": "inv-123"}
        mock_request.client
        mock_request.state.g8e_context = build_g8e_http_context(
            web_session_id="session-123",
            user_id="user-456",
            organization_id="org-789",
        )
        investigation = create_investigation_data(
            investigation_id="inv-123",
            case_id="case-789",
            user_id="other-user",
            sentinel_mode=False,
        )
        mock_inv_service = MagicMock()
        mock_inv_service.get_investigation = AsyncMock(return_value=investigation)
        mock_request.app.state.investigation_service = mock_inv_service

        with pytest.raises(AuthorizationError) as exc_info:
            await middleware.dispatch(mock_request, mock_call_next)

        assert exc_info.value.get_http_status() == 403

    async def test_null_client_does_not_crash_on_case_violation(self, middleware, mock_request, mock_call_next):
        mock_request.url.path = "/investigations"
        mock_request.query_params = {"case_id": "case-789"}
        mock_request.client
        mock_request.state.g8e_context = build_g8e_http_context(
            web_session_id="session-123",
            user_id="user-456",
            organization_id="org-789",
        )
        case = build_case_model(case_id="case-789", title="Test case", user_id="other-user")
        mock_db = MagicMock()
        mock_db.get_case = AsyncMock(return_value=case)
        mock_request.app.state.db_service = mock_db

        with pytest.raises(AuthorizationError) as exc_info:
            await middleware.dispatch(mock_request, mock_call_next)

        assert exc_info.value.get_http_status() == 403

    async def test_g8e_context_present_but_non_scoped_path_skips_checks(self, middleware, mock_request, mock_call_next):
        mock_request.url.path = "/api/internal/some-endpoint"
        mock_request.query_params = {"user_id": "attacker-999", "case_id": "case-xyz"}
        mock_request.state.g8e_context = build_g8e_http_context(
            web_session_id="session-123",
            user_id="user-456",
            organization_id="org-789",
        )

        response = await middleware.dispatch(mock_request, mock_call_next)
        assert response.status_code == 200
