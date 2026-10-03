# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

"""HTTP client for Gateway-owned document and query caches."""

import json
import logging
from typing import Any
from urllib.parse import quote

import aiohttp

from app.models.settings import GatewaySettings, TLSConfig
from app.services.infra.settings_service import SettingsService
from app.utils.aiohttp_session import create_kv_http_session
from app.constants import (
    AUTHORIZATION,
    CONTENT_TYPE,
    ErrorCode,
    G8EE_COMPONENT,
    GatewayAPIPaths,
)
from app.errors import NetworkError

logger = logging.getLogger(__name__)


def _encode_key(key: str) -> str:
    """URL encode the key for safe use in URL paths."""
    return quote(key, safe="")


class KVCacheClient:
    """
    Async HTTP client for the operator KV store.
    """

    def __init__(
        self,
        http_url: str | None = None,
        component_name: str = G8EE_COMPONENT,
        timeout: float = 10.0,
        tls_config: TLSConfig | None = None,
        operator_session_id: str | None = None,
        gateway_settings: GatewaySettings | None = None,
    ):
        if gateway_settings is None:
            service = SettingsService()
            gateway_settings = GatewaySettings.from_bootstrap(service)

        self.http_url = (http_url or gateway_settings.http_url).rstrip("/")
        self.component_name = component_name
        self._timeout = timeout

        if tls_config is not None:
            self._ca_cert_path = tls_config.ca_cert_path
            self._client_cert_path = tls_config.client_cert_path
            self._client_key_path = tls_config.client_key_path
        else:
            self._ca_cert_path = None
            self._client_cert_path = None
            self._client_key_path = None

        self._operator_session_id = operator_session_id
        self._session: aiohttp.ClientSession | None = None
        self._healthy = False

    async def _get_http_session(self) -> aiohttp.ClientSession:
        """HTTP session for KV/REST requests."""
        headers = {CONTENT_TYPE: "application/json"}
        if self._operator_session_id:
            headers[AUTHORIZATION] = f"Bearer {self._operator_session_id}"

        self._session = create_kv_http_session(
            self._session,
            base_url=self.http_url,
            timeout=aiohttp.ClientTimeout(total=self._timeout),
            ca_cert_path=self._ca_cert_path,
            client_cert_path=self._client_cert_path,
            client_key_path=self._client_key_path,
            headers=headers,
        )
        return self._session

    async def _request(self, method: str, path: str, **kwargs) -> Any:
        session = await self._get_http_session()
        url = f"{self.http_url}{path}"
        try:
            async with session.request(method, url, **kwargs) as resp:
                text = await resp.text()
                if resp.status >= 400:
                    try:
                        data = json.loads(text)
                        msg = data.get("error", f"HTTP {resp.status}")
                    except (json.JSONDecodeError, AttributeError):
                        msg = f"HTTP {resp.status}: {text[:200]}"
                    raise NetworkError(
                        msg,
                        code=ErrorCode.API_RESPONSE_ERROR,
                        details={"http_status": resp.status, "path": path},
                        component="kv_cache_client",
                    )
                try:
                    return json.loads(text)
                except json.JSONDecodeError:
                    return text
        except NetworkError:
            raise
        except Exception as e:
            raise NetworkError(
                f"HTTP request failed: {e}",
                code=ErrorCode.API_CONNECTION_ERROR,
                details={"path": path},
                component="kv_cache_client",
                cause=e,
            ) from e

    async def connect(self) -> bool:
        """Verify connectivity to the Gateway KV service."""
        try:
            result = await self._request("GET", GatewayAPIPaths.HEALTH)
            self._healthy = result.get("status") == "ok"
            if self._healthy:
                logger.info("[KV-CACHE-CLIENT] Connected to %s", self.http_url)
            return self._healthy
        except Exception as e:
            logger.error("[KV-CACHE-CLIENT] Connection failed: %s", e)
            self._healthy = False
            return False

    async def close(self):
        if self._session and not self._session.closed:
            await self._session.close()
        self._healthy = False

    async def health_check(self) -> bool:
        try:
            result = await self._request("GET", GatewayAPIPaths.HEALTH)
            self._healthy = result.get("status") == "ok"
            return self._healthy
        except Exception:
            self._healthy = False
            return False

    def is_healthy(self) -> bool:
        return self._healthy

    async def get(self, key: str) -> str | None:
        if not self._healthy:
            return None
        try:
            data = await self._request("GET", f"{GatewayAPIPaths.KV_PREFIX}{_encode_key(key)}")
            return data.get("value")
        except Exception:
            return None

    async def set(self, key: str, value: str, ex: int | None = None, px: int | None = None) -> bool:
        if not self._healthy:
            return False
        ttl = 0
        if ex is not None:
            ttl = ex
        elif px is not None:
            ttl = max(1, px // 1000)
        try:
            await self._request(
                "PUT",
                f"{GatewayAPIPaths.KV_PREFIX}{_encode_key(key)}",
                json={"value": value, "ttl": ttl},
            )
            return True
        except Exception as e:
            logger.error("[KV-CACHE-CLIENT] set failed: %s", e)
            return False

    async def delete(self, *keys: str) -> int:
        count = 0
        for key in keys:
            try:
                await self._request("DELETE", f"{GatewayAPIPaths.KV_PREFIX}{_encode_key(key)}")
                count += 1
            except Exception:
                pass
        return count

    async def keys(self, pattern: str = "*") -> list[str]:
        try:
            data = await self._request(
                "POST", f"{GatewayAPIPaths.KV_PREFIX}_keys", json={"pattern": pattern}
            )
            return data.get("keys", [])
        except Exception:
            return []

    async def delete_pattern(self, pattern: str) -> int:
        try:
            data = await self._request(
                "POST", f"{GatewayAPIPaths.KV_PREFIX}_delete_pattern", json={"pattern": pattern}
            )
            return data.get("deleted", 0)
        except Exception:
            return 0

    async def get_json(self, key: str) -> Any | None:
        raw = await self.get(key)
        if raw is None:
            return None
        try:
            return json.loads(raw)
        except (json.JSONDecodeError, TypeError):
            return raw

    async def set_json(self, key: str, value: Any, ex: int | None = None) -> bool:
        serialized = json.dumps(value)
        return await self.set(key, serialized, ex=ex)
