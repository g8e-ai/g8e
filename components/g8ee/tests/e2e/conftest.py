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

"""E2E test fixtures for operator lifecycle tests.

Provisions real operator slots via g8ed internal API, reads API keys from
g8es, launches the real operator binary, and uses the real PubSub event
pipeline to detect operator lifecycle transitions.  No polling, no mocking.
"""

from __future__ import annotations

import asyncio
import json
import logging
import os
import shutil
import signal
import ssl
import tempfile
import uuid
from dataclasses import dataclass, field
from typing import Any

import aiohttp
import pytest
import pytest_asyncio

from app.clients.pubsub_client import PubSubClient
from app.constants import ComponentName
from app.constants.channels import PubSubChannel
from app.constants.kv_keys import KVKeyPrefix
from app.db import DBClient
from app.db.db_service import DBService
from app.db.kv_service import KVService
from app.clients.kv_cache_client import KVCacheClient
from app.services.cache.cache_aside import CacheAsideService
from app.services.infra.settings_service import SettingsService
from app.services.service_factory import ServiceFactory

logger = logging.getLogger(__name__)

OPERATOR_BINARY_PATH = "/home/g8e/g8e.operator"
G8ES_BASE_URL = "https://g8es:9000"
G8ED_BASE_URL = "https://g8ed:443"
E2E_USER_PREFIX = "e2e-test-user"
E2E_ORG_ID = "e2e-test-org"


def _build_ssl_context() -> ssl.SSLContext:
    ca_path = os.environ.get("G8E_SSL_CERT_FILE", "/g8es/ca.crt")
    return ssl.create_default_context(cafile=ca_path)


def _internal_auth_token() -> str:
    token_path = os.environ.get("G8E_INTERNAL_AUTH_TOKEN_FILE", "/g8es/internal_auth_token")
    with open(token_path) as f:
        return f.read().strip()


def _internal_headers() -> dict[str, str]:
    return {
        "X-Internal-Auth": _internal_auth_token(),
        "Content-Type": "application/json",
    }


# ---------------------------------------------------------------------------
# Thin async helper for g8ed / g8es HTTP calls (uses the real TLS + token)
# ---------------------------------------------------------------------------

class _G8eHttpHelper:
    """Lightweight HTTP helper for E2E fixture setup/teardown.

    Talks directly to g8ed (internal API) and g8es (document store) using
    the platform's internal auth token.  Not a replacement for the real
    InternalHttpClient — only used in fixture plumbing.
    """

    def __init__(self) -> None:
        self._session: aiohttp.ClientSession | None = None

    async def _get_session(self) -> aiohttp.ClientSession:
        if self._session is None or self._session.closed:
            self._session = aiohttp.ClientSession(
                connector=aiohttp.TCPConnector(ssl=_build_ssl_context()),
                headers=_internal_headers(),
            )
        return self._session

    async def close(self) -> None:
        if self._session and not self._session.closed:
            await self._session.close()

    async def post(self, url: str, json_data: dict | None = None) -> dict[str, Any]:
        session = await self._get_session()
        async with session.post(url, json=json_data or {}) as resp:
            resp.raise_for_status()
            return await resp.json()

    async def get(self, url: str) -> dict[str, Any]:
        session = await self._get_session()
        async with session.get(url) as resp:
            resp.raise_for_status()
            return await resp.json()

    async def delete(self, url: str) -> dict[str, Any]:
        session = await self._get_session()
        async with session.delete(url) as resp:
            resp.raise_for_status()
            return await resp.json()

    # -- convenience wrappers --

    async def initialize_operator_slots(self, user_id: str) -> list[str]:
        data = await self.post(
            f"{G8ED_BASE_URL}/api/internal/operators/user/{user_id}/initialize-slots",
            {"organization_id": E2E_ORG_ID},
        )
        return data["operator_ids"]

    async def get_operator_api_key(self, operator_id: str) -> str:
        """Read the API key directly from the g8es document store."""
        doc = await self.get(f"{G8ES_BASE_URL}/db/operators/{operator_id}")
        api_key = doc.get("api_key", "")
        if not api_key:
            raise RuntimeError(f"Operator {operator_id} has no api_key in g8es")
        return api_key

    async def get_operator_status(self, operator_id: str) -> dict[str, Any]:
        return await self.get(
            f"{G8ED_BASE_URL}/api/internal/operators/{operator_id}/status"
        )

    async def reset_operator(self, operator_id: str) -> dict[str, Any]:
        return await self.post(
            f"{G8ED_BASE_URL}/api/internal/operators/{operator_id}/reset-cache"
        )

    async def terminate_operator(self, operator_id: str) -> dict[str, Any]:
        return await self.post(
            f"{G8ED_BASE_URL}/api/internal/operators/{operator_id}/terminate"
        )

    async def delete_operator_doc(self, operator_id: str) -> None:
        await self.delete(f"{G8ES_BASE_URL}/db/operators/{operator_id}")

    async def list_user_operators(self, user_id: str) -> list[dict[str, Any]]:
        data = await self.get(
            f"{G8ED_BASE_URL}/api/internal/operators/user/{user_id}"
        )
        return data.get("data", [])

    async def create_user(self, user_id: str, email: str, name: str) -> dict[str, Any]:
        """Create a user via g8ed internal API so the operator can authenticate."""
        return await self.post(
            f"{G8ED_BASE_URL}/api/internal/users",
            {"email": email, "name": name},
        )

    async def delete_user_doc(self, user_id: str) -> None:
        try:
            await self.delete(f"{G8ES_BASE_URL}/db/users/{user_id}")
        except Exception:
            pass


