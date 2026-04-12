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
aiohttp.ClientSession constructors.

Three transport roles exist in g8ee — each has exactly one constructor here.
No aiohttp.ClientSession(...) calls anywhere else in the codebase.

Roles
-----
new_kv_http_session        — KV/REST HTTP to the Operator listen port (kv_cache_client)
new_pubsub_ws_session      — WebSocket carrier session for pub/sub (pubsub_client)
new_component_http_session — inter-service HTTP with retry/circuit-breaker (HTTPClient , CacheAsideService)

SSL
---
_resolve_ssl_context accepts an ordered sequence of explicit cert paths (already
resolved from SSLSettings) and returns a loaded SSLContext for the first path
that exists on disk, or None if no cert is present.

WebSocket SSL is passed directly to ws_connect(), not to the session connector,
so new_pubsub_ws_session does not wire SSL — the caller resolves it via
resolve_pubsub_ssl_context(ssl_settings) and passes it to ws_connect().
"""

import ssl
import aiohttp

from app.utils.json_utils import _json_dumps


def _resolve_ssl_context(ca_cert_paths: tuple[str | None, ...], use_tls: bool = False) -> ssl.SSLContext | bool:
    """Return an SSL context for the first existing cert path, or True if TLS requested without cert."""
    for path in ca_cert_paths:
        if path:
            try:
                with open(path):
                    pass
                return ssl.create_default_context(cafile=path)
            except (OSError, IOError):
                continue
    return True if use_tls else False


def _url_uses_tls(url: str) -> bool:
    return url.startswith("https://") or url.startswith("wss://")


def new_kv_http_session(
    existing: aiohttp.ClientSession | None,
    *,
    base_url: str,
    timeout: aiohttp.ClientTimeout,
    ca_cert_path: str,
    headers: dict[str, str],
) -> aiohttp.ClientSession:
    """Session for KV/REST HTTP requests to the Operator listen port.

    SSL is applied only when base_url uses https://.
    Content-Type is set to application/json for all requests unless overridden.
    Used by KVCacheClient.
    """
    if existing is not None and not existing.closed:
        return existing

    use_tls = _url_uses_tls(base_url)
    ssl_ctx = _resolve_ssl_context((ca_cert_path,), use_tls=use_tls)
    connector = aiohttp.TCPConnector(ssl=ssl_ctx)

    default_headers = {"Content-Type": "application/json"}
    if headers:
        default_headers.update(headers)

    return aiohttp.ClientSession(
        headers=default_headers,
        timeout=timeout,
        connector=connector,
    )


def new_pubsub_ws_session(
    existing: aiohttp.ClientSession | None,
    *,
    timeout: aiohttp.ClientTimeout,
) -> aiohttp.ClientSession:
    """Carrier session for the WebSocket pub/sub connection.

    Used by PubSubClient.
    No default headers — WebSocket frames are not HTTP requests.
    SSL is NOT wired into the connector here; pass resolve_pubsub_ssl_context()
    to ws_connect() directly so the scheme check happens at connect time.
    """
    if existing is not None and not existing.closed:
        return existing

    return aiohttp.ClientSession(timeout=timeout)


def resolve_pubsub_ssl_context(
    ca_cert_path: str | None = None,
    use_tls: bool = False,
    **kwargs,
) -> ssl.SSLContext | bool:
    """Resolve the SSL context for WebSocket pub/sub connections.

    Returns True when TLS is requested but no cert is configured.
    """
    # Prefer explicit ca_cert_path, but handle legacy kwargs from older test suites
    actual_path = (
        ca_cert_path
        or kwargs.get("pubsub_ca_cert")
        or kwargs.get("ssl_cert_file")
        or kwargs.get("requests_ca_bundle")
    )
    return _resolve_ssl_context((actual_path,), use_tls=use_tls)


def new_component_http_session(
    existing: aiohttp.ClientSession | None,
    *,
    timeout: aiohttp.ClientTimeout,
    ca_cert_path: str,
    headers: dict[str, str],
) -> aiohttp.ClientSession:
    """Session for inter-component HTTP (HTTPClient , CacheAsideService).

    Always probes for a CA cert regardless of scheme — internal services
    may sit behind TLS even in dev.  Uses _json_dumps for datetime-aware
    JSON serialization.
    """
    if existing is not None and not existing.closed:
        return existing

    ssl_ctx = _resolve_ssl_context((ca_cert_path,), use_tls=True)
    connector = aiohttp.TCPConnector(ssl=ssl_ctx)

    return aiohttp.ClientSession(
        timeout=timeout,
        json_serialize=_json_dumps,
        connector=connector,
        headers=headers,
    )
