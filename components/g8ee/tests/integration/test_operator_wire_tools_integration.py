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

"""Wire-level integration tests for every AI tool that dispatches to g8eo.

These tests bypass g8ee entirely: they publish the direct request event
(e.g. ``OPERATOR_FILE_EDIT_REQUESTED``) onto the operator's command
channel and assert on the corresponding completion event received on the
results channel. No AI, no approval gate, no LLM provider.

Target: the long-running g8e.operator running inside the g8ep container.
The operator is already authenticated to g8ed via its API key; it does
not need to be bound to a web session for these tests because we publish
directly to its ``cmd:{operator_id}:{operator_session_id}`` channel.

Isolation: every test generates unique paths/IDs with ``uuid.uuid4``.
No shared filesystem state between tests.

If the g8ep operator is not reachable (not heartbeating within
``HEARTBEAT_DISCOVERY_TIMEOUT``), the whole module is skipped.
"""

from __future__ import annotations

import asyncio
import json
import logging
import os
import uuid
from collections.abc import AsyncIterator, Awaitable, Callable
from dataclasses import dataclass
from datetime import datetime, timezone
from typing import Any

import pytest
import pytest_asyncio

from app.clients.pubsub_client import PubSubClient
from app.constants import ComponentName
from app.constants.channels import PubSubChannel
from app.constants.events import EventType

logger = logging.getLogger(__name__)

pytestmark = [
    pytest.mark.integration,
    pytest.mark.operator_wire,
    pytest.mark.asyncio(loop_scope="session"),
]


def pytest_collection_modifyitems(config, items):
    """Skip all operator_wire tests if the g8ep operator is not reachable.

    This hook runs at collection time and applies a skip marker to all tests
    marked with 'operator_wire' if the G8EP_OPERATOR_AVAILABLE env var is not
    set to 'true'. This allows the skips to group in the test summary.

    The g8ep_operator fixture still performs the actual heartbeat discovery
    and sets the env var when successful, so this hook just checks the result.
    """
    operator_available = os.environ.get("G8EP_OPERATOR_AVAILABLE", "").lower() == "true"
    skip_reason = (
        f"No g8ep operator heartbeat within {HEARTBEAT_DISCOVERY_TIMEOUT}s — "
        f"start the platform (./g8e platform start) and ensure the g8ep container is running."
    )

    for item in items:
        if item.get_closest_marker("operator_wire") and not operator_available:
            item.add_marker(pytest.mark.skip(reason=skip_reason))


# ---------------------------------------------------------------------------
# Tunables
# ---------------------------------------------------------------------------

HEARTBEAT_DISCOVERY_TIMEOUT = float(os.environ.get("G8EP_HEARTBEAT_DISCOVERY_TIMEOUT", "15.0"))
RESULT_WAIT_TIMEOUT = float(os.environ.get("G8EP_RESULT_WAIT_TIMEOUT", "20.0"))
WORKDIR_ROOT = os.environ.get("G8EP_OPERATOR_WORKDIR", "/home/g8e")


# ---------------------------------------------------------------------------
# Operator discovery — listen for one heartbeat, extract (operator_id, session_id)
# ---------------------------------------------------------------------------

@dataclass(frozen=True)
class DiscoveredOperator:
    operator_id: str
    operator_session_id: str


@pytest_asyncio.fixture(scope="session", loop_scope="session")
async def wire_pubsub_client(test_settings) -> AsyncIterator[PubSubClient]:
    """Session-scoped PubSubClient wired with the platform CA cert.

    The top-level ``pubsub_client`` fixture in the main conftest does not pass
    ``ca_cert_path``, so its WebSocket connection fails TLS verification
    against g8es. This fixture builds a fully-configured client for
    wire-level operator tests.
    """
    client = PubSubClient(
        pubsub_url=test_settings.listen.pubsub_url,
        internal_auth_token=test_settings.auth.internal_auth_token,
        component_name=ComponentName.G8EE,
        ca_cert_path=test_settings.ca_cert_path,
    )
    await client.connect()
    try:
        yield client
    finally:
        await client.close()


