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
Integration tests: G8eHttpContext header round-trip.

These tests exercise the full path from raw HTTP headers through
get_g8e_http_context (app/dependencies.py) to a populated G8eHttpContext
object. Real parsing logic is used; no mocks.

    Segment 1 — happy-path extraction
      All required headers present → fully populated G8eHttpContext.

    Segment 2 — bound_operators JSON round-trip
      X-G8E-Bound-Operators JSON string → typed BoundOperator list.

    Segment 3 — new_case sentinel
      X-G8E-New-Case: true → new_case=True; case_id and investigation_id
      fall back to NEW_CASE_ID when absent.

    Segment 4 — missing required headers raise AuthenticationError
      Each required header independently missing → AuthenticationError.

    Segment 5 — source_component validation
      Unrecognised component name → AuthenticationError.

    Segment 6 — optional header passthrough
      organization_id, task_id, execution_id forwarded correctly.

Real code under test:
    get_g8e_http_context (app/dependencies.py)
    G8eHttpContext (app/models/http_context.py)
    BoundOperator (app/models/http_context.py)

Only request stub is mocked — the header-parsing function itself runs real.
"""

import json
import pytest
from typing import Dict, Any

from app.constants import ComponentName, OperatorStatus, NEW_CASE_ID, G8eHeaders, INTERNAL_AUTH_HEADER
from app.dependencies import get_g8e_http_context
from app.errors import AuthenticationError
from app.models.http_context import BoundOperator, G8eHttpContext


# ---------------------------------------------------------------------------
# Segment 1 — happy-path extraction
# ---------------------------------------------------------------------------


@pytest.mark.asyncio(loop_scope="session")
@pytest.mark.integration
class TestG8eHttpContextHappyPath:
    """All required headers present → fully populated G8eHttpContext."""

    async def test_web_session_id_extracted(self):
        request = _make_request(_base_headers(web_session_id="sess-round-trip-001"))
        ctx = await get_g8e_http_context(request)
        assert ctx.web_session_id == "sess-round-trip-001"

    async def test_user_id_extracted(self):
        request = _make_request(_base_headers(user_id="user-round-trip-001"))
        ctx = await get_g8e_http_context(request)
        assert ctx.user_id == "user-round-trip-001"

    async def test_case_id_extracted(self):
        request = _make_request(_base_headers(case_id="case-round-trip-001"))
        ctx = await get_g8e_http_context(request)
        assert ctx.case_id == "case-round-trip-001"

    async def test_investigation_id_extracted(self):
        request = _make_request(_base_headers(investigation_id="inv-round-trip-001"))
        ctx = await get_g8e_http_context(request)
        assert ctx.investigation_id == "inv-round-trip-001"

    async def test_source_component_parsed_to_enum(self):
        request = _make_request(_base_headers(source_component="g8ed"))
        ctx = await get_g8e_http_context(request)
        assert ctx.source_component == ComponentName.G8ED

    async def test_result_is_g8e_http_context(self):
        request = _make_request(_base_headers())
        ctx = await get_g8e_http_context(request)
        assert isinstance(ctx, G8eHttpContext)

    async def test_new_case_defaults_false(self):
        request = _make_request(_base_headers())
        ctx = await get_g8e_http_context(request)
        assert ctx.new_case is False

    async def test_bound_operators_defaults_empty_list(self):
        request = _make_request(_base_headers(bound_operators="[]"))
        ctx = await get_g8e_http_context(request)
        assert ctx.bound_operators == []


# ---------------------------------------------------------------------------
# Segment 2 — bound_operators JSON round-trip
# ---------------------------------------------------------------------------


@pytest.mark.asyncio(loop_scope="session")
@pytest.mark.integration
class TestBoundOperatorsRoundTrip:
    """X-G8E-Bound-Operators JSON string parses to typed BoundOperator list."""

    async def test_single_bound_operator_parsed(self):
        operators = [
            {
                "operator_id": "op-001",
                "operator_session_id": "sess-op-001",
                "status": "bound",
            }
        ]
        headers = _base_headers(bound_operators=json.dumps(operators))
        ctx = await get_g8e_http_context(_make_request(headers))

        assert len(ctx.bound_operators) == 1
        op = ctx.bound_operators[0]
        assert isinstance(op, BoundOperator)
        assert op.operator_id == "op-001"
        assert op.operator_session_id == "sess-op-001"
        assert op.status == OperatorStatus.BOUND

    async def test_multiple_bound_operators_parsed(self):
        operators = [
            {"operator_id": "op-001", "operator_session_id": "sess-op-001", "status": "bound"},
            {"operator_id": "op-002", "operator_session_id": "sess-op-002", "status": "bound"},
        ]
        headers = _base_headers(bound_operators=json.dumps(operators))
        ctx = await get_g8e_http_context(_make_request(headers))

        assert len(ctx.bound_operators) == 2
        assert ctx.bound_operators[0].operator_id == "op-001"
        assert ctx.bound_operators[1].operator_id == "op-002"

    async def test_operator_without_session_id_allowed(self):
        operators = [{"operator_id": "op-003", "status": "bound"}]
        headers = _base_headers(bound_operators=json.dumps(operators))
        ctx = await get_g8e_http_context(_make_request(headers))

        assert ctx.bound_operators[0].operator_id == "op-003"
        assert ctx.bound_operators[0].operator_session_id is None

    async def test_has_bound_operator_true_when_status_bound(self):
        operators = [{"operator_id": "op-004", "status": "bound"}]
        headers = _base_headers(bound_operators=json.dumps(operators))
        ctx = await get_g8e_http_context(_make_request(headers))
        assert ctx.has_bound_operator() is True

    async def test_has_bound_operator_false_when_empty(self):
        headers = _base_headers(bound_operators="[]")
        ctx = await get_g8e_http_context(_make_request(headers))
        assert ctx.has_bound_operator() is False

    async def test_malformed_json_raises_validation_error(self):
        headers = _base_headers(bound_operators="{not-json}")
        with pytest.raises(Exception):
            await get_g8e_http_context(_make_request(headers))


# ---------------------------------------------------------------------------
# Segment 3 — new_case sentinel
# ---------------------------------------------------------------------------


@pytest.mark.asyncio(loop_scope="session")
@pytest.mark.integration
class TestNewCaseSentinel:
    """X-G8E-New-Case: true triggers new_case=True and NEW_CASE_ID fallbacks."""

    async def test_new_case_true_header_sets_flag(self):
        headers = {
            G8eHeaders.WEB_SESSION_ID:       "sess-nc-001",
            G8eHeaders.USER_ID:          "user-nc-001",
            G8eHeaders.SOURCE_COMPONENT: "g8ed",
            G8eHeaders.NEW_CASE:         "true",
            G8eHeaders.BOUND_OPERATORS:  "[]",
        }
        ctx = await get_g8e_http_context(_make_request(headers))
        assert ctx.new_case is True

    async def test_new_case_missing_case_id_falls_back_to_new_case_id(self):
        headers = {
            G8eHeaders.WEB_SESSION_ID:       "sess-nc-002",
            G8eHeaders.USER_ID:          "user-nc-002",
            G8eHeaders.SOURCE_COMPONENT: "g8ed",
            G8eHeaders.NEW_CASE:         "true",
            G8eHeaders.BOUND_OPERATORS:  "[]",
        }
        ctx = await get_g8e_http_context(_make_request(headers))
        assert ctx.case_id == NEW_CASE_ID

    async def test_new_case_missing_investigation_id_falls_back_to_new_case_id(self):
        headers = {
            G8eHeaders.WEB_SESSION_ID:       "sess-nc-003",
            G8eHeaders.USER_ID:          "user-nc-003",
            G8eHeaders.SOURCE_COMPONENT: "g8ed",
            G8eHeaders.NEW_CASE:         "true",
            G8eHeaders.BOUND_OPERATORS:  "[]",
        }
        ctx = await get_g8e_http_context(_make_request(headers))
        assert ctx.investigation_id == NEW_CASE_ID

    async def test_new_case_false_string_treated_as_false(self):
        headers = _base_headers()
        headers[G8eHeaders.NEW_CASE] = "false"
        ctx = await get_g8e_http_context(_make_request(headers))
        assert ctx.new_case is False

    async def test_new_case_with_explicit_ids_preserved(self):
        headers = {
            G8eHeaders.WEB_SESSION_ID:       "sess-nc-004",
            G8eHeaders.USER_ID:          "user-nc-004",
            G8eHeaders.SOURCE_COMPONENT: "g8ed",
            G8eHeaders.NEW_CASE:         "true",
            G8eHeaders.CASE_ID:          "case-provided",
            G8eHeaders.INVESTIGATION_ID: "inv-provided",
            G8eHeaders.BOUND_OPERATORS:  "[]",
        }
        ctx = await get_g8e_http_context(_make_request(headers))
        assert ctx.case_id == "case-provided"
        assert ctx.investigation_id == "inv-provided"


# ---------------------------------------------------------------------------
# Segment 4 — missing required headers raise AuthenticationError
# ---------------------------------------------------------------------------


@pytest.mark.asyncio(loop_scope="session")
@pytest.mark.integration
class TestMissingRequiredHeaders:
    """Each required header independently missing → AuthenticationError."""

    async def test_missing_session_id_raises(self):
        headers = _base_headers()
        del headers[G8eHeaders.WEB_SESSION_ID]
        with pytest.raises(AuthenticationError):
            await get_g8e_http_context(_make_request(headers))

    async def test_missing_user_id_raises(self):
        headers = _base_headers()
        del headers[G8eHeaders.USER_ID]
        with pytest.raises(AuthenticationError):
            await get_g8e_http_context(_make_request(headers))

    async def test_missing_source_component_raises(self):
        headers = _base_headers()
        del headers[G8eHeaders.SOURCE_COMPONENT]
        with pytest.raises(AuthenticationError):
            await get_g8e_http_context(_make_request(headers))

    async def test_missing_case_id_without_new_case_raises(self):
        headers = _base_headers()
        del headers[G8eHeaders.CASE_ID]
        with pytest.raises(AuthenticationError):
            await get_g8e_http_context(_make_request(headers))

    async def test_missing_investigation_id_without_new_case_raises(self):
        headers = _base_headers()
        del headers[G8eHeaders.INVESTIGATION_ID]
        with pytest.raises(AuthenticationError):
            await get_g8e_http_context(_make_request(headers))


# ---------------------------------------------------------------------------
# Segment 5 — source_component validation
# ---------------------------------------------------------------------------


@pytest.mark.asyncio(loop_scope="session")
@pytest.mark.integration
class TestSourceComponentValidation:
    """Unrecognised component name → AuthenticationError."""

    async def test_unrecognised_component_raises(self):
        headers = _base_headers(source_component="totally-unknown-service")
        with pytest.raises(AuthenticationError):
            await get_g8e_http_context(_make_request(headers))

    async def test_g8ee_component_accepted(self):
        headers = _base_headers(source_component="g8ee")
        ctx = await get_g8e_http_context(_make_request(headers))
        assert ctx.source_component == ComponentName.G8EE

    async def test_g8ed_component_accepted(self):
        headers = _base_headers(source_component="g8ed")
        ctx = await get_g8e_http_context(_make_request(headers))
        assert ctx.source_component == ComponentName.G8ED


# ---------------------------------------------------------------------------
# Segment 6 — optional header passthrough
# ---------------------------------------------------------------------------


@pytest.mark.asyncio(loop_scope="session")
@pytest.mark.integration
class TestOptionalHeaderPassthrough:
    """Optional headers forwarded correctly when present."""

    async def test_organization_id_forwarded(self):
        headers = _base_headers()
        headers[G8eHeaders.ORGANIZATION_ID] = "org-passthrough-001"
        ctx = await get_g8e_http_context(_make_request(headers))
        assert ctx.organization_id == "org-passthrough-001"

    async def test_organization_id_none_when_absent(self):
        headers = _base_headers()
        ctx = await get_g8e_http_context(_make_request(headers))
        assert ctx.organization_id is None

    async def test_task_id_forwarded(self):
        headers = _base_headers()
        headers[G8eHeaders.TASK_ID] = "task-passthrough-001"
        ctx = await get_g8e_http_context(_make_request(headers))
        assert ctx.task_id == "task-passthrough-001"

    async def test_task_id_none_when_absent(self):
        headers = _base_headers()
        ctx = await get_g8e_http_context(_make_request(headers))
        assert ctx.task_id is None

    async def test_request_id_forwarded_when_present(self):
        headers = _base_headers()
        headers[G8eHeaders.EXECUTION_ID] = "req-passthrough-001"
        ctx = await get_g8e_http_context(_make_request(headers))
        assert ctx.execution_id == "req-passthrough-001"

    async def test_request_id_auto_generated_when_absent(self):
        headers = _base_headers()
        ctx = await get_g8e_http_context(_make_request(headers))
        assert ctx.execution_id is not None
        assert len(ctx.execution_id) > 0


# ---------------------------------------------------------------------------
# Helper functions for test data creation
# ---------------------------------------------------------------------------

def _base_headers(
    web_session_id: str = "test-web-session",
    user_id: str = "test-user-id", 
    case_id: str = "test-case-id",
    investigation_id: str = "test-investigation-id",
    source_component: str = "g8ed",
    bound_operators: str = "[]",
    new_case: str = "false",
    **kwargs
) -> Dict[str, str]:
    """Create base headers with all required g8e headers."""
    headers = {
        G8eHeaders.WEB_SESSION_ID: web_session_id,
        G8eHeaders.USER_ID: user_id,
        G8eHeaders.CASE_ID: case_id,
        G8eHeaders.INVESTIGATION_ID: investigation_id,
        G8eHeaders.SOURCE_COMPONENT: source_component,
        G8eHeaders.BOUND_OPERATORS: bound_operators,
        G8eHeaders.NEW_CASE: new_case,
        INTERNAL_AUTH_HEADER: "test-auth-token",
    }
    
    # Add any additional headers
    headers.update(kwargs)
    return headers


def _make_request(headers: Dict[str, str]) -> Any:
    """Create a mock request object with the given headers."""
    from unittest.mock import Mock
    
    request = Mock()
    # Convert all header keys to lowercase to match FastAPI's behavior
    request.headers = {k.lower(): v for k, v in headers.items()}
    return request
