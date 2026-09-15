# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

"""Tests proving the engine child never reads G8E_* auth or provenance
environment variables when the typed EvalEngineRequest platform context
supplies them.

The Go facade populates the platform fields from the on-disk canonical
credentials and the binary's build stamp before engine launch. These
tests clear the entire G8E_* auth surface and assert the request-driven
path still constructs a usable identity.
"""

from __future__ import annotations

from pathlib import Path

import pytest

from g8e_evals.auth_bridge import CLIAuthContext
from g8e_evals.engine_cli import EngineError
from g8e_evals.engine_protocol import (
    EVAL_ENGINE_REQUEST_SCHEMA_VERSION,
    EvalEngineRequest,
    EvalErrorCode,
    EvalOperation,
    EvalPlatformContext,
)
from g8e_evals.eval_engine_handlers import (
    _request_build_provenance,
    _request_cli_auth_context,
)
from g8e_evals.schema import SourceBuildProvenance
from g8e_evals.transport import AuthContext

pytestmark = pytest.mark.unit

_G8E_ENV_VARS = (
    "G8E_OPERATOR_SESSION_ID",
    "G8E_CLI_SESSION_ID",
    "G8E_USER_ID",
    "G8E_CLI_CERT",
    "G8E_CLI_KEY",
    "G8E_OPERATOR_ID",
    "G8E_OPERATOR_URL",
    "G8E_G8EE_URL",
    "G8E_APP_TRUST_BUNDLE",
    "G8E_APP_PKI_DIR",
    "G8E_GATEWAY_TRUST_BUNDLE",
    "G8E_GATEWAY_PKI_DIR",
    "G8E_EVALS_SOURCE_REVISION",
    "G8E_EVALS_SOURCE_TREE_STATE_HASH",
)


def _platform(**overrides: str) -> EvalPlatformContext:
    base = {
        "repository_root": "/repo",
        "eval_project": "/repo/ensemble/evals",
        "g8e_binary_path": "/repo/g8e",
        "g8e_binary_sha256": "b" * 64,
        "platform_version": "2.1.0",
        "auth_project_root": "/repo",
        "runtime_dir": "/repo/.g8e",
        "trust_bundle_path": "/repo/.g8e/pki/trust/g8eg-ca-bundle.pem",
        "gateway_http_url": "http://127.0.0.1:8080",
        "gateway_https_url": "https://127.0.0.1:8443",
        "ensemble_url": "http://127.0.0.1:8000",
        "cli_cert_path": "/repo/.g8e/cli.crt",
        "cli_key_path": "/repo/.g8e/cli.key",
        "operator_session_id": "op-session-1",
        "cli_session_id": "cli-session-1",
        "user_id": "user-1",
        "operator_id": "op-1",
        "source_revision": "deadbeef",
        "source_tree_state_hash": "a" * 64,
    }
    base.update(overrides)
    return EvalPlatformContext(**base)


def _request(platform: EvalPlatformContext | None = None) -> EvalEngineRequest:
    return EvalEngineRequest(
        schema_version=EVAL_ENGINE_REQUEST_SCHEMA_VERSION,
        operation=EvalOperation.DIAGNOSTIC_START,
        operation_id="diag-1",
        revision="rev-1",
        config_path="/tmp/config.json",
        lease_path="/tmp/lease.json",
        report_root="/tmp/report",
        platform=platform or _platform(),
    )


@pytest.fixture
def clean_g8e_env(monkeypatch: pytest.MonkeyPatch) -> None:
    for var in _G8E_ENV_VARS:
        monkeypatch.delenv(var, raising=False)


def test_request_cli_auth_context_uses_platform_fields_not_env(clean_g8e_env: None) -> None:
    ctx = _request_cli_auth_context(_request())
    assert ctx.operator_session_id == "op-session-1"
    assert ctx.cli_session_id == "cli-session-1"
    assert ctx.user_id == "user-1"
    assert ctx.operator_id == "op-1"
    assert ctx.client_cert == "/repo/.g8e/cli.crt"
    assert ctx.client_key == "/repo/.g8e/cli.key"


