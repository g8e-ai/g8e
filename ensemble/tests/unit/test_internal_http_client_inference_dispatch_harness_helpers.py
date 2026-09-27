# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

"""Unit tests for inference dispatch harness helpers."""

from __future__ import annotations

import json
from pathlib import Path
from unittest.mock import MagicMock, patch

import pytest

from tests.integration.test_internal_http_client_inference_dispatch_harness_integration import (
    _build_settings,
    _require_go,
    _wait_for_fixture,
)


def test_wait_for_fixture_success(tmp_path: Path):
    fixture_path = tmp_path / "harness.json"
    data = {"client_url": "https://localhost:8443", "ca_cert_path": "/tmp/ca.pem"}
    fixture_path.write_text(json.dumps(data), encoding="utf-8")

    result = _wait_for_fixture(fixture_path, timeout_seconds=1.0)
    assert result == data


def test_wait_for_fixture_fails_fast_on_process_exit(tmp_path: Path):
    fixture_path = tmp_path / "harness.json"
    log_path = tmp_path / "harness.log"
    log_path.write_text("go: build failed due to syntax error\n", encoding="utf-8")

    mock_proc = MagicMock()
    mock_proc.poll.return_value = 1
    mock_proc.returncode = 1

    with pytest.raises(AssertionError) as exc_info:
        _wait_for_fixture(fixture_path, proc=mock_proc, log_path=log_path, timeout_seconds=5.0)

    err = str(exc_info.value)
    assert "Go gateway harness process exited prematurely with returncode 1" in err
    assert "go: build failed due to syntax error" in err


def test_wait_for_fixture_times_out(tmp_path: Path):
    fixture_path = tmp_path / "nonexistent.json"
    log_path = tmp_path / "harness.log"
    log_path.write_text("server listening on port 8080\n", encoding="utf-8")

    mock_proc = MagicMock()
    mock_proc.poll.return_value = None

    with pytest.raises(AssertionError) as exc_info:
        _wait_for_fixture(fixture_path, proc=mock_proc, log_path=log_path, timeout_seconds=0.2)

    err = str(exc_info.value)
    assert "timed out waiting for harness fixture" in err
    assert "server listening on port 8080" in err


def test_require_go_skips_when_missing():
    with patch("shutil.which", return_value=None):
        with pytest.raises(pytest.skip.Exception):
            _require_go()


def test_require_go_skips_when_version_fails():
    with patch("shutil.which", return_value="/usr/bin/go"), \
         patch("subprocess.run", return_value=MagicMock(returncode=1, stderr="broken go")):
        with pytest.raises(pytest.skip.Exception):
            _require_go()


def test_require_go_success():
    with patch("shutil.which", return_value="/usr/bin/go"), \
         patch("subprocess.run", return_value=MagicMock(returncode=0)):
        assert _require_go() == "/usr/bin/go"


def test_build_settings():
    fixture = {
        "client_url": "https://localhost:9000",
        "ca_cert_path": "/path/to/ca.pem",
        "client_cert_path": "/path/to/client.pem",
        "client_key_path": "/path/to/client.key",
    }
    settings = _build_settings(fixture)
    assert settings.component_urls.client_url == "https://localhost:9000"
    assert settings._ca_cert_path == "/path/to/ca.pem"
    assert settings._client_cert_path == "/path/to/client.pem"
    assert settings._client_key_path == "/path/to/client.key"
