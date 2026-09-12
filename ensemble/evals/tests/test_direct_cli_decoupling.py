# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

"""Tier 1 tests for direct-arm CLI stack independence.

Verifies that the ``run`` command with ``--arm direct`` does not require
``--g8ee-url`` or ``--auth-project-root`` (the direct arm calls the model
provider directly and bypasses g8ee HTTP, SSE trails, receipt collection,
and governance events). Ensemble and governed arms must still require
these dependencies.
"""

from __future__ import annotations

import os

import pytest
from click.testing import CliRunner
from unittest.mock import AsyncMock

from g8e_evals import cli
from g8e_evals.auth_bridge import AuthBridgeError
from g8e_evals.cli import main

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


def test_direct_arm_does_not_require_g8ee_url(monkeypatch: pytest.MonkeyPatch):
    """The direct arm must not require --g8ee-url.

    The direct arm calls the model provider directly; it never contacts
    g8ee. Requiring --g8ee-url makes a large direct campaign fragile
    because every invocation depends on a running g8ee instance that
    the direct arm never uses.
    """
    monkeypatch.setattr(cli, "load_evidence_encryption_key", lambda *_: object())
    monkeypatch.setattr(cli, "_run_suite", AsyncMock())

    result = _invoke(
        CliRunner(),
        [
            "run",
            "--suite", "ifeval_subset",
            "--arm", "direct",
            "--provider", "ollama",
            "--model", "qwen3:8b",
            "--evidence-key-file", "/runtime/evidence-key.json",
        ],
    )

    assert result.exit_code == 0, result.output


def test_direct_arm_does_not_require_auth_project_root(monkeypatch: pytest.MonkeyPatch):
    """The direct arm must not require --auth-project-root.

    The direct arm does not load the canonical CLI identity because it
    never authenticates to the gateway or g8ee. The auth context is only
    needed for ensemble and governed arms that route through the live
    stack.
    """
    monkeypatch.setattr(cli, "load_evidence_encryption_key", lambda *_: object())
    monkeypatch.setattr(cli, "_run_suite", AsyncMock())

    result = _invoke(
        CliRunner(),
        [
            "run",
            "--suite", "ifeval_subset",
            "--arm", "direct",
            "--provider", "ollama",
            "--model", "qwen3:8b",
            "--evidence-key-file", "/runtime/evidence-key.json",
        ],
    )

    assert result.exit_code == 0, result.output


def test_direct_arm_does_not_load_cli_auth_context(monkeypatch: pytest.MonkeyPatch):
    """The direct arm must not call load_cli_auth_context at all.

    Loading the auth context reads from disk and fails when the runtime
    identity is not enrolled. The direct arm has no use for it.
    """
    calls: list[tuple] = []

    def _fail_load(*args, **kwargs):
        calls.append((args, kwargs))
        raise AuthBridgeError("not authenticated")

    monkeypatch.setattr(cli, "load_cli_auth_context", _fail_load)
    monkeypatch.setattr(cli, "load_evidence_encryption_key", lambda *_: object())
    monkeypatch.setattr(cli, "_run_suite", AsyncMock())

    result = _invoke(
        CliRunner(),
        [
            "run",
            "--suite", "ifeval_subset",
            "--arm", "direct",
            "--provider", "ollama",
            "--model", "qwen3:8b",
            "--evidence-key-file", "/runtime/evidence-key.json",
        ],
    )

    assert result.exit_code == 0, result.output
    assert calls == [], "load_cli_auth_context must not be called for the direct arm"


