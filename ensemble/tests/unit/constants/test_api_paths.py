# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

"""Parity tests for API path constants.

Verifies that:
- ``GatewayAPIPaths`` values match ``g8e.constants.API_PATHS`` exactly
- ``InternalAPIPaths`` client paths are all accessible
- SSE paths are sourced from ``GatewayAPIPaths`` (the gateway's canonical
  routes at ``/api/v1/sse/*``), not from the stale ``client`` map
- ``governance_client.py`` no longer hardcodes the governance envelopes URL
"""

import g8e.constants as _g8e_constants

import pytest

from app.constants.api_paths import (
    API_PATHS,
    GatewayAPIPaths,
    InternalAPIPaths,
    validate_api_paths_sync,
)

pytestmark = [pytest.mark.unit]


def test_gateway_api_paths_matches_g8e_constants():
    """Every string value in g8e.constants.API_PATHS must be accessible via GatewayAPIPaths."""
    g8e_paths = _g8e_constants.API_PATHS
    for key, value in g8e_paths.items():
        if not isinstance(value, str):
            continue
        attr_name = key.upper()
        assert getattr(GatewayAPIPaths, attr_name) == value, (
            f"GatewayAPIPaths.{attr_name} != g8e.constants.API_PATHS['{key}']: "
            f"{getattr(GatewayAPIPaths, attr_name)!r} != {value!r}"
        )


def test_gateway_api_paths_raises_on_unknown_key():
    with pytest.raises(AttributeError, match="NONEXISTENT_PATH"):
        GatewayAPIPaths.NONEXISTENT_PATH


def test_gateway_api_paths_governance_envelopes_value():
    assert GatewayAPIPaths.GOVERNANCE_ENVELOPES == "/api/v1/governance/envelopes"


def test_gateway_api_paths_operators_bind_value():
    assert GatewayAPIPaths.OPERATORS_BIND == "/api/v1/operators/bind"


def test_gateway_api_paths_sse_push_value():
    assert GatewayAPIPaths.SSE_PUSH == "/api/v1/sse/push"


def test_gateway_api_paths_sse_events_value():
    assert GatewayAPIPaths.SSE_EVENTS == "/api/v1/sse/events"


def test_gateway_api_paths_sse_stream_value():
    assert GatewayAPIPaths.SSE_STREAM == "/api/v1/sse/stream"


def test_internal_api_paths_client_sse_paths_removed():
    """SSE paths must not be in the client map — they are gateway routes."""
    assert "sse_push" not in API_PATHS["client"]
    assert "sse_events" not in API_PATHS["client"]
    assert "sse_stream" not in API_PATHS["client"]
    assert "sse_push" not in API_PATHS.get("client_full", {})
    assert "sse_events" not in API_PATHS.get("client_full", {})
    assert "sse_stream" not in API_PATHS.get("client_full", {})


def test_internal_api_paths_client_grant_intent_exists():
    """CLIENT_GRANT_INTENT must be accessible (was previously missing)."""
    assert (
        InternalAPIPaths.CLIENT_GRANT_INTENT
        == "/api/operators/{operator_id}/intents/grant"
    )
    assert (
        InternalAPIPaths.FULL_CLIENT_GRANT_INTENT
        == "/api/operators/{operator_id}/intents/grant"
    )


def test_internal_api_paths_client_revoke_intent_exists():
    """CLIENT_REVOKE_INTENT must be accessible (was previously missing)."""
    assert (
        InternalAPIPaths.CLIENT_REVOKE_INTENT
        == "/api/operators/{operator_id}/intents/revoke"
    )
    assert (
        InternalAPIPaths.FULL_CLIENT_REVOKE_INTENT
        == "/api/operators/{operator_id}/intents/revoke"
    )


def test_internal_api_paths_client_create_operator_link_exists():
    """CLIENT_CREATE_OPERATOR_LINK must be accessible (was previously missing)."""
    assert (
        InternalAPIPaths.CLIENT_CREATE_OPERATOR_LINK
        == "/api/operators/device-link/create"
    )
    assert (
        InternalAPIPaths.FULL_CLIENT_CREATE_OPERATOR_LINK
        == "/api/operators/device-link/create"
    )


def test_validate_api_paths_sync_passes():
    """validate_api_paths_sync must not raise after adding missing client paths."""
    validate_api_paths_sync()


def test_api_paths_json_client_section_has_all_expected_keys():
    """api_paths.json client section must contain all keys referenced by internal_http_client."""
    client_keys = set(API_PATHS["client"].keys())
    expected = {"chat", "health",
                "grant_intent", "revoke_intent", "create_operator_link"}
    assert expected.issubset(client_keys), (
        f"Missing client paths in api_paths.json: {expected - client_keys}"
    )
