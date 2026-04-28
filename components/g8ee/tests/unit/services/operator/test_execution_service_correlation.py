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

"""Regression tests for OperatorExecutionService Future correlation.

The primary command-execution path correlates inbound g8eo results to the
awaiting Future via the per-message execution_id carried on
g8e_message.payload.execution_id. It must NOT be keyed on
g8e_context.execution_id (which is the HTTP request id and has a different
lifetime). See the docstring on G8eoResultEnvelope.id.
"""

from __future__ import annotations

import asyncio

import pytest

from app.constants.events import EventType
from app.constants.status import AITaskId, ComponentName, ExecutionStatus
from app.models.command_request_payloads import CommandRequestPayload
from app.models.pubsub_messages import G8eMessage
from app.services.operator.execution_service import OperatorExecutionService
from app.services.operator.pubsub_service import OperatorPubSubService
from tests.fakes.factories import build_g8e_http_context
from tests.fakes.fake_g8es_clients import FakePubSubClient

pytestmark = [pytest.mark.unit, pytest.mark.asyncio(loop_scope="session")]


def _build_execution_service() -> tuple[OperatorExecutionService, OperatorPubSubService, FakePubSubClient]:
    """Wire a real OperatorExecutionService atop a real OperatorPubSubService + fake client."""
    pubsub_service = OperatorPubSubService()
    pubsub_client = FakePubSubClient()
    pubsub_service.set_pubsub_client(pubsub_client)
    # Minimal collaborators not exercised by dispatch_command itself.
    svc = OperatorExecutionService.__new__(OperatorExecutionService)
    svc._pubsub_service = pubsub_service
    svc._approval_service = None
    svc._g8ed_event_service = None
    svc._settings = None
    svc._operator_data_service = None
    svc._ai_response_analyzer = None
    svc._investigation_service = None
    return svc, pubsub_service, pubsub_client


def _build_command_message(exec_id: str, *, op_id: str = "op-1", sess_id: str = "sess-1") -> G8eMessage:
    return G8eMessage(
        id=exec_id,
        source_component=ComponentName.G8EE,
        event_type=EventType.OPERATOR_COMMAND_REQUESTED,
        case_id="case-1",
        task_id=AITaskId.COMMAND,
        investigation_id="inv-1",
        web_session_id="web-1",
        operator_id=op_id,
        operator_session_id=sess_id,
        payload=CommandRequestPayload(command="echo hi", execution_id=exec_id),
    )


class TestDispatchCommandCorrelation:
    """dispatch_command must correlate results via payload.execution_id."""

    async def test_completes_when_payload_execution_id_differs_from_http_request_id(self):
        """Regression: the Future must be keyed on payload.execution_id, not g8e_context.execution_id.

        Before the fix, dispatch_command registered the Future under
        g8e_context.execution_id while the pubsub dispatcher completed Futures
        under payload.execution_id. Every command timed out.
        """
        svc, pubsub_service, _client = _build_execution_service()
        await pubsub_service.start()

        per_message_exec_id = "per-msg-exec-id"
        g8e_context = build_g8e_http_context()
        assert g8e_context.execution_id != per_message_exec_id, (
            "sanity: the HTTP request id must differ from the per-message id for this test"
        )
        msg = _build_command_message(per_message_exec_id)

        dispatch_task = asyncio.create_task(
            svc.dispatch_command(msg, g8e_context, timeout_seconds=5)
        )
        # Yield so dispatch_command registers its Future and publishes.
        await asyncio.sleep(0)
        await asyncio.sleep(0)

        await pubsub_service._handle_pubsub_result_message(
            "op-1",
            "sess-1",
            {
                "event_type": EventType.OPERATOR_COMMAND_COMPLETED,
                "payload": {
                    "payload_type": "execution_result",
                    "execution_id": per_message_exec_id,
                    "status": ExecutionStatus.COMPLETED,
                    "stdout": "hi",
                    "return_code": 0,
                },
            },
        )

        internal_result, envelope = await asyncio.wait_for(dispatch_task, timeout=5)
        assert internal_result.status == ExecutionStatus.COMPLETED
        assert internal_result.execution_id == per_message_exec_id
        assert envelope is not None
        assert envelope.payload.execution_id == per_message_exec_id

    async def test_times_out_cleanly_when_no_result_arrives(self):
        """Sanity: with no matching result, dispatch_command returns TIMEOUT (not hangs)."""
        svc, pubsub_service, _client = _build_execution_service()
        await pubsub_service.start()

        msg = _build_command_message("lonely-exec-id")
        g8e_context = build_g8e_http_context()

        internal_result, envelope = await svc.dispatch_command(
            msg, g8e_context, timeout_seconds=0.1
        )
        assert internal_result.status == ExecutionStatus.TIMEOUT
        assert internal_result.execution_id == "lonely-exec-id"
        assert envelope is None
        assert "lonely-exec-id" not in pubsub_service._pending_futures
