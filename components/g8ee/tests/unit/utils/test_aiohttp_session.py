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

"""
Unit tests for aiohttp_session — the three named session constructors.
"""

import ssl
from unittest.mock import patch

import aiohttp
import pytest

from app.utils.aiohttp_session import (
    _resolve_ssl_context,
    _url_uses_tls,
    new_component_http_session,
    new_kv_http_session,
    new_pubsub_ws_session,
    resolve_pubsub_ssl_context,
)

pytestmark = pytest.mark.unit


# =============================================================================
# _url_uses_tls
# =============================================================================

class TestUrlUsesTls:

    def test_https_returns_true(self):
        assert _url_uses_tls("https://g8es:9000") is True

    def test_wss_returns_true(self):
        assert _url_uses_tls("wss://g8es:9001") is True

    def test_http_returns_false(self):
        assert _url_uses_tls("http://g8es:9000") is False

    def test_ws_returns_false(self):
        assert _url_uses_tls("ws://g8es") is False


# =============================================================================
# _resolve_ssl_context
# =============================================================================

class TestResolveSslContext:

    def test_returns_none_when_no_paths_given(self):
        assert _resolve_ssl_context((None, None)) is False

    def test_returns_none_when_path_does_not_exist(self):
        assert _resolve_ssl_context(("/nonexistent/ca.pem",)) is False

    def test_returns_context_when_cert_file_exists(self, tmp_path):
        cert = tmp_path / "ca.pem"
        cert.write_bytes(b"")
        with patch("ssl.SSLContext.load_verify_locations"):
            result = _resolve_ssl_context((str(cert),))
            assert isinstance(result, ssl.SSLContext)

    def test_first_resolvable_path_wins(self, tmp_path):
        cert1 = tmp_path / "ca1.pem"
        cert2 = tmp_path / "ca2.pem"
        cert1.write_bytes(b"")
        cert2.write_bytes(b"")
        with patch("ssl.SSLContext.load_verify_locations") as mock_load:
            _resolve_ssl_context((str(cert1), str(cert2)))
            assert mock_load.call_count == 1

    def test_skips_none_paths(self, tmp_path):
        cert = tmp_path / "ca.pem"
        cert.write_bytes(b"")
        with patch("ssl.SSLContext.load_verify_locations"):
            result = _resolve_ssl_context((None, str(cert)))
            assert isinstance(result, ssl.SSLContext)


# =============================================================================
# new_kv_http_session
# =============================================================================

@pytest.mark.asyncio(loop_scope="session")
class TestNewKvHttpSession:

    async def test_returns_new_session_when_none(self):
        s = new_kv_http_session(
            None,
            base_url="https://g8es:9000",
            timeout=aiohttp.ClientTimeout(total=10),
            ca_cert_path="/tmp/ca.crt",
            headers={},
        )
        try:
            assert isinstance(s, aiohttp.ClientSession)
        finally:
            await s.close()

    async def test_reuses_open_session(self):
        s = new_kv_http_session(
            None,
            base_url="https://g8es:9000",
            timeout=aiohttp.ClientTimeout(total=10),
            ca_cert_path="/tmp/ca.crt",
            headers={},
        )
        try:
            same = new_kv_http_session(
                s,
                base_url="https://g8es:9000",
                timeout=aiohttp.ClientTimeout(total=10),
                ca_cert_path="/tmp/ca.crt",
                headers={},
            )
            assert same is s
        finally:
            await s.close()

    async def test_recreates_after_close(self):
        s = new_kv_http_session(
            None,
            base_url="https://g8es:9000",
            timeout=aiohttp.ClientTimeout(total=10),
            ca_cert_path="/tmp/ca.crt",
            headers={},
        )
        await s.close()
        s2 = new_kv_http_session(
            s,
            base_url="https://g8es:9000",
            timeout=aiohttp.ClientTimeout(total=10),
            ca_cert_path="/tmp/ca.crt",
            headers={},
        )
        try:
            assert s2 is not s
            assert not s2.closed
        finally:
            await s2.close()

    async def test_sets_content_type_header(self):
        s = new_kv_http_session(
            None,
            base_url="https://g8es:9000",
            timeout=aiohttp.ClientTimeout(total=10),
            ca_cert_path="/tmp/ca.crt",
            headers={},
        )
        try:
            assert s.headers.get("Content-Type") == "application/json"
        finally:
            await s.close()


