# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

from __future__ import annotations

import json
import subprocess

import pytest

from g8e_evals.auth_bridge import AuthBridgeError, CLIAuthContext, load_cli_auth_context

pytestmark = pytest.mark.unit


def test_load_cli_auth_context_uses_typed_go_cli_output(monkeypatch: pytest.MonkeyPatch):
    payload = {
        "operator_session_id": "operator-session-123",
        "cli_session_id": "cli-session-123",
        "user_id": "user-123",
        "operator_id": "operator-123",
        "client_cert": "/runtime/cli.crt",
        "client_key": "/runtime/cli.key",
    }
    calls: list[list[str]] = []

    def run(command: list[str], **kwargs: object) -> subprocess.CompletedProcess[str]:
        calls.append(command)
        return subprocess.CompletedProcess(command, 0, stdout=json.dumps(payload), stderr="")

    monkeypatch.setattr(subprocess, "run", run)

    context = load_cli_auth_context("./g8e")

    assert context == CLIAuthContext(**payload)
    assert calls == [["./g8e", "auth", "context"]]


@pytest.mark.parametrize(
    ("process", "message"),
    [
        (subprocess.CompletedProcess(["./g8e"], 1, stdout="", stderr="not authenticated"), "not authenticated"),
        (subprocess.CompletedProcess(["./g8e"], 0, stdout="not-json", stderr=""), "invalid JSON"),
        (subprocess.CompletedProcess(["./g8e"], 0, stdout="{}", stderr=""), "invalid authentication context"),
    ],
)
def test_load_cli_auth_context_fails_closed(
    monkeypatch: pytest.MonkeyPatch,
    process: subprocess.CompletedProcess[str],
    message: str,
):
    monkeypatch.setattr(subprocess, "run", lambda *args, **kwargs: process)

    with pytest.raises(AuthBridgeError, match=message):
        load_cli_auth_context("./g8e")
