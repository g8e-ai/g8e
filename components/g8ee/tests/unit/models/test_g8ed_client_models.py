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
Unit tests for g8ed client models.

Tests:
- ChatThinkingPayload construction and field validation
- AISearchWebPayload construction and field validation
- OperatorNetworkPortCheckPayload construction and field validation
"""

import pytest
from pydantic import ValidationError as PydanticValidationError

from app.constants import ToolCallStatus, ThinkingActionType
from app.models.g8ed_client import (
    AISearchWebPayload,
    ChatThinkingPayload,
    OperatorNetworkPortCheckPayload,
)

pytestmark = pytest.mark.unit


class TestChatThinkingPayload:

    def test_construction_with_update_action(self):
        payload = ChatThinkingPayload(thinking="Analyzing the disk usage...", action_type=ThinkingActionType.UPDATE)
        assert payload.thinking == "Analyzing the disk usage..."
        assert payload.action_type == ThinkingActionType.UPDATE

    def test_construction_with_end_action(self):
        payload = ChatThinkingPayload(action_type=ThinkingActionType.END)
        assert payload.action_type == ThinkingActionType.END
        assert payload.thinking is None

    def test_construction_with_start_action(self):
        payload = ChatThinkingPayload(thinking="Starting...", action_type=ThinkingActionType.START)
        assert payload.action_type == ThinkingActionType.START

    def test_thinking_defaults_to_none(self):
        payload = ChatThinkingPayload(action_type=ThinkingActionType.UPDATE)
        assert payload.thinking is None

    def test_flatten_for_wire_includes_action_type(self):
        payload = ChatThinkingPayload(thinking="step 1", action_type=ThinkingActionType.UPDATE)
        wire = payload.flatten_for_wire()
        assert wire["action_type"] == "update"
        assert wire["thinking"] == "step 1"

    def test_action_type_required(self):
        with pytest.raises((PydanticValidationError, TypeError)):
            ChatThinkingPayload()


    def test_wire_value_is_string(self):
        payload = ChatThinkingPayload(action_type=ThinkingActionType.START)
        wire = payload.flatten_for_wire()
        assert wire["action_type"] == "start"
        assert isinstance(wire["action_type"], str)


class TestAISearchWebPayload:

    def test_construction_with_defaults(self):
        payload = AISearchWebPayload()
        assert payload.query is None
        assert payload.execution_id is None
        assert payload.status == ToolCallStatus.STARTED

    def test_construction_with_all_fields(self):
        payload = AISearchWebPayload(
            query="nginx restart",
            execution_id="exec-abc",
            status=ToolCallStatus.STARTED,
        )
        assert payload.query == "nginx restart"
        assert payload.execution_id == "exec-abc"
        assert payload.status == ToolCallStatus.STARTED

    def test_flatten_for_wire_includes_all_fields(self):
        payload = AISearchWebPayload(
            query="disk usage",
            execution_id="exec-xyz",
            status=ToolCallStatus.STARTED,
        )
        wire = payload.flatten_for_wire()
        assert wire["query"] == "disk usage"
        assert wire["execution_id"] == "exec-xyz"
        assert wire["status"] == "started"

    def test_wire_status_is_string(self):
        payload = AISearchWebPayload(status=ToolCallStatus.COMPLETED)
        wire = payload.flatten_for_wire()
        assert wire["status"] == "completed"
        assert isinstance(wire["status"], str)



class TestOperatorNetworkPortCheckPayload:

    def test_construction_with_defaults(self):
        payload = OperatorNetworkPortCheckPayload()
        assert payload.port is None
        assert payload.execution_id is None
        assert payload.status == ToolCallStatus.STARTED

    def test_construction_with_all_fields(self):
        payload = OperatorNetworkPortCheckPayload(
            port="443",
            execution_id="exec-port-001",
            status=ToolCallStatus.STARTED,
        )
        assert payload.port == "443"
        assert payload.execution_id == "exec-port-001"

    def test_flatten_for_wire_includes_all_fields(self):
        payload = OperatorNetworkPortCheckPayload(
            port="8080",
            execution_id="exec-port-xyz",
            status=ToolCallStatus.STARTED,
        )
        wire = payload.flatten_for_wire()
        assert wire["port"] == "8080"
        assert wire["execution_id"] == "exec-port-xyz"
        assert wire["status"] == "started"

    def test_wire_status_is_string(self):
        payload = OperatorNetworkPortCheckPayload(status=ToolCallStatus.COMPLETED)
        wire = payload.flatten_for_wire()
        assert wire["status"] == "completed"
        assert isinstance(wire["status"], str)