@pytest.mark.asyncio(loop_scope="session")
class TestNewKvHttpSessionSsl:

    async def test_no_ssl_for_http_url(self):
        with patch("app.utils.aiohttp_session._resolve_ssl_context", return_value=False) as mock_resolve:
            s = new_kv_http_session(
                None,
                base_url="https://g8es:9000",
                timeout=aiohttp.ClientTimeout(total=10),
                ca_cert_path="/some/ca.pem",
                headers={},
            )
            await s.close()
            mock_resolve.assert_called_once_with(("/some/ca.pem",), use_tls=True)

    async def test_ssl_attempted_for_https_url(self):
        with patch("app.utils.aiohttp_session._resolve_ssl_context", return_value=None) as mock_resolve:
            s = new_kv_http_session(
                None,
                base_url="https://g8es:9000",
                timeout=aiohttp.ClientTimeout(total=10),
                ca_cert_path="/some/ca.pem",
                headers={},
            )
            await s.close()
            mock_resolve.assert_called_once_with(("/some/ca.pem",), use_tls=True)

    async def test_no_ssl_without_ca_cert_path_for_https(self):
        with patch("app.utils.aiohttp_session._resolve_ssl_context", return_value=None) as mock_resolve:
            s = new_kv_http_session(
                None,
                base_url="https://g8es:9000",
                timeout=aiohttp.ClientTimeout(total=10),
                ca_cert_path="/nonexistent/ca.pem",
                headers={},
            )
            await s.close()
            mock_resolve.assert_called_once_with(("/nonexistent/ca.pem",), use_tls=True)


# =============================================================================
# new_pubsub_ws_session
# =============================================================================

@pytest.mark.asyncio(loop_scope="session")
class TestNewPubsubWsSession:

    async def test_returns_new_session_when_none(self):
        s = new_pubsub_ws_session(None, timeout=aiohttp.ClientTimeout(total=10))
        try:
            assert isinstance(s, aiohttp.ClientSession)
        finally:
            await s.close()

    async def test_reuses_open_session(self):
        s = new_pubsub_ws_session(None, timeout=aiohttp.ClientTimeout(total=10))
        try:
            same = new_pubsub_ws_session(s, timeout=aiohttp.ClientTimeout(total=10))
            assert same is s
        finally:
            await s.close()

    async def test_no_content_type_header(self):
        s = new_pubsub_ws_session(None, timeout=aiohttp.ClientTimeout(total=10))
        try:
            assert "Content-Type" not in s.headers
        finally:
            await s.close()


@pytest.mark.asyncio(loop_scope="session")
class TestNewPubsubWsSessionSsl:

    async def test_ssl_never_wired_into_connector(self):
        with patch("app.utils.aiohttp_session._resolve_ssl_context") as mock_resolve:
            s = new_pubsub_ws_session(None, timeout=aiohttp.ClientTimeout(total=10))
            try:
                mock_resolve.assert_not_called()
            finally:
                await s.close()


# =============================================================================
# resolve_pubsub_ssl_context
# =============================================================================