def test_request_cli_auth_context_fails_closed_on_empty_identity(clean_g8e_env: None) -> None:
    request = _request(
        _platform(
            operator_session_id="",
            cli_session_id="",
            user_id="",
            cli_cert_path="",
            cli_key_path="",
        )
    )
    with pytest.raises(EngineError) as exc_info:
        _request_cli_auth_context(request)
    assert exc_info.value.code == EvalErrorCode.PLATFORM_IDENTITY_UNAVAILABLE


def test_request_build_provenance_uses_platform_fields_not_env(clean_g8e_env: None) -> None:
    provenance = _request_build_provenance(_request())
    assert isinstance(provenance, SourceBuildProvenance)
    assert provenance.source_revision == "deadbeef"
    assert provenance.source_tree_state_hash == "a" * 64
    assert provenance.binary_sha256 == "b" * 64
    assert provenance.build_system == "g8e-facade"


def test_request_build_provenance_fails_closed_on_missing_stamp(clean_g8e_env: None) -> None:
    request = _request(_platform(source_revision="", source_tree_state_hash=""))
    with pytest.raises(EngineError) as exc_info:
        _request_build_provenance(request)
    assert exc_info.value.code == EvalErrorCode.PLATFORM_IDENTITY_UNAVAILABLE


def test_auth_context_from_platform_fields_needs_no_env(
    clean_g8e_env: None, tmp_path: Path, monkeypatch: pytest.MonkeyPatch
) -> None:
    """AuthContext.from_env with a request-supplied CLI context and trust
    bundle resolves the full identity without reading any G8E_* auth env
    var and without consulting the cwd-relative pki directory."""
    cert = tmp_path / "cli.crt"
    cert.write_text("cert")
    key = tmp_path / "cli.key"
    key.write_text("key")
    bundle = tmp_path / "bundle.pem"
    bundle.write_text("bundle")
    # Run from a directory whose relative .g8e/pki does not exist so a
    # cwd-relative bundle lookup would fail closed.
    workdir = tmp_path / "workdir"
    workdir.mkdir()
    monkeypatch.chdir(workdir)

    ctx = AuthContext.from_env(
        cli_context=CLIAuthContext(
            operator_session_id="op-session-1",
            cli_session_id="cli-session-1",
            user_id="user-1",
            operator_id="op-1",
            client_cert=str(cert),
            client_key=str(key),
        ),
        trust_bundle=str(bundle),
        operator_url="https://127.0.0.1:8443",
        g8ee_url="http://127.0.0.1:8000",
    )
    assert ctx.trust_bundle == str(bundle)
    assert ctx.operator_session_id == "op-session-1"
    assert ctx.cli_session_id == "cli-session-1"
    assert ctx.user_id == "user-1"


def test_auth_context_env_fallback_preserved_for_legacy_path(
    tmp_path: Path, monkeypatch: pytest.MonkeyPatch
) -> None:
    """The legacy direct-CLI path keeps the env-var fallback: when no
    explicit trust bundle is supplied, the pki-directory lookup still
    resolves the app bundle relative to the process cwd."""
    cert = tmp_path / "cli.crt"
    cert.write_text("cert")
    key = tmp_path / "cli.key"
    key.write_text("key")
    bundle_dir = tmp_path / ".g8e" / "pki" / "trust"
    bundle_dir.mkdir(parents=True)
    bundle = bundle_dir / "hub-bundle.pem"
    bundle.write_text("bundle")
    monkeypatch.chdir(tmp_path)
    monkeypatch.setenv("G8E_OPERATOR_SESSION_ID", "op-session-env")
    monkeypatch.setenv("G8E_CLI_SESSION_ID", "cli-session-env")
    monkeypatch.setenv("G8E_USER_ID", "user-env")
    monkeypatch.setenv("G8E_CLI_CERT", str(cert))
    monkeypatch.setenv("G8E_CLI_KEY", str(key))

    ctx = AuthContext.from_env(
        operator_url="https://127.0.0.1:8443",
        g8ee_url="http://127.0.0.1:8000",
    )
    assert Path(ctx.trust_bundle).resolve() == bundle
    assert ctx.operator_session_id == "op-session-env"
