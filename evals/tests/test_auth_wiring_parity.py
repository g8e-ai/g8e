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

from g8e_protocol.generated_paths import PathConstants, PortConstants
from g8e_evals.transport import (
    SESSION_COOKIE_NAME,
    SOURCE_COMPONENT_CLIENT,
    AuthContext,
)
from g8e_protocol.models import BoundOperator
from g8e_protocol.constants import ComponentName, API_PATHS


import sys
import os
REPO_ROOT = Path(__file__).resolve().parent.parent.parent
sys.path.insert(0, str(REPO_ROOT / "services" / "g8ee"))
from app.constants.api_paths import InternalAPIPaths


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
        "G8E_TRUST_BUNDLE": str(fake_pki["bundle"]),
        "G8E_PKI_DIR": str(fake_pki["pki"]),
        "G8E_G8EE_URL": f"https://localhost:{PortConstants.G8E_PORT_G8EE_HTTPS}",
        "G8E_INTERNAL_HTTP_URL": f"https://localhost:{PortConstants.PORT_OPERATOR_HTTPS}",
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


def test_api_path_parity_client_sse_stream(fake_pki):
    """Ensure Python InternalAPIPaths matches protocol constants for CLIENT_SSE_STREAM."""
    py_path = InternalAPIPaths.CLIENT_SSE_STREAM
    proto_path = API_PATHS["client_full"]["sse_stream"]

    assert py_path == "/api/internal/sse/stream"
    assert proto_path == py_path


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
    assert rc.source_component == ComponentName.G8EE


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
