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

2. Receipt mode without a session id used to error with a stale message
   suggesting ``./g8e login --email superadmin@g8e.local``. The sandbox
   default now is zero-arg ``./g8e login``; the error string must reflect
   that.

3. The dead ``--operator-id`` flag has been removed; passing it must fail
   with click's standard "no such option" error rather than silently being
   accepted.
"""
from __future__ import annotations

import os

import pytest
from click.testing import CliRunner

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
            "--mode", "receipt",
            "--operator-session-id", bad,
        ],
        env={"G8E_OPERATOR_ID": bad},
    )

    assert result.exit_code == 2, result.output
    assert "operator_id UUID, not a session id" in result.output
    assert "./g8e login" in result.output


def test_receipt_mode_without_session_points_at_zero_arg_login():
    """Sandbox UX: the missing-session error must suggest ``./g8e login``
    (no flags), matching the new zero-arg sandbox onboarding.
    """
    runner = CliRunner()
    result = _invoke(
        runner,
        ["run", "--suite", "ifeval_subset", "--mode", "receipt"],
    )
    assert result.exit_code == 2, result.output
    assert "operator-session-id is required" in result.output
    assert "./g8e login" in result.output
    # The legacy --email flag must not be advertised as required for sandbox.
    assert "--email superadmin@g8e.local" not in result.output


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
            "--mode", "baseline",
            "--operator-id", "ignored",
        ],
    )
    assert result.exit_code == 2, result.output
    assert "no such option" in result.output.lower() or "unrecognized" in result.output.lower()
