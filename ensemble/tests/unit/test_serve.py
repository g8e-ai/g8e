# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

"""The ensemble launcher turns explicit arguments into typed bootstrap settings."""

import pytest

from app import serve
from app.constants.bootstrap import BootstrapSettings, get_bootstrap


def test_no_arguments_leave_bootstrap_defaults():
    parsed = serve.parse_args([])

    assert parsed.bootstrap == BootstrapSettings()
    assert parsed.host == "0.0.0.0"
    assert parsed.port == 8000


def test_arguments_map_to_bootstrap_settings():
    parsed = serve.parse_args(
        [
            "--gateway-http-url", "http://g8e.local:8080",
            "--gateway-url", "https://g8e.local:8443",
            "--gateway-https-url", "https://g8e.local:8443",
            "--gateway-pubsub-url", "wss://g8e.local:8443",
            "--runtime-dir", "/root/.g8e",
            "--port", "9000",
        ]
    )

    assert parsed.bootstrap == BootstrapSettings(
        gateway_http_url="http://g8e.local:8080",
        gateway_url="https://g8e.local:8443",
        gateway_https_url="https://g8e.local:8443",
        gateway_pubsub_url="wss://g8e.local:8443",
        runtime_dir="/root/.g8e",
    )
    assert parsed.port == 9000


def test_unknown_argument_is_rejected():
    with pytest.raises(SystemExit):
        serve.parse_args(["--not-a-flag", "x"])


def test_main_installs_bootstrap_before_starting_the_server(monkeypatch: pytest.MonkeyPatch):
    started: list[tuple[str, str, int]] = []

    def fake_run(app: str, *, host: str, port: int) -> None:
        # The application is imported by uvicorn after bootstrap is installed.
        assert get_bootstrap().gateway_http_url == "http://g8e.local:8080"
        started.append((app, host, port))

    monkeypatch.setattr(serve.uvicorn, "run", fake_run)

    serve.main(["--gateway-http-url", "http://g8e.local:8080"])

    assert started == [("app.main:app", "0.0.0.0", 8000)]
