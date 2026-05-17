"""Contract test: evals Python transport must match shell-side auth wiring.

The evals harness (``g8e_evals.transport.AuthContext``) and the shell
helpers in ``scripts/cmd/common.sh`` (``_build_protocol_curl_args`` +
``_append_legacy_g8e_context_headers``) encode the *same* recipe for reaching
the running platform:

  - mTLS trust bundle (--cacert)
  - mTLS client cert + key (--cert / --key)
  - ``g8e_session`` cookie (--cookie)
  - ``Authorization: Bearer <token>`` + ``X-G8E-CLI-Session-ID`` headers (-H)
  - ``Content-Type: application/json``

This file is the canary: it spins up the shell helper under bash with a
controlled environment, captures the curl argv it would produce, then
asserts the Python ``AuthContext`` yields the same header set / cookies
/ cert paths / trust bundle. If a new required header is added on one
side and not the other (the exact failure mode described in the
``evals.sh`` divergence note), this test fails loudly instead of the
bench silently 401'ing in production.
"""

from __future__ import annotations

import os
import re
import shlex
import subprocess
from pathlib import Path

import pytest

from g8e_evals.transport import (
    SESSION_COOKIE_NAME,
    SOURCE_COMPONENT_CLIENT,
    AuthContext,
)


REPO_ROOT = Path(__file__).resolve().parent.parent.parent
COMMON_SH = REPO_ROOT / "scripts" / "cmd" / "common.sh"


def _shell_curl_argv(env: dict[str, str], use_legacy: bool = False) -> list[str]:
    """Source common.sh, run the header-building helpers, dump argv.

    The shell snippet prints each argument on its own line wrapped in
    ``shlex.quote`` form so the Python side can parse it back losslessly
    without any whitespace ambiguity.
    """
    helper = "_append_legacy_g8e_context_headers" if use_legacy else "_append_g8e_auth_headers"
    script = f"""
set -euo pipefail
source "{COMMON_SH}"
args=()
_build_protocol_curl_args args
{helper} args
for a in "${{args[@]}}"; do
    printf '%s\\n' "$a"
done
"""
    proc = subprocess.run(
        ["bash", "-c", script],
        env=env,
        capture_output=True,
        text=True,
        check=False,
    )
    if proc.returncode != 0:
        raise RuntimeError(
            f"shell helper failed (rc={proc.returncode}): "
            f"stdout={proc.stdout!r} stderr={proc.stderr!r}"
        )
    return [line for line in proc.stdout.splitlines() if line != ""]


def _parse_curl_argv(argv: list[str]) -> dict:
    """Reduce a curl argv list to (headers, cookies, cert, key, cacert)."""
    headers: dict[str, str] = {}
    cookies: dict[str, str] = {}
    cert: str | None = None
    key: str | None = None
    cacert: str | None = None

    i = 0
    while i < len(argv):
        tok = argv[i]
        if tok == "-H" and i + 1 < len(argv):
            name, _, value = argv[i + 1].partition(":")
            headers[name.strip()] = value.strip()
            i += 2
            continue
        if tok == "--cookie" and i + 1 < len(argv):
            for piece in argv[i + 1].split(";"):
                piece = piece.strip()
                if not piece:
                    continue
                k, _, v = piece.partition("=")
                cookies[k.strip()] = v.strip()
            i += 2
            continue
        if tok == "--cert" and i + 1 < len(argv):
            cert = argv[i + 1]
            i += 2
            continue
        if tok == "--key" and i + 1 < len(argv):
            key = argv[i + 1]
            i += 2
            continue
        if tok == "--cacert" and i + 1 < len(argv):
            cacert = argv[i + 1]
            i += 2
            continue
        i += 1

    return {
        "headers": headers,
        "cookies": cookies,
        "cert": cert,
        "key": key,
        "cacert": cacert,
    }


@pytest.fixture
def fake_pki(tmp_path: Path) -> dict[str, Path]:
    """Materialize a fake PKI tree on disk so the shell + Python helpers
    both pass their ``-f``/``isfile`` existence checks."""
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
        **os.environ,
        "OPERATOR_SESSION_ID": "sess-parity-001",
        "CLI_SESSION_ID": "cli-parity-001",
        "USER_ID": "user-parity-001",
        "G8E_CLI_CERT": str(fake_pki["cert"]),
        "G8E_CLI_KEY": str(fake_pki["key"]),
        "G8E_TRUST_BUNDLE": str(fake_pki["bundle"]),
        "G8E_PKI_DIR": str(fake_pki["pki"]),
        "G8EE_URL": "https://localhost:8443",
        "G8E_INTERNAL_HTTP_URL": "https://localhost:9000",
        # Make sure no stray optional headers leak in from the dev env.
        "G8E_CASE_ID": "",
        "G8E_INVESTIGATION_ID": "",
        "G8E_BOUND_OPERATORS": "",
        "G8E_TASK_ID": "",
    }


