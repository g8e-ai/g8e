# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

"""Regression tests for the evals CLI auth UX.

These cover the well-known footguns observed in the field:

1. Passing the ``operator_id`` UUID as ``--operator-session-id`` used to
   silently 401 downstream because the Gateway has no session matching
   that id. The CLI now rejects this with a hard ``UsageError``.

2. Missing canonical CLI identity state points to the supported enrollment
   and session-refresh commands without claiming credential files are parsed
   by Python.

3. The dead ``--operator-id`` flag has been removed; passing it must fail
   with click's standard "no such option" error rather than silently being
   accepted.
"""
from __future__ import annotations

import os

import pytest
from click.testing import CliRunner

from g8e_evals import cli
from g8e_evals.auth_bridge import AuthBridgeError, CLIAuthContext
from g8e_evals.cli import main
from g8e_evals.receipts.collector import ReceiptCollector
from g8e_evals.tls import RuntimeIdentity

pytestmark = pytest.mark.unit


def _invoke(runner: CliRunner, args: list[str], env: dict[str, str] | None = None):
    """Invoke the CLI with a controlled environment.

    Click reads ``envvar=`` defaults from the parent process environment, so
    we must clear ``G8E_OPERATOR_SESSION_ID`` / ``OPERATOR_SESSION_ID`` /
    ``G8E_OPERATOR_ID`` / ``OPERATOR_ID`` to avoid the dev shell leaking a
    cached session into the test.
    """
    sterile = {
        k: v
        for k, v in os.environ.items()
        if not k.startswith(("G8E_OPERATOR", "OPERATOR_SESSION", "OPERATOR_ID"))
    }
    if env:
        sterile.update(env)
    return runner.invoke(main, args, env=sterile, catch_exceptions=False)


def test_session_id_equal_to_operator_id_is_hard_error():
    """The classic footgun: pasting OPERATOR_ID where session id is expected.

    Must produce a ``UsageError`` (exit 2) with an actionable hint, NOT a
    warning-and-continue path that 401s downstream.
    """
    runner = CliRunner()
    bad = "aa237197-d287-41e6-b6d4-4be1fe30accd"

    result = _invoke(
        runner,
        [
            "run",
            "--suite", "ifeval_subset",
            "--arm", "doctrine",
            "--g8ee-url", "http://g8ee:8000",
            "--auth-project-root", "/runtime/project",
            "--operator-session-id", bad,
        ],
        env={"G8E_OPERATOR_ID": bad},
    )

    assert result.exit_code == 2, result.output
    assert "operator_id UUID, not a session id" in result.output
    assert "./g8e auth context" in result.output
    assert "./g8e login" not in result.output


def test_missing_cli_identity_points_at_enrollment_and_refresh(monkeypatch: pytest.MonkeyPatch):
    calls: list[tuple[str, str]] = []

    def load_auth_context(g8e_cli: str, project_root: str):
        calls.append((g8e_cli, project_root))
        raise AuthBridgeError("not authenticated")

    monkeypatch.setattr(cli, "load_cli_auth_context", load_auth_context)
    runner = CliRunner()

    result = _invoke(
        runner,
        [
            "run",
            "--suite", "ifeval_subset",
            "--arm", "doctrine",
            "--g8ee-url", "http://g8ee:8000",
            "--auth-project-root", "/runtime/project",
        ],
    )

    assert result.exit_code == 2, result.output
    assert calls == [("./g8e", "/runtime/project")]
    assert "./g8e auth enroll user" in result.output
    assert "./g8e auth refresh" in result.output
    assert "./g8e login" not in result.output


def test_receipt_collector_uses_typed_cli_context(monkeypatch: pytest.MonkeyPatch):
    cli_context = CLIAuthContext(
        operator_session_id="operator-session-typed",
        cli_session_id="cli-session-typed",
        user_id="user-typed",
        operator_id="operator-typed",
        client_cert="/runtime/cli.crt",
        client_key="/runtime/cli.key",
    )
    auth = object()
    calls = []

    def from_env(**kwargs):
        calls.append(kwargs)
        return auth

    monkeypatch.setattr("g8e_evals.receipts.collector.AuthContext.from_env", from_env)

    collector = ReceiptCollector("https://gateway:8443", cli_context=cli_context)

    assert collector.auth is auth
    assert calls == [{
        "operator_url": "https://gateway:8443",
        "runtime_identity": RuntimeIdentity.GATEWAY,
        "cli_context": cli_context,
    }]


def test_run_help_describes_canonical_authentication_bridge():
    result = _invoke(CliRunner(), ["run", "--help"])

    assert result.exit_code == 0, result.output
    assert "canonical CLI identity" in result.output
    assert "--g8e-cli" in result.output
    assert "--auth-project-root" in result.output
    assert "--g8ee-url" in result.output
    assert "./g8e login" not in result.output


def test_run_returns_nonzero_when_live_evidence_is_invalid(monkeypatch: pytest.MonkeyPatch):
    auth_context = CLIAuthContext(
        operator_session_id="operator-session-typed",
        cli_session_id="cli-session-typed",
        user_id="user-typed",
        operator_id="operator-typed",
        client_cert="/runtime/cli.crt",
        client_key="/runtime/cli.key",
    )

    async def fail_run(*args, **kwargs):
        raise cli.EvaluationRunError(
            "run produced invalid evidence; diagnostic report retained at /reports/failed"
        )

    monkeypatch.setattr(cli, "load_cli_auth_context", lambda *_: auth_context)
    monkeypatch.setattr(cli, "_run_suite", fail_run)

    result = _invoke(
        CliRunner(),
        [
            "run",
            "--suite", "ifeval_subset",
            "--arm", "doctrine",
            "--g8ee-url", "http://g8ee:8000",
            "--auth-project-root", "/runtime/project",
        ],
    )

    assert result.exit_code == 1, result.output
    assert "invalid evidence" in result.output
    assert "diagnostic report retained" in result.output


def test_run_requires_explicit_g8ee_endpoint():
    result = _invoke(
        CliRunner(),
        [
            "run",
            "--suite", "ifeval_subset",
            "--arm", "doctrine",
            "--auth-project-root", "/runtime/project",
        ],
    )

    assert result.exit_code == 2, result.output
    assert "Missing option '--g8ee-url'" in result.output


def test_dead_operator_id_flag_is_removed():
    """``--operator-id`` was never consumed downstream. It is removed; passing
    it must surface click's standard "no such option" error, not be silently
    accepted (which would mask user mistakes).
    """
    runner = CliRunner()
    result = _invoke(
        runner,
        [
            "run",
            "--suite", "ifeval_subset",
            "--arm", "ensemble_ungoverned",
            "--operator-id", "ignored",
        ],
    )
    assert result.exit_code == 2, result.output
    assert "no such option" in result.output.lower() or "unrecognized" in result.output.lower()