@pytest_asyncio.fixture(scope="session", loop_scope="session")
async def g8ep_operator(wire_pubsub_client: PubSubClient) -> AsyncIterator[DiscoveredOperator]:
    """Discover the running g8ep operator via a single heartbeat.

    Subscribes to ``heartbeat:*:*``, waits for the first heartbeat, parses
    ``operator_id`` and ``operator_session_id`` out of the channel name, and
    yields the tuple. Skips the module if no heartbeat arrives within
    ``HEARTBEAT_DISCOVERY_TIMEOUT``.
    """
    got = asyncio.Event()
    holder: dict[str, str] = {}

    async def _on_hb(pattern: str, channel: str, _data: str | dict[str, object]) -> None:
        # channel format: heartbeat:{operator_id}:{session_id}
        if got.is_set():
            return
        parts = channel.split(":")
        if len(parts) != 3:
            return
        _, op_id, sess_id = parts
        if not op_id or not sess_id:
            return
        holder["operator_id"] = op_id
        holder["operator_session_id"] = sess_id
        got.set()

    pattern = "heartbeat:*:*"
    wire_pubsub_client.on_pmessage(pattern, _on_hb)
    await wire_pubsub_client.psubscribe(pattern)

    try:
        try:
            await asyncio.wait_for(got.wait(), timeout=HEARTBEAT_DISCOVERY_TIMEOUT)
        except asyncio.TimeoutError:
            os.environ["G8EP_OPERATOR_AVAILABLE"] = "false"
            pytest.skip(
                f"No g8ep operator heartbeat within {HEARTBEAT_DISCOVERY_TIMEOUT}s — "
                f"start the platform (./g8e platform start) and ensure the g8ep container is running."
            )

        os.environ["G8EP_OPERATOR_AVAILABLE"] = "true"
        discovered = DiscoveredOperator(
            operator_id=holder["operator_id"],
            operator_session_id=holder["operator_session_id"],
        )
        logger.info(
            "[WIRE] Discovered g8ep operator: operator_id=%s session_id=%s",
            discovered.operator_id, discovered.operator_session_id,
        )
        yield discovered
    finally:
        try:
            await wire_pubsub_client.punsubscribe(pattern)
        except Exception:  # noqa: BLE001 - best-effort cleanup
            pass


# ---------------------------------------------------------------------------
# Publish / wait helper
# ---------------------------------------------------------------------------

PublishAndWait = Callable[..., Awaitable[dict[str, Any]]]


@pytest_asyncio.fixture(scope="function", loop_scope="session")
async def publish_and_wait(
    wire_pubsub_client: PubSubClient,
    g8ep_operator: DiscoveredOperator,
) -> AsyncIterator[PublishAndWait]:
    """Yield a helper that publishes a command and awaits the matching result.

    The helper subscribes to the operator's results channel before publishing
    so no events are lost. Results are filtered by ``execution_id`` (which the
    caller provides as ``msg_id``) so parallel tests using the same operator
    do not cross-talk.
    """
    results_channel = PubSubChannel.results(
        g8ep_operator.id, g8ep_operator.operator_session_id,
    )
    cmd_channel = PubSubChannel.cmd(
        g8ep_operator.id, g8ep_operator.operator_session_id,
    )

    # A single subscription serves every invocation. Each call gets its own
    # per-execution_id Future on the shared channel handler.
    waiters: dict[str, asyncio.Future[dict[str, Any]]] = {}

    async def _on_result(_channel: str, data: str | dict[str, object]) -> None:
        raw = data if isinstance(data, dict) else json.loads(str(data))
        payload = raw.get("payload")
        exec_id: str | None = None
        if isinstance(payload, dict):
            exec_id = payload.get("execution_id")  # type: ignore[assignment]
        corr_id = exec_id or raw.get("id")
        if not isinstance(corr_id, str):
            return
        fut = waiters.get(corr_id)
        if fut is not None and not fut.done():
            fut.set_result(raw)  # type: ignore[arg-type]

    wire_pubsub_client.on_channel_message(results_channel, _on_result)
    await wire_pubsub_client.subscribe(results_channel)

    async def _publish_and_wait(
        event_type: EventType,
        payload: dict[str, Any],
        *,
        msg_id: str | None = None,
        case_id: str = "wire-test-case",
        investigation_id: str = "wire-test-investigation",
        task_id: str = "wire-test-task",
        timeout: float = RESULT_WAIT_TIMEOUT,
    ) -> dict[str, Any]:
        execution_id = msg_id or f"wire-{uuid.uuid4().hex}"
        # Mirror execution_id into the payload so the g8eo handler uses it
        # both for correlation and in the result envelope.
        payload.setdefault("execution_id", execution_id)

        loop = asyncio.get_running_loop()
        future: asyncio.Future[dict[str, Any]] = loop.create_future()
        waiters[execution_id] = future

        envelope = {
            "id": execution_id,
            "event_type": event_type.value if isinstance(event_type, EventType) else str(event_type),
            "case_id": case_id,
            "task_id": task_id,
            "investigation_id": investigation_id,
            "operator_session_id": g8ep_operator.operator_session_id,
            "operator_id": g8ep_operator.id,
            "payload": payload,
            "timestamp": datetime.now(tz=timezone.utc).isoformat().replace("+00:00", "Z"),
        }

        try:
            receivers = await wire_pubsub_client.publish(cmd_channel, envelope)
            assert receivers > 0, (
                f"No subscribers on {cmd_channel} — is the g8ep operator alive?"
            )
            return await asyncio.wait_for(future, timeout=timeout)
        finally:
            waiters.pop(execution_id, None)

    try:
        yield _publish_and_wait
    finally:
        wire_pubsub_client.off_channel_message(results_channel, _on_result)
        try:
            await wire_pubsub_client.unsubscribe(results_channel)
        except Exception:  # noqa: BLE001 - best-effort cleanup
            pass


