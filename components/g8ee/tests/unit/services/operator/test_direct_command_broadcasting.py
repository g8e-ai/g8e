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

import asyncio
import pytest
from unittest.mock import MagicMock, AsyncMock

from app.constants.events import EventType
from app.constants.status import AITaskId, ExecutionStatus
from app.models.internal_api import DirectCommandRequest
from app.models.operators import (
    CommandResultBroadcastEvent,
    DirectCommandResult,
    OperatorDocument,
)
from app.services.operator.execution_service import OperatorExecutionService
from app.services.operator.pubsub_service import OperatorPubSubService
from tests.fakes.factories import build_g8e_http_context, build_bound_operator
from tests.fakes.fake_g8es_clients import FakePubSubClient

pytestmark = [pytest.mark.unit, pytest.mark.asyncio(loop_scope="session")]

def _build_execution_service() -> tuple[OperatorExecutionService, OperatorPubSubService, FakePubSubClient, MagicMock]:
    """Wire an OperatorExecutionService with a fake pubsub and mocked event service."""
    pubsub_service = OperatorPubSubService()
    pubsub_client = FakePubSubClient()
    pubsub_service.set_pubsub_client(pubsub_client)
    
    event_service = MagicMock()
    event_service.publish_command_event = AsyncMock()
    
    # We use __new__ to avoid full __init__ complexity
    svc = OperatorExecutionService.__new__(OperatorExecutionService)
    svc._pubsub_service = pubsub_service
    svc._g8ed_event_service = event_service
    svc._approval_service = None
    svc._settings = None
    svc._operator_data_service = None
    svc._ai_response_analyzer = None
    svc._investigation_service = None
    
    return svc, pubsub_service, pubsub_client, event_service

class TestDirectCommandBroadcasting:
    async def test_send_command_to_operator_broadcasts_result_in_background(self):
        """Test that send_command_to_operator launches a task that broadcasts the result."""
        svc, pubsub_service, _client, event_service = _build_execution_service()
        await pubsub_service.start()

        exec_id = "direct-exec-1"
        command = "ls -la"
        
        # Setup context with bound operator
        bound_op = build_bound_operator(operator_id="op-1", operator_session_id="sess-1")
        g8e_context = build_g8e_http_context()
        g8e_context.bound_operators = [bound_op]
        
        request = DirectCommandRequest(
            command=command,
            execution_id=exec_id,
            hostname="test-host"
        )

        # 1. Call send_command_to_operator
        result = await svc.send_command_to_operator(request, g8e_context)
        
        assert isinstance(result, DirectCommandResult)
        assert result.status == ExecutionStatus.EXECUTING
        assert result.execution_id == exec_id
        
        # Verify Future is registered
        assert exec_id in pubsub_service._pending_futures
        
        # 2. Simulate inbound result message from operator
        await pubsub_service._handle_pubsub_result_message(
            "op-1",
            "sess-1",
            {
                "event_type": EventType.OPERATOR_COMMAND_COMPLETED,
                "payload": {
                    "payload_type": "execution_result",
                    "execution_id": exec_id,
                    "status": ExecutionStatus.COMPLETED,
                    "stdout": "file1\nfile2",
                    "return_code": 0,
                    "duration_seconds": 1.5,
                },
            },
        )
        
        # 3. Wait for the background task to complete and publish the event
        # We need to yield to the event loop multiple times to allow the background task to progress
        for _ in range(5):
            await asyncio.sleep(0)
            
        # 4. Verify the event was published to g8ed
        event_service.publish_command_event.assert_called_once()
        args, kwargs = event_service.publish_command_event.call_args
        
        event_type = args[0]
        event_data = args[1]
        ctx = args[2]
        task_id_kwarg = kwargs.get("task_id")
        
        assert event_type == EventType.OPERATOR_COMMAND_COMPLETED
        assert isinstance(event_data, CommandResultBroadcastEvent)
        assert event_data.execution_id == exec_id
        assert event_data.command == command
        assert event_data.output == "file1\nfile2"
        assert event_data.status == ExecutionStatus.COMPLETED
        assert event_data.direct_execution is True
        assert event_data.hostname == "test-host"
        assert ctx == g8e_context
        assert task_id_kwarg == AITaskId.DIRECT_COMMAND
        
        # Verify Future was released
        assert exec_id not in pubsub_service._pending_futures

    async def test_send_command_to_operator_broadcasts_failure_result(self):
        """Test that send_command_to_operator broadcasts failure results."""
        svc, pubsub_service, _client, event_service = _build_execution_service()
        await pubsub_service.start()

        exec_id = "direct-exec-fail"
        command = "invalid-cmd"
        
        bound_op = build_bound_operator(operator_id="op-1", operator_session_id="sess-1")
        g8e_context = build_g8e_http_context()
        g8e_context.bound_operators = [bound_op]
        
        request = DirectCommandRequest(
            command=command,
            execution_id=exec_id
        )

        await svc.send_command_to_operator(request, g8e_context)
        
        # Simulate failure result
        await pubsub_service._handle_pubsub_result_message(
            "op-1",
            "sess-1",
            {
                "event_type": EventType.OPERATOR_COMMAND_FAILED,
                "payload": {
                    "payload_type": "execution_result",
                    "execution_id": exec_id,
                    "status": ExecutionStatus.FAILED,
                    "stderr": "command not found",
                    "return_code": 127,
                    "error_message": "Execution failed",
                },
            },
        )
        
        for _ in range(5):
            await asyncio.sleep(0)
            
        event_service.publish_command_event.assert_called_once()
        args, _ = event_service.publish_command_event.call_args
        
        event_type = args[0]
        event_data = args[1]
        
        assert event_type == EventType.OPERATOR_COMMAND_FAILED
        assert event_data.status == ExecutionStatus.FAILED
        assert event_data.stderr == "command not found"
        assert event_data.error == "Execution failed"
        assert event_data.direct_execution is True