# ---------------------------------------------------------------------------
# Sandbox — owns the subprocess and per-test temp directory
# ---------------------------------------------------------------------------

@dataclass
class OperatorSandbox:
    """Manages one operator binary subprocess in an isolated sandbox."""

    operator_id: str
    operator_session_id: str | None = None
    api_key: str = ""
    user_id: str = ""
    sandbox_dir: str = ""
    process: asyncio.subprocess.Process | None = None
    stdout_lines: list[str] = field(default_factory=list)
    stderr_lines: list[str] = field(default_factory=list)

    # PubSub event signals — set by the heartbeat listener
    heartbeat_received: asyncio.Event = field(default_factory=asyncio.Event)
    first_heartbeat_data: dict[str, Any] = field(default_factory=dict)

    @property
    def is_running(self) -> bool:
        return self.process is not None and self.process.returncode is None

    async def wait_for_heartbeat(self, timeout: float = 30.0) -> dict[str, Any]:
        """Block until the first heartbeat arrives via PubSub."""
        await asyncio.wait_for(self.heartbeat_received.wait(), timeout=timeout)
        return self.first_heartbeat_data

    def dump_logs(self) -> str:
        header = f"=== Operator {self.operator_id} logs ==="
        out = "\n".join(self.stdout_lines[-200:])
        err = "\n".join(self.stderr_lines[-200:])
        return f"{header}\n--- stdout ---\n{out}\n--- stderr ---\n{err}"


# ---------------------------------------------------------------------------
# Session-scoped real service infrastructure
# ---------------------------------------------------------------------------

@pytest_asyncio.fixture(scope="session", loop_scope="session")
async def e2e_settings():
    settings_service = SettingsService()
    bootstrap = settings_service.get_local_settings()

    db_client = DBClient(
        ca_cert_path=bootstrap.ca_cert_path,
        internal_auth_token=bootstrap.auth.internal_auth_token,
    )
    await db_client.connect()

    kv_client = KVCacheClient(
        component_name=ComponentName.G8EE,
        ca_cert_path=bootstrap.ca_cert_path,
        internal_auth_token=bootstrap.auth.internal_auth_token,
    )
    await kv_client.connect()

    cache_aside = CacheAsideService(
        kv=KVService(kv_client),
        db=DBService(db_client),
        component_name=ComponentName.G8EE,
    )
    settings_service._cache_aside = cache_aside

    try:
        settings = await settings_service.get_platform_settings()
    except Exception:
        logger.warning("Failed to load platform settings, falling back to bootstrap")
        settings = bootstrap
    finally:
        await kv_client.close()
        await db_client.close()

    yield settings


