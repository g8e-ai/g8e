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
KVCacheClient — HTTP-based Key-Value client for g8es.

Talks to the Operator in --listen mode via HTTP (KV store).
API: get, set, delete, exists, expire, ttl, keys,
get_json, set_json, delete_pattern, hget, hset, hgetall, hdel,
rpush, lpush, lrange, llen, ltrim, incr, decr.
"""

import json
import logging
from typing import Any
from urllib.parse import quote

import aiohttp

from app.models.settings import ListenSettings
from app.utils.aiohttp_session import new_kv_http_session
from app.constants import (
    ComponentName,
    ErrorCode,
    INTERNAL_AUTH_HEADER,
    HTTP_CONTENT_TYPE_HEADER,
)
from app.errors import NetworkError

logger = logging.getLogger(__name__)


def _encode_key(key: str) -> str:
    """URL encode the key for safe use in URL paths."""
    return quote(key, safe="")


class KVCacheClient:
    """
    Async HTTP client for the g8es KV store.
    """

    def __init__(
        self,
        http_url: str | None = None,
        component_name: ComponentName = ComponentName.G8EE,
        timeout: float = 10.0,
        ca_cert_path: str | None = None,
        internal_auth_token: str | None = None,
        listen_settings: ListenSettings | None = None,
    ):
        if listen_settings is None:
            from app.services.infra.settings_service import SettingsService
            service = SettingsService()
            listen_settings = ListenSettings.from_bootstrap(service)
        
        self.http_url = (http_url or listen_settings.http_url).rstrip("/")
        self.component_name = component_name
        self._timeout = timeout
        self._ca_cert_path = ca_cert_path
        self._internal_auth_token = internal_auth_token
        self._session: aiohttp.ClientSession | None = None
        self._healthy = False

    async def _get_http_session(self) -> aiohttp.ClientSession:
        """HTTP session for KV/REST requests."""
        headers = {HTTP_CONTENT_TYPE_HEADER: "application/json"}
        if self._internal_auth_token:
            headers[INTERNAL_AUTH_HEADER] = self._internal_auth_token

        self._session = new_kv_http_session(
            self._session,
            base_url=self.http_url,
            timeout=aiohttp.ClientTimeout(total=self._timeout),
            ca_cert_path=self._ca_cert_path,
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
            )

    async def connect(self) -> bool:
        """Verify connectivity to the g8es KV service."""
        try:
            result = await self._request("GET", "/health")
            self._healthy = result.get("status") == "ok"
            if self._healthy:
                logger.info(f"[KV-CACHE-CLIENT] Connected to {self.http_url}")
            return self._healthy
        except Exception as e:
            logger.error(f"[KV-CACHE-CLIENT] Connection failed: {e}")
            self._healthy = False
            return False

    async def close(self):
        if self._session and not self._session.closed:
            await self._session.close()
        self._healthy = False

    async def health_check(self) -> bool:
        try:
            result = await self._request("GET", "/health")
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
            data = await self._request("GET", f"/kv/{_encode_key(key)}")
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
            await self._request("PUT", f"/kv/{_encode_key(key)}", json={
                "value": value,
                "ttl": ttl
            })
            return True
        except Exception as e:
            logger.error(f"[KV-CACHE-CLIENT] set failed: {e}")
            return False

    async def setex(self, key: str, seconds: int, value: str) -> bool:
        return await self.set(key, value, ex=seconds)

    async def delete(self, *keys: str) -> int:
        count = 0
        for key in keys:
            try:
                await self._request("DELETE", f"/kv/{_encode_key(key)}")
                count += 1
            except Exception:
                pass
        return count

    async def exists(self, *keys: str) -> int:
        count = 0
        for key in keys:
            val = await self.get(key)
            if val is not None:
                count += 1
        return count

    async def expire(self, key: str, seconds: int) -> bool:
        try:
            await self._request("PUT", f"/kv/{_encode_key(key)}/_expire", json={"ttl": seconds})
            return True
        except Exception:
            return False

    async def ttl(self, key: str) -> int:
        try:
            data = await self._request("GET", f"/kv/{_encode_key(key)}/_ttl")
            return data.get("ttl", -2)
        except Exception:
            return -2

    async def keys(self, pattern: str = "*") -> list[str]:
        try:
            data = await self._request("POST", "/kv/_keys", json={"pattern": pattern})
            return data.get("keys", [])
        except Exception:
            return []

    async def delete_pattern(self, pattern: str) -> int:
        try:
            data = await self._request("POST", "/kv/_delete_pattern", json={"pattern": pattern})
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

    async def hset(self, key: str, field: str, value: object) -> int:
        existing = await self.get(key)
        h: dict[str, object] = {}
        if existing:
            try:
                h = json.loads(existing)
            except (json.JSONDecodeError, TypeError):
                pass
        h[field] = value
        await self.set(key, json.dumps(h))
        return 1

    async def hget(self, key: str, field: str) -> str | None:
        existing = await self.get(key)
        if not existing:
            return None
        try:
            h = json.loads(existing)
            val = h.get(field)
            return str(val) if val is not None else None
        except (json.JSONDecodeError, TypeError):
            return None

    async def hgetall(self, key: str) -> dict | None:
        existing = await self.get(key)
        if not existing:
            return None
        try:
            return json.loads(existing)
        except (json.JSONDecodeError, TypeError):
            return None

    async def hdel(self, key: str, *fields: str) -> int:
        existing = await self.get(key)
        if not existing:
            return 0
        try:
            h = json.loads(existing)
            count = 0
            for f in fields:
                if f in h:
                    del h[f]
                    count += 1
            await self.set(key, json.dumps(h))
            return count
        except (json.JSONDecodeError, TypeError):
            return 0

    async def rpush(self, key: str, *values: object) -> int:
        existing = await self.get(key)
        lst: list[object] = []
        if existing:
            try:
                lst = json.loads(existing)
            except (json.JSONDecodeError, TypeError):
                pass
        lst.extend(values)
        await self.set(key, json.dumps(lst))
        return len(lst)

    async def lpush(self, key: str, *values: object) -> int:
        existing = await self.get(key)
        lst: list[object] = []
        if existing:
            try:
                lst = json.loads(existing)
            except (json.JSONDecodeError, TypeError):
                pass
        for v in reversed(values):
            lst.insert(0, v)
        await self.set(key, json.dumps(lst))
        return len(lst)

    async def lrange(self, key: str, start: int, stop: int) -> list:
        existing = await self.get(key)
        if not existing:
            return []
        try:
            lst = json.loads(existing)
            if stop == -1:
                return lst[start:]
            return lst[start:stop + 1]
        except (json.JSONDecodeError, TypeError):
            return []

    async def llen(self, key: str) -> int:
        existing = await self.get(key)
        if not existing:
            return 0
        try:
            return len(json.loads(existing))
        except (json.JSONDecodeError, TypeError):
            return 0

    async def ltrim(self, key: str, start: int, stop: int) -> bool:
        existing = await self.get(key)
        if not existing:
            return True
        try:
            lst = json.loads(existing)
            trimmed = lst[start:] if stop == -1 else lst[start:stop + 1]
            await self.set(key, json.dumps(trimmed))
        except (json.JSONDecodeError, TypeError):
            pass
        return True

    async def incr(self, key: str, amount: int = 1) -> int:
        if not self._healthy:
            return 0
        raw = await self.get(key)
        current = int(raw) if raw is not None else 0
        new_val = current + amount
        await self.set(key, str(new_val))
        return new_val

    async def decr(self, key: str, amount: int = 1) -> int:
        if not self._healthy:
            return 0
        return await self.incr(key, -amount)

    async def ping(self) -> bool:
        return await self.health_check()