def _python_view(env: dict[str, str], use_legacy: bool = False) -> dict:
    """Render the Python AuthContext into the same shape as the shell parser."""
    # AuthContext.from_env reads from os.environ; swap it in for the call.
    saved = dict(os.environ)
    try:
        os.environ.clear()
        os.environ.update(env)
        ctx = AuthContext.from_env()
    finally:
        os.environ.clear()
        os.environ.update(saved)
    
    headers = ctx.legacy_substrate_headers() if use_legacy else ctx.auth_headers()
    
    return {
        "headers": dict(headers),
        "cookies": dict(ctx.cookies()),
        "cert": ctx.client_cert,
        "key": ctx.client_key,
        "cacert": ctx.trust_bundle,
    }


def test_auth_wiring_matches_shell_helpers(fake_pki):
    env = _baseline_env(fake_pki)
    shell = _parse_curl_argv(_shell_curl_argv(env, use_legacy=False))
    py = _python_view(env, use_legacy=False)

    # mTLS material parity
    assert shell["cert"] == py["cert"]
    assert shell["key"] == py["key"]
    assert shell["cacert"] == py["cacert"]

    # Cookie parity
    assert shell["cookies"] == py["cookies"]
    assert py["cookies"].get(SESSION_COOKIE_NAME) == env["OPERATOR_SESSION_ID"]

    # Header parity — the canary. Any new required header added to
    # _append_g8e_auth_headers but not AuthContext.auth_headers
    # (or vice versa) lights this up.
    assert shell["headers"] == py["headers"], (
        "Auth header drift between scripts/cmd/common.sh and "
        "evals/g8e_evals/transport.py.\n"
        f"  shell only: {set(shell['headers']) - set(py['headers'])}\n"
        f"  python only: {set(py['headers']) - set(shell['headers'])}"
    )

    # Sanity: the canonical fields we promise downstream are actually set.
    h = py["headers"]
    assert h["Content-Type"] == "application/json"
    assert h["Authorization"] == f"Bearer {env['OPERATOR_SESSION_ID']}"
    assert h["X-G8E-CLI-Session-ID"] == env["CLI_SESSION_ID"]

    # Invert the conflation check: ensure no legacy context headers leak into clean auth
    assert "X-G8E-Source-Component" not in h
    assert "X-G8E-User-ID" not in h
    assert h["Authorization"] == f"Bearer {env['OPERATOR_SESSION_ID']}"


def test_legacy_context_headers_match_when_set(fake_pki):
    env = _baseline_env(fake_pki)
    env.update({
        "G8E_CASE_ID": "case-xyz",
        "G8E_INVESTIGATION_ID": "inv-xyz",
        "G8E_BOUND_OPERATORS": "op-1,op-2",
        "G8E_TASK_ID": "task-xyz",
    })
    shell = _parse_curl_argv(_shell_curl_argv(env, use_legacy=True))

    # The Python SUT currently does not emit these optional headers
    # itself, but AuthContext supports them via per-request mutation —
    # so set them on the context and confirm parity with the shell.
    saved = dict(os.environ)
    try:
        os.environ.clear()
        os.environ.update(env)
        ctx = AuthContext.from_env()
    finally:
        os.environ.clear()
        os.environ.update(saved)
    ctx.case_id = env["G8E_CASE_ID"]
    ctx.investigation_id = env["G8E_INVESTIGATION_ID"]
    ctx.bound_operators = env["G8E_BOUND_OPERATORS"]
    ctx.task_id = env["G8E_TASK_ID"]

    assert ctx.legacy_substrate_headers() == shell["headers"]


def test_configurable_contexts_match_when_set(fake_pki):
    env = _baseline_env(fake_pki)
    env.update({
        "WEB_SESSION_ID": "web-parity-001",
        "G8E_SOURCE_COMPONENT": "g8ee",
    })
    shell = _parse_curl_argv(_shell_curl_argv(env, use_legacy=True))

    saved = dict(os.environ)
    try:
        os.environ.clear()
        os.environ.update(env)
        ctx = AuthContext.from_env()
    finally:
        os.environ.clear()
        os.environ.update(saved)

    # 1. Header parity (shell helper should now pick up G8E_SOURCE_COMPONENT)
    h = ctx.legacy_substrate_headers()
    assert h["X-G8E-Source-Component"] == "g8ee"
    assert h["X-G8E-Source-Component"] == shell["headers"]["X-G8E-Source-Component"]

    # 2. Body parity (RequestContext)
    rc = ctx.to_request_context()
    assert rc.web_session_id == "web-parity-001"
    assert rc.source_component == "g8ee"