def test_ensemble_arm_defaults_g8ee_url_to_localhost(monkeypatch: pytest.MonkeyPatch):
    """Ensemble arms must not require --g8ee-url.

    The ensemble app is assumed on localhost (the compose-mapped
    http://localhost:8000 default in transport.py), so omitting the flag
    must proceed past endpoint resolution to the auth context load.
    """
    calls: list[tuple] = []

    def _fail_load(*args, **kwargs):
        calls.append((args, kwargs))
        raise AuthBridgeError("not authenticated")

    monkeypatch.setattr(cli, "load_cli_auth_context", _fail_load)

    result = _invoke(
        CliRunner(),
        [
            "run",
            "--suite", "ifeval_subset",
            "--arm", "ensemble_ungoverned",
            "--auth-project-root", "/runtime/project",
        ],
    )

    assert result.exit_code == 2, result.output
    assert len(calls) == 1
    assert "./g8e auth enroll user" in result.output


def test_ensemble_arm_still_requires_auth_project_root():
    """Ensemble arms must still require --auth-project-root.

    The ensemble ungoverned arm authenticates to the gateway via the
    canonical CLI identity, so it must fail fast when the project root
    is missing.
    """
    result = _invoke(
        CliRunner(),
        [
            "run",
            "--suite", "ifeval_subset",
            "--arm", "ensemble_ungoverned",
            "--g8ee-url", "http://g8ee:8000",
        ],
    )

    assert result.exit_code == 2, result.output
    assert "--auth-project-root" in result.output
    assert "required for ensemble and governed arms" in result.output


def test_doctrine_arm_defaults_g8ee_url_to_localhost(monkeypatch: pytest.MonkeyPatch):
    """Governed arms must not require --g8ee-url either."""
    calls: list[tuple] = []

    def _fail_load(*args, **kwargs):
        calls.append((args, kwargs))
        raise AuthBridgeError("not authenticated")

    monkeypatch.setattr(cli, "load_cli_auth_context", _fail_load)

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
    assert len(calls) == 1
    assert "./g8e auth enroll user" in result.output


def test_doctrine_arm_still_requires_auth_project_root():
    """Governed arms must still require --auth-project-root."""
    result = _invoke(
        CliRunner(),
        [
            "run",
            "--suite", "ifeval_subset",
            "--arm", "doctrine",
            "--g8ee-url", "http://g8ee:8000",
        ],
    )

    assert result.exit_code == 2, result.output
    assert "--auth-project-root" in result.output
    assert "required for ensemble and governed arms" in result.output


def test_ensemble_arm_still_loads_cli_auth_context(monkeypatch: pytest.MonkeyPatch):
    """Ensemble arms must still call load_cli_auth_context.

    The ensemble ungoverned arm needs the canonical CLI identity to
    authenticate to the gateway. A failure to load it must surface the
    enrollment hint.
    """
    calls: list[tuple] = []

    def _fail_load(*args, **kwargs):
        calls.append((args, kwargs))
        raise AuthBridgeError("not authenticated")

    monkeypatch.setattr(cli, "load_cli_auth_context", _fail_load)

    result = _invoke(
        CliRunner(),
        [
            "run",
            "--suite", "ifeval_subset",
            "--arm", "ensemble_ungoverned",
            "--g8ee-url", "http://g8ee:8000",
            "--auth-project-root", "/runtime/project",
        ],
    )

    assert result.exit_code == 2, result.output
    assert len(calls) == 1
    assert "./g8e auth enroll user" in result.output


def test_direct_arm_help_no_longer_marks_g8ee_url_required():
    """The --g8ee-url option must not be marked required in --help.

    When the direct arm is the default or a valid selection, the option
    must appear without the "required" marker so operators can run
    direct campaigns without a g8ee endpoint.
    """
    result = _invoke(CliRunner(), ["run", "--help"])

    assert result.exit_code == 0, result.output
    # The option should still appear...
    assert "--g8ee-url" in result.output
    # ...but should not be marked as required.
    assert "[required]" not in result.output.split("--g8ee-url")[1].split("\n")[0]


def test_direct_arm_help_no_longer_marks_auth_project_root_required():
    """The --auth-project-root option must not be marked required in --help."""
    result = _invoke(CliRunner(), ["run", "--help"])

    assert result.exit_code == 0, result.output
    assert "--auth-project-root" in result.output
    assert "[required]" not in result.output.split("--auth-project-root")[1].split("\n")[0]
