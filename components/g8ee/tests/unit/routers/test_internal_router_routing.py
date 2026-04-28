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
Unit tests: Internal Router Path Registration

These tests verify that FastAPI routes are registered with the correct absolute paths.
This addresses the "off-by-prefix" risk by ensuring the router's route paths match
the InternalApiPaths constants exactly.

Tests inspect router routes directly without using TestClient to avoid
starting the actual application or mocking dependencies.
"""

import pytest
from app.routers.internal_router import router as internal_router
from app.constants import InternalApiPaths


@pytest.mark.unit
class TestInternalRouterPathRegistration:
    """Verify internal router routes are registered with correct absolute paths."""

    def test_health_check_path(self):
        """Health check endpoint should be registered at absolute path."""
        route_paths = {route.path for route in internal_router.routes}
        assert InternalApiPaths.G8EE_HEALTH in route_paths

    def test_chat_path_absolute(self):
        """Chat endpoint should be registered at absolute InternalApiPaths.G8EE_CHAT path."""
        route_paths = {route.path for route in internal_router.routes}
        assert InternalApiPaths.G8EE_CHAT in route_paths

    def test_chat_stop_path_absolute(self):
        """Chat stop endpoint should be registered at absolute path."""
        route_paths = {route.path for route in internal_router.routes}
        assert InternalApiPaths.G8EE_CHAT_STOP in route_paths

    def test_investigations_path_absolute(self):
        """Investigations endpoint should be registered at absolute path."""
        route_paths = {route.path for route in internal_router.routes}
        assert InternalApiPaths.G8EE_INVESTIGATIONS in route_paths

    def test_case_path_absolute(self):
        """Case endpoint should be registered at absolute path."""
        route_paths = {route.path for route in internal_router.routes}
        # Case path has a parameter, so check the base path
        assert InternalApiPaths.G8EE_CASE.replace("{case_id}", "test-case") in route_paths or \
               any("cases" in route.path and "{case_id}" in route.path for route in internal_router.routes)

    def test_operator_approval_respond_path_absolute(self):
        """Operator approval respond endpoint should be registered at absolute path."""
        route_paths = {route.path for route in internal_router.routes}
        assert InternalApiPaths.G8EE_OPERATOR_APPROVAL_RESPOND in route_paths

    def test_operator_direct_command_path_absolute(self):
        """Operator direct command endpoint should be registered at absolute path."""
        route_paths = {route.path for route in internal_router.routes}
        assert InternalApiPaths.G8EE_OPERATOR_DIRECT_COMMAND in route_paths

    def test_operators_terminate_path_absolute(self):
        """Operators terminate endpoint should be registered at absolute path."""
        route_paths = {route.path for route in internal_router.routes}
        assert InternalApiPaths.G8EE_OPERATORS_TERMINATE in route_paths

    def test_operators_create_slot_path_absolute(self):
        """Operators create slot endpoint should be registered at absolute path."""
        route_paths = {route.path for route in internal_router.routes}
        assert InternalApiPaths.G8EE_OPERATORS_CREATE_SLOT in route_paths

    def test_operators_bind_path_absolute(self):
        """Operators bind endpoint should be registered at absolute path."""
        route_paths = {route.path for route in internal_router.routes}
        assert InternalApiPaths.G8EE_OPERATORS_BIND in route_paths

    def test_operators_unbind_path_absolute(self):
        """Operators unbind endpoint should be registered at absolute path."""
        route_paths = {route.path for route in internal_router.routes}
        assert InternalApiPaths.G8EE_OPERATORS_UNBIND in route_paths

    def test_auth_generate_key_path_absolute(self):
        """Auth generate key endpoint should be registered at absolute path."""
        route_paths = {route.path for route in internal_router.routes}
        assert InternalApiPaths.G8EE_AUTH_GENERATE_KEY in route_paths

    def test_chat_triage_answer_path_absolute(self):
        """Chat triage answer endpoint should be registered at absolute path."""
        route_paths = {route.path for route in internal_router.routes}
        assert InternalApiPaths.G8EE_CHAT_TRIAGE_ANSWER in route_paths

    def test_chat_triage_skip_path_absolute(self):
        """Chat triage skip endpoint should be registered at absolute path."""
        route_paths = {route.path for route in internal_router.routes}
        assert InternalApiPaths.G8EE_CHAT_TRIAGE_SKIP in route_paths

    def test_chat_triage_timeout_path_absolute(self):
        """Chat triage timeout endpoint should be registered at absolute path."""
        route_paths = {route.path for route in internal_router.routes}
        assert InternalApiPaths.G8EE_CHAT_TRIAGE_TIMEOUT in route_paths

    def test_operators_g8ep_activate_path_absolute(self):
        """Operators g8ep activate endpoint should be registered at absolute path."""
        route_paths = {route.path for route in internal_router.routes}
        assert InternalApiPaths.G8EE_OPERATORS_G8EP_ACTIVATE in route_paths

    def test_operators_g8ep_relaunch_path_absolute(self):
        """Operators g8ep relaunch endpoint should be registered at absolute path."""
        route_paths = {route.path for route in internal_router.routes}
        assert InternalApiPaths.G8EE_OPERATORS_G8EP_RELAUNCH in route_paths

    def test_operators_claim_slot_path_absolute(self):
        """Operators claim slot endpoint should be registered at absolute path."""
        route_paths = {route.path for route in internal_router.routes}
        assert InternalApiPaths.G8EE_OPERATORS_CLAIM_SLOT in route_paths

    def test_operators_device_link_register_path_absolute(self):
        """Operators device link register endpoint should be registered at absolute path."""
        route_paths = {route.path for route in internal_router.routes}
        assert InternalApiPaths.G8EE_OPERATORS_DEVICE_LINK_REGISTER in route_paths

    def test_operators_register_session_path_absolute(self):
        """Operators register session endpoint should be registered at absolute path."""
        route_paths = {route.path for route in internal_router.routes}
        assert InternalApiPaths.G8EE_OPERATORS_REGISTER_SESSION in route_paths

    def test_operators_deregister_session_path_absolute(self):
        """Operators deregister session endpoint should be registered at absolute path."""
        route_paths = {route.path for route in internal_router.routes}
        assert InternalApiPaths.G8EE_OPERATORS_DEREGISTER_SESSION in route_paths

    def test_operators_stop_path_absolute(self):
        """Operators stop endpoint should be registered at absolute path."""
        route_paths = {route.path for route in internal_router.routes}
        assert InternalApiPaths.G8EE_OPERATORS_STOP in route_paths

    def test_operators_listen_session_auth_path_absolute(self):
        """Operators listen session auth endpoint should be registered at absolute path."""
        route_paths = {route.path for route in internal_router.routes}
        assert InternalApiPaths.G8EE_OPERATORS_LISTEN_SESSION_AUTH in route_paths

    def test_operators_update_api_key_path_absolute(self):
        """Operators update api key endpoint should be registered at absolute path."""
        route_paths = {route.path for route in internal_router.routes}
        assert InternalApiPaths.G8EE_OPERATORS_UPDATE_API_KEY in route_paths

    def test_auth_revoke_cert_path_absolute(self):
        """Auth revoke cert endpoint should be registered at absolute path."""
        route_paths = {route.path for route in internal_router.routes}
        assert InternalApiPaths.G8EE_AUTH_REVOKE_CERT in route_paths

    def test_investigation_path_absolute(self):
        """Investigation endpoint should be registered at absolute path."""
        route_paths = {route.path for route in internal_router.routes}
        # Investigation path has a parameter, so check the base path
        assert InternalApiPaths.G8EE_INVESTIGATION.replace("{investigation_id}", "test-inv") in route_paths or \
               any("investigations" in route.path and "{investigation_id}" in route.path for route in internal_router.routes)

    def test_operator_approval_pending_path_absolute(self):
        """Operator approval pending endpoint should be registered at absolute path."""
        route_paths = {route.path for route in internal_router.routes}
        assert InternalApiPaths.G8EE_OPERATOR_APPROVAL_PENDING in route_paths


@pytest.mark.unit
class TestInternalRouterMountedAppPaths:
    """
    Verify routes are reachable at the absolute /api/internal/... path on the
    fully-mounted FastAPI app (catches double-prefix bugs that this file's
    earlier tests miss because they inspect internal_router.routes directly).
    """

    def _mounted_paths(self):
        from fastapi import FastAPI
        app = FastAPI()
        app.include_router(internal_router)
        return {route.path for route in app.routes}

    def test_no_double_prefix_on_any_internal_route(self):
        bad = [p for p in self._mounted_paths() if p.startswith("/api/internal/api/internal")]
        assert not bad, f"Double-prefixed routes detected: {bad}"

    def test_create_slot_mounted_at_absolute_path(self):
        assert InternalApiPaths.G8EE_OPERATORS_CREATE_SLOT in self._mounted_paths()

    def test_authenticate_mounted_at_absolute_path(self):
        assert InternalApiPaths.G8EE_OPERATORS_AUTHENTICATE in self._mounted_paths()

    def test_validate_session_mounted_at_absolute_path(self):
        assert InternalApiPaths.G8EE_OPERATORS_VALIDATE_SESSION in self._mounted_paths()

    def test_refresh_session_mounted_at_absolute_path(self):
        assert InternalApiPaths.G8EE_OPERATORS_REFRESH_SESSION in self._mounted_paths()