@pytest_asyncio.fixture(scope="session", loop_scope="session")
async def e2e_cache_aside(e2e_settings):
    settings = e2e_settings

    raw_kv = KVCacheClient(
        ca_cert_path=settings.ca_cert_path,
        internal_auth_token=settings.auth.internal_auth_token,
        component_name=ComponentName.G8EE,
    )
    await raw_kv.connect()

    raw_db = DBClient(
        ca_cert_path=settings.ca_cert_path,
        internal_auth_token=settings.auth.internal_auth_token,
    )
    await raw_db.connect()

    cas = CacheAsideService(
        kv=KVService(raw_kv),
        db=DBService(raw_db),
        component_name=ComponentName.G8EE,
        default_ttl=settings.listen.default_ttl,
    )
    yield cas
    await cas.close()
    await raw_db.close()
    await raw_kv.close()


@pytest_asyncio.fixture(scope="session", loop_scope="session")
async def e2e_pubsub_client(e2e_settings):
    settings = e2e_settings

    client = PubSubClient(
        pubsub_url=settings.listen.pubsub_url,
        internal_auth_token=settings.auth.internal_auth_token,
        component_name=ComponentName.G8EE,
        ca_cert_path=settings.ca_cert_path,
    )
    await client.connect()
    yield client
    await client.close()


@pytest_asyncio.fixture(scope="session", loop_scope="session")
async def e2e_services(e2e_settings, e2e_cache_aside, e2e_pubsub_client):
    """Full g8ee service graph wired to real infrastructure."""
    services = ServiceFactory.create_all_services(
        settings=e2e_settings,
        cache_aside_service=e2e_cache_aside,
        pubsub_client=e2e_pubsub_client,
    )
    await ServiceFactory.start_services(services)
    yield services
    await ServiceFactory.stop_services(services)


@pytest_asyncio.fixture(scope="session", loop_scope="session")
async def e2e_http():
    helper = _G8eHttpHelper()
    yield helper
    await helper.close()


# ---------------------------------------------------------------------------
# Per-test operator sandbox
# ---------------------------------------------------------------------------