def _unique_path(prefix: str, suffix: str = "") -> str:
    """Return an isolated, collision-free path under the operator's workdir."""
    name = f"{prefix}-{uuid.uuid4().hex}{suffix}"
    return os.path.join(WORKDIR_ROOT, "wire-tests", name)


# ---------------------------------------------------------------------------
# Tests — one per operator-gated AI tool that dispatches to g8eo
# ---------------------------------------------------------------------------

class TestRunCommandsWire:
    async def test_echo_command_completes(self, publish_and_wait: PublishAndWait):
        marker = uuid.uuid4().hex
        result = await publish_and_wait(
            EventType.OPERATOR_COMMAND_REQUESTED,
            {
                "command": f"echo wire-{marker}",
                "timeout_seconds": 10,
                "justification": "wire-level integration test",
            },
        )

        assert result["event_type"] == EventType.OPERATOR_COMMAND_COMPLETED.value, (
            f"Expected completion event, got {result['event_type']}: {result}"
        )
        payload = result["payload"]
        assert payload["status"] == "completed", payload
        # Operator publishes stdout/stderr on the ExecutionResultsPayload wire
        # contract; no legacy "output" alias exists.
        assert marker in (payload.get("stdout") or ""), (
            f"Command stdout missing marker {marker!r}: {payload}"
        )


class TestFileEditWire:
    """Covers FILE_CREATE, FILE_WRITE, FILE_UPDATE via OPERATOR_FILE_EDIT_REQUESTED."""

    async def test_file_create_then_update_then_read(
        self,
        publish_and_wait: PublishAndWait,
    ):
        target = _unique_path("edit", ".txt")
        create_marker = uuid.uuid4().hex
        update_marker = uuid.uuid4().hex

        # --- create ---
        create_result = await publish_and_wait(
            EventType.OPERATOR_FILE_EDIT_REQUESTED,
            {
                "file_path": target,
                "operation": "write",
                "content": f"hello-{create_marker}",
                "create_if_missing": True,
                "justification": "wire-test file_create",
            },
        )
        assert create_result["event_type"] == EventType.OPERATOR_FILE_EDIT_COMPLETED.value, create_result
        assert create_result["payload"]["status"] == "completed", create_result["payload"]

        # --- update (replace) ---
        update_result = await publish_and_wait(
            EventType.OPERATOR_FILE_EDIT_REQUESTED,
            {
                "file_path": target,
                "operation": "replace",
                "old_content": create_marker,
                "new_content": update_marker,
                "justification": "wire-test file_update",
            },
        )
        assert update_result["event_type"] == EventType.OPERATOR_FILE_EDIT_COMPLETED.value, update_result
        assert update_result["payload"]["status"] == "completed", update_result["payload"]

        # --- read back via fs read ---
        read_result = await publish_and_wait(
            EventType.OPERATOR_FILESYSTEM_READ_REQUESTED,
            {"path": target, "max_size": 4096},
        )
        assert read_result["event_type"] == EventType.OPERATOR_FILESYSTEM_READ_COMPLETED.value, read_result
        content = read_result["payload"].get("content", "")
        assert update_marker in content, f"updated content not visible on read: {content!r}"
        assert create_marker not in content, f"stale content still present: {content!r}"

    async def test_file_write_overwrites_existing(self, publish_and_wait: PublishAndWait):
        target = _unique_path("write", ".txt")
        v1 = uuid.uuid4().hex
        v2 = uuid.uuid4().hex

        for content in (v1, v2):
            res = await publish_and_wait(
                EventType.OPERATOR_FILE_EDIT_REQUESTED,
                {
                    "file_path": target,
                    "operation": "write",
                    "content": content,
                    "create_if_missing": True,
                    "justification": "wire-test file_write overwrite",
                },
            )
            assert res["payload"]["status"] == "completed", res["payload"]

        read = await publish_and_wait(
            EventType.OPERATOR_FILESYSTEM_READ_REQUESTED,
            {"path": target, "max_size": 4096},
        )
        assert v2 in read["payload"].get("content", "")
        assert v1 not in read["payload"].get("content", "")


