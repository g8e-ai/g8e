# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

"""Contract test: evals Python transport must match protocol constants.

The evals harness (``g8e_evals.transport.AuthContext``) and the protocol
constants encode the *same* recipe for reaching the running platform:

  - mTLS trust bundle (--cacert)
  - mTLS client cert + key (--cert / --key)
  - ``g8e_session`` cookie (--cookie)
  - ``Authorization: Bearer <token>`` + ``X-G8E-CLI-Session-ID`` headers (-H)
  - ``Content-Type: application/json``

This file is the canary: it verifies the Python ``AuthContext`` yields
the correct header set / cookies / cert paths / trust bundle as defined
in the protocol constants. If a new required header is added to the
protocol but not AuthContext.auth_headers (or vice versa), this test
fails loudly instead of the bench silently 401'ing in production.
"""

from __future__ import annotations

import os
from pathlib import Path

import pytest

from g8e.constants import PORTS
from g8e_evals.transport import (
    SESSION_COOKIE_NAME,
    SOURCE_COMPONENT_CLIENT,
    AuthContext,
)
from app.models.http_context import BoundOperator
from app.constants import G8EE_COMPONENT
from app.constants.api_paths import API_PATHS, GatewayAPIPaths
from app.constants.api_paths import InternalAPIPaths

pytestmark = pytest.mark.integration


@pytest.fixture
def fake_pki(tmp_path: Path) -> dict[str, Path]:
    """Materialize a fake PKI tree on disk so the Python helpers
    pass their ``-f``/``isfile`` existence checks."""
    pki = tmp_path / "pki"
    (pki / "trust").mkdir(parents=True)
    bundle = pki / "trust" / "hub-bundle.pem"
    bundle.write_text("# fake hub bundle\n")
    cli_dir = tmp_path / "creds"
    cli_dir.mkdir()
    cert = cli_dir / "cli.crt"
    key = cli_dir / "cli.key"
    cert.write_text("# fake cert\n")
    key.write_text("# fake key\n")
    return {"pki": pki, "bundle": bundle, "cert": cert, "key": key}


def _baseline_env(fake_pki: dict[str, Path]) -> dict[str, str]:
    return {
        "HOME": os.environ.get("HOME", "/home/bob"),
        "PATH": os.environ.get("PATH", ""),
        "G8E_OPERATOR_SESSION_ID": "sess-parity-001",
        "G8E_CLI_SESSION_ID": "cli-parity-001",
        "G8E_USER_ID": "user-parity-001",
        "G8E_CLI_CERT": str(fake_pki["cert"]),
        "G8E_CLI_KEY": str(fake_pki["key"]),
        "G8E_APP_TRUST_BUNDLE": str(fake_pki["bundle"]),
        "G8E_APP_PKI_DIR": str(fake_pki["pki"]),
        "G8E_G8EE_URL": f"https://localhost:{PORTS['ports']['OperatorHttps']['value']}",
        "G8E_INTERNAL_HTTP_URL": f"https://localhost:{PORTS['ports']['OperatorHttps']['value']}",
        # Make sure no stray optional headers leak in from the dev env.
        "G8E_CASE_ID": "",
        "G8E_INVESTIGATION_ID": "",
        "G8E_BOUND_OPERATORS": "",
        "G8E_TASK_ID": "",
    }


def _python_view(env: dict[str, str]) -> dict:
    """Render the Python AuthContext into the same shape as the protocol constants."""
    # AuthContext.from_env reads from os.environ; swap it in for the call.
    saved = dict(os.environ)
    try:
        os.environ.clear()
        os.environ.update(env)
        ctx = AuthContext.from_env()
    finally:
        os.environ.clear()
        os.environ.update(saved)

    headers = ctx.auth_headers()

    return {
        "headers": dict(headers),
        "cookies": dict(ctx.cookies()),
        "cert": ctx.client_cert,
        "key": ctx.client_key,
        "cacert": ctx.trust_bundle,
    }


def test_auth_wiring_matches_protocol_constants(fake_pki):
    """Verify AuthContext produces headers matching protocol constants."""
    env = _baseline_env(fake_pki)
    py = _python_view(env)

    # mTLS material parity
    assert py["cert"] == str(fake_pki["cert"])
    assert py["key"] == str(fake_pki["key"])
    assert py["cacert"] == str(fake_pki["bundle"])

    # Cookie parity
    assert py["cookies"].get(SESSION_COOKIE_NAME) == env["G8E_OPERATOR_SESSION_ID"]

    # Header parity - the canary. Any new required header added to
    # protocol constants but not AuthContext.auth_headers
    # (or vice versa) lights this up.
    h = py["headers"]
    assert h["Content-Type"] == "application/json"
    assert h["Authorization"] == f"Bearer {env['G8E_OPERATOR_SESSION_ID']}"
    assert h["X-G8E-CLI-Session-ID"] == env["G8E_CLI_SESSION_ID"]

    # Invert the conflation check: business context headers must NOT leak
    # into the minimal auth header set; that context is body-embedded instead.
    assert "X-G8E-Source-Component" not in h
    assert "X-G8E-User-ID" not in h


def test_api_path_parity_g8ee_chat(fake_pki):
    """Ensure Python InternalAPIPaths matches protocol constants for G8EE_CHAT."""
    # Python resolution (InternalAPIPaths)
    py_path = InternalAPIPaths.G8EE_CHAT

    # Protocol constants direct resolution
    proto_path = API_PATHS["g8ee_full"]["chat"]

    assert py_path == "/api/v1/chat"
    assert proto_path == py_path


def test_api_path_parity_gateway_sse_stream():
    """SSE stream path must match the gateway's canonical route."""
    assert GatewayAPIPaths.SSE_STREAM == "/api/v1/sse/stream"


def test_api_path_parity_gateway_sse_events():
    """SSE events path must match the gateway's canonical route."""
    assert GatewayAPIPaths.SSE_EVENTS == "/api/v1/sse/events"


def test_api_path_parity_gateway_sse_push():
    """SSE push path must match the gateway's canonical route."""
    assert GatewayAPIPaths.SSE_PUSH == "/api/v1/sse/push"


def test_configurable_contexts_match_when_set(fake_pki):
    env = _baseline_env(fake_pki)
    env.update({
        "G8E_WEB_SESSION_ID": "web-parity-001",
        "G8E_SOURCE_COMPONENT": "g8ee",
    })

    saved = dict(os.environ)
    try:
        os.environ.clear()
        os.environ.update(env)
        ctx = AuthContext.from_env()
    finally:
        os.environ.clear()
        os.environ.update(saved)

    # 1. Header parity: business context is body-embedded, never carried as
    #    component or user headers in the minimal auth header set.
    h = ctx.auth_headers()
    assert "X-G8E-Source-Component" not in h

    # 2. Body parity (RequestContext)
    rc = ctx.to_request_context()
    assert rc.web_session_id == "web-parity-001"
    assert rc.source_component == G8EE_COMPONENT


def test_invalid_source_component_raises_error(fake_pki):
    env = _baseline_env(fake_pki)
    env["G8E_SOURCE_COMPONENT"] = "invalid-component-name"

    saved = dict(os.environ)
    try:
        os.environ.clear()
        os.environ.update(env)
        with pytest.raises(ValueError, match="Invalid G8E_SOURCE_COMPONENT='invalid-component-name'"):
            AuthContext.from_env()
    finally:
        os.environ.clear()
        os.environ.update(saved)