class TestResolvePubsubSslContext:

    def test_returns_none_when_no_cert_configured(self):
        assert resolve_pubsub_ssl_context(pubsub_ca_cert=None) is False

    def test_returns_true_when_tls_requested_no_cert(self):
        assert resolve_pubsub_ssl_context(pubsub_ca_cert=None, use_tls=True) is True

    def test_pubsub_ca_cert_takes_priority(self, tmp_path):
        pubsub_cert = tmp_path / "pubsub.pem"
        ssl_cert = tmp_path / "ssl.pem"
        pubsub_cert.write_bytes(b"")
        ssl_cert.write_bytes(b"")
        with patch("ssl.SSLContext.load_verify_locations"):
            result = resolve_pubsub_ssl_context(
                pubsub_ca_cert=str(pubsub_cert),
                ssl_cert_file=str(ssl_cert),
            )
            assert isinstance(result, ssl.SSLContext)

    def test_ssl_cert_file_used_when_no_pubsub_cert(self, tmp_path):
        cert = tmp_path / "ssl.pem"
        cert.write_bytes(b"")
        with patch("ssl.SSLContext.load_verify_locations"):
            result = resolve_pubsub_ssl_context(pubsub_ca_cert=None, ssl_cert_file=str(cert))
            assert isinstance(result, ssl.SSLContext)

    def test_requests_ca_bundle_fallback(self, tmp_path):
        cert = tmp_path / "bundle.pem"
        cert.write_bytes(b"")
        with patch("ssl.SSLContext.load_verify_locations"):
            result = resolve_pubsub_ssl_context(pubsub_ca_cert=None, requests_ca_bundle=str(cert))
            assert isinstance(result, ssl.SSLContext)

    def test_nonexistent_path_returns_none(self):
        assert resolve_pubsub_ssl_context(pubsub_ca_cert="/nonexistent.pem") is False


# =============================================================================
# new_component_http_session
# =============================================================================

@pytest.mark.asyncio(loop_scope="session")
class TestNewComponentHttpSession:

    async def test_returns_new_session_when_none(self):
        s = new_component_http_session(
            None, timeout=aiohttp.ClientTimeout(total=10), ca_cert_path="/tmp/ca.crt", headers={}
        )
        try:
            assert isinstance(s, aiohttp.ClientSession)
        finally:
            await s.close()

    async def test_reuses_open_session(self):
        s = new_component_http_session(
            None, timeout=aiohttp.ClientTimeout(total=10), ca_cert_path="/tmp/ca.crt", headers={}
        )
        try:
            same = new_component_http_session(
                s, timeout=aiohttp.ClientTimeout(total=10), ca_cert_path="/tmp/ca.crt", headers={}
            )
            assert same is s
        finally:
            await s.close()

    async def test_recreates_after_close(self):
        s = new_component_http_session(
            None, timeout=aiohttp.ClientTimeout(total=10), ca_cert_path="/tmp/ca.crt", headers={}
        )
        await s.close()
        s2 = new_component_http_session(
            s, timeout=aiohttp.ClientTimeout(total=10), ca_cert_path="/tmp/ca.crt", headers={}
        )
        try:
            assert s2 is not s
            assert not s2.closed
        finally:
            await s2.close()

    async def test_no_default_content_type_header(self):
        s = new_component_http_session(
            None, timeout=aiohttp.ClientTimeout(total=10), ca_cert_path="/tmp/ca.crt", headers={}
        )
        try:
            assert "Content-Type" not in s.headers
        finally:
            await s.close()


@pytest.mark.asyncio(loop_scope="session")
class TestNewComponentHttpSessionSsl:

    async def test_no_ssl_when_ca_cert_path_not_given(self):
        with patch("app.utils.aiohttp_session._resolve_ssl_context", return_value=None) as mock_resolve:
            s = new_component_http_session(
                None, timeout=aiohttp.ClientTimeout(total=10), ca_cert_path="/nonexistent/ca.pem", headers={}
            )
            try:
                mock_resolve.assert_called_once_with(("/nonexistent/ca.pem",), use_tls=True)
            finally:
                await s.close()

    async def test_ssl_used_when_ca_cert_path_given(self):
        with patch("app.utils.aiohttp_session._resolve_ssl_context", return_value=None) as mock_resolve:
            s = new_component_http_session(
                None,
                timeout=aiohttp.ClientTimeout(total=10),
                ca_cert_path="/some/ca.pem",
                headers={},
            )
            try:
                mock_resolve.assert_called_once_with(("/some/ca.pem",), use_tls=True)
            finally:
                await s.close()