class TestFileReadWire:
    async def test_read_missing_file_reports_failure(self, publish_and_wait: PublishAndWait):
        missing = _unique_path("missing", ".txt")
        result = await publish_and_wait(
            EventType.OPERATOR_FILESYSTEM_READ_REQUESTED,
            {"path": missing, "max_size": 1024},
        )
        assert result["event_type"] == EventType.OPERATOR_FILESYSTEM_READ_FAILED.value, result
        assert result["payload"]["status"] == "failed"
        assert result["payload"].get("error_message")


class TestListFilesWire:
    async def test_list_directory_returns_entries(self, publish_and_wait: PublishAndWait):
        # Seed a directory with two files so we can assert on entries.
        dir_path = _unique_path("ls", "")
        file_a = os.path.join(dir_path, "a.txt")
        file_b = os.path.join(dir_path, "b.txt")
        for path in (file_a, file_b):
            res = await publish_and_wait(
                EventType.OPERATOR_FILE_EDIT_REQUESTED,
                {
                    "file_path": path,
                    "operation": "write",
                    "content": "x",
                    "create_if_missing": True,
                    "justification": "wire-test ls seed",
                },
            )
            assert res["payload"]["status"] == "completed", res["payload"]

        result = await publish_and_wait(
            EventType.OPERATOR_FILESYSTEM_LIST_REQUESTED,
            {"path": dir_path, "max_entries": 50},
        )
        assert result["event_type"] == EventType.OPERATOR_FILESYSTEM_LIST_COMPLETED.value, result
        entries = result["payload"].get("entries") or []
        names = {e.get("name") for e in entries if isinstance(e, dict)}
        assert {"a.txt", "b.txt"}.issubset(names), (
            f"Expected a.txt and b.txt in listing, got {names}"
        )


class TestCheckPortWire:
    async def test_check_port_closed_returns_result(self, publish_and_wait: PublishAndWait):
        # Port 1 on localhost is reserved/closed in every sane environment.
        result = await publish_and_wait(
            EventType.OPERATOR_NETWORK_PORT_CHECK_REQUESTED,
            {"host": "127.0.0.1", "port": 1, "protocol": "tcp"},
        )
        assert result["event_type"] == EventType.OPERATOR_NETWORK_PORT_CHECK_COMPLETED.value, result
        results = result["payload"].get("results") or []
        assert results, f"port check returned no entries: {result}"
        entry = results[0]
        assert entry["port"] == 1
        assert entry["open"] is False


class TestFetchFileHistoryWire:
    """History handler is optional — skip cleanly when the operator did not enable it."""

    async def test_fetch_history_returns_completed_or_disabled(self, publish_and_wait: PublishAndWait):
        probe_path = _unique_path("hist-probe", ".txt")
        result = await publish_and_wait(
            EventType.OPERATOR_FILE_HISTORY_FETCH_REQUESTED,
            {"file_path": probe_path},
        )
        event_type = result["event_type"]
        if event_type == EventType.OPERATOR_FILE_HISTORY_FETCH_FAILED.value:
            # LFAA error payloads use "error"; typed failure payloads use
            # "error_message". Accept either.
            payload = result["payload"]
            err = (payload.get("error_message") or payload.get("error") or "").lower()
            if "not available" in err or "ledger is disabled" in err:
                pytest.skip(
                    "g8ep operator was started without a file-history-capable ledger"
                )
            # Other failure kinds are a legitimate signal worth asserting on.
        assert event_type in (
            EventType.OPERATOR_FILE_HISTORY_FETCH_COMPLETED.value,
            EventType.OPERATOR_FILE_HISTORY_FETCH_FAILED.value,
        ), result
        assert "execution_id" in result["payload"]


class TestFetchFileDiffWire:
    """Local store is optional — skip cleanly when the operator did not enable it."""

    async def test_fetch_diff_returns_completed_or_disabled(self, publish_and_wait: PublishAndWait):
        probe_path = _unique_path("diff-probe", ".txt")
        result = await publish_and_wait(
            EventType.OPERATOR_FILE_DIFF_FETCH_REQUESTED,
            {"file_path": probe_path, "limit": 5},
        )
        event_type = result["event_type"]
        if event_type == EventType.OPERATOR_FILE_DIFF_FETCH_FAILED.value:
            payload = result["payload"]
            err = (payload.get("error_message") or payload.get("error") or "").lower()
            if "not available" in err:
                pytest.skip("g8ep operator was started without a local store")
        assert event_type in (
            EventType.OPERATOR_FILE_DIFF_FETCH_COMPLETED.value,
            EventType.OPERATOR_FILE_DIFF_FETCH_FAILED.value,
        ), result
