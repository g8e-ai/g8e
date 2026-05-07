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

"""Integration test for g8ee-g8eo protobuf envelope flow."""

import asyncio
from unittest.mock import AsyncMock, MagicMock

import pytest

from app.constants import EventType, ExecutionStatus, PubSubChannel
from app.proto import common_pb2, operator_pb2
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

async def test_protobuf_envelope_flow_integration(pubsub_service):
    """Verify full flow from raw protobuf bytes to completed Future with converted enums."""
    operator_id = "op-test-1"
    session_id = "sess-test-1"
    execution_id = "exec-test-1"
    
    # 1. Register a future for the execution
    future = pubsub_service.register_future(execution_id)
    
    # 3. Simulate g8eo publishing a protobuf UniversalEnvelope
    # Build the payload (CommandResult)
    command_result = operator_pb2.CommandResult()
    command_result.execution_id = execution_id
    command_result.status = operator_pb2.EXECUTION_STATUS_COMPLETED
    command_result.output = "hello from g8eo"
    command_result.exit_code = 0
    
    # Build the envelope
    envelope = common_pb2.UniversalEnvelope()
    envelope.id = execution_id
    envelope.event_type = EventType.OPERATOR_COMMAND_COMPLETED
    envelope.operator_id = operator_id
    envelope.operator_session_id = session_id
    envelope.payload = command_result.SerializeToString()
    
    raw_bytes = envelope.SerializeToString()
    
    # 3. Dispatch the message through the pubsub service
    channel = PubSubChannel.results(operator_id, session_id)
    await pubsub_service._dispatch_results_message(channel, raw_bytes)
    
    # 4. Verify the future was completed with correct data and converted enums
    assert future.done()
    result_envelope = future.result()
    
    # Verify envelope fields
    assert result_envelope.operator_id == operator_id
    assert result_envelope.operator_session_id == session_id
    assert result_envelope.event_type == EventType.OPERATOR_COMMAND_COMPLETED
    
    # Verify payload fields and enum conversion
    payload = result_envelope.payload
    assert payload.execution_id == execution_id
    assert payload.status == ExecutionStatus.COMPLETED # Protobuf enum 2 -> "completed"
    assert payload.stdout == "hello from g8eo"
    assert payload.return_code == 0

async def test_protobuf_envelope_flow_status_update(pubsub_service):
    """Verify flow for ExecutionStatusUpdate protobuf messages."""
    operator_id = "op-test-2"
    session_id = "sess-test-2"
    execution_id = "exec-test-2"
    
    future = pubsub_service.register_future(execution_id)
    
    # Build status update payload
    status_update = operator_pb2.ExecutionStatusUpdate()
    status_update.execution_id = execution_id
    status_update.status = operator_pb2.EXECUTION_STATUS_EXECUTING
    status_update.process_alive = True
    status_update.elapsed_seconds = 10.5
    
    envelope = common_pb2.UniversalEnvelope()
    envelope.id = execution_id
    envelope.event_type = EventType.OPERATOR_COMMAND_STATUS_UPDATED_RUNNING
    envelope.operator_id = operator_id
    envelope.operator_session_id = session_id
    envelope.payload = status_update.SerializeToString()
    
    raw_bytes = envelope.SerializeToString()
    
    channel = PubSubChannel.results(operator_id, session_id)
    await pubsub_service._dispatch_results_message(channel, raw_bytes)
    
    assert future.done()
    result_envelope = future.result()
    
    payload = result_envelope.payload
    assert payload.payload_type == "execution_status"
    assert payload.status == ExecutionStatus.EXECUTING # Protobuf enum 1 -> "executing"
    assert payload.process_alive is True
    assert payload.elapsed_seconds == 10.5
