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

"""Integration test for g8ee-g8eo UAP JSON envelope flow."""

import json
from unittest.mock import AsyncMock, MagicMock

import pytest

from app.constants import EventType, ExecutionStatus, PubSubChannel
from app.services.operator.pubsub_service import OperatorPubSubService

pytestmark = [pytest.mark.unit, pytest.mark.asyncio(loop_scope="session")]

@pytest.fixture
def pubsub_service():
    svc = OperatorPubSubService()
    client = MagicMock()
    client.ensure_connected = AsyncMock()
    client.subscribe = AsyncMock()
    client.on_channel_message = MagicMock()
    svc.set_pubsub_client(client)
    return svc

async def test_uap_envelope_flow_integration(pubsub_service):
    """Verify full flow from UAP JSON GovernanceEnvelope bytes to completed Future with converted enums."""
    operator_id = "op-test-1"
    operator_session_id = "sess-test-1"
    execution_id = "exec-test-1"

    # 1. Register a future for the execution
    future = pubsub_service.register_future(execution_id)

    # 2. Simulate g8eo publishing a Protobuf GovernanceEnvelope JSON format
    envelope_data = {
        "id": execution_id,
        "event_type": EventType.OPERATOR_COMMAND_RESULT,
        "action_type": "EXECUTE_BASH_RESULT",
        "operator_id": operator_id,
        "operator_session_id": operator_session_id,
        "case_id": "case-test-1",
        "intent_data": {
            "payload_type": "execution_result",
            "execution_id": execution_id,
            "status": ExecutionStatus.COMPLETED.value,
            "stdout": "hello from g8eo",
            "return_code": 0
        }
    }

    raw_bytes = json.dumps(envelope_data).encode("utf-8")

    # 3. Dispatch the message through the pubsub service
    channel = PubSubChannel.results(operator_id, operator_session_id)
    await pubsub_service._dispatch_results_message(channel, raw_bytes)

    # 4. Verify the future was completed with correct data and converted enums
    assert future.done()
    result_envelope = future.result()

    # Verify envelope fields
    assert result_envelope.operator_id == operator_id
    assert result_envelope.operator_session_id == operator_session_id
    assert result_envelope.event_type == EventType.OPERATOR_COMMAND_RESULT

    # Verify payload fields and enum conversion
    payload = result_envelope.payload
    assert payload.execution_id == execution_id
    assert payload.status == ExecutionStatus.COMPLETED
    assert payload.stdout == "hello from g8eo"
    assert payload.return_code == 0

async def test_uap_envelope_flow_status_update(pubsub_service):
    """Verify flow for EXECUTE_STATUS_UPDATE Protobuf GovernanceEnvelope messages."""
    operator_id = "op-test-2"
    operator_session_id = "sess-test-2"
    execution_id = "exec-test-2"

    future = pubsub_service.register_future(execution_id)

    # Build status update Protobuf GovernanceEnvelope
    envelope_data = {
        "id": "msg-status-1",
        "event_type": EventType.OPERATOR_COMMAND_STATUS_UPDATED,
        "action_type": "EXECUTE_STATUS_UPDATE",
        "operator_id": operator_id,
        "operator_session_id": operator_session_id,
        "intent_data": {
            "payload_type": "execution_status",
            "execution_id": execution_id,
            "status": ExecutionStatus.EXECUTING.value,
            "process_alive": True,
            "elapsed_seconds": 10.5
        }
    }

    raw_bytes = json.dumps(envelope_data).encode("utf-8")

    channel = PubSubChannel.results(operator_id, operator_session_id)
    await pubsub_service._dispatch_results_message(channel, raw_bytes)

    assert future.done()
    result_envelope = future.result()

    assert result_envelope.event_type == EventType.OPERATOR_COMMAND_STATUS_UPDATED

    payload = result_envelope.payload
    assert payload.payload_type == "execution_status"
    assert payload.status == ExecutionStatus.EXECUTING
    assert payload.process_alive is True
    assert payload.elapsed_seconds == 10.5