@pytest_asyncio.fixture(scope="function", loop_scope="session")
async def operator_sandbox(
    e2e_settings,
    e2e_services,
    e2e_pubsub_client,
    e2e_http,
    request,
):
    """Provision one operator slot, launch the binary, wait for heartbeat.

    The operator authenticates via the real g8ed auth flow, which triggers
    session registration relay to g8ee, which subscribes to PubSub channels.
    The first heartbeat arrives event-driven via PubSub.

    Yields an OperatorSandbox with the running operator.
    Teardown: SIGTERM, cleanup sandbox directory, reset operator slot.
    """
    test_id = uuid.uuid4().hex[:8]
    user_id = f"{E2E_USER_PREFIX}-{test_id}"

    if not os.path.isfile(OPERATOR_BINARY_PATH):
        pytest.skip(f"Operator binary not found at {OPERATOR_BINARY_PATH}")

    # 1. Create a user so the operator can authenticate
    user_email = f"{user_id}@e2e-test.g8e.local"
    user_resp = await e2e_http.create_user(user_id, user_email, f"E2E Test {test_id}")
    created_user_id = user_resp.get("user", {}).get("id", user_id)

    # 2. Invalidate any stale cache for this user before provisioning slots
    kv_client = e2e_services["cache_aside_service"]._kv
    cache_aside = e2e_services["cache_aside_service"]
    try:
        doc_pattern = f"{KVKeyPrefix.CACHE_DOC}operators:{created_user_id}:*"
        await kv_client.delete_pattern(doc_pattern)
        await cache_aside.invalidate_query_cache("operators")
        logger.info("[E2E] Pre-setup cache invalidation for user %s", created_user_id)
    except Exception as exc:
        logger.warning("[E2E] Pre-setup cache invalidation failed: %s", exc)

    # 3. Provision operator slots via g8ed internal API
    operator_ids = await e2e_http.initialize_operator_slots(created_user_id)
    assert operator_ids, "No operator slots created"
    operator_id = operator_ids[0]

    # 3. Read the API key from g8es document store
    api_key = await e2e_http.get_operator_api_key(operator_id)

    # 4. Create sandbox directory and copy binary
    sandbox_dir = tempfile.mkdtemp(prefix=f"g8e-e2e-{test_id}-")
    sandbox_binary = os.path.join(sandbox_dir, "g8e.operator")
    shutil.copy2(OPERATOR_BINARY_PATH, sandbox_binary)
    os.chmod(sandbox_binary, 0o755)

    sandbox = OperatorSandbox(
        operator_id=operator_id,
        api_key=api_key,
        user_id=user_id,
        sandbox_dir=sandbox_dir,
    )

    # 5. Subscribe to the heartbeat channel BEFORE launching the binary
    #    so we catch the first heartbeat event-driven.
    heartbeat_pattern = f"heartbeat:{operator_id}:*"

    async def _on_heartbeat(pattern: str, channel: str, data: str | dict) -> None:
        raw = data if isinstance(data, dict) else json.loads(data)
        if not sandbox.heartbeat_received.is_set():
            sandbox.first_heartbeat_data = raw
            sandbox.heartbeat_received.set()
            logger.info(
                "[E2E] First heartbeat received for operator %s",
                operator_id,
            )

    e2e_pubsub_client.on_pmessage(heartbeat_pattern, _on_heartbeat)
    await e2e_pubsub_client.psubscribe(heartbeat_pattern)

    # 6. Launch the operator binary
    endpoint = e2e_settings.component_urls.g8ed_url.replace("https://", "").split(":")[0]
    env = {
        **os.environ,
        "G8E_OPERATOR_API_KEY": api_key,
    }

    process = await asyncio.create_subprocess_exec(
        sandbox_binary,
        "--endpoint", endpoint,
        "--working-dir", sandbox_dir,
        "--no-git",
        "--log", "debug",
        "--cloud",
        "--provider", "g8ep",
        stdout=asyncio.subprocess.PIPE,
        stderr=asyncio.subprocess.PIPE,
        env=env,
    )
    sandbox.process = process

    # 7. Start background log capture tasks
    async def _capture_stream(stream: asyncio.StreamReader, target: list[str]) -> None:
        while True:
            line = await stream.readline()
            if not line:
                break
            decoded = line.decode("utf-8", errors="replace").rstrip()
            target.append(decoded)

    stdout_task = asyncio.create_task(_capture_stream(process.stdout, sandbox.stdout_lines))
    stderr_task = asyncio.create_task(_capture_stream(process.stderr, sandbox.stderr_lines))

    yield sandbox

    # --- Teardown ---

    # 8. Terminate the operator process
    if sandbox.is_running:
        try:
            sandbox.process.send_signal(signal.SIGTERM)
            try:
                await asyncio.wait_for(sandbox.process.wait(), timeout=5.0)
            except asyncio.TimeoutError:
                sandbox.process.kill()
                await sandbox.process.wait()
        except ProcessLookupError:
            pass

    # Cancel log capture
    stdout_task.cancel()
    stderr_task.cancel()
    for task in (stdout_task, stderr_task):
        try:
            await task
        except asyncio.CancelledError:
            pass

    # Dump logs on test failure
    if hasattr(request.node, "rep_call") and request.node.rep_call.failed:
        logger.error(sandbox.dump_logs())

    # 9. Unsubscribe heartbeat pattern
    await e2e_pubsub_client.punsubscribe(heartbeat_pattern)

    # 10. Clean up operator slots and user in g8es via internal API
    #    Uses CacheAside to maintain cache consistency
    try:
        for oid in operator_ids:
            await e2e_http.terminate_operator(oid)
        await e2e_http.delete_user_doc(created_user_id)
    except Exception as exc:
        logger.warning("[E2E] Cleanup failed for user %s: %s", user_id, exc)

    # 11. Remove sandbox directory
    shutil.rmtree(sandbox_dir, ignore_errors=True)


# ---------------------------------------------------------------------------
# Pytest hook to capture test outcome for log dumping
# ---------------------------------------------------------------------------

@pytest.hookimpl(tryfirst=True, hookwrapper=True)
def pytest_runtest_makereport(item, call):
    outcome = yield
    rep = outcome.get_result()
    setattr(item, f"rep_{rep.when}", rep)
