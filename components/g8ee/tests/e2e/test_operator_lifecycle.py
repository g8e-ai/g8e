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

"""E2E tests for the operator lifecycle golden path.

These tests run the real compiled operator binary against the real platform
stack (g8ed, g8ee, g8es) with real PubSub event delivery.
"""

from __future__ import annotations

import asyncio
import json
import logging
import signal

import pytest

from app.constants.channels import PubSubChannel
from app.constants.events import EventType
from app.models.pubsub_messages import G8eMessage, G8eoResultEnvelope
from app.constants import ComponentName

logger = logging.getLogger(__name__)

pytestmark = [pytest.mark.e2e, pytest.mark.asyncio(loop_scope="session")]


class TestOperatorLifecycle:
    """Golden-path: provision -> launch -> heartbeat -> command -> result -> teardown."""

    async def test_operator_starts_and_sends_heartbeat(self, operator_sandbox):
        """Operator binary authenticates via g8ed and sends its first heartbeat."""
        heartbeat = await operator_sandbox.wait_for_heartbeat(timeout=30.0)

        assert operator_sandbox.is_running, (
            f"Operator process exited prematurely\n{operator_sandbox.dump_logs()}"
        )
        assert heartbeat, "Heartbeat payload was empty"
        assert "system_identity" in heartbeat or "system_info" in heartbeat, (
            f"Heartbeat missing system identity: {list(heartbeat.keys())}"
        )

    async def test_operator_reaches_active_status(self, operator_sandbox, e2e_http):
        """After heartbeat, the operator slot transitions to ACTIVE in g8ed."""
        await operator_sandbox.wait_for_heartbeat(timeout=30.0)

        status_doc = await e2e_http.get_operator_status(operator_sandbox.operator_id)

        assert status_doc["status"] in ("active", "bound"), (
            f"Expected operator status active|bound, got: {status_doc['status']}"
        )

    async def test_operator_executes_command_via_pubsub(
        self,
        operator_sandbox,
        e2e_services,
        e2e_pubsub_client,
    ):
        """Publish a command via PubSub and receive the result event-driven."""
        heartbeat = await operator_sandbox.wait_for_heartbeat(timeout=30.0)

        # Extract operator_session_id from heartbeat data
        operator_id = operator_sandbox.operator_id
        operator_session_id = heartbeat.get("operator_session_id")
        assert operator_session_id, (
            f"Heartbeat missing operator_session_id: {list(heartbeat.keys())}"
        )
        operator_sandbox.operator_session_id = operator_session_id

        # Subscribe to the results channel for this session
        result_event = asyncio.Event()
        result_data: dict = {}

        async def _on_result(channel: str, data: str | dict) -> None:
            nonlocal result_data
            raw = data if isinstance(data, dict) else json.loads(data)
            result_data.update(raw)
            result_event.set()

        results_channel = PubSubChannel.results(operator_id, operator_session_id)
        e2e_pubsub_client.on_channel_message(results_channel, _on_result)
        await e2e_pubsub_client.subscribe(results_channel)

        try:
            cmd_channel = PubSubChannel.cmd(operator_id, operator_session_id)
            
            # Operator expects PubSubCommandMessage format (g8eo structure), not G8eMessage
            command_payload = {
                "command": "echo e2e-hello",
                "justification": "E2E test command",
            }
            
            command_msg = {
                "id": "e2e-test-msg-001",
                "event_type": EventType.OPERATOR_COMMAND_REQUESTED,
                "case_id": "e2e-test-case",
                "task_id": "e2e-test-task",
                "investigation_id": "e2e-test-investigation",
                "operator_session_id": operator_session_id,
                "operator_id": operator_id,
                "payload": command_payload,
                "timestamp": "2026-04-12T17:47:00Z",
            }
            
            receivers = await e2e_pubsub_client.publish(
                cmd_channel, command_msg
            )
            assert receivers > 0, (
                f"No subscribers on command channel {cmd_channel}"
            )

            await asyncio.wait_for(result_event.wait(), timeout=15.0)

            assert result_data, "Result payload was empty"
            logger.info(
                "[E2E] Command result received: %s",
                {k: v for k, v in result_data.items() if k != "payload"},
            )

        finally:
            e2e_pubsub_client.off_channel_message(results_channel, _on_result)
            await e2e_pubsub_client.unsubscribe(results_channel)

    async def test_operator_graceful_shutdown(self, operator_sandbox):
        """Verify operator shuts down cleanly on SIGTERM (dogfooding anchor test)."""
        await operator_sandbox.wait_for_heartbeat(timeout=30.0)

        # The fixture teardown will send SIGTERM, but we test it explicitly here
        # to ensure the operator handles shutdown correctly as the final test
        assert operator_sandbox.is_running, "Operator should be running before shutdown"

        # Send SIGTERM
        operator_sandbox.process.send_signal(signal.SIGTERM)

        # Wait for process to exit gracefully
        try:
            await asyncio.wait_for(operator_sandbox.process.wait(), timeout=5.0)
        except asyncio.TimeoutError:
            # Force kill if graceful shutdown fails
            operator_sandbox.process.kill()
            await operator_sandbox.process.wait()
            raise AssertionError("Operator did not shut down gracefully within 5s")

        assert not operator_sandbox.is_running, "Operator should not be running after shutdown"

        # Verify logs show clean shutdown
        logs = operator_sandbox.dump_logs()
        assert "g8e Operator shutting down" in logs or "shutting down" in logs.lower(), (
            "Operator logs should show shutdown message"
        )
