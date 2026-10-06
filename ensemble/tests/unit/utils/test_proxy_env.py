# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

"""Unit tests for proxy_env - NO_PROXY/no_proxy bracketed-IPv6 normalization."""

import pytest

from app.utils.proxy_env import sanitize_proxy_env

pytestmark = pytest.mark.unit


def test_bracketed_ipv6_is_unbracketed():
    env = {"NO_PROXY": "localhost,[::1],example.com"}
    assert sanitize_proxy_env(env) is True
    assert env["NO_PROXY"] == "localhost,::1,example.com"


def test_bracketed_full_ipv6_is_unbracketed():
    env = {"no_proxy": "[fd8b:4f84:7d32:99::1],[fd8b:4f84:7d32:99::2]"}
    assert sanitize_proxy_env(env) is True
    assert env["no_proxy"] == "fd8b:4f84:7d32:99::1,fd8b:4f84:7d32:99::2"


def test_bare_ipv6_and_other_entries_untouched():
    env = {"NO_PROXY": "localhost,127.0.0.1,::1,192.168.0.0/16,example.com"}
    assert sanitize_proxy_env(env) is False
    assert env["NO_PROXY"] == "localhost,127.0.0.1,::1,192.168.0.0/16,example.com"


def test_non_ip_brackets_untouched():
    env = {"no_proxy": "[not-an-ip],example.com"}
    assert sanitize_proxy_env(env) is False
    assert env["no_proxy"] == "[not-an-ip],example.com"


def test_bracketed_ipv6_with_port_untouched():
    # Port-qualified entries are out of scope; never rewrite them.
    env = {"NO_PROXY": "[::1]:8080"}
    assert sanitize_proxy_env(env) is False
    assert env["NO_PROXY"] == "[::1]:8080"


def test_missing_keys_are_noops():
    assert sanitize_proxy_env({}) is False
    assert sanitize_proxy_env({"NO_PROXY": ""}) is False


def test_idempotent():
    env = {"NO_PROXY": "[::1],localhost"}
    assert sanitize_proxy_env(env) is True
    assert sanitize_proxy_env(env) is False
    assert env["NO_PROXY"] == "::1,localhost"


def test_result_parses_under_httpx():
    """The normalized value must survive httpx's own no_proxy parsing."""
    import os
    from unittest.mock import patch

    httpx_utils = pytest.importorskip("httpx._utils")

    env = {"no_proxy": "localhost,[::1],[fd8b:4f84:7d32:99::1]"}
    sanitize_proxy_env(env)
    assert env["no_proxy"] == "localhost,::1,fd8b:4f84:7d32:99::1"

    proxy_keys = (
        "HTTP_PROXY",
        "HTTPS_PROXY",
        "ALL_PROXY",
        "http_proxy",
        "https_proxy",
        "all_proxy",
    )
    with patch.dict(os.environ, {"no_proxy": env["no_proxy"]}):
        for key in proxy_keys:
            os.environ.pop(key, None)
        mounts = httpx_utils.get_environment_proxies()  # must not raise
    assert mounts  # entries produced proxy-bypass mounts
